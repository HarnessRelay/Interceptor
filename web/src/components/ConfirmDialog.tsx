import { useEffect, useState } from "react";
import { Dialog } from "./Dialog";

export type Confirmation = {
  kind: "terminate" | "kill";
  label: string;
};

export function ConfirmDialog({
  confirmation,
  busy,
  onCancel,
  onConfirm
}: {
  confirmation: Confirmation | null;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const [phrase, setPhrase] = useState("");
  useEffect(() => setPhrase(""), [confirmation]);
  const force = confirmation?.kind === "kill";
  const valid = !force || phrase === "KILL";

  return (
    <Dialog
      open={confirmation !== null}
      title={force ? "Force kill session?" : "Terminate session?"}
      description={force
        ? `${confirmation?.label} will be stopped immediately. Unsaved harness state may be lost.`
        : `${confirmation?.label} will receive a graceful termination request.`}
      onClose={onCancel}
      initialFocus="cancel"
    >
      <div className="confirm-body">
        {force && (
          <label>
            <span>Type <strong>KILL</strong> to confirm</span>
            <input
              value={phrase}
              onChange={(event) => setPhrase(event.target.value)}
              autoComplete="off"
              aria-label="Type KILL to confirm force kill"
            />
          </label>
        )}
        <div className="dialog-actions">
          <button type="button" data-dialog-cancel="true" onClick={onCancel}>Cancel</button>
          <button
            className="danger-solid-button"
            type="button"
            disabled={!valid || busy}
            onClick={onConfirm}
          >
            {busy ? "Working…" : force ? "Force kill" : "Terminate"}
          </button>
        </div>
      </div>
    </Dialog>
  );
}
