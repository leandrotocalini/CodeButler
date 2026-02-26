package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// ResearcherConfig holds Researcher-specific configuration.
type ResearcherConfig struct {
	Model      string
	MaxTurns   int
	ResearchDir string // path to .codebutler/research/
}

// DefaultResearcherConfig returns sensible Researcher defaults.
func DefaultResearcherConfig() ResearcherConfig {
	return ResearcherConfig{
		Model:    "anthropic/claude-sonnet-4-20250514",
		MaxTurns: 15,
	}
}

// ResearcherRunner wraps AgentRunner with Researcher-specific functionality.
type ResearcherRunner struct {
	*AgentRunner
	researcherConfig ResearcherConfig
	logger           *slog.Logger
}

// ResearcherRunnerOption configures the Researcher runner.
type ResearcherRunnerOption func(*ResearcherRunner)

// WithResearcherLogger sets the logger for the Researcher runner.
func WithResearcherLogger(l *slog.Logger) ResearcherRunnerOption {
	return func(r *ResearcherRunner) {
		r.logger = l
	}
}

// NewResearcherRunner creates a Researcher agent runner.
func NewResearcherRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config ResearcherConfig,
	systemPrompt string,
	opts ...ResearcherRunnerOption,
) *ResearcherRunner {
	agentConfig := AgentConfig{
		Role:         "researcher",
		Model:        config.Model,
		MaxTurns:     config.MaxTurns,
		SystemPrompt: systemPrompt,
	}

	researcher := &ResearcherRunner{
		researcherConfig: config,
		logger:           slog.Default(),
	}

	for _, opt := range opts {
		opt(researcher)
	}

	researcher.AgentRunner = NewAgentRunner(provider, sender, executor, agentConfig,
		WithLogger(researcher.logger),
	)

	return researcher
}

// Research handles a research request from another agent.
func (r *ResearcherRunner) Research(ctx context.Context, query, requester, channel, thread string) (*Result, error) {
	prompt := FormatResearchPrompt(query, requester, r.researcherConfig.ResearchDir)

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

	r.logger.Info("researcher starting research",
		"query_preview", truncate(query, 80),
		"requester", requester,
	)

	return r.AgentRunner.Run(ctx, task)
}

// --- Research Protocol ---

// ResearchFinding represents a single finding from web research.
type ResearchFinding struct {
	Key        string // key finding
	Source     string // source URL
	Confidence string // high, medium, low
}

// FormatResearchPrompt creates the research prompt.
func FormatResearchPrompt(query, requester, researchDir string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Research Request from @codebutler.%s\n\n", requester))
	b.WriteString(fmt.Sprintf("**Query:** %s\n\n", query))

	b.WriteString("### Instructions\n\n")

	if researchDir != "" {
		b.WriteString(fmt.Sprintf("1. First check existing research in `%s` — don't duplicate work\n", researchDir))
		b.WriteString("2. If existing research doesn't answer the question, use WebSearch\n")
	} else {
		b.WriteString("1. Use WebSearch to find relevant information\n")
	}
	b.WriteString("3. Use WebFetch to read the most relevant sources\n")
	b.WriteString("4. Synthesize findings into a structured response\n")
	b.WriteString("5. If findings are valuable beyond this thread, persist to research directory\n")
	b.WriteString(fmt.Sprintf("6. Reply to @codebutler.%s with your findings\n\n", requester))

	b.WriteString("### Output Format\n\n")
	b.WriteString("```\n")
	b.WriteString("Research: [topic]\n\n")
	b.WriteString("Findings:\n")
	b.WriteString("1. [key finding] — [source]\n")
	b.WriteString("2. [key finding] — [source]\n\n")
	b.WriteString("Recommendation: [what the requester should know]\n\n")
	b.WriteString("Persisted: [filepath] (or: not persisted — one-time answer)\n\n")
	b.WriteString("Sources:\n")
	b.WriteString("- [url] — [what it covers]\n")
	b.WriteString("```\n")

	return b.String()
}

// FormatResearchFindings formats findings into the spec output format.
func FormatResearchFindings(topic string, findings []ResearchFinding, recommendation string, persistedPath string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Research: %s\n\n", topic))

	b.WriteString("Findings:\n")
	for i, f := range findings {
		b.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, f.Key, f.Source))
	}

	if recommendation != "" {
		b.WriteString(fmt.Sprintf("\nRecommendation: %s\n", recommendation))
	}

	if persistedPath != "" {
		b.WriteString(fmt.Sprintf("\nPersisted: %s\n", persistedPath))
	} else {
		b.WriteString("\nPersisted: not persisted — one-time answer\n")
	}

	if len(findings) > 0 {
		b.WriteString("\nSources:\n")
		seen := make(map[string]bool)
		for _, f := range findings {
			if f.Source != "" && !seen[f.Source] {
				b.WriteString(fmt.Sprintf("- %s\n", f.Source))
				seen[f.Source] = true
			}
		}
	}

	return b.String()
}

// ParseResearchFindings extracts findings from researcher's response.
// Looks for numbered lines in the Findings section.
func ParseResearchFindings(text string) []ResearchFinding {
	var findings []ResearchFinding
	lines := strings.Split(text, "\n")

	inFindings := false
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Findings:") {
			inFindings = true
			continue
		}

		// End of findings section on next header or empty line after findings
		if inFindings && (strings.HasPrefix(line, "Recommendation:") ||
			strings.HasPrefix(line, "Persisted:") ||
			strings.HasPrefix(line, "Sources:")) {
			break
		}

		if !inFindings {
			continue
		}

		// Parse "N. key finding — source"
		if len(line) < 3 {
			continue
		}
		// Skip the number prefix
		dotIdx := strings.Index(line, ". ")
		if dotIdx < 0 || dotIdx > 3 {
			continue
		}
		rest := line[dotIdx+2:]

		parts := strings.SplitN(rest, " — ", 2)
		finding := ResearchFinding{Key: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			finding.Source = strings.TrimSpace(parts[1])
		}
		if finding.Key != "" {
			findings = append(findings, finding)
		}
	}

	return findings
}

// ResearchTopicSlug generates a filename-safe slug from a topic.
func ResearchTopicSlug(topic string) string {
	lower := strings.ToLower(topic)
	// Replace non-alphanumeric with dashes
	var b strings.Builder
	prevDash := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 60 {
		result = result[:60]
	}
	return result
}
