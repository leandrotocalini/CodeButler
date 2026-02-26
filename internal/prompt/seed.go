// Package prompt handles seed file loading and system prompt assembly.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidRoles lists all valid agent roles.
var ValidRoles = []string{"pm", "coder", "reviewer", "researcher", "artist", "lead"}

// SeedFiles holds the raw content of all seed-related files for a role.
type SeedFiles struct {
	Role      string
	Seed      string // contents of seeds/<role>.md
	Global    string // contents of seeds/global.md
	Workflows string // contents of seeds/workflows.md (PM only)
}

// LoadSeed reads a single seed file from the seeds directory.
// Returns the content with the "## Archived Learnings" section excluded.
func LoadSeed(seedsDir, filename string) (string, error) {
	path := filepath.Join(seedsDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read seed %s: %w", filename, err)
	}
	return ExcludeArchivedLearnings(string(data)), nil
}

// LoadSeedFiles loads all seed files needed for a given role.
func LoadSeedFiles(seedsDir, role string) (*SeedFiles, error) {
	seed, err := LoadSeed(seedsDir, role+".md")
	if err != nil {
		return nil, fmt.Errorf("load %s seed: %w", role, err)
	}

	global, err := LoadSeed(seedsDir, "global.md")
	if err != nil {
		return nil, fmt.Errorf("load global seed: %w", err)
	}

	sf := &SeedFiles{
		Role:   role,
		Seed:   seed,
		Global: global,
	}

	// PM also gets workflows
	if role == "pm" {
		workflows, err := LoadSeed(seedsDir, "workflows.md")
		if err != nil {
			return nil, fmt.Errorf("load workflows: %w", err)
		}
		sf.Workflows = workflows
	}

	return sf, nil
}

// ExcludeArchivedLearnings removes the "## Archived Learnings" section
// and everything after it from the content.
func ExcludeArchivedLearnings(content string) string {
	marker := "## Archived Learnings"
	idx := strings.Index(content, marker)
	if idx < 0 {
		return content
	}
	return strings.TrimRight(content[:idx], "\n\r\t ")
}
