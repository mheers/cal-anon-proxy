# Summary: 02-01 — Fix iCalendar Protocol Correctness

**Phase:** 02-fix-icalendar-protocol-correctness  
**Plan:** 01  
**Completed:** 2026-03-25  
**Requirements addressed:** REQ-05, REQ-06, REQ-07

---

## What Was Built

Three targeted fixes to `reader.go` to correct iCalendar protocol handling:

### Fix 1 — `toTZ`: VALUE=DATE early return (REQ-05)

Added a guard after the nil-prop check in `toTZ()` (line 243):

```go
if prop.ValueType() == ical.ValueDate {
    return nil
}
```

All-day events carry `DTSTART;VALUE=DATE` (no time component). Previously, `toTZ` would call `prop.DateTime(tz)` and then `props.SetDateTime(...)`, silently converting the DATE property into a `DATE-TIME` at `00:00:00 Europe/London`. With this guard, all-day events are passed through completely unchanged — `VALUE=DATE` format is preserved.

### Fix 2 — `harmonizeDurationAndEnd`: zero-duration case (REQ-06)

Replaced two separate error returns for missing DURATION with a unified zero-duration handler:

```go
if duration == nil || duration.Value == "" {
    // Zero-duration event: DTSTART only, no DTEND or DURATION — RFC 5545 §3.6.1 valid case
    startTime, err := start.DateTime(time.UTC)
    if err != nil {
        return err
    }
    event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, startTime)
    return nil
}
```

Previously, birthday reminders and other zero-duration events (DTSTART only, no DTEND or DURATION) caused the entire source download to fail with an error. RFC 5545 §3.6.1 explicitly allows this; the fix sets `DTEND = DTSTART` as the canonical representation.

### Fix 3 — `CompRequest.Props`: EXDATE + RECURRENCE-ID (REQ-07)

Added two property names to the CalDAV VEVENT comp request:

```go
"EXDATE",
"RECURRENCE-ID",
```

Without these, the upstream CalDAV server omits exception and override data from its REPORT response. `EXDATE` carries deleted occurrence dates for recurring events; `RECURRENCE-ID` identifies modified single occurrences. Both are now requested and forwarded to CalDAV clients.

---

## Files Modified

| File | Change |
|------|--------|
| `reader.go` | `toTZ`: VALUE=DATE early return guard |
| `reader.go` | `harmonizeDurationAndEnd`: zero-duration sets DTEND=DTSTART instead of error |
| `reader.go` | `CompRequest.Props`: added `"EXDATE"` and `"RECURRENCE-ID"` |

---

## Verification Results

```
go build ./...   → BUILD OK
go vet ./...     → VET OK
go test -race ./... → TESTS OK (no races, no panics)
```

Key pattern checks passed:
- `grep "ValueDate" reader.go` → line 243 (toTZ DATE guard)
- `grep "EXDATE\|RECURRENCE-ID" reader.go` → lines 76–77 (CompRequest.Props)

---

## Decisions & Notes

- `prop.ValueType()` reads the `VALUE=` parameter from the ical prop — this is the correct go-ical API for detecting all-day events (not string-matching the raw value)
- `ical.ValueDate` is `"DATE"` constant from go-ical — matches the `VALUE=DATE` parameter
- Zero-duration fix computes `startTime` locally within the early-return branch (avoids moving the existing `startTime` declaration)
- No new dependencies introduced; all fixes use existing go-ical and go-webdav APIs

---

## Patterns Established

- **DATE guard pattern:** `prop.ValueType() == ical.ValueDate` — use this in any future code that processes date/time props and needs to distinguish all-day from timed events
- **Zero-duration convention:** Represent zero-duration events as `DTEND = DTSTART` (RFC 5545 compliant, widely accepted by CalDAV clients)
