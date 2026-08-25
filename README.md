# cal-anon-proxy

This is a simple CalDAV server written in Go that proxies read requests to a real CalDAV server, but anonymizes the responses by removing all personal information from the events.

# Usage

## Run

```bash
docker compose up
```

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
- [ ] when a source event is deleted, delete the event from the proxy (thunderbird still shows the event)
- [x] frontend with calendar view
    - [x] fullcalendar
    - [x] htmgo
    - [x] default local timezone
    - [x] hide weekends
    - [x] show only working hours +/- 2 hours
- [x] ci/cd pipeline
- [x] fix recurring events (only first event is shown)
- [ ] compact overlapping events
- [ ] handle "EXDATE"s
- [x] set UTC timezone for **all** events
