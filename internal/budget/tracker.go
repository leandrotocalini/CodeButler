// Package budget implements per-thread and per-day token budget tracking
// with cost estimation and enforcement. It provides thread-safe tracking,
// persistence to JSON files, and budget limit checks.
package budget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// modelPricing maps model IDs to per-million-token prices [input, output].
// Duplicated from multimodel/cost.go to avoid circular dependencies.
var modelPricing = map[string][2]float64{
	"anthropic/claude-opus-4-6":            {15.0, 75.0},
	"anthropic/claude-sonnet-4-5-20250929": {3.0, 15.0},
	"anthropic/claude-sonnet-4-20250514":   {3.0, 15.0},
	"openai/o3":               {10.0, 40.0},
	"openai/gpt-4o":           {2.5, 10.0},
	"openai/gpt-4o-mini":      {0.15, 0.6},
	"google/gemini-2.5-pro":   {1.25, 10.0},
	"google/gemini-2.0-flash": {0.1, 0.4},
	"deepseek/deepseek-r1":    {0.55, 2.19},
	"deepseek/deepseek-chat":  {0.14, 0.28},
	"moonshotai/kimi-k2":      {0.6, 2.0},
}

const defaultInputPrice = 3.0
const defaultOutputPrice = 15.0

// TokenUsage tracks token consumption for a single LLM call.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// UsageEntry records a single LLM call's cost.
type UsageEntry struct {
	Timestamp time.Time  `json:"timestamp"`
	Agent     string     `json:"agent"`
	Model     string     `json:"model"`
	Tokens    TokenUsage `json:"tokens"`
	CostUSD   float64    `json:"cost_usd"`
}

// ThreadBudget tracks cumulative cost for a single thread.
type ThreadBudget struct {
	ThreadID    string       `json:"thread_id"`
	Entries     []UsageEntry `json:"entries"`
	TotalCost   float64      `json:"total_cost"`
	TotalTokens int          `json:"total_tokens"`
	LimitUSD    float64      `json:"limit_usd"`    // 0 = unlimited
	Paused      bool         `json:"paused"`        // true if budget exceeded and awaiting approval
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// DailyBudget tracks cumulative cost for a single day.
type DailyBudget struct {
	Date        string       `json:"date"` // YYYY-MM-DD
	Entries     []UsageEntry `json:"entries"`
	TotalCost   float64      `json:"total_cost"`
	TotalTokens int          `json:"total_tokens"`
	LimitUSD    float64      `json:"limit_usd"` // 0 = unlimited
	Exhausted   bool         `json:"exhausted"`  // true if daily budget hit
}

// BudgetConfig configures budget limits.
type BudgetConfig struct {
	PerThreadUSD float64 `json:"per_thread_usd"` // per-thread limit (0 = unlimited)
	PerDayUSD    float64 `json:"per_day_usd"`    // per-day limit (0 = unlimited)
}

// BudgetExceeded is returned when a budget limit is hit.
type BudgetExceeded struct {
	Scope     string  // "thread" or "day"
	LimitUSD  float64
	ActualUSD float64
	ThreadID  string // empty for daily
}

func (e *BudgetExceeded) Error() string {
	if e.Scope == "thread" {
		return fmt.Sprintf("thread %s budget exceeded: $%.4f / $%.4f limit",
			e.ThreadID, e.ActualUSD, e.LimitUSD)
	}
	return fmt.Sprintf("daily budget exceeded: $%.4f / $%.4f limit",
		e.ActualUSD, e.LimitUSD)
}

// Clock allows injecting time for testing.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Tracker tracks per-thread and per-day budgets. Thread-safe.
type Tracker struct {
	mu      sync.Mutex
	threads map[string]*ThreadBudget
	daily   map[string]*DailyBudget // date string → budget
	config  BudgetConfig
	dataDir string // directory for persisting budget files
	clock   Clock
}

// NewTracker creates a budget tracker with the given config and data directory.
func NewTracker(config BudgetConfig, dataDir string) *Tracker {
	return &Tracker{
		threads: make(map[string]*ThreadBudget),
		daily:   make(map[string]*DailyBudget),
		config:  config,
		dataDir: dataDir,
		clock:   realClock{},
	}
}

// NewTrackerWithClock creates a tracker with an injectable clock (for testing).
func NewTrackerWithClock(config BudgetConfig, dataDir string, clock Clock) *Tracker {
	t := NewTracker(config, dataDir)
	t.clock = clock
	return t
}

// Record adds a usage entry to both thread and daily budgets.
// Returns a *BudgetExceeded error if any limit is hit (but still records the usage).
func (t *Tracker) Record(threadID, agent, model string, tokens TokenUsage) error {
	cost := CalculateCost(model, tokens)
	entry := UsageEntry{
		Timestamp: t.clock.Now(),
		Agent:     agent,
		Model:     model,
		Tokens:    tokens,
		CostUSD:   cost,
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Record in thread budget
	tb := t.getOrCreateThread(threadID)
	tb.Entries = append(tb.Entries, entry)
	tb.TotalCost += cost
	tb.TotalTokens += tokens.TotalTokens
	tb.UpdatedAt = t.clock.Now()

	// Record in daily budget
	dateKey := t.clock.Now().Format("2006-01-02")
	db := t.getOrCreateDaily(dateKey)
	db.Entries = append(db.Entries, entry)
	db.TotalCost += cost
	db.TotalTokens += tokens.TotalTokens

	// Check thread limit
	if t.config.PerThreadUSD > 0 && tb.TotalCost > t.config.PerThreadUSD {
		tb.Paused = true
		return &BudgetExceeded{
			Scope:     "thread",
			LimitUSD:  t.config.PerThreadUSD,
			ActualUSD: tb.TotalCost,
			ThreadID:  threadID,
		}
	}

	// Check daily limit
	if t.config.PerDayUSD > 0 && db.TotalCost > t.config.PerDayUSD {
		db.Exhausted = true
		return &BudgetExceeded{
			Scope:     "day",
			LimitUSD:  t.config.PerDayUSD,
			ActualUSD: db.TotalCost,
		}
	}

	return nil
}

// CheckThread returns whether a thread can continue (not paused).
func (t *Tracker) CheckThread(threadID string) (remaining float64, paused bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	tb, ok := t.threads[threadID]
	if !ok {
		return t.config.PerThreadUSD, false
	}

	if t.config.PerThreadUSD <= 0 {
		return 0, false // unlimited
	}

	return t.config.PerThreadUSD - tb.TotalCost, tb.Paused
}

// CheckDaily returns whether the daily budget allows more work.
func (t *Tracker) CheckDaily() (remaining float64, exhausted bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	dateKey := t.clock.Now().Format("2006-01-02")
	db, ok := t.daily[dateKey]
	if !ok {
		return t.config.PerDayUSD, false
	}

	if t.config.PerDayUSD <= 0 {
		return 0, false // unlimited
	}

	return t.config.PerDayUSD - db.TotalCost, db.Exhausted
}

// ResumeThread unpauses a thread (user approved continuation).
func (t *Tracker) ResumeThread(threadID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if tb, ok := t.threads[threadID]; ok {
		tb.Paused = false
	}
}

// ThreadCost returns the total cost for a thread.
func (t *Tracker) ThreadCost(threadID string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if tb, ok := t.threads[threadID]; ok {
		return tb.TotalCost
	}
	return 0
}

// DailyCost returns today's total cost.
func (t *Tracker) DailyCost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	dateKey := t.clock.Now().Format("2006-01-02")
	if db, ok := t.daily[dateKey]; ok {
		return db.TotalCost
	}
	return 0
}

// GetThreadBudget returns a copy of a thread's budget (nil if not tracked).
func (t *Tracker) GetThreadBudget(threadID string) *ThreadBudget {
	t.mu.Lock()
	defer t.mu.Unlock()

	tb, ok := t.threads[threadID]
	if !ok {
		return nil
	}

	// Return copy
	cp := *tb
	cp.Entries = make([]UsageEntry, len(tb.Entries))
	copy(cp.Entries, tb.Entries)
	return &cp
}

// GetDailyBudget returns a copy of today's budget (nil if no activity).
func (t *Tracker) GetDailyBudget() *DailyBudget {
	t.mu.Lock()
	defer t.mu.Unlock()

	dateKey := t.clock.Now().Format("2006-01-02")
	db, ok := t.daily[dateKey]
	if !ok {
		return nil
	}

	// Return copy
	cp := *db
	cp.Entries = make([]UsageEntry, len(db.Entries))
	copy(cp.Entries, db.Entries)
	return &cp
}

// Save persists a thread budget to disk as JSON.
func (t *Tracker) Save(threadID string) error {
	t.mu.Lock()
	tb, ok := t.threads[threadID]
	if !ok {
		t.mu.Unlock()
		return nil
	}
	// Copy under lock
	cp := *tb
	cp.Entries = make([]UsageEntry, len(tb.Entries))
	copy(cp.Entries, tb.Entries)
	t.mu.Unlock()

	dir := filepath.Join(t.dataDir, "budgets")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create budget dir: %w", err)
	}

	data, err := json.MarshalIndent(&cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal budget: %w", err)
	}

	path := filepath.Join(dir, threadID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write budget: %w", err)
	}
	return os.Rename(tmp, path)
}

// Load reads a thread budget from disk.
func (t *Tracker) Load(threadID string) error {
	path := filepath.Join(t.dataDir, "budgets", threadID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read budget: %w", err)
	}

	var tb ThreadBudget
	if err := json.Unmarshal(data, &tb); err != nil {
		return fmt.Errorf("parse budget: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.threads[threadID] = &tb
	return nil
}

// CalculateCost computes the USD cost for a given model and token usage.
func CalculateCost(model string, tokens TokenUsage) float64 {
	inputPrice, outputPrice := modelPrice(model)
	return float64(tokens.PromptTokens)/1_000_000*inputPrice +
		float64(tokens.CompletionTokens)/1_000_000*outputPrice
}

// EstimateCost estimates the cost for a planned LLM call.
func EstimateCost(model string, estimatedInputTokens, estimatedOutputTokens int) float64 {
	inputPrice, outputPrice := modelPrice(model)
	return float64(estimatedInputTokens)/1_000_000*inputPrice +
		float64(estimatedOutputTokens)/1_000_000*outputPrice
}

func modelPrice(model string) (float64, float64) {
	if prices, ok := modelPricing[model]; ok {
		return prices[0], prices[1]
	}
	return defaultInputPrice, defaultOutputPrice
}

// FormatCostSummary creates a human-readable cost summary for a thread.
func FormatCostSummary(tb *ThreadBudget) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Cost Summary — Thread %s\n\n", tb.ThreadID))
	b.WriteString(fmt.Sprintf("**Total cost:** $%.4f", tb.TotalCost))
	if tb.LimitUSD > 0 {
		b.WriteString(fmt.Sprintf(" / $%.2f limit", tb.LimitUSD))
		pct := tb.TotalCost / tb.LimitUSD * 100
		b.WriteString(fmt.Sprintf(" (%.1f%%)", pct))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("**Total tokens:** %d\n\n", tb.TotalTokens))

	// Per-agent breakdown
	agentCosts := make(map[string]float64)
	agentTokens := make(map[string]int)
	for _, e := range tb.Entries {
		agentCosts[e.Agent] += e.CostUSD
		agentTokens[e.Agent] += e.Tokens.TotalTokens
	}

	b.WriteString("| Agent | Tokens | Cost |\n")
	b.WriteString("|-------|--------|------|\n")
	for agent, cost := range agentCosts {
		b.WriteString(fmt.Sprintf("| %s | %d | $%.4f |\n", agent, agentTokens[agent], cost))
	}

	if tb.Paused {
		b.WriteString("\n**Status:** Paused (budget exceeded, awaiting approval)\n")
	}

	return b.String()
}

// FormatDailySummary creates a human-readable daily cost summary.
func FormatDailySummary(db *DailyBudget) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Daily Cost Summary — %s\n\n", db.Date))
	b.WriteString(fmt.Sprintf("**Total cost:** $%.4f", db.TotalCost))
	if db.LimitUSD > 0 {
		b.WriteString(fmt.Sprintf(" / $%.2f limit", db.LimitUSD))
		pct := db.TotalCost / db.LimitUSD * 100
		b.WriteString(fmt.Sprintf(" (%.1f%%)", pct))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("**Total tokens:** %d\n", db.TotalTokens))
	b.WriteString(fmt.Sprintf("**API calls:** %d\n", len(db.Entries)))

	if db.Exhausted {
		b.WriteString("\n**Status:** Daily budget exhausted — all agents stopped\n")
	}

	return b.String()
}

// CostEstimate represents a cost estimate for a planned operation.
type CostEstimate struct {
	Model            string  `json:"model"`
	EstimatedInput   int     `json:"estimated_input_tokens"`
	EstimatedOutput  int     `json:"estimated_output_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// EstimatePlanCost estimates the total cost for executing a plan.
func EstimatePlanCost(steps []CostEstimate) float64 {
	var total float64
	for _, step := range steps {
		total += step.EstimatedCostUSD
	}
	return total
}

// FormatCostEstimate formats a cost estimate for display in a plan.
func FormatCostEstimate(steps []CostEstimate) string {
	var b strings.Builder
	var total float64

	b.WriteString("### Estimated Cost\n\n")
	b.WriteString("| Step | Model | Input | Output | Cost |\n")
	b.WriteString("|------|-------|-------|--------|------|\n")

	for i, step := range steps {
		b.WriteString(fmt.Sprintf("| %d | %s | %d | %d | $%.4f |\n",
			i+1, step.Model, step.EstimatedInput, step.EstimatedOutput, step.EstimatedCostUSD))
		total += step.EstimatedCostUSD
	}

	b.WriteString(fmt.Sprintf("\n**Total estimated:** $%.4f\n", total))
	return b.String()
}

// getOrCreateThread returns or creates a thread budget. Must be called under lock.
func (t *Tracker) getOrCreateThread(threadID string) *ThreadBudget {
	if tb, ok := t.threads[threadID]; ok {
		return tb
	}
	now := t.clock.Now()
	tb := &ThreadBudget{
		ThreadID:  threadID,
		LimitUSD:  t.config.PerThreadUSD,
		CreatedAt: now,
		UpdatedAt: now,
	}
	t.threads[threadID] = tb
	return tb
}

// getOrCreateDaily returns or creates a daily budget. Must be called under lock.
func (t *Tracker) getOrCreateDaily(dateKey string) *DailyBudget {
	if db, ok := t.daily[dateKey]; ok {
		return db
	}
	db := &DailyBudget{
		Date:     dateKey,
		LimitUSD: t.config.PerDayUSD,
	}
	t.daily[dateKey] = db
	return db
}
