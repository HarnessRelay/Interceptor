# Product

## Register

product

## Platform

web

## Users

Developers who run terminal-based AI coding harnesses (Codex, OpenCode, Grok, Claude Code) locally. They launch a harness from their terminal, then need to walk away from the machine and still approve commands, steer the session, check progress, or interrupt it — often from a phone or another device. The dashboard is a control surface, not a destination: it exists so the developer doesn't have to stay glued to the terminal.

## Product Purpose

HarnessRelay is a local-first relay/control plane for terminal-based AI coding harnesses. It launches and owns harness processes inside pseudo-terminals, then exposes a local web dashboard for observation, steering, interruption, and raw Terminal Mode fallback. The goal is not to replace the harness but to relay it — letting the developer use their preferred tool locally and control it remotely.

Success looks like: a developer starts a Codex session, walks away, opens the dashboard on their phone, approves a command, and returns to a completed task.

## Positioning

The only local relay layer that lets you walk away from your terminal AI coding agent and still control it from a web dashboard — without replacing the harness or exposing it to the internet.

## Brand Personality

Technical, precise, trustworthy. Developer-first, exact, reliable — like a well-built CLI tool that happens to have a web UI. The interface should feel like it belongs in a terminal-adjacent environment: high contrast, monospace accents for technical data, clear status indicators, and zero decorative fluff. The design serves the control surface, not itself.

## Anti-references

- **Generic SaaS dashboards** — beige surfaces, card-heavy layouts, ghost cards (1px border + wide drop shadow), arbitrary z-index values (999, 9999), uppercase tracked eyebrows above every section, hero-metric templates.
- **Consumer social media UIs** — feeds, infinite scroll, like/comment actions, TikTok-style interfaces. This is a terminal control plane, not a content feed.
- **Over-rounded everything** — 24-40px border-radius on cards and inputs. Cards top out at 12-16px.

## Design Principles

- **Terminal-first fidelity** — Terminal Mode is the source of truth; the web UI never obscures raw terminal behavior. Chat Mode is a convenience, not a replacement.
- **Local-first, token-protected** — localhost by default; no public exposure without explicit opt-in and IP allowlisting.
- **Harness-neutral** — the common architecture works for any terminal harness; harness-specific behavior lives in adapters, never in the core.
- **Precision over ornament** — every UI element serves a control purpose. No decorative illustrations, no skeleton screens, no placeholder content.
- **Trust through transparency** — show session state, command history, and approval requests clearly. Never hide what the harness is doing.
- **Raw terminal fallback always available** — if the semantic adapter fails, the raw PTY must remain accessible.

## Accessibility & Inclusion

WCAG AA compliance: minimum 4.5:1 contrast for body text, 3:1 for large text, full keyboard navigation, reduced motion support via `prefers-reduced-motion`, and semantic HTML structure. The dashboard is a control surface where status clarity matters — color is never the only indicator of state (dots, labels, and borders carry meaning alongside color).
