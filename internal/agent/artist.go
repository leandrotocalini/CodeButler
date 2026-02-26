package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// ArtistConfig holds Artist-specific configuration.
type ArtistConfig struct {
	Model     string // UX reasoning model (Claude Sonnet via OpenRouter)
	MaxTurns  int
	ImagesDir string // path to .codebutler/images/
	AssetsDir string // path to artist/assets/
}

// DefaultArtistConfig returns sensible Artist defaults.
func DefaultArtistConfig() ArtistConfig {
	return ArtistConfig{
		Model:    "anthropic/claude-sonnet-4-20250514",
		MaxTurns: 15,
	}
}

// ArtistRunner wraps AgentRunner with Artist-specific functionality.
type ArtistRunner struct {
	*AgentRunner
	artistConfig ArtistConfig
	logger       *slog.Logger
}

// ArtistRunnerOption configures the Artist runner.
type ArtistRunnerOption func(*ArtistRunner)

// WithArtistLogger sets the logger for the Artist runner.
func WithArtistLogger(l *slog.Logger) ArtistRunnerOption {
	return func(r *ArtistRunner) {
		r.logger = l
	}
}

// NewArtistRunner creates an Artist agent runner.
func NewArtistRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config ArtistConfig,
	systemPrompt string,
	opts ...ArtistRunnerOption,
) *ArtistRunner {
	agentConfig := AgentConfig{
		Role:         "artist",
		Model:        config.Model,
		MaxTurns:     config.MaxTurns,
		SystemPrompt: systemPrompt,
	}

	artist := &ArtistRunner{
		artistConfig: config,
		logger:       slog.Default(),
	}

	for _, opt := range opts {
		opt(artist)
	}

	artist.AgentRunner = NewAgentRunner(provider, sender, executor, agentConfig,
		WithLogger(artist.logger),
	)

	return artist
}

// Design handles a design request from the PM.
func (a *ArtistRunner) Design(ctx context.Context, request, channel, thread string) (*Result, error) {
	prompt := FormatDesignPrompt(request, a.artistConfig.AssetsDir)

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

	a.logger.Info("artist starting design",
		"request_preview", truncate(request, 80),
	)

	return a.AgentRunner.Run(ctx, task)
}

// --- Design Protocol ---

// DesignProposal represents a structured UX proposal.
type DesignProposal struct {
	Feature     string            // feature name
	Layout      string            // layout description
	Components  []ComponentSpec   // component specifications
	Interaction []string          // interaction/UX flow steps
	Responsive  ResponsiveSpec    // responsive behavior
	CoderNotes  []string          // implementation guidance for Coder
	Images      []string          // generated image URLs/paths
}

// ComponentSpec describes a UI component.
type ComponentSpec struct {
	Name     string   // component name
	Purpose  string   // what it does
	States   []string // possible states (empty, loading, error, populated, etc.)
	Props    []string // key props
}

// ResponsiveSpec describes responsive behavior.
type ResponsiveSpec struct {
	Desktop string
	Tablet  string
	Mobile  string
}

// FormatDesignPrompt creates the design prompt from a feature request.
func FormatDesignPrompt(request, assetsDir string) string {
	var b strings.Builder

	b.WriteString("## Design Request\n\n")
	b.WriteString(request)
	b.WriteString("\n\n")

	b.WriteString("### Instructions\n\n")
	if assetsDir != "" {
		b.WriteString(fmt.Sprintf("1. Check existing UI patterns in `%s` before proposing\n", assetsDir))
	}
	b.WriteString("2. Design coherent with existing product patterns\n")
	b.WriteString("3. Provide enough detail for the Coder to implement without ambiguity\n")
	b.WriteString("4. Generate images if visual mockups would help\n\n")

	b.WriteString("### Output Format\n\n")
	b.WriteString("```\n")
	b.WriteString("UX Proposal: [feature name]\n\n")
	b.WriteString("Layout:\n")
	b.WriteString("- [describe the layout structure]\n\n")
	b.WriteString("Components:\n")
	b.WriteString("- [component] — [purpose, behavior, states]\n\n")
	b.WriteString("Interaction:\n")
	b.WriteString("- [describe user flows, transitions, feedback]\n\n")
	b.WriteString("Responsive:\n")
	b.WriteString("- Desktop: [behavior]\n")
	b.WriteString("- Mobile: [behavior]\n\n")
	b.WriteString("Notes for Coder:\n")
	b.WriteString("- [implementation-specific guidance]\n")
	b.WriteString("```\n")

	return b.String()
}

// FormatDesignProposal formats a design proposal as text.
func FormatDesignProposal(proposal DesignProposal) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("UX Proposal: %s\n\n", proposal.Feature))

	b.WriteString("Layout:\n")
	b.WriteString(fmt.Sprintf("- %s\n\n", proposal.Layout))

	if len(proposal.Components) > 0 {
		b.WriteString("Components:\n")
		for _, c := range proposal.Components {
			b.WriteString(fmt.Sprintf("- **%s** — %s", c.Name, c.Purpose))
			if len(c.States) > 0 {
				b.WriteString(fmt.Sprintf(" (states: %s)", strings.Join(c.States, ", ")))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(proposal.Interaction) > 0 {
		b.WriteString("Interaction:\n")
		for _, step := range proposal.Interaction {
			b.WriteString(fmt.Sprintf("- %s\n", step))
		}
		b.WriteString("\n")
	}

	b.WriteString("Responsive:\n")
	if proposal.Responsive.Desktop != "" {
		b.WriteString(fmt.Sprintf("- Desktop: %s\n", proposal.Responsive.Desktop))
	}
	if proposal.Responsive.Tablet != "" {
		b.WriteString(fmt.Sprintf("- Tablet: %s\n", proposal.Responsive.Tablet))
	}
	if proposal.Responsive.Mobile != "" {
		b.WriteString(fmt.Sprintf("- Mobile: %s\n", proposal.Responsive.Mobile))
	}
	b.WriteString("\n")

	if len(proposal.CoderNotes) > 0 {
		b.WriteString("Notes for Coder:\n")
		for _, note := range proposal.CoderNotes {
			b.WriteString(fmt.Sprintf("- %s\n", note))
		}
	}

	if len(proposal.Images) > 0 {
		b.WriteString("\nGenerated Images:\n")
		for _, img := range proposal.Images {
			b.WriteString(fmt.Sprintf("- %s\n", img))
		}
	}

	return b.String()
}

// ParseDesignProposal extracts a design proposal from the artist's text response.
// This is a best-effort parser for the structured format.
func ParseDesignProposal(text string) DesignProposal {
	proposal := DesignProposal{}
	lines := strings.Split(text, "\n")

	section := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section headers
		if strings.HasPrefix(trimmed, "UX Proposal:") {
			proposal.Feature = strings.TrimSpace(strings.TrimPrefix(trimmed, "UX Proposal:"))
			continue
		}
		if trimmed == "Layout:" {
			section = "layout"
			continue
		}
		if trimmed == "Components:" {
			section = "components"
			continue
		}
		if trimmed == "Interaction:" {
			section = "interaction"
			continue
		}
		if trimmed == "Responsive:" {
			section = "responsive"
			continue
		}
		if trimmed == "Notes for Coder:" {
			section = "coder"
			continue
		}

		// Parse content based on current section
		if trimmed == "" || !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		content := strings.TrimPrefix(trimmed, "- ")

		switch section {
		case "layout":
			proposal.Layout = content
		case "components":
			proposal.Components = append(proposal.Components, ComponentSpec{
				Name:    content,
				Purpose: content,
			})
		case "interaction":
			proposal.Interaction = append(proposal.Interaction, content)
		case "responsive":
			if strings.HasPrefix(content, "Desktop:") {
				proposal.Responsive.Desktop = strings.TrimSpace(strings.TrimPrefix(content, "Desktop:"))
			} else if strings.HasPrefix(content, "Tablet:") {
				proposal.Responsive.Tablet = strings.TrimSpace(strings.TrimPrefix(content, "Tablet:"))
			} else if strings.HasPrefix(content, "Mobile:") {
				proposal.Responsive.Mobile = strings.TrimSpace(strings.TrimPrefix(content, "Mobile:"))
			}
		case "coder":
			proposal.CoderNotes = append(proposal.CoderNotes, content)
		}
	}

	return proposal
}
