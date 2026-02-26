package router

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThreadRegistry_SpawnOnFirstMessage(t *testing.T) {
	var called atomic.Int32
	registry := NewThreadRegistry(func(msg ThreadMessage) {
		called.Add(1)
	}, WithInactivityTimeout(1*time.Second))

	registry.Dispatch(ThreadMessage{
		ThreadTS: "thread-1",
		Text:     "hello",
	})

	// Give the goroutine time to process
	time.Sleep(50 * time.Millisecond)

	if called.Load() != 1 {
		t.Errorf("expected handler called once, got %d", called.Load())
	}
	if registry.ActiveThreads() != 1 {
		t.Errorf("expected 1 active thread, got %d", registry.ActiveThreads())
	}
}

func TestThreadRegistry_SameThreadSameWorker(t *testing.T) {
	var called atomic.Int32
	registry := NewThreadRegistry(func(msg ThreadMessage) {
		called.Add(1)
	}, WithInactivityTimeout(1*time.Second))

	for i := 0; i < 3; i++ {
		registry.Dispatch(ThreadMessage{
			ThreadTS: "thread-1",
			Text:     "message",
		})
	}

	time.Sleep(50 * time.Millisecond)

	if called.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", called.Load())
	}
	if registry.ActiveThreads() != 1 {
		t.Errorf("expected 1 active thread, got %d", registry.ActiveThreads())
	}
}

func TestThreadRegistry_DifferentThreadsDifferentWorkers(t *testing.T) {
	var mu sync.Mutex
	threads := make(map[string]int)

	registry := NewThreadRegistry(func(msg ThreadMessage) {
		mu.Lock()
		threads[msg.ThreadTS]++
		mu.Unlock()
	}, WithInactivityTimeout(1*time.Second))

	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "a"})
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-2", Text: "b"})
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-3", Text: "c"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(threads) != 3 {
		t.Errorf("expected 3 threads, got %d", len(threads))
	}
	if registry.ActiveThreads() != 3 {
		t.Errorf("expected 3 active threads, got %d", registry.ActiveThreads())
	}
}

func TestThreadRegistry_WorkerDiesAfterInactivity(t *testing.T) {
	registry := NewThreadRegistry(func(msg ThreadMessage) {
		// no-op
	}, WithInactivityTimeout(100*time.Millisecond))

	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "hello"})

	time.Sleep(50 * time.Millisecond)
	if registry.ActiveThreads() != 1 {
		t.Errorf("expected 1 active thread before timeout, got %d", registry.ActiveThreads())
	}

	// Wait for inactivity timeout
	time.Sleep(200 * time.Millisecond)

	if registry.ActiveThreads() != 0 {
		t.Errorf("expected 0 active threads after timeout, got %d", registry.ActiveThreads())
	}
}

func TestThreadRegistry_WorkerRespawnsOnNewMessage(t *testing.T) {
	var called atomic.Int32
	registry := NewThreadRegistry(func(msg ThreadMessage) {
		called.Add(1)
	}, WithInactivityTimeout(100*time.Millisecond))

	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "first"})
	time.Sleep(50 * time.Millisecond)

	// Wait for worker to die
	time.Sleep(200 * time.Millisecond)
	if registry.ActiveThreads() != 0 {
		t.Error("expected worker to have died")
	}

	// Send another message — should respawn
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "second"})
	time.Sleep(50 * time.Millisecond)

	if called.Load() != 2 {
		t.Errorf("expected 2 handler calls, got %d", called.Load())
	}
	if registry.ActiveThreads() != 1 {
		t.Errorf("expected 1 active thread after respawn, got %d", registry.ActiveThreads())
	}
}

func TestThreadRegistry_PanicRecovery(t *testing.T) {
	var called atomic.Int32
	registry := NewThreadRegistry(func(msg ThreadMessage) {
		called.Add(1)
		if msg.Text == "panic" {
			panic("test panic")
		}
	}, WithInactivityTimeout(1*time.Second))

	// First message panics
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "panic"})
	time.Sleep(50 * time.Millisecond)

	// Second message should still be processed (worker recovers from panic)
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "normal"})
	time.Sleep(50 * time.Millisecond)

	if called.Load() < 2 {
		t.Errorf("expected at least 2 handler calls (panic + normal), got %d", called.Load())
	}
}

func TestThreadRegistry_MessageOrdering(t *testing.T) {
	var mu sync.Mutex
	var order []string

	registry := NewThreadRegistry(func(msg ThreadMessage) {
		mu.Lock()
		order = append(order, msg.Text)
		mu.Unlock()
	}, WithInactivityTimeout(1*time.Second))

	// Send messages in order
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "first"})
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "second"})
	registry.Dispatch(ThreadMessage{ThreadTS: "thread-1", Text: "third"})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Errorf("expected [first, second, third], got %v", order)
	}
}
