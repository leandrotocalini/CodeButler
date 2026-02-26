package router

import (
	"regexp"
	"strings"
)

// mentionPattern matches @codebutler.<role> mentions in message text.
var mentionPattern = regexp.MustCompile(`@codebutler\.(\w+)`)

// ExtractMentions returns all @codebutler.<role> mentions found in text.
func ExtractMentions(text string) []string {
	matches := mentionPattern.FindAllStringSubmatch(text, -1)
	roles := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			roles = append(roles, m[1])
		}
	}
	return roles
}

// HasMention checks if text contains a specific @codebutler.<role> mention.
func HasMention(text, role string) bool {
	target := "@codebutler." + role
	return strings.Contains(text, target)
}

// HasAnyMention checks if text contains any @codebutler.<role> mention.
func HasAnyMention(text string) bool {
	return mentionPattern.MatchString(text)
}

// ShouldProcess determines if a given agent role should process this message.
// Filter rules (string match, no model involved):
//   - PM: process if message contains @codebutler.pm OR message contains NO @codebutler.* mention
//   - All other agents: process only if message contains @codebutler.<their-role>
func ShouldProcess(role, text string) bool {
	if role == "pm" {
		// PM gets messages addressed to it OR messages with no agent mention
		return HasMention(text, "pm") || !HasAnyMention(text)
	}
	return HasMention(text, role)
}
