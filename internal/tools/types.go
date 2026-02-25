// Package tools defines the tool interface, registry, sandboxed executor,
// risk tier classification, and native tool implementations.
package tools

import (
	"context"
	"encoding/json"
)

// RiskTier classifies tools by the severity of their side effects.
type RiskTier int

const (
	// Read executes immediately, no approval needed, zero side effects.
	Read RiskTier = iota
	// WriteLocal changes stay in worktree, reversible via git.
	WriteLocal
	// WriteVisible actions are visible to the team (Slack, GitHub).
	WriteVisible
	// Destructive requires explicit user approval before executing.
	Destructive
)

// String returns the human-readable name of the risk tier.
func (r RiskTier) String() string {
	switch r {
	case Read:
		return "READ"
	case WriteLocal:
		return "WRITE_LOCAL"
	case WriteVisible:
		return "WRITE_VISIBLE"
	case Destructive:
		return "DESTRUCTIVE"
	default:
		return "UNKNOWN"
	}
}

// ToolCall represents a tool invocation from the LLM.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult is the output of a tool execution, sent back to the LLM.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// Tool is the interface that all native tools implement.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string
	// Description returns a human-readable description for the LLM.
	Description() string
	// Parameters returns the JSON Schema for the tool's parameters.
	Parameters() json.RawMessage
	// RiskTier returns the tool's default risk classification.
	RiskTier() RiskTier
	// Execute runs the tool with the given arguments and returns a result.
	Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

// Role represents an agent role in the system.
type Role string

const (
	RolePM         Role = "pm"
	RoleCoder      Role = "coder"
	RoleReviewer   Role = "reviewer"
	RoleResearcher Role = "researcher"
	RoleArtist     Role = "artist"
	RoleLead       Role = "lead"
)
