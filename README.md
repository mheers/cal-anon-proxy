# cal-anon-proxy

This is a simple CalDAV server written in Go that proxies read requests to a real CalDAV server, but anonymizes the responses by removing all personal information from the events.

# Usage

## Run

```bash
docker compose up
```

## Multitenancy

Each tenant has a short name (e.g. `marcel`, `josephine`) and gets its own
isolated endpoints:

| Tenant `marcel` | Serves |
|---|---|
| `GET /marcel` | WebUI (unless toggled off) |
| `ANY /marcel/caldav/` | CalDAV endpoint (Thunderbird: `http://localhost:3000/marcel/caldav/`) |
| `GET /marcel/calendar.ics` | Merged ICS feed |
| `GET /marcel/events.json` | FullCalendar JSON feed for the WebUI |

Tenants are configured with indexed env vars (`TENANT_1_*`, `TENANT_2_*`, …,
up to 64, numbering need not be contiguous):

```bash
TENANT_1_NAME=marcel
TENANT_1_WEBUI_ENABLED=true            # default true; false -> /marcel 404s, data endpoints keep working
TENANT_1_SRC_1_URL=https://…?export
TENANT_1_SRC_1_ANON=false
TENANT_1_SRC_1_USERNAME=
TENANT_1_SRC_1_PASSWORD=
TENANT_1_DST_AUTH_ENABLED=true         # basic auth for /marcel/caldav/ only
TENANT_1_DST_USERNAME=
TENANT_1_DST_PASSWORD=

TENANT_2_NAME=josephine
TENANT_2_WEBUI_ENABLED=false
TENANT_2_SRC_1_URL=https://…?export
```

Notes:

- Up to 4 sources per tenant (`TENANT_<N>_SRC_1..4_*`), same shape as the
  legacy `SRC_1..4_*` vars.
- Each tenant has its own source proxy and event store: one tenant's failing
  source never freezes another tenant's updates.
- `WINDOW_PAST_WEEKS`, `WINDOW_FUTURE_WEEKS`, `COMPACT_OVERLAPPING_EVENTS`
  and `SRC_UPDATE_INTERVAL` stay global for all tenants.
- Tenant names must be lowercase alphanumeric with dashes (max 63 chars) and
  must not be `public`, `caldav`, `calendar.ics`, `events.json` or
  `.well-known`.
- When any `TENANT_*_NAME` is set, the legacy routes (`/`, `/caldav/`,
  `/calendar.ics`, `/events.json`) alias the **first** tenant so existing
  CalDAV clients keep working, and legacy `SRC_*/DST_*` sources are ignored.
  With no `TENANT_*_NAME` set, the server behaves exactly as before
  (single-tenant mode).
- Security: as in single-tenant mode, only `/caldav/` (per tenant:
  `/<name>/caldav/`) requires basic auth when enabled; `/calendar.ics` and
  `/events.json` remain **public by design**. Use `TENANT_<N>_SRC_*_ANON=true`
  if titles must not be exposed.

## Visibility window

Only events inside the window are served (defaults: 4 weeks back, 8 weeks
into the future):

- `WINDOW_PAST_WEEKS` (default `4`)
- `WINDOW_FUTURE_WEEKS` (default `8`)

Recurring events are always kept, regardless of how old their DTSTART is.

## CalDAV

In thunderbird calendar add a new entry for http://localhost:8086/caldav/

## Google Calendar source on a server

- Use the calendar URL as `SRC_*_URL`.
- For private Google calendars, use the **Secret address in iCal format** from Google Calendar settings (`.../private-<token>/basic.ics`).
- Do **not** use `SRC_*_USERNAME` / `SRC_*_PASSWORD` for Google calendar links (Google basic auth is not supported for this flow).
- Public Google calendar links are also accepted (`cid=...`, `embed?src=...`, or direct `.../public/basic.ics`).

## Google OAuth App Setup (for calendar cloning)

To use the calendar cloning feature with your own Google OAuth app:

1. **Create a Google Cloud Project**
   - Go to [Google Cloud Console](https://console.cloud.google.com/)
   - Click "Select a Project" → "New Project"
   - Enter a project name (for example, `cal-anon-proxy`) and click "Create"

2. **Enable the Calendar API**
   - In Cloud Console, search for "Calendar API"
   - Click on "Calendar API"
   - Click "Enable"

3. **Create an OAuth 2.0 Desktop App**
   - In Cloud Console, go to "Credentials"
   - Click "Create Credentials" → "OAuth client ID"
   - If prompted, configure the OAuth consent screen first:
     - Choose "External" user type
     - Fill in the app name and your email
     - For scopes, add `https://www.googleapis.com/auth/calendar`
     - Save and continue
   - For application type, select "Desktop app"
   - Click "Create"

4. **Copy Your Client ID**
   - You'll see a popup with "Client ID" and "Client secret"
   - **Copy the Client ID** (you only need the Client ID, the secret is not required for this app)
   - Click "OK"

You now have a `GOOGLE_CLIENT_ID` to use with `google-login` and `google-clone` commands.

## Clone subscribed Google calendar to private Google calendar

Use the new CLI command to copy events from a calendar shared with your Google account into another private calendar you own.

Required env vars:

- `GOOGLE_AUTH_MODE` (`oauth` or `sso`, default: `oauth`)
- `GOOGLE_CLIENT_ID`
- `GOOGLE_REFRESH_TOKEN`
- `GOOGLE_SOURCE_CALENDAR_ID`
- `GOOGLE_DEST_CALENDAR_ID`

Optional for `oauth` mode:

- `GOOGLE_CLIENT_SECRET`

When `GOOGLE_AUTH_MODE=sso`, you do not need `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, or `GOOGLE_REFRESH_TOKEN`.
The command uses Google Application Default Credentials (for example from `gcloud auth application-default login`, workload identity, or service account credentials).

If Google blocks the default `gcloud` app for Calendar access, use your **own Google OAuth Desktop app** and run:

```bash
cal-anon-proxy google-login --client-id "<your-google-oauth-client-id>"
```

This opens a browser login and prints `GOOGLE_REFRESH_TOKEN=...` for later use with `google-clone --auth-mode oauth`.

Optional env vars:

- `GOOGLE_SYNC_DAYS_PAST` (default: `30`)
- `GOOGLE_SYNC_DAYS_FUTURE` (default: `365`)
- `GOOGLE_SYNC_INTERVAL` (example: `15m`, default: run once)
- `GOOGLE_WIPE_DESTINATION` (default: `true`)
- `GOOGLE_ALLOW_EMPTY_SOURCE` (default: `false`)
- `GOOGLE_DRY_RUN` (default: `false`)

Run once:

```bash
cal-anon-proxy google-clone
```

Run once using SSO / ADC:

```bash
GOOGLE_AUTH_MODE=sso \
GOOGLE_SOURCE_CALENDAR_ID="<source-calendar-id>" \
GOOGLE_DEST_CALENDAR_ID="<destination-calendar-id>" \
cal-anon-proxy google-clone
```

If you get `ACCESS_TOKEN_SCOPE_INSUFFICIENT`, refresh ADC with Calendar scope:

```bash
gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/calendar
```

Notes:

- `--source-calendar-id` and `--dest-calendar-id` accept normal calendar IDs (for example `name@group.calendar.google.com`) and also base64/base64url encoded IDs.

Run continuously (for server sync):

```bash
GOOGLE_SYNC_INTERVAL=15m cal-anon-proxy google-clone
```

How it syncs:

- Reads events from the source calendar within the configured time window.
- **Safety guard**: if the source calendar returns 0 events, the run aborts without touching the destination (this protects against misconfiguration wiping your calendar). Override with `--allow-empty-source` / `GOOGLE_ALLOW_EMPTY_SOURCE=true`.
- Deletes destination events in that same window (when `GOOGLE_WIPE_DESTINATION=true`).
  - First sync: deletes all events in the window to establish a clean mirror.
  - Later syncs: deletes only previously cloned events (identified by an `extendedProperties` marker), so manually created destination events are preserved.
- Inserts fresh copies into the destination calendar. Cloned events carry the marker and their source event ID.
- Recurring events are expanded into individual instances (`singleEvents=true`) — the destination receives single events, not RRULE structures.

This makes updates and deletions from the subscribed calendar visible in the destination calendar on every sync cycle.

Security notes:

- Prefer `GOOGLE_CLIENT_SECRET` / `GOOGLE_REFRESH_TOKEN` env vars over CLI flags; flag values are visible in `ps` output to other users on the same machine.
- In server mode, when `DST_AUTH_ENABLED=true`, only `/caldav/` requires basic auth. `/calendar.ics` and `/events.json` remain **public by design** (the web UI fetches `/events.json` from the browser). Use `SRC_*_ANON=true` if titles must not be exposed.

# Build

```bash
cd ci/

export $(cat .env | xargs)
dagger call build-and-push-image --src ../ --registry-token=env:REGISTRY_ACCESS_TOKEN
```


# TODO
- [x] download from multiple cal dav sources
- [x] anonymize fields
- [x] publish calendar
- [x] add optional public authentication
- [x] auto refresh source calendars
- [x] when a source event is deleted, delete the event from the proxy (thunderbird still shows the event)
    - per-source refresh: one failing source no longer freezes updates for all others (last known good events are served per source)
    - stable `/caldav/<uid>.ics` hrefs + content ETags so clients detect additions, changes and deletions on refresh
- [x] frontend with calendar view
    - [x] fullcalendar
    - [x] htmgo
    - [x] default local timezone
    - [x] hide weekends
    - [x] show only working hours +/- 2 hours
- [x] ci/cd pipeline
- [x] fix recurring events (only first event is shown)
- [ ] compact overlapping events (implemented, default on — set `COMPACT_OVERLAPPING_EVENTS=false` to disable)
    - overlapping/touching non-recurring events merge into contiguous busy blocks
    - recurring events and recurrence overrides are left untouched
- [x] handle "EXDATE"s
    - EXDATE / RECURRENCE-ID are normalized to UTC so excluded occurrences really disappear
    - `STATUS:CANCELLED` overrides remove their occurrence from `/events.json`
- [x] set UTC timezone for **all** events
