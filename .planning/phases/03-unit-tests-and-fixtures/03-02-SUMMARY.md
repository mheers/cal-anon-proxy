---
phase: 03-unit-tests-and-fixtures
plan: 02
subsystem: testing
tags: [unit-tests, table-driven, fixtures, tdd, no-build-tag]
dependency_graph:
  requires: [testdata-fixtures]
  provides: [unit-tests-green, reader-unit-test]
  affects: [ci-pipeline]
tech_stack:
  added: []
  patterns: [table-driven-tests, fixture-loading-helper, subtests]
key_files:
  created:
    - reader_unit_test.go
  modified: []
decisions:
  - "No build tag on reader_unit_test.go — runs automatically in go test ./... without credentials"
  - "Used strings.Contains for DTEND value checks to be robust against timezone suffix variations"
  - "harmonizeDurationAndEnd 'zero-duration' path checked separately from 'DURATION present' path"
  - "toTZ 'all-day' case asserts prop.Value unchanged ('20260325') — confirmed early-return guard works"
metrics:
  duration: 95s
  completed: "2026-03-25"
  tasks_completed: 1
  files_modified: 1
---

# Phase 3 Plan 02: Reader Unit Tests Summary

**One-liner:** Fixture-based, table-driven unit tests for `summaryOfEvent`, `harmonizeDurationAndEnd`, and `toTZ` using 6 testdata ICS fixtures; 8 subtests across 3 functions, all green with no credentials needed.

---

## What Was Done

### Task 1: Write reader_unit_test.go (TDD)

Created `reader_unit_test.go` in `package main` with:
- `loadFixture` helper (opens ICS file, decodes via go-ical, wraps in `caldav.CalendarObject`)
- `TestHarmonizeDurationAndEnd` — 3 table-driven subtests
- `TestToTZ` — 3 table-driven subtests
- `TestSummaryOfEvent` — 2 table-driven subtests

No build tag — runs in `go test ./...` without any credentials or network access.

**TDD cycle:** Tests were written first, then run. All 8 subtests passed on the first run (functions already implemented correctly in Phases 1 and 2).

---

## Test Coverage

### TestHarmonizeDurationAndEnd (3 subtests)

| Subtest | Fixture | Assertion |
|---------|---------|-----------|
| `DTEND already present — no-op` | `event_with_dtend.ics` | No error; DTEND value still contains "110000" |
| `DURATION present — compute DTEND` | `event_with_duration.ics` | No error; DTEND contains "100000" (09:00Z+1h); DURATION prop deleted |
| `zero-duration — DTEND equals DTSTART` | `event_zero_duration.ics` | No error; DTEND prop set and contains "000000" |

### TestToTZ (3 subtests)

| Subtest | Fixture | Assertion |
|---------|---------|-----------|
| `UTC event — convert to London` | `event_with_dtend.ics` | No error; TZID param = "Europe/London" |
| `MS timezone — translated to London` | `event_ms_timezone.ics` | No error; TZID param = "Europe/London" (target tz, not IANA translation) |
| `all-day — skip conversion` | `event_allday.ics` | No error; prop value still "20260325" (unchanged) |

### TestSummaryOfEvent (2 subtests)

| Subtest | Fixture | Assertion |
|---------|---------|-----------|
| `has SUMMARY` | `event_with_dtend.ics` | Returns "Team Meeting" |
| `no SUMMARY` | `event_no_summary.ics` | Returns "" |

---

## Edge Cases Discovered

### harmonizeDurationAndEnd — DURATION=PT1H path has a subtle math check

The implementation (lines 298–305 of reader.go) has an intentional guard:
```go
if durationTime == 0 {
    return nil  // zero-duration via DURATION=PT0S → no DTEND set, no delete
}
if endTime.Sub(endTime.Add(durationTime)) != 0 {
    // only sets DTEND and deletes DURATION if duration is non-zero
}
```

This means a `DURATION:PT0S` value would **not** set DTEND (early return) AND would **not** delete DURATION. The `event_with_duration.ics` uses `PT1H` so the condition is true and DTEND is correctly set + DURATION deleted.

The "zero-duration" test case (`event_zero_duration.ics`) exercises the separate code path where DURATION prop is **absent entirely** (nil), which sets DTEND=DTSTART directly.

### toTZ — MS timezone TZID is set to the TARGET timezone, not the translated IANA name

When input has `TZID=Eastern Standard Time`:
1. `TranslateMSTimezoneToIANA("Eastern Standard Time")` → IANA name (for parsing)
2. After converting datetime to target tz: `prop.Params.Set(TZID, tz.String())` → sets TZID to **target** tz ("Europe/London"), not the translated IANA name
3. `SetDateTime(propName, dateTime.In(tz))` overwrites the prop with the London-localized value

The test correctly asserts TZID = "Europe/London" (not "America/New_York").

---

## Final Test Run Results

```
=== RUN   TestHarmonizeDurationAndEnd
=== RUN   TestHarmonizeDurationAndEnd/DTEND_already_present_—_no-op
=== RUN   TestHarmonizeDurationAndEnd/DURATION_present_—_compute_DTEND
=== RUN   TestHarmonizeDurationAndEnd/zero-duration_—_DTEND_equals_DTSTART
--- PASS: TestHarmonizeDurationAndEnd (0.00s)
    --- PASS: TestHarmonizeDurationAndEnd/DTEND_already_present_—_no-op (0.00s)
    --- PASS: TestHarmonizeDurationAndEnd/DURATION_present_—_compute_DTEND (0.00s)
    --- PASS: TestHarmonizeDurationAndEnd/zero-duration_—_DTEND_equals_DTSTART (0.00s)
=== RUN   TestToTZ
=== RUN   TestToTZ/UTC_event_—_convert_to_London
=== RUN   TestToTZ/MS_timezone_—_translated_to_London
=== RUN   TestToTZ/all-day_—_skip_conversion
--- PASS: TestToTZ (0.01s)
    --- PASS: TestToTZ/UTC_event_—_convert_to_London (0.00s)
    --- PASS: TestToTZ/MS_timezone_—_translated_to_London (0.00s)
    --- PASS: TestToTZ/all-day_—_skip_conversion (0.00s)
=== RUN   TestSummaryOfEvent
=== RUN   TestSummaryOfEvent/has_SUMMARY
=== RUN   TestSummaryOfEvent/no_SUMMARY
--- PASS: TestSummaryOfEvent (0.00s)
    --- PASS: TestSummaryOfEvent/has_SUMMARY (0.00s)
    --- PASS: TestSummaryOfEvent/no_SUMMARY (0.00s)
PASS
ok  github.com/mheers/cal-anon-proxy 0.017s

$ go test ./... → PASS (exit 0)
$ go test -race ./... → PASS (exit 0)
$ go test -tags integration ./... → FAIL (expected — no credentials, but compiles)
```

---

## Deviations from Plan

None — plan executed exactly as written. All 8 subtests passed on first run.

---

## Files Created

| File | Lines | Description |
|------|-------|-------------|
| `reader_unit_test.go` | 176 | Fixture-based table-driven unit tests (no build tag) |

---

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| Task 1 | 703a4c1 | `test(03-02): add fixture-based unit tests for summaryOfEvent, harmonizeDurationAndEnd, toTZ` |

---

## Self-Check: PASSED

- [x] `reader_unit_test.go` exists at project root
- [x] Commit 703a4c1 confirmed in git log
- [x] `go test ./...` exits 0
- [x] `go test -race ./...` exits 0
- [x] All 8 subtests PASS
