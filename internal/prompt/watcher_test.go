package prompt

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPromptCache_Get_BuildsPrompt(t *testing.T) {
	seedsDir := setupSeedsDir(t)
	skillsDir := setupSkillsDir(t)

	cache := NewPromptCache(seedsDir, skillsDir, "pm", WithCacheLogger(slog.Default()))

	prompt, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !containsStr(prompt, "PM Agent") {
		t.Error("missing PM seed")
	}
	if !containsStr(prompt, "Global") {
		t.Error("missing global")
	}
	if !containsStr(prompt, "Workflows") {
		t.Error("missing workflows")
	}
	if !containsStr(prompt, "Available Skills") {
		t.Error("missing skill index")
	}
}

func TestPromptCache_Get_CachesResult(t *testing.T) {
	seedsDir := setupSeedsDir(t)
	skillsDir := t.TempDir()

	cache := NewPromptCache(seedsDir, skillsDir, "coder", WithCacheLogger(slog.Default()))

	prompt1, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prompt2, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt1 != prompt2 {
		t.Error("expected same prompt from cache")
	}
}

func TestPromptCache_Get_RebuildOnChange(t *testing.T) {
	seedsDir := setupSeedsDir(t)
	skillsDir := t.TempDir()

	cache := NewPromptCache(seedsDir, skillsDir, "coder", WithCacheLogger(slog.Default()))

	prompt1, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify the seed file (ensure mod time changes)
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(seedsDir, "coder.md"), []byte("# Coder Agent v2\n\nUpdated.\n"), 0o644)

	prompt2, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt1 == prompt2 {
		t.Error("expected different prompt after file change")
	}
	if !containsStr(prompt2, "Coder Agent v2") {
		t.Error("expected updated content")
	}
}

func TestPromptCache_Invalidate(t *testing.T) {
	seedsDir := setupSeedsDir(t)
	skillsDir := t.TempDir()

	cache := NewPromptCache(seedsDir, skillsDir, "coder", WithCacheLogger(slog.Default()))

	_, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cache.Invalidate()

	// Should rebuild after invalidate
	prompt, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt == "" {
		t.Fatal("expected non-empty prompt after invalidate")
	}
}

func TestPromptCache_CoderNoSkills(t *testing.T) {
	seedsDir := setupSeedsDir(t)
	skillsDir := setupSkillsDir(t)

	cache := NewPromptCache(seedsDir, skillsDir, "coder", WithCacheLogger(slog.Default()))

	prompt, err := cache.Get()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if containsStr(prompt, "Available Skills") {
		t.Error("coder should not have skill index")
	}
}
