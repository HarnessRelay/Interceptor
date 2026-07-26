# HarnessRelay Interceptor — Project Context

## Purpose

HarnessRelay Interceptor is a Linux desktop/service-side application that launches, supervises, intercepts, and exposes terminal-based coding harnesses through a local web dashboard.

The goal of this stage is to build a reliable interceptor plus web application. The system should allow a user to run coding harnesses inside controlled pseudo-terminal sessions, view and interact with those sessions from a browser, and later support harness-specific semantic controls through a common backend API.

This document intentionally excludes any mobile application scope. Do not add mobile-app requirements, screens, Flutter architecture, push notification design, or mobile-specific assumptions unless the project owner explicitly approves a scope change.

## Current Stage

Stage 1 is focused only on:

- Interceptor daemon
- Web dashboard
- Common API
- Session management
- PTY-based process control
- Generic terminal access
- Harness adapter architecture
- Documentation and testable implementation plan

The web dashboard is the first client. It should be treated as the reference implementation for the common API.

For the Stage 1 web direction, the dashboard should be a chat-first harness control interface with a selectable raw terminal mode. Terminal mode remains mandatory as the universal fallback because the raw PTY stream is the source of truth for unsupported or uncertain harness behavior.

## Core Problem

Many coding harnesses run as terminal user interfaces. Examples include:

- Codex-style coding CLIs
- Claude Code-style coding CLIs
- OpenCode-style tools
- Kilo Code-style tools
- Command-line coding agents
- Other terminal-based AI development harnesses

These tools commonly ask users for approvals before executing commands, editing files, or performing potentially risky actions. The user wants a controlled middle layer that can run those tools with partial autonomy while still allowing review, interruption, and interaction through a browser.

The interceptor should make it possible to:

- Launch harnesses from one controlled runtime
- Keep sessions alive independently of a visible terminal window
- View live terminal output in a web dashboard
- Send keyboard input back to the harness
- Interrupt or terminate sessions
- Expose a common API that is not tied to one harness
- Add harness-specific adapters over time

## Important Technical Reality

A terminal UI usually does not expose semantic information such as:

- “This is an Approve button”
- “This is a Deny button”
- “This is a chat input”
- “This is a command palette”

Most TUIs write text and terminal escape sequences to a pseudo-terminal. Therefore, the interceptor can reliably capture terminal input/output, but it cannot universally understand every UI element without either:

1. Harness-specific adapters
2. Heuristics
3. Official machine-readable APIs/hooks from a harness
4. Raw terminal fallback

The product must be designed around this reality.

## Recommended Architecture

The interceptor should launch and own harness processes inside PTYs instead of trying to intercept already-running terminals.

```text
Browser Web Dashboard
        │
        │ HTTP / WebSocket
        ▼
Go Interceptor Daemon
        ├── API server
        ├── WebSocket event stream
        ├── Session manager
        ├── PTY manager
        ├── Process lifecycle manager
        ├── Terminal output handling
        ├── Harness adapter registry
        ├── Event bus
        ├── Audit/history storage
        └── Static web dashboard
                │
                ▼
          Pseudo-terminal
                │
                ▼
       Harness process / shell command
```

## Main Design Principle

Always provide generic terminal control first. Add semantic harness support progressively.

A supported harness may eventually expose clean actions such as “Approve once” or “Deny,” but unsupported harnesses must still work through raw terminal interaction.

The generic fallback is mandatory.

## Universal Harness Guardrails

HarnessRelay is a universal local harness relay, not a Codex-specific
controller.

The common architecture must remain harness-neutral. Common code may define
session lifecycle, PTY control, event/action envelopes, approval/permission
request models, capability vocabulary, command catalog transport, stale-action
validation, and terminal fallback behavior.

Harness-specific behavior belongs in adapters. This includes command matching,
prompt byte sequences, TUI parsing, model/version extraction, approval prompt
parsing, native command catalogs, and action execution.

The frontend must compose common actions and adapter-provided commands
dynamically from session capabilities, semantic events, and command catalogs.
It must not hardcode one harness's commands or decision model.

Codex, OpenCode, Grok, Claude Code, Kilo Code, and future harnesses must fit the
same common contracts where possible. If a change requires harness-specific
behavior in common code, stop and ask the project owner for explicit approval.

Terminal Mode is mandatory and remains the source-of-truth fallback. No task
may add automatic approval by default, weaken localhost-first security, remove
raw terminal access, or add mobile-app scope without explicit owner approval.

## Out of Scope for Stage 1

Do not implement or design the following unless explicitly approved:

- Mobile app
- Flutter client
- Cloud relay service
- Public SaaS backend
- Multi-user enterprise control plane
- Browser extension
- IDE extension
- Full AI-agent orchestration platform
- Production-grade remote access over the public internet
- Automatic approval policies that bypass user review
- Plugin marketplace

## Language and Stack

Primary implementation language:

- Go

Reasoning:

- Good fit for PTY handling
- Good process management support
- Strong concurrency model
- Easy single-binary deployment
- Suitable for Linux daemon/service development
- Familiar enough to the project owner

Frontend:

- Web application served by the Go daemon
- A browser terminal component may be used for rendering raw terminal output
- The frontend should communicate only through the common API and WebSocket protocol

Persistence:

- SQLite is acceptable for Stage 1 metadata, events, sessions, and audit records
- Avoid complex external dependencies in the first version

## Invocation Model

Preferred model:

```bash
harnessd serve
```

Starts the long-running interceptor daemon.

```bash
harnessctl run claude
```

Creates a new managed session through the daemon.

```bash
harnessctl run --name payroll-api --cwd ~/projects/payroll-api claude
```

Creates a named session in a specific working directory.

```bash
harnessctl attach <session-id>
```

Optional CLI terminal attachment to an existing session.

The daemon should own the process. CLI commands should act as clients of the daemon.

## Daemon Model

The daemon should normally run as a user-level process, not as root.

Future systemd user service target:

```bash
systemctl --user enable --now harnessd
```

Default bind address must be local-only:

```text
127.0.0.1
```

Do not bind to `0.0.0.0` by default.

Remote browser access should only be considered through secure private networking or explicit user configuration.

## Session Model

A session represents one running or completed harness process.

Suggested fields:

```text
Session ID
Name
Harness type
Command
Arguments
Working directory
Environment
PTY size
Status
PID / process group ID
Started timestamp
Exited timestamp
Exit code
Last activity timestamp
Adapter ID
```

Suggested statuses:

```text
starting
running
idle
waiting_for_input
waiting_for_approval
interrupted
exited
failed
terminated
```

Some statuses may initially be heuristic. When detection is uncertain, preserve raw terminal control and avoid pretending that semantic detection is reliable.

## PTY and Process Requirements

The runtime should:

- Allocate a PTY for each session
- Start each harness inside the PTY
- Read output from the PTY continuously
- Write user input back to the PTY
- Support terminal resizing
- Track process exit
- Start harnesses in a separate process group where appropriate
- Support interrupting the foreground process
- Support terminating the whole process group
- Preserve enough output history for reconnects and debugging

The process lifecycle manager should be separate from harness-specific adapters.

## Terminal Output Strategy

The system should maintain at least two output paths:

1. Raw terminal stream
2. Normalized events

Raw terminal stream:

- Used by the browser terminal
- Used for unsupported harnesses
- Used for debugging and replay
- Should preserve ANSI/VT escape sequences when needed

Normalized events:

- Used for common API semantics
- Used for approval cards
- Used for status changes
- Used for audit records
- Produced by generic heuristics or harness adapters

Do not depend only on line-based stdout parsing. Full-screen TUIs often redraw the same screen region and may not emit clean lines.

## Harness Adapter Strategy

Adapters should be small, capability-based, and optional.

There must always be a generic adapter.

The generic adapter should support:

- Raw terminal display
- Text input
- Special keys
- Ctrl+C / interrupt
- Resize
- Terminate
- Basic regex or visible-text detection
- No harness-specific assumptions

Harness-specific adapters may support:

- Approval detection
- Approval context extraction
- Approve action
- Deny action
- Submit prompt
- Open command list
- Change model
- Detect waiting state
- Detect current operation
- Detect command being requested
- Detect risky actions

Avoid one large interface that every harness must implement. Prefer a core adapter interface plus optional capability interfaces.

Suggested conceptual model:

```go
type HarnessAdapter interface {
    ID() string
    Match(spec LaunchSpec) MatchResult
    Parse(update TerminalUpdate) []Event
}
```

Optional capabilities may include:

```go
ApprovalHandler
PromptHandler
CommandPaletteHandler
ModelHandler
InterruptHandler
```

## Common API Principles

The API should not assume any one harness.

The frontend should render backend-provided sessions, events, capabilities, and actions.

Avoid hardcoding universal assumptions such as:

- Every approval has only Approve and Deny
- Every harness has a model switcher
- Every harness has a slash command menu
- Every harness exposes clean chat messages
- Every harness has the same permission levels

Instead, expose generic actions:

```json
{
  "event_id": "evt_123",
  "type": "approval.required",
  "actions": [
    {
      "id": "approve_once",
      "label": "Approve once",
      "style": "primary"
    },
    {
      "id": "deny",
      "label": "Deny",
      "style": "danger"
    }
  ]
}
```

The frontend should display actions supplied by the backend.

## Suggested API Surface

Initial REST endpoints:

```text
GET    /api/v1/health
GET    /api/v1/sessions
POST   /api/v1/sessions
GET    /api/v1/sessions/{id}
DELETE /api/v1/sessions/{id}

POST   /api/v1/sessions/{id}/input
POST   /api/v1/sessions/{id}/resize
POST   /api/v1/sessions/{id}/interrupt
POST   /api/v1/sessions/{id}/terminate

GET    /api/v1/sessions/{id}/snapshot
GET    /api/v1/sessions/{id}/events
POST   /api/v1/sessions/{id}/actions/{action_id}

GET    /api/v1/harnesses
GET    /api/v1/capabilities
```

Initial WebSocket endpoint:

```text
GET /api/v1/ws
```

WebSocket events should include:

```text
terminal.output
terminal.snapshot
session.created
session.updated
session.exited
session.status_changed
approval.required
action.completed
action.failed
error
```

## Security Requirements

The interceptor has access similar to a terminal session. It can type commands, interact with coding agents, and indirectly access source code, SSH keys, credentials, and local files.

Security must be part of Stage 1.

Minimum expectations:

- Bind to localhost by default
- Require authentication for web dashboard and API
- Protect WebSocket connections
- Avoid public network exposure by default
- Use CSRF protections where applicable
- Validate origins for browser clients
- Use audit logging for sensitive actions
- Tie semantic actions to active event IDs
- Reject stale approval actions
- Do not silently auto-approve actions
- Clearly display command, working directory, and session before approving
- Avoid running daemon as root
- Avoid storing secrets in plain logs

## Web Dashboard Scope

The Stage 1 web dashboard should support:

- Session list
- Create session
- View live terminal
- Send terminal input
- Resize terminal
- Interrupt session
- Terminate session
- View session status
- View basic event history
- View semantic approval cards if produced by adapters
- Fall back to raw terminal at all times

The web dashboard is not expected to be beautiful in the first milestone. It should be functional, reliable, and useful for validating the backend architecture.

## Testing Expectations

The project should include tests for:

- Session creation
- PTY process start and exit
- Input writing
- Output reading
- Resize handling
- Interrupt handling
- Termination handling
- WebSocket event delivery
- REST API behavior
- Adapter matching
- Generic adapter fallback
- Event schema stability
- Stale action rejection
- Security defaults

Use small fake terminal programs and test harnesses instead of depending only on real external coding harnesses.

## Recommended Fake Harnesses for Tests

Create simple fake harness programs/scripts that simulate:

- Plain stdout output
- Interactive prompt
- Full-screen redraw
- Approval request
- Long-running process
- Process that ignores SIGTERM
- Process that exits with failure
- Process that emits ANSI escape sequences
- Process that requests terminal resize awareness

These fake harnesses make the interceptor testable without relying on real tools.

## Project Documentation Rules

This repository contains two important documents:

```text
Docs/Spec/Context.md
Docs/Spec/Todo.md
```

These files are part of the operating instructions for AI agents working on the project.

### Context.md Rules

`Context.md` is the stable project memory.

Do not edit `Context.md` unless the project owner explicitly requests or approves the change.

If new information appears during implementation, propose a context update separately instead of editing it silently.

Any proposed context update should include:

- The exact section to update
- The proposed replacement or addition
- The reason the update is needed
- Whether it changes project scope, architecture, security, or assumptions

Never add mobile-app scope to `Context.md` unless explicitly approved.

Never remove security requirements to make implementation easier.

Never change the core architecture from PTY-owned sessions to already-running-terminal interception without explicit approval.

### Todo.md Rules

`Todo.md` is the active execution plan.

Agents may update task checkboxes in `Todo.md` as work is completed.

Agents may add new tasks to `Todo.md` only if they follow the task template at the bottom of the file.

When adding new tasks:

- Place them under the correct phase
- Use checkboxes
- Include enough detail for a smaller agent to execute
- Include acceptance criteria when possible
- Do not silently change completed task history
- Do not delete unfinished tasks unless explicitly approved
- Do not create vague tasks such as “Improve system” or “Clean code”
- Break large tasks into concrete, verifiable steps

If a task reveals a scope or architecture change, update `Todo.md` with a proposed task but do not alter `Context.md` without approval.

## AI Agent Handoff Rules

Small AI agents working on this repository must:

1. Read `Docs/Spec/Context.md` before making changes.
2. Read `Docs/Spec/Todo.md` before selecting work.
3. Work on one small task at a time unless instructed otherwise.
4. Prefer simple, testable implementation steps.
5. Keep the generic adapter working at all times.
6. Avoid hardcoding one harness into the common API.
7. Add tests for new behavior.
8. Update Todo checkboxes only after verifying the task.
9. Ask for explicit permission before editing Context.md.
10. Do not add mobile-app scope.
11. Do not weaken security defaults.
12. Do not expose the daemon publicly by default.
13. Do not run harness processes as root.
14. Do not auto-approve harness actions.
15. Preserve raw terminal fallback for every session.

## Definition of a Good Stage 1 Result

Stage 1 is successful when:

- A Go daemon can launch harness-like commands inside PTYs
- A browser dashboard can display and interact with sessions
- Sessions can be listed, opened, interrupted, resized, and terminated
- Raw terminal interaction works for generic commands
- The common API is usable by the web frontend
- A generic adapter exists
- At least one harness-specific adapter can be added without changing core process control
- Security defaults are safe
- Tests cover the core runtime
- Documentation is clear enough for smaller AI agents to continue work
