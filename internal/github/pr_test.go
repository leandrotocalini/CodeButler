package github

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
)

func TestGHOps_PRExists_Found(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: `[{"number":42,"url":"https://github.com/org/repo/pull/42","title":"feat: add feature","state":"OPEN","headRefName":"codebutler/feat"}]`, err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.PRExists(context.Background(), "codebutler/feat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr == nil {
		t.Fatal("expected PR to exist")
	}
	if pr.Number != 42 {
		t.Errorf("expected PR #42, got #%d", pr.Number)
	}
	if pr.URL != "https://github.com/org/repo/pull/42" {
		t.Errorf("unexpected URL: %s", pr.URL)
	}
}

func TestGHOps_PRExists_NotFound(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "[]", err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.PRExists(context.Background(), "codebutler/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Fatal("expected no PR")
	}
}

func TestGHOps_PRExists_Empty(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.PRExists(context.Background(), "codebutler/empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Fatal("expected no PR")
	}
}

func TestGHOps_PRExists_Error(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "not authenticated", err: fmt.Errorf("exit status 1")},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	_, err := g.PRExists(context.Background(), "codebutler/feat")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGHOps_CreatePR_New(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		// PRExists check
		{out: "[]", err: nil},
		// gh pr create
		{out: "https://github.com/org/repo/pull/99", err: nil},
		// PRExists to get full info
		{out: `[{"number":99,"url":"https://github.com/org/repo/pull/99","title":"feat: new","state":"OPEN","headRefName":"codebutler/new"}]`, err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.CreatePR(context.Background(), PRCreateInput{
		Title: "feat: new",
		Body:  "description",
		Base:  "main",
		Head:  "codebutler/new",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != 99 {
		t.Errorf("expected PR #99, got #%d", pr.Number)
	}
}

func TestGHOps_CreatePR_AlreadyExists(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		// PRExists returns existing
		{out: `[{"number":42,"url":"https://github.com/org/repo/pull/42","title":"feat: existing","state":"OPEN","headRefName":"codebutler/feat"}]`, err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.CreatePR(context.Background(), PRCreateInput{
		Title: "feat: existing",
		Body:  "description",
		Base:  "main",
		Head:  "codebutler/feat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected existing PR #42, got #%d", pr.Number)
	}
}

func TestGHOps_CreatePR_Draft(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "[]", err: nil},
		{out: "https://github.com/org/repo/pull/100", err: nil},
		{out: `[{"number":100,"url":"https://github.com/org/repo/pull/100","title":"draft","state":"OPEN","headRefName":"codebutler/draft"}]`, err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.CreatePR(context.Background(), PRCreateInput{
		Title: "draft",
		Body:  "wip",
		Base:  "main",
		Head:  "codebutler/draft",
		Draft: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != 100 {
		t.Errorf("expected PR #100, got #%d", pr.Number)
	}
}

func TestGHOps_CreatePR_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "[]", err: nil},
		{out: "not authenticated", err: fmt.Errorf("exit status 1")},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	_, err := g.CreatePR(context.Background(), PRCreateInput{
		Title: "feat",
		Body:  "body",
		Base:  "main",
		Head:  "codebutler/feat",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGHOps_EditPR_Success(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	err := g.EditPR(context.Background(), PREditInput{
		Number: 42,
		Title:  "updated title",
		Body:   "updated body",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGHOps_EditPR_TitleOnly(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	err := g.EditPR(context.Background(), PREditInput{
		Number: 42,
		Title:  "new title",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGHOps_EditPR_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "not found", err: fmt.Errorf("exit status 1")},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	err := g.EditPR(context.Background(), PREditInput{Number: 999, Title: "nope"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGHOps_MergePR_Success(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "", err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	err := g.MergePR(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGHOps_MergePR_AlreadyMerged(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "already been merged", err: fmt.Errorf("exit status 1")},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	err := g.MergePR(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error for already merged: %v", err)
	}
}

func TestGHOps_MergePR_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "merge conflict", err: fmt.Errorf("exit status 1")},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	err := g.MergePR(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGHOps_PRStatus_Success(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: `{"number":42,"url":"https://github.com/org/repo/pull/42","title":"feat","state":"OPEN","headRefName":"codebutler/feat"}`, err: nil},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	pr, err := g.PRStatus(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.State != "OPEN" {
		t.Errorf("expected OPEN, got %s", pr.State)
	}
}

func TestGHOps_PRStatus_Fails(t *testing.T) {
	runner, _ := newMockRunner([]mockCall{
		{out: "not found", err: fmt.Errorf("exit status 1")},
	})

	g := NewGHOps("/tmp/repo", WithGHCommandRunner(runner), WithGHLogger(slog.Default()))

	_, err := g.PRStatus(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error")
	}
}
