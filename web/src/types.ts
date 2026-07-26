export type SessionStatus = "starting" | "running" | "exited" | "failed" | "terminated";

export type ViewMode = "chat" | "terminal";

export type Session = {
  id: string;
  name?: string;
  harness_type: string;
  adapter_id: string;
  adapter_name: string;
  adapter_capabilities: string[];
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
  kind?: "approval" | "input" | "ui" | string;
  style?: "primary" | "secondary" | "danger";
  danger?: boolean;
  version?: number;
  requires_event_id?: boolean;
};

export type SemanticEventData = {
  message_id?: string;
  title?: string;
  summary?: string;
  description?: string;
  confidence?: number | string;
  role?: "user" | "assistant" | "system";
  content?: string;
  source?: string;
  status?: string;
  detail?: string;
  model?: string;
  version?: string;
  working_directory?: string;
  requires_terminal?: boolean;
  blocks_prompt?: boolean;
  operation_kind?: string;
  command?: string;
  prompt?: string;
  approval_event_id?: string;
  action_id?: string;
  resolution?: string;
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
  rows?: number;
  cols?: number;
  env?: Record<string, string>;
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

export type HarnessCommandInteraction = "submit" | "submit_then_terminal" | "prefill_terminal" | "insert";

export type HarnessCommand = {
  id: string;
  invocation: string;
  label: string;
  description: string;
  group: string;
  interaction: HarnessCommandInteraction;
  argument_hint?: string;
  danger?: boolean;
  availability?: string;
  availability_note?: string;
};

export type CommandCatalog = {
  supported: boolean;
  commands: HarnessCommand[];
  fallback?: "terminal" | string;
};

export type AuthStatus = {
  authenticated: boolean;
  csrf_token?: string;
};
