# Daemon Service And Shim Safety Research

Date: 2026-07-26

## Scope

This note evaluates a rootless daemon service and the safety boundary between a
shim-launched local terminal, `harnessctl attach`, and the daemon-owned PTY.
HarnessRelay remains localhost-first, authenticated, and user-owned.

## systemd user-service findings

User units belong in:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/
```

They are controlled with `systemctl --user`; root is neither needed nor
desirable. Installing or removing a unit file requires `daemon-reload`.
Enabling a unit creates the login-time relationship declared by
`WantedBy=default.target`; starting and enabling are separate state changes and
should remain explicit CLI verbs. `enable --now` is a useful manual shorthand,
but HarnessRelay should not silently enable a service during `make install`.

The unit should use an absolute `ExecStart` path. A user manager does not
necessarily inherit the interactive shell's PATH, aliases, or shell startup
files. `Type=exec` lets a start operation fail if the executable itself cannot
be invoked. `Restart=on-failure` recovers crashes without restarting after an
intentional clean stop. Logs go to the user journal and are read with:

```bash
journalctl --user --unit harnessrelay.service
```

User lingering is not required for "start at login and run while logged in".
`loginctl enable-linger` changes machine-level login-manager state and is
therefore documented only as an optional administrator/user choice for running
before login or after logout; HarnessRelay does not invoke it.

Recommended unit:

```ini
[Unit]
Description=HarnessRelay local harness daemon

[Service]
Type=exec
ExecStart=/absolute/path/to/harnessd serve
Restart=on-failure
RestartSec=2s

[Install]
WantedBy=default.target
```

No working directory is required: production dashboard assets are embedded and
configuration uses the existing XDG lookup. The service does not override
bind/token settings, so the stable token and localhost-first defaults remain
the same as manual startup.

## Command and ownership decision

The public resource group is plural:

```text
harnessctl services install
harnessctl services uninstall
harnessctl services start
harnessctl services stop
harnessctl services restart
harnessctl services enable
harnessctl services disable
harnessctl services status
harnessctl services logs
```

There is currently one owned service, `harnessrelay.service`. The plural group
keeps the command taxonomy consistent with `shims` and leaves room for a future
session supervisor without inventing top-level aliases.

The generated unit carries an exact HarnessRelay ownership marker. Install
refuses to overwrite an unmanaged unit unless a future explicit force contract
is added. Uninstall removes only the exact owned file and reloads the user
manager. Tests use temporary XDG paths and a fake command runner.

## Shim safety decision

The PTY child remains daemon-owned. This means daemon death ends the managed
child, but it must not damage the controlling terminal. `harnessctl attach` is
the process that changes the real terminal to raw mode, so it is the correct
safety boundary:

- capture terminal state before changing it
- install termination/suspend handling while raw mode is active
- restore exactly once before returning or propagating a signal
- treat an unexpected WebSocket EOF as daemon disconnection, not success
- print the disconnect explanation only after terminal restoration
- give `stty sane` as backup recovery, not the normal path

A tmux or separate supervisor backend would improve daemon-restart survival,
but it is not needed to make terminal failure safe. It remains a future
backend rather than a silent substitution for the current common PTY API.

## Sources

- local systemd 259 manuals: `systemd.service(5)`, `systemd.exec(5)`,
  `systemd.unit(5)`, `systemctl(1)`, `journalctl(1)`, `loginctl(1)`
- systemd upstream manuals: <https://www.freedesktop.org/software/systemd/man/>
- Go terminal package guidance: <https://pkg.go.dev/golang.org/x/term>
- Linux termios interface: <https://man7.org/linux/man-pages/man3/termios.3.html>
