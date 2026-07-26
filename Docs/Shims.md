# HarnessRelay Command Shims

HarnessRelay shims let you keep using normal harness commands while the daemon
creates an attachable managed PTY session:

```bash
codex
opencode
grok
```

A shim is a small user-local script with the same command name. It dispatches
to `harnessctl shim exec`, which resolves the stored real binary, creates a
daemon-owned session, and attaches the current terminal.

## Install

First install HarnessRelay itself so `harnessctl` and `harnessd` are available
from any directory:

```bash
make install
export PATH="$HOME/.local/bin:$PATH"
harnessd serve
```

This CLI PATH is separate from the shim PATH below. The installer does not
create shims or edit shell profiles. Full installation, update, and uninstall
instructions are in [Install.md](Install.md).

Then install selected shims:

```bash
harnessctl shims install codex opencode grok
```

Install every known target that is present on the current PATH:

```bash
harnessctl shims install --all-known
```

Use an explicit real executable for one target:

```bash
harnessctl shims install codex --real-binary /opt/codex/bin/codex
```

The default relay backend is the existing daemon-owned PTY:

```bash
harnessctl shims install codex --backend pty
```

Generated files live under:

```text
${XDG_DATA_HOME:-$HOME/.local/share}/harnessrelay/shims
```

Configuration lives under:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/harnessrelay/shims.json
```

Both locations can be overridden for packaging/testing with
`HARNESSRELAY_SHIMS_DIR` and `HARNESSRELAY_SHIMS_CONFIG`.

## PATH Setup

The shim directory must appear before directories containing the real harness
binaries:

```bash
export PATH="$HOME/.local/share/harnessrelay/shims:$PATH"
```

Add that line to the appropriate shell profile and restart the shell.
HarnessRelay prints it after install when PATH is missing or misordered. It
does not silently edit shell profiles.

Inspect the configured path:

```bash
harnessctl shims path
```

Because the shim intentionally shadows the real command, `which codex` should
show the HarnessRelay shim. Use `harnessctl shims status` to see the stored real
binary. If PATH contains multiple distinct executable candidates, install lists
them and stores the first candidate in PATH order.

## Status and Diagnostics

```bash
harnessctl shims list
harnessctl shims status
harnessctl shims doctor
```

- `list` shows known shim targets and installed configuration.
- `status` shows each configured shim, backend, real binary, generated file, and
  whether PATH makes it active.
- `doctor` checks config parsing, ownership/executable bits, recursion risks,
  PATH order, real binaries, daemon reachability, tmux availability, and
  fallback behavior. It is read-only.

## Runtime Behavior

For a PTY-backed entry:

```text
normal command
  -> generated shim
  -> harnessctl shim exec
  -> authenticated harnessd create-session API
  -> daemon-owned PTY
  -> local harnessctl attachment
```

Arguments, current working directory, environment, terminal dimensions, raw
terminal input/output, resize, and child exit status are preserved. The session
appears in the web UI with Shim origin and PTY backend metadata. Terminal Mode
remains available, and adapter selection uses the resolved harness executable
normally.

Detach the local terminal with `Ctrl-]`. Detaching does not terminate the
daemon-owned session.

## Backends

### PTY

`pty` is the default and the only relay-capable backend in the initial shim
release. `harnessd` owns the process and existing API, WebSocket, Chat Mode,
Terminal Mode, semantic adapter, interrupt, terminate, and audit behavior all
remain available.

### tmux

The CLI accepts and diagnoses `tmux` as a configured backend, but first-class
tmux pane registration is deferred. A standalone tmux launch would not be
visible through the existing daemon API and `capture-pane` is not equivalent to
the ordered raw PTY stream.

When a configured tmux entry runs and the daemon is available, HarnessRelay
prints a warning and uses the PTY backend. `shims doctor` reports whether tmux
is installed and explains the deferral.

### direct

`direct` replaces the shim process with the stored absolute real executable.
It preserves normal command behavior but creates no HarnessRelay session:

```bash
harnessctl shims install codex --backend direct
```

Direct mode is also the default fallback when the daemon is unavailable.

## Bypass

Bypass HarnessRelay for one invocation:

```bash
HARNESSRELAY_BYPASS=1 codex --help
```

The shim executes the stored absolute real binary directly. No daemon request
or relay session is created.

## Regeneration

After moving/upgrading `harnessctl`, regenerate owned scripts from config:

```bash
harnessctl shims reshim
```

`reshim` does not discover new binaries, change backend settings, edit PATH, or
overwrite unmanaged files.

## Uninstall

Remove selected owned shims:

```bash
harnessctl shims uninstall codex
```

Remove all configured HarnessRelay-owned shims:

```bash
harnessctl shims uninstall-all
```

Uninstall refuses to delete a file without the HarnessRelay ownership marker.
It does not remove unrelated files or edit shell profiles.

## Safety Model

- user-local installation only; no root required
- unmanaged destinations are never overwritten without explicit `--force`
- uninstall never removes an unmanaged file
- real binary paths are absolute and symlinks are resolved when safe
- PATH resolution excludes the shim directory
- generated shims and configured real paths are checked for recursion
- generated scripts are small and ownership-marked
- explicit bypass and direct fallback preserve harness availability
- no shell profile changes occur automatically
- no shim action approves a harness request
- daemon authentication, localhost defaults, Terminal Mode, and raw terminal
  fallback remain unchanged

`--force` authorizes replacement of exactly the named shim destination; it is
not a directory-wide overwrite.

## Troubleshooting

Run:

```bash
harnessctl shims doctor
```

Common fixes:

- “missing from PATH”: prepend the value from `harnessctl shims path`.
- “after the real binary”: move the shim export earlier in the profile.
- “real binary missing”: reinstall the harness or reinstall that shim with
  `--real-binary`.
- “unmanaged file”: inspect it; use a different shim directory or explicit
  `--force` only if replacement is intended.
- “daemon unavailable”: start `harnessd serve`, verify
  `HARNESSRELAY_ADDR`/`HARNESSRELAY_TOKEN`, or accept the warned direct
  fallback.
- moved `harnessctl`: run `harnessctl shims reshim`.
- `harnessctl: command not found`: install HarnessRelay and add
  `~/.local/bin` to PATH before diagnosing shim PATH.

## Current Limitations

- tmux is modeled and diagnosed but not yet a daemon-owned session backend.
- daemon restart still loses in-memory session/event history.
- local detach survival is provided by the daemon process, not tmux.
- shim config is JSON and currently has no general `harnessctl config` command.
- profile editing is manual by design in this release.
