package opencode

import (
	"bytes"

	"github.com/harnessrelay/interceptor/internal/harness"
)

const (
	adapterID       = "opencode"
	adapterPriority = 100
	kittyEnter      = "\x1b[13u"
)

// Adapter provides conservative semantic behavior for the OpenCode terminal UI.
type Adapter struct{}

func New() Adapter {
	return Adapter{}
}

func (Adapter) ID() string {
	return adapterID
}

func (Adapter) Name() string {
	return "OpenCode"
}

func (Adapter) Priority() int {
	return adapterPriority
}

func (Adapter) Match(spec harness.LaunchSpec) harness.MatchResult {
	cmd := spec.Command
	// Match "opencode" as the basename of the command path.
	if cmd == "opencode" || cmd == "opencode-ai" {
		return harness.MatchResult{
			Matched:    true,
			Confidence: 1,
			Reason:     "command executable is opencode",
		}
	}
	// Check basename for path-based commands.
	for i := len(cmd) - 1; i >= 0; i-- {
		if cmd[i] == '/' {
			base := cmd[i+1:]
			if base == "opencode" || base == "opencode-ai" {
				return harness.MatchResult{
					Matched:    true,
					Confidence: 1,
					Reason:     "command executable basename is opencode",
				}
			}
			break
		}
	}
	return harness.MatchResult{}
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
		harness.CapabilityNoiseFiltering,
		harness.CapabilityTextInput,
		harness.CapabilitySpecialKeys,
		harness.CapabilityResize,
		harness.CapabilityInterrupt,
		harness.CapabilityTerminate,
		harness.CapabilityCommandCatalog,
		harness.CapabilityCommandInvoke,
	}
}

func (Adapter) NewParser() harness.Parser {
	return &Parser{}
}

func (Adapter) PromptBytes(text string, terminalSnapshot []byte) []byte {
	if bytes.Contains(terminalSnapshot, []byte("\x1b[>4;")) {
		return append([]byte(text), []byte(kittyEnter)...)
	}
	return append([]byte(text), '\r')
}

func (Adapter) ExecuteAction(actionID string) (harness.ActionResult, bool) {
	switch actionID {
	case "opencode.approval_allow":
		return harness.ActionResult{
			Resolution:    "approved",
			Status:        "processing",
			Detail:        "Approval granted; OpenCode is running the operation.",
			TerminalInput: []byte("\r"),
			ClearsPending: true,
		}, true
	case "opencode.approval_allow_always":
		return harness.ActionResult{
			Resolution:    "approved_always",
			Status:        "processing",
			Detail:        "Approval granted for all matching operations; OpenCode is running.",
			TerminalInput: []byte("\t\r"),
			ClearsPending: true,
		}, true
	case "opencode.approval_deny":
		return harness.ActionResult{
			Resolution:    "denied",
			Status:        "processing",
			Detail:        "Approval denied; OpenCode is returning to the conversation.",
			TerminalInput: []byte{0x1b},
			ClearsPending: true,
		}, true
	default:
		return harness.ActionResult{}, false
	}
}
