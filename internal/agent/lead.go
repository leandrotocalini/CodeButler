package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// LeadConfig holds Lead-specific configuration.
type LeadConfig struct {
	Model    string
	MaxTurns int
	RepoDir  string // root repo directory for report writing
}

// DefaultLeadConfig returns sensible Lead defaults.
func DefaultLeadConfig() LeadConfig {
	return LeadConfig{
		Model:    "anthropic/claude-sonnet-4-20250514",
		MaxTurns: 20,
	}
}

// LeadRunner wraps AgentRunner with Lead-specific functionality.
type LeadRunner struct {
	*AgentRunner
	leadConfig LeadConfig
	logger     *slog.Logger
}

// LeadRunnerOption configures the Lead runner.
type LeadRunnerOption func(*LeadRunner)

// WithLeadLogger sets the logger for the Lead runner.
func WithLeadLogger(l *slog.Logger) LeadRunnerOption {
	return func(r *LeadRunner) {
		r.logger = l
	}
}

// NewLeadRunner creates a Lead agent runner.
func NewLeadRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config LeadConfig,
	systemPrompt string,
	opts ...LeadRunnerOption,
) *LeadRunner {
	agentConfig := AgentConfig{
		Role:         "lead",
		Model:        config.Model,
		MaxTurns:     config.MaxTurns,
		SystemPrompt: systemPrompt,
	}

	lead := &LeadRunner{
		leadConfig: config,
		logger:     slog.Default(),
	}

	for _, opt := range opts {
		opt(lead)
	}

	lead.AgentRunner = NewAgentRunner(provider, sender, executor, agentConfig,
		WithLogger(lead.logger),
	)

	return lead
}

// RunRetrospective starts a retrospective after a thread completes.
func (l *LeadRunner) RunRetrospective(ctx context.Context, threadSummary string, agentResults map[string]*Result, channel, thread string) (*Result, error) {
	prompt := FormatRetroPrompt(threadSummary, agentResults)

	task := Task{
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Channel: channel,
		Thread:  thread,
	}

	l.logger.Info("lead starting retrospective",
		"thread", thread,
		"agents_involved", len(agentResults),
	)

	return l.AgentRunner.Run(ctx, task)
}

// Mediate handles a disagreement between agents.
func (l *LeadRunner) Mediate(ctx context.Context, dispute string, channel, thread string) (*Result, error) {
	prompt := fmt.Sprintf(`## Mediation Request

Two agents disagree and need your decision.

### Dispute
%s

### Instructions
1. Read the positions of both agents carefully
2. Evaluate based on: code quality, team efficiency, project conventions, user intent
3. Make a clear decision with reasoning
4. If you cannot decide, summarize and ask the user`, dispute)

	task := Task{
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Channel: channel,
		Thread:  thread,
	}

	l.logger.Info("lead mediating dispute", "thread", thread)

	return l.AgentRunner.Run(ctx, task)
}

// --- Retrospective Protocol ---

// RetroProposal represents a structured proposal from the Lead.
type RetroProposal struct {
	Type        ProposalType // workflow, learning, global, guardrail
	Target      string       // target file or agent (e.g., "coder.md", "global.md", "workflows.md")
	Description string       // what to change
	Content     string       // proposed content
}

// ProposalType classifies a retrospective proposal.
type ProposalType string

const (
	ProposalWorkflow  ProposalType = "workflow"
	ProposalLearning  ProposalType = "learning"
	ProposalGlobal    ProposalType = "global"
	ProposalGuardrail ProposalType = "guardrail"
	ProposalProcess   ProposalType = "process"
	ProposalPrompt    ProposalType = "prompt"
	ProposalSkill     ProposalType = "skill"
)

// RetroResult represents the structured retrospective output.
type RetroResult struct {
	WentWell  []string        // 3 things that went well
	Friction  []string        // 3 friction points
	Proposals []RetroProposal // concrete proposals
}

// Learning represents a behavioral learning for an agent.
type Learning struct {
	When       string  // when this applies (e.g., "When reviewing auth code")
	Rule       string  // what to do
	Example    string  // concrete example
	Confidence float64 // 0.0-1.0
	Source     string  // thread ID or reason
}

// FormatLearning formats a learning for inclusion in an agent MD file.
func FormatLearning(l Learning) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- **When:** %s\n", l.When))
	b.WriteString(fmt.Sprintf("  **Rule:** %s\n", l.Rule))
	if l.Example != "" {
		b.WriteString(fmt.Sprintf("  **Example:** %s\n", l.Example))
	}
	b.WriteString(fmt.Sprintf("  **Confidence:** %.0f%% | **Source:** %s\n", l.Confidence*100, l.Source))
	return b.String()
}

// PruneLearnings removes contradictions and stale learnings.
// Returns pruned list and reasons for removal.
func PruneLearnings(learnings []Learning, maxCount int) ([]Learning, []string) {
	var pruned []Learning
	var reasons []string

	// Remove low-confidence learnings first
	for _, l := range learnings {
		if l.Confidence < 0.3 {
			reasons = append(reasons, fmt.Sprintf("removed low-confidence learning: %q (%.0f%%)", l.Rule, l.Confidence*100))
			continue
		}
		pruned = append(pruned, l)
	}

	// If still over cap, remove oldest (first in list)
	if maxCount > 0 && len(pruned) > maxCount {
		removed := len(pruned) - maxCount
		reasons = append(reasons, fmt.Sprintf("removed %d oldest learnings to stay under cap of %d", removed, maxCount))
		pruned = pruned[removed:]
	}

	return pruned, reasons
}

// --- Thread Report ---

// ThreadReport represents the structured report for a completed thread.
type ThreadReport struct {
	ThreadID          string            `json:"thread_id"`
	Timestamp         time.Time         `json:"timestamp"`
	Outcome           string            `json:"outcome"` // success, partial, failed
	AgentMetrics      map[string]AgentMetrics `json:"agent_metrics"`
	PlanDeviations    []string          `json:"plan_deviations"`
	Patterns          []ThreadPattern   `json:"patterns"`
	ReasoningMessages int               `json:"reasoning_messages"`
	TotalCost         float64           `json:"total_cost"`
	WentWell          []string          `json:"went_well"`
	Friction          []string          `json:"friction"`
	Proposals         []RetroProposal   `json:"proposals"`
}

// AgentMetrics tracks per-agent metrics for a thread.
type AgentMetrics struct {
	TurnsUsed     int     `json:"turns_used"`
	ToolCalls     int     `json:"tool_calls"`
	LoopsDetected int     `json:"loops_detected"`
	TokensUsed    int     `json:"tokens_used"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// ThreadPattern represents a pattern observed during the thread.
type ThreadPattern struct {
	Type        string `json:"type"`        // exploration_gap, wasted_turns, good_practice, etc.
	Description string `json:"description"` // what happened
	Agent       string `json:"agent"`       // which agent
	Severity    string `json:"severity"`    // info, warning, improvement
}

// NewThreadReport creates a report from agent results.
func NewThreadReport(threadID string, results map[string]*Result) ThreadReport {
	report := ThreadReport{
		ThreadID:     threadID,
		Timestamp:    time.Now(),
		AgentMetrics: make(map[string]AgentMetrics),
	}

	var totalTokens int
	for role, result := range results {
		if result == nil {
			continue
		}
		metrics := AgentMetrics{
			TurnsUsed:     result.TurnsUsed,
			ToolCalls:     result.ToolCalls,
			LoopsDetected: result.LoopsDetected,
			TokensUsed:    result.TokenUsage.TotalTokens,
		}
		report.AgentMetrics[role] = metrics
		totalTokens += result.TokenUsage.TotalTokens
	}

	// Rough cost estimate: $3/Mtokens for input, $15/Mtokens for output (Opus pricing)
	report.TotalCost = float64(totalTokens) / 1_000_000 * 9 // approximate blended rate

	return report
}

// MarshalReport serializes a thread report to JSON.
func MarshalReport(report ThreadReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// FormatUsageReport creates a human-readable usage report.
func FormatUsageReport(report ThreadReport) string {
	var b strings.Builder

	b.WriteString("## Usage Report\n\n")
	b.WriteString(fmt.Sprintf("**Thread:** %s\n", report.ThreadID))
	b.WriteString(fmt.Sprintf("**Outcome:** %s\n\n", report.Outcome))

	b.WriteString("| Agent | Turns | Tool Calls | Tokens | Loops |\n")
	b.WriteString("|-------|-------|------------|--------|-------|\n")

	for role, metrics := range report.AgentMetrics {
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
			role, metrics.TurnsUsed, metrics.ToolCalls, metrics.TokensUsed, metrics.LoopsDetected))
	}

	b.WriteString(fmt.Sprintf("\n**Estimated cost:** $%.4f\n", report.TotalCost))

	return b.String()
}

// --- Prompt Construction ---

// FormatRetroPrompt creates the retrospective prompt from thread context.
func FormatRetroPrompt(threadSummary string, agentResults map[string]*Result) string {
	var b strings.Builder

	b.WriteString("## Retrospective\n\n")
	b.WriteString("Review this completed thread and produce a structured retrospective.\n\n")

	b.WriteString("### Thread Summary\n\n")
	b.WriteString(threadSummary)
	b.WriteString("\n\n")

	b.WriteString("### Agent Metrics\n\n")
	for role, result := range agentResults {
		if result == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("- **%s**: %d turns, %d tool calls, %d tokens",
			role, result.TurnsUsed, result.ToolCalls, result.TokenUsage.TotalTokens))
		if result.LoopsDetected > 0 {
			b.WriteString(fmt.Sprintf(", %d loops detected", result.LoopsDetected))
		}
		if result.Escalated {
			b.WriteString(" (escalated)")
		}
		b.WriteString("\n")
	}

	b.WriteString("\n### Instructions\n\n")
	b.WriteString("Produce:\n")
	b.WriteString("1. **3 things that went well** — what worked, what to keep doing\n")
	b.WriteString("2. **3 friction points** — what slowed things down, caused confusion, or wasted turns\n")
	b.WriteString("3. **Proposals** (one of each):\n")
	b.WriteString("   - 1 process improvement (workflow change)\n")
	b.WriteString("   - 1 prompt improvement (agent MD update)\n")
	b.WriteString("   - 1 skill proposal (new or updated skill)\n")
	b.WriteString("   - 1 guardrail (new safety check or constraint)\n\n")
	b.WriteString("For each proposal, specify the target file and the concrete change.\n")

	return b.String()
}

// FormatMediationContext creates context for a mediation decision.
func FormatMediationContext(agent1, position1, agent2, position2 string) string {
	return fmt.Sprintf("**%s's position:** %s\n\n**%s's position:** %s",
		agent1, position1, agent2, position2)
}
