package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// --- GenerateImage Tool ---

// ImageGenerator generates images from text prompts.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, prompt, size string) (string, error) // returns URL or path
}

// GenerateImageTool allows the Artist to generate images.
type GenerateImageTool struct {
	generator ImageGenerator
}

// NewGenerateImageTool creates a new GenerateImage tool.
func NewGenerateImageTool(generator ImageGenerator) *GenerateImageTool {
	return &GenerateImageTool{generator: generator}
}

func (t *GenerateImageTool) Name() string { return "GenerateImage" }

func (t *GenerateImageTool) Description() string {
	return "Generate an image from a text prompt. Returns the URL or path of the generated image."
}

func (t *GenerateImageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "Text description of the image to generate"
			},
			"size": {
				"type": "string",
				"description": "Image size: 1024x1024, 1792x1024, or 1024x1792 (default: 1024x1024)"
			}
		},
		"required": ["prompt"]
	}`)
}

func (t *GenerateImageTool) RiskTier() RiskTier { return WriteLocal }

func (t *GenerateImageTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("invalid arguments: %s", err),
			IsError:    true,
		}, nil
	}

	if args.Prompt == "" {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    "prompt is required",
			IsError:    true,
		}, nil
	}

	if args.Size == "" {
		args.Size = "1024x1024"
	}

	url, err := t.generator.GenerateImage(ctx, args.Prompt, args.Size)
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("image generation failed: %s", err),
			IsError:    true,
		}, nil
	}

	return ToolResult{
		ToolCallID: call.ID,
		Content:    fmt.Sprintf("Image generated: %s", url),
	}, nil
}

// --- EditImage Tool ---

// ImageEditor edits existing images.
type ImageEditor interface {
	EditImage(ctx context.Context, imagePath, prompt, size string) (string, error) // returns URL or path
}

// EditImageTool allows the Artist to edit existing images.
type EditImageTool struct {
	editor ImageEditor
}

// NewEditImageTool creates a new EditImage tool.
func NewEditImageTool(editor ImageEditor) *EditImageTool {
	return &EditImageTool{editor: editor}
}

func (t *EditImageTool) Name() string { return "EditImage" }

func (t *EditImageTool) Description() string {
	return "Edit an existing image based on a text prompt. Returns the URL or path of the edited image."
}

func (t *EditImageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"image_path": {
				"type": "string",
				"description": "Path to the existing image to edit"
			},
			"prompt": {
				"type": "string",
				"description": "Description of the edits to make"
			},
			"size": {
				"type": "string",
				"description": "Output image size (default: 1024x1024)"
			}
		},
		"required": ["image_path", "prompt"]
	}`)
}

func (t *EditImageTool) RiskTier() RiskTier { return WriteLocal }

func (t *EditImageTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	var args struct {
		ImagePath string `json:"image_path"`
		Prompt    string `json:"prompt"`
		Size      string `json:"size"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("invalid arguments: %s", err),
			IsError:    true,
		}, nil
	}

	if args.ImagePath == "" || args.Prompt == "" {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    "image_path and prompt are required",
			IsError:    true,
		}, nil
	}

	if args.Size == "" {
		args.Size = "1024x1024"
	}

	url, err := t.editor.EditImage(ctx, args.ImagePath, args.Prompt, args.Size)
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("image edit failed: %s", err),
			IsError:    true,
		}, nil
	}

	return ToolResult{
		ToolCallID: call.ID,
		Content:    fmt.Sprintf("Image edited: %s", url),
	}, nil
}
