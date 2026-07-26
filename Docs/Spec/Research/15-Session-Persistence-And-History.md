# Session Persistence And History Research

Date: 2026-07-26

## Current state

`internal/session.Manager` stores session objects in memory. Each session owns a
bounded four-MiB terminal ring buffer. Completed sessions remain in the manager
until the explicit cleanup API removes them, so their metadata and terminal
snapshot remain inspectable for the daemon lifetime.

`internal/events.Bus` stores up to 1024 events per session. The REST events
endpoint and WebSocket history replay both read this history. Semantic user,
assistant, system, status, metadata, and approval events therefore remain
available after process exit during the same daemon lifetime.

The dashboard hydrates semantic Chat Mode from the REST event history. Its
session-storage cache is only a reload convenience and is not the authority.
A new process creates a new session ID; it does not replace or clean up an old
finished session.

Nothing currently survives daemon restart:

- session metadata is lost
- terminal ring buffers are lost
- semantic event histories are lost
- origin/shim metadata is lost
- completed-session list entries are lost
- in-flight PTY processes die with the daemon

The in-memory audit log is also lost. No SQLite dependency or file-backed
session store exists yet; Phase 7.1 and 7.2 remain open in `Todo.md`.

## Dogfood failure mechanism

History retention exists, but a semantic assistant projection is produced by a
three-second idle callback. Session exit stops that timer without asking the
adapter for its final idle events. A short response followed by process exit
can therefore be visible in Terminal Mode but absent from semantic history.
Local terminal prompts are also not emitted as semantic user events, which can
make a finished Chat transcript appear empty even though the event bus itself
retained everything it received.

## Minimum fix

For this dogfood checkpoint:

- flush an adapter's final pending semantic projection before publishing
  `session.exited`
- retain all emitted semantic events for the completed session
- hydrate Chat Mode from event history when selecting a completed session
- keep old and new sessions distinct in the session list
- test history after completion and after creating a second session

## Restart persistence decision

Daemon-restart persistence is explicitly deferred to the existing SQLite
milestone rather than introducing a competing partial store. A durable design
needs one transaction boundary for session metadata, typed semantic events,
bounded terminal chunks, cleanup/retention, and migration. A JSONL-only event
log would still leave metadata and terminal history inconsistent after a
crash, while making the later SQLite migration more complex.

The UI and documentation must state that completed history is daemon-lifetime
history until SQLite persistence is implemented. This is an explicit
limitation, not an implication that restart persistence exists.

## Recommended durable design

Use the configured storage path for one SQLite database with:

- versioned schema migrations
- sessions table containing lifecycle and shim-origin metadata
- append-only typed events table keyed by session/sequence
- bounded terminal chunk table or compact terminal-log files referenced by the
  database
- startup recovery that marks formerly live daemon-owned PTYs as failed and
  non-attachable
- retention policy independent of frontend caches

Prompt bodies remain sensitive. Persistence must follow the existing audit and
redaction rules and document what transcript content is stored.

