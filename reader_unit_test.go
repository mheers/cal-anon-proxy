package main

import (
	"os"
	"strings"
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, path string) *caldav.CalendarObject {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %q: %v", path, err)
	}
	defer f.Close()
	cal, err := ical.NewDecoder(f).Decode()
	if err != nil {
		t.Fatalf("decode fixture %q: %v", path, err)
	}
	return &caldav.CalendarObject{Data: cal}
}

func TestHarmonizeDurationAndEnd(t *testing.T) {
	tests := []struct {
		name           string
		fixture        string
		wantErr        bool
		checkDTEND     string // substring expected in DTEND.Value
		wantNoDURATION bool
		wantDTENDSet   bool // just assert DTEND is not nil
	}{
		{
			name:         "DTEND already present — no-op",
			fixture:      "testdata/event_with_dtend.ics",
			wantErr:      false,
			checkDTEND:   "110000", // DTEND:20260325T110000Z unchanged
			wantDTENDSet: true,
		},
		{
			name:           "DURATION present — compute DTEND",
			fixture:        "testdata/event_with_duration.ics",
			wantErr:        false,
			checkDTEND:     "100000", // DTSTART 09:00Z + PT1H = 10:00Z
			wantNoDURATION: true,
			wantDTENDSet:   true,
		},
		{
			name:         "zero-duration — DTEND equals DTSTART",
			fixture:      "testdata/event_zero_duration.ics",
			wantErr:      false,
			checkDTEND:   "000000", // DTSTART:20260325T000000Z → DTEND same
			wantDTENDSet: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := loadFixture(t, tc.fixture)
			err := harmonizeDurationAndEnd(obj, 0)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tc.wantDTENDSet {
				end := obj.Data.Children[0].Props.Get(ical.PropDateTimeEnd)
				require.NotNil(t, end, "expected DTEND prop to be set")
				if tc.checkDTEND != "" {
					require.True(t, strings.Contains(end.Value, tc.checkDTEND),
						"expected DTEND value %q to contain %q", end.Value, tc.checkDTEND)
				}
			}

			if tc.wantNoDURATION {
				dur := obj.Data.Children[0].Props.Get(ical.PropDuration)
				require.Nil(t, dur, "expected DURATION prop to be deleted after harmonize")
			}
		})
	}
}

func TestToTZ(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	require.NoError(t, err, "failed to load Europe/London timezone")

	tests := []struct {
		name                string
		fixture             string
		prop                string
		wantErr             bool
		wantNoTZID          bool
		wantValueContains   string
		wantUnmodifiedValue string // if set, prop.Value must equal this exactly (all-day case)
	}{
		{
			name:              "UTC event — normalize to UTC",
			fixture:           "testdata/event_with_dtend.ics",
			prop:              ical.PropDateTimeStart,
			wantErr:           false,
			wantNoTZID:        true,
			wantValueContains: "Z",
		},
		{
			name:              "MS timezone — normalize to UTC",
			fixture:           "testdata/event_ms_timezone.ics",
			prop:              ical.PropDateTimeStart,
			wantErr:           false,
			wantNoTZID:        true,
			wantValueContains: "Z",
		},
		{
			name:                "all-day — skip conversion",
			fixture:             "testdata/event_allday.ics",
			prop:                ical.PropDateTimeStart,
			wantErr:             false,
			wantUnmodifiedValue: "20260325", // VALUE=DATE stays unchanged
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := loadFixture(t, tc.fixture)
			err := toTZ(obj, 0, london, tc.prop)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			prop := obj.Data.Children[0].Props.Get(tc.prop)
			require.NotNil(t, prop, "expected prop %q to exist after toTZ", tc.prop)

			if tc.wantNoTZID {
				tzid := prop.Params.Get(ical.PropTimezoneID)
				require.Equal(t, "", tzid, "expected TZID param to be removed")
			}

			if tc.wantValueContains != "" {
				require.Contains(t, prop.Value, tc.wantValueContains,
					"expected datetime value %q to contain %q", prop.Value, tc.wantValueContains)
			}

			if tc.wantUnmodifiedValue != "" {
				require.Equal(t, tc.wantUnmodifiedValue, prop.Value,
					"expected all-day prop value to remain unchanged")
			}
		})
	}
}

func TestSummaryOfEvent(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			name:    "has SUMMARY",
			fixture: "testdata/event_with_dtend.ics",
			want:    "Team Meeting",
		},
		{
			name:    "no SUMMARY",
			fixture: "testdata/event_no_summary.ics",
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := loadFixture(t, tc.fixture)
			got := summaryOfEvent(obj)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeSourceURL(t *testing.T) {
	tests := []struct {
		name           string
		rawURL         string
		wantURL        string
		wantICS        bool
		wantErr        bool
		wantURLContain string
	}{
		{
			name:           "google cid URL",
			rawURL:         "https://calendar.google.com/calendar/u/0?cid=Y2FsZW5kYXIuZXhhbXBsZV9leHRAb3JnLmV4YW1wbGU",
			wantICS:        true,
			wantURLContain: "/calendar/ical/calendar.example_ext@org.example/public/basic.ics",
		},
		{
			name:           "google embed URL",
			rawURL:         "https://calendar.google.com/calendar/embed?src=calendar.example_ext%40org.example&ctz=Europe%2FAthens",
			wantICS:        true,
			wantURLContain: "/calendar/ical/calendar.example_ext@org.example/public/basic.ics",
		},
		{
			name:    "google direct ics URL",
			rawURL:  "https://calendar.google.com/calendar/ical/calendar.example_ext%40org.example/public/basic.ics",
			wantURL: "https://calendar.google.com/calendar/ical/calendar.example_ext%40org.example/public/basic.ics",
			wantICS: true,
		},
		{
			name:    "non-google ics URL",
			rawURL:  "https://example.com/public/calendar.ics",
			wantURL: "https://example.com/public/calendar.ics",
			wantICS: true,
		},
		{
			// Nextcloud public-calendar export links serve a full ICS feed;
			// they must use the ICS path, NOT the CalDAV path whose 6-week
			// comp-filter window silently drops events outside it.
			name:    "nextcloud public-calendars export URL",
			rawURL:  "https://nextcloud.example/remote.php/dav/public-calendars/abc?export",
			wantURL: "https://nextcloud.example/remote.php/dav/public-calendars/abc?export",
			wantICS: true,
		},
		{
			name:    "nextcloud public-calendars URL without export param",
			rawURL:  "https://nextcloud.example/remote.php/dav/public-calendars/abc",
			wantURL: "https://nextcloud.example/remote.php/dav/public-calendars/abc",
			wantICS: true,
		},
		{
			name:    "generic caldav URL stays on caldav path",
			rawURL:  "https://nextcloud.example/remote.php/dav/calendars/marcel/personal",
			wantURL: "https://nextcloud.example/remote.php/dav/calendars/marcel/personal",
			wantICS: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotICS, err := normalizeSourceURL(tc.rawURL)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantICS, gotICS)

			if tc.wantURL != "" {
				require.Equal(t, tc.wantURL, gotURL)
			}
			if tc.wantURLContain != "" {
				require.Contains(t, gotURL, tc.wantURLContain)
			}
		})
	}
}

func TestProcessEventsWithGloballyPrefixedTZID(t *testing.T) {
	// Outlook-synced events (Nextcloud export) carry globally-prefixed TZIDs.
	// time.LoadLocation rejects every name starting with "/", so before the
	// fix a single such event aborted the whole feed ("time: invalid location
	// name") and the proxy served nothing but the events parsed before it.
	cal := ical.NewCalendar()

	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, "outlook-synced-1")
	ev.Props.SetText(ical.PropSummary, "Outlook synced")

	start := ical.NewProp(ical.PropDateTimeStart)
	start.Value = "20260615T180000" // 18:00 Europe/Berlin (UTC+2) → 16:00Z
	start.Params = ical.Params{
		ical.ParamTimezoneID: []string{"/freeassociation.sourceforge.net/Europe/Berlin"},
	}
	ev.Props.Add(start)

	end := ical.NewProp(ical.PropDateTimeEnd)
	end.Value = "20260615T190000"
	end.Params = ical.Params{
		ical.ParamTimezoneID: []string{"/freeassociation.sourceforge.net/Europe/Berlin"},
	}
	ev.Props.Add(end)

	cal.Children = append(cal.Children, ev)

	out, err := processEvents(
		[]*caldav.CalendarObject{{Path: "/test.ics", Data: cal}},
		&Src{},
	)
	require.NoError(t, err)
	require.Len(t, out[0].Data.Children, 1)

	startProp := out[0].Data.Children[0].Props.Get(ical.PropDateTimeStart)
	require.NotNil(t, startProp)
	require.Equal(t, "20260615T160000Z", startProp.Value) // converted to UTC

	gotTime, err := startProp.DateTime(time.UTC)
	require.NoError(t, err)
	require.Equal(t, "2026-06-15T16:00:00Z", gotTime.UTC().Format(time.RFC3339))

	endProp := out[0].Data.Children[0].Props.Get(ical.PropDateTimeEnd)
	require.NotNil(t, endProp)
	require.Equal(t, "20260615T170000Z", endProp.Value)
}

func TestProcessEventsDropsUnresolvableTZID(t *testing.T) {
	cal := ical.NewCalendar()

	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, "unknown-zone-1")
	ev.Props.SetText(ical.PropSummary, "Unknown zone")

	start := ical.NewProp(ical.PropDateTimeStart)
	start.Value = "20260615T180000"
	start.Params = ical.Params{
		ical.ParamTimezoneID: []string{"/vendor.example.com/Mars/Olympus"},
	}
	ev.Props.Add(start)
	cal.Children = append(cal.Children, ev)

	out, err := processEvents(
		[]*caldav.CalendarObject{{Path: "/test.ics", Data: cal}},
		&Src{},
	)
	require.NoError(t, err)

	startProp := out[0].Data.Children[0].Props.Get(ical.PropDateTimeStart)
	require.NotNil(t, startProp)
	// Unresolvable zone: TZID dropped, value interpreted in the fallback
	// timezone and emitted as UTC instead of failing the import.
	require.Empty(t, startProp.Params.Get(ical.ParamTimezoneID))
	require.True(t, strings.HasSuffix(startProp.Value, "Z"))
}
