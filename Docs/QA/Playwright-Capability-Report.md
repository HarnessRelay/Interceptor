# Playwright Capability Report

Date: 2026-07-26
Tool used: `@playwright/test` 1.62.0 from `web/node_modules`
Is real Playwright available: yes
Browser used: system Google Chrome at `/usr/bin/google-chrome`
Can fill text: yes
Can type keys: yes
Can click: yes
Can read DOM: yes
Can take screenshots: yes
Can capture console errors: yes
Can handle dialogs: yes
Limitations:
- The managed sandbox blocks Go cache writes, so the full QA command required escalation in this environment.
- Playwright's bundled Chromium is not installed; the config launches system Google Chrome instead.
- The current Playwright pass covers QA-001 and Screens 1-9 from the QA objective at the desktop viewport used by the suite.
Decision:
Real Playwright is available and is the required browser QA path. CDP-only smoke remains legacy coverage and is not sufficient by itself for QA pass/fail decisions.

Verified capabilities:
- launch Chromium-family browser: yes, via Playwright launching `/usr/bin/google-chrome`
- navigate to the app: yes
- click elements: yes
- fill text inputs: yes
- press keys/type into controls: yes
- type/paste into xterm-backed raw input fallback: yes
- type into xterm.js directly: yes
- paste/insert text into xterm.js directly: yes
- read DOM text: yes
- take screenshots: yes, under `qa/artifacts/screenshots/`
- inspect console errors: yes, via Playwright `console` and `pageerror` listeners
- inspect network errors: yes for tested request paths, through Playwright failures and API assertions
- wait for selectors: yes
- reload page: yes
- handle confirm dialogs: yes
- compare screenshots or inspect screenshot content: screenshots were captured and manually inspected for every required screen artifact

Verification command:

```bash
HARNESSRELAY_TOKEN=dashboard-token npm --prefix web run qa
```
