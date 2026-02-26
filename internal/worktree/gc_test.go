package worktree

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockThreadChecker struct {
	lastActivity map[string]time.Time // key: channelID+threadTS
	active       map[string]bool
	err          error
}

func (m *mockThreadChecker) LastActivity(_ context.Context, channelID, threadTS string) (time.Time, error) {
	if m.err != nil {
		return time.Time{}, m.err
	}
	return m.lastActivity[channelID+threadTS], nil
}

func (m *mockThreadChecker) IsThreadActive(_ context.Context, channelID, threadTS string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.active[channelID+threadTS], nil
}

type mockPRChecker struct {
	openPRs map[string]bool // key: branch
	err     error
}

func (m *mockPRChecker) HasOpenPR(_ context.Context, head string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.openPRs[head], nil
}

type mockPhaseChecker struct {
	phases map[string]ThreadPhase // key: threadTS
	err    error
}

func (m *mockPhaseChecker) GetPhase(_ context.Context, threadID string) (ThreadPhase, error) {
	if m.err != nil {
		return PhaseUnknown, m.err
	}
	return m.phases[threadID], nil
}

type mockGCNotifier struct {
	warned []string // channelID+threadTS pairs
	err    error
}

func (m *mockGCNotifier) WarnInactive(_ context.Context, channelID, threadTS string) error {
	if m.err != nil {
		return m.err
	}
	m.warned = append(m.warned, channelID+threadTS)
	return nil
}

type mockMappingStore struct {
	mappings []WorktreeMapping
	removed  []string
	err      error
}

func (m *mockMappingStore) ListMappings(_ context.Context) ([]WorktreeMapping, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.mappings, nil
}

func (m *mockMappingStore) RemoveMapping(_ context.Context, branch string) error {
	m.removed = append(m.removed, branch)
	return nil
}

// mockWorktreeManager returns a Manager with a mock CommandRunner
// that reports given worktrees when List is called.
func newMockManager(worktrees []WorktreeInfo) *Manager {
	porcelain := ""
	for _, wt := range worktrees {
		porcelain += fmt.Sprintf("worktree %s\nbranch refs/heads/%s\n\n", wt.Path, wt.Branch)
	}
	runner := func(_ context.Context, _, name string, args ...string) (string, error) {
		// Handle "git worktree list --porcelain"
		if name == "git" && len(args) > 0 && args[0] == "worktree" && len(args) > 1 && args[1] == "list" {
			return porcelain, nil
		}
		// Handle remove/prune/branch/push silently
		return "", nil
	}
	return NewManager("/repo", "/repo/.codebutler/branches",
		WithCommandRunner(runner))
}

// --- Tests ---

func TestGC_OrphanDetected_Warned(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": now.Add(-72 * time.Hour), // 72h ago
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCLogger(slog.Default()),
		WithGCClock(func() time.Time { return now }),
	)

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.warned) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(notifier.warned))
	}
	if notifier.warned[0] != "C123T100" {
		t.Errorf("warned wrong thread: %s", notifier.warned[0])
	}
	if len(store.removed) != 0 {
		t.Error("should not have removed mapping yet (grace period)")
	}
}

func TestGC_OrphanCleaned_AfterGracePeriod(t *testing.T) {
	warnTime := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC) // 48h after warn

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": warnTime.Add(-96 * time.Hour), // very old
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCLogger(slog.Default()),
		WithGCClock(func() time.Time { return now }),
	)

	// Pre-set warned state (simulating a previous GC pass)
	gc.state.WarnedAt["codebutler/feat-a"] = warnTime

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.warned) != 0 {
		t.Error("should not warn again")
	}
	if len(store.removed) != 1 || store.removed[0] != "codebutler/feat-a" {
		t.Errorf("expected mapping removal for feat-a, got: %v", store.removed)
	}
}

func TestGC_NotOrphaned_ActiveThread(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": now.Add(-1 * time.Hour), // 1h ago — active
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.warned) != 0 {
		t.Error("should not warn active thread")
	}
}

func TestGC_NotOrphaned_CoderPhase(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": now.Add(-72 * time.Hour), // inactive
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseCoding}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.warned) != 0 {
		t.Error("should not warn thread in coder phase")
	}
}

func TestGC_NotOrphaned_OpenPR(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": now.Add(-72 * time.Hour),
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{"codebutler/feat-a": true}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.warned) != 0 {
		t.Error("should not warn thread with open PR")
	}
}

func TestGC_OrphanInGracePeriod_NotCleaned(t *testing.T) {
	warnTime := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC) // 2h after warn (< 24h grace)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": now.Add(-72 * time.Hour),
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	gc.state.WarnedAt["codebutler/feat-a"] = warnTime

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.warned) != 0 {
		t.Error("should not warn again during grace period")
	}
	if len(store.removed) != 0 {
		t.Error("should not clean during grace period")
	}
}

func TestGC_WarningReset_WhenActive(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)
	warnTime := now.Add(-1 * time.Hour)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/feat-a", Branch: "codebutler/feat-a"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C123" + "T100": now.Add(-1 * time.Hour), // became active
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T100": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/feat-a", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	gc.state.WarnedAt["codebutler/feat-a"] = warnTime

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warning should be reset
	if _, warned := gc.state.WarnedAt["codebutler/feat-a"]; warned {
		t.Error("warning should have been reset for active thread")
	}
}

func TestGC_MappingWithoutWorktree_Cleaned(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	// No local worktrees
	mgr := newMockManager(nil)

	threads := &mockThreadChecker{}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/stale", ChannelID: "C123", ThreadTS: "T100"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.removed) != 1 || store.removed[0] != "codebutler/stale" {
		t.Errorf("expected stale mapping removal, got: %v", store.removed)
	}
}

func TestGC_MultipleWorktrees(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	worktrees := []WorktreeInfo{
		{Path: "/repo/.codebutler/branches/codebutler/active", Branch: "codebutler/active"},
		{Path: "/repo/.codebutler/branches/codebutler/orphan", Branch: "codebutler/orphan"},
	}
	mgr := newMockManager(worktrees)

	threads := &mockThreadChecker{
		lastActivity: map[string]time.Time{
			"C1" + "T1": now.Add(-1 * time.Hour),  // active
			"C2" + "T2": now.Add(-72 * time.Hour), // orphan
		},
	}
	prs := &mockPRChecker{openPRs: map[string]bool{}}
	phases := &mockPhaseChecker{phases: map[string]ThreadPhase{"T1": PhaseDone, "T2": PhaseDone}}
	notifier := &mockGCNotifier{}
	store := &mockMappingStore{
		mappings: []WorktreeMapping{
			{Branch: "codebutler/active", ChannelID: "C1", ThreadTS: "T1"},
			{Branch: "codebutler/orphan", ChannelID: "C2", ThreadTS: "T2"},
		},
	}

	gc := NewGarbageCollector(mgr, threads, prs, phases, notifier, store,
		WithGCClock(func() time.Time { return now }),
	)

	err := gc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only orphan should be warned
	if len(notifier.warned) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(notifier.warned))
	}
	if notifier.warned[0] != "C2T2" {
		t.Errorf("warned wrong thread: %s", notifier.warned[0])
	}
}

func TestDefaultGCConfig(t *testing.T) {
	cfg := DefaultGCConfig()
	if cfg.Interval != 6*time.Hour {
		t.Errorf("expected 6h interval, got %v", cfg.Interval)
	}
	if cfg.InactivityTimeout != 48*time.Hour {
		t.Errorf("expected 48h inactivity timeout, got %v", cfg.InactivityTimeout)
	}
	if cfg.GracePeriod != 24*time.Hour {
		t.Errorf("expected 24h grace period, got %v", cfg.GracePeriod)
	}
}
