package scan

import (
	"log/slog"
	"sync"
)

// DomainDeduper tracks certificate domains already written to output.
type DomainDeduper struct {
	mu      sync.Mutex
	seen    map[string]struct{}
	max     int
	warned  bool
	log     *slog.Logger
}

// NewDomainDeduper creates a deduper. max <= 0 means unlimited.
func NewDomainDeduper(max int, log *slog.Logger) *DomainDeduper {
	return &DomainDeduper{
		seen: make(map[string]struct{}),
		max:  max,
		log:  loggerOrDiscard(log),
	}
}

// Add records domain and returns true if it was not seen before.
// When the table is at capacity, new domains are not stored (they may be
// reported more than once) so memory stays bounded.
func (o *DomainDeduper) Add(domain string) bool {
	domain, ok := normalizeCertificateDomain(domain)
	if !ok {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.seen[domain]; exists {
		return false
	}
	if o.max > 0 && len(o.seen) >= o.max {
		if !o.warned {
			o.warned = true
			o.log.Warn("domain dedupe table reached capacity, later domains may be reported more than once", "max", o.max)
		}
		return true
	}
	o.seen[domain] = struct{}{}
	return true
}

// Len returns the number of stored domains.
func (o *DomainDeduper) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.seen)
}
