package prompt

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PromptCache caches the built system prompt and rebuilds it when seed files change.
type PromptCache struct {
	seedsDir  string
	skillsDir string
	role      string
	logger    *slog.Logger

	mu          sync.RWMutex
	prompt      string
	lastChecked time.Time
	modTimes    map[string]time.Time
}

// CacheOption configures the prompt cache.
type CacheOption func(*PromptCache)

// WithCacheLogger sets the logger for the cache.
func WithCacheLogger(l *slog.Logger) CacheOption {
	return func(c *PromptCache) {
		c.logger = l
	}
}

// NewPromptCache creates a new prompt cache for the given role.
func NewPromptCache(seedsDir, skillsDir, role string, opts ...CacheOption) *PromptCache {
	c := &PromptCache{
		seedsDir:  seedsDir,
		skillsDir: skillsDir,
		role:      role,
		logger:    slog.Default(),
		modTimes:  make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get returns the current system prompt, rebuilding if files have changed.
func (c *PromptCache) Get() (string, error) {
	c.mu.RLock()
	if c.prompt != "" && !c.hasChanged() {
		defer c.mu.RUnlock()
		return c.prompt, nil
	}
	c.mu.RUnlock()

	// Rebuild
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.prompt != "" && !c.hasChanged() {
		return c.prompt, nil
	}

	prompt, err := c.rebuild()
	if err != nil {
		return "", err
	}

	c.prompt = prompt
	c.lastChecked = time.Now()
	c.logger.Info("prompt rebuilt", "role", c.role)
	return prompt, nil
}

// Invalidate forces the next Get() to rebuild the prompt.
func (c *PromptCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompt = ""
	c.modTimes = make(map[string]time.Time)
}

// rebuild loads seeds, scans skills, and assembles the prompt.
func (c *PromptCache) rebuild() (string, error) {
	seeds, err := LoadSeedFiles(c.seedsDir, c.role)
	if err != nil {
		return "", err
	}

	var skillIndex string
	if c.role == "pm" {
		skills, err := ScanSkillIndex(c.skillsDir)
		if err != nil {
			c.logger.Warn("failed to scan skills", "err", err)
			// Non-fatal: build prompt without skill index
		} else {
			skillIndex = FormatSkillIndex(skills)
		}
	}

	prompt := BuildSystemPrompt(seeds, skillIndex)

	// Record mod times for change detection
	c.recordModTimes()

	return prompt, nil
}

// hasChanged checks if any seed file has been modified since last check.
func (c *PromptCache) hasChanged() bool {
	files := c.watchedFiles()
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if prev, ok := c.modTimes[f]; ok {
			if info.ModTime().After(prev) {
				return true
			}
		} else {
			// New file
			return true
		}
	}
	return false
}

// recordModTimes stores current modification times for watched files.
func (c *PromptCache) recordModTimes() {
	c.modTimes = make(map[string]time.Time)
	for _, f := range c.watchedFiles() {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		c.modTimes[f] = info.ModTime()
	}
}

// watchedFiles returns all files that affect the system prompt.
func (c *PromptCache) watchedFiles() []string {
	files := []string{
		filepath.Join(c.seedsDir, c.role+".md"),
		filepath.Join(c.seedsDir, "global.md"),
	}
	if c.role == "pm" {
		files = append(files, filepath.Join(c.seedsDir, "workflows.md"))

		// Add skill files
		entries, err := os.ReadDir(c.skillsDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					files = append(files, filepath.Join(c.skillsDir, e.Name()))
				}
			}
		}
	}
	return files
}
