package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/leandrotocalini/CodeButler/internal/config"
)

type WizardConfig struct {
	UseWhisper    bool
	WhisperAPIKey string
	GroupName     string
	BotPrefix     string
	SourcesPath   string
}

func RunWizard() (*WizardConfig, error) {
	reader := bufio.NewReader(os.Stdin)
	cfg := &WizardConfig{}

	fmt.Println("🤖 CodeButler - First-time Setup")
	fmt.Println("================================")
	fmt.Println()

	// Ask about Whisper
	fmt.Println("📢 Voice Message Transcription")
	fmt.Println("   Do you want to enable voice message transcription with OpenAI Whisper?")
	fmt.Print("   Enable Whisper? (yes/no) [no]: ")

	whisperChoice, _ := reader.ReadString('\n')
	whisperChoice = strings.TrimSpace(strings.ToLower(whisperChoice))

	if whisperChoice == "yes" || whisperChoice == "y" {
		cfg.UseWhisper = true
		fmt.Println()
		fmt.Println("   ✅ Whisper enabled")
		fmt.Println("   📝 Enter your OpenAI API key:")
		fmt.Print("   API Key: ")
		apiKey, _ := reader.ReadString('\n')
		cfg.WhisperAPIKey = strings.TrimSpace(apiKey)
	} else {
		cfg.UseWhisper = false
		fmt.Println("   ⏭️  Skipping Whisper (voice messages will be ignored)")
	}

	fmt.Println()

	// Ask about group name
	fmt.Println("📱 WhatsApp Group Configuration")
	fmt.Println("   CodeButler listens to commands from a single WhatsApp group.")
	fmt.Print("   Group name [CodeButler Developer]: ")

	groupName, _ := reader.ReadString('\n')
	groupName = strings.TrimSpace(groupName)

	if groupName == "" {
		cfg.GroupName = "CodeButler Developer"
		fmt.Println("   ✅ Using default: CodeButler Developer")
	} else {
		cfg.GroupName = groupName
		fmt.Printf("   ✅ Using custom group: %s\n", groupName)
	}

	fmt.Println()

	// Ask about bot prefix
	fmt.Println("🤖 Bot Message Prefix")
	fmt.Println("   Bot messages will start with this prefix to avoid processing its own messages.")
	fmt.Print("   Bot prefix [[BOT]]: ")

	botPrefix, _ := reader.ReadString('\n')
	botPrefix = strings.TrimSpace(botPrefix)

	if botPrefix == "" {
		cfg.BotPrefix = "[BOT]"
		fmt.Println("   ✅ Using default: [BOT]")
	} else {
		cfg.BotPrefix = botPrefix
		fmt.Printf("   ✅ Using: %s\n", botPrefix)
	}

	fmt.Println()

	// Ask about Sources path
	fmt.Println("📂 Repositories Configuration")
	fmt.Println("   Where are your code repositories located?")
	fmt.Print("   Sources path [./Sources]: ")

	sourcesPath, _ := reader.ReadString('\n')
	sourcesPath = strings.TrimSpace(sourcesPath)

	if sourcesPath == "" {
		cfg.SourcesPath = "./Sources"
		fmt.Println("   ✅ Using default: ./Sources")
	} else {
		cfg.SourcesPath = sourcesPath
		fmt.Printf("   ✅ Using: %s\n", sourcesPath)
	}

	fmt.Println()

	fmt.Println("📋 Setup Summary")
	fmt.Println("================")
	fmt.Printf("   Voice transcription: %v\n", cfg.UseWhisper)
	fmt.Printf("   WhatsApp group: %s\n", cfg.GroupName)
	fmt.Printf("   Bot prefix: %s\n", cfg.BotPrefix)
	fmt.Printf("   Sources path: %s\n", cfg.SourcesPath)
	fmt.Println()

	return cfg, nil
}

func SaveConfig(wizardCfg *WizardConfig, groupJID, personalNumber string) error {
	cfg := &config.Config{
		WhatsApp: config.WhatsAppConfig{
			SessionPath:    "./whatsapp-session",
			PersonalNumber: personalNumber,
			GroupJID:       groupJID,
			GroupName:      wizardCfg.GroupName,
			BotPrefix:      wizardCfg.BotPrefix,
		},
		OpenAI: config.OpenAIConfig{
			APIKey: wizardCfg.WhisperAPIKey,
		},
		Sources: config.SourcesConfig{
			RootPath: wizardCfg.SourcesPath,
		},
	}

	return config.Save(cfg, "config.json")
}

func CreateGroupIfNeeded(groupName string) error {
	fmt.Println()
	fmt.Println("📱 WhatsApp Group Setup")
	fmt.Println("======================")
	fmt.Printf("   Please create a WhatsApp group named: \"%s\"\n", groupName)
	fmt.Println()
	fmt.Println("   Steps:")
	fmt.Println("   1. Open WhatsApp on your phone")
	fmt.Println("   2. Create a new group")
	fmt.Printf("   3. Name it: %s\n", groupName)
	fmt.Println("   4. Add yourself only (or trusted team members)")
	fmt.Println()
	fmt.Print("   Press Enter when you've created the group...")

	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	return nil
}
