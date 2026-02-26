package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/leandrotocalini/codebutler/internal/tools"
)

// MergedRegistry combines native tools and MCP tools into a single registry.
// Native tools take priority on name collisions.
type MergedRegistry struct {
	native  *tools.Registry
	manager *Manager
	logger  *slog.Logger

	// mcpMapping maps tool name → server name for MCP tools
	mcpMapping map[string]string
}

// NewMergedRegistry creates a registry that combines native and MCP tools.
func NewMergedRegistry(native *tools.Registry, manager *Manager, logger *slog.Logger) *MergedRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &MergedRegistry{
		native:     native,
		manager:    manager,
		logger:     logger,
		mcpMapping: make(map[string]string),
	}
}

// DiscoverTools refreshes the MCP tool mapping from the manager.
// Call this after manager.StartAll() to populate MCP tools.
func (r *MergedRegistry) DiscoverTools() {
	r.mcpMapping = make(map[string]string)

	mcpTools := r.manager.AllTools()
	nativeNames := r.native.List()

	nativeSet := make(map[string]bool, len(nativeNames))
	for _, name := range nativeNames {
		nativeSet[name] = true
	}

	for toolName, entry := range mcpTools {
		if nativeSet[toolName] {
			r.logger.Warn("MCP tool name conflicts with native tool, native wins",
				"tool", toolName,
				"mcp_server", entry.ServerName,
			)
			continue
		}
		r.mcpMapping[toolName] = entry.ServerName
	}

	r.logger.Info("merged registry updated",
		"native_tools", len(nativeNames),
		"mcp_tools", len(r.mcpMapping),
	)
}

// Execute routes a tool call to either native or MCP execution.
func (r *MergedRegistry) Execute(ctx context.Context, call tools.ToolCall) (tools.ToolResult, error) {
	// Check if it's an MCP tool
	if serverName, ok := r.mcpMapping[call.Name]; ok {
		return r.executeMCP(ctx, serverName, call)
	}

	// Delegate to native registry
	return r.native.Execute(ctx, call)
}

// executeMCP routes the call to the correct MCP server.
func (r *MergedRegistry) executeMCP(ctx context.Context, serverName string, call tools.ToolCall) (tools.ToolResult, error) {
	result, err := r.manager.CallTool(ctx, serverName, call.Name, call.Arguments)
	if err != nil {
		return tools.ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("MCP tool %q error: %v", call.Name, err),
			IsError:    true,
		}, err
	}

	// Convert MCP result to native ToolResult
	content := extractTextContent(result)

	return tools.ToolResult{
		ToolCallID: call.ID,
		Content:    content,
		IsError:    result.IsError,
	}, nil
}

// extractTextContent extracts text from MCP tool result content blocks.
func extractTextContent(result *ToolCallResult) string {
	var parts []string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ListAll returns the names of all available tools (native + MCP).
func (r *MergedRegistry) ListAll() []string {
	nativeNames := r.native.List()

	all := make([]string, 0, len(nativeNames)+len(r.mcpMapping))
	all = append(all, nativeNames...)
	for name := range r.mcpMapping {
		all = append(all, name)
	}
	return all
}

// ToolDefinitions returns tool definitions for all available tools,
// formatted for the LLM's tool-calling interface.
func (r *MergedRegistry) ToolDefinitions() []ToolDefinition {
	var defs []ToolDefinition

	// Native tools
	for _, t := range r.native.AllTools() {
		defs = append(defs, ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
			Source:      "native",
		})
	}

	// MCP tools
	mcpTools := r.manager.AllTools()
	for toolName, entry := range mcpTools {
		if _, isNative := r.mcpMapping[toolName]; !isNative {
			// This tool name was shadowed by native
			continue
		}
		defs = append(defs, ToolDefinition{
			Name:        entry.Tool.Name,
			Description: entry.Tool.Description,
			Parameters:  entry.Tool.InputSchema,
			Source:      "mcp:" + entry.ServerName,
		})
	}

	return defs
}

// ToolDefinition represents a tool definition for the LLM.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Source      string          `json:"source"` // "native" or "mcp:<server>"
}

// IsMCPTool returns true if the named tool is provided by an MCP server.
func (r *MergedRegistry) IsMCPTool(name string) bool {
	_, ok := r.mcpMapping[name]
	return ok
}

// MCPServerFor returns the MCP server name for a tool, or "" if native.
func (r *MergedRegistry) MCPServerFor(name string) string {
	return r.mcpMapping[name]
}
