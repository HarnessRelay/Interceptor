import type { SessionStatus } from "../types";

export function StatusBadge({ status, compact = false }: { status: SessionStatus; compact?: boolean }) {
  if (compact) {
    return (
      <span className={`status-dot status-${status}`} title={status}>
        <span className="visually-hidden">{status}</span>
      </span>
    );
  }
  return <span className={`status-pill status-${status}`}><span aria-hidden="true" className="status-badge-dot" />{status}</span>;
}
