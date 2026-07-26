# Command Shims QA

Date: 2026-07-26

Automated tests use temporary config/data/PATH directories and fake harness
binaries. They do not edit a real shell profile.

## SHIM-001: Real binary lookup recurses into generated shim

Status: verified
Area: resolution/runtime safety
Severity: critical
Steps to reproduce: put the shim directory first on PATH and resolve the
shimmed command.
Expected: resolution excludes the shim directory and rejects any
HarnessRelay-generated candidate stored as the real binary.
Actual: install finds the first executable outside the shim directory; runtime
validates canonical paths and ownership markers.
Root cause: naive `exec.LookPath` would return the active shim.
Fix summary: added ordered PATH scanning with shim-directory exclusion,
canonical comparison, and managed-shim rejection.
Regression test: `TestResolveRealBinaryPreventsRecursion`.
Verification commands: `go test ./internal/shims`.
Notes: direct execution always uses the stored absolute path.

## SHIM-002: Install or uninstall can damage an unmanaged command

Status: verified
Area: filesystem ownership
Severity: critical
Steps to reproduce: place an unrelated executable at a requested shim
destination, then install or uninstall that name.
Expected: default install refuses replacement; uninstall never deletes it.
Actual: both operations fail with an ownership error and preserve the file.
Root cause: generated command interception shares a namespace with other tools.
Fix summary: added an exact ownership marker, per-destination checks, and
atomic generated-file replacement.
Regression test: `TestInstallRefusesUnmanagedFileWithoutForce` and
`TestUninstallPreservesUnmanagedFile`.
Verification commands: `go test ./internal/shims`.
Notes: `--force` applies only to explicit install replacement.

## SHIM-003: Bypass loses normal process behavior

Status: verified
Area: runtime fallback
Severity: high
Steps to reproduce: execute a fake harness through
`HARNESSRELAY_BYPASS=1`.
Expected: args, cwd, environment, output, and exit code match direct execution.
Actual: the shim process is replaced with the stored absolute executable; the
fake returned its expected output and exit code 7.
Root cause: a subprocess wrapper can accidentally alter signals or exit status.
Fix summary: direct/bypass uses process replacement.
Regression test: `TestShimExecBypassPreservesArgsCwdEnvAndExitCode`.
Verification commands: `go test ./cmd/harnessctl`.
Notes: no relay session is created in bypass.

## SHIM-004: Daemon outage makes the real harness unavailable

Status: verified
Area: availability fallback
Severity: high
Steps to reproduce: configure a PTY shim while the daemon address is
unreachable.
Expected: warn clearly and execute the real binary directly when configured
fallback is `direct`.
Actual: stderr explains that no relay session will be created, then direct
execution succeeds.
Root cause: transparent command interception must not turn a relay outage into
a harness outage.
Fix summary: added health check and configured direct fallback.
Regression test: `TestShimExecFallsBackToDirectWhenDaemonUnavailable`.
Verification commands: `go test ./cmd/harnessctl`.
Notes: the default config uses direct fallback.

## SHIM-005: Shim-launched origin is invisible to clients

Status: verified
Area: session API/UI
Severity: medium
Steps to reproduce: create a managed session with shim origin metadata.
Expected: API exposes generic origin/backend/shim/real-binary/attachable fields;
the session rail, header, and inspector show quiet shim context; Terminal Mode
remains available.
Actual: API and UI expose those fields without adapter-specific branches.
Root cause: existing session metadata described command and adapter but not
launch origin.
Fix summary: extended common session metadata/event/DTO and added minimal UI
badges/inspector rows.
Regression test: `TestShimSessionOriginMetadata` and Playwright
`Shim session origin is visible without obscuring Terminal fallback`.
Verification commands: `go test ./internal/api`; `npm --prefix web run qa`.
Notes: direct fallback intentionally has no session metadata because it creates
no session.

## SHIM-006: Fast shim command output is missing from local attachment

Status: verified
Area: CLI attach lifecycle
Severity: high
Steps to reproduce: run a generated shim for a short-lived command such as
`echo` with stdout/stdin connected through a non-TTY runner.
Expected: final PTY output is printed and the real exit status is returned.
Actual: the corrected path prints `shim-managed-pty-ok arg-two` and exits zero.
Root cause: two races existed: the first snapshot could precede final PTY
output, and local stdin EOF could end attachment before WebSocket output.
Fix summary: completed sessions append the final snapshot delta; stdin EOF now
ends only input forwarding while output waits for session exit.
Regression test: `TestAttachReplaysFinalSnapshotForFastExitedSession`; generated
`echo` shim dogfood through a disposable daemon.
Verification commands: `go test ./cmd/harnessctl`; isolated `/tmp` generated
shim invocation.
Notes: interactive Ctrl-] detach behavior remains unchanged.

## Verification Matrix

- config read/write and malformed config: automated
- safe install/generated content: automated
- status/PATH order: automated
- doctor diagnostics: CLI behavior and full QA
- reshim: automated CLI lifecycle
- selected uninstall/uninstall-all: package/CLI lifecycle
- args/cwd/environment/exit code: direct subprocess and create-request tests
- bypass: automated
- daemon fallback: automated
- API origin metadata: automated
- UI origin badge/inspector/Terminal fallback: Playwright
- real Codex/OpenCode/Grok resolution: final safe installed-tool validation

## Final Verification

Results:

- `go test ./...`: passed, 149 tests across 16 packages.
- `make test`: passed.
- `make build`: passed; dashboard plus `harnessd` and `harnessctl` built.
- `npm --prefix web run build`: passed with the existing non-fatal Vite
  chunk-size advisory.
- `npm --prefix web run qa`: passed, 18 Playwright tests.
- `node qa/dashboard-smoke.mjs`: passed against disposable local daemon/Chrome
  processes.
- Generated `echo` shim bypass: printed `shim-bypass-ok arg-two`.
- Selected uninstall restored PATH fall-through:
  `uninstall-fallthrough-ok`.
- Generated managed PTY shim: printed `shim-managed-pty-ok arg-two`, returned
  exit zero, and appeared in the API with Shim/PTY/real-binary/attachable
  metadata.
- Installed target resolution:
  - Codex `codex-cli 0.145.0`
  - OpenCode `1.18.5`
  - Grok `0.2.112`
- `shims doctor` passed config, ownership, executable, and PATH checks for all
  isolated entries; its deliberately unavailable-daemon check reported the
  configured direct fallback.

All shim installation/validation used
`/tmp/harnessrelay-shims-qa.a8vaDh`. No real shell profile or normal user shim
directory was modified.
