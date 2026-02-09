package whatsapp

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "github.com/mattn/go-sqlite3"
)

type Client struct {
	wac         *whatsmeow.Client
	sessionPath string
}

type Info struct {
	JID  string
	Name string
}

// Connect establishes a connection to WhatsApp
// If session exists, it will resume. Otherwise, it will show a QR code for pairing.
func Connect(sessionPath string) (*Client, error) {
	// Ensure session directory exists
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	// Create database for session storage
	dbPath := sessionPath + "/session.db"
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dbPath+"?_foreign_keys=on", waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlstore: %w", err)
	}

	// Get first device from store (or create new)
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	// Create WhatsApp client
	wac := whatsmeow.NewClient(deviceStore, waLog.Noop)

	// Check if already logged in
	if wac.Store.ID == nil {
		// Not logged in, need to pair
		fmt.Println("\n📱 Scan this QR code with WhatsApp:")
		fmt.Println("   (Go to WhatsApp > Settings > Linked Devices > Link a Device)")
		fmt.Println()

		// Generate QR code
		qrChan, _ := wac.GetQRChannel(context.Background())
		err = wac.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}

		// Wait for QR scan
		for evt := range qrChan {
			if evt.Event == "code" {
				// Display QR code as ASCII art
				displayQR(evt.Code)
			} else if evt.Event == "success" {
				fmt.Println("\n✅ Successfully paired!")
				break
			}
		}
	} else {
		// Already logged in, just connect
		err = wac.Connect()
		if err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}

		// Wait for connection
		for !wac.IsConnected() {
			time.Sleep(100 * time.Millisecond)
		}

		fmt.Println("✅ Connected to WhatsApp")
	}

	return &Client{
		wac:         wac,
		sessionPath: sessionPath,
	}, nil
}

// Disconnect closes the WhatsApp connection
func (c *Client) Disconnect() {
	if c.wac != nil {
		c.wac.Disconnect()
	}
}

// GetInfo returns information about the logged-in account
func (c *Client) GetInfo() (*Info, error) {
	if c.wac.Store.ID == nil {
		return nil, fmt.Errorf("not logged in")
	}

	jid := c.wac.Store.ID.String()

	// Get push name if available
	name := ""
	if c.wac.Store.PushName != "" {
		name = c.wac.Store.PushName
	}

	return &Info{
		JID:  jid,
		Name: name,
	}, nil
}

// IsConnected returns whether the client is currently connected
func (c *Client) IsConnected() bool {
	return c.wac != nil && c.wac.IsConnected()
}

// GetJID returns the user's JID as a types.JID
func (c *Client) GetJID() types.JID {
	if c.wac.Store.ID == nil {
		return types.JID{}
	}
	return *c.wac.Store.ID
}

// ConnectWithQR returns the client and QR channel for web-based setup
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
	}

	return client, qrChan, nil
}
