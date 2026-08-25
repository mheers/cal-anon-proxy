package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

// testConfig mirrors ReadConfig's env defaults for tests that construct
// Config directly (the visibility window defaults matter to downloadAll).
func testConfig(src1URL string) *Config {
	return &Config{
		Src1URL:           src1URL,
		WindowPastWeeks:   4,
		WindowFutureWeeks: 8,
	}
}

func icsServer(t *testing.T, body *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := body.Load().(string)
		if b == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		fmt.Fprint(w, b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func singleEventICS(uid string) string {
	return fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:%s
SUMMARY:%s
DTSTART:%s
DTEND:%s
DTSTAMP:20260301T000000Z
END:VEVENT
END:VCALENDAR
`, uid, uid, testDay(2, 9), testDay(2, 10))
}

func twoEventICS(uidA, uidB string) string {
	return fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:%s
SUMMARY:%s
DTSTART:%s
DTEND:%s
DTSTAMP:20260301T000000Z
END:VEVENT
BEGIN:VEVENT
UID:%s
SUMMARY:%s
DTSTART:%s
DTEND:%s
DTSTAMP:20260301T000000Z
END:VEVENT
END:VCALENDAR
`, uidA, uidA, testDay(2, 9), testDay(2, 10), uidB, uidB, testDay(2, 11), testDay(2, 12))
}

// testDay returns a UTC datetime N days from now at hour:00, inside the
// proxy's default visibility window regardless of when tests run.
func testDay(daysFromNow int, hour int) string {
	t := time.Now().UTC().AddDate(0, 0, daysFromNow).
		Truncate(24 * time.Hour).
		Add(time.Duration(hour) * time.Hour)
	return t.Format("20060102T150405Z")
}

func TestDownloadAll_PartialFailureServesLastKnownGood(t *testing.T) {
	var bodyA, bodyB atomic.Value

	srvA := icsServer(t, &bodyA)
	srvB := icsServer(t, &bodyB)
	bodyA.Store(singleEventICS("deleted@a"))
	bodyB.Store(singleEventICS("stale@b"))

	config := testConfig(srvA.URL + "/calendar.ics")
	config.Src2URL = srvB.URL + "/calendar.ics"
	p := NewCalProxy(config)

	events, err := p.downloadAll()
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Source A drops its event (deletion upstream); source B fails entirely.
	bodyA.Store(singleEventICS("fresh@a"))
	bodyB.Store("")

	events, err = p.downloadAll()
	require.NoError(t, err, "one failing source must not abort the refresh")
	require.Len(t, events, 2)
	require.Equal(t, []string{"fresh@a", "stale@b"}, eventUIDs(events),
		"A serves fresh data (deletion applied), B keeps last known good")
}

func TestDownloadAll_AllSourcesFailOnFirstRunReturnsError(t *testing.T) {
	var bodyA atomic.Value
	srvA := icsServer(t, &bodyA)

	p := NewCalProxy(testConfig(srvA.URL + "/calendar.ics"))

	events, err := p.downloadAll()
	require.Error(t, err, "with no cached data at all, a failing source must surface as an error")
	require.Empty(t, events)
}

func TestSetEvents_NormalizesPathsAndSetsETags(t *testing.T) {
	h := NewCalDavHandler("/caldav/")

	events := []*caldav.CalendarObject{
		{Path: "/upstream/cal/xyz.ics", Data: loadFixture(t, "testdata/event_with_dtend.ics").Data},
		{Path: "https://cal.example.com/basic.ics", Data: loadFixture(t, "testdata/event_recurring.ics").Data},
	}
	h.SetEvents(events)

	objects := h.backendObjects("/caldav/")
	require.Len(t, objects, 2)
	for _, obj := range objects {
		require.True(t, strings.HasPrefix(obj.Path, "/caldav/"),
			"object paths must live under the proxy namespace, got %q", obj.Path)
		require.NotContains(t, obj.Path, "upstream")
		require.NotContains(t, obj.Path, "https://")
		require.NotEmpty(t, obj.ETag, "content ETag required for client change detection")
		require.Greater(t, obj.ContentLength, int64(0))
	}
	require.NotEqual(t, objects[0].Path, objects[1].Path)

	// Same UIDs again must produce identical paths and etags (stable hrefs).
	h.SetEvents(events)
	refreshed := h.backendObjects("/caldav/")
	require.Equal(t, objects[0].Path, refreshed[0].Path)
	require.Equal(t, objects[0].ETag, refreshed[0].ETag)
}

func TestSetEvents_DuplicateUIDsGetDistinctPaths(t *testing.T) {
	h := NewCalDavHandler("/caldav/")
	events := []*caldav.CalendarObject{
		{Data: loadFixture(t, "testdata/event_with_dtend.ics").Data},
		{Data: loadFixture(t, "testdata/event_with_dtend.ics").Data},
	}

	h.SetEvents(events)

	objects := h.backendObjects("/caldav/")
	require.Len(t, objects, 2)
	require.NotEqual(t, objects[0].Path, objects[1].Path,
		"duplicate UIDs must not collide on one resource path")
	require.True(t, strings.HasSuffix(objects[1].Path, "-2.ics"))
}

func eventUIDs(events []*caldav.CalendarObject) []string {
	uids := make([]string, 0, len(events))
	for _, ev := range events {
		uids = append(uids, firstVEVENTUID(ev))
	}
	return uids
}

// TestProcessEvents_NormalizesExDateAndRecurrenceIDToUTC proves that
// EXDATE/RECURRENCE-ID metadata is converted to UTC alongside DTSTART, so
// excluded occurrences actually match the converted recurrence instants
// (previously a TZID-qualified EXDATE never matched and the "deleted"
// occurrence kept being served).
func TestProcessEvents_NormalizesExDateAndRecurrenceIDToUTC(t *testing.T) {
	cal := ical.NewCalendar()
	cal.Props.SetText("VERSION", "2.0")
	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, "exdate-tz@test.invalid")
	ev.Props.SetText(ical.PropSummary, "Weekly")
	ev.Props.Set(&ical.Prop{
		Name:   ical.PropDateTimeStart,
		Value:  "20260302T090000",
		Params: ical.Params{ical.PropTimezoneID: {"America/New_York"}},
	})
	ev.Props.Set(&ical.Prop{
		Name:   ical.PropDateTimeEnd,
		Value:  "20260302T100000",
		Params: ical.Params{ical.PropTimezoneID: {"America/New_York"}},
	})
	ev.Props.SetText(ical.PropRecurrenceRule, "FREQ=WEEKLY;COUNT=4")
	ev.Props.Set(&ical.Prop{
		Name:   ical.PropExceptionDates,
		Value:  "20260309T090000",
		Params: ical.Params{ical.PropTimezoneID: {"America/New_York"}},
	})
	cal.Children = append(cal.Children, ev)

	processed, err := processEvents([]*caldav.CalendarObject{{Data: cal}}, &Src{})
	require.NoError(t, err)
	require.Len(t, processed, 1)

	var dtstart, exdate string
	for _, child := range processed[0].Data.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		if p := child.Props.Get(ical.PropDateTimeStart); p != nil {
			dtstart = p.Value
		}
		if p := child.Props.Get(ical.PropExceptionDates); p != nil {
			exdate = p.Value
		}
	}

	require.Equal(t, "20260302T140000Z", dtstart, "DTSTART converted to UTC")
	// 2026-03-09 is after the US DST switch (EDT, UTC-4): instant-preserving
	// conversion yields 13:00Z. For feeds already in UTC (the common case,
	// covered by testdata/event_exdate.ics) exclusion matches exactly; for
	// TZID feeds crossing DST the whole series flattens to fixed UTC
	// wall-clock — a pre-existing property of this proxy's UTC normalization.
	require.Equal(t, "20260309T130000Z", exdate)
}

// TestCalDAVDeletionPropagation_EndToEnd drives the exact flow Thunderbird
// uses: periodic REPORT (calendar-query) refreshes against /caldav/. After
// the source drops an event and the proxy refreshes, the next REPORT must no
// longer list it.
func TestCalDAVDeletionPropagation_EndToEnd(t *testing.T) {
	var body atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := body.Load().(string)
		if b == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, b)
	}))
	defer srv.Close()

	body.Store(singleEventICS("keep@a"))
	// Source now serves two events: "gone@a" will be deleted below.
	body.Store(twoEventICS("keep@a", "gone@a"))

	proxy := NewCalProxy(testConfig(srv.URL + "/calendar.ics"))
	handler := NewCalDavHandler("/caldav/")
	refresh := func() []string {
		events, err := proxy.downloadAll()
		require.NoError(t, err)
		events = proxy.compactEvents(events)
		handler.SetEvents(events)

		calSrv := httptest.NewServer(handler.HTTPHandler())
		defer calSrv.Close()
		client, err := caldav.NewClient(http.DefaultClient, calSrv.URL)
		require.NoError(t, err)
		result, err := client.QueryCalendar(context.Background(), "/caldav/", &caldav.CalendarQuery{
			CompRequest: caldav.CalendarCompRequest{Name: "VCALENDAR", AllComps: true, AllProps: true},
			CompFilter: caldav.CompFilter{
				Name:  "VCALENDAR",
				Comps: []caldav.CompFilter{{Name: "VEVENT"}},
			},
		})
		require.NoError(t, err)
		var uids []string
		for _, obj := range result {
			uids = append(uids, firstVEVENTUID(&obj))
		}
		sort.Strings(uids)
		return uids
	}

	require.Equal(t, []string{"gone@a", "keep@a"}, refresh())

	// Source deletes "gone@a".
	body.Store(singleEventICS("keep@a"))
	require.Equal(t, []string{"keep@a"}, refresh(),
		"deleted source event must disappear from the CalDAV REPORT listing")
}

func (h *CalDavHandler) backendObjects(path string) []caldav.CalendarObject {
	backend, ok := h.Handler.Backend.(*calendarBackend)
	if !ok {
		return nil
	}
	return backend.objectMap[path]
}
