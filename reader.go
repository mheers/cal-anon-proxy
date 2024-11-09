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

	calEvents := []*caldav.CalendarObject{}

	for _, eventFromQuery := range queryResult {
		event := &eventFromQuery
		if src.Anon {
			for x, vevent := range event.Data.Children {
				if vevent.Name == "VEVENT" {
					event.Data.Children[x].Props.SetText(ical.PropSummary, "unavailable")
					event.Data.Children[x].Props.SetText(ical.PropDescription, "")
					event.Data.Children[x].Props.SetText(ical.PropLocation, "")
					event.Data.Children[x].Props.SetText(ical.PropAttendee, "")
				}
			}
		}
		calEvents = append(calEvents, event)
	}

	return calEvents, nil
}
