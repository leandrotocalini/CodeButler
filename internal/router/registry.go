package router

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// defaultInactivityTimeout is how long a thread worker lives without messages.
	defaultInactivityTimeout = 60 * time.Second

	// defaultInboxSize is the buffered channel capacity per thread worker.
	defaultInboxSize = 10
)

// ThreadMessage represents a message dispatched to a thread worker.
type ThreadMessage struct {
	EventID   string
	ChannelID string
	ThreadTS  string
	MessageTS string
	UserID    string
	Text      string
}

// ThreadHandler is the callback invoked by a thread worker for each message.
type ThreadHandler func(msg ThreadMessage)

// ThreadRegistry manages goroutine-per-thread workers.
// Each active thread gets its own goroutine that processes messages sequentially.
// Workers die after an inactivity timeout and respawn on the next message.
type ThreadRegistry struct {
	mu      sync.Mutex
	workers map[string]*threadWorker
	handler ThreadHandler
	logger  *slog.Logger

	inactivityTimeout time.Duration
	inboxSize         int
}

// RegistryOption configures the thread registry.
type RegistryOption func(*ThreadRegistry)

// WithInactivityTimeout sets the worker inactivity timeout.
func WithInactivityTimeout(d time.Duration) RegistryOption {
	return func(r *ThreadRegistry) {
		r.inactivityTimeout = d
	}
}

// WithInboxSize sets the buffered channel capacity per worker.
func WithInboxSize(n int) RegistryOption {
	return func(r *ThreadRegistry) {
		r.inboxSize = n
	}
}

// WithRegistryLogger sets the logger for the registry.
func WithRegistryLogger(l *slog.Logger) RegistryOption {
	return func(r *ThreadRegistry) {
		r.logger = l
	}
}

// NewThreadRegistry creates a new thread registry.
func NewThreadRegistry(handler ThreadHandler, opts ...RegistryOption) *ThreadRegistry {
	r := &ThreadRegistry{
		workers:           make(map[string]*threadWorker),
		handler:           handler,
		logger:            slog.Default(),
		inactivityTimeout: defaultInactivityTimeout,
		inboxSize:         defaultInboxSize,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Dispatch sends a message to the appropriate thread worker.
// Creates the worker if it doesn't exist or has died.
func (r *ThreadRegistry) Dispatch(msg ThreadMessage) {
	r.mu.Lock()

	w, ok := r.workers[msg.ThreadTS]
	if !ok || !w.alive() {
		w = r.spawnWorker(msg.ThreadTS)
		r.workers[msg.ThreadTS] = w
	}

	r.mu.Unlock()

	// Non-blocking send (drop if inbox full — shouldn't happen with reasonable sizes)
	select {
	case w.inbox <- msg:
	default:
		r.logger.Warn("thread worker inbox full, dropping message",
			"thread", msg.ThreadTS,
			"event_id", msg.EventID,
		)
	}
}

// ActiveThreads returns the number of currently active thread workers.
func (r *ThreadRegistry) ActiveThreads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, w := range r.workers {
		if w.alive() {
			count++
		}
	}
	return count
}

// spawnWorker creates and starts a new thread worker goroutine.
func (r *ThreadRegistry) spawnWorker(threadTS string) *threadWorker {
	w := &threadWorker{
		threadTS: threadTS,
		inbox:    make(chan ThreadMessage, r.inboxSize),
		done:     make(chan struct{}),
		handler:  r.handler,
		timeout:  r.inactivityTimeout,
		logger:   r.logger,
	}
	go w.run()
	r.logger.Info("thread worker spawned", "thread", threadTS)
	return w
}

// threadWorker is a goroutine that processes messages for a single thread.
type threadWorker struct {
	threadTS string
	inbox    chan ThreadMessage
	done     chan struct{}
	handler  ThreadHandler
	timeout  time.Duration
	logger   *slog.Logger
}

// alive returns true if the worker goroutine is still running.
func (w *threadWorker) alive() bool {
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// run is the worker goroutine's main loop. It processes messages from the
// inbox and exits after the inactivity timeout.
func (w *threadWorker) run() {
	defer close(w.done)
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("thread worker panic recovered",
				"thread", w.threadTS,
				"panic", r,
			)
		}
	}()

	timer := time.NewTimer(w.timeout)
	defer timer.Stop()

	for {
		select {
		case msg := <-w.inbox:
			// Reset inactivity timer
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.timeout)

			// Process the message with panic recovery
			w.processMessage(msg)

		case <-timer.C:
			w.logger.Info("thread worker exiting due to inactivity",
				"thread", w.threadTS,
			)
			return
		}
	}
}

// processMessage handles a single message with panic recovery.
func (w *threadWorker) processMessage(msg ThreadMessage) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("panic in message handler",
				"thread", w.threadTS,
				"event_id", msg.EventID,
				"panic", r,
			)
		}
	}()

	w.handler(msg)
}
