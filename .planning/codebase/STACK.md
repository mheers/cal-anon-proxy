# Technology Stack

**Analysis Date:** 2026-03-25

## Languages

**Primary:**
- Go 1.23.2 — all application logic, HTTP server, CalDAV proxy, and page rendering

**Secondary:**
- TypeScript 5.5.4 — CI pipeline via Dagger SDK (`ci/dagger/src/index.ts`)
- JavaScript — frontend calendar UI (inline scripts in `pages/index.go` via htmgo)

## Runtime

**Environment:**
- Go 1.23.2 (declared in `go.mod`, enforced by CI `go-version: ">=1.23"`)
- Node.js — used only for CI tooling (Dagger SDK + Yarn); no runtime Node in the app

**Package Manager:**
- Go: standard `go mod` — lockfile: `go.sum` ✓
- CI/Dagger: Yarn 1.22.22 (`ci/dagger/package.json`)

## Frameworks

**Core:**
- `github.com/maddalax/htmgo/framework v1.0.6` — SSR HTML-over-the-wire web framework; provides routing (`go-chi/chi`), page/partial auto-registration, live reload, and server-side HTML component DSL
- `github.com/go-chi/chi/v5 v5.2.0` — HTTP router (used internally by htmgo and exposed as `app.Router`)

**Build/Dev:**
- `htmgo` CLI — builds/runs/watches the project (wraps Go build + Tailwind CSS compilation); invoked via Taskfile tasks: `htmgo run`, `htmgo build`, `htmgo watch`
- `go-task` (Taskfile v3) — task runner defined in `Taskfile.yml`
- Tailwind CSS (standalone binary downloaded at build time from GitHub releases) — utility CSS, configured in `tailwind.config.js` to scan all `**/*.go` files

**Testing:**
- `github.com/stretchr/testify v1.10.0` — assertion library (`require` package)
- Standard `testing` package — Go's built-in test runner

## Key Dependencies

**Critical:**
- `github.com/emersion/go-ical v0.0.0-20240127...` — iCalendar (RFC 5545) parsing and manipulation
- `github.com/emersion/go-webdav v0.5.1-...` — WebDAV/CalDAV client and server (forked: replaced with `github.com/mheers/go-webdav` in `go.mod`)
- `github.com/mheers/go-tz v0.0.0-20241118...` — Microsoft timezone name → IANA timezone translation
- `github.com/sethvargo/go-envconfig v1.1.0` — struct-tag-based environment variable configuration (`config.go`)
- `github.com/sirupsen/logrus v1.9.3` — structured logging
- `github.com/teambition/rrule-go v1.8.2` — iCalendar RRULE recurrence rule support (indirect, via go-ical)
- `github.com/google/uuid v1.6.0` — UUID generation (indirect)

## Configuration

**Environment:**
- Configured entirely via environment variables; parsed using struct tags in `config.go` with `go-envconfig`
- No config files — all runtime config is environment-driven

**Build:**
- `htmgo.yml` — htmgo framework config (Tailwind toggle, watch patterns, asset path)
- `tailwind.config.js` — Tailwind CSS config (scans `**/*.go`)
- `go.mod` — module `github.com/mheers/cal-anon-proxy`
- Build tags: `prod` vs non-prod control static asset embedding (`assets.go` / `assets_prod.go`)

## Platform Requirements

**Development:**
- Go 1.23+
- `htmgo` CLI installed
- Tailwind CSS standalone binary (auto-downloaded by htmgo)

**Production:**
- Single statically compiled Go binary (`dist/cal-anon-proxy`)
- Alpine Linux container (as per `ci/dagger/src/index.ts` — built from `golang:1.23-alpine`, runs on `alpine`)
- Exposed port: `8086`

---

*Stack analysis: 2026-03-25*
