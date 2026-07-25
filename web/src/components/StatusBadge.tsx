import type { SessionStatus } from "../types";

export function StatusBadge({ status, compact = false }: { status: SessionStatus; compact?: boolean }) {
  if (compact) {
    return <span className={`status-dot status-${status}`} aria-label={status} title={status} />;
  }
  return <span className={`status-pill status-${status}`}>{status}</span>;
}
