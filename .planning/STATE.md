# Project State

**Project:** cal-anon-proxy Hardening  
**Initialized:** 2026-03-25  
**Current phase:** Phase 3 — Unit Tests + Fixtures

---

## Status

| Phase | Title | Status |
|---|---|---|
| 1 | Fix Critical Runtime Bugs | **✅ Complete** |
| 2 | Fix iCalendar Protocol Correctness | **✅ Complete** |
| 3 | Unit Tests + Fixtures | 🔄 In Progress (1/2 plans complete) |
| 4 | CI Tests + Cleanup | Pending |

---

## Key Decisions

- **Concurrency fix strategy:** `sync.RWMutex` on `CalDavHandler` (write lock in `SetEvents`, read lock on handler dispatch) — more readable than `atomic.Pointer[T]` for this struct-field case
- **REPORT fix strategy:** Store each event as individual `CalendarObject` in `SetEvents`; implement `QueryCalendarObjects` via `caldav.Filter()`
- **Test strategy:** `//go:build integration` tag on existing test; new `reader_unit_test.go` with `testdata/*.ics` fixtures; no mock library needed for pure function tests
- **Fixture gitignore bypass:** `*.ics` was in `.gitignore` (for live calendar data files like `caldav.ics`); testdata fixtures force-added via `git add -f` — fixtures must be versioned while live data stays ignored

---

## Codebase Context

- Module: `github.com/mheers/cal-anon-proxy`
- Entry point: `main.go`
- Core files: `reader.go` (fetch+sanitize), `calendar.go` (CalDAV backend), `middleware.go` (auth), `config.go` (env config)
- Test file: `reader_test.go` (to be tagged integration)
- Generated files (do not edit): `__htmgo/*.go`
- Build: `go build -tags prod ./...` for production binary

---

## Research Available

- `.planning/research/GO_CONCURRENCY.md` — mutex vs atomic patterns, HTTP timeouts, slice guards
- `.planning/research/GO_TESTING.md` — fixture patterns, build tags, table-driven tests
- `.planning/research/CALDAV_PROTOCOL.md` — EXDATE, REPORT/QueryCalendarObjects, all-day events, zero-duration

---

## Completed Work

### Phase 1 — Fix Critical Runtime Bugs (2026-03-25)
- `calendar.go`: `sync.RWMutex` on `CalDavHandler` — data race eliminated
- `calendar.go`: `calendarBackend.Calendar()` guards against empty slice panic
- `calendar.go`: `SetEvents` now stores one `CalendarObject` per event (not merged)
- `calendar.go`: `QueryCalendarObjects` implemented via `caldav.Filter` — REPORT works
- `reader.go`: HTTP client timeout — 30s client + 5s DialContext + 5s TLS
- `reader.go`: `calendars[0]` guarded with descriptive error when upstream empty
- `reader_test.go`: Tagged `//go:build integration` — CI no longer needs credentials

### Phase 2 — Fix iCalendar Protocol Correctness (2026-03-25)
- `reader.go`: `toTZ` — VALUE=DATE early return guard — all-day events preserved as-is (REQ-05)
- `reader.go`: `harmonizeDurationAndEnd` — zero-duration events set DTEND=DTSTART instead of error (REQ-06)
- `reader.go`: `CompRequest.Props` — added `"EXDATE"` and `"RECURRENCE-ID"` (REQ-07)

### Phase 3 — Unit Tests + Fixtures (2026-03-25)
- `reader_test.go`: `//go:build integration` confirmed on line 1 — CI excludes integration test without credentials
- `testdata/event_with_duration.ics`: VEVENT with DTSTART + DURATION:PT1H, no DTEND
- `testdata/event_with_dtend.ics`: VEVENT with DTSTART + DTEND (SUMMARY=Team Meeting)
- `testdata/event_allday.ics`: All-day event with DTSTART;VALUE=DATE
- `testdata/event_ms_timezone.ics`: VEVENT with TZID=Eastern Standard Time (MS timezone)
- `testdata/event_no_summary.ics`: VEVENT intentionally missing SUMMARY
- `testdata/event_zero_duration.ics`: VEVENT with DTSTART only (zero-duration case)

---

## Next Action

Execute plan 03-02: Write `reader_unit_test.go` with table-driven unit tests for `summaryOfEvent`, `harmonizeDurationAndEnd`, and `toTZ` using the testdata fixtures.
