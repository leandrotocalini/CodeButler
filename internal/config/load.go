package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	repoDirName = ".codebutler"
	repoConfig  = "config.json"
	sessionDir  = "whatsapp-session"
)

// RepoDir returns the path to <dir>/.codebutler/
func RepoDir(dir string) string {
	return filepath.Join(dir, repoDirName)
}

// SessionPath returns the path to <dir>/.codebutler/whatsapp-session/
func SessionPath(dir string) string {
	return filepath.Join(RepoDir(dir), sessionDir)
}

// Exists checks if a repo has been configured (has .codebutler/config.json).
func Exists(dir string) bool {
	path := filepath.Join(RepoDir(dir), repoConfig)
	_, err := os.Stat(path)
	return err == nil
}

// LoadRepo reads <dir>/.codebutler/config.json.
func LoadRepo(dir string) (*RepoConfig, error) {
	path := filepath.Join(RepoDir(dir), repoConfig)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read repo config: %w", err)
	}

	var cfg RepoConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse repo config: %w", err)
	}

	// Apply defaults
	if cfg.Claude.MaxTurns == 0 {
		cfg.Claude.MaxTurns = 10
	}
	if cfg.Claude.Timeout == 0 {
		cfg.Claude.Timeout = 30
	}
	if cfg.WhatsApp.BotPrefix == "" {
		cfg.WhatsApp.BotPrefix = "[BOT]"
	}

	return &cfg, nil
}

// SaveRepo writes <dir>/.codebutler/config.json.
func SaveRepo(dir string, cfg *RepoConfig) error {
	rd := RepoDir(dir)
	if err := os.MkdirAll(rd, 0755); err != nil {
		return fmt.Errorf("create repo config dir: %w", err)
	}

	path := filepath.Join(rd, repoConfig)
	return saveJSON(path, cfg)
}

func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}
