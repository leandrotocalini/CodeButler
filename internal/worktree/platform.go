package worktree

import (
	"os"
	"path/filepath"
)

// Platform represents a detected project platform.
type Platform string

const (
	PlatformUnknown Platform = ""
	PlatformGo      Platform = "go"
	PlatformNode    Platform = "node"
	PlatformPython  Platform = "python"
	PlatformRust    Platform = "rust"
)

// DetectPlatform identifies the project platform from files in the directory.
func DetectPlatform(dir string) Platform {
	checks := []struct {
		file     string
		platform Platform
	}{
		{"go.mod", PlatformGo},
		{"package.json", PlatformNode},
		{"requirements.txt", PlatformPython},
		{"pyproject.toml", PlatformPython},
		{"Cargo.toml", PlatformRust},
	}

	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.platform
		}
	}

	return PlatformUnknown
}
