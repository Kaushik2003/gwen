/** A ring showing worked time against the target. */
export default function ProgressRing({ fraction, color, label }: { fraction: number; color: string; label: string }) {
  const r = 52;
  const c = 2 * Math.PI * r;
  const f = Math.max(0, Math.min(1, fraction));
  return (
    <svg viewBox="0 0 120 120" className="h-36 w-36">
      <circle cx="60" cy="60" r={r} fill="none" strokeWidth="10" className="stroke-zinc-200 dark:stroke-zinc-800" />
      <circle
        cx="60"
        cy="60"
        r={r}
        fill="none"
        strokeWidth="10"
        stroke={color}
        strokeLinecap="round"
        strokeDasharray={`${f * c} ${c}`}
        transform="rotate(-90 60 60)"
      />
      <text x="60" y="58" textAnchor="middle" className="fill-current text-[18px] font-semibold">
        {Math.round(fraction * 100)}%
      </text>
      <text x="60" y="76" textAnchor="middle" className="fill-zinc-500 text-[10px]">
        {label}
      </text>
    </svg>
  );
}
