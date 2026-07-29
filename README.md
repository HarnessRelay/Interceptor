# HarnessRelay

HarnessRelay is a local-first relay/control plane for terminal-based AI coding harnesses.

It is meant to sit around tools such as Codex, OpenCode, Grok, Claude Code, Kilo Code, and other terminal-first coding agents. It is not a replacement for those tools. It is a relay/control layer around them.

> ⚠️ **Warning: very early experimental software**
>
> HarnessRelay launches and controls terminal programs, installs command shims, starts a local daemon, and can affect your terminal state.
>
> It may break your shell, terminal, `PATH`, running harness sessions, or local environment.
>
> Try it only on a Linux machine you are comfortable debugging. Do not use it for production work, important repositories, or machines where terminal/session breakage would be costly. Use at your own risk.

## What is HarnessRelay?

HarnessRelay is an experimental local tool that sits between you and terminal-based AI coding harnesses.

Instead of replacing your harness, it tries to relay it. You can start a harness such as Codex from your normal terminal, have HarnessRelay register that session, and then open a local web dashboard to observe, steer, interrupt, terminate, or use raw Terminal Mode as a fallback.

The goal is:

```text
Use your preferred harness locally.
Walk away from your machine.
Open the HarnessRelay web UI.
Continue steering or approving the session where support exists.
Return to your terminal later.
```

That workflow is not fully solved yet. The current project is a working early foundation, not a finished product.

HarnessRelay currently focuses on:

- a local web dashboard
- raw terminal fallback
- a semantic adapter layer where available
- an approval/steering/control surface
- transparent CLI shims
- session visibility

## Why I created this

I created HarnessRelay because I use terminal-based AI coding tools, but I do not always want to stay glued to the machine just to approve a command, send a follow-up prompt, or check what the harness is doing.

Many AI coding harnesses are terminal-first. They often need approvals, steering, interruption, or clarification. Full remote desktop or raw terminal streaming can be clunky, and replacing the harness is not ideal.

The goal is not to build another coding agent. The goal is to make existing harnesses easier to control through a universal local relay layer.

## Project status: extremely early / experimental

HarnessRelay is in a very, very early stage.

It is currently suitable only for experimentation, local dogfooding, and contributors who are comfortable debugging Linux terminals and Go/TypeScript code.

Current status:

- Linux-first
- local-only by default
- actively changing architecture
- not production ready
- currently dogfooded mainly with Codex
- Generic fallback exists for arbitrary terminal commands
- Codex semantic adapter exists, but is version- and behavior-sensitive
- semantic adapters for other harnesses are not complete
- `tmux` is accepted in shim configuration diagnostics, but first-class tmux relay/adoption is deferred
- session/event history is retained only for the daemon lifetime; durable restart persistence is not complete

Do not assume stability. Expect breaking changes.

## What works today

The current implementation includes:

- `harnessd` local daemon
- `harnessctl` CLI
- local web dashboard served by the daemon
- user-local install/update/uninstall scripts
- stable local token generated on install
- localhost-first daemon defaults
- user-level systemd service management through `harnessctl services ...`
- manual daemon mode through `harnessd serve`
- managed PTY sessions
- raw Terminal Mode through the browser
- Chat Mode over the same session
- Generic adapter fallback
- Codex semantic adapter
- Codex shim dogfooding
- transparent command shims for commands such as `codex`, `opencode`, and `grok` when those binaries are installed
- shim bypass with `HARNESSRELAY_BYPASS=1`
- session list/cards in the dashboard
- session snapshots and live WebSocket output
- interrupt and terminate controls
- local attach support with `harnessctl attach <session-id>`
- terminal cleanup on normal attach exit, detach, daemon disconnect, web terminate, and force kill paths

## What does not work yet / known limitations

Important limitations:

- Linux only for now.
- This is not safe for production use.
- Terminal edge cases almost certainly remain.
- Bugs may corrupt terminal mode, keyboard protocol state, shell state, or `PATH` behavior.
- Durable session, terminal, semantic, and audit history across daemon restart is deferred.
- First-class `tmux` pane registration/relay is not implemented.
- Semantic adapters are incomplete and conservative.
- Non-Codex harnesses generally use Generic/Terminal fallback today.
- Chat Mode is not the source of truth; Terminal Mode is.
- Locally typed shim prompts cannot always be reconstructed as semantic chat messages.
- Command shims must be placed carefully in `PATH`.
- Service commands require a systemd user manager.
- The dashboard is local-first and should not be exposed publicly.
- There is no guarantee of safety for important work.

## Safety warning

HarnessRelay controls terminal processes. Treat access to the dashboard/API as local command-control access.

Specific risks:

- Command shims change how commands like `codex` resolve in your shell.
- The shim path must be before the real harness binary in `PATH`, which can surprise you if misconfigured.
- Terminal state may become corrupted if bugs occur or the machine/process is killed at the wrong time.
- Daemon and session management are still new.
- Service setup may not work on all Linux distributions.
- Session history persistence across daemon restart is not complete.
- No action is guaranteed safe for important work.

Recovery and bypass commands:

```bash
stty sane
reset
HARNESSRELAY_BYPASS=1 codex
harnessctl shims uninstall codex
harnessctl shims uninstall-all
make uninstall
```

If enhanced terminal keyboard modes are visibly stuck after an uncatchable failure, the lower-level recovery sequence documented by the project is:

```bash
printf '\033[<1u\033[>4;0m\033[?1000;1002;1003;1006l\033[?2004l'
stty sane
reset
```

## How it works

HarnessRelay launches and owns harness processes inside pseudo-terminals.

```text
Browser dashboard
        │
        │ HTTP / WebSocket
        ▼
harnessd daemon
        ├── API server
        ├── session manager
        ├── PTY runtime
        ├── event stream
        ├── adapter registry
        └── static web dashboard
                │
                ▼
          pseudo-terminal
                │
                ▼
       Codex / OpenCode / Grok / shell / other harness
```

Terminal Mode renders the raw PTY and is the source-of-truth fallback. Semantic adapters can add better Chat Mode behavior, metadata, status detection, command catalogs, or approval actions where the harness behavior is understood.

The Generic adapter is always the fallback. The Codex adapter is the current dogfood path.

## Installation on Linux

### Prerequisites

- Linux
- Go
- Node.js/npm
- systemd user services, optional but recommended
- Codex/OpenCode/Grok/etc. only if you want to relay those harnesses

Clone the repository and install locally:

```bash
git clone https://github.com/HarnessRelay/Interceptor
cd Interceptor
make install
export PATH="$HOME/.local/bin:$PATH"
harnessctl status
```

`make install` builds the web dashboard and Go binaries, installs user-local files, creates a stable local token, and does not edit shell profiles.

Default installed paths:

```text
~/.local/bin/harnessctl
~/.local/bin/harnessd
~/.config/harnessrelay/interceptor.toml
~/.config/harnessrelay/token
~/.config/harnessrelay/install-manifest
~/.local/share/harnessrelay/
~/.local/state/harnessrelay/
```

If `harnessctl` is not found after install, add the CLI binary directory to your shell profile.

For zsh:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

For bash:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Web login token

Install creates a stable local token here:

```bash
cat ~/.config/harnessrelay/token
```

Use that token to log into the local dashboard. Keep it private. It grants local control over HarnessRelay sessions.

`HARNESSRELAY_TOKEN` can override the installed token:

```bash
export HARNESSRELAY_TOKEN=...
```

## Configuration

HarnessRelay uses XDG-resolved paths for config, data, and state. The default config directory is `~/.config/harnessrelay/`.

### Config files

| File | Purpose |
|---|---|
| `~/.config/harnessrelay/interceptor.toml` | Daemon config (bind address, port, allowlist behavior). |
| `~/.config/harnessrelay/allowed_ips.txt` | Optional IP allowlist for non-local binds. |
| `~/.config/harnessrelay/token` | Stable local auth token. Keep secret. |

### `interceptor.toml` keys

| Key | Type | Default | Description |
|---|---|---|---|
| `bind_address` | string | `127.0.0.1` | Interface to bind. Use `"0.0.0.0"` for all interfaces. Non-local requires explicit allow. |
| `port` | int | `8765` | HTTP server port. |
| `allowlist_permits_nonlocal_bind` | bool | `true` | When `true`, a non-empty `allowed_ips.txt` satisfies the non-local bind safety check without needing `HARNESSRELAY_ALLOW_NONLOCAL_BIND=1`. |

Example `interceptor.toml`:

```toml
bind_address = "0.0.0.0"
port = 8765
allowlist_permits_nonlocal_bind = true
```

### `allowed_ips.txt` format

- One IP address or CIDR range per line.
- Supports IPv4 and IPv6.
- Blank lines and `#` comments are ignored.
- Malformed lines are silently skipped at startup.

Example `allowed_ips.txt`:

```text
# Local network
192.168.1.0/24

# Specific device
192.168.1.104

# IPv6 localhost
::1
```

### Environment variables

**Daemon / Network**

| Variable | Description |
|---|---|
| `HARNESSRELAY_BIND_ADDRESS` | Override `bind_address` (e.g. `0.0.0.0`). |
| `HARNESSRELAY_PORT` | Override `port` (e.g. `8765`). |
| `HARNESSRELAY_ALLOW_NONLOCAL_BIND` | Set to `1` to permit non-local bind without an allowlist. |
| `HARNESSRELAY_ALLOW_ROOT_FOR_TESTING` | Set to `1` to allow running as root (testing only). |

**Authentication**

| Variable | Description |
|---|---|
| `HARNESSRELAY_TOKEN` | Overrides the installed `~/.config/harnessrelay/token`. |

**CLI / Shim**

| Variable | Description |
|---|---|
| `HARNESSRELAY_ADDR` | `harnessctl` daemon URL (default `http://127.0.0.1:8765`). |
| `HARNESSRELAY_BYPASS` | Set to `1` to skip shims and run the real harness binary directly. |
| `HARNESSRELAY_BIN_DIR` | Override the binary install directory. |
| `HARNESSRELAY_SHIMS_CONFIG` | Override the shim config file path. |
| `HARNESSRELAY_SHIMS_DIR` | Override the shim directory path. |

**Service (advanced)**

| Variable | Description |
|---|---|
| `HARNESSRELAY_DAEMON_BINARY` | Override the `harnessd` binary path used by service commands. |
| `HARNESSRELAY_SERVICE_UNIT_PATH` | Override the systemd unit file path. |
| `HARNESSRELAY_SYSTEMCTL` | Override the `systemctl` command path. |
| `HARNESSRELAY_JOURNALCTL` | Override the `journalctl` command path. |

### Remote / LAN access

By default the daemon binds to `127.0.0.1` and is unreachable from other devices.

To expose the dashboard on your LAN:

1. Set `bind_address = "0.0.0.0"` in `interceptor.toml` (or set `HARNESSRELAY_BIND_ADDRESS=0.0.0.0`).
2. Create `~/.config/harnessrelay/allowed_ips.txt` and add the IPs or CIDR ranges you trust.
3. Restart the daemon.

When `allowed_ips.txt` is non-empty and `allowlist_permits_nonlocal_bind` is `true` (default), the daemon accepts non-local binds automatically. If you prefer to skip the allowlist and accept the risk, set `HARNESSRELAY_ALLOW_NONLOCAL_BIND=1`.

Keep your token secret: LAN exposure means anyone who can reach the port must also authenticate with the token. Prefer VPN or SSH tunnel over public internet exposure.

### Restarting after config changes

Config files are read once at startup. There is no hot reload. After editing `interceptor.toml` or `allowed_ips.txt`, restart the daemon.

**If running as a systemd user service:**

```bash
harnessctl services restart
```

**If running manually:**

Stop the running `harnessd serve` process with `Ctrl-C`, then start it again:

```bash
harnessd serve
```

Note: session and event history are kept in memory. Restarting the daemon clears all active sessions and their history. This is a current limitation.

## Quick start

This is the normal Linux path with the user service and a Codex shim:

```bash
make install
export PATH="$HOME/.local/bin:$PATH"

harnessctl services install
harnessctl services start
harnessctl services enable

harnessctl shims install codex
export PATH="$(harnessctl shims path):$PATH"

mkdir -p /tmp/harnessrelay-test
cd /tmp/harnessrelay-test
codex
```

Open:

```text
http://127.0.0.1:8765
```

Use the token from:

```text
~/.config/harnessrelay/token
```

List sessions from the CLI:

```bash
harnessctl sessions
```

## Using transparent command shims

HarnessRelay shims let you keep typing normal commands like `codex`.

The shell resolves `codex` to a small HarnessRelay shim. The shim dispatches through `harnessctl`, asks `harnessd` to create a managed PTY session, and attaches your local terminal to that session.

Install a shim:

```bash
harnessctl shims install codex
```

Install multiple shims:

```bash
harnessctl shims install codex opencode grok
```

Add the shim directory before the real harness binary in `PATH`:

```bash
export PATH="$(harnessctl shims path):$PATH"
```

Check shim state:

```bash
harnessctl shims list
harnessctl shims status
harnessctl shims doctor
harnessctl shims path
```

Debug command resolution:

```bash
which codex
type -a codex
harnessctl shims doctor
```

Bypass HarnessRelay for one invocation:

```bash
HARNESSRELAY_BYPASS=1 codex
```

Remove shims:

```bash
harnessctl shims uninstall codex
harnessctl shims uninstall-all
```

The shim path must appear before the real harness binary in `PATH`. HarnessRelay does not edit shell profiles automatically.

## Running Codex through HarnessRelay

Codex is the current main dogfood path.

```bash
harnessctl shims install codex
export PATH="$(harnessctl shims path):$PATH"

which codex
HARNESSRELAY_BYPASS=1 codex --version
codex
```

Expected behavior:

- Codex opens normally in your terminal.
- HarnessRelay creates a session.
- The session appears in the web dashboard.
- Terminal Mode shows the raw Codex TUI.
- Chat Mode uses the Codex semantic adapter where the current Codex behavior can be recognized.
- Raw terminal fallback remains available at all times.

Codex is not the only target. HarnessRelay’s common architecture is intended to stay harness-neutral.

## Service mode

Service mode is recommended for normal use on Linux systems with a systemd user manager.

Install and start the user-local service:

```bash
harnessctl services install
harnessctl services start
harnessctl services enable
harnessctl services status
```

View logs:

```bash
harnessctl services logs
```

Other service commands:

```bash
harnessctl services restart
harnessctl services stop
harnessctl services disable
harnessctl services uninstall
```

The service is rootless and user-local. It uses `systemctl --user` and `journalctl --user`. `install` writes an owned `harnessrelay.service` user unit; it does not silently start or enable it.

On non-systemd Linux environments, run the daemon manually:

```bash
harnessd serve
```

The dashboard listens on `http://127.0.0.1:8765` by default.

## Uninstalling

Remove managed HarnessRelay binaries:

```bash
make uninstall
```

or:

```bash
./scripts/uninstall.sh
```

This preserves configuration, token, state, data, and shims by default.

Remove owned shims:

```bash
harnessctl shims uninstall codex
harnessctl shims uninstall-all
```

Remove owned shims during uninstall:

```bash
./scripts/uninstall.sh --shims
```

Purge config/data/state explicitly:

```bash
./scripts/uninstall.sh --purge
```

Be careful with purge. It is intentionally explicit.

## Troubleshooting

### `harnessctl: command not found`

```bash
export PATH="$HOME/.local/bin:$PATH"
which harnessctl
```

### Daemon is unreachable

```bash
harnessctl services status
harnessctl services logs
harnessctl services start
```

Manual fallback:

```bash
harnessd serve
```

### `codex` does not use the shim

```bash
export PATH="$(harnessctl shims path):$PATH"
hash -r
which codex
type -a codex
harnessctl shims doctor
```

### Need to bypass HarnessRelay

```bash
HARNESSRELAY_BYPASS=1 codex
```

### Terminal looks broken

```bash
stty sane
reset
```

If needed:

```bash
printf '\033[<1u\033[>4;0m\033[?1000;1002;1003;1006l\033[?2004l'
stty sane
reset
```

### Stop the service

```bash
harnessctl services stop
```

### View service logs

```bash
harnessctl services logs
```

### Uninstall a shim

```bash
harnessctl shims uninstall codex
```

### Uninstall HarnessRelay

```bash
make uninstall
```

## Roadmap

Near-term work includes:

- hardening terminal cleanup across more terminals and shells
- durable SQLite-backed session/event/audit persistence
- better completed-session history
- first-class tmux backend or a clearly bounded alternative
- stronger Generic fallback behavior
- improved Codex adapter compatibility tracking
- OpenCode adapter
- Grok adapter
- Claude Code adapter
- clearer approval and permission UI
- broader Linux distro testing
- better installer diagnostics
- more Playwright and fake-harness QA coverage
- security review

No roadmap item should weaken the core constraints: localhost-first, token-protected, raw Terminal Mode fallback, and harness-neutral common architecture.

## Contributing

Contributions are very welcome. This project is early enough that design feedback, bug reports, repro steps, docs, and small tests are just as valuable as code.

Useful areas:

- testing on different Linux distributions
- terminal cleanup bugs
- tmux backend work
- OpenCode adapter
- Grok adapter
- Claude Code adapter
- UI/UX
- accessibility
- docs
- install experience
- QA and Playwright tests
- security review

When reporting issues, include:

- OS/distro
- shell
- terminal emulator
- harness used
- how the harness was installed
- exact commands run
- whether `harnessctl services status` reports a running service
- output from `harnessctl shims doctor` when shims are involved
- screenshots or logs, only if safe to share

Please do not submit changes that make the common architecture Codex-specific. Harness-specific behavior should live in harness adapters.

Before submitting code, run:

```bash
make build
make test
```

## License

No project license file is present yet. Until a license is added, do not assume permission to use, redistribute, or relicense this code beyond what GitHub’s terms allow for viewing and contributing.
