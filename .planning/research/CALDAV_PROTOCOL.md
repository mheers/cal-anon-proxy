# CalDAV Protocol Research: cal-anon-proxy

**Researched:** 2026-03-25  
**Scope:** Protocol details relevant to the existing Go implementation  
**Libraries:** `github.com/emersion/go-ical v0.0.0-20240127095438`, `github.com/mheers/go-webdav` (fork of `emersion/go-webdav v0.5.1`)

---

## 1. EXDATE — Exception Dates for Recurring Events

### What EXDATE Means
RFC 5545 §3.8.5.1: `EXDATE` lists specific datetimes that are _excluded_ from a recurrence set. When a user deletes a single occurrence of a recurring event (without editing it), the server removes that occurrence by appending the deleted datetime to the master event's `EXDATE` property.

A single `VEVENT` can have **multiple `EXDATE` properties**, each potentially listing comma-separated datetimes.

### How the go-ical Library Handles It
`components.go` `RecurrenceSet()` iterates `comp.Props[PropExceptionDates]` (line 39–45) and calls `ruleSet.ExDate(exdate)` for each. This is correct.

**Critical bug in go-ical `components.go` (line 46–52):** The `RDate` loop re-reads `PropExceptionDates` instead of `PropRecurrenceDates`:
```go
// BUG: should be PropRecurrenceDates, not PropExceptionDates
for _, rdateProp := range comp.Props[PropExceptionDates] {  // ← wrong
    rdate, err := rdateProp.DateTime(loc)
    ruleSet.RDate(rdate)
}
```
**Impact:** RDATE additions to a recurrence set are silently ignored. EXDATE exclusions are doubled (once as ExDate, once as RDate which cancels nothing). Upstream bug — filed awareness only; workaround is to expand RRULE manually or avoid RDATE.

### What the Current Code Does
`reader.go` requests only `RRULE` in `CompRequest.Props` (line 65) but does **not** request `EXDATE`. This means:
- The server-side time-range filter (`CompFilter`) may correctly exclude fully-deleted occurrences (server handles it).
- But if the proxy's `QueryCalendarObjects` backend ever needs to re-filter locally (see §2), `EXDATE` must be in the requested props.

**Fix required in `reader.go`:** Add `"EXDATE"` and `"RECURRENCE-ID"` to the `Props` list:
```go
Props: []string{
    "SUMMARY", "UID", "DTSTART", "DTEND", "DURATION",
    "RRULE", "EXDATE", "RECURRENCE-ID",   // ← add these
},
```

### RECURRENCE-ID — Modified Occurrences
When a user edits a _single_ occurrence of a recurring event, the server creates a second `VEVENT` component (an "override") with the same `UID` but with a `RECURRENCE-ID` property pointing to the original occurrence datetime. The proxy must pass these override VEVENTs through; stripping them causes the modified occurrence to revert to the master event's content.

### EXDATE with DATE (all-day) vs DATE-TIME
EXDATE must match the value type of DTSTART. If DTSTART is a `DATE` (all-day), EXDATE must also use `DATE` format. Mixed types cause rrule-go parsing to silently mismatch.

---

## 2. `QueryCalendarObjects` — The REPORT Method

### Why It Matters
When a CalDAV client (e.g. Apple Calendar, Thunderbird) subscribes to the proxy's served calendar, it sends a `REPORT` HTTP request with a `calendar-query` XML body. The go-webdav `Handler.handleQuery()` in `server.go` (line 210–245) decodes the filter and calls `Backend.QueryCalendarObjects()`.

**Current state:** `calendar.go` line 83–85 returns `nil, nil` — an empty response. This means **every CalDAV REPORT request to the proxy returns zero events**. Clients that use REPORT (which is most of them) see an empty calendar.

The `Backend.ListCalendarObjects` path (PROPFIND Depth:1) works and returns events, but clients that do REPORT-based sync (most desktop/mobile clients) silently get nothing.

### Minimal Correct Implementation

The library provides `caldav.Filter()` in `match.go` which does the heavy lifting:

```go
func (b *calendarBackend) QueryCalendarObjects(
    ctx context.Context,
    path string,
    query *caldav.CalendarQuery,
) ([]caldav.CalendarObject, error) {
    // Get all objects for this calendar path
    objects, ok := b.objectMap[path]
    if !ok {
        return nil, nil
    }
    // Use the library's built-in filter (handles time-range, comp-filter, etc.)
    return caldav.Filter(query, objects)
}
```

`caldav.Filter` calls `caldav.Match` per object, which calls `matchCompTimeRange` (which uses `RecurrenceSet` for recurring events) and `matchCompFilter` for nested filters. This is the correct approach — do not reimplement filter logic.

### How `SetEvents` Packs Data
`calendar.go` `SetEvents()` collapses all events into a **single `CalendarObject`** at `h.path`. This means `objectMap[path]` contains one object with all VEVENTs merged. `QueryCalendarObjects` will then run the filter against this single mega-object.

`matchCompTimeRange` in `match.go` calls `comp.RecurrenceSet()` which only exists on a single VEVENT component, not on VCALENDAR. The match function checks `comp.Name != ical.CompEvent` and returns false for VCALENDAR. So the single-object approach will fail time-range filtering.

**Better fix:** Store each source `CalendarObject` individually rather than merging:
```go
// In SetEvents, store each event as its own CalendarObject
objectMap := map[string][]caldav.CalendarObject{}
for _, event := range events {
    objectMap[cal.Path] = append(objectMap[cal.Path], *event)
}
```
This lets `Filter` evaluate each VEVENT independently.

---

## 3. Time-Range Filtering

### CalDAV Time-Range Filter (RFC 4791 §9.9)
The `CompFilter` with `Start`/`End` set means: "return calendar objects that have at least one VEVENT overlapping this time range." The library's `matchCompTimeRange` implements this correctly for:
- Simple events: checks start/end overlap
- Recurring events: expands via `RecurrenceSet().Between(start, end, true)` using rrule-go

### Current reader.go Query
`reader.go` sends a time-range to the upstream server (line 54–77). This is correct for fetching — the remote CalDAV server filters before returning.

**Problem:** The query window starts at `time.Now().Weekday()` (current week start) with no timezone. Since `queryStart` is in local time but the CalDAV time-range filter uses UTC (`dateWithUTCTimeLayout = "20060102T150405Z"`), the `encodeCompFilter` in `client.go` converts `filter.Start` to UTC. This is correct if `time.Now()` is used directly — Go's `time.Now()` returns local time which marshals to UTC correctly.

**Edge case:** The 6-week window (`queryStart.AddDate(0, 0, 7*6)`) is fine, but if a recurring event starts _before_ the window but has instances _within_ it, a compliant server should still return it. Not all servers are compliant — some return only events where DTSTART is in-range. The proxy should not assume completeness and should handle RECURRENCE-ID overrides that reference outside-window base events.

### Timezone in Time-Range
RFC 4791 §9.9 says time-range start/end MUST be in UTC (`YYYYMMDDTHHMMSSZ` format). The library enforces this via `dateWithUTCTime` — correct.

---

## 4. Single-Calendar Selection

### Current Behavior
`reader.go` line 45: `calendar := calendars[0]` — always picks the first calendar returned by `FindCalendars`.

### Why This Is Fragile
`FindCalendars` uses a PROPFIND Depth:1 on the calendar home set. The order of returned calendars is server-defined and not stable. On Exchange/Office 365, the first calendar may be a system calendar (birthdays, contacts, etc.) not the user's primary events calendar. The response order can change between requests.

### Fix Options

**Option A: Config-based calendar name (recommended)**  
Add `SrcNCalendarName` env vars to `Config`:
```go
Src1CalendarName string `env:"SRC_1_CALENDAR_NAME"`
```
Then in `reader.go`:
```go
calendar, err := findCalendarByName(calendars, src.CalendarName)
```
Where `findCalendarByName` falls back to `calendars[0]` if name is empty.

**Option B: Config-based calendar path**  
Add `SrcNCalendarPath` — use the exact path. Most robust but requires user to know the internal path.

**Option C: Filter by `SupportedComponentSet`**  
Only consider calendars that include `VEVENT` in `SupportedComponentSet`. This filters out task-only calendars but not multiple event calendars.

**What `Calendar` struct provides for matching:**
```go
type Calendar struct {
    Path                  string
    Name                  string   // displayname — use this for matching
    Description           string
    MaxResourceSize       int64
    SupportedComponentSet []string // ["VEVENT"] — use to filter non-event calendars
}
```

**Recommended:** Add `SRC_N_CALENDAR_NAME` env var. If set, find the calendar by `calendar.Name == src.CalendarName`. Also filter out calendars where `SupportedComponentSet` doesn't include `VEVENT`.

---

## 5. iCal Edge Cases Beyond DURATION vs DTEND

The existing `harmonizeDurationAndEnd` handles the RFC 5545 §3.6.1 rule that VEVENT must have either DTEND or DURATION (not both, not neither). Here are similar edge cases:

### 5a. All-Day Events: DATE vs DATE-TIME
DTSTART can be `VALUE=DATE` (e.g. `20240315`) for all-day events, or `VALUE=DATE-TIME` for timed events. The current `toTZ` function calls `prop.DateTime(tz)` which handles both, but then calls `props.SetDateTime` which always writes DATE-TIME format. This **converts all-day events to timed events**, which is wrong — all-day events become 12:00 AM events in the target timezone.

**Fix:** Check `prop.ValueType() == ValueDate` before calling `toTZ`; skip timezone conversion for DATE values.

```go
if prop.ValueType() == ical.ValueDate {
    return nil // all-day, no tz conversion needed
}
```

### 5b. DTSTART with No DTEND and No DURATION (Zero-Duration Event)
RFC 5545 §3.6.1: If VEVENT has DTSTART of type DATE-TIME and neither DTEND nor DURATION, the event has zero duration. The current `harmonizeDurationAndEnd` returns an error in this case (`"duration not found"`). This causes the proxy to fail on legitimately valid zero-duration events (e.g. birthday reminders stored as instants).

**Fix:** Treat missing both DTEND and DURATION as zero duration → set DTEND = DTSTART:
```go
if duration == nil || duration.Value == "" {
    // Zero-duration event: set DTEND = DTSTART
    event.Data.Children[x].Props.SetDateTime(ical.PropDateTimeEnd, startTime)
    return nil
}
```

### 5c. DTSTART with VALUE=DATE and Missing DTEND (All-Day)
For all-day events with `VALUE=DATE` DTSTART and no DTEND, RFC 5545 says the event lasts exactly one day. `ical.Event.DateTimeEnd()` handles this correctly (returns DTSTART + 24h when `startProp.ValueType() == ValueDate`). But `harmonizeDurationAndEnd` will error because both DTEND and DURATION are absent. Needs same fix as 5b.

### 5d. TRANSP Property — Transparency
`TRANSP:TRANSPARENT` means the event doesn't block time (e.g. all-day public holidays). Some clients filter these out when doing free/busy queries. The proxy doesn't touch TRANSP currently, which is correct. But if anonymizing, consider preserving TRANSP so recipients can still mark time as free.

### 5e. RECURRENCE-ID with RANGE=THISANDFUTURE
Some clients (older Outlook) send `RECURRENCE-ID;RANGE=THISANDFUTURE` when editing all future occurrences. This is deprecated in RFC 5545 but still appears in the wild. The proxy passes these through without modification, which is fine — but the upstream query may not return them if the server uses server-side RRULE expansion.

### 5f. Multiple EXDATE Values on One Property vs Multiple Properties
RFC 5545 allows EXDATE to list multiple datetimes comma-separated on one property line, or as multiple separate EXDATE properties. The go-ical library's `Props` map stores them as `[]Prop` — `props[PropExceptionDates]` is a slice. `RecurrenceSet()` iterates the slice correctly. But the proxy's `CompRequest.Props` list filters by property name; make sure `"EXDATE"` is included (see §1).

### 5g. VTIMEZONE Component Stripping
`reader.go` removes all VTIMEZONE components (lines 188–194) and replaces TZID references with IANA names. This is necessary because MS Exchange embeds proprietary VTIMEZONE definitions. After timezone normalization via `toTZ`, the TZID params are set to IANA names (e.g. `Europe/London`), so VTIMEZONE blocks are unnecessary. This is correct behavior — standard IANA timezone names don't require embedded VTIMEZONE definitions per RFC 5545 §3.6.5.

---

## Summary of Actionable Fixes

| Issue | File | Priority |
|-------|------|----------|
| `QueryCalendarObjects` returns nil → REPORT gives empty calendar | `calendar.go:83` | **Critical** |
| Events merged into one object → time-range filtering fails | `calendar.go:SetEvents` | **Critical** |
| `EXDATE`/`RECURRENCE-ID` not requested from upstream | `reader.go:59` | High |
| `calendar := calendars[0]` ignores calendar selection | `reader.go:45` | High |
| All-day `VALUE=DATE` events converted to timed events by `toTZ` | `reader.go:162-169` | High |
| `harmonizeDurationAndEnd` errors on valid zero-duration events | `reader.go:246` | Medium |
| go-ical bug: `RecurrenceSet` reads EXDATE twice instead of RDATE | upstream `components.go:46` | Upstream (low priority) |
