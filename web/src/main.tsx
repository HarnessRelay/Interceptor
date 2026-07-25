import React, { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import "./styles.css";

type SessionStatus = "starting" | "running" | "exited" | "failed" | "terminated";

type Session = {
  id: string;
  name?: string;
  harness_type: string;
  adapter_id: string;
  command: string;
  args: string[];
  cwd: string;
  status: SessionStatus;
  terminal: { rows: number; cols: number };
  created_at: string;
  updated_at: string;
  exited_at?: string;
  exit_code?: number;
};

type EventEnvelope = {
  id: string;
  type: string;
  session_id?: string;
  seq: number;
  ts: string;
  data?: unknown;
};

type SemanticAction = {
  id: string;
  label: string;
  style?: "primary" | "secondary" | "danger";
  version?: number;
  requires_event_id?: boolean;
};

type SemanticEventData = {
  title?: string;
  summary?: string;
  description?: string;
  confidence?: string;
  actions?: SemanticAction[];
};

type Snapshot = {
  session_id: string;
  rows: number;
  cols: number;
  latest_seq: number;
  history_truncated: boolean;
  chunks: Array<{ seq: number; encoding: "base64"; bytes: string }>;
};

type CreateForm = {
  name: string;
  command: string;
  args: string;
  cwd: string;
};

type AuthStatus = {
  authenticated: boolean;
  csrf_token?: string;
};

let csrfToken = "";

const api = {
  async authStatus(): Promise<AuthStatus> {
    return request<AuthStatus>("/api/v1/auth/status", { skipAuthRedirect: true });
  },
  async login(token: string): Promise<AuthStatus> {
    return request<AuthStatus>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ token }),
      skipCSRF: true,
      skipAuthRedirect: true
    });
  },
  async listSessions(): Promise<Session[]> {
    const data = await request<{ sessions: Session[] }>("/api/v1/sessions");
    return data.sessions;
  },
  async createSession(input: CreateForm): Promise<Session> {
    const data = await request<{ session: Session }>("/api/v1/sessions", {
      method: "POST",
      body: JSON.stringify({
        name: input.name || undefined,
        command: input.command,
        args: splitArgs(input.args),
        cwd: input.cwd || undefined,
        terminal: { rows: 24, cols: 80 }
      })
    });
    return data.session;
  },
  async getSession(id: string): Promise<Session> {
    const data = await request<{ session: Session }>(`/api/v1/sessions/${id}`);
    return data.session;
  },
  async input(id: string, bytes: string): Promise<void> {
    await request(`/api/v1/sessions/${id}/input`, {
      method: "POST",
      body: JSON.stringify({ mode: "raw", encoding: "base64", data: encodeBase64(bytes) })
    });
  },
  async resize(id: string, rows: number, cols: number): Promise<void> {
    await request(`/api/v1/sessions/${id}/resize`, {
      method: "POST",
      body: JSON.stringify({ rows, cols })
    });
  },
  async interrupt(id: string): Promise<void> {
    await request(`/api/v1/sessions/${id}/interrupt`, {
      method: "POST",
      body: JSON.stringify({ strategy: "ctrl_c" })
    });
  },
  async terminate(id: string): Promise<void> {
    await request(`/api/v1/sessions/${id}/terminate`, {
      method: "POST",
      body: JSON.stringify({ grace_ms: 5000 })
    });
  },
  async kill(id: string): Promise<void> {
    await request(`/api/v1/sessions/${id}/kill`, {
      method: "POST",
      body: JSON.stringify({ confirmation: "KILL" })
    });
  },
  async snapshot(id: string): Promise<Snapshot> {
    return request<Snapshot>(`/api/v1/sessions/${id}/snapshot`);
  },
  async submitAction(sessionID: string, eventID: string, action: SemanticAction): Promise<void> {
    await request(`/api/v1/sessions/${sessionID}/actions/${encodeURIComponent(action.id)}`, {
      method: "POST",
      body: JSON.stringify({
        event_id: eventID,
        action_version: action.version || 0
      })
    });
  }
};

type APIRequestInit = RequestInit & {
  skipCSRF?: boolean;
  skipAuthRedirect?: boolean;
};

async function request<T>(path: string, init: APIRequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string> | undefined)
  };
  if (!init.skipCSRF && csrfToken && isUnsafeMethod(init.method || "GET")) {
    headers["X-CSRF-Token"] = csrfToken;
  }
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers
  });
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try {
      const body = await response.json();
      if (body.error) message = body.error;
    } catch {
      // Keep the status fallback.
    }
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeID, setActiveID] = useState<string | null>(null);
  const [events, setEvents] = useState<EventEnvelope[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [authenticated, setAuthenticated] = useState(false);

  const active = useMemo(
    () => sessions.find((session) => session.id === activeID) || null,
    [activeID, sessions]
  );

  const refreshSessions = useCallback(async () => {
    const next = await api.listSessions();
    setSessions(next);
    setActiveID((current) => current || next[0]?.id || null);
  }, []);

  useEffect(() => {
    api.authStatus()
      .then((status) => {
        setAuthenticated(status.authenticated);
        csrfToken = status.csrf_token || "";
        if (status.authenticated) {
          return refreshSessions();
        }
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [refreshSessions]);

  const handleLogin = async (token: string) => {
    const status = await api.login(token);
    csrfToken = status.csrf_token || "";
    setAuthenticated(status.authenticated);
    await refreshSessions();
  };

  const handleCreated = async (session: Session) => {
    setSessions((current) => [session, ...current.filter((item) => item.id !== session.id)]);
    setActiveID(session.id);
  };

  const updateActiveSession = useCallback(async () => {
    if (!activeID) return;
    const session = await api.getSession(activeID);
    setSessions((current) => current.map((item) => (item.id === session.id ? session : item)));
  }, [activeID]);

  const handleTerminalEvent = useCallback((event: EventEnvelope) => {
    setEvents((current) => [event, ...current].slice(0, 80));
  }, []);

  if (!authenticated) {
    return <LoginScreen loading={loading} error={error} onLogin={handleLogin} onError={setError} />;
  }

  return (
    <main className="app-shell">
      <aside className="sidebar" aria-label="Sessions">
        <header className="brand">
          <div>
            <h1>HarnessRelay</h1>
            <p>Local PTY sessions</p>
          </div>
          <button className="icon-button" onClick={() => refreshSessions().catch((err) => setError(err.message))} title="Refresh sessions">
            ↻
          </button>
        </header>
        <CreateSessionForm onCreated={handleCreated} onError={setError} />
        <SessionList sessions={sessions} activeID={activeID} loading={loading} onSelect={setActiveID} />
      </aside>

      <section className="workspace" aria-label="Active terminal session">
        {error && (
          <div className="notice" role="alert">
            <span>{error}</span>
            <button onClick={() => setError(null)}>Dismiss</button>
          </div>
        )}
        {active ? (
          <>
            <SessionHeader session={active} onInterrupt={updateActiveSession} onTerminate={updateActiveSession} onError={setError} />
            <TerminalPane session={active} onSessionUpdate={updateActiveSession} onEvent={handleTerminalEvent} onError={setError} />
            <EventPanel events={events.filter((event) => !activeID || event.session_id === activeID)} onError={setError} />
          </>
        ) : (
          <EmptyState loading={loading} />
        )}
      </section>
    </main>
  );
}

function LoginScreen({ loading, error, onLogin, onError }: { loading: boolean; error: string | null; onLogin: (token: string) => Promise<void>; onError: (message: string | null) => void }) {
  const [token, setToken] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    onError(null);
    try {
      await onLogin(token);
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-shell">
      <form className="login-panel" onSubmit={submit}>
        <div>
          <h1>HarnessRelay</h1>
          <p>Enter the local dashboard token from the daemon startup log.</p>
        </div>
        <label>
          <span>Local token</span>
          <input className="visually-hidden" tabIndex={-1} autoComplete="username" value="local" readOnly aria-hidden="true" />
          <input value={token} onChange={(event) => setToken(event.target.value)} type="password" autoComplete="current-password" disabled={loading || submitting} />
        </label>
        {error && <div className="login-error">{error}</div>}
        <button className="primary-button" disabled={loading || submitting || token.trim() === ""}>
          {submitting ? "Signing in" : "Sign in"}
        </button>
      </form>
    </main>
  );
}

function CreateSessionForm({ onCreated, onError }: { onCreated: (session: Session) => void; onError: (message: string) => void }) {
  const [form, setForm] = useState<CreateForm>({ name: "", command: "/bin/bash", args: "", cwd: "" });
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!form.command.trim()) {
      onError("Command is required.");
      return;
    }
    setSubmitting(true);
    try {
      onCreated(await api.createSession(form));
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="create-form" onSubmit={submit}>
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
      <button className="primary-button" disabled={submitting}>
        {submitting ? "Creating" : "Create session"}
      </button>
    </form>
  );
}

function SessionList({ sessions, activeID, loading, onSelect }: { sessions: Session[]; activeID: string | null; loading: boolean; onSelect: (id: string) => void }) {
  if (loading) return <div className="session-empty">Loading sessions</div>;
  if (sessions.length === 0) return <div className="session-empty">No sessions yet</div>;
  return (
    <div className="session-list">
      {sessions.map((session) => (
        <button key={session.id} className={session.id === activeID ? "session-item is-active" : "session-item"} onClick={() => onSelect(session.id)}>
          <span className={`status-dot status-${session.status}`} />
          <span className="session-title">{session.name || session.command}</span>
          <span className="session-subtitle">{[session.command, ...session.args].join(" ")}</span>
        </button>
      ))}
    </div>
  );
}

function SessionHeader({ session, onInterrupt, onTerminate, onError }: { session: Session; onInterrupt: () => void; onTerminate: () => void; onError: (message: string) => void }) {
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
      <div>
        <div className="session-kicker">
          <span className={`status-pill status-${session.status}`}>{session.status}</span>
          <span>{session.adapter_id}</span>
        </div>
        <h2>{session.name || session.command}</h2>
        <p>{[session.command, ...session.args].join(" ")}</p>
        <p>{session.cwd || "daemon working directory"}</p>
      </div>
      <div className="toolbar">
        <button onClick={interrupt} disabled={!isLive(session.status)}>Interrupt</button>
        <button className="danger-button" onClick={terminate} disabled={!isLive(session.status)}>Terminate</button>
        <button className="danger-button" onClick={kill} disabled={!isLive(session.status)}>Force kill</button>
      </div>
    </header>
  );
}

function TerminalPane({ session, onSessionUpdate, onEvent, onError }: { session: Session; onSessionUpdate: () => void; onEvent: (event: EventEnvelope) => void; onError: (message: string) => void }) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const latestSeq = useRef(0);
  const [rawInput, setRawInput] = useState("");
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!hostRef.current) return;
    const term = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      fontSize: 13,
      theme: {
        background: "#101215",
        foreground: "#e9eef3",
        cursor: "#ff6b9d",
        selectionBackground: "#334155"
      }
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current);
    terminalRef.current = term;
    fitRef.current = fit;

    let socket: WebSocket | null = null;
    let resizeTimer = 0;
    let disposed = false;

    const sendResize = () => {
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        if (disposed || !fitRef.current || !terminalRef.current) return;
        fitRef.current.fit();
        const activeTerm = terminalRef.current;
        api.resize(session.id, activeTerm.rows, activeTerm.cols).catch((err) => onError(err.message));
      }, 90);
    };

    const resizeObserver = new ResizeObserver(sendResize);
    resizeObserver.observe(hostRef.current);

    term.onData((data) => {
      api.input(session.id, data).catch((err) => onError(err.message));
    });

    const writeSnapshot = (snapshot: Snapshot, reset = false) => {
      latestSeq.current = snapshot.latest_seq || 0;
      if (reset) term.reset();
      for (const chunk of snapshot.chunks) {
        term.write(decodeBase64(chunk.bytes));
      }
    };

    api.snapshot(session.id)
      .then((snapshot) => {
        writeSnapshot(snapshot);
        fit.fit();
        return api.resize(session.id, term.rows, term.cols);
      })
      .catch((err: Error) => onError(err.message))
      .finally(() => {
        if (disposed) return;
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws?session_id=${encodeURIComponent(session.id)}&after_seq=${latestSeq.current}`);
        socket.onopen = () => setConnected(true);
        socket.onclose = () => setConnected(false);
        socket.onerror = () => onError("WebSocket stream failed.");
        socket.onmessage = (message) => {
          const event = JSON.parse(message.data) as EventEnvelope;
          latestSeq.current = Math.max(latestSeq.current, event.seq || 0);
          onEvent(event);
          if (event.type === "terminal.output") {
            const payload = event.data as { data?: string; bytes?: string };
            const encoded = payload?.bytes || payload?.data;
            if (encoded) term.write(decodeBase64(encoded));
          }
          if (event.type === "session.exited") {
            api.snapshot(session.id).then((snapshot) => writeSnapshot(snapshot, true)).catch((err: Error) => onError(err.message));
          }
          if (event.type === "session.exited" || event.type === "session.status_changed") {
            onSessionUpdate();
          }
        };
      });

    term.focus();
    sendResize();

    return () => {
      disposed = true;
      window.clearTimeout(resizeTimer);
      resizeObserver.disconnect();
      socket?.close();
      term.dispose();
      terminalRef.current = null;
      fitRef.current = null;
      setConnected(false);
    };
  }, [session.id, onError, onEvent, onSessionUpdate]);

  async function sendRawInput() {
    if (!rawInput) return;
    try {
      await api.input(session.id, rawInput);
      setRawInput("");
      terminalRef.current?.focus();
    } catch (err) {
      onError((err as Error).message);
    }
  }

  return (
    <section className="terminal-section">
      <div className="terminal-bar">
        <span className={connected ? "stream-state is-connected" : "stream-state"}>{connected ? "Live stream" : "Connecting"}</span>
        <span>{session.terminal.rows}×{session.terminal.cols}</span>
      </div>
      <div className="terminal-host" ref={hostRef} />
      <div className="raw-input">
        <textarea value={rawInput} onChange={(event) => setRawInput(event.target.value)} placeholder="Raw input fallback" />
        <button onClick={sendRawInput}>Send</button>
      </div>
    </section>
  );
}

function EventPanel({ events, onError }: { events: EventEnvelope[]; onError: (message: string) => void }) {
  const [actionState, setActionState] = useState<Record<string, string>>({});

  async function submit(event: EventEnvelope, action: SemanticAction) {
    if (!event.session_id) return;
    const key = `${event.id}:${action.id}`;
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
    <section className="event-panel">
      <h3>Events</h3>
      {events.length === 0 ? (
        <p>No events for this session yet.</p>
      ) : (
        <ol>
          {events.slice(0, 12).map((event) => (
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
    </section>
  );
}

function SemanticActionCard({ event, actionState, onSubmit }: { event: EventEnvelope; actionState: Record<string, string>; onSubmit: (event: EventEnvelope, action: SemanticAction) => void }) {
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
            <button key={action.id} className={action.style === "danger" ? "danger-button" : action.style === "primary" ? "primary-button" : undefined} onClick={() => onSubmit(event, action)}>
              {state || action.label || action.id}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function EmptyState({ loading }: { loading: boolean }) {
  return (
    <div className="empty-state">
      <h2>{loading ? "Loading sessions" : "Create a session"}</h2>
      <p>{loading ? "Checking the local daemon." : "Start with /bin/bash, python3, top, or any terminal command available to the daemon."}</p>
    </div>
  );
}

function splitArgs(value: string): string[] {
  return value.match(/(?:[^\s"]+|"[^"]*")+/g)?.map((part) => part.replace(/^"|"$/g, "")) || [];
}

function decodeBase64(value: string): string {
  return atob(value);
}

function encodeBase64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function isLive(status: SessionStatus): boolean {
  return status === "starting" || status === "running";
}

function isUnsafeMethod(method: string): boolean {
  return !["GET", "HEAD", "OPTIONS", "TRACE"].includes(method.toUpperCase());
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
