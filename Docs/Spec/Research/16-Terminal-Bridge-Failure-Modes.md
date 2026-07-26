# Terminal Bridge Failure Modes

Date: 2026-07-26

## Ownership map

For a PTY shim session:

```text
shell terminal
  -> generated shim
  -> harnessctl shim exec / attach
  -> authenticated HTTP + WebSocket
  -> harnessd
  -> PTY runtime
  -> harness process group
```

`harnessctl` owns the raw-mode change on the shell terminal. `harnessd` owns
the PTY master and harness process group. The WebSocket is the liveness signal
between them.

## Audited failure modes

### Daemon exits or network connection closes

The output reader receives EOF. EOF was previously treated as a clean detach,
so users got no explanation. Although the normal Go return path ran the raw
mode defer, there was no regression test proving the ordering or state, and
signal termination could bypass it.

Mitigation: classify EOF before `session.exited` as daemon disconnect, restore
the terminal synchronously, then return a typed error that prints recovery and
bypass guidance.

### Attach process receives SIGINT, SIGTERM, SIGHUP, or SIGQUIT

Raw mode disables terminal-generated signals (`ISIG`), but signals can still
come from another process, terminal closure, service managers, or the shell.
Go's default signal action can terminate without running defers.

Mitigation: subscribe while raw mode is active, restore synchronously, stop
signal interception, and re-raise the original signal so callers observe
normal signal semantics.

### Attach process is suspended

Leaving the terminal raw while stopped corrupts the shell experience. Before
SIGTSTP, restore the original state; after SIGCONT, recapture/re-enter raw mode
only if attachment is still active. This path is best implemented through a
small terminal-state controller so restoration is idempotent.

### HTTP input forwarding fails

The input goroutine can learn about daemon death before the WebSocket reader.
That error must trigger the same restore/disconnect path. The output connection
is closed to release the other goroutine.

### Session exits normally

`session.exited` is authoritative and carries the child exit code. This is not
a daemon disconnect. Restore first, then return the child's status.

### Panic or SIGKILL

An ordinary panic unwinds only if recovered; `SIGKILL` cannot be handled by any
process. HarnessRelay prevents and tests all controllable paths. `stty sane` is
documented as last-resort recovery for uncatchable termination or kernel/power
failure, not expected operation.

## Input observation and semantic prompts

The local bridge sees raw keystrokes but not a stable "submitted prompt"
boundary for arbitrary TUIs. Editing, multiline input, paste, completion,
enhanced keyboard protocols, overlays, and terminal redraw state make recording
bytes up to Enter unsafe as semantic chat.

Codex output parsing can reconstruct assistant blocks from the rendered screen,
but the current observed output does not provide a reliable submitted-local-
prompt event in every state. HarnessRelay therefore does not fabricate user
messages from raw bytes. Shim-controlled semantic sessions display:

```text
This session was controlled from a terminal. Some terminal-entered prompts may
only be visible in Terminal Mode.
```

Web Chat submissions remain exact because the backend owns their prompt API
boundary and emits `chat.user_message`.

## Regression strategy

- unit-test idempotent terminal restoration
- run attach against a disposable fake daemon and PTY
- terminate the daemon while attached under a disposable pseudo-terminal
- assert canonical/echo flags match the state captured before attach
- assert the disconnect explanation is emitted after restoration
- retain `stty sane` in manual QA as backup only

