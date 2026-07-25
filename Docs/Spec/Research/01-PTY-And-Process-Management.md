# PTY And Process Management

## Recommendation

Use [`github.com/creack/pty`](https://pkg.go.dev/github.com/creack/pty) as the initial Go PTY package. Launch each harness command as a daemon-owned child process attached to a PTY, with the child isolated in its own session/process group and tracked by session metadata.

The runtime should expose a small `pty` package internally, not leak the third-party API across the whole codebase:

```go
type LaunchSpec struct {
    Command string
    Args    []string
    Cwd     string
    Env     map[string]string
    Rows    uint16
    Cols    uint16
}

type Process struct {
    SessionID string
    PID       int
    PGID      int
    PTY       *os.File
}
```

## Reasoning

- `creack/pty` provides `Start`, `StartWithSize`, `StartWithAttrs`, `Setsize`, and size helpers.
- The older `github.com/kr/pty` package has moved to `github.com/creack/pty`, so new code should use the maintained import path.
- Linux PTYs are byte streams with a master/slave pair. Per [`pty(7)`](https://man7.org/linux/man-pages/man7/pty.7.html), writes to the master are delivered as terminal input to the slave process, and writes from the slave are read from the master.
- TUIs often require a real terminal: stdout/stderr merging, terminal size, cursor movement, alternate screen, and interrupt characters all matter.
- Starting the process in a separate process group allows the daemon to terminate descendants that the harness started.

## Alternatives Considered

| Alternative | Decision | Reason |
| --- | --- | --- |
| `github.com/kr/pty` | Reject | It redirects/moved to `creack/pty`; use the new path. |
| Manual `syscall.Openpty`/`forkpty` wrapper | Reject for Stage 1 | More control, but more Linux-specific code and greater risk. |
| Pipes only | Reject | Pipes do not provide terminal semantics, raw mode, terminal size, or TUI behavior. |
| Attach to already-running terminal | Reject | Conflicts with project context. The daemon should launch and own sessions. |

## Launch Design

1. Validate command is non-empty and cwd exists.
2. Build `exec.Cmd` with explicit `Dir`.
3. Build environment from `os.Environ()` plus session-specific overrides.
4. Set terminal size before start with `pty.StartWithSize`.
5. Use process attributes that create a separate session/controlling terminal unless tests show `StartWithSize` default behavior is sufficient.
6. After start, record `PID` and `PGID`.
7. Start one goroutine to read PTY output until EOF/error.
8. Start one goroutine to `Wait()` on the process and publish exit status.
9. On cleanup, close the PTY master and reap the child.

Example shape:

```go
cmd := exec.Command(spec.Command, spec.Args...)
cmd.Dir = spec.Cwd
cmd.Env = mergeEnv(os.Environ(), spec.Env)

ws := &pty.Winsize{Rows: spec.Rows, Cols: spec.Cols}
ptmx, err := pty.StartWithSize(cmd, ws)
if err != nil {
    return nil, err
}

pid := cmd.Process.Pid
pgid, _ := syscall.Getpgid(pid)
```

If `StartWithAttrs` is needed to force attributes explicitly, use:

```go
attrs := &syscall.SysProcAttr{
    Setsid:  true,
    Setctty: true,
}
ptmx, err := pty.StartWithAttrs(cmd, ws, attrs)
```

Implementation note: verify the exact `SysProcAttr` combination on Linux before finalizing. `creack/pty.Start`/`StartWithSize` already starts a new session and sets the controlling terminal according to its docs.

## Linux PTY Behavior To Preserve

- Connect child stdin, stdout, and stderr to the PTY slave. Consumers read one combined terminal stream from the PTY master.
- Preserve raw bytes, including ANSI/VT escape sequences and alternate-screen control.
- Do not depend only on line parsing. Full-screen TUIs redraw cells and may never emit stable lines.
- Treat terminal size as rows/cols, with optional pixel fields if later needed.
- Terminal raw mode is usually controlled by the child TUI. The daemon should not put the child PTY into a nonstandard mode unless a test proves it is necessary.
- PTY reads may return partial escape sequences; stream events must preserve order and sequence numbers.

## Process Groups, Signals, And Interrupts

### Ctrl+C

Default Stage 1 interrupt should write byte `0x03` to the PTY master. Linux PTY semantics generate `SIGINT` for the foreground process group when the terminal has `ISIG` enabled, which matches a user pressing Ctrl+C in a terminal.

Fallback interrupt API:

```go
syscall.Kill(-pgid, syscall.SIGINT)
```

Use fallback only when raw Ctrl+C does not work or for explicit process-control interrupt. Per [`kill(2)`](https://man7.org/linux/man-pages/man2/kill.2.html), a negative PID less than `-1` targets the process group whose ID is `-pid`.

### Graceful Termination

Terminate the whole process group:

```go
syscall.Kill(-pgid, syscall.SIGTERM)
```

Then wait for a bounded grace period, recommended default `5s`.

### Force Kill

If the process group still exists after graceful termination:

```go
syscall.Kill(-pgid, syscall.SIGKILL)
```

Then wait for `cmd.Wait()` to complete. Treat `ESRCH` as already gone.

### Cleanup

- Always call `Wait()` exactly once.
- Close PTY master on session end.
- Cancel output reader context when the session exits.
- Publish `session.exited` or `session.terminated` with exit code/signal.
- If daemon shutdown occurs, send `SIGTERM` to all running process groups, then `SIGKILL` after timeout.
- Do not kill arbitrary PIDs. Only signal stored PGIDs created by this daemon.

## Resize Design

Backend endpoint receives rows/cols:

```json
{ "rows": 40, "cols": 120 }
```

Validate:

- `rows` between `1` and `500`
- `cols` between `2` and `1000`

Apply:

```go
err := pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
```

Terminal programs observe resize through `SIGWINCH`/terminal window-size queries. xterm.js recommends debounce around resize; frontend should fit the terminal container and send the final rows/cols after layout settles.

## Risks And Limitations

- Process group handling can be subtle when shells start their own jobs. Fake harness tests must validate descendants are killed.
- Writing `0x03` depends on terminal `ISIG`; some programs may run in modes that handle Ctrl+C themselves.
- A child may daemonize or escape the process group. Stage 1 should document this limitation and not promise container-grade isolation.
- PTY output can be high volume. Readers must avoid blocking on slow WebSocket clients.
- Linux-only behavior is acceptable for Stage 1, but keep platform-specific code isolated.

## Acceptance Criteria For Later Implementation

- A PTY session can start `/bin/sh` or `/bin/bash` with cwd and environment overrides.
- stdout and stderr appear in the same ordered raw terminal stream.
- Session metadata records PID, PGID, cwd, command, rows, cols, and status.
- Ctrl+C interrupts a foreground long-running command.
- Terminate sends `SIGTERM` to the process group and records termination.
- Force kill stops a SIGTERM-ignoring process group.
- Resize changes are visible to a resize-aware fake harness.
- Closing a session does not leak goroutines, file descriptors, or child processes.

## Required Tests

- Start command exits successfully and reports exit code.
- Start command in missing cwd fails before launch.
- Environment variable override is visible inside child process.
- stdout/stderr ordering is preserved as read from PTY.
- Ctrl+C stops `sleep 30` or an equivalent fake.
- `SIGTERM` stops parent and child process in same process group.
- SIGTERM-ignoring fake requires `SIGKILL`.
- PTY resize fake observes updated rows/cols.
- Output reader handles EOF and exits.
- Daemon shutdown cleanup terminates all daemon-owned process groups.

## Sources

- [`github.com/creack/pty` package docs](https://pkg.go.dev/github.com/creack/pty)
- [`pty(7)` Linux manual](https://man7.org/linux/man-pages/man7/pty.7.html)
- [`kill(2)` Linux manual](https://man7.org/linux/man-pages/man2/kill.2.html)
- [`termios(3)` Linux manual](https://www.man7.org/linux/man-pages/man3/termios.3.html)
