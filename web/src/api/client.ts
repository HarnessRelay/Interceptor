import type { AuthStatus, CreateForm, EventEnvelope, HarnessPreset, SemanticAction, Session, Snapshot } from "../types";
import { encodeBase64, isUnsafeMethod, splitArgs } from "../utils";

let csrfToken = "";

export function setCSRFToken(token: string) {
  csrfToken = token;
}

export const api = {
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
  async listHarnesses(): Promise<HarnessPreset[]> {
    const data = await request<{ harnesses: HarnessPreset[] }>("/api/v1/harnesses");
    return data.harnesses;
  },
  async createSession(input: CreateForm): Promise<Session> {
    const data = await request<{ session: Session }>("/api/v1/sessions", {
      method: "POST",
      body: JSON.stringify({
        name: input.name || undefined,
        harness_type: input.harness_type || undefined,
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
  async sendPrompt(id: string, text: string): Promise<void> {
    await request(`/api/v1/sessions/${id}/prompt`, {
      method: "POST",
      body: JSON.stringify({ text })
    });
  },
  async key(id: string, key: string): Promise<void> {
    await request(`/api/v1/sessions/${id}/input`, {
      method: "POST",
      body: JSON.stringify({ mode: "key", key })
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
  async events(id: string, afterSeq = 0, limit = 1024): Promise<EventEnvelope[]> {
    const data = await request<{ events: EventEnvelope[] }>(
      `/api/v1/sessions/${id}/events?after_seq=${afterSeq}&limit=${limit}`
    );
    return data.events;
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
