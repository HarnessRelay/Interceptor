export function EmptyState({ loading }: { loading: boolean }) {
  return (
    <div className="empty-state">
      <h2>{loading ? "Loading sessions" : "Create a session"}</h2>
      <p>{loading ? "Checking the local daemon." : "Start in Chat Mode for guided interaction, or Terminal Mode for exact raw PTY control."}</p>
    </div>
  );
}
