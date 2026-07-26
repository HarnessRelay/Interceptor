# HarnessRelay API

Base URL:

```text
http://127.0.0.1:8765/api/v1
```

## Authentication

Health is public. Session control APIs and WebSocket require authentication.

For CLI/tests:

```http
Authorization: Bearer <HARNESSRELAY_TOKEN>
```

For the dashboard:

1. `POST /auth/login` with `{ "token": "..." }`.
2. Server sets an HttpOnly `harnessrelay_session` cookie.
3. Response includes `csrf_token`.
4. Browser sends `X-CSRF-Token` on unsafe methods.

Unexpected browser `Origin` headers are rejected.

## REST

```text
GET    /health
GET    /auth/status
POST   /auth/login

GET    /sessions
POST   /sessions
GET    /sessions/{id}
DELETE /sessions/{id}

POST   /sessions/{id}/input
POST   /sessions/{id}/prompt
GET    /sessions/{id}/commands
POST   /sessions/{id}/commands/{command_id}
POST   /sessions/{id}/resize
POST   /sessions/{id}/interrupt
POST   /sessions/{id}/terminate
POST   /sessions/{id}/kill
POST   /sessions/{id}/cleanup

GET    /sessions/{id}/snapshot
GET    /sessions/{id}/events
POST   /sessions/{id}/actions/{action_id}
```

Create session:

```json
{
  "name": "shell",
  "command": "/bin/bash",
  "args": [],
  "cwd": "/home/user/project",
  "terminal": { "rows": 24, "cols": 80 }
}
```

Session responses include the selected adapter:

```json
{
  "adapter_id": "codex",
  "adapter_name": "Codex",
  "adapter_capabilities": [
    "raw_terminal",
    "semantic_chat",
    "prompt_submit",
    "approval_detection"
  ]
}
```

Input:

```json
{ "mode": "raw", "encoding": "base64", "data": "ZWNobyBoaQo=" }
```

Named special keys:

```json
{ "mode": "key", "key": "Enter" }
```

Supported names include `Enter`, `Escape`, `Tab`, `Backspace`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`, and `CtrlC`.

Adapter-aware Chat prompt:

```json
{ "text": "Summarize this repository." }
```

`POST /sessions/{id}/prompt` asks the selected adapter for one atomic prompt
submission sequence. Generic uses carriage return. Codex uses Kitty keyboard
protocol Enter when the TUI has enabled it. Empty prompts are rejected, prompts
are limited to 65536 bytes, and prompts are rejected while an approval action is
pending or while a semantic harness has not reached `idle`. Audit metadata
stores the byte count, not the prompt.

Resize:

```json
{ "rows": 40, "cols": 120 }
```

Kill requires stronger confirmation:

```json
{ "confirmation": "KILL" }
```

Cleanup removes a completed session from the in-memory session list. Running sessions are rejected; terminate or kill them first. Audit records remain in the audit log.

Snapshot returns recent raw replay chunks:

```json
{
  "session_id": "ses_...",
  "rows": 24,
  "cols": 80,
  "latest_seq": 12,
  "history_truncated": false,
  "chunks": [
    { "seq": 12, "encoding": "base64", "bytes": "..." }
  ]
}
```

Semantic actions validate the session, source event, advertised action,
version, and current pending adapter state. The adapter returns a normalized
action result with resolution, status/detail, optional terminal input, and
optional semantic events. Common code does not assume the action is a denial.
Replays, stale events, version mismatches, and unknown actions return `409`. A
recognized action without an adapter executor returns `501`.

`open_terminal` is a UI action and is handled locally by the dashboard. It is
never written to the PTY.

## Dashboard Display Modes

Chat Mode and Terminal Mode are dashboard presentation preferences over the same session APIs. No backend API shape is currently changed for mode selection.

- Chat Mode sends composer submissions to `POST /sessions/{id}/prompt`.
- Sessions with `semantic_chat` render semantic event history and live events;
  they do not project raw terminal chunks directly into assistant messages.
  Codex emits `chat.assistant_message` only after reconstructing the rendered
  response through its terminal screen model.
- Generic sessions retain conservative terminal projection.
- The `/` action menu uses existing input, interrupt, terminate, kill, snapshot, and key endpoints.
- Terminal Mode uses the same snapshot, WebSocket, input, and resize endpoints as the original xterm.js view.
- Mode preference is currently stored in browser-local state per session. The raw PTY session remains the source of truth.

## WebSocket

```text
GET /api/v1/ws?session_id=<optional>&after_seq=<optional>
```

The handshake requires auth and same-origin validation.

Events use the shared envelope:

```json
{
  "id": "evt_...",
  "type": "terminal.output",
  "session_id": "ses_...",
  "seq": 1,
  "ts": "2026-07-26T00:00:00Z",
  "data": {}
}
```

Common event types:

- `terminal.output`
- `session.created`
- `session.status_changed`
- `session.exited`
- `approval.required`
- `approval.resolved`
- `harness.detected`
- `harness.status`
- `harness.metadata`
- `chat.user_message`
- `chat.assistant_message`
- `chat.system_message`
- `terminal.noisy_output`
- `adapter.warning`
- `adapter.error`
- `error`

Terminal output bytes are JSON base64 data.

Assistant messages may include `message_id`. Multiple events with the same
`message_id` are revisions of one semantic turn; clients replace the earlier
content during live delivery and history replay.

The generic adapter may emit typed `approval.required` when terminal text looks
like an approval prompt. These events use low numeric confidence, set
`requires_terminal`, and expose only `open_terminal`.

The Codex adapter emits a typed `approval.required` payload:

```json
{
  "operation_kind": "shell_command",
  "operation_detail": "A command execution needs review.",
  "command": "npm test",
  "working_directory": "/tmp/project",
  "risk_level": "unknown",
  "adapter_source": "codex",
  "prompt": "Codex is asking whether it may run this command.",
  "confidence": 0.95,
  "blocks_prompt": true,
  "actions": [
    {
      "id": "codex.approval_deny",
      "label": "Deny",
      "kind": "approval",
      "requires_event_id": true,
      "version": 1
    },
    {
      "id": "open_terminal",
      "label": "Open Terminal",
      "kind": "ui",
      "requires_event_id": true,
      "version": 1
    }
  ]
}
```

No approve or persistent-approval action is exposed.

Permission/approval payloads may also include `file_path`, `tool_name`, and
`requires_terminal`. Any adapter can emit a terminal-only blocking decision;
the common manager never checks an adapter name or a specific operation kind.
