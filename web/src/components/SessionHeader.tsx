import { KeyboardEvent, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { Session, ViewMode } from "../types";
import { commandLine, isLive } from "../utils";
import { AdapterBadge } from "./AdapterBadge";
import { Confirmation, ConfirmDialog } from "./ConfirmDialog";
import { ModeToggle } from "./ModeToggle";
import { StatusBadge } from "./StatusBadge";

export function SessionHeader({
  session,
  mode,
  model,
  onModeChange,
  onInterrupt,
  onTerminate,
  onOpenInspector,
  onError
}: {
  session: Session;
  mode: ViewMode;
  model?: string;
  onModeChange: (mode: ViewMode) => void;
  onInterrupt: () => void;
  onTerminate: () => void;
  onOpenInspector: () => void;
  onError: (message: string) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [busy, setBusy] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const live = isLive(session.status);
  const semantic = session.adapter_capabilities?.includes("semantic_chat");

  useEffect(() => {
    if (!menuOpen) return;
    menuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus();
    function onDocumentClick(event: MouseEvent) {
      if (!menuRef.current?.contains(event.target as Node) && !triggerRef.current?.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    }
    document.addEventListener("mousedown", onDocumentClick);
    return () => document.removeEventListener("mousedown", onDocumentClick);
  }, [menuOpen]);

  async function interrupt() {
    try {
      await api.interrupt(session.id);
      await onInterrupt();
    } catch (err) {
      onError((err as Error).message);
    }
  }

  async function confirmAction() {
    if (!confirmation) return;
    setBusy(true);
    try {
      if (confirmation.kind === "terminate") await api.terminate(session.id);
      else await api.kill(session.id);
      setConfirmation(null);
      await onTerminate();
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function closeMenu() {
    setMenuOpen(false);
    window.requestAnimationFrame(() => triggerRef.current?.focus());
  }

  function runMenuAction(action: string) {
    closeMenu();
    if (action === "inspector") onOpenInspector();
    if (action === "snapshot") void api.snapshot(session.id).catch((err) => onError(err.message));
    if (action === "copy") {
      void navigator.clipboard.writeText(session.id)
        .catch(() => onError("Could not copy the session ID."));
    }
    if (action === "terminate") setConfirmation({ kind: "terminate", label: session.name || session.command });
    if (action === "kill") setConfirmation({ kind: "kill", label: session.name || session.command });
  }

  function onMenuKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)'));
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    if (event.key === "Escape") {
      event.preventDefault();
      closeMenu();
      return;
    }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    let next = current;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = items.length - 1;
    if (event.key === "ArrowDown") next = (current + 1) % items.length;
    if (event.key === "ArrowUp") next = (current - 1 + items.length) % items.length;
    items[next]?.focus();
  }

  return (
    <>
      <header className="session-header">
        <div className="session-header-main">
          <div className="session-title-row">
            <h2>{session.name || session.command}</h2>
            <StatusBadge status={session.status} />
          </div>
          <div className="session-context">
            <AdapterBadge id={session.adapter_id} name={session.adapter_name} semantic={semantic} />
            {model && <span className="metadata-chip">Model {model}</span>}
            <span className="command-summary" title={commandLine(session.command, session.args)}>{commandLine(session.command, session.args)}</span>
            <span className="path-summary" title={session.cwd || "Daemon working directory"}>{session.cwd || "Daemon working directory"}</span>
          </div>
        </div>
        <div className="session-header-actions">
          <ModeToggle value={mode} onChange={onModeChange} />
          <button className="interrupt-button" onClick={interrupt} disabled={!live}>
            <span aria-hidden="true">■</span> Interrupt
          </button>
          <div className="menu-anchor">
            <button
              ref={triggerRef}
              className="icon-button"
              type="button"
              aria-label="More session actions"
              aria-haspopup="menu"
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen((value) => !value)}
            >
              <span aria-hidden="true">•••</span>
            </button>
            {menuOpen && (
              <div ref={menuRef} className="command-menu" role="menu" aria-label="Session actions" onKeyDown={onMenuKeyDown}>
                <button role="menuitem" type="button" onClick={() => runMenuAction("inspector")}>Open inspector</button>
                <button role="menuitem" type="button" onClick={() => runMenuAction("snapshot")}>Refresh snapshot</button>
                <button role="menuitem" type="button" onClick={() => runMenuAction("copy")}>Copy session ID</button>
                <div className="menu-separator" role="separator" />
                <button role="menuitem" type="button" className="danger-menu-item" disabled={!live} onClick={() => runMenuAction("terminate")}>Terminate session</button>
                <button role="menuitem" type="button" className="danger-menu-item" disabled={!live} onClick={() => runMenuAction("kill")}>Force kill…</button>
              </div>
            )}
          </div>
        </div>
      </header>
      <ConfirmDialog
        confirmation={confirmation}
        busy={busy}
        onCancel={() => setConfirmation(null)}
        onConfirm={confirmAction}
      />
    </>
  );
}
