import { Bot, Play, ShieldCheck } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import winking from "../assets/gwen/winking.png";
import { ApiError, api, type Task } from "./api";
import { useDay } from "./day";
import { ProjectSheet } from "./ProjectSheet";
import { useStore } from "./store";
import { Sheet } from "./ui";

// Add a task on the phone (the phone redo, P6): the window's task form, shortened to what you decide on the go (what,
// which project, how long, when), and Gwen fits it into the day. "Add, hand to Claude" also starts a Claude session on it
// in the project's folder, as the window's Hand it to an agent does (api/phone.go phoneHandOff). Given a task, the sheet
// is the hand-off alone.

const lengths = [
  { label: "15m", minutes: 15 },
  { label: "30m", minutes: 30 },
  { label: "1h", minutes: 60 },
  { label: "2h", minutes: 120 },
  { label: "Not sure", minutes: 0 },
];

/** YYYY-MM-DD of a day `by` days after day. */
function addDays(day: string, by: number): string {
  const d = new Date(`${day}T12:00:00`);
  d.setDate(d.getDate() + by);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** Today as the phone's own calendar says it, when the laptop hasn't said. */
function localDay(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function dayWords(day: string): string {
  return new Date(`${day}T12:00:00`).toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "short" });
}

export function TaskSheet({ task, onClose }: { task?: Task; onClose: () => void }) {
  const { go, upsert, toast } = useStore();
  const day = useDay();
  const today = day.plan?.day || day.status?.work_day?.day || localDay();
  const handOnly = !!task;
  const [title, setTitle] = useState("");
  const [projectId, setProjectId] = useState<string>(task?.project_id ?? "");
  // A project being made, or a folder being picked for the chosen one, over the sheet.
  const [making, setMaking] = useState<"" | "new" | "folder">("");
  const [minutes, setMinutes] = useState(30);
  const [when, setWhen] = useState<"today" | "tomorrow" | "pick">("today");
  const [picked, setPicked] = useState(addDays(today, 2));
  const [brief, setBrief] = useState(task ? [task.title, task.notes].filter(Boolean).join("\n\n") : "");
  // The brief follows the title until you write in it yourself.
  const ownBrief = useRef(handOnly);
  const [sandbox, setSandbox] = useState(false);
  const [busy, setBusy] = useState<"" | "add" | "hand">("");
  const titleBox = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!handOnly) titleBox.current?.focus();
  }, [handOnly]);

  const project = day.project(projectId);
  // Projects with a folder first: those are the ones Claude can work in.
  const projects = [...day.projects].sort((a, b) => Number(!a.folder) - Number(!b.folder));
  const startDay = when === "today" ? today : when === "tomorrow" ? addDays(today, 1) : picked;
  const noFolder = !!project && !project.folder;
  const canHand = !!project && !noFolder && !!brief.trim() && (handOnly || !!title.trim());
  const name = (handOnly ? task!.title : title).trim();

  const fits =
    minutes > 0
      ? when === "today"
        ? "I'll fit it into today."
        : when === "tomorrow"
          ? "I'll fit it into tomorrow."
          : `I'll fit it in on ${dayWords(picked)}.`
      : when === "today"
        ? "It goes on today's list. Say how long and I'll find it a time."
        : `It waits until ${when === "tomorrow" ? "tomorrow" : dayWords(picked)}.`;

  async function add(): Promise<Task | null> {
    return (await api.createTask({
      title: title.trim().slice(0, 200),
      project_id: projectId || null,
      estimate_minutes: minutes > 0 ? minutes : null,
      start_day: startDay,
    } as unknown as Parameters<typeof api.createTask>[0])) as Task;
  }

  async function hand(t: Task) {
    const s = await api.handOff(t.id, { project_id: projectId || null, brief: brief.trim(), sandbox });
    upsert(s);
    return s;
  }

  async function run(kind: "add" | "hand") {
    if (busy) return;
    setBusy(kind);
    try {
      if (handOnly) {
        const s = await hand(task!);
        toast(`Claude is on ${task!.title}`);
        onClose();
        go({ name: "session", id: s.id });
        return;
      }
      const t = await add();
      if (!t) return;
      if (kind === "add") {
        toast(when === "today" ? "Added to today" : "Added");
        onClose();
        return;
      }
      try {
        const s = await hand(t);
        toast(`Claude is on ${t.title}`);
        onClose();
        go({ name: "session", id: s.id });
      } catch (e) {
        // The task is kept; only the session didn't start.
        toast(`Added, but Claude didn't start: ${e instanceof ApiError ? e.message : "try again from Today."}`);
        onClose();
      }
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't work.");
    } finally {
      setBusy("");
    }
  }

  if (making === "new") return <ProjectSheet onClose={() => setMaking("")} onMade={(p) => setProjectId(p.id)} />;
  if (making === "folder" && project) return <ProjectSheet project={project} onClose={() => setMaking("")} />;
  return (
    <Sheet title={handOnly ? "Hand it to Claude" : "New task"} onClose={onClose}>
      <form
        className="ts"
        onSubmit={(e) => {
          e.preventDefault();
          if (handOnly) canHand && run("hand");
          else if (title.trim()) run("add");
        }}
      >
        {handOnly ? (
          <div>
            <span className="ts-label">Task</span>
            <div className="ts-title">{task!.title}</div>
          </div>
        ) : (
          <label>
            <span className="ts-label">What</span>
            <input
              ref={titleBox}
              className="field"
              value={title}
              maxLength={200}
              enterKeyHint="done"
              placeholder="What needs doing"
              onChange={(e) => {
                setTitle(e.target.value);
                if (!ownBrief.current) setBrief(e.target.value);
              }}
            />
          </label>
        )}

        <div>
          <span className="ts-label" id="ts-project">
            Project
          </span>
          <div className="ts-pills" role="group" aria-labelledby="ts-project">
            {!handOnly && (
              <button type="button" className="ts-pill" aria-pressed={!projectId} onClick={() => setProjectId("")}>
                No project
              </button>
            )}
            {projects.map((p) => (
              <button key={p.id} type="button" className="ts-pill" aria-pressed={projectId === p.id} onClick={() => setProjectId(p.id)}>
                <span className="td-sq" style={{ background: p.color || "var(--dim)" }} aria-hidden />
                {p.name}
              </button>
            ))}
            <button type="button" className="ts-pill" onClick={() => setMaking("new")}>
              + New project
            </button>
          </div>
        </div>

        {!handOnly && (
          <>
            <div>
              <span className="ts-label" id="ts-long">
                How long
              </span>
              <div className="ts-pills" role="group" aria-labelledby="ts-long">
                {lengths.map((l) => (
                  <button key={l.label} type="button" className="ts-pill" aria-pressed={minutes === l.minutes} onClick={() => setMinutes(l.minutes)}>
                    {l.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="ts-label" id="ts-when">
                When
              </span>
              <div className="ts-choices" role="group" aria-labelledby="ts-when">
                <button type="button" className="ts-choice" aria-pressed={when === "today"} onClick={() => setWhen("today")}>
                  Today
                </button>
                <button type="button" className="ts-choice" aria-pressed={when === "tomorrow"} onClick={() => setWhen("tomorrow")}>
                  Tomorrow
                </button>
                <button type="button" className="ts-choice" aria-pressed={when === "pick"} onClick={() => setWhen("pick")}>
                  Pick a day
                </button>
              </div>
              {when === "pick" && (
                <label className="ts-date">
                  <span className="sr">Day</span>
                  <input className="field" type="date" min={today} value={picked} onChange={(e) => e.target.value && setPicked(e.target.value)} />
                </label>
              )}
              <div className="ts-gwen">
                <img src={winking} alt="" />
                {fits}
              </div>
            </div>
          </>
        )}

        <div className="ts-brief">
          <div className="ts-brief-head">
            <Bot size={16} aria-hidden />
            Brief for Claude
            {!handOnly && <span>only if you hand it off</span>}
          </div>
          <label>
            <span className="sr">Brief for Claude</span>
            <textarea
              className="field"
              rows={3}
              value={brief}
              placeholder="What to do, where, and how it knows it's done"
              onChange={(e) => {
                ownBrief.current = true;
                setBrief(e.target.value);
              }}
            />
          </label>
          <div className="ts-choices" role="group" aria-label="How it runs">
            <button type="button" className="ts-choice" aria-pressed={!sandbox} onClick={() => setSandbox(false)}>
              Normal
            </button>
            <button type="button" className="ts-choice" aria-pressed={sandbox} onClick={() => setSandbox(true)}>
              <ShieldCheck size={15} aria-hidden />
              Sandbox
            </button>
          </div>
          {noFolder ? (
            <p className="ts-warn">
              {project!.name} has no folder yet, and Claude needs one to work in.{" "}
              <button type="button" className="ts-link" onClick={() => setMaking("folder")}>
                Give it a folder
              </button>
            </p>
          ) : !project ? (
            <p className="ts-warn" style={{ color: "var(--faint)" }}>
              Pick a project to hand it off: Claude works in its folder.
            </p>
          ) : null}
        </div>

        <div className={handOnly ? "ts-actions one" : "ts-actions"}>
          {!handOnly && (
            <button type="submit" className="btn" disabled={!title.trim() || !!busy}>
              {busy === "add" ? <span className="spin" /> : "Add"}
            </button>
          )}
          <button type="button" className="btn primary" disabled={!canHand || !!busy} onClick={() => run("hand")} aria-label={name ? `Hand ${name} to Claude` : undefined}>
            {busy === "hand" ? (
              <span className="spin" />
            ) : (
              <>
                {handOnly ? <Play size={16} aria-hidden /> : <Bot size={16} aria-hidden />}
                {handOnly ? "Start Claude on it" : "Add, hand to Claude"}
              </>
            )}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
