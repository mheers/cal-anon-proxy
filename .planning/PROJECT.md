# Project: cal-anon-proxy Hardening

**Created:** 2026-03-25  
**Type:** Brownfield — existing codebase  
**Module:** `github.com/mheers/cal-anon-proxy`

---

## What This Project Is

`cal-anon-proxy` is a CalDAV reverse proxy written in Go. It:
- Fetches calendar events from up to 4 upstream CalDAV servers (Nextcloud, Exchange, etc.)
- Strips personal information / anonymizes event summaries
- Normalizes timezones to Europe/London
- Re-serves the processed events as a CalDAV server and a browser-based calendar viewer (FullCalendar.js via htmgo)
- Optionally protects the exposed endpoint with HTTP Basic Auth

**Current state:** The core proxy works but has multiple bugs, race conditions, and zero isolated unit tests. The codebase was rapidly prototyped and has not been hardened for production reliability.

---

## Goal

Harden the codebase to be production-reliable:
- Fix all HIGH-severity bugs (race condition, panics, no HTTP timeouts, broken CalDAV REPORT)
- Fix MEDIUM-severity bugs (all-day event corruption, error isolation, FullCalendar version mismatch)
- Replace the fragile live-credential integration test with fixture-based unit tests
- Clean up dead code and scaffolding
- Run tests in CI

---

## Technology Stack

- **Language:** Go 1.23.2
- **Web framework:** htmgo v1.0.6 (SSR, htmgo-chi router, code-generated routing)
- **CalDAV:** `github.com/mheers/go-webdav` (fork of emersion/go-webdav)
- **iCalendar:** `github.com/emersion/go-ical`
- **Logging:** `github.com/sirupsen/logrus`
- **Config:** `github.com/sethvargo/go-envconfig`
- **Testing:** Standard `testing` + `github.com/stretchr/testify`
- **Build:** htmgo CLI + Taskfile + Tailwind CSS
- **CI:** GitHub Actions → Dagger (TypeScript) → Docker Hub

---

## Key Constraints

- **No database** — all event state is in-memory, replaced on each sync cycle
- **Read-only CalDAV** — write operations are intentionally no-ops
- **Single binary** — deployed as a Docker container
- **Go 1.23+** — can use `atomic.Pointer[T]` (Go 1.19+)
- **Existing API surface must not change** — CalDAV clients already configured to use `/caldav/`

---

## Known High-Severity Issues (starting point)

| Issue | File | Fix |
|---|---|---|
| Data race: `SetEvents` swaps `h.Backend` without sync | `calendar.go:120` | Use `sync.RWMutex` or `atomic.Pointer[T]` |
| Panic: `calendars[0]` with no nil guard | `reader.go:45`, `calendar.go:33` | Add `len(calendars) == 0` guard |
| Panic: unchecked slice in test | `reader_test.go:14` | Guard + build tag |
| No HTTP client timeout — goroutine hangs forever | `reader.go:28` | `http.Client{Timeout: 30s}` |
| `QueryCalendarObjects` returns nil — REPORT gives empty calendar | `calendar.go:83` | Implement using `caldav.Filter()` |
| Single mega-CalendarObject breaks time-range filter | `calendar.go:SetEvents` | Store one object per event |
| Live integration test fails in CI without secrets | `reader_test.go` | `//go:build integration` tag |
