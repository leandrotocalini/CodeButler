package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_PMWithAll(t *testing.T) {
	seeds := &SeedFiles{
		Role:      "pm",
		Seed:      "# PM Agent\nYou are the PM.",
		Global:    "# Global\nShared knowledge.",
		Workflows: "# Workflows\nWorkflow list.",
	}
	skillIndex := "## Available Skills\n- explain: Explain code."

	prompt := BuildSystemPrompt(seeds, skillIndex)

	for _, want := range []string{"PM Agent", "Global", "Workflows", "Available Skills"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	// Check separator
	if !strings.Contains(prompt, "---") {
		t.Error("expected --- separators between sections")
	}
}

func TestBuildSystemPrompt_CoderNoWorkflowsOrSkills(t *testing.T) {
	seeds := &SeedFiles{
		Role:   "coder",
		Seed:   "# Coder Agent",
		Global: "# Global",
	}

	prompt := BuildSystemPrompt(seeds, "")

	if !strings.Contains(prompt, "Coder Agent") {
		t.Error("missing coder seed")
	}
	if !strings.Contains(prompt, "Global") {
		t.Error("missing global")
	}
	if strings.Contains(prompt, "Workflows") {
		t.Error("coder should not have workflows")
	}
	if strings.Contains(prompt, "Skills") {
		t.Error("coder should not have skills index")
	}
}

func TestBuildSystemPrompt_EmptySeeds(t *testing.T) {
	seeds := &SeedFiles{Role: "test"}
	prompt := BuildSystemPrompt(seeds, "")
	if prompt != "" {
		t.Errorf("expected empty prompt, got %q", prompt)
	}
}

func TestBuildSystemPrompt_SeedOnly(t *testing.T) {
	seeds := &SeedFiles{
		Role: "reviewer",
		Seed: "# Reviewer",
	}

	prompt := BuildSystemPrompt(seeds, "")

	if prompt != "# Reviewer" {
		t.Errorf("expected just seed, got %q", prompt)
	}
	// Should not contain separator if only one section
	if strings.Contains(prompt, "---") {
		t.Error("single section should not have separator")
	}
}

func TestBuildSystemPrompt_SectionsOrdered(t *testing.T) {
	seeds := &SeedFiles{
		Role:      "pm",
		Seed:      "SEED",
		Global:    "GLOBAL",
		Workflows: "WORKFLOWS",
	}
	skillIndex := "SKILLS"

	prompt := BuildSystemPrompt(seeds, skillIndex)

	seedIdx := strings.Index(prompt, "SEED")
	globalIdx := strings.Index(prompt, "GLOBAL")
	workflowIdx := strings.Index(prompt, "WORKFLOWS")
	skillIdx := strings.Index(prompt, "SKILLS")

	if seedIdx >= globalIdx {
		t.Error("seed should come before global")
	}
	if globalIdx >= workflowIdx {
		t.Error("global should come before workflows")
	}
	if workflowIdx >= skillIdx {
		t.Error("workflows should come before skills")
	}
}
