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

Install locally, add the CLI directory to PATH if requested, and start the
daemon:

```bash
make install
export PATH="$HOME/.local/bin:$PATH"
harnessd serve
```

Open:

```text
http://127.0.0.1:8765/
```

Install creates a stable token at `~/.config/harnessrelay/token`.
`harnessd` and `harnessctl` read it automatically;
`HARNESSRELAY_TOKEN` overrides it. Check setup and opt into a shim from another
terminal:

```bash
harnessctl status
harnessctl shims install codex
export PATH="$(harnessctl shims path):$PATH"
codex
```

Install does not edit shell profiles or create shims automatically. See
[Docs/Install.md](Docs/Install.md) for update, safe uninstall, explicit purge,
PATH troubleshooting, and security details.

## CLI

`harnessctl` uses `HARNESSRELAY_ADDR` and the shared stable token. Environment
token configuration remains supported and has precedence.

```bash
export HARNESSRELAY_ADDR=http://127.0.0.1:8765
export HARNESSRELAY_TOKEN=...

harnessctl status
harnessctl sessions
harnessctl run --name shell /bin/bash
harnessctl interrupt <session-id>
harnessctl terminate <session-id>
harnessctl attach <session-id>
```

`harnessctl attach` replays the current snapshot, streams live output, forwards local keyboard input, forwards local terminal resize, and detaches with `Ctrl-]`.

### Transparent command shims

Install user-local shims, prepend their directory to PATH, then keep using the
normal harness commands:

```bash
harnessctl shims install codex opencode grok
export PATH="$HOME/.local/share/harnessrelay/shims:$PATH"

codex
opencode
grok
```

The default shim backend creates a daemon-owned PTY session, attaches the local
terminal, and exposes the same session in the dashboard. Check configuration
with `harnessctl shims status` or `harnessctl shims doctor`. Bypass one
invocation with `HARNESSRELAY_BYPASS=1 codex`; remove shims with
`harnessctl shims uninstall` or `harnessctl shims uninstall-all`.

HarnessRelay never overwrites or deletes an unmanaged shim file by default and
does not edit shell profiles automatically. See
[Docs/Shims.md](Docs/Shims.md) for PATH setup, backend behavior, safety, and
troubleshooting.

## Dashboard

The dashboard supports:

- rectangular session cards with lifecycle grouping, search, and filters
- detected-harness shortcuts and a keyboard-accessible New Session dialog
- Chat or Terminal start mode plus progressive terminal/environment options
- chat-first harness interaction view
- xterm.js terminal fallback view
- live WebSocket output
- prompt composer that sends text into the PTY
- searchable command palette combining version-verified harness commands with
  session actions, special keys, lifecycle controls, and Terminal fallback
- keyboard input and paste in Terminal Mode
- resize propagation
- top-level interrupt plus terminate and force kill behind More and accessible
  confirmation dialogs
- reconnect by replaying recent in-memory output history
- right-side session inspector, hidden by default, with overview, events, and
  adapter capability tabs
- adapter identity, capabilities, status, and metadata
- Codex-aware prompt submission and event-bound safe denial

Chat Mode is a readable interface over the same managed PTY session. Generic
sessions conservatively project readable terminal output. Codex sessions render
backend semantic status, metadata, user, assistant, system, and approval events.
Codex assistant responses are reconstructed from a headless terminal screen
after redraw activity settles; raw TUI chunks remain exclusively in Terminal
Mode and never appear directly in the Chat transcript.

For verified Codex versions, the command palette exposes the harness's own
slash commands through the semantic adapter. Commands that open native pickers
move into Terminal Mode; sensitive commands are prefilled without Enter so the
user completes them in the source-of-truth TUI.

Terminal Mode preserves the xterm.js raw terminal experience for exact TUI rendering, keyboard capture, paste, resize, and fallback control.

The interface is a desktop/laptop workbench. Its session manager is inspired by
Termius, the conversation canvas by ChatGPT, and its progressive information
architecture by JetBrains tools. Theme tokens live in
`web/src/theme/tokens.css`; the interaction and component contracts live in
`Docs/Design/`.

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
npm --prefix web run qa
npm --prefix web run qa:a11y
```

The smoke logs in, creates sessions in Chat Mode and Terminal Mode, sends prompt/input through both views, switches between modes, verifies reconnect snapshots, and exercises interrupt/terminate controls.

The redesign QA log is `Docs/QA/UI-Revamp-QA.md`; accessibility results are in
`Docs/QA/Accessibility-QA.md`. The optional real-harness smoke is gated behind
`HARNESSRELAY_REAL_HARNESS_SMOKE=opencode` and should only be run with explicit
approval because it launches a coding harness.

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
- [Docs/Shims.md](Docs/Shims.md): transparent command shim installation,
  runtime, fallback, safety, and troubleshooting
- [Docs/Install.md](Docs/Install.md): user-local install, PATH, stable token,
  update, uninstall, purge, and recovery
- [Docs/Architecture/Command-Nomenclature.md](Docs/Architecture/Command-Nomenclature.md):
  normative CLI resource/verb taxonomy
- [Docs/Semantic-Adapters.md](Docs/Semantic-Adapters.md): adapter contracts,
  capabilities, semantic events, action safety, Codex behavior, and extension
  guidance

`Docs/Spec/Context.md` should not be edited without owner approval.
