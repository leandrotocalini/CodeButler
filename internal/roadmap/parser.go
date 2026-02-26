// Package roadmap implements the roadmap file parser, status tracking,
// and dependency resolution for .codebutler/roadmap.md.
package roadmap

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Status represents the state of a roadmap item.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusBlocked    Status = "blocked"
)

// IsValid checks if the status is recognized.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusDone, StatusBlocked:
		return true
	}
	return false
}

// Item represents a single roadmap item.
type Item struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Status     Status   `json:"status"`
	Branch     string   `json:"branch,omitempty"`
	DependsOn  []int    `json:"depends_on,omitempty"`
	Acceptance string   `json:"acceptance"`
	BlockedBy  string   `json:"blocked_by,omitempty"` // reason if blocked
}

// Roadmap holds all parsed items and the project title.
type Roadmap struct {
	Title string `json:"title"`
	Items []Item `json:"items"`
}

var (
	// ## 1. Auth system
	itemHeaderRe = regexp.MustCompile(`^##\s+(\d+)\.\s+(.+)$`)
	// - Status: done
	statusRe = regexp.MustCompile(`^-\s+Status:\s*(\S+)`)
	// - Branch: codebutler/auth-system
	branchRe = regexp.MustCompile(`^-\s+Branch:\s*(.+)`)
	// - Depends on: 1, 2
	dependsRe = regexp.MustCompile(`^-\s+Depends on:\s*(.+)`)
	// - Acceptance criteria: JWT-based auth...
	acceptanceRe = regexp.MustCompile(`^-\s+Acceptance criteria:\s*(.+)`)
	// - Blocked by: needs user input on auth approach
	blockedByRe = regexp.MustCompile(`^-\s+Blocked by:\s*(.+)`)
	// # Roadmap: Project Name
	titleRe = regexp.MustCompile(`^#\s+Roadmap:\s*(.+)`)
)

// Parse reads a roadmap from markdown text.
func Parse(r io.Reader) (*Roadmap, error) {
	scanner := bufio.NewScanner(r)
	roadmap := &Roadmap{}

	var current *Item

	for scanner.Scan() {
		line := scanner.Text()

		// Title line
		if m := titleRe.FindStringSubmatch(line); m != nil {
			roadmap.Title = strings.TrimSpace(m[1])
			continue
		}

		// Item header
		if m := itemHeaderRe.FindStringSubmatch(line); m != nil {
			// Save previous item
			if current != nil {
				roadmap.Items = append(roadmap.Items, *current)
			}
			num, _ := strconv.Atoi(m[1])
			current = &Item{
				Number: num,
				Title:  strings.TrimSpace(m[2]),
				Status: StatusPending, // default
			}
			continue
		}

		if current == nil {
			continue
		}

		// Status
		if m := statusRe.FindStringSubmatch(line); m != nil {
			current.Status = Status(strings.TrimSpace(m[1]))
			continue
		}

		// Branch
		if m := branchRe.FindStringSubmatch(line); m != nil {
			current.Branch = strings.TrimSpace(m[1])
			continue
		}

		// Depends on
		if m := dependsRe.FindStringSubmatch(line); m != nil {
			deps := parseDependencies(m[1])
			current.DependsOn = deps
			continue
		}

		// Acceptance criteria
		if m := acceptanceRe.FindStringSubmatch(line); m != nil {
			current.Acceptance = strings.TrimSpace(m[1])
			continue
		}

		// Blocked by
		if m := blockedByRe.FindStringSubmatch(line); m != nil {
			current.BlockedBy = strings.TrimSpace(m[1])
			continue
		}
	}

	// Save last item
	if current != nil {
		roadmap.Items = append(roadmap.Items, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan roadmap: %w", err)
	}

	return roadmap, nil
}

// ParseString parses a roadmap from a string.
func ParseString(s string) (*Roadmap, error) {
	return Parse(strings.NewReader(s))
}

// parseDependencies extracts item numbers from "1, 2" or "—" (none).
func parseDependencies(s string) []int {
	s = strings.TrimSpace(s)
	if s == "—" || s == "-" || s == "" || s == "none" {
		return nil
	}

	parts := strings.Split(s, ",")
	var deps []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if n, err := strconv.Atoi(p); err == nil {
			deps = append(deps, n)
		}
	}
	return deps
}

// Format serializes a roadmap back to markdown.
func Format(r *Roadmap) string {
	var b strings.Builder

	if r.Title != "" {
		fmt.Fprintf(&b, "# Roadmap: %s\n\n", r.Title)
	}

	for _, item := range r.Items {
		fmt.Fprintf(&b, "## %d. %s\n", item.Number, item.Title)
		fmt.Fprintf(&b, "- Status: %s\n", item.Status)
		if item.Branch != "" {
			fmt.Fprintf(&b, "- Branch: %s\n", item.Branch)
		}
		if len(item.DependsOn) > 0 {
			deps := make([]string, len(item.DependsOn))
			for i, d := range item.DependsOn {
				deps[i] = strconv.Itoa(d)
			}
			fmt.Fprintf(&b, "- Depends on: %s\n", strings.Join(deps, ", "))
		} else {
			fmt.Fprintf(&b, "- Depends on: —\n")
		}
		fmt.Fprintf(&b, "- Acceptance criteria: %s\n", item.Acceptance)
		if item.BlockedBy != "" {
			fmt.Fprintf(&b, "- Blocked by: %s\n", item.BlockedBy)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// GetItem returns the item with the given number, or nil if not found.
func (r *Roadmap) GetItem(number int) *Item {
	for i := range r.Items {
		if r.Items[i].Number == number {
			return &r.Items[i]
		}
	}
	return nil
}

// SetStatus updates the status of an item by number.
func (r *Roadmap) SetStatus(number int, status Status) error {
	item := r.GetItem(number)
	if item == nil {
		return fmt.Errorf("item %d not found", number)
	}
	item.Status = status
	return nil
}

// SetBranch updates the branch name of an item.
func (r *Roadmap) SetBranch(number int, branch string) error {
	item := r.GetItem(number)
	if item == nil {
		return fmt.Errorf("item %d not found", number)
	}
	item.Branch = branch
	return nil
}

// AddItem appends a new item to the roadmap.
func (r *Roadmap) AddItem(title, acceptance string, dependsOn []int) *Item {
	maxNum := 0
	for _, item := range r.Items {
		if item.Number > maxNum {
			maxNum = item.Number
		}
	}

	item := Item{
		Number:     maxNum + 1,
		Title:      title,
		Status:     StatusPending,
		DependsOn:  dependsOn,
		Acceptance: acceptance,
	}
	r.Items = append(r.Items, item)
	return &r.Items[len(r.Items)-1]
}
