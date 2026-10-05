import { CalendarDays, ChartPie, Clock, Flame, Target, Trophy, type LucideIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import { App, readOnly, wire } from "../api";
import { tooltipProps } from "../components/chart";
import Heatmap from "../components/Heatmap";
import Retro from "../components/Retro";
import { Dot, Input, Meter, PageHeader, Panel, Segmented } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDuration, parseDuration } from "../format";

type Range = "week" | "month" | "year" | "custom";

export default function Stats() {
  const d = useDaemon();
  const today = d.today();
  const thisYear = Number(today.slice(0, 4));
  const [range, setRange] = useState<Range>("week");
  const [from, setFrom] = useState(addDays(today, -6));
  const [to, setTo] = useState(today);
  const [summary, setSummary] = useState<wire.StatsSummary | null>(null);
  const [year, setYear] = useState(thisYear);
  const [heat, setHeat] = useState<wire.Heatmap | null>(null);

  function pick(r: Range) {
    setRange(r);
    if (r === "custom") return;
    setTo(today);
    setFrom(addDays(today, r === "week" ? -6 : r === "month" ? -29 : -364));
  }

  useEffect(() => {
    App.StatsSummary(from, to).then(setSummary, d.fail);
  }, [from, to, d.daysVersion, d.fail]);
  useEffect(() => {
    App.StatsHeatmap(year).then(setHeat, d.fail);
  }, [year, d.daysVersion, d.fail]);

  const target = parseDuration(d.config?.tracking.daily_target ?? "8h");
  const s = summary;
  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Stats"
        actions={
          <>
            <Segmented
              value={range}
              onChange={pick}
              label="Range"
              options={[
                { value: "week", label: "Week" },
                { value: "month", label: "Month" },
                { value: "year", label: "Year" },
                { value: "custom", label: "Custom" },
              ]}
            />
            {range === "custom" && (
              <span className="flex items-center gap-2 text-[13px] text-ink-subtle">
                <Input type="date" value={from} max={to} onChange={(e) => e.target.value && setFrom(e.target.value)} aria-label="From" />
                to
                <Input type="date" value={to} min={from} onChange={(e) => e.target.value && setTo(e.target.value)} aria-label="To" />
              </span>
            )}
          </>
        }
      />

      {s && (
        <section className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-line bg-line @xl:grid-cols-3 @4xl:grid-cols-5">
          <Kpi icon={Clock} label="Worked" value={formatDuration(s.worked_ms)} />
          <Kpi icon={CalendarDays} label="Daily average" value={formatDuration(s.avg_worked_ms)} />
          <Kpi icon={Target} label="Days on target" value={`${s.days_target_met} of ${s.days_tracked}`}>
            <Meter value={s.days_tracked ? s.days_target_met / s.days_tracked : 0} color="var(--color-working)" height={4} label="Days on target" className="mt-2.5" />
          </Kpi>
          <Kpi icon={Flame} label="Current streak" value={plural(s.current_streak, "day")} iconColor={s.current_streak > 0 ? "var(--color-idle)" : undefined} />
          <Kpi icon={Trophy} label="Longest streak" value={plural(s.longest_streak, "day")} />
        </section>
      )}

      {s && (
        <Panel title="By project" icon={ChartPie}>
          {s.by_project.length === 0 ? (
            <p className="text-[13px] text-ink-subtle">No work in this range.</p>
          ) : (
            <div className="flex flex-col items-center gap-8 @xl:flex-row">
              <div className="relative size-52 shrink-0">
                <ResponsiveContainer>
                  <PieChart>
                    <Pie data={s.by_project} dataKey="worked_ms" nameKey="name" innerRadius={68} outerRadius={96} paddingAngle={s.by_project.length > 1 ? 2 : 0} stroke="none" isAnimationActive={false}>
                      {s.by_project.map((p) => (
                        <Cell key={p.project_id ?? "none"} fill={p.color} />
                      ))}
                    </Pie>
                    <Tooltip {...tooltipProps} formatter={(v) => formatDuration(Number(v))} />
                  </PieChart>
                </ResponsiveContainer>
                <div className="pointer-events-none absolute inset-0 grid place-items-center text-center">
                  <div>
                    <div className="text-title font-semibold text-ink tabular-nums">{formatDuration(s.worked_ms)}</div>
                    <div className="text-xs text-ink-subtle">worked</div>
                  </div>
                </div>
              </div>
              <ul className="flex w-full min-w-0 flex-col gap-3.5">
                {s.by_project.map((p) => {
                  const share = s.worked_ms > 0 ? p.worked_ms / s.worked_ms : 0;
                  return (
                    <li key={p.project_id ?? "none"} className="flex flex-col gap-1.5">
                      <span className="flex items-center gap-2.5 text-sm">
                        <Dot color={p.color} />
                        <span className="min-w-0 flex-1 truncate text-ink-muted">{p.name}</span>
                        <span className="font-medium text-ink tabular-nums">{formatDuration(p.worked_ms)}</span>
                        <span className="w-10 text-right text-xs text-ink-subtle tabular-nums">{Math.round(share * 100)}%</span>
                      </span>
                      <Meter value={share} color={p.color} height={4} label={`${p.name}: ${Math.round(share * 100)}%`} />
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </Panel>
      )}

      {range === "week" && !readOnly && <Retro />}

      <Panel
        title={`${year}`}
        icon={CalendarDays}
        actions={
          <Segmented
            value={year}
            onChange={setYear}
            size="sm"
            label="Year"
            options={[3, 2, 1, 0].map((i) => ({ value: thisYear - i, label: String(thisYear - i) }))}
          />
        }
      >
        {heat && <Heatmap heatmap={heat} targetMs={target} today={today} />}
      </Panel>
    </div>
  );
}

function plural(n: number, unit: string): string {
  return `${n} ${n === 1 ? unit : unit + "s"}`;
}

function Kpi({ icon: Icon, label, value, iconColor, children }: { icon: LucideIcon; label: string; value: string; iconColor?: string; children?: React.ReactNode }) {
  return (
    <div className="bg-surface-1 px-5 py-4 last:col-span-2 @4xl:last:col-span-1">
      <div className="flex items-center gap-1.5 text-xs text-ink-subtle">
        <Icon size={13} aria-hidden style={iconColor ? { color: iconColor } : undefined} />
        {label}
      </div>
      <div className="mt-1.5 text-title font-semibold text-ink tabular-nums">{value}</div>
      {children}
    </div>
  );
}
