package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testBaseConfig() *Config {
	return &Config{
		SrcUpdateInterval:        5,
		WindowPastWeeks:          4,
		WindowFutureWeeks:        8,
		CompactOverlappingEvents: true,
	}
}

func mapLookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func TestResolveTenants_LegacyFallback(t *testing.T) {
	base := testBaseConfig()
	base.Src1URL = "https://example.com/cal.ics"

	tenants, err := resolveTenantsWith(base, mapLookup(map[string]string{}))
	require.NoError(t, err)
	require.Len(t, tenants, 1)
	require.Equal(t, "", tenants[0].Name)
	require.True(t, tenants[0].WebUIEnabled)
	require.Same(t, base, tenants[0].Config)
	require.False(t, IsMultiTenant(tenants))
}

func TestResolveTenants_TwoTenants(t *testing.T) {
	base := testBaseConfig()
	env := map[string]string{
		"TENANT_1_NAME":             "marcel",
		"TENANT_1_SRC_1_URL":        "https://example.com/marcel.ics",
		"TENANT_1_SRC_1_ANON":       "true",
		"TENANT_1_DST_AUTH_ENABLED": "true",
		"TENANT_1_DST_USERNAME":     "u1",
		"TENANT_1_DST_PASSWORD":     "p1",
		"TENANT_2_NAME":             "josephine",
		"TENANT_2_WEBUI_ENABLED":    "false",
		"TENANT_2_SRC_1_URL":        "https://example.com/josephine.ics",
		"TENANT_2_SRC_2_URL":        "https://example.com/josephine2.ics",
	}

	tenants, err := resolveTenantsWith(base, mapLookup(env))
	require.NoError(t, err)
	require.Len(t, tenants, 2)
	require.True(t, IsMultiTenant(tenants))

	marcel := tenants[0]
	require.Equal(t, "marcel", marcel.Name)
	require.True(t, marcel.WebUIEnabled)
	require.True(t, marcel.Config.DstAuthEnabled)
	require.Equal(t, "u1", marcel.Config.DstUsername)
	require.Equal(t, "p1", marcel.Config.DstPassword)
	srcs := marcel.Config.Srcs()
	require.Len(t, srcs, 1)
	require.Equal(t, "https://example.com/marcel.ics", srcs[0].URL)
	require.True(t, srcs[0].Anon)

	josephine := tenants[1]
	require.Equal(t, "josephine", josephine.Name)
	require.False(t, josephine.WebUIEnabled)
	require.False(t, josephine.Config.DstAuthEnabled)
	require.Len(t, josephine.Config.Srcs(), 2)

	// Global window settings are inherited.
	require.Equal(t, 4, marcel.Config.WindowPastWeeks)
	require.Equal(t, 8, marcel.Config.WindowFutureWeeks)
	require.True(t, marcel.Config.CompactOverlappingEvents)
	require.Equal(t, 5, marcel.Config.SrcUpdateInterval)
}

func TestResolveTenants_WebUIDefaultsToTrue(t *testing.T) {
	tenants, err := resolveTenantsWith(testBaseConfig(), mapLookup(map[string]string{
		"TENANT_1_NAME":      "marcel",
		"TENANT_1_SRC_1_URL": "https://example.com/m.ics",
	}))
	require.NoError(t, err)
	require.True(t, tenants[0].WebUIEnabled)
}

func TestResolveTenants_GapTolerant(t *testing.T) {
	tenants, err := resolveTenantsWith(testBaseConfig(), mapLookup(map[string]string{
		"TENANT_3_NAME":      "marcel",
		"TENANT_3_SRC_1_URL": "https://example.com/m.ics",
	}))
	require.NoError(t, err)
	require.Len(t, tenants, 1)
	require.Equal(t, "marcel", tenants[0].Name)
}

func TestResolveTenants_InvalidNames(t *testing.T) {
	for _, name := range []string{
		"Marcel", // uppercased input is lowercased -> valid; kept here to lock behavior below
		"has space",
		"-leading",
		"trailing-",
		"under_score",
		"caldav",       // reserved
		"public",       // reserved
		"events.json",  // reserved
		"calendar.ics", // reserved
		"a/b",
	} {
		env := map[string]string{
			"TENANT_1_NAME":      name,
			"TENANT_1_SRC_1_URL": "https://example.com/m.ics",
		}
		tenants, err := resolveTenantsWith(testBaseConfig(), mapLookup(env))
		if name == "Marcel" {
			require.NoError(t, err, "name %q", name)
			require.Equal(t, "marcel", tenants[0].Name)
			continue
		}
		require.Error(t, err, "name %q should be rejected", name)
	}
}

func TestResolveTenants_DuplicateNames(t *testing.T) {
	_, err := resolveTenantsWith(testBaseConfig(), mapLookup(map[string]string{
		"TENANT_1_NAME":      "marcel",
		"TENANT_1_SRC_1_URL": "https://example.com/a.ics",
		"TENANT_2_NAME":      "Marcel", // lowercases to the same name
		"TENANT_2_SRC_1_URL": "https://example.com/b.ics",
	}))
	require.ErrorContains(t, err, "duplicate tenant name")
}

func TestResolveTenants_InvalidBool(t *testing.T) {
	_, err := resolveTenantsWith(testBaseConfig(), mapLookup(map[string]string{
		"TENANT_1_NAME":          "marcel",
		"TENANT_1_SRC_1_URL":     "https://example.com/a.ics",
		"TENANT_1_WEBUI_ENABLED": "maybe",
	}))
	require.Error(t, err)
}
