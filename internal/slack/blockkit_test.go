package slack

import (
	"log/slog"
	"testing"
)

func TestBlockKitMessage_BuildBlocks(t *testing.T) {
	msg := &BlockKitMessage{
		HeaderText: "Plan Review",
		BodyText:   "Here's my plan:\n- Step 1\n- Step 2",
		Buttons: []ButtonOption{
			{ActionID: "approve", Text: "Approve", Value: "approve", Style: "primary"},
			{ActionID: "reject", Text: "Reject", Value: "reject", Style: "danger"},
		},
	}

	blocks := msg.BuildBlocks()

	// Should have: header section + body section + action block
	if len(blocks) != 3 {
		t.Errorf("expected 3 blocks, got %d", len(blocks))
	}
}

func TestBlockKitMessage_BuildBlocks_NoButtons(t *testing.T) {
	msg := &BlockKitMessage{
		HeaderText: "Info",
		BodyText:   "Just informational",
	}

	blocks := msg.BuildBlocks()

	// Should have: header + body (no action block)
	if len(blocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestBlockKitMessage_BuildBlocks_HeaderOnly(t *testing.T) {
	msg := &BlockKitMessage{
		HeaderText: "Title",
	}

	blocks := msg.BuildBlocks()
	if len(blocks) != 1 {
		t.Errorf("expected 1 block, got %d", len(blocks))
	}
}

func TestBlockKitMessage_BuildFallbackText(t *testing.T) {
	msg := &BlockKitMessage{
		HeaderText: "Plan Review",
		BodyText:   "Here's the plan",
		Buttons: []ButtonOption{
			{ActionID: "approve", Text: "Approve", Value: "approve"},
			{ActionID: "reject", Text: "Reject", Value: "reject"},
		},
	}

	fallback := msg.BuildFallbackText()

	if fallback == "" {
		t.Error("expected non-empty fallback text")
	}
	// Should contain numbered options
	if !contains(fallback, "1. Approve") {
		t.Errorf("expected '1. Approve' in fallback, got %q", fallback)
	}
	if !contains(fallback, "2. Reject") {
		t.Errorf("expected '2. Reject' in fallback, got %q", fallback)
	}
}

func TestPlanApproval(t *testing.T) {
	msg := PlanApproval("Step 1: Read files\nStep 2: Write code")

	if msg.HeaderText != "Plan Review" {
		t.Errorf("expected header 'Plan Review', got %q", msg.HeaderText)
	}
	if len(msg.Buttons) != 3 {
		t.Errorf("expected 3 buttons, got %d", len(msg.Buttons))
	}

	// Check button IDs
	ids := make(map[string]bool)
	for _, btn := range msg.Buttons {
		ids[btn.ActionID] = true
	}
	for _, expected := range []string{"approve_plan", "modify_plan", "reject_plan"} {
		if !ids[expected] {
			t.Errorf("missing button with action ID %q", expected)
		}
	}
}

func TestDestructiveToolApproval(t *testing.T) {
	msg := DestructiveToolApproval("Bash", "rm -rf /tmp/test")

	if msg.HeaderText != "Destructive Action Approval" {
		t.Errorf("unexpected header: %q", msg.HeaderText)
	}
	if len(msg.Buttons) != 2 {
		t.Errorf("expected 2 buttons, got %d", len(msg.Buttons))
	}
}

func TestEmojiReactionEvent(t *testing.T) {
	evt := EmojiReactionEvent("C123", "T456", "M789", "U001", "octagonal_sign")

	if evt.Type != InteractionEmojiReaction {
		t.Errorf("expected type EmojiReaction, got %v", evt.Type)
	}
	if evt.ChannelID != "C123" {
		t.Errorf("expected channel C123, got %q", evt.ChannelID)
	}
	if evt.Value != "octagonal_sign" {
		t.Errorf("expected value octagonal_sign, got %q", evt.Value)
	}
}

func TestIsStopSignal(t *testing.T) {
	stop := EmojiReactionEvent("", "", "", "", "octagonal_sign")
	if !IsStopSignal(stop) {
		t.Error("expected octagonal_sign to be a stop signal")
	}

	notStop := EmojiReactionEvent("", "", "", "", "+1")
	if IsStopSignal(notStop) {
		t.Error("expected +1 to not be a stop signal")
	}

	// Button click is never a stop signal
	btn := Interaction{Type: InteractionButtonClick, Value: "octagonal_sign"}
	if IsStopSignal(btn) {
		t.Error("expected button click to not be a stop signal")
	}
}

func TestIsApproveSignal(t *testing.T) {
	// Emoji approval
	thumbsUp := EmojiReactionEvent("", "", "", "", "+1")
	if !IsApproveSignal(thumbsUp) {
		t.Error("expected +1 to be an approve signal")
	}

	// Button approval
	btn := Interaction{Type: InteractionButtonClick, Value: "approve"}
	if !IsApproveSignal(btn) {
		t.Error("expected approve button to be an approve signal")
	}

	// Not an approval
	reject := Interaction{Type: InteractionButtonClick, Value: "reject"}
	if IsApproveSignal(reject) {
		t.Error("expected reject to not be an approve signal")
	}
}

func TestInteractionRouter_Dispatch(t *testing.T) {
	logger := slog.Default()
	router := NewInteractionRouter(logger)

	var received *Interaction
	router.Handle("approve_plan", func(i Interaction) {
		received = &i
	})

	router.Dispatch(Interaction{
		Type:     InteractionButtonClick,
		ActionID: "approve_plan",
		Value:    "approve",
		UserID:   "U123",
	})

	if received == nil {
		t.Error("expected handler to be called")
	}
	if received.Value != "approve" {
		t.Errorf("expected value 'approve', got %q", received.Value)
	}
}

func TestInteractionRouter_UnhandledAction(t *testing.T) {
	logger := slog.Default()
	router := NewInteractionRouter(logger)

	// Should not panic on unhandled action
	router.Dispatch(Interaction{
		Type:     InteractionButtonClick,
		ActionID: "unknown_action",
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
