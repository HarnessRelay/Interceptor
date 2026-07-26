export function AdapterBadge({ id, name, semantic = false }: { id: string; name?: string; semantic?: boolean }) {
  const label = name || id || "Generic";
  return (
    <span className={semantic ? "adapter-badge adapter-semantic" : "adapter-badge"}>
      <span className="badge-glyph" aria-hidden="true">{semantic ? "◆" : "◇"}</span>
      {label}
    </span>
  );
}
