# HarnessRelay UI Revamp QA

Date: 2026-07-26

## Scope

This record covers the production UI redesign described in
`Docs/Design/UI-Revamp-Plan.md`: login, session management, creation, Chat Mode,
Terminal Mode, action menus, inspector, accessibility, semantic adapters,
reconnect, and multiple sessions.

The screenshot source of truth is:

```text
qa/artifacts/screenshots/ui-revamp/
```

## Design review

Impeccable product-interface guidance was applied during implementation and
manual screenshot review. The review emphasized restrained state color,
familiar product controls, readable conversation width, non-dominant debug
data, viewport-safe overlays, explicit focus behavior, and avoiding generic
decorative card/glass patterns.

Overall result: pass. The app reads as a calm local developer workbench with a
Termius-like session rail, ChatGPT-like conversation canvas, and JetBrains-like
progressive information architecture.

## UIR-001: Detected harness version leaked ANSI warning text

Status: verified
Area: New Session dialog
Severity: medium
Steps to reproduce: Open New Session while a discovered harness prints a
colored warning during version detection.
Expected: Preset metadata is short, readable command/version text.
Actual: ANSI control text appeared in the OpenCode preset.
Root cause: Discovery output was displayed without the frontend terminal-text
sanitizer.
Fix summary: Sanitize discovered version strings before rendering.
Regression test: New Session screenshot and no-control-garbage visual review.
Screenshot: `03-create-session.png`
Verification commands: `npm --prefix web run build`; Playwright Screen 3.
Notes: The backend discovery contract remains unchanged.

## UIR-002: Creation dialog exceeded short laptop viewport

Status: verified
Area: New Session dialog
Severity: medium
Steps to reproduce: Open the dialog at a 720px-tall viewport with four detected
harness presets.
Expected: Every field and action remains reachable without page clipping.
Actual: The lower action row began below the initial viewport.
Root cause: The modal panel had no viewport maximum height.
Fix summary: Bound the panel to `100vh - 40px` and allow panel scrolling.
Regression test: dialog keyboard focus containment plus Screen 3 screenshot.
Screenshot: `03-create-session.png`
Verification commands: `npm --prefix web run qa:a11y`; Playwright Screen 3.
Notes: At the required 1440×960 review viewport the complete form is visible.

## UIR-003: Split Codex approval emitted before command context

Status: verified
Area: Semantic approval card
Severity: high
Steps to reproduce: Emit the approval heading and `$ command` in separate PTY
chunks.
Expected: The event-bound card includes the command users must review.
Actual: A card could be emitted from the heading chunk with an empty command
and then suppress the richer update.
Root cause: The parser opened approval state before `parseCommand` succeeded.
Fix summary: Wait for command context before publishing command approvals.
Regression test: `TestParserWaitsForApprovalCommandContext` and the fake Codex
Playwright flow.
Screenshot: `06-chat-mode-codex-or-fake-codex.png`
Verification commands: `go test ./internal/harness/codex`; Playwright semantic
adapter test.
Notes: Workspace trust remains a separate Terminal Mode decision and is not
affected.

## UIR-004: Completed terminal session showed a Live badge

Status: verified
Area: Terminal Mode chrome
Severity: medium
Steps to reproduce: Start `/bin/echo` in Terminal Mode and wait for exit.
Expected: The retained terminal is labelled Snapshot.
Actual: The still-connected event WebSocket caused the chrome to say Live.
Root cause: Stream label considered WebSocket connection but not process
lifecycle.
Fix summary: Live now requires both an open stream and a live session; completed
sessions say Snapshot.
Regression test: Screen 3 completed-session flow.
Screenshot: `04-session-cards.png`
Verification commands: Playwright Screen 3; frontend build.
Notes: Raw retained output remains fully visible.

## UIR-005: Browser-native destructive prompts lacked workbench focus behavior

Status: verified
Area: More and slash actions
Severity: high
Steps to reproduce: Choose Terminate or Force kill.
Expected: Clearly labelled, keyboard-cancellable, focus-contained confirmation.
Actual: `window.confirm`/`window.prompt` interrupted the product interaction
model and could not be styled or audited.
Root cause: Initial functional UI used browser-native prompts.
Fix summary: Added shared confirmation dialogs; force kill still requires
`KILL`, and cancel receives initial focus.
Regression test: Slash menu test, Terminal Mode test, and Accessibility QA.
Screenshot: `07-slash-menu.png`
Verification commands: `npm --prefix web run qa`; `npm --prefix web run
qa:a11y`.
Notes: No destructive action became easier to trigger.

## UIR-006: Terminal ResizeObserver repeated unchanged resize requests

Status: verified
Area: Terminal Mode
Severity: medium
Steps to reproduce: Keep Terminal Mode open during the legacy CDP smoke and
inspect daemon request logs.
Expected: Resize is sent only when xterm rows or columns change.
Actual: Browser layout observation could repeatedly send the same dimensions.
Root cause: The debounced callback fit xterm but did not compare the result with
the last submitted dimensions.
Fix summary: Cache both the terminal host dimensions and the last submitted
rows/columns per mounted terminal and suppress unchanged resize requests.
Regression test: Terminal Mode viewport resize still proves a real dimension
change reaches the backend.
Screenshot: `08-terminal-mode.png`
Verification commands: Playwright Screen 7; legacy dashboard smoke.
Notes: This reduces API, audit, and log noise without changing terminal fit.

## Screenshot Review: 01-login.png

Status: pass
What looks good: strong product identity, direct value proposition, clear
daemon connection task, trustworthy local-only note, and ample focus.
Issues found: none after review.
Fixes made: disabled primary state and persistent token explanation.
Remaining concerns: none.

## Screenshot Review: 02-empty-state.png

Status: pass
What looks good: rail and workspace both explain first-run action; suggested
Codex, shell, and custom paths prevent an empty void.
Issues found: none.
Fixes made: added explicit first-session actions and examples.
Remaining concerns: none.

## Screenshot Review: 03-create-session.png

Status: pass
What looks good: detected presets are compact, manual fields are clearly
labelled, Chat is the default, and advanced options stay quiet.
Issues found: UIR-001 and UIR-002.
Fixes made: sanitized version text and viewport-bounded the dialog.
Remaining concerns: large numbers of future presets may require preset search.

## Screenshot Review: 04-session-cards.png

Status: pass
What looks good: rectangular selected card, command, adapter, mode, status, and
activity are immediately scannable.
Issues found: UIR-004.
Fixes made: completed Terminal Mode now says Snapshot.
Remaining concerns: relative activity is browser-clock based and intentionally
coarse.

## Screenshot Review: 05-chat-mode-generic.png

Status: pass
What looks good: readable center column, distinct user turns, unboxed assistant
output, stable bottom composer, and visible terminal fallback.
Issues found: none.
Fixes made: long text wraps without page overflow.
Remaining concerns: generic projection remains conservative by design.

## Screenshot Review: 06-chat-mode-codex-or-fake-codex.png

Status: pass
What looks good: Codex adapter/model metadata is present but quiet; prompt and
assistant turns remain readable; no raw TUI artifacts appear.
Issues found: UIR-003.
Fixes made: preserved command context across split approval chunks.
Remaining concerns: real Codex extraction is version-sensitive as documented.

## Screenshot Review: 07-slash-menu.png

Status: pass
What looks good: actions are grouped by task, destructive controls are last,
Escape is discoverable, and the menu does not clip.
Issues found: UIR-005.
Fixes made: destructive items route to accessible confirmation dialogs.
Remaining concerns: none.

## Screenshot Review: 08-terminal-mode.png

Status: pass
What looks good: terminal gets visual priority, live/dimension chrome is compact,
Open Chat is obvious, and raw input is progressive.
Issues found: UIR-004.
Fixes made: lifecycle-aware stream label.
Remaining concerns: xterm remains the source of truth and therefore retains its
own accessibility limitations.

## Screenshot Review: 09-inspector.png

Status: pass
What looks good: inspector is absent by default, opens as a real workbench
drawer, and provides overview/events/capabilities without covering all context.
Issues found: none.
Fixes made: event payloads are collapsed individually.
Remaining concerns: very large event histories remain bounded by frontend and
backend limits.

## Screenshot Review: 10-multiple-sessions.png

Status: pass
What looks good: lifecycle grouping and selected state remain clear across many
sessions; outputs remain isolated.
Issues found: none.
Fixes made: newest cards remain first within lifecycle groups.
Remaining concerns: grouping is intentionally simple and not user-configurable.

## Screenshot Review: 11-reconnect.png

Status: pass
What looks good: restored session identity, snapshot content, mode, and sidebar
context are coherent after reload.
Issues found: none.
Fixes made: transcript and selected mode are restored per session.
Remaining concerns: session persistence is browser reload/daemon-memory scope,
not daemon restart persistence.

## Regression coverage

- Login invalid/valid/reload and console errors.
- Empty state and hidden inspector.
- Creation validation, placeholders, modes, and completed session.
- Rectangular card metadata and selection.
- Chat click/Enter/Shift+Enter and terminal-noise rejection.
- Terminal xterm input, inserted paste, raw fallback, resize, interrupt, and
  lifecycle.
- Mode switching without session replacement.
- Slash/More menus and destructive confirmation.
- Inspector hidden/open/events/close.
- Reconnect and snapshot restoration.
- Multi-session output/input isolation.
- Fake and conditional real Codex semantic flows.
- Dialog/menu/tab keyboard behavior, accessible naming, and token contrast.

## Final validation

Commands and results:

- `go test ./...`: passed, 126 tests across 14 packages.
- `make test`: passed.
- `npm --prefix web run build`: passed.
- `make build`: passed; the sandbox printed non-fatal Go module stat-cache
  write warnings, then built both binaries successfully.
- `npm --prefix web run qa`: passed, 14 Playwright tests including the
  conditional real Codex smoke.
- `npm --prefix web run qa:a11y`: passed.
- `node qa/dashboard-smoke.mjs` with the documented local daemon/CDP
  environment: passed.
- Final targeted Terminal Mode regression after the resize-loop guard: passed.

Manual checks confirmed the eleven screenshots exist, the inspector is closed
by default, menus/dialogs are not clipped at the review viewport, Chat Mode has
no TUI garbage, the real Codex response rendered cleanly with model metadata,
Terminal Mode retained raw output, and mode/session switching did not restart
or mix sessions.

## Remaining limitations

- Real Codex inference can be unavailable when the external account reaches its
  usage limit; deterministic fake Codex coverage remains mandatory.
- Real assistive-technology manual testing was unavailable.
- Vite reports one bundle above 500 kB because React, xterm.js, and the
  dashboard ship in one Stage 1 chunk. This is a performance optimization
  opportunity, not a functional blocker for the local app.
