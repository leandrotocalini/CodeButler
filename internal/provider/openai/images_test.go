package openai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type mockHTTPDoer struct {
	responses []*http.Response
	calls     int
	err       error
}

func (m *mockHTTPDoer) Do(_ *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.responses) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(bytes.NewBufferString("no more responses")),
		}, nil
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func TestClient_Generate_Success(t *testing.T) {
	doer := &mockHTTPDoer{
		responses: []*http.Response{
			jsonResponse(200, `{"data":[{"url":"https://example.com/img.png","revised_prompt":"a cat"}]}`),
		},
	}

	client := NewClient("test-key", WithHTTPClient(doer))
	resp, err := client.Generate(context.Background(), ImageGenerateRequest{
		Prompt: "a cute cat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.URL != "https://example.com/img.png" {
		t.Errorf("url: got %q", resp.URL)
	}
	if resp.RevisedPrompt != "a cat" {
		t.Errorf("revised prompt: got %q", resp.RevisedPrompt)
	}
}

func TestClient_Generate_APIError(t *testing.T) {
	doer := &mockHTTPDoer{
		responses: []*http.Response{
			jsonResponse(400, `{"error":{"message":"bad request"}}`),
		},
	}

	client := NewClient("test-key", WithHTTPClient(doer))
	_, err := client.Generate(context.Background(), ImageGenerateRequest{
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestClient_Generate_NoImages(t *testing.T) {
	doer := &mockHTTPDoer{
		responses: []*http.Response{
			jsonResponse(200, `{"data":[]}`),
		},
	}

	client := NewClient("test-key", WithHTTPClient(doer))
	_, err := client.Generate(context.Background(), ImageGenerateRequest{
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error when no images returned")
	}
}

func TestClient_Generate_DefaultValues(t *testing.T) {
	doer := &mockHTTPDoer{
		responses: []*http.Response{
			jsonResponse(200, `{"data":[{"url":"https://example.com/img.png"}]}`),
		},
	}

	client := NewClient("test-key", WithHTTPClient(doer))
	_, err := client.Generate(context.Background(), ImageGenerateRequest{
		Prompt: "test",
		// Model, Size, N should get defaults
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Edit_Success(t *testing.T) {
	doer := &mockHTTPDoer{
		responses: []*http.Response{
			jsonResponse(200, `{"data":[{"url":"https://example.com/edited.png"}]}`),
		},
	}

	client := NewClient("test-key", WithHTTPClient(doer))
	resp, err := client.Edit(context.Background(), ImageEditRequest{
		Prompt: "add a hat to the cat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.URL != "https://example.com/edited.png" {
		t.Errorf("url: got %q", resp.URL)
	}
}

func TestClient_Edit_Fails(t *testing.T) {
	doer := &mockHTTPDoer{
		responses: []*http.Response{
			jsonResponse(500, `internal server error`),
		},
	}

	client := NewClient("test-key", WithHTTPClient(doer))
	_, err := client.Edit(context.Background(), ImageEditRequest{
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewClient_Options(t *testing.T) {
	client := NewClient("key",
		WithBaseURL("https://custom.api.com"),
	)
	if client.baseURL != "https://custom.api.com" {
		t.Errorf("base URL: got %q", client.baseURL)
	}
	if client.apiKey != "key" {
		t.Errorf("api key: got %q", client.apiKey)
	}
}
