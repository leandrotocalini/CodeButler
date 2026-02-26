package skills

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestParseSkill_FullSkill(t *testing.T) {
	content := `# explain

Explain how a part of the codebase works.

## Trigger
explain {target}, how does {target} work, what does {target} do

## Agent
pm

## Prompt
Explain how {{target}} works in this codebase.

1. Find the relevant files
2. Read the key files
`

	s, err := ParseSkill(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.Name != "explain" {
		t.Errorf("name: got %q, want %q", s.Name, "explain")
	}
	if s.Description != "Explain how a part of the codebase works." {
		t.Errorf("description: got %q", s.Description)
	}
	if len(s.Triggers) != 3 {
		t.Fatalf("triggers: got %d, want 3", len(s.Triggers))
	}
	if s.Triggers[0] != "explain {target}" {
		t.Errorf("trigger[0]: got %q", s.Triggers[0])
	}
	if s.Agent != "pm" {
		t.Errorf("agent: got %q", s.Agent)
	}
	if s.Prompt == "" {
		t.Error("expected non-empty prompt")
	}

	// Check variables
	if len(s.Variables) != 1 {
		t.Fatalf("variables: got %d, want 1", len(s.Variables))
	}
	v := s.Variables[0]
	if v.Name != "target" {
		t.Errorf("variable name: got %q", v.Name)
	}
	if !v.InTrigger || !v.InPrompt {
		t.Error("target should be in both trigger and prompt")
	}
}

func TestParseSkill_WithDefault(t *testing.T) {
	content := `# brainstorm

Multi-model brainstorming.

## Trigger
brainstorm, brainstorm about {topic}

## Agent
pm

## Prompt
Run the brainstorm workflow for: {{topic | default: "ask user"}}
`

	s, err := ParseSkill(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(s.Variables) != 1 {
		t.Fatalf("variables: got %d, want 1", len(s.Variables))
	}
	v := s.Variables[0]
	if v.Name != "topic" {
		t.Errorf("variable name: got %q", v.Name)
	}
	if v.DefaultValue != "ask user" {
		t.Errorf("default: got %q, want %q", v.DefaultValue, "ask user")
	}
}

func TestParseSkill_MultipleVariables(t *testing.T) {
	content := `# deploy

Deploy to an environment.

## Trigger
deploy to {environment}, deploy {service} to {environment}

## Agent
pm

## Prompt
Deploy {{service | default: "all"}} to {{environment}}.
`

	s, err := ParseSkill(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sort vars by name for deterministic comparison
	sort.Slice(s.Variables, func(i, j int) bool {
		return s.Variables[i].Name < s.Variables[j].Name
	})

	if len(s.Variables) != 2 {
		t.Fatalf("variables: got %d, want 2", len(s.Variables))
	}

	env := s.Variables[0]
	if env.Name != "environment" || !env.InTrigger || !env.InPrompt {
		t.Errorf("environment var: %+v", env)
	}

	svc := s.Variables[1]
	if svc.Name != "service" || !svc.InTrigger || !svc.InPrompt {
		t.Errorf("service var: %+v", svc)
	}
	if svc.DefaultValue != "all" {
		t.Errorf("service default: got %q", svc.DefaultValue)
	}
}

func TestParseSkill_EmptyContent(t *testing.T) {
	s, err := ParseSkill("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "" {
		t.Errorf("expected empty name, got %q", s.Name)
	}
}

func TestValidateSkill_Valid(t *testing.T) {
	s := &Skill{
		Name:     "test",
		Triggers: []string{"test {target}"},
		Agent:    "coder",
		Prompt:   "Write tests for {{target}}.",
		Variables: []Variable{
			{Name: "target", InTrigger: true, InPrompt: true},
		},
	}

	errs := ValidateSkill(s, "test.md")
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateSkill_MissingName(t *testing.T) {
	s := &Skill{
		Triggers: []string{"test"},
		Agent:    "coder",
		Prompt:   "Do stuff.",
	}

	errs := ValidateSkill(s, "test.md")
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
	if !hasError(errs, "missing skill name") {
		t.Error("expected 'missing skill name' error")
	}
}

func TestValidateSkill_MissingTrigger(t *testing.T) {
	s := &Skill{
		Name:  "test",
		Agent: "coder",
		Prompt: "Do stuff.",
	}

	errs := ValidateSkill(s, "test.md")
	if !hasError(errs, "missing ## Trigger") {
		t.Error("expected trigger error")
	}
}

func TestValidateSkill_MissingAgent(t *testing.T) {
	s := &Skill{
		Name:     "test",
		Triggers: []string{"test"},
		Prompt:   "Do stuff.",
	}

	errs := ValidateSkill(s, "test.md")
	if !hasError(errs, "missing ## Agent") {
		t.Error("expected agent error")
	}
}

func TestValidateSkill_InvalidAgent(t *testing.T) {
	s := &Skill{
		Name:     "test",
		Triggers: []string{"test"},
		Agent:    "invalid",
		Prompt:   "Do stuff.",
	}

	errs := ValidateSkill(s, "test.md")
	if !hasError(errs, "invalid agent") {
		t.Error("expected invalid agent error")
	}
}

func TestValidateSkill_MissingPrompt(t *testing.T) {
	s := &Skill{
		Name:     "test",
		Triggers: []string{"test"},
		Agent:    "coder",
	}

	errs := ValidateSkill(s, "test.md")
	if !hasError(errs, "missing ## Prompt") {
		t.Error("expected prompt error")
	}
}

func TestValidateSkill_UndefinedVariable(t *testing.T) {
	s := &Skill{
		Name:     "test",
		Triggers: []string{"test"},
		Agent:    "coder",
		Prompt:   "Do {{something}} with {{target}}.",
		Variables: []Variable{
			{Name: "something", InPrompt: true},
			{Name: "target", InPrompt: true},
		},
	}

	errs := ValidateSkill(s, "test.md")
	if len(errs) < 2 {
		t.Fatalf("expected 2+ errors for undefined vars, got %d", len(errs))
	}
}

func TestValidateSkill_VariableWithDefault_OK(t *testing.T) {
	s := &Skill{
		Name:     "test",
		Triggers: []string{"test"},
		Agent:    "coder",
		Prompt:   "Do {{thing | default: \"stuff\"}}.",
		Variables: []Variable{
			{Name: "thing", InPrompt: true, DefaultValue: "stuff"},
		},
	}

	errs := ValidateSkill(s, "test.md")
	if len(errs) != 0 {
		t.Errorf("variable with default should be valid, got: %v", errs)
	}
}

func TestValidateAll_DuplicateTriggers(t *testing.T) {
	skills := []*Skill{
		{Name: "skill-a", Triggers: []string{"do {thing}"}},
		{Name: "skill-b", Triggers: []string{"do {stuff}"}},
	}

	errs := ValidateAll(skills)
	if len(errs) == 0 {
		t.Fatal("expected duplicate trigger error")
	}
	if !hasError(errs, "duplicate trigger") {
		t.Error("expected 'duplicate trigger' error")
	}
}

func TestValidateAll_NoDuplicates(t *testing.T) {
	skills := []*Skill{
		{Name: "explain", Triggers: []string{"explain {target}"}},
		{Name: "test", Triggers: []string{"test {target}"}},
	}

	errs := ValidateAll(skills)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestNormalizeTrigger(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"explain {target}", "explain {}"},
		{"Deploy {service} to {environment}", "deploy {} to {}"},
		{"simple trigger", "simple trigger"},
	}

	for _, tt := range tests {
		got := normalizeTrigger(tt.input)
		if got != tt.want {
			t.Errorf("normalizeTrigger(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoadIndex_RealSkills(t *testing.T) {
	// Use the actual seeds/skills directory
	skillsDir := filepath.Join("..", "..", "seeds", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Skip("seeds/skills not found, skipping integration test")
	}

	idx, err := LoadIndex(skillsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(idx.Skills) == 0 {
		t.Fatal("expected at least one skill loaded")
	}

	// All loaded skills should have required fields
	for _, s := range idx.Skills {
		if s.Name == "" {
			t.Error("loaded skill with empty name")
		}
		if s.Agent == "" {
			t.Errorf("skill %q has empty agent", s.Name)
		}
	}
}

func TestLoadIndex_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	idx, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(idx.Skills))
	}
}

func TestLoadIndex_NonexistentDir(t *testing.T) {
	idx, err := LoadIndex("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(idx.Skills))
	}
}

func TestLoadIndex_SkipsInvalid(t *testing.T) {
	dir := t.TempDir()

	// Valid skill
	writeTestFile(t, dir, "valid.md", `# valid

A valid skill.

## Trigger
valid

## Agent
pm

## Prompt
Do valid things.
`)

	// Invalid skill (missing agent)
	writeTestFile(t, dir, "invalid.md", `# invalid

Missing agent.

## Trigger
invalid

## Prompt
Do invalid things.
`)

	idx, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(idx.Skills) != 1 {
		t.Errorf("expected 1 valid skill, got %d", len(idx.Skills))
	}
	if idx.Skills[0].Name != "valid" {
		t.Errorf("expected valid skill, got %s", idx.Skills[0].Name)
	}
}

func TestValidate_ReportsAllErrors(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, dir, "bad1.md", `# bad1
No sections.
`)

	writeTestFile(t, dir, "bad2.md", `# bad2
Also bad.

## Trigger
bad2

## Agent
invalidagent

## Prompt
Do {{undefined}} things.
`)

	errs := Validate(dir)
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}

	// Should report errors for both files
	hasBad1 := false
	hasBad2 := false
	for _, e := range errs {
		if e.File == "bad1.md" {
			hasBad1 = true
		}
		if e.File == "bad2.md" {
			hasBad2 = true
		}
	}
	if !hasBad1 {
		t.Error("expected errors for bad1.md")
	}
	if !hasBad2 {
		t.Error("expected errors for bad2.md")
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasError(errs []ValidationError, substr string) bool {
	for _, e := range errs {
		if containsStr(e.Message, substr) {
			return true
		}
	}
	return false
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
