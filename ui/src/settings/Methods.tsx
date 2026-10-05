import { BatteryCharging, Flame, Sunrise } from "lucide-react";
import { useEffect, useState } from "react";
import { App, wire } from "../api";
import { useToast } from "../components/feedback";
import TimeInput from "../components/TimeInput";
import { Button, Panel, ToggleRow, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";

export const energyLevels = [
  { level: 1, label: "Drained" },
  { level: 2, label: "Low" },
  { level: 3, label: "Okay" },
  { level: 4, label: "Good" },
  { level: 5, label: "Peak" },
];

/**
 * Settings → Methods: eat the frog, and biological prime time with the
 * energy check-ins that find it (Sam Carpenter, Work the System).
 */
export default function MethodsSettings() {
  const d = useDaemon();
  const notify = useToast();
  const planner = d.config!.planner;
  const [start, setStart] = useState(planner.prime_start || "09:00");
  const [end, setEnd] = useState(planner.prime_end || "12:00");
  const [report, setReport] = useState<wire.EnergyReport | null>(null);
  const [version, setVersion] = useState(0);
  const prime = !!planner.prime_start;

  useEffect(() => {
    App.EnergyReport(0).then(setReport, d.fail);
  }, [version, d.fail]);
  useEffect(() => {
    if (planner.prime_start) setStart(planner.prime_start);
    if (planner.prime_end) setEnd(planner.prime_end);
  }, [planner.prime_start, planner.prime_end]);

  const save = (patch: Record<string, unknown>, said: string) =>
    d.act(() => App.PatchConfig({ planner: patch })).then((c) => c && notify(said, "success"));

  async function log(level: number) {
    if (await d.act(() => App.LogEnergy(wire.LogEnergyRequest.createFrom({ level, at: null })))) {
      notify(`Logged: ${energyLevels[level - 1].label}`, "success");
      setVersion((v) => v + 1);
    }
  }

  return (
    <>
      <Panel title="Eat the frog" icon={Flame}>
        <ToggleRow
          title="Hardest work first"
          description="Mark tasks Hard and Gwen plans them first thing, before the easier work, so the biggest task of the day is behind you early. The assistant does the same."
          checked={planner.eat_the_frog}
          onChange={(on) => save({ eat_the_frog: on }, on ? "Frogs go first now" : "Plans go by urgency alone")}
        />
      </Panel>

      <Panel title="Biological prime time" icon={Sunrise}>
        <p className="-mt-1 mb-3 text-[13px] leading-relaxed text-ink-subtle">
          Your energy rises and falls through the day. Set the hours you're sharpest, and Gwen puts hard work there and lighter work around it. Not sure when that is? Log your energy for a couple of weeks, here or from the tray, and Gwen will suggest it.
        </p>
        <ToggleRow
          title="Plan around my prime time"
          description={prime ? `Hard work goes between ${planner.prime_start} and ${planner.prime_end}.` : "Off: hard work goes wherever it fits."}
          checked={prime}
          onChange={(on) => save(on ? { prime_start: start, prime_end: end } : { prime_start: "", prime_end: "" }, on ? "Prime time on" : "Prime time off")}
        />
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="text-[13px] text-ink-muted">From</span>
          <TimeInput value={start} onCommit={setStart} aria-label="Prime time starts" className="w-20" />
          <span className="text-[13px] text-ink-muted">to</span>
          <TimeInput value={end} onCommit={setEnd} aria-label="Prime time ends" className="w-20" />
          <Button size="sm" tone="primary" disabled={prime && start === planner.prime_start && end === planner.prime_end} onClick={() => save({ prime_start: start, prime_end: end }, `Prime time is ${start}–${end}`)}>
            {prime ? "Save hours" : "Use these hours"}
          </Button>
        </div>

        <div className="mt-6 border-t border-line pt-5">
          <div className="flex flex-wrap items-center gap-2">
            <BatteryCharging size={16} className="text-ink-subtle" aria-hidden />
            <span className="text-[13px] font-medium text-ink">How's your energy right now?</span>
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {energyLevels.map((e) => (
              <Button key={e.level} size="sm" onClick={() => log(e.level)}>
                {e.label}
              </Button>
            ))}
          </div>
          {report && <EnergyChart report={report} />}
          {report?.suggested_start && report.suggested_end && (
            <div className="mt-4 flex flex-wrap items-center gap-3 rounded-lg border border-accent/30 bg-accent/8 px-4 py-3">
              <span className="min-w-0 flex-1 text-[13px] text-ink">
                Your energy peaks around <strong className="tabular-nums">{report.suggested_start}–{report.suggested_end}</strong>.
              </span>
              {!(planner.prime_start === report.suggested_start && planner.prime_end === report.suggested_end) && (
                <Button size="sm" tone="primary" onClick={() => save({ prime_start: report.suggested_start, prime_end: report.suggested_end }, "Prime time set from your check-ins")}>
                  Use as prime time
                </Button>
              )}
            </div>
          )}
        </div>
      </Panel>
    </>
  );
}

/** Average energy by hour of day: one bar per hour with check-ins. */
function EnergyChart({ report }: { report: wire.EnergyReport }) {
  const byHour = new Map(report.hours.map((h) => [h.hour, h]));
  const hours = report.hours.length ? Array.from({ length: Math.max(...report.hours.map((h) => h.hour)) - Math.min(...report.hours.map((h) => h.hour)) + 1 }, (_, i) => Math.min(...report.hours.map((h) => h.hour)) + i) : [];
  const last = report.logs[report.logs.length - 1];
  if (!hours.length)
    return <p className="mt-4 text-[13px] text-ink-subtle">No check-ins in the last {report.days} days yet. A few a day, at different times, are enough.</p>;
  return (
    <figure className="mt-5">
      <figcaption className="mb-2 flex items-baseline justify-between text-xs text-ink-subtle">
        <span>Average energy by hour, last {report.days} days</span>
        <span className="tabular-nums">
          {report.logs.length} check-ins{last && `, latest ${formatTime(last.at)}`}
        </span>
      </figcaption>
      <div className="flex h-32 items-end gap-0.5 border-b border-line" role="img" aria-label="Average energy by hour of day">
        {hours.map((h) => {
          const e = byHour.get(h);
          const inPrime = report.suggested_start && report.suggested_end && h >= Number(report.suggested_start.slice(0, 2)) && h < Number(report.suggested_end.slice(0, 2));
          return (
            <div key={h} className="group relative flex h-full flex-1 items-end">
              {e && (
                <div
                  className={cx("w-full rounded-t-[4px] transition-opacity group-hover:opacity-80", inPrime ? "bg-accent" : "bg-accent/45")}
                  style={{ height: `${(e.average / 5) * 100}%` }}
                />
              )}
              <div className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1 hidden -translate-x-1/2 rounded-md border border-line bg-surface-3 px-2 py-1 text-xs whitespace-nowrap text-ink group-hover:block">
                {String(h).padStart(2, "0")}:00 · {e ? `${e.average.toFixed(1)} of 5 (${e.count})` : "no check-ins"}
              </div>
            </div>
          );
        })}
      </div>
      <div className="mt-1 flex gap-0.5">
        {hours.map((h) => (
          <span key={h} className="flex-1 text-center text-[10px] text-ink-faint tabular-nums">
            {h % 2 === 0 ? String(h).padStart(2, "0") : ""}
          </span>
        ))}
      </div>
      <table className="sr-only">
        <caption>Average energy by hour</caption>
        <tbody>
          {report.hours.map((h) => (
            <tr key={h.hour}>
              <th>{h.hour}:00</th>
              <td>{h.average}</td>
              <td>{h.count} check-ins</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
