# HarnessRelay Install QA

## INSTALL-001: User-local install lifecycle

Status: Automated
Area: install/update
Severity: Critical
Steps to reproduce: Run `scripts/install_test.sh`.
Expected: Both binaries, XDG directories, stable token, config, and ownership
manifest are created without root or profile edits.
Actual: Covered by the temporary-HOME lifecycle test.
Root cause: HarnessRelay previously had only repository-local build outputs.
Fix summary: Added rootless XDG-aware install and Make targets.
Regression test: `scripts/install_test.sh`
Verification commands: `make test`; temporary-HOME `make install`
Notes: The test uses fixture binaries and never touches the real home.

## INSTALL-002: Managed update and uninstall safety

Status: Automated
Area: update/uninstall
Severity: Critical
Steps to reproduce: Update an unchanged install, then modify one installed
binary and attempt uninstall.
Expected: Update preserves token/config; uninstall removes only unchanged
manifest-owned binaries and refuses the modified binary.
Actual: Covered by `scripts/install_test.sh`.
Root cause: Binary ownership was not previously recorded.
Fix summary: Added SHA-256 ownership records and atomic replacement.
Regression test: `scripts/install_test.sh`
Verification commands: `./scripts/install_test.sh`
Notes: Configuration and data remain by default; purge is explicit.

## INSTALL-003: Stable authentication precedence

Status: Automated
Area: config/security
Severity: Critical
Steps to reproduce: Load with a token file, then set `HARNESSRELAY_TOKEN`.
Expected: Daemon and CLI use the config token; environment overrides it.
Actual: Covered by config and CLI unit tests.
Root cause: Only the environment variable was previously read.
Fix summary: Added shared config-token resolution.
Regression test: `go test ./internal/config ./cmd/harnessctl`
Verification commands: `harnessctl status`
Notes: Token source is reported without printing the secret.

## INSTALL-004: Installed fake shim dogfooding

Status: Verified
Area: shims/integration
Severity: High
Steps to reproduce: Install to a temporary HOME, install a fake harness shim,
prepend the shim directory, and invoke the normal fake command.
Expected: The generated shim calls installed `harnessctl`, preserves arguments,
and safely uses direct bypass or daemon-owned PTY behavior.
Actual: Installed fake shim invoked a copied `/bin/echo` through the
daemon-owned PTY, printed the expected argument, and appeared in
`harnessctl sessions`.
Root cause: Repository-local binaries could not prove real install behavior.
Fix summary: Installer supplies a stable PATH binary for generated shims.
Regression test: CLI shim lifecycle and temporary-HOME dogfood command.
Verification commands: Temporary-HOME install; installed daemon from `/tmp`;
`fakeharness dogfood-installed-shim`; `harnessctl sessions`.
Notes: Codex resolution was also verified with safe bypass and
`codex --version`; the real Codex binary remained untouched.
