// Package fakesemantic provides an explicitly enabled QA adapter used to prove
// that common semantic contracts do not depend on Codex.
package fakesemantic

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
)

const adapterID = "fake-semantic"

type Adapter struct{}

func New() Adapter { return Adapter{} }

func (Adapter) ID() string                { return adapterID }
func (Adapter) Name() string              { return "Fake Semantic" }
func (Adapter) Priority() int             { return 90 }
func (Adapter) NewParser() harness.Parser { return &Parser{} }

func (Adapter) Match(spec harness.LaunchSpec) harness.MatchResult {
	if filepath.Base(filepath.Clean(spec.Command)) != adapterID {
		return harness.MatchResult{}
	}
	return harness.MatchResult{Matched: true, Confidence: 1, Reason: "QA adapter executable basename matched"}
}

func (Adapter) Capabilities() []harness.Capability {
	return []harness.Capability{
		harness.CapabilityRawTerminal,
		harness.CapabilitySemanticChat,
		harness.CapabilityPromptSubmit,
		harness.CapabilityApprovalDetection,
		harness.CapabilityApprovalActions,
		harness.CapabilityStatusDetection,
		harness.CapabilityMetadataDetection,
		harness.CapabilityTextInput,
		harness.CapabilityResize,
		harness.CapabilityTerminate,
		harness.CapabilityCommandCatalog,
		harness.CapabilityCommandInvoke,
	}
}

func (Adapter) PromptBytes(text string, _ []byte) []byte {
	return append([]byte(text), '\r')
}

func (Adapter) ExecuteAction(actionID string) (harness.ActionResult, bool) {
	if actionID == "fake.silent" {
		return harness.ActionResult{
			TerminalInput: []byte("confirm\r"),
			ClearsPending: true,
		}, true
	}
	if actionID != "fake.confirm" {
		return harness.ActionResult{}, false
	}
	return harness.ActionResult{
		Resolution:    "confirmed",
		Status:        "processing",
		Detail:        "Fake Semantic completed its review.",
		TerminalInput: []byte("confirm\r"),
		ClearsPending: true,
	}, true
}

type Parser struct {
	mu       sync.Mutex
	detected bool
	approval bool
}

func (p *Parser) Process(update harness.TerminalUpdate) []events.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	text := string(update.Chunk)
	var out []events.Event
	if !p.detected && strings.Contains(text, "FAKE_READY") {
		p.detected = true
		out = append(out,
			events.Event{Type: events.TypeHarnessMetadata, Data: events.HarnessMetadata{Model: "fake-model", Version: "1.0.0", WorkDir: update.WorkDir, Confidence: 1}},
			events.Event{Type: events.TypeChatSystemMessage, Data: events.ChatMessage{Role: "system", Content: "Fake Semantic harness is ready.", Source: adapterID, Confidence: 1}},
			events.Event{Type: events.TypeHarnessStatus, Data: events.HarnessStatus{Status: "idle", Detail: "Fake Semantic is waiting for input.", Confidence: 1}},
		)
	}
	if strings.Contains(text, "FAKE_RESPONSE:") {
		content := strings.TrimSpace(strings.SplitN(text, "FAKE_RESPONSE:", 2)[1])
		out = append(out, events.Event{Type: events.TypeChatAssistantMessage, Data: events.ChatMessage{Role: "assistant", Content: content, Source: adapterID, Confidence: 1}})
	}
	if !p.approval && strings.Contains(text, "FAKE_APPROVAL") {
		p.approval = true
		out = append(out,
			events.Event{Type: events.TypeHarnessStatus, Data: events.HarnessStatus{Status: "waiting_for_approval", Detail: "Fake Semantic needs a review decision.", Confidence: 1}},
			events.Event{Type: events.TypeApprovalRequired, Data: events.ApprovalRequired{
				OperationKind: "tool_call", ToolName: "fake.review", AdapterSource: adapterID,
				Prompt: "Review the fake semantic operation.", Confidence: 1, BlocksPrompt: true,
				Actions: []events.SemanticAction{
					{ID: "fake.confirm", Label: "Confirm", Kind: "approval", RequiresEventID: true, Version: 1},
					{ID: "fake.silent", Label: "Complete silently", Kind: "approval", RequiresEventID: true, Version: 1},
					{ID: "open_terminal", Label: "Open Terminal", Kind: "ui", RequiresEventID: true, Version: 1},
				},
			}},
		)
	}
	if !p.approval && strings.Contains(text, "FAKE_TERMINAL_DECISION") {
		p.approval = true
		out = append(out,
			events.Event{Type: events.TypeHarnessStatus, Data: events.HarnessStatus{Status: "waiting_for_terminal", Detail: "Fake Semantic needs terminal interaction.", Confidence: 1}},
			events.Event{Type: events.TypeApprovalRequired, Data: events.ApprovalRequired{
				OperationKind: "unknown", AdapterSource: adapterID,
				Prompt:     "This fake decision can only be completed in Terminal Mode.",
				Confidence: 1, BlocksPrompt: true, RequiresTerminal: true,
				Actions: []events.SemanticAction{
					{ID: "open_terminal", Label: "Open Terminal", Kind: "ui", RequiresEventID: true, Version: 1},
				},
			}},
		)
	}
	return out
}

func (p *Parser) ActionResolved(string) {
	p.mu.Lock()
	p.approval = false
	p.mu.Unlock()
}

func (p *Parser) CommandCatalog() []harness.CommandDescriptor {
	return []harness.CommandDescriptor{{
		ID: "fake.status", Invocation: "/fake-status", Label: "Fake status",
		Description: "Ask the fake harness for its status.", Group: "Session",
		Interaction: harness.CommandSubmit,
	}}
}

func (p *Parser) CommandSequence(commandID, arguments string) ([][]byte, harness.CommandDescriptor, error) {
	command := p.CommandCatalog()[0]
	if commandID != command.ID {
		return nil, harness.CommandDescriptor{}, errors.New("unknown fake semantic command")
	}
	value := command.Invocation
	if strings.TrimSpace(arguments) != "" {
		value += " " + strings.TrimSpace(arguments)
	}
	return [][]byte{[]byte(value + "\r")}, command, nil
}
