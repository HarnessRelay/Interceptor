# Phase 0 Research Summary

## Universal Harness Follow-Up

Cross-harness architecture work is documented in:

- `10-Universal-Harness-Architecture.md`
- `11-Cross-Harness-Capability-Research.md`
- `12-Permission-Approval-Model.md`

The PTY remains the universal source-of-truth base. Official structured
interfaces such as Codex app-server, OpenCode OpenAPI/SSE, and Grok ACP or
streaming JSON are adapter-specific higher layers that normalize into common
events/actions without replacing Terminal Mode.

## Decisions

Phase 0 validates a localhost-first Go daemon that owns terminal harnesses through Linux PTYs and exposes both raw terminal control and optional semantic actions to a web dashboard.

| Area | Recommendation | Why |
| --- | --- | --- |
| PTY package | Use [`github.com/creack/pty`](https://pkg.go.dev/github.com/creack/pty). | It is the maintained successor to `github.com/kr/pty`, supports `Start`, `StartWithSize`, `StartWithAttrs`, `Setsize`, and maps cleanly to Linux PTY behavior. |
| Process control | Launch every harness as a daemon-owned child in its own session/process group. | The daemon needs reliable interrupt, terminate, force-kill, resize, output capture, and cleanup semantics. |
| Terminal stream | Forward raw PTY bytes as the canonical Stage 1 display path. | TUIs emit ANSI/VT control streams, not stable line-oriented semantic records. |
| Browser renderer | Use [xterm.js](https://xtermjs.org/docs/) with `@xterm/addon-fit`. | It is the standard browser terminal renderer, supports raw VT streams, input capture, resize, scrollback, and maintained addons. |
| API | Use REST for commands/state reads and WebSocket for live events. | REST keeps mutating operations explicit; WebSocket handles live terminal output and lifecycle events. |
| Frontend | Use Vite + React + TypeScript for the first dashboard. | It is simple enough for Stage 1, familiar to many agents, and suitable for stateful terminal/session UI. |
| Static serving | Embed production dashboard assets in the Go daemon using `embed.FS`. | Keeps deployment to one binary while allowing Vite dev-server proxying during development. |
| Security | Bind to `127.0.0.1` by default, require auth, validate CSRF and WebSocket Origin. | The daemon can type into local shells and coding agents, so unauthenticated browser access is equivalent to local command control. |
| First real adapter | Target OpenCode first, while keeping generic adapter mandatory. | OpenCode is terminal-first, open source, documents permissions/server APIs, and has clear approval concepts for adapter validation. |
| Testing | Build fake harness fixtures before real adapter work. | Fake harnesses make PTY, resize, lifecycle, security, stale action, and rendering behavior repeatable in CI. |

## Implementation Boundaries

- Do not add mobile-app scope.
- Do not edit `Docs/Spec/Context.md` without owner approval.
- Do not weaken localhost-first security defaults.
- Do not design the common API around a single harness.
- Do not implement production features in Phase 0 beyond tiny validation snippets in documentation.
- Preserve raw terminal fallback for every session, including harness-specific sessions.

## Phase 0 Coverage Map

| Todo section | Research file |
| --- | --- |
| 0.1 PTY and Process Management Research | `01-PTY-And-Process-Management.md` |
| 0.2 Terminal Rendering Research | `02-Terminal-Rendering.md` |
| 0.3 API and Event Schema Research | `03-API-And-Events.md` |
| 0.4 Web Dashboard Research | `04-Web-Dashboard.md` |
| 0.5 Security Research | `05-Security.md` |
| 0.6 Harness Research | `06-Harness-Research.md` |
| 0.7 Testing Research | `07-Testing-Strategy.md` |

## Acceptance Gate For Later Agents

Before Phase 1 or later implementation starts, agents should be able to answer:

- Which package starts PTY sessions and how it is called.
- Which process group receives interrupt, terminate, and kill signals.
- Which API endpoint handles input, resize, interrupt, terminate, snapshot, event history, and semantic actions.
- Which WebSocket event envelope every event uses.
- Which auth, CSRF, and Origin rules are mandatory.
- Which fake harnesses must exist before runtime work is considered verified.

## Proposed Context Updates

### Proposal: Record Phase 0 Technology Decisions

Section to update:
`Language and Stack`

Proposed text:
Add that Stage 1 should use `github.com/creack/pty` for Go PTY management, xterm.js for browser terminal rendering, and Vite + React + TypeScript for the initial web dashboard unless the owner explicitly chooses otherwise.

Reason:
These decisions remove ambiguity for smaller agents starting Phase 1 through Phase 10.

Scope impact:
No scope expansion. This narrows implementation choices inside the existing Go daemon and web dashboard scope.

Requires owner approval: yes

### Proposal: Record First Real Harness Adapter Target

Section to update:
`Harness Adapter Strategy`

Proposed text:
Use OpenCode as the first real harness-specific adapter target after the generic adapter is working. This does not make the common API OpenCode-specific, and the generic adapter remains mandatory for all sessions.

Reason:
OpenCode is terminal-first, open source, and currently documents server, SDK, command, and permission behavior useful for validation.

Scope impact:
No scope expansion. It selects the first adapter proof target for Phase 12.

Requires owner approval: yes

### Proposal: Clarify Local Auth Default

Section to update:
`Security Requirements`

Proposed text:
Even when bound to localhost, Stage 1 should require authentication for dashboard, REST API, and WebSocket access. A first-run generated local token or user-configured password is acceptable for development.

Reason:
Local web pages from other origins can still attempt requests to localhost services. Authentication and CSRF/Origin checks are needed because the daemon can control local terminal sessions.

Scope impact:
Security clarification only.

Requires owner approval: yes
