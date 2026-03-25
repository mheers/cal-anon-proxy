# Architecture

**Analysis Date:** 2026-03-25

## Pattern Overview

**Overall:** Pipeline / Proxy with embedded web server

**Key Characteristics:**
- Single Go binary serving two distinct concerns: a CalDAV server (read-only proxy) and a web UI (htmgo SSR framework)
- Pull-based event sync: a background goroutine periodically fetches from upstream CalDAV sources and writes into an in-memory backend
- No database — all state lives in memory (`calendarBackend`) and is replaced wholesale on each sync cycle
- The htmgo framework generates route-registration code at build time; hand-written pages/partials are the developer surface

## Layers

**Configuration Layer:**
- Purpose: Read environment variables into a typed struct
- Location: `config.go`
- Contains: `Config` struct, `ReadConfig()`, `Src` struct, `Srcs()` helper
- Depends on: `github.com/sethvargo/go-envconfig`
- Used by: `main.go`, `proxy.go` (via `CalProxy`)

**Proxy / Fetch Layer:**
- Purpose: Download and anonymize/normalize calendar events from upstream CalDAV servers
- Location: `proxy.go` (struct definition), `reader.go` (download logic)
- Contains: `CalProxy`, `downloadAll()`, `download()`, `toTZ()`, `harmonizeDurationAndEnd()`
- Depends on: `go-webdav/caldav`, `go-ical`, `go-tz`, Config
- Used by: `main.go` background goroutine → `CalDavHandler.SetEvents()`

**CalDAV Server Layer:**
- Purpose: Serve processed events to CalDAV clients (calendar apps)
- Location: `calendar.go`
- Contains: `calendarBackend` (in-memory store), `CalDavHandler` (http.Handler wrapper)
- Depends on: `go-webdav/caldav`, `go-ical`
- Used by: `main.go` — mounted at `/caldav/`

**HTTP Middleware Layer:**
- Purpose: Basic Auth enforcement and request tracing
- Location: `middleware.go`
- Contains: `auth` struct, `auth.middleware()`, `tracingMiddleware()`, `CtxKey`/`CtxValue` context types
- Depends on: standard `net/http`, `logrus`
- Used by: `main.go` — wraps `CalDavHandler` when `DST_AUTH_ENABLED=true`

**Web UI Layer:**
- Purpose: Browser-based calendar viewer using FullCalendar.js
- Location: `pages/index.go`, `pages/root.go`, `partials/index.go`
- Contains: Server-side rendered HTML via htmgo DSL; JS-heavy index page loads FullCalendar and points it at `/caldav/`
- Depends on: `github.com/maddalax/htmgo/framework/h`
- Used by: Routes registered in `__htmgo/pages-generated.go`

**Code-Generated Routing Layer:**
- Purpose: Auto-register page and partial routes on the chi router
- Location: `__htmgo/pages-generated.go`, `__htmgo/partials-generated.go`, `__htmgo/setup-generated.go`
- Contains: `Register(router)`, `RegisterPages()`, `RegisterPartials()`
- Depends on: `go-chi/chi`, htmgo framework
- Used by: `main.go` — `__htmgo.Register(app.Router)`

**Static Assets Layer:**
- Purpose: Serve CSS, JS, icons; dev vs. prod build variants
- Location: `assets.go` (dev, OS filesystem), `assets_prod.go` (prod, Go embed), `assets/dist/`
- Contains: Build-tag-guarded `GetStaticAssets()` function
- Depends on: `internal/embedded` (dev), `embed` (prod)
- Used by: `main.go` — mounted at `/public/*`

## Data Flow

**Event Sync (background goroutine):**

1. `main.go` spawns goroutine; calls `updateEvents(proxy, calDavHandler)` immediately, then on every `SRC_UPDATE_INTERVAL` tick
2. `CalProxy.downloadAll()` iterates configured `Src` entries; calls `CalProxy.download(src)` for each
3. `download()` connects to upstream CalDAV server with HTTP Basic Auth, queries events for current week +6 weeks
4. Each VEVENT is sanitized: private properties stripped, summary replaced with "unavailable" if `Src.Anon=true`, timezone normalized to Europe/London, `DURATION` converted to `DTEND`
5. Cleaned `[]*caldav.CalendarObject` returned to `updateEvents()`
6. `CalDavHandler.SetEvents(events)` replaces the in-memory `calendarBackend` — atomic swap, no locking visible

**CalDAV Request (calendar client → server):**

1. Calendar app (e.g. Apple Calendar) sends CalDAV request to `/caldav/`
2. If `DST_AUTH_ENABLED`, `auth.middleware` validates HTTP Basic Auth and injects `CtxValue{Username}` into request context
3. `caldav.Handler` (from go-webdav) dispatches to `calendarBackend` methods (`ListCalendars`, `ListCalendarObjects`, etc.)
4. `calendarBackend` returns pre-loaded in-memory calendar objects
5. Response serialized as iCalendar data by go-webdav

**Web UI Request (browser → server):**

1. Browser requests `/` → chi router → `pages.IndexPage` renders full HTML page with FullCalendar embedded JS
2. FullCalendar fetches `/caldav/` directly from the browser as ICS source
3. HTMX partial updates routed through `/github.com/mheers/cal-anon-proxy/partials*`

**State Management:**
- All calendar state held in `calendarBackend.objectMap` (a `map[string][]caldav.CalendarObject`)
- Replaced entirely on each sync; no incremental update, no persistence
- Concurrent access not explicitly guarded — sync goroutine and HTTP handlers share `calendarBackend` via pointer swap in `SetEvents()`

## Key Abstractions

**`calendarBackend`:**
- Purpose: In-memory implementation of the `caldav.Backend` interface
- Location: `calendar.go`
- Pattern: Implements go-webdav's `Backend` interface; write operations (`CreateCalendar`, `PutCalendarObject`, `DeleteCalendarObject`) are no-ops

**`CalProxy`:**
- Purpose: Wraps config and owns the upstream fetch + sanitization pipeline
- Location: `proxy.go`, `reader.go`
- Pattern: Thin struct with methods; no interface, not injected

**`auth` struct:**
- Purpose: HTTP middleware factory for Basic Auth
- Location: `middleware.go`
- Pattern: Middleware adapter — wraps any `http.Handler`

**htmgo Pages/Partials:**
- Purpose: Server-rendered HTML components using Go function composition
- Location: `pages/`, `partials/`
- Pattern: Functions returning `*h.Page` or `*h.Partial`; routing is code-generated into `__htmgo/`

## Entry Points

**`main()`:**
- Location: `main.go`
- Triggers: Process start
- Responsibilities: Read config → build proxy and CalDAV handler → optionally wrap with auth middleware → start sync goroutine → start htmgo HTTP server

## Error Handling

**Strategy:** Fail-fast on startup (config), log-and-skip on sync errors

**Patterns:**
- `ReadConfig()` calls `log.Fatal(err)` on bad env config — process exits
- `updateEvents()` logs errors with `logrus.Error(err)` and returns — sync is retried on next tick
- `download()` returns `error`; most helpers propagate errors with `fmt.Errorf` wrapping
- `calendar.go` write-path methods return `nil, nil` (silent no-ops) — read-only server design

## Cross-Cutting Concerns

**Logging:** `github.com/sirupsen/logrus` — used in `middleware.go` and `main.go`; `fmt.Printf` also used in `reader.go` for event names
**Validation:** None beyond env config parsing; upstream CalDAV errors surface as sync failures
**Authentication:** HTTP Basic Auth for both destination exposure (`auth.middleware`) and upstream fetching (per-src credentials in `Config`)
