// Package skills parses and validates skill markdown files.
package skills

import (
	"fmt"
	"regexp"
	"strings"
)

// Skill represents a parsed skill file.
type Skill struct {
	Name        string
	Description string
	Triggers    []string
	Agent       string
	Prompt      string
	Variables   []Variable
}

// Variable represents a parameter extracted from triggers or prompt.
type Variable struct {
	Name         string
	DefaultValue string // empty if no default
	InTrigger    bool
	InPrompt     bool
}

var (
	// {param} in triggers
	triggerVarRe = regexp.MustCompile(`\{(\w+)\}`)
	// {{param}} or {{param | default: "value"}} in prompt
	promptVarRe = regexp.MustCompile(`\{\{(\w+)(?:\s*\|\s*default:\s*"([^"]*)")?\}\}`)
)

// ParseSkill parses a skill markdown file into a structured Skill.
func ParseSkill(content string) (*Skill, error) {
	sections := parseSections(content)

	s := &Skill{}

	// Extract name from # header
	if name, ok := sections["_name"]; ok {
		s.Name = name
	}

	// Description is the content between # header and first ## section
	if desc, ok := sections["_description"]; ok {
		s.Description = strings.TrimSpace(desc)
	}

	// Trigger section
	if trigger, ok := sections["trigger"]; ok {
		trigger = strings.TrimSpace(trigger)
		// Triggers are comma-separated
		for _, t := range strings.Split(trigger, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				s.Triggers = append(s.Triggers, t)
			}
		}
	}

	// Agent section
	if agent, ok := sections["agent"]; ok {
		s.Agent = strings.TrimSpace(agent)
	}

	// Prompt section
	if prompt, ok := sections["prompt"]; ok {
		s.Prompt = strings.TrimSpace(prompt)
	}

	// Extract variables
	s.Variables = extractVariables(s.Triggers, s.Prompt)

	return s, nil
}

// parseSections splits a markdown file into sections by ## headers.
// Special keys: _name (# header), _description (content between # and first ##).
func parseSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentSection string
	var currentContent []string
	foundFirstH2 := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// # header (skill name)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			sections["_name"] = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			currentSection = "_description"
			currentContent = nil
			continue
		}

		// ## section header
		if strings.HasPrefix(trimmed, "## ") {
			// Save previous section
			if currentSection != "" {
				sections[currentSection] = strings.Join(currentContent, "\n")
			}
			foundFirstH2 = true
			currentSection = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			currentContent = nil
			continue
		}

		// Content line
		if currentSection != "" {
			// Skip blank lines right after section header
			if len(currentContent) == 0 && trimmed == "" && foundFirstH2 {
				continue
			}
			currentContent = append(currentContent, line)
		}
	}

	// Save last section
	if currentSection != "" {
		sections[currentSection] = strings.Join(currentContent, "\n")
	}

	return sections
}

// extractVariables finds all {param} in triggers and {{param}} in prompt.
func extractVariables(triggers []string, prompt string) []Variable {
	varMap := make(map[string]*Variable)

	// Extract from triggers
	for _, t := range triggers {
		matches := triggerVarRe.FindAllStringSubmatch(t, -1)
		for _, m := range matches {
			name := m[1]
			if v, ok := varMap[name]; ok {
				v.InTrigger = true
			} else {
				varMap[name] = &Variable{Name: name, InTrigger: true}
			}
		}
	}

	// Extract from prompt
	matches := promptVarRe.FindAllStringSubmatch(prompt, -1)
	for _, m := range matches {
		name := m[1]
		defaultVal := ""
		if len(m) > 2 {
			defaultVal = m[2]
		}
		if v, ok := varMap[name]; ok {
			v.InPrompt = true
			if defaultVal != "" {
				v.DefaultValue = defaultVal
			}
		} else {
			varMap[name] = &Variable{Name: name, InPrompt: true, DefaultValue: defaultVal}
		}
	}

	// Convert to slice
	vars := make([]Variable, 0, len(varMap))
	for _, v := range varMap {
		vars = append(vars, *v)
	}

	return vars
}

// ValidationError represents a skill validation error.
type ValidationError struct {
	File    string
	Message string
}

func (e ValidationError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("%s: %s", e.File, e.Message)
	}
	return e.Message
}

// ValidateSkill validates a parsed skill for correctness.
func ValidateSkill(s *Skill, filename string) []ValidationError {
	var errs []ValidationError

	// Required: name
	if s.Name == "" {
		errs = append(errs, ValidationError{File: filename, Message: "missing skill name (# header)"})
	}

	// Required: at least one trigger
	if len(s.Triggers) == 0 {
		errs = append(errs, ValidationError{File: filename, Message: "missing ## Trigger section"})
	}

	// Required: agent
	if s.Agent == "" {
		errs = append(errs, ValidationError{File: filename, Message: "missing ## Agent section"})
	} else {
		validAgents := map[string]bool{
			"pm": true, "coder": true, "reviewer": true,
			"researcher": true, "artist": true, "lead": true,
		}
		if !validAgents[s.Agent] {
			errs = append(errs, ValidationError{
				File:    filename,
				Message: fmt.Sprintf("invalid agent %q (must be one of: pm, coder, reviewer, researcher, artist, lead)", s.Agent),
			})
		}
	}

	// Required: prompt
	if s.Prompt == "" {
		errs = append(errs, ValidationError{File: filename, Message: "missing ## Prompt section"})
	}

	// Check for undefined variables: used in prompt but not in trigger and no default
	for _, v := range s.Variables {
		if v.InPrompt && !v.InTrigger && v.DefaultValue == "" {
			errs = append(errs, ValidationError{
				File:    filename,
				Message: fmt.Sprintf("variable {{%s}} in prompt has no trigger and no default", v.Name),
			})
		}
	}

	return errs
}

// ValidateAll validates a collection of skills for cross-skill issues.
func ValidateAll(skills []*Skill) []ValidationError {
	var errs []ValidationError

	// Check for duplicate triggers across skills
	triggerOwner := make(map[string]string)
	for _, s := range skills {
		for _, t := range s.Triggers {
			// Normalize: lowercase, strip {param} placeholders
			normalized := normalizeTrigger(t)
			if owner, exists := triggerOwner[normalized]; exists && owner != s.Name {
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("duplicate trigger %q in skills %q and %q", t, owner, s.Name),
				})
			}
			triggerOwner[normalized] = s.Name
		}
	}

	return errs
}

// normalizeTrigger normalizes a trigger for duplicate detection.
func normalizeTrigger(trigger string) string {
	// Lowercase and replace {param} with a placeholder
	normalized := strings.ToLower(trigger)
	normalized = triggerVarRe.ReplaceAllString(normalized, "{}")
	return strings.TrimSpace(normalized)
}
