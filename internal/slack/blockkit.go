package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/slack-go/slack"
)

// InteractionType identifies the kind of user interaction.
type InteractionType string

const (
	// InteractionButtonClick is a button press in a Block Kit message.
	InteractionButtonClick InteractionType = "button_click"
	// InteractionEmojiReaction is an emoji reaction on a message.
	InteractionEmojiReaction InteractionType = "emoji_reaction"
)

// Interaction represents a user interaction event (button click or emoji reaction).
type Interaction struct {
	Type      InteractionType
	ChannelID string
	ThreadTS  string
	MessageTS string
	UserID    string
	ActionID  string // Button action ID (e.g., "approve", "reject")
	Value     string // Button value or emoji name
}

// InteractionHandler is called when a user interacts with a Block Kit message or emoji.
type InteractionHandler func(interaction Interaction)

// ButtonOption defines a button in a Block Kit message.
type ButtonOption struct {
	ActionID string // Unique action identifier (e.g., "approve_plan")
	Text     string // Button display text
	Value    string // Value sent back on click
	Style    string // "primary" (green), "danger" (red), or "" (default)
}

// BlockKitMessage builds a Block Kit message with buttons.
type BlockKitMessage struct {
	HeaderText string
	BodyText   string
	Buttons    []ButtonOption
}

// BuildBlocks converts the message into Slack Block Kit JSON blocks.
func (m *BlockKitMessage) BuildBlocks() []slack.Block {
	blocks := make([]slack.Block, 0, 3)

	// Header section
	if m.HeaderText != "" {
		headerText := slack.NewTextBlockObject("mrkdwn", "*"+m.HeaderText+"*", false, false)
		blocks = append(blocks, slack.NewSectionBlock(headerText, nil, nil))
	}

	// Body section
	if m.BodyText != "" {
		bodyText := slack.NewTextBlockObject("mrkdwn", m.BodyText, false, false)
		blocks = append(blocks, slack.NewSectionBlock(bodyText, nil, nil))
	}

	// Buttons as actions
	if len(m.Buttons) > 0 {
		elements := make([]slack.BlockElement, 0, len(m.Buttons))
		for _, btn := range m.Buttons {
			btnText := slack.NewTextBlockObject("plain_text", btn.Text, false, false)
			button := slack.NewButtonBlockElement(btn.ActionID, btn.Value, btnText)
			if btn.Style != "" {
				button.Style = slack.Style(btn.Style)
			}
			elements = append(elements, button)
		}
		blocks = append(blocks, slack.NewActionBlock("", elements...))
	}

	return blocks
}

// BuildFallbackText creates a plain-text version for when Block Kit is unavailable.
func (m *BlockKitMessage) BuildFallbackText() string {
	text := ""
	if m.HeaderText != "" {
		text += m.HeaderText + "\n\n"
	}
	if m.BodyText != "" {
		text += m.BodyText + "\n\n"
	}
	if len(m.Buttons) > 0 {
		text += "Options:\n"
		for i, btn := range m.Buttons {
			text += fmt.Sprintf("%d. %s\n", i+1, btn.Text)
		}
	}
	return text
}

// SendBlockKit sends a Block Kit message to a channel/thread.
// Falls back to plain text if Block Kit rendering fails.
func (c *Client) SendBlockKit(ctx context.Context, channel, threadTS string, msg *BlockKitMessage) error {
	blocks := msg.BuildBlocks()
	fallback := msg.BuildFallbackText()

	opts := []slack.MsgOption{
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(fallback, false), // fallback for notifications
		slack.MsgOptionUsername(c.identity.DisplayName),
		slack.MsgOptionIconEmoji(c.identity.IconEmoji),
	}

	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}

	_, _, err := c.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		// Fall back to plain text
		c.logger.Warn("block kit failed, falling back to plain text", "err", err)
		return c.SendMessage(ctx, channel, threadTS, fallback)
	}

	return nil
}

// PlanApproval creates a standard plan approval Block Kit message.
func PlanApproval(planSummary string) *BlockKitMessage {
	return &BlockKitMessage{
		HeaderText: "Plan Review",
		BodyText:   planSummary,
		Buttons: []ButtonOption{
			{ActionID: "approve_plan", Text: "Approve", Value: "approve", Style: "primary"},
			{ActionID: "modify_plan", Text: "Modify", Value: "modify"},
			{ActionID: "reject_plan", Text: "Reject", Value: "reject", Style: "danger"},
		},
	}
}

// DestructiveToolApproval creates an approval message for destructive tool execution.
func DestructiveToolApproval(toolName, command string) *BlockKitMessage {
	return &BlockKitMessage{
		HeaderText: "Destructive Action Approval",
		BodyText:   fmt.Sprintf("Tool `%s` wants to execute:\n```\n%s\n```", toolName, command),
		Buttons: []ButtonOption{
			{ActionID: "approve_destructive", Text: "Approve", Value: "approve", Style: "danger"},
			{ActionID: "reject_destructive", Text: "Reject", Value: "reject"},
		},
	}
}

// ParseInteractionPayload parses a Slack interaction callback payload.
func ParseInteractionPayload(payload []byte) (*Interaction, error) {
	var callback slack.InteractionCallback
	if err := json.Unmarshal(payload, &callback); err != nil {
		return nil, fmt.Errorf("parse interaction payload: %w", err)
	}

	if len(callback.ActionCallback.BlockActions) == 0 {
		return nil, fmt.Errorf("no actions in interaction payload")
	}

	action := callback.ActionCallback.BlockActions[0]

	return &Interaction{
		Type:      InteractionButtonClick,
		ChannelID: callback.Channel.ID,
		ThreadTS:  callback.Message.ThreadTimestamp,
		MessageTS: callback.Message.Timestamp,
		UserID:    callback.User.ID,
		ActionID:  action.ActionID,
		Value:     action.Value,
	}, nil
}

// EmojiReactionEvent creates an Interaction from an emoji reaction.
func EmojiReactionEvent(channelID, threadTS, messageTS, userID, emoji string) Interaction {
	return Interaction{
		Type:      InteractionEmojiReaction,
		ChannelID: channelID,
		ThreadTS:  threadTS,
		MessageTS: messageTS,
		UserID:    userID,
		ActionID:  emoji,
		Value:     emoji,
	}
}

// IsStopSignal checks if an interaction is a stop signal (🛑 emoji).
func IsStopSignal(i Interaction) bool {
	return i.Type == InteractionEmojiReaction && i.Value == "octagonal_sign"
}

// IsApproveSignal checks if an interaction is an approval (👍 emoji or approve button).
func IsApproveSignal(i Interaction) bool {
	if i.Type == InteractionEmojiReaction && i.Value == "+1" {
		return true
	}
	if i.Type == InteractionButtonClick && (i.Value == "approve") {
		return true
	}
	return false
}

// InteractionRouter dispatches interactions to registered handlers.
type InteractionRouter struct {
	handlers map[string]InteractionHandler
	logger   *slog.Logger
}

// NewInteractionRouter creates a new interaction router.
func NewInteractionRouter(logger *slog.Logger) *InteractionRouter {
	return &InteractionRouter{
		handlers: make(map[string]InteractionHandler),
		logger:   logger,
	}
}

// Handle registers a handler for a specific action ID.
func (r *InteractionRouter) Handle(actionID string, handler InteractionHandler) {
	r.handlers[actionID] = handler
}

// Dispatch routes an interaction to the appropriate handler.
func (r *InteractionRouter) Dispatch(interaction Interaction) {
	handler, ok := r.handlers[interaction.ActionID]
	if !ok {
		r.logger.Warn("no handler for interaction",
			"action_id", interaction.ActionID,
			"type", interaction.Type,
		)
		return
	}
	handler(interaction)
}
