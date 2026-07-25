# HarnessRelay Interceptor

HarnessRelay Interceptor is a Go daemon for launching, supervising, and exposing terminal-based coding harnesses through a local web dashboard.

Stage 1 focuses on the local interceptor foundation: the daemon, common API, dashboard serving path, session architecture groundwork, PTY process control groundwork, and documentation-backed implementation plan. There is no mobile app in this stage.

## Current Foundation

- Go module: `github.com/harnessrelay/interceptor`
- `harnessd serve` daemon skeleton
- `harnessctl` CLI skeleton
- Local-only default bind address: `127.0.0.1:8765`
- Health endpoint: `GET /api/v1/health`
- Static placeholder dashboard served at `/`
- Standard-library structured logging with safe request/session field helpers
- Config defaults and tests

PTY runtime, sessions, WebSocket streaming, storage, authentication, harness adapters, and the React/Vite dashboard are intentionally deferred.

## Repository Map

- `cmd/harnessd`: daemon entry point and process lifecycle wiring
- `cmd/harnessctl`: CLI client entry point; session commands are placeholders for now
- `internal/api`: HTTP router, JSON helpers, health endpoint, and future REST/WebSocket handlers
- `internal/config`: defaults, config format constants, and future config loading
- `internal/logging`: structured logging setup and safe request/session ID field helpers
- `internal/session`: future session manager and lifecycle state
- `internal/pty`: future PTY process launch, input, resize, interrupt, and cleanup code
- `internal/terminal`: future terminal byte/history/rendering models
- `internal/harness`: future common harness adapter interfaces
- `internal/harness/generic`: future raw terminal fallback adapter
- `internal/events`: future internal event bus and event models
- `internal/storage`: future metadata, event, and audit persistence
- `internal/security`: future local auth and exposure controls
- `web`: static dashboard assets served by `harnessd`
- `testdata/fake-harnesses`: future fake harness programs/scripts for integration tests

Before implementing a todo item, read `Docs/Spec/Context.md`, update tests for behavior changes, and check off `Docs/Spec/Todo.md` only after verification. Do not add mobile app scope or bind publicly by default.

## Quick Start

```bash
go test ./...
go run ./cmd/harnessctl --help
go run ./cmd/harnessctl version
go run ./cmd/harnessd serve
```

Then open:

```text
http://127.0.0.1:8765/
http://127.0.0.1:8765/api/v1/health
```

The daemon binds to `127.0.0.1` by default. It does not listen on public interfaces unless a future explicit configuration feature enables that.

## Make Targets

```bash
make test
make build
make run
make fmt
```

## Documentation Map

- `Docs/Spec/Context.md`: project scope, architecture, and constraints
- `Docs/Spec/Todo.md`: active implementation checklist
- `Docs/Spec/Research/`: research notes behind the current plan
