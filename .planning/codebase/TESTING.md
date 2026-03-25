# Testing Patterns

**Analysis Date:** 2026-03-25

## Test Framework

**Runner:**
- Go standard `testing` package
- Config: none (uses `go test ./...`)

**Assertion Library:**
- `github.com/stretchr/testify v1.10.0` — `require` sub-package used
  - `require.NoError(t, err)` — fails immediately on error
  - `require.Len(t, slice, n)` — asserts slice length

**Run Commands:**
```bash
go test ./...          # Run all tests
go test -v ./...       # Verbose output
go test -run TestName  # Run specific test
go test -count=1 ./... # Disable test caching
```

## Test File Organization

**Location:**
- Co-located with source files in the same package
- Single test file exists: `reader_test.go` at project root

**Naming:**
- File: `<source_file>_test.go` (e.g., `reader_test.go` for `reader.go`)
- Package: same package as source (`package main`) — white-box testing style

**Structure:**
```
/home/user/project/
└── reader_test.go   # Only test file — tests CalProxy.download()
```

No `testdata/` directory. No test subdirectories.

## Test Structure

**Test function naming:**
```go
func TestDownload(t *testing.T) {
    // ...
}
```
- Follows standard Go `Test<FunctionName>` convention

**Current test:**
```go
func TestDownload(t *testing.T) {
    calProxy := NewCalProxy(ReadConfig())
    src1 := calProxy.config.Srcs()[0]
    events, err := calProxy.download(src1)
    require.NoError(t, err)

    saveEvents(events, "events.json")  // Side-effect: writes to disk

    require.Len(t, events, 2)
}
```

**Patterns observed:**
- No setup/teardown (no `TestMain`, no `t.Cleanup`)
- No table-driven tests (`[]struct{ ... }` pattern not used)
- No subtests (`t.Run(...)` not used)
- Test reads live config from environment via `ReadConfig()` — requires real env vars at test time

## Mocking

**Framework:** None — no mock library present (no `testify/mock`, no `gomock`).

**What is mocked:** Nothing. The only test hits a real CalDAV endpoint using credentials from environment variables.

**What is NOT mocked:**
- Network calls (real HTTP/CalDAV connection made)
- Config loading (real env vars required)
- File I/O (`saveEvents` writes `events.json` to the working directory during test)

## Fixtures and Factories

**Test Data:**
- No static fixtures or factories
- `events.json` is written as a test side-effect (not committed as fixture input)
- `saveEvents` helper function defined in test file but has no return value check at call site

**Location:**
- No dedicated fixtures directory

## Coverage

**Requirements:** None enforced — no coverage threshold configured.

**View Coverage:**
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Test Types

**Unit Tests:**
- None in the strict sense — the one test requires live network access

**Integration Tests:**
- `TestDownload` in `reader_test.go` is effectively an integration test:
  - Requires `SRC_1_URL`, `SRC_1_USERNAME`, `SRC_1_PASSWORD` env vars
  - Makes a real CalDAV network request
  - Asserts exactly 2 events returned (hardcoded expectation)

**E2E Tests:**
- Not present

## CI/CD Pipeline

**Location:** `.github/workflows/main.yml`

**Trigger:** Push to `main` branch only.

**Pipeline steps:**
1. Checkout code
2. Setup Go `>=1.23`
3. Install Dagger CLI
4. Run `dagger call build-and-push-image` from `ci/` directory

**Tests in CI:** ❌ **Tests are NOT run in CI.** The workflow only builds and pushes the Docker image. No `go test` step exists.

**CI build system:** Dagger (`ci/dagger/src/index.ts`) — TypeScript-based Dagger module that:
- Compiles with `golang:1.23-alpine`
- Builds with `htmgo build`
- Publishes to `docker.io/mheers/cal-anon-proxy:latest`

The Dagger build pipeline also does not invoke `go test`.

## Test Coverage Gaps

**`calendar.go` — Untested:**
- `calendarBackend` methods (`GetCalendar`, `GetCalendarObject`, `ListCalendarObjects`, etc.)
- `CalDavHandler.SetEvents` — calendar assembly logic
- Files: `calendar.go`
- Risk: CalDAV response format breakage goes undetected
- Priority: High

**`config.go` — Untested:**
- `ReadConfig()` env var parsing
- `Config.Srcs()` — all 4 source-slot branching logic
- Files: `config.go`
- Risk: Config parsing regressions silent
- Priority: High

**`middleware.go` — Untested:**
- `auth.middleware` Basic Auth enforcement
- `tracingMiddleware` request logging
- Files: `middleware.go`
- Risk: Auth bypass regressions go undetected
- Priority: High

**`reader.go` — Partially tested:**
- `download()` covered by `TestDownload` but only via live network
- `harmonizeDurationAndEnd`, `toTZ`, `summaryOfEvent`, `contains` — no isolated unit tests
- Files: `reader.go`
- Risk: Timezone/duration logic regressions
- Priority: Medium

**No CI test execution:**
- Tests are never run automatically on push or pull request
- Priority: High — any merge can silently break the single existing test

---

*Testing analysis: 2026-03-25*
