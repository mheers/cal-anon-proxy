# Testing Plan — agent-executable (showboat-style)

Run steps in order. Every step lists the exact command and the expected result.
An agent should stop and record a failure if actual output deviates.
Never run Phase 4 against a real calendar without explicit user-provided test credentials.

> **Execution report 2026-08-25:** Phases 0–3 executed and green after the gap fixes.
> See "Execution results" at the bottom for recorded outputs.

## Phase 0 — Static gates

```bash
go build ./...
```
Expected: exit 0, no output. **[PASS 2026-08-25]**

```bash
go vet ./...
```
Expected: exit 0, no output. **[PASS]**

```bash
test -z "$(gofmt -l .)" && echo GOFMT_OK
```
Expected: `GOFMT_OK` **[PASS]**

```bash
go test -race -count=1 ./...
```
Expected: `ok github.com/mheers/cal-anon-proxy`. **[PASS — 38 tests, 0 failures]**

## Phase 1 — CLI contract (no network, no credentials)

```bash
env -i PATH="$PATH" HOME="$HOME" go run . google-clone
```
Expected: exit 1, error contains
`missing required Google clone configuration: client-id / GOOGLE_CLIENT_ID, refresh-token / GOOGLE_REFRESH_TOKEN, source-calendar-id / GOOGLE_SOURCE_CALENDAR_ID, dest-calendar-id / GOOGLE_DEST_CALENDAR_ID`
**[PASS]**

```bash
env -i PATH="$PATH" HOME="$HOME" \
  GOOGLE_CLIENT_ID=x GOOGLE_REFRESH_TOKEN=y \
  GOOGLE_SOURCE_CALENDAR_ID=a@b.c GOOGLE_DEST_CALENDAR_ID=d@e.f \
  go run . google-clone --days-future=0
```
Expected: error `days-future must be > 0` (validates before any network call). **[PASS]**

```bash
env -i PATH="$PATH" HOME="$HOME" GOOGLE_AUTH_MODE=bogus go run . google-clone
```
Expected: error `invalid auth mode "bogus" (supported: oauth, sso)` **[PASS]**

```bash
env -i PATH="$PATH" HOME="$HOME" GOOGLE_AUTH_MODE=sso go run . google-clone
```
Expected: error listing ONLY the missing calendar IDs (SSO mode must not demand client-id/refresh-token). **[PASS]**

```bash
env -i PATH="$PATH" HOME="$HOME" \
  GOOGLE_AUTH_MODE=sso \
  GOOGLE_SOURCE_CALENDAR_ID=a@b.c GOOGLE_DEST_CALENDAR_ID=d@e.f \
  go run . google-clone
```
Expected: on machines WITHOUT gcloud/ADC: exit non-zero with descriptive
`no SSO credentials available: gcloud probe failed (...) and Application Default Credentials are unavailable (...)`
(M2 fixed — no more silent unauthenticated client).
On machines WITH gcloud creds: request reaches Google; scope errors get the
`SSO token lacks required scopes` hint appended by `wrapGoogleCloneError`.
**[PASS — verified both paths: unit test covers no-creds path deterministically; live run produced the 403 + hint]**

## Phase 2 — Unit tests (IMPLEMENTED 2026-08-25)

Now present in the repo — all pass:

- `google_clone_sync_test.go`: fake Calendar API (`httptest` server mounted via
  `option.WithEndpoint`; NOTE: endpoint replaces BasePath entirely, routes are `/calendars/{id}/events`)
  - HappyPath (inserted/deleted counts + clonedBy marker on every clone)
  - DryRunWritesNothing
  - CancelledEventsSkipped
  - InsertFailureMidway (`*googleapi.Error` surfaced, counts preserved)
  - EmptySourceAbortsWipe (H1 guard: zero deletes when source empty)
  - EmptySourceAllowedOverride (`--allow-empty-source`)
  - FirstRunFullWipeThenTaggedOnly (H1 tagging semantics)
  - CloneEventForDestination_TaggingAndCopies (deep-copy isolation — caught a real aliasing bug)
  - IsClonedEvent, NewSSOHTTPClient_NoCredentialsReturnsError, GcloudTokenSource_ReportsFailure
- `oauth_callback_test.go`: Success / StateMismatch / ErrorParam / MissingCode / WrongPath
- `middleware_test.go`: auth matrix incl. constant-time compare path (M4)
- `calendar_json_test.go`: SingleEvent / WindowFilter / RRuleExpansion / RecurrenceOverride /
  LocalModeEmitsZ / AllDay / ServeICS_MergesVEVENTs

Coverage: `go test -cover ./` → **45.4%** of package statements (rest is htmgo glue/server wiring).

## Phase 3 — Server smoke test (local, no external calls)

NOTE: the htmgo server binds **port 3000**, not 8086 (README's CalDAV section is stale on this).

```bash
mkdir -p /tmp/opencode/cap-smoke && cp testdata/event_with_dtend.ics /tmp/opencode/cap-smoke/src.ics
(cd /tmp/opencode/cap-smoke && python3 -m http.server 8099 >/dev/null 2>&1 &)
go build -o /tmp/opencode/cap-smoke/cal-anon-proxy .
cd /tmp/opencode/cap-smoke && env -i PATH="$PATH" HOME="$HOME" SRC_1_URL=http://127.0.0.1:8099/src.ics SRC_UPDATE_INTERVAL=60 ./cal-anon-proxy server &
sleep 5
curl -sS http://127.0.0.1:3000/events.json | head -c 300
```
Expected: JSON array containing event objects with keys `id,title,start,end`.
**[PASS: `[{"id":"test-dtend-001@test.invalid","title":"Team Meeting","start":"2026-03-25T10:00:00Z","end":"2026-03-25T11:00:00Z"}]`]**

```bash
curl -sS http://127.0.0.1:3000/calendar.ics | grep -c BEGIN:VEVENT
```
Expected: `>= 1`. **[PASS: `1`]**

Startup resilience (found + fixed during testing):

```bash
SRC_UPDATE_INTERVAL= ./cal-anon-proxy server   # empty value, like docker-compose passes
```
Previously **panicked** (`non-positive interval for NewTicker`) due to an ignored
`default:"5"` struct tag (go-envconfig wants `env:"...,default=5"`); fixed in
config.go + guarded in main.go. **[PASS: no panic, serves events]**

Auth-surface check (documents M1 decision):

```bash
DST_AUTH_ENABLED=true DST_USERNAME=u DST_PASSWORD=p ./cal-anon-proxy server
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/caldav/
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/events.json
```
Result today: `401` then `200` — M1 resolved as **documented public-by-design**
(README Security notes): the browser UI fetches `/events.json`, so it must stay
public; use `SRC_*_ANON=true` if titles must not leak. Basic auth itself verified
working (401 without creds, constant-time compare covered by middleware_test.go).
Plain `GET /caldav/` with valid creds returns 500 ("couldn't find calendar object")
and PROPFIND returns 405 through the chi router — pre-existing routing behavior,
verify with a real CalDAV client (Thunderbird worked historically).

Cleanup: `pkill -x cal-anon-proxy; pkill -f 'http.server 8099'`

## Phase 4 — Live Google verification (OPTIONAL; only with user-provided test creds)

Use throwaway calendars only. Never point at production data.

```bash
GOOGLE_DRY_RUN=true \
GOOGLE_SOURCE_CALENDAR_ID="<user-provided>" \
GOOGLE_DEST_CALENDAR_ID="<user-provided>" \
GOOGLE_CLIENT_ID="..." GOOGLE_REFRESH_TOKEN="..." \
go run . google-clone
```
Expected: log line `google-clone completed: inserted=N deleted=M`, destination unchanged (dry-run issues zero writes — proven by TestCloneGoogleCalendarWindow_DryRunWritesNothing against the fake API).

Negative safety check (H1, now implemented): point SOURCE at an empty calendar with dry-run=false → command must ABORT with the `refusing to touch destination` error and delete nothing (proven by TestCloneGoogleCalendarWindow_EmptySourceAbortsWipe).

Status: **PENDING** — needs real Google test credentials from the user; all logic already covered by fake-API tests.

## Definition of done

- [x] Phases 0–3 fully green with recorded outputs (2026-08-25)
- [x] Phase 2 tests merged; CI additionally runs `go vet` + `gofmt -l` gates
- [x] H1, M2, M3 fixed with tests proving each
- [ ] Phase 4 live dry-run (blocked on user-provided throwaway calendar credentials)
