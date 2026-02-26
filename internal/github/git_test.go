package github

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
)

// mockRunner returns a CommandRunner that replays recorded outputs.
type mockCall struct {
	wantName string
	wantArgs []string
	out      string
	err      error
}

func newMockRunner(calls []mockCall) (CommandRunner, *int) {
	idx := new(int)
	return func(ctx context.Context, dir, name string, args ...string) (string, error) {
		if *idx >= len(calls) {
			return "", fmt.Errorf("unexpected call #%d: %s %v", *idx, name, args)
		}
		c := calls[*idx]
		*idx++
		return c.out, c.err
	}, idx
}

func TestGitOps_Commit_Success(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},                              // git add file1.go
		{out: "", err: nil},                              // git add file2.go
		{out: "", err: fmt.Errorf("exit status 1")},      // git diff --cached --quiet (changes exist)
		{out: "abc123", err: nil},                        // git commit
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Commit(context.Background(), []string{"file1.go", "file2.go"}, "test commit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitOps_Commit_NoChanges(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil}, // git add file.go
		{out: "", err: nil}, // git diff --cached --quiet (no changes)
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Commit(context.Background(), []string{"file.go"}, "test commit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitOps_Commit_AddFails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "fatal: not a git repository", err: fmt.Errorf("exit status 128")},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Commit(context.Background(), []string{"bad.go"}, "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !contains(got, "git add") {
		t.Errorf("error should mention git add, got: %s", got)
	}
}

func TestGitOps_Commit_CommitFails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},                             // git add
		{out: "", err: fmt.Errorf("exit status 1")},     // git diff --cached --quiet (changes exist)
		{out: "error: empty commit", err: fmt.Errorf("exit status 1")}, // git commit fails
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Commit(context.Background(), []string{"file.go"}, "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !contains(got, "git commit") {
		t.Errorf("error should mention git commit, got: %s", got)
	}
}

func TestGitOps_Push_Success(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "main", err: nil},    // git rev-parse --abbrev-ref HEAD
		{out: "", err: nil},        // git push -u origin main
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Push(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitOps_Push_AlreadyUpToDate(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "feature", err: nil},
		{out: "Everything up-to-date", err: fmt.Errorf("exit status 1")},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Push(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitOps_Push_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "feature", err: nil},
		{out: "error: failed to push", err: fmt.Errorf("exit status 1")},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Push(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitOps_Pull_Success(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "Already up to date.", err: nil},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Pull(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitOps_Pull_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "conflict", err: fmt.Errorf("exit status 1")},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	err := g.Pull(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitOps_HasChanges_True(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: " M file.go\n?? new.go", err: nil},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	has, err := g.HasChanges(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Fatal("expected changes")
	}
}

func TestGitOps_HasChanges_False(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	has, err := g.HasChanges(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Fatal("expected no changes")
	}
}

func TestGitOps_CurrentBranch(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "codebutler/my-feature", err: nil},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	branch, err := g.CurrentBranch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "codebutler/my-feature" {
		t.Fatalf("expected codebutler/my-feature, got %s", branch)
	}
}

func TestGitOps_CurrentBranch_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: fmt.Errorf("exit status 128")},
	})

	g := NewGitOps("/tmp/repo", WithGitCommandRunner(runner), WithGitLogger(slog.Default()))

	_, err := g.CurrentBranch(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
