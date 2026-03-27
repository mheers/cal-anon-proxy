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
