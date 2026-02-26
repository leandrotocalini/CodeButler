package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Client implements the MCP protocol over stdio using JSON-RPC 2.0.
type Client struct {
	stdin  io.Writer
	stdout *bufio.Reader

	mu     sync.Mutex
	nextID atomic.Int64

	// pending tracks in-flight requests waiting for responses
	pendingMu sync.Mutex
	pending   map[int64]chan *jsonRPCResponse

	done chan struct{}
	err  error
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeParams for the MCP initialize handshake.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ClientInfo      ClientInfo `json:"clientInfo"`
	Capabilities    struct{}   `json:"capabilities"`
}

// ClientInfo identifies this MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server's response to initialize.
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ServerInfo      ServerInfo `json:"serverInfo"`
	Capabilities    struct {
		Tools *struct{} `json:"tools,omitempty"`
	} `json:"capabilities"`
}

// ServerInfo identifies the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPTool is a tool exposed by an MCP server.
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolsListResult is the response from tools/list.
type ToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

// ToolCallParams are the parameters for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResult is the response from tools/call.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent is a content block in a tool call result.
type ToolContent struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// NewClient creates an MCP client that communicates over the given
// stdin writer and stdout reader.
func NewClient(stdin io.Writer, stdout io.Reader) *Client {
	c := &Client{
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[int64]chan *jsonRPCResponse),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// readLoop reads JSON-RPC responses from stdout and dispatches them.
func (c *Client) readLoop() {
	defer close(c.done)
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.err = err
			// Resolve all pending requests
			c.pendingMu.Lock()
			for id, ch := range c.pending {
				ch <- &jsonRPCResponse{
					ID:    id,
					Error: &jsonRPCError{Code: -1, Message: fmt.Sprintf("read error: %v", err)},
				}
				delete(c.pending, id)
			}
			c.pendingMu.Unlock()
			return
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // skip malformed lines (notifications, etc.)
		}

		c.pendingMu.Lock()
		if ch, ok := c.pending[resp.ID]; ok {
			ch <- &resp
			delete(c.pending, resp.ID)
		}
		c.pendingMu.Unlock()
	}
}

// call sends a JSON-RPC request and waits for the response.
func (c *Client) call(ctx context.Context, method string, params interface{}) (*jsonRPCResponse, error) {
	id := c.nextID.Add(1)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	ch := make(chan *jsonRPCResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	c.mu.Lock()
	_, err = c.stdin.Write(data)
	c.mu.Unlock()

	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		return resp, nil
	case <-c.done:
		return nil, fmt.Errorf("client closed: %v", c.err)
	}
}

// Initialize performs the MCP protocol handshake.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: ClientInfo{
			Name:    "codebutler",
			Version: "1.0.0",
		},
	}

	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("initialize: server error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse initialize result: %w", err)
	}

	// Send initialized notification (no response expected, but we don't wait)
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      0,
		Method:  "notifications/initialized",
	}
	data, _ := json.Marshal(notif)
	data = append(data, '\n')
	c.mu.Lock()
	_, _ = c.stdin.Write(data)
	c.mu.Unlock()

	return &result, nil
}

// ListTools calls tools/list on the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list: server error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}

	return result.Tools, nil
}

// CallTool calls tools/call on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*ToolCallResult, error) {
	params := ToolCallParams{
		Name:      name,
		Arguments: arguments,
	}

	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/call %s: server error %d: %s", name, resp.Error.Code, resp.Error.Message)
	}

	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/call %s result: %w", name, err)
	}

	return &result, nil
}

// Close cleans up client resources.
func (c *Client) Close() {
	// Pending requests will be resolved when the read loop exits
}
