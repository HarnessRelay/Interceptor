export function EmptyState({ loading, onCreate }: { loading: boolean; onCreate: () => void }) {
  return (
    <div className="empty-state">
      <div className="empty-state-mark" aria-hidden="true"><span>›_</span></div>
      <h2>{loading ? "Loading your sessions" : "Start a local harness"}</h2>
      <p>{loading ? "Checking the local daemon." : "Start a detected coding harness or open a shell, with the complete terminal always one click away."}</p>
      {!loading && <button className="primary-button" type="button" onClick={onCreate}>New session</button>}
      {!loading && (
        <div className="empty-suggestions" aria-label="Session examples">
          <span><strong>Detected harness</strong> Semantic chat when supported</span>
          <span><strong>/bin/bash</strong> Universal fallback</span>
          <span><strong>Custom</strong> Any PTY command</span>
        </div>
      )}
    </div>
  );
}
