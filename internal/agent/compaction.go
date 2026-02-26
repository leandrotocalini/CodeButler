package agent

import (
	"context"
	"fmt"
	"log/slog"
)

const (
	// defaultCompactionThreshold is the fraction of the context window at which
	// compaction triggers (e.g., 0.8 means 80%).
	defaultCompactionThreshold = 0.8

	// defaultRecentKeep is how many recent tool call + result pairs to keep
	// verbatim during compaction. Recent context matters most.
	defaultRecentKeep = 4

	// compactionPrompt is the instruction sent to the model to summarize progress.
	compactionPrompt = "Summarize your progress so far for yourself — what you did, what you learned, what's left. " +
		"Be concise but include key facts: file paths, function names, test results, decisions made. " +
		"Format as a bulleted list under a '## Progress so far' heading."
)

// CompactionConfig controls when and how context compaction happens.
type CompactionConfig struct {
	// ContextWindowTokens is the model's context window size.
	// When total tokens approach this * Threshold, compaction triggers.
	ContextWindowTokens int

	// Threshold is the fraction of ContextWindowTokens at which to trigger
	// compaction (default 0.8).
	Threshold float64

	// RecentKeep is how many recent message pairs (assistant+tool) to preserve
	// verbatim. Default 4.
	RecentKeep int
}

// DefaultCompactionConfig returns a config with sensible defaults.
// ContextWindowTokens should be set by the caller based on the model.
func DefaultCompactionConfig(contextWindow int) CompactionConfig {
	return CompactionConfig{
		ContextWindowTokens: contextWindow,
		Threshold:           defaultCompactionThreshold,
		RecentKeep:          defaultRecentKeep,
	}
}

// NeedsCompaction checks whether the conversation is approaching the context window
// and should be compacted. It uses the cumulative token usage to estimate.
func NeedsCompaction(cfg CompactionConfig, totalTokensUsed int) bool {
	if cfg.ContextWindowTokens <= 0 {
		return false
	}
	limit := int(float64(cfg.ContextWindowTokens) * cfg.Threshold)
	return totalTokensUsed >= limit
}

// CompactConversation compresses the middle portion of the conversation by
// summarizing it using the LLM. It preserves:
//   - The system prompt (first message)
//   - The last N tool call+result pairs (recent context)
//   - Replaces everything in between with a summary
//
// The summary is generated via a single-shot LLM call and inserted as a user
// message (not system prompt), per ARCHITECTURE.md.
func CompactConversation(
	ctx context.Context,
	provider LLMProvider,
	model string,
	messages []Message,
	recentKeep int,
	logger *slog.Logger,
) ([]Message, error) {
	if len(messages) < 3 {
		// Too few messages to compact
		return messages, nil
	}

	if recentKeep <= 0 {
		recentKeep = defaultRecentKeep
	}

	// Split messages: system prompt | middle | recent
	systemMsg := messages[0]

	// Count recent messages to keep (assistant+tool pairs from the end).
	// Each "pair" is an assistant message + its tool result messages.
	recentStart := findRecentStart(messages, recentKeep)
	if recentStart <= 1 {
		// Not enough middle content to summarize
		return messages, nil
	}

	middle := messages[1:recentStart]
	recent := messages[recentStart:]

	// Need at least 2 middle messages to justify compaction
	// (a single user message isn't worth summarizing)
	if len(middle) < 2 {
		return messages, nil
	}

	// Build the summarization request
	summaryReq := ChatRequest{
		Model: model,
		Messages: append(
			[]Message{{Role: "system", Content: "You are summarizing an agent's conversation for context compaction."}},
			append(middle, Message{Role: "user", Content: compactionPrompt})...,
		),
	}

	logger.Info("compacting conversation",
		"total_messages", len(messages),
		"middle_messages", len(middle),
		"recent_kept", len(recent),
	)

	resp, err := provider.ChatCompletion(ctx, summaryReq)
	if err != nil {
		return nil, fmt.Errorf("compaction summary LLM call failed: %w", err)
	}

	// Build the compacted conversation:
	// [system] + [summary as user message] + [recent messages]
	compacted := make([]Message, 0, 2+len(recent))
	compacted = append(compacted, systemMsg)
	compacted = append(compacted, Message{
		Role:    "user",
		Content: resp.Message.Content,
	})
	compacted = append(compacted, recent...)

	logger.Info("compaction complete",
		"before", len(messages),
		"after", len(compacted),
	)

	return compacted, nil
}

// findRecentStart finds the index where "recent" messages begin.
// It counts backward from the end, keeping `keep` assistant-message groups.
// An assistant-message group is an assistant message followed by its tool results.
func findRecentStart(messages []Message, keep int) int {
	groups := 0
	i := len(messages) - 1

	for i > 0 {
		// Skip tool results
		for i > 0 && messages[i].Role == "tool" {
			i--
		}
		// Should be at an assistant message
		if i > 0 && messages[i].Role == "assistant" {
			groups++
			if groups >= keep {
				return i
			}
			i--
		} else {
			// Unexpected role; stop looking
			break
		}
	}

	return i + 1
}
