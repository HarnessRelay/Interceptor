import { KeyboardEvent } from "react";
import type { ViewMode } from "../types";

export function ModeToggle({ value, onChange, label }: { value: ViewMode; onChange: (mode: ViewMode) => void; label?: string }) {
  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const mode = event.key === "ArrowLeft" || event.key === "Home" ? "chat" : "terminal";
    onChange(mode);
    event.currentTarget.querySelector<HTMLButtonElement>(`[data-mode="${mode}"]`)?.focus();
  }

  return (
    <div className="mode-field">
      {label && <span>{label}</span>}
      <div className="segmented-control" role="tablist" aria-label={label || "Session mode"} onKeyDown={onKeyDown}>
        <button data-mode="chat" type="button" role="tab" className={value === "chat" ? "is-selected" : ""} onClick={() => onChange("chat")} aria-selected={value === "chat"} tabIndex={value === "chat" ? 0 : -1}>
          Chat
        </button>
        <button data-mode="terminal" type="button" role="tab" className={value === "terminal" ? "is-selected" : ""} onClick={() => onChange("terminal")} aria-selected={value === "terminal"} tabIndex={value === "terminal" ? 0 : -1}>
          Terminal
        </button>
      </div>
    </div>
  );
}
