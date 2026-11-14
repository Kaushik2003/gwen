import { Activity, Archive, Bot, CalendarClock, Check, CircleAlert, CircleCheck, Hand, Inbox as InboxIcon, ListTodo, LoaderCircle, Plus, Sparkles, Timer, Trash2, Wand2, X } from "lucide-react";
import { Fragment, useEffect, useRef, useState } from "react";
import { App, wire } from "../api";
import DurationInput from "../components/DurationInput";
import { useToast } from "../components/feedback";
import ScheduleInput, { atOn, efforts, type Block } from "../components/ScheduleInput";
import { Button, Field, Input, Modal, Segmented, Select, cx, priorities } from "../components/ui";
import { useDictation } from "../components/Voice";
import { useDaemon } from "../daemon";
import { formatClock, formatDuration, goDuration, parseDuration, relativeDay } from "../format";
import { useNav } from "../nav";
import { Card } from "../shell/bits";
import HandOff, { useClearOfSheet } from "../shell/HandOff";
import NeedsYou from "../shell/NeedsYou";
import { projectChoices } from "../shell/projects";
import { useWork } from "../shell/work";
import { stageName } from "../task/status";
import TaskRow from "../task/TaskRow";
import { useTaskUI } from "../task/TaskUI";

/**
 * Inbox (plan.md 3.15, board 3; Getting Things Done): what came in and has no
 * place yet. Capture everything here without sorting it, then give each thing
 * its place: plan it, start it, hand it to an agent, or process the lot one by
 * one into actions, the calendar, someone else's hands, someday, or the trash.
 * Needs you is the other view: sessions and nudges waiting on you.
 */

export type InboxView = "needs" | "captured";

export default function Inbox({ view, onView }: { view?: InboxView; onView?: (v: InboxView) => void }) {
  const d = useDaemon();
  const w = useWork();
  const ui = useTaskUI();
  const [own, setOwn] = useState<InboxView>(view ?? "captured");
  const shown = view ?? own;
  const root = useRef<HTMLDivElement>(null);
  const pad = useClearOfSheet(root, !!ui.handing);
  const open = d.tasks.filter((t) => t.status === "open" && !t.parent_id);
  const running = open.filter((t) => ui.statusOf(t).live);
  const inbox = open.filter((t) => t.stage === "inbox" && !ui.statusOf(t).live).sort((a, b) => b.created_at - a.created_at);

  const views: { id: InboxView; label: string; count: number }[] = [
    { id: "needs", label: "Needs you", count: w.needs.length },
    { id: "captured", label: stageName("inbox"), count: inbox.length },
  ];

  return (
    <div ref={root} className="flex flex-col gap-4" style={pad ? { paddingRight: pad } : undefined}>
      <header className="flex flex-wrap items-end gap-3">
        <div className="min-w-64 flex-1">
          <h1 className="text-[22px] font-semibold tracking-[-0.01em]">Inbox</h1>
          <p className="mt-1 text-[13px] text-ink-muted">What came in and has no place yet. Plan it, start it, or hand it to an agent.</p>
        </div>
        <div role="tablist" aria-label="Inbox views" className="inline-flex gap-0.5 rounded-[9px] border border-line bg-surface-1 p-[3px]">
          {views.map((v) => {
            const on = v.id === shown;
            return (
              <button
                key={v.id}
                type="button"
                role="tab"
                aria-selected={on}
                onClick={() => (onView ? onView(v.id) : setOwn(v.id))}
                className={cx("inline-flex h-[26px] items-center gap-1.5 rounded-md px-2.5 font-medium transition-colors", on ? "bg-surface-3 text-ink" : "text-ink-muted hover:text-ink")}
              >
                {v.label}
                {v.count > 0 && <span className={cx("text-[11px] tabular-nums", v.id === "needs" ? "font-semibold text-warning" : "text-ink-subtle")}>{v.count}</span>}
              </button>
            );
          })}
        </div>
      </header>

      {shown === "needs" ? (
        w.needs.length === 0 ? (
          <NeedsYou />
        ) : (
          <Card title="Needs you" icon={CircleAlert} count={w.needs.length} countTone="warning" meta="Answer here, in the session, or on your phone" bodyClassName="">
            <NeedsYou />
          </Card>
        )
      ) : (
        <ToProcess inbox={inbox} running={running} />
      )}
    </div>
  );
}

/** The To process view: live work, the capture box, and tasks to sort, wait for, or keep for someday. */
function ToProcess({ inbox, running }: { inbox: wire.Task[]; running: wire.Task[] }) {
  const d = useDaemon();
  const { go } = useNav();
  const [planning, setPlanning] = useState<string | null>(null);
  const [processing, setProcessing] = useState(false);
  const name = d.config?.llm.assistant_name || "Gwen";
  const ui = useTaskUI();
  // A task shows in one place only: one that is running now sits under Running now, not here as well.
  const someday = d.tasks.filter((t) => t.status === "open" && t.stage === "someday" && !t.parent_id && !ui.statusOf(t).live);
  const waiting = d.tasks.filter((t) => t.status === "open" && t.stage === "waiting" && !t.parent_id && !ui.statusOf(t).live);
  const loading = d.tasksVersion === 0;

  return (
    <>
      {running.length > 0 && (
        <Card title="Running now" icon={Activity} count={running.length} className="border-working/35" bodyClassName="">
          <p className="px-4 pt-3 pb-2 text-[12.5px] text-ink-subtle">What you or an agent are working on right now.</p>
          <ul>{running.map((t, i) => <TaskRow key={t.id} task={t} first={i === 0} />)}</ul>
        </Card>
      )}

      <Capture />

      <Card
        title={stageName("inbox")}
        icon={InboxIcon}
        count={inbox.length}
        bodyClassName=""
        actions={
          inbox.length > 0 && (
            <>
              <Button size="sm" tone="ghost" icon={Sparkles} onClick={() => go("assistant", "Process my inbox")}>
                Sort it with {name}
              </Button>
              <Button size="sm" icon={Wand2} onClick={() => setProcessing(true)}>
                Process one by one
              </Button>
            </>
          )
        }
      >
        {loading ? (
          <p className="flex items-center gap-2 px-4 py-4 text-[12.5px] text-ink-subtle" role="status">
            <LoaderCircle size={14} className="animate-spin" aria-hidden />
            Loading your inbox
          </p>
        ) : inbox.length === 0 ? (
          <div className="flex flex-col items-center gap-1 px-6 py-8 text-center">
            <CircleCheck size={18} className="mb-1 text-ink-subtle" aria-hidden />
            <p className="font-medium">Inbox zero</p>
            <p className="text-[12.5px] text-ink-subtle">Everything has a place. Capture new things above as they come up.</p>
          </div>
        ) : (
          <ul>
            {inbox.map((t, i) => (
              <Fragment key={t.id}>
                <TaskRow
                  task={t}
                  first={i === 0}
                  quietStatus={["inbox"]}
                  controls={
                    <Button size="sm" aria-expanded={planning === t.id} className={cx("self-center", planning === t.id && "border-line-3 bg-surface-3")} onClick={() => setPlanning(planning === t.id ? null : t.id)}>
                      Plan
                    </Button>
                  }
                />
                {planning === t.id && <li><PlanIt task={t} onClose={() => setPlanning(null)} /></li>}
              </Fragment>
            ))}
          </ul>
        )}
      </Card>

      {(waiting.length > 0 || someday.length > 0) && (
        <div className="grid items-start gap-4 @2xl:grid-cols-2">
          {waiting.length > 0 && <Waiting tasks={waiting} />}
          {someday.length > 0 && <Someday tasks={someday} />}
        </div>
      )}

      {processing && <Process tasks={inbox} onClose={() => setProcessing(false)} />}
    </>
  );
}

/** The capture box: one task per line, so a pasted list lands as several; the mic dictates into it. */
function Capture() {
  const d = useDaemon();
  const notify = useToast();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false); // Enter and the button both capture: once per press, however quick
  const field = useRef<HTMLTextAreaElement>(null);
  const before = useRef(""); // what the box held when the mic opened; the words said follow it
  const voice = useDictation(
    {
      onStart: () => {
        before.current = text;
        field.current?.focus();
      },
      onText: (said, final) => {
        setText(said ? (before.current.trim() ? `${before.current.trimEnd()} ${said}` : said) : before.current);
        if (final) field.current?.focus();
      },
    },
    busy,
  );
  const lines = text
    .split("\n")
    .map((l) => l.replace(/^[\s\-*•\d.)]+/, "").trim())
    .filter(Boolean);

  async function capture() {
    if (!lines.length || inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    let n = 0;
    for (const title of lines) if (await d.act(() => App.CreateTask(wire.CreateTaskRequest.createFrom({ title: title.slice(0, 200), stage: "inbox" })))) n++;
    setBusy(false);
    inFlight.current = false;
    if (n) {
      setText("");
      notify(n === 1 ? "Captured to your inbox" : `Captured ${n} things to your inbox`, "success");
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <form
        className="flex items-end gap-2 rounded-xl border border-line bg-surface-1 py-1.5 pr-1.5 pl-3.5 transition-colors focus-within:border-accent"
        onSubmit={(e) => {
          e.preventDefault();
          capture();
        }}
      >
        <label htmlFor="inbox-capture" className="sr-only">
          Capture
        </label>
        <textarea
          id="inbox-capture"
          ref={field}
          rows={Math.min(6, text.split("\n").length)}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              capture();
            }
          }}
          placeholder="Capture anything: a task, a link, a thought. Enter adds it."
          className="min-h-8 min-w-0 flex-1 resize-none bg-transparent py-1.5 leading-5 text-ink outline-none placeholder:text-ink-subtle"
          autoFocus
        />
        <div className="flex h-8 shrink-0 items-center gap-1.5">
          {voice.button}
          {lines.length > 0 && (
            <Button type="submit" size="sm" tone="primary" icon={Plus} busy={busy}>
              {lines.length > 1 ? `Capture ${lines.length}` : "Capture"}
            </Button>
          )}
        </div>
      </form>
      {voice.panel}
      {text.includes("\n") && <p className="px-1 text-[11.5px] text-ink-subtle">Each line becomes its own item. Shift+Enter starts a new line.</p>}
    </div>
  );
}

/** Plan a captured thing: its project and length, and either a set time or Gwen fitting it into your days. */
function PlanIt({ task: t, onClose }: { task: wire.Task; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const today = d.today();
  const name = d.config?.llm.assistant_name || "Gwen";
  const [project, setProject] = useState(t.project_id ?? "");
  const [estimate, setEstimate] = useState(t.estimate_minutes ? goDuration(t.estimate_minutes * 60_000) : "30m");
  const [block, setBlock] = useState<Block | null>(null);
  const [busy, setBusy] = useState(false);
  const length = block ? block.length : estimate;
  const minutes = Math.round(parseDuration(length) / 60_000);

  async function save() {
    setBusy(true);
    const ok = await d.run(async () => {
      await App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ stage: "todo", project_id: project || null, estimate_minutes: minutes > 0 ? minutes : null }));
      if (block) await App.ScheduleTask(wire.ScheduleRequest.createFrom({ task_id: t.id, day: block.day, start_at: atOn(block.day, block.minute), planned_minutes: minutes }));
    });
    setBusy(false);
    if (!ok) return;
    notify(block ? `${t.title} is on ${relativeDay(block.day, today)} at ${formatClock(block.minute)}` : `${t.title} is planned; ${name} fits it in`, "success");
    onClose();
  }

  return (
    <div className="flex flex-col gap-3.5 border-t border-chrome-line px-4 pt-3 pb-3.5 pl-[46px]">
      <div className="flex flex-wrap items-start gap-x-5 gap-y-3">
        <Field label="Project" className="w-52">
          <Select value={project} onChange={(e) => setProject(e.target.value)}>
            <option value="">No project</option>
            {projectChoices(d.projects).map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </Select>
        </Field>
        {!block && (
          <Field label="How long" compound hint="0 makes it a quick to-do you tick off.">
            <DurationInput value={estimate} onChange={setEstimate} label="How long" />
          </Field>
        )}
      </div>
      <Field label="When" compound>
        <ScheduleInput value={block} today={today} onChange={setBlock} />
      </Field>
      <div className="flex gap-2">
        <Button size="sm" tone="primary" icon={CalendarClock} busy={busy} disabled={!!block && minutes <= 0} onClick={save}>
          {block ? "Put it on the calendar" : "Plan it"}
        </Button>
        <Button size="sm" tone="ghost" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

/** Handed to someone, or to an agent: shown until it comes back. */
function Waiting({ tasks }: { tasks: wire.Task[] }) {
  return (
    <Card title={stageName("waiting")} icon={Hand} count={tasks.length} bodyClassName="">
      <ul>
        {tasks.map((t, i) => <TaskRow key={t.id} task={t} first={i === 0} quietStatus={["waiting"]} />)}
      </ul>
    </Card>
  );
}

/** Ideas parked for the weekly review. */
function Someday({ tasks }: { tasks: wire.Task[] }) {
  const ui = useTaskUI();
  return (
    <Card title={stageName("someday")} icon={Archive} count={tasks.length} bodyClassName="">
      <ul>
        {tasks.map((t, i) => (
          <TaskRow
            key={t.id}
            task={t}
            first={i === 0}
            quietStatus={["someday"]}
            controls={
              <Button size="sm" className="self-center" onClick={() => ui.move(t, "todo")}>
                Activate
              </Button>
            }
          />
        ))}
      </ul>
    </Card>
  );
}

type Step = "actionable" | "not" | "two" | "how" | "delegate" | "agent" | "calendar" | "next";

/** One inbox item at a time, through the GTD questions. */
function Process({ tasks, onClose }: { tasks: wire.Task[]; onClose: () => void }) {
  // Keep the original order, but read each task's current fields and skip items already processed elsewhere.
  const [queue] = useState(() => tasks.map((t) => t.id));
  const [from, setFrom] = useState(0);
  const i = queue.findIndex((id, index) => index >= from && tasks.some((t) => t.id === id));
  const t = tasks.find((t) => t.id === queue[i]);

  useEffect(() => {
    if (i > from) setFrom(i);
  }, [i, from]);

  if (!t)
    return (
      <Modal title="Inbox processed" size="sm" onClose={onClose} footer={<Button tone="primary" onClick={onClose}>Done</Button>}>
        <p className="text-sm text-ink-muted">You've reached the end of this batch. Anything you skipped stays in {stageName("inbox")}.</p>
      </Modal>
    );

  return <ProcessTask key={t.id} task={t} position={i + 1} total={queue.length} onNext={() => setFrom(i + 1)} onClose={onClose} />;
}

function ProcessTask({ task: t, position, total, onNext, onClose }: { task: wire.Task; position: number; total: number; onNext: () => void; onClose: () => void }) {
  const d = useDaemon();
  const ui = useTaskUI();
  const notify = useToast();
  const today = d.today();
  const length = t.estimate_minutes != null ? goDuration(t.estimate_minutes * 60_000) : "30m";
  const [step, setStep] = useState<Step>("actionable");
  const [who, setWho] = useState("");
  const [block, setBlock] = useState<Block | null>({ day: today, minute: 17 * 60, length });
  const [project, setProject] = useState(t.project_id ?? "");
  const [estimate, setEstimate] = useState(length);
  const [effort, setEffort] = useState(t.effort ?? 0);
  const [priority, setPriority] = useState(t.priority);
  const [busy, setBusy] = useState(false);

  async function settle(call: () => Promise<unknown>, said: string) {
    setBusy(true);
    const ok = await d.run(call);
    setBusy(false);
    if (ok) {
      notify(said, "success");
      onNext();
    }
  }
  const patch = (req: Partial<wire.PatchTaskRequest>) => () => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ ...req, rev: t.rev }));

  const choice = (icon: typeof Check, title: string, sub: string, onClick: () => void, tone?: string) => {
    const Icon = icon;
    return (
      <button type="button" onClick={onClick} disabled={busy} className={cx("flex items-start gap-3 rounded-lg border border-line-strong bg-surface-2 p-3 text-left transition-colors hover:border-accent", tone)}>
        <Icon size={18} className="mt-0.5 shrink-0 text-accent-hover" aria-hidden />
        <span>
          <span className="block text-sm font-medium text-ink">{title}</span>
          <span className="block text-xs text-ink-subtle">{sub}</span>
        </span>
      </button>
    );
  };

  return (
    <Modal
      title={`Process inbox · ${position} of ${total}`}
      onClose={onClose}
      footer={
        <>
          {step !== "actionable" && (
            <Button tone="ghost" onClick={() => setStep(step === "delegate" || step === "agent" || step === "calendar" || step === "next" ? "how" : step === "how" ? "two" : "actionable")}>
              Back
            </Button>
          )}
          <span className="flex-1" />
          <Button disabled={busy} onClick={onNext}>Skip for now</Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <button type="button" onClick={() => ui.edit(t)} className="rounded-lg border border-line bg-canvas/60 px-4 py-3 text-left text-[15px] font-medium text-ink hover:underline">{t.title}</button>

        {step === "actionable" && (
          <>
            <p className="text-sm text-ink-muted">Is it actionable: is there something you can do about it?</p>
            <div className="grid gap-2 sm:grid-cols-2">
              {choice(Check, "Yes", "There is a next step", () => setStep("two"))}
              {choice(X, "No", "Trash it, or keep it for someday", () => setStep("not"))}
            </div>
          </>
        )}

        {step === "not" && (
          <div className="grid gap-2 sm:grid-cols-2">
            {choice(Trash2, "Trash", "It doesn't matter any more", () => ui.remove(t))}
            {choice(Archive, stageName("someday"), "Park it; look again at the weekly review", () => ui.move(t, "someday"))}
          </div>
        )}

        {step === "two" && (
          <>
            <p className="text-sm text-ink-muted">Does it take less than two minutes?</p>
            <div className="grid gap-2 sm:grid-cols-2">
              {choice(Timer, "Yes: do it now", "Then tick it off here", () => ui.complete(t))}
              {choice(ListTodo, "No, it takes longer", "Decide who and when", () => setStep("how"))}
            </div>
          </>
        )}

        {step === "how" && (
          <div className="grid gap-2">
            {choice(Hand, "Delegate it", "Someone else should do it; you wait for it", () => setStep("delegate"))}
            {choice(CalendarClock, "It has a set date or time", "Put it in your calendar", () => setStep("calendar"))}
            {choice(ListTodo, "It's my next action", "Give it a project; Gwen plans it by urgency", () => setStep("next"))}
            {choice(Bot, "Hand it to an agent", "A Claude session in its project does it; you review", () => setStep("agent"))}
          </div>
        )}

        {step === "agent" && (
          <HandOff
            task={t}
            onDone={(said) => {
              notify(said, "success");
              onNext();
            }}
          />
        )}

        {step === "delegate" && (
          <>
            <Field label="Who are you handing it to?">
              <Input value={who} onChange={(e) => setWho(e.target.value)} placeholder="Alex" autoFocus maxLength={200} />
            </Field>
            <Button tone="primary" className="self-end" busy={busy} disabled={!who.trim()} onClick={() => settle(patch({ stage: "waiting", delegated_to: who }), `Waiting on ${who} for ${t.title}`)}>
              Delegate
            </Button>
          </>
        )}

        {step === "calendar" && (
          <>
            <ScheduleInput value={block} today={today} onChange={(b) => setBlock(b ?? { day: today, minute: 17 * 60, length })} />
            <Button
              tone="primary"
              className="self-end"
              busy={busy}
              disabled={!block || parseDuration(block.length) <= 0}
              onClick={() =>
                block &&
                settle(async () => {
                  const minutes = Math.round(parseDuration(block.length) / 60_000);
                  await patch({ stage: "todo", ...(t.estimate_minutes == null ? { estimate_minutes: minutes } : {}) })();
                  return App.ScheduleTask(wire.ScheduleRequest.createFrom({ task_id: t.id, day: block.day, start_at: atOn(block.day, block.minute), planned_minutes: minutes }));
                }, `Scheduled ${t.title}`)
              }
            >
              Put it on the calendar
            </Button>
          </>
        )}

        {step === "next" && (
          <>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Project">
                <Select value={project} onChange={(e) => setProject(e.target.value)} autoFocus>
                  <option value="">No project</option>
                  {projectChoices(d.projects).map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="How long" compound hint={t.estimate_minutes != null ? "Already set on this task." : "0 makes it a quick to-do you tick off."}>
                {t.estimate_minutes != null ? <p className="text-sm text-ink">{formatDuration(t.estimate_minutes * 60_000)}</p> : <DurationInput value={estimate} onChange={setEstimate} label="How long" />}
              </Field>
            </div>
            <Field label="Priority" compound>
              <Segmented value={priority} onChange={setPriority} label="Priority" className="self-start" options={priorities.map((p) => ({ value: p.value, label: p.label }))} />
            </Field>
            <Field label="How hard" compound hint={t.effort != null ? "Already set on this task." : undefined}>
              {t.effort != null ? (
                <p className="text-sm text-ink">{efforts.find((e) => e.value === t.effort)?.label ?? "Not rated"}</p>
              ) : (
                <Segmented value={effort} onChange={setEffort} label="How hard" className="self-start" options={efforts.map((e) => ({ value: e.value, label: e.label }))} />
              )}
            </Field>
            <Button
              tone="primary"
              className="self-end"
              busy={busy}
              onClick={() => {
                const minutes = Math.round(parseDuration(estimate) / 60_000);
                settle(patch({ stage: "todo", project_id: project || null, priority, ...(t.estimate_minutes == null ? { estimate_minutes: minutes > 0 ? minutes : null } : {}), ...(t.effort == null ? { effort: effort || null } : {}) }), `${t.title} is a next action`);
              }}
            >
              Make it a next action
            </Button>
          </>
        )}
      </div>
    </Modal>
  );
}
