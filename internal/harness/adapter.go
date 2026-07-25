package harness

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
	Priority() int
	Match(LaunchSpec) MatchResult
}

// Capability identifies a behavior exposed by an adapter.
type Capability string

const (
	CapabilityRawTerminal Capability = "raw_terminal"
	CapabilityTextInput   Capability = "text_input"
	CapabilitySpecialKeys Capability = "special_keys"
	CapabilityResize      Capability = "resize"
	CapabilityInterrupt   Capability = "interrupt"
	CapabilityTerminate   Capability = "terminate"
)

// CapabilityProvider is implemented by adapters that can list their features.
type CapabilityProvider interface {
	Capabilities() []Capability
}
