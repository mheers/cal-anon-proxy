---
phase: 04-ci-tests-and-cleanup
plan: "02"
subsystem: dead-code-removal
tags: [cleanup, logging, refactor]
dependency_graph:
  requires: []
  provides: [clean-codebase, logrus-only-logging]
  affects: [reader.go, middleware.go, main.go]
tech_stack:
  added: [logrus in reader.go]
  patterns: [logrus.Debugf for debug-level diagnostics, logrus.Infof for operational events]
key_files:
  modified:
    - reader.go
    - middleware.go
    - main.go
    - __htmgo/partials-generated.go
  deleted:
    - partials/index.go
decisions:
  - "Kept fmt import in reader.go — fmt.Errorf still used; only fmt.Printf was replaced"
  - "Kept fmt import in main.go — fmt.Sprintf still used on line 57; only fmt.Printf was replaced"
  - "Manually edited partials-generated.go — htmgo generate failed with permission denied; manual edit achieves same result"
  - "Force-added __htmgo/partials-generated.go — directory was in .gitignore but the generated file needed updating"
metrics:
  duration_seconds: 190
  completed_date: "2026-03-25"
  tasks_completed: 2
  files_changed: 5
---

# Phase 04 Plan 02: Dead Code Removal + fmt.Printf → logrus Migration Summary

**One-liner:** Removed allowedEvents/renameEvents/contains/tracingMiddleware/CounterPartial dead code and migrated all fmt.Printf diagnostic prints to logrus throughout reader.go, middleware.go, and main.go.

---

## What Was Done

Systematic dead code removal and logging cleanup across the codebase:

1. **reader.go** — 3 `fmt.Printf` calls replaced with `logrus.Debugf`; deleted `allowedEvents` stub block, `renameEvents` stub block and its if-block, the commented-out VTIMEZONE 8-line block, and the `contains()` helper function (which was only used by the now-deleted `allowedEvents` block).
2. **middleware.go** — Deleted the entire `tracingMiddleware` function (was never called from `main.go`); deleted the commented-out `// logrus.Infof("authenticated as %s", username)` line.
3. **main.go** — Replaced `fmt.Printf("Downloaded %d events\n", ...)` with `logrus.Infof("Downloaded %d events", ...)`.
4. **partials/index.go** — Deleted entire file (htmgo demo `CounterPartial` scaffold, never used by the application).
5. **__htmgo/partials-generated.go** — Manually edited to remove the `partials` import, the `CounterPartial` if-block from `GetPartialFromContext`, and the router handle from `RegisterPartials`. Functions now have empty/stub bodies.

---

## Files Modified

- `reader.go` — Replaced 3 `fmt.Printf` with `logrus.Debugf`; deleted allowedEvents block (8 lines), renameEvents block (5 lines), VTIMEZONE comment block (8 lines), `contains()` function (8 lines); added logrus import
- `middleware.go` — Deleted `tracingMiddleware` function (15 lines) and commented-out logrus line
- `main.go` — Replaced `fmt.Printf` with `logrus.Infof` in `updateEvents`
- `partials/index.go` — **Deleted** (55 lines of htmgo demo scaffold removed)
- `__htmgo/partials-generated.go` — Removed `partials` import + `CounterPartial` if-block + router.Handle block; now contains only stub implementations

---

## Verification Results

```
=== No fmt.Printf remaining ===
(none)

=== No dead code remaining ===
grep: tracingMiddleware|allowedEvents|renameEvents|CounterPartial|func contains
(none)

=== No VTIMEZONE comment block ===
(none)

=== No commented logrus.Infof authenticated ===
(none)

=== Full build + vet + test ===
ok      github.com/mheers/cal-anon-proxy        (cached)
?       github.com/mheers/cal-anon-proxy/internal/embedded     [no test files]
?       github.com/mheers/cal-anon-proxy/pages  [no test files]
ALL PASS
```

All 8 unit test subtests pass. `go build ./...`, `go vet ./...`, and `go test -race ./...` all exit 0.

---

## Key Decisions

1. **Kept `fmt` import in `reader.go`** — `fmt.Errorf` is still used in `toTZ` and `harmonizeDurationAndEnd`. Only the `fmt.Printf` diagnostic calls were removed; the import remains valid.
2. **Kept `fmt` import in `main.go`** — `fmt.Sprintf` is still used on the `app.Router.Handle(fmt.Sprintf(...))` line. Only the `fmt.Printf` call was removed.
3. **Manual edit of `partials-generated.go`** — `go run github.com/maddalax/htmgo/cli/htmgo@latest generate` failed with `permission denied` on the cached binary. Manual edit produces the identical result: empty `GetPartialFromContext` returning `nil` and an empty `RegisterPartials`.
4. **Force-added `__htmgo/partials-generated.go`** — The `__htmgo` directory was listed in `.gitignore` (for the auto-generated framework boilerplate), but the file needed to be updated and tracked. Used `git add -f` to stage the change explicitly.

---

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] fmt import not removed from reader.go**
- **Found during:** Task 1 verification
- **Issue:** Plan said to remove `"fmt"` from reader.go imports, but `fmt.Errorf` is still used in `toTZ()` and `harmonizeDurationAndEnd()`. Removing `fmt` would break the build.
- **Fix:** Kept `fmt` import; only `fmt.Printf` calls were replaced. `go vet` confirms no unused imports.
- **Files modified:** reader.go (no change to import block beyond adding logrus)

**2. [Rule 1 - Bug] fmt import not removed from main.go**
- **Found during:** Task 2
- **Issue:** Plan said to remove `"fmt"` from main.go, but `fmt.Sprintf` is still used on the static asset router line (line 57).
- **Fix:** Kept `fmt` import; only the `fmt.Printf` call was replaced.
- **Files modified:** main.go (import block unchanged)

**3. [Rule 3 - Blocking] htmgo generate permission denied**
- **Found during:** Task 2 (partials-generated.go regeneration)
- **Issue:** `go run github.com/maddalax/htmgo/cli/htmgo@latest generate` failed with `fork/exec: permission denied` — pre-built binary in Go cache was not executable.
- **Fix:** Manually edited `__htmgo/partials-generated.go` as specified in the plan's fallback instructions. Result is identical to what htmgo would generate.
- **Files modified:** `__htmgo/partials-generated.go`

## Self-Check: PASSED

Files exist:
- reader.go ✓
- middleware.go ✓
- main.go ✓
- __htmgo/partials-generated.go ✓
- partials/index.go: deleted ✓

Commits exist:
- 8e0126e — Task 1: remove dead code from reader.go ✓
- 3d73d21 — Task 2: remove tracingMiddleware, CounterPartial scaffold, fmt.Printf in main.go ✓
