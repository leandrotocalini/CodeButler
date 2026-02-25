package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const codebutlerDir = ".codebutler"
const configFile = "config.json"

// envVarPattern matches ${VAR_NAME} references in string values.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads both global (~/.codebutler/config.json) and per-repo
// (.codebutler/config.json) configuration files and returns the merged result.
// It walks up from startDir to find the repo root (the directory containing
// .codebutler/). globalDir overrides the default ~/.codebutler/ location
// (useful for testing).
func Load(startDir, globalDir string) (*Config, error) {
	repoRoot, err := findRepoRoot(startDir)
	if err != nil {
		return nil, fmt.Errorf("find repo root: %w", err)
	}

	if globalDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		globalDir = filepath.Join(home, codebutlerDir)
	}

	var cfg Config

	globalPath := filepath.Join(globalDir, configFile)
	if err := loadJSON(globalPath, &cfg.Global); err != nil {
		return nil, fmt.Errorf("load global config %s: %w", globalPath, err)
	}

	repoPath := filepath.Join(repoRoot, codebutlerDir, configFile)
	if err := loadJSON(repoPath, &cfg.Repo); err != nil {
		return nil, fmt.Errorf("load repo config %s: %w", repoPath, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

// findRepoRoot walks up the directory tree from startDir looking for a
// directory that contains a .codebutler/ subdirectory.
func findRepoRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, codebutlerDir)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s directory found (searched from %s to root)", codebutlerDir, startDir)
		}
		dir = parent
	}
}

// loadJSON reads a JSON file, resolves ${VAR} references, and unmarshals it
// into dest.
func loadJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	resolved := resolveEnvVars(string(data))

	if err := json.Unmarshal([]byte(resolved), dest); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	return nil
}

// resolveEnvVars replaces all ${VAR_NAME} patterns in s with the
// corresponding environment variable values. Unset variables resolve to "".
func resolveEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1] // strip ${ and }
		return os.Getenv(varName)
	})
}

// validate checks that all required fields are present.
func validate(cfg *Config) error {
	var errs []string

	if cfg.Global.Slack.BotToken == "" {
		errs = append(errs, "global: slack.botToken is required")
	}
	if cfg.Global.Slack.AppToken == "" {
		errs = append(errs, "global: slack.appToken is required")
	}
	if cfg.Global.OpenRouter.APIKey == "" {
		errs = append(errs, "global: openrouter.apiKey is required")
	}

	if cfg.Repo.Slack.ChannelID == "" {
		errs = append(errs, "repo: slack.channelID is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("missing required fields:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// RepoRoot returns the repo root directory for the given start directory.
// Useful when callers need the path without loading the full config.
func RepoRoot(startDir string) (string, error) {
	return findRepoRoot(startDir)
}
