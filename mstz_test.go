package main

import (
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

// buildSingleEventObject creates a CalendarObject with one VEVENT whose
// DTSTART/DTEND carry the given TZID parameter.
func buildSingleEventObject(t *testing.T, tzid, dtstart string) *caldav.CalendarObject {
	t.Helper()
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, "mstz-test@test.invalid")
	ev.Props.SetText(ical.PropSummary, "MS TZ Test")
	ev.Props.Set(&ical.Prop{
		Name:   ical.PropDateTimeStart,
		Value:  dtstart,
		Params: ical.Params{ical.PropTimezoneID: {tzid}},
	})
	ev.Props.Set(&ical.Prop{
		Name:   ical.PropDateTimeEnd,
		Value:  dtstart,
		Params: ical.Params{ical.PropTimezoneID: {tzid}},
	})
	cal.Children = append(cal.Children, ev)
	return &caldav.CalendarObject{Data: cal}
}

func TestTranslateMSTimezoneToIANA(t *testing.T) {
	tests := []struct {
		ms   string
		want string
	}{
		{"W. Europe Standard Time", "Europe/Berlin"},
		{"GTB Standard Time", "Europe/Athens"},
		{"GMT Standard Time", "Europe/London"},
		{"Eastern Standard Time", "America/New_York"},
		{"Pacific Standard Time", "America/Los_Angeles"},
		{"India Standard Time", "Asia/Kolkata"},
		{"China Standard Time", "Asia/Shanghai"},
		{"Tokyo Standard Time", "Asia/Tokyo"},
		{"South Africa Standard Time", "Africa/Johannesburg"},
		{"Taipei Standard Time", "Asia/Taipei"},
		// Regression: empty case body upstream returned the MS name unchanged
		{"SA Eastern Standard Time", "America/Fortaleza"},
		// Regression: missing upstream entirely (common Exchange zone)
		{"Central Standard Time", "America/Chicago"},
		{"US Mountain Standard Time", "America/Phoenix"},
		{"AUS Central Standard Time", "Australia/Darwin"},
		{"Newfoundland Standard Time", "America/St_Johns"},
		{"UTC", "UTC"},
		// Unknown and IANA names pass through untouched
		{"Mars Standard Time", "Mars Standard Time"},
		{"Europe/Berlin", "Europe/Berlin"},
		{"Asia/Kolkata", "Asia/Kolkata"},
	}

	for _, tc := range tests {
		require.Equal(t, tc.want, translateMSTimezoneToIANA(tc.ms), "input %q", tc.ms)
	}
}

// TestMSTimezone_AllTranslationsLoadable guards against typos: every mapped
// IANA name must resolve via time.LoadLocation (works even without system
// tzdata because main.go embeds time/tzdata).
func TestMSTimezone_AllTranslationsLoadable(t *testing.T) {
	for _, ms := range []string{
		"Afghanistan Standard Time", "Alaskan Standard Time", "Arab Standard Time",
		"Arabian Standard Time", "Arabic Standard Time", "Argentina Standard Time",
		"Atlantic Standard Time", "AUS Central Standard Time", "AUS Eastern Standard Time",
		"Azerbaijan Standard Time", "Azores Standard Time", "Bangladesh Standard Time",
		"Belarus Standard Time", "Canada Central Standard Time", "Cape Verde Standard Time",
		"Caucasus Standard Time", "Cen. Australia Standard Time", "Central America Standard Time",
		"Central Asia Standard Time", "Central Europe Standard Time", "Central European Standard Time",
		"Central Pacific Standard Time", "Central Standard Time", "Central Standard Time (Mexico)",
		"China Standard Time", "Dateline Standard Time", "E. Africa Standard Time",
		"E. Europe Standard Time", "E. South America Standard Time", "Eastern Standard Time",
		"Egypt Standard Time", "Fiji Standard Time", "FLE Standard Time", "Georgian Standard Time",
		"GMT Standard Time", "Greenland Standard Time", "Greenwich Standard Time", "GTB Standard Time",
		"Hawaiian Standard Time", "India Standard Time", "Iran Standard Time", "Israel Standard Time",
		"Jordan Standard Time", "Korea Standard Time", "Mauritius Standard Time",
		"Middle East Standard Time", "Montevideo Standard Time", "Morocco Standard Time",
		"Mountain Standard Time", "Myanmar Standard Time", "Nepal Standard Time",
		"Newfoundland Standard Time", "New Zealand Standard Time", "Pacific SA Standard Time",
		"Pacific Standard Time", "Pakistan Standard Time", "Paraguay Standard Time",
		"Romance Standard Time", "Russian Standard Time", "SA Eastern Standard Time",
		"SA Pacific Standard Time", "SA Western Standard Time", "Samoa Standard Time",
		"SE Asia Standard Time", "Singapore Standard Time", "South Africa Standard Time",
		"Sri Lanka Standard Time", "Syria Standard Time", "Taipei Standard Time",
		"Tokyo Standard Time", "Tonga Standard Time", "Türkiye Standard Time",
		"Ulaanbaatar Standard Time", "US Eastern Standard Time", "US Mountain Standard Time",
		"UTC", "UTC-02", "UTC-11", "UTC+12", "Venezuela Standard Time",
		"W. Central Africa Standard Time", "W. Europe Standard Time", "West Asia Standard Time",
		"West Pacific Standard Time",
	} {
		iana := translateMSTimezoneToIANA(ms)
		_, err := time.LoadLocation(iana)
		require.NoError(t, err, "MS zone %q -> IANA %q must be loadable", ms, iana)
		if ms != "UTC" { // "UTC" legitimately maps to itself
			require.NotEqual(t, ms, iana, "MS zone %q must translate, not pass through", ms)
		}
	}
}

// TestToTZ_MSCentralStandardTime proves the previously-missing US Central zone
// now converts correctly through the full pipeline.
func TestToTZ_MSCentralStandardTime(t *testing.T) {
	// 14:00 Central on 2026-03-25 (CDT, UTC-5) == 19:00Z
	obj := buildSingleEventObject(t, "Central Standard Time", "20260325T140000")
	events, err := processEvents([]*caldav.CalendarObject{obj}, &Src{})
	require.NoError(t, err)
	require.Len(t, events, 1)

	var start string
	for _, child := range events[0].Data.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		if p := child.Props.Get(ical.PropDateTimeStart); p != nil {
			start = p.Value
		}
	}
	require.Equal(t, "20260325T190000Z", start)
}
