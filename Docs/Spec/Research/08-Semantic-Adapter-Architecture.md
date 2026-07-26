# Semantic Adapter Architecture

Date: 2026-07-26

## Decision

HarnessRelay will add backend-owned, per-session semantic adapters alongside the
existing raw PTY path.

```text
PTY bytes ───────────────► terminal.output ─► Terminal Mode
   │
   └─► selected adapter ─► semantic events ─► Chat Mode
```

Terminal Mode remains the source of truth. Semantic parsing never mutates,
replaces, delays, or filters the raw terminal stream.

## Goals

- Select an adapter from the launch command without global harness assumptions.
- Always fall back to the generic adapter.
- Expose adapter identity and capabilities through session metadata.
- Process output in the backend and publish semantic events on the existing bus.
- Give every session its own stateful parser so sessions cannot mix state.
- Make prompt submission adapter-aware.
- Keep event-bound actions stale-safe and fail closed.

## Adapter Contracts

The core adapter describes identity, matching, and capabilities:

```go
type Adapter interface {
    ID() string
    Name() string
    Priority() int
    Match(LaunchSpec) MatchResult
    Capabilities() []Capability
}
```

Optional interfaces keep unsupported features out of the common core:

```go
type ParserProvider interface {
    NewParser() Parser
}

type PromptSubmitter interface {
    PromptBytes(text string, terminalSnapshot []byte) []byte
}

type ActionHandler interface {
    ActionBytes(actionID string) ([]byte, bool)
}
```

`Parser` receives the latest raw chunk plus the bounded raw snapshot. It returns
event envelopes without session IDs; the session manager attaches the session
ID and publishes them.

## Registry

The registry evaluates all adapters and chooses the highest-priority match.
Confidence breaks equal-priority ties. The generic adapter matches every launch
with the lowest priority and is always registered by the default manager.

Command matching uses the executable basename, not substring matching:

- `codex` matches.
- `/usr/local/bin/codex` matches.
- `codex-wrapper`, `mycodex`, and an argument containing `codex` do not match.

## Capabilities

Initial capability identifiers:

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

Capabilities describe behavior that is actually implemented. In particular,
approval detection does not imply approval execution.

## Semantic Events

The initial backend event vocabulary is:

- `harness.detected`
- `harness.status`
- `harness.metadata`
- `chat.user_message`
- `chat.assistant_message`
- `chat.system_message`
- `terminal.noisy_output`
- `approval.required`
- `approval.resolved`
- `adapter.warning`
- `adapter.error`

All events use the existing event envelope and per-session sequence. Semantic
payloads include confidence when derived from terminal behavior.

The initial design deliberately did not claim assistant extraction without
reliable screen reconstruction. Browser validation then justified adding a
session-scoped headless xterm model. The adapter now emits assistant messages
from the rendered prompt/response transcript after output settles, while still
refusing to turn raw redraw fragments directly into Chat messages.

## Output Processing

The session manager publishes raw output first, then offers the same chunk and
current bounded snapshot to the selected session parser. This ordering preserves
the raw path even if parsing fails.

Per-session parser state suppresses duplicate detection, metadata, status,
noise, and approval events. A short backend inactivity timer converts an active
Codex state back to `idle` after redraw output stops. Approval state cancels the
idle timer until it is resolved.

The first implementation parses chunks and snapshots. It does not implement a
VT screen model. Confidence must remain conservative because cursor-positioned
text can be reordered in raw history.

## Prompt Submission

Prompt submission is a distinct backend operation:

```text
POST /api/v1/sessions/{id}/prompt
```

The generic adapter sends UTF-8 text followed by carriage return. A specific
adapter may provide another sequence. Input limits, authentication, CSRF
protection, liveness checks, and audit logging remain common backend concerns.

The frontend does not invent harness-specific key sequences.

## Actions And Safety

Semantic actions are defined by backend event payloads and tied to:

- session ID
- event ID
- action ID
- action version
- current pending adapter state

The API first verifies event history and advertised action metadata, then asks
the session manager to execute the action. Resolved, superseded, unknown, and
cross-session actions fail with a conflict.

Rules:

- Never auto-run an action.
- Never auto-approve.
- Do not expose an approve action from a terminal parser until its selection
  and confirmation sequence is verified across supported Codex versions.
- Safe deny may be exposed when its sequence is verified.
- UI-only `open_terminal` is handled locally and never written to the PTY.

## Frontend Behavior

Generic Chat Mode keeps its conservative terminal projection.

Codex Chat Mode:

- shows adapter name and capabilities from the session API
- ignores raw `terminal.output` for transcript generation
- renders backend semantic chat/status/metadata/approval events
- keeps an Open Terminal control visible
- restores semantic history from the events API on reload

Switching views changes frontend state only and never recreates the session.

## Testing

Required deterministic coverage:

- registry ordering and generic fallback
- exact Codex command matching
- capabilities
- parser startup, metadata, status, noise, and duplicate suppression
- Kitty and generic prompt sequences
- fake Codex PTY prompt submission
- approval detection and stale deny rejection
- raw terminal preservation
- API adapter metadata
- browser reload and multi-session isolation

Real Codex validation supplements but does not replace deterministic tests.

## Future Structured Adapter

Codex app-server is a strong future adapter source because it exposes threads,
turns, streamed agent messages, commands, and structured approval requests.
Adopting it would change session ownership from a single PTY TUI process to a
coordinated app-server plus optional remote TUI. That deserves a separate design
and compatibility checkpoint; the first adapter does not silently replace the
current PTY-owned architecture.
