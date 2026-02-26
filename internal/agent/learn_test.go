package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockExplorer implements LearnExplorer for testing.
type mockExplorer struct {
	findings string
	err      error
	delay    time.Duration
	called   bool
	mu       sync.Mutex
}

func (m *mockExplorer) Explore(ctx context.Context, projectMap, channel, thread string) (string, error) {
	m.mu.Lock()
	m.called = true
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if m.err != nil {
		return "", m.err
	}
	return m.findings, nil
}

// mockSynthesizer implements LearnSynthesizer for testing.
type mockSynthesizer struct {
	result string
	err    error
}

func (m *mockSynthesizer) Synthesize(ctx context.Context, findings map[string]string, channel, thread string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.result != "" {
		return m.result, nil
	}
	// Auto-generate from findings
	var b strings.Builder
	b.WriteString("# Global Knowledge\n\n")
	for role, finding := range findings {
		fmt.Fprintf(&b, "## From %s\n%s\n\n", role, finding)
	}
	return b.String(), nil
}

func TestLearnWorkflow_FullRun(t *testing.T) {
	pm := &mockExplorer{findings: "Project has Go backend, React frontend"}
	coder := &mockExplorer{findings: "Architecture: clean architecture, uses sqlc"}
	reviewer := &mockExplorer{findings: "Test coverage: 78%, CI uses GitHub Actions"}
	artist := &mockExplorer{findings: "UI: Tailwind, component library exists"}
	synth := &mockSynthesizer{}

	explorers := map[string]LearnExplorer{
		"coder":    coder,
		"reviewer": reviewer,
		"artist":   artist,
	}

	lw := NewLearnWorkflow(pm, explorers, synth)

	cfg := LearnConfig{
		Channel: "C123",
		Thread:  "T456",
	}

	result, err := lw.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// Check PM was called
	if !pm.called {
		t.Error("PM should have been called")
	}

	// Check all explorers were called
	for role, exp := range explorers {
		me := exp.(*mockExplorer)
		if !me.called {
			t.Errorf("%s should have been called", role)
		}
	}

	// Check explorations count (pm + 3 technical)
	if len(result.Explorations) != 4 {
		t.Errorf("expected 4 explorations, got %d", len(result.Explorations))
	}

	// Check global.md was produced
	if result.GlobalMD == "" {
		t.Error("global.md should not be empty")
	}
	if !strings.Contains(result.GlobalMD, "Global Knowledge") {
		t.Error("global.md should contain synthesized content")
	}
}

func TestLearnWorkflow_PartialExplorerFailure(t *testing.T) {
	pm := &mockExplorer{findings: "Project structure mapped"}
	coder := &mockExplorer{findings: "Architecture documented"}
	reviewer := &mockExplorer{err: fmt.Errorf("failed to explore")}

	explorers := map[string]LearnExplorer{
		"coder":    coder,
		"reviewer": reviewer,
	}

	synth := &mockSynthesizer{}
	lw := NewLearnWorkflow(pm, explorers, synth)

	result, err := lw.Run(context.Background(), LearnConfig{Channel: "C", Thread: "T"})
	if err != nil {
		t.Fatalf("should not fail entirely: %v", err)
	}

	// Should have 3 explorations (pm + 2 technical)
	if len(result.Explorations) != 3 {
		t.Errorf("expected 3 explorations, got %d", len(result.Explorations))
	}

	// One explorer should have an error
	hasError := false
	for _, exp := range result.Explorations {
		if exp.Error != "" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected one exploration with error")
	}
}

func TestLearnWorkflow_PMFailure(t *testing.T) {
	pm := &mockExplorer{err: fmt.Errorf("PM crashed")}
	synth := &mockSynthesizer{}

	lw := NewLearnWorkflow(pm, nil, synth)

	_, err := lw.Run(context.Background(), LearnConfig{Channel: "C", Thread: "T"})
	if err == nil {
		t.Error("expected error when PM fails")
	}
	if !strings.Contains(err.Error(), "phase 1") {
		t.Errorf("error should mention phase 1: %v", err)
	}
}

func TestLearnWorkflow_SynthesisFailure(t *testing.T) {
	pm := &mockExplorer{findings: "map"}
	synth := &mockSynthesizer{err: fmt.Errorf("synthesis failed")}

	lw := NewLearnWorkflow(pm, map[string]LearnExplorer{}, synth)

	_, err := lw.Run(context.Background(), LearnConfig{Channel: "C", Thread: "T"})
	if err == nil {
		t.Error("expected error when synthesis fails")
	}
	if !strings.Contains(err.Error(), "phase 3") {
		t.Errorf("error should mention phase 3: %v", err)
	}
}

func TestLearnWorkflow_Relearn(t *testing.T) {
	pm := &mockExplorer{findings: "Updated project map"}
	synth := &mockSynthesizer{result: "# Updated Global Knowledge"}

	lw := NewLearnWorkflow(pm, map[string]LearnExplorer{}, synth)

	result, err := lw.Run(context.Background(), LearnConfig{
		Channel:   "C",
		Thread:    "T",
		IsRelearn: true,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !result.IsRelearn {
		t.Error("should be marked as relearn")
	}
}

func TestNeedsLearn(t *testing.T) {
	tests := []struct {
		name         string
		hasCode      bool
		globalMD     string
		expected     bool
	}{
		{"fresh repo, no code", false, "", false},
		{"code exists, no global.md", true, "", true},
		{"code exists, empty global.md", true, "  \n  ", true},
		{"code exists, has global.md", true, "# Global Knowledge\n...", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsLearn(tt.hasCode, tt.globalMD); got != tt.expected {
				t.Errorf("NeedsLearn(%v, %q) = %v, want %v", tt.hasCode, tt.globalMD, got, tt.expected)
			}
		})
	}
}

func TestFormatLearnPrompt(t *testing.T) {
	tests := []struct {
		role     string
		contains string
	}{
		{"pm", "structure, README"},
		{"coder", "architecture, patterns"},
		{"reviewer", "test coverage"},
		{"artist", "UI components"},
		{"lead", "Synthesize"},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			prompt := FormatLearnPrompt(tt.role, "some pm map", false)
			if !strings.Contains(prompt, tt.contains) {
				t.Errorf("prompt for %s should contain %q: %s", tt.role, tt.contains, prompt)
			}
		})
	}
}

func TestFormatLearnPrompt_Relearn(t *testing.T) {
	prompt := FormatLearnPrompt("pm", "", true)
	if !strings.Contains(prompt, "Re-learn") {
		t.Error("relearn prompt should mention Re-learn")
	}
	if !strings.Contains(prompt, "Remove outdated") {
		t.Error("relearn prompt should mention removing outdated info")
	}
}

func TestFormatLearnPrompt_WithPMMap(t *testing.T) {
	prompt := FormatLearnPrompt("coder", "Go backend with REST API", false)
	if !strings.Contains(prompt, "Go backend with REST API") {
		t.Error("prompt should include PM map")
	}
}

func TestFormatSynthesisPrompt(t *testing.T) {
	findings := map[string]string{
		"pm":    "Project has auth and API modules",
		"coder": "Uses clean architecture with DI",
	}

	prompt := FormatSynthesisPrompt(findings)
	if !strings.Contains(prompt, "auth and API") {
		t.Error("prompt should include PM findings")
	}
	if !strings.Contains(prompt, "clean architecture") {
		t.Error("prompt should include coder findings")
	}
}

func TestCompactKnowledge(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		fresh    string
		contains string
	}{
		{"empty existing", "", "new info", "new info"},
		{"empty fresh", "old info", "", "old info"},
		{"both present", "old info", "new info", "Existing knowledge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompactKnowledge(tt.existing, tt.fresh)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in result: %s", tt.contains, result)
			}
		})
	}
}

func TestLearnWorkflow_ParallelExploration(t *testing.T) {
	pm := &mockExplorer{findings: "Project mapped"}

	var started sync.WaitGroup
	started.Add(3)

	coder := &mockExplorer{
		findings: "coder findings",
		delay:    50 * time.Millisecond,
	}
	reviewer := &mockExplorer{
		findings: "reviewer findings",
		delay:    50 * time.Millisecond,
	}
	artist := &mockExplorer{
		findings: "artist findings",
		delay:    50 * time.Millisecond,
	}

	explorers := map[string]LearnExplorer{
		"coder":    coder,
		"reviewer": reviewer,
		"artist":   artist,
	}

	synth := &mockSynthesizer{}
	lw := NewLearnWorkflow(pm, explorers, synth)

	start := time.Now()
	result, err := lw.Run(context.Background(), LearnConfig{Channel: "C", Thread: "T"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// If all 3 explorers ran in parallel (50ms each), total should be ~50ms, not 150ms
	if elapsed > 200*time.Millisecond {
		t.Errorf("explorers should run in parallel, took %v", elapsed)
	}

	if len(result.Explorations) != 4 {
		t.Errorf("expected 4 explorations, got %d", len(result.Explorations))
	}
}
