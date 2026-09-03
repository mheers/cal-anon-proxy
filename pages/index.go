package pages

import (
	"strings"

	"github.com/maddalax/htmgo/framework/h"
)

func IndexPage(ctx *h.RequestContext) *h.Page {
	return TenantIndexPage(ctx, "", "/events.json")
}

// TenantIndexPage renders the calendar WebUI for one tenant. eventsFeedURL is
// the tenant-scoped JSON feed (e.g. "/marcel/events.json"); an empty tenant
// name keeps the legacy "Public Calendar" title.
func TenantIndexPage(ctx *h.RequestContext, tenant, eventsFeedURL string) *h.Page {
	// FullCalendar v7: CSS was split into skeleton + theme + palette, the JS
	// bundle moved to all/global(.min).js, and the theme ships as its own
	// script that self-registers via FullCalendar.Shared.
	const fcVersion = "7.0.2"
	title := "Public Calendar"
	if tenant != "" {
		title = "Public Calendar - " + tenant
	}
	// The feed URL is server-generated from a validated tenant name
	// ([a-z0-9-]), so single-quote injection is not possible here.
	script := strings.Replace(calendarScript, "'/events.json'", "'"+eventsFeedURL+"'", 1)
	return RootPageWithTitle(
		title,
		h.Link("https://cdn.jsdelivr.net/npm/fullcalendar@"+fcVersion+"/skeleton.css", "stylesheet"),
		h.Link("https://cdn.jsdelivr.net/npm/fullcalendar@"+fcVersion+"/themes/classic/theme.css", "stylesheet"),
		h.Link("https://cdn.jsdelivr.net/npm/fullcalendar@"+fcVersion+"/themes/classic/palette.css", "stylesheet"),
		h.Script("https://cdn.jsdelivr.net/npm/fullcalendar@"+fcVersion+"/all/global.min.js"),
		h.Script("https://cdn.jsdelivr.net/npm/fullcalendar@"+fcVersion+"/themes/classic/global.min.js"),

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

		h.UnsafeRawScript(script),
	)
}

const calendarScript = `
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
					// v7 disables the default toolbar — all parts must be explicit
					headerToolbar: {
						start: 'title',
						center: 'timeGridWeek,timeGridDay',
						end: 'today prev,next',
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
				window.calendar = calendar; // exposed for tests/debugging

				// when the timezone selector changes, dynamically change the calendar option
				timeZoneSelectorEl.addEventListener('change', function() {
					calendar.setOption('timeZone', this.value);
				});
			});
		`
