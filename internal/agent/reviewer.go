package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// ReviewerConfig holds Reviewer-specific configuration.
type ReviewerConfig struct {
	Model        string
	MaxTurns     int
	MaxRounds    int    // max review rounds before summarizing (default 3)
	BaseBranch   string // base branch for diffs (e.g., "main")
	CheapModel   string // model for first-pass review (empty = skip two-pass)
}

// DefaultReviewerConfig returns sensible Reviewer defaults.
func DefaultReviewerConfig() ReviewerConfig {
	return ReviewerConfig{
		Model:      "anthropic/claude-sonnet-4-20250514",
		MaxTurns:   30,
		MaxRounds:  3,
		BaseBranch: "main",
	}
}

// ReviewerRunner wraps AgentRunner with Reviewer-specific functionality.
type ReviewerRunner struct {
	*AgentRunner
	reviewerConfig ReviewerConfig
	logger         *slog.Logger
	currentRound   int
}

// ReviewerRunnerOption configures the Reviewer runner.
type ReviewerRunnerOption func(*ReviewerRunner)

// WithReviewerLogger sets the logger for the reviewer runner.
func WithReviewerLogger(l *slog.Logger) ReviewerRunnerOption {
	return func(r *ReviewerRunner) {
		r.logger = l
	}
}

// NewReviewerRunner creates a Reviewer agent runner.
func NewReviewerRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config ReviewerConfig,
	systemPrompt string,
	opts ...ReviewerRunnerOption,
) *ReviewerRunner {
	agentConfig := AgentConfig{
		Role:         "reviewer",
		Model:        config.Model,
		MaxTurns:     config.MaxTurns,
		SystemPrompt: systemPrompt,
	}

	reviewer := &ReviewerRunner{
		reviewerConfig: config,
		logger:         slog.Default(),
	}

	for _, opt := range opts {
		opt(reviewer)
	}

	reviewer.AgentRunner = NewAgentRunner(provider, sender, executor, agentConfig,
		WithLogger(reviewer.logger),
	)

	return reviewer
}

// ReviewWithDiff starts a review of the given diff content.
// The diff is injected as a user message to the conversation.
func (r *ReviewerRunner) ReviewWithDiff(ctx context.Context, diff, branch, channel, thread string) (*Result, error) {
	r.currentRound++
	round := r.currentRound

	prompt := FormatReviewPrompt(diff, branch, r.reviewerConfig.BaseBranch, round, r.reviewerConfig.MaxRounds)

	task := Task{
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Channel: channel,
		Thread:  thread,
	}

	r.logger.Info("reviewer starting review",
		"branch", branch,
		"round", round,
		"diff_size", len(diff),
	)

	return r.AgentRunner.Run(ctx, task)
}

// CanReview returns true if the reviewer has not exceeded the max review rounds.
func (r *ReviewerRunner) CanReview() bool {
	return r.currentRound < r.reviewerConfig.MaxRounds
}

// CurrentRound returns the current review round number.
func (r *ReviewerRunner) CurrentRound() int {
	return r.currentRound
}

// --- Review Protocol ---

// RiskCategory represents a category in the review risk matrix.
type RiskCategory string

const (
	RiskSecurity      RiskCategory = "security"
	RiskPerformance   RiskCategory = "performance"
	RiskCompatibility RiskCategory = "compatibility"
	RiskCorrectness   RiskCategory = "correctness"
)

// RiskLevel represents the severity of a risk.
type RiskLevel string

const (
	RiskNone   RiskLevel = "none"
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ReviewIssue represents a single issue found during review.
type ReviewIssue struct {
	Tag      string // security, test, quality, consistency, performance
	File     string // file path
	Line     int    // line number (0 if not applicable)
	Message  string // description of the issue
	Severity string // blocker, warning, suggestion
}

// ReviewResult represents the structured output of a review.
type ReviewResult struct {
	Invariants []string               // what must not break
	RiskMatrix map[RiskCategory]RiskLevel // risk assessment per category
	TestPlan   []string               // what tests should exist
	Issues     []ReviewIssue          // specific issues found
	Approved   bool                   // true if review passes
	Summary    string                 // overall summary
}

// FormatReviewPrompt creates the prompt for the reviewer with the diff and context.
func FormatReviewPrompt(diff, headBranch, baseBranch string, round, maxRounds int) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Review Round %d/%d\n\n", round, maxRounds))
	b.WriteString(fmt.Sprintf("Branch: `%s` → `%s`\n\n", headBranch, baseBranch))

	b.WriteString("### Diff\n\n```diff\n")
	b.WriteString(diff)
	b.WriteString("\n```\n\n")

	b.WriteString("### Review Protocol\n\n")
	b.WriteString("Please follow this structured review protocol:\n\n")
	b.WriteString("1. **Invariants** — List what must not break (existing behavior, API contracts, data integrity)\n")
	b.WriteString("2. **Risk Matrix** — Assess risk for: security, performance, compatibility, correctness (none/low/medium/high)\n")
	b.WriteString("3. **Test Plan** — What tests should exist for this change?\n")
	b.WriteString("4. **Issues** — List issues with tags and file:line references:\n")
	b.WriteString("   - `[security]` — injection, secrets, unsafe patterns\n")
	b.WriteString("   - `[test]` — missing or inadequate tests\n")
	b.WriteString("   - `[quality]` — readability, naming, complexity\n")
	b.WriteString("   - `[consistency]` — deviates from project patterns\n")
	b.WriteString("   - `[performance]` — inefficiency, scaling concerns\n\n")

	if round > 1 {
		b.WriteString("This is a re-review. Focus on whether previous issues were addressed.\n")
	}

	if round >= maxRounds {
		b.WriteString("\n**This is the final round.** If issues remain, summarize them in the PR description rather than requesting another round.\n")
	}

	return b.String()
}

// FormatReviewFeedback formats review issues into structured feedback for the Coder.
func FormatReviewFeedback(issues []ReviewIssue) string {
	if len(issues) == 0 {
		return "No issues found. LGTM!"
	}

	var b strings.Builder
	b.WriteString("Issues found in PR:\n")

	for i, issue := range issues {
		b.WriteString(fmt.Sprintf("%d. [%s]", i+1, issue.Tag))
		if issue.File != "" {
			b.WriteString(fmt.Sprintf(" %s", issue.File))
			if issue.Line > 0 {
				b.WriteString(fmt.Sprintf(":%d", issue.Line))
			}
		}
		b.WriteString(fmt.Sprintf(" — %s", issue.Message))
		if issue.Severity == "blocker" {
			b.WriteString(" (blocker)")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// ParseReviewIssues extracts review issues from the reviewer's text response.
// Looks for patterns like: N. [tag] file:line — description
func ParseReviewIssues(text string) []ReviewIssue {
	var issues []ReviewIssue
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match patterns like "1. [security] file.go:42 — description"
		// or "- [test] file_test.go — description"
		if !strings.Contains(line, "[") || !strings.Contains(line, "]") {
			continue
		}

		// Extract tag
		tagStart := strings.Index(line, "[")
		tagEnd := strings.Index(line, "]")
		if tagStart < 0 || tagEnd < 0 || tagEnd <= tagStart {
			continue
		}
		tag := line[tagStart+1 : tagEnd]

		// Validate tag
		validTags := map[string]bool{
			"security": true, "test": true, "quality": true,
			"consistency": true, "performance": true,
		}
		if !validTags[tag] {
			continue
		}

		// Extract the rest after the tag
		rest := strings.TrimSpace(line[tagEnd+1:])

		// Try to extract file:line and message
		issue := ReviewIssue{Tag: tag}

		// Look for " — " separator
		parts := strings.SplitN(rest, " — ", 2)
		if len(parts) == 2 {
			// First part might be file:line
			fileRef := strings.TrimSpace(parts[0])
			if fileRef != "" {
				refs := ExtractFileRefs(fileRef + ":1") // Ensure it has a line number
				if len(refs) > 0 {
					issue.File = refs[0].Path
					issue.Line = refs[0].Line
				} else {
					// Might just be a filename without line
					issue.File = fileRef
				}
			}
			issue.Message = strings.TrimSpace(parts[1])
		} else if len(parts) == 1 {
			issue.Message = strings.TrimSpace(parts[0])
		}

		// Check for severity markers
		if strings.Contains(strings.ToLower(issue.Message), "blocker") {
			issue.Severity = "blocker"
		} else if strings.Contains(strings.ToLower(issue.Message), "suggestion") {
			issue.Severity = "suggestion"
		} else {
			issue.Severity = "warning"
		}

		if issue.Message != "" {
			issues = append(issues, issue)
		}
	}

	return issues
}

// HasBlockers returns true if any issues are blockers.
func HasBlockers(issues []ReviewIssue) bool {
	for _, issue := range issues {
		if issue.Severity == "blocker" {
			return true
		}
	}
	return false
}

// CountByTag returns the number of issues per tag.
func CountByTag(issues []ReviewIssue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[issue.Tag]++
	}
	return counts
}

// TwoPassReviewPrompt creates a lightweight first-pass prompt for triaging.
// The first pass uses a cheaper model to check for obvious issues.
// If no issues found, the full review is skipped.
func TwoPassReviewPrompt(diff string) string {
	return fmt.Sprintf(`Quick review of this diff. Check for obvious issues only:
- Security vulnerabilities (injection, hardcoded secrets, unsafe patterns)
- Critical bugs (nil dereference, resource leaks, race conditions)
- Missing error handling

If none found, respond with "LGTM — no obvious issues."
If issues found, list them briefly.

`+"```diff\n%s\n```", diff)
}

// NeedsDeepReview checks if the first-pass response indicates a deep review is needed.
func NeedsDeepReview(firstPassResponse string) bool {
	lower := strings.ToLower(firstPassResponse)
	if strings.Contains(lower, "lgtm") && strings.Contains(lower, "no obvious") {
		return false
	}
	return true
}
