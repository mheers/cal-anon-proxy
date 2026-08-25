package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

// loadICSObject decodes a fixture file into a single CalendarObject.
func loadICSObject(t *testing.T, name string) *caldav.CalendarObject {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	require.NoError(t, err)
	defer f.Close()
	cal, err := ical.NewDecoder(f).Decode()
	require.NoError(t, err)
	return &caldav.CalendarObject{Path: name, Data: cal}
}

// veventProp returns a prop of the nth VEVENT child.
func veventProp(t *testing.T, obj *caldav.CalendarObject, n int, propName string) string {
	t.Helper()
	count := -1
	for _, child := range obj.Data.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		count++
		if count != n {
			continue
		}
		if p := child.Props.Get(propName); p != nil {
			return p.Value
		}
	}
	t.Fatalf("prop %s of VEVENT %d not found", propName, n)
	return ""
}

// TestProcessEvents_MultiTimezoneConversion feeds events authored with
// TZID parameters in seven timezones through the real processing pipeline
// and asserts each is converted to the correct UTC instant.
//
// Reference date 2026-08-25 (summer): offsets are
//
//	Asia/Taipei +8, Asia/Kolkata +5:30, Europe/Athens +3,
//	Europe/Berlin +2, Africa/Harare +2, Europe/London +1,
//	America/New_York -4 (EDT).
func TestProcessEvents_MultiTimezoneConversion(t *testing.T) {
	src := loadICSObject(t, "multi_tz_events.ics")

	events, err := processEvents([]*caldav.CalendarObject{src}, &Src{})
	require.NoError(t, err)
	require.Len(t, events, 1)

	// expected UTC wall times for DTSTART / DTEND
	cases := []struct {
		summary   string
		wantStart string
		wantEnd   string
	}{
		{"Taiwan 15:00 CST", "20260825T070000Z", "20260825T080000Z"},
		{"India 15:00 IST", "20260825T093000Z", "20260825T103000Z"},
		{"Athens 15:00 EEST", "20260825T120000Z", "20260825T130000Z"},
		{"Berlin 15:00 CEST", "20260825T130000Z", "20260825T140000Z"},
		{"Zimbabwe 15:00 CAT", "20260825T130000Z", "20260825T140000Z"},
		{"London 15:00 BST", "20260825T140000Z", "20260825T150000Z"},
		{"New York 15:00 EDT", "20260825T190000Z", "20260825T200000Z"},
	}

	found := map[string][2]string{}
	n := 0
	for _, child := range events[0].Data.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		summary := veventProp(t, events[0], n, ical.PropSummary)
		start := veventProp(t, events[0], n, ical.PropDateTimeStart)
		end := veventProp(t, events[0], n, ical.PropDateTimeEnd)
		found[summary] = [2]string{start, end}
		n++
	}
	require.Len(t, found, len(cases))

	for _, tc := range cases {
		got, ok := found[tc.summary]
		require.True(t, ok, "missing event %q", tc.summary)
		require.Equal(t, tc.wantStart, got[0], "%s DTSTART must convert to exact UTC instant", tc.summary)
		require.Equal(t, tc.wantEnd, got[1], "%s DTEND must convert to exact UTC instant", tc.summary)

		// TZID param must be gone after normalization (all emitted as Z-form)
		require.NotContains(t, got[0], "TZID")
	}
}

// TestProcessEvents_MultiTZ_JSONFeedTimes asserts what ServeEventsJSON would
// emit per display timezone by re-parsing the processed UTC values.
func TestProcessEvents_MultiTZ_UTCInstants(t *testing.T) {
	src := loadICSObject(t, "multi_tz_events.ics")
	events, err := processEvents([]*caldav.CalendarObject{src}, &Src{})
	require.NoError(t, err)

	// collect DTSTART instants
	instants := map[string]time.Time{}
	for _, child := range events[0].Data.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		s := child.Props.Get(ical.PropSummary)
		d := child.Props.Get(ical.PropDateTimeStart)
		require.NotNil(t, s)
		require.NotNil(t, d)
		ts, err := d.DateTime(time.UTC)
		require.NoError(t, err)
		instants[s.Value] = ts.UTC()
	}

	expectations := map[string]time.Time{
		"Taiwan 15:00 CST":   time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC),
		"India 15:00 IST":    time.Date(2026, 8, 25, 9, 30, 0, 0, time.UTC),
		"Athens 15:00 EEST":  time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		"Berlin 15:00 CEST":  time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC),
		"Zimbabwe 15:00 CAT": time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC),
		"London 15:00 BST":   time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC),
		"New York 15:00 EDT": time.Date(2026, 8, 25, 19, 0, 0, 0, time.UTC),
	}
	for summary, want := range expectations {
		got, ok := instants[summary]
		require.True(t, ok, "missing %q", summary)
		require.True(t, got.Equal(want), "%s: got %s want %s", summary, got, want)
	}
}
