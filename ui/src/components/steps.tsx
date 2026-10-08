import { Pencil, Plus, Trash2 } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { App, readOnly, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatDuration, goDuration, parseDuration } from "../format";
import { useCompleteTask } from "./complete";
import DurationInput from "./DurationInput";
import { useConfirm } from "./feedback";
import { Button, Checkbox, Field, IconButton, Input, Modal, TextArea, cx } from "./ui";

/** "3 of 6 done", for a task's steps. */
export function stepsDone(steps: wire.Task[]): string {
  return `${steps.filter((s) => s.status === "done").length} of ${steps.length} done`;
}

/**
 * A task's steps, indented under it (docs/08-clients.md#gui): tick each one,
 * read what it says to do, edit or delete it, or add another.
 */
export function StepList({ parent, steps, adding = true, className }: { parent: wire.Task; steps: wire.Task[]; adding?: boolean; className?: string }) {
  const d = useDaemon();
  const [title, setTitle] = useState("");
  const [editing, setEditing] = useState<wire.Task | null>(null);
  // One step per Enter, however fast it is pressed.
  const inFlight = useRef(false);

  async function add(e: FormEvent) {
    e.preventDefault();
    const t = title.trim();
    if (!t || inFlight.current) return;
    inFlight.current = true;
    const created = await d.act(() =>
      App.CreateTask(
        wire.CreateTaskRequest.createFrom({
          title: t,
          parent_id: parent.id,
          project_id: parent.project_id ?? null,
        }),
      ),
    );
    inFlight.current = false;
    if (created) setTitle("");
  }

  return (
    <div className={cx("ml-[1.85rem] border-l border-line pl-2", className)}>
      {steps.length > 0 && (
        <ul className="flex flex-col">
          {steps.map((s) => (
            <StepRow key={s.id} step={s} onEdit={() => setEditing(s)} />
          ))}
        </ul>
      )}
      {adding && !readOnly && parent.status === "open" && (
        <form onSubmit={add} className="flex items-center gap-2 px-2 py-1">
          <Plus size={14} className="shrink-0 text-ink-faint" aria-hidden />
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Add step"
            aria-label={`Add a step to ${parent.title}`}
            className="min-w-0 flex-1 bg-transparent py-1 text-[13px] text-ink placeholder:text-ink-faint focus:outline-none"
          />
          {title.trim() && (
            <Button size="sm" type="submit">
              Add
            </Button>
          )}
        </form>
      )}
      {editing && <StepForm step={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

function StepRow({ step: s, onEdit }: { step: wire.Task; onEdit: () => void }) {
  const d = useDaemon();
  const confirm = useConfirm();
  const done = useCompleteTask();
  const [open, setOpen] = useState(false);
  const isDone = s.status === "done";

  function toggle(on: boolean) {
    if (on) done.complete(s);
    else done.reopen(s);
  }
  async function remove() {
    if (
      await confirm({
        title: `Delete ${s.title}?`,
        body: "Only this step goes.",
        confirm: "Delete step",
        danger: true,
      })
    )
      d.run(() => App.DeleteTask(s.id));
  }

  const long = s.notes.length > 140 || s.notes.includes("\n");
  return (
    <li className="group flex items-start gap-2.5 rounded-md px-2 py-1.5 hover:bg-surface-2/60">
      <span className="pt-px">
        <Checkbox checked={isDone} disabled={readOnly} onChange={toggle} label={isDone ? `Reopen ${s.title}` : `Complete ${s.title}`} />
      </span>
      <div className="min-w-0 flex-1">
        <div className={cx("text-[13px] leading-snug", isDone ? "text-ink-faint line-through" : "text-ink")}>{s.title}</div>
        {s.notes && (
          <button
            type="button"
            onClick={() => long && setOpen(!open)}
            className={cx("mt-0.5 block w-full text-left text-xs leading-relaxed whitespace-pre-line text-ink-subtle", !open && "line-clamp-2", long ? "cursor-pointer" : "cursor-text")}
            title={long ? (open ? "Show less" : "Show all") : undefined}
          >
            {s.notes}
          </button>
        )}
      </div>
      {s.estimate_minutes != null && <span className="pt-0.5 text-xs text-ink-faint tabular-nums">{formatDuration(s.estimate_minutes * 60_000)}</span>}
      {!readOnly && (
        <span className="flex gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
          <IconButton icon={Pencil} label={`Edit ${s.title}`} size="sm" onClick={onEdit} />
          <IconButton icon={Trash2} label={`Delete ${s.title}`} size="sm" onClick={remove} />
        </span>
      )}
    </li>
  );
}

function StepForm({ step, onClose }: { step: wire.Task; onClose: () => void }) {
  const d = useDaemon();
  const [title, setTitle] = useState(step.title);
  const [notes, setNotes] = useState(step.notes);
  const [estimate, setEstimate] = useState(goDuration((step.estimate_minutes ?? 0) * 60_000));
  const [saving, setSaving] = useState(false);

  async function save() {
    setSaving(true);
    const minutes = Math.round(parseDuration(estimate) / 60_000);
    const ok = await d.act(() =>
      App.PatchTask(
        step.id,
        wire.PatchTaskRequest.createFrom({
          title,
          notes,
          estimate_minutes: minutes > 0 ? minutes : null,
          rev: step.rev,
        }),
      ),
    );
    setSaving(false);
    if (ok) onClose();
  }

  return (
    <Modal
      title="Edit step"
      size="md"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" onClick={save} disabled={!title.trim()} busy={saving}>
            Save step
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-5">
        <Field label="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </Field>
        <Field label="What to do" hint="Where to find it, the idea to try, how you know it's done.">
          <TextArea rows={4} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
        <Field label="Estimate" compound>
          <DurationInput value={estimate} onChange={setEstimate} label="Estimate" />
        </Field>
      </div>
    </Modal>
  );
}
