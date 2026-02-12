package messenger

import (
	"fmt"

	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

// WhatsAppAdapter wraps whatsapp.Client to implement the Messenger interface.
type WhatsAppAdapter struct {
	client      *whatsapp.Client
	sessionPath string
	deviceName  string
}

// NewWhatsApp creates a WhatsApp messenger. Call Connect() to establish the connection.
func NewWhatsApp(sessionPath, deviceName string) *WhatsAppAdapter {
	return &WhatsAppAdapter{
		sessionPath: sessionPath,
		deviceName:  deviceName,
	}
}

// SetClient sets an already-connected WhatsApp client (used by the daemon
// when it manages connection/reconnection externally).
func (w *WhatsAppAdapter) SetClient(client *whatsapp.Client) {
	w.client = client
}

func (w *WhatsAppAdapter) Connect() error {
	whatsapp.SetDeviceName(w.deviceName)
	client, err := whatsapp.Connect(w.sessionPath)
	if err != nil {
		return err
	}
	w.client = client
	return nil
}

func (w *WhatsAppAdapter) Disconnect() {
	if w.client != nil {
		w.client.Disconnect()
	}
}

func (w *WhatsAppAdapter) GetState() ConnectionState {
	if w.client == nil {
		return StateDisconnected
	}
	return mapWAState(w.client.GetState())
}

func (w *WhatsAppAdapter) GetOwnID() string {
	if w.client == nil {
		return ""
	}
	return w.client.GetJID().String()
}

func (w *WhatsAppAdapter) OnMessage(handler MessageHandler) {
	if w.client == nil {
		return
	}
	w.client.OnMessage(func(msg whatsapp.Message) {
		handler(Message{
			ID:       msg.ID,
			From:     msg.From,
			Chat:     msg.Chat,
			Content:  msg.Content,
			IsFromMe: msg.IsFromMe,
			IsGroup:  msg.IsGroup,
			IsVoice:  msg.IsVoice,
			IsImage:  msg.IsImage,
			RawEvent: msg.RawEvent,
		})
	})
}

func (w *WhatsAppAdapter) OnConnectionEvent(handler ConnectionHandler) {
	if w.client == nil {
		return
	}
	w.client.OnConnectionEvent(func(state whatsapp.ConnectionState) {
		handler(mapWAState(state))
	})
}

func (w *WhatsAppAdapter) SendMessage(chatID, text string) (string, error) {
	if w.client == nil {
		return "", errNotConnected
	}
	return w.client.SendMessage(chatID, text)
}

func (w *WhatsAppAdapter) SendImage(chatID string, imgData []byte, caption string) error {
	if w.client == nil {
		return errNotConnected
	}
	return w.client.SendImage(chatID, imgData, caption)
}

func (w *WhatsAppAdapter) SendVideo(chatID string, videoData []byte, caption string) error {
	if w.client == nil {
		return errNotConnected
	}
	return w.client.SendVideo(chatID, videoData, caption)
}

func (w *WhatsAppAdapter) SendPresence(chatID string, composing bool) error {
	if w.client == nil {
		return errNotConnected
	}
	return w.client.SendPresence(chatID, composing)
}

func (w *WhatsAppAdapter) MarkRead(chatID, senderID string, messageIDs []string) error {
	if w.client == nil {
		return errNotConnected
	}
	return w.client.MarkRead(chatID, senderID, messageIDs)
}

func (w *WhatsAppAdapter) DownloadAudio(msg Message) (string, error) {
	if w.client == nil {
		return "", errNotConnected
	}
	waMsg := whatsapp.Message{
		IsVoice:  msg.IsVoice,
		RawEvent: msg.RawEvent,
	}
	return w.client.DownloadAudioFromMessage(waMsg)
}

func (w *WhatsAppAdapter) DownloadImage(msg Message) ([]byte, error) {
	if w.client == nil {
		return nil, errNotConnected
	}
	waMsg := whatsapp.Message{
		IsImage:  msg.IsImage,
		RawEvent: msg.RawEvent,
	}
	return w.client.DownloadImageFromMessage(waMsg)
}

func (w *WhatsAppAdapter) Name() string {
	return "WhatsApp"
}

// Client returns the underlying whatsapp.Client for operations that need
// direct access (e.g., QR setup, groups). This is WhatsApp-specific.
func (w *WhatsAppAdapter) Client() *whatsapp.Client {
	return w.client
}

func mapWAState(s whatsapp.ConnectionState) ConnectionState {
	switch s {
	case whatsapp.StateConnected:
		return StateConnected
	case whatsapp.StateDisconnected:
		return StateDisconnected
	case whatsapp.StateReconnecting:
		return StateReconnecting
	case whatsapp.StateLoggedOut:
		return StateLoggedOut
	case whatsapp.StateConnecting:
		return StateConnecting
	default:
		return StateDisconnected
	}
}

var errNotConnected = fmt.Errorf("not connected")
