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

// ErrorPayload carries error details.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func newEventID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
