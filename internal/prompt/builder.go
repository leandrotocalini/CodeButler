package prompt

import (
	"strings"
)

// BuildSystemPrompt assembles the full system prompt for an agent role.
// Pure function: same inputs → same output.
//
// Components:
// 1. Agent seed (role-specific identity, personality, tools, rules)
// 2. Global knowledge (shared project context)
// 3. Workflows (PM only)
// 4. Skill index (PM only)
func BuildSystemPrompt(seeds *SeedFiles, skillIndex string) string {
	var parts []string

	if seeds.Seed != "" {
		parts = append(parts, seeds.Seed)
	}

	if seeds.Global != "" {
		parts = append(parts, seeds.Global)
	}

	if seeds.Workflows != "" {
		parts = append(parts, seeds.Workflows)
	}

	if skillIndex != "" {
		parts = append(parts, skillIndex)
	}

	return strings.Join(parts, "\n\n---\n\n")
}
