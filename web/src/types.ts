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
  origin?: "shim" | string;
  origin_backend?: "pty" | "tmux" | "direct" | string;
  shim_name?: string;
  real_binary?: string;
  attachable: boolean;
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
  client_class?: "host" | "lan" | "tunnel";
  token_login_allowed?: boolean;
};

export type PairingRequest = {
  device_id: string;
  device_name: string;
  platform: string;
  public_key: string;
  type?: string;
  code?: string;
  received_at: string;
};

export type PairedDevice = {
  device_id: string;
  device_name: string;
  platform: string;
  public_key: string;
  type?: string;
  custom_name?: string;
  paired_at: string;
  last_seen: string;
};

export type WebPairingSubmit = {
  request_id: string;
  code: string;
  secret: string;
};

export type WebPairingPoll = {
  status: "pending" | "accepted" | "rejected" | "expired" | "unknown";
  device_token?: string;
};

export type NetworkSettings = {
  remote_access_enabled: boolean;
  lan_ips: string[];
  allowlist: string[];
  banlist: string[];
};

export type NetworkClient = {
  key: string;
  ip: string;
  class: "host" | "lan" | "tunnel";
  mac?: string;
  hostname?: string;
  custom_name?: string;
  first_seen: number;
  last_seen: number;
  active_connections: number;
};

export type DaemonIdentity = {
  device_id: string;
  device_name: string;
  public_key: string;
};

export type TunnelInfo = {
  status: "stopped" | "starting" | "running" | "error";
  url?: string;
  error?: string;
};

export type TunnelAvailable = {
  available: boolean;
  binary: string;
};

export type TunnelConfig = {
  mode: "quick" | "token";
  hostname?: string;
  token_set: boolean;
};

export type TunnelBinary = {
  path?: string;
  source?: "env" | "managed" | "path" | "common";
  version?: string;
  managed_path: string;
};

export type TunnelDownload = {
  version: string;
  path: string;
};
