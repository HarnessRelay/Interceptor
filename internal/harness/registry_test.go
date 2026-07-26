package harness

import "testing"

type testAdapter struct {
	id         string
	priority   int
	confidence float64
	matched    bool
}

func (a testAdapter) ID() string                 { return a.id }
func (a testAdapter) Name() string               { return a.id }
func (a testAdapter) Priority() int              { return a.priority }
func (a testAdapter) Capabilities() []Capability { return nil }
func (a testAdapter) Match(LaunchSpec) MatchResult {
	return MatchResult{Matched: a.matched, Confidence: a.confidence}
}

func TestRegistrySelectsHighestPriorityMatch(t *testing.T) {
	registry := NewRegistry(
		testAdapter{id: "generic", priority: -100, confidence: 0.1, matched: true},
		testAdapter{id: "specific", priority: 10, confidence: 0.9, matched: true},
	)

	adapter, _, ok := registry.Select(LaunchSpec{Command: "tool"})
	if !ok {
		t.Fatal("Select returned ok=false")
	}
	if adapter.ID() != "specific" {
		t.Fatalf("selected adapter = %q, want specific", adapter.ID())
	}
}

func TestRegistryReturnsFalseWhenNothingMatches(t *testing.T) {
	registry := NewRegistry(testAdapter{id: "nope", priority: 1, matched: false})

	if _, _, ok := registry.Select(LaunchSpec{Command: "tool"}); ok {
		t.Fatal("Select returned ok=true")
	}
}
