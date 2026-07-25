import { useCallback, useEffect, useMemo, useState } from "react";
import { api, setCSRFToken } from "../api/client";
import type { EventEnvelope, Session, ViewMode } from "../types";
import { EmptyState } from "./EmptyState";
import { EventInspector } from "./EventInspector";
import { LoginScreen } from "./LoginScreen";
import { SessionHeader } from "./SessionHeader";
import { Sidebar } from "./Sidebar";
import { ChatView } from "./ChatView";
import { TerminalView } from "./TerminalView";

const modeStoragePrefix = "harnessrelay.sessionMode.";

export function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeID, setActiveID] = useState<string | null>(null);
  const [events, setEvents] = useState<EventEnvelope[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [authenticated, setAuthenticated] = useState(false);
  const [modeBySession, setModeBySession] = useState<Record<string, ViewMode>>({});

  const active = useMemo(
    () => sessions.find((session) => session.id === activeID) || null,
    [activeID, sessions]
  );
  const activeMode = active ? modeBySession[active.id] || "chat" : "chat";

  const refreshSessions = useCallback(async () => {
    const next = await api.listSessions();
    setSessions(next);
    setActiveID((current) => current || next[0]?.id || null);
    setModeBySession((current) => {
      const nextModes = { ...current };
      for (const session of next) {
        nextModes[session.id] = readStoredMode(session.id) || nextModes[session.id] || "chat";
      }
      return nextModes;
    });
  }, []);

  useEffect(() => {
    api.authStatus()
      .then((status) => {
        setAuthenticated(status.authenticated);
        setCSRFToken(status.csrf_token || "");
        if (status.authenticated) {
          return refreshSessions();
        }
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [refreshSessions]);

  const handleLogin = async (token: string) => {
    const status = await api.login(token);
    setCSRFToken(status.csrf_token || "");
    setAuthenticated(status.authenticated);
    await refreshSessions();
  };

  const setSessionMode = useCallback((sessionID: string, mode: ViewMode) => {
    localStorage.setItem(modeStoragePrefix + sessionID, mode);
    setModeBySession((current) => ({ ...current, [sessionID]: mode }));
  }, []);

  const handleCreated = async (session: Session, mode: ViewMode) => {
    setError(null);
    setSessions((current) => [session, ...current.filter((item) => item.id !== session.id)]);
    setSessionMode(session.id, mode);
    setActiveID(session.id);
  };

  const updateActiveSession = useCallback(async () => {
    if (!activeID) return;
    const session = await api.getSession(activeID);
    setSessions((current) => current.map((item) => (item.id === session.id ? session : item)));
  }, [activeID]);

  const handleSessionEvent = useCallback((event: EventEnvelope) => {
    setEvents((current) => [event, ...current].slice(0, 120));
  }, []);

  const activeEvents = events.filter((event) => !activeID || event.session_id === activeID);

  if (!authenticated) {
    return <LoginScreen loading={loading} error={error} onLogin={handleLogin} onError={setError} />;
  }

  return (
    <main className="app-shell">
      <Sidebar
        sessions={sessions}
        activeID={activeID}
        loading={loading}
        onRefresh={() => refreshSessions().catch((err) => setError(err.message))}
        onCreated={handleCreated}
        onError={setError}
        onSelect={setActiveID}
        modeBySession={modeBySession}
      />

      <section className="workspace" aria-label="Active harness session">
        {error && (
          <div className="notice" role="alert">
            <span>{error}</span>
            <button onClick={() => setError(null)}>Dismiss</button>
          </div>
        )}
        {active ? (
          <>
            <SessionHeader
              session={active}
              mode={activeMode}
              onModeChange={(mode) => setSessionMode(active.id, mode)}
              onInterrupt={updateActiveSession}
              onTerminate={updateActiveSession}
              onError={setError}
            />
            {activeMode === "chat" ? (
              <ChatView
                session={active}
                events={activeEvents}
                onOpenTerminal={() => setSessionMode(active.id, "terminal")}
                onSessionUpdate={updateActiveSession}
                onEvent={handleSessionEvent}
                onError={setError}
              />
            ) : (
              <TerminalView
                session={active}
                onOpenChat={() => setSessionMode(active.id, "chat")}
                onSessionUpdate={updateActiveSession}
                onEvent={handleSessionEvent}
                onError={setError}
              />
            )}
            <EventInspector events={activeEvents} onError={setError} />
          </>
        ) : (
          <EmptyState loading={loading} />
        )}
      </section>
    </main>
  );
}

function readStoredMode(sessionID: string): ViewMode | null {
  const value = localStorage.getItem(modeStoragePrefix + sessionID);
  return value === "chat" || value === "terminal" ? value : null;
}
