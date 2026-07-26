import { KeyboardEvent, useEffect, useRef } from "react";

const actionGroups = [
  {
    label: "Session",
    actions: [
      { id: "terminal", label: "Open Terminal" },
      { id: "inspector", label: "Show inspector" },
      { id: "snapshot", label: "Refresh snapshot" },
      { id: "clear", label: "Clear local transcript" }
    ]
  },
  {
    label: "Terminal keys",
    actions: [
      { id: "enter", label: "Send Enter" },
      { id: "escape", label: "Send Escape" },
      { id: "tab", label: "Send Tab" },
      { id: "ctrlc", label: "Send Ctrl+C" },
      { id: "interrupt", label: "Interrupt" }
    ]
  },
  {
    label: "Lifecycle",
    actions: [
      { id: "terminate", label: "Terminate session", danger: true },
      { id: "kill", label: "Force kill…", danger: true }
    ]
  }
];

export function SlashCommandMenu({
  open,
  onAction,
  onClose
}: {
  open: boolean;
  onAction: (action: string) => void;
  onClose: () => void;
}) {
  const menuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (open) window.requestAnimationFrame(() => menuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus());
  }, [open]);

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'));
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    let next = current;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = items.length - 1;
    if (event.key === "ArrowDown") next = (current + 1) % items.length;
    if (event.key === "ArrowUp") next = (current - 1 + items.length) % items.length;
    items[next]?.focus();
  }

  if (!open) return null;
  return (
    <div ref={menuRef} className="slash-menu" role="menu" aria-label="Session command menu" onKeyDown={onKeyDown}>
      <div className="slash-menu-header">
        <span>Session actions</span>
        <kbd>Esc</kbd>
      </div>
      {actionGroups.map((group) => (
        <div className="menu-group" key={group.label}>
          <div className="menu-group-label">{group.label}</div>
          {group.actions.map((action) => (
            <button
              key={action.id}
              type="button"
              role="menuitem"
              className={"danger" in action && action.danger ? "danger-menu-item" : undefined}
              onClick={() => onAction(action.id)}
            >
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}
