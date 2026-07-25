import { FormEvent, useState } from "react";
import { api } from "../api/client";
import type { CreateForm, Session, ViewMode } from "../types";
import { commandLine } from "../utils";
import { LogoMark } from "./LoginScreen";
import { ModeToggle } from "./ModeToggle";
import { StatusBadge } from "./StatusBadge";

export function Sidebar({
  sessions,
  activeID,
  loading,
  onRefresh,
  onCreated,
  onError,
  onSelect,
  modeBySession
}: {
  sessions: Session[];
  activeID: string | null;
  loading: boolean;
  onRefresh: () => void;
  onCreated: (session: Session, mode: ViewMode) => void;
  onError: (message: string) => void;
  onSelect: (id: string) => void;
  modeBySession: Record<string, ViewMode>;
}) {
  return (
    <aside className="sidebar" aria-label="Sessions">
      <header className="brand">
        <div className="brand-lockup">
          <LogoMark />
          <div>
            <h1>HarnessRelay</h1>
            <p>Chat-first local harness control</p>
          </div>
        </div>
        <button className="icon-button" onClick={onRefresh} title="Refresh sessions" aria-label="Refresh sessions">
          ↻
        </button>
      </header>
      <CreateSessionForm onCreated={onCreated} onError={onError} />
      <SessionList sessions={sessions} activeID={activeID} loading={loading} onSelect={onSelect} modeBySession={modeBySession} />
    </aside>
  );
}

function CreateSessionForm({ onCreated, onError }: { onCreated: (session: Session, mode: ViewMode) => void; onError: (message: string) => void }) {
  const [form, setForm] = useState<CreateForm>({ name: "", command: "/bin/bash", args: "", cwd: "", mode: "chat" });
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!form.command.trim()) {
      onError("Command is required.");
      return;
    }
    setSubmitting(true);
    try {
      onCreated(await api.createSession(form), form.mode);
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="create-form" onSubmit={submit}>
      <div className="form-section-title">New session</div>
      <label>
        <span>Name</span>
        <input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="optional label" />
      </label>
      <label>
        <span>Command</span>
        <input value={form.command} onChange={(event) => setForm({ ...form, command: event.target.value })} placeholder="/bin/bash" />
      </label>
      <label>
        <span>Args</span>
        <input value={form.args} onChange={(event) => setForm({ ...form, args: event.target.value })} placeholder="-l" />
      </label>
      <label>
        <span>CWD</span>
        <input value={form.cwd} onChange={(event) => setForm({ ...form, cwd: event.target.value })} placeholder="daemon default" />
      </label>
      <ModeToggle value={form.mode} onChange={(mode) => setForm({ ...form, mode })} label="Start in" />
      <button className="primary-button" disabled={submitting}>
        {submitting ? "Creating" : "Create session"}
      </button>
    </form>
  );
}

function SessionList({
  sessions,
  activeID,
  loading,
  onSelect,
  modeBySession
}: {
  sessions: Session[];
  activeID: string | null;
  loading: boolean;
  onSelect: (id: string) => void;
  modeBySession: Record<string, ViewMode>;
}) {
  if (loading) return <div className="session-empty">Loading sessions</div>;
  if (sessions.length === 0) return <div className="session-empty">No sessions yet</div>;
  return (
    <div className="session-list">
      {sessions.map((session) => (
        <button key={session.id} className={session.id === activeID ? "session-item is-active" : "session-item"} onClick={() => onSelect(session.id)}>
          <StatusBadge status={session.status} compact />
          <span className="session-title">{session.name || session.command}</span>
          <span className="session-subtitle">{commandLine(session.command, session.args)}</span>
          <span className="session-mode-label">{modeBySession[session.id] || "chat"}</span>
        </button>
      ))}
    </div>
  );
}
