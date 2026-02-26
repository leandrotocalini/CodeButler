// Package openai provides the client for OpenAI image generation and editing
// used by the Artist agent.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// ImageClient is the interface for image generation/editing.
type ImageClient interface {
	Generate(ctx context.Context, req ImageGenerateRequest) (*ImageResponse, error)
	Edit(ctx context.Context, req ImageEditRequest) (*ImageResponse, error)
}

// ImageGenerateRequest is a request to generate an image.
type ImageGenerateRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"` // e.g., "gpt-image-1"
	Size   string `json:"size"`  // e.g., "1024x1024", "1792x1024"
	N      int    `json:"n"`     // number of images (default 1)
}

// ImageEditRequest is a request to edit an existing image.
type ImageEditRequest struct {
	Prompt    string `json:"prompt"`
	Model     string `json:"model"`
	ImagePath string `json:"-"` // local path, not sent in JSON
	Size      string `json:"size"`
}

// ImageResponse contains the generated/edited image data.
type ImageResponse struct {
	URL       string `json:"url,omitempty"`       // URL of generated image
	B64JSON   string `json:"b64_json,omitempty"`  // base64-encoded image
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// Client implements ImageClient using the OpenAI API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient HTTPDoer
	logger     *slog.Logger
}

// HTTPDoer abstracts the HTTP client for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ClientOption configures the OpenAI client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(doer HTTPDoer) ClientOption {
	return func(c *Client) {
		c.httpClient = doer
	}
}

// WithBaseURL sets a custom base URL (for testing).
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithImageLogger sets the logger.
func WithImageLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// NewClient creates a new OpenAI client.
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.openai.com/v1",
		httpClient: http.DefaultClient,
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Generate creates an image from a text prompt.
func (c *Client) Generate(ctx context.Context, req ImageGenerateRequest) (*ImageResponse, error) {
	if req.Model == "" {
		req.Model = "gpt-image-1"
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}
	if req.N <= 0 {
		req.N = 1
	}

	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"size":   req.Size,
		"n":      req.N,
	}

	respBody, err := c.doRequest(ctx, "POST", "/images/generations", body)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	var result struct {
		Data []ImageResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no images returned")
	}

	return &result.Data[0], nil
}

// Edit modifies an existing image based on a prompt.
func (c *Client) Edit(ctx context.Context, req ImageEditRequest) (*ImageResponse, error) {
	if req.Model == "" {
		req.Model = "gpt-image-1"
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}

	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"size":   req.Size,
	}

	respBody, err := c.doRequest(ctx, "POST", "/images/edits", body)
	if err != nil {
		return nil, fmt.Errorf("image edit failed: %w", err)
	}

	var result struct {
		Data []ImageResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no images returned")
	}

	return &result.Data[0], nil
}

// doRequest makes an authenticated HTTP request to the OpenAI API.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	c.logger.Info("openai request", "method", method, "path", path)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
