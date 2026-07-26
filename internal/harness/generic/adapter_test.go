package generic

import (
	"bytes"
	"testing"

	"github.com/harnessrelay/interceptor/internal/harness"
)

func TestGenericAdapterAlwaysMatches(t *testing.T) {
	adapter := New()
	match := adapter.Match(harness.LaunchSpec{Command: "unknown-tool"})
	if !match.Matched {
		t.Fatal("generic adapter did not match unknown command")
	}
	if adapter.ID() != "generic" {
		t.Fatalf("ID = %q, want generic", adapter.ID())
	}
}

func TestGenericPromptBytesSubmitsCarriageReturn(t *testing.T) {
	if got := New().PromptBytes("hello", nil); !bytes.Equal(got, []byte("hello\r")) {
		t.Fatalf("PromptBytes = %q, want text plus carriage return", got)
	}
}

func TestGenericRegistryFallsBackToGeneric(t *testing.T) {
	registry := NewRegistry()
	adapter, match, ok := registry.Select(harness.LaunchSpec{Command: "unknown-tool"})
	if !ok {
		t.Fatal("Select returned ok=false")
	}
	if adapter.ID() != "generic" {
		t.Fatalf("selected adapter = %q, want generic", adapter.ID())
	}
	if !match.Matched {
		t.Fatal("match was not marked matched")
	}
}

func TestGenericCapabilities(t *testing.T) {
	caps := New().Capabilities()
	want := map[harness.Capability]bool{
		harness.CapabilityRawTerminal: true,
		harness.CapabilityTextInput:   true,
		harness.CapabilitySpecialKeys: true,
		harness.CapabilityResize:      true,
		harness.CapabilityInterrupt:   true,
		harness.CapabilityTerminate:   true,
	}
	for _, cap := range caps {
		delete(want, cap)
	}
	if len(want) > 0 {
		t.Fatalf("missing capabilities: %v", want)
	}
}
