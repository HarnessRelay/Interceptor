package codex

import (
	"bytes"
	"path/filepath"

	"github.com/harnessrelay/interceptor/internal/harness"
)

const (
	adapterID       = "codex"
	adapterPriority = 100
	kittyEnter      = "\x1b[13u"
)

// Adapter provides conservative semantic behavior for the Codex terminal UI.
type Adapter struct{}

func New() Adapter {
	return Adapter{}
}

func (Adapter) ID() string {
	return adapterID
}

func (Adapter) Name() string {
	return "Codex"
}

func (Adapter) Priority() int {
	return adapterPriority
}

func (Adapter) Match(spec harness.LaunchSpec) harness.MatchResult {
	if filepath.Base(filepath.Clean(spec.Command)) != "codex" {
		return harness.MatchResult{}
	}
	return harness.MatchResult{
		Matched:    true,
		Confidence: 1,
		Reason:     "command executable basename is codex",
	}
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
	}
}

func (Adapter) NewParser() harness.Parser {
	return &Parser{}
}

func (Adapter) PromptBytes(text string, terminalSnapshot []byte) []byte {
	if bytes.Contains(terminalSnapshot, []byte("\x1b[>7u")) {
		return append([]byte(text), []byte(kittyEnter)...)
	}
	return append([]byte(text), '\r')
}

func (Adapter) ActionBytes(actionID string) ([]byte, bool) {
	if actionID != "codex.approval_deny" {
		return nil, false
	}
	return []byte{0x1b}, true
}
