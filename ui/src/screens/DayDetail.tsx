import { useCallback, useEffect, useState } from "react";
import { App, wire } from "../api";
import { Button, Card, Input, Label, Modal, Select } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration, formatTime, instantOn, shortId } from "../format";

const kinds = ["work", "break_manual", "break_auto"];

/** One day's segments, editable inline, with its target and note. */
export default function DayDetail({ day, onBack, readOnly }: { day: string; onBack: () => void; readOnly: boolean }) {
  const d = useDaemon();
  const [detail, setDetail] = useState<wire.DayDetail | null>(null);
  const [adding, setAdding] = useState(false);
  const [target, setTarget] = useState("");
  const [note, setNote] = useState("");
  const rollover = d.config?.tracking.day_rollover ?? "04:00";

  const load = useCallback(() => {
    App.GetDay(day).then((x) => {
      setDetail(x);
      setTarget(String(x.work_day.target_seconds / 3600));
      setNote(x.work_day.note);
    }, d.fail);
  }, [day, d.fail]);
  useEffect(load, [load, d.daysVersion]);

  if (!detail) return null;
  const projectName = (id: string | null) => (id ? d.projects.find((p) => p.id === id)?.name ?? shortId(id) : "Unassigned");

  const patch = (s: wire.Segment, fields: Record<string, unknown>) =>
    d.act(() => App.PatchSegment(s.id, wire.PatchSegmentRequest.createFrom({ ...fields, rev: s.rev }))).then(load);
  const time = (hhmm: string) => instantOn(day, hhmm, rollover);

  async function split(s: wire.Segment) {
    const at = window.prompt("Split at (HH:MM)", formatTime((s.started_at + (s.ended_at ?? s.started_at)) / 2));
    if (!at) return;
    await d.act(() => App.SplitSegment(s.id, wire.SplitSegmentRequest.createFrom({ at: time(at) })));
    load();
  }

  async function saveDay() {
    await d.act(() =>
      App.PatchDay(day, wire.PatchDayRequest.createFrom({ target_seconds: Math.round(Number(target) * 3600), note, rev: detail!.work_day.rev })),
    );
    load();
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <Button onClick={onBack}>← Back</Button>
        <h1 className="text-xl font-semibold">{day}</h1>
        <span className="text-sm text-zinc-500">
          {formatDuration(detail.summary.worked_ms)} worked · {formatDuration(detail.summary.break_ms)} break
        </span>
      </div>
      <Card title="Segments" actions={!readOnly && <Button onClick={() => setAdding(true)}>Add segment</Button>}>
        <table className="w-full text-sm">
          <thead className="text-left text-zinc-500">
            <tr>
              <th className="py-1">Kind</th>
              <th>Project</th>
              <th>Task</th>
              <th>Start</th>
              <th>End</th>
              <th>Length</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {detail.segments.map((s) => {
              const open = s.ended_at === null;
              const locked = readOnly || open;
              return (
                <tr key={s.id} className="border-t border-zinc-200 dark:border-zinc-800">
                  <td className="py-1.5">
                    <Select value={s.kind} disabled={locked} onChange={(e) => patch(s, { kind: e.target.value })}>
                      {kinds.map((k) => (
                        <option key={k}>{k}</option>
                      ))}
                    </Select>
                  </td>
                  <td>
                    {s.kind === "work" ? (
                      <Select value={s.project_id ?? ""} disabled={locked} onChange={(e) => patch(s, { project_id: e.target.value || null })}>
                        <option value="">Unassigned</option>
                        {d.projects.map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name}
                          </option>
                        ))}
                        {s.project_id && !d.projects.some((p) => p.id === s.project_id) && (
                          <option value={s.project_id}>{projectName(s.project_id)}</option>
                        )}
                      </Select>
                    ) : (
                      <span className="text-zinc-400">—</span>
                    )}
                  </td>
                  <td>
                    {s.kind === "work" ? (
                      <Select value={s.task_id ?? ""} disabled={locked} onChange={(e) => patch(s, { task_id: e.target.value || null })}>
                        <option value="">No task</option>
                        {d.tasks.map((t) => (
                          <option key={t.id} value={t.id}>
                            {t.title}
                          </option>
                        ))}
                        {s.task_id && !d.tasks.some((t) => t.id === s.task_id) && <option value={s.task_id}>{shortId(s.task_id)}</option>}
                      </Select>
                    ) : null}
                  </td>
                  <td>
                    <Input type="time" disabled={locked} defaultValue={formatTime(s.started_at)} onBlur={(e) => e.target.value !== formatTime(s.started_at) && patch(s, { started_at: time(e.target.value) })} />
                  </td>
                  <td>
                    {open ? (
                      <span className="text-emerald-600">now</span>
                    ) : (
                      <Input type="time" disabled={locked} defaultValue={formatTime(s.ended_at!)} onBlur={(e) => e.target.value !== formatTime(s.ended_at!) && patch(s, { ended_at: time(e.target.value) })} />
                    )}
                  </td>
                  <td className="tabular-nums">{formatDuration((s.ended_at ?? d.now()) - s.started_at)}</td>
                  <td className="text-right">
                    {!locked && (
                      <div className="flex justify-end gap-1">
                        <Button onClick={() => split(s)}>Split</Button>
                        <Button tone="danger" onClick={() => d.act(() => App.DeleteSegment(s.id)).then(load)}>
                          Delete
                        </Button>
                      </div>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </Card>
      <Card title="Day">
        <div className="flex flex-wrap items-end gap-3">
          <Label text="Target (hours)">
            <Input type="number" min="0" step="0.25" value={target} disabled={readOnly} onChange={(e) => setTarget(e.target.value)} />
          </Label>
          <Label text="Note">
            <Input className="w-80" value={note} disabled={readOnly} onChange={(e) => setNote(e.target.value)} />
          </Label>
          {!readOnly && (
            <Button tone="primary" onClick={saveDay}>
              Save
            </Button>
          )}
        </div>
      </Card>
      {adding && <AddSegment day={day} rollover={rollover} onClose={() => (setAdding(false), load())} />}
    </div>
  );
}

function AddSegment({ day, rollover, onClose }: { day: string; rollover: string; onClose: () => void }) {
  const d = useDaemon();
  const [kind, setKind] = useState("work");
  const [start, setStart] = useState("09:00");
  const [end, setEnd] = useState("10:00");
  const [project, setProject] = useState("");
  async function save() {
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
    if (ok) onClose();
  }
  return (
    <Modal title={`Add a segment to ${day}`} onClose={onClose}>
      <div className="grid grid-cols-2 gap-3">
        <Label text="Kind">
          <Select value={kind} onChange={(e) => setKind(e.target.value)}>
            {kinds.map((k) => (
              <option key={k}>{k}</option>
            ))}
          </Select>
        </Label>
        <Label text="Project">
          <Select value={project} disabled={kind !== "work"} onChange={(e) => setProject(e.target.value)}>
            <option value="">Unassigned</option>
            {d.projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </Select>
        </Label>
        <Label text="Start">
          <Input type="time" value={start} onChange={(e) => setStart(e.target.value)} />
        </Label>
        <Label text="End">
          <Input type="time" value={end} onChange={(e) => setEnd(e.target.value)} />
        </Label>
      </div>
      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button tone="primary" onClick={save}>
          Add
        </Button>
      </div>
    </Modal>
  );
}
