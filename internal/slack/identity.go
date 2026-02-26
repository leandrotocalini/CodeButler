package slack

// AgentIdentity defines the display identity for an agent in Slack.
// Each agent posts with its own display name and icon emoji.
type AgentIdentity struct {
	Role        string // e.g., "pm", "coder", "reviewer"
	DisplayName string // e.g., "codebutler.pm"
	IconEmoji   string // e.g., ":clipboard:"
}

// DefaultIdentities returns the standard identities for all six agents.
func DefaultIdentities() map[string]AgentIdentity {
	return map[string]AgentIdentity{
		"pm": {
			Role:        "pm",
			DisplayName: "codebutler.pm",
			IconEmoji:   ":clipboard:",
		},
		"coder": {
			Role:        "coder",
			DisplayName: "codebutler.coder",
			IconEmoji:   ":hammer_and_wrench:",
		},
		"reviewer": {
			Role:        "reviewer",
			DisplayName: "codebutler.reviewer",
			IconEmoji:   ":mag:",
		},
		"researcher": {
			Role:        "researcher",
			DisplayName: "codebutler.researcher",
			IconEmoji:   ":books:",
		},
		"artist": {
			Role:        "artist",
			DisplayName: "codebutler.artist",
			IconEmoji:   ":art:",
		},
		"lead": {
			Role:        "lead",
			DisplayName: "codebutler.lead",
			IconEmoji:   ":star:",
		},
	}
}
