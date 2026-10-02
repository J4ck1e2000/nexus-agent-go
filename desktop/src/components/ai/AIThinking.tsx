/** Pulsing "thinking" label with three animated dots. */
export default function AIThinking({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2" role="status">
      <span className="text-xs text-muted">{label}</span>
      <span className="flex items-center gap-0.5" aria-hidden="true">
        {[0, 1, 2].map((index) => (
          <span
            key={index}
            className="h-1.5 w-1.5 rounded-full bg-accent"
            style={{ animation: `aiDotPulse 1.1s ease-in-out ${index * 0.18}s infinite` }}
          />
        ))}
      </span>
    </div>
  );
}
