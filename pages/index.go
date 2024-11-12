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
			h.Id("top"),
			h.UnsafeRaw(`
			  <div style='float:left'>
					Timezone:
					<select id='time-zone-selector'>
					<option value='local' selected>local</option>
					<option value='UTC'>UTC</option>
					</select>
				</div>

				<div style='float:right'>
					<span id='loading'>loading...</span>
				</div>

				<div style='clear:both'></div>
				`),
		),
		h.Div(
			h.Id("calendar"),
		),

		h.UnsafeRawScript(`
			var initialTimeZone = 'local';
			var timeZoneSelectorEl = document.getElementById('time-zone-selector');
			var loadingEl = document.getElementById('loading');

			const eventDataTransform = (eventData) => {
			// Extract relevant properties from the event data
			const { title, url, extendedProps, start, end } = eventData;

			// Convert start and end dates to UTC
			const startUtc = new Date(start + 'Z').toISOString();
			const endUtc = new Date(end + 'Z').toISOString();

			return {
				title,
				url,
				extendedProps,
				start: startUtc,
				end: endUtc,
				allDay: false,
				timezone: 'UTC'
			};
			};

			document.addEventListener('DOMContentLoaded', function() {
				var calendarEl = document.getElementById('calendar');
				var calendar = new FullCalendar.Calendar(calendarEl, {
					eventDataTransform: eventDataTransform,
					timeZone: initialTimeZone,
					initialView: 'timeGridWeek',
					editable: true,
    				selectable: true,
					hiddenDays: [0, 6],
					slotMinTime: '06:00:00',     // Start time for week grid
					slotMaxTime: '18:00:00',     // End time for week grid
					slotLabelFormat: {           // Set 24-hour format
						hour: '2-digit',
						minute: '2-digit',
						hour12: false            // 24-hour format
					},
					events: {
						// url: '/public/cal.ics',
						url: '/caldav/',
						format: 'ics',
					},
					headerToolbar: {
						center: 'timeGridWeek,timeGridDay',
					},
					loading: function(bool) {
						if (bool) {
							loadingEl.style.display = 'inline'; // show
						} else {
							loadingEl.style.display = 'none'; // hide
						}
					},
					eventTimeFormat: { hour: '2-digit', minute: '2-digit', timeZoneName: 'short', hour12: false },
				});
				calendar.render();

				// fetch('https://fullcalendar.io/api/demo-feeds/timezones.json')
				// .then((response) => response.json())
				// .then((timeZones) => {
				//   timeZones.forEach(function(timeZone) {
				// 	var optionEl;
			
				// 	if (timeZone !== 'UTC') { // UTC is already in the list
				// 	  optionEl = document.createElement('option');
				// 	  optionEl.value = timeZone;
				// 	  optionEl.innerText = timeZone;
				// 	  timeZoneSelectorEl.appendChild(optionEl);
				// 	}
				//   });
				// });

				// when the timezone selector changes, dynamically change the calendar option
				timeZoneSelectorEl.addEventListener('change', function() {
					calendar.setOption('timeZone', this.value);
				});
			});
		`),
	)
}
