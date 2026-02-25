package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestBashTool_Execute(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewBashTool(sb)

	tests := []struct {
		name      string
		args      bashArgs
		wantSub   string // substring expected in output
		wantError bool
	}{
		{
			name:    "simple echo",
			args:    bashArgs{Command: "echo hello"},
			wantSub: "hello",
		},
		{
			name:    "pwd shows sandbox root",
			args:    bashArgs{Command: "pwd"},
			wantSub: root,
		},
		{
			name:      "failing command",
			args:      bashArgs{Command: "false"},
			wantError: true,
		},
		{
			name:      "empty command",
			args:      bashArgs{Command: ""},
			wantError: true,
		},
		{
			name:      "destructive command blocked",
			args:      bashArgs{Command: "rm -rf /"},
			wantError: true,
		},
		{
			name:      "sudo blocked",
			args:      bashArgs{Command: "sudo ls"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)
			call := ToolCall{ID: "bash-1", Name: "Bash", Arguments: argsJSON}

			result, _ := tool.Execute(context.Background(), call)

			if tt.wantError {
				if !result.IsError {
					t.Errorf("expected error, got: %q", result.Content)
				}
				return
			}

			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}
			if tt.wantSub != "" && !containsStr(result.Content, tt.wantSub) {
				t.Errorf("output %q does not contain %q", result.Content, tt.wantSub)
			}
		})
	}
}

func TestBashTool_Timeout(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewBashTool(sb)

	timeout := 1
	argsJSON, _ := json.Marshal(bashArgs{Command: "sleep 10", Timeout: &timeout})
	call := ToolCall{ID: "bash-timeout", Name: "Bash", Arguments: argsJSON}

	result, _ := tool.Execute(context.Background(), call)

	if !result.IsError {
		t.Error("expected timeout error")
	}
	if !containsStr(result.Content, "timed out") {
		t.Errorf("error should mention timeout, got: %q", result.Content)
	}
}

func TestBashTool_CapturesStderr(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewBashTool(sb)

	argsJSON, _ := json.Marshal(bashArgs{Command: "echo error >&2"})
	call := ToolCall{ID: "bash-stderr", Name: "Bash", Arguments: argsJSON}

	result, _ := tool.Execute(context.Background(), call)

	if !containsStr(result.Content, "error") {
		t.Errorf("should capture stderr, got: %q", result.Content)
	}
}

func TestBashTool_ContextCancellation(t *testing.T) {
	root := t.TempDir()
	sb, _ := NewSandbox(root)
	tool := NewBashTool(sb)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	argsJSON, _ := json.Marshal(bashArgs{Command: "sleep 60"})
	call := ToolCall{ID: "bash-cancel", Name: "Bash", Arguments: argsJSON}

	result, _ := tool.Execute(ctx, call)

	if !result.IsError {
		t.Error("expected error from cancelled context")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
