package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// roleRestrictions maps each role to the tool names it cannot use.
// Per SPEC §15b: structural enforcement at the executor level.
var roleRestrictions = map[Role]map[string]bool{
	RolePM: {
		"Write":      true,
		"Edit":       true,
		"GitCommit":  true,
		"GitPush":    true,
		"GHCreatePR": true,
	},
	RoleResearcher: {
		"Write":     true,
		"Edit":      true,
		"Bash":      true,
		"GitCommit": true,
		"GitPush":   true,
	},
	RoleArtist: {
		"Bash":      true,
		"GitCommit": true,
		"GitPush":   true,
	},
	RoleReviewer: {
		"Write": true,
		"Edit":  true,
		"Bash":  true,
	},
	RoleLead: {
		"Bash": true,
	},
	RoleCoder: {}, // No restrictions
}

// Registry holds registered tools and enforces role-based access.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	role  Role
	log   *slog.Logger

	// idempotency: track executed tool-call IDs and their cached results
	cacheMu sync.RWMutex
	cache   map[string]ToolResult
}

// NewRegistry creates a new tool registry for the given agent role.
func NewRegistry(role Role, logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		tools: make(map[string]Tool),
		role:  role,
		log:   logger,
		cache: make(map[string]ToolResult),
	}
}

// Register adds a tool to the registry. Returns an error if a tool
// with the same name already exists.
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	r.log.Info("tool registered", "tool", name, "risk_tier", t.RiskTier().String())
	return nil
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// List returns the names of all registered tools accessible to this role.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	restricted := roleRestrictions[r.role]
	var names []string
	for name := range r.tools {
		if !restricted[name] {
			names = append(names, name)
		}
	}
	return names
}

// AllTools returns all registered tools accessible to this role.
func (r *Registry) AllTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	restricted := roleRestrictions[r.role]
	var result []Tool
	for name, t := range r.tools {
		if !restricted[name] {
			result = append(result, t)
		}
	}
	return result
}

// IsRestricted returns true if the given tool name is restricted for this role.
func (r *Registry) IsRestricted(toolName string) bool {
	restricted := roleRestrictions[r.role]
	return restricted[toolName]
}

// Execute runs a tool call with role enforcement and idempotency tracking.
func (r *Registry) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	// Check idempotency cache first
	if call.ID != "" {
		r.cacheMu.RLock()
		if cached, ok := r.cache[call.ID]; ok {
			r.cacheMu.RUnlock()
			r.log.Info("tool call cached (idempotent)", "tool", call.Name, "call_id", call.ID)
			return cached, nil
		}
		r.cacheMu.RUnlock()
	}

	// Check role restrictions
	if r.IsRestricted(call.Name) {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("tool %q is not available for role %q", call.Name, r.role),
			IsError:    true,
		}, fmt.Errorf("tool %q restricted for role %q", call.Name, r.role)
	}

	// Look up tool
	t := r.Get(call.Name)
	if t == nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("unknown tool %q", call.Name),
			IsError:    true,
		}, fmt.Errorf("unknown tool %q", call.Name)
	}

	// Execute tool
	result, err := t.Execute(ctx, call)
	result.ToolCallID = call.ID

	// Cache result for idempotency (even errors, to avoid re-executing)
	if call.ID != "" && err == nil {
		r.cacheMu.Lock()
		r.cache[call.ID] = result
		r.cacheMu.Unlock()
	}

	return result, err
}
