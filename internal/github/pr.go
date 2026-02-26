package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// PRInfo holds information about a pull request.
type PRInfo struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Branch string `json:"headRefName"`
}

// PRCreateInput holds parameters for creating a pull request.
type PRCreateInput struct {
	Title  string
	Body   string
	Base   string // base branch, e.g. "main"
	Head   string // head branch, e.g. "codebutler/feature-xyz"
	Draft  bool
}

// PREditInput holds parameters for editing a pull request.
type PREditInput struct {
	Number int
	Title  string // empty = no change
	Body   string // empty = no change
}

// GHOps provides GitHub CLI operations for pull requests.
type GHOps struct {
	dir    string
	logger *slog.Logger
	runCmd CommandRunner
}

// GHOpsOption configures GHOps.
type GHOpsOption func(*GHOps)

// WithGHLogger sets the logger.
func WithGHLogger(l *slog.Logger) GHOpsOption {
	return func(g *GHOps) {
		g.logger = l
	}
}

// WithGHCommandRunner sets a custom command runner.
func WithGHCommandRunner(r CommandRunner) GHOpsOption {
	return func(g *GHOps) {
		g.runCmd = r
	}
}

// NewGHOps creates a new GitHub operations instance for the given directory.
func NewGHOps(dir string, opts ...GHOpsOption) *GHOps {
	g := &GHOps{
		dir:    dir,
		logger: slog.Default(),
		runCmd: defaultRunner,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// PRExists checks if a PR already exists for the given head branch.
// Returns the PR info if found, nil otherwise.
// Idempotent: safe to call repeatedly.
func (g *GHOps) PRExists(ctx context.Context, head string) (*PRInfo, error) {
	out, err := g.runCmd(ctx, g.dir, "gh", "pr", "list",
		"--head", head,
		"--json", "number,url,title,state,headRefName",
		"--limit", "1",
	)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %s: %w", out, err)
	}

	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return nil, nil
	}

	var prs []PRInfo
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}

	if len(prs) == 0 {
		return nil, nil
	}

	return &prs[0], nil
}

// CreatePR creates a pull request.
// Idempotent: if a PR already exists for the head branch, returns existing PR info.
func (g *GHOps) CreatePR(ctx context.Context, input PRCreateInput) (*PRInfo, error) {
	// Check if PR already exists
	existing, err := g.PRExists(ctx, input.Head)
	if err != nil {
		return nil, fmt.Errorf("check existing PR: %w", err)
	}
	if existing != nil {
		g.logger.Info("PR already exists", "number", existing.Number, "url", existing.URL)
		return existing, nil
	}

	args := []string{"pr", "create",
		"--title", input.Title,
		"--body", input.Body,
		"--base", input.Base,
		"--head", input.Head,
	}
	if input.Draft {
		args = append(args, "--draft")
	}

	out, err := g.runCmd(ctx, g.dir, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("gh pr create: %s: %w", out, err)
	}

	// gh pr create returns the PR URL on success
	url := strings.TrimSpace(out)

	// Fetch full PR info
	pr, err := g.PRExists(ctx, input.Head)
	if err != nil {
		// Return minimal info with the URL we got
		g.logger.Warn("could not fetch PR info after create", "err", err)
		return &PRInfo{URL: url, Title: input.Title, Branch: input.Head}, nil
	}

	g.logger.Info("created PR", "number", pr.Number, "url", pr.URL)
	return pr, nil
}

// EditPR updates a pull request's title and/or body.
// Idempotent: applying the same edit twice is harmless.
func (g *GHOps) EditPR(ctx context.Context, input PREditInput) error {
	args := []string{"pr", "edit", fmt.Sprintf("%d", input.Number)}

	if input.Title != "" {
		args = append(args, "--title", input.Title)
	}
	if input.Body != "" {
		args = append(args, "--body", input.Body)
	}

	out, err := g.runCmd(ctx, g.dir, "gh", args...)
	if err != nil {
		return fmt.Errorf("gh pr edit: %s: %w", out, err)
	}

	g.logger.Info("edited PR", "number", input.Number)
	return nil
}

// MergePR merges a pull request using squash strategy.
// Idempotent: if already merged, returns nil.
func (g *GHOps) MergePR(ctx context.Context, number int) error {
	out, err := g.runCmd(ctx, g.dir, "gh", "pr", "merge",
		fmt.Sprintf("%d", number),
		"--squash",
		"--delete-branch",
	)
	if err != nil {
		// Check if already merged
		if strings.Contains(out, "already been merged") ||
			strings.Contains(out, "MERGED") {
			g.logger.Info("PR already merged", "number", number)
			return nil
		}
		return fmt.Errorf("gh pr merge: %s: %w", out, err)
	}

	g.logger.Info("merged PR", "number", number)
	return nil
}

// PRStatus returns the status of a pull request by number.
func (g *GHOps) PRStatus(ctx context.Context, number int) (*PRInfo, error) {
	out, err := g.runCmd(ctx, g.dir, "gh", "pr", "view",
		fmt.Sprintf("%d", number),
		"--json", "number,url,title,state,headRefName",
	)
	if err != nil {
		return nil, fmt.Errorf("gh pr view: %s: %w", out, err)
	}

	var pr PRInfo
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return nil, fmt.Errorf("parse pr view: %w", err)
	}

	return &pr, nil
}
