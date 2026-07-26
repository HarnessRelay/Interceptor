import { useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import { api } from "../api/client";
import type { EventEnvelope, Session, Snapshot } from "../types";
import { decodeBase64Bytes, isLive } from "../utils";

export function TerminalView({
  session,
  onOpenChat,
  onSessionUpdate,
  onEvent,
  onError
}: {
  session: Session;
  onOpenChat: () => void;
  onSessionUpdate: () => void;
  onEvent: (event: EventEnvelope) => void;
  onError: (message: string) => void;
}) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const requestFitRef = useRef<() => void>(() => {});
  const latestSeq = useRef(0);
  const [rawInput, setRawInput] = useState("");
  const [connected, setConnected] = useState(false);
  const streamLive = connected && isLive(session.status);
  const streamLabel = !isLive(session.status) ? "Snapshot" : connected ? "Live" : "Connecting";

  useEffect(() => {
    if (!hostRef.current) return;
    const term = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      fontSize: 13,
      theme: {
        background: "#080d14",
        foreground: "#f4f7fb",
        cursor: "#16e0b5",
        selectionBackground: "#263244"
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
    let acceptInput = session.status === "starting" || session.status === "running";
    let lastRows = 0;
    let lastCols = 0;

    const scheduleFit = () => {
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        if (disposed || !fitRef.current || !terminalRef.current) return;
        fitRef.current.fit();
        const activeTerm = terminalRef.current;
        if (activeTerm.rows === lastRows && activeTerm.cols === lastCols) return;
        lastRows = activeTerm.rows;
        lastCols = activeTerm.cols;
        api.resize(session.id, activeTerm.rows, activeTerm.cols).catch((err) => onError(err.message));
      }, 90);
    };
    requestFitRef.current = scheduleFit;
    const onWindowResize = () => scheduleFit();
    window.addEventListener("resize", onWindowResize);

    term.onData((data) => {
      if (!acceptInput) return;
      api.input(session.id, data).catch((err) => onError(err.message));
    });

    const writeSnapshot = (snapshot: Snapshot, reset = false) => {
      latestSeq.current = snapshot.latest_seq || 0;
      if (reset) term.reset();
      for (const chunk of snapshot.chunks) {
        term.write(decodeBase64Bytes(chunk.bytes));
      }
    };

    api.snapshot(session.id)
      .then((snapshot) => {
        writeSnapshot(snapshot);
        fit.fit();
        lastRows = term.rows;
        lastCols = term.cols;
        return api.resize(session.id, term.rows, term.cols);
      })
      .catch((err: Error) => onError(err.message))
      .finally(() => {
        if (disposed) return;
        onSessionUpdate();
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws?session_id=${encodeURIComponent(session.id)}&after_seq=${latestSeq.current}`);
        socket.onopen = () => setConnected(true);
        socket.onclose = () => setConnected(false);
        socket.onerror = () => onError("WebSocket stream failed.");
        socket.onmessage = (message) => {
          const event = JSON.parse(message.data) as EventEnvelope;
          latestSeq.current = Math.max(latestSeq.current, event.seq || 0);
          onEvent(event);
          if (event.type === "session.exited") acceptInput = false;
          if (event.type === "terminal.output") {
            const payload = event.data as { data?: string; bytes?: string };
            const encoded = payload?.bytes || payload?.data;
            if (encoded) term.write(decodeBase64Bytes(encoded));
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

    return () => {
      disposed = true;
      acceptInput = false;
      window.clearTimeout(resizeTimer);
      window.removeEventListener("resize", onWindowResize);
      socket?.close();
      term.dispose();
      terminalRef.current = null;
      fitRef.current = null;
      requestFitRef.current = () => {};
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
    <section className="terminal-section" aria-label="Terminal mode">
      <div className="terminal-bar">
        <div className="terminal-identity">
          <span className={streamLive ? "stream-state is-connected" : "stream-state"}><span className="activity-dot" aria-hidden="true" />{streamLabel}</span>
          <span className="terminal-dimensions">{session.terminal.rows} rows × {session.terminal.cols} columns</span>
        </div>
        <button className="quiet-button" onClick={onOpenChat}>Open Chat</button>
      </div>
      <div className="terminal-host" ref={hostRef} aria-label="Interactive terminal" />
      <details className="raw-input-fallback" onToggle={() => {
        window.requestAnimationFrame(() => requestFitRef.current());
      }}>
        <summary>Raw input fallback</summary>
        <div className="raw-input">
          <label>
            <span className="visually-hidden">Raw terminal input</span>
            <textarea value={rawInput} onChange={(event) => setRawInput(event.target.value)} placeholder="Type or paste exact terminal input" />
          </label>
          <button onClick={sendRawInput}>Send input</button>
        </div>
      </details>
    </section>
  );
}
