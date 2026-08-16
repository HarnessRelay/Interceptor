import { useCallback, useEffect, useMemo, useState } from "react";
import { api, setCSRFToken } from "../api/client";
import type { AuthStatus, EventEnvelope, HarnessPreset, SemanticEventData, Session, ViewMode } from "../types";
import { EmptyState } from "./EmptyState";
import { EventInspector } from "./EventInspector";
import { LoginScreen } from "./LoginScreen";
import { SessionHeader } from "./SessionHeader";
import { SettingsView } from "./SettingsView";
import { Sidebar } from "./Sidebar";
import { ChatView } from "./ChatView";
import type { ChatMessage, ChatMessagesUpdater } from "./ChatView";
import { TerminalView } from "./TerminalView";

const modeStoragePrefix = "harnessrelay.sessionMode.";
const chatMessagesStorageKey = "harnessrelay.chatMessages";

export function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeID, setActiveID] = useState<string | null>(null);
  const [events, setEvents] = useState<EventEnvelope[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [authenticated, setAuthenticated] = useState(false);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [harnesses, setHarnesses] = useState<HarnessPreset[]>([]);
  const [modeBySession, setModeBySession] = useState<Record<string, ViewMode>>({});
  const [chatMessagesBySession, setChatMessagesBySession] = useState<Record<string, ChatMessage[]>>(readStoredChatMessages);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [createSignal, setCreateSignal] = useState(0);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const active = useMemo(
    () => sessions.find((session) => session.id === activeID) || null,
    [activeID, sessions]
  );
  const activeMode = active ? modeBySession[active.id] || "chat" : "chat";
  const activeMetadata = [...events]
    .reverse()
    .find((event) => event.session_id === activeID && event.type === "harness.metadata")?.data as SemanticEventData | undefined;

  const refreshSessions = useCallback(async () => {
    const next = await api.listSessions();
    setSessions(next);
    setActiveID((current) => current || null);
    setModeBySession((current) => {
      const nextModes = { ...current };
      for (const session of next) {
        nextModes[session.id] = readStoredMode(session.id) || nextModes[session.id] || "chat";
      }
      return nextModes;
    });
    setChatMessagesBySession((current) => {
      const sessionIDs = new Set(next.map((session) => session.id));
      return Object.fromEntries(Object.entries(current).filter(([sessionID]) => sessionIDs.has(sessionID)));
    });
  }, []);

  const refreshHarnesses = useCallback(async () => {
    setHarnesses(await api.listHarnesses());
  }, []);

  useEffect(() => {
    api.authStatus()
      .then((status) => {
        setAuthStatus(status);
        setAuthenticated(status.authenticated);
        setCSRFToken(status.csrf_token || "");
        if (status.authenticated) {
          return Promise.all([refreshSessions(), refreshHarnesses()]);
        }
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, [refreshSessions]);

  useEffect(() => {
    writeStoredChatMessages(chatMessagesBySession);
  }, [chatMessagesBySession]);

  useEffect(() => {
    setInspectorOpen(false);
  }, [activeID]);

  const handleLogin = async (token: string) => {
    const status = await api.login(token);
    setAuthStatus(status);
    setCSRFToken(status.csrf_token || "");
    setAuthenticated(status.authenticated);
    await Promise.all([refreshSessions(), refreshHarnesses()]);
  };

  const handleDeviceAuthenticated = async (csrfToken: string) => {
    setCSRFToken(csrfToken);
    setAuthenticated(true);
    setAuthStatus((current) => (current ? { ...current, authenticated: true } : current));
    await Promise.all([refreshSessions(), refreshHarnesses()]);
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
    setEvents((current) => {
      if (current.some((item) => item.id === event.id)) return current;
      return [event, ...current].slice(0, 1024);
    });
  }, []);

  const setChatMessages = useCallback((sessionID: string, updater: ChatMessagesUpdater) => {
    setChatMessagesBySession((current) => {
      const existing = current[sessionID] || [];
      const messages = typeof updater === "function" ? updater(existing) : updater;
      return { ...current, [sessionID]: messages };
    });
  }, []);

  const activeEvents = events.filter((event) => !activeID || event.session_id === activeID);
  const activeMessages = active ? chatMessagesBySession[active.id] || [] : [];

  if (!authenticated) {
    return (
      <LoginScreen
        loading={loading}
        error={error}
        authStatus={authStatus}
        onLogin={handleLogin}
        onDeviceAuthenticated={handleDeviceAuthenticated}
        onError={setError}
      />
    );
  }

  return (
    <main className="app-shell">
      <Sidebar
        sessions={sessions}
        harnesses={harnesses}
        activeID={activeID}
        loading={loading}
        settingsOpen={settingsOpen}
        onOpenSettings={() => setSettingsOpen(true)}
        onRefresh={() => refreshSessions().catch((err) => setError(err.message))}
        onCreated={handleCreated}
        onError={setError}
        onSelect={(id) => {
          setActiveID(id);
          setSettingsOpen(false);
        }}
        modeBySession={modeBySession}
        createSignal={createSignal}
      />

      <section className="workspace" aria-label="Active harness session" aria-live="polite" aria-relevant="additions text">
        {error && (
          <div className="notice" role="alert">
            <span>{error}</span>
            <button onClick={() => setError(null)}>Dismiss</button>
          </div>
        )}
        {settingsOpen ? (
          <SettingsView onClose={() => setSettingsOpen(false)} onError={setError} />
        ) : active ? (
          <>
            <SessionHeader
              session={active}
              mode={activeMode}
              model={activeMetadata?.model}
              onModeChange={(mode) => setSessionMode(active.id, mode)}
              onInterrupt={updateActiveSession}
              onTerminate={updateActiveSession}
              onOpenInspector={() => setInspectorOpen(true)}
              onError={setError}
            />
            {activeMode === "chat" ? (
              <ChatView
                session={active}
                events={activeEvents}
                messages={activeMessages}
                setMessages={setChatMessages}
                onOpenTerminal={() => setSessionMode(active.id, "terminal")}
                onOpenInspector={() => setInspectorOpen(true)}
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
            <EventInspector
              open={inspectorOpen}
              session={active}
              events={activeEvents}
              onClose={() => setInspectorOpen(false)}
              onOpenTerminal={() => {
                setSessionMode(active.id, "terminal");
                setInspectorOpen(false);
              }}
              onError={setError}
            />
          </>
        ) : (
          <EmptyState loading={loading} onCreate={() => setCreateSignal((value) => value + 1)} />
        )}
      </section>
    </main>
  );
}

function readStoredMode(sessionID: string): ViewMode | null {
  const value = localStorage.getItem(modeStoragePrefix + sessionID);
  return value === "chat" || value === "terminal" ? value : null;
}

function readStoredChatMessages(): Record<string, ChatMessage[]> {
  try {
    const raw = sessionStorage.getItem(chatMessagesStorageKey);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const messagesBySession: Record<string, ChatMessage[]> = {};
    for (const [sessionID, messages] of Object.entries(parsed)) {
      if (!Array.isArray(messages)) continue;
      const validMessages = messages.filter(isChatMessage);
      if (validMessages.length > 0) messagesBySession[sessionID] = validMessages;
    }
    return messagesBySession;
  } catch {
    return {};
  }
}

function writeStoredChatMessages(messagesBySession: Record<string, ChatMessage[]>) {
  try {
    sessionStorage.setItem(chatMessagesStorageKey, JSON.stringify(messagesBySession));
  } catch {
    // Losing reload-only transcript cache should not break the live dashboard.
  }
}

function isChatMessage(value: unknown): value is ChatMessage {
  if (!value || typeof value !== "object") return false;
  const message = value as Partial<ChatMessage>;
  return typeof message.id === "string" &&
    (message.role === "user" || message.role === "assistant" || message.role === "system") &&
    typeof message.text === "string" &&
    typeof message.ts === "string";
}
