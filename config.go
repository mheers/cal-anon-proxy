package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/sethvargo/go-envconfig"
	"github.com/sirupsen/logrus"
)

type Config struct {
	// NOTE: go-envconfig expects defaults inline in the env tag ("default=5"),
	// NOT in a separate default:"..." struct tag — the latter is silently
	// ignored and left the interval at 0, panicking NewTicker on startup.
	SrcUpdateInterval int `env:"SRC_UPDATE_INTERVAL,default=5"`

	// Visibility window applied to every source: events ending before
	// (now - WindowPastWeeks) or starting after (now + WindowFutureWeeks)
	// are dropped. Recurring events are always kept — their DTSTART may be
	// months old while current occurrences still fall inside the window.
	WindowPastWeeks   int `env:"WINDOW_PAST_WEEKS,default=4"`
	WindowFutureWeeks int `env:"WINDOW_FUTURE_WEEKS,default=8"`

	// CompactOverlappingEvents merges overlapping/touching non-recurring
	// events into contiguous busy blocks (see compact.go).
	CompactOverlappingEvents bool `env:"COMPACT_OVERLAPPING_EVENTS,default=true"`

	Src1URL      string `env:"SRC_1_URL"`
	Src1Anon     bool   `env:"SRC_1_ANON"`
	Src1Username string `env:"SRC_1_USERNAME"`
	Src1Password string `env:"SRC_1_PASSWORD"`

	Src2URL      string `env:"SRC_2_URL"`
	Src2Anon     bool   `env:"SRC_2_ANON"`
	Src2Username string `env:"SRC_2_USERNAME"`
	Src2Password string `env:"SRC_2_PASSWORD"`

	Src3URL      string `env:"SRC_3_URL"`
	Src3Anon     bool   `env:"SRC_3_ANON"`
	Src3Username string `env:"SRC_3_USERNAME"`
	Src3Password string `env:"SRC_3_PASSWORD"`

	Src4URL      string `env:"SRC_4_URL"`
	Src4Anon     bool   `env:"SRC_4_ANON"`
	Src4Username string `env:"SRC_4_USERNAME"`
	Src4Password string `env:"SRC_4_PASSWORD"`

	DstAuthEnabled  bool   `env:"DST_AUTH_ENABLED"`
	DstUsername     string `env:"DST_USERNAME"`
	DstPassword     string `env:"DST_PASSWORD"`
	DstPublicDomain string `env:"DST_PUBLIC_DOMAIN"`
}

func ReadConfig() *Config {
	var c Config
	if err := envconfig.Process(context.Background(), &c); err != nil {
		log.Fatal(err)
	}
	return &c
}

func (c *Config) Srcs() []*Src {
	srcs := []*Src{}
	if c.Src1URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src1URL,
			Anon:     c.Src1Anon,
			Username: c.Src1Username,
			Password: c.Src1Password,
		})
	}

	if c.Src2URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src2URL,
			Anon:     c.Src2Anon,
			Username: c.Src2Username,
			Password: c.Src2Password,
		})
	}

	if c.Src3URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src3URL,
			Anon:     c.Src3Anon,
			Username: c.Src3Username,
			Password: c.Src3Password,
		})
	}

	if c.Src4URL != "" {
		srcs = append(srcs, &Src{
			URL:      c.Src4URL,
			Anon:     c.Src4Anon,
			Username: c.Src4Username,
			Password: c.Src4Password,
		})
	}

	return srcs
}

type Src struct {
	URL      string
	Anon     bool
	Username string
	Password string
}

// Tenant is a single named calendar tenant. Each tenant gets its own sources,
// destination auth and WebUI toggle, while the visibility window, compaction
// and refresh interval stay global (taken from the base Config).
// The per-tenant effective Config reuses the Config struct so CalProxy,
// filterWindow and compactEvents work unchanged.
type Tenant struct {
	Name         string
	WebUIEnabled bool
	Config       *Config
}

// maxTenants bounds the TENANT_<N>_NAME scan (gap-tolerant: unset slots are
// skipped, so numbering need not be contiguous).
const maxTenants = 64

var tenantNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// reservedTenantNames collide with legacy root-level routes or the public
// asset path and can never be tenant names.
var reservedTenantNames = map[string]bool{
	"public":       true,
	"caldav":       true,
	"calendar.ics": true,
	"events.json":  true,
	".well-known":  true,
}

// ResolveTenants builds the tenant list from the environment on top of the
// base (global) config. With no TENANT_*_NAME vars set it returns a single
// legacy tenant (Name "") wrapping the base config, preserving today's
// single-tenant behavior. Otherwise every TENANT_<N>_* block becomes a
// tenant; legacy SRC_*/DST_* vars are then ignored for sources/auth (with a
// warning) and the legacy routes alias tenants[0].
func ResolveTenants(base *Config) ([]*Tenant, error) {
	return resolveTenantsWith(base, os.LookupEnv)
}

func resolveTenantsWith(base *Config, lookup func(string) (string, bool)) ([]*Tenant, error) {
	var tenants []*Tenant
	// Ascending index order: deterministic, and tenants[0] is the
	// legacy-route alias target.
	for i := 1; i <= maxTenants; i++ {
		prefix := fmt.Sprintf("TENANT_%d_", i)
		rawName, ok := lookup(prefix + "NAME")
		if !ok || strings.TrimSpace(rawName) == "" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(rawName))
		if !tenantNameRe.MatchString(name) || len(name) > 63 {
			return nil, fmt.Errorf("invalid TENANT_%d_NAME %q: must match [a-z0-9-], start/end alphanumeric, max 63 chars", i, rawName)
		}
		if reservedTenantNames[name] {
			return nil, fmt.Errorf("invalid TENANT_%d_NAME %q: name is reserved", i, name)
		}

		webUIEnabled, err := parseBoolWithDefault(lookup, prefix+"WEBUI_ENABLED", true)
		if err != nil {
			return nil, fmt.Errorf("invalid %sWEBUI_ENABLED: %w", prefix, err)
		}

		cfg := &Config{
			SrcUpdateInterval:        base.SrcUpdateInterval,
			WindowPastWeeks:          base.WindowPastWeeks,
			WindowFutureWeeks:        base.WindowFutureWeeks,
			CompactOverlappingEvents: base.CompactOverlappingEvents,
			DstPublicDomain:          base.DstPublicDomain,
		}

		dstAuth, err := parseBoolWithDefault(lookup, prefix+"DST_AUTH_ENABLED", false)
		if err != nil {
			return nil, fmt.Errorf("invalid %sDST_AUTH_ENABLED: %w", prefix, err)
		}
		cfg.DstAuthEnabled = dstAuth
		if v, ok := lookup(prefix + "DST_USERNAME"); ok {
			cfg.DstUsername = v
		}
		if v, ok := lookup(prefix + "DST_PASSWORD"); ok {
			cfg.DstPassword = v
		}

		for j := 1; j <= 4; j++ {
			sp := fmt.Sprintf("%sSRC_%d_", prefix, j)
			rawURL, ok := lookup(sp + "URL")
			if !ok || strings.TrimSpace(rawURL) == "" {
				continue
			}
			anon, err := parseBoolWithDefault(lookup, sp+"ANON", false)
			if err != nil {
				return nil, fmt.Errorf("invalid %sANON: %w", sp, err)
			}
			src := &Src{URL: strings.TrimSpace(rawURL), Anon: anon}
			if v, ok := lookup(sp + "USERNAME"); ok {
				src.Username = v
			}
			if v, ok := lookup(sp + "PASSWORD"); ok {
				src.Password = v
			}
			setSrcSlot(cfg, j, src)
		}

		tenants = append(tenants, &Tenant{Name: name, WebUIEnabled: webUIEnabled, Config: cfg})
	}

	if len(tenants) == 0 {
		return []*Tenant{{Name: "", WebUIEnabled: true, Config: base}}, nil
	}

	seen := map[string]bool{}
	for _, t := range tenants {
		if seen[t.Name] {
			return nil, fmt.Errorf("duplicate tenant name %q", t.Name)
		}
		seen[t.Name] = true
		if len(t.Config.Srcs()) == 0 {
			logrus.Warnf("tenant %q has no sources configured, it will serve an empty calendar", t.Name)
		}
	}
	if len(base.Srcs()) > 0 {
		logrus.Warn("TENANT_* tenants defined: legacy SRC_*_URL sources are ignored, legacy routes alias the first tenant")
	}
	return tenants, nil
}

// IsMultiTenant reports whether the resolved tenants run in multi-tenant
// mode (as opposed to the single legacy tenant fallback).
func IsMultiTenant(tenants []*Tenant) bool {
	return !(len(tenants) == 1 && tenants[0].Name == "")
}

func setSrcSlot(cfg *Config, slot int, src *Src) {
	switch slot {
	case 1:
		cfg.Src1URL, cfg.Src1Anon, cfg.Src1Username, cfg.Src1Password = src.URL, src.Anon, src.Username, src.Password
	case 2:
		cfg.Src2URL, cfg.Src2Anon, cfg.Src2Username, cfg.Src2Password = src.URL, src.Anon, src.Username, src.Password
	case 3:
		cfg.Src3URL, cfg.Src3Anon, cfg.Src3Username, cfg.Src3Password = src.URL, src.Anon, src.Username, src.Password
	case 4:
		cfg.Src4URL, cfg.Src4Anon, cfg.Src4Username, cfg.Src4Password = src.URL, src.Anon, src.Username, src.Password
	}
}

func parseBoolWithDefault(lookup func(string) (string, bool), key string, def bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def, nil
	}
	return strconv.ParseBool(strings.TrimSpace(raw))
}
