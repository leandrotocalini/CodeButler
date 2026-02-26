package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/leandrotocalini/codebutler/internal/tools"
)

func TestMergedRegistry_DiscoverTools(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)

	// Create a manager with mock tools
	starter := newMockStarter()
	defer starter.close()

	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {Command: "mcp-server-github"},
		},
	}

	mgr := NewManager("coder", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.StartAllWith(ctx, cfg, starter); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	merged := NewMergedRegistry(native, mgr, nil)
	merged.DiscoverTools()

	// Should have MCP tools (no native tools registered in this test)
	all := merged.ListAll()
	if len(all) != 2 {
		t.Errorf("expected 2 tools (all MCP), got %d", len(all))
	}

	if !merged.IsMCPTool("github_tool1") {
		t.Error("github_tool1 should be an MCP tool")
	}
	if !merged.IsMCPTool("github_tool2") {
		t.Error("github_tool2 should be an MCP tool")
	}
}

func TestMergedRegistry_NativeWinsOnCollision(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)

	// Register a native tool named "Read"
	native.Register(&dummyTool{name: "Read", desc: "Native Read"})

	// Create MCP server with a tool also named "Read"
	server := newMockMCPServer()
	defer server.close()

	go func() {
		server.respondTo(InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: "test", Version: "1.0.0"},
		})
		buf := make([]byte, 4096)
		server.stdinR.Read(buf)

		server.respondTo(ToolsListResult{
			Tools: []MCPTool{
				{Name: "Read", Description: "MCP Read", InputSchema: json.RawMessage(`{}`)},
				{Name: "mcp_only", Description: "MCP Only", InputSchema: json.RawMessage(`{}`)},
			},
		})
	}()

	starter := &singleMockStarter{server: server}

	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"test": {Command: "test-server"},
		},
	}

	mgr := NewManager("coder", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr.StartAllWith(ctx, cfg, starter)

	merged := NewMergedRegistry(native, mgr, slog.Default())
	merged.DiscoverTools()

	// "Read" should NOT be in MCP mapping (native wins)
	if merged.IsMCPTool("Read") {
		t.Error("Read should not be an MCP tool — native wins on collision")
	}

	// "mcp_only" should be an MCP tool
	if !merged.IsMCPTool("mcp_only") {
		t.Error("mcp_only should be an MCP tool")
	}
}

func TestMergedRegistry_ExecuteNative(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)
	native.Register(&dummyTool{name: "TestTool", desc: "A test tool"})

	mgr := NewManager("coder")
	merged := NewMergedRegistry(native, mgr, nil)
	merged.DiscoverTools()

	call := tools.ToolCall{
		ID:        "t1",
		Name:      "TestTool",
		Arguments: json.RawMessage(`{}`),
	}

	result, err := merged.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.Content != "dummy result" {
		t.Errorf("content: got %q", result.Content)
	}
}

func TestMergedRegistry_ExecuteMCP(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)

	server := newMockMCPServer()
	defer server.close()

	go func() {
		// Initialize
		server.respondTo(InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: "github", Version: "1.0.0"},
		})
		buf := make([]byte, 4096)
		server.stdinR.Read(buf)

		// tools/list
		server.respondTo(ToolsListResult{
			Tools: []MCPTool{
				{Name: "get_issue", Description: "Get issue", InputSchema: json.RawMessage(`{}`)},
			},
		})

		// tools/call
		server.respondTo(ToolCallResult{
			Content: []ToolContent{
				{Type: "text", Text: "Issue #42: Fix bug"},
			},
		})
	}()

	starter := &singleMockStarter{server: server}
	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {Command: "mcp-server-github"},
		},
	}

	mgr := NewManager("coder", WithToolTimeout(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr.StartAllWith(ctx, cfg, starter)

	merged := NewMergedRegistry(native, mgr, nil)
	merged.DiscoverTools()

	call := tools.ToolCall{
		ID:        "t1",
		Name:      "get_issue",
		Arguments: json.RawMessage(`{"number":42}`),
	}

	result, err := merged.Execute(ctx, call)
	if err != nil {
		t.Fatalf("execute MCP tool failed: %v", err)
	}
	if !strings.Contains(result.Content, "Issue #42") {
		t.Errorf("expected issue content, got %q", result.Content)
	}
}

func TestMergedRegistry_ExecuteUnknown(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)
	mgr := NewManager("coder")

	merged := NewMergedRegistry(native, mgr, nil)
	merged.DiscoverTools()

	call := tools.ToolCall{
		ID:   "t1",
		Name: "nonexistent",
	}

	result, err := merged.Execute(context.Background(), call)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
	if !result.IsError {
		t.Error("result should be marked as error")
	}
}

func TestMergedRegistry_ListAll(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)
	native.Register(&dummyTool{name: "Read", desc: "Read files"})
	native.Register(&dummyTool{name: "Write", desc: "Write files"})

	starter := newMockStarter()
	defer starter.close()

	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {Command: "mcp-server-github"},
		},
	}

	mgr := NewManager("coder", WithToolTimeout(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr.StartAllWith(ctx, cfg, starter)

	merged := NewMergedRegistry(native, mgr, nil)
	merged.DiscoverTools()

	all := merged.ListAll()
	// 2 native + 2 MCP = 4
	if len(all) != 4 {
		t.Errorf("expected 4 total tools, got %d", len(all))
	}
}

func TestMergedRegistry_MCPServerFor(t *testing.T) {
	native := tools.NewRegistry(tools.RoleCoder, nil)

	starter := newMockStarter()
	defer starter.close()

	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {Command: "mcp-server-github"},
		},
	}

	mgr := NewManager("coder", WithToolTimeout(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr.StartAllWith(ctx, cfg, starter)

	merged := NewMergedRegistry(native, mgr, nil)
	merged.DiscoverTools()

	if merged.MCPServerFor("github_tool1") != "github" {
		t.Errorf("expected github server, got %q", merged.MCPServerFor("github_tool1"))
	}
	if merged.MCPServerFor("Read") != "" {
		t.Errorf("native tool should return empty server: got %q", merged.MCPServerFor("Read"))
	}
}

// dummyTool is a minimal Tool implementation for testing.
type dummyTool struct {
	name string
	desc string
}

func (d *dummyTool) Name() string                { return d.name }
func (d *dummyTool) Description() string          { return d.desc }
func (d *dummyTool) Parameters() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (d *dummyTool) RiskTier() tools.RiskTier     { return tools.Read }
func (d *dummyTool) Execute(ctx context.Context, call tools.ToolCall) (tools.ToolResult, error) {
	return tools.ToolResult{ToolCallID: call.ID, Content: "dummy result"}, nil
}

// Ensure dummyTool implements Tool interface.
var _ tools.Tool = (*dummyTool)(nil)

// Ensure mockStarter implements ProcessStarter.
var _ ProcessStarter = (*mockStarter)(nil)
var _ ProcessStarter = (*singleMockStarter)(nil)
var _ ProcessStarter = (*failingStarter)(nil)

// Ensure unused import is not needed
func init() {
	_ = (*exec.Cmd)(nil)
}
