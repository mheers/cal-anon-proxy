package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

func loadFixtureHandler(t *testing.T, files ...string) *CalDavHandler {
	t.Helper()
	h := NewCalDavHandler("/cal/")
	var events []*caldav.CalendarObject
	for _, f := range files {
		fh, err := os.Open(filepath.Join("testdata", f))
		require.NoError(t, err)
		cal, err := ical.NewDecoder(fh).Decode()
		require.NoError(t, err)
		require.NoError(t, fh.Close())
		events = append(events, &caldav.CalendarObject{Path: f, Data: cal})
	}
	h.SetEvents(events)
	return h
}

func serveEventsJSON(t *testing.T, h *CalDavHandler, query string) []jsonEvent {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/events.json"+query, nil)
	rec := httptest.NewRecorder()
	h.ServeEventsJSON(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got []jsonEvent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

func TestServeEventsJSON_SingleEvent(t *testing.T) {
	h := loadFixtureHandler(t, "event_with_dtend.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London")

	require.Len(t, got, 1)
	require.Equal(t, "test-dtend-001@test.invalid", got[0].ID)
	require.Equal(t, "Team Meeting", got[0].Title)
	// 2026-03-25 is before the London DST switch, so naive local time == UTC.
	require.Equal(t, "2026-03-25T10:00:00", got[0].Start)
	require.Equal(t, "2026-03-25T11:00:00", got[0].End)
}

func TestServeEventsJSON_WindowFiltersSingleEvent(t *testing.T) {
	h := loadFixtureHandler(t, "event_with_dtend.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-05-01T00:00:00&end=2026-06-01T00:00:00&timeZone=Europe/London")
	require.Empty(t, got)
}

func TestServeEventsJSON_RRuleExpansion(t *testing.T) {
	h := loadFixtureHandler(t, "event_recurring.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London")

	require.Len(t, got, 4, "weekly COUNT=4 must expand into four occurrences")

	wantSuffixes := []string{
		"20260302T090000Z",
		"20260309T090000Z",
		"20260316T090000Z",
		"20260323T090000Z",
	}
	for i, suffix := range wantSuffixes {
		require.Contains(t, got[i].ID, "-"+suffix)
		require.Equal(t, "Weekly Sync", got[i].Title)
		require.Equal(t, "2026-03-"+[]string{"02", "09", "16", "23"}[i]+"T09:00:00", got[i].Start)
		require.Equal(t, "2026-03-"+[]string{"02", "09", "16", "23"}[i]+"T10:00:00", got[i].End)
	}
}

func TestServeEventsJSON_RecurrenceOverrideApplied(t *testing.T) {
	h := loadFixtureHandler(t, "event_override.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London")

	require.Len(t, got, 3)

	var moved *jsonEvent
	for i := range got {
		if got[i].ID != "" && len(got[i].ID) > 20 && got[i].Start == "2026-03-03T14:00:00" {
			moved = &got[i]
		}
	}
	require.NotNil(t, moved, "override must move the 2026-03-03 occurrence to 14:00")
	require.Equal(t, "Daily Standup (moved)", moved.Title)
	require.Equal(t, "2026-03-03T14:30:00", moved.End)

	// Untouched occurrences keep base data.
	for _, ev := range got {
		if ev.Start == "2026-03-02T08:00:00" || ev.Start == "2026-03-04T08:00:00" {
			require.Equal(t, "Daily Standup", ev.Title)
			require.Equal(t, "2026-03-"+ev.Start[8:10]+"T08:30:00", ev.End)
		}
	}
}

func TestServeEventsJSON_LocalModeEmitsZ(t *testing.T) {
	h := loadFixtureHandler(t, "event_with_dtend.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=local")

	require.Len(t, got, 1)
	require.Contains(t, got[0].Start, "Z", "local mode must return UTC Z-strings")
}

func TestServeEventsJSON_AllDayEvent(t *testing.T) {
	h := loadFixtureHandler(t, "event_allday.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London")

	require.Len(t, got, 1)
	require.True(t, got[0].AllDay, "VALUE=DATE events must be flagged allDay")
	require.Equal(t, "2026-03-25", got[0].Start, "all-day start must be date-only")
	require.Equal(t, "2026-03-26", got[0].End, "all-day end must be date-only")
}

func TestServeEventsJSON_ExDateRemovesOccurrence(t *testing.T) {
	h := loadFixtureHandler(t, "event_exdate.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London")

	require.Len(t, got, 3, "EXDATE must remove the second weekly occurrence")
	for _, ev := range got {
		require.NotContains(t, ev.ID, "20260309T090000Z",
			"the EXDATE'd occurrence (2026-03-09) must not be served")
	}
}

func TestServeEventsJSON_CancelledOverrideRemovesOccurrence(t *testing.T) {
	h := loadFixtureHandler(t, "event_cancelled_override.ics")

	got := serveEventsJSON(t, h,
		"?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London")

	require.Len(t, got, 2, "STATUS:CANCELLED override must delete its occurrence")
	for _, ev := range got {
		require.NotContains(t, ev.ID, "20260303T080000Z",
			"the cancelled occurrence (2026-03-03) must not be served")
	}
}

func TestNormalizedRecurrenceKey(t *testing.T) {
	mk := func(value string) *ical.Prop {
		p := ical.NewProp(ical.PropRecurrenceID)
		p.Value = value
		return p
	}
	withTZ := func(value, tzid string) *ical.Prop {
		p := mk(value)
		p.Params.Set(ical.PropTimezoneID, tzid)
		return p
	}
	asDate := func(value string) *ical.Prop {
		p := mk(value)
		p.SetValueType(ical.ValueDate)
		return p
	}

	tests := []struct {
		name string
		prop *ical.Prop
		want string
	}{
		{"utc z-form", mk("20260303T080000Z"), "20260303T080000Z"},
		{"tzid form", withTZ("20260303T090000", "America/New_York"), "20260303T140000Z"},
		{"floating treated as provided loc", mk("20260303T080000"), "20260303T080000Z"},
		{"date-only", asDate("20260303"), "20260303T000000Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizedRecurrenceKey(tc.prop))
		})
	}
}

func TestServeICS_MergesVEVENTs(t *testing.T) {
	h := loadFixtureHandler(t, "event_with_dtend.ics", "event_recurring.ics")

	req := httptest.NewRequest(http.MethodGet, "/calendar.ics", nil)
	rec := httptest.NewRecorder()
	h.ServeICS(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, ical.MIMEType, rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	require.Contains(t, body, "BEGIN:VCALENDAR")
	require.Contains(t, body, "Team Meeting")
	require.Contains(t, body, "Weekly Sync")
	require.Equal(t, 2, strings.Count(body, "BEGIN:VEVENT"))
}
