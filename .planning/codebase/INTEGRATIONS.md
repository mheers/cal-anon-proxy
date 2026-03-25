# External Integrations

**Analysis Date:** 2026-03-25

## APIs & External Services

**CalDAV Sources (upstream calendars):**
- Any CalDAV-compliant server (e.g., Nextcloud, Exchange, Baikal, Apple Calendar Server)
  - SDK/Client: `github.com/emersion/go-webdav/caldav` (forked as `github.com/mheers/go-webdav`)
  - Auth: HTTP Basic Auth — credentials supplied via `SRC_N_USERNAME` / `SRC_N_PASSWORD` env vars
  - Up to 4 sources supported (`SRC_1_*` through `SRC_4_*`)
  - Anonymous access supported per-source via `SRC_N_ANON=true`

**Frontend CDN Libraries (loaded in browser, no server-side integration):**
- FullCalendar v6.1.15 — calendar UI renderer (`cdn.jsdelivr.net`)
- FullCalendar iCalendar plugin v6.1.15 — parses ICS feed in browser
- ical.js v1.5.0 — iCalendar parsing in browser (`cdnjs.cloudflare.com`)

## Data Storage

**Databases:**
- None — no database. Calendar events are fetched from upstream CalDAV servers, held in memory, and re-fetched on a configurable interval.

**File Storage:**
- Local filesystem only — static assets embedded via Go `embed.FS` (production build) or served from disk (dev build). No external file storage.

**Caching:**
- In-memory only — `CalDavHandler.SetEvents()` stores the latest fetched events as a `caldav.CalendarObject` in a map. No persistent or distributed cache.

## Authentication & Identity

**Outbound (to upstream CalDAV sources):**
- HTTP Basic Auth — credentials injected via `go-webdav`'s `HTTPClientWithBasicAuth` wrapper; per-source username/password in `reader.go`

**Inbound (protecting the proxy's own CalDAV endpoint):**
- HTTP Basic Auth — optional, enabled via `DST_AUTH_ENABLED=true`
- Implementation: custom middleware in `middleware.go`; single username/password (`DST_USERNAME`, `DST_PASSWORD`)
- No OAuth, no JWT, no session management

## Monitoring & Observability

**Error Tracking:**
- None — no external error tracking service (no Sentry, Rollbar, etc.)

**Logs:**
- `github.com/sirupsen/logrus v1.9.3` — structured logging to stdout
- Request logging via `tracingMiddleware` in `middleware.go`; errors logged with `logrus.Error`

## CI/CD & Deployment

**Hosting:**
- Docker container — image `mheers/cal-anon-proxy:latest` on Docker Hub
- Container registry: `docker.io` (Docker Hub), pushed with `mheers` account credentials

**CI Pipeline:**
- GitHub Actions — workflow at `.github/workflows/main.yml`
- Triggers on push to `main` branch
- Uses **Dagger** (`dagger.io`) for portable build pipeline
  - Dagger module: `ci/dagger/src/index.ts` (TypeScript SDK `@dagger.io/dagger`)
  - Builds Go binary inside `golang:1.23-alpine`, packages into `alpine` final image, pushes to Docker Hub
  - Tailwind binary fetched from GitHub releases during Dagger build

**Secrets (CI):**
- `REGISTRY_ACCESS_TOKEN` — Docker Hub registry token, stored as GitHub Actions secret

## Environment Configuration

**Application env vars (defined in `.env.example`):**

| Variable | Purpose | Default |
|---|---|---|
| `SRC_UPDATE_INTERVAL` | Minutes between CalDAV re-fetches | `5` |
| `SRC_1_URL` | CalDAV URL for source 1 | — |
| `SRC_1_ANON` | Anonymise source 1 events (bool) | — |
| `SRC_1_USERNAME` | Basic auth username for source 1 | — |
| `SRC_1_PASSWORD` | Basic auth password for source 1 | — |
| `SRC_2_*` – `SRC_4_*` | Same as SRC_1 for additional sources | — |
| `DST_AUTH_ENABLED` | Enable Basic Auth on exposed endpoint | — |
| `DST_USERNAME` | Username for inbound auth | — |
| `DST_PASSWORD` | Password for inbound auth | — |
| `DST_PUBLIC_DOMAIN` | Public domain of this proxy | — |

**Secrets location:**
- Development: `.env` file (present, gitignored)
- Production: environment variables injected into Docker container (see `docker-compose.yaml`)
- CI: GitHub Actions secrets (`REGISTRY_ACCESS_TOKEN`)

## Container Setup

**`docker-compose.yaml`:**
- Service: `cal-anon-proxy`
- Image: `mheers/cal-anon-proxy:latest`
- Port mapping: `8086:8086`
- All env vars passed through from host environment

**Dockerfile:**
- Not present as a standalone `Dockerfile`; Docker image is built entirely within the Dagger pipeline (`ci/dagger/src/index.ts`):
  - Build stage: `golang:1.23-alpine` (compiles binary with `htmgo build`)
  - Runtime stage: `alpine` (copies compiled binary, sets entrypoint)

## Webhooks & Callbacks

**Incoming:** None

**Outgoing:** None — the proxy only polls CalDAV servers on a timer; no push/webhook mechanism

---

*Integration audit: 2026-03-25*
