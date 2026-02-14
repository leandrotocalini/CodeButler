package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/leandrotocalini/CodeButler/internal/models"
)

const (
	maxReadLines  = 500 // max lines returned by ReadFile
	maxGrepLines  = 100 // max matching lines returned by Grep
	maxListFiles  = 200 // max files returned by ListFiles
	maxGitLogN    = 50  // max commits returned by GitLog
	maxDiffLines  = 300 // max diff lines returned by GitDiff
)

// Executor runs tools within a sandboxed repo directory.
type Executor struct {
	repoDir string
}

// NewExecutor creates a tool executor sandboxed to the given repo directory.
func NewExecutor(repoDir string) *Executor {
	return &Executor{repoDir: repoDir}
}

// Execute dispatches a tool call to the appropriate handler.
func (e *Executor) Execute(ctx context.Context, call models.ToolCall) models.ToolResult {
	var content string
	var err error

	switch call.Name {
	case "ReadFile":
		content, err = e.readFile(call.Arguments)
	case "Grep":
		content, err = e.grep(ctx, call.Arguments)
	case "ListFiles":
		content, err = e.listFiles(call.Arguments)
	case "GitLog":
		content, err = e.gitLog(ctx, call.Arguments)
	case "GitDiff":
		content, err = e.gitDiff(ctx, call.Arguments)
	default:
		return models.ToolResult{CallID: call.ID, Content: fmt.Sprintf("unknown tool: %s", call.Name), IsError: true}
	}

	if err != nil {
		return models.ToolResult{CallID: call.ID, Content: err.Error(), IsError: true}
	}
	return models.ToolResult{CallID: call.ID, Content: content}
}

// ---------------------------------------------------------------------------
// ReadFile
// ---------------------------------------------------------------------------

func (e *Executor) readFile(argsJSON string) (string, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	absPath, err := e.safePath(args.Path)
	if err != nil {
		return "", err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", args.Path, err)
	}
	defer f.Close()

	limit := args.Limit
	if limit <= 0 || limit > maxReadLines {
		limit = maxReadLines
	}
	offset := args.Offset
	if offset < 1 {
		offset = 1
	}

	var buf strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB line buffer
	lineNum := 0
	written := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if written >= limit {
			fmt.Fprintf(&buf, "... (%d+ more lines)\n", lineNum-written-offset+1)
			break
		}
		line := scanner.Text()
		if len(line) > 500 {
			line = line[:500] + "…"
		}
		fmt.Fprintf(&buf, "%4d│ %s\n", lineNum, line)
		written++
	}
	if err := scanner.Err(); err != nil {
		return buf.String(), fmt.Errorf("read error: %w", err)
	}
	if written == 0 {
		return "", fmt.Errorf("file is empty or offset beyond end: %s", args.Path)
	}
	return buf.String(), nil
}

// ---------------------------------------------------------------------------
// Grep
// ---------------------------------------------------------------------------

func (e *Executor) grep(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Glob    string `json:"glob"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	searchDir := e.repoDir
	if args.Path != "" {
		safe, err := e.safePath(args.Path)
		if err != nil {
			return "", err
		}
		searchDir = safe
	}

	// Use grep -rn (available everywhere). Fall back gracefully.
	cmdArgs := []string{"-rn", "--include=" + e.grepGlob(args.Glob), "-E", args.Pattern, "."}
	cmd := exec.CommandContext(ctx, "grep", cmdArgs...)
	cmd.Dir = searchDir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	_ = cmd.Run() // exit code 1 = no matches, that's fine

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return "no matches found", nil
	}

	var buf strings.Builder
	for i, line := range lines {
		if i >= maxGrepLines {
			fmt.Fprintf(&buf, "... (%d more matches)\n", len(lines)-maxGrepLines)
			break
		}
		// Clean up leading "./" from grep output
		line = strings.TrimPrefix(line, "./")
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return buf.String(), nil
}

func (e *Executor) grepGlob(glob string) string {
	if glob == "" {
		return "*"
	}
	// For simple patterns like "*.go", grep --include works fine.
	// For complex globs like "**/*.ts", extract the extension.
	if strings.HasPrefix(glob, "**/*") {
		return "*" + strings.TrimPrefix(glob, "**/*")
	}
	return glob
}

// ---------------------------------------------------------------------------
// ListFiles
// ---------------------------------------------------------------------------

func (e *Executor) listFiles(argsJSON string) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	searchDir := e.repoDir
	if args.Path != "" {
		safe, err := e.safePath(args.Path)
		if err != nil {
			return "", err
		}
		searchDir = safe
	}

	var files []string
	err := filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		// Skip hidden dirs and common noise
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(e.repoDir, path)
		matched, _ := filepath.Match(filepath.Base(args.Pattern), d.Name())
		if !matched {
			// Try matching the full relative path for patterns like "internal/**/*.go"
			matched, _ = filepath.Match(args.Pattern, rel)
		}
		if matched && len(files) < maxListFiles {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk error: %w", err)
	}

	if len(files) == 0 {
		return "no files found matching: " + args.Pattern, nil
	}

	var buf strings.Builder
	for _, f := range files {
		buf.WriteString(f)
		buf.WriteByte('\n')
	}
	if len(files) >= maxListFiles {
		buf.WriteString(fmt.Sprintf("... (showing first %d files)\n", maxListFiles))
	}
	return buf.String(), nil
}

// ---------------------------------------------------------------------------
// GitLog
// ---------------------------------------------------------------------------

func (e *Executor) gitLog(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		N    int    `json:"n"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	n := args.N
	if n <= 0 {
		n = 10
	}
	if n > maxGitLogN {
		n = maxGitLogN
	}

	cmdArgs := []string{"log", "--oneline", "--no-decorate", "-n", strconv.Itoa(n)}
	if args.Path != "" {
		safe, err := e.safePath(args.Path)
		if err != nil {
			return "", err
		}
		// Use relative path for cleaner output
		rel, _ := filepath.Rel(e.repoDir, safe)
		cmdArgs = append(cmdArgs, "--", rel)
	}

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = e.repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log failed: %w", err)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "no commits found", nil
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// GitDiff
// ---------------------------------------------------------------------------

func (e *Executor) gitDiff(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Ref  string `json:"ref"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	cmdArgs := []string{"diff", "--stat"}
	if args.Ref != "" {
		// Basic sanitization: only allow safe git ref characters
		if !isValidGitRef(args.Ref) {
			return "", fmt.Errorf("invalid git ref: %s", args.Ref)
		}
		cmdArgs = append(cmdArgs, args.Ref)
	}
	if args.Path != "" {
		safe, err := e.safePath(args.Path)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(e.repoDir, safe)
		cmdArgs = append(cmdArgs, "--", rel)
	}

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = e.repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return "no changes", nil
	}

	// If output is too long, truncate
	lines := strings.Split(result, "\n")
	if len(lines) > maxDiffLines {
		return strings.Join(lines[:maxDiffLines], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-maxDiffLines), nil
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Path safety
// ---------------------------------------------------------------------------

// safePath resolves a relative path and ensures it stays within repoDir.
// Prevents path traversal attacks (../../etc/passwd).
func (e *Executor) safePath(rel string) (string, error) {
	// Clean and resolve
	cleaned := filepath.Clean(rel)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute paths not allowed: %s", rel)
	}

	abs := filepath.Join(e.repoDir, cleaned)
	abs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// If the file doesn't exist yet, check without symlink resolution
		abs = filepath.Join(e.repoDir, cleaned)
	}

	// Ensure the resolved path is within repoDir
	repoAbs, _ := filepath.Abs(e.repoDir)
	if !strings.HasPrefix(abs, repoAbs+string(filepath.Separator)) && abs != repoAbs {
		return "", fmt.Errorf("path escapes repository: %s", rel)
	}

	return abs, nil
}

// isValidGitRef checks that a git ref contains only safe characters.
func isValidGitRef(ref string) bool {
	for _, c := range ref {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '/' || c == '-' || c == '_' || c == '.' || c == '~' || c == '^') {
			return false
		}
	}
	return len(ref) > 0 && len(ref) < 200
}
