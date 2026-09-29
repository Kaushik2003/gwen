import { useEffect, useState } from "react";
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { App, readOnly as readOnlyUI, wire } from "../api";
import { Button, Card, Input } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDuration } from "../format";
import DayDetail from "./DayDetail";

/** Days by project over a range (default the last 7 days); a click opens Day detail. */
export default function History({ readOnly = readOnlyUI }: { readOnly?: boolean }) {
  const d = useDaemon();
  const today = d.today();
  const [from, setFrom] = useState(addDays(today, -6));
  const [to, setTo] = useState(today);
  const [days, setDays] = useState<wire.DaySummary[]>([]);
  const [open, setOpen] = useState<string | null>(null);

  useEffect(() => {
    App.ListDays(from, to).then((l) => setDays(l.days), d.fail);
  }, [from, to, d.daysVersion, d.fail]);

  if (open) return <DayDetail day={open} onBack={() => setOpen(null)} readOnly={readOnly} />;

  const names = new Map<string, { name: string; color: string }>();
  days.forEach((day) => day.by_project.forEach((p) => names.set(p.project_id ?? "none", { name: p.name, color: p.color })));
  const data = days.map((day) => {
    const row: Record<string, number | string> = { day: day.day.slice(5) };
    day.by_project.forEach((p) => (row[p.project_id ?? "none"] = +(p.worked_ms / 3_600_000).toFixed(2)));
    return row;
  });

  return (
    <div className="flex flex-col gap-4">
      <Card
        title="History"
        actions={
          <div className="flex items-center gap-2 text-sm">
            <Input type="date" value={from} max={to} onChange={(e) => e.target.value && setFrom(e.target.value)} />
            <span>to</span>
            <Input type="date" value={to} min={from} onChange={(e) => e.target.value && setTo(e.target.value)} />
          </div>
        }
      >
        {days.length === 0 ? (
          <p className="text-sm text-zinc-500">Nothing tracked in this range.</p>
        ) : (
          <div className="h-72">
            <ResponsiveContainer>
              <BarChart
                data={data}
                onClick={(e: any) => {
                  const i = e?.activeTooltipIndex;
                  if (typeof i === "number" && days[i]) setOpen(days[i].day);
                }}
              >
                <CartesianGrid strokeDasharray="3 3" strokeOpacity={0.3} />
                <XAxis dataKey="day" fontSize={12} />
                <YAxis unit="h" fontSize={12} />
                <Tooltip formatter={(v: any, k: any) => [`${v}h`, names.get(String(k))?.name ?? k]} />
                {[...names.entries()].map(([id, p]) => (
                  <Bar key={id} dataKey={id} stackId="w" fill={p.color} cursor="pointer" />
                ))}
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </Card>
      <Card title="Days">
        <ul className="divide-y divide-zinc-200 dark:divide-zinc-800">
          {[...days].reverse().map((day) => (
            <li key={day.day} className="flex items-center gap-4 py-2 text-sm">
              <span className="w-28 font-medium">{day.day}</span>
              <span className="w-24 tabular-nums">{formatDuration(day.worked_ms)}</span>
              <span className="w-24 text-zinc-500 tabular-nums">{formatDuration(day.break_ms)} break</span>
              <span className={day.target_met ? "text-emerald-600" : "text-zinc-400"}>{day.target_met ? "target met" : ""}</span>
              <Button className="ml-auto" onClick={() => setOpen(day.day)}>
                Open
              </Button>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
