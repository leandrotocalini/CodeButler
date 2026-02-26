package decisions

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedClock() time.Time {
	return time.Date(2026, 2, 25, 14, 30, 12, 0, time.UTC)
}

func TestLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "pm")
	logger.now = fixedClock

	err := logger.Log(Decision{
		Type:         WorkflowSelected,
		Input:        "user said: add dark mode toggle",
		Decision:     "implement",
		Alternatives: []string{"bugfix", "refactor"},
		Evidence:     "user said 'add', no existing dark mode code found",
	})

	if err != nil {
		t.Fatalf("log failed: %v", err)
	}

	line := buf.String()
	if !strings.Contains(line, `"type":"workflow_selected"`) {
		t.Error("missing type")
	}
	if !strings.Contains(line, `"agent":"pm"`) {
		t.Error("missing agent")
	}
	if !strings.Contains(line, `"decision":"implement"`) {
		t.Error("missing decision")
	}
	if !strings.Contains(line, `"ts":"2026-02-25T14:30:12Z"`) {
		t.Error("missing timestamp")
	}
	if !strings.HasSuffix(line, "\n") {
		t.Error("line should end with newline")
	}
}

func TestLogger_LogDecision(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "coder")
	logger.now = fixedClock

	err := logger.LogDecision(
		ModelSelected,
		"fix typo in README",
		"sonnet",
		"PM classified as simple task",
		"opus", "sonnet",
	)

	if err != nil {
		t.Fatalf("log decision failed: %v", err)
	}

	line := buf.String()
	if !strings.Contains(line, `"type":"model_selected"`) {
		t.Error("missing type")
	}
	if !strings.Contains(line, `"agent":"coder"`) {
		t.Error("missing agent")
	}
}

func TestLogger_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "pm")
	logger.now = fixedClock

	logger.Log(Decision{Type: WorkflowSelected, Input: "a", Decision: "implement", Evidence: "e1"})
	logger.Log(Decision{Type: AgentDelegated, Input: "b", Decision: "coder", Evidence: "e2"})
	logger.Log(Decision{Type: SkillMatched, Input: "c", Decision: "explain", Evidence: "e3"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "pm")
	logger.now = fixedClock

	var wg sync.WaitGroup
	n := 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			logger.Log(Decision{
				Type:     WorkflowSelected,
				Input:    "concurrent test",
				Decision: "implement",
				Evidence: "test",
			})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d", n, len(lines))
	}
}

func TestReadFrom(t *testing.T) {
	data := `{"ts":"2026-02-25T14:30:12Z","agent":"pm","type":"workflow_selected","input":"add dark mode","decision":"implement","evidence":"user said add"}
{"ts":"2026-02-25T14:30:13Z","agent":"coder","type":"model_selected","input":"fix typo","decision":"sonnet","evidence":"simple task"}
`

	decisions, err := ReadFrom(strings.NewReader(data))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].Type != WorkflowSelected {
		t.Errorf("decision 0 type: got %q", decisions[0].Type)
	}
	if decisions[1].Agent != "coder" {
		t.Errorf("decision 1 agent: got %q", decisions[1].Agent)
	}
}

func TestReadFrom_SkipsMalformed(t *testing.T) {
	data := `{"ts":"2026-02-25T14:30:12Z","agent":"pm","type":"workflow_selected","input":"a","decision":"b","evidence":"c"}
not json at all
{"ts":"2026-02-25T14:30:13Z","agent":"pm","type":"skill_matched","input":"x","decision":"y","evidence":"z"}
`

	decisions, err := ReadFrom(strings.NewReader(data))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(decisions) != 2 {
		t.Errorf("expected 2 decisions (skip malformed), got %d", len(decisions))
	}
}

func TestReadFrom_EmptyLog(t *testing.T) {
	decisions, err := ReadFrom(strings.NewReader(""))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(decisions))
	}
}

func TestFilterByType(t *testing.T) {
	decisions := []Decision{
		{Type: WorkflowSelected, Agent: "pm"},
		{Type: ModelSelected, Agent: "coder"},
		{Type: WorkflowSelected, Agent: "pm"},
		{Type: StuckDetected, Agent: "coder"},
	}

	filtered := FilterByType(decisions, WorkflowSelected)
	if len(filtered) != 2 {
		t.Errorf("expected 2 workflow_selected, got %d", len(filtered))
	}
}

func TestFilterByAgent(t *testing.T) {
	decisions := []Decision{
		{Type: WorkflowSelected, Agent: "pm"},
		{Type: ModelSelected, Agent: "coder"},
		{Type: AgentDelegated, Agent: "pm"},
	}

	filtered := FilterByAgent(decisions, "pm")
	if len(filtered) != 2 {
		t.Errorf("expected 2 pm decisions, got %d", len(filtered))
	}
}

func TestSummary(t *testing.T) {
	decisions := []Decision{
		{Type: WorkflowSelected},
		{Type: ModelSelected},
		{Type: WorkflowSelected},
		{Type: StuckDetected},
		{Type: ModelSelected},
		{Type: ModelSelected},
	}

	summary := Summary(decisions)
	if summary[WorkflowSelected] != 2 {
		t.Errorf("workflow_selected: expected 2, got %d", summary[WorkflowSelected])
	}
	if summary[ModelSelected] != 3 {
		t.Errorf("model_selected: expected 3, got %d", summary[ModelSelected])
	}
	if summary[StuckDetected] != 1 {
		t.Errorf("stuck_detected: expected 1, got %d", summary[StuckDetected])
	}
}

func TestDecisionType_IsValid(t *testing.T) {
	if !WorkflowSelected.IsValid() {
		t.Error("workflow_selected should be valid")
	}
	if !CircuitBreaker.IsValid() {
		t.Error("circuit_breaker should be valid")
	}
	if DecisionType("invalid").IsValid() {
		t.Error("invalid type should not be valid")
	}
}

func TestWithOutcome(t *testing.T) {
	d := Decision{
		Type:     WorkflowSelected,
		Decision: "implement",
	}

	updated := d.WithOutcome("success")
	if updated.Outcome == nil {
		t.Fatal("outcome should be set")
	}
	if *updated.Outcome != "success" {
		t.Errorf("outcome: got %q", *updated.Outcome)
	}
	// Original should be unchanged
	if d.Outcome != nil {
		t.Error("original decision should not be modified")
	}
}

func TestNewFileLogger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "branch", "decisions.jsonl")

	logger, err := NewFileLogger(path, "pm")
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	logger.now = fixedClock

	logger.Log(Decision{
		Type:     WorkflowSelected,
		Input:    "test",
		Decision: "implement",
		Evidence: "testing",
	})
	logger.Log(Decision{
		Type:     AgentDelegated,
		Input:    "task",
		Decision: "coder",
		Evidence: "needs code",
	})

	// Read it back
	decisions, err := ReadLog(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(decisions))
	}
}

func TestReadLog_FileNotFound(t *testing.T) {
	decisions, err := ReadLog("/nonexistent/decisions.jsonl")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(decisions))
	}
}

func TestAllDecisionTypes(t *testing.T) {
	types := AllDecisionTypes()
	if len(types) != 12 {
		t.Errorf("expected 12 decision types, got %d", len(types))
	}
}

func TestNewFileLogger_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "dir", "decisions.jsonl")

	logger, err := NewFileLogger(path, "pm")
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	logger.now = fixedClock
	logger.Log(Decision{Type: WorkflowSelected, Input: "t", Decision: "d", Evidence: "e"})

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file should exist")
	}
}
