package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

type Server struct {
	config        *config.Config
	client        *whatsapp.Client
	pendingMsgs   []PendingMessage
	pendingMu     sync.Mutex
	answerChan    chan Answer
	currentChatID string
}

type PendingMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	Chat      string `json:"chat"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	IsVoice   bool   `json:"is_voice"`
}

type Answer struct {
	QuestionID string
	Selected   int
	Text       string
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		config:      cfg,
		pendingMsgs: []PendingMessage{},
		answerChan:  make(chan Answer, 1),
	}
}

func (s *Server) Run() error {
	// Connect to WhatsApp
	fmt.Fprintf(os.Stderr, "🤖 CodeButler MCP Server starting...\n")

	client, err := whatsapp.Connect(s.config.WhatsApp.SessionPath)
	if err != nil {
		return fmt.Errorf("failed to connect to WhatsApp: %w", err)
	}
	s.client = client

	// Register message handler
	botPrefix := s.config.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	client.OnMessage(func(msg whatsapp.Message) {
		// Ignore bot messages
		if len(msg.Content) > 0 && msg.Content[0:len(botPrefix)] == botPrefix {
			return
		}

		s.currentChatID = msg.Chat

		// Check if this is an answer to a question
		if len(msg.Content) == 1 && msg.Content[0] >= '1' && msg.Content[0] <= '9' {
			selected := int(msg.Content[0] - '0')
			s.answerChan <- Answer{Selected: selected}
			return
		}

		// Add to pending messages
		s.pendingMu.Lock()
		s.pendingMsgs = append(s.pendingMsgs, PendingMessage{
			ID:        uuid.New().String(),
			From:      msg.From,
			Chat:      msg.Chat,
			Content:   msg.Content,
			Timestamp: time.Now().Format(time.RFC3339),
			IsVoice:   msg.IsVoice,
		})
		s.pendingMu.Unlock()

		fmt.Fprintf(os.Stderr, "📨 Message received: %s\n", msg.Content)
	})

	fmt.Fprintf(os.Stderr, "✅ WhatsApp connected\n")
	fmt.Fprintf(os.Stderr, "📡 MCP server ready (stdio)\n")

	// Start JSON-RPC loop
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(&req)
	}

	return nil
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "resources/list":
		s.handleResourcesList(req)
	case "resources/read":
		s.handleResourcesRead(req)
	default:
		s.sendError(req.ID, -32601, "Method not found")
	}
}

func (s *Server) handleInitialize(req *JSONRPCRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "codebutler",
			"version": "1.0.0",
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *JSONRPCRequest) {
	tools := []map[string]interface{}{
		{
			"name":        "send_message",
			"description": "Send a message to WhatsApp",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "The message to send",
					},
				},
				"required": []string{"message"},
			},
		},
		{
			"name":        "ask_question",
			"description": "Ask the user a question with options. Returns their selection.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"question": map[string]interface{}{
						"type":        "string",
						"description": "The question to ask",
					},
					"options": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "List of options (2-5 choices)",
					},
					"timeout": map[string]interface{}{
						"type":        "integer",
						"description": "Timeout in seconds (default 30)",
					},
				},
				"required": []string{"question", "options"},
			},
		},
		{
			"name":        "get_pending",
			"description": "Get pending WhatsApp messages that haven't been processed yet",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "get_status",
			"description": "Get WhatsApp connection status",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	s.sendResult(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(req *JSONRPCRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params")
		return
	}

	switch params.Name {
	case "send_message":
		s.toolSendMessage(req.ID, params.Arguments)
	case "ask_question":
		s.toolAskQuestion(req.ID, params.Arguments)
	case "get_pending":
		s.toolGetPending(req.ID)
	case "get_status":
		s.toolGetStatus(req.ID)
	default:
		s.sendError(req.ID, -32602, "Unknown tool: "+params.Name)
	}
}

func (s *Server) toolSendMessage(id interface{}, args json.RawMessage) {
	var params struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		s.sendError(id, -32602, "Invalid message params")
		return
	}

	chatJID := s.currentChatID
	if chatJID == "" {
		chatJID = s.config.WhatsApp.GroupJID
	}

	botPrefix := s.config.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	message := botPrefix + " " + params.Message

	if err := s.client.SendMessage(chatJID, message); err != nil {
		s.sendError(id, -32000, "Failed to send: "+err.Error())
		return
	}

	fmt.Fprintf(os.Stderr, "📤 Message sent\n")

	s.sendResult(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": "Message sent successfully"},
		},
	})
}

func (s *Server) toolAskQuestion(id interface{}, args json.RawMessage) {
	var params struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
		Timeout  int      `json:"timeout"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		s.sendError(id, -32602, "Invalid question params")
		return
	}

	if params.Timeout == 0 {
		params.Timeout = 30
	}

	chatJID := s.currentChatID
	if chatJID == "" {
		chatJID = s.config.WhatsApp.GroupJID
	}

	botPrefix := s.config.WhatsApp.BotPrefix
	if botPrefix == "" {
		botPrefix = "[BOT]"
	}

	// Format question
	message := botPrefix + " " + params.Question + "\n"
	for i, opt := range params.Options {
		message += fmt.Sprintf("%d. %s\n", i+1, opt)
	}

	if err := s.client.SendMessage(chatJID, message); err != nil {
		s.sendError(id, -32000, "Failed to send question: "+err.Error())
		return
	}

	fmt.Fprintf(os.Stderr, "❓ Question sent, waiting for answer...\n")

	// Wait for answer
	select {
	case answer := <-s.answerChan:
		text := ""
		if answer.Selected > 0 && answer.Selected <= len(params.Options) {
			text = params.Options[answer.Selected-1]
		}

		fmt.Fprintf(os.Stderr, "✅ Answer received: %s\n", text)

		s.sendResult(id, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("User selected: %d (%s)", answer.Selected, text)},
			},
		})

	case <-time.After(time.Duration(params.Timeout) * time.Second):
		s.sendResult(id, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Question timed out - no response received"},
			},
		})
	}
}

func (s *Server) toolGetPending(id interface{}) {
	s.pendingMu.Lock()
	msgs := make([]PendingMessage, len(s.pendingMsgs))
	copy(msgs, s.pendingMsgs)
	s.pendingMsgs = []PendingMessage{} // Clear pending
	s.pendingMu.Unlock()

	if len(msgs) == 0 {
		s.sendResult(id, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "No pending messages"},
			},
		})
		return
	}

	// Format messages
	result := "Pending messages:\n"
	for _, msg := range msgs {
		result += fmt.Sprintf("\n[%s] %s: %s", msg.ID[:8], msg.From, msg.Content)
		if msg.Chat != "" {
			s.currentChatID = msg.Chat
		}
	}

	s.sendResult(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": result},
		},
	})
}

func (s *Server) toolGetStatus(id interface{}) {
	connected := s.client != nil && s.client.IsConnected()

	info := "Not connected"
	if connected {
		if userInfo, err := s.client.GetInfo(); err == nil {
			info = fmt.Sprintf("Connected as: %s\nGroup: %s", userInfo.Name, s.config.WhatsApp.GroupName)
		}
	}

	s.sendResult(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": info},
		},
	})
}

func (s *Server) handleResourcesList(req *JSONRPCRequest) {
	resources := []map[string]interface{}{
		{
			"uri":         "codebutler://config",
			"name":        "CodeButler Configuration",
			"description": "Current WhatsApp configuration",
			"mimeType":    "application/json",
		},
	}

	s.sendResult(req.ID, map[string]interface{}{"resources": resources})
}

func (s *Server) handleResourcesRead(req *JSONRPCRequest) {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params")
		return
	}

	if params.URI == "codebutler://config" {
		configJSON, _ := json.MarshalIndent(s.config, "", "  ")
		s.sendResult(req.ID, map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      params.URI,
					"mimeType": "application/json",
					"text":     string(configJSON),
				},
			},
		})
		return
	}

	s.sendError(req.ID, -32602, "Unknown resource: "+params.URI)
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func (s *Server) sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
