# Codex Adapter Research

Date: 2026-07-26
Installed version: `codex-cli 0.145.0`
Disposable repository: `/tmp/harnessrelay-codex-adapter-research`

## Safety Setup

Research used a disposable Git repository containing only:

```text
# HarnessRelay Codex Adapter Research

Disposable test repo.
```

Codex ran with:

```bash
codex --no-alt-screen --sandbox read-only --ask-for-approval untrusted
```

No destructive action was approved. A harmless `touch SHOULD_NOT_EXIST`
request was denied, and the file was verified absent.

## Startup And Idle Screen

Observed startup content:

```text
OpenAI Codex (v0.145.0)
model: gpt-5.6-sol high
directory: /tmp/harnessrelay-codex-adapter-research
```

The idle composer uses `›`. The footer repeats model and working directory.
Startup may also emit MCP boot status such as:

```text
Booting MCP server: playwright
Starting MCP servers (0/3): ...
```

For a repository not already present in Codex trust configuration, startup may
first show:

```text
Do you trust the contents of this directory?
1. Yes, continue
2. No, quit
Press enter to continue
```

Chat prompts must not be accepted while this screen is active. Trust is a
workspace policy decision, so the adapter should detect it, show Open Terminal,
and require the user to decide in the raw TUI. HarnessRelay must not select Yes
automatically.

Codex emits dense cursor movement, erase, scroll-region, style, title, and
keyboard-protocol control sequences. Raw output is not line-oriented.

## Keyboard Protocol And Prompt Submission

Startup emitted:

```text
CSI > 7 u
```

This enables the Kitty keyboard protocol. In the observed PTY:

- text plus `\r` typed text but did not submit it
- text plus `\n` did not submit it
- `CSI 13 u` submitted the current prompt

The safe prompt:

```text
Reply with exactly READY. Do not use tools or edit files.
```

produced:

```text
Working (0s • esc to interrupt)
READY
```

The first Codex adapter should therefore send prompt text followed by
`\x1b[13u` when the TUI enabled enhanced keyboard mode. Plain carriage return
remains the generic fallback.

This finding is version-specific and must have deterministic fake-harness
coverage.

Interceptor validation found two additional reliability requirements:

- Keyboard protocol state must follow the latest `CSI > flags u`, `CSI = flags
  u`, or `CSI < u` transition rather than searching all retained scrollback.
- Prompt text and the Enter sequence must use separate serialized PTY writes.
  When both were appended to one write, Codex intermittently left the prompt in
  the composer. A bounded 100 ms delay between the writes was reliable in the
  installed Codex and deterministic fixture.

## Response And Status Behavior

Useful visible states include:

- startup/MCP boot
- idle composer
- `Working (... • esc to interrupt)`
- approval overlay
- final assistant text
- interrupted conversation
- shutdown

The final text is drawn into the same cursor-addressed surface as status and
input. Raw chunks can split words and repeat prior frames. Extracting arbitrary
assistant messages without a screen model is not reliable.

The initial implementation should:

- emit `starting`, `processing`, `waiting_for_approval`, and `idle`
- emit clean system messages
- suppress all Codex raw TUI chunks from Chat transcript projection
- leave the complete raw response in Terminal Mode

### Implementation follow-up

Browser validation found that omitting assistant extraction made Chat accept a
prompt without displaying its response. The follow-up implementation therefore
adds a session-scoped headless terminal model using the pinned
`github.com/gitpod-io/xterm-go` revision. It reconstructs cursor movement,
erasure, Unicode, wrapping, and redraw state before extracting the response
associated with the submitted prompt.

Extraction runs after the terminal quiet period and emits a stable per-turn
message ID. A later projection of the same turn replaces the earlier one rather
than creating another Chat bubble. Raw `terminal.output` remains excluded from
Chat and preserved in Terminal Mode.

## Approval Prompt

The harmless request produced:

```text
Would you like to run the following command?

Environment: local

$ touch SHOULD_NOT_EXIST

1. Yes, proceed (y)
2. Yes, and don't ask again for commands that start with ... (p)
3. No, and tell Codex what to do differently (esc)
```

The exact command and environment were visible. Pressing Escape denied the
request and interrupted the conversation:

```text
You canceled the request to run touch SHOULD_NOT_EXIST
Conversation interrupted
```

Decision:

- Detect this exact prompt family with high but version-scoped confidence.
- Extract the `$ <command>` line when available.
- Expose `deny` and `open_terminal`.
- Map deny to Escape only while the matching event is pending.
- Do not expose approve-once or persistent approval in this iteration.
- Never expose the persistent option by default.

## Interrupt And Exit

At idle, Ctrl+C shut down the TUI cleanly and printed resume information.
During work, Codex advertises Escape as the task interrupt key. HarnessRelay's
existing process-level interrupt remains available as the universal fallback.

## Noise Classification

Terminal-only output includes:

- alternate/inline screen redraw controls
- cursor show/hide and position reports
- keyboard protocol negotiation
- terminal title updates
- box drawing
- spinner frames and repeated partial words
- repeated-character artifacts such as `MMMMMMMM`
- duplicated model/directory/footer lines
- prompt echo and approval menu redraws

Codex sessions should not use raw chunks as assistant chat. The adapter should
emit one clean terminal-UI system event and semantic state changes instead.

## Structured Options Investigated

### `codex exec --json`

`codex exec --json` emits JSONL events and is stable for non-interactive runs.
It does not provide the same long-lived interactive TUI session that
HarnessRelay Terminal Mode controls, so it is not a drop-in source for this
adapter.

### App Server

`codex app-server` supports JSONL over stdio and an experimental WebSocket
transport. Generated schemas for 0.145.0 include:

- `thread/start`
- `turn/start`
- `turn/interrupt`
- agent message deltas
- command execution status/output
- `item/commandExecution/requestApproval`
- command, cwd, reason, available decisions, thread ID, turn ID, and item ID

This is the best structured long-term integration surface. Official
documentation describes it as the deep-integration interface for rich clients,
but the command and WebSocket surface remain experimental and may change.

It would also require HarnessRelay to coordinate an app-server process and a
remote Codex TUI instead of merely parsing the one PTY-owned process. That is
deferred to preserve the current architecture and raw fallback.

### Session Files, Hooks, And Telemetry

Codex persists local state under `CODEX_HOME` and supports hooks and optional
OpenTelemetry events. Reading another process's SQLite/session files is not a
stable live-control contract. Hooks and telemetry may help future observation,
but they are user configuration surfaces and are not required by the first
adapter.

## Chosen First Implementation

The first Codex adapter is backend-side and PTY-derived:

1. Match only an executable basename exactly equal to `codex`.
2. Expose Codex capabilities in session metadata.
3. Publish detected, metadata, status, terminal-noise, user-message, and
   approval events.
4. Block prompt submission until the adapter reaches `idle`.
5. Route workspace trust decisions to Terminal Mode without making a choice.
6. Submit prompts with the observed Kitty Enter sequence.
7. Deny only a currently pending command approval with Escape.
8. Extract assistant text only from the rendered terminal screen after output
   settles; retain Terminal Mode as the source of truth.
9. Preserve every raw PTY byte for Terminal Mode.

## Known Limitations

- Parser patterns are version-sensitive and English-language.
- Response extraction depends on the observed `›` prompt, `•` response, and
  model-footer layout.
- Responses are quiet-period messages rather than token-level streaming.
- Approve-once and persistent approval are intentionally unavailable.
- App-server integration remains a future architecture task.

## Sources

- Installed `codex --help`, `codex exec --help`, and generated app-server JSON
  schemas for 0.145.0.
- [Codex App Server](https://developers.openai.com/codex/app-server)
- [Codex command-line reference](https://developers.openai.com/codex/cli/reference)
- [Codex non-interactive mode](https://developers.openai.com/codex/noninteractive)
- [Codex approvals and sandboxing](https://developers.openai.com/codex/security)
