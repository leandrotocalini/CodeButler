package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// mockPipe implements io.Writer for stdin and provides a reader for stdout.
type mockPipe struct {
	written bytes.Buffer
}

// mockMCPServer simulates an MCP server's stdout responses.
// Write requests to its stdin, and it responds on stdout.
type mockMCPServer struct {
	stdinR  *io.PipeReader
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stdoutW *io.PipeWriter
}

func newMockMCPServer() *mockMCPServer {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	return &mockMCPServer{
		stdinR:  stdinR,
		stdinW:  stdinW,
		stdoutR: stdoutR,
		stdoutW: stdoutW,
	}
}

// respondTo reads a request from stdin and writes a response to stdout.
func (s *mockMCPServer) respondTo(response interface{}) error {
	// Read the request line
	buf := make([]byte, 4096)
	n, err := s.stdinR.Read(buf)
	if err != nil {
		return err
	}

	// Parse request to get ID
	var req jsonRPCRequest
	if err := json.Unmarshal(buf[:n], &req); err != nil {
		return err
	}

	// Build response with matching ID
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  response,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = s.stdoutW.Write(data)
	return err
}

func (s *mockMCPServer) close() {
	s.stdinR.Close()
	s.stdinW.Close()
	s.stdoutR.Close()
	s.stdoutW.Close()
}

func TestClient_Initialize(t *testing.T) {
	server := newMockMCPServer()
	defer server.close()

	client := NewClient(server.stdinW, server.stdoutR)
	defer client.Close()

	// Handle initialize in background
	go func() {
		server.respondTo(InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo: ServerInfo{
				Name:    "test-server",
				Version: "1.0.0",
			},
		})
		// Read the initialized notification (but don't respond)
		buf := make([]byte, 4096)
		server.stdinR.Read(buf)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocol version: got %q", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("server name: got %q", result.ServerInfo.Name)
	}
}

func TestClient_ListTools(t *testing.T) {
	server := newMockMCPServer()
	defer server.close()

	client := NewClient(server.stdinW, server.stdoutR)
	defer client.Close()

	go func() {
		server.respondTo(ToolsListResult{
			Tools: []MCPTool{
				{
					Name:        "get_issue",
					Description: "Get a GitHub issue by number",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"number":{"type":"integer"}}}`),
				},
				{
					Name:        "create_pr",
					Description: "Create a pull request",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`),
				},
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "get_issue" {
		t.Errorf("tool 0 name: got %q", tools[0].Name)
	}
	if tools[1].Name != "create_pr" {
		t.Errorf("tool 1 name: got %q", tools[1].Name)
	}
}

func TestClient_CallTool(t *testing.T) {
	server := newMockMCPServer()
	defer server.close()

	client := NewClient(server.stdinW, server.stdoutR)
	defer client.Close()

	go func() {
		server.respondTo(ToolCallResult{
			Content: []ToolContent{
				{Type: "text", Text: "Issue #42: Fix login bug"},
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := client.CallTool(ctx, "get_issue", json.RawMessage(`{"number":42}`))
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Issue #42: Fix login bug" {
		t.Errorf("content: got %q", result.Content[0].Text)
	}
}

func TestClient_CallTool_Error(t *testing.T) {
	server := newMockMCPServer()
	defer server.close()

	client := NewClient(server.stdinW, server.stdoutR)
	defer client.Close()

	go func() {
		// Read request
		buf := make([]byte, 4096)
		n, _ := server.stdinR.Read(buf)

		var req jsonRPCRequest
		json.Unmarshal(buf[:n], &req)

		// Respond with error
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "tool not found",
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		server.stdoutW.Write(data)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.CallTool(ctx, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for server error response")
	}
	if !strings.Contains(err.Error(), "tool not found") {
		t.Errorf("error should mention 'tool not found': %v", err)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := newMockMCPServer()
	defer server.close()

	client := NewClient(server.stdinW, server.stdoutR)
	defer client.Close()

	// Drain stdin so writes don't block, but never respond
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := server.stdinR.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Don't respond — let context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.ListTools(ctx)
	if err == nil {
		t.Error("expected error on context timeout")
	}
}

func TestClient_ReadLoopCloses(t *testing.T) {
	server := newMockMCPServer()

	client := NewClient(server.stdinW, server.stdoutR)
	defer client.Close()

	// Close stdout to terminate the read loop
	server.stdoutW.Close()
	server.stdoutR.Close()

	// Wait for read loop to finish
	<-client.done
}
