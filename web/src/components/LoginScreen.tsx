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
      <form className="login-panel" onSubmit={submit}>
        <div className="brand-lockup">
          <LogoMark variant="wordmark" />
          <div>
            <h1 className="visually-hidden">HarnessRelay</h1>
            <p>Enter the local dashboard token from the daemon startup log.</p>
          </div>
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

export function LogoMark({ variant = "mark" }: { variant?: "mark" | "wordmark" }) {
  return <img className={variant === "wordmark" ? "logo-wordmark" : "logo-mark"} src={variant === "wordmark" ? logoWithText : logoWithoutText} alt="" aria-hidden="true" />;
}
