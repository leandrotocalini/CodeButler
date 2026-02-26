package agent

import (
	"crypto/sha256"
	"fmt"
)

// StuckSignal identifies the type of stuck condition detected.
type StuckSignal int

const (
	// SignalNone means no stuck condition detected.
	SignalNone StuckSignal = iota
	// SignalSameToolParams means the same tool+params were called 3+ times.
	SignalSameToolParams
	// SignalSameError means the same error was returned 3+ times.
	SignalSameError
	// SignalNoProgress means no new tool calls or response patterns for 3+ turns.
	SignalNoProgress
)

func (s StuckSignal) String() string {
	switch s {
	case SignalNone:
		return "none"
	case SignalSameToolParams:
		return "same_tool_params"
	case SignalSameError:
		return "same_error"
	case SignalNoProgress:
		return "no_progress"
	default:
		return "unknown"
	}
}

// EscapeLevel tracks which escape strategy to apply next.
type EscapeLevel int

const (
	// EscapeNone means no escape strategy is active.
	EscapeNone EscapeLevel = iota
	// EscapeReflection injects a reflection prompt.
	EscapeReflection
	// EscapeForceReasoning forces explicit reasoning about alternatives.
	EscapeForceReasoning
	// EscapeReduceTools temporarily removes the looping tool.
	EscapeReduceTools
	// EscapeEscalate posts to the thread asking for help.
	EscapeEscalate
)

// ProgressTracker tracks recent tool calls and detects stuck conditions.
// It maintains a rolling window of recent tool call hashes and error messages
// to detect repetitive patterns.
type ProgressTracker struct {
	// Rolling window of recent tool call hashes (tool name + args).
	recentHashes []string
	// Rolling window of recent error messages from tool results.
	recentErrors []string
	// Rolling window of recent response hashes (to detect no-progress).
	recentResponses []string

	// windowSize is the number of recent entries to track.
	windowSize int
	// threshold is how many identical entries trigger a stuck signal.
	threshold int

	// Current escape state.
	escapeLevel EscapeLevel
	// How many turns have been given to the current escape strategy.
	escapeTurns int
	// turnsPerStrategy is how many turns each strategy gets before escalating.
	turnsPerStrategy int

	// The tool name involved in the current stuck loop (for EscapeReduceTools).
	stuckToolName string
	// removedTools tracks tools temporarily removed by escape strategy.
	removedTools []string
}

// NewProgressTracker creates a tracker with the default window size (5),
// threshold (3), and turns-per-strategy (2) from ARCHITECTURE.md.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		windowSize:       5,
		threshold:        3,
		turnsPerStrategy: 2,
	}
}

// RecordToolCall records a tool call's name+args hash for cycle detection.
func (pt *ProgressTracker) RecordToolCall(name, args string) {
	h := hashToolCall(name, args)
	pt.recentHashes = appendBounded(pt.recentHashes, h, pt.windowSize)
}

// RecordError records a tool error message for repeated-error detection.
func (pt *ProgressTracker) RecordError(errMsg string) {
	pt.recentErrors = appendBounded(pt.recentErrors, errMsg, pt.windowSize)
}

// RecordResponse records a response hash for no-progress detection.
func (pt *ProgressTracker) RecordResponse(content string) {
	h := hashContent(content)
	pt.recentResponses = appendBounded(pt.recentResponses, h, pt.windowSize)
}

// Detect checks all signals and returns the first stuck condition found.
// Should be called before each LLM call.
func (pt *ProgressTracker) Detect() StuckSignal {
	if pt.detectSameToolParams() {
		return SignalSameToolParams
	}
	if pt.detectSameError() {
		return SignalSameError
	}
	if pt.detectNoProgress() {
		return SignalNoProgress
	}
	return SignalNone
}

// detectSameToolParams checks if the last `threshold` tool calls have the same hash.
func (pt *ProgressTracker) detectSameToolParams() bool {
	return hasRepeatedTail(pt.recentHashes, pt.threshold)
}

// detectSameError checks if the last `threshold` errors are identical.
func (pt *ProgressTracker) detectSameError() bool {
	return hasRepeatedTail(pt.recentErrors, pt.threshold)
}

// detectNoProgress checks if the last `threshold` responses are identical.
func (pt *ProgressTracker) detectNoProgress() bool {
	return hasRepeatedTail(pt.recentResponses, pt.threshold)
}

// NextEscapeAction determines what escape action to apply based on the current
// escape level and how many turns have been spent on it. Returns the action
// to take and any tool name to remove (for EscapeReduceTools).
//
// Call this when Detect() returns a non-None signal.
func (pt *ProgressTracker) NextEscapeAction(signal StuckSignal) EscapeLevel {
	// If we're already in an escape sequence, check if the current strategy
	// has exhausted its turns.
	if pt.escapeLevel > EscapeNone {
		pt.escapeTurns++
		if pt.escapeTurns <= pt.turnsPerStrategy {
			// Current strategy still has turns left
			return pt.escapeLevel
		}
		// Strategy exhausted, escalate to next level
		pt.escapeLevel++
		pt.escapeTurns = 1
		return pt.escapeLevel
	}

	// First stuck detection — start with reflection
	pt.escapeLevel = EscapeReflection
	pt.escapeTurns = 1
	return pt.escapeLevel
}

// ResetEscape clears the escape state when progress is detected.
// Also restores any removed tools.
func (pt *ProgressTracker) ResetEscape() {
	pt.escapeLevel = EscapeNone
	pt.escapeTurns = 0
	pt.stuckToolName = ""
	pt.removedTools = nil
}

// SetStuckTool records which tool is causing the loop (used by EscapeReduceTools).
func (pt *ProgressTracker) SetStuckTool(name string) {
	pt.stuckToolName = name
}

// StuckTool returns the tool name involved in the stuck loop.
func (pt *ProgressTracker) StuckTool() string {
	return pt.stuckToolName
}

// RemovedTools returns the list of tools temporarily removed by escape strategies.
func (pt *ProgressTracker) RemovedTools() []string {
	return pt.removedTools
}

// AddRemovedTool records a tool that was temporarily removed.
func (pt *ProgressTracker) AddRemovedTool(name string) {
	pt.removedTools = append(pt.removedTools, name)
}

// IsEscaping returns true if an escape sequence is currently active.
func (pt *ProgressTracker) IsEscaping() bool {
	return pt.escapeLevel > EscapeNone
}

// EscapeLevel returns the current escape level.
func (pt *ProgressTracker) CurrentEscapeLevel() EscapeLevel {
	return pt.escapeLevel
}

// ReflectionPrompt returns the injection prompt for the reflection strategy.
func ReflectionPrompt(detail string) string {
	return fmt.Sprintf(
		"You appear to be in a loop. %s Stop and reflect: what have you tried so far, "+
			"why didn't it work, and what fundamentally different approach could you take?",
		detail,
	)
}

// ForceReasoningPrompt returns the injection prompt for the force-reasoning strategy.
func ForceReasoningPrompt() string {
	return "List every approach you've tried and why each failed. Then propose an " +
		"approach you haven't tried yet. If you can't think of one, say so."
}

// EscalationMessage returns the message posted to the thread when all strategies are exhausted.
func EscalationMessage(role, summary string) string {
	target := "the user"
	switch role {
	case "coder":
		target = "@codebutler.pm"
	case "pm", "lead":
		target = "the user"
	default:
		target = "@codebutler.pm"
	}
	return fmt.Sprintf(
		"I'm stuck. Here's what I tried: %s. I need help. Escalating to %s.",
		summary, target,
	)
}

// hashToolCall produces a deterministic hash of tool name + arguments.
func hashToolCall(name, args string) string {
	h := sha256.Sum256([]byte(name + "|" + args))
	return fmt.Sprintf("%x", h[:8])
}

// hashContent produces a short hash of content for comparison.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}

// hasRepeatedTail checks if the last `count` elements in the slice are identical.
func hasRepeatedTail(items []string, count int) bool {
	n := len(items)
	if n < count {
		return false
	}
	last := items[n-1]
	for i := n - count; i < n-1; i++ {
		if items[i] != last {
			return false
		}
	}
	return true
}

// appendBounded appends an item to a slice, keeping it at most `max` elements.
func appendBounded(items []string, item string, max int) []string {
	items = append(items, item)
	if len(items) > max {
		items = items[len(items)-max:]
	}
	return items
}
