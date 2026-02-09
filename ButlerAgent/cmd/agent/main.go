package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/leandrotocalini/CodeButler/internal/access"
	"github.com/leandrotocalini/CodeButler/internal/audio"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/protocol"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func main() {
	fmt.Println("🤖 CodeButler Agent")
	fmt.Println()

	// Load config
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatal("❌ Failed to load config.json:", err)
	}

	fmt.Println("📝 Configuration loaded")
	fmt.Printf("   Group: %s\n", cfg.WhatsApp.GroupName)
	fmt.Printf("   Voice: %v\n", cfg.OpenAI.APIKey != "")
	fmt.Println()

	// Initialize protocol directory
	if err := protocol.Initialize(); err != nil {
		log.Fatal("❌ Failed to initialize protocol:", err)
	}

	// Connect to WhatsApp
	fmt.Println("📱 Connecting to WhatsApp...")
	client, err := whatsapp.Connect(cfg.WhatsApp.SessionPath)
	if err != nil {
		log.Fatal("❌ Failed to connect:", err)
	}
	defer client.Disconnect()

	fmt.Println("✅ Connected to WhatsApp")
	fmt.Println()

	// Get bot prefix
	botPrefix := cfg.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	// Register message handler
	fmt.Println("👂 Listening for messages...")
	fmt.Println("   Protocol: JSON files in /tmp/codebutler/")
	fmt.Println()

	client.OnMessage(func(msg whatsapp.Message) {
		// Ignore bot's own messages
		if strings.HasPrefix(msg.Content, botPrefix) {
			return
		}

		// Access control
		if !access.IsAllowed(msg, cfg) {
			fmt.Printf("⛔ Blocked message from: %s\n", msg.From)
			return
		}

		fmt.Printf("📨 Message from %s: %s\n", msg.From, msg.Content)

		// Handle voice messages
		content := msg.Content
		var transcript *string
		if msg.IsVoice {
			if cfg.OpenAI.APIKey == "" {
				fmt.Println("   ⚠️  Voice message ignored (no OpenAI key)")
				return
			}

			fmt.Println("   🎤 Transcribing voice...")
			text, err := handleVoiceMessage(client, cfg.OpenAI.APIKey, msg)
			if err != nil {
				fmt.Printf("   ❌ Transcription failed: %v\n", err)
				client.SendMessage(msg.Chat, botPrefix+" ❌ Voice transcription failed")
				return
			}

			content = text
			transcript = &text
			fmt.Printf("   ✅ Transcript: %s\n", text)
		}

		// Write incoming message
		incoming := &protocol.IncomingMessage{
			MessageID:  uuid.New().String(),
			From:       protocol.Contact{JID: msg.From, Name: ""},
			Chat:       protocol.Contact{JID: msg.Chat, Name: ""},
			Content:    content,
			IsVoice:    msg.IsVoice,
			Transcript: transcript,
		}

		if err := protocol.WriteIncoming(incoming); err != nil {
			fmt.Printf("   ❌ Failed to write incoming.json: %v\n", err)
			return
		}

		fmt.Println("   ✅ Written to /tmp/codebutler/incoming.json")
		fmt.Println("   ⏳ Waiting for Claude to respond...")
	})

	// Start outgoing monitor (sends responses to WhatsApp)
	go monitorOutgoing(client, botPrefix)

	// Start question/answer monitor
	go monitorQuestions(client, botPrefix)

	// Wait for interrupt
	fmt.Println("✅ Agent running")
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	fmt.Println("👋 Shutting down...")
}

func monitorOutgoing(client *whatsapp.Client, botPrefix string) {
	for {
		time.Sleep(1 * time.Second)

		if !protocol.FileExists(protocol.OutgoingPath) {
			continue
		}

		resp, err := protocol.ReadOutgoing()
		if err != nil {
			fmt.Printf("⚠️  Failed to read outgoing.json: %v\n", err)
			protocol.DeleteFile(protocol.OutgoingPath)
			continue
		}

		// Send to WhatsApp
		message := botPrefix + " " + resp.Content
		if err := client.SendMessage(resp.ChatJID, message); err != nil {
			fmt.Printf("❌ Failed to send message: %v\n", err)
		} else {
			fmt.Println("📤 Response sent to WhatsApp")
		}

		// Delete file
		protocol.DeleteFile(protocol.OutgoingPath)
	}
}

func monitorQuestions(client *whatsapp.Client, botPrefix string) {
	for {
		time.Sleep(1 * time.Second)

		if !protocol.FileExists(protocol.QuestionPath) {
			continue
		}

		q, err := protocol.ReadQuestion()
		if err != nil {
			fmt.Printf("⚠️  Failed to read question.json: %v\n", err)
			protocol.DeleteFile(protocol.QuestionPath)
			continue
		}

		fmt.Printf("❓ Question: %s\n", q.Text)

		// Format options
		message := fmt.Sprintf("%s %s\n", botPrefix, q.Text)
		for i, opt := range q.Options {
			message += fmt.Sprintf("%d. %s\n", i+1, opt)
		}

		// Send question
		if err := client.SendMessage(q.ChatJID, message); err != nil {
			fmt.Printf("❌ Failed to send question: %v\n", err)
			protocol.DeleteFile(protocol.QuestionPath)
			continue
		}

		// Delete question file
		protocol.DeleteFile(protocol.QuestionPath)

		// Wait for answer (with timeout)
		timeout := time.After(time.Duration(q.Timeout) * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		answered := false
		for !answered {
			select {
			case <-timeout:
				fmt.Println("   ⏱️  Question timed out")
				return

			case <-ticker.C:
				if protocol.FileExists(protocol.AnswerPath) {
					ans, err := protocol.ReadAnswer()
					if err == nil && ans.QuestionID == q.QuestionID {
						fmt.Printf("   ✅ Answer received: %s\n", ans.Text)
						answered = true
					}
				}
			}
		}
	}
}

func handleVoiceMessage(client *whatsapp.Client, apiKey string, msg whatsapp.Message) (string, error) {
	// Download audio
	audioPath, err := client.DownloadAudioFromMessage(msg)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(audioPath)

	// Transcribe
	text, err := audio.TranscribeAudio(audioPath, apiKey)
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	return text, nil
}
