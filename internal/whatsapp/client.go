package whatsapp

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3"
)

// ConnectionState represents the current WhatsApp connection state.
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
		return "logged_out"
	default:
		return "unknown"
	}
}

// ConnectionEventHandler is called when the connection state changes.
type ConnectionEventHandler func(state ConnectionState)

type Client struct {
	wac         *whatsmeow.Client
	sessionPath string
	connState   ConnectionState
	connMu      sync.RWMutex
	onConnEvent ConnectionEventHandler
}

type Info struct {
	JID  string
	Name string
}

func init() {
	// Reconnect after 30s of keepalive failures instead of the default 3 minutes.
	whatsmeow.KeepAliveMaxFailTime = 30 * time.Second
}

// SetDeviceName sets the name that appears in WhatsApp > Linked Devices.
// Must be called before Connect.
func SetDeviceName(name string) {
	store.SetOSInfo(name, [3]uint32{1, 0, 0})
}

// Connect establishes a connection to WhatsApp.
// If session exists, it will resume. Otherwise, it will show a QR code for pairing.
func Connect(sessionPath string) (*Client, error) {
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	dbPath := sessionPath + "/session.db"
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	wac := whatsmeow.NewClient(deviceStore, waLog.Noop)

	client := &Client{
		wac:         wac,
		sessionPath: sessionPath,
		connState:   StateConnecting,
	}

	// Register connection event handlers before connecting
	client.registerConnectionEvents()

	if wac.Store.ID == nil {
		// Not logged in, need to pair
		fmt.Println("\n📱 Scan this QR code with WhatsApp:")
		fmt.Println("   (Go to WhatsApp > Settings > Linked Devices > Link a Device)")
		fmt.Println()

		qrChan, _ := wac.GetQRChannel(context.Background())
		err = wac.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				displayQR(evt.Code)
			} else if evt.Event == "success" {
				fmt.Println("\n✅ Successfully paired!")
				break
			}
		}

		// Wait for connection to stabilize after pairing
		start := time.Now()
		for !wac.IsConnected() {
			if time.Since(start) > 30*time.Second {
				break // Continue anyway, might reconnect later
			}
			time.Sleep(200 * time.Millisecond)
		}
	} else {
		err = wac.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}

		// Wait for connection with timeout
		start := time.Now()
		for !wac.IsConnected() {
			if time.Since(start) > 30*time.Second {
				wac.Disconnect()
				return nil, fmt.Errorf("connection timed out after 30s")
			}
			time.Sleep(100 * time.Millisecond)
		}

		fmt.Fprintln(os.Stderr, "✅ Connected to WhatsApp")
	}

	return client, nil
}

// registerConnectionEvents sets up whatsmeow event handlers for connection lifecycle.
func (c *Client) registerConnectionEvents() {
	c.wac.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			c.setState(StateConnected)
			// Mark device as "available" so chat presence (composing) and
			// read receipts actually work. Without this, WhatsApp treats
			// the linked device as offline and ignores both.
			_ = c.wac.SendPresence(context.Background(), types.PresenceAvailable)
		case *events.Disconnected:
			// whatsmeow auto-reconnects if EnableAutoReconnect is true (default).
			// Mark as reconnecting — Connected event will fire when back.
			if c.GetState() != StateLoggedOut {
				c.setState(StateReconnecting)
			}
		case *events.KeepAliveTimeout:
			if c.GetState() == StateConnected {
				c.setState(StateReconnecting)
			}
		case *events.KeepAliveRestored:
			c.setState(StateConnected)
		case *events.LoggedOut:
			c.setState(StateLoggedOut)
		case *events.StreamReplaced:
			c.setState(StateLoggedOut)
		}
	})
}

// OnConnectionEvent registers a handler for connection state changes.
func (c *Client) OnConnectionEvent(handler ConnectionEventHandler) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.onConnEvent = handler
}

func (c *Client) setState(state ConnectionState) {
	c.connMu.Lock()
	old := c.connState
	c.connState = state
	handler := c.onConnEvent
	c.connMu.Unlock()

	if old != state {
		fmt.Fprintf(os.Stderr, "📡 WhatsApp: %s → %s\n", old, state)
		if handler != nil {
			handler(state)
		}
	}
}

// GetState returns the current connection state.
func (c *Client) GetState() ConnectionState {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connState
}

// Disconnect closes the WhatsApp connection.
func (c *Client) Disconnect() {
	if c.wac != nil {
		c.wac.Disconnect()
	}
}

// GetInfo returns information about the logged-in account.
func (c *Client) GetInfo() (*Info, error) {
	if c.wac.Store.ID == nil {
		return nil, fmt.Errorf("not logged in")
	}

	jid := c.wac.Store.ID.String()

	name := ""
	if c.wac.Store.PushName != "" {
		name = c.wac.Store.PushName
	}

	return &Info{
		JID:  jid,
		Name: name,
	}, nil
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	return c.wac != nil && c.wac.IsConnected()
}

// GetJID returns the user's JID as a types.JID.
func (c *Client) GetJID() types.JID {
	if c.wac.Store.ID == nil {
		return types.JID{}
	}
	return *c.wac.Store.ID
}

// MarkRead sends read receipts for the given WhatsApp message IDs.
func (c *Client) MarkRead(chatJID, senderJID string, messageIDs []string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}

	sender, err := types.ParseJID(senderJID)
	if err != nil {
		return fmt.Errorf("invalid sender JID: %w", err)
	}

	return c.wac.MarkRead(context.Background(), messageIDs, time.Now(), chat, sender)
}

// SendPresence sends a "composing" or "paused" chat presence indicator.
func (c *Client) SendPresence(chatJID string, composing bool) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	state := types.ChatPresencePaused
	if composing {
		state = types.ChatPresenceComposing
	}

	return c.wac.SendChatPresence(context.Background(), jid, state, "")
}

// SendImage uploads and sends an image message to a chat.
func (c *Client) SendImage(chatJID string, pngData []byte, caption string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to WhatsApp")
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	uploadResp, err := c.wac.Upload(context.Background(), pngData, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}

	fileLen := uint64(len(pngData))
	imgMsg := &waProto.ImageMessage{
		URL:           proto.String(uploadResp.URL),
		DirectPath:    proto.String(uploadResp.DirectPath),
		MediaKey:      uploadResp.MediaKey,
		Mimetype:      proto.String("image/png"),
		Caption:       proto.String(caption),
		FileEncSHA256: uploadResp.FileEncSHA256,
		FileSHA256:    uploadResp.FileSHA256,
		FileLength:    &fileLen,
	}

	msg := &waProto.Message{
		ImageMessage: imgMsg,
	}

	_, err = c.wac.SendMessage(context.Background(), jid, msg)
	if err != nil {
		return fmt.Errorf("failed to send image: %w", err)
	}

	return nil
}

// ConnectWithQR returns the client and QR channel for web-based setup.
func ConnectWithQR(sessionPath string) (*Client, <-chan whatsmeow.QRChannelItem, error) {
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	dbPath := sessionPath + "/session.db"
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", waLog.Noop)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get device: %w", err)
	}

	wac := whatsmeow.NewClient(deviceStore, waLog.Noop)

	if wac.Store.ID != nil {
		return nil, nil, fmt.Errorf("already logged in, delete session first")
	}

	qrChan, _ := wac.GetQRChannel(context.Background())
	err = wac.Connect()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := &Client{
		wac:         wac,
		sessionPath: sessionPath,
		connState:   StateConnecting,
	}
	client.registerConnectionEvents()

	return client, qrChan, nil
}
