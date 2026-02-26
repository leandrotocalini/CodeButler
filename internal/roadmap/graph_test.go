package roadmap

import (
	"testing"
)

func buildTestRoadmap() *Roadmap {
	r, _ := ParseString(`# Roadmap: Test

## 1. Auth system
- Status: done
- Depends on: —
- Acceptance criteria: auth

## 2. User profile API
- Status: in_progress
- Depends on: 1
- Acceptance criteria: profile api

## 3. Profile UI
- Status: pending
- Depends on: 1, 2
- Acceptance criteria: profile ui

## 4. Notification system
- Status: pending
- Depends on: —
- Acceptance criteria: notifications

## 5. Admin dashboard
- Status: pending
- Depends on: 2, 4
- Acceptance criteria: admin
`)
	return r
}

func TestBuildGraph_Unblocked(t *testing.T) {
	r := buildTestRoadmap()
	g := BuildGraph(r)

	unblocked := g.Unblocked()

	// Item 4 has no deps → unblocked
	// Item 3 depends on 1 (done) and 2 (in_progress) → NOT unblocked
	// Item 5 depends on 2 (in_progress) and 4 (pending) → NOT unblocked
	if len(unblocked) != 1 {
		t.Fatalf("expected 1 unblocked, got %d", len(unblocked))
	}
	if unblocked[0].Number != 4 {
		t.Errorf("expected item 4 unblocked, got %d", unblocked[0].Number)
	}
}

func TestBuildGraph_UnblockedAfterDone(t *testing.T) {
	r := buildTestRoadmap()
	// Mark item 2 as done
	r.SetStatus(2, StatusDone)

	g := BuildGraph(r)
	unblocked := g.Unblocked()

	// Item 3: deps 1 (done), 2 (done) → unblocked
	// Item 4: no deps → unblocked
	// Item 5: deps 2 (done), 4 (pending) → NOT unblocked
	if len(unblocked) != 2 {
		t.Fatalf("expected 2 unblocked, got %d", len(unblocked))
	}

	nums := make(map[int]bool)
	for _, u := range unblocked {
		nums[u.Number] = true
	}
	if !nums[3] {
		t.Error("expected item 3 unblocked")
	}
	if !nums[4] {
		t.Error("expected item 4 unblocked")
	}
}

func TestBuildGraph_NewlyUnblocked(t *testing.T) {
	r := buildTestRoadmap()
	// Mark item 2 as done
	r.SetStatus(2, StatusDone)

	g := BuildGraph(r)

	// Complete item 4 → check what becomes unblocked
	r.SetStatus(4, StatusDone)
	g = BuildGraph(r) // rebuild after status change

	newly := g.NewlyUnblocked(4)

	// Item 5 depends on 2 (done) and 4 (just done) → now unblocked
	if len(newly) != 1 {
		t.Fatalf("expected 1 newly unblocked, got %d", len(newly))
	}
	if newly[0].Number != 5 {
		t.Errorf("expected item 5 newly unblocked, got %d", newly[0].Number)
	}
}

func TestBuildGraph_Dependents(t *testing.T) {
	r := buildTestRoadmap()
	g := BuildGraph(r)

	// Item 1 has dependents: 2, 3
	deps := g.Dependents(1)
	if len(deps) != 2 {
		t.Errorf("expected 2 dependents of item 1, got %d", len(deps))
	}

	// Item 4 has dependent: 5
	deps = g.Dependents(4)
	if len(deps) != 1 {
		t.Errorf("expected 1 dependent of item 4, got %d", len(deps))
	}
}

func TestBuildGraph_NoCycle(t *testing.T) {
	r := buildTestRoadmap()
	g := BuildGraph(r)

	if g.HasCycle() {
		t.Error("expected no cycle in test roadmap")
	}
}

func TestBuildGraph_TopologicalOrder(t *testing.T) {
	r := buildTestRoadmap()
	g := BuildGraph(r)

	order, err := g.TopologicalOrder()
	if err != nil {
		t.Fatalf("topo order failed: %v", err)
	}
	if len(order) != 5 {
		t.Fatalf("expected 5 items in order, got %d", len(order))
	}

	// Verify: each item appears after all its dependencies
	position := make(map[int]int)
	for i, item := range order {
		position[item.Number] = i
	}

	for _, item := range order {
		for _, dep := range item.DependsOn {
			if position[dep] > position[item.Number] {
				t.Errorf("item %d appears before dependency %d", item.Number, dep)
			}
		}
	}
}

func TestBuildGraph_Stats(t *testing.T) {
	r := buildTestRoadmap()
	g := BuildGraph(r)

	stats := g.Stats()
	if stats.Total != 5 {
		t.Errorf("total: expected 5, got %d", stats.Total)
	}
	if stats.Done != 1 {
		t.Errorf("done: expected 1, got %d", stats.Done)
	}
	if stats.InProgress != 1 {
		t.Errorf("in_progress: expected 1, got %d", stats.InProgress)
	}
	if stats.Pending != 3 {
		t.Errorf("pending: expected 3, got %d", stats.Pending)
	}
}

func TestRoadmapStats_FormatProgress(t *testing.T) {
	stats := RoadmapStats{Total: 5, Done: 2, Pending: 2, InProgress: 1}
	s := stats.FormatProgress()

	if s != "2/5 done (40%) — 2 pending, 1 in progress, 0 blocked" {
		t.Errorf("format: got %q", s)
	}
}

func TestRoadmapStats_FormatProgress_Empty(t *testing.T) {
	stats := RoadmapStats{}
	if stats.FormatProgress() != "No items in roadmap" {
		t.Errorf("format: got %q", stats.FormatProgress())
	}
}

func TestBuildGraph_CriticalPath(t *testing.T) {
	r := buildTestRoadmap()
	g := BuildGraph(r)

	path := g.CriticalPath()

	// Longest chain: 1 → 2 → 3 (or 1 → 2 → 5 via 4)
	// 1 → 2 → 5 is length 3 (via dependency 2 and 4, but 5 depends on 2 AND 4)
	// Actually: 1→2→3 is length 3, 1→2→5 requires 4 too
	// Critical path should be at least 3 items
	if len(path) < 3 {
		t.Errorf("expected critical path of at least 3, got %d: %v", len(path), path)
	}
}

func TestBuildGraph_Empty(t *testing.T) {
	r := &Roadmap{}
	g := BuildGraph(r)

	if len(g.Unblocked()) != 0 {
		t.Error("expected 0 unblocked for empty roadmap")
	}
	if g.HasCycle() {
		t.Error("empty graph should not have cycle")
	}

	stats := g.Stats()
	if stats.Total != 0 {
		t.Errorf("expected 0 total, got %d", stats.Total)
	}
}
