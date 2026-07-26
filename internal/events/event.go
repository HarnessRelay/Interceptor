package events

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Type identifies the kind of event.
type Type string

const (
	TypeTerminalOutput       Type = "terminal.output"
	TypeTerminalSnapshot     Type = "terminal.snapshot"
	TypeSessionCreated       Type = "session.created"
	TypeSessionUpdated       Type = "session.updated"
	TypeSessionExited        Type = "session.exited"
	TypeSessionStatusChanged Type = "session.status_changed"
	TypeApprovalRequired     Type = "approval.required"
	TypeApprovalResolved     Type = "approval.resolved"
	TypeHarnessDetected      Type = "harness.detected"
	TypeHarnessStatus        Type = "harness.status"
	TypeHarnessMetadata      Type = "harness.metadata"
	TypeChatUserMessage      Type = "chat.user_message"
	TypeChatAssistantMessage Type = "chat.assistant_message"
	TypeChatSystemMessage    Type = "chat.system_message"
	TypeTerminalNoisyOutput  Type = "terminal.noisy_output"
	TypeAdapterWarning       Type = "adapter.warning"
	TypeAdapterError         Type = "adapter.error"
	TypeActionCompleted      Type = "action.completed"
	TypeActionFailed         Type = "action.failed"
	TypeError                Type = "error"
)

// Event is the standard envelope for all events.
type Event struct {
	ID        string    `json:"id"`
	Type      Type      `json:"type"`
	SessionID string    `json:"session_id,omitempty"`
	Sequence  uint64    `json:"seq"`
	Timestamp time.Time `json:"ts"`
	Data      any       `json:"data,omitempty"`
}

// TerminalOutput carries raw PTY output bytes.
type TerminalOutput struct {
	Data []byte `json:"data"`
}

// TerminalSnapshot carries a snapshot of terminal scrollback.
type TerminalSnapshot struct {
	Data []byte `json:"data"`
}

// SessionStatusChanged carries old and new session status.
type SessionStatusChanged struct {
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// SessionExited carries exit information for a completed session.
type SessionExited struct {
	ExitCode int    `json:"exit_code"`
	Signal   *int   `json:"signal,omitempty"`
	Reason   string `json:"reason"`
}

// SessionCreated is an immutable session-created event payload.
type SessionCreated struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name,omitempty"`
	HarnessType         string    `json:"harness_type"`
	AdapterID           string    `json:"adapter_id"`
	AdapterName         string    `json:"adapter_name"`
	AdapterCapabilities []string  `json:"adapter_capabilities"`
	Command             string    `json:"command"`
	Args                []string  `json:"args"`
	WorkDir             string    `json:"cwd"`
	Status              string    `json:"status"`
	StartedAt           time.Time `json:"created_at"`
}

// ErrorPayload carries error details.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HarnessDetected identifies the adapter selected for a session.
type HarnessDetected struct {
	AdapterID   string  `json:"adapter_id"`
	HarnessName string  `json:"harness_name"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason,omitempty"`
}

// HarnessStatus is an adapter-derived operational state.
type HarnessStatus struct {
	Status     string  `json:"status"`
	Detail     string  `json:"detail,omitempty"`
	Confidence float64 `json:"confidence"`
}

// HarnessMetadata contains optional adapter-derived display metadata.
type HarnessMetadata struct {
	Model      string  `json:"model,omitempty"`
	WorkDir    string  `json:"working_directory,omitempty"`
	Version    string  `json:"version,omitempty"`
	Confidence float64 `json:"confidence"`
}

// ChatMessage is a semantic chat message that is safe to render outside the terminal.
type ChatMessage struct {
	MessageID  string  `json:"message_id,omitempty"`
	Role       string  `json:"role"`
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// TerminalNoiseSuppressed explains why raw output was not projected into Chat Mode.
type TerminalNoiseSuppressed struct {
	Reason string `json:"reason"`
}

// AdapterNotice is a user-readable warning or error from semantic parsing.
type AdapterNotice struct {
	Description string `json:"description"`
	Source      string `json:"source,omitempty"`
}

// SemanticAction is a backend-defined action rendered by clients.
type SemanticAction struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Kind            string `json:"kind"`
	Style           string `json:"style,omitempty"`
	Danger          bool   `json:"danger,omitempty"`
	RequiresEventID bool   `json:"requires_event_id"`
	Version         int    `json:"version"`
}

// ApprovalRequired describes an adapter-derived approval or permission request.
type ApprovalRequired struct {
	OperationKind    string           `json:"operation_kind"`
	OperationDetail  string           `json:"operation_detail,omitempty"`
	Command          string           `json:"command,omitempty"`
	FilePath         string           `json:"file_path,omitempty"`
	ToolName         string           `json:"tool_name,omitempty"`
	WorkingDirectory string           `json:"working_directory,omitempty"`
	RiskLevel        string           `json:"risk_level,omitempty"`
	AdapterSource    string           `json:"adapter_source,omitempty"`
	Prompt           string           `json:"prompt"`
	Actions          []SemanticAction `json:"actions"`
	Confidence       float64          `json:"confidence"`
	BlocksPrompt     bool             `json:"blocks_prompt,omitempty"`
	RequiresTerminal bool             `json:"requires_terminal,omitempty"`
}

// ApprovalResolved records the terminal action used to resolve an approval.
type ApprovalResolved struct {
	ApprovalEventID string `json:"approval_event_id"`
	ActionID        string `json:"action_id"`
	Resolution      string `json:"resolution"`
}

// AdapterMessage is a warning or error emitted by an adapter.
type AdapterMessage struct {
	Message    string  `json:"message"`
	Confidence float64 `json:"confidence,omitempty"`
}

func newEventID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
