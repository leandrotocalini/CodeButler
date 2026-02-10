package daemon

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
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

// ANSI escape codes
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   LogLevel  `json:"level"`
	Message string    `json:"message"`
}

type Logger struct {
	mu      sync.Mutex
	entries []LogEntry
	maxSize int
	color   bool

	// Subscribers for real-time log streaming
	subMu sync.Mutex
	subs  map[chan LogEntry]struct{}
}

func NewLogger(maxSize int) *Logger {
	return &Logger{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
		subs:    make(map[chan LogEntry]struct{}),
		color:   isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
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
	if l.color {
		ts := ansiGray + entry.Time.Format("15:04:05") + ansiReset
		var msg string
		switch level {
		case LevelDebug:
			msg = ansiDim + entry.Message + ansiReset
		case LevelInfo:
			msg = entry.Message
		case LevelWarn:
			msg = ansiYellow + entry.Message + ansiReset
		case LevelError:
			msg = ansiBold + ansiRed + entry.Message + ansiReset
		default:
			msg = entry.Message
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", ts, msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s %s\n", entry.Time.Format("15:04:05"), level.String(), entry.Message)
	}

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

// --- TUI methods (stderr only, not ring buffer/web dashboard) ---

// Header prints a bold cyan header line.
func (l *Logger) Header(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	if l.color {
		fmt.Fprintf(os.Stderr, "\n%s%s%s\n\n", ansiBold+ansiCyan, text, ansiReset)
	} else {
		fmt.Fprintf(os.Stderr, "\n%s\n\n", text)
	}
}

// UserMsg prints an incoming user message with green arrow prefix.
func (l *Logger) UserMsg(from, content string, t time.Time) {
	ts := t.Format("15:04:05")
	if l.color {
		fmt.Fprintf(os.Stderr, "%s%s\u25b6 %s%s (%s)\n", ansiBold, ansiGreen, from, ansiReset, ts)
	} else {
		fmt.Fprintf(os.Stderr, "> %s (%s)\n", from, ts)
	}
	for _, line := range strings.Split(content, "\n") {
		fmt.Fprintf(os.Stderr, "  %s\n", line)
	}
}

// BotStart prints Claude starting indicator.
func (l *Logger) BotStart(detail string) {
	if l.color {
		fmt.Fprintf(os.Stderr, "%s\u25cf Claude \u00b7 %s%s\n", ansiDim, detail, ansiReset)
	} else {
		fmt.Fprintf(os.Stderr, "* Claude - %s\n", detail)
	}
}

// BotResult prints Claude result stats.
func (l *Logger) BotResult(elapsed time.Duration, turns int, cost float64) {
	if l.color {
		fmt.Fprintf(os.Stderr, "%s%s\u25cf Claude \u00b7 %s \u00b7 %d turns \u00b7 $%.2f%s\n",
			ansiBold, ansiCyan, elapsed.Round(time.Second), turns, cost, ansiReset)
	} else {
		fmt.Fprintf(os.Stderr, "* Claude - %s - %d turns - $%.2f\n",
			elapsed.Round(time.Second), turns, cost)
	}
}

// BotText prints indented bot response text.
func (l *Logger) BotText(text string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(os.Stderr, "  %s\n", line)
	}
}

// Status prints a dim gray status line.
func (l *Logger) Status(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	if l.color {
		fmt.Fprintf(os.Stderr, "%s  %s%s\n", ansiDim, text, ansiReset)
	} else {
		fmt.Fprintf(os.Stderr, "  %s\n", text)
	}
}

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
