package initwiz

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockPrompter implements Prompter for testing.
type mockPrompter struct {
	responses map[string]string
	confirms  map[string]bool
}

func (m *mockPrompter) Prompt(question string) (string, error) {
	if resp, ok := m.responses[question]; ok {
		return resp, nil
	}
	return "", nil
}

func (m *mockPrompter) Confirm(question string) (bool, error) {
	if resp, ok := m.confirms[question]; ok {
		return resp, nil
	}
	return true, nil
}

func TestWizard_FullRun(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	prompter := &mockPrompter{
		responses: map[string]string{},
		confirms:  map[string]bool{},
	}

	wiz := NewWizard(homeDir, repoDir, prompter)
	result, err := wiz.Run()
	if err != nil {
		t.Fatalf("wizard failed: %v", err)
	}

	if len(result.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(result.Steps))
	}

	// Verify global config was created
	globalCfg := filepath.Join(homeDir, codebutlerDir, "config.json")
	if _, err := os.Stat(globalCfg); os.IsNotExist(err) {
		t.Error("global config not created")
	}

	// Verify repo structure was created
	repoCfg := filepath.Join(repoDir, codebutlerDir, "config.json")
	if _, err := os.Stat(repoCfg); os.IsNotExist(err) {
		t.Error("repo config not created")
	}

	// Verify directories
	for _, dir := range []string{"skills", "branches", "images", "research"} {
		path := filepath.Join(repoDir, codebutlerDir, dir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("directory %s not created", dir)
		}
	}

	// Verify agent MDs
	for _, md := range []string{"pm.md", "coder.md", "reviewer.md", "researcher.md", "artist.md", "lead.md", "global.md", "workflows.md"} {
		path := filepath.Join(repoDir, codebutlerDir, md)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("agent MD %s not created", md)
		}
	}

	// Verify mcp.json
	mcpPath := filepath.Join(repoDir, codebutlerDir, "mcp.json")
	if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
		t.Error("mcp.json not created")
	}

	// Verify roadmap
	roadmapPath := filepath.Join(repoDir, codebutlerDir, "roadmap.md")
	if _, err := os.Stat(roadmapPath); os.IsNotExist(err) {
		t.Error("roadmap.md not created")
	}
}

func TestWizard_SkipsExisting(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	// Pre-create global config
	globalDir := filepath.Join(homeDir, codebutlerDir)
	os.MkdirAll(globalDir, 0700)
	os.WriteFile(filepath.Join(globalDir, "config.json"), []byte("{}"), 0600)

	// Pre-create repo dir
	os.MkdirAll(filepath.Join(repoDir, codebutlerDir), 0755)

	wiz := NewWizard(homeDir, repoDir, &mockPrompter{})
	result, err := wiz.Run()
	if err != nil {
		t.Fatalf("wizard failed: %v", err)
	}

	skipped := 0
	for _, step := range result.Steps {
		if step.Skipped {
			skipped++
		}
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped steps, got %d", skipped)
	}
}

func TestWizard_RepoConfigContent(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	wiz := NewWizard(homeDir, repoDir, &mockPrompter{})
	wiz.Run()

	// Read and parse repo config
	data, err := os.ReadFile(filepath.Join(repoDir, codebutlerDir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	// Check models exist
	models, ok := cfg["models"].(map[string]interface{})
	if !ok {
		t.Fatal("missing models in config")
	}
	for _, role := range []string{"pm", "coder", "reviewer", "researcher", "lead", "artist"} {
		if _, ok := models[role]; !ok {
			t.Errorf("missing model config for %s", role)
		}
	}
}

func TestWizard_GitignoreCreated(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	wiz := NewWizard(homeDir, repoDir, &mockPrompter{})
	wiz.Run()

	data, err := os.ReadFile(filepath.Join(repoDir, ".gitignore"))
	if err != nil {
		t.Fatalf("read gitignore: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, ".codebutler/branches/") {
		t.Error("gitignore should contain .codebutler/branches/")
	}
	if !strings.Contains(content, ".codebutler/images/") {
		t.Error("gitignore should contain .codebutler/images/")
	}
}

func TestWizard_GitignoreIdempotent(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	// Create gitignore with entries already present
	os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".codebutler/branches/\n.codebutler/images/\n"), 0644)

	wiz := NewWizard(homeDir, repoDir, &mockPrompter{})
	wiz.Run()

	data, _ := os.ReadFile(filepath.Join(repoDir, ".gitignore"))
	content := string(data)

	// Should not have duplicate entries
	count := strings.Count(content, ".codebutler/branches/")
	if count != 1 {
		t.Errorf("gitignore has %d occurrences of branches entry", count)
	}
}

func TestValidate_Complete(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	// Run wizard first
	wiz := NewWizard(homeDir, repoDir, &mockPrompter{})
	wiz.Run()

	errs := Validate(homeDir, repoDir)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_Missing(t *testing.T) {
	errs := Validate("/nonexistent", "/nonexistent")
	if len(errs) == 0 {
		t.Error("expected errors for missing setup")
	}
}

func TestServiceType(t *testing.T) {
	st := ServiceType()
	if st != "launchd" && st != "systemd" && st != "manual" {
		t.Errorf("unexpected service type: %s", st)
	}
}

func TestGenerateServiceConfig_Systemd(t *testing.T) {
	cfg := generateSystemdUnit("pm", "/usr/local/bin/codebutler", "/home/user/project")
	if !strings.Contains(cfg, "--role pm") {
		t.Error("systemd unit should contain role flag")
	}
	if !strings.Contains(cfg, "Restart=always") {
		t.Error("systemd unit should have restart policy")
	}
}

func TestGenerateServiceConfig_LaunchAgent(t *testing.T) {
	cfg := generateLaunchAgent("coder", "/usr/local/bin/codebutler", "/Users/user/project")
	if !strings.Contains(cfg, "com.codebutler.coder") {
		t.Error("plist should contain label")
	}
	if !strings.Contains(cfg, "<true/>") {
		t.Error("plist should have KeepAlive")
	}
}

func TestAgentMDContent(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()

	wiz := NewWizard(homeDir, repoDir, &mockPrompter{})
	wiz.Run()

	data, _ := os.ReadFile(filepath.Join(repoDir, codebutlerDir, "pm.md"))
	content := string(data)

	if !strings.Contains(content, "# PM Agent") {
		t.Error("pm.md should have title")
	}
	if !strings.Contains(content, "## Project Map") {
		t.Error("pm.md should have project map section")
	}
}
