package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leandrotocalini/CodeButler/internal/access"
	"github.com/leandrotocalini/CodeButler/internal/audio"
	"github.com/leandrotocalini/CodeButler/internal/bot"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/repo"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func main() {
	fmt.Println("🤖 CodeButler - Test Integration (Phase 1 + Phase 2)")
	fmt.Println()

	configPath := "config.json"
	if !fileExists(configPath) {
		fmt.Println("❌ config.json not found!")
		fmt.Println()
		fmt.Println("Create config.json based on config.sample.json:")
		fmt.Println("  cp config.sample.json config.json")
		fmt.Println()
		fmt.Println("Then edit config.json with your settings:")
		fmt.Println("  - whatsapp.sessionPath: where to store WhatsApp session")
		fmt.Println("  - openai.apiKey: your OpenAI API key")
		fmt.Println("  - sources.rootPath: path to your repositories")
		fmt.Println()
		fmt.Println("Note: groupJID and groupName will be set after first connection")
		os.Exit(1)
	}

	fmt.Println("📝 Loading configuration...")
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Printf("   ✅ Config loaded\n")
	fmt.Printf("   📁 Session path: %s\n", cfg.WhatsApp.SessionPath)
	fmt.Printf("   📂 Sources path: %s\n", cfg.Sources.RootPath)
	fmt.Println()

	fmt.Println("📱 Connecting to WhatsApp...")
	client, err := whatsapp.Connect(cfg.WhatsApp.SessionPath)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()
	fmt.Println()

	fmt.Println("⏳ Waiting for connection to stabilize...")
	time.Sleep(3 * time.Second)
	fmt.Println()

	info, err := client.GetInfo()
	if err != nil {
		log.Fatalf("Failed to get info: %v", err)
	}
	fmt.Printf("👤 Connected as: %s\n", info.JID)
	if info.Name != "" {
		fmt.Printf("   Name: %s\n", info.Name)
	}
	fmt.Println()

	fmt.Println("📋 Fetching groups...")
	groups, err := client.GetGroups()
	if err != nil {
		log.Fatalf("Failed to get groups: %v", err)
	}

	if len(groups) == 0 {
		fmt.Println("   ⚠️  No groups found")
		fmt.Println()
		fmt.Println("💡 Tip: Create a group called 'CodeButler Developer' in WhatsApp")
		fmt.Println("   Then update config.json with the group JID")
	} else {
		fmt.Printf("   Found %d group(s):\n", len(groups))
		for i, group := range groups {
			fmt.Printf("   %d. %s\n", i+1, group.Name)
			fmt.Printf("      JID: %s\n", group.JID)

			if cfg.WhatsApp.GroupJID == "" && group.Name == "CodeButler Developer" {
				fmt.Printf("      ⭐ This looks like your control group!\n")
				fmt.Printf("      💡 Add this to config.json:\n")
				fmt.Printf("         \"groupJID\": \"%s\",\n", group.JID)
				fmt.Printf("         \"groupName\": \"%s\"\n", group.Name)
			}
		}
	}
	fmt.Println()

	fmt.Println("📂 Scanning repositories...")
	repos, err := repo.ScanRepositories(cfg.Sources.RootPath)
	if err != nil {
		fmt.Printf("   ⚠️  Failed to scan repositories: %v\n", err)
	} else if len(repos) == 0 {
		fmt.Println("   ⚠️  No repositories found")
		fmt.Println()
		fmt.Printf("💡 Tip: Clone repositories to %s\n", cfg.Sources.RootPath)
		fmt.Println("   Example: git clone <url> ./Sources/my-project")
	} else {
		fmt.Printf("   Found %d repositor(y/ies):\n", len(repos))
		for i, r := range repos {
			claudeStatus := "❌"
			if r.HasClaudeMd {
				claudeStatus = "✅"
			}
			fmt.Printf("   %d. %s %s CLAUDE.md\n", i+1, r.Name, claudeStatus)
			fmt.Printf("      Path: %s\n", r.Path)
		}
	}
	fmt.Println()

	fmt.Println("🤖 Initializing CodeButler bot...")
	codebutler := bot.NewBot(cfg, func(chatID, text string) error {
		return client.SendMessage(chatID, text)
	})
	if err := codebutler.LoadRepositories(); err != nil {
		fmt.Printf("   ⚠️  Failed to load repositories: %v\n", err)
	} else {
		fmt.Println("   ✅ Bot initialized")
	}
	fmt.Println()

	fmt.Println("👂 Listening for messages... (Press Ctrl+C to stop)")
	fmt.Println()

	client.OnMessage(func(msg whatsapp.Message) {
		fmt.Printf("📨 Message received:\n")
		fmt.Printf("   From: %s\n", msg.From)
		fmt.Printf("   Chat: %s\n", msg.Chat)
		fmt.Printf("   Content: %s\n", msg.Content)
		fmt.Printf("   IsGroup: %v\n", msg.IsGroup)
		fmt.Printf("   IsFromMe: %v\n", msg.IsFromMe)

		// Check access control
		if !access.IsAllowed(msg, cfg) {
			fmt.Printf("   ⛔ BLOCKED: Not from authorized group\n")
			fmt.Println()
			return
		}

		fmt.Printf("   ⭐ From CodeButler Developer group!\n")

		if msg.IsVoice {
			fmt.Printf("   🎤 Voice message detected\n")
		}

		fmt.Println()

		if msg.IsVoice {
			fmt.Println("🎤 Processing voice message...")

			audioPath, err := client.DownloadAudioFromMessage(msg)
			if err != nil {
				fmt.Printf("   ❌ Failed to download audio: %v\n", err)
				fmt.Println()
				return
			}
			defer os.Remove(audioPath)
			fmt.Printf("   ✅ Audio downloaded: %s\n", audioPath)

			if cfg.OpenAI.APIKey == "" || cfg.OpenAI.APIKey == "sk-test-key-for-phase-testing" {
				fmt.Println("   ⚠️  OpenAI API key not configured - skipping transcription")
				fmt.Println("   💡 Add your real OpenAI API key to config.json to enable voice transcription")
				fmt.Println()
				return
			}

			fmt.Println("   🔄 Transcribing with Whisper API...")
			text, err := audio.TranscribeAudio(audioPath, cfg.OpenAI.APIKey)
			if err != nil {
				fmt.Printf("   ❌ Failed to transcribe: %v\n", err)
				fmt.Println()
				return
			}
			fmt.Printf("   ✅ Transcription: \"%s\"\n", text)

			if text == "ping" || text == "Ping" || text == "ping." {
				fmt.Println("   🤖 Sending 'pong' response...")
				if err := client.SendMessage(msg.Chat, "pong! 🏓 (from voice)"); err != nil {
					fmt.Printf("   ❌ Failed to send: %v\n", err)
				} else {
					fmt.Printf("   ✅ Response sent\n")
				}
			}
			fmt.Println()
			return
		}

		// Handle @codebutler commands
		response := codebutler.HandleCommand(msg.Chat, msg.Content)
		if response != "" {
			fmt.Println("   🤖 CodeButler command detected")
			fmt.Println("   📤 Sending response...")
			if err := client.SendMessage(msg.Chat, response); err != nil {
				fmt.Printf("   ❌ Failed to send: %v\n", err)
			} else {
				fmt.Printf("   ✅ Response sent\n")
			}
			fmt.Println()
			return
		}

		// Legacy ping/pong for testing
		if msg.Content == "ping" {
			fmt.Println("🤖 Sending 'pong' response...")
			if err := client.SendMessage(msg.Chat, "pong! 🏓"); err != nil {
				fmt.Printf("   ❌ Failed to send: %v\n", err)
			} else {
				fmt.Printf("   ✅ Response sent\n")
			}
			fmt.Println()
		}
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println()
	fmt.Println("👋 Disconnecting...")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
