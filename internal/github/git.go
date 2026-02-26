package github

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// CommandRunner abstracts command execution for testing.
type CommandRunner func(ctx context.Context, dir, name string, args ...string) (string, error)

// defaultRunner runs commands via exec.CommandContext.
func defaultRunner(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// GitOps provides git operations for agent tools.
type GitOps struct {
	dir    string
	logger *slog.Logger
	runCmd CommandRunner
}

// GitOpsOption configures GitOps.
type GitOpsOption func(*GitOps)

// WithGitLogger sets the logger.
func WithGitLogger(l *slog.Logger) GitOpsOption {
	return func(g *GitOps) {
		g.logger = l
	}
}

// WithGitCommandRunner sets a custom command runner.
func WithGitCommandRunner(r CommandRunner) GitOpsOption {
	return func(g *GitOps) {
		g.runCmd = r
	}
}

// NewGitOps creates a new git operations instance for the given directory.
func NewGitOps(dir string, opts ...GitOpsOption) *GitOps {
	g := &GitOps{
		dir:    dir,
		logger: slog.Default(),
		runCmd: defaultRunner,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Commit stages the given files and creates a commit.
// Idempotent: if there are no changes to commit, returns nil.
func (g *GitOps) Commit(ctx context.Context, files []string, message string) error {
	// Stage files
	for _, f := range files {
		out, err := g.runCmd(ctx, g.dir, "git", "add", f)
		if err != nil {
			return fmt.Errorf("git add %s: %s: %w", f, out, err)
		}
	}

	// Check if there are staged changes
	out, err := g.runCmd(ctx, g.dir, "git", "diff", "--cached", "--quiet")
	if err == nil {
		// No changes staged — nothing to commit (idempotent)
		g.logger.Info("no changes to commit")
		return nil
	}

	// Create commit
	out, err = g.runCmd(ctx, g.dir, "git", "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit: %s: %w", out, err)
	}

	g.logger.Info("committed", "message", message)
	return nil
}

// Push pushes the current branch to the remote.
// Idempotent: if the remote is already up to date, returns nil.
func (g *GitOps) Push(ctx context.Context) error {
	// Get current branch
	branch, err := g.runCmd(ctx, g.dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	out, err := g.runCmd(ctx, g.dir, "git", "push", "-u", "origin", branch)
	if err != nil {
		// Check if already up to date
		if strings.Contains(out, "Everything up-to-date") {
			g.logger.Info("remote already up to date")
			return nil
		}
		return fmt.Errorf("git push: %s: %w", out, err)
	}

	g.logger.Info("pushed", "branch", branch)
	return nil
}

// Pull pulls the latest changes from the remote.
func (g *GitOps) Pull(ctx context.Context) error {
	out, err := g.runCmd(ctx, g.dir, "git", "pull", "--rebase")
	if err != nil {
		return fmt.Errorf("git pull: %s: %w", out, err)
	}
	g.logger.Info("pulled latest changes")
	return nil
}

// HasChanges checks if there are uncommitted changes in the working directory.
func (g *GitOps) HasChanges(ctx context.Context) (bool, error) {
	out, err := g.runCmd(ctx, g.dir, "git", "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// CurrentBranch returns the name of the current branch.
func (g *GitOps) CurrentBranch(ctx context.Context) (string, error) {
	out, err := g.runCmd(ctx, g.dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("get current branch: %w", err)
	}
	return out, nil
}
