# Agent Operating Instructions — HarnessRelay Interceptor

## For AI Agents Working on This Repository

### 1. Read This File First

Every agent pass must start by reading this file. It contains the operational contract for how agents interact with this codebase, its documentation, and the knowledgebase.

### 2. Knowledgebase-First Workflow

This project maintains a **live knowledgebase mirror** in the Obsidian vault under the `HarnessRelay/` directory. The vault is queryable and writable via the `knowledgebase` MCP tools.

**Rule:** Before diving into source code, query the knowledgebase for context. It contains:

- `README.md` — project overview and quick start
- `PRODUCT.md` — product positioning and design principles
- `Docs/Developer.md` — project structure, commands, fake harnesses
- `Docs/API.md` — REST and WebSocket API reference
- `Docs/Install.md` — installation, update, uninstall
- `Docs/Shims.md` — transparent CLI shim behavior
- `Docs/Semantic-Adapters.md` — adapter contracts and extension guide
- `Docs/Architecture/Command-Nomenclature.md` — normative command naming
- `Docs/Spec/Context.md` — stable project memory and guardrails
- `Docs/Spec/Todo.md` — active implementation checklist
- `Docs/Spec/Research/*.md` — 17 research documents covering PTY, rendering, API, security, harnesses, testing, adapters, shims, services, persistence, terminal safety

**How to query the knowledgebase:**

```
knowledgebase_search_simple — full-text search with relevance scoring
knowledgebase_search_query — JsonLogic metadata search
knowledgebase_tag_list — list tags across all notes
knowledgebase_vault_list — browse directory structure
knowledgebase_vault_read — read a specific note
```

**Prefer the knowledgebase for:**
- Understanding project architecture and design decisions
- Looking up API contracts and event schemas
- Finding research on terminal behavior, PTY management, security
- Checking the Todo checklist before starting work
- Reading adapter extension guides

**Keep the repo `Docs/` as the canonical source.** When you update documentation, update both the repo files and the knowledgebase mirror. The repo files are version-controlled; the vault is the queryable second brain.

### 3. Context.md Is Sacred

`Docs/Spec/Context.md` is the stable project memory. **Do not edit it without explicit owner approval.** If you identify a needed change, propose it separately with:

- exact section to update
- proposed replacement or addition
- reason the update is needed
- whether it changes scope, architecture, security, or assumptions

### 4. Todo.md Is the Active Plan

`Docs/Spec/Todo.md` is the execution checklist. You may update task checkboxes as work is completed. You may add new tasks only if they follow the template at the bottom of the file. Do not silently change completed task history or delete unfinished tasks.

**Always read Todo.md before selecting work.** Work from top to bottom unless the project owner gives a different priority.

### 5. Hard Guardrails

These are non-negotiable. Violating them requires explicit owner approval:

- **Do not add mobile-app scope.**
- **Do not weaken security defaults.**
- **Do not remove raw terminal fallback.**
- **Do not hardcode one harness into the common API.**
- **Do not auto-approve harness actions.**
- **Do not bind publicly by default.**
- **Do not run as root.**
- **Do not edit Context.md without approval.**

### 6. Documentation Maintenance Rule

If your changes affect any behavior documented in:

- `README.md`
- `Docs/*.md`
- `Docs/Spec/*.md`

You must update both:
1. The in-repo file (canonical, version-controlled)
2. The knowledgebase mirror under `HarnessRelay/` (queryable second brain)

Failure to keep them in sync degrades the graph view and future agent context.

### 7. Test Before Declaring Done

Before marking a task complete:

```bash
make build
make test
```

If frontend changes are involved:

```bash
npm --prefix web run build
npm --prefix web run qa
npm --prefix web run qa:a11y
```

For install/service changes:

```bash
./scripts/install_test.sh
```

### 8. Agent Pass Lifecycle

A typical agent pass should follow this order:

1. **Read** `AGENTS.md` (this file)
2. **Query** the knowledgebase for relevant context
3. **Read** `Docs/Spec/Context.md` and `Docs/Spec/Todo.md`
4. **Identify** the specific task to work on
5. **Implement** with minimal, testable changes
6. **Update** tests and documentation (repo + vault)
7. **Run** build and test gates
8. **Update** Todo.md checkboxes

### 9. Knowledgebase Mirror Structure

```
HarnessRelay/
├── README.md
├── PRODUCT.md
└── Docs/
    ├── API.md
    ├── Developer.md
    ├── Install.md
    ├── Semantic-Adapters.md
    ├── Shims.md
    ├── Architecture/
    │   └── Command-Nomenclature.md
    └── Spec/
        ├── Context.md
        ├── Todo.md
        └── Research/
            ├── 00-Summary.md
            ├── 01-PTY-And-Process-Management.md
            ├── 02-Terminal-Rendering.md
            ├── 03-API-And-Events.md
            ├── 04-Web-Dashboard.md
            ├── 05-Security.md
            ├── 06-Harness-Research.md
            ├── 07-Testing-Strategy.md
            ├── 08-Semantic-Adapter-Architecture.md
            ├── 09-Codex-Adapter-Research.md
            ├── 10-Universal-Harness-Architecture.md
            ├── 11-Cross-Harness-Capability-Research.md
            ├── 12-Permission-Approval-Model.md
            ├── 13-CLI-Shim-Proxy-Mode.md
            ├── 14-Daemon-Service-And-Shim-Safety.md
            ├── 15-Session-Persistence-And-History.md
            ├── 16-Terminal-Bridge-Failure-Modes.md
            └── 17-Terminal-Protocol-Cleanup.md
```

All vault notes use Obsidian wiki-links (`[[Note Name]]`) so the graph view shows relationships. When adding new documentation, include links to related notes.
