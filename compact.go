package main

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

// compactEvents merges overlapping and touching non-recurring events into
// contiguous busy blocks. Since the proxy publishes anonymized availability,
// several overlapping "unavailable" events add no information — collapsing
// them yields a cleaner calendar for CalDAV clients, /calendar.ics and the
// web UI alike.
//
// Recurring events (RRULE/RDATE), recurrence overrides (RECURRENCE-ID) and
// EXDATE-bearing components are left untouched: compacting them correctly
// would require expanding recurrences, which would destroy the RRULE
// structure clients rely on.
func (p *CalProxy) compactEvents(events []*caldav.CalendarObject) []*caldav.CalendarObject {
	if !p.config.CompactOverlappingEvents {
		return events
	}
	return compactCalendarObjects(events)
}

type busyInterval struct {
	start  time.Time
	end    time.Time
	allDay bool
	comp   *ical.Component
	uid    string
}

func compactCalendarObjects(events []*caldav.CalendarObject) []*caldav.CalendarObject {
	var intervals []busyInterval
	out := make([]*caldav.CalendarObject, 0, len(events))

	for _, obj := range events {
		if obj.Data == nil {
			out = append(out, obj)
			continue
		}

		kept := make([]*ical.Component, 0, len(obj.Data.Children))
		consumed := false
		for _, child := range obj.Data.Children {
			if child.Name == ical.CompEvent {
				if iv, ok := extractBusyInterval(child); ok {
					intervals = append(intervals, iv)
					consumed = true
					continue
				}
			}
			kept = append(kept, child)
		}

		if !consumed {
			out = append(out, obj)
			continue
		}
		if len(kept) > 0 {
			clone := *obj
			data := *obj.Data
			data.Children = kept
			clone.Data = &data
			out = append(out, &clone)
		}
	}

	for _, cluster := range mergeBusyIntervals(intervals) {
		out = append(out, clusterToObject(cluster))
	}
	return out
}

// extractBusyInterval converts a plain single VEVENT into an interval if it
// is safe to compact: it must have both DTSTART and DTEND and carry no
// recurrence metadata or cancellation status.
func extractBusyInterval(comp *ical.Component) (busyInterval, bool) {
	if hasAnyProp(comp, ical.PropRecurrenceRule, ical.PropRecurrenceDates, ical.PropExceptionDates, ical.PropRecurrenceID) {
		return busyInterval{}, false
	}
	if isCancelledComponent(comp) {
		return busyInterval{}, false
	}

	startProp := comp.Props.Get(ical.PropDateTimeStart)
	endProp := comp.Props.Get(ical.PropDateTimeEnd)
	if startProp == nil || endProp == nil {
		return busyInterval{}, false
	}

	allDay := startProp.ValueType() == ical.ValueDate
	start, err := startProp.DateTime(time.UTC)
	if err != nil {
		return busyInterval{}, false
	}
	end, err := endProp.DateTime(time.UTC)
	if err != nil {
		return busyInterval{}, false
	}

	iv := busyInterval{start: start.UTC(), end: end.UTC(), allDay: allDay, comp: comp}
	if p := comp.Props.Get(ical.PropUID); p != nil {
		iv.uid = strings.TrimSpace(p.Value)
	}
	return iv, true
}

func hasAnyProp(comp *ical.Component, names ...string) bool {
	for _, name := range names {
		if comp.Props.Get(name) != nil {
			return true
		}
	}
	return false
}

// mergeBusyIntervals merges overlapping AND touching intervals into maximal
// clusters, keeping timed and all-day events in separate groups so they are
// never merged with each other.
func mergeBusyIntervals(intervals []busyInterval) [][]busyInterval {
	timed := make([]busyInterval, 0, len(intervals))
	allDay := make([]busyInterval, 0, len(intervals))
	for _, iv := range intervals {
		if iv.allDay {
			allDay = append(allDay, iv)
		} else {
			timed = append(timed, iv)
		}
	}

	clusters := mergeSameKind(timed)
	clusters = append(clusters, mergeSameKind(allDay)...)

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i][0].start.Before(clusters[j][0].start)
	})
	return clusters
}

func mergeSameKind(intervals []busyInterval) [][]busyInterval {
	if len(intervals) == 0 {
		return nil
	}
	sorted := make([]busyInterval, len(intervals))
	copy(sorted, intervals)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start.Equal(sorted[j].start) {
			return sorted[i].end.After(sorted[j].end)
		}
		return sorted[i].start.Before(sorted[j].start)
	})

	var clusters [][]busyInterval
	current := []busyInterval{sorted[0]}
	clusterEnd := sorted[0].end

	flush := func() {
		clusters = append(clusters, current)
	}

	for _, iv := range sorted[1:] {
		// Touching intervals (end == next start) merge too: contiguous busy
		// blocks read as one continuous block of unavailability.
		if !iv.start.After(clusterEnd) {
			current = append(current, iv)
			if iv.end.After(clusterEnd) {
				clusterEnd = iv.end
			}
			continue
		}
		flush()
		current = []busyInterval{iv}
		clusterEnd = iv.end
	}
	flush()

	return clusters
}

// clusterToObject renders a merged cluster as a new single-event object.
// The UID is derived from the sorted member UIDs so it stays stable across
// sync cycles as long as membership does not change. DTSTAMP reuses the
// newest member stamp to avoid needless ETag churn.
func clusterToObject(cluster []busyInterval) *caldav.CalendarObject {
	members := make([]busyInterval, len(cluster))
	copy(members, cluster)
	sort.Slice(members, func(i, j int) bool {
		if members[i].start.Equal(members[j].start) {
			return members[i].end.Before(members[j].end)
		}
		return members[i].start.Before(members[j].start)
	})

	uids := make([]string, 0, len(members))
	var latestStamp time.Time
	for _, m := range members {
		uids = append(uids, m.uid)
		if s := m.comp.Props.Get(ical.PropDateTimeStamp); s != nil {
			if t, err := s.DateTime(time.UTC); err == nil && t.After(latestStamp) {
				latestStamp = t
			}
		}
	}
	if latestStamp.IsZero() {
		latestStamp = time.Now().UTC()
	}

	sum := sha1.Sum([]byte(strings.Join(uids, "\n")))
	uid := "compact-" + hex.EncodeToString(sum[:8]) + "@cal-anon-proxy"

	cal := ical.NewCalendar()
	cal.Props.SetText("VERSION", "2.0")
	cal.Props.SetText("PRODID", "-//cal-anon-proxy//EN")

	ev := ical.NewComponent(ical.CompEvent)
	ev.Props.SetText(ical.PropUID, uid)
	if summary := members[0].comp.Props.Get(ical.PropSummary); summary != nil {
		ev.Props.SetText(ical.PropSummary, summary.Value)
	}

	stamp := ical.NewProp(ical.PropDateTimeStamp)
	stamp.SetDateTime(latestStamp.UTC())
	ev.Props.Set(stamp)

	start := ical.NewProp(ical.PropDateTimeStart)
	end := ical.NewProp(ical.PropDateTimeEnd)
	if members[0].allDay {
		start.SetValueType(ical.ValueDate)
		start.Value = members[0].start.UTC().Format("20060102")
		end.SetValueType(ical.ValueDate)
		end.Value = members[len(members)-1].end.UTC().Format("20060102")
	} else {
		start.SetDateTime(members[0].start.UTC())
		end.SetDateTime(members[len(members)-1].end.UTC())
	}
	ev.Props.Set(start)
	ev.Props.Set(end)

	cal.Children = append(cal.Children, ev)

	return &caldav.CalendarObject{Data: cal}
}
