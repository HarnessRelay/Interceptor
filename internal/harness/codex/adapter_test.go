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
	if len(data.Actions) != 2 || data.Actions[0].ID != "codex.approval_deny" || data.Actions[1].ID != "open_terminal" {
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

func TestActionBytesOnlySupportsVerifiedDeny(t *testing.T) {
	adapter := New()
	if got, ok := adapter.ActionBytes("codex.approval_deny"); !ok || !bytes.Equal(got, []byte{0x1b}) {
		t.Fatalf("deny bytes = %v, ok=%v", got, ok)
	}
	for _, action := range []string{"approve", "approve_always", "unknown"} {
		if _, ok := adapter.ActionBytes(action); ok {
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
