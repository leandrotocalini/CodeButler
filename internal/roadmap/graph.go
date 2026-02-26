package roadmap

import "fmt"

// DependencyGraph represents the dependency relationships between roadmap items.
type DependencyGraph struct {
	// adjacency maps item number → items that depend on it (downstream).
	adjacency map[int][]int
	// items maps item number → Item for quick lookup.
	items map[int]*Item
}

// BuildGraph creates a dependency graph from a roadmap.
func BuildGraph(r *Roadmap) *DependencyGraph {
	g := &DependencyGraph{
		adjacency: make(map[int][]int),
		items:     make(map[int]*Item),
	}

	for i := range r.Items {
		item := &r.Items[i]
		g.items[item.Number] = item
		for _, dep := range item.DependsOn {
			g.adjacency[dep] = append(g.adjacency[dep], item.Number)
		}
	}

	return g
}

// Unblocked returns items that are pending and have all dependencies satisfied
// (i.e., all dependencies are done).
func (g *DependencyGraph) Unblocked() []Item {
	var ready []Item
	for _, item := range g.items {
		if item.Status != StatusPending {
			continue
		}
		if g.allDependenciesDone(item) {
			ready = append(ready, *item)
		}
	}
	return ready
}

// allDependenciesDone checks if all dependencies of an item are done.
func (g *DependencyGraph) allDependenciesDone(item *Item) bool {
	for _, dep := range item.DependsOn {
		depItem, ok := g.items[dep]
		if !ok {
			return false // unknown dependency = not satisfied
		}
		if depItem.Status != StatusDone {
			return false
		}
	}
	return true
}

// Dependents returns the item numbers that directly depend on the given item.
func (g *DependencyGraph) Dependents(number int) []int {
	return g.adjacency[number]
}

// NewlyUnblocked returns items that become unblocked when the given item
// is marked done. Call this after updating an item's status to done.
func (g *DependencyGraph) NewlyUnblocked(completedNumber int) []Item {
	var unblocked []Item
	for _, depNum := range g.adjacency[completedNumber] {
		dep := g.items[depNum]
		if dep == nil || dep.Status != StatusPending {
			continue
		}
		if g.allDependenciesDone(dep) {
			unblocked = append(unblocked, *dep)
		}
	}
	return unblocked
}

// HasCycle returns true if the dependency graph contains a cycle.
func (g *DependencyGraph) HasCycle() bool {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)

	state := make(map[int]int)
	for num := range g.items {
		state[num] = unvisited
	}

	var dfs func(int) bool
	dfs = func(num int) bool {
		state[num] = visiting
		for _, dep := range g.items[num].DependsOn {
			if state[dep] == visiting {
				return true // back edge = cycle
			}
			if state[dep] == unvisited {
				if dfs(dep) {
					return true
				}
			}
		}
		state[num] = visited
		return false
	}

	for num := range g.items {
		if state[num] == unvisited {
			if dfs(num) {
				return true
			}
		}
	}

	return false
}

// TopologicalOrder returns items in dependency order (dependencies first).
// Returns an error if the graph has cycles.
func (g *DependencyGraph) TopologicalOrder() ([]Item, error) {
	if g.HasCycle() {
		return nil, fmt.Errorf("dependency graph has cycles")
	}

	visited := make(map[int]bool)
	var order []Item

	var visit func(int)
	visit = func(num int) {
		if visited[num] {
			return
		}
		visited[num] = true

		item := g.items[num]
		if item == nil {
			return
		}

		// Visit dependencies first
		for _, dep := range item.DependsOn {
			visit(dep)
		}

		order = append(order, *item)
	}

	for num := range g.items {
		visit(num)
	}

	return order, nil
}

// CriticalPath returns the longest dependency chain in the graph.
// Each item in the returned slice depends on the previous one.
func (g *DependencyGraph) CriticalPath() []Item {
	// Compute longest path to each node
	memo := make(map[int]int) // item number → longest path length to it
	parent := make(map[int]int)

	var longest func(int) int
	longest = func(num int) int {
		if v, ok := memo[num]; ok {
			return v
		}

		item := g.items[num]
		if item == nil {
			return 0
		}

		maxLen := 0
		maxParent := -1
		for _, dep := range item.DependsOn {
			depLen := longest(dep)
			if depLen+1 > maxLen {
				maxLen = depLen + 1
				maxParent = dep
			}
		}
		if maxLen == 0 {
			maxLen = 1 // leaf node
		}

		memo[num] = maxLen
		if maxParent >= 0 {
			parent[num] = maxParent
		}
		return maxLen
	}

	// Find the end of the longest path
	maxNum := -1
	maxLen := 0
	for num := range g.items {
		l := longest(num)
		if l > maxLen {
			maxLen = l
			maxNum = num
		}
	}

	if maxNum < 0 {
		return nil
	}

	// Trace back the path
	var path []Item
	for cur := maxNum; cur >= 0; {
		if item := g.items[cur]; item != nil {
			path = append([]Item{*item}, path...)
		}
		if p, ok := parent[cur]; ok {
			cur = p
		} else {
			break
		}
	}

	return path
}

// Stats returns summary statistics about the roadmap.
func (g *DependencyGraph) Stats() RoadmapStats {
	stats := RoadmapStats{
		Total: len(g.items),
	}
	for _, item := range g.items {
		switch item.Status {
		case StatusPending:
			stats.Pending++
		case StatusInProgress:
			stats.InProgress++
		case StatusDone:
			stats.Done++
		case StatusBlocked:
			stats.Blocked++
		}
	}
	return stats
}

// RoadmapStats holds summary counts.
type RoadmapStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
	Blocked    int `json:"blocked"`
}

// FormatProgress returns a human-readable progress string.
func (s RoadmapStats) FormatProgress() string {
	if s.Total == 0 {
		return "No items in roadmap"
	}
	pct := float64(s.Done) / float64(s.Total) * 100
	return fmt.Sprintf("%d/%d done (%.0f%%) — %d pending, %d in progress, %d blocked",
		s.Done, s.Total, pct, s.Pending, s.InProgress, s.Blocked)
}
