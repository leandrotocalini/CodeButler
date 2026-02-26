package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillSummary holds minimal skill info for the PM's system prompt.
type SkillSummary struct {
	Name        string
	Description string
	Triggers    string
}

// ScanSkillIndex scans the skills directory and builds a minimal skill index.
// Extracts name (from filename or # header), description (first paragraph),
// and triggers (from ## Trigger section).
func ScanSkillIndex(skillsDir string) ([]SkillSummary, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var skills []SkillSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(skillsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		s := parseSkillSummary(entry.Name(), string(data))
		if s.Name != "" {
			skills = append(skills, s)
		}
	}

	return skills, nil
}

// parseSkillSummary extracts name, description, and triggers from a skill file.
func parseSkillSummary(filename, content string) SkillSummary {
	lines := strings.Split(content, "\n")

	s := SkillSummary{
		Name: strings.TrimSuffix(filename, ".md"),
	}

	// Extract name from # header if present
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			s.Name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			break
		}
	}

	// Extract description: first non-empty line after the # header
	pastHeader := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			pastHeader = true
			continue
		}
		if pastHeader && trimmed != "" && !strings.HasPrefix(trimmed, "##") {
			s.Description = trimmed
			break
		}
	}

	// Extract triggers from ## Trigger section
	inTrigger := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Trigger" {
			inTrigger = true
			continue
		}
		if inTrigger {
			if strings.HasPrefix(trimmed, "##") {
				break // next section
			}
			if trimmed != "" {
				s.Triggers = trimmed
				break
			}
		}
	}

	return s
}

// FormatSkillIndex formats the skill index for inclusion in the PM's system prompt.
func FormatSkillIndex(skills []SkillSummary) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Skills\n\n")
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("- **%s**: %s", s.Name, s.Description))
		if s.Triggers != "" {
			b.WriteString(fmt.Sprintf(" Triggers: %s", s.Triggers))
		}
		b.WriteString("\n")
	}

	return b.String()
}
