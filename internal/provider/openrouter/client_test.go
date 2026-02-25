package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// noSleep is a sleep function that returns immediately (for fast tests).
func noSleep(_ context.Context, _ time.Duration) {}

// newTestServer creates an httptest server and a client wired to it with no retry delay.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := NewClient("test-key",
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithSleepFunc(noSleep),
	)
	return srv, client
}

// validChatResponse returns a minimal valid ChatResponse JSON.
func validChatResponse(content string) []byte {
	resp := ChatResponse{
		ID:    "chatcmpl-test",
		Model: "anthropic/claude-3.5-sonnet",
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: TokenUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// toolCallChatResponse returns a ChatResponse with tool calls.
func toolCallChatResponse() []byte {
	resp := ChatResponse{
		ID:    "chatcmpl-toolcall",
		Model: "anthropic/claude-3.5-sonnet",
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{
							ID:   "call_abc123",
							Type: "function",
							Function: FunctionCall{
								Name:      "Read",
								Arguments: `{"path": "main.go"}`,
							},
						},
						{
							ID:   "call_def456",
							Type: "function",
							Function: FunctionCall{
								Name:      "Grep",
								Arguments: `{"pattern": "func main"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: TokenUsage{
			PromptTokens:     20,
			CompletionTokens: 10,
			TotalTokens:      30,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestChatCompletion_TextResponse(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected application/json, got %s", got)
		}

		// Verify request body.
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "anthropic/claude-3.5-sonnet" {
			t.Errorf("expected model anthropic/claude-3.5-sonnet, got %s", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(req.Messages))
		}

		w.WriteHeader(http.StatusOK)
		w.Write(validChatResponse("Hello, world!"))
	})

	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "anthropic/claude-3.5-sonnet",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.TextContent() != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", resp.TextContent())
	}
	if resp.HasToolCalls() {
		t.Error("expected no tool calls")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
}

func TestChatCompletion_ToolCallResponse(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(toolCallChatResponse())
	})

	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "anthropic/claude-3.5-sonnet",
		Messages: []Message{
			{Role: "user", Content: "Read main.go"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "Read",
					Description: "Read a file",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.HasToolCalls() {
		t.Fatal("expected tool calls")
	}

	calls := resp.ToolCallsContent()
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}

	if calls[0].ID != "call_abc123" {
		t.Errorf("expected call_abc123, got %s", calls[0].ID)
	}
	if calls[0].Function.Name != "Read" {
		t.Errorf("expected Read, got %s", calls[0].Function.Name)
	}
	if calls[1].Function.Name != "Grep" {
		t.Errorf("expected Grep, got %s", calls[1].Function.Name)
	}
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestChatCompletion_ToolCallWithToolResults(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify tool result message format.
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				if msg.ToolCallID == "" {
					t.Error("tool message missing tool_call_id")
				}
				if msg.Content == "" {
					t.Error("tool message missing content")
				}
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write(validChatResponse("File content: package main"))
	})

	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model: "anthropic/claude-3.5-sonnet",
		Messages: []Message{
			{Role: "user", Content: "Read main.go"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{ID: "call_abc123", Type: "function", Function: FunctionCall{Name: "Read", Arguments: `{"path":"main.go"}`}},
				},
			},
			{Role: "tool", ToolCallID: "call_abc123", Content: "package main\n\nfunc main() {}"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TextContent() != "File content: package main" {
		t.Errorf("unexpected content: %q", resp.TextContent())
	}
}

func TestChatCompletion_RetryOn429(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(validChatResponse("success after retry"))
	})

	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TextContent() != "success after retry" {
		t.Errorf("expected 'success after retry', got %q", resp.TextContent())
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestChatCompletion_RetryOn502(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":{"message":"bad gateway"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(validChatResponse("recovered"))
	})

	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TextContent() != "recovered" {
		t.Errorf("expected 'recovered', got %q", resp.TextContent())
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestChatCompletion_AuthError_NoRetry(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrAuth {
		t.Errorf("expected ErrAuth, got %s", classified.Type)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts.Load())
	}
}

func TestChatCompletion_ContentFiltered_NoRetry(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"content_filter triggered","code":"content_filter"}}`))
	})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrContentFiltered {
		t.Errorf("expected ErrContentFiltered, got %s", classified.Type)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts.Load())
	}
}

func TestChatCompletion_ContextLengthExceeded(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"maximum context length exceeded","code":"context_length_exceeded"}}`))
	})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrContextTooLong {
		t.Errorf("expected ErrContextTooLong, got %s", classified.Type)
	}
	// context_length_exceeded allows 1 retry.
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", attempts.Load())
	}
}

func TestChatCompletion_MalformedJSON_RetryThenFail(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrMalformedResponse {
		t.Errorf("expected ErrMalformedResponse, got %s", classified.Type)
	}
	// Malformed response allows 3 retries.
	if attempts.Load() != 4 {
		t.Errorf("expected 4 attempts (3 retries), got %d", attempts.Load())
	}
}

func TestChatCompletion_EmptyChoices(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test","model":"test","choices":[],"usage":{}}`))
	})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}

	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrMalformedResponse {
		t.Errorf("expected ErrMalformedResponse, got %s", classified.Type)
	}
}

func TestChatCompletion_CircuitBreaker_TripsAfter3Failures(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"service unavailable"}}`))
	})

	model := "circuit-test-model"

	// First 3 calls: provider_overloaded with retries.
	// Each call retries up to 5 times (6 attempts total), but all fail.
	// After 3 consecutive failures (from the breaker's perspective) the breaker opens.
	for i := 0; i < 3; i++ {
		_, err := client.ChatCompletion(context.Background(), ChatRequest{
			Model:    model,
			Messages: []Message{{Role: "user", Content: "test"}},
		})
		if err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
	}

	// Next call should fail immediately (circuit open).
	beforeAttempts := attempts.Load()
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    model,
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	// No additional HTTP requests should have been made.
	if attempts.Load() != beforeAttempts {
		t.Error("expected no additional HTTP requests when circuit is open")
	}
}

func TestChatCompletion_CircuitBreaker_PerModel(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model == "bad-model" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"message":"service unavailable"}}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(validChatResponse("ok from good model"))
	})

	// Trip breaker for bad-model.
	for i := 0; i < 3; i++ {
		client.ChatCompletion(context.Background(), ChatRequest{
			Model:    "bad-model",
			Messages: []Message{{Role: "user", Content: "test"}},
		})
	}

	// good-model should still work.
	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "good-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("good-model should work when bad-model circuit is open: %v", err)
	}
	if resp.TextContent() != "ok from good model" {
		t.Errorf("unexpected response: %q", resp.TextContent())
	}
}

func TestChatCompletion_CircuitBreaker_AuthDoesNotTrip(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	})

	model := "auth-test-model"

	// 3 auth errors should NOT trip the circuit breaker.
	for i := 0; i < 3; i++ {
		_, err := client.ChatCompletion(context.Background(), ChatRequest{
			Model:    model,
			Messages: []Message{{Role: "user", Content: "test"}},
		})
		if err == nil {
			t.Fatal("expected error")
		}
		classified, ok := err.(*ClassifiedError)
		if !ok {
			t.Fatalf("expected ClassifiedError, got %T", err)
		}
		if classified.Type != ErrAuth {
			t.Errorf("expected ErrAuth, got %s", classified.Type)
		}
	}

	// The 4th call should still return auth error (not circuit breaker error).
	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    model,
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrAuth {
		t.Errorf("expected ErrAuth (not circuit breaker), got %s", classified.Type)
	}
}

func TestChatCompletion_ContextCanceled(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write(validChatResponse("too late"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := client.ChatCompletion(ctx, ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChatCompletion_TokenUsageExtraction(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{
			ID:    "chatcmpl-usage",
			Model: "openai/gpt-4o",
			Choices: []Choice{
				{
					Index:        0,
					Message:      Message{Role: "assistant", Content: "hi"},
					FinishReason: "stop",
				},
			},
			Usage: TokenUsage{
				PromptTokens:     150,
				CompletionTokens: 42,
				TotalTokens:      192,
			},
		}
		b, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	})

	resp, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "openai/gpt-4o",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Usage.PromptTokens != 150 {
		t.Errorf("expected 150 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 42 {
		t.Errorf("expected 42 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 192 {
		t.Errorf("expected 192 total tokens, got %d", resp.Usage.TotalTokens)
	}
	if resp.Model != "openai/gpt-4o" {
		t.Errorf("expected model openai/gpt-4o, got %s", resp.Model)
	}
}

func TestChatResponse_Helpers(t *testing.T) {
	t.Run("empty response", func(t *testing.T) {
		resp := &ChatResponse{}
		if resp.TextContent() != "" {
			t.Errorf("expected empty text, got %q", resp.TextContent())
		}
		if resp.HasToolCalls() {
			t.Error("expected no tool calls")
		}
		if resp.ToolCallsContent() != nil {
			t.Error("expected nil tool calls")
		}
	})

	t.Run("text response", func(t *testing.T) {
		resp := &ChatResponse{
			Choices: []Choice{
				{Message: Message{Role: "assistant", Content: "hello"}},
			},
		}
		if resp.TextContent() != "hello" {
			t.Errorf("expected 'hello', got %q", resp.TextContent())
		}
		if resp.HasToolCalls() {
			t.Error("expected no tool calls")
		}
	})

	t.Run("tool call response", func(t *testing.T) {
		resp := &ChatResponse{
			Choices: []Choice{
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{ID: "tc1", Type: "function", Function: FunctionCall{Name: "Read"}},
						},
					},
				},
			},
		}
		if resp.TextContent() != "" {
			t.Errorf("expected empty text, got %q", resp.TextContent())
		}
		if !resp.HasToolCalls() {
			t.Error("expected tool calls")
		}
		if len(resp.ToolCallsContent()) != 1 {
			t.Errorf("expected 1 tool call, got %d", len(resp.ToolCallsContent()))
		}
	})
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		headers    map[string]string
		wantType   ErrorType
	}{
		{
			name:       "429 rate limit",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"message":"rate limited"}}`,
			headers:    map[string]string{"Retry-After": "5"},
			wantType:   ErrRateLimit,
		},
		{
			name:       "502 bad gateway",
			statusCode: http.StatusBadGateway,
			body:       `{"error":{"message":"bad gateway"}}`,
			wantType:   ErrProviderOverloaded,
		},
		{
			name:       "503 service unavailable",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"error":{"message":"service unavailable"}}`,
			wantType:   ErrProviderOverloaded,
		},
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"message":"invalid api key"}}`,
			wantType:   ErrAuth,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"message":"forbidden"}}`,
			wantType:   ErrAuth,
		},
		{
			name:       "400 context_length_exceeded",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"message":"maximum context length exceeded","code":"context_length_exceeded"}}`,
			wantType:   ErrContextTooLong,
		},
		{
			name:       "400 content_filter",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"message":"content filtered","code":"content_filter"}}`,
			wantType:   ErrContentFiltered,
		},
		{
			name:       "400 too many tokens",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"message":"This model's maximum context length is 128000 tokens. You have too many tokens."}}`,
			wantType:   ErrContextTooLong,
		},
		{
			name:       "400 generic",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"message":"something else"}}`,
			wantType:   ErrUnknown,
		},
		{
			name:       "500 unknown",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":{"message":"internal error"}}`,
			wantType:   ErrUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			})

			_, err := client.ChatCompletion(context.Background(), ChatRequest{
				Model:    "test-model",
				Messages: []Message{{Role: "user", Content: "test"}},
			})
			if err == nil {
				t.Fatal("expected error")
			}

			classified, ok := err.(*ClassifiedError)
			if !ok {
				t.Fatalf("expected ClassifiedError, got %T: %v", err, err)
			}
			if classified.Type != tt.wantType {
				t.Errorf("expected %s, got %s", tt.wantType, classified.Type)
			}
		})
	}
}

func TestClassifiedError_String(t *testing.T) {
	t.Run("without retry after", func(t *testing.T) {
		err := &ClassifiedError{
			Type:       ErrAuth,
			StatusCode: 401,
			Message:    "invalid key",
		}
		want := "openrouter auth_error (HTTP 401): invalid key"
		if err.Error() != want {
			t.Errorf("expected %q, got %q", want, err.Error())
		}
	})

	t.Run("with retry after", func(t *testing.T) {
		err := &ClassifiedError{
			Type:       ErrRateLimit,
			StatusCode: 429,
			Message:    "rate limited",
			RetryAfter: 5 * time.Second,
		}
		got := err.Error()
		if got != "openrouter rate_limit (HTTP 429): rate limited (retry after 5s)" {
			t.Errorf("unexpected: %q", got)
		}
	})
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"5", 5 * time.Second},
		{"60", 60 * time.Second},
		{"", 0},
		{"not-a-number", 0},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := parseRetryAfter(tt.header)
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestNewClient_Options(t *testing.T) {
	client := NewClient("my-key")
	if client.apiKey != "my-key" {
		t.Errorf("expected api key 'my-key', got %q", client.apiKey)
	}
	if client.baseURL != defaultBaseURL {
		t.Errorf("expected default base URL, got %q", client.baseURL)
	}

	customClient := &http.Client{Timeout: 5 * time.Second}
	client2 := NewClient("key2", WithBaseURL("https://custom.api"), WithHTTPClient(customClient))
	if client2.baseURL != "https://custom.api" {
		t.Errorf("expected custom base URL, got %q", client2.baseURL)
	}
	if client2.httpClient != customClient {
		t.Error("expected custom HTTP client")
	}
}

func TestErrorType_String(t *testing.T) {
	tests := []struct {
		errType ErrorType
		want    string
	}{
		{ErrRateLimit, "rate_limit"},
		{ErrProviderOverloaded, "provider_overloaded"},
		{ErrContextTooLong, "context_length_exceeded"},
		{ErrContentFiltered, "content_filter"},
		{ErrAuth, "auth_error"},
		{ErrMalformedResponse, "malformed_response"},
		{ErrTimeout, "timeout"},
		{ErrUnknown, "unknown"},
		{ErrorType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.errType.String(); got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestClassifiedError_Retryable(t *testing.T) {
	tests := []struct {
		errType   ErrorType
		retryable bool
		maxRetry  int
	}{
		{ErrRateLimit, true, 5},
		{ErrProviderOverloaded, true, 5},
		{ErrContextTooLong, true, 1},
		{ErrMalformedResponse, true, 3},
		{ErrTimeout, true, 1},
		{ErrContentFiltered, false, 0},
		{ErrAuth, false, 0},
		{ErrUnknown, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.errType.String(), func(t *testing.T) {
			e := &ClassifiedError{Type: tt.errType}
			if e.Retryable() != tt.retryable {
				t.Errorf("Retryable() = %v, want %v", e.Retryable(), tt.retryable)
			}
			if e.MaxRetries() != tt.maxRetry {
				t.Errorf("MaxRetries() = %d, want %d", e.MaxRetries(), tt.maxRetry)
			}
		})
	}
}

func TestChatCompletion_RetryExhausted429(t *testing.T) {
	var attempts atomic.Int32
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	})

	_, err := client.ChatCompletion(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	classified, ok := err.(*ClassifiedError)
	if !ok {
		t.Fatalf("expected ClassifiedError, got %T", err)
	}
	if classified.Type != ErrRateLimit {
		t.Errorf("expected ErrRateLimit, got %s", classified.Type)
	}
	// 1 initial + 5 retries = 6 attempts total.
	if attempts.Load() != 6 {
		t.Errorf("expected 6 attempts, got %d", attempts.Load())
	}
}
