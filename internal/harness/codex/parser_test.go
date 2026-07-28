package codex

import (
	"testing"

	"github.com/harnessrelay/interceptor/internal/harness"
)

func TestCommandDiscovery(t *testing.T) {
	parser := &Parser{}

	// Initialize commands
	parser.ensureCommandsInitialized()

	// Verify hardcoded commands exist
	commands := parser.CommandCatalog()
	if len(commands) == 0 {
		t.Error("Expected commands to be initialized")
	}

	// Verify we have the expected number of hardcoded commands
	if len(commands) != len(codex0145Commands) {
		t.Errorf("Expected %d commands, got %d", len(codex0145Commands), len(commands))
	}

	// Simulate terminal output with new command
	parser.discoverCommandsFromOutput("/new-command - A new command\n")

	// Verify new command was added
	commands = parser.CommandCatalog()
	found := false
	for _, cmd := range commands {
		if cmd.ID == "new-command" {
			found = true
			if cmd.Description != "A new command" {
				t.Errorf("Expected description 'A new command', got '%s'", cmd.Description)
			}
			if cmd.Group != "Discovered" {
				t.Errorf("Expected group 'Discovered', got '%s'", cmd.Group)
			}
			break
		}
	}
	if !found {
		t.Error("Expected new command to be discovered")
	}
}

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *harness.CommandDescriptor
	}{
		{
			name:  "command with description",
			input: "/status - Show status",
			expected: &harness.CommandDescriptor{
				ID:          "status",
				Invocation:  "/status",
				Label:       "Status",
				Description: "Show status",
				Group:       "Discovered",
				Interaction: harness.CommandSubmit,
			},
		},
		{
			name:  "command with hyphen",
			input: "/debug-config  Show debug config",
			expected: &harness.CommandDescriptor{
				ID:          "debug-config",
				Invocation:  "/debug-config",
				Label:       "Debug Config",
				Description: "Show debug config",
				Group:       "Discovered",
				Interaction: harness.CommandSubmit,
			},
		},
		{
			name:  "command without description",
			input: "/model",
			expected: &harness.CommandDescriptor{
				ID:          "model",
				Invocation:  "/model",
				Label:       "Model",
				Description: "",
				Group:       "Discovered",
				Interaction: harness.CommandSubmit,
			},
		},
		{
			name:     "empty line",
			input:    "",
			expected: nil,
		},
		{
			name:     "not a command",
			input:    "hello world",
			expected: nil,
		},
		{
			name:     "comment line",
			input:    "// this is a comment",
			expected: nil,
		},
		{
			name:     "path with space",
			input:    "/ some path",
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseCommandFromOutput(test.input)
			if result == nil && test.expected != nil {
				t.Errorf("Expected command for input %q, got nil", test.input)
				return
			}
			if result != nil && test.expected == nil {
				t.Errorf("Expected no command for input %q, got %+v", test.input, result)
				return
			}
			if result != nil {
				if result.ID != test.expected.ID {
					t.Errorf("Expected ID %q, got %q", test.expected.ID, result.ID)
				}
				if result.Invocation != test.expected.Invocation {
					t.Errorf("Expected Invocation %q, got %q", test.expected.Invocation, result.Invocation)
				}
				if result.Label != test.expected.Label {
					t.Errorf("Expected Label %q, got %q", test.expected.Label, result.Label)
				}
				if result.Description != test.expected.Description {
					t.Errorf("Expected Description %q, got %q", test.expected.Description, result.Description)
				}
				if result.Group != test.expected.Group {
					t.Errorf("Expected Group %q, got %q", test.expected.Group, result.Group)
				}
				if result.Interaction != test.expected.Interaction {
					t.Errorf("Expected Interaction %q, got %q", test.expected.Interaction, result.Interaction)
				}
			}
		})
	}
}

func TestCommandMerging(t *testing.T) {
	parser := &Parser{}

	// Initialize commands
	parser.ensureCommandsInitialized()

	// Get initial count
	initialCommands := parser.CommandCatalog()
	initialCount := len(initialCommands)

	// Discover a new command
	parser.discoverCommandsFromOutput("/new-test-command - A test command\n")

	// Verify command was added
	commands := parser.CommandCatalog()
	if len(commands) != initialCount+1 {
		t.Errorf("Expected %d commands, got %d", initialCount+1, len(commands))
	}

	// Verify the new command exists
	found := false
	for _, cmd := range commands {
		if cmd.ID == "new-test-command" {
			found = true
			if cmd.Description != "A test command" {
				t.Errorf("Expected description 'A test command', got '%s'", cmd.Description)
			}
			break
		}
	}
	if !found {
		t.Error("Expected new-test-command to be discovered")
	}

	// Try to add the same command again with updated description
	parser.discoverCommandsFromOutput("/new-test-command - Updated description\n")

	// Verify command count is still the same (no duplicate)
	commands = parser.CommandCatalog()
	if len(commands) != initialCount+1 {
		t.Errorf("Expected %d commands (no duplicate), got %d", initialCount+1, len(commands))
	}

	// Verify description was updated (since the existing description was empty or we're updating)
	for _, cmd := range commands {
		if cmd.ID == "new-test-command" {
			// The description should be updated since we're providing a new one
			if cmd.Description != "Updated description" {
				t.Errorf("Expected description 'Updated description', got '%s'", cmd.Description)
			}
			break
		}
	}
}

func TestCommandSequence(t *testing.T) {
	parser := &Parser{}

	// Initialize commands
	parser.ensureCommandsInitialized()

	// Test known command
	parts, command, err := parser.CommandSequence("status", "")
	if err != nil {
		t.Errorf("Expected no error for known command, got %v", err)
	}
	if command.ID != "status" {
		t.Errorf("Expected command ID 'status', got '%s'", command.ID)
	}
	if len(parts) == 0 {
		t.Error("Expected PTY parts, got empty")
	}

	// Test unknown command
	_, _, err = parser.CommandSequence("unknown-command", "")
	if err == nil {
		t.Error("Expected error for unknown command")
	}

	// Test command with arguments
	_, command, err = parser.CommandSequence("rename", "my-chat")
	if err != nil {
		t.Errorf("Expected no error for command with arguments, got %v", err)
	}
	if command.ID != "rename" {
		t.Errorf("Expected command ID 'rename', got '%s'", command.ID)
	}
}

func TestVersionAgnostic(t *testing.T) {
	parser := &Parser{}

	// Set metadata with a non-0.145.x version
	parser.lastMetadata.Version = "0.150.0"

	// Commands should still work
	commands := parser.CommandCatalog()
	if len(commands) == 0 {
		t.Error("Expected commands to work with non-0.145.x version")
	}

	// Test command sequence
	_, _, err := parser.CommandSequence("status", "")
	if err != nil {
		t.Errorf("Expected command sequence to work with non-0.145.x version, got %v", err)
	}
}

func TestHelpOutputParsing(t *testing.T) {
	parser := &Parser{}

	// Initialize commands
	parser.ensureCommandsInitialized()

	// Simulate help output
	helpOutput := `
Available commands:
  /help    Show this help message
  /quit    Exit the application
  /clear   Clear the screen
`

	parser.discoverCommandsFromHelp(helpOutput)

	// Verify commands were discovered
	commands := parser.CommandCatalog()
	foundHelp := false
	foundQuit := false
	foundClear := false

	for _, cmd := range commands {
		switch cmd.ID {
		case "help":
			foundHelp = true
			if cmd.Description != "Show this help message" {
				t.Errorf("Expected help description 'Show this help message', got '%s'", cmd.Description)
			}
		case "quit":
			foundQuit = true
			if cmd.Description != "Exit the application" {
				t.Errorf("Expected quit description 'Exit the application', got '%s'", cmd.Description)
			}
		case "clear":
			foundClear = true
			// Note: /clear already exists in the hardcoded list
			// The description should be updated since we provide a new one
			if cmd.Description != "Clear the screen" {
				t.Errorf("Expected clear description 'Clear the screen', got '%s'", cmd.Description)
			}
		}
	}

	if !foundHelp {
		t.Error("Expected /help to be discovered from help output")
	}
	if !foundQuit {
		t.Error("Expected /quit to be discovered from help output")
	}
	if !foundClear {
		t.Error("Expected /clear to be discovered from help output")
	}
}

func TestFormatCommandLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"status", "Status"},
		{"debug-config", "Debug Config"},
		{"model", "Model"},
		{"new-command", "New Command"},
		{"ps", "Ps"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := formatCommandLabel(test.input)
			if result != test.expected {
				t.Errorf("Expected '%s', got '%s'", test.expected, result)
			}
		})
	}
}

func TestIsValidCommandName(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"status", true},
		{"debug-config", true},
		{"model", true},
		{"ps", true},
		{"new-command", true},
		{"command_123", true},
		{"", false},
		{"invalid command", false},
		{"invalid/command", false},
		{"invalid.command", false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := isValidCommandName(test.input)
			if result != test.expected {
				t.Errorf("Expected %v for '%s', got %v", test.expected, test.input, result)
			}
		})
	}
}
