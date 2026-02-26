package slack

import (
	"testing"
)

func TestDefaultIdentities(t *testing.T) {
	ids := DefaultIdentities()

	expectedRoles := []string{"pm", "coder", "reviewer", "researcher", "artist", "lead"}
	for _, role := range expectedRoles {
		id, ok := ids[role]
		if !ok {
			t.Errorf("missing identity for role %q", role)
			continue
		}
		if id.Role != role {
			t.Errorf("expected role %q, got %q", role, id.Role)
		}
		if id.DisplayName == "" {
			t.Errorf("expected non-empty display name for role %q", role)
		}
		if id.IconEmoji == "" {
			t.Errorf("expected non-empty icon emoji for role %q", role)
		}
	}
}

func TestAgentIdentity_DisplayName(t *testing.T) {
	ids := DefaultIdentities()

	tests := []struct {
		role     string
		wantName string
	}{
		{"pm", "codebutler.pm"},
		{"coder", "codebutler.coder"},
		{"reviewer", "codebutler.reviewer"},
		{"researcher", "codebutler.researcher"},
		{"artist", "codebutler.artist"},
		{"lead", "codebutler.lead"},
	}

	for _, tt := range tests {
		id := ids[tt.role]
		if id.DisplayName != tt.wantName {
			t.Errorf("role %q: expected display name %q, got %q", tt.role, tt.wantName, id.DisplayName)
		}
	}
}

func TestCodeSnippetThreshold(t *testing.T) {
	if codeSnippetThreshold != 20 {
		t.Errorf("expected code snippet threshold 20, got %d", codeSnippetThreshold)
	}
}

func TestMessageEvent_Fields(t *testing.T) {
	evt := MessageEvent{
		EventID:   "Ev123",
		ChannelID: "C456",
		ThreadTS:  "1234567890.123456",
		MessageTS: "1234567890.123456",
		UserID:    "U789",
		Text:      "Hello @codebutler.coder please help",
		BotID:     "",
	}

	if evt.EventID != "Ev123" {
		t.Errorf("expected EventID %q, got %q", "Ev123", evt.EventID)
	}
	if evt.ChannelID != "C456" {
		t.Errorf("expected ChannelID %q, got %q", "C456", evt.ChannelID)
	}
	if evt.ThreadTS != "1234567890.123456" {
		t.Errorf("expected ThreadTS %q, got %q", "1234567890.123456", evt.ThreadTS)
	}
	if evt.UserID != "U789" {
		t.Errorf("expected UserID %q, got %q", "U789", evt.UserID)
	}
}
