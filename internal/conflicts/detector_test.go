package conflicts

import (
	"strings"
	"sync"
	"testing"
)

func TestDetector_RegisterAndDetect(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{
		ThreadID: "T1", Branch: "codebutler/feature-a",
		Files: []string{"src/api/handler.go", "src/api/routes.go"},
	})
	d.Register(ThreadFiles{
		ThreadID: "T2", Branch: "codebutler/feature-b",
		Files: []string{"src/api/handler.go", "src/models/user.go"},
	})

	overlaps := d.DetectOverlaps()
	if len(overlaps) == 0 {
		t.Fatal("expected overlaps")
	}

	// Should find file overlap on handler.go
	found := false
	for _, o := range overlaps {
		if o.Type == FileOverlap && o.Path == "src/api/handler.go" {
			found = true
			if o.Severity != "high" {
				t.Error("file overlap should be high severity")
			}
		}
	}
	if !found {
		t.Error("expected file overlap on handler.go")
	}
}

func TestDetector_NoOverlap(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{
		ThreadID: "T1", Branch: "b1",
		Files: []string{"src/auth/login.go"},
	})
	d.Register(ThreadFiles{
		ThreadID: "T2", Branch: "b2",
		Files: []string{"src/billing/invoice.go"},
	})

	overlaps := d.DetectOverlaps()
	if len(overlaps) != 0 {
		t.Errorf("expected no overlaps, got %d", len(overlaps))
	}
}

func TestDetector_DirectoryOverlap(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{
		ThreadID: "T1", Branch: "b1",
		Files: []string{"src/api/users.go"},
	})
	d.Register(ThreadFiles{
		ThreadID: "T2", Branch: "b2",
		Files: []string{"src/api/products.go"},
	})

	overlaps := d.DetectOverlaps()

	hasDirOverlap := false
	for _, o := range overlaps {
		if o.Type == DirectoryOverlap && o.Path == "src/api" {
			hasDirOverlap = true
			if o.Severity != "medium" {
				t.Error("directory overlap should be medium severity")
			}
		}
	}
	if !hasDirOverlap {
		t.Error("expected directory overlap on src/api")
	}
}

func TestDetector_DirectoryOverlapSuppressedByFileOverlap(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{
		ThreadID: "T1", Branch: "b1",
		Files: []string{"src/api/handler.go"},
	})
	d.Register(ThreadFiles{
		ThreadID: "T2", Branch: "b2",
		Files: []string{"src/api/handler.go", "src/api/routes.go"},
	})

	overlaps := d.DetectOverlaps()

	// Should have file overlap but NOT a duplicate directory overlap
	dirCount := 0
	for _, o := range overlaps {
		if o.Type == DirectoryOverlap && o.Path == "src/api" {
			dirCount++
		}
	}
	if dirCount > 0 {
		t.Error("directory overlap should be suppressed when file overlap exists in same dir")
	}
}

func TestDetector_DetectForThread(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "b1", Files: []string{"a.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "b2", Files: []string{"a.go"}})
	d.Register(ThreadFiles{ThreadID: "T3", Branch: "b3", Files: []string{"b.go"}})

	overlaps := d.DetectForThread("T1")
	if len(overlaps) == 0 {
		t.Fatal("T1 should overlap with T2")
	}

	// Should not include T3 (no overlap)
	for _, o := range overlaps {
		if o.ThreadA == "T3" || o.ThreadB == "T3" {
			t.Error("T1 should not overlap with T3")
		}
	}
}

func TestDetector_DetectForThread_Missing(t *testing.T) {
	d := NewDetector()

	overlaps := d.DetectForThread("NONEXISTENT")
	if overlaps != nil {
		t.Error("expected nil for missing thread")
	}
}

func TestDetector_Update(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "b1", Files: []string{"a.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "b2", Files: []string{"a.go"}})

	// Initially overlaps
	if len(d.DetectOverlaps()) == 0 {
		t.Fatal("expected initial overlap")
	}

	// Update T2 to different file
	d.Update("T2", []string{"b.go"})

	overlaps := d.DetectOverlaps()
	fileOverlaps := 0
	for _, o := range overlaps {
		if o.Type == FileOverlap {
			fileOverlaps++
		}
	}
	if fileOverlaps > 0 {
		t.Error("overlap should be resolved after update")
	}
}

func TestDetector_Unregister(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "b1", Files: []string{"a.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "b2", Files: []string{"a.go"}})

	d.Unregister("T2")

	if d.ActiveThreads() != 1 {
		t.Errorf("expected 1 thread, got %d", d.ActiveThreads())
	}

	overlaps := d.DetectOverlaps()
	if len(overlaps) != 0 {
		t.Error("no overlaps after unregister")
	}
}

func TestDetector_SuggestMergeOrder(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "big-feature", Files: []string{"a.go", "b.go", "c.go", "d.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "small-fix", Files: []string{"e.go"}})
	d.Register(ThreadFiles{ThreadID: "T3", Branch: "medium-change", Files: []string{"f.go", "g.go"}})

	orders := d.SuggestMergeOrder()
	if len(orders) != 3 {
		t.Fatalf("expected 3 orders, got %d", len(orders))
	}

	// Smallest first
	if orders[0].Branch != "small-fix" {
		t.Errorf("first should be small-fix, got %s", orders[0].Branch)
	}
	if orders[0].Priority != 1 {
		t.Errorf("first priority should be 1, got %d", orders[0].Priority)
	}
}

func TestDetector_MergeOrder_NeedsRebase(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "small", Files: []string{"shared.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "big", Files: []string{"shared.go", "other.go", "more.go"}})

	orders := d.SuggestMergeOrder()

	// T1 (1 file) should be first, T2 (3 files) should need rebase
	if orders[0].NeedsRebase {
		t.Error("first to merge should not need rebase")
	}
	if !orders[1].NeedsRebase {
		t.Error("second should need rebase (shares files with first)")
	}
}

func TestDetector_ActiveThreads(t *testing.T) {
	d := NewDetector()

	if d.ActiveThreads() != 0 {
		t.Error("should start empty")
	}

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "b1", Files: []string{"a.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "b2", Files: []string{"b.go"}})

	if d.ActiveThreads() != 2 {
		t.Errorf("expected 2, got %d", d.ActiveThreads())
	}
}

func TestDetector_Concurrent(t *testing.T) {
	d := NewDetector()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Register(ThreadFiles{
				ThreadID: "T-concurrent",
				Branch:   "b",
				Files:    []string{"a.go"},
			})
			d.DetectOverlaps()
			d.DetectForThread("T-concurrent")
			d.ActiveThreads()
		}()
	}
	wg.Wait()
}

func TestAddSemanticOverlap(t *testing.T) {
	overlaps := []Overlap{}
	overlaps = AddSemanticOverlap(overlaps, "T1", "T2", "b1", "b2",
		"Both threads modify authentication logic")

	if len(overlaps) != 1 {
		t.Fatalf("expected 1 overlap, got %d", len(overlaps))
	}
	if overlaps[0].Type != SemanticOverlap {
		t.Error("should be semantic overlap")
	}
	if overlaps[0].Severity != "medium" {
		t.Error("semantic overlap should be medium severity")
	}
}

func TestFormatOverlaps_Empty(t *testing.T) {
	output := FormatOverlaps(nil)
	if !strings.Contains(output, "No conflicts") {
		t.Error("should indicate no conflicts")
	}
}

func TestFormatOverlaps_WithOverlaps(t *testing.T) {
	overlaps := []Overlap{
		{
			Type: FileOverlap, ThreadA: "T1", ThreadB: "T2",
			BranchA: "b1", BranchB: "b2",
			Path: "api/handler.go", Severity: "high",
			Detail: "file api/handler.go modified by both threads",
		},
		{
			Type: DirectoryOverlap, ThreadA: "T1", ThreadB: "T3",
			BranchA: "b1", BranchB: "b3",
			Path: "src/models", Severity: "medium",
			Detail: "directory src/models has changes from both threads",
		},
	}

	output := FormatOverlaps(overlaps)
	if !strings.Contains(output, "High Severity") {
		t.Error("should have high severity section")
	}
	if !strings.Contains(output, "Medium Severity") {
		t.Error("should have medium severity section")
	}
	if !strings.Contains(output, "handler.go") {
		t.Error("should mention file")
	}
}

func TestFormatMergeOrder_Empty(t *testing.T) {
	output := FormatMergeOrder(nil)
	if !strings.Contains(output, "No active threads") {
		t.Error("should indicate no threads")
	}
}

func TestFormatMergeOrder_WithOrders(t *testing.T) {
	orders := []MergeOrder{
		{ThreadID: "T1", Branch: "small-fix", FileCount: 1, Priority: 1},
		{ThreadID: "T2", Branch: "big-feature", FileCount: 10, Priority: 2, NeedsRebase: true},
	}

	output := FormatMergeOrder(orders)
	if !strings.Contains(output, "small-fix") {
		t.Error("should contain branch name")
	}
	if !strings.Contains(output, "yes") {
		t.Error("should show rebase needed")
	}
}

func TestDetector_ThreeWayOverlap(t *testing.T) {
	d := NewDetector()

	d.Register(ThreadFiles{ThreadID: "T1", Branch: "b1", Files: []string{"shared.go"}})
	d.Register(ThreadFiles{ThreadID: "T2", Branch: "b2", Files: []string{"shared.go"}})
	d.Register(ThreadFiles{ThreadID: "T3", Branch: "b3", Files: []string{"shared.go"}})

	overlaps := d.DetectOverlaps()

	// Should have 3 pairwise overlaps: T1-T2, T1-T3, T2-T3
	fileOverlaps := 0
	for _, o := range overlaps {
		if o.Type == FileOverlap {
			fileOverlaps++
		}
	}
	if fileOverlaps != 3 {
		t.Errorf("expected 3 pairwise file overlaps, got %d", fileOverlaps)
	}
}

func TestExtractDirs(t *testing.T) {
	files := []string{"src/api/handler.go", "src/api/routes.go", "main.go"}
	dirs := extractDirs(files)

	if !dirs["src/api"] {
		t.Error("should contain src/api")
	}
	if dirs["."] {
		t.Error("should not contain root dir '.'")
	}
}
