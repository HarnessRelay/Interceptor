# HarnessRelay Interceptor — Todo

## How To Use This Todo

This file is the active implementation checklist for the HarnessRelay Interceptor project.

Rules for agents:

- Read `Docs/Spec/Context.md` before starting any task.
- Work from top to bottom unless the project owner gives a different priority.
- Check off a task only after it is implemented and verified.
- Keep tasks small and concrete.
- Add new tasks using the template at the bottom.
- Do not edit `Context.md` without explicit project owner permission.
- Do not add mobile-app scope.
- Do not weaken security defaults.
- Do not remove the raw terminal fallback.
- Do not hardcode one harness into the common API.
- Add or update tests when behavior changes.

---

# Phase 0 — Research and Design Validation

Goal: research the technical foundations, best practices, and implementation risks before committing too much code.

## 0.1 PTY and Process Management Research

- [x] Research Go PTY libraries and choose the initial PTY package.
  - Acceptance criteria:
    - Selected package is documented in project notes.
    - Reasons for choosing it are recorded.
    - Known limitations are recorded.

- [x] Research Linux PTY behavior relevant to TUIs.
  - Cover:
    - stdin/stdout/stderr behavior
    - terminal size
    - raw mode
    - process groups
    - signal handling
    - child process cleanup

- [x] Research how to safely start commands in their own process group.
  - Acceptance criteria:
    - Interrupt strategy is documented.
    - Termination strategy is documented.
    - Whole-process-group cleanup is documented.

- [x] Research terminal resize behavior.
  - Acceptance criteria:
    - Backend resize API requirements are known.
    - Frontend resize behavior is understood.

## 0.2 Terminal Rendering Research

- [x] Research browser terminal rendering options.
  - Expected candidate:
    - xterm.js or equivalent

- [x] Decide whether Stage 1 forwards raw PTY bytes to the browser terminal.
  - Expected decision:
    - Yes, raw PTY forwarding is the first rendering path.

- [x] Research terminal output persistence options.
  - Cover:
    - Raw byte capture
    - Scrollback
    - Snapshot strategy
    - Replay/debug strategy

- [x] Decide the minimum terminal history retained per session.
  - Acceptance criteria:
    - Default limit is defined.
    - Storage impact is considered.

## 0.3 API and Event Schema Research

- [x] Research common patterns for WebSocket event streams.
- [x] Research REST + WebSocket API separation.
- [x] Draft initial API schema for sessions, terminal input, resize, interrupt, terminate, and actions.
- [x] Draft initial WebSocket event schema.
- [x] Ensure schemas do not assume a specific harness.

## 0.4 Web Dashboard Research

- [x] Research minimal web dashboard structure.
- [x] Decide frontend stack.
  - Options may include:
    - Plain HTML/TypeScript
    - React
    - Svelte
    - Other lightweight frontend approach

- [x] Decide how the Go daemon serves the web app.
  - Acceptance criteria:
    - Static asset strategy is documented.
    - Development workflow is documented.

## 0.5 Security Research

- [x] Research safe localhost-first daemon defaults.
- [x] Research browser authentication options suitable for local tools.
- [x] Research CSRF protection requirements.
- [x] Research WebSocket authentication requirements.
- [x] Define safe default bind address.
  - Expected default:
    - `127.0.0.1`

- [x] Define rules for exposing the dashboard beyond localhost.
  - Acceptance criteria:
    - Must be explicit.
    - Must require authentication.
    - Must not be enabled by default.

## 0.6 Harness Research

- [x] Research how common coding harnesses behave in a terminal.
- [x] Identify common approval prompt patterns.
- [x] Identify common command palette patterns.
- [x] Identify common interruption behavior.
- [x] Record which tools expose official hooks, JSON output, SDKs, or permission APIs.
- [x] Select the first real harness adapter target.
  - Acceptance criteria:
    - Reason for selection is documented.
    - Generic adapter still remains the priority.

## 0.7 Testing Research

- [x] Research how to test PTY-based Go programs.
- [x] Define fake harness scripts/programs for testing.
- [x] Define core test cases.
- [x] Define integration test approach.
- [x] Define manual verification checklist for real harnesses.

---

# Phase 1 — Repository and Project Foundation

Goal: create a clean Go project foundation that small agents can work on safely.

## 1.1 Go Project Setup

- [x] Initialize Go module.
- [x] Add standard project directories.
- [x] Add basic `.gitignore`.
- [x] Add README with project purpose and Stage 1 scope.
- [ ] Add license if project owner chooses one.
- [x] Add basic Makefile or task runner.
- [x] Add formatting/linting command.
- [x] Add test command.

## 1.2 Suggested Directory Structure

- [x] Create command directories:
  - `cmd/harnessd`
  - `cmd/harnessctl`

- [x] Create internal package directories:
  - `internal/api`
  - `internal/session`
  - `internal/pty`
  - `internal/terminal`
  - `internal/harness`
  - `internal/harness/generic`
  - `internal/events`
  - `internal/storage`
  - `internal/security`
  - `internal/config`

- [x] Create web directory:
  - `web`

- [x] Create test support directory:
  - `testdata/fake-harnesses`

## 1.3 Configuration Foundation

- [x] Define config file format.
- [x] Define config search paths.
- [x] Define default bind address as `127.0.0.1`.
- [x] Define default port.
- [x] Define default storage path.
- [x] Define default terminal history limits.
- [x] Add config loading tests.

## 1.4 Logging Foundation

- [x] Add structured logging.
- [x] Add log levels.
- [x] Ensure sensitive data is not logged by default.
- [x] Add request/session IDs to relevant logs.

---

# Phase 2 — Daemon Foundation

Goal: create a working `harnessd` daemon with health checks and basic lifecycle.

## 2.1 Daemon CLI

- [x] Implement `harnessd serve`.
- [x] Implement config loading.
- [x] Implement graceful shutdown.
- [x] Handle SIGINT and SIGTERM.
- [x] Log startup configuration safely.

## 2.2 Health API

- [x] Implement `GET /api/v1/health`.
- [x] Include version/build info if available.
- [x] Add tests for health endpoint.

## 2.3 Static Web Serving

- [x] Serve placeholder web dashboard.
- [x] Ensure API routes and static routes do not conflict.
- [x] Add local development instructions.

---

# Phase 3 — PTY Runtime

Goal: launch and control terminal processes through PTYs.

## 3.1 PTY Launch

- [x] Implement PTY process start.
- [x] Support command and arguments.
- [x] Support working directory.
- [x] Support environment variables.
- [x] Start process in a suitable process group.
- [x] Track PID and process group ID.
- [x] Add tests using fake harnesses.

## 3.2 PTY Output

- [ ] Continuously read PTY output.
- [ ] Publish output to internal event bus.
- [x] Preserve raw bytes.
- [x] Handle EOF and process exit.
- [x] Avoid goroutine leaks.
- [x] Add tests for output capture.

## 3.3 PTY Input

- [x] Implement writing raw input to PTY.
- [x] Implement text input.
- [ ] Implement special key support.
- [ ] Implement Enter, Escape, Tab, arrows, and Ctrl+C.
- [x] Add tests for input behavior.

## 3.4 Resize Support

- [x] Implement PTY resize operation.
- [ ] Add API-safe resize model.
- [x] Add tests for resize handling.

## 3.5 Interrupt and Termination

- [x] Implement interrupt action.
- [x] Implement graceful terminate action.
- [x] Implement force kill action if graceful termination fails.
- [x] Ensure child processes are cleaned up when possible.
- [x] Add tests for process termination.

---

# Phase 4 — Session Manager

Goal: manage multiple sessions reliably.

## 4.1 Session Creation

- [x] Implement session creation service.
- [x] Generate unique session IDs.
- [x] Store session metadata.
- [x] Validate command input.
- [x] Reject invalid working directories.
- [ ] Expose create-session API.

## 4.2 Session Listing and Reading

- [x] Implement session list.
- [x] Implement session detail.
- [x] Include status, command, working directory, timestamps, and adapter ID.
- [x] Add tests for session list/detail APIs.

## 4.3 Session Status Tracking

- [x] Track `starting`.
- [x] Track `running`.
- [x] Track `exited`.
- [x] Track `failed`.
- [x] Track `terminated`.
- [ ] Add preliminary idle detection if simple enough.
- [ ] Emit status change events.

## 4.4 Session Cleanup

- [ ] Define cleanup rules for exited sessions.
- [x] Define whether exited sessions remain visible.
- [ ] Add manual cleanup API if needed.
- [ ] Ensure cleanup does not delete useful audit history unexpectedly.

---

# Phase 5 — Event Bus and WebSocket Stream

Goal: stream live session events to the web dashboard.

## 5.1 Event Types

- [x] Define event envelope.
- [x] Define `terminal.output`.
- [x] Define `session.created`.
- [x] Define `session.updated`.
- [x] Define `session.exited`.
- [x] Define `session.status_changed`.
- [x] Define `error`.

## 5.2 Internal Event Bus

- [x] Implement publish/subscribe event bus.
- [x] Support multiple subscribers.
- [x] Avoid blocking PTY output on slow clients.
- [x] Add tests for event fanout.

## 5.3 WebSocket Endpoint

- [ ] Implement `/api/v1/ws`.
- [ ] Authenticate WebSocket connections.
- [ ] Stream terminal output events.
- [ ] Stream session lifecycle events.
- [ ] Handle client disconnects cleanly.
- [ ] Add tests where practical.

---

# Phase 6 — REST API

Goal: expose stable backend operations for the web dashboard and future clients.

## 6.1 Session APIs

- [ ] Implement `GET /api/v1/sessions`.
- [ ] Implement `POST /api/v1/sessions`.
- [ ] Implement `GET /api/v1/sessions/{id}`.
- [ ] Implement `DELETE /api/v1/sessions/{id}`.

## 6.2 Terminal Control APIs

- [ ] Implement `POST /api/v1/sessions/{id}/input`.
- [ ] Implement `POST /api/v1/sessions/{id}/resize`.
- [ ] Implement `POST /api/v1/sessions/{id}/interrupt`.
- [ ] Implement `POST /api/v1/sessions/{id}/terminate`.

## 6.3 Snapshot and Events APIs

- [ ] Implement `GET /api/v1/sessions/{id}/snapshot`.
- [ ] Implement `GET /api/v1/sessions/{id}/events`.
- [ ] Add pagination or limits for event history.
- [ ] Add tests for event retrieval.

## 6.4 Semantic Action API

- [ ] Implement `POST /api/v1/sessions/{id}/actions/{action_id}`.
- [ ] Require `event_id` for event-bound actions.
- [ ] Reject stale or unknown actions.
- [ ] Return clear action result.
- [ ] Add tests for stale action rejection.

---

# Phase 7 — Storage and Audit History

Goal: persist enough information for session history, reconnects, debugging, and auditability.

## 7.1 SQLite Setup

- [ ] Add SQLite dependency.
- [ ] Create schema migration system.
- [ ] Create sessions table.
- [ ] Create events table.
- [ ] Create audit table.
- [ ] Add migration tests.

## 7.2 Session Persistence

- [ ] Persist session metadata.
- [ ] Persist session status changes.
- [ ] Persist process exit info.
- [ ] Recover session metadata on daemon restart.
- [ ] Clearly mark processes that cannot be reattached after daemon restart.

## 7.3 Terminal History

- [ ] Store bounded terminal output history.
- [ ] Define truncation behavior.
- [ ] Avoid unbounded database growth.
- [ ] Add tests for history limits.

## 7.4 Audit Logging

- [ ] Audit session creation.
- [ ] Audit input submission where appropriate.
- [ ] Audit interrupts.
- [ ] Audit termination.
- [ ] Audit semantic actions.
- [ ] Do not log secrets or full sensitive prompts by default unless explicitly configured.

---

# Phase 8 — Generic Harness Adapter

Goal: make every terminal-based harness usable even without a dedicated adapter.

## 8.1 Adapter Registry

- [ ] Define adapter interface.
- [ ] Define adapter match result.
- [ ] Implement adapter registry.
- [ ] Ensure generic adapter is always available.
- [ ] Ensure generic adapter has lowest priority.
- [ ] Add tests for adapter selection.

## 8.2 Generic Adapter Features

- [ ] Support raw terminal passthrough.
- [ ] Support text input.
- [ ] Support special keys.
- [ ] Support interrupt.
- [ ] Support terminate.
- [ ] Support resize.
- [ ] Emit generic session events.
- [ ] Add tests for generic fallback behavior.

## 8.3 Basic Heuristics

- [ ] Add optional visible-text or regex detection for approval-like prompts.
- [ ] Mark heuristic events with confidence.
- [ ] Avoid treating heuristic events as guaranteed.
- [ ] Always provide raw terminal fallback.
- [ ] Add tests for heuristic detection.

---

# Phase 9 — Terminal Snapshot Support

Goal: support reconnects and non-streaming views.

## 9.1 Snapshot Model

- [ ] Define terminal snapshot response.
- [ ] Include dimensions.
- [ ] Include visible content.
- [ ] Include cursor position if available.
- [ ] Include sequence number.
- [ ] Include timestamp.

## 9.2 Snapshot Generation

- [ ] Build basic screen state from output or use frontend-side replay strategy.
- [ ] Decide minimum viable Stage 1 snapshot strategy.
- [ ] Implement snapshot endpoint.
- [ ] Add tests for snapshot behavior.

## 9.3 Reconnect Behavior

- [ ] When web client reconnects, load session metadata.
- [ ] Load recent terminal history or snapshot.
- [ ] Resume live event stream.
- [ ] Verify reconnect manually.

---

# Phase 10 — Web Dashboard

Goal: create a usable browser dashboard for Stage 1 validation.

## 10.1 Layout

- [ ] Create session list page/panel.
- [ ] Create active session view.
- [ ] Create terminal area.
- [ ] Create session status indicator.
- [ ] Create action/controls area.
- [ ] Create event/history panel if useful.

## 10.2 Session Creation UI

- [ ] Add create-session form.
- [ ] Support command input.
- [ ] Support working directory input.
- [ ] Support session name input.
- [ ] Show validation errors.

## 10.3 Terminal UI

- [ ] Render live terminal output.
- [ ] Send keyboard input.
- [ ] Send pasted text safely.
- [ ] Handle terminal resize.
- [ ] Support reconnect.
- [ ] Support focus/keyboard capture correctly.

## 10.4 Controls

- [ ] Add Interrupt button.
- [ ] Add Terminate button.
- [ ] Add Force Kill option behind confirmation.
- [ ] Add raw input fallback if terminal focus fails.
- [ ] Display current working directory and command.

## 10.5 Semantic Events UI

- [ ] Render backend-provided semantic events.
- [ ] Render backend-provided action buttons.
- [ ] Submit action by event ID and action ID.
- [ ] Show action success/failure.
- [ ] Fall back to raw terminal when semantic event is uncertain.

---

# Phase 11 — Security Hardening

Goal: prevent accidental dangerous exposure and unsafe control.

## 11.1 Authentication

- [ ] Implement authentication for dashboard and API.
- [ ] Do not allow unauthenticated session control.
- [ ] Protect WebSocket authentication.
- [ ] Add tests for unauthorized access.

## 11.2 Network Defaults

- [ ] Bind to `127.0.0.1` by default.
- [ ] Require explicit config for non-local bind.
- [ ] Log warning when binding outside localhost.
- [ ] Document safe remote access options.

## 11.3 CSRF and Origin Controls

- [ ] Add CSRF protection where needed.
- [ ] Validate browser origins.
- [ ] Reject unexpected origins by default.
- [ ] Add tests for origin rejection where practical.

## 11.4 Action Safety

- [ ] Require confirmation for terminate.
- [ ] Require stronger confirmation for force kill.
- [ ] Reject stale semantic actions.
- [ ] Show command/session context before approval actions.
- [ ] Never auto-approve by default.

## 11.5 Sensitive Data Protection

- [ ] Review logs for secret leakage.
- [ ] Review database contents for sensitive data.
- [ ] Add redaction helper where needed.
- [ ] Document known sensitive areas.

---

# Phase 12 — First Real Harness Adapter

Goal: prove the adapter architecture with one real harness after the generic runtime works.

## 12.1 Adapter Target Selection

- [ ] Choose first real harness adapter.
- [ ] Record why it was selected.
- [ ] Record its common approval prompt patterns.
- [ ] Record its command palette behavior.
- [ ] Record its interrupt behavior.

## 12.2 Detection

- [ ] Detect the harness command.
- [ ] Detect visible approval prompt.
- [ ] Extract useful context if possible.
- [ ] Emit `approval.required` event.
- [ ] Include confidence level.
- [ ] Preserve raw terminal fallback.

## 12.3 Actions

- [ ] Implement approve action.
- [ ] Implement deny action.
- [ ] Implement prompt submission if reliable.
- [ ] Implement command palette opening if reliable.
- [ ] Add tests with fake output matching the harness pattern.

## 12.4 Manual Validation

- [ ] Run the real harness through the interceptor.
- [ ] Verify terminal display.
- [ ] Verify typing.
- [ ] Verify interrupt.
- [ ] Verify approval detection.
- [ ] Verify approve/deny behavior.
- [ ] Record limitations.

---

# Phase 13 — CLI Client

Goal: provide a terminal client for local control and debugging.

## 13.1 Basic CLI

- [ ] Implement `harnessctl status`.
- [ ] Implement `harnessctl sessions`.
- [ ] Implement `harnessctl run`.
- [ ] Implement `harnessctl interrupt`.
- [ ] Implement `harnessctl terminate`.

## 13.2 Attach Mode

- [ ] Implement `harnessctl attach <session-id>`.
- [ ] Put local terminal into raw mode.
- [ ] Forward keyboard input to daemon.
- [ ] Render remote PTY output.
- [ ] Handle local terminal resize.
- [ ] Support detach key sequence.
- [ ] Restore local terminal state on exit.

---

# Phase 14 — Documentation

Goal: make the project easy for smaller AI agents and humans to continue.

## 14.1 README

- [ ] Document project purpose.
- [ ] Document Stage 1 scope.
- [ ] Document out-of-scope items.
- [ ] Document architecture.
- [ ] Document quick start.
- [ ] Document security warnings.

## 14.2 Developer Guide

- [ ] Document project structure.
- [ ] Document how to run tests.
- [ ] Document how to run fake harnesses.
- [ ] Document how to add an adapter.
- [ ] Document API conventions.
- [ ] Document event schema.

## 14.3 API Documentation

- [ ] Document REST endpoints.
- [ ] Document WebSocket events.
- [ ] Document action model.
- [ ] Document error responses.
- [ ] Document authentication behavior.

## 14.4 Context/Todo Maintenance

- [ ] Verify `Context.md` is still accurate.
- [ ] Propose any needed context updates to the project owner.
- [ ] Do not apply context updates without approval.
- [ ] Keep `Todo.md` aligned with completed work.

---

# Phase 15 — Full Stage 1 Validation

Goal: verify that the interceptor + web dashboard is useful and stable enough to continue.

## 15.1 Generic Command Validation

- [ ] Run `/bin/bash` through interceptor.
- [ ] Run simple commands.
- [ ] Run long-running command.
- [ ] Run interactive command.
- [ ] Verify output, input, resize, interrupt, and terminate.

## 15.2 Fake Harness Validation

- [ ] Run fake plain-output harness.
- [ ] Run fake approval harness.
- [ ] Run fake full-screen TUI harness.
- [ ] Run fake long-running harness.
- [ ] Run fake stubborn process.
- [ ] Verify expected behavior.

## 15.3 Real Harness Validation

- [ ] Run first real coding harness.
- [ ] Verify raw terminal usability.
- [ ] Verify session list.
- [ ] Verify reconnect.
- [ ] Verify interrupt.
- [ ] Verify terminate.
- [ ] Verify any adapter-specific behavior.

## 15.4 Security Validation

- [ ] Verify daemon binds to localhost by default.
- [ ] Verify unauthenticated API calls fail.
- [ ] Verify WebSocket auth is required.
- [ ] Verify stale actions fail.
- [ ] Verify logs do not expose obvious secrets.
- [ ] Verify non-local bind requires explicit config.

## 15.5 Stage 1 Completion Criteria

- [ ] Go daemon can launch PTY sessions.
- [ ] Web dashboard can view and control sessions.
- [ ] Multiple sessions work.
- [ ] Generic terminal fallback works.
- [ ] Common API is usable by the web dashboard.
- [ ] WebSocket live streaming works.
- [ ] Sessions can be interrupted and terminated.
- [ ] Basic storage/audit history exists.
- [ ] Generic adapter exists.
- [ ] At least one real harness adapter is proven or clearly planned.
- [ ] Tests cover core runtime and API.
- [ ] Documentation is sufficient for handoff to smaller agents.

---

# New Task Template

Use this template when adding new tasks.

```md
## Task: <Short task name>

- [ ] <Concrete action>
  - Phase:
  - Owner/agent:
  - Reason:
  - Files likely affected:
  - Acceptance criteria:
    - <How to verify completion>
  - Tests required:
    - <Unit/integration/manual test>
  - Notes:
    - <Constraints, risks, or references>
```

Rules for new tasks:

- Put the task under the most relevant phase.
- Use checkboxes.
- Keep the action concrete.
- Include acceptance criteria.
- Include tests when behavior changes.
- Do not add vague tasks.
- Do not add mobile-app scope.
- Do not edit `Context.md` without explicit permission.
- Do not remove or rewrite existing tasks just because they are inconvenient.
- If a task changes architecture or scope, mark it as requiring owner approval.

---

# Full Project Context Summary

HarnessRelay Interceptor is a Go-based Linux interceptor daemon plus web dashboard for controlling terminal-based coding harnesses.

The interceptor should launch harness processes inside PTYs, own their lifecycle, stream terminal output to a browser, accept browser input, expose a stable common API, and support generic terminal control for every harness. Harness-specific semantic adapters can be added later for cleaner actions such as approval, denial, prompt submission, command-list opening, and model changes.

The web dashboard is the only client in Stage 1. Mobile application work is out of scope and must not be added.

The architecture should prioritize:

- PTY-owned sessions
- Go daemon
- Web dashboard
- REST API
- WebSocket event stream
- Generic adapter fallback
- Optional harness-specific adapters
- Safe localhost-first security defaults
- SQLite-backed metadata/history where useful
- Tests using fake harnesses
- Clear documentation for smaller AI agents

The most important implementation guardrails are:

- Do not intercept already-running terminals as the main architecture.
- Do not assume TUIs expose semantic buttons.
- Do not hardcode the common API around one harness.
- Do not remove raw terminal fallback.
- Do not bind publicly by default.
- Do not run as root.
- Do not auto-approve actions.
- Do not edit `Context.md` without explicit approval.
- Do not add mobile-app scope.

Stage 1 is complete when a user can start the daemon, create one or more managed harness sessions, view and interact with them from a web dashboard, interrupt or terminate them, reconnect to them, and rely on a clean API/event model that can later support richer harness adapters.
