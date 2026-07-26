# Universal Harness Architecture

Date: 2026-07-26

## Decision

HarnessRelay is a universal local harness relay. Codex is one adapter, not the
shape of the common product.

The lowest sensible universal integration is a layered design:

```text
Layer 0  OS process, PTY, process group, lifecycle
Layer 1  ordered terminal input/output and resize
Layer 2  terminal screen/snapshot reconstruction when needed
Layer 3  generic semantic event, action, permission, and command contracts
Layer 4  harness-specific adapters
Layer 5  official structured APIs, hooks, servers, headless modes, or ACP
```

Layers 0 and 1 are mandatory and remain the source-of-truth fallback. Layers 2
through 5 are progressive enhancements. A structured Layer 5 integration is
preferred when it represents the same session safely, but it must normalize
into Layer 3 and must not remove Terminal Mode.

## Common Ownership

Common code owns:

- PTY process and session lifecycle
- raw input/output, resize, interrupt, terminate, and terminal fallback
- event and action envelopes
- adapter identity and capability vocabulary
- typed approval/permission payloads
- event/action identity, version, and stale-action validation
- adapter-neutral action results
- command catalog transport
- capability-driven frontend composition

Common code must not match a harness command, parse a harness TUI, choose a
harness permission key, or invent adapter actions.

## Adapter Ownership

Adapters own:

- exact executable matching and discovery confidence
- prompt and command byte sequences
- TUI/screen parsing and metadata extraction
- native status vocabulary translation
- approval prompt recognition and operation context
- advertised approval choices and key/API mapping
- native slash commands and availability rules
- structured API, hook, server, or ACP integration
- adapter-specific action resolution and detail

## Action Flow

1. An adapter emits a typed semantic event containing adapter-provided actions.
2. The common API validates session, event ID, action ID, and action version.
3. The active adapter returns an adapter-neutral `ActionResult`.
4. The manager writes any returned terminal input and publishes normalized
   resolution/status events.
5. Missing detail receives an adapter-neutral fallback using the adapter name.

No action is automatic. Terminal-only decisions set generic
`blocks_prompt`/`requires_terminal` fields and advertise `open_terminal`.

## Command Flow

The slash palette combines common UI/session/terminal actions with the active
adapter command catalog. Common actions are filtered by capabilities.
Harness-native commands are never compiled into frontend source.

## Integration-Level Conclusion

The PTY is the correct universal base, not the only desirable integration.
Codex app-server, OpenCode server/OpenAPI, and Grok ACP or streaming JSON are
stronger semantic sources than TUI scraping. They should be adopted per adapter
when session ownership, authentication, cancellation, permissions, and
Terminal Mode coexistence are designed explicitly. The common architecture
must never depend on one of those harness-specific transports.

## Guardrail

If a future change needs a harness name, command sequence, parser rule, native
permission choice, or command list in common backend/frontend logic, stop and
request owner approval. Adding another harness-specific branch is not an
acceptable shortcut.

