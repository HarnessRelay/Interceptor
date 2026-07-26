package harness

import "github.com/harnessrelay/interceptor/internal/events"

// LaunchSpec describes the command being matched by harness adapters.
type LaunchSpec struct {
	Command string
	Args    []string
	WorkDir string
	Env     []string
}

// MatchResult describes how confidently an adapter matches a launch spec.
type MatchResult struct {
	Matched    bool
	Confidence float64
	Reason     string
}

// Adapter is the core capability every harness adapter must provide.
type Adapter interface {
	ID() string
	Name() string
	Priority() int
	Match(LaunchSpec) MatchResult
	Capabilities() []Capability
}

// Capability identifies a behavior exposed by an adapter.
type Capability string

const (
	CapabilityRawTerminal       Capability = "raw_terminal"
	CapabilityChatProjection    Capability = "chat_projection"
	CapabilitySemanticChat      Capability = "semantic_chat"
	CapabilityPromptSubmit      Capability = "prompt_submit"
	CapabilityApprovalDetection Capability = "approval_detection"
	CapabilityApprovalActions   Capability = "approval_actions"
	CapabilityStatusDetection   Capability = "status_detection"
	CapabilityMetadataDetection Capability = "metadata_detection"
	CapabilityNoiseFiltering    Capability = "noise_filtering"
	CapabilityTextInput         Capability = "text_input"
	CapabilitySpecialKeys       Capability = "special_keys"
	CapabilityResize            Capability = "resize"
	CapabilityInterrupt         Capability = "interrupt"
	CapabilityTerminate         Capability = "terminate"
	CapabilityCommandCatalog    Capability = "command_catalog"
	CapabilityCommandInvoke     Capability = "command_invoke"
)

// CommandInteraction tells clients how invoking a harness command affects the UI.
type CommandInteraction string

const (
	CommandSubmit             CommandInteraction = "submit"
	CommandSubmitThenTerminal CommandInteraction = "submit_then_terminal"
	CommandPrefillTerminal    CommandInteraction = "prefill_terminal"
	CommandInsert             CommandInteraction = "insert"
)

// CommandDescriptor is an adapter-owned, version-verified harness command.
type CommandDescriptor struct {
	ID               string             `json:"id"`
	Invocation       string             `json:"invocation"`
	Label            string             `json:"label"`
	Description      string             `json:"description"`
	Group            string             `json:"group"`
	Interaction      CommandInteraction `json:"interaction"`
	ArgumentHint     string             `json:"argument_hint,omitempty"`
	Danger           bool               `json:"danger,omitempty"`
	Availability     string             `json:"availability,omitempty"`
	AvailabilityNote string             `json:"availability_note,omitempty"`
}

// CommandCatalogProvider exposes only commands verified for the active harness version.
type CommandCatalogProvider interface {
	CommandCatalog() []CommandDescriptor
}

// CommandSequencer maps a catalog command to ordered PTY writes.
type CommandSequencer interface {
	CommandSequence(commandID, arguments string) ([][]byte, CommandDescriptor, error)
}

// TerminalUpdate is the raw input offered to a session-scoped semantic parser.
type TerminalUpdate struct {
	Chunk    []byte
	Snapshot []byte
	Command  string
	WorkDir  string
	Rows     uint16
	Cols     uint16
}

// Parser classifies terminal updates into semantic events.
type Parser interface {
	Process(TerminalUpdate) []events.Event
}

// IdleEventProvider emits semantics that are only reliable after terminal
// output has settled, such as a completed response reconstructed from a TUI.
type IdleEventProvider interface {
	OnIdle() []events.Event
}

// ParserProvider creates isolated parser state for each session.
type ParserProvider interface {
	NewParser() Parser
}

// PromptSubmitter supplies the adapter-specific PTY sequence for a prompt.
type PromptSubmitter interface {
	PromptBytes(text string, terminalSnapshot []byte) []byte
}

// PromptSequencer provides ordered PTY writes when a harness must process
// text before receiving its submit key.
type PromptSequencer interface {
	PromptSequence(text string, terminalSnapshot []byte) [][]byte
}

// ActionHandler maps a currently valid semantic action to PTY input.
type ActionHandler interface {
	ActionBytes(actionID string) ([]byte, bool)
}

// ActionObserver lets a session-scoped parser discard state resolved outside
// terminal output parsing.
type ActionObserver interface {
	ActionResolved(actionID string)
}
