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

		calEvent := &CalendarEvent{}
		if vevent.Props.Get("UID") != nil {
			calEvent.ID = vevent.Props.Get("UID").Value
		}
		if vevent.Props.Get("SUMMARY") != nil {
			calEvent.Summary = vevent.Props.Get("SUMMARY").Value
		}
		if vevent.Props.Get("LOCATION") != nil {
			calEvent.Location = vevent.Props.Get("LOCATION").Value
		}

		startAt, err := dateTimeOfPropName(vevent, "DTSTART")
		if err != nil {
			return nil, err
		}
		calEvent.StartAt = startAt

		endAt, err := dateTimeOfPropName(vevent, "DTEND")
		if err != nil {
			return nil, err
		}
		calEvent.EndAt = endAt

		createdAt, err := dateTimeOfPropName(vevent, "CREATED")
		if err != nil {
			return nil, err
		}
		calEvent.CreatedAt = createdAt

		if vevent.Props.Get("DESCRIPTION") != nil {
			calEvent.Description = vevent.Props.Get("DESCRIPTION").Value
		}

		if src.Anon {
			calEvent.Location = ""
			calEvent.Description = ""
			calEvent.Summary = "unavailable"
		}

		calEvents = append(calEvents, calEvent)
	}

	return calEvents, nil
}

func dateTimeOfPropName(vevent *ical.Component, propName string) (time.Time, error) {
	if vevent.Props.Get(propName) != nil {
		prop := vevent.Props.Get(propName)
		tzid := prop.Params.Get(ical.PropTimezoneID)
		if tzid != "" {
			tzid = translateTZ(tzid)
			prop.Params.Set(ical.PropTimezoneID, tzid)
		}
		dt, err := prop.DateTime(time.UTC)
		if err != nil {
			return time.Time{}, err
		}
		return dt, nil
	}
	return time.Time{}, nil
}
