package router

import (
	"regexp"
	"strings"
)

// Redactor filters sensitive content from outbound messages.
// Uses a set of built-in patterns plus optional custom patterns.
type Redactor struct {
	patterns []*regexp.Regexp
}

// defaultPatterns are the built-in redaction patterns from SPEC.md.
// They catch API keys, JWTs, private keys, connection strings, and internal IPs.
var defaultPatterns = []*regexp.Regexp{
	// API keys (common prefixes)
	regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`),                      // OpenAI/Anthropic
	regexp.MustCompile(`(?i)(xoxb-[a-zA-Z0-9-]+)`),                       // Slack bot token
	regexp.MustCompile(`(?i)(xoxp-[a-zA-Z0-9-]+)`),                       // Slack user token
	regexp.MustCompile(`(?i)(xapp-[a-zA-Z0-9-]+)`),                       // Slack app token
	regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36,})`),                     // GitHub PAT
	regexp.MustCompile(`(?i)(gho_[a-zA-Z0-9]{36,})`),                     // GitHub OAuth
	regexp.MustCompile(`(?i)(AKIA[A-Z0-9]{16})`),                         // AWS access key
	regexp.MustCompile(`(?i)(AIza[A-Za-z0-9_-]{35})`),                    // Google API key

	// JWTs (three base64-encoded segments separated by dots)
	regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),

	// Private keys
	regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),

	// Connection strings
	regexp.MustCompile(`(?i)((?:postgres|mysql|mongodb|redis|amqp)://[^\s"']+)`),

	// Internal/private IPs
	regexp.MustCompile(`\b(10\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`),
	regexp.MustCompile(`\b(172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})\b`),
	regexp.MustCompile(`\b(192\.168\.\d{1,3}\.\d{1,3})\b`),
}

const redactedPlaceholder = "[REDACTED]"

// NewRedactor creates a redactor with default patterns.
func NewRedactor() *Redactor {
	return &Redactor{
		patterns: defaultPatterns,
	}
}

// AddPattern adds a custom regex pattern to the redactor.
func (r *Redactor) AddPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	r.patterns = append(r.patterns, re)
	return nil
}

// AddPatterns adds multiple custom regex patterns.
func (r *Redactor) AddPatterns(patterns []string) error {
	for _, p := range patterns {
		if err := r.AddPattern(p); err != nil {
			return err
		}
	}
	return nil
}

// Redact replaces all sensitive matches in text with [REDACTED].
// This is a pure function on the text content — microseconds, no LLM.
func (r *Redactor) Redact(text string) string {
	result := text
	for _, p := range r.patterns {
		result = p.ReplaceAllString(result, redactedPlaceholder)
	}
	return result
}

// ContainsSensitive checks if text matches any redaction pattern.
func (r *Redactor) ContainsSensitive(text string) bool {
	for _, p := range r.patterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// PrefixMessage adds the @codebutler.<role>: prefix to outbound messages.
func PrefixMessage(role, text string) string {
	prefix := "@codebutler." + role + ": "
	if strings.HasPrefix(text, prefix) {
		return text // already prefixed
	}
	return prefix + text
}
