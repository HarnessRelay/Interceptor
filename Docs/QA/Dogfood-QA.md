# HarnessRelay Dogfood Continuity QA

Date: 2026-07-26

Automated tests use disposable directories, fake commands, localhost servers,
and pseudo-terminals. They do not edit shell profiles or touch the real systemd
user-unit directory.

## DOGFOOD-001: Terminal-entered prompts missing from Chat Mode

Status: Mitigated and documented
Area: semantic Chat projection
Severity: High

Root cause: the local shim bridge observes raw TUI keystrokes, not a reliable
semantic submit boundary. Editing, multiline input, paste, completion,
overlays, and enhanced keyboard protocols make "bytes before Enter" unsafe.
The Codex output parser can reconstruct response blocks but cannot guarantee
the corresponding locally entered prompt in every terminal state.

Fix:

- web Chat submissions still emit exact `chat.user_message` events
- adapter-derived assistant events remain best-effort and confidence-scoped
- shim sessions show an explicit terminal-control limitation notice
- uncertain local prompts are not fabricated
- Terminal Mode remains the complete source of truth

Regression coverage:

- Playwright `Shim session origin is visible without obscuring Terminal fallback`
- existing semantic prompt and raw Terminal fallback tests

## DOGFOOD-002: Finished semantic history disappears

Status: Automated for daemon lifetime
Area: session/event lifecycle
Severity: Critical

Root cause: semantic response extraction is normally triggered after a
three-second quiet period. Process exit stopped the timer without flushing the
adapter, so final terminal output could exist while the assistant semantic
event never entered history.

Fix:

- PTY output processing is drained before final session state
- the adapter's final idle projection is flushed before `session.exited`
- completed sessions and their bounded semantic history remain in memory
- creating a second session does not remove the first
- Chat Mode hydrates completed history from the REST event endpoint

Regression coverage:

- Go `TestFinishedSessionFlushesAndRetainsSemanticHistory`
- Playwright semantic-adapter test selects another session, returns to the
  first, reloads, terminates, reloads again, and verifies the finished
  transcript

Known limit: daemon restart still loses session, terminal, semantic, and audit
history. This is explicitly deferred to the existing SQLite persistence
milestone; it is not presented as durable history.

## DOGFOOD-003: Daemon death corrupts local terminal

Status: Automated
Area: shim attach/raw terminal
Severity: Critical

Root cause: unexpected attach WebSocket EOF was classified as clean detach, and
default process termination signals could bypass Go defers while raw mode was
active.

Fix:

- unexpected daemon loss is a typed non-zero attach failure
- terminal restore is idempotent and occurs before the error is returned
- SIGHUP, SIGINT, SIGQUIT, SIGTERM, and suspend handling restore first
- the error reports service restart, direct bypass, and `stty sane` backup

Regression coverage:

- Go `TestRawTerminalRestoreIsIdempotent`
- Go `TestTerminationSignalRestoresRawTerminal`
- Go `TestAttachRestoresTerminalAndReportsDaemonDisconnect`

The daemon-death test uses a disposable pseudo-terminal, closes the upgraded
WebSocket, and proves the complete termios state equals its pre-attach value.

## DOGFOOD-004: Manual daemon startup

Status: Automated with fake user-service commands
Area: install/service lifecycle
Severity: Critical

Fix:

```bash
harnessctl services install
harnessctl services start
harnessctl services enable
harnessctl services status
harnessctl services logs
harnessctl services restart
harnessctl services stop
harnessctl services disable
harnessctl services uninstall
```

The generated `harnessrelay.service` is rootless, ownership-marked, uses an
absolute `harnessd` path, restarts on failure, and preserves the existing stable
token and localhost config. Install does not silently start or enable it.

Regression coverage:

- `internal/service` unit generation, ownership, lifecycle, failure safety
- CLI lifecycle with fake `systemctl`/`journalctl` and a temporary unit path
- install-script output points normal users to service setup

## Manual dogfood procedure

### Service

```bash
make install
export PATH="$HOME/.local/bin:$PATH"
harnessctl services install
harnessctl services start
harnessctl services enable
harnessctl services status
harnessctl status
harnessctl services logs
```

Expected: the user unit is active, `harnessctl status` reports reachable, and
no root command was needed.

### Shim and finished history

```bash
harnessctl shims install codex
export PATH="$(harnessctl shims path):$PATH"
codex
```

1. Enter a harmless prompt locally.
2. Verify Terminal Mode shows the complete interaction.
3. Verify Chat Mode shows the terminal-control notice and any confidently
   reconstructed response.
4. Exit Codex.
5. Select the finished session and verify terminal/semantic history.
6. Start a second Codex process and verify the old finished session remains.

### Daemon-death safety

1. Start a fake or harmless shim session in one terminal.
2. In another terminal run `harnessctl services stop`.
3. Confirm the attached command exits non-zero with `Restored terminal`.
4. Run `stty -a` and type a normal shell command.
5. Confirm echo, line editing, Enter, Ctrl+C, and shell prompt behavior are
   normal without running `stty sane`.

### Cleanup

```bash
harnessctl shims uninstall codex
harnessctl services disable
harnessctl services uninstall
```

Expected: only HarnessRelay-owned artifacts are removed. Real harness binaries,
unmanaged unit files, shell profiles, configuration, and history are preserved
according to their documented ownership rules.

## Final verification result

- `go test ./...`: passed, 167 tests across 18 packages.
- `make test`: passed, including temporary-HOME install lifecycle.
- `make build`: passed.
- `npm --prefix web run build`: passed with the existing non-fatal chunk-size
  advisory.
- `npm --prefix web run qa`: passed, 18 Playwright scenarios.
- `node qa/dashboard-smoke.mjs`: passed against disposable daemon/Chrome.
- systemd 259 `systemd-analyze verify`: accepted the generated temporary unit.
- disposable generated fake PTY shim printed `managed-shim-ok arg-two`, exited
  zero, and remained listed as a finished session.
- real Codex Playwright smoke passed in `/tmp/harnessrelay-qa-codex`; safe
  `codex --version` in `/tmp/harnessrelay-codex-dogfood` reported
  `codex-cli 0.145.0` through the configured direct daemon-unavailable fallback.
- `git diff --check`: passed.

The real user service directory and shell profiles were not modified.
