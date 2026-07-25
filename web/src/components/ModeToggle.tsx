import type { ViewMode } from "../types";

export function ModeToggle({ value, onChange, label }: { value: ViewMode; onChange: (mode: ViewMode) => void; label?: string }) {
  return (
    <div className="mode-field">
      {label && <span>{label}</span>}
      <div className="segmented-control" role="tablist" aria-label={label || "Session mode"}>
        <button type="button" className={value === "chat" ? "is-selected" : ""} onClick={() => onChange("chat")} aria-pressed={value === "chat"}>
          Chat
        </button>
        <button type="button" className={value === "terminal" ? "is-selected" : ""} onClick={() => onChange("terminal")} aria-pressed={value === "terminal"}>
          Terminal
        </button>
      </div>
    </div>
  );
}
