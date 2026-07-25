import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { EventEnvelope, Session } from "../types";
import { cleanTerminalText, decodeBase64, isLive, terminalOutputText } from "../utils";
import { SlashCommandMenu } from "./SlashCommandMenu";

type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  text: string;
  ts: string;
};

export function ChatView({
  session,
  events,
  onOpenTerminal,
  onSessionUpdate,
  onEvent,
  onError
}: {
  session: Session;
  events: EventEnvelope[];
  onOpenTerminal: () => void;
  onSessionUpdate: () => void;
  onEvent: (event: EventEnvelope) => void;
  onError: (message: string) => void;
}) {
  const [prompt, setPrompt] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [connected, setConnected] = useState(false);
  const [slashOpen, setSlashOpen] = useState(false);
  const latestSeq = useRef(0);
  const transcriptRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    let socket: WebSocket | null = null;
    let disposed = false;

    api.snapshot(session.id)
      .then((snapshot) => {
        latestSeq.current = snapshot.latest_seq || 0;
        const text = cleanTerminalText(snapshot.chunks.map((chunk) => decodeBase64(chunk.bytes)).join(""));
        if (text) {
          setMessages([{ id: "snapshot", role: "assistant", text, ts: new Date().toISOString() }]);
        } else {
          setMessages([{ id: "empty", role: "system", text: "No readable output yet. The raw terminal remains available as the source of truth.", ts: new Date().toISOString() }]);
        }
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
          if (event.type === "terminal.output") appendAssistantOutput(event);
          if (event.type === "session.exited" || event.type === "session.status_changed") onSessionUpdate();
        };
      });

    return () => {
      disposed = true;
      socket?.close();
      setConnected(false);
    };
  }, [session.id, onError, onEvent, onSessionUpdate]);

  useEffect(() => {
    transcriptRef.current?.scrollTo({ top: transcriptRef.current.scrollHeight });
  }, [messages]);

  const semanticEvents = useMemo(
    () => events.filter((event) => event.type === "approval.required" || event.type === "semantic.action_available"),
    [events]
  );

  function appendAssistantOutput(event: EventEnvelope) {
    const text = cleanTerminalText(terminalOutputText(event.data));
    if (!text) return;
    setMessages((current) => {
      const last = current[current.length - 1];
      if (last?.role === "assistant" && last.id !== "snapshot") {
        return [...current.slice(0, -1), { ...last, text: [last.text, text].filter(Boolean).join("\n") }];
      }
      return [...current, { id: event.id, role: "assistant", text, ts: event.ts }];
    });
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const value = prompt.trim();
    if (!value || !isLive(session.status)) return;
    setPrompt("");
    setMessages((current) => [...current, { id: `user-${Date.now()}`, role: "user", text: value, ts: new Date().toISOString() }]);
    try {
      await api.input(session.id, `${value}\n`);
    } catch (err) {
      onError((err as Error).message);
    }
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
      if (action === "clear") setMessages([]);
      if (action === "snapshot") {
        const snapshot = await api.snapshot(session.id);
        const text = cleanTerminalText(snapshot.chunks.map((chunk) => decodeBase64(chunk.bytes)).join(""));
        setMessages(text ? [{ id: `snapshot-${Date.now()}`, role: "assistant", text, ts: new Date().toISOString() }] : []);
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
          <span className={connected ? "stream-state is-connected" : "stream-state"}>{connected ? "Live output" : "Connecting"}</span>
          <span>Raw terminal is source of truth</span>
          <button onClick={onOpenTerminal}>Open Terminal</button>
        </div>
        {semanticEvents.length > 0 && (
          <div className="semantic-strip">
            {semanticEvents.slice(0, 2).map((event) => (
              <span key={event.id}>{event.type}</span>
            ))}
          </div>
        )}
        <div className="transcript" ref={transcriptRef}>
          {messages.length === 0 ? (
            <div className="message system-message">Readable output will appear here as terminal chunks arrive.</div>
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
        <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="Send input to the harness" disabled={!isLive(session.status)} />
        <button className="primary-button" disabled={!isLive(session.status) || prompt.trim() === ""}>Send</button>
      </form>
    </section>
  );
}
