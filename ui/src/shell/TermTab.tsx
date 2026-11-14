import { GitBranch, ShieldCheck, Square, X } from "lucide-react";
import { useEffect, useState } from "react";
import { App, wire } from "../api";
import Face from "../components/Face";
import { useConfirm } from "../components/feedback";
import { Button, IconButton, Select, cx } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { formatDuration } from "../format";
import { useTracking } from "../tracking";
import TerminalPane from "../work/TerminalPane";
import { endTerminal, linkTask, startShell } from "../work/terminals";
import { projectChoices } from "./projects";
import { tabId, useTabs, type Tab } from "./tabs";
import { useWork } from "./work";

// A terminal's tab (plan.md 3.15, board 2): the task it works on and the
// time on it, the live terminal (split into two when asked), and Claude's
// questions answered in place.

type TermTabT = Extract<Tab, { kind: "term" }>;

export default function TermTab({ tab, active }: { tab: TermTabT; active: boolean }) {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const confirm = useConfirm();
  const [focus, setFocus] = useState<"main" | "split">("main");
  const term = w.terminals.find((x) => x.address === tab.terminal);
  const project = d.projects.find((p) => p.id === (tab.project || term?.project));
  const session = term?.session ?? null;
  const id = tabId(tab);

  // A finished session you are looking at is read.
  const unread = active && session?.unread ? session.id : "";
  useEffect(() => {
    if (unread) App.CodeMark(unread, wire.CodeMark.createFrom({ seen: true })).then(w.refreshSessions, () => {});
  }, [unread, w.refreshSessions]);

  // The tab bar's split button: a shell beside this terminal, in the same folder; again closes it.
  useEffect(() => {
    const onSplit = async (e: Event) => {
      if ((e as CustomEvent).detail !== id) return;
      if (tab.split) return tabs.update(id, { ...tab, split: undefined });
      const cwd = project?.folder || term?.cwd;
      if (!cwd) return;
      try {
        const t = await startShell(cwd, project?.id ?? "");
        tabs.update(id, { ...tab, split: t.address });
        setFocus("split");
      } catch (err) {
        d.fail(err);
      }
    };
    window.addEventListener("gwen:split", onSplit);
    return () => window.removeEventListener("gwen:split", onSplit);
  }, [id, tab, tabs, project, term, d]);

  async function closeShell() {
    if (!term || term.claude) return;
    if (!(await confirm({ title: "End this shell?", body: "Whatever runs in it stops. Closing the tab instead keeps it running.", confirm: "End shell", danger: true }))) return;
    await endTerminal(term.address).catch(d.fail);
    tabs.close(id);
  }

  return (
    <div className="flex h-full flex-col">
      <TaskStrip tab={tab} onEndShell={term && !term.claude ? closeShell : undefined} />
      <div className="relative flex min-h-0 flex-1 bg-canvas">
        <div className={cx("min-w-0 flex-1", tab.split && focus === "main" && "ring-1 ring-line-strong ring-inset")} onMouseDown={() => setFocus("main")}>
          <TerminalPane terminal={tab.terminal} focused={active && focus === "main"} />
        </div>
        {tab.split && (
          <>
            <div className="w-px shrink-0 bg-chrome-line" />
            <div className={cx("relative min-w-0 flex-1", focus === "split" && "ring-1 ring-line-strong ring-inset")} onMouseDown={() => setFocus("split")}>
              <TerminalPane terminal={tab.split} focused={active && focus === "split"} onClosed={() => tabs.update(id, { ...tab, split: undefined })} />
              <IconButton
                icon={X}
                label="Close the split (the shell keeps running)"
                size="sm"
                className="absolute top-1.5 right-1.5 bg-surface-2/80"
                onClick={() => tabs.update(id, { ...tab, split: undefined })}
              />
            </div>
          </>
        )}
      </div>
      {session?.state === "needs_you" && session.waiting && <AnswerBar session={session} />}
    </div>
  );
}

/** The strip above a terminal: its task, the time on it, its branch, and how it runs. */
function TaskStrip({ tab, onEndShell }: { tab: TermTabT; onEndShell?: () => void }) {
  const d = useDaemon();
  const w = useWork();
  const t = useTracking();
  useTick(1000);
  const term = w.terminals.find((x) => x.address === tab.terminal);
  const projectId = tab.project || term?.project || "";
  const project = d.projects.find((p) => p.id === projectId);
  const session = term?.session ?? null;
  const task = d.tasks.find((x) => x.id === term?.task);
  const st = d.status;
  const tracking = !!st && t.working && st.project_id === projectId && (!task || st.task_id === task.id);
  const live = tracking && st?.open_segment ? d.now() - st.open_segment.started_at : 0;
  const today = d.today();
  // Open top-level tasks of this terminal's project (with its sub-projects'), and always the one it is linked to.
  const options = d.tasks.filter(
    (x) =>
      x.id === task?.id ||
      (x.status === "open" && !x.parent_id && !x.rrule && (!x.occurrence_day || x.occurrence_day <= today) && (!projectId || x.project_id === projectId || d.projects.some((p) => p.id === x.project_id && p.parent_id === projectId))),
  );
  const taskGroups = [...projectChoices(d.projects), { id: "", label: "No project" }]
    .map((p) => ({ ...p, tasks: options.filter((x) => (x.project_id ?? "") === p.id) }))
    .filter((p) => p.tasks.length > 0);
  const follow = d.config?.tracking.follow_tabs !== false;

  async function setTask(taskId: string) {
    if (!term) return;
    await linkTask(term.address, taskId).catch(d.fail);
    w.refreshTerminals();
    if (taskId && t.working && st?.project_id === projectId) t.switchTo(projectId || null, taskId);
  }

  return (
    <div className="flex h-10 shrink-0 items-center gap-3 overflow-hidden border-b border-chrome-line bg-chrome-deep px-3.5 text-[12px] whitespace-nowrap text-ink-muted">
      <span className="text-ink-subtle">Task</span>
      <Select value={task?.id ?? ""} onChange={(e) => setTask(e.target.value)} aria-label="This terminal's task" className="h-7 w-56 shrink-0 text-[12.5px]" disabled={!term}>
        <option value="">No task</option>
        {taskGroups.map((p) => (
          <optgroup key={p.id} label={p.label}>
            {p.tasks.map((x) => (
              <option key={x.id} value={x.id}>
                {x.title}
              </option>
            ))}
          </optgroup>
        ))}
      </Select>
      {project && (
        <span className="inline-flex min-w-0 items-center gap-1.5 truncate">
          {tracking ? (
            <>
              <span className="size-[7px] animate-live rounded-full bg-working motion-reduce:animate-none" />
              <span className="font-mono text-ink tabular-nums">{formatDuration(live)}</span> on it
            </>
          ) : task ? (
            <span>{formatDuration(task.tracked_ms)} tracked</span>
          ) : null}
          {follow && <span className="truncate text-ink-subtle">· tracked while this tab has focus</span>}
        </span>
      )}
      <span className="flex-1" />
      {session?.branch && (
        <span className="inline-flex items-center gap-1 font-mono text-[11.5px]">
          <GitBranch size={12} aria-hidden />
          {session.branch}
        </span>
      )}
      {session && (
        <span className={cx("inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11.5px]", session.sandbox ? "bg-success/12 text-success" : "bg-surface-2")}>
          {session.sandbox && <ShieldCheck size={12} aria-hidden />}
          {session.sandbox ? "Sandbox" : "Normal"}
        </span>
      )}
      {session && session.state !== "ended" && (
        <Button size="sm" tone="ghost" icon={Square} onClick={() => App.CodeControl(session.id, wire.CodeControl.createFrom({ action: "end" })).catch(d.fail)}>
          End session
        </Button>
      )}
      {onEndShell && (
        <Button size="sm" tone="ghost" icon={Square} onClick={onEndShell}>
          End shell
        </Button>
      )}
    </div>
  );
}

/** What Claude is waiting on, answered here: the options it offers, as buttons. */
function AnswerBar({ session }: { session: wire.CodeSession }) {
  const w = useWork();
  const [busy, setBusy] = useState("");
  const [note, setNote] = useState("");
  const wait = session.waiting!;
  const name = "Gwen";

  async function answer(key: string) {
    setBusy(key);
    const r = await App.CodeAnswer(session.id, wire.CodeAnswer.createFrom({ key })).catch((e) => ({ ok: false, message: String(e) }));
    setBusy("");
    if (!r.ok) setNote(r.message || "That didn't go through. Answer in the terminal.");
    w.refreshSessions();
  }

  return (
    <div role="status" className="flex shrink-0 flex-wrap items-center gap-3 border-t border-chrome-line bg-surface-1 px-4 py-3">
      <Face mood="thinking" className="size-8 shrink-0" />
      <div className="min-w-64 flex-1">
        <div className="text-[12px] text-ink-subtle">{name}</div>
        <p className="text-[13px]">
          {wait.kind === "question" ? wait.question : `Claude wants to use ${wait.tool}${wait.summary ? `: ` : ""}`}
          {wait.kind !== "question" && wait.summary && <code className="font-mono text-[12px] text-ink-muted">{wait.summary}</code>}
        </p>
        {(note || wait.note) && <p className="mt-0.5 text-[12px] text-ink-subtle">{note || wait.note}</p>}
      </div>
      <div className="flex flex-wrap gap-1.5">
        {wait.options.map((o, i) => (
          <Button key={o.key} size="sm" tone={i === 0 ? "primary" : o.kind === "deny" ? "ghost" : "secondary"} busy={busy === o.key} onClick={() => answer(o.key)}>
            {o.label}
          </Button>
        ))}
      </div>
    </div>
  );
}
