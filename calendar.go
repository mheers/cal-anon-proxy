package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	path string
	*caldav.Handler
}

func (h *CalDavHandler) HTTPHandler() http.Handler {
	return h.Handler
}

func (h *CalDavHandler) SetEvents(events []*CalendarEvent) {
	sessionsCal := caldav.Calendar{
		Path:                  h.path,
		SupportedComponentSet: []string{ical.CompEvent},
	}

	calendars := []caldav.Calendar{
		sessionsCal,
	}
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//xyz Corp//NONSGML PDA Calendar Version 1.0//EN")

	cal.Children = []*ical.Component{}

	for _, event := range events {
		cal.Children = append(cal.Children, event.toICalEvent().Component)
	}

	object := caldav.CalendarObject{
		Path: h.path,
		Data: cal,
	}

	h.Backend = &calendarBackend{
		calendars: calendars,
		objectMap: map[string][]caldav.CalendarObject{
			sessionsCal.Path: {object},
		},
	}
}

func NewCalDavHandler(path string) *CalDavHandler {
	return &CalDavHandler{
		Handler: &caldav.Handler{
			Prefix: path,
		},
		path: path,
	}
}

type CalendarEvent struct {
	ID          string
	CreatedAt   time.Time
	StartAt     time.Time
	EndAt       time.Time
	Summary     string
	Location    string
	Description string
}

func (e *CalendarEvent) toICalEvent() *ical.Event {
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, e.ID)
	event.Props.SetDateTime(ical.PropDateTimeStamp, e.CreatedAt)
	event.Props.SetDateTime(ical.PropDateTimeStart, e.StartAt)
	event.Props.SetDateTime(ical.PropDateTimeEnd, e.EndAt)
	event.Props.SetText(ical.PropSummary, e.Summary)
	event.Props.SetText(ical.PropLocation, e.Location)
	event.Props.SetText(ical.PropDescription, e.Description)
	return event
}
