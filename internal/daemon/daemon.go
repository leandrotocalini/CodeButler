package daemon

import (
	"context"
	"fmt"
	"os"
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
	"github.com/leandrotocalini/CodeButler/internal/store"
	"github.com/leandrotocalini/CodeButler/internal/transcribe"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
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
	client    *whatsapp.Client
	agent     *agent.Agent
	log       *Logger

	clientMu  sync.Mutex
	connState whatsapp.ConnectionState

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

func New(repoCfg *config.RepoConfig, repoDir, version string) *Daemon {
	return &Daemon{
		repoCfg:        repoCfg,
		repoDir:        repoDir,
		version:        version,
		log:            NewLogger(500),
		imgHandler:     newImageCommandHandler(),
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
	d.log.Header("CodeButler \u00b7 %s \u00b7 http://localhost:%d", d.repoCfg.WhatsApp.GroupName, d.webPort)

	// Connect WhatsApp
	if err := d.connectWhatsApp(); err != nil {
		return fmt.Errorf("connect WhatsApp: %w", err)
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

	d.clientMu.Lock()
	if d.client != nil {
		d.client.Disconnect()
	}
	d.clientMu.Unlock()

	d.log.Status("Goodbye!")
	d.log.Cleanup()
	return nil
}

func (d *Daemon) connectWhatsApp() error {
	sessionPath := config.SessionPath(d.repoDir)
	whatsapp.SetDeviceName("CodeButler:" + filepath.Base(d.repoDir))

	d.log.Status("WhatsApp: connecting...")
	for attempt := 1; attempt <= 5; attempt++ {
		client, err := whatsapp.Connect(sessionPath)
		if err != nil {
			d.log.Error("WhatsApp attempt %d/5: %v", attempt, err)
			if attempt < 5 {
				delay := time.Duration(min(attempt*5, 30)) * time.Second
				time.Sleep(delay)
				continue
			}
			return fmt.Errorf("all connection attempts failed")
		}

		d.setupClient(client)
		d.log.Status("WhatsApp: connected")

		// Announce startup (only on initial connect, not reconnects)
		d.sendMessage(d.repoCfg.WhatsApp.GroupJID, fmt.Sprintf("I am back. I am version %s", d.version))

		return nil
	}
	return fmt.Errorf("unreachable")
}

func (d *Daemon) setupClient(client *whatsapp.Client) {
	botPrefix := d.repoCfg.WhatsApp.BotPrefix
	groupJID := d.repoCfg.WhatsApp.GroupJID

	client.OnConnectionEvent(func(state whatsapp.ConnectionState) {
		d.clientMu.Lock()
		prev := d.connState
		d.connState = state
		d.clientMu.Unlock()
		switch {
		case state == whatsapp.StateConnected && prev != whatsapp.StateConnected:
			d.log.Status("WhatsApp: reconnected")
		case state == whatsapp.StateLoggedOut:
			d.log.Warn("WhatsApp: logged out")
		case state != whatsapp.StateConnected && prev == whatsapp.StateConnected:
			d.log.Warn("WhatsApp: %s", state)
		}
	})

	client.OnMessage(func(msg whatsapp.Message) {
		// Filter by group
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

		// User is engaged — mark old bot messages as read
		d.markAllBotMessages()

		// Mark as read immediately so the sender sees blue ticks
		if msg.ID != "" {
			if err := client.MarkRead(msg.Chat, msg.From, []string{msg.ID}); err != nil {
				d.log.Warn("MarkRead failed: %v", err)
			}
		}

		content := msg.Content

		// Transcribe voice messages
		if msg.IsVoice {
			apiKey := d.repoCfg.OpenAI.APIKey
			if apiKey == "" {
				d.log.Warn("Voice message received but no OpenAI API key configured")
				content = "[Voice message — no OpenAI key for transcription]"
			} else {
				audioPath, err := client.DownloadAudioFromMessage(msg)
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

		// Download image messages
		if msg.IsImage {
			imgData, err := client.DownloadImageFromMessage(msg)
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
		d.log.UserMsg("WhatsApp", content, time.Now(), msg.IsVoice, msg.IsImage)
	})

	d.clientMu.Lock()
	d.client = client
	d.connState = client.GetState()
	d.clientMu.Unlock()
}

func (d *Daemon) connectionWatchdog() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	disconnectedAt := time.Time{}

	for range ticker.C {
		d.clientMu.Lock()
		state := d.connState
		client := d.client
		d.clientMu.Unlock()

		switch state {
		case whatsapp.StateConnected:
			disconnectedAt = time.Time{}

		case whatsapp.StateLoggedOut:
			d.log.Warn("WhatsApp logged out — run codebutler --setup to re-scan QR")
			return

		case whatsapp.StateDisconnected, whatsapp.StateReconnecting:
			if disconnectedAt.IsZero() {
				disconnectedAt = time.Now()
				d.log.Warn("WhatsApp disconnected, watching...")
			} else if time.Since(disconnectedAt) > 2*time.Minute {
				d.log.Warn("Disconnected >2min, forcing reconnect...")
				if client != nil {
					client.Disconnect()
				}
				d.clientMu.Lock()
				d.client = nil
				d.clientMu.Unlock()

				if err := d.connectWhatsApp(); err != nil {
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
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()

	if client == nil {
		return
	}

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
		if err := client.MarkRead(chatJID, sender, ids); err != nil {
			d.log.Warn("MarkRead failed: %v", err)
		}
	}
}

func (d *Daemon) sendPresence(chatJID string, composing bool) {
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()

	if client == nil {
		return
	}

	if err := client.SendPresence(chatJID, composing); err != nil {
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
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()

	if client == nil {
		d.log.Error("Can't send: WhatsApp not connected")
		return
	}

	// Mark previous bot messages as read before sending a new one
	d.markOldBotMessages(client, chatJID)

	botPrefix := d.repoCfg.WhatsApp.BotPrefix
	message := botPrefix + " " + text

	msgID, err := client.SendMessage(chatJID, message)
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
func (d *Daemon) markOldBotMessages(client *whatsapp.Client, chatJID string) {
	d.botMsgMu.Lock()
	ids := d.botMsgIDs
	d.botMsgIDs = nil
	d.botMsgMu.Unlock()

	if len(ids) == 0 {
		return
	}

	ownJID := client.GetJID().String()
	if err := client.MarkRead(chatJID, ownJID, ids); err != nil {
		d.log.Warn("MarkRead (bot msgs): %v", err)
	}
}

// markAllBotMessages marks all tracked bot messages as read (user is engaged).
func (d *Daemon) markAllBotMessages() {
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()

	if client == nil {
		return
	}

	d.botMsgMu.Lock()
	ids := d.botMsgIDs
	chat := d.botMsgChat
	d.botMsgIDs = nil
	d.botMsgMu.Unlock()

	if len(ids) == 0 {
		return
	}

	ownJID := client.GetJID().String()
	if err := client.MarkRead(chat, ownJID, ids); err != nil {
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

		chatJID := d.repoCfg.WhatsApp.GroupJID
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

// processResponseImages extracts <send-image> tags from the response, sends each
// image via WhatsApp, and returns the remaining text with tags removed.
func (d *Daemon) processResponseImages(chatJID, response string) string {
	matches := sendImageRe.FindAllStringSubmatchIndex(response, -1)
	if len(matches) == 0 {
		return response
	}

	// Process in reverse so indices stay valid after removal
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		imgPath := response[m[2]:m[3]]
		caption := ""
		if m[4] >= 0 {
			caption = response[m[4]:m[5]]
		}

		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			d.log.Error("send-image: read %s: %v", imgPath, err)
			response = response[:m[0]] + response[m[1]:]
			continue
		}

		d.sendImage(chatJID, imgData, caption)
		d.log.Info("Sent image: %s (%d bytes)", imgPath, len(imgData))

		response = response[:m[0]] + response[m[1]:]
	}

	return strings.TrimSpace(response)
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
	chatJID := d.repoCfg.WhatsApp.GroupJID
	botPrefix := d.repoCfg.WhatsApp.BotPrefix

	// Echo to WhatsApp so the conversation is visible there
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()
	if client != nil && chatJID != "" {
		echoMsg := fmt.Sprintf("%s [TUI] %s", botPrefix, text)
		if _, err := client.SendMessage(chatJID, echoMsg); err != nil {
			d.log.Warn("TUI echo to WhatsApp failed: %v", err)
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
