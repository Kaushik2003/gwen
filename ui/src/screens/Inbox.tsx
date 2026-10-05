import { Archive, CalendarClock, Check, CircleCheck, Hand, Inbox as InboxIcon, ListTodo, Plus, Sparkles, Timer, Trash2, Wand2, X } from "lucide-react";
import { useEffect, useState } from "react";
import { App, wire } from "../api";
import DurationInput from "../components/DurationInput";
import { useConfirm, useToast } from "../components/feedback";
import ScheduleInput, { atOn, efforts, type Block } from "../components/ScheduleInput";
import { ProjectTag } from "../components/tags";
import { Button, Empty, Field, IconButton, Input, Modal, PageHeader, Panel, Segmented, Select, cx, priorities } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo, parseDuration } from "../format";
import { useNav } from "../nav";

/**
 * Inbox (Getting Things Done): capture everything here without sorting it,
 * then process it into actions, the calendar, someone else's hands, someday,
 * or the trash.
 */
export default function Inbox() {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const { go } = useNav();
  const [text, setText] = useState("");
  const [capturing, setCapturing] = useState(false);
  const [processing, setProcessing] = useState(false);
  const inbox = d.tasks.filter((t) => t.stage === "inbox" && !t.parent_id);
  const someday = d.tasks.filter((t) => t.stage === "someday" && !t.parent_id);
  const waiting = d.tasks.filter((t) => t.stage === "waiting" && !t.parent_id);

  async function capture() {
    // One task per line, so a pasted list lands as several.
    const lines = text
      .split("\n")
      .map((l) => l.replace(/^[\s\-*•\d.)]+/, "").trim())
      .filter(Boolean);
    if (!lines.length) return;
    setCapturing(true);
    let n = 0;
    for (const title of lines) if (await d.act(() => App.CreateTask(wire.CreateTaskRequest.createFrom({ title: title.slice(0, 200), stage: "inbox" })))) n++;
    setCapturing(false);
    if (n) {
      setText("");
      notify(n === 1 ? "Captured to your inbox" : `Captured ${n} things to your inbox`, "success");
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Inbox"
        subtitle="Get it out of your head: capture everything here, sort it later."
        actions={
          inbox.length > 0 && (
            <>
              <Button icon={Sparkles} onClick={() => go("assistant", "Process my inbox")}>
                Let the assistant sort it
              </Button>
              <Button tone="primary" icon={Wand2} onClick={() => setProcessing(true)}>
                Process inbox
              </Button>
            </>
          )
        }
      />

      <div className="lift rounded-xl border border-line bg-surface-1 p-4">
        <form
          className="flex items-start gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            capture();
          }}
        >
          <textarea
            className="field min-h-10 flex-1 resize-y"
            rows={text.includes("\n") ? 4 : 1}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                capture();
              }
            }}
            placeholder="What's on your mind? Buy printer ink, email Sam about the trip…"
            aria-label="Capture to inbox"
            autoFocus
          />
          <Button type="submit" tone="primary" icon={Plus} busy={capturing} disabled={!text.trim()}>
            Capture
          </Button>
        </form>
        <p className="mt-2 text-xs text-ink-faint">Enter captures; Shift+Enter for a new line, and each line becomes its own item.</p>
      </div>

      <Panel title={`To process · ${inbox.length}`} icon={InboxIcon}>
        {inbox.length === 0 ? (
          <Empty icon={CircleCheck} title="Inbox zero">
            Everything is sorted. Capture new things above as they come up.
          </Empty>
        ) : (
          <ul className="-my-2 divide-y divide-line">
            {inbox.map((t) => (
              <li key={t.id} className="group flex items-center gap-3 py-2.5">
                <span className="min-w-0 flex-1 truncate text-sm text-ink">{t.title}</span>
                <span className="text-xs text-ink-faint">{formatAgo(t.created_at, d.now())}</span>
                <IconButton
                  icon={Trash2}
                  label="Delete"
                  size="sm"
                  className="opacity-0 group-hover:opacity-100"
                  onClick={async () => {
                    if (await confirm({ title: `Delete "${t.title}"?`, confirm: "Delete", danger: true })) d.act(() => App.DeleteTask(t.id));
                  }}
                />
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <div className="grid gap-5 @2xl:grid-cols-2">
        <Panel title={`Waiting for · ${waiting.length}`} icon={Hand}>
          {waiting.length === 0 ? (
            <p className="text-[13px] text-ink-subtle">Nothing handed off. Delegated tasks show here until they come back.</p>
          ) : (
            <ul className="-my-2 divide-y divide-line">
              {waiting.map((t) => (
                <li key={t.id} className="flex items-center gap-3 py-2">
                  <span className="min-w-0 flex-1 truncate text-[13px] text-ink">{t.title}</span>
                  {t.delegated_to && <span className="text-xs text-ink-subtle">{t.delegated_to}</span>}
                  <Button size="sm" onClick={() => d.act(() => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ stage: "todo" })))}>
                    It's back
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </Panel>
        <Panel title={`Someday / maybe · ${someday.length}`} icon={Archive}>
          {someday.length === 0 ? (
            <p className="text-[13px] text-ink-subtle">Ideas you parked. Look through them at your weekly review.</p>
          ) : (
            <ul className="-my-2 divide-y divide-line">
              {someday.map((t) => (
                <li key={t.id} className="flex items-center gap-3 py-2">
                  <span className="min-w-0 flex-1 truncate text-[13px] text-ink">{t.title}</span>
                  <ProjectTag project={d.projects.find((p) => p.id === t.project_id)} />
                  <Button size="sm" onClick={() => d.act(() => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ stage: "todo" })))}>
                    Activate
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>

      {processing && <Process tasks={inbox} onClose={() => setProcessing(false)} />}
    </div>
  );
}

type Step = "actionable" | "not" | "two" | "how" | "delegate" | "calendar" | "next";

/** One inbox item at a time, through the GTD questions. */
function Process({ tasks, onClose }: { tasks: wire.Task[]; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const today = d.today();
  // The list as it was when processing began; items leave the inbox as they are sorted.
  const [queue] = useState(tasks);
  const [i, setI] = useState(0);
  const [step, setStep] = useState<Step>("actionable");
  const [who, setWho] = useState("");
  const [block, setBlock] = useState<Block | null>(null);
  const [project, setProject] = useState("");
  const [estimate, setEstimate] = useState("30m");
  const [effort, setEffort] = useState(0);
  const [priority, setPriority] = useState(2);
  const [busy, setBusy] = useState(false);
  const t = queue[i];

  useEffect(() => {
    setStep("actionable");
    setWho("");
    setBlock({ day: today, minute: 17 * 60, length: "30m" });
    setProject(t?.project_id ?? "");
    setEstimate("30m");
    setEffort(0);
    setPriority(t?.priority ?? 2);
  }, [i]);

  if (!t)
    return (
      <Modal title="Inbox processed" size="sm" onClose={onClose} footer={<Button tone="primary" onClick={onClose}>Done</Button>}>
        <p className="text-sm text-ink-muted">Every item has a place now. Your mind can let go of them.</p>
      </Modal>
    );

  async function settle(call: () => Promise<unknown>, said: string) {
    setBusy(true);
    const ok = await d.run(call);
    setBusy(false);
    if (ok) {
      notify(said, "success");
      setI((n) => n + 1);
    }
  }
  const patch = (req: Record<string, unknown>) => () => App.PatchTask(t.id, wire.PatchTaskRequest.createFrom(req));

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
      title={`Process inbox · ${i + 1} of ${queue.length}`}
      onClose={onClose}
      footer={
        <>
          {step !== "actionable" && (
            <Button tone="ghost" onClick={() => setStep(step === "delegate" || step === "calendar" || step === "next" ? "how" : step === "how" ? "two" : "actionable")}>
              Back
            </Button>
          )}
          <span className="flex-1" />
          <Button onClick={() => setI((n) => n + 1)}>Skip for now</Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="rounded-lg border border-line bg-canvas/60 px-4 py-3 text-[15px] font-medium text-ink">{t.title}</div>

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
            {choice(Trash2, "Trash", "It doesn't matter any more", () => settle(() => App.DeleteTask(t.id), `Deleted ${t.title}`))}
            {choice(Archive, "Someday / maybe", "Park it; look again at the weekly review", () => settle(patch({ stage: "someday" }), `Parked ${t.title} for someday`))}
          </div>
        )}

        {step === "two" && (
          <>
            <p className="text-sm text-ink-muted">Does it take less than two minutes?</p>
            <div className="grid gap-2 sm:grid-cols-2">
              {choice(Timer, "Yes: do it now", "Then tick it off here", () => settle(() => App.CompleteTask(t.id, wire.CompleteTaskRequest.createFrom({})), `Done: ${t.title}`))}
              {choice(ListTodo, "No, it takes longer", "Decide who and when", () => setStep("how"))}
            </div>
          </>
        )}

        {step === "how" && (
          <div className="grid gap-2">
            {choice(Hand, "Delegate it", "Someone else should do it; you wait for it", () => setStep("delegate"))}
            {choice(CalendarClock, "It has a set date or time", "Put it in your calendar", () => setStep("calendar"))}
            {choice(ListTodo, "It's my next action", "Give it a project; Gwen plans it by urgency", () => setStep("next"))}
          </div>
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
            <ScheduleInput value={block} today={today} onChange={(b) => setBlock(b ?? { day: today, minute: 17 * 60, length: "30m" })} />
            <Button
              tone="primary"
              className="self-end"
              busy={busy}
              disabled={!block || parseDuration(block.length) <= 0}
              onClick={() =>
                block &&
                settle(async () => {
                  const minutes = Math.round(parseDuration(block.length) / 60_000);
                  await App.PatchTask(t.id, wire.PatchTaskRequest.createFrom({ estimate_minutes: minutes, stage: "todo" }));
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
                  <option value="">None</option>
                  {d.projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="How long" compound hint="0 makes it a quick to-do you tick off.">
                <DurationInput value={estimate} onChange={setEstimate} label="How long" />
              </Field>
            </div>
            <Field label="Priority" compound>
              <Segmented value={priority} onChange={setPriority} label="Priority" className="self-start" options={priorities.map((p) => ({ value: p.value, label: p.label }))} />
            </Field>
            <Field label="How hard" compound>
              <Segmented value={effort} onChange={setEffort} label="How hard" className="self-start" options={efforts.map((e) => ({ value: e.value, label: e.label }))} />
            </Field>
            <Button
              tone="primary"
              className="self-end"
              busy={busy}
              onClick={() => {
                const minutes = Math.round(parseDuration(estimate) / 60_000);
                settle(patch({ stage: "todo", project_id: project || null, estimate_minutes: minutes > 0 ? minutes : null, priority, effort: effort || null }), `${t.title} is a next action`);
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
