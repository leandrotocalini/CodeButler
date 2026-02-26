package worktree

import (
	"context"
	"log/slog"
	"testing"
)

func TestRecovery_ThreadGone_CleanedUp(t *testing.T) {
	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/gone", Branch: "codebutler/gone"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		active: map[string]bool{
			"C1" + "T1": false, // thread gone
		},
	}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/gone", ChannelID: "C1", ThreadTS: "T1"},
		},
	}

	r := NewRecoveryHandler(mgr, threads, store, WithRecoveryLogger(slog.Default()))

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WorktreesFound != 1 {
		t.Errorf("expected 1 worktree found, got %d", result.WorktreesFound)
	}
	if result.CleanedUp != 1 {
		t.Errorf("expected 1 cleaned up, got %d", result.CleanedUp)
	}
	if len(store.removed) != 1 || store.removed[0] != "codebutler/gone" {
		t.Errorf("expected mapping removal for gone, got: %v", store.removed)
	}
}

func TestRecovery_ThreadActive_LeftAlone(t *testing.T) {
	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/active", Branch: "codebutler/active"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		active: map[string]bool{
			"C1" + "T1": true, // thread exists
		},
	}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/active", ChannelID: "C1", ThreadTS: "T1"},
		},
	}

	r := NewRecoveryHandler(mgr, threads, store, WithRecoveryLogger(slog.Default()))

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CleanedUp != 0 {
		t.Errorf("expected 0 cleaned up, got %d", result.CleanedUp)
	}
	if len(store.removed) != 0 {
		t.Errorf("expected no removals, got: %v", store.removed)
	}
}

func TestRecovery_NoMapping_Orphaned(t *testing.T) {
	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/orphan", Branch: "codebutler/orphan"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{}
	store := &mockMappingStore{mappings: nil}

	r := NewRecoveryHandler(mgr, threads, store, WithRecoveryLogger(slog.Default()))

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Orphaned != 1 {
		t.Errorf("expected 1 orphaned, got %d", result.Orphaned)
	}
	if result.CleanedUp != 0 {
		t.Errorf("expected 0 cleaned up, got %d", result.CleanedUp)
	}
}

func TestRecovery_Mixed(t *testing.T) {
	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/active", Branch: "codebutler/active"},
		{Path: "/repo/.codebutler/branches/codebutler/gone", Branch: "codebutler/gone"},
		{Path: "/repo/.codebutler/branches/codebutler/orphan", Branch: "codebutler/orphan"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		active: map[string]bool{
			"C1" + "T1": true,  // active
			"C2" + "T2": false, // gone
		},
	}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/active", ChannelID: "C1", ThreadTS: "T1"},
			{Branch: "codebutler/gone", ChannelID: "C2", ThreadTS: "T2"},
			// orphan has no mapping
		},
	}

	r := NewRecoveryHandler(mgr, threads, store, WithRecoveryLogger(slog.Default()))

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WorktreesFound != 3 {
		t.Errorf("expected 3 worktrees, got %d", result.WorktreesFound)
	}
	if result.CleanedUp != 1 {
		t.Errorf("expected 1 cleaned up, got %d", result.CleanedUp)
	}
	if result.Orphaned != 1 {
		t.Errorf("expected 1 orphaned, got %d", result.Orphaned)
	}
}

func TestRecovery_EmptyState(t *testing.T) {
	mgr := newMockManager(nil)
	threads := &mockThreadChecker{}
	store := &mockMappingStore{}

	r := NewRecoveryHandler(mgr, threads, store, WithRecoveryLogger(slog.Default()))

	result, err := r.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WorktreesFound != 0 || result.CleanedUp != 0 || result.Orphaned != 0 {
		t.Errorf("expected all zeros, got: %+v", result)
	}
}
