package messenger

import (
	"fmt"
	"strings"
	"sync"
)

// MultiMessenger wraps multiple Messenger backends. Messages from any backend
// are forwarded to the handler. Sends go to all connected backends.
// If one backend fails to connect, the others keep running.
//
// Each backend has its own chatID (WhatsApp groupJID, Slack channelID).
// The daemon uses a single chatID; MultiMessenger translates it to each
// backend's chatID when sending.
type MultiMessenger struct {
	backends []Messenger
	chatIDs  map[string]string // backend.Name() → chatID
	names    []string

	connMu    sync.RWMutex
	connState ConnectionState

	msgHandler  MessageHandler
	connHandler ConnectionHandler
}

// BackendChat maps a backend to its chat target.
type BackendChat struct {
	Backend Messenger
	ChatID  string
}

// NewMulti creates a multi-messenger. Each BackendChat pairs a backend
// with the chatID it should send to.
func NewMulti(entries ...BackendChat) *MultiMessenger {
	backends := make([]Messenger, len(entries))
	chatIDs := make(map[string]string, len(entries))
	names := make([]string, len(entries))
	for i, e := range entries {
		backends[i] = e.Backend
		chatIDs[e.Backend.Name()] = e.ChatID
		names[i] = e.Backend.Name()
	}
	return &MultiMessenger{
		backends:  backends,
		chatIDs:   chatIDs,
		names:     names,
		connState: StateDisconnected,
	}
}

func (m *MultiMessenger) Connect() error {
	var connected int
	var lastErr error

	for _, b := range m.backends {
		if err := b.Connect(); err != nil {
			lastErr = err
			continue
		}
		connected++
	}

	if connected == 0 {
		return fmt.Errorf("all backends failed to connect (last: %w)", lastErr)
	}

	// Wire up handlers after connect
	for _, b := range m.backends {
		backend := b // capture
		if m.msgHandler != nil {
			backend.OnMessage(m.msgHandler)
		}
		if m.connHandler != nil {
			backend.OnConnectionEvent(func(state ConnectionState) {
				m.updateState()
				m.connHandler(state)
			})
		}
	}

	m.updateState()
	return nil
}

func (m *MultiMessenger) Disconnect() {
	for _, b := range m.backends {
		b.Disconnect()
	}
	m.connMu.Lock()
	m.connState = StateDisconnected
	m.connMu.Unlock()
}

func (m *MultiMessenger) GetState() ConnectionState {
	m.connMu.RLock()
	defer m.connMu.RUnlock()
	return m.connState
}

func (m *MultiMessenger) GetOwnID() string {
	// Return first non-empty
	for _, b := range m.backends {
		if id := b.GetOwnID(); id != "" {
			return id
		}
	}
	return ""
}

func (m *MultiMessenger) OnMessage(handler MessageHandler) {
	m.msgHandler = handler
	// Wire up to already-connected backends
	for _, b := range m.backends {
		b.OnMessage(handler)
	}
}

func (m *MultiMessenger) OnConnectionEvent(handler ConnectionHandler) {
	m.connHandler = handler
	for _, b := range m.backends {
		backend := b
		_ = backend
		b.OnConnectionEvent(func(state ConnectionState) {
			m.updateState()
			handler(state)
		})
	}
}

func (m *MultiMessenger) SendMessage(chatID, text string) (string, error) {
	var lastID string
	var lastErr error
	for _, b := range m.backends {
		if b.GetState() != StateConnected {
			continue
		}
		target := m.resolveChat(b, chatID)
		id, err := b.SendMessage(target, text)
		if err != nil {
			lastErr = err
			continue
		}
		lastID = id
	}
	if lastID == "" && lastErr != nil {
		return "", lastErr
	}
	return lastID, nil
}

func (m *MultiMessenger) SendImage(chatID string, imgData []byte, caption string) error {
	var lastErr error
	for _, b := range m.backends {
		if b.GetState() != StateConnected {
			continue
		}
		target := m.resolveChat(b, chatID)
		if err := b.SendImage(target, imgData, caption); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (m *MultiMessenger) SendVideo(chatID string, videoData []byte, caption string) error {
	var lastErr error
	for _, b := range m.backends {
		if b.GetState() != StateConnected {
			continue
		}
		target := m.resolveChat(b, chatID)
		if err := b.SendVideo(target, videoData, caption); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (m *MultiMessenger) SendPresence(chatID string, composing bool) error {
	for _, b := range m.backends {
		if b.GetState() == StateConnected {
			target := m.resolveChat(b, chatID)
			b.SendPresence(target, composing)
		}
	}
	return nil
}

func (m *MultiMessenger) MarkRead(chatID, senderID string, messageIDs []string) error {
	for _, b := range m.backends {
		if b.GetState() == StateConnected {
			target := m.resolveChat(b, chatID)
			b.MarkRead(target, senderID, messageIDs)
		}
	}
	return nil
}

// resolveChat returns the backend's own chatID if registered, otherwise falls back to the given chatID.
func (m *MultiMessenger) resolveChat(b Messenger, fallback string) string {
	if id, ok := m.chatIDs[b.Name()]; ok && id != "" {
		return id
	}
	return fallback
}

func (m *MultiMessenger) DownloadAudio(msg Message) (string, error) {
	// Try each backend — only the one that owns the message will succeed
	for _, b := range m.backends {
		path, err := b.DownloadAudio(msg)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no backend could download audio")
}

func (m *MultiMessenger) DownloadImage(msg Message) ([]byte, error) {
	for _, b := range m.backends {
		data, err := b.DownloadImage(msg)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("no backend could download image")
}

func (m *MultiMessenger) Name() string {
	var active []string
	for _, b := range m.backends {
		active = append(active, b.Name())
	}
	return strings.Join(active, "+")
}

// updateState sets overall state to Connected if ANY backend is connected.
func (m *MultiMessenger) updateState() {
	m.connMu.Lock()
	defer m.connMu.Unlock()

	for _, b := range m.backends {
		if b.GetState() == StateConnected {
			m.connState = StateConnected
			return
		}
	}
	m.connState = StateDisconnected
}
