package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

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
	return nil, nil
}

type CalDavHandler struct {
	mu   sync.RWMutex
	path string
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
	h.mu.Unlock()
}

func NewCalDavHandler(path string) *CalDavHandler {
	return &CalDavHandler{
		Handler: &caldav.Handler{
			Prefix: path,
		},
		path: path,
	}
}
