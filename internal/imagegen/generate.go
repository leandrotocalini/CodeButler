package imagegen

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"
)

// Generate creates an image from a text prompt using OpenAI's gpt-image-1 model.
// Returns raw PNG bytes.
func Generate(apiKey, prompt string) ([]byte, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"model":  "gpt-image-1",
		"prompt": prompt,
		"n":      1,
		"size":   "1024x1024",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/generations", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image generation request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("image API error %d: %s", resp.StatusCode, string(respBody))
	}

	return decodeImageResponse(respBody)
}

// Edit modifies image(s) based on a text prompt using OpenAI's gpt-image-1 model.
// refImages contains raw image bytes (PNG/JPEG) to use as references.
// Returns raw PNG bytes.
func Edit(apiKey, prompt string, refImages [][]byte) ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	for i, img := range refImages {
		mimeType := http.DetectContentType(img)
		if mimeType == "application/octet-stream" {
			mimeType = "image/png" // fallback
		}
		exts, _ := mime.ExtensionsByType(mimeType)
		ext := ".png"
		if len(exts) > 0 {
			ext = exts[0]
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="image%d%s"`, i, ext))
		h.Set("Content-Type", mimeType)
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("create form part: %w", err)
		}
		if _, err := part.Write(img); err != nil {
			return nil, fmt.Errorf("write image data: %w", err)
		}
	}

	if err := w.WriteField("prompt", prompt); err != nil {
		return nil, fmt.Errorf("write prompt field: %w", err)
	}
	if err := w.WriteField("model", "gpt-image-1"); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}

	w.Close()

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/edits", &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image edit request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("image API error %d: %s", resp.StatusCode, string(respBody))
	}

	return decodeImageResponse(respBody)
}

func decodeImageResponse(respBody []byte) ([]byte, error) {
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Data) == 0 || result.Data[0].B64JSON == "" {
		return nil, fmt.Errorf("no image data in response")
	}

	pngData, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	return pngData, nil
}

var urlRegex = regexp.MustCompile(`https?://\S+`)
var multiSpace = regexp.MustCompile(`\s+`)

// ExtractURLs extracts HTTP/HTTPS URLs from text and returns the cleaned text
// (without URLs) and the list of extracted URLs.
func ExtractURLs(text string) (string, []string) {
	raw := urlRegex.FindAllString(text, -1)
	if len(raw) == 0 {
		return text, nil
	}
	// Strip trailing punctuation that the regex may have captured
	var urls []string
	for _, u := range raw {
		u = strings.TrimRight(u, ").,;:!?\"'")
		urls = append(urls, u)
	}
	cleaned := urlRegex.ReplaceAllString(text, "")
	cleaned = strings.TrimSpace(multiSpace.ReplaceAllString(cleaned, " "))
	return cleaned, urls
}

// DownloadURL downloads a URL and returns the raw bytes only if the content
// is an image (image/png, image/jpeg, image/webp). Non-image URLs are skipped.
func DownloadURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download URL status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read URL response: %w", err)
	}

	ct := http.DetectContentType(data)
	if !strings.HasPrefix(ct, "image/") {
		return nil, fmt.Errorf("URL is not an image (detected %s), skipping", ct)
	}

	return data, nil
}
