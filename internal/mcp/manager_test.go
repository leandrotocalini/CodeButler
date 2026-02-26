package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
	"time"
)

// mockStarter implements ProcessStarter for testing.
type mockStarter struct {
	servers map[string]*mockMCPServer
}

func newMockStarter() *mockStarter {
	return &mockStarter{
		servers: make(map[string]*mockMCPServer),
	}
}

func (s *mockStarter) Start(name string, config ServerConfig) (*exec.Cmd, *Client, error) {
	server := newMockMCPServer()
	s.servers[name] = server

	// Start a background goroutine to handle the protocol
	go func() {
		// Handle initialize
		server.respondTo(InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: name, Version: "1.0.0"},
		})
		// Read the initialized notification
		buf := make([]byte, 4096)
		server.stdinR.Read(buf)

		// Handle tools/list
		server.respondTo(ToolsListResult{
			Tools: []MCPTool{
				{
					Name:        name + "_tool1",
					Description: "Tool 1 from " + name,
					InputSchema: json.RawMessage(`{"type":"object"}`),
				},
				{
					Name:        name + "_tool2",
					Description: "Tool 2 from " + name,
					InputSchema: json.RawMessage(`{"type":"object"}`),
				},
			},
		})
	}()

	client := NewClient(server.stdinW, server.stdoutR)

	// Use a dummy exec.Cmd (not started)
	cmd := &exec.Cmd{}

	return cmd, client, nil
}

func (s *mockStarter) close() {
	for _, server := range s.servers {
		server.close()
	}
}

func TestManager_StartAll(t *testing.T) {
	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {
				Command: "mcp-server-github",
				Roles:   []string{"pm"},
			},
			"postgres": {
				Command: "mcp-server-postgres",
				Roles:   []string{"coder"},
			},
		},
	}

	starter := newMockStarter()
	defer starter.close()

	mgr := NewManager("pm", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.StartAllWith(ctx, cfg, starter); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// PM should only have github (not postgres)
	if mgr.ServerCount() != 1 {
		t.Errorf("expected 1 server for PM, got %d", mgr.ServerCount())
	}
	if !mgr.IsAlive("github") {
		t.Error("github should be alive")
	}
	if mgr.IsAlive("postgres") {
		t.Error("postgres should not be started for PM role")
	}
}

func TestManager_AllTools(t *testing.T) {
	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {
				Command: "mcp-server-github",
				// No roles = all roles
			},
		},
	}

	starter := newMockStarter()
	defer starter.close()

	mgr := NewManager("coder", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.StartAllWith(ctx, cfg, starter); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	tools := mgr.AllTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	if entry, ok := tools["github_tool1"]; ok {
		if entry.ServerName != "github" {
			t.Errorf("tool1 server: got %q", entry.ServerName)
		}
	} else {
		t.Error("missing github_tool1")
	}
}

func TestManager_CallTool(t *testing.T) {
	server := newMockMCPServer()
	defer server.close()

	// Set up a mock server process that handles tool calls
	go func() {
		// Handle initialize
		server.respondTo(InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: "test", Version: "1.0.0"},
		})
		// Read initialized notification
		buf := make([]byte, 4096)
		server.stdinR.Read(buf)

		// Handle tools/list
		server.respondTo(ToolsListResult{
			Tools: []MCPTool{
				{Name: "get_issue", Description: "Get issue", InputSchema: json.RawMessage(`{}`)},
			},
		})

		// Handle tools/call
		server.respondTo(ToolCallResult{
			Content: []ToolContent{
				{Type: "text", Text: "Issue #1: Hello"},
			},
		})
	}()

	// Create a starter that uses our mock server
	starter := &singleMockStarter{server: server}

	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {Command: "mcp-server-github"},
		},
	}

	mgr := NewManager("pm", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.StartAllWith(ctx, cfg, starter); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	result, err := mgr.CallTool(ctx, "github", "get_issue", json.RawMessage(`{"number":1}`))
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Issue #1: Hello" {
		t.Errorf("content: got %q", result.Content[0].Text)
	}
}

// singleMockStarter creates a client from an existing mock server.
type singleMockStarter struct {
	server *mockMCPServer
}

func (s *singleMockStarter) Start(name string, config ServerConfig) (*exec.Cmd, *Client, error) {
	client := NewClient(s.server.stdinW, s.server.stdoutR)
	return &exec.Cmd{}, client, nil
}

func TestManager_CallTool_ServerNotFound(t *testing.T) {
	mgr := NewManager("pm")

	ctx := context.Background()
	_, err := mgr.CallTool(ctx, "nonexistent", "tool", nil)
	if err == nil {
		t.Error("expected error for unknown server")
	}
}

func TestManager_StopAll(t *testing.T) {
	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {Command: "mcp-server-github"},
		},
	}

	starter := newMockStarter()
	defer starter.close()

	mgr := NewManager("pm", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.StartAllWith(ctx, cfg, starter); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if mgr.ServerCount() != 1 {
		t.Fatalf("expected 1 server, got %d", mgr.ServerCount())
	}

	mgr.StopAll()

	if mgr.ServerCount() != 0 {
		t.Errorf("expected 0 servers after stop, got %d", mgr.ServerCount())
	}
}

func TestManager_StartAll_FailedServerSkipped(t *testing.T) {
	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"good": {Command: "good-server"},
			"bad":  {Command: "bad-server"},
		},
	}

	starter := &failingStarter{
		failFor: "bad",
		inner:   newMockStarter(),
	}
	defer starter.inner.close()

	mgr := NewManager("pm", WithToolTimeout(5*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should not return error — failed servers are logged and skipped
	if err := mgr.StartAllWith(ctx, cfg, starter); err != nil {
		t.Fatalf("start should not fail: %v", err)
	}

	if mgr.ServerCount() != 1 {
		t.Errorf("expected 1 server (bad skipped), got %d", mgr.ServerCount())
	}
	if !mgr.IsAlive("good") {
		t.Error("good server should be alive")
	}
}

// failingStarter wraps another starter but fails for a specific server name.
type failingStarter struct {
	failFor string
	inner   *mockStarter
}

func (s *failingStarter) Start(name string, config ServerConfig) (*exec.Cmd, *Client, error) {
	if name == s.failFor {
		return nil, nil, io.ErrClosedPipe
	}
	return s.inner.Start(name, config)
}
