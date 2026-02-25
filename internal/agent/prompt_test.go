package agent

import "testing"

func TestBuildSystemPrompt(t *testing.T) {
	tests := []struct {
		name      string
		seed      string
		global    string
		workflows string
		isPM      bool
		want      string
	}{
		{
			name: "seed only",
			seed: "You are a coder.",
			want: "You are a coder.",
		},
		{
			name:   "seed + global",
			seed:   "You are a coder.",
			global: "Project uses Go.",
			want:   "You are a coder.\n\nProject uses Go.",
		},
		{
			name:      "PM includes workflows",
			seed:      "You are PM.",
			global:    "Project info.",
			workflows: "Available workflows: implement, bugfix.",
			isPM:      true,
			want:      "You are PM.\n\nProject info.\n\nAvailable workflows: implement, bugfix.",
		},
		{
			name:      "non-PM excludes workflows",
			seed:      "You are a coder.",
			global:    "Project info.",
			workflows: "Available workflows: implement, bugfix.",
			isPM:      false,
			want:      "You are a coder.\n\nProject info.",
		},
		{
			name:   "empty global skipped",
			seed:   "You are a coder.",
			global: "",
			want:   "You are a coder.",
		},
		{
			name:      "PM with empty workflows",
			seed:      "You are PM.",
			global:    "Info.",
			workflows: "",
			isPM:      true,
			want:      "You are PM.\n\nInfo.",
		},
		{
			name: "all empty",
			want: "",
		},
		{
			name:   "global only",
			global: "Shared knowledge.",
			want:   "Shared knowledge.",
		},
		{
			name:      "PM with all components",
			seed:      "Identity.",
			global:    "Global.",
			workflows: "Workflows.",
			isPM:      true,
			want:      "Identity.\n\nGlobal.\n\nWorkflows.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSystemPrompt(tt.seed, tt.global, tt.workflows, tt.isPM)
			if got != tt.want {
				t.Errorf("BuildSystemPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}
