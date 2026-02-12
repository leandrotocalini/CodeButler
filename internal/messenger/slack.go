package messenger

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// SlackAdapter implements Messenger using Slack's Socket Mode.
type SlackAdapter struct {
	botToken  string
	appToken  string
	channelID string
	botPrefix string

	api    *slack.Client
	sm     *socketmode.Client
	botUID string // resolved on connect

	connState ConnectionState
	connMu    sync.RWMutex

	msgHandler  MessageHandler
	connHandler ConnectionHandler
}

// NewSlack creates a Slack messenger. Call Connect() to start Socket Mode.
func NewSlack(botToken, appToken, channelID, botPrefix string) *SlackAdapter {
	return &SlackAdapter{
		botToken:  botToken,
		appToken:  appToken,
		channelID: channelID,
		botPrefix: botPrefix,
		connState: StateDisconnected,
	}
}

func (s *SlackAdapter) Connect() error {
	s.api = slack.New(
		s.botToken,
		slack.OptionAppLevelToken(s.appToken),
	)

	// Resolve bot's own user ID
	authResp, err := s.api.AuthTest()
	if err != nil {
		return fmt.Errorf("slack auth test: %w", err)
	}
	s.botUID = authResp.UserID

	s.sm = socketmode.New(
		s.api,
		socketmode.OptionLog(log.New(os.Stderr, "slack-sm: ", log.Lshortfile|log.LstdFlags)),
	)

	s.setState(StateConnecting)

	// Run event processing loop
	go s.runSocketMode()

	// Run Socket Mode connection (blocking in goroutine)
	go func() {
		if err := s.sm.Run(); err != nil {
			s.setState(StateDisconnected)
		}
	}()

	return nil
}

func (s *SlackAdapter) runSocketMode() {
	for evt := range s.sm.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			s.setState(StateConnecting)

		case socketmode.EventTypeConnected:
			s.setState(StateConnected)

		case socketmode.EventTypeDisconnect:
			s.setState(StateReconnecting)

		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			s.sm.Ack(*evt.Request)
			s.handleEventsAPI(eventsAPIEvent)
		}
	}
}

func (s *SlackAdapter) handleEventsAPI(event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			// Ignore bot's own messages
			if ev.User == s.botUID || ev.BotID != "" {
				return
			}
			// Filter by channel
			if s.channelID != "" && ev.Channel != s.channelID {
				return
			}
			// Ignore message subtypes (edits, deletes, etc.) — only process new messages
			if ev.SubType != "" {
				return
			}

			if s.msgHandler != nil {
				msg := Message{
					ID:       ev.TimeStamp,
					From:     ev.User,
					Chat:     ev.Channel,
					Content:  ev.Text,
					IsGroup:  true,
					ThreadID: ev.ThreadTimeStamp,
					RawEvent: ev,
				}
				s.msgHandler(msg)
			}
		}
	}
}

func (s *SlackAdapter) Disconnect() {
	// socketmode.Client doesn't have a clean shutdown — it stops when context is cancelled.
	// For now, just update state.
	s.setState(StateDisconnected)
}

func (s *SlackAdapter) GetState() ConnectionState {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.connState
}

func (s *SlackAdapter) GetOwnID() string {
	return s.botUID
}

func (s *SlackAdapter) OnMessage(handler MessageHandler) {
	s.msgHandler = handler
}

func (s *SlackAdapter) OnConnectionEvent(handler ConnectionHandler) {
	s.connHandler = handler
}

func (s *SlackAdapter) SendMessage(chatID, text string) (string, error) {
	if s.api == nil {
		return "", fmt.Errorf("not connected")
	}
	_, ts, err := s.api.PostMessage(chatID, slack.MsgOptionText(text, false))
	if err != nil {
		return "", fmt.Errorf("slack send: %w", err)
	}
	return ts, nil
}

func (s *SlackAdapter) SendImage(chatID string, imgData []byte, caption string) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}

	params := slack.UploadFileV2Parameters{
		Channel:        chatID,
		Reader:         bytes.NewReader(imgData),
		Filename:       "image.png",
		Title:          caption,
		FileSize:       len(imgData),
		InitialComment: caption,
	}

	_, err := s.api.UploadFileV2(params)
	return err
}

func (s *SlackAdapter) SendVideo(chatID string, videoData []byte, caption string) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}

	params := slack.UploadFileV2Parameters{
		Channel:        chatID,
		Reader:         bytes.NewReader(videoData),
		Filename:       "video.mp4",
		Title:          caption,
		FileSize:       len(videoData),
		InitialComment: caption,
	}

	_, err := s.api.UploadFileV2(params)
	return err
}

func (s *SlackAdapter) SendPresence(chatID string, composing bool) error {
	// Slack doesn't have a "typing" indicator for bots — no-op
	return nil
}

func (s *SlackAdapter) MarkRead(chatID, senderID string, messageIDs []string) error {
	// Slack bots don't send read receipts — no-op
	return nil
}

func (s *SlackAdapter) DownloadAudio(msg Message) (string, error) {
	// Slack audio messages are files — download the file
	return "", fmt.Errorf("audio download not yet implemented for Slack")
}

func (s *SlackAdapter) DownloadImage(msg Message) ([]byte, error) {
	// Slack images are files attached to messages — need file download
	return nil, fmt.Errorf("image download not yet implemented for Slack")
}

func (s *SlackAdapter) Name() string {
	return "Slack"
}

// FindOrCreateChannel looks up a Slack channel by name and sets it as the
// target. If it doesn't exist, creates it. Must be called after the API
// client is initialized (i.e., after Connect sets s.api, or pre-init the API).
func (s *SlackAdapter) FindOrCreateChannel(name string) (string, error) {
	// Init API client if not yet done (for pre-connect channel resolution)
	if s.api == nil {
		s.api = slack.New(s.botToken, slack.OptionAppLevelToken(s.appToken))
	}

	// Slugify: Slack channel names must be lowercase, no spaces
	slugName := slackChannelSlug(name)

	// Search existing channels
	params := &slack.GetConversationsParameters{
		Types:           []string{"public_channel", "private_channel"},
		Limit:           200,
		ExcludeArchived: true,
	}
	for {
		channels, cursor, err := s.api.GetConversations(params)
		if err != nil {
			return "", fmt.Errorf("list channels: %w", err)
		}
		for _, ch := range channels {
			if ch.Name == slugName {
				s.channelID = ch.ID
				return ch.ID, nil
			}
		}
		if cursor == "" {
			break
		}
		params.Cursor = cursor
	}

	// Not found — create it
	ch, err := s.api.CreateConversation(slack.CreateConversationParams{
		ChannelName: slugName,
		IsPrivate:   false,
	})
	if err != nil {
		return "", fmt.Errorf("create channel %q: %w", slugName, err)
	}

	s.channelID = ch.ID
	return ch.ID, nil
}

// slackChannelSlug converts a name like "CodeButler Romina-MBP" to "codebutler-romina-mbp"
func slackChannelSlug(name string) string {
	var result []byte
	for _, r := range []byte(strings.ToLower(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result = append(result, r)
		} else if r == ' ' || r == '_' || r == '.' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result = append(result, '-')
			}
		}
	}
	return strings.Trim(string(result), "-")
}

func (s *SlackAdapter) setState(state ConnectionState) {
	s.connMu.Lock()
	old := s.connState
	s.connState = state
	handler := s.connHandler
	s.connMu.Unlock()

	if old != state && handler != nil {
		handler(state)
	}
}
