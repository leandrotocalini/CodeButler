package slack

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// codeSnippetThreshold is the line count above which code is uploaded as a file.
const codeSnippetThreshold = 20

// Client wraps the Slack API and Socket Mode for agent communication.
type Client struct {
	api      *slack.Client
	socket   *socketmode.Client
	identity AgentIdentity
	dedup    *DedupSet
	logger   *slog.Logger

	// handler is called for each new message event that passes dedup.
	handler func(evt MessageEvent)
}

// MessageEvent is a simplified Slack message event for agent processing.
type MessageEvent struct {
	EventID   string
	ChannelID string
	ThreadTS  string // thread timestamp (empty for non-threaded messages)
	MessageTS string // message timestamp
	UserID    string
	Text      string
	BotID     string // non-empty if sent by a bot
}

// ClientOption configures the Slack client.
type ClientOption func(*Client)

// WithSlackLogger sets the structured logger.
func WithSlackLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithDedupSet sets a custom dedup set (useful for testing).
func WithDedupSet(d *DedupSet) ClientOption {
	return func(c *Client) {
		c.dedup = d
	}
}

// NewClient creates a Slack client with Socket Mode support.
// botToken is the xoxb-... token, appToken is the xapp-... token.
func NewClient(botToken, appToken string, identity AgentIdentity, opts ...ClientOption) *Client {
	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	socket := socketmode.New(api)

	c := &Client{
		api:      api,
		socket:   socket,
		identity: identity,
		dedup:    NewDedupSet(),
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// OnMessage registers a handler for incoming message events.
// The handler is called for each new, non-duplicate message.
func (c *Client) OnMessage(handler func(evt MessageEvent)) {
	c.handler = handler
}

// Listen starts the Socket Mode event loop. Blocks until context is cancelled.
// Events are filtered through the dedup set before being dispatched.
func (c *Client) Listen(ctx context.Context) error {
	go func() {
		for evt := range c.socket.Events {
			c.handleSocketEvent(evt)
		}
	}()

	return c.socket.RunContext(ctx)
}

// handleSocketEvent processes a single Socket Mode event.
func (c *Client) handleSocketEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		c.socket.Ack(*evt.Request)
		c.handleEventsAPI(evt)

	case socketmode.EventTypeInteractive:
		c.socket.Ack(*evt.Request)
		// Block Kit interactions handled in M10

	case socketmode.EventTypeConnecting:
		c.logger.Info("connecting to Slack")

	case socketmode.EventTypeConnected:
		c.logger.Info("connected to Slack")

	case socketmode.EventTypeConnectionError:
		c.logger.Error("slack connection error")

	default:
		// Ignore other event types
	}
}

// handleEventsAPI processes Events API events (messages).
func (c *Client) handleEventsAPI(evt socketmode.Event) {
	eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	if eventsAPI.Type == slackevents.CallbackEvent {
		c.handleCallbackEvent(eventsAPI)
	}
}

// handleCallbackEvent processes callback events (message.channels, message.groups).
func (c *Client) handleCallbackEvent(evt slackevents.EventsAPIEvent) {
	innerEvent := evt.InnerEvent

	// Extract event ID from the callback event data if available
	var eventID string
	if cbEvt, ok := evt.Data.(*slackevents.EventsAPICallbackEvent); ok {
		eventID = cbEvt.EventID
	}

	switch ev := innerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		// Skip bot messages (prevent self-loops)
		if ev.BotID != "" {
			return
		}
		// Skip message subtypes (edits, deletions, etc.)
		if ev.SubType != "" {
			return
		}

		if eventID == "" {
			eventID = ev.TimeStamp // fallback
		}

		// Dedup check
		if !c.dedup.Check(eventID) {
			c.logger.Debug("duplicate event skipped", "event_id", eventID)
			return
		}

		// Determine thread timestamp
		threadTS := ev.ThreadTimeStamp
		if threadTS == "" {
			threadTS = ev.TimeStamp // top-level message is its own thread
		}

		msgEvt := MessageEvent{
			EventID:   eventID,
			ChannelID: ev.Channel,
			ThreadTS:  threadTS,
			MessageTS: ev.TimeStamp,
			UserID:    ev.User,
			Text:      ev.Text,
			BotID:     ev.BotID,
		}

		c.logger.Info("message received",
			"channel", msgEvt.ChannelID,
			"thread", msgEvt.ThreadTS,
			"user", msgEvt.UserID,
		)

		if c.handler != nil {
			c.handler(msgEvt)
		}
	}
}

// SendMessage posts a message to a Slack channel/thread with the agent's identity.
func (c *Client) SendMessage(ctx context.Context, channel, threadTS, text string) error {
	opts := []slack.MsgOption{
		slack.MsgOptionText(text, false),
		slack.MsgOptionUsername(c.identity.DisplayName),
		slack.MsgOptionIconEmoji(c.identity.IconEmoji),
	}

	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}

	_, _, err := c.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		return fmt.Errorf("slack send message: %w", err)
	}

	return nil
}

// SendCodeSnippet posts code as a file upload if it exceeds the threshold,
// or as an inline code block if short enough.
func (c *Client) SendCodeSnippet(ctx context.Context, channel, threadTS, filename, content string) error {
	lines := strings.Count(content, "\n") + 1

	if lines < codeSnippetThreshold {
		// Inline code block
		text := fmt.Sprintf("```%s\n%s\n```", filename, content)
		return c.SendMessage(ctx, channel, threadTS, text)
	}

	// File upload for longer snippets
	params := slack.FileUploadParameters{
		Filename: filename,
		Content:  content,
		Channels: []string{channel},
	}
	if threadTS != "" {
		params.ThreadTimestamp = threadTS
	}

	_, err := c.api.UploadFileContext(ctx, params)
	if err != nil {
		return fmt.Errorf("slack file upload: %w", err)
	}

	return nil
}

// AddReaction adds an emoji reaction to a message.
func (c *Client) AddReaction(ctx context.Context, channel, messageTS, emoji string) error {
	ref := slack.ItemRef{
		Channel:   channel,
		Timestamp: messageTS,
	}
	err := c.api.AddReactionContext(ctx, emoji, ref)
	if err != nil {
		return fmt.Errorf("slack add reaction: %w", err)
	}
	return nil
}

// RemoveReaction removes an emoji reaction from a message.
func (c *Client) RemoveReaction(ctx context.Context, channel, messageTS, emoji string) error {
	ref := slack.ItemRef{
		Channel:   channel,
		Timestamp: messageTS,
	}
	err := c.api.RemoveReactionContext(ctx, emoji, ref)
	if err != nil {
		return fmt.Errorf("slack remove reaction: %w", err)
	}
	return nil
}

// ReactProcessing adds the 👀 reaction to indicate the agent is processing.
func (c *Client) ReactProcessing(ctx context.Context, channel, messageTS string) error {
	return c.AddReaction(ctx, channel, messageTS, "eyes")
}

// ReactDone replaces 👀 with ✅ to indicate processing is complete.
func (c *Client) ReactDone(ctx context.Context, channel, messageTS string) error {
	// Best-effort removal of processing reaction
	_ = c.RemoveReaction(ctx, channel, messageTS, "eyes")
	return c.AddReaction(ctx, channel, messageTS, "white_check_mark")
}
