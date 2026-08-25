package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/sirupsen/logrus"
)

func (p *CalProxy) downloadAll() ([]*caldav.CalendarObject, error) {
	events := []*caldav.CalendarObject{}
	for _, src := range p.config.Srcs() {
		srcEvents, err := p.download(src)
		if err != nil {
			return nil, err
		}
		events = append(events, srcEvents...)
	}
	p.filterWindow(events)
	return events, nil
}

// filterWindow removes VEVENTs that lie entirely outside the configured
// visibility window (WINDOW_PAST_WEEKS .. WINDOW_FUTURE_WEEKS, defaults 4/8).
// Applied to all source paths: ICS feeds otherwise carry their full history,
// and the CalDAV window alone cannot express "past weeks". Recurring events
// (RRULE) are always kept — their DTSTART may be far in the past while
// current occurrences still fall inside the window. Unparsable events are
// kept rather than silently dropped.
func (p *CalProxy) filterWindow(events []*caldav.CalendarObject) {
	windowStart := time.Now().AddDate(0, 0, -7*p.config.WindowPastWeeks)
	windowEnd := time.Now().AddDate(0, 0, 7*p.config.WindowFutureWeeks)

	for _, obj := range events {
		kept := make([]*ical.Component, 0, len(obj.Data.Children))
		for _, child := range obj.Data.Children {
			if child.Name != ical.CompEvent {
				kept = append(kept, child)
				continue
			}
			if child.Props.Get(ical.PropRecurrenceRule) != nil {
				kept = append(kept, child)
				continue
			}
			start, err := child.Props.DateTime(ical.PropDateTimeStart, time.UTC)
			if err != nil {
				kept = append(kept, child)
				continue
			}
			end, err := child.Props.DateTime(ical.PropDateTimeEnd, time.UTC)
			if err != nil {
				end = start
			}
			if end.Before(windowStart) || start.After(windowEnd) {
				continue
			}
			kept = append(kept, child)
		}
		obj.Data.Children = kept
	}
}

func (p *CalProxy) download(src *Src) ([]*caldav.CalendarObject, error) {
	normalizedURL, useICS, err := normalizeSourceURL(src.URL)
	if err != nil {
		return nil, err
	}

	if useICS {
		return p.downloadICS(src, normalizedURL)
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
	caldavClient, err := caldav.NewClient(webdav.HTTPClientWithBasicAuth(httpClient, src.Username, src.Password), normalizedURL)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	homeset := ""
	calendars, err := caldavClient.FindCalendars(ctx, homeset)
	if err != nil {
		return nil, err
	}

	for _, calendar := range calendars {
		logrus.Debugf("Calendar: %s", calendar.Name)
	}

	if len(calendars) == 0 {
		return nil, fmt.Errorf("no calendars found for source %s", normalizedURL)
	}
	calendar := calendars[0]

	// Server-side query window, anchored at "now" and driven by the same
	// WINDOW_PAST_WEEKS / WINDOW_FUTURE_WEEKS settings as filterWindow
	// (which re-applies the window client-side after processing).
	queryStart := time.Now().AddDate(0, 0, -7*p.config.WindowPastWeeks)
	queryEnd := time.Now().AddDate(0, 0, 7*p.config.WindowFutureWeeks)

	// print start date
	logrus.Debugf("Looking for events from %s to %s", queryStart.Format(time.RFC3339), queryEnd.Format(time.RFC3339))

	queryResult, err := caldavClient.QueryCalendar(ctx, calendar.Path, &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name: "VCALENDAR",
			Comps: []caldav.CalendarCompRequest{{
				Name: "VEVENT",
				Props: []string{
					"SUMMARY",
					"UID",
					"DTSTART",
					"DTEND",
					"DURATION",
					"RRULE",
					"EXDATE",
					"RECURRENCE-ID",
				},
			}},
		},
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{{
				Name:  "VEVENT",
				Start: queryStart,
				End:   queryEnd,
			}},
		},
	})
	if err != nil {
		return nil, err
	}

	calEvents := make([]*caldav.CalendarObject, 0, len(queryResult))
	for i := range queryResult {
		calEvents = append(calEvents, &queryResult[i])
	}

	return processEvents(calEvents, src)
}

func (p *CalProxy) downloadICS(src *Src, sourceURL string) ([]*caldav.CalendarObject, error) {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	if src.Username != "" || src.Password != "" {
		req.SetBasicAuth(src.Username, src.Password)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		hint := ""
		if isGoogleCalendarURL(sourceURL) && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
			hint = " (for Google Calendar on servers, use the Secret address in iCal format as SRC_*_URL; Google username/password basic auth is not supported)"
		}
		return nil, fmt.Errorf("failed to fetch %s: status %d: %s%s", sourceURL, resp.StatusCode, strings.TrimSpace(string(body)), hint)
	}

	cal, err := ical.NewDecoder(resp.Body).Decode()
	if err != nil {
		return nil, err
	}

	events := []*caldav.CalendarObject{{
		Path: sourceURL,
		Data: cal,
	}}

	return processEvents(events, src)
}

func normalizeSourceURL(rawURL string) (string, bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false, err
	}

	isGoogleCalendar := strings.EqualFold(u.Host, "calendar.google.com")
	isICSURL := strings.HasSuffix(strings.ToLower(u.Path), ".ics")

	if !isGoogleCalendar {
		// Nextcloud-style public-calendar export links
		// (/remote.php/dav/public-calendars/<token>?export) serve a complete
		// ICS feed but have no .ics path suffix. They must NOT fall through to
		// the CalDAV code path: its comp-filter window (current week + 6 weeks)
		// silently drops every event outside it. The ?export query parameter
		// and the /public-calendars/ path segment both unambiguously identify
		// a read-only ICS download.
		if u.Query().Has("export") || strings.Contains(strings.ToLower(u.Path), "/public-calendars/") {
			return rawURL, true, nil
		}
		return rawURL, isICSURL, nil
	}

	if isICSURL || strings.Contains(strings.ToLower(u.Path), "/calendar/ical/") {
		return rawURL, true, nil
	}

	query := u.Query()
	calendarID := strings.TrimSpace(query.Get("src"))
	if calendarID == "" {
		cid := strings.TrimSpace(query.Get("cid"))
		if cid != "" {
			calendarID, err = decodeGoogleCalendarCID(cid)
			if err != nil {
				return "", false, err
			}
		}
	}

	if calendarID == "" {
		return rawURL, false, nil
	}

	calendarID, err = url.QueryUnescape(calendarID)
	if err != nil {
		return "", false, err
	}
	calendarID = strings.TrimPrefix(calendarID, "mailto:")

	normalized := "https://calendar.google.com/calendar/ical/" + url.PathEscape(calendarID) + "/public/basic.ics"
	return normalized, true, nil
}

func isGoogleCalendarURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, "calendar.google.com")
}

func decodeGoogleCalendarCID(cid string) (string, error) {
	if strings.Contains(cid, "@") {
		return cid, nil
	}

	decode := func(encoding *base64.Encoding) (string, error) {
		decoded, err := encoding.DecodeString(cid)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	if decoded, err := decode(base64.RawURLEncoding); err == nil {
		return decoded, nil
	}
	if decoded, err := decode(base64.URLEncoding); err == nil {
		return decoded, nil
	}
	if decoded, err := decode(base64.RawStdEncoding); err == nil {
		return decoded, nil
	}
	if decoded, err := decode(base64.StdEncoding); err == nil {
		return decoded, nil
	}

	return "", fmt.Errorf("unable to decode google calendar cid")
}

// anonKeepProps lists the only properties an anonymized (SRC_*_ANON=true)
// event keeps: what a calendar needs to display busy time. Anything not
// listed is dropped, so unknown vendor extensions can never leak private
// data (titles are additionally forced to "unavailable").
var anonKeepProps = map[string]bool{
	ical.PropUID:            true,
	ical.PropDateTimeStamp:  true,
	ical.PropDateTimeStart:  true,
	ical.PropDateTimeEnd:    true,
	ical.PropDuration:       true,
	ical.PropRecurrenceRule: true,
	ical.PropExceptionDates: true,
	ical.PropRecurrenceID:   true,
	ical.PropSummary:        true,
	ical.PropSequence:       true,
	ical.PropTransparency:   true,
	ical.PropStatus:         true,
	ical.PropCreated:        true,
	ical.PropLastModified:   true,
}

func processEvents(events []*caldav.CalendarObject, src *Src) ([]*caldav.CalendarObject, error) {
	calEvents := []*caldav.CalendarObject{}

	// Fallback location for floating (timezone-less) event times; loaded once
	// instead of per event. toTZ ultimately emits UTC for all timed events.
	tz, err := time.LoadLocation("Europe/London")
	if err != nil {
		return nil, err
	}

	for _, eventFromQuery := range events {
		event := eventFromQuery

		for x, vevent := range event.Data.Children {
			if vevent.Name == "VEVENT" {

				cleanOutProps := []string{
					ical.PropDescription,
					ical.PropLocation,
					ical.PropAttendee,
					ical.PropOrganizer,
					ical.PropPriority,
					"X-MICROSOFT-CDO-ALLDAYEVENT",
					"X-MICROSOFT-CDO-APPT-SEQUENCE",
					"X-MICROSOFT-CDO-BUSYSTATUS",
					"X-MICROSOFT-CDO-IMPORTANCE",
					"X-MICROSOFT-CDO-INSTTYPE",
					"X-MICROSOFT-CDO-INTENDEDSTATUS",
					"X-MICROSOFT-CDO-OWNERAPPTID",
					"X-MICROSOFT-DISALLOW-COUNTER",
					"X-MICROSOFT-DONOTFORWARDMEETING",
					"X-MICROSOFT-ISRESPONSEREQUESTED",
					"X-MICROSOFT-LOCATIONS",
					"X-MICROSOFT-REQUESTEDATTENDANCEMODE",
					"X-MOZ-INVITED-ATTENDEE",
					"X-MOZ-RECEIVED-DTSTAMP",
					"X-MOZ-RECEIVED-SEQUENCE",
					"X-MICROSOFT-LOCATIONDISPLAYNAME",
					"X-MICROSOFT-LOCATIONSOURCE",
					"X-MOZ-GENERATION",
				}

				for _, prop := range cleanOutProps {
					event.Data.Children[x].Props.Del(prop)
				}

				if src.Anon {
					event.Data.Children[x].Props.SetText(ical.PropSummary, "unavailable")
					// Whitelist: in anonymized mode keep only the properties a
					// calendar needs to render busy time. Everything else is
					// dropped outright — blacklisting individual fields is not
					// enough, because Outlook extensions such as
					// X-MICROSOFT-SKYPETEAMSMEETINGURL or X-ALT-DESC leak
					// join links and participant identities.
					vevent := event.Data.Children[x]
					for name := range vevent.Props {
						if !anonKeepProps[name] {
							delete(vevent.Props, name)
						}
					}
				}

				s := event.Data.Children[x].Props.Get(ical.PropSummary)
				if s == nil {
					continue
				}
				summaryValue := s.Value
				logrus.Debugf("Event: %s", summaryValue)

				fixInvalidTZIDs(event.Data.Children[x])

				// DTSTAMP is required by RFC 5545 and by go-ical's encoder;
				// some source feeds omit it, which would break /calendar.ics.
				if event.Data.Children[x].Props.Get(ical.PropDateTimeStamp) == nil {
					event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
				}

				event.Data.Children[x].Props.Del(ical.PropTimezoneName)
				event.Data.Children[x].Props.Del(ical.PropTimezoneID)

				if err := harmonizeDurationAndEnd(event, x); err != nil {
					return nil, err
				}

				if err := toTZ(event, x, tz, ical.PropDateTimeStart); err != nil {
					return nil, err
				}

				if err := toTZ(event, x, tz, ical.PropDateTimeEnd); err != nil {
					return nil, err
				}

				if err := toTZ(event, x, tz, ical.PropDateTimeStamp); err != nil {
					return nil, err
				}
			}
		}

		children := []*ical.Component{}
		for _, child := range event.Data.Children {
			if child.Name != "VTIMEZONE" {
				children = append(children, child)
			}
		}
		event.Data.Children = children

		calEvents = append(calEvents, event)
	}

	return calEvents, nil
}

func summaryOfEvent(event *caldav.CalendarObject) string {
	for _, vevent := range event.Data.Children {
		if vevent.Name == "VEVENT" {
			s := vevent.Props.Get(ical.PropSummary)
			if s == nil {
				continue
			}
			return s.Value
		}
	}
	return ""
}

// fixInvalidTZIDs rewrites TZID parameters that are not valid IANA location
// names. Outlook-synced events exported by Nextcloud carry globally-prefixed
// TZIDs such as "/freeassociation.sourceforge.net/Europe/Berlin"; Go's
// time.LoadLocation rejects every name starting with "/" ("time: invalid
// location name"), so go-ical's Prop.DateTime fails and processEvents aborts
// the whole feed at the first affected event. Strategy per affected property:
//  1. keep valid IANA names as-is,
//  2. try the MS-timezone translation table,
//  3. try IANA-looking suffixes of the prefixed name ("/vendor/Europe/Berlin"
//     → "Europe/Berlin"),
//  4. drop the parameter entirely, so the property is interpreted in the
//     feed-wide fallback timezone instead of failing the import.
func fixInvalidTZIDs(vevent *ical.Component) {
	for name := range vevent.Props {
		for i := range vevent.Props[name] {
			prop := &vevent.Props[name][i]
			tzid := prop.Params.Get(ical.ParamTimezoneID)
			if tzid == "" {
				continue
			}
			if _, err := time.LoadLocation(tzid); err == nil {
				continue // already a valid IANA name
			}

			fixed := ""
			if translated := tzLib.TranslateMSTimezoneToIANA(tzid); translated != "" {
				if _, err := time.LoadLocation(translated); err == nil {
					fixed = translated
				}
			}
			if fixed == "" {
				parts := strings.Split(strings.TrimPrefix(tzid, "/"), "/")
				for j := 1; j < len(parts)-1 && fixed == ""; j++ {
					candidate := strings.Join(parts[j:], "/")
					if _, err := time.LoadLocation(candidate); err == nil {
						fixed = candidate
					}
				}
			}

			if fixed != "" {
				prop.Params.Set(ical.ParamTimezoneID, fixed)
			} else {
				prop.Params.Del(ical.ParamTimezoneID)
			}
		}
	}
}

func toTZ(event *caldav.CalendarObject, x int, tz *time.Location, propName string) error {
	eventProps := event.Data.Children[x].Props
	prop := eventProps.Get(propName)
	if prop == nil {
		// Property is optional (e.g. DTSTAMP) — nothing to convert
		return nil
	}
	// All-day events use VALUE=DATE — skip timezone conversion, preserve as-is
	if prop.ValueType() == ical.ValueDate {
		return nil
	}

	// Translate Microsoft timezone names (e.g. "Eastern Standard Time") to IANA before parsing
	tzID := prop.Params.Get(ical.PropTimezoneID)
	if tzID != "" {
		ianaName := translateMSTimezoneToIANA(tzID)
		loc, err := time.LoadLocation(ianaName)
		if err == nil {
			tz = loc
		}
		prop.Params.Set(ical.PropTimezoneID, ianaName)
	}

	dateTime, err := prop.DateTime(tz)
	if err != nil {
		return err
	}

	// Emit as UTC ("Z" form) — unambiguous and allows FullCalendar to display
	// in any viewer-selected timezone correctly.
	utc := dateTime.UTC()
	prop.Params.Del(ical.PropTimezoneID)
	event.Data.Children[x].Props.SetDateTime(propName, utc)

	return nil
}

func harmonizeDurationAndEnd(event *caldav.CalendarObject, x int) error {
	eventProps := event.Data.Children[x].Props
	end := eventProps.Get(ical.PropDateTimeEnd)
	if end != nil {
		return nil
	}

	start := eventProps.Get(ical.PropDateTimeStart)
	if start == nil {
		return fmt.Errorf("start not found for event %s", summaryOfEvent(event))
	}

	duration := eventProps.Get(ical.PropDuration)
	if duration == nil || duration.Value == "" {
		// Zero-duration event: DTSTART only, no DTEND or DURATION — RFC 5545 §3.6.1 valid case
		// Set DTEND = DTSTART so downstream processing and clients handle it correctly
		startTime, err := start.DateTime(time.UTC)
		if err != nil {
			return err
		}
		event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, startTime)
		return nil
	}

	startTime, err := start.DateTime(time.UTC)
	if err != nil {
		return err
	}

	durationTime, err := duration.Duration()
	if err != nil {
		return err
	}

	if durationTime != 0 {
		event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, startTime.Add(durationTime))
		event.Data.Children[x].Props.Del(ical.PropDuration)
	}

	return nil
}
