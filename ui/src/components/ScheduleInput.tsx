import { CalendarClock, Sparkles } from "lucide-react";
import { addDays, minutesOf } from "../format";
import { clockOf } from "../rrule";
import DurationInput from "./DurationInput";
import TimeInput from "./TimeInput";
import { Input, Segmented } from "./ui";

/** A block placed by hand: a day, a start time, and how long, as a Go duration. */
export interface Block {
  day: string;
  minute: number;
  length: string;
}

/**
 * When a task happens: left to the planner, or at a time the user sets. The
 * block is pinned, so the planner fits everything else around it.
 */
export default function ScheduleInput({ value, today, onChange }: { value: Block | null; today: string; onChange: (b: Block | null) => void }) {
  const tomorrow = addDays(today, 1);
  const on = value != null;
  return (
    <div className="flex flex-col gap-3">
      <Segmented
        value={on ? "set" : "auto"}
        onChange={(v) => onChange(v === "set" ? (value ?? { day: today, minute: nextQuarter(), length: "30m" }) : null)}
        label="When"
        className="self-start"
        options={[
          { value: "auto", label: "Gwen fits it in", icon: Sparkles },
          { value: "set", label: "At a set time", icon: CalendarClock },
        ]}
      />
      {value && (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-surface-2/60 p-3">
          <Segmented
            size="sm"
            value={value.day === today ? "today" : value.day === tomorrow ? "tomorrow" : ""}
            onChange={(v) => onChange({ ...value, day: v === "today" ? today : tomorrow })}
            label="Day"
            options={[
              { value: "today", label: "Today" },
              { value: "tomorrow", label: "Tomorrow" },
            ]}
          />
          <Input type="date" value={value.day} min={today} onChange={(e) => e.target.value && onChange({ ...value, day: e.target.value })} aria-label="Day" className="w-40" />
          <span className="text-xs text-ink-subtle">at</span>
          <TimeInput value={clockOf(value.minute)} onCommit={(hhmm) => hhmm && onChange({ ...value, minute: minutesOf(hhmm) })} aria-label="Start time" className="w-20" />
          <span className="text-xs text-ink-subtle">for</span>
          <DurationInput value={value.length} onChange={(length) => onChange({ ...value, length })} label="How long" />
        </div>
      )}
    </div>
  );
}

/** The instant of minute on the calendar date day, as a block's start. */
export function atOn(day: string, minute: number): number {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d, Math.floor(minute / 60), minute % 60).getTime();
}

/** The next quarter hour from now, in minutes after midnight. */
function nextQuarter(): number {
  const d = new Date();
  return Math.min(23 * 60 + 45, Math.ceil((d.getHours() * 60 + d.getMinutes() + 1) / 15) * 15);
}

export const efforts: { value: number; label: string; hint: string }[] = [
  { value: 0, label: "Not rated", hint: "" },
  { value: 1, label: "Easy", hint: "light work" },
  { value: 2, label: "Medium", hint: "" },
  { value: 3, label: "Hard", hint: "a frog: Gwen puts it first" },
];

export const stages: { value: string; label: string; hint: string }[] = [
  { value: "inbox", label: "Inbox", hint: "Captured, not sorted yet" },
  { value: "todo", label: "To do", hint: "Ready to be planned" },
  { value: "doing", label: "In progress", hint: "Being worked on" },
  { value: "waiting", label: "Waiting", hint: "Handed to someone, or blocked" },
  { value: "someday", label: "Someday", hint: "Parked; never planned" },
];

export function stageLabel(stage: string): string {
  return stages.find((s) => s.value === stage)?.label ?? stage;
}
