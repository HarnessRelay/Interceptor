# HarnessRelay UI Revamp Plan

## Product goals

HarnessRelay should feel like a dependable local control plane rather than a
terminal embedded in a browser. Chat Mode is the default semantic experience;
Terminal Mode remains the unfiltered source of truth. The redesign must make it
easy to start, recognize, switch, monitor, and safely stop several sessions
without exposing implementation noise.

## Design inspiration

- Termius: calm, rectangular session cards with immediate identity and status.
- ChatGPT Web: centered readable transcript, low-clutter conversation chrome,
  and a stable bottom composer.
- JetBrains IDEs: a restrained workbench, strong mode/navigation affordances,
  and advanced detail behind deliberate menus and drawers.

The result uses HarnessRelay's own dark navy identity, controlled cyan/teal
accents, compact developer-tool typography, and a denser workbench rhythm. It
does not copy any source product.

## Information architecture

```text
App shell
├── Session rail
│   ├── Product identity
│   ├── Search and filters
│   ├── Session cards grouped by lifecycle
│   └── New session action
└── Active workspace
    ├── Session header
    │   ├── Identity and metadata
    │   ├── Chat / Terminal modes
    │   └── Interrupt + More actions
    ├── Chat or Terminal canvas
    └── Inspector drawer (closed by default)
```

## Screen inventory

1. Login/auth entry.
2. Authenticated empty state.
3. Session list with running, exited, and failed groups.
4. New-session dialog with progressive advanced options.
5. Active Chat Mode, including system and approval states.
6. Active Terminal Mode.
7. Slash/action menu.
8. More menu and destructive confirmation.
9. Session inspector drawer.
10. Reloaded and multi-session states.

## Layout system

- Desktop workbench: 320px session rail and a fluid workspace.
- Workspace header is fixed in the layout; the active canvas owns scrolling.
- Chat content is centered at a readable maximum width while status and
  composer controls remain full-context.
- Terminal fills the available canvas and never depends on page scroll.
- At laptop widths the session rail narrows; below 900px it becomes a compact
  top region. Mobile-app navigation is intentionally out of scope.
- Spacing follows a 4px base scale. Component radii top out at 14px.

## Navigation model

- Session cards are native buttons and expose `aria-current`.
- Chat/Terminal is a two-option tab interface with arrow-key behavior.
- New Session opens a labelled modal dialog; focus returns to the trigger.
- More and slash menus use menu semantics, roving focus, Escape close, and
  focus restoration.
- Inspector is a complementary drawer and may be closed with Escape.

## Component inventory

- AppShell, GlobalSidebar, SessionCard, SessionList
- SessionCreateDialog, SessionHeader, ModeSwitcher
- ChatView, ChatTranscript, ChatMessage, SystemStatusCard, ApprovalCard
- ChatComposer, SlashCommandMenu
- TerminalView, TerminalPanel, RawInputFallback
- InspectorDrawer, EventList
- Button, Dialog, Menu, ConfirmDialog, Toast/ErrorBanner
- StatusBadge, AdapterBadge

## Theme tokens

Tokens live in `web/src/theme/tokens.css` and cover:

- root, sidebar, canvas, surface, elevated, selected, and danger backgrounds
- primary, secondary, tertiary, inverse, success, warning, danger text
- neutral, strong, focus, semantic state, and adapter borders
- teal/cyan brand accents and semantic green/yellow/red
- 4px spacing scale, three radii, typography, shadows, motion, and z-index

Accent is reserved for primary actions, focus, selection, and live state. The
palette is checked against WCAG AA contrast for normal text.

## Accessibility goals

- WCAG 2.2 AA-oriented color contrast and visible focus.
- All controls named without relying on icon glyphs.
- Status is communicated by text and shape as well as color.
- Native forms, buttons, details, and dialog behavior where practical.
- Dialog focus trap, initial focus, Escape close, and trigger restoration.
- Menu arrow-key navigation and Escape behavior.
- `aria-live` for connection/session notices without announcing raw streams.
- Reduced-motion support and no content gated by animation.

## Keyboard interaction model

| Surface | Keys |
| --- | --- |
| Login | Tab through token and submit; Enter submits |
| Session cards | Tab to focus; Enter/Space selects |
| Mode switch | Left/Right moves and selects; Home/End supported |
| Composer | Enter sends; Shift+Enter inserts newline |
| Slash/More menu | Up/Down, Home/End, Enter/Space; Escape closes |
| Dialogs | Tab/Shift+Tab trapped; Escape cancels |
| Inspector | Escape closes and restores focus |
| Confirm dialog | Tab between cancel/confirm; Escape cancels |

## Responsive desktop behavior

- 1440px+: 328px rail, generous conversation gutters.
- 1024–1439px: 292px rail and compact metadata.
- 900–1023px: stacked rail/workspace with horizontally wrapping session cards.
- Short viewports keep the composer visible by confining scroll to transcript.
- Long commands, paths, messages, and event data wrap or truncate with titles;
  they never create horizontal page scrolling.

## Risk areas

- xterm measurement after layout changes and drawer open/close.
- WebSocket event duplication across reload and mode switches.
- Generic terminal output accidentally appearing as chat.
- Codex readiness and quiet-period semantic projection.
- Focus restoration across nested menus, confirmations, and drawer.
- Browser screenshots becoming timing-sensitive with live PTYs.
- Existing CDP smoke selectors changing with the new creation flow.

## Implementation checkpoints

1. Preserve and document the semantic/terminal baseline.
2. Introduce complete theme tokens and shared accessibility primitives.
3. Rebuild login and app shell.
4. Replace launcher/form with session cards and a New Session dialog.
5. Rebuild the session header and move dangerous actions under More.
6. Refine Chat Mode, composer, approval cards, and slash menu.
7. Refine Terminal Mode and collapse raw input fallback.
8. Replace footer debug panel with an inspector drawer.
9. Add keyboard, contrast, and automated accessibility checks.
10. Capture/review screenshots, track issues, and rerun the full regression set.

## QA strategy

- Preserve backend Go unit/integration tests and existing semantic adapter
  coverage.
- Extend Playwright around roles and visible product behavior rather than CSS
  implementation details.
- Add deterministic login, empty, create, card, chat, terminal, menu, inspector,
  reload, multi-session, and accessibility scenarios.
- Keep real Codex as a conditional disposable-directory smoke.
- Capture the eleven named revamp screenshots at 1440×960.
- Review every screenshot for hierarchy, spacing, contrast, clipping, terminal
  integrity, and unwanted debug/TUI noise.
- Log issues as UIR-### and only mark them verified after their regression and
  all earlier issue regressions pass.
