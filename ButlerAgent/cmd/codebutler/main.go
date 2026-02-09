package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/leandrotocalini/CodeButler/internal/access"
	"github.com/leandrotocalini/CodeButler/internal/audio"
	"github.com/leandrotocalini/CodeButler/internal/bot"
	"github.com/leandrotocalini/CodeButler/internal/commands"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/setup"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func main() {
	fmt.Println("🤖 CodeButler - WhatsApp Agent for Claude Code SDK")
	fmt.Println()

	// Check if config exists
	_, err := os.Stat("config.json")
	if os.IsNotExist(err) {
		fmt.Println("📋 No config.json found. Starting setup wizard...")
		fmt.Println()

		// Run wizard
		wizardCfg, err := setup.RunWizard()
		if err != nil {
			log.Fatalf("❌ Setup failed: %v", err)
		}

		// Ask to create group
		if err := setup.CreateGroupIfNeeded(wizardCfg.GroupName); err != nil {
			log.Fatalf("❌ Failed to create group: %v", err)
		}

		// Connect to WhatsApp to get group JID
		fmt.Println()
		fmt.Println("📱 Connecting to WhatsApp...")
		client, err := whatsapp.Connect("./whatsapp-session")
		if err != nil {
			log.Fatalf("❌ Failed to connect: %v", err)
		}
		fmt.Println()

		// Wait for connection to stabilize
		time.Sleep(3 * time.Second)

		// Get user info
		info, err := client.GetInfo()
		if err != nil {
			log.Fatalf("❌ Failed to get user info: %v", err)
		}
		fmt.Printf("👤 Connected as: %s\n", info.JID)
		if info.Name != "" {
			fmt.Printf("   Name: %s\n", info.Name)
		}
		fmt.Println()

		// Find the group
		fmt.Println("📋 Looking for your group...")
		groups, err := client.GetGroups()
		if err != nil {
			log.Fatalf("❌ Failed to list groups: %v", err)
		}

		var targetGroupJID string
		for _, group := range groups {
			if group.Name == wizardCfg.GroupName {
				targetGroupJID = group.JID
				fmt.Printf("✅ Found group: %s\n", group.Name)
				fmt.Printf("   JID: %s\n", group.JID)
				break
			}
		}

		if targetGroupJID == "" {
			log.Fatalf("❌ Group '%s' not found. Please create it and try again.", wizardCfg.GroupName)
		}
		fmt.Println()

		// Save config
		fmt.Println("💾 Saving configuration...")
		if err := setup.SaveConfig(wizardCfg, targetGroupJID, info.JID); err != nil {
			log.Fatalf("❌ Failed to save config: %v", err)
		}

		fmt.Println("✅ Configuration saved to config.json")
		fmt.Println()
		fmt.Println("🎉 Setup complete!")
		fmt.Println()
		fmt.Println("📱 Starting CodeButler...")
		fmt.Println()

		// Disconnect and reconnect with new config
		client.Disconnect()
	}

	// Load config
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// Validate and fix config if needed
	configUpdated, err := setup.ValidateAndFixConfig(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to validate config: %v", err)
	}

	// Save updated config if needed
	if configUpdated {
		fmt.Println("💾 Saving updated configuration...")
		if err := config.Save(cfg, "config.json"); err != nil {
			log.Fatalf("❌ Failed to save config: %v", err)
		}
		fmt.Println("✅ Configuration updated")
		fmt.Println()
	}

	fmt.Println("📝 Configuration loaded")
	fmt.Printf("   Group: %s\n", cfg.WhatsApp.GroupName)
	fmt.Printf("   Sources: %s\n", cfg.Sources.RootPath)
	if cfg.OpenAI.APIKey != "" && cfg.OpenAI.APIKey != "sk-test-key-for-phase-testing" {
		fmt.Println("   Voice: Enabled (Whisper)")
	} else {
		fmt.Println("   Voice: Disabled")
	}
	fmt.Println()

	// Connect to WhatsApp
	fmt.Println("📱 Connecting to WhatsApp...")
	client, err := whatsapp.Connect(cfg.WhatsApp.SessionPath)
	if err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer client.Disconnect()
	fmt.Println()

	// Wait for connection to stabilize
	time.Sleep(3 * time.Second)

	// Show connection info
	info, err := client.GetInfo()
	if err != nil {
		log.Fatalf("❌ Failed to get info: %v", err)
	}
	fmt.Printf("👤 Connected as: %s\n", info.JID)
	if info.Name != "" {
		fmt.Printf("   Name: %s\n", info.Name)
	}
	fmt.Println()

	// Create bot with message wrapper that adds bot prefix
	botPrefix := cfg.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]" // Default if not configured
	}

	sendWithPrefix := func(chatID, text string) error {
		return client.SendMessage(chatID, botPrefix+" "+text)
	}

	codebutler := bot.NewBot(cfg, sendWithPrefix)

	// Load repositories
	fmt.Println("📂 Scanning repositories...")
	if err := codebutler.LoadRepositories(); err != nil {
		fmt.Printf("   ⚠️  Failed to load repositories: %v\n", err)
	}
	fmt.Println()

	// Register message handler
	fmt.Println("👂 Listening for messages...")
	fmt.Println()

	client.OnMessage(func(msg whatsapp.Message) {
		// Log message
		fmt.Println("📨 Message received:")
		fmt.Printf("   From: %s\n", msg.From)
		fmt.Printf("   Chat: %s\n", msg.Chat)
		fmt.Printf("   Content: %s\n", msg.Content)
		fmt.Printf("   IsGroup: %v\n", msg.IsGroup)
		fmt.Printf("   IsFromMe: %v\n", msg.IsFromMe)

		// Ignore bot's own messages
		if strings.HasPrefix(msg.Content, botPrefix) {
			fmt.Printf("   🤖 Bot message (%s) - ignoring\n", botPrefix)
			fmt.Println()
			return
		}

		// Access control
		if !access.IsAllowed(msg, cfg) {
			fmt.Println("   ⛔ BLOCKED: Not from authorized group")
			fmt.Println()
			return
		}

		fmt.Println("   ⭐ From authorized group!")

		// Handle voice messages
		if msg.IsVoice {
			if cfg.OpenAI.APIKey == "" || cfg.OpenAI.APIKey == "sk-test-key-for-phase-testing" {
				fmt.Println("   🎤 Voice message ignored (Whisper not configured)")
				fmt.Println()
				return
			}

			fmt.Println("   🎤 Voice message detected")
			fmt.Println()

			// Download and transcribe
			text, err := handleVoiceMessage(client, cfg.OpenAI.APIKey, msg)
			if err != nil {
				// Log technical error
				fmt.Printf("   ❌ Failed to process voice: %v\n", err)
				fmt.Println()

				// Send user-friendly error message
				userMsg := getVoiceErrorMessage(err)
				if sendErr := client.SendMessage(msg.Chat, userMsg); sendErr != nil {
					fmt.Printf("   ❌ Failed to send error message: %v\n", sendErr)
				} else {
					fmt.Println("   📤 Error notification sent to user")
				}
				return
			}

			// Update message content with transcription
			msg.Content = text
			fmt.Printf("   ✅ Transcription: \"%s\"\n", text)
		}

		// Try to parse as command (uses Claude AI if ANTHROPIC_API_KEY is set)
		cmd, _ := commands.ParseIntent(msg.Content)
		if cmd != nil {
			fmt.Println("   🤖 Command detected")

			response := codebutler.HandleCommand(msg.Chat, msg.Content)
			if response != "" {
				fmt.Println("   📤 Sending response...")
				if err := sendWithPrefix(msg.Chat, response); err != nil {
					fmt.Printf("   ❌ Failed to send: %v\n", err)
				} else {
					fmt.Println("   ✅ Response sent")
				}
			}
		} else if msg.Content == "ping" {
			// Legacy ping command
			suffix := ""
			if msg.IsVoice {
				suffix = " (from voice)"
			}

			if err := sendWithPrefix(msg.Chat, "pong! 🏓"+suffix); err != nil {
				fmt.Printf("   ❌ Failed to send pong: %v\n", err)
			} else {
				fmt.Println("   🤖 Sent 'pong' response")
			}
		} else {
			// Not a command, ignore
			fmt.Println("   💬 Not a command - ignoring")
		}

		fmt.Println()
	})

	// Wait for interrupt
	fmt.Println("✅ CodeButler is running!")
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	fmt.Println("👋 Shutting down...")
}

func handleVoiceMessage(client *whatsapp.Client, apiKey string, msg whatsapp.Message) (string, error) {
	fmt.Println("🎤 Processing voice message...")

	// Download audio
	audioPath, err := client.DownloadAudioFromMessage(msg)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(audioPath)

	fmt.Printf("   ✅ Audio downloaded: %s\n", audioPath)

	// Transcribe
	fmt.Println("   🔄 Transcribing with Whisper API...")
	text, err := audio.TranscribeAudio(audioPath, apiKey)
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	return text, nil
}

func getVoiceErrorMessage(err error) string {
	errStr := err.Error()

	// Detect quota exceeded
	if strings.Contains(errStr, "insufficient_quota") || strings.Contains(errStr, "429") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"💳 Tu cuenta de OpenAI se quedó sin créditos.\n" +
			"💡 Agregá saldo en: https://platform.openai.com/account/billing"
	}

	// Detect invalid API key
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "invalid") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"🔑 El API key de OpenAI es inválido.\n" +
			"💡 Verificá tu configuración."
	}

	// Detect rate limit
	if strings.Contains(errStr, "rate_limit") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"⏳ Demasiadas solicitudes a OpenAI.\n" +
			"💡 Intentá de nuevo en unos minutos."
	}

	// Detect network errors
	if strings.Contains(errStr, "download failed") {
		return "❌ No pude transcribir el mensaje de voz.\n\n" +
			"📡 Error al descargar el audio de WhatsApp.\n" +
			"💡 Intentá enviarlo de nuevo."
	}

	// Generic error
	return "❌ No pude transcribir el mensaje de voz.\n\n" +
		"⚠️  Error desconocido.\n" +
		"💡 Intentá de nuevo más tarde."
}
