package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/go-chi/chi/v5"
	"github.com/maddalax/htmgo/framework/h"
	"github.com/stretchr/testify/require"
)

func testTenant(name string, webUI bool, auth bool) *Tenant {
	cfg := &Config{
		SrcUpdateInterval:        5,
		WindowPastWeeks:          4,
		WindowFutureWeeks:        8,
		CompactOverlappingEvents: true,
	}
	if auth {
		cfg.DstAuthEnabled = true
		cfg.DstUsername = "u-" + name
		cfg.DstPassword = "p-" + name
	}
	return &Tenant{Name: name, WebUIEnabled: webUI, Config: cfg}
}

// newTestRuntime builds a runtime with an (empty) backend so requests reach
// the CalDAV logic instead of the "no backend available" guard.
func newTestRuntime(name string, webUI bool, auth bool) *TenantRuntime {
	rt := buildTenantRuntime(testTenant(name, webUI, auth))
	rt.Dav.SetEvents(nil)
	return rt
}

func setFixtureEvents(t *testing.T, rt *TenantRuntime, file string) {
	t.Helper()
	fh, err := os.Open(filepath.Join("testdata", file))
	require.NoError(t, err)
	cal, err := ical.NewDecoder(fh).Decode()
	require.NoError(t, err)
	require.NoError(t, fh.Close())
	rt.Dav.SetEvents([]*caldav.CalendarObject{{Path: file, Data: cal}})
}

func withRequestContext(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), h.RequestContextKey, &h.RequestContext{}))
}

func TestRegisterRoutes_SingleTenantLegacy(t *testing.T) {
	rt := newTestRuntime("", true, false)
	r := chi.NewRouter()
	registerRoutes(r, []*TenantRuntime{rt})

	// CalDAV collection is reachable (GET is routed by chi; REPORT/PROPFIND
	// return chi's 405 for every route, including legacy — pre-existing).
	req := httptest.NewRequest(http.MethodGet, "/caldav/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusNotFound, rec.Code)

	// JSON + ICS feeds are public and empty.
	for _, path := range []string{"/events.json", "/calendar.ics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
	}
}

func TestRegisterRoutes_MultiTenant(t *testing.T) {
	marcel := newTestRuntime("marcel", true, false)
	josephine := newTestRuntime("josephine", false, true)
	setFixtureEvents(t, marcel, "event_with_dtend.ics")

	r := chi.NewRouter()
	registerRoutes(r, []*TenantRuntime{marcel, josephine})

	feedQuery := "/events.json?start=2026-03-01T00:00:00&end=2026-04-01T00:00:00&timeZone=Europe/London"

	// Tenant feed serves tenant data...
	req := httptest.NewRequest(http.MethodGet, "/marcel"+feedQuery, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Team Meeting")

	// ...isolated from the other tenant...
	req = httptest.NewRequest(http.MethodGet, "/josephine"+feedQuery, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "Team Meeting")

	// ...and the legacy path aliases the first tenant.
	req = httptest.NewRequest(http.MethodGet, feedQuery, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "Team Meeting")

	// Tenant ICS + CalDAV routes exist with the same behavior as legacy.
	for _, path := range []string{"/marcel/calendar.ics", "/josephine/calendar.ics", "/calendar.ics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
	}
	legacy := get(t, r, "/caldav/")
	require.NotEqual(t, http.StatusNotFound, legacy)
	require.Equal(t, legacy, get(t, r, "/marcel/caldav/"),
		"tenant CalDAV must behave exactly like the legacy alias")
	require.Equal(t, http.StatusNotFound, get(t, r, "/nope/caldav/"))
}

func get(t *testing.T, r *chi.Mux, path string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestRegisterRoutes_WebUIToggle(t *testing.T) {
	marcel := newTestRuntime("marcel", true, false)
	josephine := newTestRuntime("josephine", false, false)

	r := chi.NewRouter()
	registerRoutes(r, []*TenantRuntime{marcel, josephine})

	// Enabled WebUI renders with the tenant-scoped feed URL.
	req := withRequestContext(httptest.NewRequest(http.MethodGet, "/marcel", nil))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "/marcel/events.json")

	// Disabled WebUI 404s (both spellings)...
	for _, path := range []string{"/josephine", "/josephine/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code, path)
	}

	// ...but its data endpoints keep working.
	req = httptest.NewRequest(http.MethodGet, "/josephine/events.json", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRegisterRoutes_PerTenantAuth(t *testing.T) {
	marcel := newTestRuntime("marcel", true, false)
	josephine := newTestRuntime("josephine", true, true)

	r := chi.NewRouter()
	registerRoutes(r, []*TenantRuntime{marcel, josephine})

	// Open tenant: no auth required (reaches the handler).
	req := httptest.NewRequest(http.MethodGet, "/marcel/caldav/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusUnauthorized, rec.Code)
	require.NotEqual(t, http.StatusNotFound, rec.Code)

	// Protected tenant: 401 without creds...
	req = httptest.NewRequest(http.MethodGet, "/josephine/caldav/", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// ...passes with its own creds...
	req = httptest.NewRequest(http.MethodGet, "/josephine/caldav/", nil)
	req.SetBasicAuth("u-josephine", "p-josephine")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusUnauthorized, rec.Code)
	require.NotEqual(t, http.StatusNotFound, rec.Code)

	// ...and rejects the other tenant's credentials.
	req = httptest.NewRequest(http.MethodGet, "/josephine/caldav/", nil)
	req.SetBasicAuth("u-marcel", "p-marcel")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// JSON feed stays public by design even for protected tenants.
	req = httptest.NewRequest(http.MethodGet, "/josephine/events.json", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestBuildTenantRuntime_Prefix(t *testing.T) {
	legacy := buildTenantRuntime(&Tenant{Name: "", Config: &Config{}})
	require.Equal(t, "/caldav/", legacy.Dav.path)

	marcel := buildTenantRuntime(testTenant("marcel", true, false))
	require.Equal(t, "/marcel/caldav/", marcel.Dav.path)
	require.True(t, strings.HasPrefix(marcel.Dav.path, "/marcel/"))
}
