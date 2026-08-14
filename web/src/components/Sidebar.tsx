import { FormEvent, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { CreateForm, HarnessPreset, Session, ViewMode } from "../types";
import { cleanTerminalText, commandLine } from "../utils";
import { AdapterBadge } from "./AdapterBadge";
import { Dialog } from "./Dialog";
import { LogoMark } from "./LoginScreen";
import { ModeToggle } from "./ModeToggle";
import { PairingPanel } from "./PairingPanel";
import { StatusBadge } from "./StatusBadge";

type Filter = "all" | "running" | "finished";

export function Sidebar({
  sessions,
  harnesses,
  activeID,
  loading,
  onRefresh,
  onCreated,
  onError,
  onSelect,
  modeBySession,
  createSignal = 0
}: {
  sessions: Session[];
  harnesses: HarnessPreset[];
  activeID: string | null;
  loading: boolean;
  onRefresh: () => void;
  onCreated: (session: Session, mode: ViewMode) => void;
  onError: (message: string) => void;
  onSelect: (id: string) => void;
  modeBySession: Record<string, ViewMode>;
  createSignal?: number;
}) {
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("running");
  useEffect(() => {
    if (createSignal > 0) setCreateOpen(true);
  }, [createSignal]);

  return (
    <aside className="sidebar" aria-label="Session manager">
      <header className="sidebar-header">
        <div className="brand-lockup">
          <LogoMark />
          <div className="brand-copy">
            <h1>HarnessRelay</h1>
            <p>Local harness control</p>
          </div>
        </div>
        <button className="icon-button" onClick={onRefresh} title="Refresh sessions" aria-label="Refresh sessions">
          <span aria-hidden="true">↻</span>
        </button>
      </header>

      <button className="primary-button new-session-button" type="button" onClick={() => setCreateOpen(true)}>
        <span aria-hidden="true">＋</span>
        New session
      </button>

      <div className="session-tools">
        <label className="search-field">
          <span className="visually-hidden">Search sessions</span>
          <span className="search-icon" aria-hidden="true">⌕</span>
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search sessions"
          />
        </label>
        <div className="filter-row" aria-label="Filter sessions">
          {(["all", "running", "finished"] as const).map((value) => (
            <button
              key={value}
              type="button"
              className={filter === value ? "filter-button is-selected" : "filter-button"}
              aria-pressed={filter === value}
              onClick={() => setFilter(value)}
            >
              {value === "all" ? "All" : value === "running" ? "Live" : "Finished"}
            </button>
          ))}
        </div>
      </div>

      <SessionList
        sessions={sessions}
        activeID={activeID}
        loading={loading}
        onSelect={onSelect}
        modeBySession={modeBySession}
        search={search}
        filter={filter}
        onCreate={() => setCreateOpen(true)}
      />

      <PairingPanel />

      <div className="sidebar-footer">
        <span className="local-indicator"><span aria-hidden="true" /> Local daemon</span>
        <span>{sessions.filter((session) => isSessionLive(session)).length} live</span>
      </div>

      <SessionCreateDialog
        open={createOpen}
        harnesses={harnesses}
        onClose={() => setCreateOpen(false)}
        onCreated={(session, mode) => {
          setCreateOpen(false);
          setFilter("all");
          onCreated(session, mode);
        }}
        onError={onError}
      />
    </aside>
  );
}

function SessionCreateDialog({
  open,
  harnesses,
  onClose,
  onCreated,
  onError
}: {
  open: boolean;
  harnesses: HarnessPreset[];
  onClose: () => void;
  onCreated: (session: Session, mode: ViewMode) => void;
  onError: (message: string) => void;
}) {
  const [form, setForm] = useState<CreateForm>({
    name: "",
    command: "/bin/bash",
    args: "",
    cwd: "",
    mode: "chat",
    rows: 24,
    cols: 80
  });
  const [advanced, setAdvanced] = useState(false);
  const [envText, setEnvText] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [fieldError, setFieldError] = useState("");

  function usePreset(preset: HarnessPreset) {
    setForm((current) => ({
      ...current,
      name: preset.name,
      harness_type: preset.id,
      command: preset.command,
      args: preset.args.join(" "),
      mode: preset.default_mode
    }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!form.command.trim()) {
      setFieldError("Enter a command to start this session.");
      return;
    }
    setFieldError("");
    setSubmitting(true);
    try {
      const env = parseEnvironment(envText);
      onCreated(await api.createSession({ ...form, env }), form.mode);
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      title="New session"
      description="Start a detected coding harness or enter an exact command."
      onClose={onClose}
    >
      <form className="create-form" onSubmit={submit}>
        {harnesses.length > 0 && (
          <div className="preset-section">
            <div className="field-label">Detected harnesses</div>
            <div className="preset-list">
              {harnesses.map((harness) => (
                <button key={harness.id} className="preset-button" type="button" onClick={() => usePreset(harness)}>
                  <span className="preset-name">{harness.name}</span>
                  <span className="preset-meta">{[harness.command, harness.version ? cleanTerminalText(harness.version) : ""].filter(Boolean).join(" · ")}</span>
                </button>
              ))}
            </div>
          </div>
        )}

        <div className="form-grid">
          <label>
            <span>Name <small>optional</small></span>
            <input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Project or task label" />
          </label>
          <label>
            <span>Command</span>
            <input
              value={form.command}
              onChange={(event) => {
                setFieldError("");
                setForm({ ...form, command: event.target.value, harness_type: undefined });
              }}
              placeholder="/bin/bash"
              aria-invalid={fieldError ? "true" : undefined}
              aria-describedby={fieldError ? "command-error" : undefined}
            />
            {fieldError && <small id="command-error" className="field-error">{fieldError}</small>}
          </label>
          <label>
            <span>Arguments <small>optional</small></span>
            <input value={form.args} onChange={(event) => setForm({ ...form, args: event.target.value })} placeholder="No arguments" />
          </label>
          <label>
            <span>Working directory <small>optional</small></span>
            <input value={form.cwd} onChange={(event) => setForm({ ...form, cwd: event.target.value })} placeholder="Use daemon working directory" />
          </label>
        </div>

        <ModeToggle value={form.mode} onChange={(mode) => setForm({ ...form, mode })} label="Start mode" />

        <details className="advanced-fields" open={advanced} onToggle={(event) => setAdvanced(event.currentTarget.open)}>
          <summary>Advanced options</summary>
          <div className="advanced-content">
            <div className="dimension-fields">
              <label>
                <span>Rows</span>
                <input type="number" min={1} max={500} value={form.rows} onChange={(event) => setForm({ ...form, rows: Number(event.target.value) })} />
              </label>
              <label>
                <span>Columns</span>
                <input type="number" min={2} max={1000} value={form.cols} onChange={(event) => setForm({ ...form, cols: Number(event.target.value) })} />
              </label>
            </div>
            <label>
              <span>Environment <small>one KEY=value per line</small></span>
              <textarea value={envText} onChange={(event) => setEnvText(event.target.value)} placeholder="NO_COLOR=0" />
            </label>
          </div>
        </details>

        <div className="dialog-actions">
          <button type="button" data-dialog-cancel="true" onClick={onClose}>Cancel</button>
          <button className="primary-button" disabled={submitting}>
            {submitting ? "Starting…" : "Start session"}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function SessionList({
  sessions,
  activeID,
  loading,
  onSelect,
  modeBySession,
  search,
  filter,
  onCreate
}: {
  sessions: Session[];
  activeID: string | null;
  loading: boolean;
  onSelect: (id: string) => void;
  modeBySession: Record<string, ViewMode>;
  search: string;
  filter: Filter;
  onCreate: () => void;
}) {
  const visible = useMemo(() => {
    const query = search.trim().toLowerCase();
    return sessions.filter((session) => {
      const live = isSessionLive(session);
      if (filter === "running" && !live) return false;
      if (filter === "finished" && live) return false;
      if (!query) return true;
      return [session.name, session.command, session.cwd, session.adapter_name, session.adapter_id]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(query));
    });
  }, [sessions, search, filter]);

  if (loading) {
    return <div className="session-list-state" role="status">Loading sessions…</div>;
  }
  if (sessions.length === 0) {
    return (
      <div className="session-list-state">
        <span className="empty-glyph" aria-hidden="true">⌁</span>
        <strong>No sessions yet</strong>
        <p>Start a detected coding harness or a shell.</p>
        <button type="button" onClick={onCreate}>Create your first session</button>
      </div>
    );
  }
  if (visible.length === 0) {
    return <div className="session-list-state"><strong>No matching sessions</strong><p>Try another search or filter.</p></div>;
  }

  const groups = [
    { label: "Running", items: visible.filter(isSessionLive) },
    { label: "Finished", items: visible.filter((session) => !isSessionLive(session)) }
  ].filter((group) => group.items.length > 0);

  return (
    <div className="session-list" aria-live="polite" aria-relevant="additions text">
      {groups.map((group) => (
        <section className="session-group" key={group.label} aria-label={`${group.label} sessions`}>
          <h2>{group.label}<span>{group.items.length}</span></h2>
          <div className="session-cards">
            {group.items.map((session) => (
              <SessionCard
                key={session.id}
                session={session}
                selected={session.id === activeID}
                mode={modeBySession[session.id] || "chat"}
                onSelect={() => onSelect(session.id)}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function SessionCard({ session, selected, mode, onSelect }: { session: Session; selected: boolean; mode: ViewMode; onSelect: () => void }) {
  const semantic = session.adapter_capabilities?.includes("semantic_chat");
  return (
    <button
      type="button"
      className={selected ? "session-card is-active" : "session-card"}
      onClick={onSelect}
      aria-current={selected ? "page" : undefined}
    >
      <span className="session-card-topline">
        <span className="session-card-name">{session.name || session.command}</span>
        <StatusBadge status={session.status} compact />
      </span>
      <span className="session-card-command" title={commandLine(session.command, session.args)}>
        {commandLine(session.command, session.args)}
      </span>
      <span className="session-card-path" title={session.cwd || "Daemon working directory"}>
        {session.cwd || "Daemon working directory"}
      </span>
      <span className="session-card-meta">
        <AdapterBadge id={session.adapter_id} name={session.adapter_name} semantic={semantic} />
        {session.origin === "shim" && (
          <span className="mode-badge">
            Shim · {(session.origin_backend || "pty").toUpperCase()} · {session.attachable ? "Attachable" : "Detached"}
          </span>
        )}
        <span className="mode-badge">{mode === "chat" ? "Chat" : "Terminal"}</span>
        <time dateTime={session.updated_at}>{relativeTime(session.updated_at || session.created_at)}</time>
      </span>
    </button>
  );
}

function parseEnvironment(value: string): Record<string, string> {
  const result: Record<string, string> = {};
  for (const line of value.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const separator = trimmed.indexOf("=");
    if (separator <= 0) continue;
    result[trimmed.slice(0, separator).trim()] = trimmed.slice(separator + 1);
  }
  return result;
}

function isSessionLive(session: Session) {
  return session.status === "starting" || session.status === "running";
}

function relativeTime(value: string) {
  const milliseconds = Date.now() - new Date(value).getTime();
  if (!Number.isFinite(milliseconds) || milliseconds < 60_000) return "now";
  const minutes = Math.floor(milliseconds / 60_000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}
