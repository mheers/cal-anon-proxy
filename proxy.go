package main

import (
	"sync"

	"github.com/emersion/go-webdav/caldav"
)

type CalProxy struct {
	config *Config

	// tenant is the owning tenant's name ("" in single-tenant mode), used
	// only to scope log lines. Set by buildTenantRuntime.
	tenant string

	// lastKnownGood caches the most recent successful download per source
	// URL. When a source fails transiently, its cached events keep being
	// served instead of freezing ALL sources at their previous state (the
	// old all-or-nothing behavior kept deleted events alive forever as long
	// as any other source errored).
	mu            sync.Mutex
	lastKnownGood map[string][]*caldav.CalendarObject
}

func NewCalProxy(config *Config) *CalProxy {
	return &CalProxy{
		config:        config,
		lastKnownGood: make(map[string][]*caldav.CalendarObject),
	}
}

func (p *CalProxy) cachedEvents(srcURL string) []*caldav.CalendarObject {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastKnownGood[srcURL]
}

func (p *CalProxy) setCachedEvents(srcURL string, events []*caldav.CalendarObject) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastKnownGood[srcURL] = events
}
