import { ReactNode, useEffect, useRef } from "react";

const focusableSelector = [
  "button:not([disabled])",
  "[href]",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])"
].join(",");

export function Dialog({
  open,
  title,
  description,
  children,
  onClose,
  initialFocus
}: {
  open: boolean;
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
  initialFocus?: "first" | "cancel";
}) {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    triggerRef.current = document.activeElement as HTMLElement;
    const panel = panelRef.current;
    const focusables = panel ? Array.from(panel.querySelectorAll<HTMLElement>(focusableSelector)) : [];
    const preferred = initialFocus === "cancel"
      ? focusables.find((element) => element.dataset.dialogCancel === "true")
      : focusables[0];
    window.requestAnimationFrame(() => preferred?.focus());

    function onKeyDown(event: globalThis.KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== "Tab" || !panelRef.current) return;
      const next = Array.from(panelRef.current.querySelectorAll<HTMLElement>(focusableSelector));
      if (next.length === 0) return;
      const first = next[0];
      const last = next[next.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      window.requestAnimationFrame(() => triggerRef.current?.focus());
    };
  }, [open, onClose, initialFocus]);

  if (!open) return null;
  const titleID = "dialog-title";
  const descriptionID = description ? "dialog-description" : undefined;

  return (
    <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div
        ref={panelRef}
        className="dialog-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={descriptionID}
      >
        <header className="dialog-header">
          <div>
            <h2 id={titleID}>{title}</h2>
            {description && <p id={descriptionID}>{description}</p>}
          </div>
          <button className="icon-button" type="button" onClick={onClose} aria-label={`Close ${title}`}>
            <span aria-hidden="true">×</span>
          </button>
        </header>
        {children}
      </div>
    </div>
  );
}
