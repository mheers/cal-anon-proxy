# Coding Conventions

**Analysis Date:** 2026-03-25

## Naming Patterns

**Files:**
- Snake_case with descriptive names: `reader.go`, `middleware.go`, `calendar.go`, `config.go`
- Test files follow Go convention: `reader_test.go` (same package, co-located)
- Build-tag variants use suffix: `assets.go` (dev) / `assets_prod.go` (prod)
- Generated files use suffix: `pages-generated.go`, `partials-generated.go`, `assets-generated.go`
- Packages under subdirectories match directory name: `pages/`, `partials/`, `internal/embedded/`

**Types (structs):**
- PascalCase exported: `CalProxy`, `CalDavHandler`, `Config`, `Src`, `CtxKey`, `CtxValue`
- camelCase unexported: `calendarBackend`, `auth`

**Functions/Methods:**
- PascalCase for exported: `ReadConfig`, `NewCalProxy`, `NewCalDavHandler`, `GetStaticAssets`
- camelCase for unexported: `downloadAll`, `download`, `summaryOfEvent`, `contains`, `toTZ`, `harmonizeDurationAndEnd`
- Constructor functions follow `New<TypeName>` pattern: `NewCalProxy`, `NewCalDavHandler`, `NewOsFs`

**Variables:**
- camelCase: `calDavHandler`, `httpClient`, `queryStart`, `queryEnd`, `calEvents`
- Short idiomatic receivers: `p` for `*CalProxy`, `b` for `*calendarBackend`, `h` for `*CalDavHandler`, `a` for `*auth`, `c` for `*Config`
- Exception: `receiver` used in `internal/embedded/os.go` (non-idiomatic)

**Constants:**
- PascalCase in generated asset files: `AppleTouchIconPng`, `FaviconIco`, `MainCss`
- Package-level `const` for image/build targets in CI TypeScript: `buildImage`, `baseImage`, `targetImage` (camelCase)

## Code Style

**Formatting:**
- Standard Go formatting (gofmt) — no explicit config file present (`.golangci.yml` absent)
- No Prettier or ESLint config found for TypeScript CI code

**Packages:**
- All main application code lives in `package main` at root (monolithic package pattern)
- Sub-packages used for htmgo UI concerns: `pages`, `partials`, `internal/embedded`
- Generated code lives in `__htmgo/` package

**Build Tags:**
- Dual-file build-tag pattern for dev vs. production asset embedding:
  ```go
  //go:build !prod   // assets.go — dev: OS filesystem
  //go:build prod    // assets_prod.go — prod: embedded FS
  ```
- Both new (`//go:build`) and legacy (`// +build`) constraint syntax present

## Import Organization

**Standard Go grouping** (separated by blank lines):
1. Standard library packages
2. Third-party packages
3. Internal/local module packages

Example from `main.go`:
```go
import (
    "fmt"
    "io/fs"
    "net/http"
    "time"

    hConfig "github.com/maddalax/htmgo/framework/config"
    "github.com/maddalax/htmgo/framework/h"
    "github.com/maddalax/htmgo/framework/service"
    "github.com/mheers/cal-anon-proxy/__htmgo"
    "github.com/sirupsen/logrus"
)
```

**Aliased imports** used when names would collide or for clarity:
- `tzLib "github.com/mheers/go-tz"` — disambiguates from `time` stdlib
- `hConfig "github.com/maddalax/htmgo/framework/config"` — avoids conflict with local `config`
- `ical "github.com/emersion/go-ical"` (package name is `ical` without alias, imported as-is)

## Error Handling

**Primary pattern:** Immediate propagation — errors are returned up the call stack without wrapping:
```go
if err != nil {
    return nil, err
}
```

**Contextual errors** use `fmt.Errorf` with descriptive messages when creating new errors:
```go
return nil, fmt.Errorf("calendar for path: %s not found", path)
return nil, fmt.Errorf("property %s not found for event %s", propName, summaryOfEvent(event))
```

**Fatal errors** at startup use `log.Fatal` (config loading in `config.go`):
```go
if err := envconfig.Process(context.Background(), &c); err != nil {
    log.Fatal(err)
}
```

**Panic** is used only for unrecoverable startup errors (missing embedded FS sub):
```go
if err != nil {
    panic(err)
}
```

**Soft errors** (non-critical path): In `main.go`, `updateEvents` logs with `logrus.Error` and returns without crashing:
```go
if err != nil {
    logrus.Error(err)
    return
}
```

**No sentinel errors** or custom error types defined in application code (only third-party Dagger SDK has custom error types in `ci/dagger/sdk/`).

## Logging

**Mixed approach:** both `fmt.Printf` and `logrus` are used inconsistently:
- `reader.go`: uses `fmt.Printf` for diagnostic output (calendar names, event names, date ranges)
- `main.go`: uses `logrus.Error` for errors, `fmt.Printf` for download count
- `middleware.go`: uses `logrus.Infof` for HTTP request tracing

**logrus** is the structured logger (`github.com/sirupsen/logrus`), used for HTTP layer.
**fmt.Printf** is used ad-hoc for operational diagnostic output in business logic.

## Comments

**Inline comments** for non-obvious logic:
```go
// queryStart of current week
queryStart := time.Now().AddDate(0, 0, -int(time.Now().Weekday()))
```

**Section markers** for multi-step operations:
```go
// harmonize DURATION and DTEND
// set timezone for start
// set timezone for end
```

**Commented-out code** is present in several places:
- `middleware.go`: two commented `logrus.Infof` statements and a `start := time.Now()` block
- `reader.go`: an entire `VTIMEZONE` handling block (lines 177–184)
- `pages/index.go`: a large timezone-fetching block (lines 95–108)

**No GoDoc** comments on any exported types or functions — documentation strings are absent.

**Single-line comment style** (`//`) used throughout; no block comments (`/* */`) in application code.

## Common Patterns

**Constructor pattern:**
```go
func NewCalProxy(config *Config) *CalProxy {
    return &CalProxy{config: config}
}
```

**HTTP middleware wrapping:**
```go
func (a *auth) middleware(actualHandler http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ... })
}
```

**Slice accumulation:**
```go
result := []T{}
for _, item := range items {
    result = append(result, item)
}
return result, nil
```

**Context passing** through all HTTP/CalDAV backend methods as first parameter (standard Go pattern).

**Build-time asset selection** via Go build tags (dev/prod variants in `assets.go` / `assets_prod.go`).

**Generated code separation** — htmgo framework generates route registration and asset path constants into `__htmgo/`, clearly marked `// THIS FILE IS GENERATED. DO NOT EDIT.`

---

*Convention analysis: 2026-03-25*
