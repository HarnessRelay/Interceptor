# HarnessRelay API

Base URL:

```text
http://127.0.0.1:8765/api/v1
```

## Authentication

Health is public. Session control APIs and WebSocket require authentication.

Clients are classified as `host` (the daemon machine), `lan` (direct network
connection), or `tunnel` (arriving through cloudflared, identified by
`CF-Connecting-IP` on a loopback connection). The static daemon token —
including logins that mint host cookie sessions — only works from `host`.
Remote clients must use device credentials.

For CLI/tests on the host machine:

```http
Authorization: Bearer <HARNESSRELAY_TOKEN>
```

For the dashboard on the host:

1. `POST /auth/login` with `{ "token": "..." }`.
2. Server sets an HttpOnly `harnessrelay_session` cookie.
3. Response includes `csrf_token`.
4. Browser sends `X-CSRF-Token` on unsafe methods.

Paired mobile devices sign every request with Ed25519 headers
(`X-Device-ID`, `X-Signature`, `X-Timestamp`; see `Docs/Auth.md`). Remote web
browsers pair through `POST /pairing/web`, receive a one-time `hrk_` device
token, and exchange it via `POST /auth/device-session` for a device cookie
session that works everywhere, including WebSockets.

`GET /auth/status` reports `client_class` and `token_login_allowed` so
clients know which flow to use. Unexpected browser `Origin` headers are
rejected. Login attempts are rate limited per client IP (5 per minute);
pairing submissions are limited to 10 per minute.

## REST

```text
GET    /health
GET    /auth/status
POST   /auth/login
POST   /auth/device-session

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

POST   /pairing/request
GET    /pairing/status
POST   /pairing/web
GET    /pairing/web/{id}
GET    /pairing/requests
POST   /pairing/accept
POST   /pairing/reject
GET    /pairing/devices
DELETE /pairing/devices/{id}
PUT    /pairing/devices/{id}/name

GET    /network/settings
PUT    /network/settings
POST   /network/allow
DELETE /network/allow
POST   /network/ban
DELETE /network/ban
GET    /network/clients
PUT    /network/clients/{key}/name

GET    /tunnel/available
GET    /tunnel
POST   /tunnel/start
POST   /tunnel/stop
GET    /tunnel/config
PUT    /tunnel/config
GET    /tunnel/binary
POST   /tunnel/download
GET    /tunnel/logs
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

Shim-launched managed sessions add generic origin metadata:

```json
{
  "origin": "shim",
  "origin_backend": "pty",
  "shim_name": "codex",
  "real_binary": "/home/user/.local/bin/codex",
  "attachable": true
}
```

These are additive session fields. `origin` is omitted for ordinary explicit
session creation. Direct shim bypass/fallback creates no session. The initial
accepted shim session backend is `pty`; `tmux` is reserved for the documented
future daemon-owned backend.

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

## Tunnel (Remote Access)

All tunnel endpoints require authentication. The tunnel runs a local
`cloudflared` child process bound to the daemon's shutdown context; it is
opt-in, never started automatically, and the daemon keeps binding to
`127.0.0.1`.

`GET /tunnel` returns the current state:

```json
{ "status": "running", "url": "https://random-name.trycloudflare.com" }
```

Statuses: `stopped`, `starting`, `running`, `error` (with `error` detail).
`POST /tunnel/start` and `POST /tunnel/stop` control the process.

Two modes are supported via `PUT /tunnel/config`:

```json
{ "mode": "quick" }
{ "mode": "token", "token": "<cloudflare tunnel token>", "hostname": "https://relay.example.com" }
```

- `quick` (default) runs a zero-config Cloudflare Quick Tunnel with a random
  `trycloudflare.com` URL that changes on every start. Cloudflare documents
  quick tunnels as development/testing-only with a 200 concurrent request cap.
- `token` runs a named, remotely-managed tunnel via
  `cloudflared tunnel run --token` for a stable URL. `hostname` is an optional
  informational label; the real public hostname is configured in Cloudflare.
- The stored token is never returned; `GET /tunnel/config` reports
  `{ "mode": "...", "hostname": "...", "token_set": true }`.
- Configuration can only be changed while the tunnel is stopped (`409` otherwise).

The cloudflared binary is app-managed:

- `GET /tunnel/binary` reports the resolved binary path, source
  (`env`/`managed`/`path`/`common`), version, and the managed path
  (`~/.local/share/harnessrelay/bin/cloudflared`, XDG-aware).
- `POST /tunnel/download` downloads the latest official release from
  Cloudflare's GitHub releases with digest verification, a `--version` sanity
  check, and an atomic swap that keeps the previous binary as `cloudflared.previous`
  for rollback. Failures never damage an existing installation.
- `install.sh` performs a best-effort initial download; the dashboard can
  download or update later on demand.

`GET /tunnel/logs` returns the most recent cloudflared output lines (bounded
ring buffer) for the dashboard debug console.

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

## Pairing v2 (6-digit verification codes)

Every pairing request generates a 6-digit code shown on both the requesting
device and the daemon approval dialog; the user compares them before
accepting.

`POST /pairing/request` (mobile devices; unauthenticated, rate limited):

```json
{
  "device_id": "dev-123",
  "device_name": "Pixel 9",
  "platform": "android",
  "public_key": "<base64 ed25519 public key>"
}
```

Response includes the code the device must display:

```json
{ "status": "pending", "code": "482913" }
```

Web-browser pairing (`POST /pairing/web`, unauthenticated, same-origin):

```json
{ "device_name": "Kitchen Tablet" }
```

Response (private to the requester):

```json
{ "request_id": "pr_…", "code": "482913", "secret": "…" }
```

Poll with `GET /pairing/web/{id}` and header `X-Pairing-Secret: <secret>`:
returns `{"status":"pending"}` until the daemon owner accepts or rejects.
On acceptance the poll returns the one-time device token:

```json
{ "status": "accepted", "device_token": "hrk_…" }
```

The token is delivered exactly once. Exchange it for a device cookie session:

```http
POST /auth/device-session
Authorization: Bearer hrk_…
```

→ sets the session cookie and returns `{ "authenticated": true, "csrf_token": "…" }`.
Device sessions are valid from host, LAN, and tunnel clients.

Pending requests (`GET /pairing/requests`, authenticated) include `type`
(`mobile` | `web`) and `code`. Paired devices may be renamed via
`PUT /pairing/devices/{id}/name` with `{ "name": "…" }` (empty resets).

## Network Access Controls

All authenticated. Applies to LAN/tunnel clients; the host machine is never
filtered.

`GET /network/settings`:

```json
{
  "remote_access_enabled": true,
  "lan_ips": ["192.168.1.20"],
  "allowlist": ["192.168.1.0/24"],
  "banlist": []
}
```

- `PUT /network/settings` `{ "remote_access_enabled": false }` blocks every
  non-host client except `/api/v1/health`.
- The allowlist (persisted in `allowed_ips.txt`) gates direct LAN
  connections; an empty list allows the whole LAN.
- The banlist (persisted in `banned_ips.txt`) blocks the real client IP on
  both LAN and tunnel connections.
- `POST /network/allow` / `DELETE /network/allow` and
  `POST /network/ban` / `DELETE /network/ban` take `{ "entry": "ip-or-cidr" }`
  and return the updated settings.

`GET /network/clients` lists recently seen clients with IP, class, MAC (from
the ARP table when known), reverse-DNS hostname, active connection count,
and any custom name. `PUT /network/clients/{key}/name` renames (empty
resets); the key prefers the MAC address so names survive IP changes.
