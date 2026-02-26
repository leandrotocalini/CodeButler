package router

import (
	"testing"
)

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"no mentions", "hello world", nil},
		{"single mention", "@codebutler.coder please help", []string{"coder"}},
		{"multiple mentions", "@codebutler.coder @codebutler.reviewer look at this", []string{"coder", "reviewer"}},
		{"PM mention", "@codebutler.pm implement feature", []string{"pm"}},
		{"mention in middle", "hey @codebutler.lead can you help?", []string{"lead"}},
		{"duplicate mentions", "@codebutler.coder and @codebutler.coder again", []string{"coder", "coder"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMentions(tt.text)
			if len(got) != len(tt.want) {
				t.Errorf("ExtractMentions(%q) = %v, want %v", tt.text, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mention[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHasMention(t *testing.T) {
	tests := []struct {
		text string
		role string
		want bool
	}{
		{"@codebutler.coder help", "coder", true},
		{"@codebutler.coder help", "pm", false},
		{"hello world", "pm", false},
		{"@codebutler.pm @codebutler.coder", "coder", true},
		{"@codebutler.pm @codebutler.coder", "pm", true},
	}

	for _, tt := range tests {
		got := HasMention(tt.text, tt.role)
		if got != tt.want {
			t.Errorf("HasMention(%q, %q) = %v, want %v", tt.text, tt.role, got, tt.want)
		}
	}
}

func TestHasAnyMention(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"@codebutler.coder help", true},
		{"hello world", false},
		{"some text @codebutler.pm more text", true},
		{"email@codebutler.pm", true}, // still matches the pattern
	}

	for _, tt := range tests {
		got := HasAnyMention(tt.text)
		if got != tt.want {
			t.Errorf("HasAnyMention(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		name string
		role string
		text string
		want bool
	}{
		// PM routing rules
		{"PM gets direct mention", "pm", "@codebutler.pm plan this feature", true},
		{"PM gets unmentioned messages", "pm", "implement a new feature", true},
		{"PM skips messages to other agents", "pm", "@codebutler.coder implement this", false},
		{"PM gets mixed mention", "pm", "@codebutler.pm @codebutler.coder do this", true},

		// Other agent routing rules
		{"Coder gets its mention", "coder", "@codebutler.coder implement this", true},
		{"Coder ignores PM messages", "coder", "@codebutler.pm plan this", false},
		{"Coder ignores unmentioned", "coder", "implement a feature", false},
		{"Reviewer gets its mention", "reviewer", "@codebutler.reviewer review PR", true},
		{"Reviewer ignores unmentioned", "reviewer", "review the code", false},
		{"Lead gets its mention", "lead", "@codebutler.lead run retro", true},
		{"Researcher gets its mention", "researcher", "@codebutler.researcher look up X", true},
		{"Artist gets its mention", "artist", "@codebutler.artist design UI", true},

		// Multi-agent messages
		{"Both coder and reviewer", "coder", "@codebutler.coder @codebutler.reviewer", true},
		{"Both coder and reviewer (reviewer)", "reviewer", "@codebutler.coder @codebutler.reviewer", true},
		{"Lead not mentioned", "lead", "@codebutler.coder @codebutler.reviewer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldProcess(tt.role, tt.text)
			if got != tt.want {
				t.Errorf("ShouldProcess(%q, %q) = %v, want %v", tt.role, tt.text, got, tt.want)
			}
		})
	}
}
