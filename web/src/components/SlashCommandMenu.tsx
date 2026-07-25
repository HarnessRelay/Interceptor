const actions = [
  { id: "interrupt", label: "Interrupt" },
  { id: "terminate", label: "Terminate" },
  { id: "kill", label: "Force kill" },
  { id: "escape", label: "Send Escape" },
  { id: "ctrlc", label: "Send Ctrl+C" },
  { id: "tab", label: "Send Tab" },
  { id: "enter", label: "Send Enter" },
  { id: "terminal", label: "Open Terminal Mode" },
  { id: "clear", label: "Clear local transcript" },
  { id: "snapshot", label: "Refresh snapshot" }
];

export function SlashCommandMenu({ open, onAction }: { open: boolean; onAction: (action: string) => void }) {
  if (!open) return null;
  return (
    <div className="slash-menu" role="menu">
      {actions.map((action) => (
        <button key={action.id} type="button" role="menuitem" onClick={() => onAction(action.id)}>
          {action.label}
        </button>
      ))}
    </div>
  );
}
