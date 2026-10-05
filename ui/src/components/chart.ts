// Recharts props for the dark surfaces: quiet axes, hairline grid, and a
// tooltip that looks like the rest of the dashboard.

export const axisProps = {
  stroke: "#34343a",
  tick: { fill: "#8a8f98", fontSize: 12 },
  tickLine: false,
  axisLine: false,
} as const;

export const gridProps = {
  stroke: "#23252a",
  vertical: false,
} as const;

export const tooltipProps = {
  cursor: { fill: "rgba(255, 255, 255, 0.04)" },
  contentStyle: {
    background: "#18191a",
    border: "1px solid #34343a",
    borderRadius: 10,
    boxShadow: "0 12px 32px rgba(0, 0, 0, 0.6)",
    padding: "8px 12px",
    fontSize: 13,
  },
  labelStyle: { color: "#f7f8f8", fontWeight: 600, marginBottom: 4 },
  itemStyle: { color: "#d0d6e0", padding: "1px 0" },
} as const;
