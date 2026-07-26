# Cross-Harness Capability Research

Date: 2026-07-26

Local versions:

- Codex: `codex-cli 0.145.0`
- OpenCode: `1.18.5`
- Grok Build: `0.2.112`

## Codex Research

Official sources:

- [CLI reference](https://learn.chatgpt.com/docs/developer-commands?surface=cli)
- [Security, sandboxing, and approvals](https://learn.chatgpt.com/docs/security)
- [App Server](https://learn.chatgpt.com/docs/app-server)

CLI behavior:
Codex provides an interactive terminal UI, non-interactive `codex exec`, and
an app-server integration surface. Local PTY research for 0.145.0 is recorded
in `09-Codex-Adapter-Research.md`; prompt submission uses the negotiated Kitty
keyboard protocol and raw line parsing is insufficient for screen content.

Approval/permission behavior:
Approval policy and sandbox policy are separate. The observed TUI has
workspace-trust and command-execution decisions. HarnessRelay exposes only
verified actions and never persistent approval by default.

Command/action behavior:
Native slash commands and approval key mappings are version-specific and
adapter-owned. Command approvals can carry command, cwd, reason, and decisions.

Structured integration options:
`codex exec --json` serves one-shot automation. Codex app-server exposes
threads, turns, messages, command execution, and structured approval requests.

What is common:
Session lifecycle, raw terminal, prompt contract, operation context, action
identity/version, resolution, terminal fallback, and command catalog transport.

What is adapter-specific:
Kitty Enter, screen parsing, workspace-trust recognition, Codex decision IDs,
slash commands, and app-server protocol mapping.

Risks:
TUI and command catalogs are version/language sensitive. App-server adoption
changes process/session coordination and requires a separate design checkpoint.

## OpenCode Research

Official sources:

- [CLI](https://opencode.ai/docs/cli/)
- [Permissions](https://opencode.ai/docs/permissions)
- [Commands](https://opencode.ai/docs/commands/)
- [Server](https://opencode.ai/docs/server/)

CLI behavior:
`opencode` starts the TUI. `opencode run` is non-interactive and supports
`--format json`, session continuation, server attachment, and `--auto`.
Sessions can be exported/imported as JSON.

Permission model:
Rules resolve to `allow`, `ask`, or `deny`; most permissions default to allow,
while external-directory and repeated-loop guards default to ask. Tool and
path/command patterns are configurable.

Approval choices:
An ask prompt offers `once`, `always` for suggested patterns during the current
session, and `reject`. HarnessRelay must advertise only choices the active
OpenCode adapter can bind reliably.

Command/action behavior:
The TUI has built-in and project/user-defined slash commands. Because custom
commands may override built-ins, a future catalog must be discovered from the
active OpenCode configuration/server rather than hardcoded.

Structured integration options:
The TUI is a client of an internal HTTP server. `opencode serve` exposes
OpenAPI 3.1, SSE events, session/message APIs, a permission response endpoint,
and `/tui` controls. The server defaults to `127.0.0.1`; password protection is
optional and must be enabled when HarnessRelay coordinates it.

What is common:
Operation/tool context, allow/deny request presentation, dynamic actions,
event-bound validation, commands, prompt submission, and Terminal Mode.

What is adapter-specific:
OpenCode permission IDs/responses, `once`/`always` mapping, server discovery and
auth, custom command discovery, and TUI key behavior.

Risks:
`--auto` changes ask behavior and must never be enabled by HarnessRelay by
default. A standalone server is a different process from an existing TUI unless
explicitly coordinated.

## Grok Build Research

Official sources:

- [Grok Build overview](https://docs.x.ai/build/overview)
- [Modes and commands](https://docs.x.ai/build/modes-and-commands)
- [Permissions](https://docs.x.ai/build/features/permissions)
- [Headless and scripting](https://docs.x.ai/build/cli/headless-scripting)
- [Official source repository](https://github.com/xai-org/grok-build)

CLI behavior:
`grok` starts a full-screen, mouse-aware TUI. First launch uses browser
authentication; `grok login --device-auth` and `XAI_API_KEY` support headless
environments. Headless mode accepts `-p`, named/resumed sessions, `--cwd`,
`--no-alt-screen`, and plain, JSON, or streaming-JSON output.

Permission/approval behavior:
Ask is the default. Auto uses a classifier and may still prompt for dangerous
tools. Always-approve skips prompts except deny rules/hooks. Permission rules
cover Bash, Edit, Read, Grep, MCP, and web tools; deny wins over allow.
Permissions and the filesystem/network sandbox are separate.

Command/action behavior:
The TUI has a dynamic command palette, native slash commands, conditional
commands, and user-invocable skills as commands. Enter submits; Escape/Ctrl+C
cancel; keyboard behavior varies with terminal protocol.

Structured integration options:
Streaming JSON provides incremental headless events. `grok agent stdio`
provides ACP over JSON-RPC with session updates. These are promising adapter
sources and stronger than TUI scraping for semantic data.

What is common:
PTY/session fallback, prompt/action transport, permission operation context,
dynamic commands/actions, stale validation, and terminal-only decisions.

What is adapter-specific:
Ask/Auto/Always modes, classifier semantics, Grok rules, TUI keys/commands,
ACP mapping, auth/device-code presentation, and JSON event mapping.

Risks:
HarnessRelay must never enable `--always-approve` or `--auto`. Auth files and
API keys are sensitive. ACP/headless sessions may not be identical to an
interactive TUI session, so Terminal Mode coexistence needs explicit design.

## Capability Matrix

| Capability | Codex | OpenCode | Grok | Generic |
| --- | --- | --- | --- | --- |
| Interactive PTY/TUI | yes | yes | yes | yes |
| Headless structured output | JSONL exec | JSON run | JSON/streaming JSON | no |
| Structured long-lived API | app-server | HTTP/OpenAPI/SSE | ACP stdio | no |
| Permission requests | yes | yes | yes | heuristic only |
| Dynamic native commands | yes | built-in + custom | native + skills | no |
| Universal raw fallback | required | required | required | native path |

