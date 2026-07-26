# Command Nomenclature Guidelines

Status: normative

This document is the source of truth for HarnessRelay command naming. Command
names are part of the product architecture: they communicate which binary owns
a responsibility, which resource is being addressed, and whether an operation
observes or changes state.

If a future task needs a new top-level command or a verb not approved here, it
must update this document first and explain why the existing taxonomy cannot
express the operation. The implementation and user documentation must follow
the approved terminology.

## Command Philosophy

HarnessRelay uses a small noun-and-verb vocabulary:

```text
<binary> <resource-group> <verb> [target] [options]
<binary> <approved-top-level-verb> [target] [options]
```

- Prefer a resource group when several operations act on the same kind of
  object.
- Prefer established developer-tool terms over HarnessRelay-specific synonyms.
- Give one public concept one canonical name.
- Keep compatibility aliases hidden from primary help and documentation unless
  users need them for migration.
- Use `--` before a user command when HarnessRelay options and child arguments
  could be ambiguous.
- Do not encode a harness name into common command structure.

## Binary Roles

### `harnessd`

`harnessd` is the daemon binary. It owns the API server, session manager, PTY
runtime, event stream, audit path, and dashboard.

Approved public commands:

```bash
harnessd serve
harnessd version
```

Do not add user workflows such as `harnessd codex`, `harnessd shims`, or
`harnessd attach`. User and operator workflows belong to `harnessctl`.

### `harnessctl`

`harnessctl` is the user/operator CLI. It creates and controls managed
sessions, attaches local terminals, and manages user-local shims.

## Naming Principles

### Nouns for resource groups

Use plural resource groups for collections and their lifecycle:

- `shims`
- `sessions`
- `adapters`
- `services`

Use singular nouns only for a deliberately internal namespace or when a
command acts on one named mechanism rather than a collection. The internal
shim runtime is therefore `harnessctl shim exec`; users manage the collection
with `harnessctl shims ...`.

### Verbs for actions

A verb must describe the user-visible effect, not the implementation technique.
Do not create synonyms merely because a new code path is different internally.

### Stable argument boundaries

Use:

```bash
harnessctl run --backend pty -- codex --model MODEL
harnessctl shim exec codex -- --model MODEL
```

The first `--` ends HarnessRelay option parsing. Everything after it belongs to
the launched harness and must be preserved byte-for-byte as process arguments.

## Resource Groups

### `shims`

Owns installation state, configuration, generated shim files, PATH diagnostics,
and regeneration:

```bash
harnessctl shims install <name>...
harnessctl shims uninstall <name>...
harnessctl shims uninstall-all
harnessctl shims list
harnessctl shims status
harnessctl shims doctor
harnessctl shims reshim
harnessctl shims path
```

### `sessions`

`sessions` denotes the session collection. The current
`harnessctl sessions` command lists sessions and remains canonical until a
separate grouped session command migration is designed. Do not add a competing
top-level `list` command. A future grouped form may be introduced only with an
explicit compatibility plan:

```bash
harnessctl sessions list
```

### `adapters`

Reserved for inspection and configuration of adapter resources. Adapter-native
commands continue to flow through the session command catalog; do not add a
top-level verb for each adapter.

### `services`

Owns installation and lifecycle of HarnessRelay user services:

```bash
harnessctl services install
harnessctl services uninstall
harnessctl services start
harnessctl services stop
harnessctl services restart
harnessctl services enable
harnessctl services disable
harnessctl services status
harnessctl services logs
```

The initial implementation manages only `harnessrelay.service`, a rootless
systemd user unit. `install` creates the owned unit but does not silently start
or enable it. `enable` controls login-time startup; `start` controls current
runtime state. Do not use a singular top-level `service` alias.

## Approved Verbs

| Verb | Meaning | Constraints |
| --- | --- | --- |
| `serve` | Run a long-lived service. | Daemon-only. |
| `run` | Explicitly start a HarnessRelay-managed command and attach locally when requested by the command contract. | Never means “resume” or “install”. |
| `attach` | Connect a local terminal or client to an existing session. | Must not kill the session on detach. |
| `detach` | Disconnect a client while leaving the session alive where the backend supports it. | Must state when a backend cannot preserve the session. |
| `list` | Enumerate resources or available targets. | Prefer inside a resource group. It does not diagnose health. |
| `status` | Inspect current state. | Read-only; concise; does not repair. |
| `doctor` | Diagnose configuration/runtime problems and suggest specific fixes. | Read-only by default; must not silently repair or edit profiles. |
| `install` | Create and configure a HarnessRelay-managed artifact. | Must not overwrite unmanaged artifacts without explicit `--force`. |
| `uninstall` | Remove selected artifacts that HarnessRelay owns. | Must preserve unmanaged artifacts and unrelated configuration. |
| `uninstall-all` | Remove every artifact of the current resource group that HarnessRelay owns. | Group-scoped, never machine-wide by implication. |
| `reshim` | Regenerate shim files from existing shim configuration. | Does not discover or install new harness binaries. |
| `path` | Print the canonical filesystem path for a resource. | Read-only and script-friendly. |
| `discover` | Scan external candidates that HarnessRelay does not own. | Does not create sessions or adopt processes. |
| `adopt` | Convert a technically attachable discovered candidate into a managed session. | Must not imply that arbitrary PTYs can be adopted. |
| `interrupt` | Request an in-session interruption, normally Ctrl+C semantics. | Does not terminate the session by definition. |
| `terminate` | Request graceful session termination. | Stronger than interrupt; weaker than force kill. |
| `kill` | Force termination after explicit confirmation. | Never a default or implicit fallback without the documented grace path. |
| `start` | Transition an already configured resource to running. | Do not use instead of `run` for a new harness command. |
| `stop` | Transition a long-lived service/resource to stopped. | Do not use as the canonical session termination verb. |
| `enable` | Turn on an existing configured feature. | Does not create installation artifacts. |
| `disable` | Turn off an existing configured feature without deleting it. | Does not uninstall. |
| `config` | Read or change configuration through a structured subcommand. | Avoid until a coherent config command group is designed. |
| `exec` | Internal dispatch that replaces the current process with a resolved command. | Use within a mechanism namespace, not as a random top-level command. |

`version` and `help` are conventional binary metadata commands and are exempt
from resource grouping.

## Discouraged Verbs and Names

Do not use vague, branded, or implementation-led names such as:

```text
magic
wire
hookup
condex
relayify
hijack
takeover
spin
proxything
proxy-run
```

Use `shim`/`shims`, not `wrapper`, `hook`, `intercept`, or `proxy` for the
installed command artifacts. “Proxy” may describe architecture in prose but is
not a public command family.

Avoid new top-level aliases. Existing compatibility aliases such as
`harnessctl list` for `sessions` and `harnessctl stop` for `terminate` are not
precedent for new names and should not appear in primary examples.

## Shim Command Group

Canonical public surface:

```bash
harnessctl shims install codex opencode grok
harnessctl shims install --all-known
harnessctl shims uninstall codex
harnessctl shims uninstall-all
harnessctl shims list
harnessctl shims status
harnessctl shims doctor
harnessctl shims reshim
harnessctl shims path
```

Canonical internal runtime:

```bash
harnessctl shim exec <shim-name> -- <harness-args...>
```

The singular internal namespace keeps generated files auditable and avoids a
vague top-level `proxy-run`. It may be documented for troubleshooting, but it
is not the normal user entry point.

## Discovery and Adoption Command Group

The approved future surface is:

```bash
harnessctl discover
harnessctl discover --refresh
harnessctl adopt <candidate-id>
```

`discover` observes. `adopt` changes ownership only when the external candidate
is technically attachable. Neither is a synonym for installing shims.

## Session Commands

Canonical current and planned concepts:

```bash
harnessctl run [options] -- <command> [args...]
harnessctl attach <session-id>
harnessctl interrupt <session-id>
harnessctl terminate <session-id>
harnessctl sessions
```

`detach` describes leaving an attached session alive. The existing attach-mode
key sequence is a detach operation even if no standalone command is required.

## Examples

Good:

```bash
harnessctl shims doctor
harnessctl shims reshim
harnessctl run --backend pty -- opencode
harnessctl attach ses_123
```

Rejected:

```bash
harnessctl fix-shims
harnessctl proxy-run opencode
harnessctl tmux-codex
harnessd adopt-shell
```

The rejected forms invent verbs, expose backend details as top-level concepts,
or put user workflows on the daemon binary.

## How to Add a Future Command

1. Identify the resource owner and binary.
2. Check whether an existing group and approved verb express the operation.
3. Define the state transition and whether the command is read-only.
4. Specify argument boundaries, output contract, failure behavior, and safety
   requirements.
5. Add the proposed name and rationale here before implementation if it needs a
   new top-level command, resource group, or verb.
6. Add help text, user documentation, and CLI tests together.
7. Preserve compatibility deliberately; do not add undocumented aliases.

## Review Checklist

- Does the command belong to `harnessctl`, not `harnessd`?
- Is the resource group a stable product noun?
- Is the verb already approved and used with its documented meaning?
- Is there exactly one canonical spelling?
- Are child-command arguments separated with `--`?
- Does help distinguish read-only inspection from mutation?
- Do destructive operations name their exact ownership scope?
- Does uninstall preserve unmanaged files?
- Does the name remain valid for Codex, OpenCode, Grok, and future harnesses?
- Are the guideline, help, tests, and docs updated before implementation?
