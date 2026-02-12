package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/imagegen"
	"github.com/leandrotocalini/CodeButler/internal/messenger"
)

type pendingImage struct {
	imageData []byte
	prompt    string
	chatJID   string
	createdAt time.Time
}

type imageCommandHandler struct {
	mu      sync.Mutex
	pending map[string]*pendingImage // key: chatJID
}

func newImageCommandHandler() *imageCommandHandler {
	return &imageCommandHandler{
		pending: make(map[string]*pendingImage),
	}
}

// IsCreateImageCommand returns true if the message text starts with /create-image.
func IsCreateImageCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/create-image")
}

// IsConfirmationReply returns true if the text is "1" or "2" and there's a
// pending image for the given chat.
func (h *imageCommandHandler) IsConfirmationReply(chatJID, text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed != "1" && trimmed != "2" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.pending[chatJID]
	return ok
}

// HandleCreateImage orchestrates image generation: parse prompt, download
// reference images (attachment + URLs), call OpenAI API, send preview,
// store pending state for confirmation.
func (d *Daemon) HandleCreateImage(msg messenger.Message) {
	text := strings.TrimSpace(msg.Content)
	prompt := strings.TrimSpace(strings.TrimPrefix(text, "/create-image"))

	if prompt == "" && !msg.IsImage {
		d.sendMessage(msg.Chat, "Usage: /create-image <description>")
		return
	}

	apiKey := d.repoCfg.OpenAI.APIKey
	if apiKey == "" {
		d.sendMessage(msg.Chat, "No OpenAI API key configured")
		return
	}

	d.log.Info("Image command: %s", prompt)

	// Collect reference images
	var refImages [][]byte

	// 1. Image attachment
	if msg.IsImage {
		imgData, err := d.msger.DownloadImage(msg)
		if err != nil {
			d.log.Error("Failed to download image attachment: %v", err)
		} else {
			refImages = append(refImages, imgData)
			d.log.Info("Downloaded image attachment (%d bytes)", len(imgData))
		}
	}

	// 2. URLs in prompt
	cleanPrompt, urls := imagegen.ExtractURLs(prompt)
	if len(urls) > 0 {
		prompt = cleanPrompt
		for _, u := range urls {
			imgData, err := imagegen.DownloadURL(u)
			if err != nil {
				d.log.Error("Failed to download URL %s: %v", u, err)
			} else {
				refImages = append(refImages, imgData)
				d.log.Info("Downloaded image from URL (%d bytes)", len(imgData))
			}
		}
	}

	if prompt == "" {
		prompt = "edit this image"
	}

	// Show typing
	d.sendPresence(msg.Chat, true)

	// Generate or edit
	var pngData []byte
	var err error

	if len(refImages) > 0 {
		d.log.Info("Editing image with %d reference(s)...", len(refImages))
		pngData, err = imagegen.Edit(apiKey, prompt, refImages)
	} else {
		d.log.Info("Generating image...")
		pngData, err = imagegen.Generate(apiKey, prompt)
	}

	d.sendPresence(msg.Chat, false)

	if err != nil {
		d.log.Error("Image generation failed: %v", err)
		d.sendMessage(msg.Chat, "Image generation failed: "+err.Error())
		return
	}

	d.log.Info("Image generated (%d bytes)", len(pngData))

	// Send preview
	d.sendImage(msg.Chat, pngData, "Reply 1 to save, 2 to discard")

	// Store pending
	createdAt := time.Now()
	d.imgHandler.mu.Lock()
	d.imgHandler.pending[msg.Chat] = &pendingImage{
		imageData: pngData,
		prompt:    prompt,
		chatJID:   msg.Chat,
		createdAt: createdAt,
	}
	d.imgHandler.mu.Unlock()

	// Auto-discard after 5 minutes
	go func() {
		time.Sleep(5 * time.Minute)
		d.imgHandler.mu.Lock()
		p, ok := d.imgHandler.pending[msg.Chat]
		if ok && p.createdAt.Equal(createdAt) {
			delete(d.imgHandler.pending, msg.Chat)
			d.imgHandler.mu.Unlock()
			d.sendMessage(msg.Chat, "Image discarded (timeout)")
			d.log.Info("Image auto-discarded after 5min")
		} else {
			d.imgHandler.mu.Unlock()
		}
	}()
}

// HandleImageConfirmation handles "1" (save) or "2" (discard) replies.
func (d *Daemon) HandleImageConfirmation(msg messenger.Message) {
	text := strings.TrimSpace(msg.Content)

	d.imgHandler.mu.Lock()
	p, ok := d.imgHandler.pending[msg.Chat]
	if !ok {
		d.imgHandler.mu.Unlock()
		return
	}
	delete(d.imgHandler.pending, msg.Chat)
	d.imgHandler.mu.Unlock()

	if text == "1" {
		imagesDir := filepath.Join(config.RepoDir(d.repoDir), "images")
		if err := os.MkdirAll(imagesDir, 0755); err != nil {
			d.log.Error("Failed to create images dir: %v", err)
			d.sendMessage(msg.Chat, "Failed to save: "+err.Error())
			return
		}

		filename := generateFilename(p.prompt)
		path := filepath.Join(imagesDir, filename)

		if err := os.WriteFile(path, p.imageData, 0644); err != nil {
			d.log.Error("Failed to save image: %v", err)
			d.sendMessage(msg.Chat, "Failed to save: "+err.Error())
			return
		}

		d.log.Info("Image saved: %s", path)
		d.sendMessage(msg.Chat, fmt.Sprintf("Image saved: %s", path))
	} else {
		d.log.Info("Image discarded by user")
		d.sendMessage(msg.Chat, "Image discarded")
	}
}

func (d *Daemon) sendImage(chatJID string, pngData []byte, caption string) {
	botPrefix := d.botPrefix
	fullCaption := botPrefix + " " + caption

	if err := d.msger.SendImage(chatJID, pngData, fullCaption); err != nil {
		d.log.Error("Failed to send image: %v", err)
	} else {
		d.log.Info("Image sent (%d bytes)", len(pngData))
	}
}

// generateFilename creates a slugified filename from the prompt with a timestamp.
// e.g. "cyberpunk-cat-20260210-143052.png"
func generateFilename(prompt string) string {
	words := strings.Fields(prompt)
	if len(words) > 5 {
		words = words[:5]
	}
	slug := strings.Join(words, "-")
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return unicode.ToLower(r)
		}
		return -1
	}, slug)
	if len(slug) > 40 {
		slug = slug[:40]
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "image"
	}
	return fmt.Sprintf("%s-%s.png", slug, time.Now().Format("20060102-150405"))
}
