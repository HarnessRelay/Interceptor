# Semantic Adapters

## Purpose

HarnessRelay owns a raw PTY for every session. Terminal Mode renders that PTY
as the source of truth. A semantic adapter is an optional backend classifier
that turns harness-specific terminal behavior into stable product events for
Chat Mode.

```text
Harness process
  -> PTY runtime
  -> raw terminal.output -> Terminal Mode
  -> selected adapter -> semantic events -> Chat Mode
```

Generic Chat Mode remains a best-effort terminal projection. A semantic adapter
can identify status, metadata, approvals, and prompt submission rules that the
generic projection cannot know.

## Registry And Selection

`internal/harness.Registry` holds independent adapters. On session creation it
evaluates each `Match` result and chooses the highest-priority match. Confidence
breaks equal-priority ties.

The production registry contains:

- `codex`, priority `100`, matching only an executable basename exactly equal
  to `codex`
- `generic`, priority `-1000`, matching every launch as the mandatory fallback

The `fake-semantic` QA adapter is registered only when
`HARNESSRELAY_ENABLE_FAKE_ADAPTER=1`. It proves a third semantic adapter can
provide metadata, commands, terminal-only decisions, and executable actions
without frontend adapter branches.

`codex-helper`, `mycodex`, and an argument containing `codex` do not select the
Codex adapter.

## Adapter Contracts

Every adapter implements identity, selection, and capabilities:

```go
type Adapter interface {
    ID() string
    Name() string
    Priority() int
    Match(LaunchSpec) MatchResult
    Capabilities() []Capability
}
```

Optional behavior uses narrow interfaces:

- `ParserProvider`: creates isolated parser state for one session
- `PromptSubmitter`: creates one atomic prompt plus submit byte sequence
- `PromptSequencer`: creates ordered text and submit-key PTY writes when the
  harness must process them separately
- `ActionHandler`: returns an adapter-neutral result for a valid action,
  including resolution, detail, optional PTY input, and optional events
- `ActionObserver`: clears parser state after an action or raw fallback input
- `CommandCatalogProvider`: exposes version-verified harness commands
- `CommandSequencer`: validates a catalog command and produces ordered PTY
  writes without opening an agent turn

The session manager attaches session IDs and sequences to parser events and
publishes them on the existing event bus. Raw output is published first and is
never delayed, mutated, or removed by parsing.

## Capabilities

Session responses expose `adapter_id`, `adapter_name`, and
`adapter_capabilities`. Initial identifiers are:

- `raw_terminal`
- `chat_projection`
- `semantic_chat`
- `prompt_submit`
- `approval_detection`
- `approval_actions`
- `status_detection`
- `metadata_detection`
- `noise_filtering`
- `text_input`
- `special_keys`
- `resize`
- `interrupt`
- `terminate`
- `command_catalog`
- `command_invoke`

Capabilities describe implemented behavior. They are not permission grants.

## Harness Commands

`GET /api/v1/sessions/{id}/commands` returns the active adapter's normalized
catalog. `POST /api/v1/sessions/{id}/commands/{command_id}` validates the
catalog ID, readiness, and pending-approval state before writing through the
adapter's current keyboard protocol.

Command interaction modes tell the dashboard whether to remain in Chat Mode,
switch to a native terminal picker, prefill a sensitive command without Enter,
or insert an argument-bearing command into the composer. The Codex catalog is
currently verified for `codex-cli 0.145.x`; unknown versions return an empty
catalog and Terminal fallback rather than stale claims.

The dashboard combines this catalog with common UI/session/terminal actions.
Common actions are filtered by capabilities such as `special_keys`,
`interrupt`, and `terminate`; adapter commands require no frontend edits.

## Semantic Events

Adapters publish through the normal event envelope:

- `harness.detected`: selected adapter, name, match confidence, and reason
- `harness.status`: adapter state such as `processing`, `idle`,
  `terminal_ui_active`, or `waiting_for_approval`
- `harness.metadata`: optional model, version, and working directory
- `chat.user_message`: prompt accepted by the backend
- `chat.assistant_message`: a rendered response reconstructed after output
  reaches a quiet period
- `chat.system_message`: safe adapter guidance
- `terminal.noisy_output`: raw output intentionally excluded from Chat
- `approval.required`: event-bound operation and available actions
- `approval.resolved`: action and resolution for the approval event
- `adapter.warning` and `adapter.error`: classifier diagnostics

Codex raw chunks never become Chat messages directly. A session-scoped,
xterm-compatible screen model applies cursor movement, erasure, redraw, Unicode,
and wrapping before extracting the response associated with the submitted
prompt.

## Generic Adapter

Generic provides raw terminal control, conservative Chat projection, carriage
return prompt submission, special keys, resize, interrupt, and termination.

Its approval-like text detection emits typed `events.ApprovalRequired` with low
confidence and `requires_terminal`. It exposes only `open_terminal`; it does
not advertise approve or deny because a generic adapter cannot know the active
terminal selection or key mapping.

## Codex Adapter

The Codex adapter is PTY-derived and version-scoped.

It provides:

- exact executable detection
- one clean terminal-interface system message
- model, Codex version, and working-directory metadata where visible
- basic processing, terminal UI, approval, and idle status
- complete exclusion of Codex `terminal.output` from Chat transcript rendering
- assistant response extraction from the rendered screen after output settles
- stable per-turn message IDs so a later response revision replaces its earlier
  projection instead of creating a duplicate bubble
- Kitty keyboard protocol Enter (`CSI 13 u`) after Codex enables `CSI > 7 u`
- carriage-return fallback when enhanced keyboard mode was not observed
- exact approval prompt detection and command extraction
- event-bound deny through Escape
- workspace trust detection with a terminal-only decision card
- raw Terminal Mode access at all times

The screen model is provided by the pinned `github.com/gitpod-io/xterm-go`
module, a headless Go port of xterm.js. Extraction finds the submitted `›`
prompt and the final following `•` response block. Startup notices, composer
placeholders, model footers, transient working text, and duplicate redraws are
excluded.

Local shim input is raw TUI input, not the semantic prompt API. Editing,
multiline input, paste, completion, overlays, and enhanced keyboard modes make
"bytes before Enter" an unsafe universal prompt reconstruction rule. The
dashboard therefore shows an explicit terminal-control notice for shim
sessions. It continues to reconstruct reliable adapter output, but does not
invent uncertain user messages. Terminal Mode remains the complete record.

## Prompt Submission

Chat sends:

```text
POST /api/v1/sessions/{id}/prompt
{"text":"..."}
```

The backend checks liveness, input size, and pending approvals, then asks the
selected adapter for an atomic prompt sequence. The accepted user prompt and a
processing status are emitted as semantic events. Audit records store only the
byte count, not prompt contents.

Codex tracks the current Kitty keyboard protocol push/pop state in its
session-scoped parser. It writes prompt text first, then the currently valid
Enter sequence in a separate PTY write after a short bounded delay. A session
input mutex prevents raw terminal input or semantic actions from interleaving
between those writes.

Semantic prompts are accepted only when the backend adapter state is `idle`.
Startup, processing, command approval, and workspace trust states reject prompt
input instead of typing into the wrong TUI surface.

## Actions And Safety

Actions are backend-defined and frontend-rendered. Executable actions are bound
to the current session, source event ID, action ID, and action version.

Adapters return `ActionResult` with adapter-owned resolution/status/detail,
optional terminal input, and optional semantic events. Common code does not
assume denial or mention a harness by name. When detail is absent it emits the
neutral fallback `"{adapter name} completed the requested action."`

The server rejects:

- unknown or cross-session event IDs
- action IDs not advertised by that event
- version mismatches
- actions that are no longer pending
- replays after resolution

The Codex adapter exposes:

- `codex.approval_deny`, kind `approval`, mapped to the observed Escape sequence
- `open_terminal`, kind `ui`, handled entirely in the browser

It does not expose approve-once or persistent approval. No action is automatic.
Force kill retains typed confirmation and termination retains confirmation.

Any adapter may set `blocks_prompt` and `requires_terminal` on an approval or
permission request. Common state blocks prompts from these fields rather than
adapter identity or operation kind. Codex workspace trust uses this mechanism
and exposes only Open Terminal; HarnessRelay does not choose a trust level.

## Adding An Adapter

1. Add a package under `internal/harness/<id>`.
2. Match the executable conservatively and test false positives.
3. Declare only implemented capabilities.
4. Keep parser state session-scoped and suppress duplicate events.
5. Treat terminal chunks and snapshots as untrusted, cursor-addressed data.
6. Assign conservative confidence to inferred state.
7. Implement prompt bytes only after PTY research and deterministic testing.
8. Expose executable actions only after exact request detection and key-sequence
   validation.
9. Register the adapter without changing Generic fallback behavior.
10. Add unit, fake PTY, API, browser, and safe real-harness validation.

11. Verify Generic and another semantic adapter do not receive the new
    adapter's labels, commands, or action resolution.

## Testing

Deterministic coverage must include matching, capabilities, parser
classification, noise suppression, duplicates, prompt bytes, real PTY prompt
receipt, action freshness, raw terminal preservation, reload, and multi-session
isolation.

The fake executable `testdata/fake-harnesses/codex` simulates keyboard protocol
negotiation, metadata, `MMMMMMMM`, a full-screen frame, prompt submission,
processing, a cursor-addressed assistant response, and an approval overlay.

`testdata/fake-harnesses/fake-semantic` is the cross-adapter proof. It must
remain non-Codex branded and its commands/actions must flow through the same
registry, API, and frontend contracts.

Real Codex tests must use a disposable `/tmp` repository, must not approve
destructive actions, and supplement rather than replace fake tests.

## Known Limitations

- Codex patterns are English-language and tied to observed `codex-cli 0.145.0`
  behavior.
- Response boundaries currently depend on observed Codex `›` prompt, `•`
  response, and model-footer conventions.
- Extraction occurs after a three-second terminal quiet period rather than
  exposing token-level streaming.
- Approve-once and persistent approval remain intentionally unavailable.
- Model and other native pickers are launched from the unified palette but are
  completed in Terminal Mode.
- Semantic/event history remains in memory and is lost on daemon restart.
- Finished-session semantic history, including a final response flushed during
  process exit, remains available for the daemon lifetime.
- `codex app-server` is the preferred future structured source, but adopting it
  requires a separate session-ownership design.

Research details are in
`Docs/Spec/Research/08-Semantic-Adapter-Architecture.md` and
`Docs/Spec/Research/09-Codex-Adapter-Research.md`.
Cross-harness architecture and permission research is in
`Docs/Spec/Research/10-Universal-Harness-Architecture.md` through
`Docs/Spec/Research/12-Permission-Approval-Model.md`.
