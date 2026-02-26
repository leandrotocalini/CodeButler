package decisions

import "time"

// DecisionType enumerates the kinds of decisions agents make.
type DecisionType string

const (
	// WorkflowSelected — PM chose a workflow over others.
	WorkflowSelected DecisionType = "workflow_selected"
	// SkillMatched — PM matched a skill to user input.
	SkillMatched DecisionType = "skill_matched"
	// AgentDelegated — PM delegated work to another agent.
	AgentDelegated DecisionType = "agent_delegated"
	// ModelSelected — any agent chose a specific model.
	ModelSelected DecisionType = "model_selected"
	// ToolChosen — agent chose a tool when alternatives exist.
	ToolChosen DecisionType = "tool_chosen"
	// StuckDetected — agent detected a stuck condition.
	StuckDetected DecisionType = "stuck_detected"
	// Escalated — agent escalated to Lead, PM, or user.
	Escalated DecisionType = "escalated"
	// PlanDeviated — Coder diverged from PM's plan.
	PlanDeviated DecisionType = "plan_deviated"
	// ReviewIssue — Reviewer found an issue.
	ReviewIssue DecisionType = "review_issue"
	// LearningProposed — Lead proposed a new learning.
	LearningProposed DecisionType = "learning_proposed"
	// CompactionTriggered — conversation was compacted.
	CompactionTriggered DecisionType = "compaction_triggered"
	// CircuitBreaker — circuit breaker state changed.
	CircuitBreaker DecisionType = "circuit_breaker"
)

// Decision is a structured log entry recording a significant choice point.
// Not every action is a decision — only points where the agent chose
// between alternatives.
type Decision struct {
	Timestamp    time.Time      `json:"ts"`
	Agent        string         `json:"agent"`
	Type         DecisionType   `json:"type"`
	Input        string         `json:"input"`
	State        map[string]any `json:"state,omitempty"`
	Decision     string         `json:"decision"`
	Alternatives []string       `json:"alternatives,omitempty"`
	Evidence     string         `json:"evidence"`
	Outcome      *string        `json:"outcome,omitempty"`
}

// WithOutcome returns a copy of the decision with the outcome field set.
func (d Decision) WithOutcome(outcome string) Decision {
	d.Outcome = &outcome
	return d
}

// AllDecisionTypes returns all valid decision types.
func AllDecisionTypes() []DecisionType {
	return []DecisionType{
		WorkflowSelected,
		SkillMatched,
		AgentDelegated,
		ModelSelected,
		ToolChosen,
		StuckDetected,
		Escalated,
		PlanDeviated,
		ReviewIssue,
		LearningProposed,
		CompactionTriggered,
		CircuitBreaker,
	}
}

// IsValid checks if a decision type is one of the known types.
func (dt DecisionType) IsValid() bool {
	for _, valid := range AllDecisionTypes() {
		if dt == valid {
			return true
		}
	}
	return false
}
