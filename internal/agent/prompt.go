package agent

import "strings"

// BuildSystemPrompt assembles the system prompt from seed components.
// The PM role additionally receives workflows content for intent classification.
func BuildSystemPrompt(seed, global, workflows string, isPM bool) string {
	var parts []string

	if seed != "" {
		parts = append(parts, seed)
	}
	if global != "" {
		parts = append(parts, global)
	}
	if isPM && workflows != "" {
		parts = append(parts, workflows)
	}

	return strings.Join(parts, "\n\n")
}
