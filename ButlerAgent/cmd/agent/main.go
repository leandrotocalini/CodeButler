package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/leandrotocalini/CodeButler/internal/access"
	"github.com/leandrotocalini/CodeButler/internal/audio"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

var (
	currentChatID      string
	waitingForResponse bool
)

func main() {
	// Load config from current directory
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Connect to WhatsApp
	fmt.Println("📱 Connecting to WhatsApp...")
	client, err := whatsapp.Connect(cfg.WhatsApp.SessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	botPrefix := cfg.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	fmt.Println("✅ CodeButler agent running")
	fmt.Printf("📋 Monitoring group: %s\n", cfg.WhatsApp.GroupName)
	fmt.Println()

	// Start file watchers in background
	go watchResponses(client, botPrefix, cfg)
	go watchQuestions(client, botPrefix, cfg)

	// Register message handler
	client.OnMessage(func(msg whatsapp.Message) {
		// Ignore bot's own messages
		if strings.HasPrefix(msg.Content, botPrefix) {
			return
		}

		// Access control
		if !access.IsAllowed(msg, cfg) {
			return
		}

		currentChatID = msg.Chat

		// Handle voice messages
		content := msg.Content
		if msg.IsVoice {
			if cfg.OpenAI.APIKey == "" || cfg.OpenAI.APIKey == "sk-test-key-for-phase-testing" {
				return
			}

			text, err := handleVoiceMessage(client, cfg.OpenAI.APIKey, msg)
			if err != nil {
				client.SendMessage(msg.Chat, getVoiceErrorMessage(err))
				return
			}
			content = text
			fmt.Printf("🎤 Voice transcribed: \"%s\"\n", content)
		}

		// Check if this is a numeric response to a question
		if waitingForResponse && isNumericResponse(content) {
			// Write answer to file for Claude Code to read
			os.WriteFile(".codebutler-answer", []byte(content), 0644)
			waitingForResponse = false
			fmt.Printf("📝 Answer written: %s\n", content)
			return
		}

		// Print incoming message for Claude Code to see
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("📨 INCOMING WHATSAPP MESSAGE")
		fmt.Printf("From: %s\n", msg.From)
		fmt.Printf("Chat: %s\n", msg.Chat)
		fmt.Printf("Content: %s\n", content)
		if msg.IsVoice {
			fmt.Println("Type: Voice (transcribed)")
		}
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Printf("EXECUTE_PROMPT: %s\n", content)
		fmt.Println()

		// Send acknowledgment
		client.SendMessage(msg.Chat, botPrefix+" Mensaje recibido! Procesando...")
	})

	// Keep alive
	fmt.Println("👂 Listening for WhatsApp messages...")
	fmt.Println("   Also monitoring for responses and questions")
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()
	select {}
}

// Watch for .codebutler-response file (Claude Code sends final response)
func watchResponses(client *whatsapp.Client, botPrefix string, cfg *config.Config) {
	for {
		time.Sleep(500 * time.Millisecond)

		if _, err := os.Stat(".codebutler-response"); err == nil {
			// File exists, read and send to WhatsApp
			content, err := os.ReadFile(".codebutler-response")
			if err != nil {
				continue
			}

			message := botPrefix + " " + string(content)
			if currentChatID != "" {
				client.SendMessage(currentChatID, message)
				fmt.Printf("📤 Response sent to WhatsApp\n")
			} else {
				// Fallback to configured group
				client.SendMessage(cfg.WhatsApp.GroupJID, message)
				fmt.Printf("📤 Response sent to configured group\n")
			}

			// Delete file after sending
			os.Remove(".codebutler-response")
		}
	}
}

// Watch for .codebutler-question file (Claude Code asks a question)
func watchQuestions(client *whatsapp.Client, botPrefix string, cfg *config.Config) {
	for {
		time.Sleep(500 * time.Millisecond)

		if _, err := os.Stat(".codebutler-question"); err == nil {
			// File exists, read and send to WhatsApp
			content, err := os.ReadFile(".codebutler-question")
			if err != nil {
				continue
			}

			message := botPrefix + " " + string(content)
			if currentChatID != "" {
				client.SendMessage(currentChatID, message)
				fmt.Printf("❓ Question sent to WhatsApp\n")
			} else {
				// Fallback to configured group
				client.SendMessage(cfg.WhatsApp.GroupJID, message)
				fmt.Printf("❓ Question sent to configured group\n")
			}

			// Mark that we're waiting for response
			waitingForResponse = true

			// Delete question file after sending
			os.Remove(".codebutler-question")
		}
	}
}

func isNumericResponse(text string) bool {
	text = strings.TrimSpace(text)
	_, err := strconv.Atoi(text)
	return err == nil
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

func getVoiceErrorMessage(err error) string {
	errStr := err.Error()

	if strings.Contains(errStr, "insufficient_quota") || strings.Contains(errStr, "429") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"💳 Tu cuenta de OpenAI se quedó sin créditos.\n" +
			"💡 Agregá saldo en: https://platform.openai.com/account/billing"
	}

	if strings.Contains(errStr, "401") || strings.Contains(errStr, "invalid") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"🔑 El API key de OpenAI es inválido.\n" +
			"💡 Verificá tu configuración."
	}

	if strings.Contains(errStr, "rate_limit") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"⏳ Demasiadas solicitudes a OpenAI.\n" +
			"💡 Intentá de nuevo en unos minutos."
	}

	if strings.Contains(errStr, "download failed") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"📡 Error al descargar el audio de WhatsApp.\n" +
			"💡 Intentá enviarlo de nuevo."
	}

	return "❌ No pude transcribir el mensaje de voz.\n\n" +
		"⚠️  Error desconocido.\n" +
		"💡 Intentá de nuevo más tarde."
}
