package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// envVarPattern matches ${VAR_NAME} references in string values.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// MCPConfig is the top-level structure of .codebutler/mcp.json.
type MCPConfig struct {
	Servers map[string]ServerConfig `json:"servers"`
}

// ServerConfig defines a single MCP server.
type ServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Roles   []string `json:"roles,omitempty"` // if empty, all roles get access
}

// LoadConfig reads and parses an mcp.json file.
// Environment variables in ${VAR} syntax are resolved.
func LoadConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No mcp.json is valid — agent starts with zero MCP tools
			return &MCPConfig{Servers: make(map[string]ServerConfig)}, nil
		}
		return nil, fmt.Errorf("read mcp.json: %w", err)
	}

	resolved := resolveEnvVars(string(data))

	var cfg MCPConfig
	if err := json.Unmarshal([]byte(resolved), &cfg); err != nil {
		return nil, fmt.Errorf("parse mcp.json: %w", err)
	}

	if cfg.Servers == nil {
		cfg.Servers = make(map[string]ServerConfig)
	}

	return &cfg, nil
}

// ParseConfig parses mcp.json from raw bytes (no env var resolution).
func ParseConfig(data []byte) (*MCPConfig, error) {
	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mcp.json: %w", err)
	}
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]ServerConfig)
	}
	return &cfg, nil
}

// FilterByRole returns only the servers accessible to the given role.
// If a server's Roles list is empty, it's accessible to all roles.
func FilterByRole(cfg *MCPConfig, role string) map[string]ServerConfig {
	filtered := make(map[string]ServerConfig)
	for name, server := range cfg.Servers {
		if len(server.Roles) == 0 {
			filtered[name] = server
			continue
		}
		for _, r := range server.Roles {
			if r == role {
				filtered[name] = server
				break
			}
		}
	}
	return filtered
}

// resolveEnvVars replaces all ${VAR_NAME} patterns with environment variable values.
func resolveEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1] // strip ${ and }
		return os.Getenv(varName)
	})
}

// ResolveArgs resolves environment variables in server arguments.
func ResolveArgs(args []string) []string {
	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = resolveEnvVars(arg)
	}
	return resolved
}
