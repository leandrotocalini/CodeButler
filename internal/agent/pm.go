package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// IntentType represents the classification of a user's intent.
type IntentType string

const (
	IntentWorkflow IntentType = "workflow"
	IntentSkill    IntentType = "skill"
	IntentAmbiguous IntentType = "ambiguous"
)

// Intent represents a classified user intent.
type Intent struct {
	Type     IntentType
	Name     string   // workflow or skill name
	Params   map[string]string // extracted parameters
}

// WorkflowDef represents a workflow available for matching.
type WorkflowDef struct {
	Name        string
	Description string
	Keywords    []string // keywords that suggest this workflow
}

// SkillDef represents a skill available for matching.
type SkillDef struct {
	Name        string
	Description string
	Triggers    []string
	Agent       string
}

// DefaultWorkflows returns the standard set of workflows.
func DefaultWorkflows() []WorkflowDef {
	return []WorkflowDef{
		{Name: "implement", Description: "build a feature or change", Keywords: []string{"implement", "build", "add", "create", "feature"}},
		{Name: "bugfix", Description: "find and fix a bug", Keywords: []string{"fix", "bug", "broken", "error", "crash", "issue"}},
		{Name: "question", Description: "answer a question about the codebase", Keywords: []string{"what", "how", "why", "where", "explain", "?"}},
		{Name: "refactor", Description: "restructure existing code", Keywords: []string{"refactor", "restructure", "reorganize", "clean up", "simplify"}},
		{Name: "discover", Description: "plan multiple features, build a roadmap", Keywords: []string{"discover", "plan", "roadmap", "multiple", "batch"}},
		{Name: "learn", Description: "explore the codebase and build knowledge", Keywords: []string{"learn", "onboard", "understand", "explore"}},
	}
}

// ClassifyIntent classifies a user message into a workflow or skill.
// This is a deterministic pre-filter — the LLM (PM) makes the final decision.
// Returns IntentAmbiguous if no clear match is found.
func ClassifyIntent(message string, workflows []WorkflowDef, skills []SkillDef) Intent {
	lower := strings.ToLower(message)

	// Check skills first (more specific)
	for _, s := range skills {
		for _, trigger := range s.Triggers {
			// Simple prefix/substring match on trigger keywords
			triggerLower := strings.ToLower(trigger)
			// Remove {param} placeholders for matching
			triggerClean := strings.NewReplacer("{", "", "}", "").Replace(triggerLower)
			words := strings.Fields(triggerClean)
			if len(words) > 0 && strings.Contains(lower, words[0]) {
				return Intent{
					Type: IntentSkill,
					Name: s.Name,
				}
			}
		}
	}

	// Check workflows (broader keywords)
	bestMatch := ""
	bestScore := 0
	for _, w := range workflows {
		score := 0
		for _, kw := range w.Keywords {
			if strings.Contains(lower, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestMatch = w.Name
		}
	}

	if bestScore >= 1 {
		return Intent{
			Type: IntentWorkflow,
			Name: bestMatch,
		}
	}

	return Intent{Type: IntentAmbiguous}
}

// PMConfig holds PM-specific configuration.
type PMConfig struct {
	Model       string
	MaxTurns    int
	ModelPool   []string // available models for hot swap
	SeedsDir   string
	SkillsDir  string
}

// DefaultPMConfig returns sensible PM defaults.
func DefaultPMConfig() PMConfig {
	return PMConfig{
		Model:    "anthropic/claude-sonnet-4-20250514",
		MaxTurns: 15,
	}
}

// TaskComplexity represents the assessed complexity of a coding task.
type TaskComplexity string

const (
	ComplexitySimple  TaskComplexity = "simple"
	ComplexityMedium  TaskComplexity = "medium"
	ComplexityComplex TaskComplexity = "complex"
)

// ClassifyComplexity determines task complexity for dynamic model routing.
// Simple tasks (1-3 files, straightforward) use cheaper models.
// Complex tasks (multi-file, architectural) use more capable models.
func ClassifyComplexity(planDescription string) TaskComplexity {
	lower := strings.ToLower(planDescription)

	// Complex signals
	complexSignals := []string{
		"architect", "redesign", "refactor", "migration",
		"multiple services", "distributed", "concurrent",
		"security", "encryption", "authentication",
		"performance", "optimization",
	}
	for _, sig := range complexSignals {
		if strings.Contains(lower, sig) {
			return ComplexityComplex
		}
	}

	// Simple signals
	simpleSignals := []string{
		"typo", "rename", "simple", "one file", "single file",
		"add comment", "update text", "change string",
		"fix import", "update version",
	}
	for _, sig := range simpleSignals {
		if strings.Contains(lower, sig) {
			return ComplexitySimple
		}
	}

	return ComplexityMedium
}

// ModelForComplexity returns the recommended model for a given complexity level.
func ModelForComplexity(complexity TaskComplexity, defaultModel string) string {
	switch complexity {
	case ComplexitySimple:
		return "anthropic/claude-sonnet-4-20250514"
	case ComplexityComplex:
		return "anthropic/claude-opus-4-20250514"
	default:
		if defaultModel != "" {
			return defaultModel
		}
		return "anthropic/claude-sonnet-4-20250514"
	}
}

// FormatWorkflowMenu formats the available workflows and skills as a Slack message.
func FormatWorkflowMenu(workflows []WorkflowDef, skills []SkillDef) string {
	var b strings.Builder
	b.WriteString("I can help you with:\n")

	for _, w := range workflows {
		b.WriteString(fmt.Sprintf("- *%s* — %s\n", w.Name, w.Description))
	}

	if len(skills) > 0 {
		b.WriteString("- _(skills: ")
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = s.Name
		}
		b.WriteString(strings.Join(names, ", "))
		b.WriteString(")_\n")
	}

	b.WriteString("\nWhat would you like to do?")
	return b.String()
}

// DelegationMessage creates a message for delegating work to another agent.
func DelegationMessage(targetRole, plan, context string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("@codebutler.%s\n\n", targetRole))
	b.WriteString("## Task\n\n")
	b.WriteString(plan)
	if context != "" {
		b.WriteString("\n\n## Context\n\n")
		b.WriteString(context)
	}
	return b.String()
}

// PMRunner wraps AgentRunner with PM-specific functionality.
type PMRunner struct {
	*AgentRunner
	workflows []WorkflowDef
	skills    []SkillDef
	pmConfig  PMConfig
	logger    *slog.Logger
}

// PMRunnerOption configures the PM runner.
type PMRunnerOption func(*PMRunner)

// WithPMWorkflows sets the available workflows.
func WithPMWorkflows(w []WorkflowDef) PMRunnerOption {
	return func(r *PMRunner) {
		r.workflows = w
	}
}

// WithPMSkills sets the available skills.
func WithPMSkills(s []SkillDef) PMRunnerOption {
	return func(r *PMRunner) {
		r.skills = s
	}
}

// NewPMRunner creates a PM agent runner.
func NewPMRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config PMConfig,
	systemPrompt string,
	opts ...PMRunnerOption,
) *PMRunner {
	agentConfig := AgentConfig{
		Role:         "pm",
		Model:        config.Model,
		MaxTurns:     config.MaxTurns,
		SystemPrompt: systemPrompt,
	}

	pm := &PMRunner{
		workflows: DefaultWorkflows(),
		pmConfig:  config,
		logger:    slog.Default(),
	}

	for _, opt := range opts {
		opt(pm)
	}

	pm.AgentRunner = NewAgentRunner(provider, sender, executor, agentConfig,
		WithLogger(pm.logger),
	)

	return pm
}

// ClassifyAndRun classifies the user's intent and runs the PM agent.
func (pm *PMRunner) ClassifyAndRun(ctx context.Context, task Task) (*Result, Intent, error) {
	// Pre-classify intent for logging/metrics (PM still makes the final call)
	userMessage := ""
	for _, m := range task.Messages {
		if m.Role == "user" {
			userMessage = m.Content
			break
		}
	}

	intent := ClassifyIntent(userMessage, pm.workflows, pm.skills)
	pm.logger.Info("pre-classified intent",
		"type", intent.Type,
		"name", intent.Name,
		"message_preview", truncate(userMessage, 80),
	)

	result, err := pm.AgentRunner.Run(ctx, task)
	return result, intent, err
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
