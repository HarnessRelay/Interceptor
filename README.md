# HarnessRelay Interceptor

HarnessRelay Interceptor is a local Go daemon for launching and controlling terminal-based harnesses through managed PTY sessions and a browser dashboard.

Stage 1 scope is local-only:

- `harnessd serve` daemon
- REST API and WebSocket event stream
- Vite/React/xterm.js dashboard
- raw terminal fallback for arbitrary commands
- generic harness adapter foundation
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
- create session form
- xterm.js terminal view
- live WebSocket output
- keyboard input and paste
- resize propagation
- interrupt, terminate, and force-kill controls
- reconnect by replaying recent in-memory output history
- basic event list

Run dashboard development mode:

```bash
cd web
npm install
npm run dev
```

The Vite dev server proxies `/api` to `http://127.0.0.1:8765`.

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

`Docs/Spec/Context.md` should not be edited without owner approval.
