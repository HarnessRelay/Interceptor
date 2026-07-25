# HarnessRelay Web App QA

Date: 2026-07-26

## Browser Automation Capability

Status: verified with real Playwright

Method: `@playwright/test` in `qa/playwright/harnessrelay.spec.ts`, launched through `npm --prefix web run qa`.

Verified capabilities:

- launch browser: Playwright launches system Google Chrome at `/usr/bin/google-chrome`
- navigate to a URL: yes
- query elements and read visible text: yes
- click elements: yes
- type/fill text: yes
- submit forms: yes
- type/paste into terminal raw input fallback: yes
- type/paste into xterm.js directly: yes
- take screenshots: yes, saved under `qa/artifacts/screenshots/`
- capture console errors/page errors: yes
- inspect tested network errors: yes
- wait for selectors/states: yes
- reload page: yes
- handle confirmations: yes

Detailed report: `Docs/QA/Playwright-Capability-Report.md`.

Verification command:

```bash
HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa
```

Note: in the managed sandbox this command required escalation because `go build` writes to the existing Go cache outside the workspace. The previous CDP smoke remains legacy coverage and is not sufficient by itself for QA sign-off.

## QA-001: Chat Mode renders raw terminal/TUI garbage for Codex output

Status: verified fixed
Screen: Chat Mode
Severity: critical
Steps to reproduce:
Create a Chat Mode session that emits full-screen TUI redraw bytes, or create a disposable Codex session using command `codex`, empty args, and cwd `/tmp/harnessrelay-qa-codex`.
Expected:
Chat Mode must not show mojibake, box-drawing frames, raw ANSI/redraw fragments, or duplicated terminal frames. It should show user-submitted prompts and a clean status card directing the user to Terminal Mode when output cannot be converted to readable chat. Open Terminal must remain visible and work.
Actual:
Playwright verified synthetic full-screen TUI output is collapsed to a system card: `Terminal interface output is available in Terminal Mode. Chat Mode could not convert this PTY redraw into readable chat.` No box drawing, mojibake, or escape fragments appeared in the Chat transcript. The Codex smoke in `/tmp/harnessrelay-qa-codex` showed the submitted user prompt in Chat Mode and raw output in Terminal Mode.
Root cause:
Chat Mode decoded base64 output with `atob` as a binary string and stripped only simple ANSI escapes. Full-screen TUI bytes, alternate-screen/cursor controls, box drawing, and mojibake-like text were treated as readable assistant text.
Fix summary:
Added UTF-8 decoding for chat-visible terminal text, byte-preserving xterm writes for Terminal Mode, and Chat Mode projection logic that classifies terminal redraw/TUI output as unsafe for chat and replaces it with a clean Terminal Mode status card.
Regression test:
`qa/playwright/harnessrelay.spec.ts` includes a synthetic full-screen TUI regression and a disposable Codex smoke when `codex` is installed.
Verification commands:
`npm --prefix web run build`; `go test ./...`; `HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshots inspected: `qa/artifacts/screenshots/chat-codex.png`, `qa/artifacts/screenshots/chat-codex-real.png`, `qa/artifacts/screenshots/chat-shell.png`, and `qa/artifacts/screenshots/terminal-mode.png`. The Chat Mode screenshot is acceptable for QA-001: no raw TUI garbage is visible, the status card is readable, and Open Terminal is visible.

## QA-002: Chat Mode leaks noisy Codex/TUI text on initial session start

Status: verified fixed
Screen: Chat Mode
Severity: critical
Steps to reproduce:
Create a Chat Mode session using `testdata/fake-harnesses/noisy-tui-artifact.sh`, which emits repeated `MMMMMMMM` text plus TUI control output. Observe initial live Chat Mode, switch to Terminal Mode, switch back to Chat Mode, then reload/reconnect.
Expected:
Chat Mode must never render `MMMMMMMM`, mojibake, box-drawing fragments, ANSI redraw fragments, or other noisy TUI remnants as assistant messages. It should show only the clean Terminal Mode status card when PTY output is not readable chat. Terminal Mode must preserve the raw stream.
Actual:
Playwright verified the initial live transcript shows the clean status card, never shows `MMMMMMMM`, and a mutation observer did not see transient `MMMMMMMM` leakage in `.transcript`. Terminal/raw snapshot still contains `MMMMMMMM`. After Terminal→Chat switching and reload/reconnect, Chat Mode still suppresses the artifact.
Root cause:
The Chat projection rejected alternate-screen/box-drawing/mojibake-heavy output, but did not classify short repeated-character TUI remnants such as `MMMMMMMM` as terminal noise. Those short chunks could pass through live event handling as readable assistant text.
Fix summary:
Extended the shared Chat projection helper to classify repeated-character artifacts as terminal-only output. The same `projectTerminalOutputForChat` path is used for live events, snapshots, mode switching, and reconnect reconstruction.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `QA-002: Chat Mode suppresses live noisy TUI artifacts consistently`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
The deterministic fake harness is `testdata/fake-harnesses/noisy-tui-artifact.sh`. This specifically catches the original live-leak behavior before mode switching.

## QA-003: Chat Mode Send does not submit Enter to Codex/TUI

Status: verified fixed
Screen: Chat Mode composer
Severity: critical
Steps to reproduce:
Create a Chat Mode session using `testdata/fake-harnesses/ready-received.sh`. Wait for `READY`, click Send with `hello-from-chat`, then type `hello-from-enter` and press Enter in the composer.
Expected:
Chat Mode Send and composer Enter both write the prompt and a real terminal Enter to the PTY. The fake harness should emit `RECEIVED:<prompt>` without switching to Terminal Mode. This must be generic and not Codex-specific.
Actual:
Playwright verified both clicking Send and pressing Enter produce `RECEIVED:<prompt>` in Chat Mode without switching to Terminal Mode. The existing `/bin/bash` Chat Mode test also verifies command execution after Send/Enter.
Root cause:
Chat Mode was appending `\n` to the raw text payload. Terminal/xterm Enter and the backend special-key endpoint use carriage return `\r`, which interactive TUIs such as Codex expect for prompt submission.
Fix summary:
Added `api.sendPrompt(sessionID, text)`, which writes raw prompt text and then sends the backend `Enter` special key. Chat Mode now uses this helper for button Send and composer Enter. Shift+Enter still inserts a newline locally.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `QA-003: Chat Mode Send and Enter submit a real terminal Enter`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
The deterministic fake harness is `testdata/fake-harnesses/ready-received.sh`. The real Codex smoke also uses the same Chat send path.

## Screen 1 — Login/Auth

Status: verified
Screen: Login / auth screen
Severity: high
Steps to reproduce:
Open the dashboard, verify unauthenticated session APIs fail, enter an invalid token, then enter `dashboard-token` and submit with Enter.
Expected:
The login page loads with the HarnessRelay logo/theme, the token field accepts text, invalid auth shows a useful error, protected APIs fail without auth, valid auth enters the app, reload stays authenticated, and no unexpected console/page errors occur.
Actual:
Playwright verified `GET /api/v1/sessions` returns 401 before auth, invalid token shows an error, Enter submits the valid token, the app shell loads, and reload preserves authenticated dashboard state.
Root cause:
No critical product defect found in this pass. The disabled primary button looked too close to enabled state in the screenshot.
Fix summary:
Adjusted disabled `.primary-button` styling so the empty-token Sign in state is visually distinct.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 1: Login/Auth`.
Verification commands:
`go test ./...`; `HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/login.png`. The login screen is acceptable after disabled-button polish.

## Screen 2 — Empty App Shell

Status: verified
Screen: App shell / sidebar
Severity: medium
Steps to reproduce:
Log in and wait for the session sidebar and create-session form.
Expected:
The sidebar looks clean, create-session form is visible, empty state is clear, refresh is visible, there is no horizontal overflow, and no unexpected console/page errors occur.
Actual:
Playwright verified the brand/sidebar, form, `No sessions yet` state, centered empty-state guidance, refresh button, and no horizontal overflow.
Root cause:
No product defect found in this pass.
Fix summary:
No code fix required.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 2: Empty App Shell`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/empty-app-shell.png`. The empty shell is acceptable for desktop viewport; mobile/responsive coverage remains later scope.

## Screen 3 — Create Session

Status: verified
Screen: Create session form
Severity: high
Steps to reproduce:
Use the sidebar form. Verify default Chat mode, empty command rejection, bad command failure, and valid `/bin/echo` session creation with args and `/tmp` cwd.
Expected:
Name, command, args, CWD, and mode controls accept input. Args is empty by default and does not imply `-`; empty command is rejected client-side; bad command fails gracefully; valid command creates the newest visible session; stale errors clear after success.
Actual:
Playwright verified all fields, default Chat mode, empty command notice, bad command notice, valid `/bin/echo` Terminal session, newest session ordering, and visible output. The first visual pass caught a stale bad-command notice persisting after valid creation; the final screenshot shows it fixed.
Root cause:
The create flow kept the global error banner after successful session creation. The Args field also used `-l` as placeholder text, which could be mistaken for a default argument.
Fix summary:
Clear global error on successful session creation and changed the Args placeholder to `optional arguments`.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 3: Create Session`.
Verification commands:
`go test ./...`; `HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/create-session.png`. The create-session screen is acceptable for current desktop QA.

## Screen 4 — Chat Mode With Simple Shell

Status: verified
Screen: Chat Mode
Severity: high
Steps to reproduce:
Create a Chat Mode `/bin/bash` session with cwd `/tmp`. Send `echo chat-mode-works`, send another command with Enter, verify Shift+Enter keeps a newline, then send a long `printf` command and open Terminal Mode.
Expected:
User messages appear, readable terminal text appears or a clean status card appears, no terminal control garbage is shown, composer remains usable, Send button works, Enter sends, Shift+Enter keeps multiline editing, long output does not create horizontal page overflow, and Open Terminal works.
Actual:
Playwright verified button send, Enter send, Shift+Enter newline behavior, long terminal text wrapping/no horizontal page overflow, visible readable transcript content, and successful switch to Terminal Mode.
Root cause:
Enter in the Chat composer previously inserted a newline and did not submit, which made keyboard sending inconsistent with common chat behavior.
Fix summary:
Added Chat composer key handling: Enter submits; Shift+Enter preserves a newline for multiline input.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 4: Chat Mode with simple shell`.
Verification commands:
`go test ./...`; `HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/chat-shell.png`. The view is acceptable for current desktop QA: no mojibake/control garbage, no horizontal overflow, composer visible, and Open Terminal visible.

## Screen 5 — Chat Mode With Codex

Status: verified through QA-001 regression
Screen: Chat Mode with Codex
Severity: critical
Steps to reproduce:
Create disposable `/tmp/harnessrelay-qa-codex`, start `codex` in Chat Mode with empty args, send the safe summary prompt, open Terminal Mode, interrupt, and clean up if the process remains live.
Expected:
Codex TUI output must not corrupt Chat Mode, Chat Mode must avoid mojibake/box-drawing/raw redraw blocks, user prompt appears as a user message, Open Terminal works, Terminal Mode remains the raw source of truth, and interrupt/terminate cleanup is safe.
Actual:
Playwright verified the real Codex smoke when `codex` is installed. Chat Mode showed either the clean Terminal Mode status or limited readable information, retained the submitted user prompt, showed no corrupted TUI garbage, opened Terminal Mode, and handled Interrupt. If Interrupt exits Codex, the test verifies the exited/terminated state; otherwise it terminates the live session.
Root cause:
Covered by QA-001.
Fix summary:
Covered by QA-001.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `QA-001 Codex smoke in disposable directory`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshots inspected: `qa/artifacts/screenshots/chat-codex.png` and `qa/artifacts/screenshots/chat-codex-real.png`. Do not run Codex in the HarnessRelay repo for QA.

## Screen 6 — Slash Command Menu

Status: verified
Screen: Slash command menu
Severity: medium
Steps to reproduce:
Create a Chat Mode `/bin/bash` session, open the `/` menu, inspect action visibility, trigger Refresh Snapshot, Send Escape, Send Ctrl+C, Send Enter, Open Terminal Mode, and verify destructive Terminate/Force kill actions prompt before proceeding.
Expected:
The `/` button is visible, menu opens and closes, all listed actions are visible, safe actions run without console/page errors, Open Terminal Mode switches modes, destructive actions require browser confirmation/prompt, and the menu is not clipped.
Actual:
Playwright verified menu role/menuitem structure, action visibility, menu close behavior, Refresh Snapshot, Send Escape, Send Ctrl+C, Send Enter, Open Terminal Mode, Terminate confirmation, and Force kill prompt.
Root cause:
No critical product defect found in this pass.
Fix summary:
No code fix required.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 6: Slash Command Menu`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/slash-menu.png`. The menu is acceptable for current desktop QA and is not clipped.

## Screen 7 — Terminal Mode

Status: verified
Screen: Terminal Mode
Severity: high
Steps to reproduce:
Create a Terminal-start `/bin/bash` session in `/tmp`. Type through xterm, insert pasted text through xterm, use raw input fallback, resize the browser, interrupt `sleep 5`, verify the shell remains usable, switch Chat/Terminal modes, check Force kill prompt, then terminate.
Expected:
xterm renders correctly, keyboard typing works, paste-like inserted text works, raw input fallback works, resize reaches backend, interrupt works, terminate works, Force kill requires typed confirmation, debug/events panel is not visually dominant, and switching modes does not restart the session.
Actual:
Playwright verified xterm rows, keyboard input, inserted paste text, raw fallback input, backend terminal width change from resize, interrupt recovery, compact debug footer, mode switching without losing output, Force kill prompt, and terminate status.
Root cause:
No product defect found in this pass.
Fix summary:
No code fix required.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 7: Terminal Mode`.
Verification commands:
`go test ./...`; `HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/terminal-mode.png`. Terminal Mode is acceptable for current desktop QA: raw xterm is readable, the fallback input remains visible, resize is reflected as `36×152`, and debug events stay a small footer.

## Screen 8 — Reconnect/Reload

Status: verified
Screen: Reconnect / reload
Severity: high
Steps to reproduce:
Create a Chat Mode `/bin/bash` session, produce output, reload the page, verify auth/session list restoration, reopen the session, verify snapshot replay, produce more output over WebSocket, switch modes, and resize after opening a completed `/bin/echo` session.
Expected:
Login state survives reload, sessions reload, selected session can reopen, snapshot shows prior output, WebSocket resumes live output, mode switching still works, and completed-session resize does not crash.
Actual:
Playwright verified reload, session list restoration, snapshot text before reload, live output after reload, Chat/Terminal switching, completed session output, and no console/page errors after viewport resize on the completed session.
Root cause:
No product defect found in this pass.
Fix summary:
No code fix required.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 8: Reconnect and Reload`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/reconnect.png`. The reconnect view is acceptable for current desktop QA. Persistence remains in-memory across browser reloads, not daemon restarts.

## Screen 9 — Multiple Sessions

Status: verified
Screen: Multiple sessions
Severity: high
Steps to reproduce:
Create two Terminal Mode `/bin/bash` sessions, send distinct output to each, switch between sessions, send more input to the selected session, terminate one, then verify the other remains usable.
Expected:
Multiple sessions can be created and switched, output does not leak between sessions, input goes only to the selected session, terminating one does not terminate another, and newest/session ordering is consistent.
Actual:
Playwright verified newest session ordering, distinct backend snapshots, selected-session input routing, no cross-session output leaks, confirmed termination of one session, and continued input/output in the other session.
Root cause:
No product defect found in this pass.
Fix summary:
No code fix required.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `Screen 9: Multiple Sessions`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Screenshot inspected: `qa/artifacts/screenshots/multiple-sessions.png`. The multiple-session view is acceptable for current desktop QA.

## QA-009: Real Harness Invocation Smoke

Status: superseded by Screen 5 Codex Playwright smoke
Screen: Real harness invocation smoke test
Severity: medium
Steps to reproduce:
Create disposable `/tmp/harnessrelay-qa-codex`, start `codex` in Chat Mode with empty args, send the harmless summary prompt, interrupt, and terminate only if the process remains live.
Expected:
The real harness TUI remains controllable through Terminal Mode, Chat Mode does not display corrupted raw TUI output, and the test is explicitly skipped if `codex` is unavailable.
Actual:
The current Playwright suite runs the Codex disposable-directory smoke when `codex` is installed. It verifies Chat Mode cleanliness, user prompt visibility, Terminal Mode access, Interrupt behavior, and conditional Terminate cleanup.
Root cause:
The older CDP smoke used an opt-in OpenCode path. The current required real-harness coverage is the Codex Playwright smoke tied to QA-001.
Fix summary:
Added Codex coverage to `qa/playwright/harnessrelay.spec.ts` using `/tmp/harnessrelay-qa-codex` and the documented safe prompt.
Regression test:
`qa/playwright/harnessrelay.spec.ts` test `QA-001 Codex smoke in disposable directory`.
Verification commands:
`HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa`
Notes:
Use only a disposable `/tmp` working directory and do not approve destructive actions.

## Final Verification — QA-002/QA-003 Pass

Date: 2026-07-26

Commands run:

```bash
rtk go test ./...
rtk make test
rtk npm --prefix web run build
rtk make build
rtk npm --prefix web run qa
rtk env HARNESSRELAY_TOKEN=dashboard-token HARNESSRELAY_DASHBOARD_URL=http://127.0.0.1:8767/ CHROME_CDP_URL=http://127.0.0.1:9222/json/list node qa/dashboard-smoke.mjs
```

Results:

- `go test ./...`: passed, 104 tests across 13 packages.
- `make test`: passed.
- `npm --prefix web run build`: passed.
- `make build`: passed.
- Playwright QA: passed, 12 tests.
- Legacy CDP dashboard smoke: passed after starting local `harnessd` and headless Chrome CDP.

Manual/real Codex validation:

- `codex --version`: `codex-cli 0.145.0`.
- Playwright created `/tmp/harnessrelay-qa-codex`, launched `codex` in Chat Mode with empty args, verified no noisy Chat Mode TUI artifacts, submitted the safe prompt through Chat Mode, opened Terminal Mode, and verified the prompt was submitted into the raw TUI without a manual Terminal Mode Enter. The raw Codex TUI remained visible/usable and Interrupt/cleanup behavior was exercised.
