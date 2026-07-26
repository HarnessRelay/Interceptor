import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { EventEnvelope, SemanticAction, SemanticEventData, Session } from "../types";
import { commandLine } from "../utils";
import { AdapterBadge } from "./AdapterBadge";

type InspectorTab = "overview" | "events" | "capabilities";

export function EventInspector({
  open,
  session,
  events,
  onClose,
  onOpenTerminal,
  onError
}: {
  open: boolean;
  session: Session;
  events: EventEnvelope[];
  onClose: () => void;
  onOpenTerminal: () => void;
  onError: (message: string) => void;
}) {
  const [tab, setTab] = useState<InspectorTab>("overview");
  const [actionState, setActionState] = useState<Record<string, string>>({});
  const closeRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (!open) return;
    window.requestAnimationFrame(() => closeRef.current?.focus());
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, onClose]);

  async function submit(event: EventEnvelope, action: SemanticAction) {
    if (!event.session_id) return;
    const key = `${event.id}:${action.id}`;
    if (action.kind === "ui" && action.id === "open_terminal") {
      onOpenTerminal();
      setActionState((current) => ({ ...current, [key]: "Opened" }));
      return;
    }
    setActionState((current) => ({ ...current, [key]: "Submitting…" }));
    try {
      await api.submitAction(event.session_id, event.id, action);
      setActionState((current) => ({ ...current, [key]: "Accepted" }));
    } catch (err) {
      const message = (err as Error).message;
      setActionState((current) => ({ ...current, [key]: message }));
      onError(message);
    }
  }

  if (!open) return null;
  const semantic = session.adapter_capabilities?.includes("semantic_chat");

  return (
    <aside className="inspector-drawer event-panel is-open" aria-label="Session inspector">
      <header className="inspector-header">
        <div>
          <span className="panel-kicker">Inspector</span>
          <h2>{session.name || session.command}</h2>
        </div>
        <button ref={closeRef} className="icon-button" type="button" onClick={onClose} aria-label="Close inspector">×</button>
      </header>
      <div className="inspector-tabs" role="tablist" aria-label="Inspector sections">
        {(["overview", "events", "capabilities"] as const).map((value) => (
          <button
            key={value}
            role="tab"
            type="button"
            aria-selected={tab === value}
            className={tab === value ? "is-selected" : ""}
            onClick={() => setTab(value)}
          >
            {value === "overview" ? "Overview" : value === "events" ? `Events ${events.length}` : "Capabilities"}
          </button>
        ))}
      </div>
      <div className="inspector-content">
        {tab === "overview" && (
          <dl className="metadata-list">
            <Metadata label="Status" value={session.status} />
            <div>
              <dt>Adapter</dt>
              <dd><AdapterBadge id={session.adapter_id} name={session.adapter_name} semantic={semantic} /></dd>
            </div>
            <Metadata label="Command" value={commandLine(session.command, session.args)} code />
            <Metadata label="Working directory" value={session.cwd || "Daemon working directory"} code />
            <Metadata label="Terminal" value={`${session.terminal.rows} rows × ${session.terminal.cols} columns`} />
            <Metadata label="Session ID" value={session.id} code />
            <Metadata label="Created" value={new Date(session.created_at).toLocaleString()} />
            {session.exit_code !== undefined && <Metadata label="Exit code" value={String(session.exit_code)} />}
          </dl>
        )}
        {tab === "events" && (
          <div className="event-list">
            {events.length === 0 ? (
              <div className="inspector-empty">No events for this session yet.</div>
            ) : (
              <ol>
                {events.slice(0, 100).map((event) => (
                  <li key={event.id}>
                    <div className="event-row">
                      <span>{event.type}</span>
                      <time dateTime={event.ts}>{new Date(event.ts).toLocaleTimeString()}</time>
                    </div>
                    <details>
                      <summary>Payload</summary>
                      <pre>{JSON.stringify(event.data ?? {}, null, 2)}</pre>
                    </details>
                    <SemanticActionCard event={event} actionState={actionState} onSubmit={submit} />
                  </li>
                ))}
              </ol>
            )}
          </div>
        )}
        {tab === "capabilities" && (
          <div>
            <p className="inspector-description">Capabilities are reported by the selected backend adapter.</p>
            <ul className="capability-list">
              {(session.adapter_capabilities || []).map((capability) => <li key={capability}>{capability.replaceAll("_", " ")}</li>)}
            </ul>
          </div>
        )}
      </div>
    </aside>
  );
}

function Metadata({ label, value, code = false }: { label: string; value: string; code?: boolean }) {
  return <div><dt>{label}</dt><dd className={code ? "metadata-code" : undefined}>{value}</dd></div>;
}

function SemanticActionCard({
  event,
  actionState,
  onSubmit
}: {
  event: EventEnvelope;
  actionState: Record<string, string>;
  onSubmit: (event: EventEnvelope, action: SemanticAction) => void;
}) {
  const data = event.data as SemanticEventData | undefined;
  if (!data?.actions || data.actions.length === 0) return null;
  return (
    <div className="action-card">
      <strong>{data.title || data.summary || "Action available"}</strong>
      {data.description && <p>{data.description}</p>}
      <div className="action-buttons">
        {data.actions.map((action) => (
          <button
            key={action.id}
            className={action.danger || action.style === "danger" ? "danger-button" : action.style === "primary" ? "primary-button" : undefined}
            onClick={() => onSubmit(event, action)}
          >
            {actionState[`${event.id}:${action.id}`] || action.label || action.id}
          </button>
        ))}
      </div>
    </div>
  );
}
