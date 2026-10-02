export default function MiniProgress({
  value,
  max = 100,
  color = '#5f9189',
  track = '#e5dfd5',
}: {
  value: number | null;
  max?: number;
  color?: string;
  track?: string;
}) {
  const percent =
    value === null || !Number.isFinite(value)
      ? 0
      : Math.min(100, Math.max(0, (value / max) * 100));

  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full" style={{ background: track }}>
      <div
        className="h-full rounded-full transition-[width] duration-500"
        style={{ width: `${percent}%`, background: color }}
      />
    </div>
  );
}
