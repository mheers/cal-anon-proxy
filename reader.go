package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	tzLib "github.com/mheers/go-tz"
	"github.com/sirupsen/logrus"
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
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
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
		logrus.Debugf("Calendar: %s", calendar.Name)
	}

	if len(calendars) == 0 {
		return nil, fmt.Errorf("no calendars found for source %s", src.URL)
	}
	calendar := calendars[0]

	// queryStart of current week
	queryStart := time.Now().AddDate(0, 0, -int(time.Now().Weekday()))
	queryEnd := queryStart.AddDate(0, 0, 7*6) // 6 weeks

	// print start date
	logrus.Debugf("Looking for events from %s to %s", queryStart.Format(time.RFC3339), queryEnd.Format(time.RFC3339))

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
					"EXDATE",
					"RECURRENCE-ID",
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
				logrus.Debugf("Event: %s", summaryValue)

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

func toTZ(event *caldav.CalendarObject, x int, tz *time.Location, propName string) error {
	eventProps := event.Data.Children[x].Props
	prop := eventProps.Get(propName)
	if prop == nil {
		return fmt.Errorf("property %s not found for event %s", propName, summaryOfEvent(event))
	}
	// All-day events use VALUE=DATE — skip timezone conversion, preserve as-is
	if prop.ValueType() == ical.ValueDate {
		return nil
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
	if duration == nil || duration.Value == "" {
		// Zero-duration event: DTSTART only, no DTEND or DURATION — RFC 5545 §3.6.1 valid case
		// Set DTEND = DTSTART so downstream processing and clients handle it correctly
		startTime, err := start.DateTime(time.UTC)
		if err != nil {
			return err
		}
		event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, startTime)
		return nil
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
