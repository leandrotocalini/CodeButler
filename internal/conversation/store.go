// Package conversation provides persistent conversation storage for agent loops.
// Conversations are stored as JSON arrays of messages with crash-safe file writes.
// Each agent maintains its own conversation file per thread for crash recovery.
package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/leandrotocalini/codebutler/internal/agent"
)

// FileStore persists conversations as JSON files with crash-safe writes.
// The file path follows the convention:
//
//	.codebutler/branches/<branch>/conversations/<role>.json
//
// Crash safety is achieved by writing to a temporary file first, then
// renaming it to the target path. This ensures that a crash during write
// never corrupts the existing conversation file.
type FileStore struct {
	path   string
	logger *slog.Logger
}

// Option configures a FileStore.
type Option func(*FileStore)

// WithLogger sets the structured logger for the store.
func WithLogger(l *slog.Logger) Option {
	return func(s *FileStore) {
		s.logger = l
	}
}

// NewFileStore creates a store that persists conversations at the given file path.
// The path should be the full path to the JSON file, e.g.:
//
//	.codebutler/branches/codebutler-add-login/conversations/coder.json
func NewFileStore(path string, opts ...Option) *FileStore {
	s := &FileStore{
		path:   path,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Path returns the file path of the conversation file.
func (s *FileStore) Path() string {
	return s.path
}

// Load reads the persisted conversation from the JSON file.
// Returns nil, nil if the file does not exist (first activation).
// Returns an error if the file exists but cannot be read or parsed.
func (s *FileStore) Load(_ context.Context) ([]agent.Message, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read conversation file: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	var messages []agent.Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("parse conversation file: %w", err)
	}

	s.logger.Info("loaded conversation", "path", s.path, "messages", len(messages))
	return messages, nil
}

// Save writes the full conversation to the JSON file using crash-safe writes.
// The parent directory is created if it does not exist.
//
// Crash-safe write protocol:
//  1. Write conversation JSON to a temporary file (<path>.tmp)
//  2. Rename the temporary file to the target path (atomic on POSIX)
//
// If the process crashes between steps 1 and 2, the original file is intact.
// The temporary file is cleaned up on the next successful save.
func (s *FileStore) Save(_ context.Context, messages []agent.Message) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create conversation directory: %w", err)
	}

	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal conversation: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp conversation file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp) // best effort cleanup
		return fmt.Errorf("rename conversation file: %w", err)
	}

	s.logger.Debug("saved conversation", "path", s.path, "messages", len(messages))
	return nil
}

// FilePath constructs the conversation file path for a given branch and role.
// The returned path is relative to the repository root:
//
//	.codebutler/branches/<branch>/conversations/<role>.json
//
// For an absolute path, pass an absolute baseDir.
func FilePath(baseDir, branch, role string) string {
	return filepath.Join(baseDir, ".codebutler", "branches", branch, "conversations", role+".json")
}
