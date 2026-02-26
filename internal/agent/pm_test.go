package agent

import (
	"testing"
)

func TestClassifyIntent_Workflow(t *testing.T) {
	workflows := DefaultWorkflows()
	var skills []SkillDef

	tests := []struct {
		message string
		want    string
	}{
		{"implement a login page", "implement"},
		{"fix the broken auth flow", "bugfix"},
		{"how does the router work?", "question"},
		{"refactor the database layer", "refactor"},
		{"build a new search feature", "implement"},
		{"this is crashing on startup", "bugfix"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			intent := ClassifyIntent(tt.message, workflows, skills)
			if intent.Type != IntentWorkflow {
				t.Errorf("expected workflow, got %s", intent.Type)
			}
			if intent.Name != tt.want {
				t.Errorf("got %q, want %q", intent.Name, tt.want)
			}
		})
	}
}

func TestClassifyIntent_Skill(t *testing.T) {
	workflows := DefaultWorkflows()
	skills := []SkillDef{
		{Name: "explain", Triggers: []string{"explain {target}", "how does {target} work"}},
		{Name: "changelog", Triggers: []string{"changelog", "generate changelog"}},
		{Name: "hotfix", Triggers: []string{"hotfix {description}", "quick fix {description}"}},
	}

	tests := []struct {
		message string
		want    string
	}{
		{"changelog for the last release", "changelog"},
		{"hotfix the login timeout", "hotfix"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			intent := ClassifyIntent(tt.message, workflows, skills)
			if intent.Type != IntentSkill {
				t.Errorf("expected skill, got %s (name: %s)", intent.Type, intent.Name)
			}
			if intent.Name != tt.want {
				t.Errorf("got %q, want %q", intent.Name, tt.want)
			}
		})
	}
}

func TestClassifyIntent_Ambiguous(t *testing.T) {
	workflows := DefaultWorkflows()
	var skills []SkillDef

	intent := ClassifyIntent("hello there", workflows, skills)
	if intent.Type != IntentAmbiguous {
		t.Errorf("expected ambiguous, got %s (name: %s)", intent.Type, intent.Name)
	}
}

func TestClassifyComplexity(t *testing.T) {
	tests := []struct {
		desc string
		want TaskComplexity
	}{
		{"fix a typo in the readme", ComplexitySimple},
		{"rename the getUserById function", ComplexitySimple},
		{"add a new API endpoint for user profiles", ComplexityMedium},
		{"redesign the authentication architecture", ComplexityComplex},
		{"implement distributed caching with Redis", ComplexityComplex},
		{"refactor the entire database migration system", ComplexityComplex},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := ClassifyComplexity(tt.desc)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModelForComplexity(t *testing.T) {
	tests := []struct {
		complexity TaskComplexity
		wantModel  string
	}{
		{ComplexitySimple, "anthropic/claude-sonnet-4-20250514"},
		{ComplexityMedium, "default-model"},
		{ComplexityComplex, "anthropic/claude-opus-4-20250514"},
	}

	for _, tt := range tests {
		t.Run(string(tt.complexity), func(t *testing.T) {
			got := ModelForComplexity(tt.complexity, "default-model")
			if got != tt.wantModel {
				t.Errorf("got %q, want %q", got, tt.wantModel)
			}
		})
	}
}

func TestFormatWorkflowMenu(t *testing.T) {
	workflows := DefaultWorkflows()
	skills := []SkillDef{
		{Name: "explain"},
		{Name: "test"},
	}

	menu := FormatWorkflowMenu(workflows, skills)

	for _, want := range []string{"implement", "bugfix", "question", "explain", "test"} {
		if !containsStr(menu, want) {
			t.Errorf("menu missing %q", want)
		}
	}
}

func TestFormatWorkflowMenu_NoSkills(t *testing.T) {
	workflows := DefaultWorkflows()
	menu := FormatWorkflowMenu(workflows, nil)

	if containsStr(menu, "skills:") {
		t.Error("should not show skills section when none")
	}
}

func TestDelegationMessage(t *testing.T) {
	msg := DelegationMessage("coder", "Implement login page", "Auth module is at internal/auth/")

	if !containsStr(msg, "@codebutler.coder") {
		t.Error("missing target mention")
	}
	if !containsStr(msg, "Implement login page") {
		t.Error("missing plan")
	}
	if !containsStr(msg, "Auth module") {
		t.Error("missing context")
	}
}

func TestDelegationMessage_NoContext(t *testing.T) {
	msg := DelegationMessage("reviewer", "Review the PR", "")

	if containsStr(msg, "## Context") {
		t.Error("should not have context section")
	}
}

func TestDefaultPMConfig(t *testing.T) {
	cfg := DefaultPMConfig()
	if cfg.MaxTurns != 15 {
		t.Errorf("expected 15 max turns, got %d", cfg.MaxTurns)
	}
	if cfg.Model == "" {
		t.Error("expected non-empty model")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("this is a longer string", 10); got != "this is a ..." {
		t.Errorf("got %q", got)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
