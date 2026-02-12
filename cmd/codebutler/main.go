package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/daemon"
	"github.com/leandrotocalini/CodeButler/internal/messenger"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

//go:embed templates/setup.html
var templates embed.FS

//go:embed VERSION
var Version string

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var waClient *whatsapp.Client

func main() {
	forceSetup := false
	for _, arg := range os.Args[1:] {
		if arg == "--setup" {
			forceSetup = true
		}
	}

	repoDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	// First time or forced: run setup, then start daemon
	if forceSetup || !config.Exists(repoDir) {
		if err := runSetup(repoDir); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Setup failed: %v\n", err)
			os.Exit(1)
		}
		// Disconnect setup client — daemon creates its own
		if waClient != nil {
			waClient.Disconnect()
			waClient = nil
		}
	}

	// Load repo config and run daemon
	repoCfg, err := config.LoadRepo(repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load repo config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run codebutler --setup to reconfigure\n")
		os.Exit(1)
	}

	// Check for missing optional config
	needSave := false
	if repoCfg.OpenAI.APIKey == "" {
		fmt.Println("⚙️  OpenAI API key not configured (needed for voice messages)")
		fmt.Print("   Enter key (or press Enter to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		key := strings.TrimSpace(scanner.Text())
		if key != "" {
			repoCfg.OpenAI.APIKey = key
			needSave = true
			fmt.Println("   ✅ OpenAI key set")
		} else {
			fmt.Println("   Skipped — voice messages won't be transcribed")
		}
		fmt.Println()
	}

	if repoCfg.Moonshot.APIKey == "" {
		fmt.Println("⚙️  Moonshot API key not configured (needed for /draft-mode with Kimi)")
		fmt.Print("   Enter key (or press Enter to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		key := strings.TrimSpace(scanner.Text())
		if key != "" {
			repoCfg.Moonshot.APIKey = key
			needSave = true
			fmt.Println("   ✅ Moonshot key set")
		} else {
			fmt.Println("   Skipped — /draft-mode won't be available")
		}
		fmt.Println()
	}

	if needSave {
		if err := config.SaveRepo(repoDir, repoCfg); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Failed to save config: %v\n", err)
		}
	}

	// Build messenger(s)
	var backends []messenger.BackendChat

	// WhatsApp — always try if config exists
	if repoCfg.WhatsApp.GroupJID != "" {
		sessionPath := config.SessionPath(repoDir)
		deviceName := "CodeButler:" + filepath.Base(repoDir)
		wa := messenger.NewWhatsApp(sessionPath, deviceName)
		backends = append(backends, messenger.BackendChat{Backend: wa, ChatID: repoCfg.WhatsApp.GroupJID})
	}

	// Slack — try if global tokens exist
	slackCfg, slackErr := config.LoadSlack()
	if slackErr == nil && slackCfg.BotToken != "" && slackCfg.AppToken != "" {
		botPrefix := repoCfg.WhatsApp.BotPrefix
		if botPrefix == "" {
			botPrefix = "[BOT]"
		}
		channelID := repoCfg.Slack.ChannelID
		sl := messenger.NewSlack(slackCfg.BotToken, slackCfg.AppToken, channelID, botPrefix)

		if channelID == "" {
			// Auto-find or create channel using "repoName hostname" pattern
			repoName := filepath.Base(repoDir)
			hostname, _ := os.Hostname()
			suggestedName := repoName + " " + hostname

			fmt.Printf("🔍 Slack: looking for channel %q...\n", suggestedName)
			chID, err := sl.FindOrCreateChannel(suggestedName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Slack channel setup failed: %v\n", err)
				fmt.Println("   Slack will be skipped — WhatsApp only")
			} else {
				fmt.Printf("✅ Slack channel: %s\n", chID)
				// Save channel ID so we don't search next time
				repoCfg.Slack.ChannelID = chID
				config.SaveRepo(repoDir, repoCfg)
				backends = append(backends, messenger.BackendChat{Backend: sl, ChatID: chID})
			}
		} else {
			backends = append(backends, messenger.BackendChat{Backend: sl, ChatID: channelID})
		}
	}

	if len(backends) == 0 {
		fmt.Fprintf(os.Stderr, "❌ No messenger configured\n")
		fmt.Fprintf(os.Stderr, "Run codebutler --setup for WhatsApp, or create ~/.codebutler/slack.json for Slack\n")
		os.Exit(1)
	}

	// Build the messenger: single or multi
	var msger messenger.Messenger
	chatID := backends[0].ChatID
	if len(backends) == 1 {
		msger = backends[0].Backend
	} else {
		msger = messenger.NewMulti(backends...)
	}

	botPrefix := repoCfg.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}
	groupName := repoCfg.WhatsApp.GroupName
	if groupName == "" {
		groupName = filepath.Base(repoDir)
	}

	d := daemon.New(repoCfg, repoDir, strings.TrimSpace(Version), msger, chatID, botPrefix, groupName)
	if err := d.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Daemon error: %v\n", err)
		os.Exit(1)
	}
}

func runSetup(repoDir string) error {
	repoName := filepath.Base(repoDir)
	fmt.Println("🤖 CodeButler Setup")
	fmt.Printf("   Repo: %s\n\n", repoDir)

	sessionPath := config.SessionPath(repoDir)
	whatsapp.SetDeviceName("CodeButler:" + repoName)

	// Try to connect with existing session
	client, err := whatsapp.Connect(sessionPath)
	if err == nil && client.IsConnected() {
		waClient = client
		info, _ := client.GetInfo()
		fmt.Printf("✅ WhatsApp connected as: %s\n\n", info.Name)
		return interactiveSetup(repoDir)
	}

	// No session — Connect will show QR in terminal
	fmt.Println("📱 Connecting to WhatsApp...")
	fmt.Println()

	client, err = whatsapp.Connect(sessionPath)
	if err != nil {
		return fmt.Errorf("WhatsApp connection failed: %w", err)
	}

	if !client.IsConnected() {
		return fmt.Errorf("WhatsApp failed to connect")
	}

	waClient = client
	info, _ := client.GetInfo()
	fmt.Printf("✅ WhatsApp connected as: %s\n\n", info.Name)
	return interactiveSetup(repoDir)
}

// interactiveSetup runs when WhatsApp is already connected.
// Lists groups and lets user pick one from the terminal.
func interactiveSetup(repoDir string) error {
	if waClient == nil {
		return fmt.Errorf("WhatsApp not connected")
	}

	var groups []whatsapp.Group
	var groupErr error
	for attempt := 1; attempt <= 3; attempt++ {
		groups, groupErr = waClient.GetGroups()
		if groupErr == nil {
			break
		}
		if attempt < 3 {
			fmt.Printf("⏳ Waiting for connection to stabilize (attempt %d/3)...\n", attempt)
			time.Sleep(3 * time.Second)
		}
	}
	if groupErr != nil {
		return fmt.Errorf("failed to list groups: %w", groupErr)
	}

	// Suggest a group name: "repo hostname"
	repoName := filepath.Base(repoDir)
	hostname, _ := os.Hostname()
	suggestedName := repoName + " " + hostname

	scanner := bufio.NewScanner(os.Stdin)
	var selectedGroup whatsapp.Group

	if len(groups) > 0 {
		fmt.Println("Available WhatsApp groups:")
		fmt.Println()
		for i, g := range groups {
			fmt.Printf("  %d. %s\n", i+1, g.Name)
		}
		fmt.Println()
		fmt.Printf("Select group number, or Enter to create \"%s\": ", suggestedName)
	} else {
		fmt.Printf("No existing groups. Press Enter to create \"%s\" (or type a name): ", suggestedName)
	}

	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		// Create with suggested name
		jid, err := waClient.CreateGroup(suggestedName, []string{})
		if err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}
		selectedGroup = whatsapp.Group{JID: jid, Name: suggestedName}
		fmt.Printf("✅ Group created: %s\n", suggestedName)
	} else if idx, err := fmt.Sscanf(input, "%d", new(int)); err == nil && idx == 1 {
		var n int
		fmt.Sscanf(input, "%d", &n)
		if n < 1 || n > len(groups) {
			return fmt.Errorf("invalid selection")
		}
		selectedGroup = groups[n-1]
	} else {
		// Custom name typed
		jid, err := waClient.CreateGroup(input, []string{})
		if err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}
		selectedGroup = whatsapp.Group{JID: jid, Name: input}
		fmt.Printf("✅ Group created: %s\n", input)
	}

	// Ask for bot prefix
	fmt.Print("Bot prefix [BOT]: ")
	scanner.Scan()
	botPrefix := strings.TrimSpace(scanner.Text())
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	// Ask for OpenAI key (optional, for voice transcription)
	fmt.Print("OpenAI API key (for voice messages, or skip): ")
	scanner.Scan()
	openaiKey := strings.TrimSpace(scanner.Text())

	// Ask for Moonshot key (optional, for draft mode with Kimi)
	fmt.Print("Moonshot API key (for /draft-mode with Kimi, or skip): ")
	scanner.Scan()
	moonshotKey := strings.TrimSpace(scanner.Text())

	// Save repo config
	repoCfg := &config.RepoConfig{
		WhatsApp: config.WhatsAppConfig{
			GroupJID:  selectedGroup.JID,
			GroupName: selectedGroup.Name,
			BotPrefix: botPrefix,
		},
		Claude: config.ClaudeConfig{
			MaxTurns: 10,
			Timeout:  30,
		},
		OpenAI: config.OpenAIConfig{
			APIKey: openaiKey,
		},
		Moonshot: config.MoonshotConfig{
			APIKey: moonshotKey,
		},
	}

	if err := config.SaveRepo(repoDir, repoCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	addToGitignore(repoDir)

	fmt.Println()
	fmt.Printf("✅ Configured! Group: %s\n", selectedGroup.Name)
	fmt.Println("   Starting daemon...")
	return nil
}

func addToGitignore(repoDir string) {
	gitignorePath := repoDir + "/.gitignore"

	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return
	}

	if strings.Contains(string(content), ".codebutler/") {
		return
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	entry := ".codebutler/\n"
	if len(content) > 0 && content[len(content)-1] != '\n' {
		entry = "\n" + entry
	}
	f.WriteString(entry)
}

func handleWebSetup(w http.ResponseWriter, r *http.Request, repoDir string) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		GroupName string `json:"group_name"`
		BotPrefix string `json:"bot_prefix"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if waClient == nil {
		http.Error(w, "WhatsApp not connected", http.StatusBadRequest)
		return
	}

	groups, _ := waClient.GetGroups()
	var groupJID string
	for _, g := range groups {
		if g.Name == data.GroupName {
			groupJID = g.JID
			break
		}
	}

	if groupJID == "" {
		jid, err := waClient.CreateGroup(data.GroupName, []string{})
		if err != nil {
			http.Error(w, "Failed to create group: "+err.Error(), http.StatusInternalServerError)
			return
		}
		groupJID = jid
	}

	botPrefix := data.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	repoCfg := &config.RepoConfig{
		WhatsApp: config.WhatsAppConfig{
			GroupJID:  groupJID,
			GroupName: data.GroupName,
			BotPrefix: botPrefix,
		},
		Claude: config.ClaudeConfig{
			MaxTurns: 10,
			Timeout:  30,
		},
	}

	if err := config.SaveRepo(repoDir, repoCfg); err != nil {
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	addToGitignore(repoDir)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleQRWebSocket(w http.ResponseWriter, r *http.Request, sessionPath string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	client, err := whatsapp.Connect(sessionPath)
	if err == nil && client.IsConnected() {
		waClient = client
		info, _ := client.GetInfo()
		conn.WriteJSON(map[string]interface{}{
			"type": "connected",
			"user": map[string]string{
				"jid":  info.JID,
				"name": info.Name,
			},
		})
		return
	}

	client2, qrChan, err := whatsapp.ConnectWithQR(sessionPath)
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	waClient = client2

	for evt := range qrChan {
		switch evt.Event {
		case "code":
			conn.WriteJSON(map[string]interface{}{
				"type": "qr",
				"code": evt.Code,
			})
		case "success":
			info, _ := client2.GetInfo()
			conn.WriteJSON(map[string]interface{}{
				"type": "connected",
				"user": map[string]string{
					"jid":  info.JID,
					"name": info.Name,
				},
			})
			return
		}
	}
}
