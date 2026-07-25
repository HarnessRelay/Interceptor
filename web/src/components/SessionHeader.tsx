import { api } from "../api/client";
import type { Session, ViewMode } from "../types";
import { commandLine, isLive } from "../utils";
import { ModeToggle } from "./ModeToggle";
import { StatusBadge } from "./StatusBadge";

export function SessionHeader({
  session,
  mode,
  onModeChange,
  onInterrupt,
  onTerminate,
  onError
}: {
  session: Session;
  mode: ViewMode;
  onModeChange: (mode: ViewMode) => void;
  onInterrupt: () => void;
  onTerminate: () => void;
  onError: (message: string) => void;
}) {
  async function interrupt() {
    try {
      await api.interrupt(session.id);
      await onInterrupt();
    } catch (err) {
      onError((err as Error).message);
    }
  }

  async function terminate() {
    if (!window.confirm(`Terminate ${session.name || session.command}?`)) return;
    try {
      await api.terminate(session.id);
      await onTerminate();
    } catch (err) {
      onError((err as Error).message);
    }
  }

  async function kill() {
    const confirmation = window.prompt(`Force kill ${session.name || session.command}? Type KILL to continue.`);
    if (confirmation !== "KILL") return;
    try {
      await api.kill(session.id);
      await onTerminate();
    } catch (err) {
      onError((err as Error).message);
    }
  }

  return (
    <header className="session-header">
      <div className="session-header-main">
        <div className="session-kicker">
          <StatusBadge status={session.status} />
          <span>{session.adapter_id}</span>
          <span>{session.terminal.rows}×{session.terminal.cols}</span>
        </div>
        <h2>{session.name || session.command}</h2>
        <p>{commandLine(session.command, session.args)}</p>
        <p>{session.cwd || "daemon working directory"}</p>
      </div>
      <div className="session-header-actions">
        <ModeToggle value={mode} onChange={onModeChange} />
        <div className="toolbar">
          <button onClick={interrupt} disabled={!isLive(session.status)}>Interrupt</button>
          <button className="danger-button" onClick={terminate} disabled={!isLive(session.status)}>Terminate</button>
          <button className="danger-button" onClick={kill} disabled={!isLive(session.status)}>Force kill</button>
        </div>
      </div>
    </header>
  );
}
