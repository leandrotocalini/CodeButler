package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig(t *testing.T) {
	data := []byte(`{
		"servers": {
			"github": {
				"command": "mcp-server-github",
				"args": ["--token", "abc123"],
				"roles": ["pm", "reviewer"]
			},
			"postgres": {
				"command": "mcp-server-postgres",
				"args": ["--dsn", "postgres://localhost/db"],
				"roles": ["coder"]
			}
		}
	}`)

	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cfg.Servers))
	}
	if cfg.Servers["github"].Command != "mcp-server-github" {
		t.Errorf("github command: got %q", cfg.Servers["github"].Command)
	}
	if len(cfg.Servers["github"].Roles) != 2 {
		t.Errorf("github roles: expected 2, got %d", len(cfg.Servers["github"].Roles))
	}
	if len(cfg.Servers["postgres"].Args) != 2 {
		t.Errorf("postgres args: expected 2, got %d", len(cfg.Servers["postgres"].Args))
	}
}

func TestParseConfig_Empty(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{}`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.Servers == nil {
		t.Error("servers map should be initialized")
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(cfg.Servers))
	}
}

func TestParseConfig_Invalid(t *testing.T) {
	_, err := ParseConfig([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFilterByRole(t *testing.T) {
	cfg := &MCPConfig{
		Servers: map[string]ServerConfig{
			"github": {
				Command: "mcp-server-github",
				Roles:   []string{"pm", "reviewer"},
			},
			"postgres": {
				Command: "mcp-server-postgres",
				Roles:   []string{"coder"},
			},
			"shared": {
				Command: "mcp-server-shared",
				// No roles = accessible to all
			},
		},
	}

	tests := []struct {
		role     string
		expected []string
	}{
		{"pm", []string{"github", "shared"}},
		{"coder", []string{"postgres", "shared"}},
		{"reviewer", []string{"github", "shared"}},
		{"artist", []string{"shared"}},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			filtered := FilterByRole(cfg, tt.role)
			if len(filtered) != len(tt.expected) {
				t.Errorf("role %s: expected %d servers, got %d", tt.role, len(tt.expected), len(filtered))
			}
			for _, name := range tt.expected {
				if _, ok := filtered[name]; !ok {
					t.Errorf("role %s: missing expected server %q", tt.role, name)
				}
			}
		})
	}
}

func TestFilterByRole_Empty(t *testing.T) {
	cfg := &MCPConfig{Servers: make(map[string]ServerConfig)}
	filtered := FilterByRole(cfg, "pm")
	if len(filtered) != 0 {
		t.Errorf("expected 0 servers for empty config, got %d", len(filtered))
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/mcp.json")
	if err != nil {
		t.Fatalf("missing file should not be an error: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(cfg.Servers))
	}
}

func TestLoadConfig_WithEnvVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	t.Setenv("TEST_MCP_TOKEN", "secret-token-123")

	data := []byte(`{
		"servers": {
			"github": {
				"command": "mcp-server-github",
				"args": ["--token", "${TEST_MCP_TOKEN}"],
				"roles": ["pm"]
			}
		}
	}`)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Servers["github"].Args[1] != "secret-token-123" {
		t.Errorf("env var not resolved: got %q", cfg.Servers["github"].Args[1])
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	if err := os.WriteFile(path, []byte(`{invalid`), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestResolveArgs(t *testing.T) {
	t.Setenv("TEST_DB_URL", "postgres://localhost/mydb")

	args := []string{"--dsn", "${TEST_DB_URL}", "--verbose"}
	resolved := ResolveArgs(args)

	if resolved[0] != "--dsn" {
		t.Errorf("arg 0: got %q", resolved[0])
	}
	if resolved[1] != "postgres://localhost/mydb" {
		t.Errorf("arg 1: got %q", resolved[1])
	}
	if resolved[2] != "--verbose" {
		t.Errorf("arg 2: got %q", resolved[2])
	}
}

func TestResolveArgs_UnsetVar(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR_12345")
	args := []string{"--token", "${NONEXISTENT_VAR_12345}"}
	resolved := ResolveArgs(args)

	if resolved[1] != "" {
		t.Errorf("unset var should resolve to empty: got %q", resolved[1])
	}
}
