# Testing Strategy

## Recommendation

Build the fake harness suite early and use it as the primary validation surface for PTY/process behavior, API contracts, WebSocket streaming, stale actions, and dashboard rendering. Real harness testing should happen after generic runtime correctness is proven.

## Alternatives Considered

| Alternative | Decision | Reason |
| --- | --- | --- |
| Test only with real harnesses | Reject | Real tools are versioned, authenticated, timing-sensitive, and hard to run in CI. |
| Unit tests only | Reject | PTY, signal, resize, WebSocket, and cleanup behavior require integration coverage. |
| Shell-only fake harnesses | Defer for simple cases | Useful for basic output, but Go fakes are better for signal and resize determinism. |
| Browser manual tests only | Reject | Manual checks are needed, but core protocol and process behavior must be automated. |

## Test Layers

| Layer | Purpose |
| --- | --- |
| Unit tests | Validate config, API schemas, event envelopes, ring buffers, redaction, adapter matching. |
| PTY integration tests | Validate process start, output, input, resize, interrupt, terminate, cleanup. |
| API tests | Validate REST request/response behavior and auth/CSRF requirements. |
| WebSocket tests | Validate event delivery, replay, auth, Origin checks, slow clients. |
| Adapter tests | Validate generic fallback and harness-specific detection/action mapping. |
| Dashboard tests | Validate API client, action cards, terminal lifecycle, reconnect behavior. |
| Manual tests | Validate real terminal UX and real harness behavior. |

## Fake Harness Fixtures

Place later fixtures under:

```text
testdata/fake-harnesses/
```

Prefer small Go programs for PTY-specific behavior when shell portability is weak. Shell scripts are acceptable for simple output.

### Plain Output Fake Harness

Behavior:

- prints fixed stdout lines
- prints fixed stderr lines
- exits `0`

Verifies:

- PTY launch
- combined output stream
- exit code capture

### Interactive Prompt Fake Harness

Behavior:

- prints `Name: `
- reads one line
- prints `Hello <name>`
- exits `0`

Verifies:

- input writing
- Enter handling
- prompt rendering without newline

### Approval Prompt Fake Harness

Behavior:

- prints approval-like text with command and cwd
- waits for one key or line
- accepts `y`, denies `n`
- emits clear result

Verifies:

- heuristic/adapter detection
- `approval.required`
- action execution
- stale action rejection after prompt resolved

### Full-Screen Redraw Fake Harness

Behavior:

- enters alternate screen
- repeatedly clears/redraws status area
- moves cursor
- exits after input

Verifies:

- raw ANSI preservation
- xterm.js rendering
- no line-parser dependency
- reconnect limitations

### Long-Running Fake Harness

Behavior:

- prints heartbeat every second
- handles Ctrl+C by exiting with known code/message

Verifies:

- streaming over time
- interrupt
- session status changes

### SIGTERM-Ignoring Fake Harness

Behavior:

- traps/ignores SIGTERM
- continues printing
- exits only on SIGKILL or specific input

Verifies:

- graceful terminate timeout
- force kill
- process group cleanup

### ANSI Escape Fake Harness

Behavior:

- prints colors, bold, hyperlinks if desired, cursor movement, and reset sequences

Verifies:

- raw byte capture
- xterm.js rendering
- snapshot replay

### Resize-Aware Fake Harness

Behavior:

- listens for `SIGWINCH`
- queries terminal rows/cols
- prints observed size

Verifies:

- frontend resize API
- backend `pty.Setsize`
- child sees updated terminal size

### Failure Exit Fake Harness

Behavior:

- prints error
- exits with nonzero code

Verifies:

- failed/exited status
- exit code capture
- event publication

### Child Process Fake Harness

Behavior:

- starts a child process in same process group
- child prints heartbeat
- parent waits

Verifies:

- process group termination kills descendants
- no orphan child after session cleanup

## Core Test Cases

### PTY And Process

- launch valid command
- reject invalid command/cwd
- environment merge visible in child
- stdin/stdout/stderr through PTY
- output reader exits on EOF
- exit code and signal recorded
- Ctrl+C interrupt
- SIGTERM group termination
- SIGKILL fallback
- no process leak after cleanup
- resize visible to child

### API

- health returns version/healthy
- create session validates command/cwd/terminal size
- list/detail return expected metadata
- input accepts valid base64 and rejects invalid/oversized payload
- resize validates bounds
- interrupt/terminate/kill require auth and CSRF
- snapshot returns bounded chunks
- events endpoint paginates/limits
- action endpoint rejects stale events

### WebSocket

- unauthenticated handshake rejected
- unexpected Origin rejected
- authenticated client receives lifecycle and terminal events
- `after_seq` replay works
- slow client does not block PTY reader
- disconnect cleans subscriber
- malformed client messages are rejected or ignored safely

### Security

- default bind address is localhost
- non-local bind requires explicit config and auth
- root startup fails by default
- CSRF missing token rejected
- session cookie has secure local attributes where applicable
- redaction masks configured secret keys
- audit log excludes raw input payload

### Dashboard

- session list renders backend data
- create session sends correct request
- terminal component writes output chunks
- terminal input posts raw base64
- resize posts rows/cols
- action cards render backend-provided actions
- terminate/kill confirmations work
- reconnect replays snapshot and resumes stream

## Manual Verification With Real Tools

After fake harnesses pass:

1. Run `/bin/bash`.
2. Verify prompt, typing, command output, Ctrl+C, resize, and terminate.
3. Run OpenCode through generic adapter.
4. Verify raw terminal usability.
5. Trigger or simulate an approval flow.
6. Record visible prompt patterns and whether actions are reliable.
7. Only then enable OpenCode-specific adapter behavior.

Manual test record should include:

- harness name/version
- command used
- cwd
- terminal size
- observed approval prompt text
- interrupt behavior
- resize behavior
- limitations

## Acceptance Criteria For Later Implementation

- Fake harnesses exist and can run in CI on Linux.
- PTY tests do not depend on real coding harnesses.
- API and WebSocket schema tests protect event contract stability.
- Security tests enforce localhost/auth/CSRF/Origin defaults.
- Stale action tests cover all rejection branches.
- Process cleanup tests prove no child remains after terminate/kill.
- Manual checklist is completed before declaring a real adapter validated.

## Risks And Limitations

- PTY tests can be timing-sensitive. Use explicit readiness markers and bounded timeouts.
- CI environments may have PTY limits or different shells. Prefer Go fake harnesses for critical behavior.
- Real harness behavior changes; manual validation must record versions.
- Dashboard terminal rendering requires browser/manual or Playwright-style tests in later phases.

## Sources

- [`github.com/creack/pty` package docs](https://pkg.go.dev/github.com/creack/pty)
- [`pty(7)` Linux manual](https://man7.org/linux/man-pages/man7/pty.7.html)
- [OWASP WebSocket Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html)
