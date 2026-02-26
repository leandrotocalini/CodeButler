package slack

import (
	"sync"
	"testing"
	"time"
)

func TestDedupSet_NewEventIsAccepted(t *testing.T) {
	d := NewDedupSet()

	if !d.Check("event-1") {
		t.Error("expected new event to be accepted")
	}
}

func TestDedupSet_DuplicateIsRejected(t *testing.T) {
	d := NewDedupSet()

	d.Check("event-1")
	if d.Check("event-1") {
		t.Error("expected duplicate event to be rejected")
	}
}

func TestDedupSet_DifferentEventsAccepted(t *testing.T) {
	d := NewDedupSet()

	if !d.Check("event-1") {
		t.Error("expected event-1 to be accepted")
	}
	if !d.Check("event-2") {
		t.Error("expected event-2 to be accepted")
	}
	if !d.Check("event-3") {
		t.Error("expected event-3 to be accepted")
	}
}

func TestDedupSet_ExpiredEntryReaccepted(t *testing.T) {
	now := time.Now()
	clock := &mockClock{t: now}

	d := NewDedupSet(
		WithDedupTTL(1*time.Minute),
		WithClock(clock.Now),
	)

	// Add event
	d.Check("event-1")

	// Advance past TTL
	clock.Advance(2 * time.Minute)

	// Should be accepted again (expired)
	if !d.Check("event-1") {
		t.Error("expected expired event to be re-accepted")
	}
}

func TestDedupSet_MaxEntriesEviction(t *testing.T) {
	now := time.Now()
	clock := &mockClock{t: now}

	d := NewDedupSet(
		WithMaxEntries(3),
		WithDedupTTL(10*time.Minute),
		WithClock(clock.Now),
	)

	// Fill to capacity
	d.Check("a")
	clock.Advance(1 * time.Second)
	d.Check("b")
	clock.Advance(1 * time.Second)
	d.Check("c")

	// At capacity (3). Next check should trigger eviction.
	// Since all are within TTL, eviction won't free anything,
	// but the new entry will still be added.
	clock.Advance(1 * time.Second)
	d.Check("d")

	if d.Len() > 4 {
		t.Errorf("expected at most 4 entries, got %d", d.Len())
	}
}

func TestDedupSet_EvictExpired(t *testing.T) {
	now := time.Now()
	clock := &mockClock{t: now}

	d := NewDedupSet(
		WithDedupTTL(1*time.Minute),
		WithClock(clock.Now),
	)

	d.Check("a")
	d.Check("b")
	d.Check("c")

	if d.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", d.Len())
	}

	// Advance past TTL
	clock.Advance(2 * time.Minute)
	d.EvictExpired()

	if d.Len() != 0 {
		t.Errorf("expected 0 entries after eviction, got %d", d.Len())
	}
}

func TestDedupSet_ConcurrentAccess(t *testing.T) {
	d := NewDedupSet()

	var wg sync.WaitGroup
	accepted := make([]bool, 100)

	// 100 goroutines all trying to claim the same event
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			accepted[idx] = d.Check("same-event")
		}(i)
	}

	wg.Wait()

	// Exactly one should have been accepted
	count := 0
	for _, a := range accepted {
		if a {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 accepted, got %d", count)
	}
}

func TestDedupSet_DefaultConfig(t *testing.T) {
	d := NewDedupSet()

	if d.maxEntries != 10000 {
		t.Errorf("expected maxEntries 10000, got %d", d.maxEntries)
	}
	if d.ttl != 5*time.Minute {
		t.Errorf("expected TTL 5m, got %v", d.ttl)
	}
}

// mockClock provides a controllable time source for testing.
type mockClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
