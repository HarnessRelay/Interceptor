# HarnessRelay Interceptor

HarnessRelay Interceptor is a local Go daemon for launching and controlling terminal-based harnesses through managed PTY sessions and a browser dashboard.

Stage 1 scope is local-only:

- `harnessd serve` daemon
- REST API and WebSocket event stream
- Vite/React/xterm.js dashboard
- raw terminal fallback for arbitrary commands
- semantic adapter registry with Generic fallback and a Codex adapter
- bounded in-memory terminal history and audit metadata

No mobile app, cloud relay, public service, enterprise multi-user control plane, or automatic approval policy is included in Stage 1.

## Quick Start

Set a stable local token, build, and start the daemon:

```bash
export HARNESSRELAY_TOKEN="$(openssl rand -base64 32)"
make build
./bin/harnessd serve
```

Open:

```text
http://127.0.0.1:8765/
```

Enter the token in the dashboard login screen. If `HARNESSRELAY_TOKEN` is not set, `harnessd` generates a process-local token and prints it once at startup.

Health remains public:

```bash
curl http://127.0.0.1:8765/api/v1/health
```

Authenticated API example:

```bash
curl -H "Authorization: Bearer $HARNESSRELAY_TOKEN" \
  http://127.0.0.1:8765/api/v1/sessions
```

## CLI

`harnessctl` uses `HARNESSRELAY_ADDR` and `HARNESSRELAY_TOKEN`.

```bash
export HARNESSRELAY_ADDR=http://127.0.0.1:8765
export HARNESSRELAY_TOKEN=...

./bin/harnessctl status
./bin/harnessctl sessions
./bin/harnessctl run --name shell /bin/bash
./bin/harnessctl interrupt <session-id>
./bin/harnessctl terminate <session-id>
./bin/harnessctl attach <session-id>
```

`harnessctl attach` replays the current snapshot, streams live output, forwards local keyboard input, forwards local terminal resize, and detaches with `Ctrl-]`.

## Dashboard

The dashboard supports:

- session list
- create session form with Chat or Terminal start mode
- chat-first harness interaction view
- xterm.js terminal fallback view
- live WebSocket output
- prompt composer that sends text into the PTY
- slash-command menu for interrupt, terminate, special keys, snapshot refresh, and Terminal Mode fallback
- keyboard input and paste in Terminal Mode
- resize propagation
- interrupt, terminate, and force-kill controls
- reconnect by replaying recent in-memory output history
- compact debug/event inspector
- adapter identity, capabilities, status, and metadata
- Codex-aware prompt submission and event-bound safe denial

Chat Mode is a readable interface over the same managed PTY session. Generic
sessions conservatively project readable terminal output. Codex sessions render
backend semantic status, metadata, user, assistant, system, and approval events.
Codex assistant responses are reconstructed from a headless terminal screen
after redraw activity settles; raw TUI chunks remain exclusively in Terminal
Mode and never appear directly in the Chat transcript.

Terminal Mode preserves the xterm.js raw terminal experience for exact TUI rendering, keyboard capture, paste, resize, and fallback control.

The dashboard logo assets live in `web/src/assets/`. `HarnessRelay_Without_Text.png` is used for the favicon and compact app mark; `HarnessRelay_With_Text.png` is used where the full wordmark fits.

Run dashboard development mode:

```bash
cd web
npm install
npm run dev
```

The Vite dev server proxies `/api` to `http://127.0.0.1:8765`.

Dashboard smoke coverage:

```bash
HARNESSRELAY_TOKEN=dashboard-token node qa/dashboard-smoke.mjs
```

The smoke logs in, creates sessions in Chat Mode and Terminal Mode, sends prompt/input through both views, switches between modes, verifies reconnect snapshots, and exercises interrupt/terminate controls.

The QA log is `Docs/QA/WebApp-QA.md`. The optional real-harness smoke is gated behind `HARNESSRELAY_REAL_HARNESS_SMOKE=opencode` and should only be run with explicit approval because it launches a coding harness.

## Security

The daemon controls local terminal sessions. Treat access as local command-control access.

Current Stage 1 defaults:

- binds to `127.0.0.1:8765`
- requires local-token authentication for session API and WebSocket
- supports HttpOnly cookie login for the dashboard
- requires CSRF headers for cookie-authenticated state-changing requests
- rejects unexpected browser origins
- rejects non-local bind addresses unless explicitly allowed
- refuses to run as root unless `HARNESSRELAY_ALLOW_ROOT_FOR_TESTING=1`
- does not log raw terminal input in audit records

Remote access is not recommended for Stage 1. Prefer SSH tunnels or private networking, keep authentication enabled, and do not expose the daemon publicly. Binding outside localhost requires both `HARNESSRELAY_BIND_ADDRESS` and `HARNESSRELAY_ALLOW_NONLOCAL_BIND=1`.

## Testing

```bash
go test ./...
make test
make build
```

Fake harnesses live under `testdata/fake-harnesses/` and cover plain output, interactive input, long-running sessions, SIGTERM ignoring behavior, and resize awareness.

## Documentation

- [Docs/Spec/Context.md](Docs/Spec/Context.md): stable scope and architecture constraints
- [Docs/Spec/Todo.md](Docs/Spec/Todo.md): active implementation checklist
- [Docs/API.md](Docs/API.md): REST and WebSocket API
- [Docs/Developer.md](Docs/Developer.md): project structure, tests, adapters, and fake harnesses
- [Docs/Semantic-Adapters.md](Docs/Semantic-Adapters.md): adapter contracts,
  capabilities, semantic events, action safety, Codex behavior, and extension
  guidance

`Docs/Spec/Context.md` should not be edited without owner approval.
