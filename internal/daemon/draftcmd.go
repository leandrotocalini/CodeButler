package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/leandrotocalini/CodeButler/internal/kimi"
	"github.com/leandrotocalini/CodeButler/internal/store"
)

const draftSystemPrompt = `You are a prompt engineer assistant. The user is drafting a request for a coding AI assistant (Claude Code) that has full access to their repository.

Your job:
1. Take the user's raw notes/messages and transform them into a clear, structured, actionable prompt
2. Preserve the user's intent exactly — don't add features they didn't ask for
3. Be specific: mention file paths, function names, or patterns if the user mentioned them
4. Structure the prompt so Claude can act on it immediately without asking clarifying questions
5. Write the refined prompt in the same language the user used (Spanish, English, etc.)
6. Output ONLY the refined prompt — no meta-commentary, no "here's your prompt", just the prompt itself`

type draftState struct {
	messages  []string  // accumulated raw messages
	history   []kimi.Message // conversation history with Kimi (for iterations)
	chatJID   string
	startedAt time.Time
}

type draftHandler struct {
	mu      sync.Mutex
	pending map[string]*draftState // key: chatJID
}

func newDraftHandler() *draftHandler {
	return &draftHandler{
		pending: make(map[string]*draftState),
	}
}

// IsDraftModeCommand returns true if the text is /draft-mode.
func IsDraftModeCommand(text string) bool {
	return strings.TrimSpace(text) == "/draft-mode"
}

// IsDraftDoneCommand returns true if the text is /draft-done.
func IsDraftDoneCommand(text string) bool {
	return strings.TrimSpace(text) == "/draft-done"
}

// IsDraftDiscardCommand returns true if the text is /draft-discard.
func IsDraftDiscardCommand(text string) bool {
	return strings.TrimSpace(text) == "/draft-discard"
}

// IsInDraftMode returns true if the given chat is currently in draft mode.
func (h *draftHandler) IsInDraftMode(chatJID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.pending[chatJID]
	return ok
}

// IsDraftConfirmation returns true if text is 1/2/3 and there's a pending
// draft waiting for confirmation (after Kimi has refined it).
func (h *draftHandler) IsDraftConfirmation(chatJID, text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed != "1" && trimmed != "2" && trimmed != "3" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	state, ok := h.pending[chatJID]
	// Only a confirmation if we have Kimi history (meaning refinement happened)
	return ok && len(state.history) > 0
}

// StartDraft begins draft mode for a chat.
func (d *Daemon) StartDraft(chatJID string) {
	d.draftHandler.mu.Lock()
	d.draftHandler.pending[chatJID] = &draftState{
		chatJID:   chatJID,
		startedAt: time.Now(),
	}
	d.draftHandler.mu.Unlock()

	d.log.Info("Draft mode started for %s", chatJID)
	d.sendMessage(chatJID, "Draft mode ON. Write your idea freely — nothing goes to Claude yet.\n\nWhen done, send /draft-done to refine with Kimi.\nOr /draft-discard to cancel.")
}

// AccumulateDraft adds a message to the draft buffer.
func (d *Daemon) AccumulateDraft(chatJID, content string) {
	d.draftHandler.mu.Lock()
	state, ok := d.draftHandler.pending[chatJID]
	if ok {
		state.messages = append(state.messages, content)
	}
	d.draftHandler.mu.Unlock()

	if ok {
		d.log.Info("Draft message accumulated (%d total)", len(state.messages))
	}
}

// FinishDraft sends accumulated messages to Kimi for refinement.
func (d *Daemon) FinishDraft(chatJID string) {
	d.draftHandler.mu.Lock()
	state, ok := d.draftHandler.pending[chatJID]
	if !ok {
		d.draftHandler.mu.Unlock()
		d.sendMessage(chatJID, "No active draft. Start one with /draft-mode")
		return
	}

	if len(state.messages) == 0 {
		d.draftHandler.mu.Unlock()
		d.sendMessage(chatJID, "Draft is empty — write some messages first, then /draft-done")
		return
	}

	// Build the user content from accumulated messages
	rawContent := strings.Join(state.messages, "\n")
	d.draftHandler.mu.Unlock()

	apiKey := d.repoCfg.Moonshot.APIKey
	if apiKey == "" {
		d.sendMessage(chatJID, "No Moonshot API key configured. Run codebutler --setup or add it to config.json")
		return
	}

	d.log.Info("Sending draft to Kimi for refinement...")
	d.sendPresence(chatJID, true)

	client := kimi.New(apiKey)
	messages := []kimi.Message{
		{Role: "system", Content: draftSystemPrompt},
		{Role: "user", Content: rawContent},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	refined, err := client.Chat(ctx, messages)
	d.sendPresence(chatJID, false)

	if err != nil {
		d.log.Error("Kimi refinement failed: %v", err)
		d.sendMessage(chatJID, "Kimi error: "+err.Error())
		return
	}

	// Save history for potential iterations
	d.draftHandler.mu.Lock()
	state, ok = d.draftHandler.pending[chatJID]
	if ok {
		state.history = append(messages, kimi.Message{Role: "assistant", Content: refined})
	}
	d.draftHandler.mu.Unlock()

	d.log.Info("Draft refined by Kimi (%d chars)", len(refined))

	response := fmt.Sprintf("*Refined prompt:*\n\n%s\n\n*1* — Send to Claude\n*2* — Iterate (give feedback, Kimi refines again)\n*3* — Discard", refined)
	d.sendMessage(chatJID, response)
}

// HandleDraftConfirmation handles 1/2/3 replies after Kimi refinement.
func (d *Daemon) HandleDraftConfirmation(chatJID, text string) {
	choice := strings.TrimSpace(text)

	d.draftHandler.mu.Lock()
	state, ok := d.draftHandler.pending[chatJID]
	if !ok {
		d.draftHandler.mu.Unlock()
		return
	}

	switch choice {
	case "1":
		// Send to Claude — extract the refined prompt from Kimi's last response
		var refined string
		for i := len(state.history) - 1; i >= 0; i-- {
			if state.history[i].Role == "assistant" {
				refined = state.history[i].Content
				break
			}
		}
		delete(d.draftHandler.pending, chatJID)
		d.draftHandler.mu.Unlock()

		d.log.Info("Draft approved — sending to Claude")
		d.sendMessage(chatJID, "Sending refined prompt to Claude...")
		d.injectMessage(chatJID, refined)

	case "2":
		// Iterate — keep state, wait for feedback
		d.draftHandler.mu.Unlock()
		d.sendMessage(chatJID, "Send your feedback — Kimi will refine again.")

	case "3":
		// Discard
		delete(d.draftHandler.pending, chatJID)
		d.draftHandler.mu.Unlock()
		d.log.Info("Draft discarded")
		d.sendMessage(chatJID, "Draft discarded.")
	}
}

// HandleDraftIteration processes feedback during iteration (choice 2).
func (d *Daemon) HandleDraftIteration(chatJID, feedback string) {
	d.draftHandler.mu.Lock()
	state, ok := d.draftHandler.pending[chatJID]
	if !ok || len(state.history) == 0 {
		d.draftHandler.mu.Unlock()
		return
	}

	// Add feedback to conversation history
	history := append(state.history, kimi.Message{Role: "user", Content: feedback})
	d.draftHandler.mu.Unlock()

	apiKey := d.repoCfg.Moonshot.APIKey
	if apiKey == "" {
		d.sendMessage(chatJID, "No Moonshot API key configured")
		return
	}

	d.log.Info("Sending iteration feedback to Kimi...")
	d.sendPresence(chatJID, true)

	client := kimi.New(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	refined, err := client.Chat(ctx, history)
	d.sendPresence(chatJID, false)

	if err != nil {
		d.log.Error("Kimi iteration failed: %v", err)
		d.sendMessage(chatJID, "Kimi error: "+err.Error())
		return
	}

	// Update history
	d.draftHandler.mu.Lock()
	state, ok = d.draftHandler.pending[chatJID]
	if ok {
		state.history = append(history, kimi.Message{Role: "assistant", Content: refined})
	}
	d.draftHandler.mu.Unlock()

	d.log.Info("Draft re-refined by Kimi (%d chars)", len(refined))

	response := fmt.Sprintf("*Refined prompt:*\n\n%s\n\n*1* — Send to Claude\n*2* — Iterate again\n*3* — Discard", refined)
	d.sendMessage(chatJID, response)
}

// injectMessage inserts a message into the store and triggers processing,
// as if the user had sent it via WhatsApp.
func (d *Daemon) injectMessage(chatJID, content string) {
	msg := store.Message{
		ID:        uuid.New().String(),
		From:      "Draft",
		Chat:      chatJID,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if err := d.store.Insert(msg); err != nil {
		d.log.Error("Failed to inject draft message: %v", err)
		return
	}

	select {
	case d.msgNotify <- struct{}{}:
	default:
	}
	d.signalActivity()
}

// DiscardDraft cancels draft mode.
func (d *Daemon) DiscardDraft(chatJID string) {
	d.draftHandler.mu.Lock()
	delete(d.draftHandler.pending, chatJID)
	d.draftHandler.mu.Unlock()

	d.log.Info("Draft discarded for %s", chatJID)
	d.sendMessage(chatJID, "Draft discarded.")
}
