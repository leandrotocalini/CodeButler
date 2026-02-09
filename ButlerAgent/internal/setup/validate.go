package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/leandrotocalini/CodeButler/internal/config"
)

func ValidateAndFixConfig(cfg *config.Config) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	updated := false

	// Check bot prefix
	if cfg.WhatsApp.BotPrefix == "" {
		fmt.Println()
		fmt.Println("⚠️  Bot Prefix Not Configured")
		fmt.Println("   Bot messages need a prefix to avoid processing its own messages.")
		fmt.Println()
		fmt.Print("   Bot prefix [[BOT]]: ")

		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)

		if response == "" {
			cfg.WhatsApp.BotPrefix = "[BOT]"
			fmt.Println("   ✅ Using default: [BOT]")
		} else {
			cfg.WhatsApp.BotPrefix = response
			fmt.Printf("   ✅ Using: %s\n", response)
		}
		updated = true
		fmt.Println()
	}

	// Check OpenAI API key
	if cfg.OpenAI.APIKey == "" || cfg.OpenAI.APIKey == "sk-test-key-for-phase-testing" {
		fmt.Println()
		fmt.Println("⚠️  Voice Transcription Not Configured")
		fmt.Println("   You don't have a valid OpenAI API key for Whisper transcription.")
		fmt.Println("   Voice messages will be ignored.")
		fmt.Println()
		fmt.Print("   Do you want to enable voice transcription now? (yes/no) [no]: ")

		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "yes" || response == "y" {
			fmt.Println()
			fmt.Println("   📝 Enter your OpenAI API key:")
			fmt.Print("   API Key: ")

			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			if apiKey != "" {
				cfg.OpenAI.APIKey = apiKey
				updated = true
				fmt.Println("   ✅ API key saved")
			} else {
				fmt.Println("   ⏭️  Skipping - no key entered")
			}
		} else {
			fmt.Println("   ⏭️  Skipping voice transcription")
		}
		fmt.Println()
	}

	return updated, nil
}
