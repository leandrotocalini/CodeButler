// Package conflicts provides file and directory overlap detection between
// active threads and merge coordination for parallel development branches.
package conflicts

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ThreadFiles tracks which files a thread has modified.
type ThreadFiles struct {
	ThreadID string   `json:"thread_id"`
	Branch   string   `json:"branch"`
	Files    []string `json:"files"` // relative paths from repo root
}

// Overlap represents a conflict between two threads.
type Overlap struct {
	Type     OverlapType `json:"type"`      // file, directory, or semantic
	ThreadA  string      `json:"thread_a"`  // first thread ID
	ThreadB  string      `json:"thread_b"`  // second thread ID
	BranchA  string      `json:"branch_a"`  // first branch name
	BranchB  string      `json:"branch_b"`  // second branch name
	Path     string      `json:"path"`      // overlapping file or directory path
	Severity string      `json:"severity"`  // "high", "medium", "low"
	Detail   string      `json:"detail"`    // human-readable description
}

// OverlapType classifies the kind of overlap.
type OverlapType string

const (
	FileOverlap      OverlapType = "file"
	DirectoryOverlap OverlapType = "directory"
	SemanticOverlap  OverlapType = "semantic"
)

// MergeOrder represents the suggested merge order for threads.
type MergeOrder struct {
	ThreadID    string `json:"thread_id"`
	Branch      string `json:"branch"`
	FileCount   int    `json:"file_count"`
	Priority    int    `json:"priority"`     // lower = merge first
	NeedsRebase bool   `json:"needs_rebase"` // true if an earlier merge changes shared files
}

// Detector detects conflicts between active threads. Thread-safe.
type Detector struct {
	mu      sync.RWMutex
	threads map[string]*ThreadFiles // thread ID → files
}

// NewDetector creates a new conflict detector.
func NewDetector() *Detector {
	return &Detector{
		threads: make(map[string]*ThreadFiles),
	}
}

// Register records the files modified by a thread.
func (d *Detector) Register(tf ThreadFiles) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Normalize paths
	normalized := make([]string, len(tf.Files))
	for i, f := range tf.Files {
		normalized[i] = filepath.Clean(f)
	}
	tf.Files = normalized
	d.threads[tf.ThreadID] = &tf
}

// Update replaces the file list for an existing thread.
func (d *Detector) Update(threadID string, files []string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if tf, ok := d.threads[threadID]; ok {
		normalized := make([]string, len(files))
		for i, f := range files {
			normalized[i] = filepath.Clean(f)
		}
		tf.Files = normalized
	}
}

// Unregister removes a thread from tracking.
func (d *Detector) Unregister(threadID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.threads, threadID)
}

// DetectOverlaps finds all file and directory overlaps between active threads.
func (d *Detector) DetectOverlaps() []Overlap {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var overlaps []Overlap

	// Collect thread IDs for deterministic iteration
	ids := make([]string, 0, len(d.threads))
	for id := range d.threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Compare each pair of threads
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a := d.threads[ids[i]]
			b := d.threads[ids[j]]
			overlaps = append(overlaps, detectPairOverlaps(a, b)...)
		}
	}

	return overlaps
}

// DetectForThread finds overlaps for a specific thread against all others.
func (d *Detector) DetectForThread(threadID string) []Overlap {
	d.mu.RLock()
	defer d.mu.RUnlock()

	target, ok := d.threads[threadID]
	if !ok {
		return nil
	}

	var overlaps []Overlap
	for id, tf := range d.threads {
		if id == threadID {
			continue
		}
		overlaps = append(overlaps, detectPairOverlaps(target, tf)...)
	}

	return overlaps
}

// SuggestMergeOrder returns threads ordered by merge priority (smallest first).
func (d *Detector) SuggestMergeOrder() []MergeOrder {
	d.mu.RLock()
	defer d.mu.RUnlock()

	orders := make([]MergeOrder, 0, len(d.threads))
	for _, tf := range d.threads {
		orders = append(orders, MergeOrder{
			ThreadID:  tf.ThreadID,
			Branch:    tf.Branch,
			FileCount: len(tf.Files),
		})
	}

	// Sort by file count (smallest first)
	sort.Slice(orders, func(i, j int) bool {
		return orders[i].FileCount < orders[j].FileCount
	})

	// Assign priority and detect rebase needs
	overlaps := d.detectOverlapsUnlocked()
	overlapSet := make(map[string]map[string]bool) // threadA → threadB → true
	for _, o := range overlaps {
		if overlapSet[o.ThreadA] == nil {
			overlapSet[o.ThreadA] = make(map[string]bool)
		}
		if overlapSet[o.ThreadB] == nil {
			overlapSet[o.ThreadB] = make(map[string]bool)
		}
		overlapSet[o.ThreadA][o.ThreadB] = true
		overlapSet[o.ThreadB][o.ThreadA] = true
	}

	for i := range orders {
		orders[i].Priority = i + 1

		// Check if any earlier-priority thread overlaps with this one
		for j := 0; j < i; j++ {
			if overlapSet[orders[i].ThreadID] != nil && overlapSet[orders[i].ThreadID][orders[j].ThreadID] {
				orders[i].NeedsRebase = true
				break
			}
		}
	}

	return orders
}

// ActiveThreads returns the number of tracked threads.
func (d *Detector) ActiveThreads() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.threads)
}

// detectOverlapsUnlocked is the internal implementation without locking.
func (d *Detector) detectOverlapsUnlocked() []Overlap {
	var overlaps []Overlap

	ids := make([]string, 0, len(d.threads))
	for id := range d.threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a := d.threads[ids[i]]
			b := d.threads[ids[j]]
			overlaps = append(overlaps, detectPairOverlaps(a, b)...)
		}
	}

	return overlaps
}

// detectPairOverlaps finds overlaps between two threads.
func detectPairOverlaps(a, b *ThreadFiles) []Overlap {
	var overlaps []Overlap

	// Build file set for thread B
	bFiles := make(map[string]bool, len(b.Files))
	for _, f := range b.Files {
		bFiles[f] = true
	}

	// File overlap: exact same file modified by both
	for _, f := range a.Files {
		if bFiles[f] {
			overlaps = append(overlaps, Overlap{
				Type:     FileOverlap,
				ThreadA:  a.ThreadID,
				ThreadB:  b.ThreadID,
				BranchA:  a.Branch,
				BranchB:  b.Branch,
				Path:     f,
				Severity: "high",
				Detail:   fmt.Sprintf("file %s modified by both threads", f),
			})
		}
	}

	// Directory overlap: files in the same directory
	aDirs := extractDirs(a.Files)
	bDirs := extractDirs(b.Files)

	for dir := range aDirs {
		if bDirs[dir] {
			// Only report if no file overlap already covers this
			if !hasFileOverlapInDir(overlaps, dir) {
				overlaps = append(overlaps, Overlap{
					Type:     DirectoryOverlap,
					ThreadA:  a.ThreadID,
					ThreadB:  b.ThreadID,
					BranchA:  a.Branch,
					BranchB:  b.Branch,
					Path:     dir,
					Severity: "medium",
					Detail:   fmt.Sprintf("directory %s has changes from both threads", dir),
				})
			}
		}
	}

	return overlaps
}

// extractDirs returns the set of directories containing the given files.
func extractDirs(files []string) map[string]bool {
	dirs := make(map[string]bool)
	for _, f := range files {
		dir := filepath.Dir(f)
		if dir != "." {
			dirs[dir] = true
		}
	}
	return dirs
}

// hasFileOverlapInDir checks if there's already a file overlap in the given directory.
func hasFileOverlapInDir(overlaps []Overlap, dir string) bool {
	for _, o := range overlaps {
		if o.Type == FileOverlap && filepath.Dir(o.Path) == dir {
			return true
		}
	}
	return false
}

// AddSemanticOverlap adds a PM-driven semantic overlap to the overlap list.
func AddSemanticOverlap(overlaps []Overlap, threadA, threadB, branchA, branchB, description string) []Overlap {
	return append(overlaps, Overlap{
		Type:     SemanticOverlap,
		ThreadA:  threadA,
		ThreadB:  threadB,
		BranchA:  branchA,
		BranchB:  branchB,
		Severity: "medium",
		Detail:   description,
	})
}

// FormatOverlaps creates a human-readable summary of overlaps.
func FormatOverlaps(overlaps []Overlap) string {
	if len(overlaps) == 0 {
		return "No conflicts detected between active threads."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Conflict Report (%d overlaps)\n\n", len(overlaps)))

	// Group by severity
	high := filterBySeverity(overlaps, "high")
	medium := filterBySeverity(overlaps, "medium")
	low := filterBySeverity(overlaps, "low")

	if len(high) > 0 {
		b.WriteString("### High Severity\n\n")
		for _, o := range high {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s ↔ %s — %s\n",
				o.Type, o.Path, o.BranchA, o.BranchB, o.Detail))
		}
		b.WriteString("\n")
	}

	if len(medium) > 0 {
		b.WriteString("### Medium Severity\n\n")
		for _, o := range medium {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s ↔ %s — %s\n",
				o.Type, o.Path, o.BranchA, o.BranchB, o.Detail))
		}
		b.WriteString("\n")
	}

	if len(low) > 0 {
		b.WriteString("### Low Severity\n\n")
		for _, o := range low {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s ↔ %s — %s\n",
				o.Type, o.Path, o.BranchA, o.BranchB, o.Detail))
		}
	}

	return b.String()
}

// FormatMergeOrder creates a human-readable merge order suggestion.
func FormatMergeOrder(orders []MergeOrder) string {
	if len(orders) == 0 {
		return "No active threads to merge."
	}

	var b strings.Builder
	b.WriteString("## Suggested Merge Order\n\n")
	b.WriteString("| Priority | Branch | Files | Needs Rebase |\n")
	b.WriteString("|----------|--------|-------|--------------|\n")

	for _, o := range orders {
		rebase := "no"
		if o.NeedsRebase {
			rebase = "yes"
		}
		b.WriteString(fmt.Sprintf("| %d | %s | %d | %s |\n",
			o.Priority, o.Branch, o.FileCount, rebase))
	}

	return b.String()
}

func filterBySeverity(overlaps []Overlap, severity string) []Overlap {
	var filtered []Overlap
	for _, o := range overlaps {
		if o.Severity == severity {
			filtered = append(filtered, o)
		}
	}
	return filtered
}
