import { CalendarDays, ChartPie, Coffee, Flame, Trophy, type LucideIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { App, readOnly, wire } from "../api";
import { CalendarHeatmap, longestStreak } from "../components/liquid-charts/liquid/charts/cells";
import { StatCard } from "../components/liquid-charts/liquid/charts/kpi";
import { DonutChart } from "../components/liquid-charts/liquid/charts/ring";
import type { ChartConfig } from "../components/liquid-charts/liquid/core";
import Retro from "../components/Retro";
import { Dot, Input, Meter, PageHeader, Panel, Segmented } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, daysBetween, formatDate, formatDuration, parseDuration } from "../format";

type Range = "week" | "month" | "year" | "custom";

/** The "working" token as hex: the Liquid charts' glass takes colours it can parse, not a CSS variable. */
const working = "#27a644";
const hour = 3_600_000;

export default function Stats() {
  const d = useDaemon();
  const today = d.today();
  const thisYear = Number(today.slice(0, 4));
  const [range, setRange] = useState<Range>("week");
  const [from, setFrom] = useState(addDays(today, -6));
  const [to, setTo] = useState(today);
  const [summary, setSummary] = useState<wire.StatsSummary | null>(null);
  // The range just before this one, for the Worked card's change.
  const [before, setBefore] = useState<wire.StatsSummary | null>(null);
  // Tasks finished in the range by project: the donut's second measure, shown as the slab's height when it tilts.
  const [finished, setFinished] = useState<Map<string, number>>(new Map());
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
    const length = daysBetween(from, to) + 1;
    App.StatsSummary(addDays(from, -length), addDays(from, -1)).then(setBefore, () => setBefore(null));
  }, [from, to, d.daysVersion, d.fail]);
  useEffect(() => {
    App.ListTasks(wire.TaskQuery.createFrom({ status: "done" })).then((l) => {
      const lo = new Date(from + "T00:00").getTime();
      const hi = new Date(addDays(to, 1) + "T00:00").getTime();
      const byProject = new Map<string, number>();
      for (const t of l.tasks) {
        if ((t.done_at ?? 0) < lo || (t.done_at ?? 0) >= hi) continue;
        byProject.set(t.project_id ?? "none", (byProject.get(t.project_id ?? "none") ?? 0) + 1);
      }
      setFinished(byProject);
    }, d.fail);
  }, [from, to, d.tasksVersion, d.fail]);
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
        <section className="grid gap-4 @3xl:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
          <StatCard
            title="Worked"
            subtitle={`${formatDate(from)} to ${formatDate(to)} · hours · ${s.days_target_met} of ${s.days_tracked} tracked days on target`}
            value={s.worked_ms / hour}
            format={{ kind: "number", decimals: 1 }}
            valueLabel={`hours, ${s.days_target_met} of ${s.days_tracked} days on target`}
            delta={before && before.worked_ms > 0 ? { comparison: before.worked_ms / hour, label: "vs the range before" } : undefined}
            progress={{ value: s.days_target_met, max: Math.max(s.days_tracked, 1), colorKey: "worked", label: "Days on target" }}
            config={workedConfig}
            theme="dark"
            // The card's own square frame takes the page's surface and radius.
            className="overflow-hidden rounded-xl [--kpi-card:var(--color-surface-1)]!"
            label="Hours worked in this range, against the range before, with the days on target"
          />
          <div className="grid grid-cols-2 grid-rows-2 gap-px overflow-hidden rounded-xl border border-line bg-line">
            <Kpi icon={CalendarDays} label="Daily average" value={formatDuration(s.avg_worked_ms)} />
            <Kpi icon={Coffee} label="Breaks" value={formatDuration(s.break_ms)} />
            <Kpi icon={Flame} label="Current streak" value={plural(s.current_streak, "day")} iconColor={s.current_streak > 0 ? "var(--color-idle)" : undefined} />
            <Kpi icon={Trophy} label="Longest streak" value={plural(s.longest_streak, "day")} />
          </div>
        </section>
      )}

      {s && (
        <Panel title="By project" icon={ChartPie}>
          {s.by_project.length === 0 ? (
            <p className="text-[13px] text-ink-subtle">No work in this range.</p>
          ) : (
            <div className="flex flex-col items-center gap-8 @xl:flex-row">
              <div className="w-60 shrink-0">
                <DonutChart
                  data={s.by_project.map((p) => ({ key: p.project_id ?? "none", value: +(p.worked_ms / hour).toFixed(2), measure: finished.get(p.project_id ?? "none") ?? 0 }))}
                  config={Object.fromEntries(s.by_project.map((p) => [p.project_id ?? "none", { label: p.name, colors: { dark: [p.color], light: [p.color] } }])) satisfies ChartConfig}
                  format={{ kind: "number", decimals: 1 }}
                  centreLabel="hours worked"
                  labels="none"
                  legend={false}
                  height={240}
                  heightMeaning="second-measure"
                  measureLabel="Tasks finished"
                  measureFormat={{ kind: "number" }}
                  theme="dark"
                  label="Hours worked by project in this range. Use the arrow keys to read each project."
                />
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
        {heat && <Contributions heat={heat} targetMs={target} today={today} />}
      </Panel>
    </div>
  );
}

const workedConfig: ChartConfig = { worked: { label: "Worked", colors: { dark: [working], light: [working] } } };

/** A year of days as hours worked, the colour deepening to the daily target, the longest run of tracked days outlined. */
function Contributions({ heat, targetMs, today }: { heat: wire.Heatmap; targetMs: number; today: string }) {
  const { data, streak, tracked } = useMemo(() => {
    const data = heat.days.map((x) => ({ date: x.day, value: +(x.worked_ms / hour).toFixed(2) }));
    const byDay = new Map(data.map((x) => [x.date, x.value]));
    // The streak's indices count from the first day shown, so lay the year out day by day.
    const first = `${heat.year}-01-01`;
    const every: number[] = [];
    for (let day = first; day.startsWith(String(heat.year)); day = addDays(day, 1)) every.push(byDay.get(day) ?? 0);
    const run = longestStreak(every);
    return { data, streak: run && { from: addDays(first, run.from), to: addDays(first, run.to), length: run.length }, tracked: data.filter((x) => x.value > 0).length };
  }, [heat]);
  const target = Math.max(targetMs / hour, 1);
  return (
    <CalendarHeatmap
      data={data}
      from={`${heat.year}-01-01`}
      to={`${heat.year}-12-31`}
      today={today}
      range={streak && { from: streak.from, to: streak.to, label: `Longest streak · ${streak.length} ${streak.length === 1 ? "day" : "days"}` }}
      scale={{ kind: "sequential", key: "worked", domain: [0, target] }}
      config={workedConfig}
      format={{ kind: "number", decimals: 1, unit: "h" }}
      valueName="Worked"
      theme="dark"
      label={`${heat.year}: ${tracked} days tracked, hours worked per day. Left and Right step a day, Up and Down a week.`}
    />
  );
}

function plural(n: number, unit: string): string {
  return `${n} ${n === 1 ? unit : unit + "s"}`;
}

function Kpi({ icon: Icon, label, value, iconColor, children }: { icon: LucideIcon; label: string; value: string; iconColor?: string; children?: React.ReactNode }) {
  return (
    <div className="bg-surface-1 px-5 py-4">
      <div className="flex items-center gap-1.5 text-xs text-ink-subtle">
        <Icon size={13} aria-hidden style={iconColor ? { color: iconColor } : undefined} />
        {label}
      </div>
      <div className="mt-1.5 text-title font-semibold text-ink tabular-nums">{value}</div>
      {children}
    </div>
  );
}
