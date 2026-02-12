package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/leandrotocalini/CodeButler/internal/agent"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/messenger"
	"github.com/leandrotocalini/CodeButler/internal/store"
	"github.com/leandrotocalini/CodeButler/internal/transcribe"
	"golang.org/x/term"
)

const (
	// AccumulationWindow is how long we wait after the first message
	// before spawning Claude, to batch rapid-fire messages together.
	AccumulationWindow = 3 * time.Second

	// ReplyWindow is how long we wait for the user to reply after Claude
	// responds. If no reply arrives, the conversation ends and queued
	// messages get processed as a new cold batch. No timer limits the
	// conversation itself — it stays active as long as the user keeps replying.
	ReplyWindow = 60 * time.Second

	// ColdPollInterval is the normal polling interval when idle.
	ColdPollInterval = 2 * time.Second

	// CompactDelay is how long the daemon waits in complete silence before
	// compacting the session. Only fires when idle — any activity resets it.
	CompactDelay = 10 * time.Minute
)

type Daemon struct {
	repoCfg *config.RepoConfig
	repoDir string
	store     *store.Store
	msger     messenger.Messenger
	agent     *agent.Agent
	log       *Logger

	// Resolved chat target — WhatsApp groupJID or Slack channelID
	chatID    string
	botPrefix string
	groupName string

	msgerMu   sync.Mutex
	connState messenger.ConnectionState

	// Claude busy state
	busyMu sync.Mutex
	busy   bool

	// Conversation state: active after Claude responds, waiting for user reply.
	// No timer — stays active indefinitely until ReplyWindow expires with no reply.
	convMu           sync.Mutex
	convActive       bool      // true while waiting for user reply
	convResponse     time.Time // when Claude last responded (cutoff for follow-ups vs queued)
	convChat         string    // chat JID of the active conversation
	convWaitingInput bool      // true when Claude's last response had [NEED_USER_INPUT]

	// Image generation command handler
	imgHandler *imageCommandHandler

	// Draft mode handler (Kimi prompt refinement)
	draftHandler *draftHandler

	// Message notification channel
	msgNotify chan struct{}

	// Activity signal — resets the compact timer
	activityNotify chan struct{}

	// Track bot's own messages so we can mark old ones as read
	botMsgMu     sync.Mutex
	botMsgIDs    []string // message IDs sent by the bot (oldest first)
	botMsgChat   string   // chat JID for those messages

	// Web server
	webPort   int
	startTime time.Time

	// Version string, set at build time
	version string
}

func New(repoCfg *config.RepoConfig, repoDir, version string, m messenger.Messenger, chatID, botPrefix, groupName string) *Daemon {
	return &Daemon{
		repoCfg:        repoCfg,
		repoDir:        repoDir,
		version:        version,
		msger:          m,
		chatID:         chatID,
		botPrefix:      botPrefix,
		groupName:      groupName,
		log:            NewLogger(500),
		imgHandler:     newImageCommandHandler(),
		draftHandler:   newDraftHandler(),
		msgNotify:      make(chan struct{}, 1),
		activityNotify: make(chan struct{}, 1),
		startTime:      time.Now(),
	}
}

func (d *Daemon) signalActivity() {
	select {
	case d.activityNotify <- struct{}{}:
	default:
	}
}

func (d *Daemon) isBusy() bool {
	d.busyMu.Lock()
	defer d.busyMu.Unlock()
	return d.busy
}

func (d *Daemon) setBusy(b bool) {
	d.busyMu.Lock()
	d.busy = b
	d.busyMu.Unlock()
}

func (d *Daemon) Run() error {
	// Open store
	dbPath := filepath.Join(config.RepoDir(d.repoDir), "store.db")
	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	d.store = st
	defer st.Close()

	// Clean up old tmp files from previous runs
	os.RemoveAll(config.TmpPath(d.repoDir))

	// Create agent
	d.agent = agent.New(d.repoDir, d.repoCfg.Claude)

	// Start web server
	d.startWeb()

	// Show header
	d.log.Clear()
	d.log.Header("CodeButler \u00b7 %s \u00b7 http://localhost:%d", d.groupName, d.webPort)

	// Connect messenger
	if err := d.connectMessenger(); err != nil {
		return fmt.Errorf("connect %s: %w", d.msger.Name(), err)
	}

	// Start watchdog
	go d.connectionWatchdog()

	// Start poll loop in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.pollLoop(ctx)
	go d.compactWatchdog(ctx)

	// Start TUI input if terminal supports it
	var inputDone chan struct{}
	if d.log.InputMode() {
		inputDone = make(chan struct{})
		go func() {
			d.startInput(ctx)
			close(inputDone)
		}()
	}

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh

	d.log.Status("Received %s, shutting down...", sig)
	cancel()

	// Wait for input goroutine to restore terminal state before Cleanup writes to stderr
	if inputDone != nil {
		select {
		case <-inputDone:
		case <-time.After(500 * time.Millisecond):
		}
	}

	d.msgerMu.Lock()
	if d.msger != nil {
		d.msger.Disconnect()
	}
	d.msgerMu.Unlock()

	d.log.Status("Goodbye!")
	d.log.Cleanup()
	return nil
}

func (d *Daemon) connectMessenger() error {
	name := d.msger.Name()
	d.log.Status("%s: connecting...", name)
	for attempt := 1; attempt <= 5; attempt++ {
		if err := d.msger.Connect(); err != nil {
			d.log.Error("%s attempt %d/5: %v", name, attempt, err)
			if attempt < 5 {
				delay := time.Duration(min(attempt*5, 30)) * time.Second
				time.Sleep(delay)
				continue
			}
			return fmt.Errorf("all connection attempts failed")
		}

		d.setupMessenger()
		d.log.Status("%s: connected", name)

		// Announce startup (only on initial connect, not reconnects)
		d.sendMessage(d.chatID, fmt.Sprintf("I am back. I am version %s", d.version))

		return nil
	}
	return fmt.Errorf("unreachable")
}

func (d *Daemon) setupMessenger() {
	botPrefix := d.botPrefix
	groupJID := d.chatID

	d.msger.OnConnectionEvent(func(state messenger.ConnectionState) {
		d.msgerMu.Lock()
		prev := d.connState
		d.connState = state
		d.msgerMu.Unlock()
		name := d.msger.Name()
		switch {
		case state == messenger.StateConnected && prev != messenger.StateConnected:
			d.log.Status("%s: reconnected", name)
		case state == messenger.StateLoggedOut:
			d.log.Warn("%s: logged out", name)
		case state != messenger.StateConnected && prev == messenger.StateConnected:
			d.log.Warn("%s: %s", name, state)
		}
	})

	d.msger.OnMessage(func(msg messenger.Message) {
		// Filter by group/channel
		if groupJID != "" && msg.Chat != groupJID {
			return
		}

		// Filter bot's own responses (they start with the prefix)
		if botPrefix != "" && strings.HasPrefix(msg.Content, botPrefix) {
			return
		}

		// Intercept /exit command
		if isExitCommand(msg.Content) {
			d.handleExit(msg.Chat)
			return
		}
		// Intercept /help command
		if isHelpCommand(msg.Content) {
			go d.handleHelp(msg.Chat)
			return
		}
		// Intercept /cleanSession command
		if isCleanSessionCommand(msg.Content) {
			go d.handleCleanSession(msg.Chat)
			return
		}
		// Intercept /create-image command
		if IsCreateImageCommand(msg.Content) {
			go d.HandleCreateImage(msg)
			return
		}
		// Intercept image confirmation (1/2) when there's a pending image
		if d.imgHandler.IsConfirmationReply(msg.Chat, msg.Content) {
			go d.HandleImageConfirmation(msg)
			return
		}
		// Transcribe voice messages early so draft mode and all handlers
		// receive the transcribed text instead of raw audio references.
		content := msg.Content
		if msg.IsVoice {
			apiKey := d.repoCfg.OpenAI.APIKey
			if apiKey == "" {
				d.log.Warn("Voice message received but no OpenAI API key configured")
				content = "[Voice message — no OpenAI key for transcription]"
			} else {
				audioPath, err := d.msger.DownloadAudio(msg)
				if err != nil {
					d.log.Error("Failed to download audio: %v", err)
					content = "[Voice message — download failed]"
				} else {
					audioData, err := os.ReadFile(audioPath)
					if err != nil {
						d.log.Error("Failed to read audio file: %v", err)
						content = "[Voice message — read failed]"
					} else {
						text, err := transcribe.Whisper(apiKey, audioData)
						if err != nil {
							d.log.Error("Whisper transcription failed: %v", err)
							content = "[Voice message — transcription failed]"
						} else {
							content = "<transcribed-voice-message>" + text + "</transcribed-voice-message>"
							d.log.Info("Transcribed voice: %s", text[:min(len(text), 80)])
						}
					}
					os.Remove(audioPath)
				}
			}
		}

		// Intercept /draft-mode command
		if IsDraftModeCommand(content) {
			go d.StartDraft(msg.Chat)
			return
		}
		// Intercept /draft-done command
		if IsDraftDoneCommand(content) {
			go d.FinishDraft(msg.Chat)
			return
		}
		// Intercept /draft-discard command
		if IsDraftDiscardCommand(content) {
			go d.DiscardDraft(msg.Chat)
			return
		}
		// Intercept draft confirmation (1/2/3) when Kimi has refined a draft
		if d.draftHandler.IsDraftConfirmation(msg.Chat, content) {
			go d.HandleDraftConfirmation(msg.Chat, content)
			return
		}
		// In draft mode: accumulate messages instead of sending to Claude
		if d.draftHandler.IsInDraftMode(msg.Chat) {
			// Check if Kimi already refined and user is iterating (has history)
			d.draftHandler.mu.Lock()
			state := d.draftHandler.pending[msg.Chat]
			hasHistory := state != nil && len(state.history) > 0
			d.draftHandler.mu.Unlock()

			if hasHistory {
				// User is providing feedback for iteration
				go d.HandleDraftIteration(msg.Chat, content)
			} else {
				// Still accumulating raw messages
				d.AccumulateDraft(msg.Chat, content)
			}
			return
		}

		// User is engaged — mark old bot messages as read
		d.markAllBotMessages()

		// Mark as read immediately so the sender sees blue ticks
		if msg.ID != "" {
			if err := d.msger.MarkRead(msg.Chat, msg.From, []string{msg.ID}); err != nil {
				d.log.Warn("MarkRead failed: %v", err)
			}
		}

		// Download image messages
		if msg.IsImage {
			imgData, err := d.msger.DownloadImage(msg)
			if err != nil {
				d.log.Error("Failed to download image: %v", err)
				content = "[Image — download failed]"
			} else {
				tmpDir := config.TmpPath(d.repoDir)
				os.MkdirAll(tmpDir, 0755)
				imgPath := filepath.Join(tmpDir, fmt.Sprintf("image-%s.jpg", uuid.New().String()[:8]))
				if err := os.WriteFile(imgPath, imgData, 0644); err != nil {
					d.log.Error("Failed to save image: %v", err)
					content = "[Image — save failed]"
				} else {
					caption := msg.Content
					if caption == "[Image]" {
						caption = ""
					}
					if caption != "" {
						content = fmt.Sprintf("<attached-image path=\"%s\">%s</attached-image>", imgPath, caption)
					} else {
						content = fmt.Sprintf("<attached-image path=\"%s\" />", imgPath)
					}
				}
			}
		}

		// Persist message
		pending := store.Message{
			ID:         uuid.New().String(),
			WhatsAppID: msg.ID,
			From:       msg.From,
			Chat:       msg.Chat,
			Content:    content,
			Timestamp:  time.Now().Format(time.RFC3339),
			IsVoice:    msg.IsVoice,
		}

		if err := d.store.Insert(pending); err != nil {
			d.log.Error("Failed to store message: %v", err)
			return
		}

		// Signal new message
		select {
		case d.msgNotify <- struct{}{}:
		default:
		}

		d.signalActivity()
		d.log.UserMsg(d.msger.Name(), content, time.Now(), msg.IsVoice, msg.IsImage)
	})

	d.msgerMu.Lock()
	d.connState = d.msger.GetState()
	d.msgerMu.Unlock()
}

func (d *Daemon) connectionWatchdog() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	disconnectedAt := time.Time{}
	name := d.msger.Name()

	for range ticker.C {
		d.msgerMu.Lock()
		state := d.connState
		d.msgerMu.Unlock()

		switch state {
		case messenger.StateConnected:
			disconnectedAt = time.Time{}

		case messenger.StateLoggedOut:
			d.log.Warn("%s logged out — run codebutler --setup to reconfigure", name)
			return

		case messenger.StateDisconnected, messenger.StateReconnecting:
			if disconnectedAt.IsZero() {
				disconnectedAt = time.Now()
				d.log.Warn("%s disconnected, watching...", name)
			} else if time.Since(disconnectedAt) > 2*time.Minute {
				d.log.Warn("Disconnected >2min, forcing reconnect...")
				d.msger.Disconnect()

				if err := d.connectMessenger(); err != nil {
					d.log.Error("Reconnect failed: %v", err)
				}
				return
			}
		}
	}
}

func (d *Daemon) isConversationActive() bool {
	d.convMu.Lock()
	defer d.convMu.Unlock()
	return d.convActive
}

func (d *Daemon) getConversationResponseTime() time.Time {
	d.convMu.Lock()
	defer d.convMu.Unlock()
	return d.convResponse
}

func (d *Daemon) isWaitingInput() bool {
	d.convMu.Lock()
	defer d.convMu.Unlock()
	return d.convWaitingInput
}

func (d *Daemon) startConversation(chatJID string, waitingInput bool) {
	d.convMu.Lock()
	d.convActive = true
	d.convResponse = time.Now()
	d.convChat = chatJID
	d.convWaitingInput = waitingInput
	d.convMu.Unlock()
}

func (d *Daemon) endConversation() string {
	d.convMu.Lock()
	chatJID := d.convChat
	d.convActive = false
	d.convResponse = time.Time{}
	d.convChat = ""
	d.convWaitingInput = false
	d.convMu.Unlock()
	return chatJID
}

func (d *Daemon) pollLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.msgNotify:
			d.handleNewMessages(ctx)
		case <-time.After(ColdPollInterval):
			// Periodic safety net — catch anything missed
			d.handleNewMessages(ctx)
		}
	}
}

// handleNewMessages implements the conversation-first model:
//
// When a conversation is active (Claude responded and is waiting for user reply):
//   - follow-ups (arrived AFTER Claude's response) → processed immediately with --resume
//   - queued (arrived BEFORE response, during processing) → held indefinitely
//   - if no follow-up arrives within ReplyWindow → conversation ends, queued msgs processed
//
// The conversation has absolute priority. No timer limits it — it stays active
// as long as the user keeps replying. Queued messages NEVER interrupt the conversation.
// They only get processed once the back-and-forth is truly done.
//
// In cold mode (no active conversation), all messages get batched after AccumulationWindow.
func (d *Daemon) handleNewMessages(ctx context.Context) {
	msgs, err := d.store.GetPending()
	if err != nil {
		d.log.Error("Failed to get pending: %v", err)
		return
	}
	if len(msgs) == 0 {
		return
	}

	if d.isConversationActive() {
		cutoff := d.getConversationResponseTime()
		var followUps, queued []store.Message

		for _, m := range msgs {
			t, err := time.Parse(time.RFC3339, m.Timestamp)
			if err != nil {
				followUps = append(followUps, m)
				continue
			}
			if t.After(cutoff) {
				followUps = append(followUps, m)
			} else {
				queued = append(queued, m)
			}
		}

		if len(followUps) > 0 {
			d.processBatch(ctx, followUps)
			return
		}

		// No follow-ups yet. Wait for user to reply up to ReplyWindow.
		// If a message arrives during the wait, re-check immediately.
		d.log.Status("Waiting for reply...")
		select {
		case <-ctx.Done():
			return
		case <-d.msgNotify:
			// New message arrived — re-check if it's a follow-up
			d.handleNewMessages(ctx)
			return
		case <-time.After(ReplyWindow):
			// No reply — conversation is done
			d.log.Status("Conversation ended")
			d.endConversation()
		}

		// Fall through: conversation ended, process queued messages as cold batch
		msgs, err = d.store.GetPending()
		if err != nil {
			d.log.Error("Failed to get pending: %v", err)
			return
		}
		if len(msgs) == 0 {
			return
		}
		// Process all remaining as new cold batch (no accumulation wait — they've waited long enough)
		d.log.Status("Processing queued messages...")
		d.processBatch(ctx, msgs)
		return
	}

	// Cold: wait for more messages to accumulate
	d.log.Status("Accumulating messages...")
	select {
	case <-ctx.Done():
		return
	case <-time.After(AccumulationWindow):
	}

	// Re-fetch — more messages may have arrived during the window
	msgs, err = d.store.GetPending()
	if err != nil {
		d.log.Error("Failed to get pending: %v", err)
		return
	}
	if len(msgs) == 0 {
		return
	}

	d.processBatch(ctx, msgs)
}

func (d *Daemon) processBatch(ctx context.Context, msgs []store.Message) {
	d.setBusy(true)
	defer d.setBusy(false)

	// Get chat JID from first message
	chatJID := msgs[0].Chat

	// Build prompt from all pending messages
	var prompt strings.Builder

	// Prepend context summary from previous conversations (if no active session)
	sessionID, _ := d.store.GetSession(chatJID)
	if sessionID == "" {
		if summary := d.store.GetSummary(chatJID); summary != "" {
			prompt.WriteString("<previous-context>\n")
			prompt.WriteString(summary)
			prompt.WriteString("\n</previous-context>\n\n")
		}
	}

	for _, msg := range msgs {
		prompt.WriteString(fmt.Sprintf("[%s] %s: %s\n", msg.Timestamp, msg.From, msg.Content))
	}

	// Show "typing..." in WhatsApp while Claude processes.
	// WhatsApp expires composing after ~25s, so refresh every 20s.
	typingDone := make(chan struct{})
	go func() {
		d.sendPresence(chatJID, true)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-ticker.C:
				d.sendPresence(chatJID, true)
			}
		}
	}()

	currentPrompt := prompt.String()
	totalTurns := 0
	var totalCost float64
	needInput := false

	// Claude loop — runs multiple times if Claude hits max turns and signals continuation
	for {
		if sessionID != "" {
			d.log.BotStart(fmt.Sprintf("resuming %s\u2026", sessionID[:min(len(sessionID), 8)]))
		} else {
			d.log.BotStart("new session")
		}

		start := time.Now()
		result, err := d.agent.Run(ctx, currentPrompt, sessionID)
		elapsed := time.Since(start)

		if err != nil {
			close(typingDone)
			d.log.Error("Claude error after %s: %v", elapsed.Round(time.Second), err)
			d.sendPresence(chatJID, false)
			d.sendMessage(chatJID, fmt.Sprintf("Error: %v", err))
			for _, msg := range msgs {
				d.store.Ack(msg.ID)
			}
			d.markRead(msgs)
			return
		}

		totalTurns += result.NumTurns
		totalCost += result.CostUSD

		// Save session for resume
		if result.SessionID != "" {
			d.store.SetSession(chatJID, result.SessionID)
			sessionID = result.SessionID
		}

		d.log.BotResult(elapsed, result.NumTurns, result.CostUSD)

		response := result.Result
		if result.IsError {
			response = "Error: " + response
			d.log.BotText("\u26a0 " + response)
		}

		// Check for markers
		continuing := strings.Contains(response, agent.ContinuationMarker)
		needInput = strings.Contains(response, agent.NeedInputMarker)

		// Strip markers before sending
		if continuing {
			response = strings.ReplaceAll(response, agent.ContinuationMarker, "")
		}
		if needInput {
			response = strings.ReplaceAll(response, agent.NeedInputMarker, "")
		}
		response = strings.TrimSpace(response)

		// Extract and send any <send-image> tags as WhatsApp images
		response = d.processResponseImages(chatJID, response)

		// Send partial or final response
		if response != "" {
			preview := response
			if len(preview) > 200 {
				preview = preview[:200] + "\u2026"
			}
			if !result.IsError {
				d.log.BotText(preview)
			}
			d.sendMessage(chatJID, response)
		} else if result.NumTurns > 0 && result.SessionID != "" && !continuing {
			// Claude did work but returned no text — resume asking for a summary
			d.log.BotText("\u26a0 Empty result \u2014 retrying for summary\u2026")
			summary, err := d.agent.Run(ctx, "Reply with a brief summary of what you just did. Do not run any more tools.", result.SessionID)
			if err != nil || strings.TrimSpace(summary.Result) == "" {
				d.sendMessage(chatJID, "Done \u2713")
			} else {
				preview := summary.Result
				if len(preview) > 200 {
					preview = preview[:200] + "\u2026"
				}
				d.log.BotText(preview)
				d.sendMessage(chatJID, summary.Result)
			}
		} else if !continuing {
			d.sendMessage(chatJID, "Done \u2713")
		}

		if !continuing {
			break
		}

		// Auto-continue: resume Claude with no new user input
		d.log.Status("Auto-continuing (total: %d turns, $%.2f)...", totalTurns, totalCost)
		currentPrompt = "Continue from where you left off."
	}

	close(typingDone)
	d.startConversation(chatJID, needInput)
	d.signalActivity()

	d.log.Status("Done (%d total turns, $%.2f)", totalTurns, totalCost)

	// Stop "typing..." indicator
	d.sendPresence(chatJID, false)

	// Ack all messages and mark as read in WhatsApp
	for _, msg := range msgs {
		d.store.Ack(msg.ID)
	}
	d.markRead(msgs)
	d.log.Status("Waiting for reply...")
}

func (d *Daemon) markRead(msgs []store.Message) {
	// Group by sender for correct read receipts
	bySender := make(map[string][]string)
	chatJID := ""
	for _, m := range msgs {
		if m.WhatsAppID == "" {
			continue
		}
		chatJID = m.Chat
		bySender[m.From] = append(bySender[m.From], m.WhatsAppID)
	}

	for sender, ids := range bySender {
		if err := d.msger.MarkRead(chatJID, sender, ids); err != nil {
			d.log.Warn("MarkRead failed: %v", err)
		}
	}
}

func (d *Daemon) sendPresence(chatJID string, composing bool) {
	if err := d.msger.SendPresence(chatJID, composing); err != nil {
		d.log.Error("SendPresence failed: %v", err)
	}
}

func isHelpCommand(text string) bool {
	return strings.TrimSpace(text) == "/help"
}

func isCleanSessionCommand(text string) bool {
	cmd := strings.TrimSpace(text)
	return cmd == "/cleanSession" || cmd == "/cleansession"
}

func isExitCommand(text string) bool {
	return strings.TrimSpace(text) == "/exit"
}

func (d *Daemon) handleHelp(chatJID string) {
	help := "*Butler Commands*\n\n" +
		"/help — Show this message\n" +
		"/exit — Restart the daemon\n" +
		"/cleanSession — Clear session and context (fresh start)\n" +
		"/create-image <prompt> — Generate an image\n" +
		"/create-image <prompt> <url> — Edit image from URL\n" +
		"Photo + caption /create-image <prompt> — Edit attached image\n\n" +
		"*Draft Mode (Kimi):*\n" +
		"/draft-mode — Start drafting (messages go to buffer, not Claude)\n" +
		"/draft-done — Send buffer to Kimi for prompt refinement\n" +
		"/draft-discard — Cancel draft mode\n\n" +
		"All other /commands (/compact, /new, /think, etc.) and messages are passed directly to Claude."
	d.sendMessage(chatJID, help)
}

func (d *Daemon) handleExit(chatJID string) {
	d.sendMessage(chatJID, "Bye!")
	d.log.Status("Exit requested, shutting down...")
	d.log.Cleanup()
	os.Exit(0)
}

func (d *Daemon) handleCleanSession(chatJID string) {
	if err := d.store.ClearSession(chatJID); err != nil {
		d.log.Error("Failed to clear session: %v", err)
		d.sendMessage(chatJID, "Error clearing session")
		return
	}
	d.endConversation()
	d.log.Status("Session cleared for %s", chatJID)
	d.sendMessage(chatJID, "Session cleared \u2713 Next message starts fresh (no context).")
}

func (d *Daemon) sendMessage(chatJID, text string) {
	// Mark previous bot messages as read before sending a new one
	d.markOldBotMessages(chatJID)

	botPrefix := d.botPrefix
	message := botPrefix + " " + text

	msgID, err := d.msger.SendMessage(chatJID, message)
	if err != nil {
		d.log.Error("Failed to send message: %v", err)
		return
	}

	// Track this message so we can mark it as read later
	d.botMsgMu.Lock()
	d.botMsgIDs = append(d.botMsgIDs, msgID)
	d.botMsgChat = chatJID
	d.botMsgMu.Unlock()
}

// markOldBotMessages marks all tracked bot messages as read (clears old notifications).
func (d *Daemon) markOldBotMessages(chatJID string) {
	d.botMsgMu.Lock()
	ids := d.botMsgIDs
	d.botMsgIDs = nil
	d.botMsgMu.Unlock()

	if len(ids) == 0 {
		return
	}

	ownJID := d.msger.GetOwnID()
	if err := d.msger.MarkRead(chatJID, ownJID, ids); err != nil {
		d.log.Warn("MarkRead (bot msgs): %v", err)
	}
}

// markAllBotMessages marks all tracked bot messages as read (user is engaged).
func (d *Daemon) markAllBotMessages() {
	d.botMsgMu.Lock()
	ids := d.botMsgIDs
	chat := d.botMsgChat
	d.botMsgIDs = nil
	d.botMsgMu.Unlock()

	if len(ids) == 0 {
		return
	}

	ownJID := d.msger.GetOwnID()
	if err := d.msger.MarkRead(chat, ownJID, ids); err != nil {
		d.log.Warn("MarkRead (bot msgs): %v", err)
	}
}

// compactWatchdog waits for CompactDelay of total silence, then compacts the
// session. Any activity (message in, Claude response) resets the timer.
func (d *Daemon) compactWatchdog(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.activityNotify:
			// Activity detected — start/reset the idle timer
		}

		// Wait for CompactDelay of silence
	waitLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.activityNotify:
				// Activity during wait — restart the timer
				continue waitLoop
			case <-time.After(CompactDelay):
				break waitLoop
			}
		}

		// Don't compact if busy, in active conversation, or waiting for user input
		if d.isBusy() || d.isConversationActive() || d.isWaitingInput() {
			continue
		}

		chatJID := d.chatID
		sessionID, _ := d.store.GetSession(chatJID)
		if sessionID == "" {
			continue
		}

		d.compactSession(ctx, chatJID)
	}
}

// compactSession asks Claude to summarize the conversation, then clears the
// session and saves the summary. The next message starts a fresh session with
// the summary prepended as context. Runs in a background goroutine.
const compactPrompt = "Summarize this entire conversation into a concise context note for yourself: " +
	"key decisions, what was done, current state of the code, and any pending items. " +
	"Do not run any tools. This summary will be your context when the conversation resumes."

func (d *Daemon) compactSession(ctx context.Context, chatJID string) {
	sessionID, _ := d.store.GetSession(chatJID)
	if sessionID == "" {
		return
	}

	d.log.Status("Compacting session %s\u2026", sessionID[:min(len(sessionID), 8)])

	result, err := d.agent.Run(ctx, compactPrompt, sessionID)
	if err != nil {
		d.log.Warn("Compact failed: %v", err)
		return
	}

	summary := strings.TrimSpace(result.Result)
	if summary != "" {
		d.store.SetSummary(chatJID, summary)
	}

	// Clear only the session ID, keep the summary for next prompt
	d.store.ResetSession(chatJID)
	d.log.Status("Session compacted ($%.2f)", result.CostUSD)
}

// sendImageRe matches <send-image path="...">caption</send-image> or self-closing <send-image path="..." />.
var sendImageRe = regexp.MustCompile(`(?s)<send-image\s+path="([^"]+)"(?:\s*/>|>(.*?)</send-image>)`)

// processResponseImages extracts <send-image> tags from the response.
// Single image: sent as a WhatsApp image. Multiple images: combined into
// a slideshow video (5s per image) via ffmpeg and sent as a video.
func (d *Daemon) processResponseImages(chatJID, response string) string {
	matches := sendImageRe.FindAllStringSubmatchIndex(response, -1)
	if len(matches) == 0 {
		return response
	}

	// Collect images in document order before modifying the string
	var images []responseImage
	for _, m := range matches {
		imgPath := response[m[2]:m[3]]
		caption := ""
		if m[4] >= 0 {
			caption = response[m[4]:m[5]]
		}
		images = append(images, responseImage{imgPath, caption})
	}

	// Strip all tags (reverse order so indices stay valid)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		response = response[:m[0]] + response[m[1]:]
	}

	if len(images) == 1 {
		// Single image — send as image
		imgData, err := os.ReadFile(images[0].path)
		if err != nil {
			d.log.Error("send-image: read %s: %v", images[0].path, err)
		} else {
			d.sendImage(chatJID, imgData, images[0].caption)
			d.log.Info("Sent image: %s (%d bytes)", images[0].path, len(imgData))
		}
	} else {
		// Multiple images — create slideshow video
		d.sendImageSlideshow(chatJID, images)
	}

	return strings.TrimSpace(response)
}

// sendImageSlideshow creates a video slideshow from multiple images using ffmpeg
// (5 seconds per image) and sends it as a WhatsApp video.
type responseImage struct {
	path    string
	caption string
}

func (d *Daemon) sendImageSlideshow(chatJID string, images []responseImage) {
	tmpDir := config.TmpPath(d.repoDir)
	os.MkdirAll(tmpDir, 0755)

	// Build ffmpeg concat input file
	inputFile := filepath.Join(tmpDir, fmt.Sprintf("slideshow-input-%s.txt", uuid.New().String()[:8]))
	outputFile := filepath.Join(tmpDir, fmt.Sprintf("slideshow-%s.mp4", uuid.New().String()[:8]))

	var inputContent strings.Builder
	for _, img := range images {
		// ffmpeg concat format: file 'path'\nduration N\n
		inputContent.WriteString(fmt.Sprintf("file '%s'\nduration 1\n", img.path))
	}
	// Repeat last image to ensure it shows for the full duration
	if len(images) > 0 {
		inputContent.WriteString(fmt.Sprintf("file '%s'\n", images[len(images)-1].path))
	}

	if err := os.WriteFile(inputFile, []byte(inputContent.String()), 0644); err != nil {
		d.log.Error("slideshow: write input file: %v", err)
		d.sendImagesFallback(chatJID, images)
		return
	}
	defer os.Remove(inputFile)

	// Run ffmpeg
	d.log.Info("Creating slideshow from %d images...", len(images))
	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", inputFile,
		"-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black",
		"-c:v", "libx264",
		"-r", "30",
		"-pix_fmt", "yuv420p",
		"-preset", "fast",
		outputFile,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		d.log.Error("slideshow: ffmpeg failed: %v\n%s", err, string(output))
		// Fallback: send images individually
		d.sendImagesFallback(chatJID, images)
		return
	}
	defer os.Remove(outputFile)

	videoData, err := os.ReadFile(outputFile)
	if err != nil {
		d.log.Error("slideshow: read output: %v", err)
		d.sendImagesFallback(chatJID, images)
		return
	}

	// Build caption from all image captions
	var captions []string
	for i, img := range images {
		if img.caption != "" {
			captions = append(captions, fmt.Sprintf("%d. %s", i+1, img.caption))
		}
	}
	caption := strings.Join(captions, "\n")
	if caption == "" {
		caption = fmt.Sprintf("%d images", len(images))
	}

	d.sendVideo(chatJID, videoData, caption)
	d.log.Info("Sent slideshow video: %d images, %d bytes", len(images), len(videoData))
}

// sendImagesFallback sends images individually (used when ffmpeg is unavailable).
func (d *Daemon) sendImagesFallback(chatJID string, images []responseImage) {
	d.log.Warn("Falling back to individual image sends")
	for i, img := range images {
		imgData, err := os.ReadFile(img.path)
		if err != nil {
			d.log.Error("send-image: read %s: %v", img.path, err)
			continue
		}
		if i > 0 {
			time.Sleep(1 * time.Second)
		}
		d.sendImage(chatJID, imgData, img.caption)
		d.log.Info("Sent image: %s (%d bytes)", img.path, len(imgData))
	}
}

// sendVideo sends a video message with the bot prefix.
func (d *Daemon) sendVideo(chatJID string, videoData []byte, caption string) {
	botPrefix := d.botPrefix
	fullCaption := botPrefix + " " + caption

	if err := d.msger.SendVideo(chatJID, videoData, fullCaption); err != nil {
		d.log.Error("Failed to send video: %v", err)
	}
}

// startInput reads from stdin in raw mode and processes input as local messages.
// Raw mode disables terminal echo and line buffering, giving us full control
// over what appears on screen — typed characters are echoed at the prompt row
// via the Logger, preventing them from corrupting the scroll region.
func (d *Daemon) startInput(ctx context.Context) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		d.log.Warn("Raw mode unavailable: %v", err)
		return
	}
	defer term.Restore(fd, oldState)

	var line []byte

	// Reader goroutine — reads stdin in background so we can select on ctx.Done
	readCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				readCh <- data
			}
			if err != nil {
				close(readCh)
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-readCh:
			if !ok {
				return
			}

			i := 0
			for i < len(data) {
				b := data[i]

				switch {
				case b == 3: // Ctrl+C
					term.Restore(fd, oldState)
					p, _ := os.FindProcess(os.Getpid())
					p.Signal(syscall.SIGINT)
					return

				case b == 4: // Ctrl+D — quit if line empty
					if len(line) == 0 {
						term.Restore(fd, oldState)
						p, _ := os.FindProcess(os.Getpid())
						p.Signal(syscall.SIGINT)
						return
					}
					i++

				case b == 13 || b == 10: // Enter
					text := strings.TrimSpace(string(line))
					line = nil
					d.log.ClearInput()
					if text != "" {
						d.handleTUIInput(text)
					}
					i++

				case b == 21: // Ctrl+U — clear line
					line = nil
					d.log.UpdateInput("")
					i++

				case b == 23: // Ctrl+W — delete word
					s := strings.TrimRight(string(line), " ")
					if idx := strings.LastIndex(s, " "); idx >= 0 {
						line = []byte(s[:idx+1])
					} else {
						line = nil
					}
					d.log.UpdateInput(string(line))
					i++

				case b == 127 || b == 8: // Backspace
					if len(line) > 0 {
						_, size := utf8.DecodeLastRune(line)
						line = line[:len(line)-size]
						d.log.UpdateInput(string(line))
					}
					i++

				case b == 27: // ESC — skip escape sequences (arrow keys, etc.)
					i++
					if i < len(data) && data[i] == '[' {
						i++
						for i < len(data) && ((data[i] >= '0' && data[i] <= '9') || data[i] == ';') {
							i++
						}
						if i < len(data) {
							i++ // skip final byte (letter or ~)
						}
					}

				case b >= 32: // Printable character (ASCII + UTF-8 multi-byte)
					rn, size := utf8.DecodeRune(data[i:])
					if rn != utf8.RuneError && size > 0 {
						line = append(line, data[i:i+size]...)
						i += size
					} else {
						i++ // skip invalid byte
					}
					d.log.UpdateInput(string(line))

				default:
					i++ // skip unknown control characters
				}
			}
		}
	}
}

// handleTUIInput processes a submitted line from TUI input.
func (d *Daemon) handleTUIInput(text string) {
	chatJID := d.chatID
	botPrefix := d.botPrefix

	// Echo to messenger so the conversation is visible there
	if chatJID != "" {
		echoMsg := fmt.Sprintf("%s [TUI] %s", botPrefix, text)
		if _, err := d.msger.SendMessage(chatJID, echoMsg); err != nil {
			d.log.Warn("TUI echo failed: %v", err)
		}
	}

	// Insert into store as a regular message
	msg := store.Message{
		ID:        uuid.New().String(),
		From:      "TUI",
		Chat:      chatJID,
		Content:   text,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if err := d.store.Insert(msg); err != nil {
		d.log.Error("Failed to store TUI message: %v", err)
		return
	}

	// Log and notify poll loop
	d.log.UserMsg("TUI", text, time.Now(), false, false)
	d.signalActivity()

	select {
	case d.msgNotify <- struct{}{}:
	default:
	}
}
