---
phase: 01-fix-critical-runtime-bugs
plan: "01"
subsystem: caldav
tags: [concurrency, bugfix, caldav, race-condition]
dependency_graph:
  requires: []
  provides: [thread-safe-caldav-handler, per-event-calendar-objects, caldav-filter-query]
  affects: [calendar.go]
tech_stack:
  added: [sync.RWMutex]
  patterns: [read-write-lock, minimal-critical-section, per-item-storage]
key_files:
  created: []
  modified: [calendar.go]
decisions:
  - Used sync.RWMutex (per project decision in STATE.md) — more readable than atomic.Pointer for this struct-field case
  - Lock wraps only Backend pointer swap, not the entire request (minimal critical section)
  - HTTPHandler() returns h itself since CalDavHandler now implements http.Handler via ServeHTTP
  - SetEvents stores N CalendarObjects for N events (not all merged into one) to enable per-event filtering
metrics:
  duration: "143 seconds"
  completed: "2026-03-25T09:59:47Z"
  tasks_completed: 3
  files_modified: 1
---

# Phase 01 Plan 01: Fix calendar.go Summary

## One-Liner

Thread-safe CalDavHandler with per-event CalendarObjects and caldav.Filter-based REPORT query handling.

## What Was Built

- Added `sync.RWMutex` to `CalDavHandler` for thread-safe `Backend` access
- `ServeHTTP` method reads the inner handler under `RLock`, allowing concurrent requests
- `SetEvents` acquires `Lock` only for the Backend pointer swap (builds new backend outside lock)
- `HTTPHandler()` now returns `h` itself since `CalDavHandler` implements `http.Handler` via `ServeHTTP`
- `calendarBackend.Calendar()` guards against empty slice with a descriptive error
- `SetEvents` now stores one `CalendarObject` per source event (not all merged into one); paths derived from UID or index fallback
- `QueryCalendarObjects` implemented using `caldav.Filter(query, objects)` for correct REPORT filtering

## Files Modified

- `calendar.go`

## Key Decisions

- **sync.RWMutex** chosen per project decision in STATE.md — more readable than `atomic.Pointer[T]` for this struct-field case
- **Lock only wraps the Backend pointer swap**, not the entire request — minimal critical section, maximum concurrency
- **HTTPHandler() returns h** (self) since `CalDavHandler` now implements `http.Handler` via `ServeHTTP` — `main.go` needed no changes as it was already using `handler = a.middleware(calDavHandler)` which passes the full `CalDavHandler`
- **Per-event objects** in `SetEvents` so `QueryCalendarObjects` can filter individual events via `caldav.Filter`

## Verification Results

```
$ go build ./...
(no output — clean)

$ go vet ./...
(no output — clean)

$ go test -race ./...
?   	github.com/mheers/cal-anon-proxy	[no test files]
?   	github.com/mheers/cal-anon-proxy/internal/embedded	[no test files]
?   	github.com/mheers/cal-anon-proxy/pages	[no test files]
?   	github.com/mheers/cal-anon-proxy/partials	[no test files]
(no DATA RACE output)
```

## Deviations from Plan

None — plan executed exactly as written.

## Requirements Addressed

- **REQ-01:** Data race on `CalDavHandler.Backend` fixed via `sync.RWMutex` — `ServeHTTP` reads under `RLock`, `SetEvents` writes under `Lock`
- **REQ-02:** Empty slice panic in `Calendar()` guarded with `len(b.calendars) == 0` check returning descriptive error
- **REQ-04:** `QueryCalendarObjects` implemented via `caldav.Filter`; `SetEvents` stores individual `CalendarObject` per event enabling proper REPORT filtering

## Self-Check: PASSED

- [x] `calendar.go` modified with all three fixes
- [x] Commit `8627352` — Task 1 (sync.RWMutex + ServeHTTP)
- [x] Commit `1e622a5` — Task 2 (Calendar() guard)
- [x] Commit `22f6b8c` — Task 3 (QueryCalendarObjects via caldav.Filter)
- [x] `go build ./...` passes cleanly
- [x] `go vet ./...` passes cleanly
- [x] `go test -race ./...` shows no DATA RACE
