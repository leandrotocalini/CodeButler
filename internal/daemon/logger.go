package daemon

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
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

	// TUI input support
	outMu     sync.Mutex // protects cursor management during output
	inputMode bool       // true when terminal supports input row
	termRows  int        // terminal height in rows
	inputBuf  string     // current text being typed (protected by outMu)
}

func NewLogger(maxSize int) *Logger {
	stderrTTY := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

	return &Logger{
		entries:   make([]LogEntry, 0, maxSize),
		maxSize:   maxSize,
		subs:      make(map[chan LogEntry]struct{}),
		color:     stderrTTY,
		inputMode: stderrTTY && stdinTTY,
	}
}

// InputMode returns true if the terminal supports the input row.
func (l *Logger) InputMode() bool {
	return l.inputMode
}

// printLines outputs lines inside the scroll region and redraws the input prompt.
// All TUI output should go through this method when inputMode is active.
func (l *Logger) printLines(lines ...string) {
	l.outMu.Lock()
	defer l.outMu.Unlock()

	if !l.inputMode || l.termRows == 0 {
		// No input row — write directly
		for _, line := range lines {
			fmt.Fprintf(os.Stderr, "%s\n", line)
		}
		return
	}

	scrollBottom := l.termRows - 1

	// Save cursor, move to bottom of scroll region, print lines (scrolling region up),
	// then redraw the input prompt on the last row.
	for _, line := range lines {
		// Move to scroll bottom, newline to scroll, clear line, print
		fmt.Fprintf(os.Stderr, "\033[%d;1H\n\033[2K%s", scrollBottom, line)
	}

	// Redraw input prompt on last row
	l.drawPromptLocked()
}

// DrawPrompt redraws the "> " prompt on the input row.
func (l *Logger) DrawPrompt() {
	l.outMu.Lock()
	defer l.outMu.Unlock()
	l.drawPromptLocked()
}

// drawPromptLocked redraws the prompt with current input text; caller must hold outMu.
func (l *Logger) drawPromptLocked() {
	if !l.inputMode || l.termRows == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\033[%d;1H\033[2K%s> %s%s", l.termRows, ansiDim, ansiReset, l.inputBuf)
}

// UpdateInput sets the current input text and redraws the prompt.
func (l *Logger) UpdateInput(text string) {
	l.outMu.Lock()
	defer l.outMu.Unlock()
	l.inputBuf = text
	l.drawPromptLocked()
}

// ClearInput clears the input buffer and redraws the prompt.
func (l *Logger) ClearInput() {
	l.outMu.Lock()
	defer l.outMu.Unlock()
	l.inputBuf = ""
	l.drawPromptLocked()
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
		ts := ansiGray + entry.Time.Format("2006-01-02 15:04:05") + ansiReset
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
		l.printLines(fmt.Sprintf("%s %s", ts, msg))
	} else {
		fmt.Fprintf(os.Stderr, "%s %s %s\n", entry.Time.Format("2006-01-02 15:04:05"), level.String(), entry.Message)
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

func (l *Logger) Debug(format string, args ...interface{}) { l.log(LevelInfo, format, args...) }
func (l *Logger) Info(format string, args ...interface{})  { l.log(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.log(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.log(LevelError, format, args...) }

// --- TUI methods (stderr only, not ring buffer/web dashboard) ---

// Clear clears the terminal screen and resets any scroll region.
func (l *Logger) Clear() {
	if l.color {
		fmt.Fprint(os.Stderr, "\033[r\033[2J\033[H")
	}
}

// Header prints a bold cyan header line with a separator, then sets up
// an ANSI scroll region so the header stays fixed at the top.
// If inputMode is active, the bottom row is reserved for the input prompt.
func (l *Logger) Header(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	sep := strings.Repeat("\u2500", max(len(text), 40))
	if l.color {
		// Row 1: blank, Row 2: header, Row 3: separator, Row 4: blank -> cursor at row 5
		fmt.Fprintf(os.Stderr, "\n%s%s%s\n%s%s%s\n\n", ansiBold+ansiCyan, text, ansiReset, ansiDim, sep, ansiReset)

		if l.inputMode {
			_, rows, err := term.GetSize(int(os.Stderr.Fd()))
			if err == nil && rows > 6 {
				l.termRows = rows
				scrollBottom := rows - 1
				// Scroll region: row 5 to row N-1 (reserve row N for input)
				fmt.Fprintf(os.Stderr, "\033[5;%dr\033[5;1H", scrollBottom)
				l.drawPromptLocked()
			} else {
				// Fallback: no input row
				l.inputMode = false
				fmt.Fprint(os.Stderr, "\033[5;9999r\033[5;1H")
			}
		} else {
			fmt.Fprint(os.Stderr, "\033[5;9999r\033[5;1H")
		}
	} else {
		fmt.Fprintf(os.Stderr, "\n%s\n%s\n\n", text, sep)
	}
}

// Cleanup resets the scroll region and moves the cursor to the bottom.
// Call on exit so the terminal returns to normal.
func (l *Logger) Cleanup() {
	if l.color {
		l.outMu.Lock()
		if l.inputMode && l.termRows > 0 {
			// Move past the input row before resetting
			fmt.Fprintf(os.Stderr, "\033[r\033[%d;1H\n", l.termRows)
		} else {
			fmt.Fprint(os.Stderr, "\033[r\033[9999;1H\n")
		}
		l.outMu.Unlock()
	}
}

// UserMsg prints an incoming user message notification and the processed prompt.
func (l *Logger) UserMsg(label, content string, t time.Time, isVoice, isImage bool) {
	ts := t.Format("2006-01-02 15:04:05")

	// Determine message type description
	msgType := "text"
	switch {
	case isImage && strings.Contains(content, "</attached-image>"):
		msgType = "image + caption"
	case isImage:
		msgType = "image"
	case isVoice:
		msgType = "voice"
	}

	// Truncate prompt preview
	preview := content
	if len(preview) > 150 {
		preview = preview[:150] + "\u2026"
	}

	if l.color {
		l.printLines(
			fmt.Sprintf("%s%s\u25b6 %s%s %s(%s \u00b7 %s)%s",
				ansiBold, ansiGreen, label, ansiReset, ansiGray, ts, msgType, ansiReset),
			fmt.Sprintf("%s  \u2192 %s%s", ansiDim, preview, ansiReset),
		)
	} else {
		fmt.Fprintf(os.Stderr, "> %s (%s · %s)\n", label, ts, msgType)
		fmt.Fprintf(os.Stderr, "  -> %s\n", preview)
	}
}

// BotStart prints Claude starting indicator.
func (l *Logger) BotStart(detail string) {
	if l.color {
		l.printLines(fmt.Sprintf("%s\u25cf Claude \u00b7 %s%s", ansiDim, detail, ansiReset))
	} else {
		fmt.Fprintf(os.Stderr, "* Claude - %s\n", detail)
	}
}

// BotResult prints Claude result stats.
func (l *Logger) BotResult(elapsed time.Duration, turns int, cost float64) {
	if l.color {
		l.printLines(fmt.Sprintf("%s%s\u25cf Claude \u00b7 %s \u00b7 %d turns \u00b7 $%.2f%s",
			ansiBold, ansiCyan, elapsed.Round(time.Second), turns, cost, ansiReset))
	} else {
		fmt.Fprintf(os.Stderr, "* Claude - %s - %d turns - $%.2f\n",
			elapsed.Round(time.Second), turns, cost)
	}
}

// BotText prints indented bot response text.
func (l *Logger) BotText(text string) {
	lines := strings.Split(text, "\n")
	formatted := make([]string, len(lines))
	for i, line := range lines {
		formatted[i] = "  " + line
	}
	if l.color && l.inputMode {
		l.printLines(formatted...)
	} else {
		for _, line := range formatted {
			fmt.Fprintf(os.Stderr, "%s\n", line)
		}
	}
}

// Status prints a dim gray status line.
func (l *Logger) Status(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	if l.color {
		l.printLines(fmt.Sprintf("%s  %s%s", ansiDim, text, ansiReset))
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
