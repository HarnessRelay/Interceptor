import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { TunnelBinary, TunnelConfig, TunnelInfo } from "../types";

export function TunnelTab({ onError }: { onError: (message: string) => void }) {
  const [config, setConfig] = useState<TunnelConfig | null>(null);
  const [info, setInfo] = useState<TunnelInfo>({ status: "stopped" });
  const [binary, setBinary] = useState<TunnelBinary | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [copied, setCopied] = useState(false);

  // Config form state.
  const [mode, setMode] = useState<"quick" | "token">("quick");
  const [token, setToken] = useState("");
  const [hostname, setHostname] = useState("");

  const logsRef = useRef<HTMLDivElement>(null);
  const running = info.status === "running" || info.status === "starting";

  const refresh = useCallback(async () => {
    try {
      const [nextConfig, nextInfo, nextBinary, nextLogs] = await Promise.all([
        api.tunnelConfig(),
        api.tunnelStatus(),
        api.tunnelBinary(),
        api.tunnelLogs()
      ]);
      setConfig(nextConfig);
      setInfo(nextInfo);
      setBinary(nextBinary);
      setLogs(nextLogs);
    } catch (err) {
      onError((err as Error).message);
    }
  }, [onError]);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  useEffect(() => {
    if (config) {
      setMode(config.mode);
      setHostname(config.hostname || "");
    }
  }, [config]);

  useEffect(() => {
    if (logsRef.current) {
      logsRef.current.scrollTop = logsRef.current.scrollHeight;
    }
  }, [logs]);

  const start = async () => {
    setBusy(true);
    try {
      setInfo(await api.tunnelStart());
      refresh();
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const stop = async () => {
    setBusy(true);
    try {
      setInfo(await api.tunnelStop());
      refresh();
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const saveConfig = async (event: FormEvent) => {
    event.preventDefault();
    try {
      await api.updateTunnelConfig({
        mode,
        ...(mode === "token" && token.trim() ? { token: token.trim() } : {}),
        ...(mode === "token" && hostname.trim() ? { hostname: hostname.trim() } : {})
      });
      setToken("");
      await refresh();
    } catch (err) {
      onError((err as Error).message);
    }
  };

  const download = async () => {
    setDownloading(true);
    try {
      await api.tunnelDownload();
      await refresh();
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setDownloading(false);
    }
  };

  const copyURL = async () => {
    if (!info.url) return;
    try {
      await navigator.clipboard.writeText(info.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be unavailable; the URL input remains selectable.
    }
  };

  return (
    <div className="settings-sections">
      <section className="settings-section" aria-labelledby="tunnel-status-heading">
        <h3 id="tunnel-status-heading">Tunnel status</h3>
        <div className="tunnel-status-line">
          <span className={`tunnel-dot ${info.status}`} aria-hidden="true" />
          <span className="tunnel-status-text">{info.status}</span>
          {info.error && <span className="tunnel-error-text" role="alert">{info.error}</span>}
        </div>
        {info.status === "running" && info.url && (
          <div className="tunnel-url-line">
            <input className="tunnel-url-field" value={info.url} readOnly onClick={(e) => (e.target as HTMLInputElement).select()} aria-label="Public tunnel URL" />
            <button type="button" className="quiet-button" onClick={copyURL}>{copied ? "Copied" : "Copy"}</button>
          </div>
        )}
        <div className="tunnel-controls">
          {running ? (
            <button type="button" className="danger-button" onClick={stop} disabled={busy}>
              {busy ? "Stopping…" : "Stop tunnel"}
            </button>
          ) : (
            <button type="button" className="primary-button" onClick={start} disabled={busy || !binary?.path}>
              {busy ? "Starting…" : "Start tunnel"}
            </button>
          )}
          {info.status === "running" && (
            <p className="settings-hint">Anyone with this URL reaches the login screen; only approved devices can sign in.</p>
          )}
        </div>
      </section>

      <section className="settings-section" aria-labelledby="tunnel-config-heading">
        <h3 id="tunnel-config-heading">Configuration</h3>
        <form onSubmit={saveConfig} className="tunnel-config-form">
          <div className="segmented-control tunnel-mode-select" role="radiogroup" aria-label="Tunnel mode">
            <button
              type="button"
              role="radio"
              aria-checked={mode === "quick"}
              className={mode === "quick" ? "is-selected" : ""}
              onClick={() => setMode("quick")}
            >
              Quick Tunnel
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={mode === "token"}
              className={mode === "token" ? "is-selected" : ""}
              onClick={() => setMode("token")}
            >
              Named tunnel
            </button>
          </div>
          <p className="settings-hint">
            {mode === "quick"
              ? "Zero-config random trycloudflare.com URL. Best for temporary access; the URL changes on every start."
              : "Stable URL from a remotely-managed Cloudflare tunnel. Paste the tunnel token from the Zero Trust dashboard."}
          </p>
          {mode === "token" && (
            <>
              <label>
                <span>Tunnel token {config?.token_set && "(stored)"}</span>
                <input
                  value={token}
                  onChange={(event) => setToken(event.target.value)}
                  type="password"
                  placeholder={config?.token_set ? "Leave blank to keep the stored token" : "Paste the cloudflared tunnel token"}
                  autoComplete="off"
                />
              </label>
              <label>
                <span>Public hostname (optional, shown as the tunnel URL)</span>
                <input
                  value={hostname}
                  onChange={(event) => setHostname(event.target.value)}
                  placeholder="https://relay.example.com"
                />
              </label>
            </>
          )}
          <div className="tunnel-config-actions">
            <button type="submit" className="primary-button" disabled={running}>
              {running ? "Stop the tunnel to change config" : "Save configuration"}
            </button>
          </div>
        </form>
      </section>

      <section className="settings-section" aria-labelledby="tunnel-binary-heading">
        <h3 id="tunnel-binary-heading">cloudflared binary</h3>
        <dl className="binary-facts">
          <div>
            <dt>Version</dt>
            <dd>{binary?.version || "not installed"}</dd>
          </div>
          <div>
            <dt>Source</dt>
            <dd>{binary?.source || "—"}</dd>
          </div>
          <div>
            <dt>Path</dt>
            <dd><code>{binary?.path || binary?.managed_path}</code></dd>
          </div>
        </dl>
        <button type="button" className="quiet-button" onClick={download} disabled={downloading}>
          {downloading ? "Downloading…" : binary?.source === "managed" ? "Check for updates & update" : "Download cloudflared"}
        </button>
        <p className="settings-hint">
          Downloads are verified against Cloudflare's published checksum and installed atomically; the previous binary is kept as a fallback.
        </p>
      </section>

      <section className="settings-section" aria-labelledby="tunnel-logs-heading">
        <h3 id="tunnel-logs-heading">Debug console</h3>
        <div className="tunnel-console custom-scrollbar" ref={logsRef} aria-live="off" role="log" aria-label="cloudflared output">
          {logs.length === 0 ? (
            <p className="settings-empty">No tunnel output yet. Logs appear when the tunnel starts.</p>
          ) : (
            logs.map((line, index) => <pre key={index}>{line}</pre>)
          )}
        </div>
      </section>
    </div>
  );
}
