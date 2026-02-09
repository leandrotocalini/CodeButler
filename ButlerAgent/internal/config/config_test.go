package config

import (
	"os"
	"testing"
)

func TestLoadAndSave(t *testing.T) {
	testConfig := &Config{
		WhatsApp: WhatsAppConfig{
			SessionPath:    "./test-session",
			PersonalNumber: "1234567890@s.whatsapp.net",
			GroupJID:       "120363123456789012@g.us",
			GroupName:      "Test Group",
		},
		OpenAI: OpenAIConfig{
			APIKey: "sk-test",
		},
		Claude: ClaudeConfig{
			OAuthToken: "test-token",
		},
		Sources: SourcesConfig{
			RootPath: "./test-sources",
		},
	}

	testFile := "test-config.json"
	defer os.Remove(testFile)

	if err := Save(testConfig, testFile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(testFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.WhatsApp.GroupName != testConfig.WhatsApp.GroupName {
		t.Errorf("Expected GroupName %s, got %s", testConfig.WhatsApp.GroupName, loaded.WhatsApp.GroupName)
	}

	if loaded.OpenAI.APIKey != testConfig.OpenAI.APIKey {
		t.Errorf("Expected APIKey %s, got %s", testConfig.OpenAI.APIKey, loaded.OpenAI.APIKey)
	}

	if loaded.Sources.RootPath != testConfig.Sources.RootPath {
		t.Errorf("Expected RootPath %s, got %s", testConfig.Sources.RootPath, loaded.Sources.RootPath)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				WhatsApp: WhatsAppConfig{
					SessionPath: "./session",
					GroupJID:    "123@g.us",
					GroupName:   "Test",
				},
				OpenAI: OpenAIConfig{
					APIKey: "sk-test",
				},
				Sources: SourcesConfig{
					RootPath: "./sources",
				},
			},
			expectError: false,
		},
		{
			name: "missing session path",
			config: Config{
				WhatsApp: WhatsAppConfig{
					GroupJID:  "123@g.us",
					GroupName: "Test",
				},
				OpenAI: OpenAIConfig{
					APIKey: "sk-test",
				},
				Sources: SourcesConfig{
					RootPath: "./sources",
				},
			},
			expectError: true,
		},
		{
			name: "missing group JID - should pass",
			config: Config{
				WhatsApp: WhatsAppConfig{
					SessionPath: "./session",
					GroupName:   "Test",
				},
				OpenAI: OpenAIConfig{
					APIKey: "sk-test",
				},
				Sources: SourcesConfig{
					RootPath: "./sources",
				},
			},
			expectError: false,
		},
		{
			name: "missing OpenAI key - should pass",
			config: Config{
				WhatsApp: WhatsAppConfig{
					SessionPath: "./session",
					GroupJID:    "123@g.us",
					GroupName:   "Test",
				},
				Sources: SourcesConfig{
					RootPath: "./sources",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(&tt.config)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestLoadOAuthFromEnv(t *testing.T) {
	testConfig := &Config{
		WhatsApp: WhatsAppConfig{
			SessionPath: "./test-session",
			GroupJID:    "123@g.us",
			GroupName:   "Test",
		},
		OpenAI: OpenAIConfig{
			APIKey: "sk-test",
		},
		Sources: SourcesConfig{
			RootPath: "./test-sources",
		},
	}

	testFile := "test-oauth-config.json"
	defer os.Remove(testFile)

	if err := Save(testConfig, testFile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	os.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "env-token")
	defer os.Unsetenv("CLAUDE_CODE_OAUTH_TOKEN")

	loaded, err := Load(testFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Claude.OAuthToken != "env-token" {
		t.Errorf("Expected OAuth token from env 'env-token', got '%s'", loaded.Claude.OAuthToken)
	}
}
