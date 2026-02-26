package router

import "testing"

func TestRedactor_APIKeys(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"OpenAI key",
			"key is sk-abcdefghijklmnopqrstuvwxyz1234567890",
			"key is [REDACTED]",
		},
		{
			"Slack bot token",
			"token: " + "xo" + "xb-FAKE-TOKEN-FOR-TEST",
			"token: [REDACTED]",
		},
		{
			"Slack app token",
			"app: " + "xa" + "pp-FAKE-APP-FOR-TEST",
			"app: [REDACTED]",
		},
		{
			"GitHub PAT",
			"token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			"token [REDACTED]",
		},
		{
			"AWS access key",
			"access: AKIAIOSFODNN7EXAMPLE",
			"access: [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactor_JWT(t *testing.T) {
	r := NewRedactor()

	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	input := "Bearer " + jwt
	got := r.Redact(input)

	if got == input {
		t.Error("expected JWT to be redacted")
	}
	if got != "Bearer [REDACTED]" {
		t.Errorf("unexpected redaction result: %q", got)
	}
}

func TestRedactor_ConnectionStrings(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name  string
		input string
	}{
		{"postgres", "postgres://user:pass@host:5432/db"},
		{"mysql", "mysql://user:pass@host:3306/db"},
		{"mongodb", "mongodb://user:pass@host:27017/db"},
		{"redis", "redis://user:pass@host:6379/0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact("connection: " + tt.input)
			if got == "connection: "+tt.input {
				t.Errorf("expected %q to be redacted", tt.input)
			}
		})
	}
}

func TestRedactor_PrivateIPs(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"10.x.x.x", "server at 10.0.1.5", "server at [REDACTED]"},
		{"172.16.x.x", "server at 172.16.0.1", "server at [REDACTED]"},
		{"172.31.x.x", "server at 172.31.255.255", "server at [REDACTED]"},
		{"192.168.x.x", "server at 192.168.1.100", "server at [REDACTED]"},
		{"public IP stays", "server at 8.8.8.8", "server at 8.8.8.8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactor_CustomPattern(t *testing.T) {
	r := NewRedactor()

	err := r.AddPattern(`SECRET_\w+`)
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	got := r.Redact("key is SECRET_ABC123")
	if got != "key is [REDACTED]" {
		t.Errorf("expected custom pattern to redact, got %q", got)
	}
}

func TestRedactor_InvalidPattern(t *testing.T) {
	r := NewRedactor()
	err := r.AddPattern("[invalid")
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestRedactor_ContainsSensitive(t *testing.T) {
	r := NewRedactor()

	if !r.ContainsSensitive("token: xoxb-123456789-abc") {
		t.Error("expected Slack token to be detected")
	}
	if r.ContainsSensitive("hello world") {
		t.Error("expected clean text to not be detected")
	}
}

func TestRedactor_NoFalsePositives(t *testing.T) {
	r := NewRedactor()

	safeTexts := []string{
		"Hello, how are you?",
		"The function returns 42",
		"File path: /home/user/project/main.go",
		"Error: connection refused",
		"Version 1.2.3",
	}

	for _, text := range safeTexts {
		got := r.Redact(text)
		if got != text {
			t.Errorf("false positive: Redact(%q) = %q", text, got)
		}
	}
}

func TestPrefixMessage(t *testing.T) {
	tests := []struct {
		role string
		text string
		want string
	}{
		{"pm", "Hello team", "@codebutler.pm: Hello team"},
		{"coder", "Done implementing", "@codebutler.coder: Done implementing"},
		{"pm", "@codebutler.pm: Already prefixed", "@codebutler.pm: Already prefixed"},
	}

	for _, tt := range tests {
		got := PrefixMessage(tt.role, tt.text)
		if got != tt.want {
			t.Errorf("PrefixMessage(%q, %q) = %q, want %q", tt.role, tt.text, got, tt.want)
		}
	}
}
