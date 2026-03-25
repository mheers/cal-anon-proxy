# Project State

**Project:** cal-anon-proxy Hardening  
**Initialized:** 2026-03-25  
**Current phase:** Not started — ready for Phase 1

---

## Status

| Phase | Title | Status |
|---|---|---|
| 1 | Fix Critical Runtime Bugs | **Ready to start** |
| 2 | Fix iCalendar Protocol Correctness | Pending |
| 3 | Unit Tests + Fixtures | Pending |
| 4 | CI Tests + Cleanup | Pending |

---

## Key Decisions

- **Concurrency fix strategy:** `sync.RWMutex` on `CalDavHandler` (write lock in `SetEvents`, read lock on handler dispatch) — more readable than `atomic.Pointer[T]` for this struct-field case
- **REPORT fix strategy:** Store each event as individual `CalendarObject` in `SetEvents`; implement `QueryCalendarObjects` via `caldav.Filter()`
- **Test strategy:** `//go:build integration` tag on existing test; new `reader_unit_test.go` with `testdata/*.ics` fixtures; no mock library needed for pure function tests
- **HTTP timeout:** `30s` client-level + transport-level `DialContext` 5s + `TLSHandshakeTimeout` 5s

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

## Next Action

Run `/gsd-plan-phase 1` to create a detailed execution plan for Phase 1.
