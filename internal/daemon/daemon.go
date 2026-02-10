package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/leandrotocalini/CodeButler/internal/agent"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/store"
	"github.com/leandrotocalini/CodeButler/internal/transcribe"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
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
	convMu       sync.Mutex
	convActive   bool      // true while waiting for user reply
	convResponse time.Time // when Claude last responded (cutoff for follow-ups vs queued)

	// Image generation command handler
	imgHandler *imageCommandHandler

	// Message notification channel
	msgNotify chan struct{}

	// Web server
	webPort   int
	startTime time.Time
}

func New(repoCfg *config.RepoConfig, repoDir string) *Daemon {
	return &Daemon{
		repoCfg:    repoCfg,
		repoDir:    repoDir,
		log:        NewLogger(500),
		imgHandler: newImageCommandHandler(),
		msgNotify:  make(chan struct{}, 1),
		startTime:  time.Now(),
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
	repoName := filepath.Base(d.repoDir)
	d.log.Info("CodeButler starting for %s", repoName)
	d.log.Info("Repo: %s", d.repoDir)
	d.log.Info("Group: %s", d.repoCfg.WhatsApp.GroupName)

	// Open store
	dbPath := filepath.Join(config.RepoDir(d.repoDir), "store.db")
	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	d.store = st
	defer st.Close()
	d.log.Info("Store opened: %s", dbPath)

	// Create agent
	d.agent = agent.New(d.repoDir, d.repoCfg.Claude)
	d.log.Info("Agent ready (maxTurns=%d, timeout=%dm)", d.repoCfg.Claude.MaxTurns, d.repoCfg.Claude.Timeout)

	// Start web server
	d.startWeb()

	// Connect WhatsApp
	if err := d.connectWhatsApp(); err != nil {
		return fmt.Errorf("connect WhatsApp: %w", err)
	}

	// Start watchdog
	go d.connectionWatchdog()

	d.log.Info("Daemon running. Waiting for messages...")

	// Start poll loop in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.pollLoop(ctx)

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh

	d.log.Info("Received %s, shutting down...", sig)
	cancel()

	d.clientMu.Lock()
	if d.client != nil {
		d.client.Disconnect()
	}
	d.clientMu.Unlock()

	d.log.Info("Goodbye!")
	return nil
}

func (d *Daemon) connectWhatsApp() error {
	sessionPath := config.SessionPath(d.repoDir)
	whatsapp.SetDeviceName("CodeButler:" + filepath.Base(d.repoDir))

	for attempt := 1; attempt <= 5; attempt++ {
		d.log.Info("WhatsApp connecting (attempt %d/5)...", attempt)

		client, err := whatsapp.Connect(sessionPath)
		if err != nil {
			d.log.Error("WhatsApp connect attempt %d/5: %v", attempt, err)
			if attempt < 5 {
				delay := time.Duration(min(attempt*5, 30)) * time.Second
				d.log.Info("Retrying in %s...", delay)
				time.Sleep(delay)
				continue
			}
			return fmt.Errorf("all connection attempts failed")
		}

		d.setupClient(client)
		d.log.Info("WhatsApp connected")
		return nil
	}
	return fmt.Errorf("unreachable")
}

func (d *Daemon) setupClient(client *whatsapp.Client) {
	botPrefix := d.repoCfg.WhatsApp.BotPrefix
	groupJID := d.repoCfg.WhatsApp.GroupJID

	client.OnConnectionEvent(func(state whatsapp.ConnectionState) {
		d.clientMu.Lock()
		d.connState = state
		d.clientMu.Unlock()
		d.log.Info("WhatsApp state: %s", state)
	})

	d.log.Info("Listening on group: %s (JID: %s)", d.repoCfg.WhatsApp.GroupName, groupJID)
	d.log.Info("Bot prefix: %q", botPrefix)

	client.OnMessage(func(msg whatsapp.Message) {
		// Filter by group
		if groupJID != "" && msg.Chat != groupJID {
			return
		}

		d.log.Debug("Incoming: from=%s fromMe=%v content=%q",
			msg.From, msg.IsFromMe, msg.Content[:min(len(msg.Content), 60)])
		// Filter bot's own responses (they start with the prefix)
		if botPrefix != "" && strings.HasPrefix(msg.Content, botPrefix) {
			d.log.Debug("Filtered: bot prefix match")
			return
		}

		// Intercept /help command
		if isHelpCommand(msg.Content) {
			go d.handleHelp(msg.Chat)
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

		// Truncate for display
		preview := content
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		d.log.Info("Message from %s: %s", msg.From, preview)
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

func (d *Daemon) startConversation() {
	d.convMu.Lock()
	d.convActive = true
	d.convResponse = time.Now()
	d.convMu.Unlock()
}

func (d *Daemon) endConversation() {
	d.convMu.Lock()
	d.convActive = false
	d.convResponse = time.Time{}
	d.convMu.Unlock()
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
			if len(queued) > 0 {
				d.log.Info("Conversation active — processing %d follow-up(s), %d queued for later", len(followUps), len(queued))
			} else {
				d.log.Info("Conversation active — processing %d follow-up(s)", len(followUps))
			}
			d.processBatch(ctx, followUps)
			return
		}

		// No follow-ups yet. Wait for user to reply up to ReplyWindow.
		// If a message arrives during the wait, re-check immediately.
		d.log.Info("Waiting for user reply (%s timeout, %d queued)...", ReplyWindow, len(queued))
		select {
		case <-ctx.Done():
			return
		case <-d.msgNotify:
			// New message arrived — re-check if it's a follow-up
			d.handleNewMessages(ctx)
			return
		case <-time.After(ReplyWindow):
			// No reply — conversation is done
			d.log.Info("No reply in %s — conversation ended", ReplyWindow)
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
		d.log.Info("Processing %d queued message(s) from ended conversation", len(msgs))
		d.processBatch(ctx, msgs)
		return
	}

	// Cold: wait for more messages to accumulate
	d.log.Info("Accumulating messages (waiting %s)...", AccumulationWindow)
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

	// Build prompt from all pending messages
	var prompt strings.Builder
	for _, msg := range msgs {
		prompt.WriteString(fmt.Sprintf("[%s] %s: %s\n", msg.Timestamp, msg.From, msg.Content))
	}

	// Get chat JID from first message
	chatJID := msgs[0].Chat

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

	// Get existing session for this chat
	sessionID, _ := d.store.GetSession(chatJID)

	d.log.Info("Spawning Claude (%d message(s))...", len(msgs))
	d.log.Info("── Input ──\n%s── End Input ──", prompt.String())
	if sessionID != "" {
		d.log.Info("Resuming session %s", sessionID[:min(len(sessionID), 12)])
	}

	start := time.Now()
	result, err := d.agent.Run(ctx, prompt.String(), sessionID)
	elapsed := time.Since(start)
	close(typingDone)

	if err != nil {
		d.log.Error("Claude error after %s: %v", elapsed.Round(time.Second), err)
		d.sendPresence(chatJID, false)
		d.sendMessage(chatJID, fmt.Sprintf("Error: %v", err))
		// Still ack messages so they don't loop forever
		for _, msg := range msgs {
			d.store.Ack(msg.ID)
		}
		d.markRead(msgs)
		return
	}

	d.log.Info("Claude finished in %s (turns=%d, cost=$%.4f, error=%v, resultLen=%d)",
		elapsed.Round(time.Second), result.NumTurns, result.CostUSD, result.IsError, len(result.Result))
	d.startConversation()

	// Save new session
	if result.SessionID != "" {
		d.store.SetSession(chatJID, result.SessionID)
		d.log.Debug("Session saved: %s", result.SessionID[:min(len(result.SessionID), 12)])
	}

	// Send response
	response := result.Result
	if result.IsError {
		response = "Error: " + response
		d.log.Warn("Claude returned error:\n%s", response)
	} else {
		d.log.Info("── Output ──\n%s\n── End Output ──", response)
	}
	if strings.TrimSpace(response) != "" {
		d.sendMessage(chatJID, response)
	} else {
		d.log.Warn("Claude returned empty result (likely all tool-use, no text)")
		d.log.Warn("Raw JSON: %s", result.RawJSON[:min(len(result.RawJSON), 500)])
		d.sendMessage(chatJID, "Done ✓")
	}

	// Stop "typing..." indicator
	d.sendPresence(chatJID, false)

	// Ack all messages and mark as read in WhatsApp
	for _, msg := range msgs {
		d.store.Ack(msg.ID)
	}
	d.markRead(msgs)
	d.log.Info("Acked %d message(s)", len(msgs))
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
			d.log.Debug("MarkRead failed: %v", err)
		}
	}
}

func (d *Daemon) sendPresence(chatJID string, composing bool) {
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()

	if client == nil {
		d.log.Warn("SendPresence: client is nil")
		return
	}

	state := "paused"
	if composing {
		state = "composing"
	}

	if err := client.SendPresence(chatJID, composing); err != nil {
		d.log.Error("SendPresence(%s) failed: %v", state, err)
	} else {
		d.log.Info("Presence: %s", state)
	}
}

func isHelpCommand(text string) bool {
	return strings.TrimSpace(text) == "/help"
}

func (d *Daemon) handleHelp(chatJID string) {
	help := "*Butler Commands*\n\n" +
		"/help — Show this message\n" +
		"/create-image <prompt> — Generate an image\n" +
		"/create-image <prompt> <url> — Edit image from URL\n" +
		"Photo + caption /create-image <prompt> — Edit attached image\n\n" +
		"All other /commands (/compact, /new, /think, etc.) and messages are passed directly to Claude."
	d.sendMessage(chatJID, help)
}

func (d *Daemon) sendMessage(chatJID, text string) {
	d.clientMu.Lock()
	client := d.client
	d.clientMu.Unlock()

	if client == nil {
		d.log.Error("Can't send: WhatsApp not connected")
		return
	}

	botPrefix := d.repoCfg.WhatsApp.BotPrefix
	message := botPrefix + " " + text

	if err := client.SendMessage(chatJID, message); err != nil {
		d.log.Error("Failed to send message: %v", err)
	} else {
		d.log.Info("Message sent to WhatsApp (%d chars)", len(text))
	}
}
