package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestRouter_Dispatch(t *testing.T) {
	router := NewRouter()

	var called bool
	router.Register(&Command{
		Name:        "test",
		Description: "Test command",
		Run: func(args []string) error {
			called = true
			return nil
		},
	})

	if err := router.Dispatch([]string{"test"}); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if !called {
		t.Error("command not called")
	}
}

func TestRouter_Dispatch_WithArgs(t *testing.T) {
	router := NewRouter()

	var receivedArgs []string
	router.Register(&Command{
		Name: "echo",
		Run: func(args []string) error {
			receivedArgs = args
			return nil
		},
	})

	router.Dispatch([]string{"echo", "hello", "world"})

	if len(receivedArgs) != 2 {
		t.Fatalf("expected 2 args, got %d", len(receivedArgs))
	}
	if receivedArgs[0] != "hello" || receivedArgs[1] != "world" {
		t.Errorf("args: got %v", receivedArgs)
	}
}

func TestRouter_Dispatch_Unknown(t *testing.T) {
	router := NewRouter()

	err := router.Dispatch([]string{"nonexistent"})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestRouter_Dispatch_Empty(t *testing.T) {
	router := NewRouter()

	err := router.Dispatch([]string{})
	if err == nil {
		t.Error("expected error for empty args")
	}
}

func TestRouter_HasCommand(t *testing.T) {
	router := NewRouter()
	router.Register(&Command{Name: "init", Run: func([]string) error { return nil }})

	if !router.HasCommand("init") {
		t.Error("expected init command")
	}
	if router.HasCommand("nonexistent") {
		t.Error("unexpected command")
	}
}

func TestRouter_Usage(t *testing.T) {
	router := NewRouter()
	router.Register(&Command{Name: "init", Description: "Initialize project", Run: func([]string) error { return nil }})
	router.Register(&Command{Name: "start", Description: "Start agents", Run: func([]string) error { return nil }})

	usage := router.Usage()
	if !strings.Contains(usage, "init") {
		t.Error("usage should contain init")
	}
	if !strings.Contains(usage, "start") {
		t.Error("usage should contain start")
	}
}

func TestRouter_CommandError(t *testing.T) {
	router := NewRouter()
	router.Register(&Command{
		Name: "fail",
		Run: func([]string) error {
			return fmt.Errorf("command failed")
		},
	})

	err := router.Dispatch([]string{"fail"})
	if err == nil {
		t.Error("expected error")
	}
	if err.Error() != "command failed" {
		t.Errorf("wrong error: %v", err)
	}
}

func TestFormatStatus(t *testing.T) {
	statuses := []ServiceStatus{
		{Role: "pm", Running: true, PID: 1234},
		{Role: "coder", Running: false},
		{Role: "reviewer", Running: false, Error: "not installed"},
	}

	output := FormatStatus(statuses)
	if !strings.Contains(output, "pm") {
		t.Error("should contain pm")
	}
	if !strings.Contains(output, "running") {
		t.Error("should contain running")
	}
	if !strings.Contains(output, "stopped") {
		t.Error("should contain stopped")
	}
	if !strings.Contains(output, "not installed") {
		t.Error("should contain error message")
	}
}

func TestServiceManager_ServiceLabel(t *testing.T) {
	sm := NewServiceManager("/usr/local/bin/codebutler", "/home/user/project")
	if sm.serviceLabel("pm") != "com.codebutler.pm" {
		t.Errorf("label: got %q", sm.serviceLabel("pm"))
	}
}

func TestServiceManager_UnitName(t *testing.T) {
	sm := NewServiceManager("/usr/local/bin/codebutler", "/home/user/project")
	if sm.unitName("coder") != "codebutler-coder" {
		t.Errorf("unit: got %q", sm.unitName("coder"))
	}
}

func TestAgentRoles(t *testing.T) {
	if len(AgentRoles) != 6 {
		t.Errorf("expected 6 roles, got %d", len(AgentRoles))
	}
	expected := map[string]bool{
		"pm": true, "coder": true, "reviewer": true,
		"researcher": true, "artist": true, "lead": true,
	}
	for _, role := range AgentRoles {
		if !expected[role] {
			t.Errorf("unexpected role: %s", role)
		}
	}
}

func TestRouter_ListCommands(t *testing.T) {
	router := NewRouter()
	router.Register(&Command{Name: "init", Description: "Init", Run: func([]string) error { return nil }})
	router.Register(&Command{Name: "start", Description: "Start", Run: func([]string) error { return nil }})

	cmds := router.ListCommands()
	if len(cmds) != 2 {
		t.Errorf("expected 2 commands, got %d", len(cmds))
	}
}
