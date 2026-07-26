# Semantic Adapter QA

Date: 2026-07-26

## SA-001: Chat Mode is terminal projection, not semantic

Status: verified
Area: Codex Chat Mode
Severity: critical
Steps to reproduce:
Create `testdata/fake-harnesses/codex` in Chat Mode and inspect startup, prompts,
status, mode switching, and reload.
Expected:
Codex Chat uses backend semantic events and retains raw output only in Terminal
Mode.
Actual:
The session shows a Codex adapter badge, capabilities, status, metadata, system
guidance, and event-backed user messages. Reload hydrates semantic event
history. Terminal Mode renders the same live PTY.
Root cause:
The original Chat view projected all `terminal.output` events.
Fix summary:
Added backend adapter selection and semantic events. Chat selects semantic or
generic rendering from session capabilities.
Regression test:
`Semantic adapter: fake Codex remains coherent across chat, terminal, approval,
and reload` in `qa/playwright/harnessrelay.spec.ts`.
Verification commands:
`rtk go test ./internal/harness/... ./internal/session ./internal/api`;
`rtk npm --prefix web run qa -- --grep "Semantic adapter: fake Codex"`
Notes:
Assistant extraction is covered separately by SA-007.

## SA-002: Codex prompt submit requires Terminal Mode Enter

Status: verified
Area: Prompt submission
Severity: critical
Steps to reproduce:
Start the fake or real Codex TUI, enter a prompt in Chat, and use both Send and
composer Enter.
Expected:
The prompt is submitted without opening Terminal Mode.
Actual:
Fake Codex reports `RECEIVED:<prompt>` for both paths. Direct installed Codex
research confirmed `CSI 13 u` submits after enhanced keyboard mode is enabled.
Root cause:
Codex enables Kitty keyboard protocol; plain carriage return or line feed may
only enter text.
Fix summary:
Added `POST /sessions/{id}/prompt` and adapter-specific atomic prompt bytes.
Codex sends `CSI 13 u` when `CSI > 7 u` is present; Generic uses carriage return.
Regression test:
Codex adapter prompt-byte unit test, manager PTY integration test, API test, and
the semantic Playwright scenario.
Verification commands:
`rtk go test ./internal/harness/... ./internal/session ./internal/api`
Notes:
The observed real version is `codex-cli 0.145.0`.

## SA-003: Codex/TUI redraw noise leaks into Chat Mode

Status: verified
Area: Output classification
Severity: critical
Steps to reproduce:
Run the fake Codex fixture, which emits ANSI controls, a box frame, and
`MMMMMMMM`; observe Chat, Terminal, mode switching, and reload.
Expected:
No raw redraw artifact appears in Chat. Raw output remains in Terminal.
Actual:
Chat shows only the clean semantic system message and events. The raw xterm and
snapshot retain `MMMMMMMM`.
Root cause:
All adapters previously used frontend terminal projection.
Fix summary:
The Codex parser emits `terminal.noisy_output` and a safe system event. Chat
ignores `terminal.output` when `semantic_chat` is present.
Regression test:
Codex parser unit test, manager raw-preservation test, and semantic Playwright
scenario.
Verification commands:
`rtk go test ./internal/harness/codex ./internal/session`;
`rtk npm --prefix web run qa -- --grep "Semantic adapter: fake Codex"`
Notes:
Raw chunks remain excluded even though the rendered screen now supplies
assistant semantics.

## SA-004: Adapter capabilities are not visible in UI/API

Status: verified
Area: Session API and header
Severity: high
Steps to reproduce:
Create fake Codex and `/bin/bash` sessions, then inspect session responses and
the active header.
Expected:
Clients can distinguish Codex from Generic and discover implemented behavior.
Actual:
Session responses expose `adapter_id`, `adapter_name`, and
`adapter_capabilities`. The header shows `Codex` and `Semantic chat`; Generic
shows `Generic`.
Root cause:
The manager and DTO previously hardcoded or exposed only `generic`.
Fix summary:
Session creation selects from the registry and persists adapter metadata and
capabilities through the API and frontend model.
Regression test:
Manager integration, API integration, and semantic Playwright assertions.
Verification commands:
`rtk go test ./internal/session ./internal/api`
Notes:
Capabilities describe behavior, not authorization.

## SA-005: Approval prompts are not represented as semantic events

Status: verified
Area: Approval detection and actions
Severity: critical
Steps to reproduce:
Send `request approval` to the fake Codex session.
Expected:
Chat shows command context with explicit safe choices. Unknown, stale, replayed,
or automatic actions fail closed.
Actual:
The parser emits `approval.required` with command, cwd, confidence, `Deny`, and
`Open Terminal`. Deny is event-bound, sends the verified Escape sequence, emits
`approval.resolved`, and cannot be replayed. Prompts are blocked while a
decision is pending.
Root cause:
Generic heuristics could identify text but had no reliable harness-specific
state or key mapping.
Fix summary:
Added exact Codex approval classification, pending action state, safe denial,
raw-terminal resolution, and conflict handling. Generic now exposes only Open
Terminal.
Regression test:
Parser duplicate/reset unit test, manager action test, API action/replay test,
and semantic Playwright approval flow.
Verification commands:
`rtk go test ./internal/harness/codex ./internal/session ./internal/api`;
`rtk npm --prefix web run qa -- --grep "Semantic adapter: fake Codex"`
Notes:
Approve-once and persistent approval are intentionally deferred. HarnessRelay
never auto-approves.

## SA-006: Chat prompt can enter the Codex workspace trust overlay

Status: verified
Area: Codex startup readiness
Severity: critical
Steps to reproduce:
Start real Codex in a repository that has no stored trust decision and send a
Chat prompt as soon as the terminal interface is detected.
Expected:
Chat identifies the trust screen, blocks prompt submission, and directs the
user to Terminal Mode without choosing a trust level.
Actual:
The trust screen now produces `waiting_for_terminal` and an event-bound
workspace-trust card with only Open Terminal. Chat remains disabled until the
explicit terminal interaction completes and the backend emits `idle`.
Root cause:
WebSocket connectivity and initial terminal UI detection were treated as prompt
readiness. A user prompt could therefore be written into the startup decision
surface.
Fix summary:
Added workspace trust parsing, terminal-only pending state, backend idle gating,
explicit terminal-to-idle transitions, and semantic UI readiness driven only by
backend status.
Regression test:
Codex parser workspace-trust unit test and the real Codex Playwright smoke.
Verification commands:
`rtk go test ./internal/harness/codex ./internal/session`;
`rtk npm --prefix web run qa -- --grep "Codex smoke in disposable"`
Notes:
The test explicitly handles only the disposable `/tmp` workspace decision and
then proves submission by waiting for a model-generated token not present
verbatim in the prompt.

## SA-007: Codex response appears only in Terminal Mode

Status: verified
Area: Codex assistant responses
Severity: critical
Steps to reproduce:
Create a Codex session in Chat Mode, send `Hi`, wait for the response, and
compare Chat with Terminal Mode.
Expected:
Chat shows the rendered assistant response without exposing cursor controls,
redraw duplicates, startup notices, composer placeholders, or footer text.
Actual:
The adapter emits one `chat.assistant_message` with the rendered response after
the terminal quiet period. Terminal Mode still shows the complete raw TUI.
Root cause:
The initial semantic adapter intentionally omitted assistant extraction because
stripping ANSI bytes cannot reconstruct a cursor-addressed screen.
Fix summary:
Added a session-scoped headless xterm model, prompt/response boundary
extraction, quiet-period flushing, and stable per-turn message IDs. Chat
replaces later revisions of the same response during live delivery and reload.
Regression test:
Codex parser redraw and revision tests, manager/API fake-PTY tests, and both fake
and real Codex Playwright scenarios.
Verification commands:
`rtk go test ./internal/harness/codex ./internal/session ./internal/api`;
`rtk npm --prefix web run qa -- --grep "Semantic adapter|Codex smoke"`
Notes:
The exact raw capture from the reported failing session was replayed through the
Go model and extracted `Hi! What are we working on today?`. The installed Codex
smoke also produced a clean `SEMANTIC_ADAPTER_OK` assistant bubble in
`qa/artifacts/screenshots/chat-codex-real.png`.

## Known Limitations

- Codex parsing is based on English `codex-cli 0.145.0` terminal behavior.
- Response boundaries depend on observed `›` prompt, `•` response, and model
  footer conventions.
- Responses appear after a three-second quiet period, not as token deltas.
- Approve-once, persistent approval, command palette, and model changes are not
  implemented.
- App-server integration remains a future structured-adapter project.
- Event and terminal replay history remain in memory across browser reload, not
  daemon restart.

## Evidence

- Fake harness: `testdata/fake-harnesses/codex`
- Browser screenshot: `qa/artifacts/screenshots/semantic-codex.png`
- Research: `Docs/Spec/Research/08-Semantic-Adapter-Architecture.md` and
  `Docs/Spec/Research/09-Codex-Adapter-Research.md`

## Final Verification

Commands:

```bash
rtk go test ./...
rtk go test -race ./internal/harness/... ./internal/session ./internal/api
rtk make test
rtk make build
rtk npm --prefix web run build
rtk npm --prefix web run qa
rtk env HARNESSRELAY_TOKEN=dashboard-token \
  HARNESSRELAY_DASHBOARD_URL=http://127.0.0.1:8767/ \
  CHROME_CDP_URL=http://127.0.0.1:9222/json/list \
  node qa/dashboard-smoke.mjs
```

Results:

- Go: 124 tests passed across 14 packages.
- Focused race run: passed.
- `make test`: passed.
- `make build`: passed.
- Frontend TypeScript/Vite build: passed.
- Final deterministic Playwright run: 12 tests passed.
- The installed Codex smoke passed earlier in this fix and its clean Chat
  response screenshot was inspected. A later rerun after the metadata
  assertion was added reached the external Codex account usage limit before
  inference, so final real-harness revalidation was unavailable.
- Legacy dashboard CDP smoke: passed.
- Vite reported its existing non-fatal chunk-size warning.
