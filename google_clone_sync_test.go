package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// fakeCalendarAPI is a minimal in-memory Google Calendar REST API for testing
// cloneGoogleCalendarWindow without touching real Google infrastructure.
type fakeCalendarAPI struct {
	mu sync.Mutex

	source      map[string][]*calendar.Event // calendarID -> events
	dest        map[string][]*calendar.Event // calendarID -> events
	inserts     int
	deletes     []string
	insertFails map[int]int // nth insert (1-based) -> status code

	nextEventID int
}

func newFakeCalendarAPI() *fakeCalendarAPI {
	return &fakeCalendarAPI{
		source: map[string][]*calendar.Event{},
		dest:   map[string][]*calendar.Event{},
	}
}

func (f *fakeCalendarAPI) start(t *testing.T) *calendar.Service {
	t.Helper()
	// NOTE: option.WithEndpoint replaces the client's entire BasePath, so the
	// generated client resolves relative paths like "calendars/{id}/events"
	// directly against srv.URL (no /calendar/v3 prefix).
	mux := http.NewServeMux()
	mux.HandleFunc("/calendars/", f.handle)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	svc, err := calendar.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
		option.WithScopes(calendar.CalendarScope),
	)
	require.NoError(t, err)
	return svc
}

func (f *fakeCalendarAPI) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calendars/"), "/")
	if len(parts) < 2 {
		http.Error(w, `{"error":{"code":404,"message":"bad path"}}`, http.StatusNotFound)
		return
	}
	calID := parts[0]

	switch {
	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet:
		items := f.dest[calID]
		if calID == "src" {
			items = f.source[calID]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kind":  "calendar#events",
			"items": items,
		})

	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodPost:
		f.inserts++
		if code, ok := f.insertFails[f.inserts]; ok {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": code, "message": "injected failure"},
			})
			return
		}
		var ev calendar.Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			http.Error(w, `{"error":{"code":400,"message":"bad json"}}`, http.StatusBadRequest)
			return
		}
		f.nextEventID++
		ev.Id = fmt.Sprintf("new-%d", f.nextEventID)
		f.dest[calID] = append(f.dest[calID], &ev)
		_ = json.NewEncoder(w).Encode(&ev)

	case len(parts) == 3 && parts[1] == "events" && r.Method == http.MethodDelete:
		eid := parts[2]
		f.deletes = append(f.deletes, eid)
		kept := f.dest[calID][:0]
		for _, ev := range f.dest[calID] {
			if ev.Id != eid {
				kept = append(kept, ev)
			}
		}
		f.dest[calID] = kept
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, `{"error":{"code":404,"message":"not found"}}`, http.StatusNotFound)
	}
}

// Accessors (lock-safe snapshots for assertions).

func (f *fakeCalendarAPI) destStore() []*calendar.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*calendar.Event, len(f.dest["dst"]))
	copy(out, f.dest["dst"])
	return out
}

func (f *fakeCalendarAPI) destIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.dest["dst"]))
	for _, ev := range f.dest["dst"] {
		ids = append(ids, ev.Id)
	}
	return ids
}

func (f *fakeCalendarAPI) insertCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inserts
}

func (f *fakeCalendarAPI) deleteLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.deletes))
	copy(out, f.deletes)
	return out
}

func (f *fakeCalendarAPI) resetLogs() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserts = 0
	f.deletes = nil
}

func (f *fakeCalendarAPI) setSource(events ...*calendar.Event) { f.source["src"] = events }
func (f *fakeCalendarAPI) setDest(events ...*calendar.Event)   { f.dest["dst"] = events }

func testWindow() (time.Time, time.Time) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 30)
}

func TestCloneGoogleCalendarWindow_HappyPath(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource(
		&calendar.Event{Id: "s1", Summary: "One", Start: &calendar.EventDateTime{DateTime: "2026-03-02T10:00:00Z"}, End: &calendar.EventDateTime{DateTime: "2026-03-02T11:00:00Z"}},
		&calendar.Event{Id: "s2", Summary: "Two", Start: &calendar.EventDateTime{DateTime: "2026-03-03T10:00:00Z"}, End: &calendar.EventDateTime{DateTime: "2026-03-03T11:00:00Z"}},
	)
	api.setDest(
		&calendar.Event{Id: "d1"},
		&calendar.Event{Id: "d2"},
		&calendar.Event{Id: "d3"},
	)

	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		SourceCalendarID: "src",
		DestCalendarID:   "dst",
		WipeDestination:  true,
	}

	inserted, deleted, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, 2, inserted)
	require.Equal(t, 3, deleted)
	require.Len(t, api.destStore(), 2)

	for _, ev := range api.destStore() {
		require.Equal(t, clonedByValue, ev.ExtendedProperties.Private[clonedByMarker],
			"every cloned event must carry the clonedBy marker")
	}
}

func TestCloneGoogleCalendarWindow_DryRunWritesNothing(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource(&calendar.Event{Id: "s1", Status: "confirmed"})
	api.setDest(&calendar.Event{Id: "d1"})

	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		SourceCalendarID: "src",
		DestCalendarID:   "dst",
		WipeDestination:  true,
		DryRun:           true,
	}

	inserted, deleted, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	require.Equal(t, 1, deleted)
	require.Len(t, api.destStore(), 1, "dry-run must not delete")
	require.Equal(t, 0, api.insertCount(), "dry-run must not insert")
	require.Empty(t, api.deleteLog(), "dry-run must not delete")
}

func TestCloneGoogleCalendarWindow_CancelledEventsSkipped(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource(
		&calendar.Event{Id: "s1", Status: "confirmed"},
		&calendar.Event{Id: "s2", Status: "cancelled"},
	)

	cfg := googleCloneConfig{SourceCalendarID: "src", DestCalendarID: "dst"}

	inserted, _, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
}

func TestCloneGoogleCalendarWindow_InsertFailureMidway(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource(
		&calendar.Event{Id: "s1"},
		&calendar.Event{Id: "s2"},
	)
	api.insertFails = map[int]int{2: http.StatusInternalServerError}

	cfg := googleCloneConfig{SourceCalendarID: "src", DestCalendarID: "dst"}

	inserted, deleted, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.Error(t, err)
	require.Equal(t, 1, inserted)
	require.Equal(t, 0, deleted)

	var gErr *googleapi.Error
	require.ErrorAs(t, err, &gErr)
	require.Equal(t, http.StatusInternalServerError, gErr.Code)
}

func TestCloneGoogleCalendarWindow_EmptySourceAbortsWipe(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource()
	api.setDest(&calendar.Event{Id: "d1"}, &calendar.Event{Id: "d2"})

	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		ClientID:         "x",
		RefreshToken:     "y",
		SourceCalendarID: "src",
		DestCalendarID:   "dst",
		WipeDestination:  true,
	}

	_, _, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to touch destination")

	require.Empty(t, api.deleteLog(), "empty source must never trigger deletes")
	require.Equal(t, 0, api.insertCount())
}

func TestCloneGoogleCalendarWindow_EmptySourceAllowedOverride(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource()
	api.setDest(&calendar.Event{Id: "d1"})

	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		ClientID:         "x",
		RefreshToken:     "y",
		SourceCalendarID: "src",
		DestCalendarID:   "dst",
		WipeDestination:  true,
		AllowEmptySource: true,
	}

	inserted, deleted, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
	require.Equal(t, 1, deleted, "explicit override permits the wipe")
}

func TestCloneGoogleCalendarWindow_FirstRunFullWipeThenTaggedOnly(t *testing.T) {
	api := newFakeCalendarAPI()
	svc := api.start(t)
	windowStart, windowEnd := testWindow()

	api.setSource(&calendar.Event{Id: "s1"})
	api.setDest(&calendar.Event{Id: "manual-1"})

	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		ClientID:         "x",
		RefreshToken:     "y",
		SourceCalendarID: "src",
		DestCalendarID:   "dst",
		WipeDestination:  true,
	}

	// First run: no tagged events exist yet -> full wipe establishes the mirror.
	_, deleted, err := cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, 1, deleted, "first run wipes untagged events too")

	// Simulate a manually created event appearing between syncs.
	store := api.destStore()
	api.setDest(store[0], &calendar.Event{Id: "manual-2"})
	api.resetLogs()

	// Second run: only tagged events are removed; manual events survive.
	_, deleted, err = cloneGoogleCalendarWindow(context.Background(), svc, cfg, windowStart, windowEnd)
	require.NoError(t, err)
	require.Equal(t, 1, deleted, "subsequent runs only delete cloned events")

	require.Contains(t, api.destIDs(), "manual-2", "manual destination event must survive")
}

func TestCloneEventForDestination_TaggingAndCopies(t *testing.T) {
	src := &calendar.Event{
		Id:          "source-id",
		Summary:     "Title",
		Description: "Desc",
		Location:    "Somewhere",
		Status:      "cancelled",
		Start:       &calendar.EventDateTime{DateTime: "2026-03-02T09:00:00Z", TimeZone: "Europe/London"},
		End:         &calendar.EventDateTime{DateTime: "2026-03-02T10:00:00Z", TimeZone: "Europe/London"},
		Recurrence:  []string{"RRULE:FREQ=DAILY;COUNT=2"},
		Reminders: &calendar.EventReminders{
			UseDefault: false,
			Overrides:  []*calendar.EventReminder{{Method: "email", Minutes: 10}},
		},
	}

	clone := cloneEventForDestination(src)

	require.Equal(t, "Title", clone.Summary)
	require.Equal(t, "Desc", clone.Description)
	require.Equal(t, "Somewhere", clone.Location)
	require.Equal(t, "confirmed", clone.Status, "clones must not inherit cancelled status")
	require.Equal(t, src.Recurrence, clone.Recurrence)
	require.True(t, isClonedEvent(clone))
	require.Equal(t, "source-id", clone.ExtendedProperties.Private["sourceEventId"])
	require.Equal(t, "2026-03-02T09:00:00Z", clone.Start.DateTime)
	require.Equal(t, "Europe/London", clone.Start.TimeZone)

	clone.Reminders.Overrides[0].Minutes = 99
	require.Equal(t, int64(10), src.Reminders.Overrides[0].Minutes, "reminders must be deep-copied")

	nilTimes := cloneEventForDestination(&calendar.Event{Id: "x"})
	require.Nil(t, nilTimes.Start)
	require.Nil(t, nilTimes.End)
	require.True(t, isClonedEvent(nilTimes))
}

func TestIsClonedEvent(t *testing.T) {
	require.False(t, isClonedEvent(&calendar.Event{}))
	require.False(t, isClonedEvent(&calendar.Event{
		ExtendedProperties: &calendar.EventExtendedProperties{Private: map[string]string{"other": "x"}},
	}))
	require.True(t, isClonedEvent(&calendar.Event{
		ExtendedProperties: &calendar.EventExtendedProperties{Private: map[string]string{clonedByMarker: clonedByValue}},
	}))
}

func TestNewSSOHTTPClient_NoCredentialsReturnsError(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/credentials.json")
	t.Setenv("HOME", t.TempDir())
	origBinary := gcloudBinary
	gcloudBinary = "/nonexistent/gcloud-binary-for-test"
	defer func() { gcloudBinary = origBinary }()

	client, err := newSSOHTTPClient(context.Background())
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "no SSO credentials available")
	require.Contains(t, err.Error(), "gcloud probe failed")
	require.Contains(t, err.Error(), "Application Default Credentials")
}

func TestGcloudTokenSource_ReportsFailure(t *testing.T) {
	orig := gcloudBinary
	gcloudBinary = "/nonexistent/gcloud-binary-for-test"
	defer func() { gcloudBinary = orig }()

	ts := &gcloudTokenSource{}
	_, err := ts.Token()
	require.Error(t, err)
	require.Contains(t, err.Error(), "gcloud auth print-access-token failed")
}
