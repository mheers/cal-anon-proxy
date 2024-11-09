package pages

import (
	"github.com/maddalax/htmgo/framework/h"
)

func IndexPage(ctx *h.RequestContext) *h.Page {
	return RootPage(
		h.Link("https://cdn.jsdelivr.net/npm/fullcalendar@5.5.1/main.min.css", "stylesheet"),
		h.Script("https://cdnjs.cloudflare.com/ajax/libs/ical.js/1.5.0/ical.min.js"),
		h.Script("https://cdn.jsdelivr.net/npm/fullcalendar@6.1.15/index.global.min.js"),
		h.Script("https://cdn.jsdelivr.net/npm/@fullcalendar/icalendar@6.1.15/index.global.min.js"),

		h.Div(
			h.Id("calendar"),
		),

		h.UnsafeRawScript(`
		  document.addEventListener('DOMContentLoaded', function() {
			var calendarEl = document.getElementById('calendar');
			var calendar = new FullCalendar.Calendar(calendarEl, {
			initialView: 'timeGridWeek',
			events: {
				// url: 'http://localhost:8086/caldav/',
				url: '/caldav.ics',
				format: 'ics',
			},
			headerToolbar: {
				center: 'timeGridWeek',
			},
			});
			calendar.render();
		  });
		`),
	)
}
