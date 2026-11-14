import { ChevronLeft, Ellipsis, Info, Laptop, Lock, Pin, RotateCcw, Shield, ShieldAlert, Square, Trash2, Wrench, X } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { ApiError, api, type Session, type SessionState } from "./api";
import { ASKS, ChangesTab } from "./Changes";
import { Chat } from "./Chat";
import { Composer, type Ask } from "./Composer";
import { useDay } from "./day";
import { keepTab, lastTab, noteSent, useStore, type SessionTab } from "./store";
import { usePanes } from "./TerminalScreen";
import { TerminalView } from "./TerminalView";
import { Dock, MODE_WORDS, Sheet, baseName, wants } from "./ui";

/** Chips above the composer on the Chat tab: each only fills the composer, for you to edit and send. */
const CHAT_ASKS: Ask[] = [
  { label: "Continue", text: "Continue." },
  { label: "Run the tests", text: "Run the tests and tell me what fails." },
  { label: "Commit", text: "Commit what you changed with a clear message." },
];

/** A session's state as its header chip says it. */
const CHIP: Partial<Record<SessionState, [string, string]>> = {
  needs_you: ["Needs you", "needs"], error: ["Error", "error"], working: ["Working", "working"], starting: ["Starting", "working"], done: ["Done", "done"],
  idle: ["Idle", "quiet"], ended: ["Ended", "quiet"],
};

/** One Claude session: its live terminal, the conversation, and what it changed (plan.md 3.14.13). */
export function SessionScreen({ id }: { id: string }) {
  const { sessions, route, go, back, upsert, toast, online } = useStore();
  const { projects } = useDay();
  const s = sessions.find((x) => x.id === id);
  const [menu, setMenu] = useState(false);
  const [confirm, setConfirm] = useState<"clear" | "end" | null>(null);
  const [resume, setResume] = useState(false);
  const [missing, setMissing] = useState(false);
  const panes = usePanes(s?.terminal ?? "");
  const ended = s?.state === "ended";
  // The phone opens a session on the tab you used last; an ended one has no terminal to show.
  const kept = route.tab ?? lastTab();
  const tab: SessionTab = ended && kept === "terminal" ? "chat" : kept;
  const show = (t: SessionTab) => {
    keepTab(t);
    go({ name: "session", id, tab: t }, true);
  };

  // Opening a session is looking at it: it stops being unread, and a finished one becomes idle.
  const unread = s?.unread;
  useEffect(() => {
    if (unread) api.mark(id, { seen: true }).then(upsert, () => {});
  }, [id, unread, upsert]);
  useEffect(() => {
    if (s || online !== true) return;
    api.session(id).then(upsert, () => setMissing(true));
  }, [id, s, online, upsert]);

  const control = async (action: string, said?: string) => {
    setMenu(false);
    setConfirm(null);
    try {
      upsert(await api.control(id, action));
      if (said) toast(said);
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
    }
  };

  if (!s) {
    return (
      <>
        <header className="head">
          <button className="ghost" aria-label="Back" onClick={back}><ChevronLeft size={22} /></button>
        </header>
        <div className="empty">{missing ? <><h2>That session is gone</h2><p>It ended more than a day ago, or it was on another laptop.</p></> : <span className="spin" />}</div>
      </>
    );
  }

  const project = projects.find((p) => p.folder && (s.cwd === p.folder || s.cwd.startsWith(p.folder.replace(/\/+$/, "") + "/")));
  const sub = ["Claude", project?.name || baseName(s.cwd), s.branch, s.sandbox ? "Sandbox" : ""].filter(Boolean).join(" · ");
  const [word, tone] = CHIP[s.state] ?? ["", ""];
  const count = s.changes?.files ?? 0;
  const waiting = s.waiting && !ended ? s.waiting : null;
  // The terminal shows a dialog on the laptop by itself; a held request only shows here, so the Terminal tab says so.
  const waitBar = waiting && (tab !== "terminal" || waiting.held) && (
    <div className="waiting-bar" role="status">
      <ShieldAlert size={16} aria-hidden />
      <span className="grow clip">{waiting.kind === "question" ? "Claude has a question" : `Claude wants to ${wants(waiting.tool)}`}</span>
      <button className="btn xs primary" onClick={() => show("chat")}>Answer</button>
    </div>
  );
  const sendPrompt = async (text: string, key: string) => {
    const { result } = await api.prompt(id, text, key);
    noteSent(id, text);
    return result === "queued" ? "Queued until Claude finishes this step" : undefined;
  };
  const modeChip = s.permission_mode ? (
    <button className="mode hit" aria-label={`Permission mode: ${MODE_WORDS[s.permission_mode] ?? s.permission_mode}. Tap to change it.`} onClick={() => control("cycle_mode", "Switched the permission mode")}>
      {MODE_WORDS[s.permission_mode] ?? s.permission_mode}
    </button>
  ) : undefined;

  return (
    <>
      <header className="head" {...(tab === "terminal" ? panes.swipe : {})}>
        <button className="ghost" aria-label="Back" onClick={back}><ChevronLeft size={22} /></button>
        <div className="grow col" style={{ gap: 2, minWidth: 0 }}>
          <span className="title clip">{s.name}</span>
          <span className="sub clip">
            {project && <span className="sq" style={{ background: project.color || undefined }} aria-hidden />}
            {sub}
          </span>
        </div>
        {word && <span className={`state-chip ${tone}`}>{word}</span>}
        <button className="ghost soft" aria-label="Session actions" onClick={() => setMenu(true)}><Ellipsis size={20} /></button>
      </header>
      <nav className="seg-tabs" role="tablist" aria-label="Session views">
        <button role="tab" aria-selected={tab === "chat"} onClick={() => show("chat")}>Chat</button>
        {!ended && <button role="tab" aria-selected={tab === "terminal"} onClick={() => show("terminal")}>Terminal</button>}
        <button role="tab" aria-selected={tab === "changes"} onClick={() => show("changes")}>
          Changes{count > 0 && <span className="count">{count}</span>}
        </button>
      </nav>
      {s.notice && <div className="info inset" role="note"><Info size={16} aria-hidden /><span>{s.notice}</span></div>}

      {tab === "terminal" && !ended && <TerminalView terminal={s.terminal} pane={panes.spot} bar={waitBar} />}
      {tab === "chat" && <Chat s={s} ask={waiting && <WaitingCard s={s} />} />}
      {tab === "changes" && <ChangesTab s={s} />}

      {ended ? (
        <Dock className="cta">
          <button className="btn primary big full" onClick={() => setResume(true)}><RotateCcw size={18} aria-hidden /> Resume</button>
        </Dock>
      ) : (
        tab !== "terminal" && (
          <Dock>
            {tab === "changes" && waitBar}
            <Composer
              terminal={s.terminal}
              placeholder={s.state === "working" ? "Tell Claude… (queued until this step ends)" : "Tell Claude…"}
              disabled={s.state === "needs_you" ? "Answer Claude first" : s.state === "starting" ? "Claude is starting…" : undefined}
              chip={tab === "chat" ? modeChip : undefined}
              lead={
                (s.state === "working" || s.state === "needs_you" || s.state === "starting") && (
                  <button className="chip stop hit" onClick={() => control("interrupt", "Pressed Esc")}>
                    <Square size={11} fill="currentColor" strokeWidth={0} aria-hidden /> Stop
                  </button>
                )
              }
              asks={tab === "changes" ? (count > 0 ? ASKS : undefined) : CHAT_ASKS}
              send={sendPrompt}
            />
          </Dock>
        )
      )}
      {panes.sheet}

      {menu && (
        <Sheet label="Session actions" onClose={() => setMenu(false)}>
          {(s.state === "working" || s.state === "needs_you") && <button className="item" onClick={() => control("interrupt", "Pressed Esc")}><Square size={19} /> Stop Claude (press Esc)</button>}
          {!ended && <button className="item" onClick={() => control("open_on_laptop", "Opened on the laptop")}><Laptop size={19} /> Open on laptop</button>}
          {!ended && s.permission_mode && (
            <button className="item" onClick={() => control("cycle_mode", "Switched the permission mode")}>
              <Shield size={19} /> <span className="grow">Permission mode</span> <span className="muted">{MODE_WORDS[s.permission_mode] ?? s.permission_mode}</span>
            </button>
          )}
          <button className="item" onClick={async () => (setMenu(false), upsert(await api.mark(id, { pinned: !s.pinned })))}><Pin size={19} /> {s.pinned ? "Unpin" : "Pin to the top"}</button>
          {!ended && <button className="item" onClick={() => control("compact", "Asked Claude to compact")}><Wrench size={19} /> Compact the conversation</button>}
          {!ended && <button className="item" onClick={() => (setMenu(false), setConfirm("clear"))}><Trash2 size={19} /> Clear the conversation</button>}
          {!ended && <button className="item danger" onClick={() => (setMenu(false), setConfirm("end"))}><X size={19} /> End session</button>}
        </Sheet>
      )}
      {confirm && (
        <Sheet title={confirm === "clear" ? "Clear this conversation?" : "End this session?"} onClose={() => setConfirm(null)}>
          <p className="note">
            {confirm === "clear" ? "Claude starts over with nothing remembered from this conversation. The files stay as they are." : "Claude exits. Its tab on the laptop stays open, and you can resume the conversation later."}
          </p>
          <div className="pad">
            <button className="btn danger full" onClick={() => control(confirm, confirm === "clear" ? "Cleared" : "Ending…")}>{confirm === "clear" ? "Clear" : "End session"}</button>
            <button className="btn full" onClick={() => setConfirm(null)}>Cancel</button>
          </div>
        </Sheet>
      )}
      {resume && <ResumeSheet s={s} onClose={() => setResume(false)} />}
    </>
  );
}

/** Normal or Sandbox, as two cards side by side (New session › How it runs). */
export function HowItRuns({ sandbox, onChange, short, last }: { sandbox: boolean; onChange: (sandbox: boolean) => void; short?: boolean; last?: boolean }) {
  const like = (mine: boolean) => (last !== undefined && last === mine ? "Like last time. " : "");
  return (
    <div className="modes" role="radiogroup" aria-label="How it runs">
      <button role="radio" aria-checked={!sandbox} onClick={() => onChange(false)}>
        <b>Normal</b>
        <span>{like(false)}{short ? "Your usual permission rules." : <>Same as typing <span className="mono">claude</span> in a terminal. Your usual permission rules apply.</>}</span>
      </button>
      <button role="radio" aria-checked={sandbox} onClick={() => onChange(true)}>
        <b><Lock size={14} aria-hidden /> Sandbox</b>
        <span>{like(true)}{short ? "Only this folder." : "Can change only this folder. No SSH keys, other folders, or apps. Network stays on."}</span>
      </button>
    </div>
  );
}

/** Resume starts the same conversation again in the same folder; the sheet can switch Normal and Sandbox. */
function ResumeSheet({ s, onClose }: { s: Session; onClose: () => void }) {
  const { go, upsert, toast } = useStore();
  const [sandbox, setSandbox] = useState(s.sandbox);
  const [busy, setBusy] = useState(false);
  const start = async () => {
    setBusy(true);
    try {
      const started = await api.start(s.claude_session_id ? { dir: s.cwd, resume: s.claude_session_id, sandbox } : { dir: s.cwd, sandbox });
      upsert(started);
      go({ name: "session", id: started.id }, true);
    } catch (e) {
      const err = e as ApiError;
      // One Claude per conversation: if it is already running somewhere, open that one.
      if (err.details?.reason === "running") return go({ name: "session", id: String(err.details.session_id) }, true);
      toast(err.message ?? "Couldn't resume.");
      setBusy(false);
    }
  };
  return (
    <Sheet title={`Resume in ${s.name}`} onClose={onClose}>
      <div className="pad">
        <HowItRuns sandbox={sandbox} onChange={setSandbox} short last={s.sandbox} />
        <button className="btn primary big full" disabled={busy} onClick={start}>{busy ? <span className="spin" /> : "Resume"}</button>
      </div>
    </Sheet>
  );
}

/** "Always allow npm run build in this folder", with the command in code type. */
function Label({ text, code }: { text: string; code: string }) {
  const at = code ? text.indexOf(code) : -1;
  if (at < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, at)}
      <span className="mono" style={{ fontSize: 12.5 }}>{code}</span>
      {text.slice(at + code.length)}
    </>
  );
}

/** What Claude is waiting for, with the choices the phone can give, at the end of the conversation. */
function WaitingCard({ s }: { s: Session }) {
  const { toast, upsert } = useStore();
  const w = s.waiting!;
  const [busy, setBusy] = useState("");
  const [denying, setDenying] = useState(false);
  const [why, setWhy] = useState("");

  const answer = async (key: string, message?: string) => {
    setBusy(key);
    try {
      const res = await api.answer(s.id, key, message);
      if (!res.ok) toast(res.message || "Didn't go through. Answer on the laptop.");
      else api.session(s.id).then(upsert, () => {});
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
    } finally {
      setBusy("");
      setDenying(false);
    }
  };
  const spin = (key: string, label: ReactNode) => (busy === key ? <span className="spin" /> : label);
  const allow = w.options.find((o) => o.kind === "allow");
  const always = w.options.filter((o) => o.kind === "always");
  const deny = w.options.find((o) => o.kind === "deny");
  const other = w.options.filter((o) => o.kind === "option" || (o.kind === "allow" && o !== allow));
  // A held permission is Claude waiting on the phone; otherwise the dialog is on the laptop and the phone presses the same choice there.
  const held = w.kind === "permission" && w.held;

  let choices: ReactNode;
  if (w.note) choices = null;
  else if (denying) {
    choices = (
      <>
        <label className="sr" htmlFor="why">Why you said no</label>
        <textarea id="why" className="field" rows={2} autoFocus placeholder="Tell Claude what to do instead" value={why} onChange={(e) => setWhy(e.target.value)} />
        <div className="ask-row two">
          <button className="btn" onClick={() => setDenying(false)}>Back</button>
          <button className="btn danger" disabled={!!busy || !deny} onClick={() => deny && answer(deny.key, why)}>{spin(deny?.key ?? "", "Deny")}</button>
        </div>
      </>
    );
  } else {
    // Allow, Always here, Deny side by side, as in the redo's card; an always that names a command says it in full below.
    const one = always.length === 1 ? always[0] : undefined;
    const row = [allow, one, deny].filter(Boolean).length;
    choices = (
      <>
        {row > 0 && (
          <div className={`ask-row ${row === 3 ? "three" : row === 2 ? "two" : ""}`}>
            {allow && <button className="btn primary" disabled={!!busy} onClick={() => answer(allow.key)}>{spin(allow.key, "Allow")}</button>}
            {one && (
              <button className="btn" disabled={!!busy} aria-label={one.label} onClick={() => answer(one.key)}>
                {spin(one.key, "Always here")}
              </button>
            )}
            {deny && <button className="btn" disabled={!!busy} onClick={() => answer(deny.key)}>{spin(deny.key, "Deny")}</button>}
          </div>
        )}
        {one && w.summary && (
          <p className="ask-always">
            Always here: <Label text={one.label} code={w.summary} />
          </p>
        )}
        {always.length > 1 &&
          always.map((o) => (
            <button key={o.key} className="btn left" disabled={!!busy} onClick={() => answer(o.key)}>
              {spin(o.key, <span><Label text={o.label} code={w.summary} /></span>)}
            </button>
          ))}
        {other.map((o) => (
          <button key={o.key} className="btn left" disabled={!!busy} onClick={() => answer(o.key)}>
            {spin(o.key, <span><Label text={o.label} code={w.summary} /></span>)}
          </button>
        ))}
        {held && deny && (
          <button className="ask-why" disabled={!!busy} onClick={() => setDenying(true)}>
            Deny and say why
          </button>
        )}
      </>
    );
  }

  return (
    <section className="ask" aria-label={w.kind === "question" ? "Question from Claude" : "Permission request"}>
      <div className="ask-title">
        <Shield size={18} aria-hidden />
        <span>{w.kind === "question" ? "Claude has a question" : `Claude wants to ${wants(w.tool)}`}</span>
      </div>
      {w.question && <p className="question">{w.question}</p>}
      {w.summary && <code className="code">{w.summary}</code>}
      {choices}
      <p className="foot">
        {w.note
          ? `${w.note}.`
          : held
            ? "You're away from the laptop, so your answer goes straight to Claude. If you sit down first, the question moves back to the laptop."
            : "This is also showing on the laptop. Your choice is pressed there."}
      </p>
    </section>
  );
}
