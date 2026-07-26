# CLI Shim / Proxy Mode Research

Date: 2026-07-26

## Decision Summary

HarnessRelay should use small user-local shims in:

```text
~/.local/share/harnessrelay/shims
```

with configuration in:

```text
~/.config/harnessrelay/shims.json
```

The public lifecycle belongs under `harnessctl shims`. Generated files dispatch
through the narrowly scoped internal command
`harnessctl shim exec <name> -- <args...>`.

The first relay-capable backend should use the existing daemon-owned PTY and
`harnessctl attach` path. Direct execution is the safe availability fallback.
A tmux backend is architecturally feasible, but it is deferred until tmux
panes can be registered as first-class HarnessRelay sessions without creating
a second, partially controlled process owner.

## Tools Reviewed

### Volta

Volta installs user-local launcher/shim binaries and puts its shim directory on
`PATH`. `volta setup` performs shell setup, while `volta which` resolves the
actual executable behind a shim. Its installer allows users to skip profile
modification.

Relevant patterns:

- a dedicated user-local bin/shim directory
- explicit `setup` for PATH integration
- `install`, `uninstall`, `list`, and `which` with conventional meanings
- a way to opt out of automatic profile changes

Sources:

- [Volta installers](https://docs.volta.sh/advanced/installers)
- [volta setup](https://docs.volta.sh/reference/setup)
- [volta which](https://docs.volta.sh/reference/which)

### pyenv

pyenv prepends a shim directory to `PATH`. Shims are lightweight dispatchers,
and `pyenv rehash` rebuilds them from the installed executable set. Its docs
make PATH order and regeneration explicit.

Relevant patterns:

- the shim directory must precede real binary directories
- `rehash` has the narrow meaning “rebuild shims”
- users can inspect shim paths and disable interception by removing shell
  initialization
- resolution has a fall-through/system path rather than recursively invoking
  the shim

Sources:

- [pyenv README: understanding shims](https://github.com/pyenv/pyenv/blob/master/README.md)
- [pyenv command reference](https://github.com/pyenv/pyenv/blob/master/COMMANDS.md)

### mise

mise places shims in `~/.local/share/mise/shims`, describes them as small
interceptors, and supports `mise reshim`. Its documentation warns that a
reshim owns the directory contents and may remove unmanaged additions.

Relevant patterns:

- XDG-style user-local data directory
- `reshim` is the established modern spelling
- PATH order and `which` behavior are part of user expectations
- regeneration should operate only on files whose ownership is clear

Sources:

- [mise shims](https://mise.jdx.dev/dev-tools/shims.html)
- [mise reshim](https://mise.jdx.dev/cli/reshim.html)

### asdf

asdf requires its shim directory at the front of `PATH` and exposes
`asdf reshim` to recalculate executable shims. It also provides shim
introspection.

Relevant patterns:

- explicit shell-specific PATH instructions
- resource-oriented core commands
- regeneration and inspection are separate operations

Sources:

- [asdf getting started](https://asdf-vm.com/guide/getting-started.html)
- [asdf core commands](https://asdf-vm.com/manage/core.html)

## Common Naming Patterns

- `install` creates managed artifacts or tool installations.
- `uninstall` removes artifacts owned by the tool.
- `list` enumerates resources.
- `status` reports current state.
- `doctor` is conventionally diagnostic and should suggest remediation rather
  than mutate state.
- `rehash` and `reshim` rebuild shim files. `reshim` is clearer for
  HarnessRelay because the resource is explicitly named `shims`.
- `setup` usually means shell/profile integration. HarnessRelay will not add a
  top-level `setup`; manual PATH instructions are sufficient for the first
  release.
- `which` commonly resolves the real executable. HarnessRelay stores the
  resolved absolute path and exposes it through `shims status`/`doctor` rather
  than adding another public verb initially.

These findings produce the normative taxonomy in
`Docs/Architecture/Command-Nomenclature.md`.

## Common Install / Setup Patterns

1. Create a user-local data directory.
2. Create small executable shims or links.
3. Put the shim directory before normal binary directories on `PATH`.
4. Explain or explicitly perform shell configuration.
5. Retain enough metadata to resolve and remove owned shims safely.
6. Provide a bypass or fall-through path.

HarnessRelay should not silently edit shell profiles. The install command
prints the exact `export PATH=...` line when the directory is absent or ordered
too late. A future explicit shell-update flag needs separate shell-specific
tests and ownership markers.

## Shim Directory Patterns

Recommended default:

```text
${XDG_DATA_HOME:-$HOME/.local/share}/harnessrelay/shims
```

This matches the project’s existing user-local storage convention and mise’s
XDG-style shim placement. It avoids root/system installation and keeps
generated interception artifacts separate from application binaries.

Config default:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/harnessrelay/shims.json
```

The shim file is an auditable POSIX shell script:

```sh
#!/bin/sh
# Generated by HarnessRelay. Do not edit manually.
exec "/absolute/path/to/harnessctl" shim exec "codex" -- "$@"
```

An absolute `harnessctl` path avoids resolving a different CLI at runtime and
keeps behavior stable if the calling PATH is unusual. `reshim` refreshes that
path after HarnessRelay moves or is upgraded.

## PATH Setup Patterns

The shim directory must appear before the directory containing the real
binary. Presence alone is insufficient.

Diagnostics distinguish:

- missing from PATH
- present after the real binary directory
- active and first matching command
- duplicate shim-directory entries
- shadowing an unexpected command

Manual setup:

```sh
export PATH="$HOME/.local/share/harnessrelay/shims:$PATH"
```

The runtime does not rely on PATH to find the real harness: installation stores
an absolute executable path found by searching PATH with the shim directory
excluded.

## Rehash / Reshim Patterns

`reshim` means “recreate generated shim files from existing HarnessRelay shim
configuration.” It does not:

- discover newly installed harnesses
- change PATH
- overwrite unmanaged files
- change backend selection
- repair malformed config silently

Regeneration should write a temporary file in the same directory, apply
executable permissions, and rename it into place. Each destination is checked
for a HarnessRelay ownership marker before replacement.

## Doctor / Status Patterns

`status` is concise observation:

- configured entry
- enabled/disabled state
- shim path
- real binary path
- selected backend
- whether the shim is active on PATH

`doctor` diagnoses and suggests fixes:

- missing or misordered shim directory
- missing, non-executable, or unmanaged shim file
- unmanaged files in the shim directory that may shadow unrelated commands
- missing or non-executable real binary
- real binary resolving back to a managed shim
- malformed config
- unavailable daemon
- unavailable requested backend
- missing bypass documentation

Neither command modifies files.

## Uninstall Patterns

Uninstall must be ownership-safe:

- remove a selected generated shim only when its content contains the exact
  HarnessRelay ownership marker
- remove its config entry only after the file decision is known
- refuse to delete an unmanaged destination
- make `uninstall-all` mean all HarnessRelay-owned shim entries, not the entire
  parent directory
- leave shell profiles untouched unless they were changed through a separately
  designed, ownership-marked feature

## Real Binary Resolution and Recursion Prevention

Installation resolution:

1. Split `PATH` in order.
2. remove empty entries and the canonical shim directory
3. look for the target executable in remaining directories
4. reject HarnessRelay-managed shim content
5. resolve symlinks for comparison where possible
6. require a regular executable file
7. store the absolute path

If more than one distinct executable remains, installation prints every
candidate and stores the first in PATH order unless the user supplies
`--real-binary`.

An explicit real-binary override is accepted for one target per install
invocation and must pass the same validation.

Runtime checks:

- the configured real path exists and is executable
- it is not the configured shim path
- canonical real and shim paths differ
- it does not contain the HarnessRelay shim marker
- a recursion-depth environment marker is not already set unexpectedly

Direct fallback executes the stored absolute path, never `exec.LookPath(name)`.

## tmux Launch and Adoption Research

tmux sessions own windows and panes, and each pane owns a pseudo-terminal. A
local terminal may attach when the session is created or later with
`attach-session`. The installed development host has tmux 3.7b.

Relevant primitives:

- `tmux new-session -d -s <name> -c <cwd> -- <command> <args...>` creates a
  collision-resistant detached session in a chosen working directory.
- `tmux display-message -p -t <target> '#{pane_id}'` returns the stable pane ID.
- `tmux attach-session -t <name>` attaches the local terminal.
- `tmux send-keys -t <pane-id> ...` sends terminal keys.
- `tmux capture-pane -p -e -t <pane-id>` captures visible/history text, not the
  original ordered raw PTY stream.
- tmux session names should use a HarnessRelay prefix plus a random session ID;
  user-provided harness names are metadata, not collision keys.

Source:

- [tmux(1) manual](https://man7.org/linux/man-pages/man1/tmux.1.html)

### Chosen tmux strategy

The eventual safe strategy is:

1. ask `harnessd` to allocate the HarnessRelay session ID and metadata
2. create a detached tmux session with a collision-resistant name and explicit
   cwd/environment
3. record tmux session name and pane ID in the managed session
4. attach the local client
5. bridge tmux pane output/input into the same API/event/adapter contracts
6. preserve detach without killing the pane

Launching tmux only inside `harnessctl` would make tmux—not `harnessd`—the
process owner and would not make the pane visible or controllable through the
existing web API. Scraping `capture-pane` is not equivalent to the raw ordered
PTY stream and would weaken Terminal Mode and semantic adapter behavior.

Therefore tmux is explicitly deferred for the first implementation. The config
and CLI recognize it as a backend choice, but runtime must report it unavailable
and use a documented fallback; it must not pretend that a direct tmux launch is
relay-capable.

## Direct Managed PTY Proxy Fallback Research

The current project already provides the necessary path:

```text
harnessctl
  -> POST /api/v1/sessions
  -> harnessd session manager
  -> daemon-owned PTY
  -> GET snapshot + WebSocket output
  -> POST input/resize/interrupt
```

`harnessctl attach` already:

- replays the snapshot
- puts a local TTY into raw mode
- forwards input
- forwards resize signals
- restores terminal state
- detaches on Ctrl-]

The shim runtime can create a session with the stored absolute real binary and
immediately use this attachment path. Required hardening is to observe the
session exit event, return the child exit code, and ensure direct fallback is
chosen before session creation when the daemon is unreachable.

This makes `pty` the initial relay-capable backend.

## Backend Selection

Initial behavior:

```text
configured pty
  daemon healthy -> create managed PTY session and attach
  daemon unavailable -> direct fallback with warning

configured direct
  execute real binary directly; no relay session

configured tmux
  report tmux relay backend deferred
  use configured fallback (pty when daemon is available, otherwise direct)
```

This differs from a naive “tmux installed means tmux default” rule because
availability of the `tmux` executable does not prove HarnessRelay can register
and control that pane. The default is `pty` until the tmux session backend is a
first-class daemon runtime.

## Daemon Fallback and Bypass

Default daemon-unavailable behavior is `direct` with a concise stderr warning.
Direct mode does not register a relay session.

Explicit bypass:

```bash
HARNESSRELAY_BYPASS=1 codex
```

Bypass preserves args, cwd, environment, signals, and exit code by replacing
the shim process with the stored absolute real executable.

## Risks and User Expectations

- PATH interception is powerful and must be visible, reversible, and
  user-local.
- Users expect `which codex` to show the shim; `shims status` must show the real
  target.
- A moved/deleted real binary must fail clearly rather than recurse.
- Shim installation must not replace a file owned by another tool.
- Direct fallback preserves tool availability but intentionally loses relay
  visibility.
- Daemon authentication must remain required; shim runtime uses the same local
  token and API security model.
- PTY attach currently cannot survive daemon restart.
- tmux detach durability is not claimed until tmux is a daemon-registered
  backend.
- No backend may auto-approve harness actions.

## Recommended HarnessRelay Approach

1. Establish the command taxonomy before CLI implementation.
2. Add an isolated `internal/shims` package for config, safe file ownership,
   PATH analysis, real-binary resolution, and backend policy.
3. Implement the complete `harnessctl shims` resource group.
4. Generate small absolute-`harnessctl` shim scripts.
5. Implement `harnessctl shim exec` with bypass, PTY relay, and direct
   fallback.
6. Add generic origin metadata (`origin`, `origin_backend`, `shim_name`,
   `real_binary`, `attachable`) to managed sessions.
7. Render quiet origin/backend labels in existing session UI.
8. Keep tmux as an explicit, diagnosed deferred backend until the daemon owns
   its lifecycle and output bridge.
9. Validate with fake binaries and temporary XDG/PATH directories; never edit
   the real shell profile in tests.
