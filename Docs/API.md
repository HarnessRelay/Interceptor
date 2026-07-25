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

Input:

```json
{ "mode": "raw", "encoding": "base64", "data": "ZWNobyBoaQo=" }
```

Named special keys:

```json
{ "mode": "key", "key": "Enter" }
```

Supported names include `Enter`, `Escape`, `Tab`, `Backspace`, `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight`, and `CtrlC`.

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

Semantic actions currently validate event/action freshness and return `501` for recognized actions because no real harness action executor is implemented yet. Stale or unknown actions return `409`.

## Dashboard Display Modes

Chat Mode and Terminal Mode are dashboard presentation preferences over the same session APIs. No backend API shape is currently changed for mode selection.

- Chat Mode sends composer submissions to `POST /sessions/{id}/input` as raw PTY bytes followed by Enter.
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
- `error`

Terminal output bytes are JSON base64 data.

The generic adapter may emit `approval.required` when terminal text looks like an approval prompt. These events include `confidence: "heuristic"` and command/cwd context. Raw terminal access remains the source of truth. Action buttons are exposed for UI consistency, but Stage 1 returns `501` until a specific adapter implements reliable approve/deny execution.
