package agent

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// CoderConfig holds Coder-specific configuration.
type CoderConfig struct {
	Model        string
	MaxTurns     int
	WorktreeDir  string // path to the worktree for this task
	BaseBranch   string // base branch (e.g., "main")
	HeadBranch   string // working branch (e.g., "codebutler/feat-xyz")
}

// DefaultCoderConfig returns sensible Coder defaults.
func DefaultCoderConfig() CoderConfig {
	return CoderConfig{
		Model:      "anthropic/claude-sonnet-4-20250514",
		MaxTurns:   50,
		BaseBranch: "main",
	}
}

// CoderRunner wraps AgentRunner with Coder-specific functionality.
type CoderRunner struct {
	*AgentRunner
	coderConfig CoderConfig
	logger      *slog.Logger
}

// CoderRunnerOption configures the Coder runner.
type CoderRunnerOption func(*CoderRunner)

// WithCoderLogger sets the logger for the coder runner.
func WithCoderLogger(l *slog.Logger) CoderRunnerOption {
	return func(r *CoderRunner) {
		r.logger = l
	}
}

// NewCoderRunner creates a Coder agent runner.
func NewCoderRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config CoderConfig,
	systemPrompt string,
	opts ...CoderRunnerOption,
) *CoderRunner {
	agentConfig := AgentConfig{
		Role:         "coder",
		Model:        config.Model,
		MaxTurns:     config.MaxTurns,
		SystemPrompt: systemPrompt,
	}

	coder := &CoderRunner{
		coderConfig: config,
		logger:      slog.Default(),
	}

	for _, opt := range opts {
		opt(coder)
	}

	coder.AgentRunner = NewAgentRunner(provider, sender, executor, agentConfig,
		WithLogger(coder.logger),
	)

	return coder
}

// RunWithPlan executes the Coder with a plan from the PM.
// The plan is injected as a user message to the conversation.
func (c *CoderRunner) RunWithPlan(ctx context.Context, plan string, channel, thread string) (*Result, error) {
	task := Task{
		Messages: []Message{
			{
				Role:    "user",
				Content: plan,
			},
		},
		Channel: channel,
		Thread:  thread,
	}

	c.logger.Info("coder starting with plan",
		"plan_preview", truncate(plan, 100),
		"worktree", c.coderConfig.WorktreeDir,
		"branch", c.coderConfig.HeadBranch,
	)

	return c.AgentRunner.Run(ctx, task)
}

// ParsePlan extracts structured information from a PM plan message.
// Returns the plan body and any file references found.
func ParsePlan(message string) (plan string, fileRefs []FileRef) {
	plan = message

	// Extract file:line references
	fileRefs = ExtractFileRefs(message)

	return plan, fileRefs
}

// FileRef represents a file:line reference in a plan.
type FileRef struct {
	Path string
	Line int
}

// fileRefRe matches patterns like file.go:42, internal/auth/handler.go:15
var fileRefRe = regexp.MustCompile(`([\w./\-]+\.[\w]+):(\d+)`)

// ExtractFileRefs extracts file:line references from text.
func ExtractFileRefs(text string) []FileRef {
	matches := fileRefRe.FindAllStringSubmatch(text, -1)
	var refs []FileRef
	seen := make(map[string]bool)

	for _, m := range matches {
		key := m[0]
		if seen[key] {
			continue
		}
		seen[key] = true

		line := 0
		fmt.Sscanf(m[2], "%d", &line)
		refs = append(refs, FileRef{Path: m[1], Line: line})
	}

	return refs
}

// SandboxValidator validates that file paths and commands stay within
// the worktree sandbox.
type SandboxValidator struct {
	worktreeDir string
}

// NewSandboxValidator creates a validator for the given worktree directory.
func NewSandboxValidator(worktreeDir string) *SandboxValidator {
	return &SandboxValidator{worktreeDir: worktreeDir}
}

// ValidatePath checks if a file path is within the worktree.
func (v *SandboxValidator) ValidatePath(path string) error {
	// Reject absolute paths that don't start with the worktree
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, v.worktreeDir) {
		return fmt.Errorf("path %q is outside the worktree %q", path, v.worktreeDir)
	}

	// Reject directory traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("path %q contains directory traversal", path)
	}

	return nil
}

// ValidateCommand checks if a shell command is allowed within the sandbox.
// Returns nil if allowed, error with reason if blocked.
func (v *SandboxValidator) ValidateCommand(command string) error {
	lower := strings.ToLower(command)

	// Dangerous literal patterns
	dangerous := []string{
		"rm -rf /",
		"sudo",
		"chmod 777",
		"eval",
		"> /dev/",
	}

	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return fmt.Errorf("command contains dangerous pattern %q", d)
		}
	}

	// Detect pipe-to-shell: anything piped into sh, bash, zsh, etc.
	// This catches "curl ... | sh", "wget ... | bash", etc.
	if pipeIdx := strings.Index(lower, "|"); pipeIdx >= 0 {
		after := strings.TrimSpace(lower[pipeIdx+1:])
		shells := []string{"sh", "bash", "zsh", "dash"}
		for _, sh := range shells {
			if after == sh || strings.HasPrefix(after, sh+" ") {
				return fmt.Errorf("command pipes into shell %q", sh)
			}
		}
	}

	return nil
}

// PRDescription generates a PR description from the plan and implementation context.
func PRDescription(plan string, filesChanged []string) string {
	var b strings.Builder

	b.WriteString("## Summary\n\n")

	// Extract first paragraph of the plan as summary
	paragraphs := strings.SplitN(plan, "\n\n", 2)
	if len(paragraphs) > 0 {
		b.WriteString(paragraphs[0])
	}

	b.WriteString("\n\n## Changes\n\n")
	for _, f := range filesChanged {
		b.WriteString(fmt.Sprintf("- `%s`\n", f))
	}

	b.WriteString("\n---\n*Generated by CodeButler*\n")
	return b.String()
}
