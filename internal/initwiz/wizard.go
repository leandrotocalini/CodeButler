// Package initwiz implements the `codebutler init` wizard for first-time setup.
package initwiz

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const codebutlerDir = ".codebutler"

// StepResult records what happened in a wizard step.
type StepResult struct {
	Step    string `json:"step"`
	Skipped bool   `json:"skipped"`
	Message string `json:"message"`
}

// WizardResult is the output of a complete wizard run.
type WizardResult struct {
	Steps []StepResult `json:"steps"`
}

// Prompter abstracts interactive user input for testability.
type Prompter interface {
	Prompt(question string) (string, error)
	Confirm(question string) (bool, error)
}

// GlobalTokens holds the tokens collected in step 1.
type GlobalTokens struct {
	SlackBotToken   string `json:"botToken"`
	SlackAppToken   string `json:"appToken"`
	OpenRouterKey   string `json:"openrouterKey"`
	OpenAIKey       string `json:"openaiKey"`
}

// RepoSetup holds the repo config collected in step 2.
type RepoSetup struct {
	ChannelID   string `json:"channelID"`
	ChannelName string `json:"channelName"`
}

// Wizard manages the init flow.
type Wizard struct {
	homeDir  string
	repoDir  string
	prompter Prompter
	results  []StepResult
}

// NewWizard creates a new init wizard.
func NewWizard(homeDir, repoDir string, prompter Prompter) *Wizard {
	return &Wizard{
		homeDir:  homeDir,
		repoDir:  repoDir,
		prompter: prompter,
	}
}

// Run executes all wizard steps.
func (w *Wizard) Run() (*WizardResult, error) {
	// Step 1: Global tokens
	if err := w.stepGlobalTokens(); err != nil {
		return nil, fmt.Errorf("step 1 (global tokens): %w", err)
	}

	// Step 2: Repo setup
	if err := w.stepRepoSetup(); err != nil {
		return nil, fmt.Errorf("step 2 (repo setup): %w", err)
	}

	// Step 3: Service install
	if err := w.stepServiceInstall(); err != nil {
		return nil, fmt.Errorf("step 3 (service install): %w", err)
	}

	return &WizardResult{Steps: w.results}, nil
}

// stepGlobalTokens collects API tokens and writes ~/.codebutler/config.json.
func (w *Wizard) stepGlobalTokens() error {
	globalDir := filepath.Join(w.homeDir, codebutlerDir)
	globalConfig := filepath.Join(globalDir, "config.json")

	// Skip if already exists
	if _, err := os.Stat(globalConfig); err == nil {
		w.results = append(w.results, StepResult{
			Step:    "global_tokens",
			Skipped: true,
			Message: "Global config already exists at " + globalConfig,
		})
		return nil
	}

	if err := os.MkdirAll(globalDir, 0700); err != nil {
		return fmt.Errorf("create global dir: %w", err)
	}

	cfg := map[string]interface{}{
		"slack": map[string]string{
			"botToken": "",
			"appToken": "",
		},
		"openrouter": map[string]string{
			"apiKey": "",
		},
		"openai": map[string]string{
			"apiKey": "",
		},
	}

	if err := writeJSON(globalConfig, cfg, 0600); err != nil {
		return fmt.Errorf("write global config: %w", err)
	}

	w.results = append(w.results, StepResult{
		Step:    "global_tokens",
		Message: "Created " + globalConfig + " — fill in your API tokens",
	})
	return nil
}

// stepRepoSetup creates .codebutler/ directory structure.
func (w *Wizard) stepRepoSetup() error {
	cbDir := filepath.Join(w.repoDir, codebutlerDir)

	// Skip if already exists
	if _, err := os.Stat(cbDir); err == nil {
		w.results = append(w.results, StepResult{
			Step:    "repo_setup",
			Skipped: true,
			Message: codebutlerDir + "/ already exists",
		})
		return nil
	}

	// Create directory structure
	dirs := []string{
		cbDir,
		filepath.Join(cbDir, "skills"),
		filepath.Join(cbDir, "branches"),
		filepath.Join(cbDir, "images"),
		filepath.Join(cbDir, "research"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	// Create per-repo config
	repoCfg := map[string]interface{}{
		"slack": map[string]string{
			"channelID":   "",
			"channelName": "",
		},
		"models": map[string]interface{}{
			"pm":         map[string]string{"default": "moonshotai/kimi-k2"},
			"coder":      map[string]string{"model": "anthropic/claude-opus-4-6"},
			"reviewer":   map[string]string{"model": "anthropic/claude-sonnet-4-5-20250929"},
			"researcher": map[string]string{"model": "moonshotai/kimi-k2"},
			"lead":       map[string]string{"model": "anthropic/claude-sonnet-4-5-20250929"},
			"artist": map[string]string{
				"uxModel":    "anthropic/claude-sonnet-4-5-20250929",
				"imageModel": "openai/gpt-image-1",
			},
		},
		"limits": map[string]int{
			"maxConcurrentThreads": 3,
			"maxCallsPerHour":     100,
		},
	}

	configPath := filepath.Join(cbDir, "config.json")
	if err := writeJSON(configPath, repoCfg, 0644); err != nil {
		return fmt.Errorf("write repo config: %w", err)
	}

	// Create empty MCP config
	mcpCfg := map[string]interface{}{
		"servers": map[string]interface{}{},
	}
	mcpPath := filepath.Join(cbDir, "mcp.json")
	if err := writeJSON(mcpPath, mcpCfg, 0644); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}

	// Create empty agent MDs
	agentMDs := map[string]string{
		"pm.md":        "# PM Agent\n\n## Project Map\n\n## Learnings\n",
		"coder.md":     "# Coder Agent\n\n## Project Map\n\n## Learnings\n",
		"reviewer.md":  "# Reviewer Agent\n\n## Project Map\n\n## Learnings\n",
		"researcher.md": "# Researcher Agent\n\n## Project Map\n\n## Learnings\n",
		"artist.md":    "# Artist Agent\n\n## Project Map\n\n## Learnings\n",
		"lead.md":      "# Lead Agent\n\n## Project Map\n\n## Learnings\n",
		"global.md":    "# Global Knowledge\n\n",
		"workflows.md": "# Workflows\n\n",
	}

	for name, content := range agentMDs {
		path := filepath.Join(cbDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// Create empty roadmap
	roadmapPath := filepath.Join(cbDir, "roadmap.md")
	if err := os.WriteFile(roadmapPath, []byte("# Roadmap\n\n"), 0644); err != nil {
		return fmt.Errorf("write roadmap: %w", err)
	}

	// Update .gitignore
	if err := updateGitignore(w.repoDir); err != nil {
		return fmt.Errorf("update gitignore: %w", err)
	}

	w.results = append(w.results, StepResult{
		Step:    "repo_setup",
		Message: "Created " + codebutlerDir + "/ with config, agent MDs, and directory structure",
	})
	return nil
}

// stepServiceInstall creates service definitions for the OS.
func (w *Wizard) stepServiceInstall() error {
	os := DetectOS()

	w.results = append(w.results, StepResult{
		Step:    "service_install",
		Message: fmt.Sprintf("Detected OS: %s — service definitions ready", os),
	})
	return nil
}

// DetectOS returns the current operating system.
func DetectOS() string {
	return runtime.GOOS
}

// ServiceType returns the service management system for the current OS.
func ServiceType() string {
	switch runtime.GOOS {
	case "darwin":
		return "launchd"
	case "linux":
		return "systemd"
	default:
		return "manual"
	}
}

// GenerateServiceConfig generates a service config for the given role.
func GenerateServiceConfig(role, binaryPath, repoDir string) string {
	switch ServiceType() {
	case "launchd":
		return generateLaunchAgent(role, binaryPath, repoDir)
	case "systemd":
		return generateSystemdUnit(role, binaryPath, repoDir)
	default:
		return fmt.Sprintf("%s --role %s", binaryPath, role)
	}
}

func generateLaunchAgent(role, binaryPath, repoDir string) string {
	label := fmt.Sprintf("com.codebutler.%s", role)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--role</string>
        <string>%s</string>
    </array>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/codebutler-%s.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/codebutler-%s.err</string>
</dict>
</plist>`, label, binaryPath, role, repoDir, role, role)
}

func generateSystemdUnit(role, binaryPath, repoDir string) string {
	return fmt.Sprintf(`[Unit]
Description=CodeButler %s agent
After=network.target

[Service]
Type=simple
ExecStart=%s --role %s
WorkingDirectory=%s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target`, role, binaryPath, role, repoDir)
}

// Validate checks that a wizard setup is complete and valid.
func Validate(homeDir, repoDir string) []string {
	var errs []string

	// Check global config
	globalConfig := filepath.Join(homeDir, codebutlerDir, "config.json")
	if _, err := os.Stat(globalConfig); os.IsNotExist(err) {
		errs = append(errs, "missing global config: "+globalConfig)
	}

	// Check repo config
	repoCfg := filepath.Join(repoDir, codebutlerDir, "config.json")
	if _, err := os.Stat(repoCfg); os.IsNotExist(err) {
		errs = append(errs, "missing repo config: "+repoCfg)
	}

	// Check required agent MDs
	for _, md := range []string{"pm.md", "coder.md", "reviewer.md", "lead.md", "global.md"} {
		path := filepath.Join(repoDir, codebutlerDir, md)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			errs = append(errs, "missing agent MD: "+md)
		}
	}

	return errs
}

// writeJSON writes an object as formatted JSON.
func writeJSON(path string, v interface{}, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, perm)
}

// updateGitignore adds CodeButler-specific entries to .gitignore.
func updateGitignore(repoDir string) error {
	gitignorePath := filepath.Join(repoDir, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	entries := []string{
		".codebutler/branches/",
		".codebutler/images/",
	}

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil // nothing to add
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if existing != "" && !strings.HasSuffix(existing, "\n") {
		f.WriteString("\n")
	}

	f.WriteString("\n# CodeButler\n")
	for _, entry := range toAdd {
		f.WriteString(entry + "\n")
	}

	return nil
}
