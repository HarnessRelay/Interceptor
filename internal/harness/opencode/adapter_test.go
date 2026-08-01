package opencode

import (
	"bytes"
	"fmt"
	"testing"

	xterm "github.com/gitpod-io/xterm-go"
	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
)

func TestAdapterMatchesExactExecutable(t *testing.T) {
	adapter := New()
	for _, command := range []string{"opencode", "opencode-ai", "/usr/local/bin/opencode", "./tools/opencode"} {
		if match := adapter.Match(harness.LaunchSpec{Command: command}); !match.Matched || match.Confidence != 1 {
			t.Fatalf("Match(%q) = %+v, want exact confident match", command, match)
		}
	}
	for _, command := range []string{"opencode-helper", "/tmp/not-opencode", "myopencode", "codex"} {
		if match := adapter.Match(harness.LaunchSpec{Command: command}); match.Matched {
			t.Fatalf("Match(%q) unexpectedly matched", command)
		}
	}
}

func TestAdapterMatchesNpmGlobalInstall(t *testing.T) {
	adapter := New()
	match := adapter.Match(harness.LaunchSpec{Command: "/home/user/.nvm/versions/node/v24.15.0/bin/opencode"})
	if !match.Matched || match.Confidence != 1 {
		t.Fatalf("npm global install match = %+v", match)
	}
}

func TestAdapterCapabilities(t *testing.T) {
	capabilities := New().Capabilities()
	for _, wanted := range []harness.Capability{
		harness.CapabilitySemanticChat,
		harness.CapabilityPromptSubmit,
		harness.CapabilityApprovalDetection,
		harness.CapabilityApprovalActions,
		harness.CapabilityNoiseFiltering,
		harness.CapabilityCommandCatalog,
		harness.CapabilityCommandInvoke,
	} {
		if !containsCapability(capabilities, wanted) {
			t.Fatalf("Capabilities() missing %q: %v", wanted, capabilities)
		}
	}
}

func TestPromptBytesUsesKittyProtocolWhenDetected(t *testing.T) {
	adapter := New()
	// OpenCode uses >4;1m format
	if got := adapter.PromptBytes("hello", []byte("prefix\x1b[>4;1msuffix")); !bytes.Equal(got, []byte("hello\x1b[13u")) {
		t.Fatalf("PromptBytes with OpenCode Kitty mode = %q", got)
	}
	if got := adapter.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\r")) {
		t.Fatalf("PromptBytes fallback = %q", got)
	}
}

func TestParserPromptBytesTracksKeyboardProtocol(t *testing.T) {
	parser := &Parser{}
	// OpenCode uses xterm modifyOtherKeys (>4;1m), not Kitty keyboard protocol.
	// Enter is still \r in this mode.
	parser.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[>4;1m"), Snapshot: []byte("\x1b[>4;1m")})
	if got := parser.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\r")) {
		t.Fatalf("PromptBytes with xterm modifyOtherKeys = %q", got)
	}
	parts := parser.PromptSequence("hello", nil)
	if len(parts) != 2 || !bytes.Equal(parts[0], []byte("hello")) || !bytes.Equal(parts[1], []byte("\r")) {
		t.Fatalf("PromptSequence with xterm modifyOtherKeys = %q", parts)
	}

	// If Kitty keyboard protocol IS enabled (by some other mechanism),
	// CSI u Enter should be used.
	parser2 := &Parser{}
	parser2.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[>7u"), Snapshot: []byte("\x1b[>7u")})
	if got := parser2.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\x1b[13u")) {
		t.Fatalf("PromptBytes with Kitty protocol = %q", got)
	}
	parts = parser2.PromptSequence("hello", nil)
	if len(parts) != 2 || !bytes.Equal(parts[1], []byte("\x1b[13u")) {
		t.Fatalf("PromptSequence with Kitty protocol = %q", parts)
	}

	// Disable Kitty keyboard
	parser2.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[<u"), Snapshot: []byte("\x1b[>7u\x1b[<u")})
	if got := parser2.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\r")) {
		t.Fatalf("PromptBytes after Kitty disable = %q", got)
	}
}

func TestParserDetectsOpenCodeUI(t *testing.T) {
	parser := &Parser{}
	raw := []byte("█▀▀█ █▀▀█ █▀▀█ █▀▀▄ █▀▀▀ █▀▀█\r\n█  █ █  █ █▀▀▀ █  █ █    █  █\r\n▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀  ▀ ▀▀▀▀ ▀▀▀▀\r\n1.18.3\r\n")
	got := parser.Process(harness.TerminalUpdate{
		Chunk:    raw,
		Snapshot: raw,
		WorkDir:  "/tmp/test",
	})

	assertEventType(t, got, events.TypeTerminalNoisyOutput)
	assertEventType(t, got, events.TypeChatSystemMessage)
	for _, event := range got {
		if event.Type == events.TypeChatAssistantMessage {
			t.Fatalf("raw TUI artifact emitted as assistant message: %+v", event)
		}
	}
}

func TestParserExtractsVersionFromFooter(t *testing.T) {
	parser := &Parser{}
	raw := []byte("█▀▀█ █▀▀█\r\n/tmp/test  1.18.3\r\n")
	parser.Process(harness.TerminalUpdate{
		Chunk:    raw,
		Snapshot: raw,
		WorkDir:  "/tmp/test",
	})
	if parser.lastMetadata.Version != "1.18.3" {
		t.Fatalf("version = %q, want 1.18.3", parser.lastMetadata.Version)
	}
}

func TestParserExtractsModelFromStatusBar(t *testing.T) {
	parser := &Parser{}
	raw := []byte("█▀▀█ █▀▀█\r\nBuild · Kimi K2.6 Canopy Wave Coding Plan\r\n1.18.3\r\n")
	parser.Process(harness.TerminalUpdate{
		Chunk:    raw,
		Snapshot: raw,
		WorkDir:  "/tmp/test",
	})
	if parser.lastMetadata.Model != "Kimi K2.6 Canopy Wave Coding Plan" {
		t.Fatalf("model = %q", parser.lastMetadata.Model)
	}
}

func TestParserDetectsPermissionPrompt(t *testing.T) {
	parser := &Parser{}
	screen := []byte(
		"█▀▀█ █▀▀█\r\n" +
			"┃ △ Permission required\r\n" +
			"┃ # Shell command\r\n" +
			"┃ $ echo hello\r\n" +
			"┃ Allow once   Allow always   Reject\r\n" +
			"┃ enter confirm\r\n" +
			"1.18.3\r\n",
	)
	got := parser.Process(harness.TerminalUpdate{
		Chunk:    screen,
		Snapshot: screen,
		WorkDir:  "/tmp/test",
	})

	approval := eventOfType(t, got, events.TypeApprovalRequired)
	data := approval.Data.(events.ApprovalRequired)
	if data.OperationKind != "shell_command" {
		t.Fatalf("operation_kind = %q, want shell_command", data.OperationKind)
	}
	if data.Command != "echo hello" {
		t.Fatalf("command = %q, want echo hello", data.Command)
	}
	if data.ToolName != "Shell command" {
		t.Fatalf("tool_name = %q", data.ToolName)
	}
	if len(data.Actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(data.Actions))
	}
	if data.Actions[0].ID != "opencode.approval_allow" {
		t.Fatalf("first action = %q, want opencode.approval_allow", data.Actions[0].ID)
	}
	if data.Actions[1].ID != "opencode.approval_allow_always" {
		t.Fatalf("second action = %q", data.Actions[1].ID)
	}
	if data.Actions[2].ID != "opencode.approval_deny" {
		t.Fatalf("third action = %q", data.Actions[2].ID)
	}
}

func TestParserDetectsPermissionOnceAndSuppressesDuplicate(t *testing.T) {
	parser := &Parser{}
	screen := []byte(
		"█▀▀█ █▀▀█\r\n" +
			"┃ △ Permission required\r\n" +
			"┃ # Edit\r\n" +
			"┃ $ Write src/main.go\r\n" +
			"┃ Allow once   Allow always   Reject\r\n",
	)

	first := parser.Process(harness.TerminalUpdate{Chunk: screen, Snapshot: screen, WorkDir: "/tmp/test"})
	approval := eventOfType(t, first, events.TypeApprovalRequired)
	data := approval.Data.(events.ApprovalRequired)
	if data.OperationKind != "file_edit" {
		t.Fatalf("edit operation_kind = %q", data.OperationKind)
	}

	// Second identical update should NOT emit another approval
	if got := parser.Process(harness.TerminalUpdate{Chunk: screen, Snapshot: screen, WorkDir: "/tmp/test"}); eventCount(got, events.TypeApprovalRequired) != 0 {
		t.Fatalf("duplicate update emitted approval: %+v", got)
	}

	// After action resolution, identical request should emit again
	parser.ActionResolved("opencode.approval_allow")
	if got := parser.Process(harness.TerminalUpdate{Chunk: screen, Snapshot: screen, WorkDir: "/tmp/test"}); eventCount(got, events.TypeApprovalRequired) != 1 {
		t.Fatalf("identical later request did not emit once: %+v", got)
	}
}

func TestApprovalActionsWork(t *testing.T) {
	adapter := New()

	got, ok := adapter.ExecuteAction("opencode.approval_allow")
	if !ok || !bytes.Equal(got.TerminalInput, []byte("\r")) {
		t.Fatalf("allow result = %+v, ok=%v", got, ok)
	}
	if got.Resolution != "approved" || got.Detail == "" || !got.ClearsPending {
		t.Fatalf("allow result = %+v", got)
	}

	got, ok = adapter.ExecuteAction("opencode.approval_allow_always")
	if !ok || !bytes.Equal(got.TerminalInput, []byte("\t\r")) {
		t.Fatalf("allow_always result = %+v, ok=%v", got, ok)
	}
	if got.Resolution != "approved_always" || got.Detail == "" || !got.ClearsPending {
		t.Fatalf("allow_always result = %+v", got)
	}

	got, ok = adapter.ExecuteAction("opencode.approval_deny")
	if !ok || !bytes.Equal(got.TerminalInput, []byte{0x1b}) {
		t.Fatalf("deny result = %+v, ok=%v", got, ok)
	}
	if got.Resolution != "denied" || got.Detail == "" || !got.ClearsPending {
		t.Fatalf("deny result = %+v", got)
	}

	for _, action := range []string{"approve", "approve_always", "unknown"} {
		if _, ok := adapter.ExecuteAction(action); ok {
			t.Fatalf("unsafe action %q was accepted", action)
		}
	}
}

func TestParserDetectsProcessingStatus(t *testing.T) {
	parser := &Parser{}
	// First trigger UI detection
	parser.Process(harness.TerminalUpdate{
		Chunk:    []byte("█▀▀█ █▀▀█\r\n1.18.3\r\n"),
		Snapshot: []byte("█▀▀█ █▀▀█\r\n1.18.3\r\n"),
	})

	// Then detect thinking
	got := parser.Process(harness.TerminalUpdate{
		Chunk:    []byte("⠋ Thinking\r\n"),
		Snapshot: []byte("█▀▀█ █▀▀█\r\n⠋ Thinking\r\n"),
	})
	status := eventOfType(t, got, events.TypeHarnessStatus)
	data := status.Data.(events.HarnessStatus)
	if data.Status != "processing" {
		t.Fatalf("status = %q, want processing", data.Status)
	}
}

func TestCommandCatalogHasBuiltInCommands(t *testing.T) {
	parser := &Parser{}
	commands := parser.CommandCatalog()
	if len(commands) < 10 {
		t.Fatalf("expected at least 10 built-in commands, got %d", len(commands))
	}

	// Verify some key commands exist
	found := map[string]bool{}
	for _, cmd := range commands {
		found[cmd.ID] = true
	}
	for _, id := range []string{"compact", "exit", "help", "init", "models", "new", "sessions", "undo"} {
		if !found[id] {
			t.Fatalf("missing built-in command: %s", id)
		}
	}
}

func TestCommandSequenceUsesCorrectProtocol(t *testing.T) {
	parser := &Parser{}
	// OpenCode uses xterm modifyOtherKeys, not Kitty keyboard protocol.
	// Command sequence should use \r for Enter.
	parser.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[>4;1m"), Snapshot: []byte("\x1b[>4;1m")})

	parts, command, err := parser.CommandSequence("compact", "")
	if err != nil {
		t.Fatalf("CommandSequence(compact): %v", err)
	}
	if command.Invocation != "/compact" {
		t.Fatalf("invocation = %q", command.Invocation)
	}
	if len(parts) != 2 || !bytes.Equal(parts[0], []byte("/compact")) || !bytes.Equal(parts[1], []byte("\r")) {
		t.Fatalf("compact sequence = %q", parts)
	}

	// Test with Kitty keyboard protocol enabled (hypothetical)
	parser2 := &Parser{}
	parser2.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[>7u"), Snapshot: []byte("\x1b[>7u")})
	parts, _, err = parser2.CommandSequence("compact", "")
	if err != nil {
		t.Fatalf("CommandSequence with Kitty: %v", err)
	}
	if !bytes.Equal(parts[1], []byte(kittyEnter)) {
		t.Fatalf("compact sequence with Kitty = %q", parts)
	}
}

func TestCommandSequenceWithArguments(t *testing.T) {
	parser := &Parser{}
	parts, command, err := parser.CommandSequence("sessions", "list")
	if err != nil {
		t.Fatalf("CommandSequence(sessions): %v", err)
	}
	if command.Invocation != "/sessions" {
		t.Fatalf("invocation = %q", command.Invocation)
	}
	if len(parts) == 0 {
		t.Fatal("expected PTY parts")
	}
	_ = parts
}

func TestCommandSequenceRejectsUnknownCommand(t *testing.T) {
	parser := &Parser{}
	if _, _, err := parser.CommandSequence("nonexistent", ""); err == nil {
		t.Fatal("unknown command returned nil error")
	}
}

func TestCommandDiscovery(t *testing.T) {
	parser := &Parser{}
	parser.ensureCommandsInitialized()

	initial := len(parser.CommandCatalog())

	// Simulate terminal output with a new command
	parser.discoverCommandsFromOutput("/test-command - A test command\n")

	commands := parser.CommandCatalog()
	if len(commands) != initial+1 {
		t.Fatalf("expected %d commands, got %d", initial+1, len(commands))
	}

	found := false
	for _, cmd := range commands {
		if cmd.ID == "test-command" {
			found = true
			if cmd.Description != "A test command" {
				t.Errorf("description = %q", cmd.Description)
			}
			break
		}
	}
	if !found {
		t.Error("expected test-command to be discovered")
	}
}

func TestExtractResponseFromRealOpenCodeScreen(t *testing.T) {
	// Real OpenCode v1.18.3 layout: assistant responses are OUTSIDE borders.
	screen := xterm.New(xterm.WithRows(40), xterm.WithCols(120), xterm.WithScrollback(2000))
	lines := []string{
		"\x1b[2;1H  ┃  hello\x1b[K",                                      // user prompt (bordered)
		"\x1b[5;1H     + Thought: 24ms\x1b[K",                            // thinking indicator
		"\x1b[7;1H     Here is the answer to your question.\x1b[K",       // response (no border!)
		"\x1b[8;1H     It contains multiple paragraphs.\x1b[K",           // response continuation
		"\x1b[10;1H     ▣  Build · Kimi K2.6 · 3.4s\x1b[K",               // model footer with timing
		"\x1b[33;1H  ┃ \x1b[K",                                           // input area
		"\x1b[36;1H  ┃  Build · Kimi K2.6 Canopy Wave Coding Plan\x1b[K", // status bar in input
		"\x1b[37;1H  ╹" + repeat("▀", 118),                               // separator
		"\x1b[38;1H   /tmp/project   ctrl+p commands\x1b[K",              // footer
	}
	for _, line := range lines {
		_, _ = screen.Write([]byte(line))
	}
	response := extractAssistantResponse(screen, "hello")
	expected := "Here is the answer to your question.\nIt contains multiple paragraphs."
	if response != expected {
		t.Fatalf("extracted response = %q, want %q", response, expected)
	}
}

func TestExtractResponseDoesNotIncludeStatusBar(t *testing.T) {
	screen := xterm.New(xterm.WithRows(40), xterm.WithCols(80), xterm.WithScrollback(2000))
	lines := []string{
		"\x1b[2;1H  ┃  explain the codebase\x1b[K",
		"\x1b[5;1H     + Thought: 100ms\x1b[K",
		"\x1b[7;1H     The project is a relay layer.\x1b[K",
		"\x1b[8;1H     It uses adapters to parse harness-specific output.\x1b[K",
		"\x1b[10;1H     ▣  Build · gpt-5.6-sol high · 2.1s\x1b[K",
		"\x1b[33;1H  ┃ \x1b[K",
		"\x1b[36;1H  ┃  Build · gpt-5.6-sol high\x1b[K",
		"\x1b[37;1H  ╹" + repeat("▀", 78),
	}
	for _, line := range lines {
		_, _ = screen.Write([]byte(line))
	}
	response := extractAssistantResponse(screen, "explain the codebase")
	if bytes.Contains([]byte(response), []byte("Build")) {
		t.Fatalf("response contains status bar: %q", response)
	}
	if bytes.Contains([]byte(response), []byte("▀")) {
		t.Fatalf("response contains separator: %q", response)
	}
	expected := "The project is a relay layer.\nIt uses adapters to parse harness-specific output."
	if response != expected {
		t.Fatalf("response = %q, want %q", response, expected)
	}
}

func TestExtractResponseFromWrappedLines(t *testing.T) {
	screen := xterm.New(xterm.WithRows(30), xterm.WithCols(80), xterm.WithScrollback(2000))
	lines := []string{
		"\x1b[2;1H  ┃  hi\x1b[K",
		"\x1b[7;1H     This is a long response that wraps across\x1b[K",
		"\x1b[8;1H     multiple lines in the terminal.\x1b[K",
		"\x1b[10;1H     ▣  Build · model · 1.0s\x1b[K",
		"\x1b[37;1H  ╹" + repeat("▀", 78),
	}
	for _, line := range lines {
		_, _ = screen.Write([]byte(line))
	}
	response := extractAssistantResponse(screen, "hi")
	expected := "This is a long response that wraps across\nmultiple lines in the terminal."
	if response != expected {
		t.Fatalf("response = %q, want %q", response, expected)
	}
}

func TestExtractResponseSkipsActivityIndicator(t *testing.T) {
	screen := xterm.New(xterm.WithRows(30), xterm.WithCols(80), xterm.WithScrollback(2000))
	lines := []string{
		"\x1b[2;1H  ┃  hello\x1b[K",
		"\x1b[5;1H     + Thought: 50ms\x1b[K",
		"\x1b[7;1H     Done.\x1b[K",
		"\x1b[10;1H     ▣  Build · model · 0.5s\x1b[K",
	}
	for _, line := range lines {
		_, _ = screen.Write([]byte(line))
	}
	response := extractAssistantResponse(screen, "hello")
	if response != "Done." {
		t.Fatalf("response = %q, want %q", response, "Done.")
	}
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func dumpScreenForTest(t *testing.T, term *xterm.Terminal) {
	t.Helper()
	buffer := term.Buffer()
	for i := 0; i < buffer.Lines.Length(); i++ {
		line := buffer.Lines.Get(i)
		if line == nil {
			continue
		}
		text := line.TranslateToString(true, 0, -1)
		wrapped := ""
		if line.IsWrapped {
			wrapped = " [W]"
		}
		fmt.Printf("  [%2d] %q%s\n", i, text, wrapped)
	}
}

func containsCapability(values []harness.Capability, wanted harness.Capability) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func assertEventType(t *testing.T, values []events.Event, wanted events.Type) {
	t.Helper()
	_ = eventOfType(t, values, wanted)
}

func eventOfType(t *testing.T, values []events.Event, wanted events.Type) events.Event {
	t.Helper()
	for _, value := range values {
		if value.Type == wanted {
			return value
		}
	}
	t.Fatalf("events missing %q: %+v", wanted, values)
	return events.Event{}
}

func eventCount(values []events.Event, wanted events.Type) int {
	count := 0
	for _, value := range values {
		if value.Type == wanted {
			count++
		}
	}
	return count
}
