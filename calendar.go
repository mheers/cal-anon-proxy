package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

func currentUsername(ctx context.Context) (string, error) {
	if v, ok := ctx.Value(CtxKey{}).(CtxValue); ok {
		return v.Username, nil
	}
	return "", errors.New("not authenticated")
}

type calendarBackend struct {
	prefix string

	calendars []caldav.Calendar
	objectMap map[string][]caldav.CalendarObject
}

func (b *calendarBackend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) error {
	return nil
}

func (b *calendarBackend) Calendar(ctx context.Context) (*caldav.Calendar, error) {
	if len(b.calendars) == 0 {
		return nil, fmt.Errorf("no calendars available")
	}
	return &b.calendars[0], nil
}

func (b *calendarBackend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	return b.calendars, nil
}

func (b *calendarBackend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	for _, cal := range b.calendars {
		if cal.Path == path {
			return &cal, nil
		}
	}
	return nil, fmt.Errorf("calendar for path: %s not found", path)
}

func (b *calendarBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	username, err := currentUsername(ctx)
	return fmt.Sprintf("/%s/%s/calendars/", b.prefix, username), err
}

// must begin and end with a slash
func (b *calendarBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	username, err := currentUsername(ctx)
	return "/" + url.PathEscape(username) + "/", err
}

func (b *calendarBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	return nil
}

func (b *calendarBackend) GetCalendarObject(ctx context.Context, path string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	for _, objs := range b.objectMap {
		for _, obj := range objs {
			if obj.Path == path {
				return &obj, nil
			}
		}
	}
	return nil, fmt.Errorf("couldn't find calendar object at: %s", path)
}

func (b *calendarBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	return nil, nil
}

func (b *calendarBackend) ListCalendarObjects(ctx context.Context, path string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	return b.objectMap[path], nil
}

func (b *calendarBackend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	objects, ok := b.objectMap[path]
	if !ok {
		return nil, nil
	}
	if query == nil {
		return objects, nil
	}
	return caldav.Filter(query, objects)
}

type CalDavHandler struct {
	mu     sync.RWMutex
	path   string
	events []*caldav.CalendarObject
	*caldav.Handler
}

func (h *CalDavHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	handler := h.Handler
	h.mu.RUnlock()
	handler.ServeHTTP(w, r)
}

func (h *CalDavHandler) HTTPHandler() http.Handler {
	return h
}

func (h *CalDavHandler) SetEvents(events []*caldav.CalendarObject) {
	sessionsCal := caldav.Calendar{
		Path:                  h.path,
		SupportedComponentSet: []string{ical.CompEvent},
	}
	calendars := []caldav.Calendar{sessionsCal}

	// Build individual objects per event (OUTSIDE lock)
	objects := make([]caldav.CalendarObject, 0, len(events))
	for i, event := range events {
		objPath := event.Path
		if objPath == "" {
			// Try UID from first VEVENT child
			uid := ""
			if len(event.Data.Children) > 0 {
				uidProp := event.Data.Children[0].Props.Get(ical.PropUID)
				if uidProp != nil {
					uid = uidProp.Value
				}
			}
			if uid != "" {
				objPath = fmt.Sprintf("%s%s.ics", h.path, uid)
			} else {
				objPath = fmt.Sprintf("%s%d.ics", h.path, i)
			}
		}
		objects = append(objects, caldav.CalendarObject{
			Path: objPath,
			Data: event.Data,
		})
	}

	newBackend := &calendarBackend{
		calendars: calendars,
		objectMap: map[string][]caldav.CalendarObject{
			sessionsCal.Path: objects,
		},
	}

	h.mu.Lock()
	h.Handler.Backend = newBackend
	h.events = events
	h.mu.Unlock()
}

// ServeICS writes all stored events as a single merged iCalendar (.ics) file.
func (h *CalDavHandler) ServeICS(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	events := h.events
	h.mu.RUnlock()

	cal := ical.NewCalendar()
	cal.Props.SetText("VERSION", "2.0")
	cal.Props.SetText("PRODID", "-//cal-anon-proxy//EN")

	for _, obj := range events {
		for _, child := range obj.Data.Children {
			if child.Name == ical.CompEvent {
				cal.Children = append(cal.Children, child)
			}
		}
	}

	w.Header().Set("Content-Type", ical.MIMEType)
	if err := ical.NewEncoder(w).Encode(cal); err != nil {
		http.Error(w, "failed to encode calendar: "+err.Error(), http.StatusInternalServerError)
	}
}

type jsonEvent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// ServeEventsJSON serves a FullCalendar-compatible JSON event feed.
// It expands RRULE recurrences within the requested window and applies
// RECURRENCE-ID overrides. Times are returned as naive ISO strings in
// the timezone requested by FullCalendar's ?timeZone= query parameter,
// which is required for FullCalendar's timezone display to work correctly.
func (h *CalDavHandler) ServeEventsJSON(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	events := h.events
	h.mu.RUnlock()

	// FullCalendar sends naive start/end in the requested timezone, plus ?timeZone=
	q := r.URL.Query()

	// Determine display timezone.
	// "local" is a special FullCalendar value meaning "use the browser's local timezone".
	// In that case we return UTC Z-strings so FullCalendar converts them to the browser's
	// local timezone itself. For any named IANA timezone we return naive ISO strings.
	tzName := q.Get("timeZone")
	isLocal := tzName == "" || tzName == "local"
	displayTZ := time.UTC
	if !isLocal {
		if loc, err := time.LoadLocation(tzName); err == nil {
			displayTZ = loc
		}
	}

	// formatTime formats a UTC time for the JSON feed.
	// For local mode: return a UTC Z-string so FullCalendar applies the browser timezone.
	// For named timezones: return a naive ISO string (no Z) in the display timezone.
	formatTime := func(t time.Time) string {
		if isLocal {
			return t.UTC().Format("2006-01-02T15:04:05") + "Z"
		}
		return t.In(displayTZ).Format("2006-01-02T15:04:05")
	}

	// Parse window bounds — FullCalendar sends naive times in the display timezone,
	// or UTC-anchored times for local mode.
	windowStart, windowEnd := time.Time{}, time.Time{}
	parseNaive := func(s string) time.Time {
		t, err := time.ParseInLocation("2006-01-02T15:04:05", s, displayTZ)
		if err != nil {
			// fallback: try RFC3339 with offset
			t, _ = time.Parse(time.RFC3339, s)
		}
		return t.UTC()
	}
	if s := q.Get("start"); s != "" {
		windowStart = parseNaive(s)
	}
	if e := q.Get("end"); e != "" {
		windowEnd = parseNaive(e)
	}

	// Group VEVENTs by UID: collect the base event and overrides separately.
	type uidGroup struct {
		base      *ical.Component            // the VEVENT without RECURRENCE-ID
		overrides map[string]*ical.Component // keyed by RECURRENCE-ID value
	}
	groups := map[string]*uidGroup{}

	for _, obj := range events {
		for _, child := range obj.Data.Children {
			if child.Name != ical.CompEvent {
				continue
			}
			uid := child.Props.Get(ical.PropUID)
			if uid == nil {
				continue
			}
			key := uid.Value
			if groups[key] == nil {
				groups[key] = &uidGroup{overrides: map[string]*ical.Component{}}
			}
			if child.Props.Get(ical.PropRecurrenceID) != nil {
				recID := child.Props.Get(ical.PropRecurrenceID).Value
				groups[key].overrides[recID] = child
			} else {
				groups[key].base = child
			}
		}
	}

	out := []jsonEvent{}

	for uid, g := range groups {
		if g.base == nil {
			continue
		}

		dtstart := g.base.Props.Get(ical.PropDateTimeStart)
		if dtstart == nil {
			continue
		}
		duration := time.Duration(0)
		if dtend := g.base.Props.Get(ical.PropDateTimeEnd); dtend != nil {
			startT, err1 := dtstart.DateTime(time.UTC)
			endT, err2 := dtend.DateTime(time.UTC)
			if err1 == nil && err2 == nil {
				duration = endT.Sub(startT)
			}
		}

		summary := ""
		if s := g.base.Props.Get(ical.PropSummary); s != nil {
			summary = s.Value
		}

		// Collect all occurrence times from the RRULE (or just the single DTSTART).
		rset, err := g.base.RecurrenceSet(time.UTC)
		if err != nil || rset == nil {
			// No recurrence — single event
			startT, err := dtstart.DateTime(time.UTC)
			if err != nil {
				continue
			}
			endT := startT.Add(duration)
			if !windowStart.IsZero() && endT.Before(windowStart) {
				continue
			}
			if !windowEnd.IsZero() && startT.After(windowEnd) {
				continue
			}
			out = append(out, jsonEvent{
				ID:    uid,
				Title: summary,
				Start: formatTime(startT),
				End:   formatTime(endT),
			})
			continue
		}

		// Expand occurrences within the window (with a generous buffer).
		queryStart := time.Now().Add(-365 * 24 * time.Hour)
		queryEnd := time.Now().Add(365 * 24 * time.Hour)
		if !windowStart.IsZero() {
			queryStart = windowStart.Add(-24 * time.Hour)
		}
		if !windowEnd.IsZero() {
			queryEnd = windowEnd.Add(24 * time.Hour)
		}
		occurrences := rset.Between(queryStart, queryEnd, true)

		for _, occ := range occurrences {
			endT := occ.Add(duration)
			occSummary := summary

			// Check if this occurrence has a RECURRENCE-ID override.
			occKey := occ.UTC().Format("20060102T150405Z")
			if override, ok := g.overrides[occKey]; ok {
				if overrideStart := override.Props.Get(ical.PropDateTimeStart); overrideStart != nil {
					if st, err := overrideStart.DateTime(time.UTC); err == nil {
						occ = st
					}
				}
				if overrideEnd := override.Props.Get(ical.PropDateTimeEnd); overrideEnd != nil {
					if et, err := overrideEnd.DateTime(time.UTC); err == nil {
						endT = et
					} else {
						endT = occ.Add(duration)
					}
				} else {
					endT = occ.Add(duration)
				}
				if overrideSummary := override.Props.Get(ical.PropSummary); overrideSummary != nil {
					occSummary = overrideSummary.Value
				}
			}

			if !windowStart.IsZero() && endT.Before(windowStart) {
				continue
			}
			if !windowEnd.IsZero() && occ.After(windowEnd) {
				continue
			}

			out = append(out, jsonEvent{
				ID:    fmt.Sprintf("%s-%s", uid, occ.UTC().Format("20060102T150405Z")),
				Title: occSummary,
				Start: formatTime(occ),
				End:   formatTime(endT),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func NewCalDavHandler(path string) *CalDavHandler {
	return &CalDavHandler{
		Handler: &caldav.Handler{
			Prefix: path,
		},
		path: path,
	}
}
