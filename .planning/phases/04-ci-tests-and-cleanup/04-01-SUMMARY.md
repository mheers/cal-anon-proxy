# 04-01 Summary: CI Test Step + FullCalendar Version Fix

## What Was Done

Added `go test -race ./...` as a CI step in GitHub Actions (runs before the Dagger/Docker build), and fixed a FullCalendar CSS/JS version mismatch where the CSS was pinned to v5.5.1 while the JS was already at v6.1.15.

## Files Modified

- `.github/workflows/main.yml` — inserted a new "Run tests" step (`go test -race ./...`) between "Setup Go" and "Install Dagger CLI"; no secrets required since the integration test is behind `//go:build integration`
- `pages/index.go` — updated FullCalendar CSS CDN URL from `fullcalendar@5.5.1/main.min.css` to `fullcalendar@6.1.15/main.min.css`, matching the existing JS references

## Verification Results

```
=== Task 1: go test step present ===
21:        run: go test -race ./...
PASS

=== Task 2: no v5 CSS reference remains ===
No v5 references remain — PASS
PASS: no v5

=== All FullCalendar refs now at v6.1.15 ===
h.Link("https://cdn.jsdelivr.net/npm/fullcalendar@6.1.15/main.min.css", "stylesheet"),
h.Script("https://cdn.jsdelivr.net/npm/fullcalendar@6.1.15/index.global.min.js"),
h.Script("https://cdn.jsdelivr.net/npm/@fullcalendar/icalendar@6.1.15/index.global.min.js"),

=== Build + unit tests ===
ok      github.com/mheers/cal-anon-proxy
(all other packages: no test files — expected)
```

## Key Decisions

- The "Run tests" step requires no `env` block or secrets because `reader_test.go` carries `//go:build integration` (added in Phase 1/3), so only the fixture-based unit tests in `reader_unit_test.go` run automatically.
- No other workflow changes were made — the Dagger build step, its secrets, and all other steps are untouched.
- The FullCalendar CSS fix is a one-line CDN URL change; JS lines were already correct and left unmodified.
