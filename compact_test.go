package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

func loadFixtureObjects(t *testing.T, files ...string) []*caldav.CalendarObject {
	t.Helper()
	var events []*caldav.CalendarObject
	for _, f := range files {
		fh, err := os.Open(filepath.Join("testdata", f))
		require.NoError(t, err)
		cal, err := ical.NewDecoder(fh).Decode()
		require.NoError(t, err)
		require.NoError(t, fh.Close())
		events = append(events, &caldav.CalendarObject{Path: f, Data: cal})
	}
	return events
}

func mkEventObject(uid, summary, startUTC, endUTC string) *caldav.CalendarObject {
	cal := ical.NewCalendar()
	cal.Props.SetText("VERSION", "2.0")
	cal.Props.SetText("PRODID", "-//cal-anon-proxy//EN")

	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, uid)
	if summary != "" {
		ev.Props.SetText(ical.PropSummary, summary)
	}
	start := ical.NewProp(ical.PropDateTimeStart)
	start.SetDateTime(mustParseUTC(startUTC))
	ev.Props.Set(start)
	end := ical.NewProp(ical.PropDateTimeEnd)
	end.SetDateTime(mustParseUTC(endUTC))
	ev.Props.Set(end)
	stamp := ical.NewProp(ical.PropDateTimeStamp)
	stamp.SetDateTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ev.Props.Set(stamp)

	cal.Children = append(cal.Children, ev)
	return &caldav.CalendarObject{Path: uid + ".ics", Data: cal}
}

func mustParseUTC(s string) time.Time {
	t, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		panic(err)
	}
	return t
}

func veventProps(t *testing.T, obj *caldav.CalendarObject) map[string]*ical.Prop {
	t.Helper()
	for _, child := range obj.Data.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		props := map[string]*ical.Prop{}
		for _, name := range []string{ical.PropUID, ical.PropSummary, ical.PropDateTimeStart, ical.PropDateTimeEnd} {
			props[name] = child.Props.Get(name)
		}
		return props
	}
	t.Fatal("no VEVENT found")
	return nil
}

func TestCompactEvents_MergesOverlappingTimedEvents(t *testing.T) {
	events := []*caldav.CalendarObject{
		mkEventObject("a@x", "A", "20260325T090000Z", "20260325T100000Z"),
		mkEventObject("b@x", "B", "20260325T093000Z", "20260325T110000Z"),
		mkEventObject("c@x", "C", "20260325T120000Z", "20260325T130000Z"),
	}

	got := compactCalendarObjects(events)

	require.Len(t, got, 2, "overlap must collapse into one block; disjoint stays separate")
	first := veventProps(t, got[0])
	require.Equal(t, "20260325T090000Z", first[ical.PropDateTimeStart].Value)
	require.Equal(t, "20260325T110000Z", first[ical.PropDateTimeEnd].Value)
	require.Equal(t, "A", first[ical.PropSummary].Value, "summary comes from earliest member")
	second := veventProps(t, got[1])
	require.Equal(t, "20260325T120000Z", second[ical.PropDateTimeStart].Value)
}

func TestCompactEvents_MergesTouchingIntervals(t *testing.T) {
	events := []*caldav.CalendarObject{
		mkEventObject("a@x", "", "20260325T090000Z", "20260325T100000Z"),
		mkEventObject("b@x", "", "20260325T100000Z", "20260325T110000Z"),
	}

	got := compactCalendarObjects(events)

	require.Len(t, got, 1, "touching intervals form one continuous busy block")
	props := veventProps(t, got[0])
	require.Equal(t, "20260325T090000Z", props[ical.PropDateTimeStart].Value)
	require.Equal(t, "20260325T110000Z", props[ical.PropDateTimeEnd].Value)
}

func TestCompactEvents_StableUIDAcrossRuns(t *testing.T) {
	events := []*caldav.CalendarObject{
		mkEventObject("a@x", "", "20260325T090000Z", "20260325T100000Z"),
		mkEventObject("b@x", "", "20260325T093000Z", "20260325T110000Z"),
	}

	first := veventProps(t, compactCalendarObjects(events)[0])
	second := veventProps(t, compactCalendarObjects(events)[0])

	require.Equal(t, first[ical.PropUID].Value, second[ical.PropUID].Value,
		"merged UID must be deterministic so clients see a stable resource")
	require.True(t, strings.HasPrefix(first[ical.PropUID].Value, "compact-"))
}

func TestCompactEvents_LeavesRecurringAndOverridesUntouched(t *testing.T) {
	events := loadFixtureObjects(t, "event_recurring.ics", "event_override.ics")

	got := compactCalendarObjects(events)

	require.Len(t, got, 2, "recurrence-bearing objects must pass through unchanged")
	for _, obj := range got {
		body := encodeObjectForTest(t, obj)
		require.Contains(t, body, "RRULE", "recurring events must keep their RRULE")
	}
}

func TestCompactEvents_AllDayMergesSeparatelyFromTimed(t *testing.T) {
	allDay1 := mkAllDayObject("d1@x", "20260325", "20260326")
	allDay2 := mkAllDayObject("d2@x", "20260326", "20260327")
	timed := mkEventObject("t@x", "", "20260325T090000Z", "20260325T100000Z")

	got := compactCalendarObjects([]*caldav.CalendarObject{allDay1, allDay2, timed})

	require.Len(t, got, 2, "all-day pair merges into one; timed stays separate")
	var allDayBlock, timedObj *caldav.CalendarObject
	for _, obj := range got {
		props := veventProps(t, obj)
		if props[ical.PropDateTimeStart].ValueType() == ical.ValueDate {
			allDayBlock = obj
		} else {
			timedObj = obj
		}
	}
	require.NotNil(t, allDayBlock)
	require.NotNil(t, timedObj)

	adProps := veventProps(t, allDayBlock)
	require.Equal(t, "20260325", adProps[ical.PropDateTimeStart].Value)
	require.Equal(t, "20260327", adProps[ical.PropDateTimeEnd].Value)
}

func mkAllDayObject(uid, start, end string) *caldav.CalendarObject {
	cal := ical.NewCalendar()
	cal.Props.SetText("VERSION", "2.0")
	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, uid)
	startProp := ical.NewProp(ical.PropDateTimeStart)
	startProp.SetValueType(ical.ValueDate)
	startProp.Value = start
	ev.Props.Set(startProp)
	endProp := ical.NewProp(ical.PropDateTimeEnd)
	endProp.SetValueType(ical.ValueDate)
	endProp.Value = end
	ev.Props.Set(endProp)
	cal.Children = append(cal.Children, ev)
	return &caldav.CalendarObject{Path: uid + ".ics", Data: cal}
}

func encodeObjectForTest(t *testing.T, obj *caldav.CalendarObject) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, ical.NewEncoder(&buf).Encode(obj.Data))
	return buf.String()
}

func TestCompactEvents_DisabledPassthrough(t *testing.T) {
	p := NewCalProxy(&Config{CompactOverlappingEvents: false})
	events := []*caldav.CalendarObject{
		mkEventObject("a@x", "", "20260325T090000Z", "20260325T100000Z"),
		mkEventObject("b@x", "", "20260325T093000Z", "20260325T110000Z"),
	}

	got := p.compactEvents(events)

	require.Len(t, got, 2, "disabled compaction must serve events untouched")
}
