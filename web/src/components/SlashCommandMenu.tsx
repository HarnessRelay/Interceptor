import { KeyboardEvent, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { HarnessCommand } from "../types";

type RelayAction = {
  id: string;
  label: string;
  description: string;
  group: string;
  danger?: boolean;
  shortcut?: string;
};

const relayActions: RelayAction[] = [
  { id: "terminal", label: "Open Terminal", description: "Use the complete live harness interface.", group: "HarnessRelay" },
  { id: "inspector", label: "Show inspector", description: "Inspect raw semantic events and session state.", group: "HarnessRelay" },
  { id: "snapshot", label: "Refresh snapshot", description: "Rebuild this transcript from current session history.", group: "HarnessRelay" },
  { id: "clear", label: "Clear local transcript", description: "Clear only this browser's projected conversation.", group: "HarnessRelay" },
  { id: "enter", label: "Send Enter", description: "Send an Enter key directly to the PTY.", group: "Terminal keys", shortcut: "↵" },
  { id: "escape", label: "Send Escape", description: "Close the current terminal overlay or prompt.", group: "Terminal keys", shortcut: "Esc" },
  { id: "tab", label: "Send Tab", description: "Send a Tab key directly to the PTY.", group: "Terminal keys", shortcut: "Tab" },
  { id: "ctrlc", label: "Send Ctrl+C", description: "Send the terminal interrupt key.", group: "Terminal keys", shortcut: "Ctrl C" },
  { id: "interrupt", label: "Interrupt", description: "Interrupt the foreground harness operation.", group: "Terminal keys" },
  { id: "terminate", label: "Terminate session", description: "Ask the process group to stop gracefully.", group: "Lifecycle", danger: true },
  { id: "kill", label: "Force kill…", description: "Immediately kill the harness process group.", group: "Lifecycle", danger: true }
];

type PaletteItem =
  | { kind: "harness"; id: string; group: string; command: HarnessCommand }
  | { kind: "relay"; id: string; group: string; action: RelayAction };

export function SlashCommandMenu({
  open,
  harnessName,
  harnessCommands,
  catalogLoading,
  onHarnessCommand,
  onAction,
  onClose
}: {
  open: boolean;
  harnessName: string;
  harnessCommands: HarnessCommand[];
  catalogLoading: boolean;
  onHarnessCommand: (command: HarnessCommand) => void;
  onAction: (action: string) => void;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const searchRef = useRef<HTMLInputElement | null>(null);

  const items = useMemo(() => {
    const normalized = query.trim().toLowerCase().replace(/^\//, "");
    const all: PaletteItem[] = [
      ...harnessCommands.map((command): PaletteItem => ({
        kind: "harness",
        id: `harness:${command.id}`,
        group: `${harnessName} · ${command.group}`,
        command
      })),
      ...relayActions.map((action): PaletteItem => ({
        kind: "relay",
        id: `relay:${action.id}`,
        group: action.group,
        action
      }))
    ];
    if (!normalized) return all;
    return all.filter((item) => {
      const text = item.kind === "harness"
        ? `${item.command.invocation} ${item.command.label} ${item.command.description} ${item.command.group}`
        : `${item.action.label} ${item.action.description} ${item.action.group}`;
      return text.toLowerCase().includes(normalized);
    });
  }, [harnessCommands, harnessName, query]);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setActiveIndex(0);
    window.requestAnimationFrame(() => searchRef.current?.focus());
  }, [open]);

  useEffect(() => {
    if (activeIndex >= items.length) setActiveIndex(Math.max(0, items.length - 1));
  }, [activeIndex, items.length]);

  function choose(item: PaletteItem) {
    if (item.kind === "harness") onHarnessCommand(item.command);
    else onAction(item.action.id);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Home" || event.key === "End") {
      event.preventDefault();
      if (event.key === "Home") setActiveIndex(0);
      else if (event.key === "End") setActiveIndex(Math.max(0, items.length - 1));
      else setActiveIndex((current) => {
        const delta = event.key === "ArrowDown" ? 1 : -1;
        return (current + delta + items.length) % Math.max(1, items.length);
      });
      return;
    }
    if (event.key === "Enter" && items[activeIndex]) {
      event.preventDefault();
      choose(items[activeIndex]);
    }
  }

  if (!open) return null;
  let previousGroup = "";
  return createPortal(
    <div className="command-palette-layer" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose();
    }}>
      <section className="slash-menu" role="dialog" aria-modal="false" aria-label="Session command palette">
        <header className="slash-menu-header">
          <div>
            <strong>Commands</strong>
            <span>{harnessCommands.length > 0 ? `${harnessName} and HarnessRelay` : "HarnessRelay controls"}</span>
          </div>
          <kbd>Esc</kbd>
        </header>
        <label className="command-search">
          <span aria-hidden="true">/</span>
          <input
            ref={searchRef}
            role="combobox"
            aria-controls="session-command-results"
            aria-expanded="true"
            aria-activedescendant={items[activeIndex]?.id}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setActiveIndex(0);
            }}
            onKeyDown={onKeyDown}
            placeholder="Search commands and session actions"
          />
        </label>
        <div className="command-results" id="session-command-results" role="listbox" aria-label="Available commands">
          {catalogLoading && <div className="command-empty">Loading harness commands…</div>}
          {!catalogLoading && items.length === 0 && <div className="command-empty">No matching commands</div>}
          {items.map((item, index) => {
            const showGroup = item.group !== previousGroup;
            previousGroup = item.group;
            const label = item.kind === "harness" ? item.command.label : item.action.label;
            const description = item.kind === "harness" ? item.command.description : item.action.description;
            const danger = item.kind === "harness" ? item.command.danger : item.action.danger;
            const meta = item.kind === "harness"
              ? item.command.invocation
              : item.action.shortcut;
            return (
              <div className="command-group-fragment" key={item.id}>
                {showGroup && <div className="menu-group-label">{item.group}</div>}
                <button
                  id={item.id}
                  type="button"
                  role="option"
                  aria-selected={index === activeIndex}
                  className={`${index === activeIndex ? "is-active " : ""}${danger ? "danger-menu-item" : ""}`.trim()}
                  onMouseMove={() => setActiveIndex(index)}
                  onClick={() => choose(item)}
                >
                  <span className="command-copy">
                    <strong>{label}</strong>
                    <small>{description}</small>
                  </span>
                  <span className="command-meta">
                    {item.kind === "harness" && item.command.availability === "conditional" && <em title={item.command.availability_note}>Conditional</em>}
                    {meta && <kbd>{meta}</kbd>}
                  </span>
                </button>
              </div>
            );
          })}
        </div>
      </section>
    </div>,
    document.body
  );
}
