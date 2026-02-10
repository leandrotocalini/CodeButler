package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leandrotocalini/CodeButler/internal/access"
	"github.com/leandrotocalini/CodeButler/internal/audio"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/mcp"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

//go:embed templates/*
var templates embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	currentConfig *config.Config
	waClient      *whatsapp.Client
	agentRunning  = false
	setupMode     = true
	taskMu        sync.Mutex   // Serialize Claude tasks (one at a time)
	chatSessions  sync.Map     // map[chatJID]string — last session_id per chat
)

func main() {
	// Check for MCP mode
	for _, arg := range os.Args[1:] {
		if arg == "--mcp" {
			runMCPServer()
			return
		}
	}

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

	// Find group or create it
	groups, _ := waClient.GetGroups()
	var groupJID string
	for _, g := range groups {
		if g.Name == data.GroupName {
			groupJID = g.JID
			break
		}
	}

	if groupJID == "" {
		// Group not found, create it
		fmt.Printf("📱 Creating WhatsApp group: %s\n", data.GroupName)
		jid, err := waClient.CreateGroup(data.GroupName, []string{})
		if err != nil {
			http.Error(w, "Failed to create group: "+err.Error(), http.StatusInternalServerError)
			return
		}
		groupJID = jid
		fmt.Printf("✅ Group created: %s\n", groupJID)
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

	// Create MCP config for Claude
	if err := createMCPConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ Failed to create MCP config: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "✅ Created .mcp.json\n")
	}

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

	// Try to reconnect with existing session first
	client, err := whatsapp.Connect("./whatsapp-session")
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

	// No existing session, need QR scan
	client2, qrChan, err := whatsapp.ConnectWithQR("./whatsapp-session")
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	waClient = client2

	// Send QR codes
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

	fmt.Println("✅ Agent running - messages will be sent to Claude")

	botPrefix := currentConfig.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	// Register message handler - invokes Claude on each message
	waClient.OnMessage(func(msg whatsapp.Message) {
		if msg.IsFromMe {
			return
		}

		if strings.HasPrefix(msg.Content, botPrefix) {
			return
		}

		if !access.IsAllowed(msg, currentConfig) {
			return
		}

		fmt.Printf("📨 Message from %s: %s\n", msg.From, msg.Content)

		// Handle voice transcription
		content := msg.Content
		if msg.IsVoice && currentConfig.OpenAI.APIKey != "" {
			text, err := transcribeVoice(msg)
			if err == nil {
				content = text
				fmt.Printf("   🎤 Transcript: %s\n", text)
			}
		}

		// Run Claude task in background (serialized)
		go runClaudeTask(content, msg.Chat, waClient, botPrefix)
	})
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

// streamEvent represents a line from claude --output-format stream-json
type streamEvent struct {
	Type         string          `json:"type"`
	SessionID    string          `json:"session_id,omitempty"`
	Result       string          `json:"result,omitempty"`
	Subtype      string          `json:"subtype,omitempty"`
	ContentBlock json.RawMessage `json:"content_block,omitempty"`
	Message      json.RawMessage `json:"message,omitempty"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type assistantMessage struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content"`
}

// runClaudeTask spawns the claude CLI with streaming output, sends progress
// updates to WhatsApp, and tracks session_id for conversation continuity.
// Tasks are serialized — only one Claude instance runs at a time.
func runClaudeTask(content string, chatJID string, client *whatsapp.Client, botPrefix string) {
	taskMu.Lock()
	defer taskMu.Unlock()

	if !agentRunning {
		return
	}

	// Notify user we're working on it
	preview := content
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	client.SendMessage(chatJID, botPrefix+" 🔄 Processing: "+preview)

	fmt.Printf("🤖 Running Claude: %s\n", preview)

	// Determine working directory
	workDir := currentConfig.Claude.WorkDir
	if workDir == "" {
		workDir = currentConfig.Sources.RootPath
	}
	if workDir == "" {
		workDir = "."
	}
	if !filepath.IsAbs(workDir) {
		if abs, err := filepath.Abs(workDir); err == nil {
			workDir = abs
		}
	}

	// Determine claude command
	claudeCmd := currentConfig.Claude.Command
	if claudeCmd == "" {
		claudeCmd = "claude"
	}

	maxTurns := currentConfig.Claude.MaxTurns
	if maxTurns == 0 {
		maxTurns = 10
	}

	timeoutMin := currentConfig.Claude.Timeout
	if timeoutMin == 0 {
		timeoutMin = 30
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if timeoutMin > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutMin)*time.Minute)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	// Build args with streaming output
	args := []string{
		"-p", content,
		"--output-format", "stream-json",
		"--max-turns", fmt.Sprintf("%d", maxTurns),
	}

	// Resume previous conversation if one exists for this chat
	if sessionID, ok := chatSessions.Load(chatJID); ok {
		args = append(args, "--resume", sessionID.(string))
		fmt.Printf("   🔄 Resuming session: %s\n", sessionID)
	}

	cmd := exec.CommandContext(ctx, claudeCmd, args...)
	cmd.Dir = workDir

	// Set up pipes for streaming
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		client.SendMessage(chatJID, botPrefix+" ❌ Failed to start Claude: "+err.Error())
		return
	}

	cmd.Stderr = os.Stderr // Let Claude's stderr pass through to agent logs

	fmt.Printf("   📂 Working dir: %s\n", workDir)
	start := time.Now()

	if err := cmd.Start(); err != nil {
		client.SendMessage(chatJID, botPrefix+" ❌ Failed to start Claude: "+err.Error())
		return
	}

	// Read streaming events
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large outputs

	var lastUpdate time.Time
	var toolUses []string
	var finalResult string
	var sessionID string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var evt streamEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "system":
			if evt.SessionID != "" {
				sessionID = evt.SessionID
			}

		case "content_block_start":
			// Detect tool use starts
			var cb contentBlock
			if err := json.Unmarshal(evt.ContentBlock, &cb); err == nil {
				if cb.Type == "tool_use" && cb.Name != "" {
					toolName := formatToolName(cb.Name)
					toolUses = append(toolUses, toolName)
					fmt.Printf("   🔧 %s\n", toolName)

					// Send update every 30 seconds or on first tool use
					if time.Since(lastUpdate) > 30*time.Second || len(toolUses) == 1 {
						client.SendMessage(chatJID, botPrefix+" 🔧 "+toolName)
						lastUpdate = time.Now()
					}
				}
			}

		case "result":
			finalResult = evt.Result
			if evt.SessionID != "" {
				sessionID = evt.SessionID
			}
		}
	}

	cmd.Wait()
	elapsed := time.Since(start)
	fmt.Printf("   ⏱️  Completed in %s (%d tool uses)\n", elapsed.Round(time.Second), len(toolUses))

	// Save session ID for conversation continuity
	if sessionID != "" {
		chatSessions.Store(chatJID, sessionID)
		fmt.Printf("   💾 Session saved: %s\n", sessionID)
	}

	// Send final result
	result := strings.TrimSpace(finalResult)
	if result == "" {
		result = "✅ Task completed (no output)"
	}

	if len(result) > 4000 {
		result = result[:4000] + "\n\n...(truncated)"
	}

	client.SendMessage(chatJID, botPrefix+" "+result)
	fmt.Printf("   📤 Response sent (%d chars)\n", len(result))
}

// formatToolName makes tool use events human-readable for WhatsApp updates
func formatToolName(name string) string {
	switch name {
	case "Read":
		return "Reading file..."
	case "Edit":
		return "Editing file..."
	case "Write":
		return "Writing file..."
	case "Bash":
		return "Running command..."
	case "Glob":
		return "Searching files..."
	case "Grep":
		return "Searching code..."
	case "WebFetch":
		return "Fetching web content..."
	case "WebSearch":
		return "Searching the web..."
	case "TodoWrite":
		return "Updating task list..."
	case "Task":
		return "Running subtask..."
	default:
		return "Using " + name + "..."
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

func runMCPServer() {
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run ./butler.sh first to configure CodeButler\n")
		os.Exit(1)
	}

	server := mcp.NewServer(cfg)
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func createMCPConfig() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	mcpConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"codebutler": map[string]interface{}{
				"type":    "stdio",
				"command": filepath.Join(cwd, "codebutler"),
				"args":    []string{"--mcp"},
			},
		},
	}

	mcpPath := ".mcp.json"
	data, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(mcpPath, data, 0644)
}
