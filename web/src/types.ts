export type SessionStatus = "starting" | "running" | "exited" | "failed" | "terminated";

export type ViewMode = "chat" | "terminal";

export type Session = {
  id: string;
  name?: string;
  harness_type: string;
  adapter_id: string;
  command: string;
  args: string[];
  cwd: string;
  status: SessionStatus;
  terminal: { rows: number; cols: number };
  created_at: string;
  updated_at: string;
  exited_at?: string;
  exit_code?: number;
};

export type EventEnvelope = {
  id: string;
  type: string;
  session_id?: string;
  seq: number;
  ts: string;
  data?: unknown;
};

export type SemanticAction = {
  id: string;
  label: string;
  style?: "primary" | "secondary" | "danger";
  version?: number;
  requires_event_id?: boolean;
};

export type SemanticEventData = {
  title?: string;
  summary?: string;
  description?: string;
  confidence?: string;
  actions?: SemanticAction[];
};

export type Snapshot = {
  session_id: string;
  rows: number;
  cols: number;
  latest_seq: number;
  history_truncated: boolean;
  chunks: Array<{ seq: number; encoding: "base64"; bytes: string }>;
};

export type CreateForm = {
  name: string;
  harness_type?: string;
  command: string;
  args: string;
  cwd: string;
  mode: ViewMode;
};

export type HarnessPreset = {
  id: string;
  name: string;
  command: string;
  args: string[];
  installed: boolean;
  path?: string;
  version?: string;
  default_mode: ViewMode;
  description: string;
};

export type AuthStatus = {
  authenticated: boolean;
  csrf_token?: string;
};
