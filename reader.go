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

func (p *CalProxy) downloadAll() ([]*CalendarEvent, error) {
	events := []*CalendarEvent{}
	for _, src := range p.config.Srcs() {
		srcEvents, err := p.download(src)
		if err != nil {
			return nil, err
		}
		events = append(events, srcEvents...)
	}
	return events, nil
}

func (p *CalProxy) download(src *Src) ([]*CalendarEvent, error) {
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

	// start of current week
	start := time.Now().AddDate(0, 0, -int(time.Now().Weekday()))
	end := start.AddDate(0, 0, 7*4) // 4 weeks

	// print start date
	fmt.Printf("Looking for events from %s to %s\n", start.Format(time.RFC3339), end.Format(time.RFC3339))

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
				Start: start,
				End:   end,
			}},
		},
	})
	if err != nil {
		return nil, err
	}

	calEvents := []*CalendarEvent{}

	for _, eventFromQuery := range queryResult {

		vevent := &ical.Component{}
		// vtimezone := &ical.Component{}
		for _, child := range eventFromQuery.Data.Children {
			if child.Name == "VEVENT" {
				vevent = child
			}
			// if child.Name == "VTIMEZONE" {
			// 	vtimezone = child
			// }
		}

		calEvent := &CalendarEvent{
			Props: vevent.Props,
		}

		if src.Anon {
			calEvent.Props.SetText(ical.PropSummary, "unavailable")
			calEvent.Props.SetText(ical.PropLocation, "")
			calEvent.Props.SetText(ical.PropDescription, "")
			calEvent.Props.SetText(ical.PropAttendee, "")
		}

		calEvents = append(calEvents, calEvent)
	}

	return calEvents, nil
}
