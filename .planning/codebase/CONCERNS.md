# Codebase Concerns

**Analysis Date:** 2026-03-25

---

## Security Considerations

**HTTP Basic Auth over plaintext:**
- Risk: Credentials transmitted in cleartext if TLS is not enforced at the infrastructure level. The app itself does not redirect HTTP → HTTPS nor enforce TLS.
- Files: `middleware.go:29`, `reader.go:29`
- Current mitigation: Comment in code says "do NOT store passwords in plaintext" but the auth middleware stores and compares credentials directly from `Config` (loaded from env vars). No TLS enforcement in the Go app itself.
- Recommendations: Document TLS requirement prominently; add a `DstTLSRequired` guard or at minimum a startup warning when TLS is not detected.
- Severity: **MEDIUM**

**LiveReload enabled unconditionally in production binary:**
- Risk: htmgo's `LiveReload: true` injects a WebSocket endpoint intended for development hot-reload. Shipping this in a production binary exposes an unnecessary attack surface and leaks implementation details.
- Files: `main.go:46`
- Recommendations: Gate on a `DEV` or `APP_ENV` env var; disable for production builds.
- Severity: **MEDIUM**

**Calendar marked `editable: true` and `selectable: true` in FullCalendar UI:**
- Risk: The frontend calendar renders with drag-and-drop editing enabled. The CalDAV backend silently discards writes (`PutCalendarObject` returns `nil, nil`), so no data is lost — but users will see apparent success on drag/drop with no actual effect, which is confusing and misleading.
- Files: `pages/index.go:66-67`, `calendar.go:75-77`
- Recommendations: Set `editable: false, selectable: false` in the FullCalendar config since the backend is intentionally read-only.
- Severity: **MEDIUM**

---

## Technical Debt

**Hard-coded source count (max 4 sources):**
- Issue: `Config` struct and `Srcs()` method enumerate `Src1`–`Src4` individually with copy-pasted blocks. Adding a 5th source requires editing both `config.go:10-36` and `config.go:47-85`.
- Files: `config.go`
- Impact: Maintenance burden; easy to miss one of the four blocks. No documentation on the 4-source limit.
- Fix approach: Use a slice-based config pattern. Accept `SRC_COUNT` env var and loop, or accept JSON-encoded sources array.
- Severity: **MEDIUM**

**Dead / inert code — `allowedEvents` and `renameEvents`:**
- Issue: `allowedEvents` is always an empty slice (`[]string{}`), so the filter block (`reader.go:87-94`) never executes. `renameEvents` is always an empty map (`map[string]string{}`), so the rename block (`reader.go:150-152`) never executes. Both were clearly intended as configurable features but are now dead code.
- Files: `reader.go:87-96`, `reader.go:150-152`
- Impact: Misleading to future developers; increases cognitive load.
- Fix approach: Either expose these as config options (env vars or a config file) or remove the dead blocks entirely.
- Severity: **LOW**

**Commented-out code blocks:**
- Issue: Several sections of code are commented out rather than deleted: VTIMEZONE translation logic (`reader.go:177-184`), timezone selector population from a remote API (`pages/index.go:95-108`), request duration logging (`middleware.go:49`, `middleware.go:58-59`).
- Files: `reader.go:177-184`, `pages/index.go:95-108`, `middleware.go:49`, `middleware.go:59`
- Impact: Code is harder to read; intent is unclear.
- Fix approach: Remove permanently or file issues for planned features.
- Severity: **LOW**

**`tracingMiddleware` function is defined but never called:**
- Issue: `middleware.go:47-60` defines `tracingMiddleware` which is never registered in `main.go`.
- Files: `middleware.go:47-60`
- Fix approach: Either wire it into the router or delete it.
- Severity: **LOW**

**`partials/index.go` is a framework scaffold, not application code:**
- Issue: `CounterPartial`, `CounterForm`, and `SubmitButton` are htmgo framework demo/template code with no relation to the calendar proxy functionality.
- Files: `partials/index.go`
- Impact: Confuses the purpose of the codebase; unused partials are still registered as HTTP routes by the generated code.
- Fix approach: Delete the file and regenerate routes.
- Severity: **LOW**

---

## Performance Concerns

**No HTTP client timeout:**
- Problem: `reader.go:28` creates `&http.Client{}` with no timeout. A slow or unresponsive upstream CalDAV server will block the refresh goroutine indefinitely.
- Files: `reader.go:28`
- Cause: Default `http.Client` has no timeout.
- Improvement path: Set `Timeout: 30 * time.Second` (or a configurable value) on the HTTP client.
- Severity: **HIGH**

**Single-calendar selection (always `calendars[0]`):**
- Problem: `reader.go:45` and `calendar.go:33` always pick index 0 of the calendars slice. If a CalDAV account has multiple calendars, only the first is used, silently ignoring the rest.
- Files: `reader.go:45`, `calendar.go:33`
- Cause: No calendar selection logic.
- Improvement path: Add a `SRC_N_CALENDAR` config option to specify the target calendar by name or path, and validate against the list returned by `FindCalendars`.
- Severity: **MEDIUM**

**No error isolation between sources in `downloadAll`:**
- Problem: If any one of the upstream sources fails, `downloadAll` returns immediately with an error, discarding events already downloaded from successful sources.
- Files: `reader.go:15-25`
- Improvement path: Collect per-source errors, log them, and continue aggregating events from healthy sources.
- Severity: **MEDIUM**

---

## Fragile Areas

**Unchecked slice index access — potential panic:**
- Files: `reader.go:45`, `calendar.go:33`, `reader_test.go:14`
- Why fragile: If `FindCalendars` returns an empty slice (empty account, permission error, etc.), `calendars[0]` panics with an index out of range. Same for `calProxy.config.Srcs()[0]` in the test when no sources are configured.
- Safe modification: Always guard with `if len(calendars) == 0 { return nil, fmt.Errorf(...) }` before indexing.
- Test coverage: No tests cover the empty-calendars case.
- Severity: **HIGH**

**Race condition on `CalDavHandler.Backend`:**
- Files: `calendar.go:120`, `main.go:32-39`
- Why fragile: The background goroutine calls `calDavHandler.SetEvents(events)` which replaces `h.Backend` with a new `*calendarBackend` pointer. Concurrent HTTP requests handled by `calDavHandler.Handler` read `h.Backend` without synchronization. This is a data race.
- Safe modification: Protect `h.Backend` with a `sync.RWMutex`; hold the write lock in `SetEvents` and a read lock in HTTP handler dispatch.
- Test coverage: None.
- Severity: **HIGH**

**`toTZ` fails fatally for events missing `DTSTAMP`:**
- Files: `reader.go:173`, `reader.go:224-244`
- Why fragile: `toTZ` returns an error with message `"property DTSTAMP not found for event …"` if a VEVENT lacks a DTSTAMP. This causes `download()` to fail for the entire source, not just the offending event. DTSTAMP is technically optional in some iCal implementations.
- Safe modification: Log a warning and skip rather than hard-failing on missing optional properties.
- Severity: **MEDIUM**

---

## Scalability Limits

**In-memory event storage — no persistence:**
- Current capacity: All events are held in a single `caldav.CalendarObject` in `calendarBackend.objectMap`, rebuilt on every refresh cycle.
- Limit: If the source calendars have a very large number of events, memory pressure scales linearly. No eviction or pagination.
- Scaling path: Currently acceptable for a personal/small-team proxy. For larger deployments, persist events to a file or embedded DB.
- Severity: **LOW**

**Hard-coded 6-week event window:**
- Files: `reader.go:48-49`
- Current capacity: Fetches from "start of current week" to 6 weeks ahead. Past events and events beyond 6 weeks are invisible.
- Limit: Not configurable. Applications that need historical data or longer-horizon planning are not served.
- Scaling path: Expose `SRC_LOOKBACK_DAYS` / `SRC_LOOKAHEAD_DAYS` env vars.
- Severity: **LOW**

---

## Missing Critical Features / Gaps

**No startup validation of required config:**
- Problem: `Config` has no `required` tags. If `SRC_1_URL` (or any credential) is empty, the application starts silently and only fails at the first refresh tick.
- Files: `config.go:10-37`
- Blocks: Early detection of misconfiguration.
- Fix: Add `required` tags via `go-envconfig` or add an explicit `config.Validate()` called in `main`.
- Severity: **MEDIUM**

**`QueryCalendarObjects` always returns nil:**
- Problem: `calendar.go:83-85` — the CalDAV `REPORT` query method returns `nil, nil`, meaning any CalDAV client that uses REPORT-based queries (time-range queries) gets an empty response instead of events.
- Files: `calendar.go:83-85`
- Blocks: Compatibility with CalDAV clients that rely on REPORT rather than PROPFIND.
- Severity: **MEDIUM**

---

## Mixed Logging

**`fmt.Printf` and `logrus` used inconsistently:**
- Files: `reader.go:42`, `reader.go:52`, `reader.go:148`, `main.go:74` use `fmt.Printf`; `middleware.go` uses `logrus`.
- Impact: Log output has no consistent level, timestamp, or structured format; `fmt.Printf` output cannot be silenced or redirected through a logging framework.
- Fix approach: Replace all `fmt.Printf` debug prints with `logrus.Debugf` / `logrus.Infof`.
- Severity: **LOW**

---

## Dependencies at Risk

**Forked/replaced upstream dependency:**
- Risk: `go.mod:25` replaces `github.com/emersion/go-webdav` with `github.com/mheers/go-webdav` — a personal fork pinned to a specific commit. If the upstream library releases a security fix or breaking change, the fork may diverge or go unupdated.
- Impact: May miss security patches; fork may break on Go version upgrades.
- Migration plan: Track upstream; document why the fork exists and what patch it carries so it can be upstreamed or removed when no longer needed.
- Severity: **MEDIUM**

**FullCalendar version mismatch in frontend:**
- Risk: `pages/index.go:9` loads FullCalendar CSS at `v5.5.1` but the JS is loaded at `v6.1.15` (`index.go:11-12`). Mixing major versions of a library's CSS and JS can cause visual or functional breakage.
- Files: `pages/index.go:9,11`
- Fix: Align both to `v6.1.15`.
- Severity: **MEDIUM**

---

## Test Coverage Gaps

**Single integration test requiring live credentials:**
- What's not tested: All core logic — timezone normalization, anonymization, DURATION→DTEND harmonization, empty-calendar guard, race conditions.
- Files: `reader_test.go`
- Risk: The only test (`TestDownload`) calls the live CalDAV endpoint and requires real env vars. It will always fail in CI unless secrets are injected, and it asserts a hard-coded count of exactly 2 events (`require.Len(t, events, 2)`) — fragile against any upstream data change.
- Recommendation: Add unit tests using fixture `.ics` data; extract and test `harmonizeDurationAndEnd`, `toTZ`, and anonymization logic in isolation. Move integration test behind a build tag (`//go:build integration`).
- Priority: **High**
- Severity: **HIGH**

**`events.json` written to disk by test:**
- What's not tested: This is a side-effect concern — `saveEvents` writes `events.json` to the working directory; the file is gitignored but will accumulate sensitive data locally.
- Files: `reader_test.go:19`, `.gitignore:10`
- Risk: Accidental commit of event data; sensitive calendar content on disk.
- Recommendation: Write to `t.TempDir()` instead.
- Severity: **LOW**

---

*Concerns audit: 2026-03-25*
