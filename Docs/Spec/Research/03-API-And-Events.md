# API And Event Schema

## Recommendation

Use REST for request/response operations and WebSocket for live event streaming. Keep the common API harness-agnostic: sessions expose capabilities and backend-provided actions, while raw terminal input/output always remains available.

Base path: `/api/v1`

WebSocket path: `/api/v1/ws`

## Alternatives Considered

| Alternative | Decision | Reason |
| --- | --- | --- |
| REST only | Reject | Polling cannot handle low-latency terminal output or lifecycle events cleanly. |
| WebSocket only | Reject | State-changing operations such as terminate, resize, and semantic actions need normal HTTP auth, CSRF, status codes, and audit boundaries. |
| Server-Sent Events plus REST input | Defer | SSE is useful for one-way events, but Stage 1 also needs authenticated bidirectional control and future client messages. |
| Harness-specific API paths | Reject | Would violate the common API requirement and make the dashboard hardcode individual tools. |

## REST Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Daemon health and version. |
| `GET` | `/sessions` | List sessions. |
| `POST` | `/sessions` | Create a session. |
| `GET` | `/sessions/{id}` | Read session metadata. |
| `DELETE` | `/sessions/{id}` | Terminate/delete session according to policy. |
| `POST` | `/sessions/{id}/input` | Write raw/text input to PTY. |
| `POST` | `/sessions/{id}/resize` | Resize PTY. |
| `POST` | `/sessions/{id}/interrupt` | Send Ctrl+C/SIGINT strategy. |
| `POST` | `/sessions/{id}/terminate` | Gracefully terminate session process group. |
| `POST` | `/sessions/{id}/kill` | Force kill, behind stronger confirmation. |
| `GET` | `/sessions/{id}/snapshot` | Fetch reconnect snapshot/replay chunks. |
| `GET` | `/sessions/{id}/events` | Fetch paginated event history. |
| `POST` | `/sessions/{id}/actions/{action_id}` | Execute backend-provided semantic action. |
| `GET` | `/harnesses` | List known adapters/harness definitions. |
| `GET` | `/capabilities` | List daemon/API capabilities. |

## Request And Response Examples

### Create Session

Request:

```json
{
  "name": "payroll-api",
  "harness_type": "generic",
  "command": "opencode",
  "args": [],
  "cwd": "/home/user/projects/payroll-api",
  "env": {
    "NO_COLOR": "0"
  },
  "terminal": {
    "rows": 40,
    "cols": 120
  }
}
```

Response:

```json
{
  "session": {
    "id": "ses_01JZABCDEF123",
    "name": "payroll-api",
    "harness_type": "generic",
    "adapter_id": "generic",
    "command": "opencode",
    "args": [],
    "cwd": "/home/user/projects/payroll-api",
    "status": "starting",
    "pid": 12345,
    "pgid": 12345,
    "terminal": { "rows": 40, "cols": 120 },
    "created_at": "2026-07-25T10:00:00Z",
    "updated_at": "2026-07-25T10:00:00Z"
  }
}
```

### Input

```json
{
  "mode": "raw",
  "encoding": "base64",
  "data": "bHMgLWxhDQo="
}
```

Rules:

- `raw` writes decoded bytes directly.
- `text` encodes as UTF-8.
- Reject requests above configured byte limit.
- Do not log payload by default.

### Resize

```json
{ "rows": 42, "cols": 132 }
```

### Interrupt

```json
{
  "strategy": "ctrl_c"
}
```

Valid strategies:

- `ctrl_c` default, writes `0x03`.
- `sigint_group`, optional fallback, sends SIGINT to process group.

### Terminate

```json
{
  "grace_ms": 5000,
  "confirmation": "payroll-api"
}
```

### Semantic Action

```json
{
  "event_id": "evt_01JZAPPROVAL",
  "action_version": 1,
  "params": {
    "reason": "User approved after reviewing command"
  }
}
```

Response:

```json
{
  "result": {
    "status": "accepted",
    "event_id": "evt_01JZAPPROVAL",
    "action_id": "approve_once"
  }
}
```

## WebSocket Event Envelope

Every event uses the same envelope:

```json
{
  "id": "evt_01JZXYZ",
  "type": "terminal.output",
  "version": 1,
  "session_id": "ses_01JZABC",
  "seq": 42,
  "ts": "2026-07-25T10:00:01.000Z",
  "source": "pty",
  "data": {}
}
```

Fields:

- `id`: globally unique event ID.
- `type`: stable event type string.
- `version`: event payload schema version.
- `session_id`: omitted only for daemon-wide events.
- `seq`: per-session monotonic sequence for session events.
- `ts`: RFC3339 timestamp.
- `source`: `daemon`, `pty`, `adapter.generic`, or harness adapter ID.
- `data`: event-specific object.

Clients should ignore unknown event types and unknown fields.

## Core Event Types

### Terminal Output

```json
{
  "type": "terminal.output",
  "data": {
    "encoding": "base64",
    "bytes": "G1szMm1PSxsWzBtDQo="
  }
}
```

### Session Created

```json
{
  "type": "session.created",
  "data": {
    "session": { "id": "ses_123", "status": "starting" }
  }
}
```

### Session Status Changed

```json
{
  "type": "session.status_changed",
  "data": {
    "previous_status": "running",
    "status": "waiting_for_approval",
    "confidence": "heuristic"
  }
}
```

### Session Exited

```json
{
  "type": "session.exited",
  "data": {
    "exit_code": 0,
    "signal": null,
    "reason": "process_exit"
  }
}
```

### Error

```json
{
  "type": "error",
  "data": {
    "code": "session_not_found",
    "message": "Session not found",
    "retryable": false
  }
}
```

### Semantic Action Event

Generic shape:

```json
{
  "type": "semantic.action_available",
  "data": {
    "title": "Harness action available",
    "description": "Backend detected an action the user may take.",
    "confidence": "adapter",
    "actions": [
      {
        "id": "open_palette",
        "label": "Open command palette",
        "style": "secondary",
        "requires_event_id": false
      }
    ]
  }
}
```

### Approval Required

```json
{
  "type": "approval.required",
  "data": {
    "title": "Command approval requested",
    "summary": "Harness wants to run a command",
    "confidence": "adapter",
    "context": {
      "command": "git status --short",
      "cwd": "/home/user/projects/payroll-api",
      "risk": "low"
    },
    "actions": [
      {
        "id": "approve_once",
        "label": "Approve once",
        "style": "primary",
        "requires_event_id": true,
        "version": 1
      },
      {
        "id": "deny",
        "label": "Deny",
        "style": "danger",
        "requires_event_id": true,
        "version": 1
      }
    ]
  }
}
```

The frontend renders supplied actions. It must not invent approval semantics from the harness name.

## Stale Action Rejection Model

Event-bound actions must include `event_id` and `action_version`.

Reject action if:

- Event ID is unknown.
- Event is not active.
- Action ID is not present on that event.
- Event has already been resolved.
- Session status no longer matches the event.
- `action_version` does not match.
- Adapter says the visible terminal state no longer matches.

Return `409 Conflict`:

```json
{
  "error": {
    "code": "stale_action",
    "message": "The action is no longer valid for the current session state.",
    "event_id": "evt_01JZAPPROVAL"
  }
}
```

## Versioning Approach

- URL version: `/api/v1`.
- Event envelope has `version`.
- Event payload changes that remove/rename fields require a new event payload version.
- Additive fields are allowed.
- Frontend should display unknown actions as disabled only if required fields are missing.
- Keep action IDs stable per adapter but never globalize harness-specific meanings.

## Risks And Limitations

- JSON base64 terminal output is simple but less efficient than binary frames.
- Heuristic semantic events may be wrong; include confidence.
- Event replay needs careful `seq` handling to avoid duplicates after reconnect.
- REST action and WebSocket event delivery can race; stale action rejection is mandatory.

## Acceptance Criteria For Later Implementation

- API can create, list, inspect, interrupt, resize, terminate, and kill generic sessions.
- WebSocket emits terminal output and lifecycle events with monotonic per-session sequence.
- Frontend can render action cards without harness-specific code.
- Stale event-bound actions return `409`.
- Unknown harnesses still work through raw terminal endpoints.
- API docs include examples matching implemented structs.

## Required Tests

- REST create/session/list/detail schema tests.
- Input rejects oversized payloads and malformed base64.
- Resize validates bounds.
- Interrupt and terminate publish audit/event records.
- WebSocket event envelope schema is stable.
- Event replay after `after_seq` returns expected event sequence.
- Stale action rejection cases cover resolved event, unknown event, version mismatch, and status mismatch.

## Sources

- [OWASP WebSocket Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html)
- [OpenCode server documentation](https://dev.opencode.ai/docs/server/)
- [OpenAI Codex app-server approval notes](https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md)
