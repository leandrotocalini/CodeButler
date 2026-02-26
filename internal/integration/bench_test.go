package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leandrotocalini/codebutler/internal/agent"
	"github.com/leandrotocalini/codebutler/internal/budget"
	"github.com/leandrotocalini/codebutler/internal/conflicts"
	"github.com/leandrotocalini/codebutler/internal/conversation"
	"github.com/leandrotocalini/codebutler/internal/decisions"
	"github.com/leandrotocalini/codebutler/internal/roadmap"
	"github.com/leandrotocalini/codebutler/internal/skills"
)

// --- Agent Loop Benchmarks ---

func BenchmarkConversation_SaveLoad(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench", "pm.json")
	// Suppress logging noise in benchmarks
	nopLogger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	store := conversation.NewFileStore(path, conversation.WithLogger(nopLogger))

	// Build a realistic conversation
	msgs := make([]agent.Message, 20)
	for i := range msgs {
		if i%2 == 0 {
			msgs[i] = agent.Message{Role: "user", Content: "What's the architecture of this project?"}
		} else {
			msgs[i] = agent.Message{Role: "assistant", Content: strings.Repeat("The project uses a clean architecture pattern with ", 10)}
		}
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Save(ctx, msgs)
		store.Load(ctx)
	}
}

func BenchmarkDecisionLogger_Write(b *testing.B) {
	dir := b.TempDir()
	logPath := filepath.Join(dir, "decisions.jsonl")
	logger, err := decisions.NewFileLogger(logPath, "bench-agent")
	if err != nil {
		b.Fatalf("create logger: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.LogDecision(decisions.WorkflowSelected,
			"user wants a new feature",
			"implement workflow",
			"matched implement pattern",
		)
	}
}

func BenchmarkBudgetTracker_Record(b *testing.B) {
	tracker := budget.NewTracker(budget.BudgetConfig{
		PerThreadUSD: 100.0,
		PerDayUSD:    1000.0,
	}, "")

	tokens := budget.TokenUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.Record("T1", "coder", "openai/gpt-4o", tokens)
	}
}

func BenchmarkConflictDetector_Detect(b *testing.B) {
	detector := conflicts.NewDetector()

	// Register 10 threads with overlapping files
	for i := 0; i < 10; i++ {
		files := make([]string, 5)
		for j := range files {
			files[j] = fmt.Sprintf("src/pkg%c/file%d.go", rune('a'+i), j)
		}
		// Add a shared file to create overlaps
		files = append(files, "shared/config.go")
		detector.Register(conflicts.ThreadFiles{
			ThreadID: fmt.Sprintf("T%d", i),
			Branch:   "branch",
			Files:    files,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetectOverlaps()
	}
}

func BenchmarkRoadmapParse(b *testing.B) {
	md := `# Roadmap

## Phase 1

### M1 — Bootstrap ` + "`done`" + `

- [x] Task 1
- [x] Task 2

### M2 — Config ` + "`done`" + `
Depends on: M1

- [x] Task 1
- [x] Task 2

### M3 — Client ` + "`pending`" + `
Depends on: M2

- [ ] Task 1
- [ ] Task 2
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		roadmap.ParseString(md)
	}
}

func BenchmarkSkillParse(b *testing.B) {
	skillMD := `# explain

Explain code in detail.

## Trigger

explain {file}

## Agent

pm

## Prompt

Read the file {{file}} and explain it in detail.
Focus on architecture and patterns.
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		skills.ParseSkill(skillMD)
	}
}

func BenchmarkBudgetCalculateCost(b *testing.B) {
	tokens := budget.TokenUsage{
		PromptTokens:     5000,
		CompletionTokens: 2000,
		TotalTokens:      7000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		budget.CalculateCost("anthropic/claude-opus-4-6", tokens)
	}
}

func BenchmarkJSONMarshal_ThreadReport(b *testing.B) {
	report := map[string]interface{}{
		"thread_id": "T123",
		"agents": map[string]interface{}{
			"pm":       map[string]interface{}{"turns": 5, "tokens": 15000, "cost": 0.15},
			"coder":    map[string]interface{}{"turns": 20, "tokens": 80000, "cost": 1.2},
			"reviewer": map[string]interface{}{"turns": 3, "tokens": 10000, "cost": 0.1},
		},
		"total_cost": 1.45,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(report)
	}
}
