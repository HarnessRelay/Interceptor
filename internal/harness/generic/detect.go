package generic

import (
	"strings"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
)

type Parser struct {
	approvalEmitted bool
}

func (p *Parser) Process(update harness.TerminalUpdate) []events.Event {
	if p.approvalEmitted {
		return nil
	}
	event, ok := DetectApproval(string(update.Snapshot), update.Command, update.WorkDir)
	if !ok {
		return nil
	}
	p.approvalEmitted = true
	return []events.Event{event}
}

func DetectApproval(text, command, cwd string) (events.Event, bool) {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "approval required") && !strings.Contains(lower, "approve?") {
		return events.Event{}, false
	}
	return events.Event{
		Type: events.TypeApprovalRequired,
		Data: events.ApprovalRequired{
			OperationKind:    "unknown",
			OperationDetail:  "The generic adapter found text that looks like an approval prompt.",
			Command:          command,
			WorkingDirectory: cwd,
			AdapterSource:    adapterID,
			Prompt:           "Review the raw terminal before taking any action.",
			Confidence:       0.35,
			RequiresTerminal: true,
			Actions: []events.SemanticAction{
				{
					ID:              "open_terminal",
					Label:           "Open Terminal",
					Kind:            "ui",
					Style:           "secondary",
					RequiresEventID: true,
					Version:         1,
				},
			},
		},
	}, true
}
