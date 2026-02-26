package agent

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultArtistConfig(t *testing.T) {
	cfg := DefaultArtistConfig()
	if cfg.MaxTurns != 15 {
		t.Errorf("expected 15 max turns, got %d", cfg.MaxTurns)
	}
	if cfg.Model == "" {
		t.Error("expected non-empty model")
	}
}

func TestArtistRunner_Design(t *testing.T) {
	ctx := context.Background()

	provider := &mockProvider{
		responses: []*ChatResponse{
			// Artist reads existing UI patterns
			{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "a-1", Name: "Glob", Arguments: `{"pattern":"src/components/**/*.tsx"}`},
			}}},
			// Artist produces design proposal
			{Message: Message{Role: "assistant", Content: `UX Proposal: Login Page

Layout:
- Full-width centered card on gradient background

Components:
- LoginForm — email/password inputs with validation (states: idle, loading, error, success)
- SocialLogin — Google/GitHub OAuth buttons
- ForgotPassword — link to password reset flow

Interaction:
- User enters credentials, form validates on blur
- Submit shows loading spinner, disables button
- Error shows inline message below the field
- Success redirects to dashboard

Responsive:
- Desktop: centered card, 400px width
- Mobile: full-width card, no gradient background

Notes for Coder:
- Use existing Button component from src/components/ui/Button.tsx
- Form validation with react-hook-form (already in deps)
- OAuth flow uses existing auth provider from src/lib/auth.ts`}},
		},
	}

	executor := &mockExecutor{
		results: map[string]ToolResult{
			"Glob": {Content: "src/components/ui/Button.tsx\nsrc/components/ui/Input.tsx"},
		},
		toolDefs: []ToolDefinition{
			{Name: "Read"}, {Name: "Grep"}, {Name: "Glob"},
			{Name: "GenerateImage"}, {Name: "SendMessage"},
		},
	}

	artist := NewArtistRunner(
		provider,
		&discardSender{},
		executor,
		ArtistConfig{
			Model:     "anthropic/claude-sonnet-4-20250514",
			MaxTurns:  15,
			AssetsDir: "artist/assets/",
		},
		"You are the Artist agent.",
	)

	result, err := artist.Design(ctx, "Design a login page with email/password and social login options", "C-test", "T-test")
	if err != nil {
		t.Fatalf("design failed: %v", err)
	}
	if result.Response == "" {
		t.Error("expected design response")
	}
	if !strings.Contains(result.Response, "UX Proposal") {
		t.Error("response should contain UX Proposal")
	}
}

func TestFormatDesignPrompt(t *testing.T) {
	prompt := FormatDesignPrompt("Design a dashboard with charts", "artist/assets/")

	if !strings.Contains(prompt, "Design Request") {
		t.Error("missing design request header")
	}
	if !strings.Contains(prompt, "dashboard with charts") {
		t.Error("missing request content")
	}
	if !strings.Contains(prompt, "artist/assets/") {
		t.Error("missing assets dir reference")
	}
	if !strings.Contains(prompt, "UX Proposal") {
		t.Error("missing output format")
	}
}

func TestFormatDesignPrompt_NoAssetsDir(t *testing.T) {
	prompt := FormatDesignPrompt("test request", "")
	if strings.Contains(prompt, "Check existing UI") {
		t.Error("should not mention existing patterns when no assets dir")
	}
}

func TestFormatDesignProposal(t *testing.T) {
	proposal := DesignProposal{
		Feature: "Login Page",
		Layout:  "Centered card on gradient background",
		Components: []ComponentSpec{
			{Name: "LoginForm", Purpose: "Email/password inputs", States: []string{"idle", "loading", "error"}},
			{Name: "SocialLogin", Purpose: "OAuth buttons"},
		},
		Interaction: []string{
			"Validates on blur",
			"Shows loading on submit",
		},
		Responsive: ResponsiveSpec{
			Desktop: "centered card, 400px",
			Mobile:  "full-width card",
		},
		CoderNotes: []string{
			"Use existing Button component",
			"Form validation with react-hook-form",
		},
		Images: []string{
			".codebutler/images/login-mockup.png",
		},
	}

	text := FormatDesignProposal(proposal)

	if !strings.Contains(text, "Login Page") {
		t.Error("missing feature name")
	}
	if !strings.Contains(text, "Centered card") {
		t.Error("missing layout")
	}
	if !strings.Contains(text, "LoginForm") {
		t.Error("missing component")
	}
	if !strings.Contains(text, "idle, loading, error") {
		t.Error("missing component states")
	}
	if !strings.Contains(text, "Validates on blur") {
		t.Error("missing interaction")
	}
	if !strings.Contains(text, "Desktop: centered card") {
		t.Error("missing desktop responsive")
	}
	if !strings.Contains(text, "Mobile: full-width") {
		t.Error("missing mobile responsive")
	}
	if !strings.Contains(text, "Use existing Button") {
		t.Error("missing coder notes")
	}
	if !strings.Contains(text, "login-mockup.png") {
		t.Error("missing generated images")
	}
}

func TestParseDesignProposal(t *testing.T) {
	text := `UX Proposal: Login Page

Layout:
- Centered card on gradient background

Components:
- LoginForm — email/password with validation
- SocialLogin — OAuth buttons

Interaction:
- Validates on blur
- Shows loading spinner on submit

Responsive:
- Desktop: centered 400px card
- Mobile: full-width card

Notes for Coder:
- Use existing Button component
- Add aria-labels for accessibility`

	proposal := ParseDesignProposal(text)

	if proposal.Feature != "Login Page" {
		t.Errorf("feature: got %q", proposal.Feature)
	}
	if proposal.Layout != "Centered card on gradient background" {
		t.Errorf("layout: got %q", proposal.Layout)
	}
	if len(proposal.Components) != 2 {
		t.Errorf("expected 2 components, got %d", len(proposal.Components))
	}
	if len(proposal.Interaction) != 2 {
		t.Errorf("expected 2 interactions, got %d", len(proposal.Interaction))
	}
	if proposal.Responsive.Desktop != "centered 400px card" {
		t.Errorf("desktop: got %q", proposal.Responsive.Desktop)
	}
	if proposal.Responsive.Mobile != "full-width card" {
		t.Errorf("mobile: got %q", proposal.Responsive.Mobile)
	}
	if len(proposal.CoderNotes) != 2 {
		t.Errorf("expected 2 coder notes, got %d", len(proposal.CoderNotes))
	}
}

func TestParseDesignProposal_Empty(t *testing.T) {
	proposal := ParseDesignProposal("No structured proposal here.")
	if proposal.Feature != "" {
		t.Error("expected empty feature for unstructured text")
	}
}
