# Universal Harness Architecture QA

Date: 2026-07-26

## Environment

- Codex: `codex-cli 0.145.0`
- OpenCode: `1.18.5`
- Grok Build: `0.2.112`
- Browser: system Google Chrome through Playwright
- Disposable directories: `/tmp/harnessrelay-qa-codex`,
  `/tmp/harnessrelay-qa-opencode`, and `/tmp/harnessrelay-qa-grok`

No destructive operation was requested or approved. OpenCode and Grok were
launched only to validate selection and Terminal fallback; no prompts were
submitted. HarnessRelay did not enable auto/always approval.

## Automated Results

Passed:

```text
go test ./...
make test
make build
npm --prefix web run build
npm --prefix web run qa
node qa/dashboard-smoke.mjs
```

Full Playwright result: 17 passed.

`make build` completed both frontend and Go binaries. The Go command emitted a
non-fatal warning that its external module-download stat cache is read-only in
the managed environment. Vite emitted the existing chunk-size advisory.

## Validation Matrix

### Generic `/bin/bash`

- selected `generic`
- Chat prompt submission and terminal projection passed
- Terminal Mode input, resize, interrupt, and terminate passed
- slash palette contained common actions only
- no Codex command catalog or Chat labels appeared

### Fake Semantic

- selected `fake-semantic` only with the QA environment flag
- exposed semantic metadata and a dynamic `/fake-status` command
- narrower capabilities removed unsupported key/interrupt palette actions
- non-Codex action returned `confirmed` and Fake Semantic detail
- an empty-detail action used the neutral common fallback
- terminal-only blocking used `blocks_prompt` and `requires_terminal`
- no frontend fake-adapter branch or Codex text was required

### Codex

- selected `codex`
- Codex catalog appeared only for the Codex session
- harmless prompt returned `SEMANTIC_ADAPTER_OK`
- semantic assistant text contained no raw TUI artifacts
- Terminal Mode remained available
- lifecycle cleanup passed in the disposable repository
- no destructive approval was performed

### OpenCode

- installed version launched in `/tmp/harnessrelay-qa-opencode`
- selected `generic`; it was not treated as Codex
- Terminal Mode rendered and remained usable
- Chat slash palette did not contain Codex commands
- session terminated without sending a prompt or granting permission

### Grok Build

- installed version launched in `/tmp/harnessrelay-qa-grok`
- selected `generic`; it was not treated as Codex
- Terminal Mode rendered and remained usable
- Chat slash palette did not contain Codex commands
- session terminated without sending a prompt or granting permission

## Guardrail Coverage

Tests fail on:

- untyped Generic approval payloads
- Codex action text in the fake adapter flow
- non-neutral empty action detail
- adapter-specific terminal-only branches
- stale or replayed actions
- Generic/fake command catalog leakage
- capability-insensitive slash actions
- Codex labels in Generic/fake Chat surfaces

