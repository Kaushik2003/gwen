// Recharts props for the dark surfaces: quiet axes, hairline grid, and a
// tooltip that looks like the rest of the dashboard.

export const axisProps = {
  stroke: "var(--color-line-strong)",
  tick: { fill: "var(--color-ink-subtle)", fontSize: 12 },
  tickLine: false,
  axisLine: false,
} as const;

export const gridProps = {
  stroke: "var(--color-line)",
  vertical: false,
} as const;

export const tooltipProps = {
  cursor: { fill: "rgba(255, 255, 255, 0.04)" },
  contentStyle: {
    background: "var(--color-surface-3)",
    border: "1px solid var(--color-line-strong)",
    borderRadius: 10,
    boxShadow: "0 12px 32px rgba(0, 0, 0, 0.6)",
    padding: "8px 12px",
    fontSize: 13,
  },
  labelStyle: { color: "var(--color-ink)", fontWeight: 600, marginBottom: 4 },
  itemStyle: { color: "var(--color-ink-muted)", padding: "1px 0" },
} as const;
