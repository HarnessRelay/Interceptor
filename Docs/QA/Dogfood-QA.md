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

## DOGFOOD-003: Terminal protocol cleanup

Status: Automated
Area: shim attach/raw terminal
Severity: Critical

Root cause: the earlier fix restored the kernel termios structure but did not
restore terminal-emulator protocol state. Codex pushes Kitty/CSI-u keyboard
mode (observed `CSI > 7 u`) and enables other TUI modes. Web terminate or force
kill can prevent Codex from popping/resetting those modes, so the shell receives
ordinary keypresses as visible CSI-u sequences even though `stty` is cooked.

Fix:

- local cleanup restores termios and writes protocol resets to the user's local
  terminal, not the daemon PTY
- Kitty stacks are popped on both the active and main screens around alternate
  screen exit
- modifyOtherKeys, bracketed paste, mouse/focus, cursor, cursor-key, keypad, and
  synchronized-output modes are normalized without a destructive full terminal
  reset
- `session.exited` identifies `terminate` and `kill`; local attach reports
  remote termination after restoring the terminal
- WebSocket EOF/daemon loss remains a typed non-zero failure with exact protocol
  recovery, service restart, direct bypass, `stty sane`, and `reset` guidance
- SIGHUP, SIGINT, SIGQUIT, SIGTERM, detach, EOF, and suspend share repeat-safe
  cleanup

Regression coverage:

- Go `internal/terminalcleanup` sequence, repeat-safety, nil/closed writer, and
  flush tests
- Go `TestRawTerminalRestoreIsIdempotent`
- Go `TestTerminationSignalsRestoreRawTerminal`
- Go `TestAttachRestoresTerminalAndReportsDaemonDisconnect`
- Go `TestShimAttachRemoteStopCleansFakeKittyProtocol` for both terminate and
  force kill

The fake harness enables alternate screen, synchronized output, Kitty
`CSI > 7 u`, modifyOtherKeys, bracketed paste, mouse/focus, and hidden cursor
modes, ignores SIGTERM, and cannot perform its own cleanup.

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

### DOGFOOD-003 terminal protocol cleanup

1. Ensure the service and Codex shim are active:

   ```bash
   harnessctl services status
   which codex
   harnessctl shims doctor
   ```

2. Start Codex:

   ```bash
   mkdir -p /tmp/harnessrelay-codex-dogfood
   cd /tmp/harnessrelay-codex-dogfood
   codex
   ```

3. Open the web UI, select the running Codex session, and click **Terminate**.
4. Return to the local terminal and press normal letter/number keys.
5. Verify characters do not turn into CSI-u text such as `s5;1:3u`.
6. Verify prompt editing, Enter, Backspace, Ctrl+C, paste, cursor visibility,
   and mouse selection are normal.
7. Run:

   ```bash
   echo terminal-ok
   ```

   Expected:

   ```text
   terminal-ok
   ```

8. Repeat steps 2–7 with **Force kill…**, type `KILL`, and confirm.
9. Start Codex once more, then in another terminal run:

   ```bash
   harnessctl services stop
   ```

10. Verify the local CLI exits non-zero, recovery instructions are shown, the
    terminal remains usable, and no CSI-u characters appear.

If an uncatchable failure still corrupts the terminal, use:

```bash
printf '\033[<1u\033[>4;0m\033[?1000;1002;1003;1006l\033[?2004l'
stty sane
reset
```

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

- `go test ./...`: passed, 179 tests across 19 packages.
- `make test`: passed, including temporary-HOME install lifecycle.
- `make build`: passed.
- `npm --prefix web run build`: passed with the existing non-fatal chunk-size
  advisory.
- `npm --prefix web run qa`: passed, 18 Playwright scenarios.
- disposable fake protocol shim enabled Kitty/CSI-u and related TUI modes;
  remote terminate emitted cleanup and the enclosing shell printed
  `terminal-ok`.
- real `codex-cli 0.145.0` enabled `CSI > 7 u`; remote terminate and separately
  confirmed Force Kill both emitted cleanup and the enclosing shell printed
  `codex-terminal-ok` / `codex-kill-terminal-ok`.
- stopping the disposable daemon during the fake protocol shim emitted cleanup,
  printed recovery instructions, exited non-zero, and the enclosing shell
  printed `disconnect-terminal-ok`.
- `node qa/dashboard-smoke.mjs`: passed against disposable daemon/Chrome.
- systemd 259 `systemd-analyze verify`: accepted the generated temporary unit.
- disposable generated fake PTY shim printed `managed-shim-ok arg-two`, exited
  zero, and remained listed as a finished session.
- real Codex Playwright smoke passed in `/tmp/harnessrelay-qa-codex`; safe
  `codex --version` in `/tmp/harnessrelay-codex-dogfood` reported
  `codex-cli 0.145.0` through the configured direct daemon-unavailable fallback.
- `git diff --check`: passed.

The real user service directory and shell profiles were not modified.
