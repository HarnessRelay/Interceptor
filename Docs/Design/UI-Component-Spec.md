# HarnessRelay UI Component Specification

## Component: AppShell

Purpose: Own the two-region product workbench and global announcements.
Props/data needed: auth state, sessions, active session, current mode, errors.
States: loading, empty, active, disconnected, error.
Interactions: routes session selection and global actions.
Accessibility requirements: one `main`, named navigation/sidebar and workspace,
live error region, logical DOM/focus order.
Visual notes: restrained navy canvas; sidebar and workspace are structurally
distinct without decorative glass.
Testing requirements: no horizontal overflow at supported desktop widths.

## Component: GlobalSidebar

Purpose: Provide product identity, create/search/filter controls, and sessions.
Props/data needed: sessions, active ID, harness presets, modes.
States: loading, empty, populated, filtered-no-results.
Interactions: new session, search, lifecycle filter, refresh, select.
Accessibility requirements: labelled `aside`, labelled search, selected session
exposes `aria-current`.
Visual notes: calm rail with sticky header/actions and independently scrolling
list.
Testing requirements: empty, multiple, filter, select, focus-visible.

## Component: SessionCard

Purpose: Make one session identifiable and selectable at a glance.
Props/data needed: name, command, adapter, status, mode, updated time, selected.
States: default, hover, focused, selected, running, exited, failed.
Interactions: click/Enter/Space selects.
Accessibility requirements: native button, accessible status/name, no
color-only state.
Visual notes: rectangular 12px surface; primary identity, command, badge row,
and relative activity; selected state uses border/background, not a side stripe.
Testing requirements: badges, selected state, long text, multiple cards.

## Component: SessionGrid/List

Purpose: Group and order cards by lifecycle.
Props/data needed: cards, search/filter.
States: loading, empty, no matches, grouped.
Interactions: preserves focus and selection while data refreshes.
Accessibility requirements: named groups with counts.
Visual notes: Running first, then Exited and Failed; groups remain compact.
Testing requirements: ordering, grouping, filtering.

## Component: SessionCreateDialog

Purpose: Create detected or manual sessions without crowding the rail.
Props/data needed: harness presets and create API callback.
States: closed, open, validating, submitting, error, advanced expanded.
Interactions: preset selection, field editing, mode selection, advanced
rows/cols/environment, cancel/create.
Accessibility requirements: labelled modal, trapped focus, Escape close,
initial focus, inline errors tied with `aria-describedby`, focus restored.
Visual notes: one clear form; presets are shortcuts, not a competing form.
Testing requirements: keyboard flow, validation, both modes, placeholder
omission, advanced values, close/cancel.

## Component: ActiveSessionLayout

Purpose: Bound header, canvas, composer, and inspector without page scroll traps.
Props/data needed: active session and active mode.
States: live, ended, reconnecting, inspector open.
Interactions: delegates mode and inspector changes.
Accessibility requirements: named workspace and complementary drawer.
Visual notes: header/tool chrome is dense; content is quieter.
Testing requirements: resize, drawer open/close, no canvas clipping.

## Component: SessionHeader

Purpose: Show identity/context and the safest common controls.
Props/data needed: name, status, adapter, model, command, cwd, mode.
States: live, exited, generic, semantic, menu open.
Interactions: mode switch, Interrupt, More.
Accessibility requirements: full labels/tooltips, menu semantics, status text.
Visual notes: destructive actions live only in More.
Testing requirements: metadata, interrupt, menu, confirmation routing.

## Component: ChatTranscript

Purpose: Present semantic conversation and conservative generic projection.
Props/data needed: messages, status cards, approval events.
States: waiting, conversation, terminal-only, approval, ended.
Interactions: scrolls to new content without stealing focus.
Accessibility requirements: log/feed semantics and meaningful role labels.
Visual notes: centered 760px reading column; assistant content is unboxed,
user turns use a quiet selected surface, system events are compact cards.
Testing requirements: semantic upserts, no ANSI/mojibake/`MMMMMMMM`, long text.

## Component: ChatMessage

Purpose: Render one user, assistant, or system turn.
Props/data needed: stable ID, role, text, timestamp.
States: user, assistant, system.
Interactions: text selection.
Accessibility requirements: role announced in accessible name/text.
Visual notes: prose wraps; code preserves whitespace without page overflow.
Testing requirements: roles, multiline, long tokens.

## Component: SystemStatusCard

Purpose: Explain terminal-only, reconnect, adapter, or ended states.
Props/data needed: title, detail, state, optional action.
States: neutral, info, warning, error.
Interactions: optional Open Terminal/retry.
Accessibility requirements: `status` or alert only when urgency warrants.
Visual notes: compact full-border tinted surface; never a raw event dump.
Testing requirements: terminal fallback and ended state.

## Component: ApprovalCard

Purpose: Let the user review and safely act on backend-supplied actions.
Props/data needed: prompt, command, cwd, event ID, actions, pending state.
States: pending, submitting, resolved, stale/error.
Interactions: backend-provided actions and Open Terminal.
Accessibility requirements: labelled region, context before action buttons,
busy/disabled semantics.
Visual notes: warning surface with readable command context.
Testing requirements: render, deny, stale action, session isolation.

## Component: ChatComposer

Purpose: Submit prompts and expose secondary actions.
Props/data needed: value, readiness, adapter name, send callback.
States: ready, disabled/waiting, submitting, multiline.
Interactions: Enter send, Shift+Enter newline, slash menu, button send.
Accessibility requirements: persistent label, readiness description, named
slash/send buttons.
Visual notes: stable bottom dock with a single visual primary action.
Testing requirements: click/Enter/Shift+Enter, disabled state, focus retention.

## Component: SlashCommandMenu

Purpose: Expose session/chat actions without toolbar clutter.
Props/data needed: available actions and disabled/destructive state.
States: closed, open, focused item.
Interactions: click, arrows, Home/End, Enter, Escape.
Accessibility requirements: menu/menuitem roles, roving focus, trigger
restoration.
Visual notes: fixed/portal-like positioning above composer; section labels
separate navigation, terminal keys, and lifecycle actions.
Testing requirements: open/close, keyboard navigation, no clipping, actions.

## Component: TerminalPanel

Purpose: Preserve the complete raw PTY source of truth.
Props/data needed: session, snapshot, stream, dimensions.
States: connecting, live, ended, error.
Interactions: terminal typing/paste, fit/resize, Open Chat.
Accessibility requirements: named region and explicit fallback input.
Visual notes: intentional dark terminal frame with compact live chrome.
Testing requirements: xterm render/input/paste, resize, snapshot/reconnect.

## Component: InspectorDrawer

Purpose: Make adapter, process, stream, snapshot, and event details available
without dominating the conversation.
Props/data needed: session metadata, stream state, events, capabilities.
States: closed, open, overview/events/capabilities tabs.
Interactions: tabs, close/Escape, refresh/copy.
Accessibility requirements: labelled complementary region, focus entry and
restoration, accessible tabs.
Visual notes: right-side elevated workbench panel; closed by default.
Testing requirements: hidden default, open/close, events visible, no reflow bug.

## Component: CommandMenu

Purpose: Implement the header More menu.
Props/data needed: session lifecycle and callbacks.
States: closed/open; actions enabled/disabled.
Interactions: refresh snapshot, copy ID, inspector, raw fallback, terminate,
force kill.
Accessibility requirements: same keyboard contract as SlashCommandMenu.
Visual notes: safe actions first, destructive section last.
Testing requirements: menu keyboard behavior and destructive routing.

## Component: ConfirmDialog

Purpose: Confirm terminate and strongly confirm force kill.
Props/data needed: title, description, confirm label, required phrase.
States: closed/open/invalid/submitting.
Interactions: cancel, Escape, confirm; force kill requires `KILL`.
Accessibility requirements: alert dialog, trapped focus, initial cancel focus,
described danger, focus restoration.
Visual notes: danger is clear but not theatrical.
Testing requirements: cancel, keyboard, wrong phrase, correct confirmation.

## Component: Toast/ErrorBanner

Purpose: Communicate recoverable global failures.
Props/data needed: message and dismiss/retry.
States: info, error, dismissed.
Interactions: dismiss, optional retry.
Accessibility requirements: alert for errors, no repeated live announcements.
Visual notes: compact banner inside workspace, not an overlay that blocks work.
Testing requirements: API failure, dismiss, stale error cleared after success.

## Component: StatusBadge

Purpose: Express lifecycle in text and color.
Props/data needed: status and compact mode.
States: starting, running, exited, failed, terminated.
Interactions: none.
Accessibility requirements: full status text available in compact cards.
Visual notes: semantic dot plus label; consistent vocabulary.
Testing requirements: every status and contrast.

## Component: AdapterBadge

Purpose: Distinguish Generic, Codex, and future adapters.
Props/data needed: adapter ID/name and semantic capability.
States: generic, Codex, future/unknown.
Interactions: optional capability tooltip.
Accessibility requirements: adapter name remains visible text.
Visual notes: cyan for semantic adapters, neutral for generic.
Testing requirements: generic and fake/real Codex.

## Component: ModeSwitcher

Purpose: Switch local presentation without restarting the session.
Props/data needed: Chat/Terminal value and callback.
States: either tab selected, disabled only when no session.
Interactions: click, Left/Right, Home/End.
Accessibility requirements: tabs with `aria-selected` and tabpanel relation.
Visual notes: compact two-segment workbench control.
Testing requirements: keyboard and same-session preservation.

