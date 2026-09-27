import { useEffect, useState } from "react";
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import { App, wire } from "../api";
import Heatmap from "../components/Heatmap";
import { Button, Card, Input, Select } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDuration } from "../format";

type Range = "week" | "month" | "year" | "custom";

export default function Stats() {
  const d = useDaemon();
  const today = d.today();
  const [range, setRange] = useState<Range>("week");
  const [from, setFrom] = useState(addDays(today, -6));
  const [to, setTo] = useState(today);
  const [summary, setSummary] = useState<wire.StatsSummary | null>(null);
  const [year, setYear] = useState(Number(today.slice(0, 4)));
  const [heat, setHeat] = useState<wire.Heatmap | null>(null);

  function pick(r: Range) {
    setRange(r);
    if (r === "custom") return;
    setTo(today);
    setFrom(addDays(today, r === "week" ? -6 : r === "month" ? -29 : -365));
  }

  useEffect(() => {
    App.StatsSummary(from, to).then(setSummary, d.fail);
  }, [from, to, d.daysVersion, d.fail]);
  useEffect(() => {
    App.StatsHeatmap(year).then(setHeat, d.fail);
  }, [year, d.daysVersion, d.fail]);

  const target = parseDuration(d.config?.tracking.daily_target ?? "8h");
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        {(["week", "month", "year", "custom"] as Range[]).map((r) => (
          <Button key={r} tone={range === r ? "primary" : "plain"} onClick={() => pick(r)}>
            {r[0].toUpperCase() + r.slice(1)}
          </Button>
        ))}
        {range === "custom" && (
          <div className="ml-2 flex items-center gap-2 text-sm">
            <Input type="date" value={from} onChange={(e) => e.target.value && setFrom(e.target.value)} />
            to
            <Input type="date" value={to} onChange={(e) => e.target.value && setTo(e.target.value)} />
          </div>
        )}
      </div>
      {summary && (
        <>
          <div className="grid grid-cols-5 gap-3">
            <Stat label="Worked" value={formatDuration(summary.worked_ms)} />
            <Stat label="Average per tracked day" value={formatDuration(summary.avg_worked_ms)} />
            <Stat label="Days target met" value={`${summary.days_target_met} of ${summary.days_tracked}`} />
            <Stat label="Current streak" value={`${summary.current_streak} days`} />
            <Stat label="Longest streak" value={`${summary.longest_streak} days`} />
          </div>
          <Card title="By project">
            {summary.by_project.length === 0 ? (
              <p className="text-sm text-zinc-500">No work in this range.</p>
            ) : (
              <div className="flex items-center gap-6">
                <div className="h-56 w-56">
                  <ResponsiveContainer>
                    <PieChart>
                      <Pie data={summary.by_project} dataKey="worked_ms" nameKey="name" innerRadius={55} outerRadius={90} paddingAngle={1}>
                        {summary.by_project.map((p) => (
                          <Cell key={p.project_id ?? "none"} fill={p.color} />
                        ))}
                      </Pie>
                      <Tooltip formatter={(v: any) => formatDuration(Number(v))} />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
                <ul className="flex flex-col gap-2 text-sm">
                  {summary.by_project.map((p) => (
                    <li key={p.project_id ?? "none"} className="flex items-center gap-2">
                      <span className="inline-block h-3 w-3 rounded-sm" style={{ backgroundColor: p.color }} />
                      <span className="w-40">{p.name}</span>
                      <span className="tabular-nums">{formatDuration(p.worked_ms)}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </Card>
        </>
      )}
      <Card
        title="Year"
        actions={
          <Select value={year} onChange={(e) => setYear(Number(e.target.value))}>
            {[0, 1, 2, 3].map((i) => {
              const y = Number(today.slice(0, 4)) - i;
              return (
                <option key={y} value={y}>
                  {y}
                </option>
              );
            })}
          </Select>
        }
      >
        {heat && <Heatmap heatmap={heat} targetMs={target} />}
      </Card>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <div className="text-xs text-zinc-500">{label}</div>
      <div className="mt-1 text-xl font-semibold">{value}</div>
    </Card>
  );
}

/** Milliseconds of a Go duration string such as "8h" or "7h30m". */
export function parseDuration(s: string): number {
  let ms = 0;
  for (const [, n, unit] of s.matchAll(/(\d+(?:\.\d+)?)(h|m|s|ms)/g)) {
    ms += Number(n) * ({ h: 3_600_000, m: 60_000, s: 1000, ms: 1 } as Record<string, number>)[unit];
  }
  return ms;
}
