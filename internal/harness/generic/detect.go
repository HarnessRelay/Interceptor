package generic

import (
	"strings"

	"github.com/harnessrelay/interceptor/internal/events"
)

func DetectApproval(text, command, cwd string) (events.Event, bool) {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "approval required") && !strings.Contains(lower, "approve?") {
		return events.Event{}, false
	}
	return events.Event{
		Type: events.TypeApprovalRequired,
		Data: map[string]any{
			"title":       "Approval-like prompt detected",
			"summary":     "The generic adapter found text that looks like an approval prompt.",
			"description": "Review the raw terminal before taking any action.",
			"confidence":  "heuristic",
			"context": map[string]any{
				"command": command,
				"cwd":     cwd,
			},
			"actions": []map[string]any{
				{
					"id":                "approve_once",
					"label":             "Approve once",
					"style":             "primary",
					"requires_event_id": true,
					"version":           1,
				},
				{
					"id":                "deny",
					"label":             "Deny",
					"style":             "danger",
					"requires_event_id": true,
					"version":           1,
				},
			},
		},
	}, true
}
