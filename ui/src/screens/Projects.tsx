import { useEffect, useState } from "react";
import { App, wire } from "../api";
import { Button, Card, Input, Label, Modal, Select } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDuration } from "../format";

export default function Projects() {
  const d = useDaemon();
  const [projects, setProjects] = useState<wire.Project[]>([]);
  const [showArchived, setShowArchived] = useState(false);
  const [newName, setNewName] = useState("");
  const [filterProject, setFilterProject] = useState("");
  const [status, setStatus] = useState("open");
  const [tasks, setTasks] = useState<wire.Task[]>([]);
  const [editing, setEditing] = useState<wire.Task | "new" | null>(null);

  useEffect(() => {
    App.ListProjects(showArchived ? "all" : "false").then((l) => setProjects(l.projects), d.fail);
  }, [showArchived, d.projectsVersion, d.fail]);
  useEffect(() => {
    App.ListTasks(wire.TaskQuery.createFrom({ project_id: filterProject, status, due_before: "" })).then((l) => setTasks(l.tasks), d.fail);
  }, [filterProject, status, d.tasksVersion, d.fail]);

  const patchProject = (p: wire.Project, fields: Record<string, unknown>) =>
    d.act(() => App.PatchProject(p.id, wire.PatchProjectRequest.createFrom({ ...fields, rev: p.rev })));
  const projectName = (id: string | null) => (id ? projects.find((p) => p.id === id)?.name ?? "" : "");

  return (
    <div className="grid grid-cols-[22rem_1fr] gap-4">
      <Card title="Projects" actions={<label className="flex items-center gap-1 text-xs"><input type="checkbox" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} />archived</label>}>
        <form
          className="mb-3 flex gap-2"
          onSubmit={async (e) => {
            e.preventDefault();
            if (await d.act(() => App.CreateProject(wire.CreateProjectRequest.createFrom({ name: newName })))) setNewName("");
          }}
        >
          <Input className="flex-1" placeholder="New project" value={newName} onChange={(e) => setNewName(e.target.value)} />
          <Button tone="primary" type="submit" disabled={!newName.trim()}>
            Add
          </Button>
        </form>
        <ul className="flex flex-col gap-2">
          {projects.map((p) => (
            <li key={p.id} className="flex items-center gap-2">
              <input type="color" value={p.color} onChange={(e) => patchProject(p, { color: e.target.value })} className="h-6 w-6 cursor-pointer rounded border-0 bg-transparent p-0" />
              <Input
                className={`flex-1 ${p.archived_at ? "text-zinc-400" : ""}`}
                defaultValue={p.name}
                onBlur={(e) => e.target.value.trim() && e.target.value !== p.name && patchProject(p, { name: e.target.value })}
              />
              <Button onClick={() => patchProject(p, { archived: !p.archived_at })}>{p.archived_at ? "Unarchive" : "Archive"}</Button>
              <Button
                tone="danger"
                onClick={() => window.confirm(`Delete ${p.name} and its tasks?`) && d.act(() => App.DeleteProject(p.id))}
              >
                ✕
              </Button>
            </li>
          ))}
        </ul>
      </Card>
      <Card
        title="Tasks"
        actions={
          <>
            <Select value={filterProject} onChange={(e) => setFilterProject(e.target.value)}>
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
            <Select value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="open">Open</option>
              <option value="done">Done</option>
              <option value="all">All</option>
            </Select>
            <Button tone="primary" onClick={() => setEditing("new")}>
              New task
            </Button>
          </>
        }
      >
        {tasks.length === 0 && <p className="text-sm text-zinc-500">No tasks.</p>}
        <ul className="divide-y divide-zinc-200 dark:divide-zinc-800">
          {tasks.map((t) => (
            <li key={t.id} className="flex items-center gap-3 py-2 text-sm">
              <input
                type="checkbox"
                checked={t.status === "done"}
                onChange={() =>
                  d.act(() => (t.status === "done" ? App.ReopenTask(t.id) : App.CompleteTask(t.id, wire.CompleteTaskRequest.createFrom({}))))
                }
              />
              <span className={`flex-1 ${t.status === "done" ? "text-zinc-400 line-through" : ""}`}>{t.title}</span>
              <span className="text-xs text-zinc-500">{projectName(t.project_id ?? null)}</span>
              <span className="text-xs">P{t.priority}</span>
              {t.due_day && <span className="text-xs text-zinc-500">due {t.due_day}</span>}
              <span className="w-14 text-right text-xs tabular-nums">{formatDuration(t.tracked_ms)}</span>
              <Button onClick={() => setEditing(t)}>Edit</Button>
              <Button tone="danger" onClick={() => d.act(() => App.DeleteTask(t.id))}>
                ✕
              </Button>
            </li>
          ))}
        </ul>
      </Card>
      {editing && <TaskForm task={editing === "new" ? null : editing} projects={projects} onClose={() => setEditing(null)} />}
    </div>
  );
}

function TaskForm({ task, projects, onClose }: { task: wire.Task | null; projects: wire.Project[]; onClose: () => void }) {
  const d = useDaemon();
  const [title, setTitle] = useState(task?.title ?? "");
  const [project, setProject] = useState(task?.project_id ?? "");
  const [priority, setPriority] = useState(task?.priority ?? 2);
  const [due, setDue] = useState(task?.due_day ?? "");
  const [estimate, setEstimate] = useState(task?.estimate_minutes ? String(task.estimate_minutes) : "");
  const [notes, setNotes] = useState(task?.notes ?? "");

  async function save() {
    const fields = {
      title,
      project_id: project || null,
      priority,
      due_day: due || null,
      estimate_minutes: estimate ? Number(estimate) : null,
      notes,
    };
    const ok = task
      ? await d.act(() => App.PatchTask(task.id, wire.PatchTaskRequest.createFrom({ ...fields, rev: task.rev })))
      : await d.act(() => App.CreateTask(wire.CreateTaskRequest.createFrom(fields)));
    if (ok) onClose();
  }

  return (
    <Modal title={task ? "Edit task" : "New task"} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <Label text="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </Label>
        <div className="grid grid-cols-2 gap-3">
          <Label text="Project">
            <Select value={project} onChange={(e) => setProject(e.target.value)}>
              <option value="">None</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </Label>
          <Label text="Priority">
            <Select value={priority} onChange={(e) => setPriority(Number(e.target.value))}>
              <option value={1}>1 — low</option>
              <option value={2}>2</option>
              <option value={3}>3</option>
              <option value={4}>4 — high</option>
            </Select>
          </Label>
          <Label text="Due">
            <Input type="date" value={due} onChange={(e) => setDue(e.target.value)} />
          </Label>
          <Label text="Estimate (minutes)">
            <Input type="number" min="1" value={estimate} onChange={(e) => setEstimate(e.target.value)} />
          </Label>
        </div>
        <Label text="Notes">
          <Input value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Label>
      </div>
      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button tone="primary" onClick={save} disabled={!title.trim()}>
          Save
        </Button>
      </div>
    </Modal>
  );
}
