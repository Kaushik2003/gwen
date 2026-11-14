import { Check, ChevronDown, FolderOpen, Paperclip, Play, ShieldCheck, Smartphone, X } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { App, wire } from "../api";
import Face from "../components/Face";
import { useToast } from "../components/feedback";
import { Button, IconButton, TextArea, cx, unassignedColor } from "../components/ui";
import { useDaemon } from "../daemon";
import { attachmentsBrief, useAttachments } from "../task/Attachments";
import { linkTask } from "../work/terminals";
import { ClaudeMark, Swatch, tildify, useHome } from "./bits";
import { startClaude } from "./ProjectHome";
import { projectChoices, projectPath } from "./projects";
import { screenTab, useTabsMaybe } from "./tabs";

// Hand it to an agent (plan.md 3.15, board 3): a Claude session in the
// task's project folder, with the brief as its first message, its terminal
// linked to the task (so time spent in its tab goes to the task), and the
// task waiting on Claude. A project without a folder gets one here: Gwen
// saves it as the project's folder when the session starts.
//
// Two ways in: HandOff is the bare form (the inbox's one-by-one processing
// puts it in its dialog), HandOffSheet the panel at the window's right edge
// that any task's Hand off button opens.

/** How wide the hand-off sheet is; screens under it keep their content clear of it with useClearOfSheet. */
export const sheetWidth = 400;

export default function HandOff({ task, onDone }: { task: wire.Task; onDone: (said: string) => void }) {
  return <HandOffForm task={task} onDone={onDone} />;
}

/** The hand-off as a panel at the window's right edge: a header with close, the form, Start and Cancel. Escape closes it. */
export function HandOffSheet({ task, onClose }: { task: wire.Task; onClose: () => void }) {
  const notify = useToast();
  const box = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;

  useEffect(() => {
    box.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      // A dialog over the sheet takes Escape first.
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) return;
      e.preventDefault();
      close.current();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return createPortal(
    <aside
      ref={box}
      tabIndex={-1}
      role="dialog"
      aria-label="Hand off to an agent"
      className="fixed top-0 right-0 bottom-[30px] z-40 flex flex-col border-l border-chrome-line bg-chrome text-[13px] text-ink outline-none"
      style={{ width: sheetWidth, maxWidth: "100vw" }}
    >
      <header className="flex h-11 shrink-0 items-center gap-2 border-b border-chrome-line pr-3 pl-4">
        <h2 className="font-semibold">Hand off to an agent</h2>
        <IconButton icon={X} label="Close" size="sm" className="ml-auto" onClick={() => close.current()} />
      </header>
      <HandOffForm
        task={task}
        showTask
        onCancel={() => close.current()}
        onDone={(said) => {
          notify(said, "success");
          close.current();
        }}
      />
    </aside>,
    document.body,
  );
}

/**
 * Keeps a screen's content out from under the hand-off sheet while it is open: the right padding the element needs so
 * nothing it holds sits beneath the sheet, measured against the window as the window and the element resize.
 */
export function useClearOfSheet(ref: RefObject<HTMLElement | null>, open: boolean): number {
  const [pad, setPad] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!open || !el) return setPad(0);
    const measure = () => setPad(Math.max(0, Math.ceil(el.getBoundingClientRect().right - (window.innerWidth - sheetWidth)) + 20));
    measure();
    const seen = new ResizeObserver(measure);
    seen.observe(el);
    window.addEventListener("resize", measure);
    return () => {
      seen.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [open, ref]);
  return pad;
}

function HandOffForm({
  task,
  onDone,
  onCancel,
  showTask,
}: {
  task: wire.Task;
  onDone: (said: string) => void;
  onCancel?: () => void;
  showTask?: boolean;
}) {
  const d = useDaemon();
  const tabs = useTabsMaybe();
  const home = useHome();
  const [pickedProjectId, setPickedProjectId] = useState<string | null>(null);
  const projectId = pickedProjectId ?? task.project_id ?? "";
  // A folder chosen here for a project that had none; it becomes the project's when the session starts.
  const [chosen, setChosen] = useState<Record<string, string>>({});
  const [sandbox, setSandbox] = useState(false);
  const [brief, setBrief] = useState([task.title, task.notes].filter(Boolean).join("\n\n"));
  const [remote, setRemote] = useState<wire.RemoteStatus | null>(null);
  const [busy, setBusy] = useState(false);
  // The task's attached context goes with the brief, its text included, so the agent starts with it.
  const attached = useAttachments(task.id) ?? [];
  const project = d.projects.find((p) => p.id === projectId);
  const folder = project ? project.folder || chosen[project.id] || "" : "";
  const fresh = !!project && !project.folder && !!folder;

  useEffect(() => {
    let live = true;
    App.RemoteStatus().then(
      (r) => live && setRemote(r),
      () => {},
    );
    return () => {
      live = false;
    };
  }, []);

  // The sandbox needs bubblewrap; with none, a session can only run as you.
  const noSandbox = remote != null && !remote.sandbox;
  useEffect(() => {
    if (noSandbox) setSandbox(false);
  }, [noSandbox]);

  async function chooseFolder(p: wire.Project) {
    const path = await App.PickProjectFolder(chosen[p.id] || home).catch((e) => {
      d.fail(e);
      return "";
    });
    if (path) setChosen((c) => ({ ...c, [p.id]: path }));
  }

  async function start() {
    if (!project || !folder || !brief.trim()) return;
    setBusy(true);
    try {
      if (fresh) {
        await App.PatchProject(project.id, wire.PatchProjectRequest.createFrom({ folder }));
        d.refresh();
      }
      const s = await startClaude(folder, project.id, { sandbox, prompt: [brief.trim(), attachmentsBrief(attached)].filter(Boolean).join("\n\n") }, (proj, terminal) => tabs?.open({ kind: "term", project: proj, terminal }, { background: true }));
      if (s.terminal) await linkTask(s.terminal, task.id).catch(() => d.fail("The Claude session started, but it could not be linked to this task."));
      await App.PatchTask(task.id, wire.PatchTaskRequest.createFrom({ stage: "waiting", delegated_to: "Claude", ...(pickedProjectId != null && pickedProjectId !== task.project_id ? { project_id: pickedProjectId } : {}) }));
      onDone(`Claude is on ${task.title} in ${projectPath(project, d.projects)}`);
    } catch (e) {
      d.fail(e);
    } finally {
      setBusy(false);
    }
  }

  const st = d.status;
  const tracked = st && st.state !== "off" ? (d.tasks.find((x) => x.id === st.task_id)?.title ?? d.projects.find((p) => p.id === st.project_id)?.name) : undefined;
  const timer =
    st?.state === "off"
      ? "You're off the clock, and this doesn't start your timer."
      : st?.state === "break_auto" || st?.state === "break_manual"
        ? "Your break carries on; this doesn't touch your timer."
        : tracked
          ? `Your timer stays on ${tracked}.`
          : "Your timer stays where it is.";
  const gwenSays = `${timer} It works ${project ? `in ${project.name}` : "in the project you pick"}, in a tab of its own, and I'll put it in Needs you when it's done.`;
  const openPhoneSettings = () => tabs?.open(screenTab("settings/phone")!);

  return (
    <div className={cx("flex flex-col gap-4", onCancel && "min-h-0 flex-1 overflow-y-auto p-4")}>
      {showTask && (
        <div>
          <Label>Task</Label>
          <div className="mt-[3px] text-[15px] leading-snug font-semibold">{task.title}</div>
        </div>
      )}

      <div>
        <Label id="handoff-folder">Project folder</Label>
        <FolderPicker
          labelledBy="handoff-folder"
          projects={d.projects}
          value={projectId}
          chosen={chosen}
          home={home}
          onPick={(id) => {
            setPickedProjectId(id);
            const p = d.projects.find((p) => p.id === id);
            if (p && !p.folder && !chosen[p.id]) chooseFolder(p);
          }}
        />
        {project && !folder ? (
          <div className="mt-2 flex items-center gap-2.5">
            <Button size="sm" icon={FolderOpen} onClick={() => chooseFolder(project)}>
              Choose a folder
            </Button>
            <span className="text-[11.5px] leading-snug text-ink-subtle">{project.name} has no folder yet. The agent needs one to work in.</span>
          </div>
        ) : fresh ? (
          <Hint>
            Saved as {project!.name}'s folder when the session starts.{" "}
            <button type="button" className="text-ink-muted underline-offset-2 hover:text-ink hover:underline" onClick={() => chooseFolder(project!)}>
              Choose another
            </button>
          </Hint>
        ) : !project ? (
          <Hint>{d.projects.length ? "Pick the project it works for." : "Make a project first: the agent works in a project's folder."}</Hint>
        ) : (
          <Hint>It works right in this folder, beside anything else running there.</Hint>
        )}
      </div>

      <div>
        <Label>Agent</Label>
        <div className="mt-1.5 flex items-center gap-2 font-medium">
          <ClaudeMark />
          Claude Code
        </div>
      </div>

      <div role="radiogroup" aria-labelledby="handoff-safety">
        <Label id="handoff-safety">Safety</Label>
        <div className="mt-1.5 grid grid-cols-2 gap-1.5">
          <Choice on={!sandbox} onClick={() => setSandbox(false)}>
            Normal
          </Choice>
          <Choice on={sandbox} onClick={() => setSandbox(true)} disabled={noSandbox}>
            <ShieldCheck size={13} aria-hidden />
            Sandbox
          </Choice>
        </div>
        <Hint>
          {noSandbox
            ? "Runs as you, like a session you start yourself. The sandbox needs bubblewrap, which isn't installed."
            : sandbox
              ? `Sees only this folder and your toolchains, with a Claude folder of its own.${remote && !remote.sandbox_signed_in ? " It isn't signed in yet, so it asks you to log in once." : ""}`
              : "Runs as you, like a session you start yourself."}
        </Hint>
      </div>

      <label className="block">
        <Label>Brief</Label>
        <TextArea rows={5} value={brief} onChange={(e) => setBrief(e.target.value)} className="mt-1.5 w-full resize-y !py-2.5 leading-normal" placeholder="What should it do, and when should it stop and ask you?" />
        {attached.length > 0 && (
          <span className="mt-1.5 flex items-start gap-1.5 text-[11.5px] leading-snug text-ink-subtle">
            <Paperclip size={12} className="mt-px shrink-0" aria-hidden />
            <span>
              With its {attached.length === 1 ? "attachment" : `${attached.length} attachments`}: {attached.map((a) => a.title).join(", ")}. Notes and app items go in with their text.
            </span>
          </span>
        )}
      </label>

      <p className="flex items-start gap-2.5 text-ink-muted">
        <Smartphone size={15} className="mt-px shrink-0 text-ink-subtle" aria-hidden />
        <span>
          {!remote ? (
            "When it needs you, it shows in Needs you."
          ) : !remote.hooks ? (
            <>
              When it stops, it shows in Needs you. To answer its questions here, <SettingsLink onClick={openPhoneSettings}>turn on Claude Code's hooks</SettingsLink>.
            </>
          ) : remote.devices > 0 && remote.published ? (
            "When it needs you, it asks here and on your phone."
          ) : remote.devices > 0 ? (
            <>
              When it needs you, it asks here. Your phone is off: <SettingsLink onClick={openPhoneSettings}>turn it on</SettingsLink> to answer there too.
            </>
          ) : (
            <>
              When it needs you, it asks here. <SettingsLink onClick={openPhoneSettings}>Pair your phone</SettingsLink> to answer there too.
            </>
          )}
        </span>
      </p>

      <div className={cx("flex items-start gap-2.5 rounded-[10px] px-3 py-2.5", onCancel ? "bg-surface-1" : "bg-surface-2")}>
        <Face mood="winking" className="size-[26px] shrink-0" />
        <p className="text-[12.5px] leading-snug text-ink-muted">{gwenSays}</p>
      </div>

      <div className={cx("flex gap-2", onCancel ? "mt-auto pt-1" : "justify-end")}>
        <Button tone="primary" icon={Play} busy={busy} disabled={!project || !folder || !brief.trim()} onClick={start} className={onCancel ? "flex-1" : undefined}>
          Start the session
        </Button>
        {onCancel && <Button onClick={onCancel}>Cancel</Button>}
      </div>
    </div>
  );
}

/** The project and its folder, as one button that opens the list of projects. */
function FolderPicker({
  projects,
  value,
  chosen,
  home,
  labelledBy,
  onPick,
}: {
  projects: wire.Project[];
  value: string;
  chosen: Record<string, string>;
  home: string;
  labelledBy: string;
  onPick: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const project = projects.find((p) => p.id === value);
  const folderOf = (id: string) => projects.find((p) => p.id === id)?.folder || chosen[id] || "";
  // Projects with a folder first, then the ones that still need one.
  const listed = projectChoices(projects).sort((a, b) => Number(!folderOf(a.id)) - Number(!folderOf(b.id)));

  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setOpen(false);
    // Escape closes the list only, not the sheet or dialog around it.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      setOpen(false);
    };
    document.addEventListener("mousedown", away);
    window.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("mousedown", away);
      window.removeEventListener("keydown", onKey, true);
    };
  }, [open]);

  const path = project ? folderOf(project.id) : "";
  return (
    <div ref={box} className="relative mt-1.5">
      <button
        type="button"
        aria-labelledby={labelledBy}
        aria-haspopup="listbox"
        aria-expanded={open}
        disabled={projects.length === 0}
        onClick={() => setOpen(!open)}
        className="flex h-9 w-full items-center gap-2 rounded-lg border border-line bg-surface-1 px-2.5 text-left transition-colors hover:border-line-strong disabled:opacity-45"
      >
        {project ? (
          <>
            <Swatch color={project.color} />
            <span className="shrink-0 font-medium">{projectPath(project, projects)}</span>
            <span className={cx("min-w-0 flex-1 truncate text-[11.5px]", path ? "font-mono text-ink-subtle" : "text-warning")}>{path ? tildify(path, home) : "No folder yet"}</span>
          </>
        ) : (
          <span className="flex-1 text-ink-subtle">{projects.length ? "Pick a project" : "No projects yet"}</span>
        )}
        <ChevronDown size={13} className="shrink-0 text-ink-subtle" aria-hidden />
      </button>
      {open && (
        <ul role="listbox" aria-labelledby={labelledBy} className="absolute inset-x-0 top-[calc(100%+4px)] z-10 max-h-72 overflow-y-auto rounded-lg border border-line-strong bg-surface-2 py-1">
          {listed.map((p) => {
            const f = folderOf(p.id);
            const on = p.id === value;
            return (
              <li key={p.id} role="option" aria-selected={on}>
                <button
                  type="button"
                  onClick={() => {
                    setOpen(false);
                    onPick(p.id);
                  }}
                  className={cx("flex h-9 w-full items-center gap-2 px-2.5 text-left hover:bg-surface-3", on && "bg-surface-3/60")}
                >
                  <Swatch color={projects.find((project) => project.id === p.id)?.color ?? unassignedColor} />
                  <span className="shrink-0 font-medium">{p.label}</span>
                  <span className={cx("min-w-0 flex-1 truncate text-[11.5px]", f ? "font-mono text-ink-subtle" : "text-ink-faint")}>{f ? tildify(f, home) : "No folder yet: choose one"}</span>
                  {on && <Check size={13} className="shrink-0 text-ink-muted" aria-hidden />}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function Label({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <span id={id} className="block text-[12px] text-ink-subtle">
      {children}
    </span>
  );
}

function Hint({ children }: { children: ReactNode }) {
  return <p className="mt-1.5 text-[11.5px] leading-snug text-ink-subtle">{children}</p>;
}

/** One of two side-by-side choices; the chosen one is lifted with a ring. */
function Choice({ on, onClick, disabled, children }: { on: boolean; onClick: () => void; disabled?: boolean; children: ReactNode }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={on}
      disabled={disabled}
      onClick={onClick}
      className={cx(
        "inline-flex h-[34px] items-center justify-center gap-1.5 rounded-lg font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40",
        on ? "bg-surface-3 text-ink shadow-[inset_0_0_0_1px_var(--color-line-3)]" : "bg-surface-1 text-ink-muted shadow-[inset_0_0_0_1px_var(--color-line)] hover:text-ink",
      )}
    >
      {children}
    </button>
  );
}

function SettingsLink({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" onClick={onClick} className="text-ink underline decoration-line-3 underline-offset-2 hover:decoration-ink">
      {children}
    </button>
  );
}
