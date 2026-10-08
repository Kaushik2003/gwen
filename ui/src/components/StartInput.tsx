import { addDays, minutesOf } from "../format";
import { clockOf } from "../rrule";
import TimeInput from "./TimeInput";
import { Input, Segmented } from "./ui";

/**
 * When a task starts: a day, Today or Tomorrow in one click or any date, and
 * an optional 24-hour time on it. A time with no day starts today.
 */
export default function StartInput({ day, minute, today, onChange }: { day: string; minute: number | null; today: string; onChange: (day: string, minute: number | null) => void }) {
  const tomorrow = addDays(today, 1);
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Segmented
        value={day === today ? "today" : day === tomorrow ? "tomorrow" : ""}
        onChange={(v) => onChange(v === "today" ? today : tomorrow, minute)}
        label="Starts"
        options={[
          { value: "today", label: "Today" },
          { value: "tomorrow", label: "Tomorrow" },
        ]}
      />
      <Input type="date" value={day} min={today} onChange={(e) => onChange(e.target.value, e.target.value ? minute : null)} aria-label="Start day" className="w-40" />
      <span className="text-xs text-ink-subtle">at</span>
      <TimeInput value={minute != null ? clockOf(minute) : ""} onCommit={(hhmm) => onChange(day || today, hhmm ? minutesOf(hhmm) : null)} allowEmpty aria-label="Start time" className="w-24" />
    </div>
  );
}
