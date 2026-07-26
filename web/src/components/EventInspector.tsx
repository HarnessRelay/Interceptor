import { useState } from "react";
import { api } from "../api/client";
import type { EventEnvelope, SemanticAction, SemanticEventData } from "../types";

export function EventInspector({
  events,
  onOpenTerminal,
  onError
}: {
  events: EventEnvelope[];
  onOpenTerminal: () => void;
  onError: (message: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [actionState, setActionState] = useState<Record<string, string>>({});

  async function submit(event: EventEnvelope, action: SemanticAction) {
    if (!event.session_id) return;
    const key = `${event.id}:${action.id}`;
    if (action.kind === "ui" && action.id === "open_terminal") {
      onOpenTerminal();
      setActionState((current) => ({ ...current, [key]: "Opened" }));
      return;
    }
    setActionState((current) => ({ ...current, [key]: "Submitting" }));
    try {
      await api.submitAction(event.session_id, event.id, action);
      setActionState((current) => ({ ...current, [key]: "Accepted" }));
    } catch (err) {
      const message = (err as Error).message;
      setActionState((current) => ({ ...current, [key]: message }));
      onError(message);
    }
  }

  return (
    <section className={open ? "event-panel is-open" : "event-panel"}>
      <button className="event-toggle" onClick={() => setOpen((value) => !value)} aria-expanded={open}>
        <span>Debug events</span>
        <span>{events.length}</span>
      </button>
      {open && (
        <div className="event-list">
          {events.length === 0 ? (
            <p>No events for this session yet.</p>
          ) : (
            <ol>
              {events.slice(0, 14).map((event) => (
                <li key={event.id}>
                  <div className="event-row">
                    <span>{event.type}</span>
                    <time>{new Date(event.ts).toLocaleTimeString()}</time>
                  </div>
                  <SemanticActionCard event={event} actionState={actionState} onSubmit={submit} />
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
    </section>
  );
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
      <div>
        <strong>{data.title || data.summary || "Action available"}</strong>
        {data.description && <p>{data.description}</p>}
        {data.confidence && <span className="confidence">{data.confidence}</span>}
      </div>
      <div className="action-buttons">
        {data.actions.map((action) => {
          const state = actionState[`${event.id}:${action.id}`];
          return (
            <button key={action.id} className={action.danger || action.style === "danger" ? "danger-button" : action.style === "primary" ? "primary-button" : undefined} onClick={() => onSubmit(event, action)}>
              {state || action.label || action.id}
            </button>
          );
        })}
      </div>
    </div>
  );
}
