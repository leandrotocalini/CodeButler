package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// AgentRunner executes the agent loop: prompt → LLM → tool calls → execute → repeat.
// Same struct powers all six agents — different config, same loop.
type AgentRunner struct {
	provider LLMProvider
	sender   MessageSender
	executor ToolExecutor
	config   AgentConfig
	logger   *slog.Logger
	store    ConversationStore // optional, for crash recovery
}

// RunnerOption configures optional AgentRunner parameters.
type RunnerOption func(*AgentRunner)

// WithLogger sets the structured logger for the runner.
func WithLogger(l *slog.Logger) RunnerOption {
	return func(r *AgentRunner) {
		r.logger = l
	}
}

// WithConversationStore sets the conversation store for crash recovery.
// When set, the runner saves the conversation after every model round
// and supports resuming from the last saved state.
func WithConversationStore(store ConversationStore) RunnerOption {
	return func(r *AgentRunner) {
		r.store = store
	}
}

// NewAgentRunner creates a new agent runner with the given dependencies.
// Interfaces are defined by the consumer (this package), not the implementer.
func NewAgentRunner(
	provider LLMProvider,
	sender MessageSender,
	executor ToolExecutor,
	config AgentConfig,
	opts ...RunnerOption,
) *AgentRunner {
	r := &AgentRunner{
		provider: provider,
		sender:   sender,
		executor: executor,
		config:   config,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run executes the agent loop until one of:
// - The LLM returns a text response (no tool calls)
// - MaxTurns is reached
// - The context is cancelled
//
// The turn counter is checked BEFORE each LLM call, never after, so it cannot overshoot.
//
// When a ConversationStore is configured, Run saves the conversation after every
// model round (assistant response + tool results). On the next call, it loads the
// stored conversation and resumes from the last saved round, enabling crash recovery.
func (r *AgentRunner) Run(ctx context.Context, task Task) (*Result, error) {
	log := r.logger.With("role", r.config.Role, "thread", task.Thread)

	var messages []Message
	var startTurn int

	// Try loading persisted conversation for resume
	if r.store != nil {
		loaded, err := r.store.Load(ctx)
		if err != nil {
			log.Error("failed to load conversation, starting fresh", "err", err)
		} else if len(loaded) > 0 {
			messages = loaded

			// Count completed turns (each assistant message = 1 LLM call)
			for _, m := range loaded {
				if m.Role == "assistant" {
					startTurn++
				}
			}

			// If the conversation already completed (last message is a text
			// response), return immediately without another LLM call.
			last := loaded[len(loaded)-1]
			if last.Role == "assistant" && len(last.ToolCalls) == 0 {
				log.Info("conversation already completed", "turns", startTurn)
				return &Result{
					Response:  last.Content,
					TurnsUsed: startTurn,
				}, nil
			}

			log.Info("resuming conversation", "messages", len(loaded), "completedTurns", startTurn)

			// Append any new messages from the task (e.g., new @mentions
			// received while the agent was down).
			messages = append(messages, task.Messages...)
		}
	}

	// Build conversation from scratch if nothing was loaded
	if len(messages) == 0 {
		messages = make([]Message, 0, len(task.Messages)+1)
		messages = append(messages, Message{
			Role:    "system",
			Content: r.config.SystemPrompt,
		})
		messages = append(messages, task.Messages...)
	}

	tools := r.executor.ListTools()

	var totalUsage TokenUsage
	var totalToolCalls int

	for turn := startTurn; turn < r.config.MaxTurns; turn++ {
		// Check context before LLM call (never after)
		if err := ctx.Err(); err != nil {
			log.Info("context cancelled", "turn", turn)
			return &Result{
				TurnsUsed:  turn,
				TokenUsage: totalUsage,
				ToolCalls:  totalToolCalls,
			}, ctx.Err()
		}

		log.Info("llm call", "turn", turn, "messages", len(messages))

		resp, err := r.provider.ChatCompletion(ctx, ChatRequest{
			Model:    r.config.Model,
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return &Result{
				TurnsUsed:  turn,
				TokenUsage: totalUsage,
				ToolCalls:  totalToolCalls,
			}, fmt.Errorf("llm call failed on turn %d: %w", turn, err)
		}

		// Accumulate token usage
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		// Append assistant message to conversation
		messages = append(messages, resp.Message)

		// Text response (no tool calls) → done
		if len(resp.Message.ToolCalls) == 0 {
			r.saveConversation(ctx, log, messages)
			log.Info("text response", "turn", turn+1)
			return &Result{
				Response:   resp.Message.Content,
				TurnsUsed:  turn + 1,
				TokenUsage: totalUsage,
				ToolCalls:  totalToolCalls,
			}, nil
		}

		// Execute tool calls (parallel when multiple)
		log.Info("executing tools", "count", len(resp.Message.ToolCalls))
		results := r.executeToolCalls(ctx, resp.Message.ToolCalls)
		totalToolCalls += len(results)

		// Append tool results as messages for the next LLM call
		for _, result := range results {
			messages = append(messages, Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
		}

		// Save after each model round (crash-safe)
		r.saveConversation(ctx, log, messages)
	}

	// MaxTurns reached without a text response
	log.Warn("max turns reached", "maxTurns", r.config.MaxTurns, "toolCalls", totalToolCalls)
	return &Result{
		TurnsUsed:  r.config.MaxTurns,
		TokenUsage: totalUsage,
		ToolCalls:  totalToolCalls,
	}, nil
}

// saveConversation persists the conversation if a store is configured.
// Errors are logged but not propagated — a save failure should not stop the agent loop.
func (r *AgentRunner) saveConversation(ctx context.Context, log *slog.Logger, messages []Message) {
	if r.store == nil {
		return
	}
	if err := r.store.Save(ctx, messages); err != nil {
		log.Error("failed to save conversation", "err", err)
	}
}

// executeToolCalls dispatches tool calls, running them in parallel when there
// are multiple independent calls in a single LLM response.
func (r *AgentRunner) executeToolCalls(ctx context.Context, calls []ToolCall) []ToolResult {
	if len(calls) == 1 {
		return []ToolResult{r.executeSingleTool(ctx, calls[0])}
	}

	// Parallel execution for multiple tool calls
	results := make([]ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = r.executeSingleTool(ctx, call)
		}()
	}
	wg.Wait()
	return results
}

// executeSingleTool executes one tool call, converting executor errors into
// error ToolResults so the LLM can handle them.
func (r *AgentRunner) executeSingleTool(ctx context.Context, call ToolCall) ToolResult {
	log := r.logger.With("tool", call.Name, "call_id", call.ID)
	log.Info("tool execute start")

	result, err := r.executor.Execute(ctx, call)
	if err != nil {
		log.Error("tool execute failed", "err", err)
		return ToolResult{
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("error: %s", err),
			IsError:    true,
		}
	}

	log.Info("tool execute done")
	return result
}
