import { CalendarDays, CircleAlert, Clock, Flag, ScrollText, Sparkles } from "lucide-react";
import { useEffect, useState } from "react";
import { App, apiError, wire } from "../api";
import { useDaemon, useTick } from "../daemon";
import { addDays, formatDate, formatDuration, parseDuration } from "../format";
import type { BreakdownOutput, RetroOutput, RunError } from "../llm";
import { useNav } from "../nav";
import Face from "./Face";
import { useToast } from "./feedback";
import Markdown from "./Markdown";
import { Button, Callout, Checkbox, Field, Segmented, TextArea, priorityOf } from "./ui";

/** The provider as a person would name it. */
export function providerName(llm?: wire.LLMConfig | null): string {
  switch (llm?.provider) {
    case "claude_code":
      return "Claude Code";
    case "anthropic":
      return "the Anthropic API";
    case "openai_compatible":
      return llm.model || "your model";
    default:
      return "no provider";
  }
}

/** A Claude subscription as its name is written: "max" is the Max plan. */
export function planName(plan: string): string {
  return plan ? plan[0].toUpperCase() + plan.slice(1) : "";
}

/** The daemon's messages end with a CLI hint; the dashboard has its own way there. */
export function withoutCliHint(message: string): string {
  return message.replace(/\s*[,;]\s*(?:or give its path with|run)\s+gwen setup.*$/i, "").replace(/\s*\(run gwen setup.*\)$/i, "");
}

/** What a request is doing while the provider thinks, with how long it has taken. */
export function AiProgress({ since, doing }: { since: number; doing: string }) {
  const d = useDaemon();
  useTick(1000);
  const secs = Math.max(0, Math.floor((Date.now() - since) / 1000));
  const limit = parseDuration(d.config?.llm.timeout ?? "2m");
  return (
    <div className="rounded-lg border border-accent/30 bg-accent/[0.06] p-4" role="status">
      <div className="flex items-center gap-2 text-[13px] text-ink-muted">
        <Face mood="thinking" className="-my-2.5 size-10" />
        <span>
          {doing} with {providerName(d.config?.llm)}
        </span>
        <span className="ml-auto text-ink-faint tabular-nums">{secs}s</span>
      </div>
      <div className="mt-3 h-1 overflow-hidden rounded-full bg-surface-3">
        <div className="animate-indeterminate h-full w-1/3 rounded-full bg-accent" />
      </div>
      {limit > 0 && <p className="mt-2 text-xs text-ink-faint">It gives up after {formatDuration(limit)}.</p>}
    </div>
  );
}

/** The provider is not set up: say so and go there. */
export function AiUnavailable({ message }: { message: string }) {
  const { go } = useNav();
  return (
    <Callout
      tone="idle"
      icon={Sparkles}
      action={
        <Button size="sm" onClick={() => go("settings", "ai")}>
          Set up AI
        </Button>
      }
    >
      The assistant can't answer yet: {withoutCliHint(message)}.
    </Callout>
  );
}

/** A failed run: the provider's reason, and the setting that usually fixes it. */
export function AiFailed({ error }: { error: string }) {
  const { go } = useNav();
  const timedOut = /timed out/i.test(error);
  return (
    <Callout
      tone="danger"
      icon={CircleAlert}
      action={
        <Button size="sm" onClick={() => go("settings", "ai")}>
          {timedOut ? "Allow more time" : "Check AI settings"}
        </Button>
      }
    >
      {withoutCliHint(error)}
    </Callout>
  );
}

/** The proposed tasks' indexes by due day, in order: a quantity goal's sessions. */
function byDay(tasks: { due_day: string }[]): { day: string; indexes: number[] }[] {
  const out: { day: string; indexes: number[] }[] = [];
  tasks.forEach((t, i) => {
    const last = out[out.length - 1];
    if (last && last.day === t.due_day) last.indexes.push(i);
    else out.push({ day: t.due_day, indexes: [i] });
  });
  return out;
}

/**
 * Goal → Break down: ask for tasks, pick the good ones, add them
 * (docs/08-clients.md#v3-additions). For a quantity goal the assistant lines
 * up the next sessions' units, one task each with what to do, shown by day.
 */
export function BreakdownFlow({ goal, onDone }: { goal: wire.Goal; onDone?: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const quantity = goal.kind === "quantity";
  const unit = goal.unit || "units";
  const [instructions, setInstructions] = useState("");
  const [sessions, setSessions] = useState(7);
  const [run, setRun] = useState<wire.LlmRun | null>(null);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [since, setSince] = useState<number | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setRun(null);
    setPicked(new Set());
    setUnavailable(null);
    setRefused(null);
  }, [goal.id]);

  const tasks = run?.status === "ok" ? (run.output as BreakdownOutput).tasks : [];
  const failure = run?.status === "failed" ? (run.output as RunError).error : null;
  const pickedMinutes = tasks.reduce((sum, t, i) => sum + (picked.has(i) ? t.estimate_minutes : 0), 0);
  const groups = quantity ? byDay(tasks) : [{ day: "", indexes: tasks.map((_, i) => i) }];

  async function ask() {
    setSince(Date.now());
    setUnavailable(null);
    setRefused(null);
    try {
      const r = await App.GoalBreakdown(
        goal.id,
        wire.BreakdownRequest.createFrom({
          instructions,
          sessions: quantity ? sessions : 0,
        }),
      );
      setRun(r);
      setPicked(r.status === "ok" ? new Set((r.output as BreakdownOutput).tasks.map((_, i) => i)) : new Set());
    } catch (e) {
      const err = apiError(e);
      if (err.code === "unavailable") setUnavailable(err.message);
      else if (err.code === "invalid_request") setRefused(err.message);
      else d.fail(e);
    } finally {
      setSince(null);
    }
  }
  async function add() {
    if (!run) return;
    setSaving(true);
    const added = await d.act(() =>
      App.AcceptLLMRun(
        run.id,
        wire.AcceptRunRequest.createFrom({
          indexes: [...picked].sort((a, b) => a - b),
        }),
      ),
    );
    setSaving(false);
    if (!added) return;
    const n = added.tasks.length;
    notify(quantity ? `Lined up ${n} ${unit} for ${goal.title}` : `Added ${n} ${n === 1 ? "task" : "tasks"} to ${goal.title}`, "success");
    setRun(null);
    onDone?.();
  }
  async function discard() {
    if (run?.status === "ok") await d.run(() => App.RejectLLMRun(run.id));
    setRun(null);
    setPicked(new Set());
    onDone?.();
  }
  const toggle = (i: number) =>
    setPicked((p) => {
      const n = new Set(p);
      if (n.has(i)) n.delete(i);
      else n.add(i);
      return n;
    });

  return (
    <div className="flex flex-col gap-4">
      {quantity && tasks.length === 0 && (
        <Field
          label="How far ahead"
          compound
          hint={goal.progress.lined_up > 0 ? `${goal.progress.lined_up} ${unit} are already lined up; these come after them.` : `Each session gets its ${unit}, one task each, with what to do.`}
        >
          <Segmented
            value={sessions}
            onChange={setSessions}
            label="How far ahead"
            className="self-start"
            options={[
              { value: 3, label: "Next 3 sessions" },
              { value: 7, label: "A week" },
              { value: 14, label: "Two weeks" },
            ]}
          />
        </Field>
      )}
      <Field
        label="What should the tasks focus on?"
        hint={quantity ? "Optional. For example: arrays and strings first, LeetCode mediums, easier ones on weekdays." : "Optional. For example: the chapters in order, a review before each test."}
      >
        <TextArea rows={2} value={instructions} onChange={(e) => setInstructions(e.target.value)} disabled={since !== null} />
      </Field>
      {tasks.length === 0 && (
        <div className="flex flex-wrap items-center gap-3">
          <Button tone="primary" icon={Sparkles} onClick={ask} busy={since !== null}>
            {run ? "Ask again" : quantity ? `Line up ${unit}` : "Propose tasks"}
          </Button>
          <span className="text-xs text-ink-subtle">Nothing is added until you choose.</span>
        </div>
      )}
      {since !== null && <AiProgress since={since} doing={quantity ? `Lining up ${unit}` : "Breaking the goal down"} />}
      {unavailable && <AiUnavailable message={unavailable} />}
      {refused && (
        <Callout tone="neutral" icon={CircleAlert}>
          {refused[0].toUpperCase() + refused.slice(1)}.
        </Callout>
      )}
      {failure && <AiFailed error={failure} />}
      {tasks.length > 0 && (
        <div className="overflow-hidden rounded-lg border border-line">
          <div className="flex items-center gap-3 border-b border-line bg-surface-2 px-4 py-2.5">
            <Checkbox
              square
              checked={picked.size === tasks.length}
              onChange={(on) => setPicked(on ? new Set(tasks.map((_, i) => i)) : new Set())}
              label={picked.size === tasks.length ? "Pick none" : "Pick all"}
            />
            <span className="text-[13px] text-ink-muted">
              {picked.size} of {tasks.length} picked
            </span>
            <span className="ml-auto text-xs text-ink-subtle tabular-nums">{formatDuration(pickedMinutes * 60_000)} of work</span>
            <Button size="sm" tone="ghost" icon={Sparkles} onClick={ask} busy={since !== null}>
              Ask again
            </Button>
          </div>
          <div className="max-h-[min(26rem,44vh)] overflow-y-auto">
            {groups.map((g) => (
              <section key={g.day || "all"}>
                {quantity && (
                  <h4 className="sticky top-0 z-[1] flex items-center gap-2 border-b border-line bg-surface-1 px-4 py-2 text-xs font-medium text-ink-muted">
                    <CalendarDays size={13} aria-hidden />
                    {formatDate(g.day)}
                    <span className="font-normal text-ink-subtle tabular-nums">
                      · {g.indexes.length} {unit} · about {formatDuration(g.indexes.reduce((sum, i) => sum + tasks[i].estimate_minutes, 0) * 60_000)}
                    </span>
                  </h4>
                )}
                <ul className="divide-y divide-line">
                  {g.indexes.map((i) => {
                    const t = tasks[i];
                    const p = priorityOf(t.priority);
                    return (
                      <li key={i} className="flex items-start gap-3 px-4 py-3 hover:bg-surface-2/60">
                        <span className="pt-0.5">
                          <Checkbox square checked={picked.has(i)} onChange={() => toggle(i)} label={`Pick ${t.title}`} />
                        </span>
                        <button type="button" className="min-w-0 flex-1 text-left" onClick={() => toggle(i)}>
                          <span className={`block text-sm ${picked.has(i) ? "text-ink" : "text-ink-subtle"}`}>{t.title}</span>
                          {t.notes && (
                            <span className="mt-0.5 line-clamp-3 block text-xs leading-relaxed whitespace-pre-line text-ink-subtle" title={t.notes}>
                              {t.notes}
                            </span>
                          )}
                          <span className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-subtle">
                            <span className="inline-flex items-center gap-1">
                              <Clock size={12} aria-hidden />
                              {formatDuration(t.estimate_minutes * 60_000)}
                            </span>
                            {!quantity && <span>Due {formatDate(t.due_day)}</span>}
                            {!quantity && (
                              <span className="inline-flex items-center gap-1" style={{ color: p.color }}>
                                <Flag size={12} aria-hidden />
                                {p.label}
                              </span>
                            )}
                            {t.quantity != null && t.quantity !== 1 && (
                              <span>
                                {t.quantity} {unit}
                              </span>
                            )}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              </section>
            ))}
          </div>
          <div className="flex items-center justify-end gap-2 border-t border-line bg-surface-2 px-4 py-2.5">
            <Button onClick={discard} disabled={saving}>
              Discard
            </Button>
            <Button tone="primary" onClick={add} busy={saving} disabled={picked.size === 0}>
              {quantity ? `Line up ${picked.size}` : `Add ${picked.size} ${picked.size === 1 ? "task" : "tasks"}`}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

/** The Monday on or before day. */
export function mondayOf(day: string): string {
  const [y, m, d] = day.split("-").map(Number);
  return addDays(day, -((new Date(y, m - 1, d).getDay() + 6) % 7));
}

/** Stats → Week → Retro (docs/08-clients.md#v3-additions): a look back at one week; fixedWeek pins it to one. */
export function RetroFlow({ fixedWeek }: { fixedWeek?: string }) {
  const d = useDaemon();
  const thisWeek = mondayOf(d.today());
  const weeks = [0, 1, 2, 3].map((i) => addDays(thisWeek, -7 * i));
  const [picked, setWeek] = useState(weeks[1]);
  const week = fixedWeek ?? picked;
  const [out, setOut] = useState<RetroOutput | null>(null);
  const [since, setSince] = useState<number | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);

  async function generate() {
    setSince(Date.now());
    setUnavailable(null);
    try {
      const run = await App.Retro(wire.RetroRequest.createFrom({ week_start: week }));
      setOut(run.output as RetroOutput);
    } catch (e) {
      const err = apiError(e);
      if (err.code === "unavailable") setUnavailable(err.message);
      else d.fail(e);
    } finally {
      setSince(null);
    }
  }

  const label = (w: string, i: number) => (i === 0 ? "This week" : i === 1 ? "Last week" : formatDate(w, false));
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        {!fixedWeek && <Segmented
          value={week}
          onChange={(w) => {
            setWeek(w);
            setOut(null);
          }}
          options={weeks.map((w, i) => ({ value: w, label: label(w, i) }))}
          label="Week"
        />}
        <span className="text-[13px] text-ink-subtle tabular-nums">
          {formatDate(week, false)} to {formatDate(addDays(week, 6), false)}
        </span>
        <Button tone="primary" icon={ScrollText} onClick={generate} busy={since !== null} className="ml-auto">
          {out ? "Write again" : "Write retro"}
        </Button>
      </div>
      {since !== null && <AiProgress since={since} doing="Writing your retro" />}
      {unavailable && <AiUnavailable message={unavailable} />}
      {out && (
        <div className="rounded-lg border border-line bg-surface-2 p-5">
          <Markdown text={out.markdown} />
          {out.generated_by === "rules" && <p className="mt-4 border-t border-line pt-3 text-xs text-ink-subtle">Written from your numbers alone, because the AI did not answer.</p>}
        </div>
      )}
      {!out && since === null && !unavailable && <p className="text-[13px] text-ink-subtle">What went well, what slipped, and one or two things to try next week, written from that week's numbers.</p>}
    </div>
  );
}
