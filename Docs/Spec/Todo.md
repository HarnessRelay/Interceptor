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

- [x] Continuously read PTY output.
- [x] Publish output to internal event bus.
- [x] Preserve raw bytes.
- [x] Handle EOF and process exit.
- [x] Avoid goroutine leaks.
- [x] Add tests for output capture.

## 3.3 PTY Input

- [x] Implement writing raw input to PTY.
- [x] Implement text input.
- [x] Implement special key support.
- [x] Implement Enter, Escape, Tab, arrows, and Ctrl+C.
- [x] Add tests for input behavior.

## 3.4 Resize Support

- [x] Implement PTY resize operation.
- [x] Add API-safe resize model.
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
- [x] Expose create-session API.

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
- [x] Emit status change events.

## 4.4 Session Cleanup

- [x] Define cleanup rules for exited sessions.
- [x] Define whether exited sessions remain visible.
- [x] Add manual cleanup API if needed.
- [x] Ensure cleanup does not delete useful audit history unexpectedly.

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

- [x] Implement `/api/v1/ws`.
- [x] Authenticate WebSocket connections.
- [x] Stream terminal output events.
- [x] Stream session lifecycle events.
- [x] Handle client disconnects cleanly.
- [x] Add tests where practical.

---

# Phase 6 — REST API

Goal: expose stable backend operations for the web dashboard and future clients.

## 6.1 Session APIs

- [x] Implement `GET /api/v1/sessions`.
- [x] Implement `POST /api/v1/sessions`.
- [x] Implement `GET /api/v1/sessions/{id}`.
- [x] Implement `DELETE /api/v1/sessions/{id}`.

## 6.2 Terminal Control APIs

- [x] Implement `POST /api/v1/sessions/{id}/input`.
- [x] Implement `POST /api/v1/sessions/{id}/resize`.
- [x] Implement `POST /api/v1/sessions/{id}/interrupt`.
- [x] Implement `POST /api/v1/sessions/{id}/terminate`.

## 6.3 Snapshot and Events APIs

- [x] Implement `GET /api/v1/sessions/{id}/snapshot`.
- [x] Implement `GET /api/v1/sessions/{id}/events`.
- [x] Add pagination or limits for event history.
- [x] Add tests for event retrieval.

## 6.4 Semantic Action API

- [x] Implement `POST /api/v1/sessions/{id}/actions/{action_id}`.
- [x] Require `event_id` for event-bound actions.
- [x] Reject stale or unknown actions.
- [x] Return clear action result.
- [x] Add tests for stale action rejection.

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

- [x] Store bounded terminal output history.
- [x] Define truncation behavior.
- [x] Avoid unbounded database growth.
- [x] Add tests for history limits.

## 7.4 Audit Logging

- [x] Audit session creation.
- [x] Audit input submission where appropriate.
- [x] Audit interrupts.
- [x] Audit termination.
- [x] Audit semantic actions.
- [x] Do not log secrets or full sensitive prompts by default unless explicitly configured.

---

# Phase 8 — Generic Harness Adapter

Goal: make every terminal-based harness usable even without a dedicated adapter.

## 8.1 Adapter Registry

- [x] Define adapter interface.
- [x] Define adapter match result.
- [x] Implement adapter registry.
- [x] Ensure generic adapter is always available.
- [x] Ensure generic adapter has lowest priority.
- [x] Add tests for adapter selection.

## 8.2 Generic Adapter Features

- [x] Support raw terminal passthrough.
- [x] Support text input.
- [x] Support special keys.
- [x] Support interrupt.
- [x] Support terminate.
- [x] Support resize.
- [x] Emit generic session events.
- [x] Add tests for generic fallback behavior.

## 8.3 Basic Heuristics

- [x] Add optional visible-text or regex detection for approval-like prompts.
- [x] Mark heuristic events with confidence.
- [x] Avoid treating heuristic events as guaranteed.
- [x] Always provide raw terminal fallback.
- [x] Add tests for heuristic detection.

---

# Phase 9 — Terminal Snapshot Support

Goal: support reconnects and non-streaming views.

## 9.1 Snapshot Model

- [x] Define terminal snapshot response.
- [x] Include dimensions.
- [x] Include visible content.
- [ ] Include cursor position if available.
- [x] Include sequence number.
- [x] Include timestamp.

## 9.2 Snapshot Generation

- [ ] Build basic screen state from output or use frontend-side replay strategy.
- [x] Decide minimum viable Stage 1 snapshot strategy.
- [x] Implement snapshot endpoint.
- [x] Add tests for snapshot behavior.

## 9.3 Reconnect Behavior

- [x] When web client reconnects, load session metadata.
- [x] Load recent terminal history or snapshot.
- [x] Resume live event stream.
- [x] Verify reconnect manually.

---

# Phase 10 — Web Dashboard

Goal: create a usable browser dashboard for Stage 1 validation.

## 10.1 Layout

- [x] Create session list page/panel.
- [x] Create active session view.
- [x] Create terminal area.
- [x] Create session status indicator.
- [x] Create action/controls area.
- [x] Create event/history panel if useful.

## 10.2 Session Creation UI

- [x] Add create-session form.
- [x] Support command input.
- [x] Support working directory input.
- [x] Support session name input.
- [x] Add Chat/Terminal mode selector before session start.
- [x] Show validation errors.

## 10.3 Terminal UI

- [x] Render live terminal output.
- [x] Send keyboard input.
- [x] Send pasted text safely.
- [x] Handle terminal resize.
- [x] Support reconnect.
- [x] Support focus/keyboard capture correctly.

## 10.4 Controls

- [x] Add Interrupt button.
- [x] Add Terminate button.
- [x] Add Force Kill option behind confirmation.
- [x] Add raw input fallback if terminal focus fails.
- [x] Display current working directory and command.

## 10.5 Semantic Events UI

- [x] Render backend-provided semantic events.
- [x] Render backend-provided action buttons.
- [x] Submit action by event ID and action ID.
- [x] Show action success/failure.
- [x] Fall back to raw terminal when semantic event is uncertain.

## 10.6 Chat-First Web Direction

- [x] Refactor dashboard frontend out of one large `main.tsx`.
- [x] Add Chat Mode as the default dashboard view.
- [x] Add Terminal Mode as a selectable raw xterm.js fallback view.
- [x] Allow switching Chat/Terminal views without restarting the session.
- [x] Send Chat Mode composer submissions into the PTY.
- [x] Render terminal output in readable transcript-style blocks.
- [x] Add `/` slash-command action menu.
- [x] Keep force kill behind confirmation.
- [x] Make event/debug output collapsible instead of dominant.
- [x] Apply navy/teal HarnessRelay theme direction.
- [x] Document logo asset placement.
- [x] Extend dashboard smoke coverage for Chat Mode, Terminal Mode, switching, reconnect, interrupt, and terminate.

## 10.7 Browser QA Pass

- [x] Verify browser automation capability and document it in `Docs/QA/WebApp-QA.md`.
- [x] Run screen-by-screen dashboard QA for login, app shell/sidebar, create session, Chat Mode, slash menu, Terminal Mode, reconnect/reload, and multiple sessions.
- [x] Add QA IDs to dashboard smoke coverage.
- [x] Run approved real-harness smoke with OpenCode in a disposable `/tmp` fixture.
- [x] Re-run `go test ./...`, `make test`, `make build`, and dashboard smoke commands after QA updates.

---

# Phase 11 — Security Hardening

Goal: prevent accidental dangerous exposure and unsafe control.

## 11.1 Authentication

- [x] Implement authentication for dashboard and API.
- [x] Do not allow unauthenticated session control.
- [x] Protect WebSocket authentication.
- [x] Add tests for unauthorized access.

## 11.2 Network Defaults

- [x] Bind to `127.0.0.1` by default.
- [x] Require explicit config for non-local bind.
- [x] Log warning when binding outside localhost.
- [x] Document safe remote access options.

## 11.3 CSRF and Origin Controls

- [x] Add CSRF protection where needed.
- [x] Validate browser origins.
- [x] Reject unexpected origins by default.
- [x] Add tests for origin rejection where practical.

## 11.4 Action Safety

- [x] Require confirmation for terminate.
- [x] Require stronger confirmation for force kill.
- [x] Reject stale semantic actions.
- [x] Show command/session context before approval actions.
- [x] Never auto-approve by default.

## 11.5 Sensitive Data Protection

- [x] Review logs for secret leakage.
- [x] Review database contents for sensitive data.
- [x] Add redaction helper where needed.
- [x] Document known sensitive areas.

---

# Phase 12 — First Real Harness Adapter

Goal: prove the adapter architecture with one real harness after the generic runtime works.

## 12.1 Adapter Target Selection

- [x] Choose first real harness adapter.
- [x] Record why it was selected.
- [x] Record its common approval prompt patterns.
- [x] Record its command palette behavior.
- [x] Record its interrupt behavior.

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

- [x] Implement `harnessctl status`.
- [x] Implement `harnessctl sessions`.
- [x] Implement `harnessctl run`.
- [x] Implement `harnessctl interrupt`.
- [x] Implement `harnessctl terminate`.

## 13.2 Attach Mode

- [x] Implement `harnessctl attach <session-id>`.
- [x] Put local terminal into raw mode.
- [x] Forward keyboard input to daemon.
- [x] Render remote PTY output.
- [x] Handle local terminal resize.
- [x] Support detach key sequence.
- [x] Restore local terminal state on exit.

---

# Phase 14 — Documentation

Goal: make the project easy for smaller AI agents and humans to continue.

## 14.1 README

- [x] Document project purpose.
- [x] Document Stage 1 scope.
- [x] Document out-of-scope items.
- [x] Document architecture.
- [x] Document quick start.
- [x] Document security warnings.

## 14.2 Developer Guide

- [x] Document project structure.
- [x] Document how to run tests.
- [x] Document how to run fake harnesses.
- [x] Document how to add an adapter.
- [x] Document API conventions.
- [x] Document event schema.

## 14.3 API Documentation

- [x] Document REST endpoints.
- [x] Document WebSocket events.
- [x] Document action model.
- [x] Document error responses.
- [x] Document authentication behavior.

## 14.4 Context/Todo Maintenance

- [x] Verify `Context.md` is still accurate.
- [x] Propose any needed context updates to the project owner.
- [x] Do not apply context updates without approval.
- [x] Keep `Todo.md` aligned with completed work.

---

# Phase 15 — Full Stage 1 Validation

Goal: verify that the interceptor + web dashboard is useful and stable enough to continue.

## 15.1 Generic Command Validation

- [x] Run `/bin/bash` through interceptor.
- [x] Run simple commands.
- [x] Run long-running command.
- [x] Run interactive command.
- [x] Verify output, input, resize, interrupt, and terminate.

## 15.2 Fake Harness Validation

- [x] Run fake plain-output harness.
- [x] Run fake approval harness.
- [x] Run fake full-screen TUI harness.
- [x] Run fake long-running harness.
- [x] Run fake stubborn process.
- [x] Verify expected behavior.

## 15.3 Real Harness Validation

- [ ] Run first real coding harness.
- [ ] Verify raw terminal usability.
- [ ] Verify session list.
- [ ] Verify reconnect.
- [ ] Verify interrupt.
- [ ] Verify terminate.
- [ ] Verify any adapter-specific behavior.

## 15.4 Security Validation

- [x] Verify daemon binds to localhost by default.
- [x] Verify unauthenticated API calls fail.
- [x] Verify WebSocket auth is required.
- [x] Verify stale actions fail.
- [x] Verify logs do not expose obvious secrets.
- [x] Verify non-local bind requires explicit config.

## 15.5 Stage 1 Completion Criteria

- [x] Go daemon can launch PTY sessions.
- [x] Web dashboard can view and control sessions.
- [x] Multiple sessions work.
- [x] Generic terminal fallback works.
- [x] Common API is usable by the web dashboard.
- [x] WebSocket live streaming works.
- [x] Sessions can be interrupted and terminated.
- [x] Basic storage/audit history exists.
- [x] Generic adapter exists.
- [x] At least one real harness adapter is proven or clearly planned.
- [x] Tests cover core runtime and API.
- [x] Documentation is sufficient for handoff to smaller agents.

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
