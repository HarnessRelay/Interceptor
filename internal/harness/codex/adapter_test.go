package codex

import (
	"bytes"
	"testing"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
)

func TestAdapterMatchesExactExecutableBasename(t *testing.T) {
	adapter := New()
	for _, command := range []string{"codex", "/usr/local/bin/codex", "./tools/codex"} {
		if match := adapter.Match(harness.LaunchSpec{Command: command}); !match.Matched || match.Confidence != 1 {
			t.Fatalf("Match(%q) = %+v, want exact confident match", command, match)
		}
	}
	for _, command := range []string{"codex-helper", "/tmp/not-codex-tool", "mycodex"} {
		if match := adapter.Match(harness.LaunchSpec{Command: command}); match.Matched {
			t.Fatalf("Match(%q) unexpectedly matched", command)
		}
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
	} {
		if !containsCapability(capabilities, wanted) {
			t.Fatalf("Capabilities() missing %q: %v", wanted, capabilities)
		}
	}
}

func TestPromptBytesUsesKittyEnterWhenEnabled(t *testing.T) {
	adapter := New()
	if got := adapter.PromptBytes("hello", []byte("prefix\x1b[>7usuffix")); !bytes.Equal(got, []byte("hello\x1b[13u")) {
		t.Fatalf("PromptBytes with Kitty mode = %q", got)
	}
	if got := adapter.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\r")) {
		t.Fatalf("PromptBytes fallback = %q", got)
	}
}

func TestParserPromptBytesTracksCurrentKeyboardProtocol(t *testing.T) {
	parser := &Parser{}
	parser.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[>7u"), Snapshot: []byte("\x1b[>7u")})
	if got := parser.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\x1b[13u")) {
		t.Fatalf("PromptBytes after Kitty enable = %q", got)
	}
	parts := parser.PromptSequence("hello", nil)
	if len(parts) != 2 || !bytes.Equal(parts[0], []byte("hello")) || !bytes.Equal(parts[1], []byte("\x1b[13u")) {
		t.Fatalf("PromptSequence after Kitty enable = %q", parts)
	}

	parser.Process(harness.TerminalUpdate{Chunk: []byte("\x1b[<u"), Snapshot: []byte("\x1b[>7u\x1b[<u")})
	if got := parser.PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\r")) {
		t.Fatalf("PromptBytes after Kitty disable = %q", got)
	}
	parts = parser.PromptSequence("hello", nil)
	if len(parts) != 2 || !bytes.Equal(parts[1], []byte{'\r'}) {
		t.Fatalf("PromptSequence after Kitty disable = %q", parts)
	}
}

func TestCommandCatalogIsVersionVerifiedAndUsesCurrentKeyboardProtocol(t *testing.T) {
	parser := &Parser{}
	if commands := parser.CommandCatalog(); len(commands) != 0 {
		t.Fatalf("catalog before version detection = %v", commands)
	}
	parser.Process(harness.TerminalUpdate{
		Chunk:    []byte("\x1b[>7uOpenAI Codex (v0.145.0)\r\n"),
		Snapshot: []byte("OpenAI Codex (v0.145.0)\r\n"),
	})
	commands := parser.CommandCatalog()
	if len(commands) < 20 {
		t.Fatalf("catalog has %d commands, want a complete verified catalog", len(commands))
	}
	parts, command, err := parser.CommandSequence("status", "")
	if err != nil {
		t.Fatalf("CommandSequence(status): %v", err)
	}
	if command.Invocation != "/status" || len(parts) != 2 ||
		!bytes.Equal(parts[0], []byte("/status")) || !bytes.Equal(parts[1], []byte(kittyEnter)) {
		t.Fatalf("status sequence = %q, command = %+v", parts, command)
	}
	parts, command, err = parser.CommandSequence("delete", "")
	if err != nil {
		t.Fatalf("CommandSequence(delete): %v", err)
	}
	if !command.Danger || command.Interaction != harness.CommandPrefillTerminal ||
		len(parts) != 1 || !bytes.Equal(parts[0], []byte("/delete")) {
		t.Fatalf("delete sequence = %q, command = %+v", parts, command)
	}
	if _, _, err := parser.CommandSequence("missing", ""); err == nil {
		t.Fatal("unknown command returned nil error")
	}
}

func TestCommandCatalogRejectsUnknownCodexVersion(t *testing.T) {
	parser := &Parser{}
	parser.Process(harness.TerminalUpdate{
		Chunk:    []byte("OpenAI Codex (v9.0.0)\r\n"),
		Snapshot: []byte("OpenAI Codex (v9.0.0)\r\n"),
	})
	if commands := parser.CommandCatalog(); len(commands) != 0 {
		t.Fatalf("unknown-version catalog = %v", commands)
	}
}

func TestParserClassifiesCodexWithoutEmittingTerminalArtifacts(t *testing.T) {
	parser := &Parser{}
	raw := []byte("\x1b[>7u\x1b[2JOpenAI Codex (v0.145.0)\r\nmodel: gpt-test high\r\nMMMMMMMM\r\n┌──┐")
	got := parser.Process(harness.TerminalUpdate{
		Chunk:    raw,
		Snapshot: raw,
		WorkDir:  "/tmp/research",
	})

	assertEventType(t, got, events.TypeTerminalNoisyOutput)
	assertEventType(t, got, events.TypeChatSystemMessage)
	assertEventType(t, got, events.TypeHarnessMetadata)
	for _, event := range got {
		if event.Type == events.TypeChatAssistantMessage {
			t.Fatalf("raw TUI artifact emitted as assistant message: %+v", event)
		}
	}
}

func TestParserExtractsRenderedAssistantResponseOnIdle(t *testing.T) {
	parser := &Parser{}
	parser.PromptSequence("Hi", nil)

	frame := []byte(
		"\x1b[2J" +
			"\x1b[10;1H• Startup notice that is not a response\x1b[K" +
			"\x1b[15;1H› Hi\x1b[K" +
			"\x1b[18;1H• Hi! What are we working on today?\x1b[K" +
			"\x1b[21;1H› Write tests for @filename\x1b[K" +
			"\x1b[23;1Hgpt-5.6-sol high · /tmp/project\x1b[K",
	)
	parser.Process(harness.TerminalUpdate{
		Chunk:   frame,
		Rows:    24,
		Cols:    120,
		WorkDir: "/tmp/project",
	})

	message := eventOfType(t, parser.OnIdle(), events.TypeChatAssistantMessage)
	data := message.Data.(events.ChatMessage)
	if data.MessageID != "codex-turn-1" || data.Content != "Hi! What are we working on today?" {
		t.Fatalf("assistant message = %+v", data)
	}
	if duplicate := parser.OnIdle(); len(duplicate) != 0 {
		t.Fatalf("duplicate idle emitted events: %+v", duplicate)
	}

	parser.Process(harness.TerminalUpdate{
		Chunk: []byte(
			"\x1b[18;1H• Hi! What are we working on today? I can help.\x1b[K",
		),
		Rows: 24,
		Cols: 120,
	})
	revision := eventOfType(t, parser.OnIdle(), events.TypeChatAssistantMessage)
	revised := revision.Data.(events.ChatMessage)
	if revised.MessageID != data.MessageID || revised.Content != "Hi! What are we working on today? I can help." {
		t.Fatalf("assistant revision = %+v", revised)
	}
}

func TestParserExtractsLastAssistantBlockAndWrappedLines(t *testing.T) {
	parser := &Parser{}
	parser.PromptSequence("Explain the change", nil)
	parser.Process(harness.TerminalUpdate{
		Chunk: []byte(
			"\x1b[2J" +
				"\x1b[5;1H› Explain the change\x1b[K" +
				"\x1b[8;1H• Explored repository files\x1b[K" +
				"\x1b[10;1H• The semantic adapter now reconstructs the rendered response.\x1b[K" +
				"\x1b[11;1H  It suppresses redraw artifacts and duplicate revisions.\x1b[K" +
				"\x1b[12;1H⚠ MCP startup incomplete\x1b[K" +
				"\x1b[13;1H  Approaching rate limits\x1b[K" +
				"\x1b[14;1H› Write tests for @filename\x1b[K" +
				"\x1b[16;1Hgpt-test high · /tmp/project\x1b[K",
		),
		Rows: 20,
		Cols: 100,
	})

	event := eventOfType(t, parser.OnIdle(), events.TypeChatAssistantMessage)
	data := event.Data.(events.ChatMessage)
	want := "The semantic adapter now reconstructs the rendered response.\nIt suppresses redraw artifacts and duplicate revisions."
	if data.Content != want {
		t.Fatalf("assistant content = %q, want %q", data.Content, want)
	}
}

func TestParserExtractsAssistantResponseAfterApprovalOverlay(t *testing.T) {
	parser := &Parser{}
	parser.PromptSequence("request approval", nil)
	parser.Process(harness.TerminalUpdate{
		Chunk: []byte(
			"\x1b[2J" +
				"\x1b[5;1H› request approval\x1b[K" +
				"\x1b[8;1HWould you like to run the following command?\x1b[K" +
				"\x1b[10;1H$ printf safe\x1b[K" +
				"\x1b[12;1H› 1. Yes, proceed (y)\x1b[K" +
				"\x1b[15;1H• Created example.txt after approval.\x1b[K" +
				"\x1b[18;1H› Write tests for @filename\x1b[K" +
				"\x1b[20;1Hgpt-fake high · /tmp/harnessrelay-fake\x1b[K",
		),
		Rows: 24,
		Cols: 100,
	})

	event := eventOfType(t, parser.OnIdle(), events.TypeChatAssistantMessage)
	data := event.Data.(events.ChatMessage)
	if data.Content != "Created example.txt after approval." {
		t.Fatalf("assistant content = %q", data.Content)
	}
}

func TestParseMetadataUsesRenderedModelFooter(t *testing.T) {
	metadata, ok := parseMetadata(
		"OpenAI Codex (v0.145.0)\n  gpt-5.6-sol high · /tmp/project",
		"/tmp/project",
	)
	if !ok || metadata.Model != "gpt-5.6-sol high" || metadata.Version != "0.145.0" {
		t.Fatalf("metadata = %+v, ok=%v", metadata, ok)
	}
}

func TestParseMetadataIgnoresLoadingPlaceholderForRenderedModel(t *testing.T) {
	metadata, ok := parseMetadata(
		"OpenAI Codex (v0.145.0)\nmodel: loading\n  gpt-5.6-sol high · /tmp/project",
		"/tmp/project",
	)
	if !ok || metadata.Model != "gpt-5.6-sol high" {
		t.Fatalf("metadata = %+v, ok=%v", metadata, ok)
	}
}

func TestParserDetectsApprovalOnceAndAllowsLaterIdenticalRequest(t *testing.T) {
	parser := &Parser{}
	overlay := []byte("Would you like to run the following command?\r\n$ printf safe\r\n1. Yes proceed\r\n3. No and tell Codex what to do differently")
	update := harness.TerminalUpdate{Chunk: overlay, Snapshot: overlay, WorkDir: "/tmp/project"}

	first := parser.Process(update)
	approval := eventOfType(t, first, events.TypeApprovalRequired)
	data := approval.Data.(events.ApprovalRequired)
	if data.Command != "printf safe" || data.WorkingDirectory != "/tmp/project" {
		t.Fatalf("approval data = %+v", data)
	}
	if len(data.Actions) != 3 || data.Actions[0].ID != "codex.approval_allow" || data.Actions[1].ID != "codex.approval_deny" || data.Actions[2].ID != "open_terminal" {
		t.Fatalf("approval actions = %+v", data.Actions)
	}
	if got := parser.Process(update); eventCount(got, events.TypeApprovalRequired) != 0 {
		t.Fatalf("duplicate update emitted approval: %+v", got)
	}

	parser.ActionResolved("codex.approval_deny")
	if got := parser.Process(update); eventCount(got, events.TypeApprovalRequired) != 1 {
		t.Fatalf("identical later request did not emit once: %+v", got)
	}
}

func TestParserDetectsCodex0145ApprovalPrompt(t *testing.T) {
	for _, commandLine := range []string{"$ touch example.txt", "  $ touch example.txt"} {
		t.Run(commandLine, func(t *testing.T) {
			parser := &Parser{}
			overlay := []byte("Would you like to run the following command?\r\n\r\n" +
				"Environment: local\r\n\r\n" +
				"Reason: Do you want to allow creating example.txt in the workspace using the harness permission system?\r\n\r\n" +
				commandLine + "\r\n\r\n" +
				"1. Yes, proceed (y)\r\n" +
				"2. Yes, and don't ask again for commands that start with `touch example.txt` (p)\r\n" +
				"3. No, and tell Codex what to do differently (esc)")
			update := harness.TerminalUpdate{Chunk: overlay, Snapshot: overlay, WorkDir: "/tmp/project"}

			approval := eventOfType(t, parser.Process(update), events.TypeApprovalRequired).Data.(events.ApprovalRequired)
			if approval.OperationKind != "shell_command" || approval.Command != "touch example.txt" {
				t.Fatalf("approval data = %+v", approval)
			}
			if len(approval.Actions) != 3 || approval.Actions[0].ID != "codex.approval_allow" || approval.Actions[1].ID != "codex.approval_deny" || approval.Actions[2].ID != "open_terminal" {
				t.Fatalf("approval actions = %+v", approval.Actions)
			}
			if got := parser.Process(update); eventCount(got, events.TypeApprovalRequired) != 0 {
				t.Fatalf("duplicate update emitted approval: %+v", got)
			}
		})
	}
}

func TestParserDetectsApprovalFromRenderedScreen(t *testing.T) {
	parser := &Parser{}
	frame := []byte(
		"\x1b[2J" +
			"\x1b[4;5HWould you like to run the following command?\x1b[K" +
			"\x1b[6;5HEnvironment: local\x1b[K" +
			"\x1b[8;5HReason: Do you want to allow creating example.txt in the workspace using the harness permission system?\x1b[K" +
			"\x1b[10;7H$ touch example.txt\x1b[K" +
			"\x1b[12;5H1. Yes, proceed (y)\x1b[K" +
			"\x1b[13;5H2. Yes, and don't ask again for commands that start with `touch example.txt` (p)\x1b[K" +
			"\x1b[14;5H3. No, and tell Codex what to do differently (esc)\x1b[K",
	)

	approval := eventOfType(t, parser.Process(harness.TerminalUpdate{
		Chunk:   frame,
		Rows:    24,
		Cols:    120,
		WorkDir: "/tmp/project",
	}), events.TypeApprovalRequired).Data.(events.ApprovalRequired)
	if approval.Command != "touch example.txt" {
		t.Fatalf("approval command = %q", approval.Command)
	}
}

func TestParserDetectsCapturedCodexApprovalPane(t *testing.T) {
	parser := &Parser{}
	pane := []byte("• Running rtk printf 'example\\n' | rtk tee example.txt\n\n" +
		"  Would you like to run the following command?\n\n" +
		"  Environment: local\n\n" +
		"  Reason: Do you want to allow me to create /home/nethunranasingha/MyData/Projects/GO/HarnessRelay/Interceptor/example.txt with simple\n" +
		"  example text?\n\n" +
		"  $ rtk printf 'example\\n' | rtk tee example.txt\n\n" +
		"› 1. Yes, proceed (y)\n" +
		"  2. Yes, and don't ask again for commands that start with `rtk printf \"example\\\\n\"` (p)\n" +
		"  3. No, and tell Codex what to do differently (esc)\n\n" +
		"  Press enter to confirm or esc to cancel")

	approval := eventOfType(t, parser.Process(harness.TerminalUpdate{
		Chunk:   pane,
		WorkDir: "/tmp/project",
	}), events.TypeApprovalRequired).Data.(events.ApprovalRequired)
	if approval.Command != "rtk printf 'example\\n' | rtk tee example.txt" {
		t.Fatalf("approval command = %q", approval.Command)
	}
}

func TestParserDetectsCollapsedPTYApprovalPrompt(t *testing.T) {
	parser := &Parser{}
	cleaned := []byte("Runningprintf 'This is a simple example file.\\n' > example.txt" +
		"Would you like to run the following command?" +
		"Environment:local" +
		"Reason:Do you want to allow me to create example.txt in the project workspace?" +
		"$printf 'This is a simple example file.\\n' > example.txt 1. Yes, proceed (y)" +
		"2.Yes,anddon'taskagainforcommandsthatstartwith`printf'Thisisasimpleexamplefile.\\n'>example.txt`(p)" +
		"3.No,andtellCodexwhattododifferently(esc)" +
		"Press enter to confirm or esc to cancel")

	approval := eventOfType(t, parser.Process(harness.TerminalUpdate{
		Chunk:   cleaned,
		WorkDir: "/tmp/project",
	}), events.TypeApprovalRequired).Data.(events.ApprovalRequired)
	if approval.Command != "printf 'This is a simple example file.\\n' > example.txt" {
		t.Fatalf("approval command = %q", approval.Command)
	}
}

func TestParserWaitsForApprovalCommandContext(t *testing.T) {
	parser := &Parser{}
	heading := []byte("Would you like to run the following command?\r\n")
	if got := parser.Process(harness.TerminalUpdate{Chunk: heading, Snapshot: heading, WorkDir: "/tmp/project"}); eventCount(got, events.TypeApprovalRequired) != 0 {
		t.Fatalf("approval emitted before command context: %+v", got)
	}

	command := []byte("$ printf safe\r\n1. Yes proceed\r\n3. No and tell Codex what to do differently")
	snapshot := append(append([]byte(nil), heading...), command...)
	got := parser.Process(harness.TerminalUpdate{Chunk: command, Snapshot: snapshot, WorkDir: "/tmp/project"})
	approval := eventOfType(t, got, events.TypeApprovalRequired).Data.(events.ApprovalRequired)
	if approval.Command != "printf safe" {
		t.Fatalf("approval command = %q", approval.Command)
	}
}

func TestParserRoutesWorkspaceTrustDecisionToTerminal(t *testing.T) {
	parser := &Parser{}
	overlay := []byte("Do you trust the contents of this directory?\r\n1. Yes, continue\r\n2. No, quit\r\nPress enter to continue")
	got := parser.Process(harness.TerminalUpdate{Chunk: overlay, Snapshot: overlay, WorkDir: "/tmp/project"})

	status := eventOfType(t, got, events.TypeHarnessStatus).Data.(events.HarnessStatus)
	if status.Status != "waiting_for_terminal" {
		t.Fatalf("status = %+v", status)
	}
	approval := eventOfType(t, got, events.TypeApprovalRequired).Data.(events.ApprovalRequired)
	if approval.OperationKind != "workspace_trust" || len(approval.Actions) != 1 || approval.Actions[0].ID != "open_terminal" {
		t.Fatalf("workspace trust event = %+v", approval)
	}
	if duplicate := parser.Process(harness.TerminalUpdate{Chunk: overlay, Snapshot: overlay, WorkDir: "/tmp/project"}); eventCount(duplicate, events.TypeApprovalRequired) != 0 {
		t.Fatalf("duplicate trust event = %+v", duplicate)
	}
}

func TestActionResultOnlySupportsVerifiedApprovalActions(t *testing.T) {
	adapter := New()
	if got, ok := adapter.ExecuteAction("codex.approval_allow"); !ok || !bytes.Equal(got.TerminalInput, []byte("y")) {
		t.Fatalf("allow result = %+v, ok=%v", got, ok)
	} else if got.Resolution != "approved" || got.Detail == "" || !got.ClearsPending {
		t.Fatalf("allow result = %+v", got)
	}
	if got, ok := adapter.ExecuteAction("codex.approval_deny"); !ok || !bytes.Equal(got.TerminalInput, []byte{0x1b}) {
		t.Fatalf("deny result = %+v, ok=%v", got, ok)
	} else if got.Resolution != "denied" || got.Detail == "" || !got.ClearsPending {
		t.Fatalf("deny result = %+v", got)
	}
	for _, action := range []string{"approve", "approve_always", "unknown"} {
		if _, ok := adapter.ExecuteAction(action); ok {
			t.Fatalf("unsafe action %q was accepted", action)
		}
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
