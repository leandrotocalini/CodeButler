package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setupRepoDir creates a temporary repo directory with a .codebutler/config.json
// from the given fixture file. Returns the repo root path.
func setupRepoDir(t *testing.T, repoFixture string) string {
	t.Helper()
	tmpDir := t.TempDir()
	cbDir := filepath.Join(tmpDir, ".codebutler")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(repoFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cbDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

// setupGlobalDir creates a temporary global config directory with a config.json
// from the given fixture file. Returns the directory path.
func setupGlobalDir(t *testing.T, globalFixture string) string {
	t.Helper()
	tmpDir := t.TempDir()
	data, err := os.ReadFile(globalFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

func TestLoad_ValidConfig(t *testing.T) {
	globalDir := setupGlobalDir(t, "testdata/global_valid.json")
	repoDir := setupRepoDir(t, "testdata/repo_valid.json")

	cfg, err := Load(repoDir, globalDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Global fields
	if cfg.Global.Slack.BotToken != "xoxb-test-bot-token" {
		t.Errorf("BotToken = %q, want %q", cfg.Global.Slack.BotToken, "xoxb-test-bot-token")
	}
	if cfg.Global.Slack.AppToken != "xapp-test-app-token" {
		t.Errorf("AppToken = %q, want %q", cfg.Global.Slack.AppToken, "xapp-test-app-token")
	}
	if cfg.Global.OpenRouter.APIKey != "sk-or-test-key" {
		t.Errorf("OpenRouter APIKey = %q, want %q", cfg.Global.OpenRouter.APIKey, "sk-or-test-key")
	}
	if cfg.Global.OpenAI.APIKey != "sk-test-openai-key" {
		t.Errorf("OpenAI APIKey = %q, want %q", cfg.Global.OpenAI.APIKey, "sk-test-openai-key")
	}

	// Repo fields
	if cfg.Repo.Slack.ChannelID != "C0123456789" {
		t.Errorf("ChannelID = %q, want %q", cfg.Repo.Slack.ChannelID, "C0123456789")
	}
	if cfg.Repo.Slack.ChannelName != "codebutler-myproject" {
		t.Errorf("ChannelName = %q, want %q", cfg.Repo.Slack.ChannelName, "codebutler-myproject")
	}

	// Models
	if cfg.Repo.Models.PM == nil {
		t.Fatal("PM model config is nil")
	}
	if cfg.Repo.Models.PM.Default != "moonshotai/kimi-k2" {
		t.Errorf("PM Default = %q, want %q", cfg.Repo.Models.PM.Default, "moonshotai/kimi-k2")
	}
	if len(cfg.Repo.Models.PM.Pool) != 2 {
		t.Errorf("PM Pool len = %d, want 2", len(cfg.Repo.Models.PM.Pool))
	}
	if cfg.Repo.Models.Coder == nil || cfg.Repo.Models.Coder.Model != "anthropic/claude-opus-4-6" {
		t.Errorf("Coder model mismatch")
	}
	if cfg.Repo.Models.Artist == nil || cfg.Repo.Models.Artist.UXModel != "anthropic/claude-sonnet-4-5-20250929" {
		t.Errorf("Artist UX model mismatch")
	}
	if cfg.Repo.Models.Artist.ImageModel != "openai/gpt-image-1" {
		t.Errorf("Artist Image model = %q, want %q", cfg.Repo.Models.Artist.ImageModel, "openai/gpt-image-1")
	}

	// MultiModel
	if len(cfg.Repo.MultiModel.Models) != 4 {
		t.Errorf("MultiModel.Models len = %d, want 4", len(cfg.Repo.MultiModel.Models))
	}
	if cfg.Repo.MultiModel.MaxAgentsPerRound != 6 {
		t.Errorf("MaxAgentsPerRound = %d, want 6", cfg.Repo.MultiModel.MaxAgentsPerRound)
	}
	if cfg.Repo.MultiModel.MaxCostPerRound != 1.0 {
		t.Errorf("MaxCostPerRound = %f, want 1.0", cfg.Repo.MultiModel.MaxCostPerRound)
	}

	// Limits
	if cfg.Repo.Limits.MaxConcurrentThreads != 3 {
		t.Errorf("MaxConcurrentThreads = %d, want 3", cfg.Repo.Limits.MaxConcurrentThreads)
	}
	if cfg.Repo.Limits.MaxCallsPerHour != 100 {
		t.Errorf("MaxCallsPerHour = %d, want 100", cfg.Repo.Limits.MaxCallsPerHour)
	}
}

func TestLoad_MinimalRepoConfig(t *testing.T) {
	globalDir := setupGlobalDir(t, "testdata/global_valid.json")
	repoDir := setupRepoDir(t, "testdata/repo_minimal.json")

	cfg, err := Load(repoDir, globalDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.Slack.ChannelID != "C999" {
		t.Errorf("ChannelID = %q, want %q", cfg.Repo.Slack.ChannelID, "C999")
	}
	// Optional fields should be zero values
	if cfg.Repo.Models.PM != nil {
		t.Errorf("PM should be nil when not configured")
	}
	if cfg.Repo.Limits.MaxConcurrentThreads != 0 {
		t.Errorf("MaxConcurrentThreads should be 0 when not set")
	}
}

func TestLoad_EnvVarResolution(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-from-env")
	t.Setenv("SLACK_APP_TOKEN", "xapp-from-env")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-from-env")
	t.Setenv("OPENAI_API_KEY", "sk-from-env")

	globalDir := setupGlobalDir(t, "testdata/global_with_env.json")
	repoDir := setupRepoDir(t, "testdata/repo_minimal.json")

	cfg, err := Load(repoDir, globalDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Global.Slack.BotToken != "xoxb-from-env" {
		t.Errorf("BotToken = %q, want %q", cfg.Global.Slack.BotToken, "xoxb-from-env")
	}
	if cfg.Global.Slack.AppToken != "xapp-from-env" {
		t.Errorf("AppToken = %q, want %q", cfg.Global.Slack.AppToken, "xapp-from-env")
	}
	if cfg.Global.OpenRouter.APIKey != "sk-or-from-env" {
		t.Errorf("OpenRouter APIKey = %q, want %q", cfg.Global.OpenRouter.APIKey, "sk-or-from-env")
	}
	if cfg.Global.OpenAI.APIKey != "sk-from-env" {
		t.Errorf("OpenAI APIKey = %q, want %q", cfg.Global.OpenAI.APIKey, "sk-from-env")
	}
}

func TestLoad_EnvVarUnset(t *testing.T) {
	// Ensure the env vars are not set
	t.Setenv("SLACK_BOT_TOKEN", "")
	t.Setenv("SLACK_APP_TOKEN", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	globalDir := setupGlobalDir(t, "testdata/global_with_env.json")
	repoDir := setupRepoDir(t, "testdata/repo_minimal.json")

	_, err := Load(repoDir, globalDir)
	if err == nil {
		t.Fatal("expected validation error for empty env vars, got nil")
	}
}

func TestLoad_ValidationMissingGlobalFields(t *testing.T) {
	globalDir := setupGlobalDir(t, "testdata/global_missing_fields.json")
	repoDir := setupRepoDir(t, "testdata/repo_valid.json")

	_, err := Load(repoDir, globalDir)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	errMsg := err.Error()
	tests := []struct {
		name    string
		pattern string
	}{
		{"appToken", "slack.appToken is required"},
		{"openrouter", "openrouter.apiKey is required"},
	}
	for _, tt := range tests {
		if !contains(errMsg, tt.pattern) {
			t.Errorf("error should mention %q, got: %s", tt.pattern, errMsg)
		}
	}
}

func TestLoad_ValidationMissingChannelID(t *testing.T) {
	globalDir := setupGlobalDir(t, "testdata/global_valid.json")

	// Create repo config without channelID
	repoDir := t.TempDir()
	cbDir := filepath.Join(repoDir, ".codebutler")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cbDir, "config.json"), []byte(`{"slack":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(repoDir, globalDir)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !contains(err.Error(), "slack.channelID is required") {
		t.Errorf("error should mention channelID, got: %s", err.Error())
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	globalDir := setupGlobalDir(t, "testdata/global_valid.json")
	repoDir := setupRepoDir(t, "testdata/invalid.json")

	_, err := Load(repoDir, globalDir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoad_MissingGlobalFile(t *testing.T) {
	repoDir := setupRepoDir(t, "testdata/repo_valid.json")
	nonexistentDir := filepath.Join(t.TempDir(), "nonexistent")

	_, err := Load(repoDir, nonexistentDir)
	if err == nil {
		t.Fatal("expected error for missing global config, got nil")
	}
}

func TestLoad_MissingRepoDir(t *testing.T) {
	globalDir := setupGlobalDir(t, "testdata/global_valid.json")

	_, err := Load(t.TempDir(), globalDir)
	if err == nil {
		t.Fatal("expected error for missing .codebutler dir, got nil")
	}
	if !contains(err.Error(), "no .codebutler directory found") {
		t.Errorf("error should mention missing .codebutler, got: %s", err.Error())
	}
}

func TestFindRepoRoot_WalksUp(t *testing.T) {
	// Create nested directory structure: root/.codebutler + root/a/b/c
	root := t.TempDir()
	cbDir := filepath.Join(root, ".codebutler")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findRepoRoot(nested)
	if err != nil {
		t.Fatalf("findRepoRoot() error = %v", err)
	}
	if got != root {
		t.Errorf("findRepoRoot() = %q, want %q", got, root)
	}
}

func TestFindRepoRoot_CurrentDir(t *testing.T) {
	root := t.TempDir()
	cbDir := filepath.Join(root, ".codebutler")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findRepoRoot(root)
	if err != nil {
		t.Fatalf("findRepoRoot() error = %v", err)
	}
	if got != root {
		t.Errorf("findRepoRoot() = %q, want %q", got, root)
	}
}

func TestResolveEnvVars(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		envs   map[string]string
		want   string
	}{
		{
			name:  "single var",
			input: `{"key": "${MY_VAR}"}`,
			envs:  map[string]string{"MY_VAR": "hello"},
			want:  `{"key": "hello"}`,
		},
		{
			name:  "multiple vars",
			input: `{"a": "${VAR_A}", "b": "${VAR_B}"}`,
			envs:  map[string]string{"VAR_A": "alpha", "VAR_B": "beta"},
			want:  `{"a": "alpha", "b": "beta"}`,
		},
		{
			name:  "unset var resolves to empty",
			input: `{"key": "${UNSET_VAR}"}`,
			envs:  map[string]string{},
			want:  `{"key": ""}`,
		},
		{
			name:  "no vars",
			input: `{"key": "literal"}`,
			envs:  map[string]string{},
			want:  `{"key": "literal"}`,
		},
		{
			name:  "mixed literal and var",
			input: `{"url": "https://api.example.com/${PATH}"}`,
			envs:  map[string]string{"PATH": "v1"},
			want:  `{"url": "https://api.example.com/v1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envs {
				t.Setenv(k, v)
			}
			got := resolveEnvVars(tt.input)
			if got != tt.want {
				t.Errorf("resolveEnvVars() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsgs []string
	}{
		{
			name: "valid config",
			cfg: Config{
				Global: GlobalConfig{
					Slack:      GlobalSlack{BotToken: "xoxb-x", AppToken: "xapp-x"},
					OpenRouter: GlobalOpenRouter{APIKey: "sk-or-x"},
				},
				Repo: RepoConfig{
					Slack: RepoSlack{ChannelID: "C123"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing all required",
			cfg: Config{
				Global: GlobalConfig{},
				Repo:   RepoConfig{},
			},
			wantErr: true,
			errMsgs: []string{
				"slack.botToken is required",
				"slack.appToken is required",
				"openrouter.apiKey is required",
				"slack.channelID is required",
			},
		},
		{
			name: "openai key optional",
			cfg: Config{
				Global: GlobalConfig{
					Slack:      GlobalSlack{BotToken: "xoxb-x", AppToken: "xapp-x"},
					OpenRouter: GlobalOpenRouter{APIKey: "sk-or-x"},
					OpenAI:     GlobalOpenAI{}, // empty is fine
				},
				Repo: RepoConfig{
					Slack: RepoSlack{ChannelID: "C123"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(&tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				for _, msg := range tt.errMsgs {
					if !contains(err.Error(), msg) {
						t.Errorf("error should contain %q, got: %s", msg, err.Error())
					}
				}
			}
		})
	}
}

func TestRepoRoot(t *testing.T) {
	root := t.TempDir()
	cbDir := filepath.Join(root, ".codebutler")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := RepoRoot(root)
	if err != nil {
		t.Fatalf("RepoRoot() error = %v", err)
	}
	if got != root {
		t.Errorf("RepoRoot() = %q, want %q", got, root)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
