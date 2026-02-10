package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/leandrotocalini/CodeButler/internal/access"
	"github.com/leandrotocalini/CodeButler/internal/audio"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/protocol"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

//go:embed templates/*
var templates embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	currentConfig   *config.Config
	waClient        *whatsapp.Client
	agentRunning    = false
	setupMode       = true
)

func main() {
	fmt.Println("🤖 CodeButler")
	fmt.Println()

	// Check if config exists
	if _, err := os.Stat("config.json"); err == nil {
		// Load existing config
		cfg, err := config.Load("config.json")
		if err != nil {
			log.Fatal("❌ Failed to load config:", err)
		}
		currentConfig = cfg
		setupMode = false
		fmt.Println("✅ Configuration loaded")
	} else {
		fmt.Println("📋 No configuration found - running setup wizard")
		setupMode = true
	}

	// Initialize protocol
	protocol.Initialize()

	// Setup HTTP routes
	http.HandleFunc("/", serveUI)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/setup", handleSetup)
	http.HandleFunc("/api/config", handleConfig)
	http.HandleFunc("/api/agent/start", handleAgentStart)
	http.HandleFunc("/api/agent/stop", handleAgentStop)
	http.HandleFunc("/ws/qr", handleQRWebSocket)

	// Start web server
	port := "3000"
	url := "http://localhost:" + port

	go func() {
		fmt.Printf("🌐 Web UI: %s\n", url)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatal(err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	// Open browser
	fmt.Println("📱 Opening browser...")
	openBrowser(url)
	fmt.Println()

	// If config exists, auto-start agent
	if !setupMode {
		fmt.Println("🚀 Starting agent...")
		go startAgent()
	}

	// Wait for interrupt
	fmt.Println("✅ CodeButler running")
	fmt.Println("   Web UI: " + url)
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	fmt.Println("👋 Shutting down...")
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	var tmpl *template.Template
	var err error

	if setupMode {
		tmpl, err = template.ParseFS(templates, "templates/setup.html")
	} else {
		tmpl, err = template.ParseFS(templates, "templates/dashboard.html")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SetupMode":    setupMode,
		"AgentRunning": agentRunning,
	}

	if currentConfig != nil {
		data["Config"] = currentConfig
	}

	tmpl.Execute(w, data)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"setup_mode":    setupMode,
		"agent_running": agentRunning,
		"config":        currentConfig,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		OpenAIKey  string `json:"openai_key"`
		GroupName  string `json:"group_name"`
		BotPrefix  string `json:"bot_prefix"`
		SourcesDir string `json:"sources_dir"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if waClient == nil {
		http.Error(w, "WhatsApp not connected", http.StatusBadRequest)
		return
	}

	// Get user info
	info, _ := waClient.GetInfo()

	// Find group
	groups, _ := waClient.GetGroups()
	var groupJID string
	for _, g := range groups {
		if g.Name == data.GroupName {
			groupJID = g.JID
			break
		}
	}

	if groupJID == "" {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	// Create config
	cfg := &config.Config{
		WhatsApp: config.WhatsAppConfig{
			SessionPath: "./whatsapp-session",
			GroupJID:    groupJID,
			GroupName:   data.GroupName,
			BotPrefix:   data.BotPrefix,
		},
		OpenAI: config.OpenAIConfig{
			APIKey: data.OpenAIKey,
		},
		Sources: config.SourcesConfig{
			RootPath: data.SourcesDir,
		},
	}

	// Save config
	if err := config.Save(cfg, "config.json"); err != nil {
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	currentConfig = cfg
	setupMode = false

	// Write setup status
	status := map[string]interface{}{
		"type":      "setup_complete",
		"timestamp": time.Now().Format(time.RFC3339),
		"success":   true,
		"user": map[string]string{
			"jid":  info.JID,
			"name": info.Name,
		},
		"group": map[string]string{
			"jid":  groupJID,
			"name": data.GroupName,
		},
		"voice_enabled": data.OpenAIKey != "",
		"config_path":   "./config.json",
	}

	statusJSON, _ := json.MarshalIndent(status, "", "  ")
	os.WriteFile("/tmp/codebutler/setup-status.json", statusJSON, 0644)

	// Start agent
	go startAgent()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(currentConfig)
		return
	}

	if r.Method == "POST" {
		var cfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := config.Save(&cfg, "config.json"); err != nil {
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}

		currentConfig = &cfg
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleAgentStart(w http.ResponseWriter, r *http.Request) {
	if agentRunning {
		http.Error(w, "Agent already running", http.StatusBadRequest)
		return
	}

	go startAgent()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleAgentStop(w http.ResponseWriter, r *http.Request) {
	if !agentRunning {
		http.Error(w, "Agent not running", http.StatusBadRequest)
		return
	}

	stopAgent()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func handleQRWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	// Connect to WhatsApp
	client, qrChan, err := whatsapp.ConnectWithQR("./whatsapp-session")
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	waClient = client

	// Send QR codes
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			conn.WriteJSON(map[string]interface{}{
				"type": "qr",
				"code": evt.Code,
			})

		case "success":
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
	}
}

func startAgent() {
	if agentRunning || currentConfig == nil {
		return
	}

	agentRunning = true
	fmt.Println("🚀 Agent starting...")

	// Connect to WhatsApp (reuse existing or create new)
	if waClient == nil || !waClient.IsConnected() {
		client, err := whatsapp.Connect(currentConfig.WhatsApp.SessionPath)
		if err != nil {
			fmt.Printf("❌ Failed to connect: %v\n", err)
			agentRunning = false
			return
		}
		waClient = client
	}

	fmt.Println("✅ Agent running")

	botPrefix := currentConfig.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	// Register message handler
	waClient.OnMessage(func(msg whatsapp.Message) {
		if strings.HasPrefix(msg.Content, botPrefix) {
			return
		}

		if !access.IsAllowed(msg, currentConfig) {
			return
		}

		fmt.Printf("📨 Message: %s\n", msg.Content)

		// Handle voice
		content := msg.Content
		var transcript *string
		if msg.IsVoice && currentConfig.OpenAI.APIKey != "" {
			text, err := transcribeVoice(msg)
			if err == nil {
				content = text
				transcript = &text
				fmt.Printf("   🎤 Transcript: %s\n", text)
			}
		}

		// Write incoming
		protocol.WriteIncoming(&protocol.IncomingMessage{
			MessageID:  uuid.New().String(),
			From:       protocol.Contact{JID: msg.From, Name: ""},
			Chat:       protocol.Contact{JID: msg.Chat, Name: ""},
			Content:    content,
			IsVoice:    msg.IsVoice,
			Transcript: transcript,
		})

		fmt.Println("   ✅ Written to /tmp/codebutler/incoming.json")
	})

	// Start monitors
	go monitorOutgoing(waClient, botPrefix)
	go monitorQuestions(waClient, botPrefix)
}

func stopAgent() {
	if !agentRunning {
		return
	}

	agentRunning = false
	if waClient != nil {
		waClient.Disconnect()
		waClient = nil
	}

	fmt.Println("🛑 Agent stopped")
}

func monitorOutgoing(client *whatsapp.Client, botPrefix string) {
	for agentRunning {
		time.Sleep(1 * time.Second)

		if !protocol.FileExists(protocol.OutgoingPath) {
			continue
		}

		resp, err := protocol.ReadOutgoing()
		if err != nil {
			protocol.DeleteFile(protocol.OutgoingPath)
			continue
		}

		message := botPrefix + " " + resp.Content
		client.SendMessage(resp.ChatJID, message)
		fmt.Println("📤 Response sent")

		protocol.DeleteFile(protocol.OutgoingPath)
	}
}

func monitorQuestions(client *whatsapp.Client, botPrefix string) {
	for agentRunning {
		time.Sleep(1 * time.Second)

		if !protocol.FileExists(protocol.QuestionPath) {
			continue
		}

		q, err := protocol.ReadQuestion()
		if err != nil {
			protocol.DeleteFile(protocol.QuestionPath)
			continue
		}

		message := fmt.Sprintf("%s %s\n", botPrefix, q.Text)
		for i, opt := range q.Options {
			message += fmt.Sprintf("%d. %s\n", i+1, opt)
		}

		client.SendMessage(q.ChatJID, message)
		protocol.DeleteFile(protocol.QuestionPath)
	}
}

func transcribeVoice(msg whatsapp.Message) (string, error) {
	audioPath, err := waClient.DownloadAudioFromMessage(msg)
	if err != nil {
		return "", err
	}
	defer os.Remove(audioPath)

	return audio.TranscribeAudio(audioPath, currentConfig.OpenAI.APIKey)
}

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}

	if cmd != nil {
		cmd.Start()
	}
}
