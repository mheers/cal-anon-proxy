package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
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
	return events, nil
}

func (p *CalProxy) download(src *Src) ([]*caldav.CalendarObject, error) {
	httpClient := &http.Client{}
	caldavClient, err := caldav.NewClient(webdav.HTTPClientWithBasicAuth(httpClient, src.Username, src.Password), src.URL)
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
		fmt.Printf("Calendar: %s\n", calendar.Name)
	}

	calendar := calendars[0]

	// queryStart of current week
	queryStart := time.Now().AddDate(0, 0, -int(time.Now().Weekday()))
	queryEnd := queryStart.AddDate(0, 0, 7*6) // 6 weeks

	// print start date
	fmt.Printf("Looking for events from %s to %s\n", queryStart.Format(time.RFC3339), queryEnd.Format(time.RFC3339))

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

	calEvents := []*caldav.CalendarObject{}

	for _, eventFromQuery := range queryResult {
		event := &eventFromQuery

		allowedEvents := []string{}

		if len(allowedEvents) > 0 {
			summary := summaryOfEvent(event)
			if !contains(allowedEvents, summary) {
				continue
			}
		}

		renameEvents := map[string]string{}

		for x, vevent := range event.Data.Children {
			if vevent.Name == "VEVENT" {
				if src.Anon {
					event.Data.Children[x].Props.SetText(ical.PropSummary, "unavailable")
					event.Data.Children[x].Props.SetText(ical.PropDescription, "")
					event.Data.Children[x].Props.SetText(ical.PropLocation, "")
					event.Data.Children[x].Props.SetText(ical.PropAttendee, "")
				}

				s := event.Data.Children[x].Props.Get(ical.PropSummary)
				if s == nil {
					continue
				}
				summaryValue := s.Value
				fmt.Printf("Event: %s\n", summaryValue)

				if newSummary, ok := renameEvents[summaryValue]; ok {
					event.Data.Children[x].Props.SetText(ical.PropSummary, newSummary)
				}

				// set timezone to UTC for start
				startTzID := event.Data.Children[x].Props.Get(ical.PropDateTimeStart).Params.Get(ical.PropTimezoneID)
				if startTzID != "" {
					tz := translateTZ(startTzID)
					event.Data.Children[x].Props.Get(ical.PropDateTimeStart).Params.Set(ical.PropTimezoneID, tz)
					event.Data.Children[x].Props.SetText(ical.PropTimezoneName, tz)

					utc, err := time.LoadLocation("UTC")
					if err != nil {
						return nil, err
					}

					startAt, err := event.Data.Children[x].Props.Get(ical.PropDateTimeStart).DateTime(utc)
					if err != nil {
						return nil, err
					}
					event.Data.Children[x].Props.Get(ical.PropDateTimeStart).Params.Set(ical.PropTimezoneID, "UTC")
					event.Data.Children[x].Props.SetText(ical.PropTimezoneName, "UTC")
					event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeStart, startAt.UTC())
				}

				// set timezone to UTC for end
				endTzID := event.Data.Children[x].Props.Get(ical.PropDateTimeEnd).Params.Get(ical.PropTimezoneID)
				if endTzID != "" {
					tz := translateTZ(endTzID)
					event.Data.Children[x].Props.Get(ical.PropDateTimeEnd).Params.Set(ical.PropTimezoneID, tz)
					event.Data.Children[x].Props.SetText(ical.PropTimezoneName, tz)

					utc, err := time.LoadLocation("UTC")
					if err != nil {
						return nil, err
					}

					endAt, err := event.Data.Children[x].Props.Get(ical.PropDateTimeEnd).DateTime(utc)
					if err != nil {
						return nil, err
					}
					event.Data.Children[x].Props.Get(ical.PropDateTimeEnd).Params.Set(ical.PropTimezoneID, "UTC")
					event.Data.Children[x].Props.SetText(ical.PropTimezoneName, "UTC")
					event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, endAt.UTC())
				}
			}
			if vevent.Name == "VTIMEZONE" {
				// tzid := event.Data.Children[x].Props.Get(ical.PropTimezoneID)
				// if tzid != nil {
				// 	tz := translateTZ(tzid.Value)
				// 	event.Data.Children[x].Props.SetText(ical.PropTimezoneID, tz)
				// }
				// event.Data.Children[x].Props.SetText(ical.PropTimezoneID, "UTC")
			}
		}

		// remove VTIMEZONE
		// children := []*ical.Component{}
		// for _, child := range event.Data.Children {
		// 	if child.Name != "VTIMEZONE" {
		// 		children = append(children, child)
		// 	}
		// }
		// event.Data.Children = children

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

func contains(arr []string, s string) bool {
	for _, a := range arr {
		if a == s {
			return true
		}
	}
	return false
}
