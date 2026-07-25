# Web Dashboard

## Recommendation

Use Vite + React + TypeScript for the first web dashboard, with xterm.js as the terminal component. The Go daemon should serve the production build as embedded static assets and provide a dev configuration that proxies API/WebSocket calls to `harnessd`.

## Reasoning

- The dashboard is a stateful app: sessions, live WebSocket events, terminal instance lifecycle, action cards, and reconnect state.
- React + TypeScript is widely understood and reduces handoff risk for smaller agents.
- Vite has simple development and production build behavior and outputs static assets suitable for embedding.
- Go `embed.FS` supports shipping static files inside a single daemon binary.

## Alternatives Considered

| Alternative | Decision | Reason |
| --- | --- | --- |
| Plain HTML/TypeScript | Reject for Stage 1 dashboard | Fewer dependencies, but state management around xterm/session events becomes ad hoc quickly. |
| React + TypeScript + Vite | Choose | Practical balance of familiarity, tooling, and UI state structure. |
| Svelte | Defer | Good fit technically, but fewer agents are likely to share the same conventions. |
| Server-rendered Go templates | Reject | Live terminal and WebSocket UI are client-heavy. |

## Static Asset Strategy

Recommended layout for later phases:

```text
web/
  package.json
  vite.config.ts
  index.html
  src/

internal/webassets/
  embed.go
```

Production:

1. Run `npm run build` in `web/`.
2. Vite writes `web/dist`.
3. Go embeds `web/dist` with `//go:embed`.
4. `harnessd serve` serves API under `/api/` and dashboard assets for all non-API paths.

Development:

- Run `harnessd serve --dev-cors-origin http://127.0.0.1:5173`.
- Run `npm run dev` in `web/`.
- Vite proxies `/api` and `/api/v1/ws` to the daemon, or the frontend reads `VITE_HARNESS_API_BASE`.

Do not require Node.js to run the production daemon.

## Minimum Dashboard Screens

### Session List

Display:

- session name
- status
- command
- cwd
- adapter ID
- started/last activity
- quick open/terminate controls

### Create Session

Fields:

- name optional
- command required
- args optional as shell-like text or repeated fields
- cwd required/default current configured workspace
- harness type optional/default `generic`
- terminal rows/cols optional/default browser fit

### Active Session View

Required panels:

- xterm.js terminal area
- session header with command, cwd, status, adapter ID
- controls: interrupt, terminate, force kill behind confirmation
- semantic event/action panel
- event history list

### Reconnect View

Behavior:

- fetch session metadata
- fetch snapshot/history chunks
- replay into terminal
- connect WebSocket with `after_seq`
- show stale/offline state if reconnect fails

## Terminal Component Guidance

- Create one xterm.js `Terminal` per active session view.
- Load `FitAddon`.
- Dispose terminal and subscriptions on unmount.
- Send `onData` bytes to `/input`.
- Use `ResizeObserver` and debounce resize.
- Keep terminal focus behavior explicit: clicking terminal focuses it; action buttons should not steal focus permanently.
- Do not use `innerHTML` for terminal/event content.

## UI Security Requirements

- Include CSRF header on state-changing REST requests.
- Use same-origin API URLs in production.
- Do not store auth tokens in localStorage unless there is no safer option.
- Do not display untrusted event text as HTML.
- Confirmation is required for terminate and stronger confirmation for force kill.
- Approval cards must show session command and cwd context.

## Manual Testing Flow

1. Start daemon on localhost.
2. Open dashboard.
3. Authenticate.
4. Create `/bin/bash` session.
5. Run `echo hello`.
6. Verify output appears in terminal.
7. Type command through browser.
8. Paste multiline input.
9. Resize browser window and run `stty size`.
10. Interrupt `sleep 30`.
11. Terminate session.
12. Reconnect browser and verify history/snapshot behavior.
13. Run fake approval harness and verify action card plus raw terminal fallback.

## Risks And Limitations

- xterm.js lifecycle bugs can leak listeners if components remount frequently.
- Vite dev proxy and production same-origin behavior must both be tested.
- Browser terminal focus/paste can be surprising; provide a raw input fallback in Phase 10.
- The first dashboard should be functional, not a full design system.

## Acceptance Criteria For Later Implementation

- Production daemon serves embedded dashboard without Node.js.
- Development workflow supports Vite dev server and daemon API.
- Dashboard can create and open sessions.
- xterm.js renders raw output and sends input.
- Resize reaches backend.
- Interrupt/terminate controls call correct APIs.
- Semantic event cards are rendered from backend-provided actions.
- Unknown harness sessions remain usable through raw terminal.

## Required Tests

- Frontend unit test for API client request shapes.
- Component test for action card rendering from backend event data.
- Browser/manual test for xterm input/output.
- Browser/manual test for resize.
- Browser/manual test for reconnect replay.
- Static serving test verifies `/api` routes are not swallowed by dashboard fallback.

## Sources

- [Vite production build docs](https://vite.dev/guide/build)
- [Vite getting started docs](https://vite.dev/guide/)
- [Go embed docs](https://go.dev/pkg/embed/)
- [xterm.js addon guide](https://xtermjs.org/docs/guides/using-addons/)
