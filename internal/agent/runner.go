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

	// Safety features (M7)
	compaction *CompactionConfig  // optional, for context compaction
	tracker    *ProgressTracker   // stuck detection + escape strategies
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

// WithCompaction enables context compaction with the given config.
// When the conversation approaches the context window, old messages are
// summarized to free up space.
func WithCompaction(cfg CompactionConfig) RunnerOption {
	return func(r *AgentRunner) {
		r.compaction = &cfg
	}
}

// WithProgressTracker enables stuck detection and escape strategies.
// If not set, a default tracker is used automatically.
func WithProgressTracker(pt *ProgressTracker) RunnerOption {
	return func(r *AgentRunner) {
		r.tracker = pt
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
		tracker:  NewProgressTracker(),
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
// Safety features (M7):
// - Stuck detection: detects repeated tool calls, errors, and no-progress patterns
// - Escape strategies: reflection → forced reasoning → tool removal → escalation
// - Context compaction: summarizes old messages when approaching the context window
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
	activeTools := tools // may be reduced by escape strategies

	var totalUsage TokenUsage
	var totalToolCalls int
	var loopsDetected int

	for turn := startTurn; turn < r.config.MaxTurns; turn++ {
		// Check context before LLM call (never after)
		if err := ctx.Err(); err != nil {
			log.Info("context cancelled", "turn", turn)
			return &Result{
				TurnsUsed:     turn,
				TokenUsage:    totalUsage,
				ToolCalls:     totalToolCalls,
				LoopsDetected: loopsDetected,
			}, ctx.Err()
		}

		// --- Stuck detection (M7) ---
		if signal := r.tracker.Detect(); signal != SignalNone {
			loopsDetected++
			log.Warn("stuck detected", "signal", signal.String(), "turn", turn)

			action := r.tracker.NextEscapeAction(signal)
			messages, activeTools = r.applyEscapeStrategy(ctx, log, action, signal, messages, tools)

			if action >= EscapeEscalate {
				// All strategies exhausted — escalate and stop
				log.Warn("all escape strategies exhausted, escalating", "turn", turn)
				return &Result{
					TurnsUsed:     turn,
					TokenUsage:    totalUsage,
					ToolCalls:     totalToolCalls,
					LoopsDetected: loopsDetected,
					Escalated:     true,
				}, nil
			}
		}

		// --- Context compaction (M7) ---
		if r.compaction != nil && NeedsCompaction(*r.compaction, totalUsage.TotalTokens) {
			log.Info("triggering context compaction", "tokens", totalUsage.TotalTokens)
			compacted, err := CompactConversation(
				ctx, r.provider, r.config.Model, messages,
				r.compaction.RecentKeep, log,
			)
			if err != nil {
				log.Error("compaction failed, continuing with full context", "err", err)
			} else {
				messages = compacted
			}
		}

		log.Info("llm call", "turn", turn, "messages", len(messages))

		resp, err := r.provider.ChatCompletion(ctx, ChatRequest{
			Model:    r.config.Model,
			Messages: messages,
			Tools:    activeTools,
		})
		if err != nil {
			return &Result{
				TurnsUsed:     turn,
				TokenUsage:    totalUsage,
				ToolCalls:     totalToolCalls,
				LoopsDetected: loopsDetected,
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
			r.tracker.RecordResponse(resp.Message.Content)
			r.saveConversation(ctx, log, messages)
			log.Info("text response", "turn", turn+1)
			return &Result{
				Response:      resp.Message.Content,
				TurnsUsed:     turn + 1,
				TokenUsage:    totalUsage,
				ToolCalls:     totalToolCalls,
				LoopsDetected: loopsDetected,
			}, nil
		}

		// Record tool calls for stuck detection
		for _, tc := range resp.Message.ToolCalls {
			r.tracker.RecordToolCall(tc.Name, tc.Arguments)
			r.tracker.SetStuckTool(tc.Name) // track last tool for potential removal
		}

		// Execute tool calls (parallel when multiple)
		log.Info("executing tools", "count", len(resp.Message.ToolCalls))
		results := r.executeToolCalls(ctx, resp.Message.ToolCalls)
		totalToolCalls += len(results)

		// Record errors for stuck detection, and check for progress
		hasNewError := false
		for _, result := range results {
			messages = append(messages, Message{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
			if result.IsError {
				r.tracker.RecordError(result.Content)
				hasNewError = true
			}
		}

		// If we were in an escape sequence and made progress, reset
		if r.tracker.IsEscaping() && !hasNewError {
			// Check if tool calls are different from the stuck pattern
			// (progress = new tool call or different args)
			signal := r.tracker.Detect()
			if signal == SignalNone {
				log.Info("progress detected, resetting escape state")
				// Restore removed tools if any
				if len(r.tracker.RemovedTools()) > 0 {
					activeTools = tools
				}
				r.tracker.ResetEscape()
			}
		}

		// Save after each model round (crash-safe)
		r.saveConversation(ctx, log, messages)
	}

	// MaxTurns reached without a text response
	log.Warn("max turns reached", "maxTurns", r.config.MaxTurns, "toolCalls", totalToolCalls)
	return &Result{
		TurnsUsed:     r.config.MaxTurns,
		TokenUsage:    totalUsage,
		ToolCalls:     totalToolCalls,
		LoopsDetected: loopsDetected,
	}, nil
}

// applyEscapeStrategy applies the appropriate escape strategy based on the level.
// Returns possibly modified messages and tools.
func (r *AgentRunner) applyEscapeStrategy(
	ctx context.Context,
	log *slog.Logger,
	level EscapeLevel,
	signal StuckSignal,
	messages []Message,
	allTools []ToolDefinition,
) ([]Message, []ToolDefinition) {
	switch level {
	case EscapeReflection:
		detail := describeSignal(signal, r.tracker)
		prompt := ReflectionPrompt(detail)
		log.Info("escape: injecting reflection prompt")
		messages = append(messages, Message{
			Role:    "user",
			Content: prompt,
		})
		return messages, allTools

	case EscapeForceReasoning:
		log.Info("escape: forcing reasoning step")
		messages = append(messages, Message{
			Role:    "user",
			Content: ForceReasoningPrompt(),
		})
		return messages, allTools

	case EscapeReduceTools:
		stuckTool := r.tracker.StuckTool()
		log.Info("escape: removing stuck tool", "tool", stuckTool)
		r.tracker.AddRemovedTool(stuckTool)

		reduced := make([]ToolDefinition, 0, len(allTools))
		for _, t := range allTools {
			if t.Name != stuckTool {
				reduced = append(reduced, t)
			}
		}
		messages = append(messages, Message{
			Role: "user",
			Content: fmt.Sprintf(
				"The tool %q has been temporarily disabled because you were using it in a loop. "+
					"Find an alternative approach using your remaining tools.",
				stuckTool,
			),
		})
		return messages, reduced

	case EscapeEscalate:
		log.Warn("escape: escalating to user/PM")
		summary := describeSignal(signal, r.tracker)
		msg := EscalationMessage(r.config.Role, summary)
		// Post escalation to the thread via MessageSender
		if r.sender != nil {
			if err := r.sender.SendMessage(ctx, "", "", msg); err != nil {
				log.Error("failed to send escalation message", "err", err)
			}
		}
		return messages, allTools

	default:
		return messages, allTools
	}
}

// describeSignal returns a human-readable description of the stuck signal.
func describeSignal(signal StuckSignal, pt *ProgressTracker) string {
	switch signal {
	case SignalSameToolParams:
		return fmt.Sprintf("You've called the same tool (%s) with the same parameters multiple times.", pt.StuckTool())
	case SignalSameError:
		return "You've received the same error multiple times in a row."
	case SignalNoProgress:
		return "Your recent responses show no observable progress."
	default:
		return "A stuck condition was detected."
	}
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
