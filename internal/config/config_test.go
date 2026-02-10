package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()

	cfg := &RepoConfig{
		WhatsApp: WhatsAppConfig{
			GroupJID:  "120363123456789012@g.us",
			GroupName: "Test Group",
			BotPrefix: "[TEST]",
		},
		Claude: ClaudeConfig{
			MaxTurns: 15,
			Timeout:  60,
		},
	}

	if err := SaveRepo(dir, cfg); err != nil {
		t.Fatalf("SaveRepo failed: %v", err)
	}

	// Check file was created
	path := filepath.Join(dir, ".codebutler", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Config file not created: %v", err)
	}

	loaded, err := LoadRepo(dir)
	if err != nil {
		t.Fatalf("LoadRepo failed: %v", err)
	}

	if loaded.WhatsApp.GroupName != cfg.WhatsApp.GroupName {
		t.Errorf("GroupName: got %q, want %q", loaded.WhatsApp.GroupName, cfg.WhatsApp.GroupName)
	}
	if loaded.WhatsApp.GroupJID != cfg.WhatsApp.GroupJID {
		t.Errorf("GroupJID: got %q, want %q", loaded.WhatsApp.GroupJID, cfg.WhatsApp.GroupJID)
	}
	if loaded.Claude.MaxTurns != cfg.Claude.MaxTurns {
		t.Errorf("MaxTurns: got %d, want %d", loaded.Claude.MaxTurns, cfg.Claude.MaxTurns)
	}
}

func TestRepoDefaults(t *testing.T) {
	dir := t.TempDir()

	// Save config with zero values for Claude
	cfg := &RepoConfig{
		WhatsApp: WhatsAppConfig{
			GroupJID:  "123@g.us",
			GroupName: "Test",
		},
	}

	if err := SaveRepo(dir, cfg); err != nil {
		t.Fatalf("SaveRepo failed: %v", err)
	}

	loaded, err := LoadRepo(dir)
	if err != nil {
		t.Fatalf("LoadRepo failed: %v", err)
	}

	if loaded.Claude.MaxTurns != 10 {
		t.Errorf("Default MaxTurns: got %d, want 10", loaded.Claude.MaxTurns)
	}
	if loaded.Claude.Timeout != 30 {
		t.Errorf("Default Timeout: got %d, want 30", loaded.Claude.Timeout)
	}
	if loaded.WhatsApp.BotPrefix != "[BOT]" {
		t.Errorf("Default BotPrefix: got %q, want %q", loaded.WhatsApp.BotPrefix, "[BOT]")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Error("Exists should return false for unconfigured dir")
	}

	cfg := &RepoConfig{
		WhatsApp: WhatsAppConfig{
			GroupJID:  "123@g.us",
			GroupName: "Test",
		},
	}
	if err := SaveRepo(dir, cfg); err != nil {
		t.Fatalf("SaveRepo failed: %v", err)
	}

	if !Exists(dir) {
		t.Error("Exists should return true after SaveRepo")
	}
}
