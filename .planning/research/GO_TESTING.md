# Go Testing: Unit Tests for `reader.go`

**Codebase:** `github.com/mheers/cal-anon-proxy`  
**Researched:** 2026-03-25  
**Confidence:** HIGH (standard Go patterns, verified against official docs)

---

## 1. Build Tag: Isolating the Integration Test

Add `//go:build integration` to `reader_test.go` so `go test ./...` never requires live credentials.

```go
//go:build integration

package main
```

Run integration tests explicitly:
```bash
go test -tags integration ./...
```

Unit tests run without any tag: `go test ./...`

**Why:** The existing `TestDownload` calls `NewCalProxy(ReadConfig())` which reads real env vars and hits a live CalDAV server. Tagging it `integration` keeps CI green without credentials.

---

## 2. `testdata/` Fixture Files for iCal Parsing

Place `.ics` fixtures under `testdata/` — Go tooling ignores this directory during builds but `os.ReadFile` and `os.Open` resolve paths relative to the test's working directory (the package root), so `testdata/event_with_duration.ics` just works.

```
testdata/
  event_with_duration.ics      # DTSTART + DURATION, no DTEND
  event_with_dtend.ics         # DTSTART + DTEND, no DURATION
  event_zero_duration.ics      # DURATION=PT0S edge case
  event_ms_timezone.ics        # TZID="Eastern Standard Time" (MS tz name)
  event_no_summary.ics         # missing SUMMARY prop
  event_anon.ics               # for testing Anon=true stripping
```

**Minimal fixture skeleton** (`event_with_duration.ics`):
```ical
BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:test-duration-001@example.com
SUMMARY:Team Standup
DTSTART;TZID=UTC:20260325T090000
DURATION:PT1H
END:VEVENT
END:VCALENDAR
```

**Parsing fixture into `*caldav.CalendarObject`:**
```go
import (
    "os"
    "github.com/emersion/go-ical"
    "github.com/emersion/go-webdav/caldav"
)

func loadFixture(t *testing.T, path string) *caldav.CalendarObject {
    t.Helper()
    f, err := os.Open(path)
    if err != nil {
        t.Fatalf("open fixture: %v", err)
    }
    defer f.Close()
    cal, err := ical.NewDecoder(f).Decode()
    if err != nil {
        t.Fatalf("decode fixture: %v", err)
    }
    return &caldav.CalendarObject{Data: cal}
}
```

---

## 3. Table-Driven Tests

Go's idiomatic pattern for the pure functions in this file:

```go
func TestHarmonizeDurationAndEnd(t *testing.T) {
    tests := []struct {
        name        string
        fixture     string   // path under testdata/
        wantErr     bool
        wantDTEND   string   // expected DTEND value after harmonize
        wantNoDURATION bool
    }{
        {
            name:           "adds DTEND from DURATION",
            fixture:        "testdata/event_with_duration.ics",
            wantDTEND:      "20260325T100000Z",
            wantNoDURATION: true,
        },
        {
            name:    "no-op when DTEND already present",
            fixture: "testdata/event_with_dtend.ics",
            wantErr: false,
        },
        {
            name:    "zero duration is no-op",
            fixture: "testdata/event_zero_duration.ics",
            wantErr: false,
        },
        {
            name:    "error when no DTSTART and no DTEND",
            fixture: "testdata/event_missing_start.ics",
            wantErr: true,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            obj := loadFixture(t, tc.fixture)
            err := harmonizeDurationAndEnd(obj, 0)
            if tc.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            if tc.wantDTEND != "" {
                end := obj.Data.Children[0].Props.Get(ical.PropDateTimeEnd)
                require.NotNil(t, end)
                require.Equal(t, tc.wantDTEND, end.Value)
            }
            if tc.wantNoDURATION {
                dur := obj.Data.Children[0].Props.Get(ical.PropDuration)
                require.Nil(t, dur)
            }
        })
    }
}
```

Same pattern applies to `TestToTZ` and `TestSummaryOfEvent`.

---

## 4. Testing `summaryOfEvent`

This function is pure (no I/O, no side effects). No mocking needed.

```go
func TestSummaryOfEvent(t *testing.T) {
    tests := []struct {
        name    string
        fixture string
        want    string
    }{
        {"returns summary", "testdata/event_with_dtend.ics", "Team Standup"},
        {"empty when no SUMMARY", "testdata/event_no_summary.ics", ""},
        {"empty when no VEVENT", "testdata/event_empty_calendar.ics", ""},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            obj := loadFixture(t, tc.fixture)
            require.Equal(t, tc.want, summaryOfEvent(obj))
        })
    }
}
```

---

## 5. Testing `toTZ`

Key cases:
- MS timezone name in TZID param (e.g., `"Eastern Standard Time"`) → translated via `tzLib.TranslateMSTimezoneToIANA`
- Missing property → should return error (current code returns error if prop is nil — note: `toTZ` will panic if the prop is nil because it calls `.DateTime()` on the result without nil check; this is a bug to surface via test)

```go
func TestToTZ(t *testing.T) {
    london, _ := time.LoadLocation("Europe/London")

    tests := []struct {
        name     string
        fixture  string
        prop     string
        wantErr  bool
        wantTZID string
    }{
        {
            name:     "converts UTC to London",
            fixture:  "testdata/event_with_dtend.ics",
            prop:     ical.PropDateTimeStart,
            wantTZID: "Europe/London",
        },
        {
            name:     "translates MS timezone name",
            fixture:  "testdata/event_ms_timezone.ics",
            prop:     ical.PropDateTimeStart,
            wantTZID: "Europe/London",
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            obj := loadFixture(t, tc.fixture)
            err := toTZ(obj, 0, london, tc.prop)
            if tc.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            tzid := obj.Data.Children[0].Props.Get(tc.prop).Params.Get(ical.PropTimezoneID)
            require.Equal(t, tc.wantTZID, tzid)
        })
    }
}
```

---

## 6. Mocking the CalDAV HTTP Client for `download()`

`download()` currently hardcodes `&http.Client{}` and `caldav.NewClient(...)` — it cannot be unit tested without refactoring. The minimum change:

### Step 1: Define an interface

```go
// In reader.go or a new caldav_client.go
type CalDAVClient interface {
    FindCalendars(ctx context.Context, homeset string) ([]caldav.Calendar, error)
    QueryCalendar(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error)
}
```

### Step 2: Inject the client

```go
// Change download() signature:
func (p *CalProxy) download(src *Src) ([]*caldav.CalendarObject, error) {
    client := p.newCalDAVClient(src)  // factory method, defaults to real client
    return p.downloadWithClient(src, client)
}

func (p *CalProxy) downloadWithClient(src *Src, client CalDAVClient) ([]*caldav.CalendarObject, error) {
    // ... existing logic using client ...
}
```

### Step 3: Mock in tests

```go
type mockCalDAVClient struct {
    calendars []caldav.Calendar
    objects   []caldav.CalendarObject
    err       error
}

func (m *mockCalDAVClient) FindCalendars(_ context.Context, _ string) ([]caldav.Calendar, error) {
    return m.calendars, m.err
}

func (m *mockCalDAVClient) QueryCalendar(_ context.Context, _ string, _ *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
    return m.objects, m.err
}
```

**Alternative (no interface refactor):** Use `net/http/httptest` with a mock CalDAV HTTP server for a higher-level test. More complex but tests real HTTP parsing.

---

## 7. `t.TempDir()` for File I/O Side Effects

`saveEvents()` in the current test writes `events.json` to the working directory — a test side effect. Fix:

```go
func TestSaveEvents(t *testing.T) {
    dir := t.TempDir() // auto-cleaned after test
    path := filepath.Join(dir, "events.json")

    events := []*caldav.CalendarObject{ /* ... */ }
    err := saveEvents(events, path)
    require.NoError(t, err)

    data, err := os.ReadFile(path)
    require.NoError(t, err)
    require.Contains(t, string(data), `"Data"`)
}
```

`t.TempDir()` creates a unique temp directory per test and removes it (including contents) when the test ends. No manual cleanup needed.

---

## 8. Recommended File Structure

```
reader.go                        # production code (unchanged initially)
reader_test.go                   # ADD: //go:build integration at top
reader_unit_test.go              # NEW: unit tests (no build tag)
testdata/
  event_with_duration.ics
  event_with_dtend.ics
  event_zero_duration.ics
  event_ms_timezone.ics
  event_no_summary.ics
  event_missing_start.ics
```

`reader_unit_test.go` has no build tag — runs with plain `go test ./...`.

---

## 9. Bug Surfaced by Tests

`toTZ` has a latent nil-dereference: if `prop` is nil, the function returns an error — but then the caller proceeds. More critically, line 236 `event.Data.Children[x].Props.Get(propName).DateTime(tz)` will panic if `Get` returns nil. Tests for missing props will expose this.

---

## 10. Quick Reference

| Goal | Approach |
|------|----------|
| Skip live tests in CI | `//go:build integration` on `reader_test.go` |
| Test pure functions | `testdata/*.ics` fixtures + table-driven tests |
| Test `download()` | Extract `CalDAVClient` interface, inject mock |
| Test file output | `t.TempDir()` + `filepath.Join` |
| Run only unit tests | `go test ./...` |
| Run integration tests | `go test -tags integration ./...` |
| Assert with testify | `require.NoError`, `require.Equal`, `require.Nil` (already a dep) |
