package access

import (
	"testing"

	"github.com/leandrotocalini/CodeButler/internal/config"
	"github.com/leandrotocalini/CodeButler/internal/whatsapp"
)

func TestIsAllowed(t *testing.T) {
	cfg := &config.Config{
		WhatsApp: config.WhatsAppConfig{
			GroupJID: "120363405395407771@g.us",
		},
	}

	tests := []struct {
		name     string
		msg      whatsapp.Message
		expected bool
	}{
		{
			name: "message from correct group",
			msg: whatsapp.Message{
				Chat:    "120363405395407771@g.us",
				IsGroup: true,
				Content: "test message",
			},
			expected: true,
		},
		{
			name: "message from different group",
			msg: whatsapp.Message{
				Chat:    "120363999999999999@g.us",
				IsGroup: true,
				Content: "test message",
			},
			expected: false,
		},
		{
			name: "message from personal chat",
			msg: whatsapp.Message{
				Chat:    "5491134567890@s.whatsapp.net",
				IsGroup: false,
				Content: "test message",
			},
			expected: false,
		},
		{
			name: "message when no group configured",
			msg: whatsapp.Message{
				Chat:    "120363405395407771@g.us",
				IsGroup: true,
				Content: "test message",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCfg := cfg
			if tt.name == "message when no group configured" {
				testCfg = &config.Config{
					WhatsApp: config.WhatsAppConfig{
						GroupJID: "",
					},
				}
			}

			result := IsAllowed(tt.msg, testCfg)
			if result != tt.expected {
				t.Errorf("IsAllowed() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsAllowedMultipleGroups(t *testing.T) {
	cfg := &config.Config{
		WhatsApp: config.WhatsAppConfig{
			GroupJID: "120363405395407771@g.us",
		},
	}

	testGroups := []string{
		"120363405395407771@g.us",
		"120363111111111111@g.us",
		"120363222222222222@g.us",
		"120363333333333333@g.us",
	}

	for _, groupJID := range testGroups {
		msg := whatsapp.Message{
			Chat:    groupJID,
			IsGroup: true,
			Content: "test",
		}

		result := IsAllowed(msg, cfg)
		expected := (groupJID == cfg.WhatsApp.GroupJID)

		if result != expected {
			t.Errorf("Group %s: IsAllowed() = %v, expected %v", groupJID, result, expected)
		}
	}
}
