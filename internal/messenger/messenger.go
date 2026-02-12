// Package messenger defines the interface for messaging backends (WhatsApp, Slack, etc.).
// The daemon talks to this interface and doesn't know which backend is active.
package messenger

// ConnectionState represents the connection status of a messenger backend.
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateReconnecting
	StateLoggedOut
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateLoggedOut:
		return "logged out"
	default:
		return "unknown"
	}
}

// Message represents an incoming message from any backend.
type Message struct {
	ID        string // Backend-specific message ID
	From      string // Sender identifier
	Chat      string // Chat/channel identifier
	Content   string // Text content
	IsFromMe  bool
	IsGroup   bool
	IsVoice   bool   // Voice/audio message
	IsImage   bool   // Image message
	ThreadID  string // Thread ID (Slack threads, empty for WhatsApp)
	RawEvent  interface{}
}

// MessageHandler is a callback for incoming messages.
type MessageHandler func(Message)

// ConnectionHandler is a callback for connection state changes.
type ConnectionHandler func(ConnectionState)

// Messenger is the interface that messaging backends must implement.
type Messenger interface {
	// Connection lifecycle
	Connect() error
	Disconnect()
	GetState() ConnectionState
	GetOwnID() string

	// Event handlers
	OnMessage(handler MessageHandler)
	OnConnectionEvent(handler ConnectionHandler)

	// Sending
	SendMessage(chatID, text string) (string, error)
	SendImage(chatID string, imgData []byte, caption string) error
	SendVideo(chatID string, videoData []byte, caption string) error
	SendPresence(chatID string, composing bool) error

	// Read receipts
	MarkRead(chatID, senderID string, messageIDs []string) error

	// Media downloads
	DownloadAudio(msg Message) (string, error)   // returns temp file path
	DownloadImage(msg Message) ([]byte, error)    // returns image bytes

	// Backend name for logging
	Name() string
}
