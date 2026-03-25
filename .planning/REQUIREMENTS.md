# Requirements: cal-anon-proxy Hardening

**Created:** 2026-03-25  
**Scope:** Bug fixes, reliability hardening, test coverage, cleanup  
**Out of scope:** New user-facing features, UI redesign, database integration

---

## REQ-01: Fix Data Race on CalDAV Backend

**Priority:** Critical  
**Source:** CONCERNS.md — Race condition HIGH; GO_CONCURRENCY.md research

The background sync goroutine calls `calDavHandler.SetEvents(events)` which replaces `h.Backend` with a new `*calendarBackend` pointer. Concurrent HTTP handlers read `h.Backend` without any synchronization — this is a data race detectable by `go test -race`.

**Acceptance criteria:**
- `SetEvents` and all HTTP handler reads of the backend are synchronized
- Use `sync.RWMutex` on `CalDavHandler` (write lock in `SetEvents`, read lock before accessing `h.Handler`)
- `go test -race ./...` produces no race condition warnings
- Behavior of the CalDAV endpoint is unchanged

---

## REQ-02: Guard All Unchecked Slice Index Accesses

**Priority:** Critical  
**Source:** CONCERNS.md — Panic guards HIGH; GO_CONCURRENCY.md §5

`reader.go:45` (`calendars[0]`) and `calendar.go:33` will panic with an index-out-of-range if the upstream CalDAV account returns zero calendars. This crashes the server process.

**Acceptance criteria:**
- `reader.go`: `len(calendars) == 0` guard before `calendars[0]`; return descriptive `fmt.Errorf` if empty
- `calendar.go`: equivalent guard before any slice index
- No panics on empty-calendar responses; error is logged and sync is retried on next tick
- Unit test covers the empty-calendars case (via mock/fixture)

---

## REQ-03: Add HTTP Client Timeout

**Priority:** Critical  
**Source:** CONCERNS.md — HTTP client timeout HIGH; GO_CONCURRENCY.md §4

`reader.go:28` creates `&http.Client{}` with no timeout. A slow or unresponsive upstream CalDAV server blocks the refresh goroutine indefinitely, starving the proxy of updates.

**Acceptance criteria:**
- HTTP client created with `Timeout: 30 * time.Second` (or configurable via `SRC_TIMEOUT_SECONDS` env var)
- Transport also sets `DialContext` timeout (5s) and `TLSHandshakeTimeout` (5s)
- If upstream times out, `download()` returns an error, `updateEvents` logs it and continues to next tick

---

## REQ-04: Fix `QueryCalendarObjects` — Implement REPORT Support

**Priority:** Critical  
**Source:** CALDAV_PROTOCOL.md §2; CONCERNS.md `QueryCalendarObjects` MEDIUM

`calendar.go:83-85` returns `nil, nil` for all CalDAV REPORT queries. Most CalDAV clients (Apple Calendar, Thunderbird) use REPORT for initial sync and incremental updates — they receive zero events from the proxy.

**Acceptance criteria:**
- `QueryCalendarObjects` implemented using `caldav.Filter(query, objects)` from the go-webdav library
- Each event stored as an individual `CalendarObject` in `SetEvents` (not one merged mega-object) so time-range filtering operates correctly per-event
- Apple Calendar and Thunderbird successfully sync events via REPORT

---

## REQ-05: Fix All-Day Event Corruption in `toTZ`

**Priority:** High  
**Source:** CALDAV_PROTOCOL.md §5a

`toTZ` calls `prop.DateTime(tz)` then `props.SetDateTime(...)`, which always writes `DATE-TIME` format. All-day events with `DTSTART;VALUE=DATE` are silently converted to `00:00:00` timed events in Europe/London — corrupting the event.

**Acceptance criteria:**
- `toTZ` detects `VALUE=DATE` properties and returns early (no conversion needed for all-day events)
- All-day events pass through the proxy with `VALUE=DATE` format preserved
- Unit test: fixture with an all-day event verifies `VALUE=DATE` is preserved after processing

---

## REQ-06: Fix `harmonizeDurationAndEnd` for Zero-Duration Events

**Priority:** High  
**Source:** CALDAV_PROTOCOL.md §5b

When a VEVENT has `DTSTART` but neither `DTEND` nor `DURATION`, `harmonizeDurationAndEnd` returns an error, causing the entire source to fail. RFC 5545 says this is a valid zero-duration event (e.g. birthday reminders).

**Acceptance criteria:**
- Missing both DTEND and DURATION is treated as zero duration: `DTEND = DTSTART`
- No error returned for valid zero-duration events
- Unit test covers this case

---

## REQ-07: Add `EXDATE` and `RECURRENCE-ID` to Upstream CalDAV Query

**Priority:** High  
**Source:** CALDAV_PROTOCOL.md §1

`reader.go` does not request `EXDATE` or `RECURRENCE-ID` from upstream servers. Without `EXDATE`, deleted occurrences of recurring events remain visible in the proxy. Without `RECURRENCE-ID`, modified single occurrences revert to the master event content.

**Acceptance criteria:**
- `"EXDATE"` and `"RECURRENCE-ID"` added to `CompRequest.Props` in `reader.go`
- Proxy correctly forwards exception/override VEVENTs to CalDAV clients

---

## REQ-08: Replace Live Integration Test with Fixture-Based Unit Tests

**Priority:** High  
**Source:** CONCERNS.md — Test coverage HIGH; TESTING.md; GO_TESTING.md

The only test (`TestDownload`) requires live CalDAV credentials and hardcodes `require.Len(t, events, 2)` — it fails in CI without secrets and breaks on any upstream data change. There are zero isolated unit tests.

**Acceptance criteria:**
- `reader_test.go` tagged `//go:build integration` — no longer runs in `go test ./...`
- `testdata/` directory with `.ics` fixture files covering: event with DURATION, event with DTEND, all-day event, event with MS timezone, event without SUMMARY, zero-duration event
- `reader_unit_test.go` with table-driven unit tests for:
  - `harmonizeDurationAndEnd` — all fixture cases
  - `toTZ` — UTC event, MS timezone event, all-day event
  - `summaryOfEvent` — event with/without summary
- Tests pass with `go test ./...` (no credentials needed)
- `saveEvents` in integration test uses `t.TempDir()` not CWD

---

## REQ-09: Run Tests in CI

**Priority:** High  
**Source:** TESTING.md — Tests NOT run in CI

Tests are never run automatically. Any push to main can silently break the codebase.

**Acceptance criteria:**
- `.github/workflows/main.yml` includes a `go test ./...` step before the Dagger build step
- `go test -race ./...` run in CI (catches REQ-01)
- Integration tests (requiring credentials) are excluded via build tag (no secrets required for CI pass)

---

## REQ-10: Fix FullCalendar Version Mismatch

**Priority:** Medium  
**Source:** CONCERNS.md — FullCalendar version mismatch MEDIUM

`pages/index.go` loads FullCalendar CSS at `v5.5.1` but JS at `v6.1.15`. Mixing major versions of CSS and JS from the same library causes visual or functional breakage.

**Acceptance criteria:**
- Both CSS and JS CDN references use FullCalendar `v6.1.15`
- Calendar UI renders correctly after the change

---

## REQ-11: Remove Scaffold and Dead Code

**Priority:** Medium  
**Source:** CONCERNS.md — LOW severity items combined

Multiple items of dead/unused code add cognitive overhead:
- `partials/index.go` — htmgo demo `CounterPartial` (unrelated to app; registered as HTTP route)
- `tracingMiddleware` in `middleware.go` — defined but never called
- `allowedEvents` and `renameEvents` in `reader.go` — always empty; filter blocks never execute
- Commented-out code blocks in `reader.go:177-184`, `pages/index.go:95-108`, `middleware.go:49,59`

**Acceptance criteria:**
- `partials/index.go` deleted; htmgo routes regenerated
- `tracingMiddleware` removed from `middleware.go`
- Dead `allowedEvents`/`renameEvents` blocks removed from `reader.go`
- Commented-out code blocks removed
- All existing functionality continues to work after cleanup
- `go build ./...` passes cleanly

---

## REQ-12: Consistent Logging

**Priority:** Low  
**Source:** CONCERNS.md — Mixed logging LOW

`fmt.Printf` and `logrus` are used inconsistently. `fmt.Printf` output cannot be silenced or leveled.

**Acceptance criteria:**
- All `fmt.Printf`/`fmt.Println` calls in application code replaced with appropriate `logrus.Debugf`/`logrus.Infof`
- `fmt` package removed from import in files where it was only used for logging

---

## Out of Scope

- Config refactor (SRC_1..4 to slice-based) — low risk, deferred
- Configurable time window (SRC_LOOKBACK_DAYS etc.) — feature, deferred
- Calendar selection by name (SRC_N_CALENDAR_NAME) — feature, deferred
- TLS enforcement / HTTPS redirect — infrastructure concern
- Persistent event storage — architecture change, deferred
- Error isolation between sources in `downloadAll` — medium complexity, deferred to later phase
