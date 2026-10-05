import { CalendarCheck, ChevronLeft, ChevronRight, CircleCheck, ClipboardCheck, Lightbulb, MessageSquareText, NotebookPen, ScrollText, Sparkles } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, wire } from "../api";
import { RetroFlow, mondayOf } from "../components/ai";
import { Button, Checkbox, IconButton, PageHeader, Panel, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { addDays, formatDate, formatDuration } from "../format";
import { useNav } from "../nav";

/** The review's steps, a bit each in WeeklyReview.checklist. */
const steps: { title: string; detail: string; screen?: string; param?: string }[] = [
  { title: "Empty your inbox", detail: "Process everything you captured this week.", screen: "inbox" },
  { title: "Look back at the week", detail: "What you finished, and how your time went.", screen: "stats" },
  { title: "Review every project", detail: "Each one has a clear next action.", screen: "board" },
  { title: "Check what you're waiting for", detail: "Chase what is overdue from others.", screen: "inbox" },
  { title: "Look through someday / maybe", detail: "Activate anything whose time has come.", screen: "inbox" },
  { title: "Check your goals' pace", detail: "Are they still SMART, and on track?", screen: "goals" },
  { title: "Plan the week ahead", detail: "Put the important, not urgent work on the calendar.", screen: "board", param: "matrix" },
];

type Text = "went_well" | "went_badly" | "energy" | "decisions" | "improvements";

const prompts: { key: Text; title: string; placeholder: string }[] = [
  { key: "went_well", title: "What worked for you?", placeholder: "Waking up at 06:00 made a real productivity change for me." },
  { key: "went_badly", title: "What didn't?", placeholder: "Eating a large meal at lunch threw me off my groove." },
  { key: "energy", title: "How was your energy?", placeholder: "Sharp in the mornings, flat after 15:00." },
  { key: "decisions", title: "Which decisions would you change?", placeholder: "I said yes to two meetings I didn't need." },
  { key: "improvements", title: "What will you do differently next week?", placeholder: "I don't estimate how long tasks take well: add 30% to every estimate." },
];

/**
 * Weekly review: once a week, sit down somewhere quiet and look back at your
 * productivity and energy, what worked and what didn't, then turn it into
 * changes for the next week. The assistant reads it.
 */
export default function Review() {
  const d = useDaemon();
  const { go } = useNav();
  const thisWeek = mondayOf(d.today());
  // The week just ended is the one to review, until it is a few days old.
  const [week, setWeek] = useState(() => (new Date().getDay() >= 1 && new Date().getDay() <= 4 ? addDays(thisWeek, -7) : thisWeek));
  const [review, setReview] = useState<wire.WeeklyReview | null>(null);
  const [summary, setSummary] = useState<wire.StatsSummary | null>(null);
  const [finished, setFinished] = useState<number | null>(null);
  const [saved, setSaved] = useState<number | null>(null);

  useEffect(() => {
    let live = true;
    setReview(null);
    App.GetReview(week).then((r) => live && setReview(r), d.fail);
    App.StatsSummary(week, addDays(week, 6)).then((s) => live && setSummary(s), () => live && setSummary(null));
    App.ListTasks(wire.TaskQuery.createFrom({ status: "done" })).then((l) => {
      const from = new Date(week + "T00:00").getTime();
      const to = from + 7 * 86_400_000;
      if (live) setFinished(l.tasks.filter((t) => (t.done_at ?? 0) >= from && (t.done_at ?? 0) < to).length);
    }, d.fail);
    return () => {
      live = false;
    };
  }, [week, d.fail]);

  async function save(patch: Partial<Record<Text, string>> & { checklist?: number }) {
    const r = await d.act(() => App.SaveReview(week, wire.SaveReviewRequest.createFrom(patch)));
    if (r) {
      setReview(r);
      setSaved(Date.now());
    }
  }

  const checked = review?.checklist ?? 0;
  const doneSteps = steps.filter((_, i) => checked & (1 << i)).length;
  const inbox = d.tasks.filter((t) => t.stage === "inbox").length;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Weekly review"
        subtitle="Once a week, somewhere quiet: look back at your productivity and energy, what worked and what didn't, and decide what changes."
        actions={
          <div className="flex items-center gap-1">
            <IconButton icon={ChevronLeft} label="Earlier week" onClick={() => setWeek(addDays(week, -7))} />
            <span className="min-w-44 text-center text-[13px] font-medium text-ink tabular-nums">
              {week === thisWeek ? "This week" : week === addDays(thisWeek, -7) ? "Last week" : `Week of ${formatDate(week, false)}`}
              <span className="block text-xs font-normal text-ink-subtle">
                {formatDate(week, false)} – {formatDate(addDays(week, 6), false)}
              </span>
            </span>
            <IconButton icon={ChevronRight} label="Later week" disabled={week >= thisWeek} onClick={() => setWeek(addDays(week, 7))} />
          </div>
        }
      />

      <div className="grid gap-3 sm:grid-cols-4">
        <Stat label="Worked" value={summary ? formatDuration(summary.worked_ms) : "–"} />
        <Stat label="Days on target" value={summary ? `${summary.days_target_met} of ${summary.days_tracked}` : "–"} />
        <Stat label="Tasks finished" value={finished ?? "–"} />
        <Stat label="Review steps" value={`${doneSteps} of ${steps.length}`} />
      </div>

      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
        <Panel title="Checklist" icon={ClipboardCheck}>
          <ul className="-my-1 flex flex-col">
            {steps.map((s, i) => {
              const on = !!(checked & (1 << i));
              return (
                <li key={s.title} className="flex items-start gap-3 rounded-md px-1 py-2">
                  <span className="pt-0.5">
                    <Checkbox checked={on} onChange={() => save({ checklist: checked ^ (1 << i) })} label={s.title} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className={cx("block text-[13px] font-medium", on ? "text-ink-subtle line-through" : "text-ink")}>
                      {s.title}
                      {i === 0 && inbox > 0 && <span className="ml-2 rounded-full bg-accent/15 px-1.5 text-xs text-accent-hover">{inbox}</span>}
                    </span>
                    <span className="block text-xs text-ink-subtle">{s.detail}</span>
                  </span>
                  {s.screen && (
                    <Button size="sm" tone="ghost" onClick={() => go(s.screen!, s.param ?? null)}>
                      Open
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
          {doneSteps === steps.length && (
            <p className="mt-3 inline-flex items-center gap-1.5 text-[13px] text-working">
              <CircleCheck size={14} aria-hidden /> Review done. Enjoy your week.
            </p>
          )}
        </Panel>

        <Panel
          title="Reflect"
          icon={NotebookPen}
          actions={saved && <span className="text-xs text-ink-faint">Saved</span>}
        >
          {review ? (
            <div className="flex flex-col gap-4">
              {prompts.map((p) => (
                <Reflection key={week + p.key} title={p.title} placeholder={p.placeholder} value={review[p.key]} onSave={(v) => v !== review[p.key] && save({ [p.key]: v })} />
              ))}
              <p className="text-xs leading-relaxed text-ink-subtle">The assistant reads your latest review, so it can keep what worked and help with what didn't.</p>
            </div>
          ) : (
            <p className="text-[13px] text-ink-subtle">Loading…</p>
          )}
        </Panel>
      </div>

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel title="Retro from your numbers" icon={ScrollText}>
          <RetroFlow key={week} fixedWeek={week} />
        </Panel>
        <Panel title="Talk it through" icon={MessageSquareText}>
          <div className="flex flex-col gap-3">
            <p className="text-[13px] leading-relaxed text-ink-subtle">Go through the week with the assistant: it sees your tasks, goals, and this review, and can reschedule and plan next week with you as you talk.</p>
            <div className="flex flex-wrap gap-2">
              <Button tone="primary" icon={Sparkles} onClick={() => go("assistant", "Let's do my weekly review: what should I change for next week?")}>
                Review with the assistant
              </Button>
              <Button icon={CalendarCheck} onClick={() => go("assistant", "Plan my week ahead, important work first")}>
                Plan next week
              </Button>
              <Button icon={Lightbulb} onClick={() => go("board", "matrix")}>
                Eisenhower matrix
              </Button>
            </div>
          </div>
        </Panel>
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="lift rounded-xl border border-line bg-surface-1 px-4 py-3">
      <div className="text-xs text-ink-subtle">{label}</div>
      <div className="mt-1 text-lg font-semibold text-ink tabular-nums">{value}</div>
    </div>
  );
}

/** One question, saved when the field is left. */
function Reflection({ title, placeholder, value, onSave }: { title: string; placeholder: string; value: string; onSave: (v: string) => void }) {
  const [text, setText] = useState(value);
  const last = useRef(value);
  useEffect(() => {
    if (value !== last.current) setText(value);
    last.current = value;
  }, [value]);
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[13px] font-medium text-ink-muted">{title}</span>
      <textarea className="field min-h-16 resize-y" rows={2} value={text} placeholder={placeholder} maxLength={4000} onChange={(e) => setText(e.target.value)} onBlur={() => onSave(text.trim())} />
    </label>
  );
}
