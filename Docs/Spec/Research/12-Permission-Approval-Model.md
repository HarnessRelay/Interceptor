# Permission And Approval Model

Date: 2026-07-26

## Purpose

Normalize a user decision without erasing harness-specific meaning. The common
layer transports and validates requests; the adapter discovers the request,
advertises valid choices, and executes the chosen action.

## Request Shape

`events.ApprovalRequired` represents both approvals and permissions:

```text
operation_kind
operation_detail
command
file_path
tool_name
working_directory
risk_level
adapter_source
confidence
prompt
actions
blocks_prompt
requires_terminal
```

Suggested operation kinds are `shell_command`, `file_edit`, `file_read`,
`network`, `workspace_trust`, `model_change`, `tool_call`, and `unknown`.
Unknown values remain forward-compatible.

`confidence` describes extraction confidence, not safety. A low-confidence
Generic event exposes only `open_terminal`.

## Action Shape

Adapters dynamically provide `SemanticAction` values. Common action kinds
include:

- `approval`: an event-bound harness decision
- `ui`: local UI behavior such as `open_terminal`
- `adapter_specific`: a native action with adapter-defined semantics

Stable conceptual action IDs include `approve_once`, `deny`, `open_terminal`,
`continue_in_terminal`, `edit_request`, and `view_details`, but adapters may use
namespaced IDs. The common UI never invents a choice.

## Execution Result

The adapter returns:

```go
type ActionResult struct {
    Resolution    string
    Status        string
    Detail        string
    TerminalInput []byte
    ClearsPending bool
    Events        []events.Event
}
```

This avoids assuming every action is a denial or PTY key. `Resolution` and
`Detail` are adapter-owned. Empty detail receives a neutral
`"{adapter name} completed the requested action."` fallback.

## Blocking And Terminal Fallback

`blocks_prompt` prevents new semantic prompts while the request is active.
`requires_terminal` tells clients that no reliable semantic resolution exists.
Any adapter can emit both fields; common code never checks adapter ID or a
specific operation kind. Raw terminal input resolves a terminal-only pending
decision as `handled_in_terminal`.

## Validation Invariants

- No action runs automatically.
- The API requires active session, event ID, advertised action ID, and version.
- Resolved, superseded, unknown, wrong-version, and cross-session actions fail
  closed.
- UI-only actions never write to the PTY.
- Adapter execution occurs only after common validation.
- Persistent/always approval is never inferred from a generic action.
- Raw terminal remains available for verification and uncertain decisions.

## Cross-Harness Mapping

| Common concept | Codex | OpenCode | Grok |
| --- | --- | --- | --- |
| approve once | documented decision when exposed | `once` | ask-mode allow once |
| persistent/session grant | adapter-specific, withheld by default | `always` patterns | remembered/config allow |
| deny | deny/cancel mapping | `reject` | deny/reject |
| operation | command/tool/workspace trust | tool + pattern/path | Bash/Edit/Read/MCP/web tool |
| terminal-only | trust or unverified TUI choice | unbound TUI prompt | unbound TUI/plan review |

The mapping is intentionally lossy only at display vocabulary. Native IDs,
suggested patterns, and exact response payloads remain adapter data.

