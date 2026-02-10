package whatsapp

import (
	"context"
	"fmt"
	"os"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Message represents a WhatsApp message
type Message struct {
	ID        string
	From      string    // JID of sender
	Chat      string    // JID of chat (group or personal)
	Content   string    // Text content
	Timestamp int64     // Unix timestamp
	IsFromMe  bool      // Whether we sent this message
	IsGroup   bool      // Whether from a group
	IsVoice   bool      // Whether it's a voice message
	IsImage   bool      // Whether it's an image message
	MediaURL  string    // URL or path to media (for voice messages)
	RawEvent  interface{} // Raw event for downloading media
}

// MessageHandler is a callback function for new messages
type MessageHandler func(Message)

// OnMessage registers a handler for incoming messages
func (c *Client) OnMessage(handler MessageHandler) {
	c.wac.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			msg := c.parseMessage(v)
			if msg != nil {
				handler(*msg)
			}
		}
	})
}

// parseMessage converts a whatsmeow event to our Message struct
func (c *Client) parseMessage(evt *events.Message) *Message {
	info := evt.Info

	// Skip messages from ourselves (except in groups where we need to see them)
	if info.IsFromMe && !info.IsGroup {
		return nil
	}

	msg := &Message{
		ID:        info.ID,
		From:      info.Sender.String(),
		Chat:      info.Chat.String(),
		Timestamp: info.Timestamp.Unix(),
		IsFromMe:  info.IsFromMe,
		IsGroup:   info.IsGroup,
	}

	// Extract content based on message type
	if evt.Message.Conversation != nil {
		msg.Content = *evt.Message.Conversation
	} else if evt.Message.ExtendedTextMessage != nil {
		msg.Content = *evt.Message.ExtendedTextMessage.Text
	} else if evt.Message.AudioMessage != nil {
		msg.IsVoice = true
		msg.Content = "[Voice Message]"
		msg.RawEvent = evt
	} else if evt.Message.ImageMessage != nil {
		msg.IsImage = true
		msg.RawEvent = evt
		msg.Content = "[Image]"
		if evt.Message.ImageMessage.Caption != nil {
			msg.Content = *evt.Message.ImageMessage.Caption
		}
	} else if evt.Message.DocumentMessage != nil {
		msg.Content = "[Document]"
		if evt.Message.DocumentMessage.Caption != nil {
			msg.Content = *evt.Message.DocumentMessage.Caption
		}
	} else {
		// Unsupported message type
		return nil
	}

	return msg
}

// SendMessage sends a text message to a chat
func (c *Client) SendMessage(chatJID string, text string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to WhatsApp")
	}

	// Parse JID
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Send message
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}

	_, err = c.wac.SendMessage(context.Background(), jid, msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// DownloadAudio downloads a voice message and returns the local file path
func (c *Client) DownloadAudio(evt *events.Message) (string, error) {
	if evt.Message.AudioMessage == nil {
		return "", fmt.Errorf("not an audio message")
	}

	// Download the audio file
	data, err := c.wac.Download(context.Background(), evt.Message.AudioMessage)
	if err != nil {
		return "", fmt.Errorf("failed to download audio: %w", err)
	}

	// Save to temp file
	tempPath := fmt.Sprintf("/tmp/codebutler-audio-%d.ogg", evt.Info.Timestamp.Unix())
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save audio: %w", err)
	}

	return tempPath, nil
}

// DownloadAudioFromMessage downloads a voice message from a Message struct
func (c *Client) DownloadAudioFromMessage(msg Message) (string, error) {
	if !msg.IsVoice {
		return "", fmt.Errorf("not a voice message")
	}

	if msg.RawEvent == nil {
		return "", fmt.Errorf("no raw event available for download")
	}

	evt, ok := msg.RawEvent.(*events.Message)
	if !ok {
		return "", fmt.Errorf("invalid raw event type")
	}

	return c.DownloadAudio(evt)
}

// DownloadImage downloads an image message and returns the raw bytes.
func (c *Client) DownloadImage(evt *events.Message) ([]byte, error) {
	if evt.Message.ImageMessage == nil {
		return nil, fmt.Errorf("not an image message")
	}

	data, err := c.wac.Download(context.Background(), evt.Message.ImageMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	return data, nil
}

// DownloadImageFromMessage downloads an image from a Message struct.
func (c *Client) DownloadImageFromMessage(msg Message) ([]byte, error) {
	if !msg.IsImage {
		return nil, fmt.Errorf("not an image message")
	}

	if msg.RawEvent == nil {
		return nil, fmt.Errorf("no raw event available for download")
	}

	evt, ok := msg.RawEvent.(*events.Message)
	if !ok {
		return nil, fmt.Errorf("invalid raw event type")
	}

	return c.DownloadImage(evt)
}
