// Package tools defines read-only tools available to the ProductManager.
//
// These tools let the PM autonomously explore the repository during the
// planning phase. All tools are strictly read-only and confined to the
// repo directory — no writes, no shell execution, no path traversal.
package tools

import (
	"github.com/leandrotocalini/CodeButler/internal/models"
)

// PMTools returns the set of read-only tools available to the ProductManager.
// All file operations are sandboxed to repoDir.
func PMTools(repoDir string) []models.Tool {
	return []models.Tool{
		{
			Name:        "ReadFile",
			Description: "Read the contents of a file in the repository. Returns the file text with line numbers. Use offset/limit for large files.",
			Parameters: jsonSchema(map[string]propDef{
				"path":   {Type: "string", Desc: "Relative path from repo root (e.g. 'internal/daemon/daemon.go')", Required: true},
				"offset": {Type: "integer", Desc: "Start reading from this line number (1-based). Omit to start from beginning."},
				"limit":  {Type: "integer", Desc: "Max number of lines to return. Omit to read the whole file (capped at 500 lines)."},
			}),
		},
		{
			Name:        "Grep",
			Description: "Search for a regex pattern across files in the repository. Returns matching lines with file paths and line numbers.",
			Parameters: jsonSchema(map[string]propDef{
				"pattern": {Type: "string", Desc: "Regex pattern to search for (e.g. 'func.*Login', 'TODO')", Required: true},
				"glob":    {Type: "string", Desc: "File glob filter (e.g. '*.go', '**/*.ts'). Omit to search all files."},
				"path":    {Type: "string", Desc: "Subdirectory to search in, relative to repo root. Omit for entire repo."},
			}),
		},
		{
			Name:        "ListFiles",
			Description: "List files matching a glob pattern. Returns file paths relative to the repo root, sorted alphabetically.",
			Parameters: jsonSchema(map[string]propDef{
				"pattern": {Type: "string", Desc: "Glob pattern (e.g. '**/*.go', 'internal/**/*.go', '*.md')", Required: true},
				"path":    {Type: "string", Desc: "Subdirectory to search in, relative to repo root. Omit for entire repo."},
			}),
		},
		{
			Name:        "GitLog",
			Description: "Show recent git commits. Returns commit hash, author, date, and message.",
			Parameters: jsonSchema(map[string]propDef{
				"n":    {Type: "integer", Desc: "Number of commits to show (default: 10, max: 50)."},
				"path": {Type: "string", Desc: "Only show commits touching this file or directory. Omit for all commits."},
			}),
		},
		{
			Name:        "GitDiff",
			Description: "Show git diff for uncommitted changes or between refs.",
			Parameters: jsonSchema(map[string]propDef{
				"ref":  {Type: "string", Desc: "Git ref to diff against (e.g. 'HEAD~3', 'main'). Omit for uncommitted changes."},
				"path": {Type: "string", Desc: "Only show diff for this file or directory. Omit for all changes."},
			}),
		},
	}
}

// ---------------------------------------------------------------------------
// JSON Schema builder helpers
// ---------------------------------------------------------------------------

type propDef struct {
	Type     string
	Desc     string
	Required bool
}

// jsonSchema builds an OpenAI function-calling-compatible JSON Schema object.
func jsonSchema(props map[string]propDef) map[string]interface{} {
	properties := make(map[string]interface{}, len(props))
	var required []interface{}

	for name, p := range props {
		prop := map[string]interface{}{
			"type":        p.Type,
			"description": p.Desc,
		}
		properties[name] = prop
		if p.Required {
			required = append(required, name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
