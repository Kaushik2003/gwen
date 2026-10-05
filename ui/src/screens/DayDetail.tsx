import { ArrowLeft, Plus, Scissors, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { App, apiError, wire } from "../api";
import DurationInput from "../components/DurationInput";
import { useConfirm, useToast } from "../components/feedback";
import TimeInput from "../components/TimeInput";
import Timeline from "../components/Timeline";
import { Button, DotSelect, Field, IconButton, Modal, Panel, Select, TextArea, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration, formatLongDate, formatTime, goDuration, instantOn, parseDuration, shortId } from "../format";

const kinds = [
  { value: "work", label: "Work" },
  { value: "break_manual", label: "Break" },
  { value: "break_auto", label: "Auto break" },
];

/** One day's segments, editable inline, with its target and note. */
export default function DayDetail({ day, onBack, readOnly }: { day: string; onBack: () => void; readOnly: boolean }) {
  const d = useDaemon();
  const confirm = useConfirm();
  const notify = useToast();
  const [detail, setDetail] = useState<wire.DayDetail | null>(null);
  const [missing, setMissing] = useState(false);
  const [adding, setAdding] = useState(false);
  const [splitting, setSplitting] = useState<wire.Segment | null>(null);
  const [target, setTarget] = useState("8h");
  const [note, setNote] = useState("");
  const [focus, setFocus] = useState<string | null>(null);
  // Every task, done ones too, so past time shows what it was spent on.
  const [allTasks, setAllTasks] = useState<wire.Task[]>([]);
  const rollover = d.config?.tracking.day_rollover ?? "04:00";

  const load = useCallback(() => {
    App.GetDay(day).then(
      (x) => {
        setDetail(x);
        setMissing(false);
        setTarget(goDuration(x.work_day.target_seconds * 1000));
        setNote(x.work_day.note);
      },
      (e) => {
        if (apiError(e).code === "not_found") setMissing(true);
        else d.fail(e);
      },
    );
  }, [day, d.fail]);
  useEffect(load, [load, d.daysVersion]);
  useEffect(() => {
    App.ListTasks(wire.TaskQuery.createFrom({ project_id: "", status: "all", due_before: "" })).then(
      (l) => setAllTasks(l.tasks),
      () => setAllTasks([]),
    );
  }, [d.tasksVersion]);

  const colors = useMemo(() => {
    const m = new Map<string | null, string>([[null, unassignedColor]]);
    d.projects.forEach((p) => m.set(p.id, p.color));
    detail?.summary.by_project.forEach((p) => m.set(p.project_id ?? null, p.color));
    return m;
  }, [d.projects, detail]);
  const names = useMemo(() => {
    const m = new Map<string | null, string>([[null, "Unassigned"]]);
    d.projects.forEach((p) => m.set(p.id, p.name));
    detail?.summary.by_project.forEach((p) => m.set(p.project_id ?? null, p.name));
    return m;
  }, [d.projects, detail]);

  const header = (
    <div className="mb-6 flex flex-wrap items-center gap-x-4 gap-y-3">
      <Button icon={ArrowLeft} tone="ghost" onClick={onBack} className="-ml-2">
        History
      </Button>
      <h1 className="text-headline font-semibold text-ink">{formatLongDate(day)}</h1>
      {!readOnly && (
        <Button icon={Plus} className="ml-auto" onClick={() => setAdding(true)}>
          Add time
        </Button>
      )}
    </div>
  );

  if (missing && !detail)
    return (
      <div>
        {header}
        <p className="text-sm text-ink-subtle">Nothing was tracked on this day.{!readOnly && " Add time to record work you did away from the computer."}</p>
        {adding && <AddSegment day={day} rollover={rollover} onClose={() => (setAdding(false), load())} />}
      </div>
    );
  if (!detail) return header;

  const patch = (s: wire.Segment, fields: Record<string, unknown>) =>
    d.act(() => App.PatchSegment(s.id, wire.PatchSegmentRequest.createFrom({ ...fields, rev: s.rev }))).then(load);
  const time = (hhmm: string) => instantOn(day, hhmm, rollover);
  const live = detail.segments.some((s) => s.ended_at == null);

  async function remove(s: wire.Segment) {
    const ok = await confirm({
      title: "Delete this time?",
      body: `${kinds.find((k) => k.value === s.kind)?.label ?? s.kind} from ${formatTime(s.started_at)} to ${formatTime(s.ended_at!)} is removed from the day.`,
      confirm: "Delete",
      danger: true,
    });
    if (ok && (await d.run(() => App.DeleteSegment(s.id)))) load();
  }

  async function saveDay() {
    const ok = await d.act(() =>
      App.PatchDay(day, wire.PatchDayRequest.createFrom({ target_seconds: Math.round(parseDuration(target) / 1000), note, rev: detail!.work_day.rev })),
    );
    if (ok) notify("Saved the day's target and note", "success");
    load();
  }

  const s = detail.summary;
  const dayTarget = detail.work_day.target_seconds * 1000;
  return (
    <div className="flex flex-col gap-5">
      {header}
      <section className="lift rounded-2xl border border-line bg-surface-1">
        <dl className="grid grid-cols-2 gap-4 px-6 pt-5 sm:grid-cols-4">
          <Figure label="Worked" value={formatDuration(s.worked_ms)} />
          <Figure label="Break" value={formatDuration(s.break_ms)} />
          <Figure label="Target" value={formatDuration(dayTarget)} />
          <Figure label={s.target_met ? "Target met" : "Short of target"} value={`${dayTarget > 0 ? Math.round((s.worked_ms / dayTarget) * 100) : 0}%`} tone={s.target_met ? "text-working" : undefined} />
        </dl>
        <div className="mt-5 border-t border-line px-6 py-4">
          <Timeline
            segments={detail.segments}
            colors={colors}
            names={names}
            now={d.now()}
            live={live}
            from={instantOn(day, d.config?.planner.day_start ?? "09:00", rollover)}
            to={instantOn(day, d.config?.planner.day_end ?? "23:00", rollover)}
            onSegment={(seg) => {
              setFocus(seg.id);
              document.getElementById(`seg-${seg.id}`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
            }}
          />
        </div>
      </section>

      <Panel title="Time" bodyClassName="px-3 pt-2 pb-3">
        <ul className="flex flex-col gap-1">
          {detail.segments.map((seg) => {
            const open = seg.ended_at == null;
            const locked = readOnly || open;
            const work = seg.kind === "work";
            const color = work ? colors.get(seg.project_id ?? null) ?? unassignedColor : "var(--color-break)";
            return (
              <li
                id={`seg-${seg.id}`}
                key={seg.id}
                className={cx("flex flex-wrap items-center gap-2 rounded-lg py-2 pr-2 pl-3 transition-colors md:flex-nowrap", focus === seg.id ? "bg-accent/10" : "hover:bg-surface-2/60")}
                onFocus={() => setFocus(seg.id)}
              >
                <span className={cx("h-8 w-1 shrink-0 rounded-full", !work && "hatched")} style={work ? { backgroundColor: color } : undefined} aria-hidden />
                <Select value={seg.kind} disabled={locked} onChange={(e) => patch(seg, { kind: e.target.value })} className="w-32 shrink-0" aria-label="Kind">
                  {kinds.map((k) => (
                    <option key={k.value} value={k.value}>
                      {k.label}
                    </option>
                  ))}
                </Select>
                {work ? (
                  <>
                    <DotSelect color={color} value={seg.project_id ?? ""} disabled={locked} onChange={(e) => patch(seg, { project_id: e.target.value || null })} className="w-40 shrink-0" aria-label="Project">
                      <option value="">Unassigned</option>
                      {d.projects.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                      {seg.project_id && !d.projects.some((p) => p.id === seg.project_id) && <option value={seg.project_id}>{names.get(seg.project_id) ?? shortId(seg.project_id)}</option>}
                    </DotSelect>
                    <Select value={seg.task_id ?? ""} disabled={locked} onChange={(e) => patch(seg, { task_id: e.target.value || null })} className="w-40 min-w-0 flex-1" aria-label="Task">
                      <option value="">No task</option>
                      {d.tasks.map((t) => (
                        <option key={t.id} value={t.id}>
                          {t.title}
                        </option>
                      ))}
                      {seg.task_id && !d.tasks.some((t) => t.id === seg.task_id) && (
                        <option value={seg.task_id}>{allTasks.find((t) => t.id === seg.task_id)?.title ?? shortId(seg.task_id)}</option>
                      )}
                    </Select>
                  </>
                ) : (
                  <span className="min-w-0 flex-1" />
                )}
                <span className="flex shrink-0 items-center gap-1.5">
                  <TimeInput disabled={locked} aria-label="Start" className="w-20" value={formatTime(seg.started_at)} onCommit={(v) => patch(seg, { started_at: time(v) })} />
                  <span className="text-ink-faint">–</span>
                  {open ? (
                    <span className="w-20 text-center text-sm font-medium text-working">now</span>
                  ) : (
                    <TimeInput disabled={locked} aria-label="End" className="w-20" value={formatTime(seg.ended_at!)} onCommit={(v) => patch(seg, { ended_at: time(v) })} />
                  )}
                </span>
                <span className="w-16 shrink-0 text-right text-sm text-ink-muted tabular-nums">{formatDuration((seg.ended_at ?? d.now()) - seg.started_at)}</span>
                {!readOnly && (
                  <span className="flex shrink-0 gap-0.5">
                    <IconButton icon={Scissors} label="Split in two" size="sm" disabled={locked} onClick={() => setSplitting(seg)} />
                    <IconButton icon={Trash2} label="Delete" size="sm" disabled={locked} onClick={() => remove(seg)} />
                  </span>
                )}
              </li>
            );
          })}
        </ul>
        {live && !readOnly && <p className="mt-2 px-3 text-xs text-ink-subtle">The running segment can be changed once it ends.</p>}
      </Panel>

      <Panel title="Day">
        <div className="grid gap-4 sm:grid-cols-[auto_1fr]">
          <Field label="Target" compound>
            <DurationInput value={target} onChange={setTarget} label="Target" disabled={readOnly} />
          </Field>
          <Field label="Note">
            <TextArea rows={2} value={note} disabled={readOnly} onChange={(e) => setNote(e.target.value)} placeholder="Anything worth remembering about this day" />
          </Field>
        </div>
        {!readOnly && (
          <div className="mt-4 flex justify-end">
            <Button tone="primary" onClick={saveDay}>
              Save day
            </Button>
          </div>
        )}
      </Panel>
      {adding && <AddSegment day={day} rollover={rollover} onClose={() => (setAdding(false), load())} />}
      {splitting && <SplitSegment segment={splitting} day={day} rollover={rollover} onClose={() => (setSplitting(null), load())} />}
    </div>
  );
}

function Figure({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div>
      <dt className="text-xs text-ink-subtle">{label}</dt>
      <dd className={cx("mt-0.5 text-title font-semibold tabular-nums", tone ?? "text-ink")}>{value}</dd>
    </div>
  );
}

function SplitSegment({ segment: s, day, rollover, onClose }: { segment: wire.Segment; day: string; rollover: string; onClose: () => void }) {
  const d = useDaemon();
  const [at, setAt] = useState(formatTime((s.started_at + (s.ended_at ?? s.started_at)) / 2));
  async function split() {
    if (await d.act(() => App.SplitSegment(s.id, wire.SplitSegmentRequest.createFrom({ at: instantOn(day, at, rollover) })))) onClose();
  }
  return (
    <Modal
      title="Split in two"
      description={`${formatTime(s.started_at)} to ${formatTime(s.ended_at!)} becomes two pieces you can label separately.`}
      size="sm"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" icon={Scissors} onClick={split}>
            Split
          </Button>
        </>
      }
    >
      <Field label="Split at">
        <TimeInput value={at} onCommit={setAt} className="w-24" autoFocus />
      </Field>
    </Modal>
  );
}

function AddSegment({ day, rollover, onClose }: { day: string; rollover: string; onClose: () => void }) {
  const d = useDaemon();
  const [kind, setKind] = useState("work");
  const [start, setStart] = useState("09:00");
  const [end, setEnd] = useState("10:00");
  const [project, setProject] = useState("");
  const [saving, setSaving] = useState(false);
  const length = instantOn(day, end, rollover) - instantOn(day, start, rollover);
  async function save() {
    setSaving(true);
    const ok = await d.act(() =>
      App.CreateSegment(
        wire.CreateSegmentRequest.createFrom({
          day,
          kind,
          project_id: kind === "work" && project ? project : null,
          task_id: null,
          started_at: instantOn(day, start, rollover),
          ended_at: instantOn(day, end, rollover),
        }),
      ),
    );
    setSaving(false);
    if (ok) onClose();
  }
  return (
    <Modal
      title="Add time"
      description={`Record time on ${formatLongDate(day)} that Gwen did not see, such as work away from the computer.`}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} busy={saving} disabled={length <= 0}>
            Add {length > 0 ? formatDuration(length) : "time"}
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-4">
        <Field label="Kind">
          <Select value={kind} onChange={(e) => setKind(e.target.value)}>
            {kinds.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Project">
          <Select value={project} disabled={kind !== "work"} onChange={(e) => setProject(e.target.value)}>
            <option value="">Unassigned</option>
            {d.projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="From">
          <TimeInput value={start} onCommit={setStart} />
        </Field>
        <Field label="To" error={length <= 0 ? "The end is before the start." : undefined}>
          <TimeInput value={end} onCommit={setEnd} />
        </Field>
      </div>
    </Modal>
  );
}
