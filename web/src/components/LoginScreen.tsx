import { FormEvent, useEffect, useRef, useState } from "react";
import { api, setDeviceToken } from "../api/client";
import type { AuthStatus } from "../types";
import logoWithText from "../assets/HarnessRelay_With_Text.png";
import logoWithoutText from "../assets/HarnessRelay_Without_Text.png";

export function LoginScreen({
  loading,
  error,
  authStatus,
  onLogin,
  onDeviceAuthenticated,
  onError
}: {
  loading: boolean;
  error: string | null;
  authStatus: AuthStatus | null;
  onLogin: (token: string) => Promise<void>;
  onDeviceAuthenticated: (csrfToken: string) => Promise<void>;
  onError: (message: string | null) => void;
}) {
  const tokenLoginAllowed = authStatus?.token_login_allowed !== false;

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
        {tokenLoginAllowed ? (
          <TokenForm loading={loading} error={error} onLogin={onLogin} onError={onError} />
        ) : (
          <DeviceAccessForm
            clientClass={authStatus?.client_class || "lan"}
            error={error}
            onDeviceAuthenticated={onDeviceAuthenticated}
            onError={onError}
          />
        )}
        <footer className="login-footer">
          <span><span className="dot" aria-hidden="true" /> 127.0.0.1 only</span>
          <span>Terminal source of truth</span>
        </footer>
      </section>
    </main>
  );
}

function TokenForm({
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
  );
}

type PairingPhase = "name" | "waiting" | "accepted" | "rejected" | "expired";

function DeviceAccessForm({
  clientClass,
  error,
  onDeviceAuthenticated,
  onError
}: {
  clientClass: string;
  error: string | null;
  onDeviceAuthenticated: (csrfToken: string) => Promise<void>;
  onError: (message: string | null) => void;
}) {
  const [deviceName, setDeviceName] = useState("");
  const [phase, setPhase] = useState<PairingPhase>("name");
  const [submitting, setSubmitting] = useState(false);
  const pairingRef = useRef<{ id: string; secret: string } | null>(null);

  // Poll the pairing request until it resolves.
  useEffect(() => {
    if (phase !== "waiting" || !pairingRef.current) return;
    let cancelled = false;
    const interval = setInterval(async () => {
      if (!pairingRef.current) return;
      try {
        const poll = await api.pollWebPairing(pairingRef.current.id, pairingRef.current.secret);
        if (cancelled) return;
        if (poll.status === "accepted" && poll.device_token) {
          clearInterval(interval);
          setDeviceToken(poll.device_token);
          try {
            const session = await api.mintDeviceSession();
            await onDeviceAuthenticated(session.csrf_token || "");
          } catch (err) {
            onError((err as Error).message);
            setPhase("name");
          }
          return;
        }
        if (poll.status === "rejected" || poll.status === "expired" || poll.status === "unknown") {
          clearInterval(interval);
          setPhase(poll.status === "rejected" ? "rejected" : "expired");
        }
      } catch {
        // Transient poll errors are ignored; the interval retries.
      }
    }, 2000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [phase, onDeviceAuthenticated, onError]);

  async function requestAccess(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    onError(null);
    try {
      const submit = await api.requestWebPairing(deviceName);
      pairingRef.current = { id: submit.request_id, secret: submit.secret };
      setPhase("waiting");
      setPairingCode(submit.code);
    } catch (err) {
      onError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  const [pairingCode, setPairingCode] = useState<string | null>(null);

  return (
    <form className="login-panel" onSubmit={requestAccess}>
      <div className="login-form-heading">
        <span className="connection-mark remote" aria-hidden="true"><span className="dot" /></span>
        <div>
          <h2>Request device access</h2>
          <p>
            You are connecting {clientClass === "tunnel" ? "through a secure tunnel" : "from the network"}.
            The static token only works on the host machine—ask the daemon to approve this device.
          </p>
        </div>
      </div>

      {phase === "name" && (
        <>
          <label>
            <span>This device's name</span>
            <input
              value={deviceName}
              onChange={(event) => setDeviceName(event.target.value)}
              disabled={submitting}
              placeholder='e.g. "Kitchen Tablet"'
              autoFocus
            />
          </label>
          {error && <div className="login-error" role="alert">{error}</div>}
          <button className="primary-button login-submit" disabled={submitting || deviceName.trim() === ""}>
            {submitting ? "Requesting…" : "Request access"}
            <span aria-hidden="true">→</span>
          </button>
          <p className="login-help">Approval happens on the daemon dashboard with a matching 6-digit code.</p>
        </>
      )}

      {phase === "waiting" && pairingCode && (
        <div className="pairing-code-block" aria-live="polite">
          <p className="pairing-code-label">Show this code on the daemon's approval dialog, then approve when both codes match:</p>
          <p className="pairing-code-digits" aria-label={`Verification code ${pairingCode}`}>{pairingCode}</p>
          <p className="login-help">Waiting for approval…</p>
        </div>
      )}

      {(phase === "rejected" || phase === "expired") && (
        <>
          <div className="login-error" role="alert">
            {phase === "rejected"
              ? "The request was rejected on the daemon. You can try again."
              : "The request expired. You can try again."}
          </div>
          <button
            type="button"
            className="primary-button login-submit"
            onClick={() => {
              setPhase("name");
              pairingRef.current = null;
              setPairingCode(null);
            }}
          >
            Try again
          </button>
        </>
      )}
    </form>
  );
}

export function LogoMark({ variant = "mark" }: { variant?: "mark" | "wordmark" }) {
  return <img className={variant === "wordmark" ? "logo-wordmark" : "logo-mark"} src={variant === "wordmark" ? logoWithText : logoWithoutText} alt="" aria-hidden="true" loading="lazy" />;
}
