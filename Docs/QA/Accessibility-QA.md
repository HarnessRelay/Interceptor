# Accessibility QA

Date: 2026-07-26

## Automated checks

`npm --prefix web run qa:a11y` runs the Playwright test `Accessibility QA:
keyboard, labels, focus, and contrast`.

Verified:

- every visible button has an accessible name
- every non-hidden input and textarea has a wrapped/explicit label or
  `aria-label`
- primary text against the workspace canvas is at least 4.5:1
- secondary text against product surfaces is at least 4.5:1
- selected modes expose tab semantics and `aria-selected`
- selected session cards expose `aria-current="page"`
- inspector and modal regions have names

Result: passed.

## Keyboard checks

- Login token entry and Enter submit: passed.
- New Session trigger and dialog entry: passed.
- Dialog Tab/Shift+Tab focus containment: passed.
- Escape closes the creation and confirmation dialogs and restores focus:
  passed.
- Chat/Terminal Left/Right switching without session restart: passed.
- Slash menu entry and Escape close: passed.
- Composer Enter and Shift+Enter behavior: passed in the main suite.
- Destructive confirmation can be cancelled without a pointer: passed.

## Screen reader/ARIA checks

- Product regions use `main`, named `aside`, workspace `section`, transcript
  `log`, inspector `aside`, `dialog`, `tablist`, `tab`, `menu`, and `menuitem`.
- Connection and activity text uses a polite status region.
- Compact status dots retain visually hidden lifecycle text.
- Icon-only refresh, close, More, and slash controls have accessible names.
- Approval context precedes its backend-provided action buttons.

## Contrast checks

The automated check reads the committed CSS tokens and calculates WCAG relative
luminance for primary/canvas and secondary/surface pairs. Both pass 4.5:1.
Focus uses a high-chroma teal border plus a 3px translucent ring; status does
not rely on color alone because visible text is retained.

## Issues found

- UIR-002: creation dialog could extend beyond a short viewport.
- UIR-005: former browser confirmation/prompt flows did not provide
  product-consistent focus containment.

## Fixes applied

- Dialogs have a viewport-bounded scrolling region.
- A shared dialog primitive traps Tab/Shift+Tab, closes on Escape, and restores
  the invoking control.
- Terminate and force kill now use accessible in-product dialogs. Force kill
  still requires the exact `KILL` phrase.
- Mode controls use tabs with arrow, Home, and End key behavior.
- Menus use arrow, Home, End, Enter/Space, and Escape behavior.
- Labels and persistent composer guidance were added.
- Reduced-motion rules remain enabled for all state transitions.

## Remaining limitations

- A full assistive-technology pass with NVDA, JAWS, or VoiceOver was not
  available in this Linux automation environment.
- xterm.js is an inherently terminal-oriented surface; the labelled raw input
  fallback remains the more predictable screen-reader input path.
- The supported responsive target is desktop/laptop. No mobile-app scope was
  added.
