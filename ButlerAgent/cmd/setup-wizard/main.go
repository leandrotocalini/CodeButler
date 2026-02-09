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
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

//go:embed templates/*
var templates embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type SetupData struct {
	OpenAIKey  string `json:"openai_key"`
	GroupName  string `json:"group_name"`
	BotPrefix  string `json:"bot_prefix"`
	SourcesDir string `json:"sources_dir"`
}

type SetupStatus struct {
	Type         string    `json:"type"`
	Timestamp    string    `json:"timestamp"`
	Success      bool      `json:"success"`
	User         UserInfo  `json:"user"`
	Group        GroupInfo `json:"group"`
	VoiceEnabled bool      `json:"voice_enabled"`
	ConfigPath   string    `json:"config_path"`
}

type UserInfo struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

type GroupInfo struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

var (
	waClient    *whatsapp.Client
	currentConn *websocket.Conn
	setupDone   = make(chan bool)
)

func main() {
	fmt.Println("🌐 CodeButler Setup Wizard")
	fmt.Println()

	// Setup HTTP handlers
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws/qr", handleQRWebSocket)
	http.HandleFunc("/api/setup", handleSetup)

	// Start server
	port := "3000"
	url := "http://localhost:" + port

	go func() {
		fmt.Printf("🚀 Server starting at %s\n", url)
		fmt.Println()

		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait a bit for server to start
	time.Sleep(500 * time.Millisecond)

	// Open browser
	fmt.Println("📱 Opening browser...")
	openBrowser(url)

	// Wait for setup to complete
	<-setupDone

	fmt.Println()
	fmt.Println("✅ Setup wizard finished!")
	fmt.Println()
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(templates, "templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, nil)
}

func handleQRWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	currentConn = conn

	// Connect to WhatsApp
	client, qrChan, err := whatsapp.ConnectWithQR("./whatsapp-session")
	if err != nil {
		sendWSMessage(conn, map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	waClient = client

	// Send QR codes as they arrive
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			sendWSMessage(conn, map[string]interface{}{
				"type": "qr",
				"code": evt.Code,
			})

		case "success":
			// Get user info
			info, _ := client.GetInfo()

			sendWSMessage(conn, map[string]interface{}{
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

func handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data SetupData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get user info
	info, err := waClient.GetInfo()
	if err != nil {
		http.Error(w, "WhatsApp not connected", http.StatusBadRequest)
		return
	}

	// Find group
	groups, err := waClient.GetGroups()
	if err != nil {
		http.Error(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}

	var groupJID string
	var groupName string
	for _, g := range groups {
		if g.Name == data.GroupName {
			groupJID = g.JID
			groupName = g.Name
			break
		}
	}

	if groupJID == "" {
		http.Error(w, "Group '"+data.GroupName+"' not found", http.StatusNotFound)
		return
	}

	// Create config
	cfg := &config.Config{
		WhatsApp: config.WhatsAppConfig{
			SessionPath: "./whatsapp-session",
			GroupJID:    groupJID,
			GroupName:   groupName,
			BotPrefix:   data.BotPrefix,
		},
		OpenAI: config.OpenAIConfig{
			APIKey: data.OpenAIKey,
		},
		Sources: config.SourcesConfig{
			RootPath: data.SourcesDir,
		},
	}

	// Save config.json
	if err := config.Save(cfg, "config.json"); err != nil {
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	// Create setup status JSON
	status := SetupStatus{
		Type:      "setup_complete",
		Timestamp: time.Now().Format(time.RFC3339),
		Success:   true,
		User: UserInfo{
			JID:  info.JID,
			Name: info.Name,
		},
		Group: GroupInfo{
			JID:  groupJID,
			Name: groupName,
		},
		VoiceEnabled: data.OpenAIKey != "",
		ConfigPath:   "./config.json",
	}

	// Ensure /tmp/codebutler exists
	os.MkdirAll("/tmp/codebutler", 0755)

	// Write setup-status.json
	statusJSON, _ := json.MarshalIndent(status, "", "  ")
	os.WriteFile("/tmp/codebutler/setup-status.json", statusJSON, 0644)

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})

	// Signal setup done
	go func() {
		time.Sleep(1 * time.Second)
		setupDone <- true
	}()
}

func sendWSMessage(conn *websocket.Conn, msg interface{}) {
	if err := conn.WriteJSON(msg); err != nil {
		log.Println("WebSocket write error:", err)
	}
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
	default:
		fmt.Printf("⚠️  Please open manually: %s\n", url)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("⚠️  Couldn't open browser. Open manually: %s\n", url)
	}
}
