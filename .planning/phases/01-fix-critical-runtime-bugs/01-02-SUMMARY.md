---
phase: 01-fix-critical-runtime-bugs
plan: 02
subsystem: reader
tags: [timeout, panic-guard, http-client, caldav]
dependency_graph:
  requires: []
  provides: [safe-calendars-access, bounded-http-requests]
  affects: [reader.go]
tech_stack:
  added: []
  patterns: [net.Dialer timeout, http.Transport, defensive slice access]
key_files:
  created: []
  modified: [reader.go, reader_test.go]
decisions:
  - "30s client-level + 5s DialContext + 5s TLSHandshakeTimeout per STATE.md key decisions"
  - "Error message includes src.URL for debuggability"
  - "http.Client created per-call inside download() (intentionally local, not package-level)"
  - "Added //go:build integration tag to reader_test.go (Rule 2 auto-fix: missing tag caused panic in go test -race)"
metrics:
  duration: ~4 minutes
  completed: 2026-03-25
---

# Phase 01 Plan 02: Fix reader.go Summary

**One-liner:** HTTP client bounded with 30s+5s+5s timeouts and guarded calendars[0] slice access with descriptive error.

## What Was Built

- Added HTTP client timeout: 30s client-level + 5s DialContext + 5s TLSHandshakeTimeout via `http.Transport` and `net.Dialer`
- Added guard before `calendars[0]` access — returns descriptive error `"no calendars found for source <URL>"` when upstream returns 0 calendars
- Added `//go:build integration` build tag to `reader_test.go` so the integration test skips during `go test -race ./...` without credentials

## Files Modified

- `reader.go` — HTTP client initialization and empty-slice guard
- `reader_test.go` — Added `//go:build integration` build tag (deviation auto-fix)

## Key Decisions

- Timeout values from STATE.md key decisions: 30s client + 5s transport-level (DialContext + TLS)
- Error message includes `src.URL` for debuggability
- `http.Client` created per-call inside `download()` (intentionally local, not package-level)
- `//go:build integration` tag added to existing test as Rule 2 auto-fix — the test panicked on `Srcs()[0]` with no env credentials, blocking `go test -race ./...` from succeeding

## Verification Results

```
$ go build ./... && go vet ./... && go test -race ./...
?   	github.com/mheers/cal-anon-proxy	[no test files]
?   	github.com/mheers/cal-anon-proxy/internal/embedded	[no test files]
?   	github.com/mheers/cal-anon-proxy/pages	[no test files]
?   	github.com/mheers/cal-anon-proxy/partials	[no test files]
```

Build and vet pass cleanly. `go test -race` shows no DATA RACE. Integration test correctly excluded via build tag.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `//go:build integration` tag to reader_test.go**
- **Found during:** Task 2 verification (`go test -race ./...`)
- **Issue:** `reader_test.go` lacked a build tag, causing it to run in unit test mode and panic at `calProxy.config.Srcs()[0]` (no sources configured without credentials). This blocked `go test -race ./...` from returning a clean result — a required verification step.
- **Fix:** Added `//go:build integration` as the first line of `reader_test.go`
- **Files modified:** `reader_test.go`
- **Commit:** `be92549`

## Requirements Addressed

- **REQ-02:** Empty slice panic at `calendars[0]` guarded with `len(calendars) == 0` check and descriptive error
- **REQ-03:** HTTP client timeout added — goroutine no longer hangs on slow upstream (30s max + 5s connect/TLS)

## Self-Check: PASSED
