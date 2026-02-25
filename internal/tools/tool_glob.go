package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GlobTool finds files matching a glob pattern within the sandbox.
type GlobTool struct {
	sandbox *Sandbox
}

// NewGlobTool creates a GlobTool sandboxed to the given root.
func NewGlobTool(sandbox *Sandbox) *GlobTool {
	return &GlobTool{sandbox: sandbox}
}

type globArgs struct {
	Pattern string `json:"pattern"`
}

func (t *GlobTool) Name() string        { return "Glob" }
func (t *GlobTool) Description() string { return "Find files matching a glob pattern" }
func (t *GlobTool) RiskTier() RiskTier  { return Read }

func (t *GlobTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern to match files (e.g. '**/*.go', 'src/*.ts')"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args globArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Pattern == "" {
		return ToolResult{Content: "pattern is required", IsError: true}, nil
	}

	matches, err := globWalk(t.sandbox.Root, args.Pattern)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("glob error: %v", err), IsError: true}, nil
	}

	if len(matches) == 0 {
		return ToolResult{Content: "no files matched"}, nil
	}

	return ToolResult{Content: strings.Join(matches, "\n")}, nil
}

// globWalk walks the directory tree and matches files against a glob pattern.
// Supports ** for recursive directory matching.
func globWalk(root, pattern string) ([]string, error) {
	var matches []string

	// If pattern contains **, use filepath.Walk for recursive matching
	if strings.Contains(pattern, "**") {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if info.IsDir() {
				return nil
			}

			// Get relative path
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}

			if matchDoubleGlob(rel, pattern) {
				matches = append(matches, rel)
			}
			return nil
		})
		return matches, err
	}

	// Simple glob without **
	fullPattern := filepath.Join(root, pattern)
	found, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}

	for _, f := range found {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			continue
		}
		// Only include files, not directories
		info, err := os.Stat(f)
		if err != nil || info.IsDir() {
			continue
		}
		matches = append(matches, rel)
	}
	return matches, nil
}

// matchDoubleGlob matches a relative path against a pattern that may contain **.
// ** matches zero or more directory levels.
func matchDoubleGlob(path, pattern string) bool {
	// Split both path and pattern by /
	pathParts := strings.Split(filepath.ToSlash(path), "/")
	patternParts := strings.Split(filepath.ToSlash(pattern), "/")

	return matchParts(pathParts, patternParts)
}

func matchParts(pathParts, patternParts []string) bool {
	if len(patternParts) == 0 {
		return len(pathParts) == 0
	}

	if patternParts[0] == "**" {
		// ** matches zero or more directories
		rest := patternParts[1:]
		for i := 0; i <= len(pathParts); i++ {
			if matchParts(pathParts[i:], rest) {
				return true
			}
		}
		return false
	}

	if len(pathParts) == 0 {
		return false
	}

	matched, err := filepath.Match(patternParts[0], pathParts[0])
	if err != nil || !matched {
		return false
	}

	return matchParts(pathParts[1:], patternParts[1:])
}
