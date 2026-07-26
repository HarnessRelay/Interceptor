# Installing HarnessRelay

HarnessRelay supports a rootless, user-local installation. The installer builds
the dashboard and Go binaries, installs only HarnessRelay-owned files, creates a
stable local authentication token, and leaves shell profiles and harness shims
unchanged.

## User-local install

From the repository root:

```bash
make install
```

The equivalent script entry point is:

```bash
./scripts/install.sh
```

The default XDG-style layout is:

```text
~/.local/bin/harnessctl
~/.local/bin/harnessd
~/.config/harnessrelay/interceptor.toml
~/.config/harnessrelay/token
~/.config/harnessrelay/install-manifest
~/.local/share/harnessrelay/
~/.local/state/harnessrelay/
```

`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_STATE_HOME` are honored.
`HARNESSRELAY_BIN_DIR` can override the binary destination for packaging and
tests. No default path requires root.

An existing binary destination is replaced only when the install manifest
proves it is an unchanged HarnessRelay-managed installation. Inspect an
unmanaged destination before using the narrowly scoped `--force` option:

```bash
./scripts/install.sh --force
```

## PATH setup

HarnessRelay has two distinct PATH layers.

The CLI binary directory makes `harnessctl` and `harnessd` available:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

The installer reports this command when the directory is missing from PATH. It
does not edit `.zshrc`, `.bashrc`, or any other profile.

The shim directory makes intercepted harness commands such as `codex` active
and must precede the real harness directories:

```bash
export PATH="$(harnessctl shims path):$PATH"
```

Do not substitute one PATH entry for the other.

## Starting the daemon

Start the user-level daemon in a terminal:

```bash
harnessd serve
```

The dashboard remains localhost-first at `http://127.0.0.1:8765`.
Production dashboard assets are embedded in `harnessd`, so the installed daemon
can be started from any working directory.

## Stable auth token

Install creates a cryptographically random token only when this file is
missing:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/harnessrelay/token
```

The configuration directory is mode `0700`; the token, config, and install
manifest are mode `0600`. Both `harnessd` and `harnessctl` read this token.
`HARNESSRELAY_TOKEN` has higher precedence and is reported as the `env` source
by status. The token is not printed during normal installed startup.

The installer preserves an existing token and configuration during reinstall
and update. Keep the token secret: it grants local control over terminal
sessions.

## Checking status

```bash
harnessctl status
```

Status is read-only and reports the configured daemon address and reachability,
daemon version, token source (`env`, `config`, or `missing`), config/token
paths, active and PATH-resolved CLI binaries, and canonical shim path. An
unreachable daemon is a diagnosed state, not a status-command crash.

## Installing shims

Start the daemon, then install only the harnesses you want to intercept:

```bash
harnessctl shims install codex opencode
export PATH="$(harnessctl shims path):$PATH"
harnessctl shims doctor
which codex
codex
```

The installer never installs shims automatically. See
[Shims.md](Shims.md) for ownership, bypass, fallback, and PATH-order details.

## Updating

From an updated repository checkout:

```bash
make update
```

or:

```bash
./scripts/install.sh --update
```

Update rebuilds and atomically replaces unchanged managed binaries. It
preserves configuration, token, data, state, and shim configuration, then runs
the installed binaries' help commands as a basic health check. A locally
modified installed binary is not overwritten without explicit `--force`.

After moving `harnessctl` to a different binary directory, regenerate existing
owned shims with `harnessctl shims reshim`.

## Uninstalling

Remove only unchanged, manifest-owned HarnessRelay binaries:

```bash
make uninstall
```

or:

```bash
./scripts/uninstall.sh
```

Configuration, stable token, data, state, real harness binaries, and shims are
preserved. If an installed binary was modified after installation, uninstall
leaves it and the manifest in place and explains the conflict.

Remove HarnessRelay-owned shims at the same time:

```bash
./scripts/uninstall.sh --shims
```

Shim removal uses HarnessRelay's ownership markers and never deletes the stored
real harness executable.

## Purging config/data

Purging is explicit:

```bash
./scripts/uninstall.sh --purge
```

This first removes owned shims and managed binaries, then deletes the
HarnessRelay config, data, and state directories. It refuses to purge if a
modified installed binary remains, preserving the manifest needed for
recovery.

## Troubleshooting

- `harnessctl: command not found`: add `~/.local/bin` to PATH and restart the
  shell.
- `codex` still resolves to the real binary: prepend the value from
  `harnessctl shims path`, then run `harnessctl shims doctor`.
- `token source: missing`: rerun `make install`, or set
  `HARNESSRELAY_TOKEN` explicitly.
- daemon unreachable: start `harnessd serve` and inspect
  `HARNESSRELAY_ADDR`.
- install refuses a destination: inspect it. Use a different
  `HARNESSRELAY_BIN_DIR` or explicit `--force` for that exact install.
- uninstall refuses a modified binary: preserve or move the local changes,
  restore the installed version, then rerun uninstall.

## Security notes

- installation is user-local and rootless by default
- bind remains `127.0.0.1` by default and authentication is never bypassed
- shell profiles are never edited automatically
- shims are opt-in and do not auto-approve harness actions
- unmanaged files are not overwritten or deleted
- uninstall preserves user configuration and data unless purge is explicit
- Terminal Mode remains the raw source-of-truth fallback

## Service status

A systemd user service is deferred. `harnessd serve` remains the documented
startup path. A future service feature must remain rootless, preserve the same
token and localhost defaults, and first be added to the normative command
nomenclature.
