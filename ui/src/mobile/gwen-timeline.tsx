import { Check, ChevronDown, ChevronRight, CircleX, Clock, Globe, ListChecks, LoaderCircle, Pencil, ShieldAlert, SquareTerminal, Trash2 } from "lucide-react";
import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { AgentChatDetail, AgentStep, Approval } from "./api";
import { busyState } from "./gwen-chat";
import { Markdown } from "./Markdown";
import { useStore } from "./store";
import { clock } from "./ui";

// The conversation with Gwen on the phone, in time order: your messages, what she did (reads folded into one line), the Claude jobs
// she runs, her replies, her reply so far while she writes it, and what waits for your OK. Ported from the window's AgentChat.

type Message = AgentChatDetail["messages"][number];

const isEvent = (s: AgentStep) => s.tool === "gwen:session";
const input = (s: { input?: unknown }) => ((s.input ?? {}) as Record<string, unknown>);
const field = (s: AgentStep, k: string) => (typeof input(s)[k] === "string" ? (input(s)[k] as string) : "");
// A job's step carries its session's id as "[session:<id>]", which the daemon writes so its card finds its session.
const sessionOf = (s: AgentStep) => /\[session:([^\]]+)\]/.exec(s.output ?? "")?.[1];
const stripMark = (t: string) => t.replace(/\s*\[session:[^\]]+\]/, "");
const readLike = (s: AgentStep) => (s.capability === "fs.read" || s.capability === "gwen.read" || s.capability === "app.read") && s.status === "done";

export function Timeline({ detail, partial, sending, name, onDecide }: { detail: AgentChatDetail; partial: string; sending: string | null; name: string; onDecide: OnDecide }) {
  const { messages, steps } = detail;
  const items = useMemo(() => {
    type Item = { at: number; message?: Message; index?: number; step?: AgentStep };
    const all: Item[] = [...messages.map((m, i) => ({ at: m.at ?? 0, message: m, index: i })), ...steps.map((st) => ({ at: st.started_at, step: st }))];
    all.sort((a, b) => a.at - b.at || (a.message ? -1 : 1));
    const out: ({ kind: "message"; m: Message; key: string } | { kind: "steps"; steps: AgentStep[]; key: string })[] = [];
    for (const it of all) {
      if (it.message) out.push({ kind: "message", m: it.message, key: it.message.id ?? `m${it.index}` });
      else {
        const last = out[out.length - 1];
        if (last?.kind === "steps" && !isEvent(it.step!) && !isEvent(last.steps[last.steps.length - 1])) last.steps.push(it.step!);
        else out.push({ kind: "steps", steps: [it.step!], key: it.step!.id });
      }
    }
    return out;
  }, [messages, steps]);
  const busy = busyState(detail.chat.state);
  return (
    <>
      {items.map((it) =>
        it.kind === "message" ? (
          it.m.role === "user" ? <Bubble key={it.key} text={it.m.text} at={it.m.at} /> : <Reply key={it.key} text={it.m.text} at={it.m.at} name={name} />
        ) : (
          <Steps key={it.key} steps={it.steps} />
        ),
      )}
      {busy && detail.approvals.length === 0 && <Working partial={partial} name={name} />}
      {sending && <Bubble text={sending} pending />}
      {detail.queue.map((q, i) => (
        <Bubble key={`q${i}`} text={q} queued />
      ))}
      {detail.approvals.map((a) => (
        <ApprovalCard key={a.id} approval={a} onDecide={onDecide} />
      ))}
    </>
  );
}

function Bubble({ text, at, pending, queued }: { text: string; at?: number; pending?: boolean; queued?: boolean }) {
  return (
    <div className="g-you">
      <div className={pending || queued ? "g-bubble dim" : "g-bubble"}>{text}</div>
      {queued ? <span className="g-when">Waits for her to finish</span> : pending ? <span className="g-when">Sending…</span> : at ? <span className="g-when">{clock(at)}</span> : null}
    </div>
  );
}

function Reply({ text, at, name }: { text: string; at?: number; name: string }) {
  return (
    <div className="g-reply">
      <Markdown text={text} />
      {at ? (
        <span className="g-when">
          {name} · {clock(at)}
        </span>
      ) : null}
    </div>
  );
}

function Working({ partial, name }: { partial: string; name: string }) {
  if (partial.trim())
    return (
      <div className="g-reply">
        <Markdown text={partial} />
        <span className="g-caret" aria-hidden />
      </div>
    );
  return (
    <p className="g-thinking" role="status">
      <LoaderCircle size={14} className="g-spin" aria-hidden />
      {name} is thinking
    </p>
  );
}

/** A turn's tool calls: reads folded into one line, everything else one row each. */
function Steps({ steps }: { steps: AgentStep[] }) {
  const groups: (AgentStep | AgentStep[])[] = [];
  for (const s of steps) {
    const last = groups[groups.length - 1];
    if (readLike(s) && Array.isArray(last)) last.push(s);
    else groups.push(readLike(s) ? [s] : s);
  }
  return (
    <ul className="g-steps" aria-label="What she did">
      {groups.map((g) => (Array.isArray(g) ? <ReadGroup key={g[0].id} steps={g} /> : <StepRow key={g.id} step={g} />))}
    </ul>
  );
}

function ReadGroup({ steps }: { steps: AgentStep[] }) {
  const [open, setOpen] = useState(false);
  if (steps.length === 1) return <StepRow step={steps[0]} />;
  const files = steps.filter((s) => s.capability === "fs.read").length;
  const n = (k: number, one: string, many: string) => `${k} ${k === 1 ? one : many}`;
  const label = files === steps.length ? `Read ${n(files, "file", "files")}` : files === 0 ? `Looked at ${n(steps.length, "thing", "things")}` : `Read ${n(files, "file", "files")} and looked at ${n(steps.length - files, "thing", "things")}`;
  return (
    <li>
      <button type="button" className="g-step g-fold" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        {open ? <ChevronDown size={14} aria-hidden /> : <ChevronRight size={14} aria-hidden />}
        <span>{label}</span>
      </button>
      {open && (
        <ul className="g-steps inner">
          {steps.map((s) => (
            <StepRow key={s.id} step={s} />
          ))}
        </ul>
      )}
    </li>
  );
}

function Mark({ status }: { status: string }) {
  switch (status) {
    case "running":
      return <LoaderCircle size={13} className="g-spin g-info" aria-label="Running" />;
    case "waiting":
      return <Clock size={13} className="g-warn" aria-label="Waiting for your OK" />;
    case "done":
      return <Check size={13} strokeWidth={2.6} className="g-ok" aria-label="Done" />;
    case "denied":
      return <CircleX size={13} className="g-bad" aria-label="Not allowed" />;
    case "failed":
      return <CircleX size={13} className="g-bad" aria-label="Failed" />;
  }
  return <span className="g-ring" aria-label="Stopped" />;
}

function StepRow({ step: s }: { step: AgentStep }) {
  const [open, setOpen] = useState(false);
  if (s.tool === "mcp__gwen__session_start") return <JobCard step={s} />;
  if (isEvent(s)) return <JobNews step={s} />;
  const failed = s.status === "failed" || s.status === "denied";
  const gwen = s.tool.startsWith("mcp__gwen__");
  const text = gwen && s.output && s.status === "done" ? stripMark(s.output) : s.title;
  const more = failed && !!s.output;
  return (
    <li>
      <button type="button" className={`g-step${failed ? " bad" : s.status === "waiting" ? " warn" : ""}`} onClick={() => more && setOpen((o) => !o)} aria-expanded={more ? open : undefined}>
        <Mark status={s.status} />
        <span className="g-step-text">{text}</span>
      </button>
      {more && open && <p className="g-step-out">{s.output}</p>}
    </li>
  );
}

/** A job she handed to Claude: its title, what Claude is doing now, and Open. */
function JobCard({ step: s }: { step: AgentStep }) {
  const { sessions, go } = useStore();
  const id = sessionOf(s);
  const session = id ? sessions.find((x) => x.id === id) : undefined;
  const failed = s.status === "failed" || s.status === "denied";
  const state = failed
    ? { label: stripMark(s.output) || "It didn't start.", tone: "bad" }
    : s.status === "waiting"
      ? { label: "Waits for your OK to start", tone: "warn" }
      : session?.state === "needs_you"
        ? { label: session.waiting?.summary || "Needs you", tone: "warn" }
        : session?.state === "working" || session?.state === "starting"
          ? { label: session.step || "Working", tone: "info" }
          : session?.state === "done" || session?.state === "idle"
            ? { label: "Finished", tone: "ok" }
            : session
              ? { label: "Ended", tone: "" }
              : { label: "Started", tone: "" };
  return (
    <li className={`g-job ${state.tone}`}>
      <div className="g-job-row">
        <span className="g-job-mark" aria-hidden>
          <SquareTerminal size={15} />
        </span>
        <span className="grow">
          <span className="g-job-title clip">{field(s, "title") || "A job for Claude"}</span>
          <span className={`g-job-state clip ${state.tone}`}>{state.label}</span>
        </span>
        {session && session.state !== "ended" && (
          <button type="button" className="btn sm" onClick={() => go({ name: "session", id: session.id })}>
            {session.state === "needs_you" ? "Answer" : "Open"}
          </button>
        )}
      </div>
    </li>
  );
}

/** News of a job: Claude finished, needs you, or stopped. */
function JobNews({ step: s }: { step: AgentStep }) {
  const kind = field(s, "kind");
  return (
    <li>
      <p className={`g-step g-news ${kind === "finished" ? "ok" : kind === "needs" ? "warn" : ""}`}>
        <SquareTerminal size={13} aria-hidden />
        <span className="g-step-text">{s.title}</span>
      </p>
    </li>
  );
}

// --- Approvals ---

export type OnDecide = (a: Approval, approve: boolean, note?: string) => Promise<unknown>;

const riskWords: Record<string, string> = {
  read: "Reads",
  local: "On this laptop",
  write: "Changes something",
  external: "Leaves this computer",
  destructive: "Can't be undone",
};

const appNames: Record<string, string> = {
  GMAIL: "Gmail",
  SLACK: "Slack",
  GITHUB: "GitHub",
  GOOGLECALENDAR: "Google Calendar",
  NOTION: "Notion",
  GOOGLESHEETS: "Google Sheets",
  GOOGLEDRIVE: "Google Drive",
  GOOGLEDOCS: "Google Docs",
  LINEAR: "Linear",
  JIRA: "Jira",
};
// Who it goes to, then what it says; any other field after them (as the window's ApprovalCard).
const appKeys = ["to", "recipient_email", "recipient", "recipients", "extra_recipients", "cc", "bcc", "channel", "channel_id", "channel_name", "user", "user_id", "users",
  "owner", "repo", "issue_number", "pull_number", "calendar_id", "attendees", "page_id", "parent_id", "database_id", "spreadsheet_id",
  "subject", "title", "name", "summary", "text", "markdown_text", "body", "message", "content", "description", "comment"];

const str = (v: unknown) => (typeof v === "string" ? v : "");
const shown = (v: unknown) => v !== undefined && v !== null && v !== "" && !(Array.isArray(v) && v.length === 0);
/** A value as words: lists joined, a nested object as "key: value" pairs, never raw JSON. */
function words(v: unknown): string {
  if (typeof v === "string") return v;
  if (Array.isArray(v)) return v.map(words).join(", ");
  if (v && typeof v === "object") return Object.entries(v as Record<string, unknown>).filter(([, x]) => shown(x)).map(([k, x]) => `${k.replace(/_/g, " ")}: ${words(x)}`).join(" · ");
  return String(v);
}

function Rows({ args, keys }: { args: Record<string, unknown>; keys?: string[] }) {
  const order = keys ? [...keys.filter((k) => k in args), ...Object.keys(args).filter((k) => !keys.includes(k))] : Object.keys(args);
  const list = order.filter((k) => shown(args[k]));
  if (list.length === 0) return null;
  return (
    <dl className="g-rows">
      {list.map((k) => (
        <Fragment key={k}>
          <dt>{k.replace(/_/g, " ")}</dt>
          <dd>{words(args[k])}</dd>
        </Fragment>
      ))}
    </dl>
  );
}

/** What the approval will do, by its tool, the way you'd check it. */
function What({ payload }: { payload: { tool?: string; input?: Record<string, unknown>; folder?: string } }) {
  const inp = payload.input ?? {};
  switch (payload.tool) {
    case "app_run": {
      const a = inp.arguments;
      const args = a && typeof a === "object" && !Array.isArray(a) ? (a as Record<string, unknown>) : {};
      return Object.keys(args).some((k) => shown(args[k])) ? <Rows args={args} keys={appKeys} /> : null;
    }
    case "Bash":
      return <pre className="g-code">{"$ " + str(inp.command)}</pre>;
    case "Edit":
    case "MultiEdit":
    case "Write":
    case "NotebookEdit":
    case "Read":
      return <p className="g-code">{str(inp.file_path) || str(inp.notebook_path) || str(inp.path)}</p>;
    case "WebFetch":
    case "open":
      return <p className="g-code">{str(inp.url) || str(inp.folder) || str(inp.tab)}</p>;
    case "WebSearch":
      return <p className="g-plain">{str(inp.query)}</p>;
    case "session_start":
      return <p className="g-plain">{str(inp.brief)}</p>;
    case "session_send":
      return <p className="g-plain">{str(inp.text)}</p>;
  }
  return <Rows args={inp} />;
}

/** One approval waiting for you: what will happen, how risky it is, and Allow or Deny (with a note for her). */
function ApprovalCard({ approval: a, onDecide }: { approval: Approval; onDecide: OnDecide }) {
  const [busy, setBusy] = useState("");
  const [denying, setDenying] = useState(false);
  const [note, setNote] = useState("");
  // The note and its buttons open below the card's fold: bring them up above the composer.
  const card = useRef<HTMLElement>(null);
  useEffect(() => {
    if (denying) card.current?.scrollIntoView({ block: "end", behavior: "smooth" });
  }, [denying]);
  const [title, said = ""] = a.summary.split(" · ");
  const risk = riskWords[a.risk] ?? a.risk;
  const reason = said.startsWith(`${risk}: `) ? `Because ${said.slice(risk.length + 2)}.` : said;
  const risky = a.risk === "external" || a.risk === "destructive";
  const payload = (a.payload ?? {}) as { tool?: string; input?: Record<string, unknown>; folder?: string; why?: string };
  const decide = async (key: string, approve: boolean, text?: string) => {
    setBusy(key);
    try {
      await onDecide(a, approve, text);
    } finally {
      setBusy("");
    }
  };
  let heading: ReactNode = title;
  if (payload.tool === "app_run") {
    const [app = "", ...action] = str(payload.input?.tool).split("_");
    heading = `${appNames[app] ?? app.charAt(0) + app.slice(1).toLowerCase()}: ${action.join(" ").toLowerCase()}`;
  }
  return (
    <section ref={card} className={`g-ask${risky ? " risky" : ""}`} aria-label={`Gwen asks: ${title}`}>
      <div className="g-ask-head">
        <span className="g-ask-icon" aria-hidden>
          {a.risk === "destructive" ? <Trash2 size={15} /> : a.risk === "external" ? <Globe size={15} /> : payload.tool === "Bash" ? <SquareTerminal size={15} /> : payload.tool?.startsWith("Edit") || payload.tool === "Write" ? <Pencil size={15} /> : payload.tool?.startsWith("session") ? <ListChecks size={15} /> : <ShieldAlert size={15} />}
        </span>
        <b className="grow">{heading}</b>
        <span className={`g-risk${risky ? " risky" : ""}`}>{risk}</span>
      </div>
      {(payload.why || reason) && <p className="g-why">{payload.why ? `Because ${payload.why}.` : reason}</p>}
      <What payload={payload} />
      {denying ? (
        <form
          className="g-deny"
          onSubmit={(e) => {
            e.preventDefault();
            decide("deny", false, note.trim() || undefined);
          }}
        >
          <label className="sr" htmlFor={`note-${a.id}`}>
            Note for Gwen
          </label>
          <input id={`note-${a.id}`} autoFocus value={note} onChange={(e) => setNote(e.target.value)} placeholder="Why not, or what to do instead (optional)" className="g-note" />
          <div className="g-ask-buttons">
            <button type="submit" className="btn g-deny-btn" disabled={!!busy}>
              {busy === "deny" ? "Saying no…" : "Deny"}
            </button>
            <button type="button" className="btn plain" onClick={() => setDenying(false)} disabled={!!busy}>
              Back
            </button>
          </div>
        </form>
      ) : (
        <div className="g-ask-buttons">
          <button type="button" className="btn primary" disabled={!!busy} onClick={() => decide("allow", true)}>
            {busy === "allow" ? "Allowing…" : "Allow"}
          </button>
          <button type="button" className="btn" disabled={!!busy} onClick={() => setDenying(true)}>
            Deny
          </button>
        </div>
      )}
    </section>
  );
}
