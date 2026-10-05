import { useState } from "react";
import { dateOf } from "../format";
import { buildRule, describeRule, parseRule, weekdayCodes, type Rule, type WeekdayCode } from "../rrule";
import { Input, Segmented, Toggle, cx } from "./ui";

type Mode = "none" | "daily" | "weekdays" | "weekly" | "monthly" | "custom";

const letters: Record<WeekdayCode, string> = { MO: "M", TU: "T", WE: "W", TH: "T", FR: "F", SA: "S", SU: "S" };
const names: Record<WeekdayCode, string> = { MO: "Monday", TU: "Tuesday", WE: "Wednesday", TH: "Thursday", FR: "Friday", SA: "Saturday", SU: "Sunday" };
const workweek: WeekdayCode[] = ["MO", "TU", "WE", "TH", "FR"];

function modeOf(value: string, allowNone: boolean): Mode {
  if (!value.trim()) return allowNone ? "none" : "daily";
  const r = parseRule(value);
  if (!r) return "custom";
  if (r.freq === "DAILY") return "daily";
  if (r.freq === "MONTHLY") return "monthly";
  const plainWorkweek = r.interval === 1 && r.byDay.length === 5 && workweek.every((d) => r.byDay.includes(d));
  return plainWorkweek ? "weekdays" : "weekly";
}

/**
 * Picks a repeat rule without typing RRULE: every day, weekdays, chosen days,
 * or a day of the month, with an optional interval and end date. Custom keeps
 * the text editable for anything else the planner accepts.
 */
export default function Recurrence({
  value,
  onChange,
  anchorDay,
  allowNone = false,
}: {
  value: string;
  onChange: (rule: string) => void;
  /** The day the rule starts from, for the default weekday and day of month. */
  anchorDay: string;
  /** Offers "Doesn't repeat", which is the empty rule. */
  allowNone?: boolean;
}) {
  const [mode, setMode] = useState<Mode>(() => modeOf(value, allowNone));
  const anchor = dateOf(anchorDay);
  const anchorCode = weekdayCodes[(anchor.getDay() + 6) % 7];
  const rule: Rule = parseRule(value) ?? { freq: "DAILY", interval: 1, byDay: [], byMonthDay: 0, until: null };

  function pick(m: Mode) {
    setMode(m);
    const keep = { interval: 1, until: rule.until };
    if (m === "none") onChange("");
    else if (m === "daily") onChange(buildRule({ ...rule, ...keep, freq: "DAILY", byDay: [], byMonthDay: 0 }));
    else if (m === "weekdays") onChange(buildRule({ ...rule, ...keep, freq: "WEEKLY", byDay: workweek, byMonthDay: 0 }));
    else if (m === "weekly")
      onChange(buildRule({ ...rule, ...keep, freq: "WEEKLY", byDay: rule.freq === "WEEKLY" && rule.byDay.length ? rule.byDay : [anchorCode], byMonthDay: 0 }));
    else if (m === "monthly") onChange(buildRule({ ...rule, ...keep, freq: "MONTHLY", byDay: [], byMonthDay: Math.min(anchor.getDate(), 28) }));
  }
  const set = (patch: Partial<Rule>) => onChange(buildRule({ ...rule, ...patch }));
  const toggleDay = (d: WeekdayCode) => {
    const on = rule.byDay.includes(d) ? rule.byDay.filter((x) => x !== d) : [...rule.byDay, d];
    if (on.length) set({ byDay: on });
  };
  const unit = mode === "daily" ? "day" : mode === "monthly" ? "month" : "week";

  const options = [
    ...(allowNone ? [{ value: "none" as Mode, label: "Doesn't repeat" }] : []),
    { value: "daily" as Mode, label: "Daily" },
    { value: "weekdays" as Mode, label: "Weekdays" },
    { value: "weekly" as Mode, label: "Some days" },
    { value: "monthly" as Mode, label: "Monthly" },
    { value: "custom" as Mode, label: "Custom" },
  ];

  return (
    <div className="flex flex-col gap-3">
      <Segmented value={mode} onChange={pick} options={options} size="sm" label="Repeats" className="self-start" />
      {mode === "weekly" && (
        <div className="flex gap-1.5" role="group" aria-label="On these days">
          {weekdayCodes.map((d) => {
            const on = rule.byDay.includes(d);
            return (
              <button
                key={d}
                type="button"
                aria-pressed={on}
                aria-label={names[d]}
                title={names[d]}
                onClick={() => toggleDay(d)}
                className={cx(
                  "grid size-8 place-items-center rounded-full text-xs font-semibold transition-colors",
                  on ? "bg-accent text-white" : "border border-line-strong bg-surface-2 text-ink-subtle hover:text-ink",
                )}
              >
                {letters[d]}
              </button>
            );
          })}
        </div>
      )}
      {mode === "monthly" && (
        <label className="flex items-center gap-2 text-[13px] text-ink-muted">
          On day
          <Input type="number" min={1} max={28} className="w-16 text-center" value={rule.byMonthDay || 1} onChange={(e) => set({ byMonthDay: clampInt(e.target.value, 1, 28) })} />
          of the month
        </label>
      )}
      {mode !== "none" && mode !== "custom" && mode !== "weekdays" && (
        <label className="flex items-center gap-2 text-[13px] text-ink-muted">
          Every
          <Input type="number" min={1} max={99} className="w-16 text-center" value={rule.interval} onChange={(e) => set({ interval: clampInt(e.target.value, 1, 99) })} />
          {rule.interval === 1 ? unit : unit + "s"}
        </label>
      )}
      {mode === "custom" && (
        <Input value={value} onChange={(e) => onChange(e.target.value.toUpperCase())} placeholder="FREQ=WEEKLY;INTERVAL=2;BYDAY=MO" spellCheck={false} />
      )}
      {mode !== "none" && mode !== "custom" && (
        <div className="flex items-center gap-3 text-[13px] text-ink-muted">
          <Toggle checked={rule.until !== null} onChange={(on) => set({ until: on ? anchorDay : null })} label="Ends on a date" />
          <span>Ends on a date</span>
          {rule.until !== null && <Input type="date" value={rule.until} min={anchorDay} onChange={(e) => e.target.value && set({ until: e.target.value })} />}
        </div>
      )}
      {mode !== "none" && value && <p className="text-xs text-ink-subtle">{describeRule(value)}</p>}
    </div>
  );
}

function clampInt(s: string, lo: number, hi: number): number {
  const n = Math.round(Number(s));
  return Number.isFinite(n) ? Math.max(lo, Math.min(hi, n)) : lo;
}
