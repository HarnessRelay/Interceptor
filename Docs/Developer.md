# Developer Guide

## Project Structure

- `cmd/harnessd`: daemon entry point
- `cmd/harnessctl`: CLI client
- `internal/api`: REST, WebSocket, auth integration, static serving
- `internal/config`: defaults and environment-backed security options
- `internal/events`: in-memory event bus and bounded event history
- `internal/harness`: adapter interfaces and registry
- `internal/harness/generic`: mandatory raw terminal fallback adapter
- `internal/pty`: PTY process runtime
- `internal/security`: local token auth, CSRF, and origin helpers
- `internal/session`: session manager and bounded terminal output buffer
- `internal/storage`: bounded in-memory audit metadata
- `web`: Vite/React/xterm.js dashboard
- `testdata/fake-harnesses`: repeatable fake terminal programs

The dashboard source is split into:

- `web/src/api`: browser API client and CSRF-aware request helper
- `web/src/components`: sidebar, session header, Chat Mode, Terminal Mode, slash menu, and event inspector
- `web/src/theme`: shared navy/teal product tokens
- `web/src/assets`: owner-provided logo placement notes

## Commands

```bash
go test ./...
make test
make build
make run
```

`make build` builds the dashboard first, then `bin/harnessd` and `bin/harnessctl`.

## Dashboard Smoke

The browser smoke script drives the built dashboard through Chrome DevTools Protocol. Start `harnessd` and a headless Chrome instance first:

```bash
HARNESSRELAY_TOKEN=dashboard-token HARNESSRELAY_PORT=8767 ./bin/harnessd serve
google-chrome --headless=new --remote-debugging-address=127.0.0.1 --remote-debugging-port=9222 --user-data-dir=/tmp/hri-dashboard-chrome http://127.0.0.1:8767/
HARNESSRELAY_TOKEN=dashboard-token node qa/dashboard-smoke.mjs
```

The smoke logs in, creates a Chat Mode shell session, sends a prompt through the composer, switches to Terminal Mode, sends raw terminal input, switches back to Chat Mode, reloads to verify reconnect/snapshot behavior, creates a Terminal Mode session, and exercises interrupt/terminate controls.

The active browser QA log lives at `Docs/QA/WebApp-QA.md`. Each regression covered by the smoke is labeled with its QA ID in `qa/dashboard-smoke.mjs`.

An optional real-harness smoke path is available only when explicitly approved:

```bash
HARNESSRELAY_TOKEN=dashboard-token HARNESSRELAY_REAL_HARNESS_SMOKE=opencode node qa/dashboard-smoke.mjs
```

That path launches `opencode` in `/tmp/harnessrelay-qa-opencode`, sends the documented harmless prompt, interrupts, and terminates it. Do not run a real-harness smoke without explicit approval because coding harnesses may perform external network calls.

## Fake Harnesses

Use fake harnesses instead of real coding tools for automated tests:

- `plain-output.sh`: stdout/stderr and exit
- `interactive-echo.sh`: prompt and input echo
- `ready-received.sh`: deterministic Chat Mode submit/Enter regression fixture
- `noisy-tui-artifact.sh`: repeated-character and redraw artifact fixture for Chat Mode filtering
- `long-running.sh`: heartbeat and Ctrl+C behavior
- `ignore-term.sh`: SIGTERM escalation behavior
- `resize-aware.sh`: terminal size observation

Example:

```bash
./bin/harnessctl run --name resize /bin/sh testdata/fake-harnesses/resize-aware.sh
```

Attach to a running session:

```bash
./bin/harnessctl attach <session-id>
```

Attach mode puts the local terminal in raw mode, forwards input through the daemon input API, streams output through the WebSocket event stream, sends resize updates on `SIGWINCH`, and restores local terminal state on exit. Detach with `Ctrl-]`.

## Adapter Notes

Every session must remain usable through raw terminal fallback. Harness-specific adapters should be optional and capability-based.

Chat Mode is intentionally a friendly projection over raw PTY output. Terminal output is stripped into readable transcript blocks where possible, but uncertain TUI state must be verified in Terminal Mode. Do not present Chat Mode as a complete semantic parser until a harness adapter provides reliable structured events.

Core adapter rules:

- do not shape common API around one harness
- keep generic adapter lowest priority and always available
- mark heuristic events with confidence
- never auto-approve actions
- keep raw terminal visible and usable

## Security Notes

Set `HARNESSRELAY_TOKEN` for a stable local token. If omitted, `harnessd` generates a process-local token at startup.

State-changing browser requests use cookie auth plus `X-CSRF-Token`. CLI and tests should use bearer auth.

Do not run `harnessd` as root. `HARNESSRELAY_ALLOW_ROOT_FOR_TESTING=1` exists only for test environments.

Remote access is not production-hardened. Use SSH tunnels or private networking if needed, and keep auth enabled. Non-local binding requires explicit configuration:

```bash
export HARNESSRELAY_BIND_ADDRESS=0.0.0.0
export HARNESSRELAY_ALLOW_NONLOCAL_BIND=1
export HARNESSRELAY_TOKEN=...
```

## Current Persistence Limits

Stage 1 currently uses in-memory session metadata, event history, terminal replay history, and audit records. Daemon restart loses this state. SQLite migration and restart recovery remain future work in `Docs/Spec/Todo.md`.

Completed sessions remain visible by default so users can inspect status and recent output. `POST /api/v1/sessions/{id}/cleanup` manually removes only completed sessions from the active in-memory session list; it does not erase audit records.

## Sensitive Data Areas

Terminal input and output may contain secrets because users can type tokens, print environment variables, or run tools that expose credentials. Audit records intentionally store metadata such as byte counts, session IDs, dimensions, action IDs, and timestamps rather than raw input payloads.

No SQLite database is currently written by Stage 1, so there are no database contents to review yet. When SQLite is added, terminal history and audit tables must be treated as sensitive local data.

Use `security.RedactSecret` before logging environment-like key/value pairs. The daemon startup log prints a generated process-local token only when `HARNESSRELAY_TOKEN` is unset so the user can log in; configured tokens are not logged.
