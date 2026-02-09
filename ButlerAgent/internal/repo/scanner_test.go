package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanRepositories(t *testing.T) {
	tempDir := t.TempDir()

	// Create test repos
	testRepos := []struct {
		name        string
		hasClaudeMd bool
	}{
		{"project-with-claude", true},
		{"project-without-claude", false},
		{"another-project", true},
	}

	for _, tr := range testRepos {
		repoPath := filepath.Join(tempDir, tr.name)
		if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0755); err != nil {
			t.Fatalf("Failed to create test repo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repoPath, ".git", "HEAD"), []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create git HEAD: %v", err)
		}

		if tr.hasClaudeMd {
			if err := os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("# Instructions"), 0644); err != nil {
				t.Fatalf("Failed to create CLAUDE.md: %v", err)
			}
		}
	}

	// Create non-git directory (should be ignored)
	nonGitDir := filepath.Join(tempDir, "not-a-repo")
	if err := os.MkdirAll(nonGitDir, 0755); err != nil {
		t.Fatalf("Failed to create non-git dir: %v", err)
	}

	repos, err := ScanRepositories(tempDir)
	if err != nil {
		t.Fatalf("ScanRepositories failed: %v", err)
	}

	if len(repos) != len(testRepos) {
		t.Errorf("Expected %d repositories, got %d", len(testRepos), len(repos))
	}

	for _, tr := range testRepos {
		found := false
		for _, repo := range repos {
			if repo.Name == tr.name {
				found = true
				if repo.HasClaudeMd != tr.hasClaudeMd {
					t.Errorf("Repo %s: expected HasClaudeMd=%v, got %v", tr.name, tr.hasClaudeMd, repo.HasClaudeMd)
				}
				expectedPath := filepath.Join(tempDir, tr.name)
				if repo.Path != expectedPath {
					t.Errorf("Repo %s: expected path %s, got %s", tr.name, expectedPath, repo.Path)
				}
				break
			}
		}
		if !found {
			t.Errorf("Repository %s not found", tr.name)
		}
	}
}

func TestGetRepository(t *testing.T) {
	tempDir := t.TempDir()

	repoPath := filepath.Join(tempDir, "test-repo")
	os.MkdirAll(filepath.Join(repoPath, ".git"), 0755)
	os.WriteFile(filepath.Join(repoPath, ".git", "HEAD"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("# Test"), 0644)

	repo, err := GetRepository(tempDir, "test-repo")
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}

	if repo.Name != "test-repo" {
		t.Errorf("Expected name 'test-repo', got '%s'", repo.Name)
	}

	if !repo.HasClaudeMd {
		t.Error("Expected HasClaudeMd to be true")
	}

	_, err = GetRepository(tempDir, "non-existent")
	if err == nil {
		t.Error("Expected error for non-existent repo, got nil")
	}
}

func TestScanRepositories_NonExistentPath(t *testing.T) {
	_, err := ScanRepositories("/non/existent/path")
	if err == nil {
		t.Error("Expected error for non-existent path, got nil")
	}
}

func TestIsGitRepo(t *testing.T) {
	tempDir := t.TempDir()

	gitRepo := filepath.Join(tempDir, "git-repo")
	os.MkdirAll(filepath.Join(gitRepo, ".git"), 0755)

	if !isGitRepo(gitRepo) {
		t.Error("Expected isGitRepo to return true for git repo")
	}

	nonGitRepo := filepath.Join(tempDir, "non-git")
	os.MkdirAll(nonGitRepo, 0755)

	if isGitRepo(nonGitRepo) {
		t.Error("Expected isGitRepo to return false for non-git repo")
	}
}

func TestClaudeMdDetection(t *testing.T) {
	tempDir := t.TempDir()

	withClaudeMd := filepath.Join(tempDir, "with-claude")
	os.MkdirAll(filepath.Join(withClaudeMd, ".git"), 0755)
	os.WriteFile(filepath.Join(withClaudeMd, ".git", "HEAD"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(withClaudeMd, "CLAUDE.md"), []byte("# Claude instructions"), 0644)

	withoutClaudeMd := filepath.Join(tempDir, "without-claude")
	os.MkdirAll(filepath.Join(withoutClaudeMd, ".git"), 0755)
	os.WriteFile(filepath.Join(withoutClaudeMd, ".git", "HEAD"), []byte("test"), 0644)

	repos, err := ScanRepositories(tempDir)
	if err != nil {
		t.Fatalf("ScanRepositories failed: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("Expected 2 repos, got %d", len(repos))
	}

	for _, repo := range repos {
		if repo.Name == "with-claude" {
			if !repo.HasClaudeMd {
				t.Error("Expected HasClaudeMd to be true for with-claude")
			}
			expectedPath := filepath.Join(withClaudeMd, "CLAUDE.md")
			if repo.ClaudeMdPath != expectedPath {
				t.Errorf("Expected ClaudeMdPath %s, got %s", expectedPath, repo.ClaudeMdPath)
			}
		} else if repo.Name == "without-claude" {
			if repo.HasClaudeMd {
				t.Error("Expected HasClaudeMd to be false for without-claude")
			}
		}
	}
}
