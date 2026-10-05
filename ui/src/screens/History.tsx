import { ChevronRight, CircleCheck, History as HistoryIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Bar, BarChart, CartesianGrid, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { App, readOnly as readOnlyUI, wire } from "../api";
import { axisProps, gridProps, tooltipProps } from "../components/chart";
import { Empty, Input, PageHeader, Panel, Segmented } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, dateOf, formatDate, formatDuration, parseDuration } from "../format";
import DayDetail from "./DayDetail";

type Range = "7" | "14" | "30" | "custom";

/** Days by project over a range (default the last 7 days); a click opens Day detail. */
export default function History({ readOnly = readOnlyUI }: { readOnly?: boolean }) {
  const d = useDaemon();
  const today = d.today();
  const [range, setRange] = useState<Range>("7");
  const [from, setFrom] = useState(addDays(today, -6));
  const [to, setTo] = useState(today);
  const [days, setDays] = useState<wire.DaySummary[] | null>(null);
  const [open, setOpen] = useState<string | null>(null);

  useEffect(() => {
    App.ListDays(from, to).then((l) => setDays(l.days), d.fail);
  }, [from, to, d.daysVersion, d.fail]);

  function pick(r: Range) {
    setRange(r);
    if (r === "custom") return;
    setTo(today);
    setFrom(addDays(today, -(Number(r) - 1)));
  }

  const target = parseDuration(d.config?.tracking.daily_target ?? "8h");
  const { data, projects, all } = useMemo(() => {
    const byDay = new Map((days ?? []).map((x) => [x.day, x]));
    const projects = new Map<string, { name: string; color: string }>();
    (days ?? []).forEach((x) => x.by_project.forEach((p) => projects.set(p.project_id ?? "none", { name: p.name, color: p.color })));
    // Every day of the range, so untracked days show as gaps rather than vanish.
    const all: wire.DaySummary[] = [];
    for (let day = from; day <= to && all.length < 400; day = addDays(day, 1)) {
      all.push(byDay.get(day) ?? wire.DaySummary.createFrom({ day, worked_ms: 0, break_ms: 0, target_met: false, by_project: [], target_seconds: 0 }));
    }
    const data = all.map((x) => {
      const row: Record<string, number | string> = { day: x.day, label: label(x.day, all.length) };
      x.by_project.forEach((p) => (row[p.project_id ?? "none"] = +(p.worked_ms / 3_600_000).toFixed(2)));
      return row;
    });
    return { data, projects, all };
  }, [days, from, to]);

  if (open) return <DayDetail day={open} onBack={() => setOpen(null)} readOnly={readOnly} />;

  const tracked = all.filter((x) => x.worked_ms > 0);
  const total = tracked.reduce((sum, x) => sum + x.worked_ms, 0);
  const scale = Math.max(target, ...all.map((x) => x.worked_ms), 1);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="History"
        subtitle={
          tracked.length
            ? `${formatDuration(total)} over ${tracked.length} ${tracked.length === 1 ? "day" : "days"}, ${all.filter((x) => x.target_met).length} on target`
            : "Every day you tracked, by project."
        }
        actions={
          <>
            <Segmented
              value={range}
              onChange={pick}
              label="Range"
              options={[
                { value: "7", label: "7 days" },
                { value: "14", label: "14 days" },
                { value: "30", label: "30 days" },
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

      <Panel>
        {days && tracked.length === 0 ? (
          <Empty icon={HistoryIcon} title="Nothing tracked in this range">
            Clock in on Today, or add time by hand from a day below.
          </Empty>
        ) : (
          <div className="h-72">
            <ResponsiveContainer>
              <BarChart
                data={data}
                margin={{ top: 8, right: 8, bottom: 0, left: -12 }}
                onClick={(e: unknown) => {
                  const i = Number((e as { activeTooltipIndex?: unknown } | null)?.activeTooltipIndex);
                  if (Number.isInteger(i) && all[i]) setOpen(all[i].day);
                }}
              >
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="label" {...axisProps} interval="preserveStartEnd" minTickGap={8} />
                <YAxis unit="h" {...axisProps} width={44} allowDecimals={false} />
                <Tooltip
                  {...tooltipProps}
                  labelFormatter={(_, payload) => {
                    const day = (payload?.[0]?.payload as { day?: string } | undefined)?.day;
                    return day ? formatDate(day) : "";
                  }}
                  formatter={(v, k) => [formatDuration(Number(v) * 3_600_000), projects.get(String(k))?.name ?? String(k)]}
                />
                {target > 0 && (
                  <ReferenceLine y={target / 3_600_000} stroke="#62666d" strokeDasharray="4 4" label={{ value: "Target", position: "insideTopRight", fill: "#8a8f98", fontSize: 11 }} />
                )}
                {[...projects.entries()].map(([id, p], i, arr) => (
                  <Bar key={id} dataKey={id} stackId="w" fill={p.color} cursor="pointer" radius={i === arr.length - 1 ? [3, 3, 0, 0] : 0} maxBarSize={44} isAnimationActive={false} />
                ))}
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </Panel>

      <Panel title="Days" bodyClassName="px-2 pt-1 pb-3">
        <ul className="flex flex-col">
          {[...all].reverse().map((x) => (
            <li key={x.day}>
              <button type="button" onClick={() => setOpen(x.day)} className="group flex w-full items-center gap-4 rounded-lg px-3 py-2.5 text-left hover:bg-surface-2">
                <span className="w-28 shrink-0">
                  <span className="block text-sm text-ink">{formatDate(x.day)}</span>
                  {x.day === today && <span className="text-xs text-accent-hover">Today</span>}
                </span>
                <span className="relative h-2 flex-1 overflow-hidden rounded-full bg-surface-3">
                  <span className="absolute inset-y-0 left-0 flex" style={{ width: `${(x.worked_ms / scale) * 100}%` }}>
                    {x.by_project.map((p) => (
                      <span key={p.project_id ?? "none"} style={{ width: `${(p.worked_ms / Math.max(x.worked_ms, 1)) * 100}%`, backgroundColor: p.color }} />
                    ))}
                  </span>
                  {target > 0 && <span className="absolute inset-y-0 w-px bg-ink-faint" style={{ left: `${(target / scale) * 100}%` }} />}
                </span>
                <span className="w-16 shrink-0 text-right text-sm font-medium text-ink tabular-nums">{x.worked_ms > 0 ? formatDuration(x.worked_ms) : "—"}</span>
                <span className="w-20 shrink-0 text-right text-xs text-ink-subtle tabular-nums">{x.break_ms > 0 ? `${formatDuration(x.break_ms)} break` : ""}</span>
                <span className="w-5 shrink-0 text-working">{x.target_met && <CircleCheck size={16} aria-label="Target met" />}</span>
                <ChevronRight size={16} className="shrink-0 text-ink-faint group-hover:text-ink-subtle" aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  );
}

/** "Mon 5" for a week, "5" for longer ranges with the month on the 1st. */
function label(day: string, n: number): string {
  const d = dateOf(day);
  if (n <= 14) return `${["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][d.getDay()]} ${d.getDate()}`;
  return d.getDate() === 1 ? formatDate(day, false) : String(d.getDate());
}
