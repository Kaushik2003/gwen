import { useEffect, useState } from "react";
import { App, readOnly, wire } from "../api";
import Breakdown from "../components/Breakdown";
import { Button, Card, Input, Label, Modal, Select } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration } from "../format";
import { clockOf } from "../rrule";

const paceStyle: Record<string, string> = {
  ahead: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300",
  on_track: "bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300",
  behind: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300",
};

export function PaceChip({ pace }: { pace: string }) {
  return <span className={`rounded px-1.5 py-0.5 text-xs ${paceStyle[pace] ?? paceStyle.on_track}`}>{pace.replace("_", " ")}</span>;
}

/** "12 / 300 problems" for a quantity goal, "3 / 8 tasks" for a tasks goal. */
export function goalAmount(g: wire.Goal): string {
  const p = g.progress;
  const unit = g.kind === "tasks" ? "tasks" : g.unit;
  return `${p.done_quantity} / ${p.done_quantity + p.remaining_quantity}${unit ? " " + unit : ""}`;
}

export function GoalBar({ g }: { g: wire.Goal }) {
  const total = g.progress.done_quantity + g.progress.remaining_quantity;
  const frac = total > 0 ? g.progress.done_quantity / total : 0;
  return (
    <div className="h-2 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800">
      <div className="h-full rounded-full bg-emerald-500" style={{ width: `${Math.min(100, frac * 100)}%` }} />
    </div>
  );
}

export default function Goals() {
  const d = useDaemon();
  const [status, setStatus] = useState("active");
  const [goals, setGoals] = useState<wire.Goal[]>([]);
  const [editing, setEditing] = useState<wire.Goal | "new" | null>(null);
  const [breakingDown, setBreakingDown] = useState<wire.Goal | null>(null);

  useEffect(() => {
    App.ListGoals(status).then((l) => setGoals(l.goals), d.fail);
  }, [status, d.goalsVersion, d.tasksVersion, d.fail]);

  return (
    <div className="flex flex-col gap-4">
      <Card
        title="Goals"
        actions={
          <>
            <Select value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="active">Active</option>
              <option value="done">Done</option>
              <option value="abandoned">Abandoned</option>
              <option value="all">All</option>
            </Select>
            {!readOnly && (
              <Button tone="primary" onClick={() => setEditing("new")}>
                New goal
              </Button>
            )}
          </>
        }
      >
        {goals.length === 0 && <p className="text-sm text-zinc-500">No goals here yet.</p>}
        <ul className="flex flex-col divide-y divide-zinc-200 dark:divide-zinc-800">
          {goals.map((g) => (
            <li key={g.id} className="flex flex-col gap-2 py-3">
              <div className="flex items-center gap-3">
                <span className="flex-1 font-medium">{g.title}</span>
                {g.status !== "active" && <span className="text-xs text-zinc-500">{g.status}</span>}
                <PaceChip pace={g.progress.pace} />
                {!readOnly && (
                  <>
                    <Button onClick={() => setBreakingDown(g)}>Break down</Button>
                    <Button onClick={() => setEditing(g)}>Edit</Button>
                    <Button
                      tone="danger"
                      onClick={() => window.confirm(`Delete the goal ${g.title}?`) && d.act(() => App.DeleteGoal(g.id))}
                    >
                      ✕
                    </Button>
                  </>
                )}
              </div>
              <GoalBar g={g} />
              <div className="flex flex-wrap gap-x-4 text-xs text-zinc-500">
                <span className="tabular-nums">{goalAmount(g)}</span>
                <span>
                  needs {g.progress.required_per_day.toFixed(1)} a day · doing {g.progress.actual_per_day.toFixed(1)}
                </span>
                <span>due {g.due_day}</span>
                <span>{g.progress.projected_finish_day ? `on pace to finish ${g.progress.projected_finish_day}` : "no finish in sight yet"}</span>
              </div>
            </li>
          ))}
        </ul>
      </Card>
      <Commitments />
      {editing && <GoalForm goal={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {breakingDown && <Breakdown goal={breakingDown} onClose={() => setBreakingDown(null)} />}
    </div>
  );
}

function GoalForm({ goal, onClose }: { goal: wire.Goal | null; onClose: () => void }) {
  const d = useDaemon();
  const [title, setTitle] = useState(goal?.title ?? "");
  const [kind, setKind] = useState(goal?.kind ?? "quantity");
  const [unit, setUnit] = useState(goal?.unit ?? "");
  const [target, setTarget] = useState(goal?.target_quantity ? String(goal.target_quantity) : "");
  const [perUnit, setPerUnit] = useState(goal?.minutes_per_unit ? String(goal.minutes_per_unit) : "");
  const [project, setProject] = useState(goal?.project_id ?? "");
  const [start, setStart] = useState(goal?.start_day ?? d.today());
  const [due, setDue] = useState(goal?.due_day ?? "");
  const [status, setStatus] = useState(goal?.status ?? "active");
  const [template, setTemplate] = useState<wire.Task | null>(null);
  const [rule, setRule] = useState("");

  useEffect(() => {
    if (!goal || goal.kind !== "quantity") return;
    App.ListTasks(wire.TaskQuery.createFrom({ goal_id: goal.id, templates: true, status: "all" })).then((l) => {
      const t = l.tasks.find((x) => x.rrule);
      setTemplate(t ?? null);
      setRule(t?.rrule ?? "");
    }, d.fail);
  }, [goal, d.fail]);

  const quantity = kind === "quantity";
  async function save() {
    const fields = {
      title,
      unit,
      target_quantity: quantity ? Number(target) : null,
      minutes_per_unit: quantity ? Number(perUnit) : null,
      project_id: project || null,
      start_day: start,
      due_day: due,
    };
    const ok = goal
      ? await d.act(() => App.PatchGoal(goal.id, wire.PatchGoalRequest.createFrom({ ...fields, status, rev: goal.rev })))
      : await d.act(() => App.CreateGoal(wire.CreateGoalRequest.createFrom({ ...fields, kind })));
    if (!ok) return;
    if (template && rule !== template.rrule) {
      const saved = await d.act(() => App.PatchTask(template.id, wire.PatchTaskRequest.createFrom({ rrule: rule, rev: template.rev })));
      if (!saved) return;
    }
    onClose();
  }

  const valid = title.trim() && due && start && (!quantity || (Number(target) > 0 && Number(perUnit) > 0));
  return (
    <Modal title={goal ? "Edit goal" : "New goal"} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <Label text="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </Label>
        <Label text="Kind">
          <Select value={kind} onChange={(e) => setKind(e.target.value)} disabled={!!goal}>
            <option value="quantity">A quantity, such as 300 problems</option>
            <option value="tasks">A set of tasks</option>
          </Select>
        </Label>
        {quantity && (
          <div className="grid grid-cols-3 gap-3">
            <Label text="Target">
              <Input type="number" min="1" value={target} onChange={(e) => setTarget(e.target.value)} />
            </Label>
            <Label text="Unit">
              <Input value={unit} placeholder="problems" onChange={(e) => setUnit(e.target.value)} />
            </Label>
            <Label text="Minutes each">
              <Input type="number" min="1" value={perUnit} onChange={(e) => setPerUnit(e.target.value)} />
            </Label>
          </div>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Label text="Start">
            <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
          </Label>
          <Label text="Due">
            <Input type="date" value={due} onChange={(e) => setDue(e.target.value)} />
          </Label>
          <Label text="Project">
            <Select value={project} onChange={(e) => setProject(e.target.value)}>
              <option value="">None</option>
              {d.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </Label>
          {goal && (
            <Label text="Status">
              <Select value={status} onChange={(e) => setStatus(e.target.value)}>
                <option value="active">Active</option>
                <option value="done">Done</option>
                <option value="abandoned">Abandoned</option>
              </Select>
            </Label>
          )}
        </div>
        {template && (
          <Label text="Sessions repeat">
            <Input value={rule} onChange={(e) => setRule(e.target.value)} placeholder="FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" />
          </Label>
        )}
      </div>
      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button tone="primary" onClick={save} disabled={!valid}>
          Save
        </Button>
      </div>
    </Modal>
  );
}

function Commitments() {
  const d = useDaemon();
  const [list, setList] = useState<wire.Commitment[]>([]);
  const [editing, setEditing] = useState<wire.Commitment | "new" | null>(null);
  useEffect(() => {
    App.ListCommitments().then((l) => setList(l.commitments), d.fail);
  }, [d.goalsVersion, d.fail]);
  const projectName = (id?: string | null) => (id ? d.projects.find((p) => p.id === id)?.name ?? "" : "");

  return (
    <Card
      title="Commitments"
      actions={
        !readOnly && (
          <Button tone="primary" onClick={() => setEditing("new")}>
            New commitment
          </Button>
        )
      }
    >
      <p className="mb-3 text-xs text-zinc-500">Recurring time that is not planned work, such as an internship or a class.</p>
      {list.length === 0 && <p className="text-sm text-zinc-500">No commitments.</p>}
      <ul className="divide-y divide-zinc-200 dark:divide-zinc-800">
        {list.map((c) => (
          <li key={c.id} className="flex items-center gap-3 py-2 text-sm">
            <span className="w-28 tabular-nums text-zinc-500">
              {c.start_minute == null ? "any time" : `${clockOf(c.start_minute)}–${clockOf(c.start_minute + c.duration_minutes)}`}
            </span>
            <span className="flex-1">{c.title}</span>
            <span className="text-xs text-zinc-500">{projectName(c.project_id)}</span>
            <span className="text-xs">{formatDuration(c.duration_minutes * 60_000)}</span>
            <span className="font-mono text-xs text-zinc-500">{c.rrule}</span>
            {!c.counts_toward_target && <span className="text-xs text-zinc-500">not counted</span>}
            {!readOnly && (
              <>
                <Button onClick={() => setEditing(c)}>Edit</Button>
                <Button tone="danger" onClick={() => d.act(() => App.DeleteCommitment(c.id))}>
                  ✕
                </Button>
              </>
            )}
          </li>
        ))}
      </ul>
      {editing && <CommitmentForm commitment={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
    </Card>
  );
}

function CommitmentForm({ commitment: c, onClose }: { commitment: wire.Commitment | null; onClose: () => void }) {
  const d = useDaemon();
  const [title, setTitle] = useState(c?.title ?? "");
  const [rule, setRule] = useState(c?.rrule ?? "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR");
  const [at, setAt] = useState(c?.start_minute != null ? clockOf(c.start_minute) : "");
  const [duration, setDuration] = useState(c ? String(c.duration_minutes) : "60");
  const [project, setProject] = useState(c?.project_id ?? "");
  const [counts, setCounts] = useState(c?.counts_toward_target ?? true);
  const [from, setFrom] = useState(c?.active_from ?? d.today());
  const [until, setUntil] = useState(c?.active_until ?? "");

  async function save() {
    const [h, m] = at.split(":").map(Number);
    const fields = {
      title,
      rrule: rule,
      start_minute: at ? h * 60 + m : null,
      duration_minutes: Number(duration),
      project_id: project || null,
      counts_toward_target: counts,
      active_from: from,
      active_until: until || null,
    };
    const ok = c
      ? await d.act(() => App.PatchCommitment(c.id, wire.PatchCommitmentRequest.createFrom({ ...fields, rev: c.rev })))
      : await d.act(() => App.CreateCommitment(wire.CreateCommitmentRequest.createFrom(fields)));
    if (ok) onClose();
  }

  return (
    <Modal title={c ? "Edit commitment" : "New commitment"} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <Label text="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </Label>
        <Label text="Repeats">
          <Input value={rule} onChange={(e) => setRule(e.target.value)} />
        </Label>
        <div className="grid grid-cols-2 gap-3">
          <Label text="Starts at (empty for any time)">
            <Input type="time" value={at} onChange={(e) => setAt(e.target.value)} />
          </Label>
          <Label text="Minutes">
            <Input type="number" min="1" max="1440" value={duration} onChange={(e) => setDuration(e.target.value)} />
          </Label>
          <Label text="From">
            <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          </Label>
          <Label text="Until">
            <Input type="date" value={until} onChange={(e) => setUntil(e.target.value)} />
          </Label>
          <Label text="Project">
            <Select value={project} onChange={(e) => setProject(e.target.value)}>
              <option value="">None</option>
              {d.projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </Label>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={counts} onChange={(e) => setCounts(e.target.checked)} />
          Counts toward the daily target
        </label>
      </div>
      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button tone="primary" onClick={save} disabled={!title.trim() || !rule.trim() || !(Number(duration) > 0) || !from}>
          Save
        </Button>
      </div>
    </Modal>
  );
}
