package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockRunner records commands and returns preconfigured responses.
type mockRunner struct {
	calls   []mockCall
	results map[string]mockResult
}

type mockCall struct {
	Dir  string
	Name string
	Args []string
}

type mockResult struct {
	Output string
	Err    error
}

func (m *mockRunner) run(_ context.Context, dir, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	m.calls = append(m.calls, mockCall{Dir: dir, Name: name, Args: args})

	if r, ok := m.results[key]; ok {
		return r.Output, r.Err
	}
	return "", nil
}

func TestBranchSlug(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"implement auth", "codebutler/implement-auth"},
		{"Fix Bug #123", "codebutler/fix-bug-123"},
		{"add user authentication module", "codebutler/add-user-authentication-module"},
		{"  spaces  ", "codebutler/spaces"},
		{"special!@#chars$%", "codebutler/special-chars"},
		{"UPPER CASE", "codebutler/upper-case"},
		{"already-kebab-case", "codebutler/already-kebab-case"},
		{strings.Repeat("very-long-", 20), "codebutler/" + strings.Repeat("very-long-", 5)[:50]},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := BranchSlug(tt.desc)
			if !strings.HasPrefix(got, "codebutler/") {
				t.Errorf("expected prefix 'codebutler/', got %q", got)
			}
			if len(got) > 62 { // "codebutler/" (11) + 50 max slug
				t.Errorf("slug too long: %d chars, %q", len(got), got)
			}
		})
	}
}

func TestManager_Create(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")

	runner := &mockRunner{results: make(map[string]mockResult)}

	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	path, err := m.Create(context.Background(), "codebutler/test-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(basePath, "codebutler/test-feature")
	if path != expected {
		t.Errorf("expected path %q, got %q", expected, path)
	}

	// Should have called git worktree add
	if len(runner.calls) == 0 {
		t.Error("expected at least one git command")
	}
}

func TestManager_Create_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")

	// Create the directory to simulate existing worktree
	worktreePath := filepath.Join(basePath, "codebutler/existing")
	os.MkdirAll(worktreePath, 0o755)

	runner := &mockRunner{results: make(map[string]mockResult)}
	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	path, err := m.Create(context.Background(), "codebutler/existing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path != worktreePath {
		t.Errorf("expected %q, got %q", worktreePath, path)
	}

	// Should not have called git (worktree already exists)
	if len(runner.calls) != 0 {
		t.Errorf("expected no git calls for existing worktree, got %d", len(runner.calls))
	}
}

func TestManager_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")

	runner := &mockRunner{results: make(map[string]mockResult)}
	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	err := m.Remove(context.Background(), "codebutler/old-feature", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called git worktree remove, git branch -D, git push --delete
	hasWorktreeRemove := false
	hasBranchDelete := false
	hasPushDelete := false

	for _, call := range runner.calls {
		args := strings.Join(call.Args, " ")
		if strings.Contains(args, "worktree remove") {
			hasWorktreeRemove = true
		}
		if strings.Contains(args, "branch -D") {
			hasBranchDelete = true
		}
		if strings.Contains(args, "push origin --delete") {
			hasPushDelete = true
		}
	}

	if !hasWorktreeRemove {
		t.Error("expected git worktree remove call")
	}
	if !hasBranchDelete {
		t.Error("expected git branch -D call")
	}
	if !hasPushDelete {
		t.Error("expected git push origin --delete call")
	}
}

func TestManager_Remove_NoRemoteDelete(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")

	runner := &mockRunner{results: make(map[string]mockResult)}
	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	err := m.Remove(context.Background(), "codebutler/feature", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, call := range runner.calls {
		if call.Name == "git" && len(call.Args) > 0 && call.Args[0] == "push" {
			t.Error("should not push --delete when deleteRemote is false")
		}
	}
}

func TestManager_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")

	m := NewManager(tmpDir, basePath)

	// Does not exist yet
	if m.Exists("codebutler/nonexistent") {
		t.Error("expected worktree to not exist")
	}

	// Create it
	os.MkdirAll(filepath.Join(basePath, "codebutler/existing"), 0o755)
	if !m.Exists("codebutler/existing") {
		t.Error("expected worktree to exist")
	}
}

func TestManager_Path(t *testing.T) {
	m := NewManager("/repo", "/repo/.codebutler/branches")
	path := m.Path("codebutler/feature")
	expected := "/repo/.codebutler/branches/codebutler/feature"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestParseWorktreeList(t *testing.T) {
	output := `worktree /repo
HEAD abc123
branch refs/heads/main

worktree /repo/.codebutler/branches/codebutler/feature-a
HEAD def456
branch refs/heads/codebutler/feature-a

worktree /repo/.codebutler/branches/codebutler/feature-b
HEAD ghi789
branch refs/heads/codebutler/feature-b
`

	results := parseWorktreeList(output, "/repo/.codebutler/branches")

	if len(results) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(results))
	}

	if results[0].Branch != "codebutler/feature-a" {
		t.Errorf("expected branch 'codebutler/feature-a', got %q", results[0].Branch)
	}
	if results[1].Branch != "codebutler/feature-b" {
		t.Errorf("expected branch 'codebutler/feature-b', got %q", results[1].Branch)
	}
}

func TestParseWorktreeList_Empty(t *testing.T) {
	results := parseWorktreeList("", "/base")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestManager_Init_GoPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")
	worktreePath := filepath.Join(basePath, "codebutler/go-project")
	os.MkdirAll(worktreePath, 0o755)

	// Create go.mod to signal Go platform
	os.WriteFile(filepath.Join(worktreePath, "go.mod"), []byte("module test"), 0o644)

	runner := &mockRunner{results: make(map[string]mockResult)}
	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	err := m.Init(context.Background(), "codebutler/go-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Go projects need no init commands
	if len(runner.calls) != 0 {
		t.Errorf("expected no commands for Go platform, got %d", len(runner.calls))
	}
}

func TestManager_Init_NodePlatform(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")
	worktreePath := filepath.Join(basePath, "codebutler/node-project")
	os.MkdirAll(worktreePath, 0o755)

	// Create package.json to signal Node platform
	os.WriteFile(filepath.Join(worktreePath, "package.json"), []byte("{}"), 0o644)

	runner := &mockRunner{results: make(map[string]mockResult)}
	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	err := m.Init(context.Background(), "codebutler/node-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have run npm ci
	if len(runner.calls) != 1 || runner.calls[0].Name != "npm" {
		t.Errorf("expected npm ci call, got %v", runner.calls)
	}
}

func TestManager_Init_UnknownPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")
	worktreePath := filepath.Join(basePath, "codebutler/unknown")
	os.MkdirAll(worktreePath, 0o755)

	runner := &mockRunner{results: make(map[string]mockResult)}
	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	err := m.Init(context.Background(), "codebutler/unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No commands for unknown platform
	if len(runner.calls) != 0 {
		t.Errorf("expected no commands for unknown platform, got %d", len(runner.calls))
	}
}

func TestManager_Create_GitError(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "branches")

	runner := &mockRunner{results: map[string]mockResult{
		"git worktree add -b codebutler/fail " + filepath.Join(basePath, "codebutler/fail"): {
			Output: "fatal: error",
			Err:    fmt.Errorf("exit 128"),
		},
		"git worktree add " + filepath.Join(basePath, "codebutler/fail") + " codebutler/fail": {
			Output: "fatal: error",
			Err:    fmt.Errorf("exit 128"),
		},
	}}

	m := NewManager(tmpDir, basePath, WithCommandRunner(runner.run))

	_, err := m.Create(context.Background(), "codebutler/fail")
	if err == nil {
		t.Error("expected error on git failure")
	}
}
