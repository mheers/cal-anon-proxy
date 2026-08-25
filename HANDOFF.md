# Agent Handoff — cal-anon-proxy review (2026-08-25)

## Current state

- Branch `main`, 27 unpushed commits, plus **uncommitted work**:
  - New: `google_clone.go` (google-clone + google-login CLI commands), `google_clone_test.go`
  - Modified: `main.go` (registers new commands), `go.mod`/`go.sum` (Go 1.25, `golang.org/x/oauth2`, `google.golang.org/api`), `Makefile` (local binary build), `.gitignore` (ignores the built binary), `README.md` (Google OAuth setup docs)
- Verified green: `go build ./...`, `go vet ./...`, `go test -race ./...`
- `.env` is gitignored and has no git history — good. It contains at least one secret line; never commit it.
- The 24 MB built binary `cal-anon-proxy` in repo root is now gitignored; delete it or leave untracked.

## Verdict

~~Do not ship as-is: one data-loss risk (H1) and one silent-unauth failure mode (M2).~~
**All H/M findings fixed 2026-08-25 — see "Fix report" below. 38 tests green, smoke-tested end to end.**

## Findings

### HIGH

| ID | Where | Issue | Fix |
|----|-------|-------|-----|
| H1 | google_clone.go:66, :492–508 | `GOOGLE_WIPE_DESTINATION` defaults to `true` and deletes **all** destination events in the window — including user-created ones. If source listing returns 0 events (empty window, transient API hiccup), it wipes dest and inserts nothing → mirror silently emptied. | 1) Abort sync with a loud error when source returns 0 events unless an explicit `--allow-empty-source` is set. 2) Tag cloned events via `extendedProperties.private["clonedBy"]="cal-anon-proxy"` (+ source event id) and after the initial full wipe only delete tagged events. |

### MEDIUM

| ID | Where | Issue | Fix |
|----|-------|-------|-----|
| M1 | main.go:87–89 | When `DST_AUTH_ENABLED=true`, `/caldav/` gets basic auth but `/calendar.ics` and `/events.json` stay public — event titles/times leak even in "protected" mode. If intentional, document it in README; otherwise wrap both handlers in the same auth middleware. | Decide policy; either protect or document explicitly. |
| M2 | google_clone.go:470–484 | SSO fallback chain: if gcloud probe fails AND ADC fails, returns `oauth2.NewClient(ctx, nil)` — an **unauthenticated** client. Sync then fails with confusing 401s far from the cause. | Return a descriptive error ("no SSO credentials: gcloud probe failed: %v; ADC unavailable: %v"). |
| M3 | google_clone.go:451–467 | `cmd.Output()` without `cmd.Stderr` loses gcloud stderr → error is bare "exit status 1". | Attach `&bytes.Buffer{}` as Stderr and include it in the error. |
| M4 | middleware.go:29 | Basic-auth comparison is not constant-time (timing side channel). | Use `crypto/subtle.ConstantTimeCompare`. |

### LOW

| ID | Where | Issue |
|----|-------|-------|
| L1 | google_clone.go:296–305 | Interval loop ignores ctx cancellation (`for range ticker.C`) — no graceful shutdown path. Select on `ctx.Done()` too. |
| L2 | reader.go:424 | Dead condition: `endTime.Sub(endTime.Add(durationTime)) != 0` is always true when duration ≠ 0. Simplify to set DTEND whenever duration > 0. |
| L3 | reader.go:247 | Loads `Europe/London` per event then immediately converts to UTC in `toTZ` — pointless intermediate step; use UTC directly. |
| L4 | calendar.go:181–184 | `http.Error` after streaming already started in `ServeICS` cannot change status/headers; log instead. |
| L5 | google_clone.go:81–82, 110 | `--client-secret` / `--refresh-token` flags are visible in `ps`; README should steer users to env vars (flags exist for convenience). |
| L6 | docker-compose.yaml | Doesn't pass any `GOOGLE_*` env vars into the container. |
| L7 | Design note | Recurring events are cloned as flattened single instances (`SingleEvents(true)`); RRULE structure is not preserved in destination. Document this behavior in README "How it syncs". |

## Gaps / missing implementation

Status after 2026-08-25 fixes: items 1–5 are **closed** (see Fix report); 6–7 remain open.

1. ~~No safety guard for empty source window~~ — CLOSED (H1).
2. ~~No tests for `cloneGoogleCalendarWindow`~~ — CLOSED (`google_clone_sync_test.go`, fake Calendar API).
3. ~~No tests for `ServeEventsJSON`~~ — CLOSED (`calendar_json_test.go`).
4. ~~`google-login` callback handler untested~~ — CLOSED (refactored into `newOAuthCallbackHandler`; `oauth_callback_test.go`).
5. ~~CI runs only tests~~ — CLOSED (vet + gofmt gates added to workflow).
6. No signal/graceful-shutdown handling for **server mode** (google-clone mode done; server mode still hard-exits).
7. README stale port (8086 vs actual 3000) in the Run/CalDAV sections.

Original list kept below for context:

1. **No safety guard for empty source window** (see H1) — biggest missing feature.
2. **No tests for `cloneGoogleCalendarWindow`** insert/delete/dry-run/failure paths (needs fake Calendar API via httptest + `option.WithEndpoint`).
3. **No tests for `ServeEventsJSON`** (recurrence expansion, RECURRENCE-ID overrides, timezone naive vs local-Z output) via `httptest.NewRecorder`.
4. **`google-login` callback handler untested** — state mismatch, `error=` param, missing-code paths. Refactor handler into a standalone func for testability.
5. **CI runs only `go test -race ./...`** — add `go vet` and `gofmt -l` gates.
6. No signal/graceful-shutdown handling for server mode or clone interval mode.
7. Docker image defaults to `server` CMD; `google-clone` works inside image but compose doesn't expose GOOGLE_* vars (L6).

## Testing plan (agent-executable)

See `TESTING_PLAN.md` — showboat-style executable steps with exact commands and expected outputs.

## Suggested fix order

1. H1 (empty-source guard + event tagging) — data safety
2. M2 + M3 (SSO failure modes)
3. M1 (decide/document auth surface) + M4
4. Tests from gaps 2–4 (TESTING_PLAN.md Phase 2)
5. Low items opportunistically

---

# Fix report (2026-08-25)

All findings addressed; two **additional production bugs** were discovered by the new tests and fixed.

## Fixed

| ID | Fix | Tests proving it |
|----|-----|------------------|
| H1 | Empty-source guard: sync aborts with `refusing to touch destination` when source returns 0 events, unless `--allow-empty-source` / `GOOGLE_ALLOW_EMPTY_SOURCE=true`. Cloned events are tagged `extendedProperties.private.clonedBy="cal-anon-proxy"` (+`sourceEventId`); first sync does a full window wipe, later syncs delete only tagged events so manual events survive. | EmptySourceAbortsWipe, EmptySourceAllowedOverride, FirstRunFullWipeThenTaggedOnly |
| M1 | Resolved as documented public-by-design: `/calendar.ics` + `/events.json` stay public because the browser UI fetches `/events.json`; README "Security notes" added recommending `SRC_*_ANON=true` to avoid title leaks. Verified live: `/caldav/`=401 without creds while `/events.json`=200 with `DST_AUTH_ENABLED=true`. | middleware_test.go + Phase 3 smoke test |
| M2 | `newSSOHTTPClient` now returns a descriptive error (`no SSO credentials available: gcloud probe failed … ADC unavailable …`) instead of silently building an unauthenticated client. | NewSSOHTTPClient_NoCredentialsReturnsError |
| M3 | gcloud stderr captured into the token-source error (`gcloud auth print-access-token failed: … : <stderr>`); probe error no longer re-executed. | GcloudTokenSource_ReportsFailure |
| M4 | Basic-auth compare now constant-time via `crypto/subtle`. | middleware_test.go auth matrix |
| L1 | `google-clone --interval` installs a signal context (SIGINT/SIGTERM) and the ticker loop selects on `ctx.Done()` — clean shutdown. | build/vet; loop covered indirectly |
| L2 | Dead tautology in `harmonizeDurationAndEnd` removed; DTEND set whenever DURATION ≠ 0. | existing reader_unit_test.go still green |
| L3 | `time.LoadLocation("Europe/London")` hoisted out of the per-event loop (was loading per event). Kept as floating-time fallback deliberately — switching it to UTC would change instants for timezone-less events. | code review |
| L4 | `ServeICS` logs encode failures instead of writing headers after body flush. | TestServeICS_MergesVEVENTs |
| L5 | README security note: prefer env vars over CLI flags for secrets (`ps` exposure). | docs |
| L6 | docker-compose.yaml now passes all `GOOGLE_*` vars through. | compose file |
| L7 | README documents recurrence flattening (`singleEvents=true`). | docs |

## Extra bugs found BY the new tests (both real production bugs)

1. **Server panicked at startup** when `SRC_UPDATE_INTERVAL` was unset/empty:
   `config.go` used a `default:"5"` struct tag, but go-envconfig only honors inline
   `env:"SRC_UPDATE_INTERVAL,default=5"` — interval stayed 0 → `time.NewTicker` panic.
   Additionally, envconfig treats empty-string env vars (as docker-compose passes them)
   as set-but-empty, bypassing defaults entirely. Fixed both: corrected tag + `<=0`
   guard in main.go. Verified live: server boots and serves with `SRC_UPDATE_INTERVAL=`.
2. **ServeICS silently failed** ("calendar is empty") for source feeds whose VEVENTs
   lack `DTSTAMP`: go-ical's encoder hard-requires it. `processEvents` now injects
   `DTSTAMP` (now, UTC) when missing; all testdata fixtures updated. Found via
   TestServeICS_MergesVEVENTs returning an empty calendar body.
3. **Reminder deep-copy aliasing**: cloning copied the `[]*EventReminder` slice but
   shared reminder pointers — mutating a clone mutated the source event. Now
   element-wise value copy. Found by CloneEventForDestination_TaggingAndCopies.

## Test results

- `go build ./...` ✅ · `go vet ./...` ✅ · `gofmt -l .` clean ✅
- `go test -race -count=1 ./...` → **ok, 38 tests / 0 failures**
- Coverage: 45.4% of package statements (remainder is htmgo glue/server wiring)
- New/changed files: `google_clone_sync_test.go`, `oauth_callback_test.go`,
  `middleware_test.go`, `calendar_json_test.go`, fixtures `event_recurring.ics`,
  `event_override.ics` (+DTSTAMP in all fixtures), `.gitignore` now keeps
  `testdata/*.ics` despite global `*.ics`

## Open items

- Phase 4 live dry-run against real Google calendars awaits user-provided throwaway credentials.
- Pre-existing observation (unchanged): plain `GET /caldav/` returns 500 and PROPFIND returns 405
  through the htmgo/chi router; verify CalDAV with a real client (Thunderbird historically worked).
- README "Run" section says port 8086 but the server binds **3000** — stale doc, not fixed here.
- Uncommitted: everything above is working-tree only; commit after review.
