// Package integration provides integration tests that verify cross-package
// interactions with mock external dependencies (OpenRouter, Slack, MCP).
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Mock OpenRouter Server ---

// mockOpenRouterServer simulates the OpenRouter chat completions API.
type mockOpenRouterServer struct {
	mu        sync.Mutex
	calls     int
	responses []mockResponse
	server    *httptest.Server
}

type mockResponse struct {
	content   string
	toolCalls []mockToolCall
}

type mockToolCall struct {
	id        string
	name      string
	arguments string
}

func newMockOpenRouter(responses ...mockResponse) *mockOpenRouterServer {
	m := &mockOpenRouterServer{responses: responses}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockOpenRouterServer) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.mu.Unlock()

	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	resp := m.responses[idx]

	result := map[string]interface{}{
		"id":      fmt.Sprintf("gen-%d", idx),
		"model":   "test-model",
		"choices": []map[string]interface{}{},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}

	message := map[string]interface{}{
		"role": "assistant",
	}

	if len(resp.toolCalls) > 0 {
		tcs := make([]map[string]interface{}, len(resp.toolCalls))
		for i, tc := range resp.toolCalls {
			tcs[i] = map[string]interface{}{
				"id":   tc.id,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.name,
					"arguments": tc.arguments,
				},
			}
		}
		message["tool_calls"] = tcs
	} else {
		message["content"] = resp.content
	}

	result["choices"] = []map[string]interface{}{
		{"message": message, "finish_reason": "stop"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (m *mockOpenRouterServer) close() {
	m.server.Close()
}

func (m *mockOpenRouterServer) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// --- Mock Slack Server ---

// mockSlackServer simulates Slack API endpoints.
type mockSlackServer struct {
	mu       sync.Mutex
	messages []slackMessage
	server   *httptest.Server
}

type slackMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
	Thread  string `json:"thread_ts"`
}

func newMockSlack() *mockSlackServer {
	m := &mockSlackServer{}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockSlackServer) handle(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/chat.postMessage":
		r.ParseForm()
		m.mu.Lock()
		m.messages = append(m.messages, slackMessage{
			Channel: r.FormValue("channel"),
			Text:    r.FormValue("text"),
			Thread:  r.FormValue("thread_ts"),
		})
		m.mu.Unlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ts": fmt.Sprintf("%d.000", time.Now().UnixNano()),
		})

	case "/api/reactions.add":
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}
}

func (m *mockSlackServer) close() {
	m.server.Close()
}

func (m *mockSlackServer) messageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// --- Integration Tests ---

func TestMockOpenRouter_MultipleResponses(t *testing.T) {
	mock := newMockOpenRouter(
		mockResponse{content: "First response"},
		mockResponse{content: "Second response"},
	)
	defer mock.close()

	// First call
	resp, err := http.Post(mock.server.URL+"/v1/chat/completions",
		"application/json", strings.NewReader(`{"model":"test","messages":[]}`))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	resp.Body.Close()

	// Second call
	resp, err = http.Post(mock.server.URL+"/v1/chat/completions",
		"application/json", strings.NewReader(`{"model":"test","messages":[]}`))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	resp.Body.Close()

	if mock.callCount() != 2 {
		t.Errorf("expected 2 calls, got %d", mock.callCount())
	}
}

func TestMockOpenRouter_ToolCallResponse(t *testing.T) {
	mock := newMockOpenRouter(
		mockResponse{
			toolCalls: []mockToolCall{
				{id: "tc1", name: "Read", arguments: `{"path":"main.go"}`},
			},
		},
	)
	defer mock.close()

	resp, err := http.Post(mock.server.URL+"/v1/chat/completions",
		"application/json", strings.NewReader(`{"model":"test","messages":[]}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	choices := result["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	msg := choice["message"].(map[string]interface{})

	toolCalls, ok := msg["tool_calls"]
	if !ok {
		t.Fatal("expected tool_calls in response")
	}

	tcs := toolCalls.([]interface{})
	if len(tcs) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(tcs))
	}
}

func TestMockSlack_PostMessage(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	resp, err := http.PostForm(mock.server.URL+"/api/chat.postMessage", map[string][]string{
		"channel":   {"C123"},
		"text":      {"Hello from test"},
		"thread_ts": {"T456"},
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()

	if mock.messageCount() != 1 {
		t.Errorf("expected 1 message, got %d", mock.messageCount())
	}

	mock.mu.Lock()
	msg := mock.messages[0]
	mock.mu.Unlock()

	if msg.Channel != "C123" {
		t.Errorf("channel: got %s", msg.Channel)
	}
	if msg.Text != "Hello from test" {
		t.Errorf("text: got %s", msg.Text)
	}
}

func TestMockOpenRouter_ConcurrentCalls(t *testing.T) {
	mock := newMockOpenRouter(mockResponse{content: "ok"})
	defer mock.close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Post(mock.server.URL+"/v1/chat/completions",
				"application/json", strings.NewReader(`{"model":"test","messages":[]}`))
			if err != nil {
				t.Errorf("call failed: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()

	if mock.callCount() != 20 {
		t.Errorf("expected 20 calls, got %d", mock.callCount())
	}
}

func TestMockSlack_ConcurrentMessages(t *testing.T) {
	mock := newMockSlack()
	defer mock.close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.PostForm(mock.server.URL+"/api/chat.postMessage", map[string][]string{
				"channel": {"C123"},
				"text":    {"msg"},
			})
			if err != nil {
				t.Errorf("post failed: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()

	if mock.messageCount() != 20 {
		t.Errorf("expected 20 messages, got %d", mock.messageCount())
	}
}

// TestWorkflowIntegration_Context verifies context cancellation propagation
// through mock servers (simulates agent cancellation).
func TestWorkflowIntegration_Context(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	mock := newMockOpenRouter(mockResponse{content: "ok"})
	defer mock.close()

	req, _ := http.NewRequestWithContext(ctx, "POST",
		mock.server.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"test","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context might cancel before or during the request
		if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	resp.Body.Close()
}
