package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Index holds all loaded and validated skills.
type Index struct {
	Skills  []*Skill
	ByName  map[string]*Skill
	logger  *slog.Logger
}

// LoaderOption configures the skill loader.
type LoaderOption func(*Index)

// WithLoaderLogger sets the logger.
func WithLoaderLogger(l *slog.Logger) LoaderOption {
	return func(idx *Index) {
		idx.logger = l
	}
}

// LoadIndex scans a skills directory, parses all .md files, validates them,
// and returns an index. Invalid skills are skipped with a warning (not fatal).
func LoadIndex(skillsDir string, opts ...LoaderOption) (*Index, error) {
	idx := &Index{
		ByName: make(map[string]*Skill),
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(idx)
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			idx.logger.Info("skills directory not found, no skills loaded", "dir", skillsDir)
			return idx, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(skillsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			idx.logger.Warn("failed to read skill file", "file", entry.Name(), "err", err)
			continue
		}

		skill, err := ParseSkill(string(data))
		if err != nil {
			idx.logger.Warn("failed to parse skill file", "file", entry.Name(), "err", err)
			continue
		}

		// Validate individual skill
		errs := ValidateSkill(skill, entry.Name())
		if len(errs) > 0 {
			for _, e := range errs {
				idx.logger.Warn("skill validation error", "file", entry.Name(), "err", e.Message)
			}
			continue // Skip invalid skills
		}

		idx.Skills = append(idx.Skills, skill)
		idx.ByName[skill.Name] = skill
	}

	// Cross-skill validation
	crossErrs := ValidateAll(idx.Skills)
	for _, e := range crossErrs {
		idx.logger.Warn("cross-skill validation error", "err", e.Message)
	}

	idx.logger.Info("skills loaded", "count", len(idx.Skills))
	return idx, nil
}

// Validate runs validation on all skill files in a directory and returns all errors.
// Unlike LoadIndex, this does not skip invalid files — it reports all issues.
func Validate(skillsDir string) []ValidationError {
	var allErrors []ValidationError
	var skills []*Skill

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		allErrors = append(allErrors, ValidationError{Message: fmt.Sprintf("read skills dir: %v", err)})
		return allErrors
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(skillsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			allErrors = append(allErrors, ValidationError{File: entry.Name(), Message: fmt.Sprintf("read file: %v", err)})
			continue
		}

		skill, err := ParseSkill(string(data))
		if err != nil {
			allErrors = append(allErrors, ValidationError{File: entry.Name(), Message: fmt.Sprintf("parse: %v", err)})
			continue
		}

		errs := ValidateSkill(skill, entry.Name())
		allErrors = append(allErrors, errs...)

		if len(errs) == 0 {
			skills = append(skills, skill)
		}
	}

	// Cross-skill validation
	allErrors = append(allErrors, ValidateAll(skills)...)

	return allErrors
}
