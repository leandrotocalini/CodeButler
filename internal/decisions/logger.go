package decisions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger writes decisions to an append-only JSONL file.
// Thread-safe: multiple goroutines can log concurrently.
type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	agent string
	now   func() time.Time // injectable clock for testing
}

// NewLogger creates a decision logger for the given agent.
// Writes are appended to the provided writer.
func NewLogger(w io.Writer, agent string) *Logger {
	return &Logger{
		w:     w,
		agent: agent,
		now:   time.Now,
	}
}

// NewFileLogger creates a decision logger that appends to a JSONL file.
// Creates the file and parent directories if they don't exist.
func NewFileLogger(path, agent string) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create decision log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open decision log: %w", err)
	}

	return NewLogger(f, agent), nil
}

// Log writes a decision entry to the log.
func (l *Logger) Log(d Decision) error {
	d.Timestamp = l.now()
	d.Agent = l.agent

	data, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal decision: %w", err)
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	_, err = l.w.Write(data)
	if err != nil {
		return fmt.Errorf("write decision: %w", err)
	}

	return nil
}

// LogDecision is a convenience method that builds and logs a decision.
func (l *Logger) LogDecision(typ DecisionType, input, decision, evidence string, alternatives ...string) error {
	return l.Log(Decision{
		Type:         typ,
		Input:        input,
		Decision:     decision,
		Evidence:     evidence,
		Alternatives: alternatives,
	})
}

// ReadLog reads all decisions from a JSONL file.
func ReadLog(path string) ([]Decision, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no log yet
		}
		return nil, fmt.Errorf("open decision log: %w", err)
	}
	defer f.Close()

	return ReadFrom(f)
}

// ReadFrom reads decisions from a reader containing JSONL data.
func ReadFrom(r io.Reader) ([]Decision, error) {
	var decisions []Decision
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var d Decision
		if err := json.Unmarshal(line, &d); err != nil {
			continue // skip malformed lines
		}
		decisions = append(decisions, d)
	}

	if err := scanner.Err(); err != nil {
		return decisions, fmt.Errorf("read decision log: %w", err)
	}

	return decisions, nil
}

// FilterByType returns only decisions of the given type.
func FilterByType(decisions []Decision, typ DecisionType) []Decision {
	var filtered []Decision
	for _, d := range decisions {
		if d.Type == typ {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// FilterByAgent returns only decisions from the given agent.
func FilterByAgent(decisions []Decision, agent string) []Decision {
	var filtered []Decision
	for _, d := range decisions {
		if d.Agent == agent {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// Summary returns a count of decisions by type.
func Summary(decisions []Decision) map[DecisionType]int {
	counts := make(map[DecisionType]int)
	for _, d := range decisions {
		counts[d.Type]++
	}
	return counts
}
