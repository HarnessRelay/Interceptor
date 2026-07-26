package generic

import "github.com/harnessrelay/interceptor/internal/harness"

const adapterID = "generic"

// Adapter is the mandatory raw terminal fallback adapter.
type Adapter struct{}

func New() Adapter {
	return Adapter{}
}

func NewRegistry(adapters ...harness.Adapter) *harness.Registry {
	all := append([]harness.Adapter{}, adapters...)
	all = append(all, New())
	return harness.NewRegistry(all...)
}

func (Adapter) ID() string {
	return adapterID
}

func (Adapter) Name() string {
	return "Generic"
}

func (Adapter) Priority() int {
	return -1000
}

func (Adapter) Match(harness.LaunchSpec) harness.MatchResult {
	return harness.MatchResult{
		Matched:    true,
		Confidence: 0.1,
		Reason:     "raw terminal fallback",
	}
}

func (Adapter) Capabilities() []harness.Capability {
	return []harness.Capability{
		harness.CapabilityRawTerminal,
		harness.CapabilityChatProjection,
		harness.CapabilityPromptSubmit,
		harness.CapabilityTextInput,
		harness.CapabilitySpecialKeys,
		harness.CapabilityResize,
		harness.CapabilityInterrupt,
		harness.CapabilityTerminate,
	}
}

func (Adapter) PromptBytes(text string, _ []byte) []byte {
	return append([]byte(text), '\r')
}
