import { ArrowLeft, Check, ChevronRight, CircleAlert, Ellipsis, Laptop, Pin, RotateCcw, Shield, Square, Trash2, Wrench, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, api, type Change, type Changes, type Diff, type Session, type TranscriptEvent } from "./api";
import { Composer } from "./Composer";
import { Markdown } from "./Markdown";
import { useStore, type SessionTab } from "./store";
import { TerminalView } from "./TerminalView";
import { Dot, MODE_WORDS, STATE_WORDS, Sheet, sessionLine } from "./ui";

/** One Claude session: its live terminal first, as in Herdr's phone app, then Gwen's Chat and Changes (plan.md 3.14.13). */
export function SessionScreen({ id }: { id: string }) {
  const { sessions, route, go, back, upsert, toast, online } = useStore();
  const s = sessions.find((x) => x.id === id);
  const [menu, setMenu] = useState(false);
  const [confirm, setConfirm] = useState<"clear" | "end" | null>(null);
  const [resume, setResume] = useState(false);
  const [missing, setMissing] = useState(false);
  const ended = s?.state === "ended";
  const tab: SessionTab = route.tab ?? (ended ? "chat" : "terminal");

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
        <header className="bar">
          <button className="icon-btn" aria-label="Back" onClick={back}><ArrowLeft size={21} /></button>
        </header>
        <div className="empty">{missing ? <><h2>That session is gone</h2><p>It ended more than a day ago, or it was on another laptop.</p></> : <span className="spin" />}</div>
      </>
    );
  }

  return (
    <>
      <header className="bar">
        <button className="icon-btn" aria-label="Back" onClick={back}><ArrowLeft size={21} /></button>
        <div className="grow col" style={{ gap: 1 }}>
          <div className="row" style={{ gap: 7 }}>
            <h1 className="clip">{s.name}</h1>
            {s.sandbox && <span className="tag">Sandbox</span>}
          </div>
          <div className="row sub" style={{ gap: 6 }}>
            <Dot state={s.state} />
            <span className="clip" style={{ marginLeft: -2 }}>
              {STATE_WORDS[s.state]}
              {s.branch && ` · ${s.branch}`}
              {s.permission_mode && !ended && ` · ${MODE_WORDS[s.permission_mode] ?? s.permission_mode}`}
            </span>
          </div>
        </div>
        {(s.state === "working" || s.state === "needs_you") && (
          <button className="icon-btn" aria-label="Stop" onClick={() => control("interrupt", "Pressed Esc")}><Square size={18} /></button>
        )}
        <button className="icon-btn" aria-label="More" onClick={() => setMenu(true)}><Ellipsis size={21} /></button>
      </header>
      <nav className="subtabs">
        {!ended && <button className={tab === "terminal" ? "on" : ""} onClick={() => go({ name: "session", id, tab: "terminal" }, true)}>Terminal</button>}
        <button className={tab === "chat" ? "on" : ""} onClick={() => go({ name: "session", id, tab: "chat" }, true)}>Chat</button>
        <button className={tab === "changes" ? "on" : ""} onClick={() => go({ name: "session", id, tab: "changes" }, true)}>Changes</button>
      </nav>
      {s.notice && <div className="banner">{s.notice}</div>}

      {tab === "terminal" && !ended && <TerminalView terminal={s.terminal} />}
      {tab === "chat" && <Chat s={s} />}
      {tab === "changes" && <ChangesTab s={s} />}

      {s.waiting && !ended && <WaitingCard s={s} />}
      {ended ? (
        <div className="composer">
          <button className="btn primary full" onClick={() => setResume(true)}><RotateCcw size={17} /> Resume</button>
        </div>
      ) : (
        tab !== "terminal" && (
          <Composer
            terminal={s.terminal}
            placeholder={s.state === "working" ? "Queue the next message" : "Message Claude"}
            disabled={s.state === "needs_you" ? "Answer Claude first." : s.state === "starting" ? "Claude is starting…" : undefined}
            send={async (text, key) => ((await api.prompt(id, text, key)).result === "queued" ? "Queued behind what Claude is doing" : undefined)}
          />
        )
      )}

      {menu && (
        <Sheet onClose={() => setMenu(false)}>
          {!ended && <button className="item" onClick={() => control("open_on_laptop", "Opened on the laptop")}><Laptop size={19} /> Open on laptop</button>}
          {!ended && <button className="item" onClick={() => control("cycle_mode", "Switched the permission mode")}><Shield size={19} /> Change permission mode</button>}
          <button className="item" onClick={async () => (setMenu(false), upsert(await api.mark(id, { pinned: !s.pinned })))}><Pin size={19} /> {s.pinned ? "Unpin" : "Pin to the top"}</button>
          {!ended && <button className="item" onClick={() => control("compact", "Asked Claude to compact")}><Wrench size={19} /> Compact the conversation</button>}
          {!ended && <button className="item" onClick={() => (setMenu(false), setConfirm("clear"))}><Trash2 size={19} /> Clear the conversation</button>}
          {!ended && <button className="item danger" onClick={() => (setMenu(false), setConfirm("end"))}><X size={19} /> End session</button>}
        </Sheet>
      )}
      {confirm && (
        <Sheet title={confirm === "clear" ? "Clear this conversation?" : "End this session?"} onClose={() => setConfirm(null)}>
          <p className="muted" style={{ padding: "0 16px 12px" }}>
            {confirm === "clear" ? "Claude starts over with nothing remembered from this conversation. The files stay as they are." : "Claude exits. Its tab on the laptop stays open, and you can resume the conversation later."}
          </p>
          <div className="pad" style={{ paddingTop: 0 }}>
            <button className="btn danger full" onClick={() => control(confirm, confirm === "clear" ? "Cleared" : "Ending…")}>{confirm === "clear" ? "Clear" : "End session"}</button>
            <button className="btn full" onClick={() => setConfirm(null)}>Cancel</button>
          </div>
        </Sheet>
      )}
      {resume && <ResumeSheet s={s} onClose={() => setResume(false)} />}
    </>
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
      <div className="pad" style={{ paddingTop: 0 }}>
        <div className="seg">
          <button className={!sandbox ? "on" : ""} onClick={() => setSandbox(false)}>Normal</button>
          <button className={sandbox ? "on" : ""} onClick={() => setSandbox(true)}>Sandbox</button>
        </div>
        <p className="small muted">{sandbox ? "Claude only sees this folder and its own files." : "Claude runs as on the laptop, with your own permission rules."}</p>
        <button className="btn primary full" disabled={busy} onClick={start}>{busy ? <span className="spin" /> : "Resume"}</button>
      </div>
    </Sheet>
  );
}

/** What Claude is waiting for, with the choices the phone can give. */
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

  return (
    <section className="waiting" aria-label="Claude is waiting">
      <div className="row" style={{ gap: 8 }}>
        <CircleAlert size={17} style={{ color: "var(--blocked)", flex: "none" }} />
        <b className="grow">{w.kind === "question" ? "Claude has a question" : w.tool ? `Claude wants to use ${w.tool}` : "Claude needs your permission"}</b>
      </div>
      {w.question && <p>{w.question}</p>}
      {w.summary && <div className="what">{w.summary}</div>}
      {w.note ? (
        <p className="small muted">{w.note}.</p>
      ) : denying ? (
        <div className="col" style={{ gap: 8 }}>
          <textarea className="field" rows={2} autoFocus placeholder="Tell Claude what to do instead (optional)" value={why} onChange={(e) => setWhy(e.target.value)} />
          <div className="row">
            <button className="btn grow" onClick={() => setDenying(false)}>Back</button>
            <button className="btn danger grow" disabled={!!busy} onClick={() => answer("deny", why)}>Deny</button>
          </div>
        </div>
      ) : (
        <div className="choices">
          {w.options.map((o) => (
            <button
              key={o.key}
              className={`btn full ${o.kind === "allow" ? "primary" : o.kind === "deny" ? "danger" : ""}`}
              style={{ justifyContent: "flex-start", textAlign: "left", minHeight: 42, padding: "8px 12px", fontWeight: 500 }}
              disabled={!!busy}
              onClick={() => (w.held && o.kind === "deny" ? setDenying(true) : answer(o.key))}
            >
              {busy === o.key ? <span className="spin" /> : o.kind === "allow" ? <Check size={17} /> : o.kind === "deny" ? <X size={17} /> : null}
              <span className="grow">{o.label}</span>
            </button>
          ))}
        </div>
      )}
      {!w.held && !w.note && <p className="small muted">This is also showing on the laptop. Your choice is pressed there.</p>}
    </section>
  );
}

/** The conversation, read from Claude Code's transcript on the laptop. */
function Chat({ s }: { s: Session }) {
  const [events, setEvents] = useState<TranscriptEvent[] | null>(null);
  const [before, setBefore] = useState<number | null>(null);
  const after = useRef(-1);
  const box = useRef<HTMLElement>(null);
  const stick = useRef(true);
  const loadingOlder = useRef(false);

  const poll = useCallback(async () => {
    try {
      if (after.current < 0) {
        const t = await api.transcript(s.id, { limit: 60 });
        after.current = t.after;
        setEvents(t.events);
        setBefore(t.before);
      } else {
        const t = await api.transcript(s.id, { after: after.current });
        after.current = t.after;
        if (t.events.length) setEvents((old) => [...(old ?? []), ...t.events]);
      }
    } catch {
      setEvents((old) => old ?? []);
    }
  }, [s.id]);

  // New lines arrive while Claude works; every change to the session is a reason to look, and so is a short tick while it is busy.
  useEffect(() => {
    poll();
  }, [poll, s.updated_at]);
  useEffect(() => {
    if (s.state !== "working" && s.state !== "needs_you") return;
    const t = window.setInterval(poll, 2000);
    return () => window.clearInterval(t);
  }, [poll, s.state]);
  useEffect(() => {
    const el = box.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [events, s.step]);

  const older = async () => {
    if (before === null || loadingOlder.current) return;
    loadingOlder.current = true;
    const el = box.current;
    const from = el ? el.scrollHeight - el.scrollTop : 0;
    try {
      const t = await api.transcript(s.id, { before, limit: 60 });
      setEvents((old) => [...t.events, ...(old ?? [])]);
      setBefore(t.before);
      requestAnimationFrame(() => el && (el.scrollTop = el.scrollHeight - from));
    } finally {
      loadingOlder.current = false;
    }
  };

  return (
    <main
      className="page"
      ref={box}
      onScroll={(e) => {
        const el = e.currentTarget;
        stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        if (el.scrollTop < 60) older();
      }}
    >
      {events === null ? (
        <div className="empty"><span className="spin" /></div>
      ) : events.length === 0 ? (
        <div className="empty">
          <h2>Nothing said yet</h2>
          <p>{s.state === "ended" ? "This conversation has no messages." : "Write the first message below, or watch the Terminal tab."}</p>
        </div>
      ) : (
        <div className="chat">
          {before !== null && <button className="btn sm" style={{ alignSelf: "center" }} onClick={older}>Earlier messages</button>}
          {events.map((e, i) => <Event key={`${e.cursor}-${i}`} e={e} />)}
          {s.state === "working" && <div className="msg-tool"><span className="spin" style={{ width: 12, height: 12 }} /> {sessionLine(s)}</div>}
        </div>
      )}
    </main>
  );
}

function Event({ e }: { e: TranscriptEvent }) {
  const [open, setOpen] = useState(false);
  switch (e.kind) {
    case "you":
      return <div className="msg-you">{e.text}</div>;
    case "claude":
      return <div className="msg-claude"><Markdown text={e.text} /></div>;
    case "notice":
      return <div className="msg-notice">{e.text}</div>;
  }
  return (
    <div className={`msg-tool ${e.ok ? "" : "bad"}`} onClick={() => e.output && setOpen((o) => !o)}>
      <span className="name">{e.tool}</span>
      <span className="grow col">
        <span className={`sum ${open ? "" : "clip"}`} style={open ? { whiteSpace: "pre-wrap", wordBreak: "break-word" } : undefined}>{e.text}</span>
        {!e.ok && !open && <span>Failed{e.output ? " · tap to see why" : ""}</span>}
        {open && <pre>{e.output}</pre>}
      </span>
      {(e.added > 0 || e.removed > 0) && (
        <span className="mono" style={{ fontSize: 12, flex: "none" }}>
          <span className="plus">+{e.added}</span> <span className="minus">−{e.removed}</span>
        </span>
      )}
    </div>
  );
}

const STATUS_LETTER: Record<string, string> = { added: "A", untracked: "A", modified: "M", deleted: "D", renamed: "R" };

/** What the folder has changed since its last commit, and each file's diff. */
function ChangesTab({ s }: { s: Session }) {
  const [changes, setChanges] = useState<Changes | null>(null);
  const [open, setOpen] = useState<Change | null>(null);
  const [failed, setFailed] = useState("");

  useEffect(() => {
    let live = true;
    api.changes(s.id).then(
      (c) => live && (setChanges(c), setFailed("")),
      (e) => live && setFailed(e instanceof ApiError ? e.message : "Couldn't read the changes."),
    );
    return () => {
      live = false;
    };
    // The step changes each time Claude touches something, which is when the list may have changed.
  }, [s.id, s.state, s.step]);

  if (open) return <DiffView s={s} file={open} onBack={() => setOpen(null)} />;
  if (failed) return <main className="page"><div className="empty"><p>{failed}</p></div></main>;
  if (!changes) return <main className="page"><div className="empty"><span className="spin" /></div></main>;
  const mine = changes.files.filter((f) => !f.before_session);
  const earlier = changes.files.filter((f) => f.before_session);
  const row = (f: Change) => (
    <button key={f.path} className="file" onClick={() => changes.git && setOpen(f)}>
      <span className={`st ${f.status}`}>{STATUS_LETTER[f.status] ?? "M"}</span>
      <span className="path grow clip"><bdi>{f.path}</bdi></span>
      {f.binary ? <span className="nums muted">binary</span> : <span className="nums"><span className="plus">+{f.added}</span> <span className="minus">−{f.removed}</span></span>}
      {changes.git && <ChevronRight size={16} className="muted" style={{ flex: "none" }} />}
    </button>
  );
  return (
    <main className="page">
      {changes.files.length === 0 ? (
        <div className="empty">
          <h2>No changes</h2>
          <p>{changes.git ? "Everything matches the last commit." : "Claude hasn't edited a file in this session."}</p>
        </div>
      ) : (
        <>
          {!changes.git && <div className="banner">This folder isn't a git repository, so Gwen can list the files Claude edited but not show what changed in them.</div>}
          {mine.length > 0 && <div className="section">{changes.git ? "Changed" : "Edited by Claude"} <span className="count">{mine.length}</span></div>}
          {mine.map(row)}
          {earlier.length > 0 && <div className="section">Already changed before this session <span className="count">{earlier.length}</span></div>}
          {earlier.map(row)}
        </>
      )}
    </main>
  );
}

function DiffView({ s, file, onBack }: { s: Session; file: Change; onBack: () => void }) {
  const [diff, setDiff] = useState<Diff | null>(null);
  const [failed, setFailed] = useState("");
  useEffect(() => {
    api.diff(s.id, file.path).then(setDiff, (e) => setFailed(e instanceof ApiError ? e.message : "Couldn't read that file's changes."));
  }, [s.id, file.path]);
  const cls = (l: string) => (l.startsWith("@@") ? "hunk" : l.startsWith("+++") || l.startsWith("---") || l.startsWith("diff ") || l.startsWith("index ") ? "head" : l.startsWith("+") ? "add" : l.startsWith("-") ? "del" : "");
  return (
    <main className="page">
      <button className="file" onClick={onBack} style={{ position: "sticky", top: 0, background: "var(--bg)", zIndex: 1 }}>
        <ArrowLeft size={17} className="muted" />
        <span className="path grow clip"><bdi>{file.path}</bdi></span>
      </button>
      {failed ? (
        <div className="empty"><p>{failed}</p></div>
      ) : !diff ? (
        <div className="empty"><span className="spin" /></div>
      ) : diff.binary ? (
        <div className="empty"><p>A binary file, {Math.max(1, Math.round(diff.size / 1024))} KB. There is nothing to show line by line.</p></div>
      ) : (
        <>
          <div className="diff">{diff.diff.split("\n").map((l, i) => <div key={i} className={cls(l)}>{l || " "}</div>)}</div>
          {diff.truncated && <div className="banner">This diff is long; the first 2,000 lines are shown. See the rest on the laptop.</div>}
        </>
      )}
    </main>
  );
}
