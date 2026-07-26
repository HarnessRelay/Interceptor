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
	data, ok := event.Data.(events.ApprovalRequired)
	if !ok {
		t.Fatalf("payload type = %T, want events.ApprovalRequired", event.Data)
	}
	if data.Confidence >= 0.5 || !data.RequiresTerminal || data.BlocksPrompt {
		t.Fatalf("heuristic approval = %+v", data)
	}
	if len(data.Actions) != 1 || data.Actions[0].ID != "open_terminal" {
		t.Fatalf("actions = %+v", data.Actions)
	}
}

func TestDetectApprovalIgnoresNormalOutput(t *testing.T) {
	if _, ok := DetectApproval("plain terminal output", "/bin/sh", ""); ok {
		t.Fatal("DetectApproval matched plain output")
	}
}
