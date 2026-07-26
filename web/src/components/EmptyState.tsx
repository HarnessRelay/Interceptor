export function EmptyState({ loading }: { loading: boolean }) {
  return (
    <div className="empty-state">
      <h2>{loading ? "Loading sessions" : "Create a session"}</h2>
      <p>{loading ? "Checking the local daemon." : "Choose a detected harness in the sidebar, or open Manual for an exact command."}</p>
    </div>
  );
}
