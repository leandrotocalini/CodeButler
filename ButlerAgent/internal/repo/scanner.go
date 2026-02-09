package repo

import (
	"fmt"
	"os"
	"path/filepath"
)

type Repository struct {
	Name         string
	Path         string
	HasClaudeMd  bool
	ClaudeMdPath string
}

func ScanRepositories(rootPath string) ([]Repository, error) {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("sources path does not exist: %s", rootPath)
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sources directory: %w", err)
	}

	var repos []Repository
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(rootPath, entry.Name())

		if !isGitRepo(repoPath) {
			continue
		}

		claudeMdPath := filepath.Join(repoPath, "CLAUDE.md")
		hasClaudeMd := fileExists(claudeMdPath)

		repo := Repository{
			Name:         entry.Name(),
			Path:         repoPath,
			HasClaudeMd:  hasClaudeMd,
			ClaudeMdPath: claudeMdPath,
		}

		repos = append(repos, repo)
	}

	return repos, nil
}

func isGitRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func GetRepository(rootPath, name string) (*Repository, error) {
	repos, err := ScanRepositories(rootPath)
	if err != nil {
		return nil, err
	}

	for _, repo := range repos {
		if repo.Name == name {
			return &repo, nil
		}
	}

	return nil, fmt.Errorf("repository not found: %s", name)
}
