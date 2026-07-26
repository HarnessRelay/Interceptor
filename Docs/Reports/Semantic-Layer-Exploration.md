# Semantic Layer Exploration Report

## Executive Summary

HarnessRelay is still fundamentally a generic multi-harness control plane. The backend owns arbitrary PTY sessions, exposes raw terminal input/output for every session, selects adapters through a generic registry, and surfaces adapter identity, capabilities, semantic events, action catalogs, and command catalogs through generic API shapes.

Codex support is currently implemented as one high-priority adapter over the same contracts. Most Codex-specific parsing, command catalog, prompt sequencing, and safe denial behavior is correctly isolated under `internal/harness/codex/`.

There are, however, several Codex leaks and Codex-forward UX assumptions:

- `internal/session/manager.go` emits a Codex-specific status detail from common action execution after any semantic action.
- pending action state has a Codex-specific special case for workspace trust.
- frontend Chat Mode labels the metadata strip as Codex metadata and displays `Codex {version}` for any semantic adapter.
- empty states and some QA labels present Codex as the default chat-first path.
- tests strongly cover Generic and Codex, but do not prove a third semantic adapter can expose actions/commands without frontend edits or Codex assumptions.

The current risk is manageable. The architecture is not blocked for future adapters, but the common layer should be cleaned before adding Claude/OpenCode/KiloCode adapters, otherwise these small Codex-specific assumptions will become copied patterns.

## Verdict

Generic architecture status:
Mostly generic, with a working Generic fallback and clean adapter interface.

Codex leakage risk:
Medium. Codex is not baked into the base adapter interface or API schema, but it has leaked into some common session action behavior and frontend semantic labels.

Overall risk level:
Medium. The project can support future adapters, but should address common-layer wording and action status handling before adding the next semantic adapter.

## Current Adapter Architecture

The adapter architecture is centered on `internal/harness.Adapter`, which exposes only identity, priority, matching, and capabilities:

- `ID()`
- `Name()`
- `Priority()`
- `Match(LaunchSpec)`
- `Capabilities()`

Evidence: `internal/harness/adapter.go:20`.

Optional behavior is split into narrow interfaces:

- `ParserProvider`
- `PromptSubmitter`
- `PromptSequencer`
- `ActionHandler`
- `ActionObserver`
- `CommandCatalogProvider`
- `CommandSequencer`

Evidence: `internal/harness/adapter.go:75`, `internal/harness/adapter.go:80`, `internal/harness/adapter.go:95`, `internal/harness/adapter.go:106`, `internal/harness/adapter.go:111`, `internal/harness/adapter.go:116`, `internal/harness/adapter.go:122`.

The registry itself is generic. It stores arbitrary adapters, sorts by priority, and selects the highest-priority matching adapter, with confidence as a tie-breaker for equal priority. Evidence: `internal/harness/registry.go:19`, `internal/harness/registry.go:37`.

The default registry currently registers Codex plus Generic:

```go
return generic.NewRegistry(codex.New())
```

Evidence: `internal/session/manager.go:220`.

This is acceptable as product composition, not a base-interface leak. New adapters can be added to default registration or injected in tests via `NewManagerWithRegistry`. Evidence: `internal/session/manager.go:208`.

Current adapters:

- `generic`: mandatory fallback, priority `-1000`, matches every launch.
- `codex`: semantic adapter, priority `100`, exact executable basename match.

Codex matching is conservative. It checks only `filepath.Base(filepath.Clean(spec.Command)) == "codex"`. It does not match `mycodex`, `codex-helper`, or an argument containing `codex`. Evidence: `internal/harness/codex/adapter.go:35`.

The separate detected-harness launcher catalog includes Codex, OpenCode, Claude Code, Aider, and Gemini CLI. Evidence: `internal/harness/discovery.go:28`. This catalog is generic launcher UX, not adapter selection.

## Current Generic Adapter Behavior

The Generic adapter is a true fallback:

- ID: `generic`
- Name: `Generic`
- Priority: `-1000`
- `Match` always returns `Matched: true`
- prompt submission appends carriage return
- capabilities include raw terminal, chat projection, prompt submit, text input, special keys, resize, interrupt, and terminate

Evidence: `internal/harness/generic/adapter.go:20`, `internal/harness/generic/adapter.go:28`, `internal/harness/generic/adapter.go:32`, `internal/harness/generic/adapter.go:40`, `internal/harness/generic/adapter.go:53`.

Generic approval detection is heuristic and exposes only `open_terminal`, which is appropriate because Generic cannot know the active TUI selection or safe approve/deny key mapping. Evidence: `internal/harness/generic/detect.go:9`, `internal/harness/generic/detect.go:25`.

One implementation concern: generic heuristic approval payload is a `map[string]any`, while Codex approval payload uses the typed `events.ApprovalRequired`. That makes generic events harder for backend action-state handling to reason about and pushes shape flexibility to clients. Evidence: `internal/harness/generic/detect.go:14`; typed shape: `internal/events/event.go:143`.

## Current Codex Adapter Behavior

The Codex adapter is isolated in `internal/harness/codex/` and implements semantic behavior through generic contracts.

It exposes generic capability constants such as `semantic_chat`, `prompt_submit`, `approval_detection`, `approval_actions`, `status_detection`, `metadata_detection`, `noise_filtering`, `command_catalog`, and `command_invoke`. Evidence: `internal/harness/codex/adapter.go:46`.

Codex-specific behavior includes:

- exact `codex` executable matching
- Kitty keyboard protocol Enter handling
- session-scoped parser
- Codex version/model metadata parsing
- Codex TUI noise suppression
- workspace trust detection
- command approval detection
- event-bound deny via Escape
- version-scoped slash command catalog for `codex-cli 0.145.x`

Evidence:

- matching: `internal/harness/codex/adapter.go:35`
- action bytes: `internal/harness/codex/adapter.go:77`
- parser status/system messages: `internal/harness/codex/parser.go:66`
- workspace trust: `internal/harness/codex/parser.go:98`
- command approval: `internal/harness/codex/parser.go:120`
- command catalog: `internal/harness/codex/parser.go:202`
- assistant extraction: `internal/harness/codex/parser.go:292`

This is appropriate adapter-specific code. The concern is not the existence of this code; the concern is when common code assumes Codex behavior after invoking adapter-defined contracts.

## Event Model Review

Shared event types are generic:

- `terminal.output`
- `terminal.snapshot`
- `session.created`
- `session.updated`
- `session.exited`
- `session.status_changed`
- `approval.required`
- `approval.resolved`
- `harness.detected`
- `harness.status`
- `harness.metadata`
- `chat.user_message`
- `chat.assistant_message`
- `chat.system_message`
- `terminal.noisy_output`
- `adapter.warning`
- `adapter.error`
- `action.completed`
- `action.failed`
- `error`

Evidence: `internal/events/event.go:12`.

Payloads are mostly generic and include adapter/source concepts:

- `HarnessDetected` includes `adapter_id`, `harness_name`, confidence, and reason.
- `HarnessMetadata` includes generic model, working directory, version, confidence.
- `ChatMessage` includes source and confidence.
- `AdapterNotice` includes source.
- `SemanticAction` is backend-defined and generic.
- `ApprovalRequired` is generic around operation kind, command, working directory, prompt, actions, and confidence.

Evidence: `internal/events/event.go:89`, `internal/events/event.go:104`, `internal/events/event.go:112`, `internal/events/event.go:126`, `internal/events/event.go:132`, `internal/events/event.go:143`.

There are no Codex-specific event types in `internal/events`.

Risk: status strings are open-ended, not centrally typed. This is flexible for future adapters, but the frontend currently recognizes specific status strings like `processing`, `idle`, `terminal_ui_active`, and `waiting_for_approval`. This should be documented as common vocabulary before additional adapters add incompatible values. Evidence: frontend handling in `web/src/components/ChatView.tsx` status logic around semantic activity, and backend status storage in `internal/session/manager.go:759`.

## Action Model Review

There are two action models:

1. Semantic event actions from the backend, represented by `events.SemanticAction`.
2. Static frontend relay/session actions in the slash menu.

Semantic actions are backend-defined and validated by event ID, action ID, version, and pending state. Evidence:

- payload: `internal/events/event.go:132`
- API validation: `internal/api/router.go:675`, `internal/api/router.go:758`
- session pending-state validation: `internal/session/manager.go:533`

This is a good generic safety model. Unknown/stale actions are rejected, and unsupported adapter executors return `501`. Evidence: `internal/api/router.go:697`, `internal/api/router.go:701`, `internal/api/router.go:705`.

Codex exposes only `codex.approval_deny` plus browser-local `open_terminal`. Evidence: `internal/harness/codex/parser.go:144`, `internal/harness/codex/parser.go:152`.

Destructive lifecycle actions have confirmation dialogs in the frontend. Force kill requires the API confirmation string `KILL`; terminate uses a confirmation dialog but does not require a typed token. Evidence: `internal/api/router.go:583`, `web/src/components/SessionHeader.tsx:86`, `web/src/components/ChatView.tsx` `confirmAction`.

Main action-model leak:

`ExecuteAction` emits `Resolution: "denied"` and `Detail: "Approval denied; Codex is returning to the conversation."` from common session code after any adapter action. Evidence: `internal/session/manager.go:582`, `internal/session/manager.go:590`. This is Codex-specific and denial-specific in the common product architecture.

Secondary action-model issue:

`emitSemanticEvent` has a Codex-specific pending-action special case for `workspace_trust`. Evidence: `internal/session/manager.go:782`. The behavior should be modeled generically as “terminal-only blocking decision” or “blocks_prompt_until_terminal_input” rather than checking adapter ID and operation kind.

## Slash Menu Review

The slash command menu is mostly structured as:

- adapter-provided harness commands from `harnessCommands`
- static HarnessRelay controls
- terminal key actions
- lifecycle actions

Evidence:

- static relay actions: `web/src/components/SlashCommandMenu.tsx:14`
- adapter command merge: `web/src/components/SlashCommandMenu.tsx:55`
- adapter command group labeling: `web/src/components/SlashCommandMenu.tsx:59`
- header label: `web/src/components/SlashCommandMenu.tsx:125`

The static global actions are:

- Open Terminal
- Show inspector
- Refresh snapshot
- Clear local transcript
- Send Enter
- Send Escape
- Send Tab
- Send Ctrl+C
- Interrupt
- Terminate session
- Force kill

Evidence: `web/src/components/SlashCommandMenu.tsx:14`.

These are generic session/terminal controls, not Codex commands. They are shown globally for generic `/bin/bash` sessions, which is mostly appropriate. However, they are not currently filtered by adapter/session capabilities. For example, special-key actions appear regardless of whether `special_keys` is advertised, and lifecycle actions appear regardless of `interrupt`/`terminate` capabilities. Today Generic and Codex both advertise these capabilities, so there is no visible bug. Future adapters with narrower capabilities would need frontend changes or would receive actions that should have been hidden.

Codex slash commands are not hardcoded in the frontend. They arrive through `GET /api/v1/sessions/{id}/commands`, are stored in `harnessCommands`, and are invoked through `POST /api/v1/sessions/{id}/commands/{command_id}`. Evidence: `web/src/components/ChatView.tsx` command loading/invocation and `internal/api/router.go:448`.

Codex command catalog itself is correctly located in `internal/harness/codex/parser.go:250`.

Future adapters can add commands without editing many frontend files if they implement `CommandCatalogProvider` and `CommandSequencer`. The missing part is UI filtering of static relay actions by session capabilities.

## Chat Mode Review

Chat Mode decides whether to render semantic history or generic terminal projection based on the `semantic_chat` capability:

```ts
const semanticAdapter = session.adapter_capabilities?.includes("semantic_chat") ?? false;
```

Evidence: `web/src/components/ChatView.tsx:55`.

For semantic adapters, Chat Mode renders semantic event history and live semantic events. For non-semantic sessions, it projects terminal output into readable chat text when safe. Evidence: `web/src/components/ChatView.tsx:62`, `web/src/components/ChatView.tsx:66`, `web/src/components/ChatView.tsx:70`, `web/src/components/ChatView.tsx:97`.

Generic sessions still work for `/bin/bash`: Playwright creates `/bin/bash` in Chat Mode, sends commands, sees output, switches to Terminal Mode, sends raw input, and switches back. Evidence: `qa/playwright/harnessrelay.spec.ts:100`.

Chat Mode preserves Open Terminal fallback globally. Evidence: `web/src/components/ChatView.tsx` status row includes `Open Terminal`; Terminal Mode itself exposes `Open Chat` and raw input fallback in `web/src/components/TerminalView.tsx`.

Noise suppression is partly generic and partly adapter-specific:

- generic frontend projection suppresses TUI-looking output with ANSI/control/box-drawing/repeated-artifact heuristics. Evidence: `web/src/utils.ts:61`, `web/src/utils.ts:65`, `web/src/utils.ts:74`.
- Codex backend parser suppresses Codex raw TUI chunks and reconstructs semantic assistant messages. Evidence: `internal/harness/codex/parser.go:66`, `internal/harness/codex/parser.go:292`.

Prompt submission uses backend adapter behavior rather than frontend Codex logic. Evidence: `internal/session/manager.go:384`, `internal/session/manager.go:408`, `internal/session/manager.go:409`, `internal/session/manager.go:413`; frontend uses `api.sendPrompt` generically.

Chat Mode Codex leaks:

- metadata strip uses `aria-label="Codex metadata"` and displays `Codex {metadata.version}` for any semantic adapter. Evidence: `web/src/components/ChatView.tsx` semantic strip rendering.
- fallback approval card says `"Codex is waiting for a decision."` when the event has no prompt. This should be adapter/session neutral.

## Terminal Mode Review

Terminal Mode remains adapter-agnostic and PTY-first:

- it requests the current snapshot
- opens a WebSocket scoped to the session
- writes `terminal.output` bytes to xterm.js
- sends xterm `onData` input directly to `/input`
- sends resize updates
- provides raw input fallback

Evidence: `web/src/components/TerminalView.tsx`.

Terminal Mode does not special-case Codex. It is still the source-of-truth fallback for any PTY session.

Backend terminal input, resize, interrupt, terminate, and kill are session/runtime operations, not adapter operations. Evidence: `internal/session/manager.go:365`, `internal/session/manager.go:601`, `internal/session/manager.go:623`, `internal/session/manager.go:638`, `internal/session/manager.go:656`.

## API Surface Review

Session responses include generic adapter metadata:

- `harness_type`
- `adapter_id`
- `adapter_name`
- `adapter_capabilities`

Evidence: `internal/api/router.go` `sessionDTO`; `internal/api/router.go:798`.

There are no Codex-specific fields in common session DTOs.

API endpoints are generic:

- sessions
- terminal input
- prompt
- commands
- resize
- interrupt
- terminate
- kill
- cleanup
- snapshot
- events
- actions

Evidence: route registration in `internal/api/router.go`.

Command catalog API is generic and returns `supported`, `commands`, and `fallback`. Unsupported adapters return `supported: false`, empty commands, and `fallback: "terminal"`. Evidence: `internal/api/router.go:448`, `internal/api/router.go:456`, `internal/api/router.go:466`.

Semantic actions are generic at the route level: `POST /api/v1/sessions/{id}/actions/{action_id}`. Evidence: `internal/api/router.go:675`.

API docs are generally generic, but include detailed Codex examples. This is acceptable as long as examples are labeled as Codex-specific. Evidence: `Docs/API.md` describes generic session fields and then a Codex approval payload example.

## Frontend UX/Labels Review

Generic UX is present:

- app brand says “Local harness control”.
- create dialog says “Start a detected coding harness or enter an exact command.”
- command field defaults to `/bin/bash`.
- session cards show adapter badges and mode.
- session header displays adapter identity and command/cwd.

Evidence: `web/src/components/Sidebar.tsx:49`, `web/src/components/Sidebar.tsx:132`, `web/src/components/Sidebar.tsx:175`, `web/src/components/SessionHeader.tsx:116`.

Codex-forward labels:

- empty state says “Create a chat-first Codex session or open a shell...”. Evidence: `web/src/components/EmptyState.tsx:6`.
- session-list empty state says “Start Codex, a shell, or another local harness.” Evidence: `web/src/components/Sidebar.tsx:299`.
- semantic strip is labeled “Codex metadata” and prefixes version with “Codex”. This is a common UI leak for future semantic adapters.

These are UX/naming issues, not hard architectural blockers.

## Test Coverage Review

Coverage is strong for current Generic and Codex behavior:

- Generic fallback selected for plain shell API sessions. Evidence: `internal/api/router_test.go:87`, `internal/api/router_test.go:108`.
- Generic prompt submission uses line input fallback. Evidence: `internal/session/manager_test.go:175`.
- Generic approval heuristic emits `approval.required`. Evidence: `internal/session/manager_test.go:221`.
- Codex adapter matching, capabilities, parser behavior, metadata, prompt submission, command approval, safe deny, workspace trust, and command catalog are unit-tested. Evidence: `internal/harness/codex/adapter_test.go`.
- Manager-level Codex semantic flow is tested. Evidence: `internal/session/manager_test.go:281`.
- API-level Codex prompt, command catalog, and approval action are tested. Evidence: `internal/api/router_test.go:168`.
- Browser `/bin/bash` Chat Mode, Terminal Mode, slash menu, reload, multi-session, generic noisy TUI suppression, and fake Codex semantic flow are covered. Evidence: `qa/playwright/harnessrelay.spec.ts:100`, `qa/playwright/harnessrelay.spec.ts:144`, `qa/playwright/harnessrelay.spec.ts:189`, `qa/playwright/harnessrelay.spec.ts:250`, `qa/playwright/harnessrelay.spec.ts:408`.

Coverage gaps:

- no fake third semantic adapter proving adapter extensibility without Codex assumptions.
- no frontend test proving Codex command catalog items are absent for generic sessions beyond implicit `/bin/bash` slash menu checks.
- no test that static slash relay actions are filtered by adapter capabilities.
- no test that common action resolution text is adapter-neutral.
- no test for an adapter-specific non-Codex action with non-denial resolution.
- no test for generic command catalog fallback behavior in the UI.

## Findings

### Finding SL-001: Common action resolution emits Codex-specific text

Severity:
high

Area:
Action model / session manager

Evidence:
`internal/session/manager.go:590` emits `Detail: "Approval denied; Codex is returning to the conversation."` after `ExecuteAction`, regardless of which adapter handled the action.

Why it matters:
Any future adapter action would produce Codex-branded status. The common session manager is also assuming the action resolution is always `denied`, which is only true for the current Codex deny action.

Recommendation:
Move adapter-specific post-action status/resolution into adapter-provided action metadata or a generic result from `ActionHandler`. At minimum, make common text adapter-neutral.

### Finding SL-002: Pending terminal-only decision has a Codex-specific special case

Severity:
medium

Area:
Semantic event/action state

Evidence:
`internal/session/manager.go:782` checks `s.AdapterID == "codex" && approval.OperationKind == "workspace_trust"` to keep a UI-only terminal decision pending.

Why it matters:
Future adapters may have terminal-only decisions where Chat should block prompt submission until the user handles the TUI in Terminal Mode. Encoding this as Codex/workspace-trust makes the common layer harder to extend.

Recommendation:
Represent terminal-only blocking decisions generically in `ApprovalRequired` or `SemanticAction`, for example `blocks_prompt: true`, `resolution: handled_in_terminal`, or an action kind dedicated to terminal fallback.

### Finding SL-003: Semantic metadata strip is Codex-specific in common Chat Mode UI

Severity:
medium

Area:
Frontend Chat Mode labels

Evidence:
Chat Mode renders semantic metadata with an aria label of “Codex metadata” and displays `Codex {metadata.version}` for any semantic adapter.

Why it matters:
A future Claude/OpenCode/KiloCode semantic adapter would show incorrect Codex terminology even though the API already provides `adapter_name`.

Recommendation:
Use adapter-neutral text: “Harness metadata”, `Version {metadata.version}`, or `{session.adapter_name} {metadata.version}`.

### Finding SL-004: Slash menu static relay actions are not capability-filtered

Severity:
medium

Area:
Slash menu / frontend capabilities

Evidence:
`web/src/components/SlashCommandMenu.tsx:14` defines global relay actions. `web/src/components/SlashCommandMenu.tsx:55` always merges them with adapter commands. There is no use of `session.adapter_capabilities` in `SlashCommandMenu`.

Why it matters:
Today Generic and Codex both support the shown capabilities, so it works. Future adapters with narrower capabilities would still show unsupported terminal keys or lifecycle operations.

Recommendation:
Pass capabilities into the menu and filter actions by required capabilities. Keep truly UI-local actions like Open Terminal and Show inspector always available.

### Finding SL-005: Generic heuristic approval uses untyped map payload

Severity:
medium

Area:
Event model / generic adapter

Evidence:
`internal/harness/generic/detect.go:14` emits `approval.required` with `map[string]any`, while typed approval events use `events.ApprovalRequired` at `internal/events/event.go:143`.

Why it matters:
Common backend pending-action tracking only recognizes typed `events.ApprovalRequired` in `emitSemanticEvent`. Generic approval events are renderable but not consistently modeled for server-side action lifecycle.

Recommendation:
Use the typed `events.ApprovalRequired` payload for Generic as well, with `Confidence` represented consistently. If a qualitative confidence is needed, add a typed confidence kind/source field instead of changing number/string shape.

### Finding SL-006: Empty states make Codex the implied primary harness

Severity:
low

Area:
Frontend UX labels

Evidence:
`web/src/components/EmptyState.tsx:6` says “Create a chat-first Codex session or open a shell...” and `web/src/components/Sidebar.tsx:299` says “Start Codex, a shell, or another local harness.”

Why it matters:
This creates product-positioning drift. It does not break architecture, but it teaches users and future contributors that Codex is the default mental model.

Recommendation:
Lead with “coding harness”, “detected harness”, or “semantic session”, then list Codex as one example.

### Finding SL-007: Future adapter extensibility lacks proof tests

Severity:
medium

Area:
Tests / architecture validation

Evidence:
Existing tests cover Generic and Codex paths extensively, but no fake third semantic adapter exercises command catalog, custom action IDs, non-denial action resolution, status/metadata labels, or capability-filtered slash UI.

Why it matters:
The architecture appears extensible by inspection, but regressions will be easy when the second real adapter is added because current tests only prove “Generic + Codex”.

Recommendation:
Add a fake non-Codex semantic adapter test suite before implementing a real Claude/OpenCode/KiloCode adapter.

### Finding SL-008: `harness_type` can diverge from selected adapter ID

Severity:
info

Area:
Session metadata

Evidence:
`internal/session/manager.go:265` preserves caller-provided `HarnessType`; if omitted it defaults to selected adapter ID.

Why it matters:
This allows launcher intent and adapter selection to diverge. That can be useful, but docs and UI should distinguish “requested harness type” from “selected adapter”.

Recommendation:
Keep both fields, but document that `adapter_id` is authoritative for behavior and `harness_type` is request/user metadata.

## Codex-Specific Assumptions Found

| Area | File | Codex-specific behavior | Should it be generic? | Recommendation |
| ---- | ---- | ----------------------- | --------------------- | -------------- |
| Default adapter registration | `internal/session/manager.go:220` | Default registry composes Codex plus Generic. | Acceptable composition. | Keep, but add future adapters by registration, not common branching. |
| Action resolution detail | `internal/session/manager.go:590` | Common manager says “Codex is returning to the conversation.” | Yes. | Make result detail adapter-neutral or adapter-supplied. |
| Action resolution semantic | `internal/session/manager.go:582` | Common manager records every adapter action as `Resolution: "denied"`. | Yes. | Return action resolution from adapter or event action metadata. |
| Terminal-only pending state | `internal/session/manager.go:782` | Common manager special-cases Codex workspace trust. | Yes. | Model terminal-only blocking decisions generically. |
| Semantic metadata UI | `web/src/components/ChatView.tsx` | Metadata strip is labeled “Codex metadata” and prefixes version with “Codex”. | Yes. | Use adapter name or neutral “Version”. |
| Approval fallback text | `web/src/components/ChatView.tsx` | Fallback prompt says Codex is waiting. | Yes. | Use `{session.adapter_name || "Harness"}`. |
| Empty state | `web/src/components/EmptyState.tsx:6` | Codex is presented as the chat-first session path. | Mostly. | Make Codex one example rather than the lead noun. |
| Sidebar empty state | `web/src/components/Sidebar.tsx:299` | “Start Codex, a shell, or another local harness.” | Partly. | Prefer “Start a detected harness, shell, or custom command.” |
| Codex command catalog | `internal/harness/codex/parser.go:250` | Codex slash commands and labels. | No, adapter-specific. | Keep in Codex adapter. |
| Codex matching | `internal/harness/codex/adapter.go:35` | Exact executable basename match. | No, adapter-specific. | Keep conservative. |
| Codex parser strings | `internal/harness/codex/parser.go` | OpenAI Codex UI/status/approval patterns. | No, adapter-specific. | Keep isolated and version-scoped. |
| API docs example | `Docs/API.md` | Codex approval example with `codex.approval_deny`. | Acceptable if labeled. | Retain as example; add generic and future-adapter examples later. |
| README dashboard description | `README.md` | Codex-aware prompt submission and event-bound safe denial are highlighted. | Acceptable but Codex-forward. | Balance with a “future adapters use same contracts” note. |

## Generic Extension Points Found

| Extension point | File | How future adapters use it | Notes |
| --------------- | ---- | -------------------------- | ----- |
| Adapter base interface | `internal/harness/adapter.go:20` | Implement ID/name/priority/match/capabilities. | Clean, no Codex methods. |
| Capability constants | `internal/harness/adapter.go:32` | Advertise implemented behaviors to API/frontend. | Generic vocabulary. |
| Registry | `internal/harness/registry.go:19` | Register adapter and rely on priority/confidence selection. | Allows future adapters without changing selection algorithm. |
| Custom manager registry | `internal/session/manager.go:208` | Tests or alternate composition can inject adapters. | Useful for fake future-adapter proof. |
| ParserProvider | `internal/harness/adapter.go:106` | Create session-scoped parser state. | Correct isolation boundary. |
| PromptSubmitter / PromptSequencer | `internal/harness/adapter.go:111` | Provide harness-specific prompt bytes/order. | Works for Codex and future TUIs. |
| CommandCatalogProvider / CommandSequencer | `internal/harness/adapter.go:75` | Expose version-verified slash/native commands. | Frontend already renders returned catalog generically. |
| ActionHandler / ActionObserver | `internal/harness/adapter.go:122` | Map backend-advertised action IDs to PTY bytes and clear parser state. | Needs generic result metadata cleanup. |
| Event envelope | `internal/events/event.go:35` | Publish adapter semantic events with session/source/context. | Event type vocabulary is generic. |
| Session DTO adapter metadata | `internal/api/router.go:798` | Expose adapter ID/name/capabilities to clients. | No Codex-specific DTO fields. |
| Commands API | `internal/api/router.go:448` | Return active adapter catalog or terminal fallback. | Good future adapter path. |
| Semantic action API | `internal/api/router.go:675` | Validate event-bound action invocation. | Safety model is generic. |
| Terminal Mode | `web/src/components/TerminalView.tsx` | Any PTY session gets raw xterm rendering/input. | Preserves universal fallback. |
| Chat Mode capability switch | `web/src/components/ChatView.tsx:55` | Semantic adapters use events; Generic uses projection. | Good generic rendering split. |

## Recommended Architecture Direction

Keep the current architecture: a generic PTY/session control plane with optional semantic adapters.

Do not remove Terminal Mode. Do not remove Codex support. The right direction is to make the common semantic layer more adapter-neutral before adding the next real adapter.

Recommended shape:

- Common backend owns lifecycle, PTY input/output, snapshots, history, session DTOs, event envelope, stale-action validation, and generic prompt/action APIs.
- Adapters own matching, parser state, semantic event emission, prompt sequencing, command catalog, action byte mapping, action resolution metadata, and adapter-specific text.
- Frontend renders generic events/actions/catalogs based on capability and payload, while using adapter name/source only for scoped badges and metadata.
- Slash menu becomes a composition of always-available UI controls, capability-gated session controls, and adapter-provided commands.
- Third-adapter fake tests prove the contracts before a real non-Codex semantic adapter is implemented.

## Recommended Follow-up Tasks

1. Clean action resolution taxonomy: make `ActionHandler` return resolution/detail or define action result metadata.
2. Replace Codex-specific common action status text with adapter-neutral text.
3. Replace Codex workspace-trust special case with a generic terminal-only blocking decision field.
4. Convert Generic approval heuristic to typed `events.ApprovalRequired`.
5. Pass session capabilities into `SlashCommandMenu` and filter static relay actions.
6. Replace Chat Mode semantic strip labels with adapter-neutral labels.
7. Update empty-state copy to position Codex as one example, not the product default.
8. Add fake non-Codex semantic adapter tests for matching, status, metadata, prompt, command catalog, and actions.
9. Add frontend tests proving Generic sessions do not show adapter command catalog items.
10. Add API/docs examples for one generic event and one non-Codex hypothetical adapter event.
11. Document status vocabulary for `harness.status`.
12. Clarify `harness_type` vs `adapter_id` semantics in API docs.

## Questions for Project Owner

1. Should Codex remain the only real semantic adapter for now, or should the next proof be a fake non-Codex adapter before any real Claude/OpenCode work?
2. Should `harness_type` remain user/request metadata, or should it always be normalized to the selected adapter ID?
3. Should semantic actions support only approval flows, or should the action model be broadened now for future harness operations such as model changes, permission menus, and tool toggles?
4. Should slash menu static terminal-key actions be shown for every PTY session, or only when the adapter advertises `special_keys`?
5. Should command catalog IDs be adapter-local (`status`) or globally namespaced (`codex.status`, `opencode.status`) in API responses?

