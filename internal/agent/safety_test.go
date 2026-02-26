package agent

import "testing"

func TestProgressTracker_DetectSameToolParams(t *testing.T) {
	pt := NewProgressTracker()

	// Record 2 identical calls — not yet stuck
	pt.RecordToolCall("Read", `{"path":"main.go"}`)
	pt.RecordToolCall("Read", `{"path":"main.go"}`)
	if signal := pt.Detect(); signal != SignalNone {
		t.Errorf("expected SignalNone after 2 calls, got %v", signal)
	}

	// 3rd identical call — stuck
	pt.RecordToolCall("Read", `{"path":"main.go"}`)
	if signal := pt.Detect(); signal != SignalSameToolParams {
		t.Errorf("expected SignalSameToolParams after 3 calls, got %v", signal)
	}
}

func TestProgressTracker_DetectSameToolParams_DifferentArgs(t *testing.T) {
	pt := NewProgressTracker()

	pt.RecordToolCall("Read", `{"path":"a.go"}`)
	pt.RecordToolCall("Read", `{"path":"b.go"}`)
	pt.RecordToolCall("Read", `{"path":"c.go"}`)
	if signal := pt.Detect(); signal != SignalNone {
		t.Errorf("expected SignalNone for different args, got %v", signal)
	}
}

func TestProgressTracker_DetectSameError(t *testing.T) {
	pt := NewProgressTracker()

	pt.RecordError("file not found: /tmp/missing.go")
	pt.RecordError("file not found: /tmp/missing.go")
	if signal := pt.Detect(); signal != SignalNone {
		t.Errorf("expected SignalNone after 2 errors, got %v", signal)
	}

	pt.RecordError("file not found: /tmp/missing.go")
	if signal := pt.Detect(); signal != SignalSameError {
		t.Errorf("expected SignalSameError after 3 errors, got %v", signal)
	}
}

func TestProgressTracker_DetectSameError_DifferentErrors(t *testing.T) {
	pt := NewProgressTracker()

	pt.RecordError("error A")
	pt.RecordError("error B")
	pt.RecordError("error C")
	if signal := pt.Detect(); signal != SignalNone {
		t.Errorf("expected SignalNone for different errors, got %v", signal)
	}
}

func TestProgressTracker_DetectNoProgress(t *testing.T) {
	pt := NewProgressTracker()

	pt.RecordResponse("I'll try again.")
	pt.RecordResponse("I'll try again.")
	if signal := pt.Detect(); signal != SignalNone {
		t.Errorf("expected SignalNone after 2 responses, got %v", signal)
	}

	pt.RecordResponse("I'll try again.")
	if signal := pt.Detect(); signal != SignalNoProgress {
		t.Errorf("expected SignalNoProgress after 3 identical responses, got %v", signal)
	}
}

func TestProgressTracker_SameToolParamsHigherPriority(t *testing.T) {
	pt := NewProgressTracker()

	// Record both signals simultaneously
	pt.RecordToolCall("Read", `{"path":"x"}`)
	pt.RecordToolCall("Read", `{"path":"x"}`)
	pt.RecordToolCall("Read", `{"path":"x"}`)
	pt.RecordError("same error")
	pt.RecordError("same error")
	pt.RecordError("same error")

	// SameToolParams should be detected first (higher priority)
	if signal := pt.Detect(); signal != SignalSameToolParams {
		t.Errorf("expected SignalSameToolParams (higher priority), got %v", signal)
	}
}

func TestProgressTracker_WindowBounded(t *testing.T) {
	pt := NewProgressTracker()

	// Fill window with different calls
	pt.RecordToolCall("Read", `{"path":"a"}`)
	pt.RecordToolCall("Read", `{"path":"b"}`)
	pt.RecordToolCall("Read", `{"path":"c"}`)
	pt.RecordToolCall("Read", `{"path":"d"}`)
	pt.RecordToolCall("Read", `{"path":"e"}`)

	// Window is full (5 items). Now add 3 identical.
	pt.RecordToolCall("Write", `{"path":"x"}`)
	pt.RecordToolCall("Write", `{"path":"x"}`)
	pt.RecordToolCall("Write", `{"path":"x"}`)

	// Should detect stuck even though earlier entries were different
	if signal := pt.Detect(); signal != SignalSameToolParams {
		t.Errorf("expected SignalSameToolParams, got %v", signal)
	}
}

func TestProgressTracker_EscapeStrategies(t *testing.T) {
	pt := NewProgressTracker()

	// First detection → reflection
	action := pt.NextEscapeAction(SignalSameToolParams)
	if action != EscapeReflection {
		t.Errorf("expected EscapeReflection, got %v", action)
	}

	// Second call within turnsPerStrategy → still reflection
	action = pt.NextEscapeAction(SignalSameToolParams)
	if action != EscapeReflection {
		t.Errorf("expected EscapeReflection (2nd turn), got %v", action)
	}

	// Third call → exhausted reflection, escalate to force reasoning
	action = pt.NextEscapeAction(SignalSameToolParams)
	if action != EscapeForceReasoning {
		t.Errorf("expected EscapeForceReasoning, got %v", action)
	}

	// Continue escalation
	pt.NextEscapeAction(SignalSameToolParams) // 2nd turn of force reasoning
	action = pt.NextEscapeAction(SignalSameToolParams)
	if action != EscapeReduceTools {
		t.Errorf("expected EscapeReduceTools, got %v", action)
	}

	pt.NextEscapeAction(SignalSameToolParams) // 2nd turn of reduce tools
	action = pt.NextEscapeAction(SignalSameToolParams)
	if action != EscapeEscalate {
		t.Errorf("expected EscapeEscalate, got %v", action)
	}
}

func TestProgressTracker_ResetEscape(t *testing.T) {
	pt := NewProgressTracker()

	// Enter escape mode
	pt.NextEscapeAction(SignalSameToolParams)
	if !pt.IsEscaping() {
		t.Error("expected IsEscaping() to be true")
	}

	// Reset
	pt.ResetEscape()
	if pt.IsEscaping() {
		t.Error("expected IsEscaping() to be false after reset")
	}
	if pt.CurrentEscapeLevel() != EscapeNone {
		t.Errorf("expected EscapeNone after reset, got %v", pt.CurrentEscapeLevel())
	}
}

func TestProgressTracker_StuckTool(t *testing.T) {
	pt := NewProgressTracker()

	pt.SetStuckTool("Read")
	if pt.StuckTool() != "Read" {
		t.Errorf("expected stuck tool 'Read', got %q", pt.StuckTool())
	}

	pt.AddRemovedTool("Read")
	removed := pt.RemovedTools()
	if len(removed) != 1 || removed[0] != "Read" {
		t.Errorf("expected removed tools [Read], got %v", removed)
	}

	pt.ResetEscape()
	if pt.RemovedTools() != nil {
		t.Error("expected nil removed tools after reset")
	}
}

func TestReflectionPrompt(t *testing.T) {
	prompt := ReflectionPrompt("You've called Read 3 times.")
	if prompt == "" {
		t.Error("expected non-empty reflection prompt")
	}
}

func TestForceReasoningPrompt(t *testing.T) {
	prompt := ForceReasoningPrompt()
	if prompt == "" {
		t.Error("expected non-empty force reasoning prompt")
	}
}

func TestEscalationMessage(t *testing.T) {
	tests := []struct {
		role     string
		wantTarget string
	}{
		{"coder", "@codebutler.pm"},
		{"pm", "the user"},
		{"lead", "the user"},
		{"reviewer", "@codebutler.pm"},
	}

	for _, tt := range tests {
		msg := EscalationMessage(tt.role, "tried X three times")
		if msg == "" {
			t.Errorf("expected non-empty escalation message for role %s", tt.role)
		}
	}
}

func TestStuckSignal_String(t *testing.T) {
	tests := []struct {
		signal StuckSignal
		want   string
	}{
		{SignalNone, "none"},
		{SignalSameToolParams, "same_tool_params"},
		{SignalSameError, "same_error"},
		{SignalNoProgress, "no_progress"},
		{StuckSignal(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.signal.String(); got != tt.want {
			t.Errorf("StuckSignal(%d).String() = %q, want %q", tt.signal, got, tt.want)
		}
	}
}

func TestHashToolCall_Deterministic(t *testing.T) {
	h1 := hashToolCall("Read", `{"path":"main.go"}`)
	h2 := hashToolCall("Read", `{"path":"main.go"}`)
	if h1 != h2 {
		t.Errorf("expected deterministic hash, got %q and %q", h1, h2)
	}

	h3 := hashToolCall("Read", `{"path":"other.go"}`)
	if h1 == h3 {
		t.Errorf("expected different hash for different args, both got %q", h1)
	}
}

func TestAppendBounded(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		item  string
		max   int
		want  int
	}{
		{"under limit", []string{"a", "b"}, "c", 5, 3},
		{"at limit", []string{"a", "b", "c", "d"}, "e", 5, 5},
		{"over limit", []string{"a", "b", "c", "d", "e"}, "f", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendBounded(tt.items, tt.item, tt.max)
			if len(result) != tt.want {
				t.Errorf("expected len %d, got %d", tt.want, len(result))
			}
			if result[len(result)-1] != tt.item {
				t.Errorf("expected last item %q, got %q", tt.item, result[len(result)-1])
			}
		})
	}
}

func TestHasRepeatedTail(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		count int
		want  bool
	}{
		{"empty", nil, 3, false},
		{"too few", []string{"a", "a"}, 3, false},
		{"all same", []string{"a", "a", "a"}, 3, true},
		{"tail same", []string{"b", "a", "a", "a"}, 3, true},
		{"not all same", []string{"a", "b", "a"}, 3, false},
		{"exactly one", []string{"a"}, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasRepeatedTail(tt.items, tt.count)
			if got != tt.want {
				t.Errorf("hasRepeatedTail(%v, %d) = %v, want %v", tt.items, tt.count, got, tt.want)
			}
		})
	}
}
