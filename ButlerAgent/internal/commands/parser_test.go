package commands

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType CommandType
		expectedArgs []string
	}{
		{
			name:         "help command",
			input:        "@codebutler help",
			expectedType: CommandHelp,
			expectedArgs: []string{},
		},
		{
			name:         "help shorthand",
			input:        "@codebutler h",
			expectedType: CommandHelp,
			expectedArgs: []string{},
		},
		{
			name:         "empty command",
			input:        "@codebutler",
			expectedType: CommandHelp,
			expectedArgs: []string{},
		},
		{
			name:         "repos command",
			input:        "@codebutler repos",
			expectedType: CommandRepos,
			expectedArgs: []string{},
		},
		{
			name:         "list alias",
			input:        "@codebutler list",
			expectedType: CommandRepos,
			expectedArgs: []string{},
		},
		{
			name:         "use command",
			input:        "@codebutler use aurum",
			expectedType: CommandUse,
			expectedArgs: []string{"aurum"},
		},
		{
			name:         "run command",
			input:        "@codebutler run add a new function",
			expectedType: CommandRun,
			expectedArgs: []string{"add", "a", "new", "function"},
		},
		{
			name:         "status command",
			input:        "@codebutler status",
			expectedType: CommandStatus,
			expectedArgs: []string{},
		},
		{
			name:         "clear command",
			input:        "@codebutler clear",
			expectedType: CommandClear,
			expectedArgs: []string{},
		},
		{
			name:         "unknown command",
			input:        "@codebutler foo",
			expectedType: CommandUnknown,
			expectedArgs: []string{},
		},
		{
			name:         "not a command",
			input:        "hello world",
			expectedType: "",
			expectedArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Parse(tt.input)

			if tt.expectedType == "" {
				if cmd != nil {
					t.Errorf("Expected nil command, got %v", cmd)
				}
				return
			}

			if cmd == nil {
				t.Fatal("Expected command, got nil")
			}

			if cmd.Type != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, cmd.Type)
			}

			if len(cmd.Args) != len(tt.expectedArgs) {
				t.Errorf("Expected %d args, got %d", len(tt.expectedArgs), len(cmd.Args))
			}

			for i, arg := range tt.expectedArgs {
				if cmd.Args[i] != arg {
					t.Errorf("Arg %d: expected %s, got %s", i, arg, cmd.Args[i])
				}
			}
		})
	}
}

func TestCommandGetArg(t *testing.T) {
	cmd := Parse("@codebutler use aurum")

	if cmd.GetArg(0) != "aurum" {
		t.Errorf("Expected 'aurum', got '%s'", cmd.GetArg(0))
	}

	if cmd.GetArg(1) != "" {
		t.Errorf("Expected empty string, got '%s'", cmd.GetArg(1))
	}

	if cmd.GetArg(-1) != "" {
		t.Errorf("Expected empty string for negative index, got '%s'", cmd.GetArg(-1))
	}
}

func TestCommandGetArgsString(t *testing.T) {
	cmd := Parse("@codebutler run add a new function")
	expected := "add a new function"

	if cmd.GetArgsString() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, cmd.GetArgsString())
	}
}

func TestCommandHasArgs(t *testing.T) {
	cmdWithArgs := Parse("@codebutler use aurum")
	cmdWithoutArgs := Parse("@codebutler help")

	if !cmdWithArgs.HasArgs() {
		t.Error("Expected HasArgs to be true")
	}

	if cmdWithoutArgs.HasArgs() {
		t.Error("Expected HasArgs to be false")
	}
}

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "valid help",
			input:       "@codebutler help",
			expectError: false,
		},
		{
			name:        "valid use",
			input:       "@codebutler use aurum",
			expectError: false,
		},
		{
			name:        "invalid use - no args",
			input:       "@codebutler use",
			expectError: true,
		},
		{
			name:        "valid run",
			input:       "@codebutler run test",
			expectError: false,
		},
		{
			name:        "invalid run - no args",
			input:       "@codebutler run",
			expectError: true,
		},
		{
			name:        "unknown command",
			input:       "@codebutler foo",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Parse(tt.input)
			err := ValidateCommand(cmd)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}
