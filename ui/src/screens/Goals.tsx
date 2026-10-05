import { CalendarRange, CircleCheck, Clock, Hash, ListChecks, Pencil, Plus, Sparkles, Target, Trash2, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { App, readOnly, wire } from "../api";
import Breakdown from "../components/Breakdown";
import DurationInput from "../components/DurationInput";
import { useConfirm, useToast } from "../components/feedback";
import { GoalMeter, PaceChip, goalFraction } from "../components/goal";
import Recurrence from "../components/Recurrence";
import TimeInput from "../components/TimeInput";
import { ProjectTag } from "../components/tags";
import { Badge, Button, Callout, Checkbox, Empty, Field, IconButton, Input, Modal, PageHeader, Panel, Segmented, Select, TextArea, ToggleRow, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { daysBetween, formatDate, formatDuration, goDuration, parseDuration } from "../format";
import { clockOf, describeRule, litWeekdays, weekdayCodes } from "../rrule";

export default function Goals() {
  const d = useDaemon();
  const [status, setStatus] = useState("active");
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [editing, setEditing] = useState<wire.Goal | "new" | null>(null);
  const [breakingDown, setBreakingDown] = useState<wire.Goal | null>(null);
  const [deleting, setDeleting] = useState<wire.Goal | null>(null);

  useEffect(() => {
    App.ListGoals(status).then((l) => {
      setGoals(l.goals);
      setLoaded(true);
    }, d.fail);
  }, [status, d.goalsVersion, d.tasksVersion, d.fail]);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Goals"
        subtitle="Targets with a date. Gwen paces them into your daily plan."
        actions={
          <>
            <Segmented
              value={status}
              onChange={setStatus}
              label="Show"
              options={[
                { value: "active", label: "Active" },
                { value: "done", label: "Done" },
                { value: "abandoned", label: "Abandoned" },
                { value: "all", label: "All" },
              ]}
            />
            {!readOnly && (
              <Button tone="primary" icon={Plus} onClick={() => setEditing("new")}>
                New goal
              </Button>
            )}
          </>
        }
      />

      {loaded && goals.length === 0 ? (
        <Empty
          icon={Target}
          title={status === "active" ? "No active goals" : "Nothing here"}
          action={
            !readOnly &&
            status === "active" && (
              <Button tone="primary" icon={Plus} onClick={() => setEditing("new")}>
                New goal
              </Button>
            )
          }
        >
          {status === "active" && "Try 300 problems by December, or a set of tasks such as finishing a course."}
        </Empty>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {goals.map((g) => (
            <GoalCard
              key={g.id}
              g={g}
              today={d.today()}
              project={d.projects.find((p) => p.id === g.project_id)}
              onEdit={() => setEditing(g)}
              onBreakdown={() => setBreakingDown(g)}
              onDelete={() => setDeleting(g)}
            />
          ))}
        </div>
      )}

      <Commitments />
      {editing && <GoalForm goal={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {breakingDown && <Breakdown goal={breakingDown} onClose={() => setBreakingDown(null)} />}
      {deleting && <DeleteGoal goal={deleting} onClose={() => setDeleting(null)} />}
    </div>
  );
}

/** Delete a goal, and by default its open tasks: the problems it lined up and anything else linked to it. */
function DeleteGoal({ goal: g, onClose }: { goal: wire.Goal; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [open, setOpen] = useState<number | null>(null);
  const [withTasks, setWithTasks] = useState(true);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    App.ListTasks(wire.TaskQuery.createFrom({ goal_id: g.id, status: "open" })).then(
      (l) => setOpen(l.tasks.filter((t) => !t.template_id).length),
      () => setOpen(0),
    );
  }, [g.id]);

  async function remove() {
    setBusy(true);
    const ok = await d.run(() => App.DeleteGoal(g.id, withTasks && (open ?? 0) > 0));
    setBusy(false);
    if (!ok) return;
    notify(`Deleted ${g.title}`, "success");
    onClose();
  }

  return (
    <Modal
      title={`Delete ${g.title}?`}
      size="sm"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="destroy" onClick={remove} busy={busy} disabled={open === null}>
            Delete goal
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4 text-sm leading-relaxed text-ink-muted">
        <p>{g.kind === "quantity" ? "Its daily session stops. " : ""}Time already tracked stays in your history.</p>
        {open !== null && open > 0 && (
          <label className="flex items-start gap-3 rounded-lg border border-line bg-surface-2 p-3">
            <Checkbox square checked={withTasks} onChange={setWithTasks} label="Also delete its open tasks" />
            <span>
              <span className="block text-ink">
                Also delete its {open} open {open === 1 ? "task" : "tasks"}
              </span>
              <span className="block text-xs text-ink-subtle">{withTasks ? "Done ones stay, so your history adds up." : "They stay, as ordinary tasks on your plan."}</span>
            </span>
          </label>
        )}
      </div>
    </Modal>
  );
}

function GoalCard({ g, today, project, onEdit, onBreakdown, onDelete }: { g: wire.Goal; today: string; project?: wire.Project; onEdit: () => void; onBreakdown: () => void; onDelete: () => void }) {
  const p = g.progress;
  const total = p.done_quantity + p.remaining_quantity;
  const unit = g.kind === "tasks" ? "tasks" : g.unit || "units";
  const left = daysBetween(today, g.due_day);
  const keepingUp = p.actual_per_day >= p.required_per_day;
  return (
    <article className="lift flex min-w-0 flex-col gap-4 rounded-xl border border-line bg-surface-1 p-5">
      <header className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-[15px] font-semibold tracking-[-0.01em] text-ink">{g.title}</h3>
          <div className="mt-1 flex items-center gap-3">
            {project && <ProjectTag project={project} />}
            <span className="text-xs text-ink-subtle">{goalShape(g)}</span>
          </div>
        </div>
        {g.status === "active" ? <PaceChip pace={p.pace} /> : <Badge>{g.status === "done" ? "Done" : "Abandoned"}</Badge>}
      </header>
      <SmartSummary g={g} />

      <div>
        <div className="mb-2.5 flex items-baseline gap-2">
          <span className="text-title font-semibold text-ink tabular-nums">{p.done_quantity}</span>
          <span className="text-sm text-ink-subtle">
            of {total} {unit}
          </span>
          <span className="ml-auto text-sm font-medium text-ink-muted tabular-nums">{Math.round(goalFraction(g) * 100)}%</span>
        </div>
        <GoalMeter g={g} today={today} height={8} />
      </div>

      <dl className="grid grid-cols-3 gap-3">
        <Stat label="Needed a day" value={p.required_per_day.toFixed(1)} />
        <Stat label="Doing a day" value={p.actual_per_day.toFixed(1)} tone={g.status !== "active" ? undefined : keepingUp ? "text-working" : "text-idle"} />
        <Stat label={left >= 0 ? `Due in ${left} ${left === 1 ? "day" : "days"}` : "Overdue"} value={formatDate(g.due_day)} />
      </dl>

      <p className="text-xs leading-relaxed text-ink-subtle">
        {p.projected_finish_day ? `At this pace you finish on ${formatDate(p.projected_finish_day)}.` : "Not enough done yet to say when you will finish."}
        {g.kind === "quantity" && ` ${p.lined_up} lined up.`}
      </p>

      {g.status === "active" && p.reachable_quantity < total && (
        <Callout
          tone="idle"
          icon={TriangleAlert}
          action={
            !readOnly && (
              <Button size="sm" onClick={onEdit}>
                Fix
              </Button>
            )
          }
        >
          At this time a day you reach {p.reachable_quantity} of {total} by {formatDate(g.due_day)}.
        </Callout>
      )}

      {!readOnly && (
        <footer className="-mb-1 flex items-center gap-2 border-t border-line pt-3">
          <Button size="sm" icon={Sparkles} iconColor="var(--color-accent-hover)" onClick={onBreakdown}>
            Break down with AI
          </Button>
          <div className="flex-1" />
          <IconButton icon={Pencil} label={`Edit ${g.title}`} size="sm" onClick={onEdit} />
          <IconButton icon={Trash2} label={`Delete ${g.title}`} size="sm" onClick={onDelete} />
        </footer>
      )}
    </article>
  );
}

/** "2h a day · 6 a session" for a quantity goal with a daily time. */
function goalShape(g: wire.Goal): string {
  if (g.kind === "tasks") return "A set of tasks";
  const each = `${g.minutes_per_unit ?? 0} min each`;
  if (g.daily_minutes == null) return each;
  return `${formatDuration(g.daily_minutes * 60_000)} a day · ${g.progress.per_session ?? 0} a session · ${each}`;
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="min-w-0">
      <dt className="truncate text-xs text-ink-subtle">{label}</dt>
      <dd className={cx("mt-0.5 text-sm font-medium tabular-nums", tone ?? "text-ink")}>{value}</dd>
    </div>
  );
}

function GoalForm({ goal, onClose }: { goal: wire.Goal | null; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [title, setTitle] = useState(goal?.title ?? "");
  const [kind, setKind] = useState(goal?.kind ?? "quantity");
  const [unit, setUnit] = useState(goal?.unit ?? "");
  const [target, setTarget] = useState(goal?.target_quantity ? String(goal.target_quantity) : "");
  const [perUnit, setPerUnit] = useState(goal?.minutes_per_unit ? String(goal.minutes_per_unit) : "");
  const [daily, setDaily] = useState(goDuration((goal?.daily_minutes ?? 0) * 60_000));
  const [preview, setPreview] = useState<wire.GoalProgress | null>(null);
  const [project, setProject] = useState(goal?.project_id ?? "");
  const [start, setStart] = useState(goal?.start_day ?? d.today());
  const [due, setDue] = useState(goal?.due_day ?? "");
  const [status, setStatus] = useState(goal?.status ?? "active");
  const [template, setTemplate] = useState<wire.Task | null>(null);
  const [rule, setRule] = useState("");
  const [smart, setSmart] = useState<Smart>({ specific: goal?.specific ?? "", measurable: goal?.measurable ?? "", assignable: goal?.assignable ?? "", realistic: goal?.realistic ?? "" });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!goal || goal.kind !== "quantity") return;
    App.ListTasks(
      wire.TaskQuery.createFrom({
        goal_id: goal.id,
        templates: true,
        status: "all",
      }),
    ).then((l) => {
      const t = l.tasks.find((x) => x.rrule);
      setTemplate(t ?? null);
      setRule(t?.rrule ?? "");
    }, d.fail);
  }, [goal, d.fail]);

  const quantity = kind === "quantity";
  const dailyMinutes = Math.round(parseDuration(daily) / 60_000);
  // What the goal would look like, checked by the daemon as it is typed.
  useEffect(() => {
    setPreview(null);
    if (!quantity || !(Number(target) > 0) || !(Number(perUnit) > 0) || !start || !due || due < start) return;
    const timer = setTimeout(() => {
      App.PreviewGoal(
        wire.GoalPreviewRequest.createFrom({
          goal_id: goal?.id ?? null,
          kind,
          target_quantity: Number(target),
          minutes_per_unit: Number(perUnit),
          daily_minutes: dailyMinutes >= 5 ? dailyMinutes : null,
          start_day: start,
          due_day: due,
        }),
      ).then(setPreview, () => setPreview(null));
    }, 250);
    return () => clearTimeout(timer);
  }, [quantity, kind, target, perUnit, dailyMinutes, start, due, goal?.id]);

  async function save() {
    setSaving(true);
    const fields = {
      title,
      unit,
      target_quantity: quantity ? Number(target) : null,
      minutes_per_unit: quantity ? Number(perUnit) : null,
      daily_minutes: quantity && dailyMinutes >= 5 ? dailyMinutes : null,
      project_id: project || null,
      start_day: start,
      due_day: due,
      ...smart,
    };
    const ok = goal
      ? await d.act(() =>
          App.PatchGoal(
            goal.id,
            wire.PatchGoalRequest.createFrom({
              ...fields,
              status,
              rev: goal.rev,
            }),
          ),
        )
      : await d.act(() => App.CreateGoal(wire.CreateGoalRequest.createFrom({ ...fields, kind })));
    if (ok && template && rule !== template.rrule) {
      const saved = await d.act(() => App.PatchTask(template.id, wire.PatchTaskRequest.createFrom({ rrule: rule, rev: template.rev })));
      if (!saved) {
        setSaving(false);
        return;
      }
    }
    setSaving(false);
    if (!ok) return;
    notify(goal ? `Saved ${title}` : `Created ${title}`, "success");
    onClose();
  }

  const valid = title.trim() && due && start && due >= start && (!quantity || (Number(target) > 0 && Number(perUnit) > 0 && (dailyMinutes === 0 || dailyMinutes >= 5)));
  return (
    <Modal
      title={goal ? "Edit goal" : "New goal"}
      size="lg"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} disabled={!valid} busy={saving}>
            {goal ? "Save goal" : "Create goal"}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <Field label="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Solve 300 DSA problems" autoFocus />
        </Field>
        {!goal && (
          <Field label="What kind of goal?" compound>
            <div className="grid grid-cols-2 gap-3">
              <KindTile icon={Hash} on={kind === "quantity"} onClick={() => setKind("quantity")} title="A number to reach" body="Such as 300 problems. Gwen plans a session for it each day." />
              <KindTile icon={ListChecks} on={kind === "tasks"} onClick={() => setKind("tasks")} title="A set of tasks" body="Finish tasks you add, or ones the assistant proposes." />
            </div>
          </Field>
        )}
        {quantity && (
          <>
            <div className="grid grid-cols-2 gap-3">
              <Field label="How many">
                <Input type="number" min="1" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="300" />
              </Field>
              <Field label="Of what">
                <Input value={unit} placeholder="problems" onChange={(e) => setUnit(e.target.value)} />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Time you can give it a day" compound hint="Each day's session is one block of this time.">
                <DurationInput value={daily} onChange={setDaily} label="Time a day" />
              </Field>
              <Field label={`Minutes for one ${singular(unit)}`} hint="Your average. It sizes each session.">
                <Input type="number" min="1" value={perUnit} onChange={(e) => setPerUnit(e.target.value)} placeholder="20" />
              </Field>
            </div>
          </>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Starts">
            <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
          </Field>
          <Field label="Due" error={due && start && due < start ? "The due day is before the start." : undefined}>
            <Input type="date" value={due} min={start} onChange={(e) => setDue(e.target.value)} />
          </Field>
        </div>
        {quantity && preview && (
          <Fit p={preview} target={Number(target)} unit={unit || "units"} due={due} onDaily={(m) => setDaily(goDuration(m * 60_000))} onDue={setDue} onTarget={(n) => setTarget(String(n))} />
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Project">
            <Select value={project} onChange={(e) => setProject(e.target.value)}>
              <option value="">None</option>
              {d.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </Field>
          {goal && (
            <Field label="Status" compound>
              <Segmented
                value={status}
                onChange={setStatus}
                className="self-start"
                options={[
                  { value: "active", label: "Active" },
                  { value: "done", label: "Done" },
                  { value: "abandoned", label: "Abandoned" },
                ]}
              />
            </Field>
          )}
        </div>
        {template && (
          <Field label="Daily session repeats" compound hint="Which days the goal gets a session on your plan.">
            <Recurrence value={rule} onChange={setRule} anchorDay={start} />
          </Field>
        )}
        <SmartFields
          value={smart}
          onChange={setSmart}
          measured={quantity && Number(target) > 0 ? `${target} ${unit || "units"}` : null}
          timed={start && due ? `${formatDate(start, false)} to ${formatDate(due, false)}` : null}
          reachable={quantity && preview ? preview.reachable_quantity >= Number(target) : null}
        />
      </div>
    </Modal>
  );
}

function singular(unit: string): string {
  if (!unit) return "unit";
  return unit.endsWith("s") ? unit.slice(0, -1) : unit;
}

/** Whether the goal fits its time a day, and when it does not, the three ways to make it fit. */
function Fit({
  p,
  target,
  unit,
  due,
  onDaily,
  onDue,
  onTarget,
}: {
  p: wire.GoalProgress;
  target: number;
  unit: string;
  due: string;
  onDaily: (minutes: number) => void;
  onDue: (day: string) => void;
  onTarget: (n: number) => void;
}) {
  const perDay = p.required_per_day;
  if (p.per_session == null) {
    return (
      <p className="-mt-2 flex items-center gap-2 text-[13px] text-ink-subtle">
        <CalendarRange size={14} aria-hidden />
        About {perDay < 10 ? perDay.toFixed(1) : Math.round(perDay)} {unit} a day. Set a time a day to check it fits.
      </p>
    );
  }
  if (p.reachable_quantity >= target) {
    return (
      <Callout tone="working" icon={CircleCheck}>
        It fits: each session holds up to {p.per_session} {unit}, and {Math.ceil(perDay)} a day reaches all {target} by {formatDate(due)}.
      </Callout>
    );
  }
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-idle/30 bg-idle/[0.06] p-4">
      <p className="flex items-start gap-2 text-[13px] leading-relaxed text-ink-muted">
        <TriangleAlert size={15} className="mt-0.5 shrink-0 text-idle" aria-hidden />
        <span>
          That time fits {p.per_session} {unit} a session, so you reach {p.reachable_quantity} of {target} by {formatDate(due)}. You need about {Math.ceil(perDay)} a day. Pick one:
        </span>
      </p>
      <div className="flex flex-wrap gap-2">
        {p.needed_daily_minutes != null && (
          <Button size="sm" icon={Clock} onClick={() => onDaily(p.needed_daily_minutes!)}>
            Give it {formatDuration(p.needed_daily_minutes * 60_000)} a day
          </Button>
        )}
        {p.needed_due_day && (
          <Button size="sm" icon={CalendarRange} onClick={() => onDue(p.needed_due_day!)}>
            Finish by {formatDate(p.needed_due_day)}
          </Button>
        )}
        <Button size="sm" icon={Target} onClick={() => onTarget(p.reachable_quantity)}>
          Aim for {p.reachable_quantity}
        </Button>
      </div>
    </div>
  );
}

function KindTile({ icon: Icon, on, onClick, title, body }: { icon: typeof Hash; on: boolean; onClick: () => void; title: string; body: string }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cx("flex flex-col gap-1.5 rounded-lg border p-3.5 text-left transition-colors", on ? "border-accent bg-accent/10" : "border-line-strong bg-surface-2 hover:border-line-3")}
    >
      <Icon size={18} className={on ? "text-accent-hover" : "text-ink-subtle"} aria-hidden />
      <span className="text-sm font-medium text-ink">{title}</span>
      <span className="text-xs leading-relaxed text-ink-subtle">{body}</span>
    </button>
  );
}

const dayLetters = ["M", "T", "W", "T", "F", "S", "S"];

/** The weekdays a rule falls on as seven dots, or the rule in words when it is not weekly. */
function WeekdayDots({ rule }: { rule: string }) {
  const lit = litWeekdays(rule);
  if (!lit) return <span className="w-34 shrink-0 truncate text-xs text-ink-subtle">{describeRule(rule)}</span>;
  return (
    <span className="flex w-34 shrink-0 gap-1" title={describeRule(rule)} aria-label={describeRule(rule)}>
      {weekdayCodes.map((c, i) => (
        <span key={c} aria-hidden className={cx("grid size-4 place-items-center rounded-full text-[9px] font-semibold", lit.has(c) ? "bg-accent/80 text-white" : "bg-surface-3 text-ink-faint")}>
          {dayLetters[i]}
        </span>
      ))}
    </span>
  );
}

function Commitments() {
  const d = useDaemon();
  const confirm = useConfirm();
  const [list, setList] = useState<wire.Commitment[]>([]);
  const [editing, setEditing] = useState<wire.Commitment | "new" | null>(null);
  useEffect(() => {
    App.ListCommitments().then((l) => setList(l.commitments), d.fail);
  }, [d.goalsVersion, d.fail]);

  async function remove(c: wire.Commitment) {
    if (
      await confirm({
        title: `Delete ${c.title}?`,
        body: "It stops reserving time on your plans.",
        confirm: "Delete commitment",
        danger: true,
      })
    )
      d.run(() => App.DeleteCommitment(c.id));
  }

  return (
    <Panel
      title="Commitments"
      icon={CalendarRange}
      actions={
        !readOnly && (
          <Button size="sm" icon={Plus} onClick={() => setEditing("new")}>
            New commitment
          </Button>
        )
      }
    >
      <p className="mb-3 text-[13px] text-ink-subtle">Recurring time that is not planned work, such as an internship or a class. Plans leave room for it.</p>
      {list.length === 0 ? (
        <p className="text-[13px] text-ink-faint">No commitments.</p>
      ) : (
        <ul className="-mx-2 flex flex-col">
          {list.map((c) => (
            <li key={c.id} className="flex items-center gap-4 rounded-lg px-2 py-2.5 hover:bg-surface-2/60">
              <WeekdayDots rule={c.rrule} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm text-ink">{c.title}</div>
                <div className="mt-0.5 flex items-center gap-3">
                  {c.project_id && <ProjectTag project={d.projects.find((p) => p.id === c.project_id)} />}
                  {!c.counts_toward_target && <span className="text-xs text-ink-faint">Not counted toward the target</span>}
                  {c.active_until && <span className="text-xs text-ink-subtle">Until {formatDate(c.active_until)}</span>}
                </div>
              </div>
              <span className="w-28 text-right text-[13px] text-ink-muted tabular-nums">
                {c.start_minute == null ? "Any time" : `${clockOf(c.start_minute)}–${clockOf(c.start_minute + c.duration_minutes)}`}
              </span>
              <span className="w-14 text-right text-[13px] text-ink-subtle tabular-nums">{formatDuration(c.duration_minutes * 60_000)}</span>
              {!readOnly && (
                <span className="flex gap-0.5">
                  <IconButton icon={Pencil} label={`Edit ${c.title}`} size="sm" onClick={() => setEditing(c)} />
                  <IconButton icon={Trash2} label={`Delete ${c.title}`} size="sm" onClick={() => remove(c)} />
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      {editing && <CommitmentForm commitment={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
    </Panel>
  );
}

function CommitmentForm({ commitment: c, onClose }: { commitment: wire.Commitment | null; onClose: () => void }) {
  const d = useDaemon();
  const [title, setTitle] = useState(c?.title ?? "");
  const [rule, setRule] = useState(c?.rrule ?? "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR");
  const [anyTime, setAnyTime] = useState(c ? c.start_minute == null : false);
  const [at, setAt] = useState(c?.start_minute != null ? clockOf(c.start_minute) : "10:00");
  const [duration, setDuration] = useState(goDuration((c?.duration_minutes ?? 60) * 60_000));
  const [project, setProject] = useState(c?.project_id ?? "");
  const [counts, setCounts] = useState(c?.counts_toward_target ?? true);
  const [from, setFrom] = useState(c?.active_from ?? d.today());
  const [until, setUntil] = useState(c?.active_until ?? "");
  const [saving, setSaving] = useState(false);
  const minutes = Math.round(parseDuration(duration) / 60_000);

  async function save() {
    setSaving(true);
    const [h, m] = at.split(":").map(Number);
    const fields = {
      title,
      rrule: rule,
      start_minute: anyTime || !at ? null : h * 60 + m,
      duration_minutes: minutes,
      project_id: project || null,
      counts_toward_target: counts,
      active_from: from,
      active_until: until || null,
    };
    const ok = c
      ? await d.act(() => App.PatchCommitment(c.id, wire.PatchCommitmentRequest.createFrom({ ...fields, rev: c.rev })))
      : await d.act(() => App.CreateCommitment(wire.CreateCommitmentRequest.createFrom(fields)));
    setSaving(false);
    if (ok) onClose();
  }

  return (
    <Modal
      title={c ? "Edit commitment" : "New commitment"}
      size="lg"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} busy={saving} disabled={!title.trim() || !rule.trim() || !(minutes > 0 && minutes <= 1440) || !from}>
            {c ? "Save commitment" : "Create commitment"}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <Field label="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Internship" autoFocus />
        </Field>
        <Field label="Repeats" compound>
          <Recurrence value={rule} onChange={setRule} anchorDay={from} />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label="Starts at" compound hint={anyTime ? "Plans leave room for it, wherever it fits." : undefined}>
            <div className="flex items-center gap-3">
              <TimeInput value={at} disabled={anyTime} onCommit={setAt} className="w-24" aria-label="Starts at" />
              <span className="flex items-center gap-2 text-[13px] text-ink-muted">
                <Checkbox square checked={anyTime} onChange={setAnyTime} label="Any time" />
                Any time
              </span>
            </div>
          </Field>
          <Field label="Lasts" compound>
            <DurationInput value={duration} onChange={setDuration} label="Lasts" />
          </Field>
          <Field label="From">
            <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          </Field>
          <Field label="Until" hint="Empty for no end.">
            <Input type="date" value={until} min={from} onChange={(e) => setUntil(e.target.value)} />
          </Field>
          <Field label="Project">
            <Select value={project} onChange={(e) => setProject(e.target.value)}>
              <option value="">None</option>
              {d.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <ToggleRow title="Counts toward the daily target" description="Turn off for time that is not work, such as a class you only attend." checked={counts} onChange={setCounts} />
      </div>
    </Modal>
  );
}

type Smart = { specific: string; measurable: string; assignable: string; realistic: string };

const smartLetters: { key: keyof Smart | "time"; letter: string; name: string; ask: string; placeholder: string }[] = [
  { key: "specific", letter: "S", name: "Specific", ask: "What exactly will you achieve?", placeholder: "Solve the 150 NeetCode problems, every topic, in Python." },
  { key: "measurable", letter: "M", name: "Measurable", ask: "How will you measure progress?", placeholder: "Problems solved, and a timed mock interview each Sunday." },
  { key: "assignable", letter: "A", name: "Assignable", ask: "Who does it?", placeholder: "Me, with Sam reviewing my solutions on Fridays." },
  { key: "realistic", letter: "R", name: "Realistic", ask: "Why can you do it with the time you have?", placeholder: "Two problems a day fits in my 1h evening slot." },
  { key: "time", letter: "T", name: "Time-related", ask: "By when?", placeholder: "" },
];

/** The goal form's SMART check: each letter, filled in or not. */
function SmartFields({ value, onChange, measured, timed, reachable }: { value: Smart; onChange: (s: Smart) => void; measured: string | null; timed: string | null; reachable: boolean | null }) {
  const filled = smartLetters.filter((l) => (l.key === "time" ? !!timed : l.key === "measurable" ? !!value.measurable.trim() || !!measured : !!value[l.key].trim())).length;
  return (
    <section className="flex flex-col gap-3 rounded-lg border border-line bg-surface-2/50 p-4">
      <header className="flex items-center gap-3">
        <span className="text-[13px] font-semibold text-ink">Make it SMART</span>
        <SmartLetters lit={(k) => (k === "time" ? !!timed : k === "measurable" ? !!value.measurable.trim() || !!measured : !!value[k].trim())} />
        <span className="ml-auto text-xs text-ink-subtle tabular-nums">{filled} of 5</span>
      </header>
      <p className="-mt-1 text-xs leading-relaxed text-ink-subtle">Concrete goals are easier to prioritise. Fill in what you can; the assistant reads these when it plans with you.</p>
      {smartLetters
        .filter((l) => l.key !== "time")
        .map((l) => {
          const k = l.key as keyof Smart;
          const extra = k === "measurable" && measured ? `Counted: ${measured}.` : k === "realistic" && reachable != null ? (reachable ? "The time you set a day reaches the target." : "The time you set a day falls short of the target; see above.") : undefined;
          return (
            <Field key={k} label={`${l.letter} · ${l.ask}`} hint={extra}>
              <TextArea rows={1} value={value[k]} maxLength={1000} placeholder={l.placeholder} onChange={(e) => onChange({ ...value, [k]: e.target.value })} className="resize-y" />
            </Field>
          );
        })}
      <p className="text-xs text-ink-subtle">T · Time-related: {timed ?? "set the start and due days above."}</p>
    </section>
  );
}

function SmartLetters({ lit }: { lit: (k: keyof Smart | "time") => boolean }) {
  return (
    <span className="inline-flex gap-1" aria-label="SMART">
      {smartLetters.map((l) => (
        <span
          key={l.letter}
          title={`${l.name}${lit(l.key) ? "" : ": not set"}`}
          className={cx("grid size-5 place-items-center rounded text-[11px] font-bold", lit(l.key) ? "bg-accent/20 text-accent-hover" : "bg-surface-3 text-ink-faint")}
        >
          {l.letter}
        </span>
      ))}
    </span>
  );
}

/** A goal card's SMART letters, with the text on hover and on open. */
function SmartSummary({ g }: { g: wire.Goal }) {
  const [open, setOpen] = useState(false);
  const text: Record<string, string> = { specific: g.specific, measurable: g.measurable || (g.kind === "quantity" ? `${g.target_quantity} ${g.unit || "units"}` : ""), assignable: g.assignable, realistic: g.realistic };
  const lit = (k: keyof Smart | "time") => (k === "time" ? true : !!text[k]);
  const any = Object.values(text).some(Boolean);
  return (
    <div className="-mt-1">
      <button type="button" onClick={() => setOpen(!open)} className="flex items-center gap-2 text-xs text-ink-subtle hover:text-ink" disabled={!any}>
        <SmartLetters lit={lit} />
        {any ? (open ? "Hide" : "Why and how") : "Not SMART yet: edit the goal to add it"}
      </button>
      {open && (
        <dl className="mt-2 flex flex-col gap-1.5 rounded-lg bg-surface-2 p-3 text-xs">
          {smartLetters
            .filter((l) => l.key !== "time" && text[l.key])
            .map((l) => (
              <div key={l.letter} className="flex gap-2">
                <dt className="w-24 shrink-0 font-medium text-ink-muted">{l.name}</dt>
                <dd className="text-ink">{text[l.key]}</dd>
              </div>
            ))}
        </dl>
      )}
    </div>
  );
}
