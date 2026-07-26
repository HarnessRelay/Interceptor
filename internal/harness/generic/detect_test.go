package generic

import (
	"testing"

	"github.com/harnessrelay/interceptor/internal/events"
)

func TestDetectApprovalHeuristic(t *testing.T) {
	event, ok := DetectApproval("approval required\napprove? [y/N]", "/bin/sh", "/tmp/project")
	if !ok {
		t.Fatal("DetectApproval returned ok=false")
	}
	if event.Type != events.TypeApprovalRequired {
		t.Fatalf("event type = %q, want approval.required", event.Type)
	}
	data := event.Data.(map[string]any)
	if data["confidence"] != "heuristic" {
		t.Fatalf("confidence = %v, want heuristic", data["confidence"])
	}
	actions := data["actions"].([]map[string]any)
	if len(actions) != 1 || actions[0]["id"] != "open_terminal" {
		t.Fatalf("actions = %+v", data["actions"])
	}
}

func TestDetectApprovalIgnoresNormalOutput(t *testing.T) {
	if _, ok := DetectApproval("plain terminal output", "/bin/sh", ""); ok {
		t.Fatal("DetectApproval matched plain output")
	}
}
