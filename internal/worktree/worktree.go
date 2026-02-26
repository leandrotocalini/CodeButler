package worktree

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager handles git worktree lifecycle: create, initialize, remove.
type Manager struct {
	// repoRoot is the absolute path to the main repository.
	repoRoot string
	// basePath is where worktrees are stored (typically .codebutler/branches/).
	basePath string
	logger   *slog.Logger
	// runCmd is injectable for testing (defaults to exec.CommandContext).
	runCmd CommandRunner
}

// CommandRunner abstracts command execution for testing.
type CommandRunner func(ctx context.Context, dir, name string, args ...string) (string, error)

// ManagerOption configures the worktree manager.
type ManagerOption func(*Manager)

// WithWorktreeLogger sets the logger.
func WithWorktreeLogger(l *slog.Logger) ManagerOption {
	return func(m *Manager) {
		m.logger = l
	}
}

// WithCommandRunner sets a custom command runner (for testing).
func WithCommandRunner(r CommandRunner) ManagerOption {
	return func(m *Manager) {
		m.runCmd = r
	}
}

// NewManager creates a new worktree manager.
// repoRoot is the path to the main git repository.
// basePath is the directory for worktrees (e.g., ".codebutler/branches").
func NewManager(repoRoot, basePath string, opts ...ManagerOption) *Manager {
	m := &Manager{
		repoRoot: repoRoot,
		basePath: basePath,
		logger:   slog.Default(),
		runCmd:   defaultCommandRunner,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// defaultCommandRunner runs a command and returns its combined output.
func defaultCommandRunner(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Create creates a new git worktree with the given branch name.
// Branch naming convention: codebutler/<slug>
// Path: basePath/<branchName>/
func (m *Manager) Create(ctx context.Context, branchName string) (string, error) {
	worktreePath := filepath.Join(m.basePath, branchName)

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		m.logger.Info("worktree already exists", "branch", branchName, "path", worktreePath)
		return worktreePath, nil
	}

	// Ensure base directory exists
	if err := os.MkdirAll(m.basePath, 0o755); err != nil {
		return "", fmt.Errorf("create base directory: %w", err)
	}

	// Create the worktree with a new branch
	out, err := m.runCmd(ctx, m.repoRoot, "git", "worktree", "add", "-b", branchName, worktreePath)
	if err != nil {
		// Branch might already exist — try without -b
		out, err = m.runCmd(ctx, m.repoRoot, "git", "worktree", "add", worktreePath, branchName)
		if err != nil {
			return "", fmt.Errorf("git worktree add: %s: %w", out, err)
		}
	}

	m.logger.Info("worktree created", "branch", branchName, "path", worktreePath)
	return worktreePath, nil
}

// Remove removes a git worktree and optionally deletes the remote branch.
func (m *Manager) Remove(ctx context.Context, branchName string, deleteRemote bool) error {
	worktreePath := filepath.Join(m.basePath, branchName)

	// Remove the worktree
	out, err := m.runCmd(ctx, m.repoRoot, "git", "worktree", "remove", "--force", worktreePath)
	if err != nil {
		// Worktree might already be gone — try manual cleanup
		if removeErr := os.RemoveAll(worktreePath); removeErr != nil {
			m.logger.Warn("manual worktree cleanup failed", "path", worktreePath, "err", removeErr)
		}
		// Prune stale worktree entries
		m.runCmd(ctx, m.repoRoot, "git", "worktree", "prune")
		m.logger.Warn("force-removed worktree", "branch", branchName, "git_err", out)
	}

	// Delete local branch
	m.runCmd(ctx, m.repoRoot, "git", "branch", "-D", branchName)

	// Delete remote branch if requested
	if deleteRemote {
		out, err := m.runCmd(ctx, m.repoRoot, "git", "push", "origin", "--delete", branchName)
		if err != nil {
			m.logger.Warn("failed to delete remote branch", "branch", branchName, "err", out)
			// Not fatal — remote branch might not exist
		}
	}

	m.logger.Info("worktree removed", "branch", branchName, "deleteRemote", deleteRemote)
	return nil
}

// List returns all worktree branches managed under basePath.
func (m *Manager) List(ctx context.Context) ([]WorktreeInfo, error) {
	out, err := m.runCmd(ctx, m.repoRoot, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	return parseWorktreeList(out, m.basePath), nil
}

// Exists checks if a worktree for the given branch exists.
func (m *Manager) Exists(branchName string) bool {
	worktreePath := filepath.Join(m.basePath, branchName)
	_, err := os.Stat(worktreePath)
	return err == nil
}

// Path returns the filesystem path for a worktree.
func (m *Manager) Path(branchName string) string {
	return filepath.Join(m.basePath, branchName)
}

// Init runs per-platform initialization in the worktree directory.
func (m *Manager) Init(ctx context.Context, branchName string) error {
	worktreePath := filepath.Join(m.basePath, branchName)

	platform := DetectPlatform(worktreePath)
	if platform == PlatformUnknown {
		m.logger.Info("no platform detected, skipping init", "branch", branchName)
		return nil
	}

	m.logger.Info("initializing worktree", "branch", branchName, "platform", platform)
	return m.initPlatform(ctx, worktreePath, platform)
}

// initPlatform runs platform-specific initialization.
func (m *Manager) initPlatform(ctx context.Context, dir string, platform Platform) error {
	switch platform {
	case PlatformGo:
		// Go: nothing needed (go mod download is implicit)
		return nil
	case PlatformNode:
		out, err := m.runCmd(ctx, dir, "npm", "ci")
		if err != nil {
			return fmt.Errorf("npm ci: %s: %w", out, err)
		}
	case PlatformPython:
		// Create venv and install dependencies
		out, err := m.runCmd(ctx, dir, "python3", "-m", "venv", ".venv")
		if err != nil {
			return fmt.Errorf("create venv: %s: %w", out, err)
		}
		pip := filepath.Join(dir, ".venv", "bin", "pip")
		if _, err := os.Stat(filepath.Join(dir, "requirements.txt")); err == nil {
			out, err = m.runCmd(ctx, dir, pip, "install", "-r", "requirements.txt")
			if err != nil {
				return fmt.Errorf("pip install: %s: %w", out, err)
			}
		}
	case PlatformRust:
		// Cargo: nothing needed (build is implicit)
		return nil
	}
	return nil
}

// WorktreeInfo holds information about a worktree.
type WorktreeInfo struct {
	Path   string
	Branch string
}

// parseWorktreeList parses the output of `git worktree list --porcelain`
// and filters to worktrees under basePath.
func parseWorktreeList(output, basePath string) []WorktreeInfo {
	var results []WorktreeInfo
	var current WorktreeInfo

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "worktree ") {
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "" && current.Path != "" {
			// End of entry
			if basePath == "" || strings.HasPrefix(current.Path, basePath) {
				results = append(results, current)
			}
			current = WorktreeInfo{}
		}
	}
	// Handle last entry (no trailing newline)
	if current.Path != "" {
		if basePath == "" || strings.HasPrefix(current.Path, basePath) {
			results = append(results, current)
		}
	}

	return results
}

// BranchSlug generates a branch name from a description.
// Convention: codebutler/<sanitized-slug>
func BranchSlug(description string) string {
	// Lowercase, replace spaces and special chars with hyphens
	slug := strings.ToLower(description)
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, slug)

	// Collapse multiple hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")

	// Truncate to reasonable length
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}

	return "codebutler/" + slug
}
