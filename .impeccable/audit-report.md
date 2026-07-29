# Audit Report: HarnessRelay Dashboard

## Audit Health Score

| # | Dimension | Score | Key Finding |
|---|-----------|-------|-------------|
| 1 | Accessibility | 2 | Placeholder text fails 4.5:1 contrast; body `min-width: 760px` blocks mobile |
| 2 | Performance | 2 | No lazy loading on images; global CSS imports; minor gradient overhead |
| 3 | Responsive Design | 1 | `body { min-width: 760px }` makes the app unusable on mobile |
| 4 | Theming | 1 | 30+ hard-coded hex/rgba colors bypass design tokens |
| 5 | Anti-Patterns | 2 | Ghost-card pattern (border + shadow ≥16px) in 4 components |
| **Total** | | **9/20** | **Poor — significant work needed** |

**Rating band**: 6-9 Poor (major overhaul)

---

## Anti-Patterns Verdict

**Start here.** Does this look AI-generated? The answer is mostly no — the design is intentional and terminal-adjacent. But there are clear AI-slop tells that need addressing:

1. **Ghost-card pattern (P1)**: `.dialog-panel`, `.slash-menu`, `.composer`, and `.empty-state-mark` all pair `border: 1px solid` with `box-shadow` having blur ≥ 16px (40px, 40px, 30px, 30px respectively). This is the "ghost card" decoration pattern — pick one or the other, never both.
2. **Excessive glow on primary button (P2)**: `.primary-button` uses `box-shadow: 0 0 20px -6px rgba(6, 214, 160, 0.2)` — a 20px blur glow that reads as decorative rather than functional.
3. **Hard-coded colors (P1)**: 30+ instances of hard-coded hex/rgba values in `styles.css` that should use CSS variables from `tokens.css`.

**What's clean**: No gradient text, no glassmorphism, no hero metrics, no card grids, no uppercase eyebrows on every section, no numbered markers, no sketchy SVG, no repeating-linear-gradient stripes, no decorative grid backgrounds. Border radii are appropriate (8-16px for cards, 999px for pills).

---

## Executive Summary

- **Audit Health Score**: **9/20 (Poor)**
- **Total issues found**: 15 issues (1 P0, 4 P1, 6 P2, 4 P3)
- **Top 3-5 critical issues**:
  1. `body { min-width: 760px }` makes the dashboard completely unusable on mobile devices
  2. Placeholder text fails WCAG AA contrast (effective ~2.1:1 vs required 4.5:1)
  3. Ghost-card pattern in 4 components (border + shadow with blur ≥ 16px)
  4. 30+ hard-coded colors bypassing the design token system
  5. `overflow: hidden` on body prevents scrolling and can trap keyboard focus
- **Recommended next steps**: Fix responsive foundation first, then address ghost-card pattern and theming consistency

---

## Detailed Findings by Severity

### P0 Blocking

#### [P0] Body min-width prevents mobile use entirely
- **Location**: `web/src/styles.css`, line 16
- **Category**: Responsive Design
- **Impact**: The dashboard cannot be used on any screen narrower than 760px. This includes virtually all mobile phones. The `min-width: 760px` on `body` forces horizontal overflow on small viewports, making the app completely unusable on mobile.
- **WCAG/Standard**: Violates responsive design best practices; WCAG 1.4.10 (Reflow)
- **Recommendation**: Remove `min-width: 760px` from `body`. Use responsive breakpoints to collapse the sidebar and reflow content on narrow viewports. The existing 900px media query already starts this but the min-width override prevents it from working.
- **Suggested command**: `$impeccable adapt`

#### [P0] Overflow hidden on body traps focus and prevents scrolling
- **Location**: `web/src/styles.css`, line 19
- **Category**: Accessibility / Responsive Design
- **Impact**: `overflow: hidden` on `body` prevents scrolling on mobile devices and can trap keyboard focus within the page. Combined with `min-width: 760px`, this makes the app completely broken on mobile.
- **WCAG/Standard**: WCAG 2.1.1 (Keyboard), WCAG 1.4.10 (Reflow)
- **Recommendation**: Remove `overflow: hidden` from `body`. Use targeted `overflow: hidden` on specific containers that need it (e.g., terminal viewport) instead of the global body.
- **Suggested command**: `$impeccable adapt`

### P1 Major

#### [P1] Placeholder text fails contrast requirements
- **Location**: `web/src/styles.css`, lines 87-91
- **Category**: Accessibility
- **Impact**: Placeholder text uses `color: #7a8fa8` with `opacity: 0.5`, resulting in an effective contrast ratio of approximately 2.1:1 against the input background (`#0c1525`). This fails the WCAG AA requirement of 4.5:1 for normal text. Users with low vision cannot read placeholder text.
- **WCAG/Standard**: WCAG 1.4.3 (Contrast Minimum) — AA
- **Recommendation**: Remove `opacity: 0.5` from placeholder styles, or use a dedicated CSS variable with sufficient contrast (e.g., `var(--color-text-muted)` without opacity reduction). If the placeholder needs to be visually de-emphasized, use a color that still meets 4.5:1 against the input background.
- **Suggested command**: `$impeccable colorize`

#### [P1] Ghost-card pattern in dialog, slash menu, composer, and empty state
- **Location**: `web/src/styles.css`, lines 789-798 (`.dialog-panel`), 1531-1542 (`.slash-menu`), 1446-1462 (`.composer`), 1132-1145 (`.empty-state-mark`)
- **Category**: Anti-Pattern
- **Impact**: These components pair `border: 1px solid` with `box-shadow` having blur ≥ 16px (40px, 40px, 30px, 30px respectively). This is the "ghost card" decoration pattern — a 1px border plus a soft wide drop shadow on the same element. It reads as decorative rather than functional.
- **Recommendation**: Pick one treatment per element. For dialogs/menus, use a solid border at the brand color OR a defined shadow at no more than 8px blur, never both as decoration. For the empty state mark, use either the border or the glow, not both.
- **Suggested command**: `$impeccable polish`

#### [P1] Hard-coded colors bypass design tokens (30+ instances)
- **Location**: `web/src/styles.css` — throughout (e.g., `#0a1628`, `#0c2b4a`, `#0a3d5c`, `#061a30`, `#0b111d`, `#0d1626`, `#2a6247`, `#6d3138`, `#ff767d`, `#2d4d5b`, `#0d1f2a`, `#43c8ed`, `#286b5d`, `#a6f0dc`, `#693239`, `#285d52`, `#ffadb1`, `#7a8fa8`, `rgba(6, 214, 160, ...)`, `rgba(0, 180, 216, ...)`, `rgba(6, 8, 12, 0.18)`, `rgba(0, 0, 0, 0.5)`)
- **Category**: Theming
- **Impact**: Colors are hard-coded as hex/rgba values throughout `styles.css` instead of using the CSS variables defined in `tokens.css`. This means theme switching is broken, dark mode variants can't be added, and maintaining color consistency requires finding and replacing values across many files. The design tokens exist but are inconsistently applied.
- **Recommendation**: Replace all hard-coded colors with their corresponding CSS variables from `tokens.css`. For colors that don't have a token (e.g., adapter-specific colors like `#43c8ed`), add new semantic tokens.
- **Suggested command**: `$impeccable document` (to capture the full visual system), then `$impeccable polish`

#### [P1] No aria-live for dynamic content updates
- **Location**: `web/src/components/App.tsx`, `web/src/components/Sidebar.tsx`
- **Category**: Accessibility
- **Impact**: Dynamic content updates (session list, error messages, loading states) don't use `aria-live` regions. Screen reader users won't be notified when sessions are added, removed, or when errors occur. The error notice uses `role="alert"` (good) but the session list and loading states don't have appropriate live region announcements.
- **WCAG/Standard**: WCAG 4.1.3 (Status Messages)
- **Recommendation**: Add `aria-live="polite"` to the session list container and loading state containers. Ensure error messages are announced via `role="alert"` (already done for the error notice).
- **Suggested command**: `$impeccable harden`

### P2 Minor

#### [P2] No lazy loading on images
- **Location**: `web/src/components/LoginScreen.tsx`, line 91 (`LogoMark` component)
- **Category**: Performance
- **Impact**: The `LogoMark` component renders `<img>` elements without `loading="lazy"`. While these are small logo images, lazy loading is a best practice that defers off-screen images.
- **Recommendation**: Add `loading="lazy"` to `<img>` elements in `LogoMark`.
- **Suggested command**: `$impeccable optimize`

#### [P2] Global CSS import for xterm
- **Location**: `web/src/main.tsx`, line 3
- **Category**: Performance
- **Impact**: `@xterm/xterm/css/xterm.css` is imported globally, adding to the initial bundle size. This CSS is only needed when the terminal view is active.
- **Recommendation**: Consider code-splitting the xterm CSS or importing it only when the TerminalView component mounts.
- **Suggested command**: `$impeccible optimize`

#### [P2] Excessive glow on primary button
- **Location**: `web/src/styles.css`, lines 109-116 (`.primary-button`)
- **Category**: Anti-Pattern
- **Impact**: The primary button uses `box-shadow: 0 0 20px -6px rgba(6, 214, 160, 0.2)` — a 20px blur glow. While the border is transparent (so it's not technically the ghost-card pattern), the glow is excessive for a primary action button and reads as decorative rather than functional.
- **Recommendation**: Reduce the glow to ≤ 8px blur, or remove it entirely and rely on the gradient background for emphasis.
- **Suggested command**: `$impeccable polish`

#### [P2] Hard-coded rgba values in gradients and shadows
- **Location**: `web/src/styles.css` — multiple gradient and shadow definitions
- **Category**: Theming
- **Impact**: Gradients and shadows use hard-coded rgba values (e.g., `rgba(6, 214, 160, 0.12)`, `rgba(6, 214, 160, 0.15)`, `rgba(6, 214, 160, 0.2)`, `rgba(0, 180, 216, 0.1)`, `rgba(6, 8, 12, 0.18)`, `rgba(0, 0, 0, 0.5)`) instead of CSS variables. This prevents theme switching and makes color management harder.
- **Recommendation**: Define these as CSS variables in `tokens.css` (e.g., `--shadow-glow-teal`, `--overlay-dark`) and use them throughout.
- **Suggested command**: `$impeccable document`

#### [P2] No tablet-specific breakpoints
- **Location**: `web/src/styles.css`, lines 1928-2006
- **Category**: Responsive Design
- **Impact**: Media queries exist only at 1180px and 900px. There's no breakpoint targeting tablet devices (768px-1024px range). The 900px breakpoint collapses the sidebar but doesn't optimize the layout for tablet form factors.
- **Recommendation**: Add a breakpoint at 768px to optimize the layout for tablets, potentially keeping the sidebar as a collapsible overlay rather than a full-width element.
- **Suggested command**: `$impeccable adapt`

#### [P2] Fixed-width login panel elements
- **Location**: `web/src/styles.css`, lines 181-185 (`.login-shell`), 334-339 (`.login-panel`)
- **Category**: Responsive Design
- **Impact**: The login shell uses `grid-template-columns: minmax(420px, 1.08fr) minmax(420px, 0.92fr)` which requires a minimum of 840px width. On screens between 760px and 840px, the layout may not display correctly.
- **Recommendation**: Reduce the minimum column width or add a breakpoint to stack the login form on narrow viewports.
- **Suggested command**: `$impecable adapt`

### P3 Polish

#### [P3] Non-semantic `<i>` tags for icons
- **Location**: `web/src/components/LoginScreen.tsx` (lines 45, 55, 77, 82), `web/src/components/Sidebar.tsx` (lines 54, 59, 66, 101)
- **Category**: Accessibility
- **Impact**: Icon elements use `<i>` tags with `aria-hidden="true"`. While this is acceptable (the `<i>` tag is hidden from screen readers), it's not semantically ideal. The `<i>` tag is intended for idiomatic text, not icons.
- **Recommendation**: Use `<span>` with `aria-hidden="true"` instead of `<i>` for icon elements, or use SVG icons with proper `aria-hidden` attributes.
- **Suggested command**: `$impeccable polish`

#### [P3] Dialog doesn't use native `<dialog>` element
- **Location**: `web/src/components/Dialog.tsx`
- **Category**: Accessibility
- **Impact**: The custom Dialog component implements its own focus trapping and Escape key handling (which is good), but doesn't use the native `<dialog>` element or Popover API. The native `<dialog>` element provides built-in accessibility features and focus management.
- **Recommendation**: Consider migrating to the native `<dialog>` element or Popover API for better accessibility and less custom code. The current implementation is functional but could be simplified.
- **Suggested command**: `$impeccable polish`

#### [P3] Hard-coded color in `.danger-button`
- **Location**: `web/src/styles.css`, lines 131-134
- **Category**: Theming
- **Impact**: `.danger-button` uses hard-coded `#6d3138` and `#ff767d` instead of CSS variables like `var(--color-danger)` and `var(--color-bg-danger)`.
- **Recommendation**: Use CSS variables from `tokens.css`.
- **Suggested command**: `$impeccable polish`

#### [P3] Hard-coded color in `.field-error`
- **Location**: `web/src/styles.css`, line 888
- **Category**: Theming
- **Impact**: `.field-error` uses hard-coded `#ffadb1` instead of a CSS variable.
- **Recommendation**: Add a `--color-danger-strong` token (already exists in `tokens.css` as `#ff9297`) and use it, or add a dedicated error text token.
- **Suggested command**: `$impeccable polish`

---

## Patterns & Systemic Issues

1. **Hard-coded colors appear in 30+ locations throughout `styles.css`**, bypassing the design token system in `tokens.css`. This is a systemic theming issue — the tokens exist but are inconsistently applied. Gradients, borders, backgrounds, text colors, and shadows all use hard-coded hex/rgba values.

2. **Ghost-card pattern is systemic** — 4 components (`.dialog-panel`, `.slash-menu`, `.composer`, `.empty-state-mark`) all pair `border: 1px solid` with `box-shadow` blur ≥ 16px. This suggests a pattern in the codebase where decorative borders and shadows are applied together without consideration of the anti-pattern.

3. **Responsive foundation is broken** — `body { min-width: 760px; overflow: hidden; }` makes the app completely unusable on mobile. This is a fundamental architectural issue that affects all components.

4. **Placeholder contrast is a systemic issue** — the `opacity: 0.5` on placeholder text affects all input and textarea elements project-wide.

---

## Positive Findings

- **Dialog component has excellent focus management**: proper focus trapping, Escape key handling, focus restoration, `aria-modal`, `aria-labelledby`, and `aria-describedby`.
- **Semantic HTML is used well**: `<main>`, `<section>`, `<aside>`, `<header>`, `<form>`, `<label>`, `<button>` elements are used appropriately.
- **ARIA attributes are used correctly**: `aria-current="page"` on session cards, `aria-pressed` on filter buttons, `aria-label` on icon buttons, `role="alert"` on error messages.
- **`prefers-reduced-motion` media query exists** and disables transitions and scroll-behavior.
- **Design tokens are well-structured** in `tokens.css` with semantic naming (surface colors, border colors, text colors, status colors, spacing, radius, shadows, motion, z-index).
- **Brand colors are consistent**: teal (`#06d6a0`) and cyan (`#00b4d8`) are used consistently for accents and status indicators.
- **Border radii are appropriate**: 8-16px for cards, 999px for pills/buttons. No over-rounded elements.
- **No AI slop tells**: no gradient text, glassmorphism, hero metrics, card grids, uppercase eyebrows, numbered markers, sketchy SVG, or decorative grid backgrounds.
- **Color strategy is restrained**: dark theme with teal/cyan accents, appropriate for a terminal-adjacent product UI.
- **Component state vocabulary is complete**: buttons have default, hover, focus, active, disabled states defined in CSS.

---

## Recommended Actions

1. **[P0] `$impeccable adapt`**: Remove `body { min-width: 760px; overflow: hidden; }` and add responsive breakpoints to make the dashboard usable on mobile.
2. **[P1] `$impeccable polish`**: Fix the ghost-card pattern in `.dialog-panel`, `.slash-menu`, `.composer`, and `.empty-state-mark` — remove either the border or the shadow (prefer border for definition, shadow ≤ 8px blur for depth).
3. **[P1] `$impeccable colorize`**: Fix placeholder text contrast by removing `opacity: 0.5` and using a proper contrast color.
4. **[P1] `$impeccable document`**: Generate DESIGN.md to capture the full visual system, then systematically replace 30+ hard-coded colors with CSS variables.
5. **[P1] `$impeccable harden`**: Add `aria-live` regions for dynamic content updates (session list, loading states).
6. **[P2] `$impeccable optimize`**: Add `loading="lazy"` to images, consider code-splitting xterm CSS.
7. **[P2] `$impeccable adapt`**: Add tablet-specific breakpoint (768px) and fix fixed-width login panel elements.
8. **[P3] `$impeccable polish`**: Replace non-semantic `<i>` tags with `<span>`, consider native `<dialog>` element, fix remaining hard-coded colors.

> You can ask me to run these one at a time, all at once, or in any order you prefer.
>
> Re-run `$impeccable audit` after fixes to see your score improve.
