package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupSkillsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile(t, dir, "explain.md", `# explain

Explain how a part of the codebase works.

## Trigger
explain {target}, how does {target} work, what does {target} do

## Agent
pm

## Prompt
Explain how {{target}} works.
`)

	writeFile(t, dir, "test.md", `# test

Write tests for a module.

## Trigger
test {target}, write tests for {target}

## Agent
coder

## Prompt
Write comprehensive tests for {{target}}.
`)

	writeFile(t, dir, "changelog.md", `# changelog

Generate a changelog.

## Trigger
changelog, generate changelog

## Agent
pm

## Prompt
Generate a changelog.
`)

	return dir
}

func TestScanSkillIndex_Success(t *testing.T) {
	dir := setupSkillsDir(t)

	skills, err := ScanSkillIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}

	// Find explain
	var explain *SkillSummary
	for i := range skills {
		if skills[i].Name == "explain" {
			explain = &skills[i]
			break
		}
	}
	if explain == nil {
		t.Fatal("explain skill not found")
	}
	if explain.Description != "Explain how a part of the codebase works." {
		t.Errorf("unexpected description: %s", explain.Description)
	}
	if !strings.Contains(explain.Triggers, "explain {target}") {
		t.Errorf("unexpected triggers: %s", explain.Triggers)
	}
}

func TestScanSkillIndex_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	skills, err := ScanSkillIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestScanSkillIndex_NonexistentDir(t *testing.T) {
	skills, err := ScanSkillIndex("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error for missing dir: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil skills, got %v", skills)
	}
}

func TestScanSkillIndex_SkipsNonMD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.txt", "not a skill")
	writeFile(t, dir, "skill.md", "# skill\n\nA skill.\n\n## Trigger\nskill\n")

	skills, err := ScanSkillIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestScanSkillIndex_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	writeFile(t, dir, "skill.md", "# skill\n\nA skill.\n\n## Trigger\nskill\n")

	skills, err := ScanSkillIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestParseSkillSummary(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		wantName string
		wantDesc string
		wantTrig string
	}{
		{
			name:     "full skill",
			filename: "explain.md",
			content:  "# explain\n\nExplain code.\n\n## Trigger\nexplain {target}\n\n## Agent\npm\n",
			wantName: "explain",
			wantDesc: "Explain code.",
			wantTrig: "explain {target}",
		},
		{
			name:     "no trigger",
			filename: "simple.md",
			content:  "# simple\n\nA simple skill.\n",
			wantName: "simple",
			wantDesc: "A simple skill.",
			wantTrig: "",
		},
		{
			name:     "no header uses filename",
			filename: "myskill.md",
			content:  "Just a description.\n",
			wantName: "myskill",
			wantDesc: "",
			wantTrig: "",
		},
		{
			name:     "empty content",
			filename: "empty.md",
			content:  "",
			wantName: "empty",
			wantDesc: "",
			wantTrig: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := parseSkillSummary(tt.filename, tt.content)
			if s.Name != tt.wantName {
				t.Errorf("name: got %q, want %q", s.Name, tt.wantName)
			}
			if s.Description != tt.wantDesc {
				t.Errorf("description: got %q, want %q", s.Description, tt.wantDesc)
			}
			if s.Triggers != tt.wantTrig {
				t.Errorf("triggers: got %q, want %q", s.Triggers, tt.wantTrig)
			}
		})
	}
}

func TestFormatSkillIndex(t *testing.T) {
	skills := []SkillSummary{
		{Name: "explain", Description: "Explain code.", Triggers: "explain {target}"},
		{Name: "test", Description: "Write tests.", Triggers: "test {target}"},
	}

	result := FormatSkillIndex(skills)

	if !strings.Contains(result, "Available Skills") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "**explain**") {
		t.Error("missing explain skill")
	}
	if !strings.Contains(result, "**test**") {
		t.Error("missing test skill")
	}
	if !strings.Contains(result, "Triggers:") {
		t.Error("missing triggers")
	}
}

func TestFormatSkillIndex_Empty(t *testing.T) {
	result := FormatSkillIndex(nil)
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
