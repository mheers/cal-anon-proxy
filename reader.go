package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	tzLib "github.com/mheers/go-tz"
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

		tz, err := time.LoadLocation("Europe/London")
		if err != nil {
			return nil, err
		}

		for x, vevent := range event.Data.Children {
			if vevent.Name == "VEVENT" {

				cleanOutProps := []string{
					ical.PropDescription,
					ical.PropLocation,
					ical.PropAttendee,
					ical.PropOrganizer,
					ical.PropPriority,
					"X-MICROSOFT-CDO-ALLDAYEVENT",
					"X-MICROSOFT-CDO-APPT-SEQUENCE",
					"X-MICROSOFT-CDO-BUSYSTATUS",
					"X-MICROSOFT-CDO-IMPORTANCE",
					"X-MICROSOFT-CDO-INSTTYPE",
					"X-MICROSOFT-CDO-INTENDEDSTATUS",
					"X-MICROSOFT-CDO-OWNERAPPTID",
					"X-MICROSOFT-DISALLOW-COUNTER",
					"X-MICROSOFT-DONOTFORWARDMEETING",
					"X-MICROSOFT-ISRESPONSEREQUESTED",
					"X-MICROSOFT-LOCATIONS",
					"X-MICROSOFT-REQUESTEDATTENDANCEMODE",
					"X-MOZ-INVITED-ATTENDEE",
					"X-MOZ-RECEIVED-DTSTAMP",
					"X-MOZ-RECEIVED-SEQUENCE",
					"X-MICROSOFT-LOCATIONDISPLAYNAME",
					"X-MICROSOFT-LOCATIONSOURCE",
					"X-MOZ-GENERATION",
				}

				for _, prop := range cleanOutProps {
					event.Data.Children[x].Props.Del(prop)
				}

				if src.Anon {
					event.Data.Children[x].Props.SetText(ical.PropSummary, "unavailable")
					for _, prop := range cleanOutProps {
						event.Data.Children[x].Props.SetText(prop, "")
					}
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

				event.Data.Children[x].Props.SetText(ical.PropTimezoneName, tz.String())
				event.Data.Children[x].Props.SetText(ical.PropTimezoneID, tz.String())

				// harmonize DURATION and DTEND
				if err := harmonizeDurationAndEnd(event, x); err != nil {
					return nil, err
				}

				// set timezone for start
				if err := toTZ(event, x, tz, ical.PropDateTimeStart); err != nil {
					return nil, err
				}

				// set timezone for end
				if err := toTZ(event, x, tz, ical.PropDateTimeEnd); err != nil {
					return nil, err
				}

				// set timezone for dtstamp
				if err := toTZ(event, x, tz, ical.PropDateTimeStamp); err != nil {
					return nil, err
				}
			}
			// if vevent.Name == "VTIMEZONE" {
			// 	tzid := event.Data.Children[x].Props.Get(ical.PropTimezoneID)
			// 	if tzid != nil {
			// 		tz := translateTZ(tzid.Value)
			// 		event.Data.Children[x].Props.SetText(ical.PropTimezoneID, tz)
			// 	}
			// 	event.Data.Children[x].Props.SetText(ical.PropTimezoneID, tz.String())
			// }
		}

		// remove VTIMEZONE
		children := []*ical.Component{}
		for _, child := range event.Data.Children {
			if child.Name != "VTIMEZONE" {
				children = append(children, child)
			}
		}
		event.Data.Children = children

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

func toTZ(event *caldav.CalendarObject, x int, tz *time.Location, propName string) error {
	eventProps := event.Data.Children[x].Props
	prop := eventProps.Get(propName)
	if prop == nil {
		return fmt.Errorf("property %s not found for event %s", propName, summaryOfEvent(event))
	}
	tzID := prop.Params.Get(ical.PropTimezoneID)
	if tzID != "" {
		tz := tzLib.TranslateMSTimezoneToIANA(tzID)
		event.Data.Children[x].Props.Get(propName).Params.Set(ical.PropTimezoneID, tz)
	}

	dateTime, err := event.Data.Children[x].Props.Get(propName).DateTime(tz)
	if err != nil {
		return err
	}
	event.Data.Children[x].Props.Get(propName).Params.Set(ical.PropTimezoneID, tz.String())
	event.Data.Children[x].Props.SetDateTime(propName, dateTime.In(tz))

	return nil
}

func harmonizeDurationAndEnd(event *caldav.CalendarObject, x int) error {
	eventProps := event.Data.Children[x].Props
	end := eventProps.Get(ical.PropDateTimeEnd)
	if end != nil {
		return nil
	}

	start := eventProps.Get(ical.PropDateTimeStart)
	if start == nil {
		return fmt.Errorf("start not found for event %s", summaryOfEvent(event))
	}

	duration := eventProps.Get(ical.PropDuration)
	if duration == nil {
		return fmt.Errorf("duration not found for event %s", summaryOfEvent(event))
	}
	if duration.Value == "" {
		return fmt.Errorf("duration not found for event %s", summaryOfEvent(event))
	}

	startTime, err := start.DateTime(time.UTC)
	if err != nil {
		return err
	}

	endTime := startTime

	durationTime, err := duration.Duration()
	if err != nil {
		return err
	}

	if durationTime == 0 {
		return nil
	}

	if endTime.Sub(endTime.Add(durationTime)) != 0 {
		event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, endTime.Add(durationTime))
		event.Data.Children[x].Props.Del(ical.PropDuration)
	}

	return nil
}
