# Roadmap: cal-anon-proxy Hardening

**Created:** 2026-03-25  
**Goal:** Production-reliable CalDAV proxy — no crashes, no races, correct REPORT support, tested  
**Strategy:** Critical bugs first → protocol correctness → tests → CI → cleanup

---

## Phase 1: Fix Critical Runtime Bugs

**Goal:** The proxy no longer crashes, hangs, or silently serves empty calendars.

**Covers:** REQ-01, REQ-02, REQ-03, REQ-04

**Plans:** 1/2 plans executed

Plans:
- [ ] 01-01-PLAN.md — Fix data race, empty-slice panic, and QueryCalendarObjects in calendar.go (Wave 1)
- [ ] 01-02-PLAN.md — Add HTTP client timeout and guard calendars slice in reader.go (Wave 1)

### Tasks

1. **Fix data race on `CalDavHandler.Backend`** (`calendar.go`)
   - Add `sync.RWMutex mu` field to `CalDavHandler`
   - Wrap `h.Handler` dispatch in `RLock/RUnlock`
   - Wrap `SetEvents` pointer swap in `Lock/Unlock`
   - Verify with `go test -race ./...`

2. **Add empty-slice guards** (`reader.go`, `calendar.go`)
   - `reader.go:45`: guard `len(calendars) == 0`, return `fmt.Errorf`
   - `calendar.go:33`: equivalent guard
   - Both return descriptive errors; `updateEvents` logs and continues

3. **Add HTTP client timeout** (`reader.go`)
   - Replace `&http.Client{}` with client having `Timeout: 30*time.Second`
   - Add `Transport` with `DialContext` (5s) and `TLSHandshakeTimeout` (5s)

4. **Implement `QueryCalendarObjects`** (`calendar.go`)
   - Change `SetEvents` to store each `*caldav.CalendarObject` individually (not merged) in `objectMap`
   - Implement `QueryCalendarObjects` using `caldav.Filter(query, objects)`
   - Verify Apple Calendar / Thunderbird sync works via REPORT

**Definition of done:**
- `go build ./...` clean
- `go test -race ./...` passes with no races
- Server starts and serves CalDAV without panicking on empty upstream responses
- CalDAV REPORT requests return events (not empty)

---

## Phase 2: Fix iCalendar Protocol Correctness

**Goal:** All-day events pass through correctly; deleted recurring occurrences are honoured.

**Covers:** REQ-05, REQ-06, REQ-07

**Plans:** 1 plan

Plans:
- [ ] 02-01-PLAN.md — Fix toTZ DATE guard, harmonizeDurationAndEnd zero-duration, and EXDATE/RECURRENCE-ID props (Wave 1)

### Tasks

1. **Fix all-day event corruption in `toTZ`** (`reader.go`)
   - Detect `VALUE=DATE` in DTSTART/DTEND props before calling `toTZ`
   - Return early (no timezone conversion) for DATE-type properties
   - Write fixture: `testdata/event_allday.ics` for later test coverage

2. **Fix zero-duration event handling in `harmonizeDurationAndEnd`** (`reader.go`)
   - When VEVENT has DTSTART but neither DTEND nor DURATION: set `DTEND = DTSTART`
   - Return `nil` (not error) for this valid RFC 5545 case

3. **Add `EXDATE` and `RECURRENCE-ID` to upstream query** (`reader.go`)
   - Add `"EXDATE"` and `"RECURRENCE-ID"` to `CompRequest.Props`
   - Verify deleted single occurrences of recurring events no longer appear in proxy

**Definition of done:**
- All-day events served with `VALUE=DATE` format preserved
- Zero-duration events no longer cause sync errors
- `EXDATE` is requested from upstream; deleted occurrences not served

---

## Phase 3: Unit Tests + Integration Test Isolation

**Goal:** `go test ./...` passes without credentials; all core logic is tested with fixtures.

**Covers:** REQ-08

**Plans:** 1/2 plans executed

Plans:
- [ ] 03-01-PLAN.md — Tag integration test + create testdata/ fixture files (Wave 1)
- [ ] 03-02-PLAN.md — Write reader_unit_test.go with table-driven unit tests (Wave 2)

### Tasks

1. **Tag integration test** (`reader_test.go`)
   - Add `//go:build integration` to top of file
   - Fix `saveEvents` to use `t.TempDir()` path instead of CWD

2. **Create `testdata/` fixtures**
   - `testdata/event_with_duration.ics` — DTSTART + DURATION, no DTEND
   - `testdata/event_with_dtend.ics` — DTSTART + DTEND, no DURATION
   - `testdata/event_allday.ics` — DTSTART VALUE=DATE (all-day)
   - `testdata/event_ms_timezone.ics` — TZID="Eastern Standard Time"
   - `testdata/event_no_summary.ics` — missing SUMMARY
   - `testdata/event_zero_duration.ics` — DTSTART only, no DTEND/DURATION

3. **Write `reader_unit_test.go`** (no build tag — runs in plain `go test`)
   - `loadFixture(t, path)` helper
   - `TestHarmonizeDurationAndEnd` — table-driven, all fixture cases
   - `TestToTZ` — UTC, MS timezone, all-day skip
   - `TestSummaryOfEvent` — with/without SUMMARY

**Definition of done:**
- `go test ./...` passes with zero credentials
- All new unit tests pass
- All fixture edge cases covered
- `go test -tags integration ./...` still works (with credentials)

---

## Phase 4: CI Tests + Cleanup

**Goal:** Every push is tested automatically; dead code is removed.

**Covers:** REQ-09, REQ-10, REQ-11, REQ-12

### Tasks

1. **Add test step to GitHub Actions** (`.github/workflows/main.yml`)
   - Add `go test -race ./...` step before Dagger build
   - No secrets required (integration tests excluded by build tag)

2. **Fix FullCalendar version mismatch** (`pages/index.go`)
   - Update CSS CDN reference from `v5.5.1` to `v6.1.15`
   - Verify calendar UI renders correctly

3. **Remove scaffold and dead code**
   - Delete `partials/index.go` (demo CounterPartial)
   - Regenerate `__htmgo/partials-generated.go` via `htmgo build`
   - Remove `tracingMiddleware` from `middleware.go`
   - Remove `allowedEvents`, `renameEvents` dead blocks from `reader.go`
   - Remove commented-out code from `reader.go`, `pages/index.go`, `middleware.go`

4. **Consistent logging** (`reader.go`, `main.go`)
   - Replace all `fmt.Printf` diagnostic prints with `logrus.Debugf`/`logrus.Infof`
   - Remove `fmt` import where it's no longer used

**Definition of done:**
- CI runs `go test -race ./...` on every push to main
- FullCalendar renders correctly (CSS + JS same version)
- `go build ./...` and `go vet ./...` clean
- No commented-out code, no dead code blocks
- All log output routed through logrus

---

## Phase Summary

| Phase | Focus | Requirements | Risk |
|---|---|---|---|
| 1 | 1/2 | In Progress|  |
| 2 | Protocol correctness | REQ-05, 06, 07 | Medium — iCal edge cases |
| 3 | 1/2 | In Progress|  |
| 4 | CI + cleanup | REQ-09, 10, 11, 12 | Low — mostly removal |

**Estimated total:** 4 focused phases, each committable independently.  
**Start with:** `/gsd-plan-phase 1`
