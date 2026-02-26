package slack

import (
	"sync"
	"time"
)

const (
	// defaultMaxEntries is the maximum number of event IDs to track.
	defaultMaxEntries = 10000
	// defaultTTL is how long an event ID is remembered.
	defaultTTL = 5 * time.Minute
	// evictionInterval is how often expired entries are cleaned up.
	evictionInterval = 1 * time.Minute
)

// DedupSet is a bounded, TTL-based set for Slack event deduplication.
// Thread-safe via mutex. Each agent process maintains one instance.
type DedupSet struct {
	mu         sync.Mutex
	entries    map[string]time.Time
	maxEntries int
	ttl        time.Duration
	now        func() time.Time // injectable for testing
}

// DedupOption configures the DedupSet.
type DedupOption func(*DedupSet)

// WithMaxEntries sets the maximum number of tracked event IDs.
func WithMaxEntries(n int) DedupOption {
	return func(d *DedupSet) {
		d.maxEntries = n
	}
}

// WithTTL sets the time-to-live for event IDs.
func WithDedupTTL(ttl time.Duration) DedupOption {
	return func(d *DedupSet) {
		d.ttl = ttl
	}
}

// WithClock sets a custom time function (for testing).
func WithClock(fn func() time.Time) DedupOption {
	return func(d *DedupSet) {
		d.now = fn
	}
}

// NewDedupSet creates a new deduplication set with the given options.
func NewDedupSet(opts ...DedupOption) *DedupSet {
	d := &DedupSet{
		entries:    make(map[string]time.Time),
		maxEntries: defaultMaxEntries,
		ttl:        defaultTTL,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Check returns true if the event ID is new (not seen before).
// If new, it adds the ID to the set. If already seen, returns false.
// This is the primary API: check-and-add in one atomic operation.
func (d *DedupSet) Check(eventID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()

	// Check if already present and not expired
	if t, ok := d.entries[eventID]; ok {
		if now.Sub(t) < d.ttl {
			return false // duplicate
		}
		// Expired entry — treat as new
	}

	// Evict if at capacity
	if len(d.entries) >= d.maxEntries {
		d.evictLocked(now)
	}

	d.entries[eventID] = now
	return true // new event
}

// Len returns the number of tracked event IDs.
func (d *DedupSet) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.entries)
}

// evictLocked removes expired entries. Must be called with mutex held.
func (d *DedupSet) evictLocked(now time.Time) {
	for id, t := range d.entries {
		if now.Sub(t) >= d.ttl {
			delete(d.entries, id)
		}
	}
}

// EvictExpired removes expired entries. Safe to call periodically from a goroutine.
func (d *DedupSet) EvictExpired() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.evictLocked(d.now())
}
