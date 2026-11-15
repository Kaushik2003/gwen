import { ArrowLeftRight, Bot, Check, CircleCheck, Coffee, Globe, Play, Plus, SquareTerminal, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import smiling from "../assets/gwen/smiling.png";
import thinking from "../assets/gwen/thinking.png";
import type { wire } from "../wailsjs/go/models";
import { ApiError, api, type Approval, type PlanItem, type Project, type Session, type Task } from "./api";
import { useDay } from "./day";
import { PushCard } from "./PushCard";
import { useStore } from "./store";
import { TaskSheet } from "./TaskSheet";
import { Sheet, agoWords, clock, wants } from "./ui";
import "./today.css";

// Today on the phone (the phone redo, P1): what your time goes to now, what needs you (Gwen's approvals, Claude sessions
// asking, sessions that finished), and today's plan, with + to add a task. Your day comes from useDay, the sessions from
// the store; both follow the laptop's events.

/** "1h 30m", "45m": a span of minutes. */
function span(minutes: number): string {
  const h = Math.floor(minutes / 60);
  const m = Math.round(minutes % 60);
  return h ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`;
}

/** h:mm:ss, the Now card's clock; seconds come back separately so they can be drawn smaller. */
function clockFace(ms: number): [string, string] {
  const s = Math.max(0, Math.floor(ms / 1000));
  return [`${Math.floor(s / 3600)}:${String(Math.floor((s % 3600) / 60)).padStart(2, "0")}`, String(s % 60).padStart(2, "0")];
}

/** Re-renders every second while on. */
function useSecond(on: boolean) {
  const [, setN] = useState(0);
  useEffect(() => {
    if (!on) return;
    const t = window.setInterval(() => setN((n) => n + 1), 1000);
    return () => window.clearInterval(t);
  }, [on]);
}

const working = (state?: string) => state === "working" || state === "idle_pending";
const onBreak = (state?: string) => state === "break_auto" || state === "break_manual";

export function Today() {
  const { meta, online, sessions, go } = useStore();
  const day = useDay();
  const graph = useGraph(day.plan?.day, `${day.plan?.items.map((i) => `${i.id}:${i.status}:${i.start_at}`).join(",")}|${sessions.map((s) => `${s.id}:${s.state}`).join(",")}|${day.status?.task_id}`);
  const [sheet, setSheet] = useState<null | { kind: "new" } | { kind: "hand"; task: Task } | { kind: "switch" }>(null);

  const dateWords = new Date(`${day.plan?.day ?? new Date().toISOString().slice(0, 10)}T12:00:00`).toLocaleDateString(undefined, {
    weekday: "long",
    day: "numeric",
    month: "short",
  });

  const needs = sessions.filter((s) => s.state === "needs_you");
  const finished = sessions.filter((s) => s.state === "done" && s.unread);
  const count = day.approvals.length + needs.length + finished.length;

  return (
    <main className="page">
      <div className="td">
        <header className="td-head">
          <div className="grow">
            <h1>{dateWords}</h1>
            <div className="status">
              <span className={`led ${online === false ? "warn" : "on"}`} aria-hidden />
              <span className="clip">
                {meta.machine || "Your laptop"} · {online === false ? "can't be reached" : clock(Date.now())}
              </span>
            </div>
          </div>
          <button type="button" className="td-face" aria-label="Talk to Gwen" onClick={() => go({ name: "gwen" }, true)}>
            <img src={smiling} alt="" />
          </button>
        </header>

        <PushCard />

        <NowCard onSwitch={() => setSheet({ kind: "switch" })} onHand={(task) => setSheet({ kind: "hand", task })} />

        <section aria-label="Needs you">
          <div className="td-eyebrow">
            <b>Needs you</b>
            {count > 0 && <span className="td-count">{count}</span>}
          </div>
          <div className="td-card">
            {day.approvals.map((a) => (
              <ApprovalItem key={a.id} a={a} />
            ))}
            {needs.map((s) => (
              <SessionNeed key={s.id} s={s} />
            ))}
            {finished.map((s) => (
              <button key={s.id} type="button" className="td-done-row" onClick={() => go({ name: "session", id: s.id })}>
                <span className="td-need-ic green" aria-hidden>
                  <Check size={16} strokeWidth={2.6} />
                </span>
                <span className="td-need-text">
                  <b>Claude in {s.name}</b> <span className="muted">finished</span>
                  <span className="td-why clip">
                    {s.changes && s.changes.files > 0 ? `${s.changes.files} ${s.changes.files === 1 ? "file" : "files"} changed · ` : ""}
                    {agoWords(s.updated_at)}
                  </span>
                </span>
                <span className="td-open">Review</span>
              </button>
            ))}
            {count === 0 && (
              <div className="td-empty">
                <CircleCheck size={18} aria-hidden />
                Nothing needs you. Claude sessions that ask, and Gwen's questions, show here.
              </div>
            )}
          </div>
        </section>

        <InYourWay graph={graph} onAsk={() => go({ name: "gwen" }, true)} />
        <Agenda graph={graph} onHand={(task) => setSheet({ kind: "hand", task })} />
        <OpenTasks onHand={(task) => setSheet({ kind: "hand", task })} />
      </div>

      <button type="button" className="td-fab" aria-label="Add a task" onClick={() => setSheet({ kind: "new" })}>
        <Plus size={24} strokeWidth={2.4} aria-hidden />
      </button>

      {sheet?.kind === "new" && <TaskSheet onClose={() => setSheet(null)} />}
      {sheet?.kind === "hand" && <TaskSheet task={sheet.task} onClose={() => setSheet(null)} />}
      {sheet?.kind === "switch" && <SwitchSheet onClose={() => setSheet(null)} />}
    </main>
  );
}

/** Now: what your time goes to, how long, against the plan; Switch, Break, and handing it to Claude. */
function NowCard({ onSwitch, onHand }: { onSwitch: () => void; onHand: (t: Task) => void }) {
  const day = useDay();
  const st = day.status;
  // The laptop's clock, as the phone's may be a little off.
  const skew = useMemo(() => (st ? st.server_now_at - Date.now() : 0), [st]);
  // tracked_ms is as of when the tasks were read; the open stretch adds to it from then.
  const tasksAt = useRef(Date.now());
  useEffect(() => {
    tasksAt.current = Date.now() + skew;
  }, [day.tasks, day.plan, skew]);
  const live = working(st?.state) || onBreak(st?.state);
  useSecond(live);
  if (!st) return <section className="td-now" aria-label="Now" style={{ minHeight: 132 }} aria-busy />;

  const now = Date.now() + skew;
  const project = day.project(st.project_id);
  const item = st.task_id ? day.plan?.items.find((i) => i.task_id === st.task_id && i.status !== "skipped") : undefined;
  const task = st.task_id ? (day.tasks.find((t) => t.id === st.task_id) ?? item?.task) : undefined;
  const off = st.state === "off";
  const brk = onBreak(st.state);
  const seg = st.open_segment;
  const elapsed = off ? 0 : brk ? now - st.state_since_at : seg ? now - seg.started_at : 0;
  const [hm, ss] = clockFace(elapsed);
  const planned = (item?.planned_minutes ?? 0) * 60_000;
  const tracked = task ? task.tracked_ms + (working(st.state) && seg ? Math.max(0, now - Math.max(tasksAt.current, seg.started_at)) : 0) : 0;

  const title = off ? "Off the clock" : brk ? "On a break" : (task?.title ?? project?.name ?? "Not on anything");
  const stateWord = off ? "Off" : brk ? "Break" : st.state === "idle_pending" ? "Away?" : "Now";
  const color = off ? "var(--dim)" : brk ? "var(--amber)" : "var(--green-dot)";
  const sub = off
    ? "Clock in when you start."
    : brk
      ? task
        ? `Then back to ${task.title}.`
        : "Take your time."
      : [task && project?.name, planned > 0 ? `${span(tracked / 60_000)} of ${span(planned / 60_000)} planned` : null].filter(Boolean).join(" · ") ||
        "Switch to say what this time is for.";

  return (
    <section className={`td-now ${off ? "" : brk ? "td-brk" : "td-on"}`} aria-label="Now">
      <div className="td-now-top">
        <span className="td-dot" style={{ background: color }} aria-hidden />
        <span className="td-state" style={{ color }}>
          {stateWord}
        </span>
        {!off && (
          <span className="td-clock" aria-label={`${hm} and ${ss} seconds`}>
            {hm}
            <small>:{ss}</small>
          </span>
        )}
      </div>
      <div className="td-now-title">{title}</div>
      <div className="td-now-sub">
        {!off && !brk && project && <span className="td-sq" style={{ background: project.color || "var(--dim)" }} aria-hidden />}
        <span className="clip">{sub}</span>
      </div>
      {planned > 0 && !off && (
        <div className="td-bar" aria-hidden>
          <i style={{ width: `${Math.min(100, (tracked / planned) * 100)}%` }} />
        </div>
      )}
      {off ? (
        <div className="td-now-actions two">
          <button type="button" className="btn primary" onClick={() => day.act(() => api.clockIn(st.project_id ?? null, st.task_id ?? null))}>
            <Play size={15} aria-hidden />
            Clock in
          </button>
          <button type="button" className="btn" onClick={onSwitch}>
            <ArrowLeftRight size={15} aria-hidden />
            Start on…
          </button>
        </div>
      ) : (
        <div className={task && task.stage !== "waiting" ? "td-now-actions" : "td-now-actions two"}>
          <button type="button" className="btn" onClick={onSwitch}>
            <ArrowLeftRight size={15} aria-hidden />
            Switch
          </button>
          {brk ? (
            <button type="button" className="btn primary" onClick={() => day.act(api.breakEnd)}>
              Back to work
            </button>
          ) : (
            <button type="button" className="btn" onClick={() => day.act(api.breakStart)}>
              <Coffee size={15} aria-hidden />
              Break
            </button>
          )}
          {task && task.stage !== "waiting" && (
            <button type="button" className="btn" onClick={() => onHand(task)}>
              <Bot size={15} aria-hidden />
              To Claude
            </button>
          )}
        </div>
      )}
    </section>
  );
}

const riskWords: Record<string, string> = { read: "Reads", local: "On this laptop", write: "Changes something", external: "Leaves this computer", destructive: "Can't be undone" };

// Who it goes to, then what it says; any other field after them (as the window's ApprovalCard).
const appKeys = ["to", "recipient_email", "recipient", "recipients", "extra_recipients", "cc", "bcc", "channel", "channel_id", "channel_name", "user", "user_id", "users",
  "owner", "repo", "issue_number", "pull_number", "calendar_id", "attendees", "page_id", "parent_id", "database_id", "spreadsheet_id",
  "subject", "title", "name", "summary", "text", "markdown_text", "body", "message", "content", "description", "comment"];

const str = (v: unknown) => (typeof v === "string" ? v : "");
const words = (v: unknown): string =>
  typeof v === "string"
    ? v
    : Array.isArray(v)
      ? v.map(words).join(", ")
      : v && typeof v === "object"
        ? Object.entries(v as Record<string, unknown>)
            .map(([k, x]) => `${k.replace(/_/g, " ")}: ${words(x)}`)
            .join("; ")
        : String(v);

/** What an approval will do, as rows a person reads: the fields an app action sends, a command, a file, a link. */
function payloadRows(p: { tool?: string; input?: Record<string, unknown> }): { k: string; v: string; mono?: boolean }[] {
  const input = p.input ?? {};
  const shown = (v: unknown) => v !== undefined && v !== null && v !== "" && !(Array.isArray(v) && v.length === 0);
  switch (p.tool) {
    case "app_run": {
      const args = (input.arguments && typeof input.arguments === "object" && !Array.isArray(input.arguments) ? input.arguments : {}) as Record<string, unknown>;
      const keys = [...appKeys.filter((k) => k in args), ...Object.keys(args).filter((k) => !appKeys.includes(k))].filter((k) => shown(args[k]));
      return keys.map((k) => ({ k: k.replace(/_/g, " "), v: words(args[k]) }));
    }
    case "Bash":
      return [{ k: "command", v: str(input.command), mono: true }];
    case "Edit":
    case "MultiEdit":
    case "Write":
    case "NotebookEdit":
      return [{ k: "file", v: str(input.file_path) || str(input.notebook_path), mono: true }];
    case "WebFetch":
    case "open":
      return [{ k: input.url ? "link" : "open", v: str(input.url) || str(input.folder) || str(input.tab) }];
    case "WebSearch":
      return [{ k: "search", v: str(input.query) }];
    case "session_start":
      return [{ k: "brief", v: str(input.brief) }];
    case "session_send":
      return [{ k: "message", v: str(input.text) }];
  }
  return Object.entries(input)
    .filter(([, v]) => shown(v))
    .map(([k, v]) => ({ k: k.replace(/_/g, " "), v: words(v) }));
}

/** One approval of Gwen's agent: what she wants to do, how risky, exactly what goes out, and Allow or Deny. */
function ApprovalItem({ a }: { a: Approval }) {
  const day = useDay();
  const [busy, setBusy] = useState("");
  const [title, said = ""] = a.summary.split(" · ");
  const prefix = `${riskWords[a.risk]}: `;
  const why = said.startsWith(prefix) ? `Because ${said.slice(prefix.length)}.` : said;
  const p = (a.payload ?? {}) as { tool?: string; input?: Record<string, unknown> };
  const rows = payloadRows(p);
  const risky = a.risk === "external" || a.risk === "destructive";
  const decide = async (approve: boolean) => {
    setBusy(approve ? "allow" : "deny");
    await day.act(() => api.decide(a.id, approve), approve ? "Allowed" : "Denied");
    setBusy("");
    day.reload();
  };
  return (
    <div className="td-need" role="group" aria-label={`Gwen asks: ${title}`}>
      <div className="td-need-head">
        <span className="td-need-ic face" aria-hidden>
          <img src={thinking} alt="" />
        </span>
        <span className="td-need-text">
          <b>Gwen</b> <span className="muted">needs your OK</span>
          <span className="td-need-title">{title}</span>
          {why && <span className="td-why">{why}</span>}
          <span className={risky ? "td-risk" : "td-risk calm"}>
            {a.risk === "destructive" ? <Trash2 size={11} aria-hidden /> : a.risk === "external" ? <Globe size={11} aria-hidden /> : null}
            {riskWords[a.risk] ?? a.risk}
          </span>
        </span>
      </div>
      {rows.length > 0 && (
        <dl className="td-kv">
          {rows.map((r, i) => (
            <Row key={i} k={r.k} v={r.v} mono={r.mono} />
          ))}
        </dl>
      )}
      <div className="td-buttons">
        <button type="button" className="btn primary" disabled={!!busy} onClick={() => decide(true)}>
          {busy === "allow" ? <span className="spin" /> : "Allow"}
        </button>
        <button type="button" className="btn" disabled={!!busy} onClick={() => decide(false)}>
          {busy === "deny" ? <span className="spin" /> : "Deny"}
        </button>
      </div>
    </div>
  );
}

function Row({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <>
      <dt>{k}</dt>
      <dd className={mono ? "mono" : undefined}>{v}</dd>
    </>
  );
}

/** A Claude session waiting for you: a permission answered right here, or a question opened in the session. */
function SessionNeed({ s }: { s: Session }) {
  const { go, upsert, toast } = useStore();
  const [busy, setBusy] = useState("");
  const w = s.waiting;
  const permission = w?.kind === "permission" && !w.note;
  const allow = permission ? w!.options.find((o) => o.kind === "allow") : undefined;
  const deny = permission ? w!.options.find((o) => o.kind === "deny") : undefined;
  const open = () => go({ name: "session", id: s.id });
  const answer = async (key: string) => {
    setBusy(key);
    try {
      const res = await api.answer(s.id, key);
      if (!res.ok) toast(res.message || "Didn't go through. Answer in the session.");
      else api.session(s.id).then(upsert, () => {});
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
    } finally {
      setBusy("");
    }
  };
  let ask: ReactNode;
  if (w?.kind === "permission") ask = w.note ? `${w.note}.` : `asks to ${wants(w.tool)}`;
  else if (w?.kind === "question") ask = "has a question";
  else ask = "needs you";
  return (
    <div className="td-need" role="group" aria-label={`Claude in ${s.name} needs you`}>
      <div className="td-need-head">
        <span className="td-need-ic amber" aria-hidden>
          <SquareTerminal size={16} strokeWidth={2.2} />
        </span>
        <span className="td-need-text">
          <b>Claude in {s.name}</b> <span className="muted">{ask}</span>
          {w?.kind === "question" && w.question && <span className="td-why clip">{w.question}</span>}
        </span>
      </div>
      {w?.kind === "permission" && w.summary && (
        <dl className="td-kv">
          <Row k={w.tool === "Bash" ? "command" : "what"} v={w.summary} mono={w.tool === "Bash"} />
        </dl>
      )}
      {allow && deny ? (
        <div className="td-buttons three">
          <button type="button" className="btn primary" disabled={!!busy} onClick={() => answer(allow.key)}>
            {busy === allow.key ? <span className="spin" /> : "Allow"}
          </button>
          <button type="button" className="btn" disabled={!!busy} onClick={() => answer(deny.key)}>
            {busy === deny.key ? <span className="spin" /> : "Deny"}
          </button>
          <button type="button" className="btn" onClick={open}>
            Open
          </button>
        </div>
      ) : (
        <button type="button" className="btn primary full" onClick={open}>
          {w?.kind === "question" ? "Answer" : "Open"}
        </button>
      )}
    </div>
  );
}

/** Starts a task: switches to it, or clocks in on it when you're off. */
function useStart() {
  const day = useDay();
  return (task: Task) =>
    day.act(() => (day.status?.state === "off" ? api.clockIn(task.project_id ?? null, task.id) : api.switchTo(task.project_id ?? null, task.id)), `On ${task.title}`);
}

/** Ticks a task off, or back on. */
function useTick() {
  const day = useDay();
  return (task: Task) =>
    task.status === "done" ? day.act(() => api.reopenTask(task.id), "Back on the list") : day.act(() => api.completeTask(task.id), `Done: ${task.title}`);
}

const claudeOn = (t: Task) => t.stage === "waiting" && /claude/i.test(t.delegated_to);

/** Today's plan, in order: tick, what and when, and Start. */
function Agenda({ graph, onHand }: { graph: wire.DayGraph | null; onHand: (t: Task) => void }) {
  const day = useDay();
  const items = (day.plan?.items ?? []).filter((i) => i.status !== "skipped" && i.task);
  const left = items.filter((i) => i.task.status !== "done");
  const minutes = left.reduce((n, i) => n + i.planned_minutes, 0);
  return (
    <section aria-label="Today's plan">
      <div className="td-eyebrow">
        <b>Today</b>
        {items.length > 0 && <span className="td-meta">{left.length ? `${left.length} left · ${span(minutes)}` : "all done"}</span>}
      </div>
      <div className="td-card">
        {items.map((i) => (
          <AgendaRow key={i.id} item={i} onHand={onHand} node={graph?.tasks.find((t) => t.id === i.task.id)} />
        ))}
        {items.length === 0 && (
          <p className="td-none">{day.loaded ? "Nothing planned today. Your tasks are below; add one with + and Gwen fits it in." : "Loading your day…"}</p>
        )}
      </div>
    </section>
  );
}

function AgendaRow({ item, onHand, node }: { item: PlanItem; onHand: (t: Task) => void; node?: wire.GraphTask }) {
  const day = useDay();
  const start = useStart();
  const tick = useTick();
  const t = item.task;
  const p = day.project(t.project_id);
  const done = t.status === "done";
  const now = day.status?.task_id === t.id && working(day.status?.state);
  const when = item.start_at ? clock(item.start_at) : "";
  return (
    <div className={done ? "td-ag done" : "td-ag"}>
      <button type="button" className={done ? "td-tick on" : "td-tick"} aria-label={done ? `Not done: ${t.title}` : `Done: ${t.title}`} onClick={() => tick(t)}>
        <i>{done && <Check size={14} strokeWidth={3} aria-hidden />}</i>
      </button>
      <div className="td-ag-text">
        <div className="td-ag-title">{t.title}</div>
        <div className="td-ag-sub">
          {p && <span className="td-sq" style={{ background: p.color || "var(--dim)" }} aria-hidden />}
          <span className="clip">{[p?.name, when, item.planned_minutes ? span(item.planned_minutes) : ""].filter(Boolean).join(" · ")}</span>
        </div>
        {!done && node && (node.state === "blocked" || node.state === "at_risk" || node.state === "waiting_on_you") && (
          <div className={`td-flag ${node.state === "blocked" ? "red" : "amber"}`}>{node.reason}</div>
        )}
      </div>
      {done ? null : claudeOn(t) ? (
        <span className="td-chip blue">Claude is on it</span>
      ) : now ? (
        <span className="td-chip green">Now</span>
      ) : (
        <>
          {p?.folder && (
            <button type="button" className="td-start" aria-label={`Hand ${t.title} to Claude`} onClick={() => onHand(t)}>
              <Bot size={16} aria-hidden />
            </button>
          )}
          <button type="button" className="td-start" aria-label={`Start ${t.title}`} onClick={() => start(t)}>
            <Play size={15} fill="currentColor" aria-hidden />
          </button>
        </>
      )}
    </div>
  );
}

/** Open tasks grouped by project, in project order; tasks without a project last. */
function byProject(tasks: Task[], projects: Project[]): { project: Project | null; tasks: Task[] }[] {
  const out: { project: Project | null; tasks: Task[] }[] = projects.map((p) => ({ project: p, tasks: tasks.filter((t) => t.project_id === p.id) }));
  out.push({ project: null, tasks: tasks.filter((t) => !t.project_id || !projects.some((p) => p.id === t.project_id)) });
  return out.filter((g) => g.tasks.length > 0 || g.project);
}

/** Switch: what your time goes to next, a task or just a project. Picking one starts it (or clocks in on it). */
function SwitchSheet({ onClose }: { onClose: () => void }) {
  const day = useDay();
  const st = day.status;
  const off = st?.state === "off";
  const pick = async (project: string | null, task: string | null) => {
    const ok = await day.act(() => (off ? api.clockIn(project, task) : api.switchTo(project, task)));
    if (ok) onClose();
  };
  const open = day.tasks.filter((t) => t.status !== "done" && !t.parent_id);
  return (
    <Sheet title={off ? "Start on" : "Switch to"} onClose={onClose}>
      <div className="td-sheet-list">
        {byProject(open, day.projects).map((g) => (
          <div key={g.project?.id ?? "none"}>
            <div className="td-pick-head">
              {g.project && <span className="td-sq" style={{ background: g.project.color || "var(--dim)" }} aria-hidden />}
              {g.project?.name ?? "No project"}
            </div>
            {g.project && (
              <button type="button" className={st?.project_id === g.project.id && !st?.task_id ? "td-pick on" : "td-pick"} onClick={() => pick(g.project!.id, null)}>
                <span className="grow" style={{ color: "var(--muted)" }}>
                  Just {g.project.name}
                </span>
              </button>
            )}
            {g.tasks.map((t) => (
              <button key={t.id} type="button" className={st?.task_id === t.id ? "td-pick on" : "td-pick"} onClick={() => pick(t.project_id ?? null, t.id)}>
                <span className="grow clip">{t.title}</span>
                {t.estimate_minutes ? <span className="td-ag-sub">{span(t.estimate_minutes)}</span> : null}
              </button>
            ))}
          </div>
        ))}
        {!off && (
          <button type="button" className="td-pick" onClick={() => pick(null, null)}>
            <span className="grow" style={{ color: "var(--muted)" }}>
              Nothing in particular
            </span>
          </button>
        )}
      </div>
    </Sheet>
  );
}

/** Every other open task, right on Today, by project: tick it, start it, or hand it to Claude. */
function OpenTasks({ onHand }: { onHand: (t: Task) => void }) {
  const day = useDay();
  const planned = new Set((day.plan?.items ?? []).filter((i) => i.status !== "skipped" && i.task).map((i) => i.task.id));
  const open = day.tasks.filter((t) => !t.parent_id && t.status !== "done" && !planned.has(t.id));
  const groups = byProject(open, day.projects).filter((g) => g.tasks.length > 0);
  if (!day.loaded) return null;
  return (
    <>
      {groups.map((g) => (
        <section key={g.project?.id ?? "none"} aria-label={g.project?.name ?? "No project"}>
          <div className="td-eyebrow">
            {g.project && <span className="td-sq" style={{ background: g.project.color || "var(--dim)" }} aria-hidden />}
            <b>{g.project?.name ?? "No project"}</b>
            <span className="td-meta">{g.tasks.length}</span>
          </div>
          <div className="td-card">
            {g.tasks.map((t) => (
              <TaskRow key={t.id} task={t} canHand={!!g.project?.folder} onHand={onHand} />
            ))}
          </div>
        </section>
      ))}
      {groups.length === 0 && day.plan?.items?.length ? <p className="td-none">Nothing else open. Add a task with +.</p> : null}
    </>
  );
}

/** One open task: tick, what it is (and how long, when it's due, whether Claude has it), hand to Claude, Start. */
function TaskRow({ task: t, canHand, onHand }: { task: Task; canHand: boolean; onHand: (t: Task) => void }) {
  const day = useDay();
  const start = useStart();
  const tick = useTick();
  const now = day.status?.task_id === t.id && working(day.status?.state);
  const sub = [t.estimate_minutes ? span(t.estimate_minutes) : "", t.due_day ? `due ${t.due_day}` : "", t.stage === "doing" ? "in progress" : ""].filter(Boolean).join(" · ");
  return (
    <div className="td-ag">
      <button type="button" className="td-tick" aria-label={`Done: ${t.title}`} onClick={() => tick(t)}>
        <i />
      </button>
      <div className="td-ag-text">
        <div className="td-ag-title">{t.title}</div>
        {sub && (
          <div className="td-ag-sub">
            <span className="clip">{sub}</span>
          </div>
        )}
      </div>
      {claudeOn(t) ? (
        <span className="td-chip blue">Claude is on it</span>
      ) : now ? (
        <span className="td-chip green">Now</span>
      ) : (
        <>
          {canHand && (
            <button type="button" className="td-start" aria-label={`Hand ${t.title} to Claude`} onClick={() => onHand(t)}>
              <Bot size={16} aria-hidden />
            </button>
          )}
          <button type="button" className="td-start" aria-label={`Start ${t.title}`} onClick={() => start(t)}>
            <Play size={15} fill="currentColor" aria-hidden />
          </button>
        </>
      )}
    </div>
  );
}

/** The day's graph from the laptop, read again when the plan, the sessions, or what you track change, and each minute. */
function useGraph(day: string | undefined, key: string): wire.DayGraph | null {
  const [graph, setGraph] = useState<wire.DayGraph | null>(null);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const t = window.setInterval(() => setTick((n) => n + 1), 60_000);
    return () => window.clearInterval(t);
  }, []);
  useEffect(() => {
    if (!day) return;
    let live = true;
    const t = window.setTimeout(() => {
      api.dayGraph(day).then(
        (g) => live && setGraph(g),
        () => {}, // an older laptop has no graph: the section stays away
      );
    }, 300);
    return () => {
      live = false;
      window.clearTimeout(t);
    };
  }, [day, key, tick]);
  return graph;
}

/** What is blocked, running late, or waiting on you across the day, and why; Gwen can sort it out. */
function InYourWay({ graph, onAsk }: { graph: wire.DayGraph | null; onAsk: () => void }) {
  const issues = graph?.issues ?? [];
  if (!graph || issues.length === 0) return null;
  return (
    <section aria-label="In your way">
      <div className="td-eyebrow">
        <b>In your way</b>
        <span className="td-count">{issues.length}</span>
      </div>
      <div className="td-card">
        {issues.slice(0, 4).map((is) => (
          <div key={`${is.kind}:${is.task_id}`} className="td-way">
            <span className={`td-dot ${is.kind === "blocked" ? "red" : "amber"}`} aria-hidden />
            <span className="td-way-text">{is.text}</span>
          </div>
        ))}
        {issues.length > 4 && <p className="td-none">{issues.length - 4} more on the laptop's timeline</p>}
        <button type="button" className="td-way-ask" onClick={onAsk}>
          Ask Gwen to sort it out
        </button>
      </div>
    </section>
  );
}
