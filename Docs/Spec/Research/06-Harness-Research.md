# Harness Research

## Recommendation

Keep the generic adapter mandatory and implement harness-specific adapters as optional capability providers. Use OpenCode as the first real harness adapter target after the generic PTY runtime works.

## Alternatives Considered

| Alternative | Decision | Reason |
| --- | --- | --- |
| Generic adapter only forever | Reject | Raw terminal is mandatory, but semantic approvals need optional adapter capability. |
| First adapter for Claude Code | Defer | Strong hooks/permission docs, but behavior and availability should be validated after generic runtime. |
| First adapter for Codex | Defer | Important target, but current approval surfaces are broader than a first conservative adapter needs. |
| First adapter for OpenCode | Choose | Open source, terminal-first, documented server/SDK/permissions, and good approval concepts. |
| Common API shaped around first adapter | Reject | The dashboard must render backend-provided actions for any harness. |

## Common Terminal Harness Behavior

Terminal-based coding harnesses commonly:

- run as full-screen or semi-full-screen TUIs
- use ANSI/VT escape sequences and alternate screen buffers
- accept typed prompts in a terminal input area
- expose slash commands or command palettes
- ask permission before edits, shell commands, network access, external-directory access, or other risky tools
- support Ctrl+C or Escape-like interruption
- redraw status bars, spinners, and progress areas in place
- may provide separate headless/server/API modes that differ from TUI behavior

The interceptor cannot assume a universal "Approve button" exists in the PTY stream. It can always provide terminal display/input and only sometimes infer semantics.

## Approval Prompt Patterns

Common action choices:

- approve once
- approve always / remember for session
- deny/reject
- cancel
- edit requested command/prompt
- ask for more information

Common context fields:

- command to run
- working directory
- file path to edit
- tool name
- requested permission class
- risk or reason text

Adapter guidance:

- Semantic approval events must include confidence.
- Event-bound actions must include `event_id`.
- Do not auto-approve by default.
- Always show raw terminal nearby so the user can verify context.

## Command Palette Patterns

Common patterns:

- `/` opens slash command autocomplete in many coding TUIs.
- Ctrl-based shortcuts may open command dialogs.
- Menus are often rendered with cursor movement, not stable line output.
- Some tools expose commands through documented APIs or config files instead of TUI scraping.

Generic adapter should not claim command-palette support beyond raw input. Harness-specific adapters may expose `open_palette` only after manual validation.

## Interruption Patterns

Common interruption options:

- Ctrl+C cancels current generation/tool execution or exits if idle.
- Escape may close a menu or cancel a prompt in some TUIs.
- A second Ctrl+C may force exit in some tools.
- Headless/server APIs may expose structured cancellation.

Generic behavior:

- `interrupt` writes Ctrl+C to PTY.
- `terminate` sends SIGTERM to process group.
- `kill` sends SIGKILL after confirmation.

Harness-specific adapter may override interrupt only if documented and reliable.

## Structured APIs, Hooks, SDKs, And Permission Surfaces

### OpenCode

OpenCode currently documents:

- terminal TUI
- `opencode serve` local HTTP server
- OpenAPI 3.1 spec endpoint
- JS/TS SDK
- permission config with `allow`, `ask`, `deny`
- approval outcomes including once, always, reject
- command palette and slash commands

OpenCode server defaults include localhost binding and optional HTTP basic auth via environment variables. Its documented server/API shape makes it useful for adapter research without making HarnessRelay depend on it.

### Claude Code

Claude Code currently documents:

- permission modes such as `default`, `acceptEdits`, `plan`, `auto`, `dontAsk`, and `bypassPermissions`
- fine-grained permissions
- `/permissions`, slash commands, and hooks
- hook events and JSON payloads for lifecycle/tool events
- security warnings for command hooks

Claude Code is important for future adapter research because hooks can expose structured lifecycle information. TUI behavior still needs raw PTY fallback.

### OpenAI Codex CLI

OpenAI Codex currently documents:

- approval modes and sandbox behavior
- local terminal operation
- app-server approval flow in the repository docs, including command execution approval request shapes and decisions
- SDK/sandbox documentation in repository docs

Codex is an important later adapter target. Its surfaces may change, so adapter work should verify against installed version and official docs at implementation time.

### Other Harnesses

Other terminal coding tools should start with the generic adapter unless they expose official hooks, server APIs, or stable prompt structures. Do not add harness-specific assumptions to the common API.

## First Real Adapter Target

Recommendation: OpenCode.

Reasoning:

- It is open source and terminal-first.
- It has documented permission behavior and server/SDK surfaces.
- It exposes a local server architecture that may offer structured integration paths.
- Approval concepts map well to the proposed generic `approval.required` event without forcing the common API to be OpenCode-specific.
- It is practical for fake-output tests and manual validation.

The first adapter should start conservative:

- detect command `opencode`
- identify visible approval-like prompt patterns with confidence
- emit `approval.required` only when confidence is high enough
- implement raw terminal fallback for approve/deny if reliable key sequences are validated
- optionally use OpenCode server/API only after confirming it can attach to or represent the same session model safely

## Generic Adapter Requirements

Generic adapter must always provide:

- raw terminal output
- raw input
- text input
- special keys through terminal bytes
- resize
- interrupt
- terminate
- event pass-through
- optional low-confidence heuristic prompt detection

Generic adapter must not:

- invent approve/deny for every harness
- hide raw terminal
- claim structured state from text heuristics as certain
- hardcode one harness into common event schema

## Risks And Limitations

- TUI text scraping is fragile across versions, themes, terminal widths, and localization.
- Some tools have server APIs that create new sessions instead of controlling an existing TUI.
- Approval prompts may include sensitive command or file context.
- "Always approve" style actions are risky and should not be enabled until action safety rules are implemented.
- Harness docs and behavior change over time; adapter implementation must verify current versions.

## Acceptance Criteria For Later Implementation

- Generic adapter is selected for unknown commands.
- OpenCode adapter selection is based on command/spec matching, not global assumptions.
- Semantic approval events include confidence, context, and backend-provided actions.
- Raw terminal remains usable when adapter detection fails.
- Stale action rejection protects approval actions.
- Adapter tests use fake harness output before real harness manual tests.

## Required Tests

- Unknown command uses generic adapter.
- `opencode` command matches OpenCode adapter when adapter is enabled.
- Fake approval prompt emits `approval.required` with event-bound actions.
- Low-confidence heuristic event is marked as heuristic.
- Action against stale approval is rejected.
- Raw terminal input still works while an approval card is displayed.
- Ctrl+C interrupt works for generic and adapter sessions.

## Sources

- [OpenCode permissions docs](https://opencode.ai/docs/permissions)
- [OpenCode server docs](https://dev.opencode.ai/docs/server/)
- [OpenCode SDK docs](https://dev.opencode.ai/docs/sdk)
- [Claude Code permissions docs](https://code.claude.com/docs/en/permissions)
- [Claude Code hooks docs](https://code.claude.com/docs/en/hooks)
- [OpenAI Codex CLI help article](https://help.openai.com/en/articles/11096431)
- [OpenAI Codex app-server docs](https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md)
