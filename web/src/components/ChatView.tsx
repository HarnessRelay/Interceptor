import { FormEvent, KeyboardEvent, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { EventEnvelope, SemanticAction, SemanticEventData, Session } from "../types";
import { decodeBase64, isLive, projectTerminalOutputForChat, terminalOutputText } from "../utils";
import { SlashCommandMenu } from "./SlashCommandMenu";

export type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  text: string;
  ts: string;
};

export type ChatMessagesUpdater = ChatMessage[] | ((current: ChatMessage[]) => ChatMessage[]);
type ActivityState = "connecting" | "ready" | "thinking" | "streaming" | "terminal" | "approval" | "ended";

const terminalStatusMessageID = "terminal-output-status";

export function ChatView({
  session,
  events,
  messages,
  setMessages,
  onOpenTerminal,
  onSessionUpdate,
  onEvent,
  onError
}: {
  session: Session;
  events: EventEnvelope[];
  messages: ChatMessage[];
  setMessages: (sessionID: string, updater: ChatMessagesUpdater) => void;
  onOpenTerminal: () => void;
  onSessionUpdate: () => void;
  onEvent: (event: EventEnvelope) => void;
  onError: (message: string) => void;
}) {
  const [prompt, setPrompt] = useState("");
  const [connected, setConnected] = useState(false);
  const [activity, setActivity] = useState<ActivityState>(isLive(session.status) ? "connecting" : "ended");
  const [activityDetail, setActivityDetail] = useState("");
  const [slashOpen, setSlashOpen] = useState(false);
  const [actionState, setActionState] = useState<Record<string, string>>({});
  const latestSeq = useRef(0);
  const activityTimer = useRef<number | null>(null);
  const transcriptRef = useRef<HTMLDivElement | null>(null);
  const semanticAdapter = session.adapter_capabilities?.includes("semantic_chat") ?? false;
  const canSend = isLive(session.status) && (!semanticAdapter || activity === "ready");

  useEffect(() => {
    let socket: WebSocket | null = null;
    let disposed = false;

    Promise.all([api.snapshot(session.id), api.events(session.id)])
      .then(([snapshot, history]) => {
        latestSeq.current = Math.max(snapshot.latest_seq || 0, ...history.map((event) => event.seq || 0));
        history.forEach(onEvent);
        if (semanticAdapter) {
          setMessages(session.id, (current) => mergeMessages(current, semanticMessages(history)));
          const latestStatus = [...history].reverse().find((event) => event.type === "harness.status");
          if (latestStatus) applyStatus(latestStatus);
        } else {
          const projection = projectTerminalOutputForChat(snapshot.chunks.map((chunk) => decodeBase64(chunk.bytes)).join(""));
          const hydratedMessages = snapshotMessages(projection, new Date().toISOString());
          setMessages(session.id, (current) => current.length > 0 ? current : hydratedMessages);
        }
      })
      .catch((err: Error) => onError(err.message))
      .finally(() => {
        if (disposed) return;
        onSessionUpdate();
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws?session_id=${encodeURIComponent(session.id)}&after_seq=${latestSeq.current}`);
        socket.onopen = () => {
          setConnected(true);
          if (!semanticAdapter) {
            setActivity((current) => current === "connecting" ? "ready" : current);
          }
        };
        socket.onclose = () => {
          setConnected(false);
          setActivity(isLive(session.status) ? "connecting" : "ended");
        };
        socket.onerror = () => onError("WebSocket stream failed.");
        socket.onmessage = (message) => {
          const event = JSON.parse(message.data) as EventEnvelope;
          latestSeq.current = Math.max(latestSeq.current, event.seq || 0);
          onEvent(event);
          if (semanticAdapter) {
            appendSemanticEvent(event);
          } else {
            if (event.type === "chat.user_message") appendSemanticEvent(event);
            if (event.type === "terminal.output") appendAssistantOutput(event);
          }
          if (event.type === "session.exited" || event.type === "session.status_changed") onSessionUpdate();
        };
      });

    return () => {
      disposed = true;
      socket?.close();
      clearActivityTimer();
      setConnected(false);
    };
  }, [session.id, semanticAdapter, setMessages, onError, onEvent, onSessionUpdate]);

  useEffect(() => {
    if (!isLive(session.status)) setActivity("ended");
    else if (!connected) setActivity("connecting");
    else if (!semanticAdapter) setActivity((current) => current === "ended" || current === "connecting" ? "ready" : current);
  }, [session.status, connected, semanticAdapter]);

  useEffect(() => {
    transcriptRef.current?.scrollTo({ top: transcriptRef.current.scrollHeight });
  }, [messages]);

  const metadata = useMemo(() => {
    const event = events.find((item) => item.type === "harness.metadata");
    return event?.data as SemanticEventData | undefined;
  }, [events]);

  const approvals = useMemo(() => {
    const resolved = new Set(
      events
        .filter((event) => event.type === "approval.resolved")
        .map((event) => (event.data as SemanticEventData | undefined)?.approval_event_id)
        .filter(Boolean)
    );
    return events.filter((event) => event.type === "approval.required" && !resolved.has(event.id)).slice(0, 2);
  }, [events]);

  function appendSemanticEvent(event: EventEnvelope) {
    if (event.type === "harness.status") {
      applyStatus(event);
      return;
    }
    const next = semanticMessage(event);
    if (next) {
      setMessages(session.id, (current) => mergeMessages(current, [next]));
    }
    if (event.type === "approval.required") {
      setActivity("approval");
      setActivityDetail("Codex is waiting for an explicit decision.");
    }
  }

  function applyStatus(event: EventEnvelope) {
    const data = event.data as SemanticEventData | undefined;
    setActivityDetail(data?.detail || "");
    switch (data?.status) {
      case "processing":
      case "thinking":
        setActivity("thinking");
        break;
      case "waiting_for_approval":
      case "waiting_for_terminal":
        setActivity("approval");
        break;
      case "terminal_ui_active":
        setActivity("terminal");
        break;
      case "idle":
        setActivity("ready");
        break;
    }
  }

  function appendAssistantOutput(event: EventEnvelope) {
    const projection = projectTerminalOutputForChat(terminalOutputText(event.data));
    if (!projection.text) return;
    setActivity(projection.kind === "terminal" ? "terminal" : "streaming");
    scheduleReadyState();
    setMessages(session.id, (current) => {
      if (projection.kind === "terminal") {
        if (current.some((message) => message.id === terminalStatusMessageID)) return current;
        const withoutEmpty = current.filter((message) => message.id !== "empty");
        return [...withoutEmpty, { id: terminalStatusMessageID, role: "system", text: projection.text, ts: event.ts }];
      }
      const last = current[current.length - 1];
      if (last?.role === "assistant" && last.id !== "snapshot") {
        return [...current.slice(0, -1), { ...last, text: [last.text, projection.text].filter(Boolean).join("\n") }];
      }
      return [...current, { id: event.id, role: "assistant", text: projection.text, ts: event.ts }];
    });
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    await sendPrompt();
  }

  async function sendPrompt() {
    const value = prompt.trim();
    if (!value || !canSend) return;
    setPrompt("");
    setActivity("thinking");
    setActivityDetail(`${session.adapter_name || session.adapter_id} is receiving the prompt.`);
    try {
      await api.sendPrompt(session.id, value);
    } catch (err) {
      setPrompt(value);
      onError((err as Error).message);
    }
  }

  async function submitSemanticAction(event: EventEnvelope, action: SemanticAction) {
    const key = `${event.id}:${action.id}`;
    if (action.kind === "ui" && action.id === "open_terminal") {
      onOpenTerminal();
      setActionState((current) => ({ ...current, [key]: "Opened" }));
      return;
    }
    setActionState((current) => ({ ...current, [key]: "Submitting" }));
    try {
      await api.submitAction(session.id, event.id, action);
      setActionState((current) => ({ ...current, [key]: "Denied" }));
    } catch (err) {
      const message = (err as Error).message;
      setActionState((current) => ({ ...current, [key]: message }));
      onError(message);
    }
  }

  function clearActivityTimer() {
    if (activityTimer.current === null) return;
    window.clearTimeout(activityTimer.current);
    activityTimer.current = null;
  }

  function scheduleReadyState() {
    clearActivityTimer();
    activityTimer.current = window.setTimeout(() => {
      if (isLive(session.status)) setActivity("ready");
      activityTimer.current = null;
    }, 1400);
  }

  function handlePromptKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return;
    event.preventDefault();
    void sendPrompt();
  }

  async function runAction(action: string) {
    setSlashOpen(false);
    try {
      if (action === "interrupt") await api.interrupt(session.id);
      if (action === "terminate") {
        if (!window.confirm(`Terminate ${session.name || session.command}?`)) return;
        await api.terminate(session.id);
      }
      if (action === "kill") {
        const confirmation = window.prompt(`Force kill ${session.name || session.command}? Type KILL to continue.`);
        if (confirmation !== "KILL") return;
        await api.kill(session.id);
      }
      if (action === "escape") await api.key(session.id, "Escape");
      if (action === "ctrlc") await api.key(session.id, "CtrlC");
      if (action === "tab") await api.key(session.id, "Tab");
      if (action === "enter") await api.key(session.id, "Enter");
      if (action === "terminal") onOpenTerminal();
      if (action === "clear") setMessages(session.id, []);
      if (action === "snapshot") {
        if (semanticAdapter) {
          const history = await api.events(session.id);
          setMessages(session.id, semanticMessages(history));
          history.forEach(onEvent);
        } else {
          const snapshot = await api.snapshot(session.id);
          const projection = projectTerminalOutputForChat(snapshot.chunks.map((chunk) => decodeBase64(chunk.bytes)).join(""));
          setMessages(session.id, snapshotMessages(projection, new Date().toISOString()));
        }
      }
      await onSessionUpdate();
    } catch (err) {
      onError((err as Error).message);
    }
  }

  return (
    <section className="chat-view" aria-label="Chat mode">
      <div className="chat-main">
        <div className="chat-status-row">
          <span className={activityClassName(activity)}>{activityLabel(activity)}</span>
          <span>{activityDetail || activityDescription(activity)}</span>
          <button onClick={onOpenTerminal}>Open Terminal</button>
        </div>
        {semanticAdapter && metadata && (
          <div className="semantic-strip" aria-label="Codex metadata">
            {metadata.model && <span>Model {metadata.model}</span>}
            {metadata.version && <span>Codex {metadata.version}</span>}
            {metadata.working_directory && <span title={metadata.working_directory}>{metadata.working_directory}</span>}
          </div>
        )}
        {approvals.map((event) => {
          const data = event.data as SemanticEventData;
          return (
            <section className="approval-card" key={event.id} aria-label="Approval required">
              <div>
                <strong>Approval required</strong>
                <p>{data.prompt || "Codex is waiting for a decision."}</p>
                {data.command && <code>$ {data.command}</code>}
                {data.working_directory && <small>{data.working_directory}</small>}
              </div>
              <div className="action-buttons">
                {(data.actions || []).map((action) => (
                  <button
                    key={action.id}
                    className={action.danger || action.style === "danger" ? "danger-button" : undefined}
                    onClick={() => submitSemanticAction(event, action)}
                  >
                    {actionState[`${event.id}:${action.id}`] || action.label}
                  </button>
                ))}
              </div>
            </section>
          );
        })}
        <div className="transcript" ref={transcriptRef}>
          {messages.length === 0 ? (
            <div className="message system-message">
              {semanticAdapter
                ? "Waiting for semantic events. Raw terminal output remains available in Terminal Mode."
                : "Readable output will appear here as terminal chunks arrive."}
            </div>
          ) : (
            messages.map((message) => (
              <article key={message.id} className={`message message-${message.role}`}>
                <div className="message-role">{message.role}</div>
                <pre>{message.text}</pre>
              </article>
            ))
          )}
        </div>
      </div>
      <form className="composer" onSubmit={submit}>
        <div className="composer-actions">
          <button type="button" className="slash-button" onClick={() => setSlashOpen((open) => !open)} aria-expanded={slashOpen}>
            /
          </button>
          <SlashCommandMenu open={slashOpen} onAction={runAction} />
        </div>
        <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} onKeyDown={handlePromptKeyDown} placeholder={semanticAdapter && !canSend ? "Waiting for the harness to become ready" : "Send input to the harness"} disabled={!canSend} />
        <button className="primary-button" disabled={!canSend || prompt.trim() === ""}>Send</button>
      </form>
    </section>
  );
}

function semanticMessages(eventList: EventEnvelope[]): ChatMessage[] {
  return eventList.map(semanticMessage).filter((message): message is ChatMessage => message !== null);
}

function semanticMessage(event: EventEnvelope): ChatMessage | null {
  const data = event.data as SemanticEventData | undefined;
  if (event.type === "chat.user_message" || event.type === "chat.assistant_message" || event.type === "chat.system_message") {
    const fallbackRole = event.type === "chat.user_message" ? "user" : event.type === "chat.assistant_message" ? "assistant" : "system";
    return data?.content ? { id: data.message_id || event.id, role: data.role || fallbackRole, text: data.content, ts: event.ts } : null;
  }
  if (event.type === "adapter.warning" || event.type === "adapter.error") {
    const text = data?.description || data?.detail || data?.content;
    return text ? { id: event.id, role: "system", text, ts: event.ts } : null;
  }
  return null;
}

function mergeMessages(current: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  const byID = new Map(current.filter((message) => message.id !== "empty").map((message) => [message.id, message]));
  for (const message of incoming) byID.set(message.id, message);
  return [...byID.values()].sort((left, right) => left.ts.localeCompare(right.ts));
}

function snapshotMessages(projection: ReturnType<typeof projectTerminalOutputForChat>, ts: string): ChatMessage[] {
  if (projection.text) {
    return [{ id: projection.kind === "terminal" ? terminalStatusMessageID : "snapshot", role: projection.kind === "terminal" ? "system" : "assistant", text: projection.text, ts }];
  }
  return [{ id: "empty", role: "system", text: "No readable output yet. The raw terminal remains available as the source of truth.", ts }];
}

function activityClassName(activity: ActivityState): string {
  return `stream-state activity-${activity} ${activity === "ready" || activity === "streaming" ? "is-connected" : ""}`.trim();
}

function activityLabel(activity: ActivityState): string {
  switch (activity) {
    case "connecting": return "Connecting";
    case "thinking": return "Processing";
    case "streaming": return "Streaming text";
    case "terminal": return "Terminal UI active";
    case "approval": return "Approval required";
    case "ended": return "Session ended";
    case "ready": return "Ready";
  }
}

function activityDescription(activity: ActivityState): string {
  switch (activity) {
    case "connecting": return "Opening the live event stream";
    case "thinking": return "Waiting for the harness to respond";
    case "streaming": return "Plain text output is being added below";
    case "terminal": return "Live screen output is available in Terminal Mode";
    case "approval": return "Review the requested operation before deciding";
    case "ended": return "This harness process has exited";
    case "ready": return "Connected and waiting for input";
  }
}
