import { ChevronRight, CircleCheck, History as HistoryIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { App, readOnly as readOnlyUI, wire } from "../api";
import { BarChart } from "../components/liquid-charts/liquid/charts/bar";
import type { ChartConfig } from "../components/liquid-charts/liquid/core";
import { Empty, Input, PageHeader, Panel, Segmented } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, dateOf, formatDate, formatDuration, parseDuration } from "../format";
import DayDetail from "./DayDetail";

type Range = "7" | "14" | "30" | "custom";

/**
 * Days by project over a range (default the last 7 days); a click opens Day detail. With `scope`, only the time on those projects
 * counts (one project and its sub-projects, as on its home), without breaks or the daily target, which are the whole day's.
 */
export default function History({ readOnly = readOnlyUI, scope }: { readOnly?: boolean; scope?: { ids: Set<string>; name: string } }) {
  const d = useDaemon();
  const today = d.today();
  const [range, setRange] = useState<Range>("7");
  const [from, setFrom] = useState(addDays(today, -6));
  const [to, setTo] = useState(today);
  const [days, setDays] = useState<wire.DaySummary[] | null>(null);
  const [open, setOpen] = useState<string | null>(null);

  useEffect(() => {
    App.ListDays(from, to).then(
      (l) =>
        setDays(
          scope
            ? l.days.map((x) => {
                const by_project = x.by_project.filter((p) => scope.ids.has(p.project_id ?? ""));
                return wire.DaySummary.createFrom({ ...x, by_project, worked_ms: by_project.reduce((n, p) => n + p.worked_ms, 0), break_ms: 0, target_met: false });
              })
            : l.days,
        ),
      d.fail,
    );
    // The scope's ids are a new Set each render; its name and size stand for it.
  }, [from, to, d.daysVersion, d.fail, scope?.name, scope?.ids.size]); // eslint-disable-line react-hooks/exhaustive-deps

  function pick(r: Range) {
    setRange(r);
    if (r === "custom") return;
    setTo(today);
    setFrom(addDays(today, -(Number(r) - 1)));
  }

  const target = scope ? 0 : parseDuration(d.config?.tracking.daily_target ?? "8h");
  const { data, config, all } = useMemo(() => {
    const byDay = new Map((days ?? []).map((x) => [x.day, x]));
    const config: ChartConfig = {};
    (days ?? []).forEach((x) => x.by_project.forEach((p) => (config[p.project_id ?? "none"] = { label: p.name, colors: { dark: [p.color], light: [p.color] } })));
    // Every day of the range, so untracked days show as gaps rather than vanish.
    const all: wire.DaySummary[] = [];
    for (let day = from; day <= to && all.length < 400; day = addDays(day, 1)) {
      all.push(byDay.get(day) ?? wire.DaySummary.createFrom({ day, worked_ms: 0, break_ms: 0, target_met: false, by_project: [], target_seconds: 0 }));
    }
    // Hours per project, every project on every row (0 where none), so each stack is whole.
    const data = all.map((x) => {
      const row: Record<string, number | string> = { day: x.day, label: label(x.day, all.length) };
      Object.keys(config).forEach((id) => (row[id] = 0));
      x.by_project.forEach((p) => (row[p.project_id ?? "none"] = +(p.worked_ms / 3_600_000).toFixed(2)));
      return row;
    });
    return { data, config, all };
  }, [days, from, to]);
  // The bar under the pointer (or keyboard focus): a click or Enter opens its day.
  const active = useRef<number | null>(null);
  const openActive = () => {
    const x = active.current === null ? undefined : all[active.current];
    if (x) setOpen(x.day);
  };

  if (open) return <DayDetail day={open} onBack={() => setOpen(null)} readOnly={readOnly} />;

  const tracked = all.filter((x) => x.worked_ms > 0);
  const total = tracked.reduce((sum, x) => sum + x.worked_ms, 0);
  const scale = Math.max(target, ...all.map((x) => x.worked_ms), 1);

  const rangePicker = (
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
  );

  return (
    <div className="flex flex-col gap-5">
      {scope ? (
        <header className="flex flex-wrap items-center gap-2">
          <p className="min-w-0 flex-[1_1_12rem] truncate text-[12.5px] text-ink-subtle">
            {tracked.length ? `${formatDuration(total)} on ${scope.name} over ${tracked.length} ${tracked.length === 1 ? "day" : "days"}` : `No time on ${scope.name} in this range.`}
          </p>
          {rangePicker}
        </header>
      ) : (
        <PageHeader
          title="History"
          subtitle={
            tracked.length
              ? `${formatDuration(total)} over ${tracked.length} ${tracked.length === 1 ? "day" : "days"}, ${all.filter((x) => x.target_met).length} on target`
              : "Every day you tracked, by project."
          }
          actions={rangePicker}
        />
      )}

      <Panel>
        {days && tracked.length === 0 ? (
          <Empty icon={HistoryIcon} title="Nothing tracked in this range">
            {scope ? `Work in one of ${scope.name}'s tabs while tracking is on, or add time by hand from a day below.` : "Clock in on Today, or add time by hand from a day below."}
          </Empty>
        ) : (
          <div className="cursor-pointer" onClick={openActive} onKeyDown={(e) => e.key === "Enter" && openActive()}>
            <BarChart
              data={data}
              config={config}
              categoryKey="label"
              categoryTitle={(_, i) => `${formatDate(all[i].day)} · ${all[i].worked_ms > 0 ? formatDuration(all[i].worked_ms) : "nothing tracked"}`}
              stack="stacked"
              format={{ kind: "number", decimals: 1 }}
              tickFormat={(v) => `${v}h`}
              renderLabel={({ index }) => (
                <span className="text-[12.5px] font-semibold text-ink tabular-nums" style={{ fontFamily: "var(--font-chart)" }}>
                  {formatDuration(all[index].worked_ms)}
                </span>
              )}
              // Tall enough for the target line, which the bars alone may never reach. A reference line, not `target`:
              // on stacks `target` draws no line, only a note at the edge.
              domain={[0, Math.ceil(scale / 3_600_000)]}
              references={target > 0 ? [{ value: target / 3_600_000, label: `Target ${formatDuration(target)}` }] : undefined}
              maxThickness={44}
              height={288}
              theme="dark"
              onActiveChange={(i) => (active.current = i)}
              label={`Hours worked each day, by project${scope ? ` in ${scope.name}` : ""}. Use the arrow keys to read each day, Enter to open it.`}
            />
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
