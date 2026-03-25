package pages

import (
	"github.com/maddalax/htmgo/framework/h"
)

func IndexPage(ctx *h.RequestContext) *h.Page {
	return RootPage(
		h.Link("https://cdn.jsdelivr.net/npm/fullcalendar@6.1.15/main.min.css", "stylesheet"),
		h.Script("https://cdn.jsdelivr.net/npm/fullcalendar@6.1.15/index.global.min.js"),

		h.Div(
			h.Id("top"),
			h.UnsafeRaw(`
			  <div style='float:left'>
					Timezone:
					<select id='time-zone-selector'>
				<option value='local' selected>local</option>
				<option value='UTC'>UTC</option>
				<option value='Europe/Athens'>Europe/Athens</option>
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

			document.addEventListener('DOMContentLoaded', function() {
				var calendarEl = document.getElementById('calendar');
				var calendar = new FullCalendar.Calendar(calendarEl, {
					timeZone: initialTimeZone,
					initialView: 'timeGridWeek',
					editable: true,
    				selectable: true,
					hiddenDays: [0, 6],
					slotMinTime: '06:00:00',     // Start time for week grid
					slotMaxTime: '23:00:00',     // End time for week grid
					slotLabelFormat: {           // Set 24-hour format
						hour: '2-digit',
						minute: '2-digit',
						hour12: false            // 24-hour format
					},
					events: '/events.json',
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

				// when the timezone selector changes, dynamically change the calendar option
				timeZoneSelectorEl.addEventListener('change', function() {
					calendar.setOption('timeZone', this.value);
				});
			});
		`),
	)
}
