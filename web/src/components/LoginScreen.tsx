import { FormEvent, useState } from "react";
import logoWithText from "../assets/HarnessRelay_With_Text.png";
import logoWithoutText from "../assets/HarnessRelay_Without_Text.png";

export function LoginScreen({
  loading,
  error,
  onLogin,
  onError
}: {
  loading: boolean;
  error: string | null;
  onLogin: (token: string) => Promise<void>;
  onError: (message: string | null) => void;
}) {
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
      <section className="login-product" aria-labelledby="login-heading">
        <div className="login-brand">
          <LogoMark />
          <span>HarnessRelay</span>
        </div>
        <div className="login-value">
          <p className="login-kicker">Local control plane</p>
          <h1 id="login-heading">Your coding harnesses,<br />under one roof.</h1>
          <p>Run, inspect, and steer terminal-based coding agents through a calm semantic workspace—with the raw terminal always available.</p>
        </div>
        <div className="security-note">
          <span className="security-icon" aria-hidden="true">⌾</span>
          <div>
            <strong>Local-first by design</strong>
            <p>HarnessRelay binds to your machine and protects every session with an access token.</p>
          </div>
        </div>
      </section>
      <section className="login-form-region" aria-label="Dashboard sign in">
        <form className="login-panel" onSubmit={submit}>
          <div className="login-form-heading">
            <span className="connection-mark" aria-hidden="true"><span className="dot" /></span>
            <div>
              <h2>Connect to daemon</h2>
              <p>Enter the token printed when <code>harnessd</code> started.</p>
            </div>
          </div>
          <label>
            <span>Dashboard token</span>
            <input className="visually-hidden" tabIndex={-1} autoComplete="username" value="local" readOnly aria-hidden="true" />
            <input
              value={token}
              onChange={(event) => setToken(event.target.value)}
              type="password"
              autoComplete="current-password"
              disabled={loading || submitting}
              placeholder="Paste your local access token"
              autoFocus
            />
          </label>
          {error && <div className="login-error" role="alert">{error}</div>}
          <button className="primary-button login-submit" disabled={loading || submitting || token.trim() === ""}>
            {submitting ? "Connecting…" : "Connect securely"}
            <span aria-hidden="true">→</span>
          </button>
          <p className="login-help">The token stays in an HttpOnly local session. It is never stored in browser storage.</p>
        </form>
        <footer className="login-footer">
          <span><span className="dot" aria-hidden="true" /> 127.0.0.1 only</span>
          <span>Terminal source of truth</span>
        </footer>
      </section>
    </main>
  );
}

export function LogoMark({ variant = "mark" }: { variant?: "mark" | "wordmark" }) {
  return <img className={variant === "wordmark" ? "logo-wordmark" : "logo-mark"} src={variant === "wordmark" ? logoWithText : logoWithoutText} alt="" aria-hidden="true" loading="lazy" />;
}
