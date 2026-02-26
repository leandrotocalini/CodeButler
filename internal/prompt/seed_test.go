package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func setupSeedsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile(t, dir, "pm.md", "# PM Agent\n\nYou are the PM.\n\n## Learnings\n\nSome learnings.\n")
	writeFile(t, dir, "coder.md", "# Coder Agent\n\nYou write code.\n")
	writeFile(t, dir, "global.md", "# Global\n\nShared knowledge.\n")
	writeFile(t, dir, "workflows.md", "# Workflows\n\n## implement\n\nStandard workflow.\n")

	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSeed_Success(t *testing.T) {
	dir := setupSeedsDir(t)

	content, err := LoadSeed(dir, "pm.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("expected non-empty content")
	}
	if !containsStr(content, "PM Agent") {
		t.Errorf("expected PM Agent in content, got: %s", content)
	}
}

func TestLoadSeed_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadSeed(dir, "nonexistent.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadSeedFiles_PM(t *testing.T) {
	dir := setupSeedsDir(t)

	files, err := LoadSeedFiles(dir, "pm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files.Role != "pm" {
		t.Errorf("expected role pm, got %s", files.Role)
	}
	if files.Seed == "" {
		t.Error("expected non-empty seed")
	}
	if files.Global == "" {
		t.Error("expected non-empty global")
	}
	if files.Workflows == "" {
		t.Error("PM should have workflows")
	}
}

func TestLoadSeedFiles_Coder(t *testing.T) {
	dir := setupSeedsDir(t)

	files, err := LoadSeedFiles(dir, "coder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files.Workflows != "" {
		t.Error("Coder should not have workflows")
	}
}

func TestLoadSeedFiles_MissingRole(t *testing.T) {
	dir := setupSeedsDir(t)

	_, err := LoadSeedFiles(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing role")
	}
}

func TestExcludeArchivedLearnings(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
	}{
		{
			name:  "no archived section",
			input: "# Agent\n\n## Learnings\n\nSome content.\n",
			want:  "# Agent\n\n## Learnings\n\nSome content.\n",
		},
		{
			name:  "with archived section",
			input: "# Agent\n\n## Learnings\n\nActive.\n\n## Archived Learnings\n\nOld stuff.\n",
			want:  "# Agent\n\n## Learnings\n\nActive.",
		},
		{
			name:  "archived at start",
			input: "## Archived Learnings\n\nEverything archived.\n",
			want:  "",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExcludeArchivedLearnings(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
