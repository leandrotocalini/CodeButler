package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// GitCommitter provides git commit operations.
type GitCommitter interface {
	Commit(ctx context.Context, files []string, message string) error
}

// GitPusher provides git push operations.
type GitPusher interface {
	Push(ctx context.Context) error
}

// PRCreator provides GitHub PR operations.
type PRCreator interface {
	CreatePR(ctx context.Context, title, body, base, head string) (string, error)
}

// --- GitCommit Tool ---

// GitCommitTool stages files and creates a commit.
type GitCommitTool struct {
	git GitCommitter
}

// NewGitCommitTool creates a GitCommit tool.
func NewGitCommitTool(git GitCommitter) *GitCommitTool {
	return &GitCommitTool{git: git}
}

func (t *GitCommitTool) Name() string        { return "GitCommit" }
func (t *GitCommitTool) Description() string  {
	return "Stage files and create a git commit. Idempotent: if no changes to commit, succeeds silently."
}
func (t *GitCommitTool) RiskTier() RiskTier   { return WriteVisible }
func (t *GitCommitTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"files": {
				"type": "array",
				"items": {"type": "string"},
				"description": "List of file paths to stage"
			},
			"message": {
				"type": "string",
				"description": "Commit message"
			}
		},
		"required": ["files", "message"]
	}`)
}

func (t *GitCommitTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		Files   []string `json:"files"`
		Message string   `json:"message"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if len(args.Files) == 0 {
		return ToolResult{Content: "at least one file is required", IsError: true}, nil
	}
	if args.Message == "" {
		return ToolResult{Content: "commit message is required", IsError: true}, nil
	}

	if err := t.git.Commit(ctx, args.Files, args.Message); err != nil {
		return ToolResult{Content: fmt.Sprintf("git commit failed: %v", err), IsError: true}, nil
	}

	return ToolResult{Content: "Committed successfully."}, nil
}

// --- GitPush Tool ---

// GitPushTool pushes the current branch to the remote.
type GitPushTool struct {
	git GitPusher
}

// NewGitPushTool creates a GitPush tool.
func NewGitPushTool(git GitPusher) *GitPushTool {
	return &GitPushTool{git: git}
}

func (t *GitPushTool) Name() string        { return "GitPush" }
func (t *GitPushTool) Description() string  {
	return "Push the current branch to the remote. Idempotent: if already up to date, succeeds silently."
}
func (t *GitPushTool) RiskTier() RiskTier   { return WriteVisible }
func (t *GitPushTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

func (t *GitPushTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	if err := t.git.Push(ctx); err != nil {
		return ToolResult{Content: fmt.Sprintf("git push failed: %v", err), IsError: true}, nil
	}

	return ToolResult{Content: "Pushed successfully."}, nil
}

// --- GHCreatePR Tool ---

// GHCreatePRTool creates a GitHub pull request.
type GHCreatePRTool struct {
	pr PRCreator
}

// NewGHCreatePRTool creates a GHCreatePR tool.
func NewGHCreatePRTool(pr PRCreator) *GHCreatePRTool {
	return &GHCreatePRTool{pr: pr}
}

func (t *GHCreatePRTool) Name() string        { return "GHCreatePR" }
func (t *GHCreatePRTool) Description() string  {
	return "Create a GitHub pull request. Idempotent: if a PR already exists for the branch, returns its URL."
}
func (t *GHCreatePRTool) RiskTier() RiskTier   { return WriteVisible }
func (t *GHCreatePRTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {
				"type": "string",
				"description": "PR title"
			},
			"body": {
				"type": "string",
				"description": "PR description body"
			},
			"base": {
				"type": "string",
				"description": "Base branch (e.g., main)"
			},
			"head": {
				"type": "string",
				"description": "Head branch (e.g., codebutler/feature-xyz)"
			}
		},
		"required": ["title", "body", "base", "head"]
	}`)
}

func (t *GHCreatePRTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Base  string `json:"base"`
		Head  string `json:"head"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Title == "" || args.Base == "" || args.Head == "" {
		return ToolResult{Content: "title, base, and head are required", IsError: true}, nil
	}

	url, err := t.pr.CreatePR(ctx, args.Title, args.Body, args.Base, args.Head)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("PR creation failed: %v", err), IsError: true}, nil
	}

	return ToolResult{Content: fmt.Sprintf("PR created: %s", url)}, nil
}
