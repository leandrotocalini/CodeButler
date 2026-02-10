package daemon

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "?"
	}
}

func (l LogLevel) Icon() string {
	switch l {
	case LevelDebug:
		return "🔍"
	case LevelInfo:
		return "📋"
	case LevelWarn:
		return "⚠️"
	case LevelError:
		return "❌"
	default:
		return "?"
	}
}

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   LogLevel  `json:"level"`
	Message string    `json:"message"`
}

type Logger struct {
	mu      sync.Mutex
	entries []LogEntry
	maxSize int

	// Subscribers for real-time log streaming
	subMu sync.Mutex
	subs  map[chan LogEntry]struct{}
}

func NewLogger(maxSize int) *Logger {
	return &Logger{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
		subs:    make(map[chan LogEntry]struct{}),
	}
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	}

	// Store in ring buffer
	l.mu.Lock()
	if len(l.entries) >= l.maxSize {
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry)
	l.mu.Unlock()

	// Print to stderr
	fmt.Fprintf(os.Stderr, "%s %s %s\n", entry.Time.Format("15:04:05"), level.Icon(), entry.Message)

	// Notify subscribers (non-blocking)
	l.subMu.Lock()
	for ch := range l.subs {
		select {
		case ch <- entry:
		default:
		}
	}
	l.subMu.Unlock()
}

func (l *Logger) Debug(format string, args ...interface{}) { l.log(LevelDebug, format, args...) }
func (l *Logger) Info(format string, args ...interface{})  { l.log(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.log(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.log(LevelError, format, args...) }

// Entries returns a copy of all stored log entries.
func (l *Logger) Entries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]LogEntry, len(l.entries))
	copy(cp, l.entries)
	return cp
}

// Subscribe returns a channel that receives new log entries in real time.
func (l *Logger) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 64)
	l.subMu.Lock()
	l.subs[ch] = struct{}{}
	l.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (l *Logger) Unsubscribe(ch chan LogEntry) {
	l.subMu.Lock()
	delete(l.subs, ch)
	l.subMu.Unlock()
	close(ch)
}
