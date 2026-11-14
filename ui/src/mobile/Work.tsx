import { AlertCircle, Check, LoaderCircle, Mic, Monitor, Plus, Square, SquareTerminal } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ApiError, api, idem, type Project, type Session, type SessionState, type Space, type Terminal } from "./api";
import { useDay } from "./day";
import { ProjectSheet } from "./ProjectSheet";
import { noteSent, useStore } from "./store";
import { Sheet, agoWords, baseName, lasted, sessionLine, tilde } from "./ui";
import { useHoldToTalk } from "./voice";
import "./work.css";

// Work (the phone redo, P2): everything running on the laptop in one list, Claude sessions and shells, grouped by project, what
// needs you first. Chips filter it; Shell opens a terminal in a project, Claude starts a session, the mic talks to one.

type Filter = "all" | "needs" | "working" | "done";
type Tone = "needs" | "working" | "done" | "quiet" | "error" | "ended";

/** A row of the list: a Claude session or a shell. */
interface Row {
  key: string;
  tone: Tone;
  /** Where it sorts in its group: what needs you first. */
  rank: number;
  at: number;
  filter: Filter | "";
  node: ReactNode;
}

interface Group {
  key: string;
  name: string;
  folder: string;
  color: string;
  space?: Space;
  rows: Row[];
}

const SESSION: Record<SessionState, { tone: Tone; rank: number; filter: Filter | "" }> = {
  needs_you: { tone: "needs", rank: 0, filter: "needs" },
  error: { tone: "error", rank: 1, filter: "needs" },
  working: { tone: "working", rank: 2, filter: "working" },
  starting: { tone: "working", rank: 2, filter: "working" },
  done: { tone: "done", rank: 3, filter: "done" },
  idle: { tone: "quiet", rank: 4, filter: "" },
  ended: { tone: "ended", rank: 6, filter: "" },
};
const TERMINAL: Record<string, { tone: Tone; rank: number; filter: Filter | ""; word: string }> = {
  blocked: { tone: "needs", rank: 0, filter: "needs", word: "Needs you" },
  error: { tone: "error", rank: 1, filter: "needs", word: "Error" },
  working: { tone: "working", rank: 2, filter: "working", word: "Running" },
  done: { tone: "done", rank: 3, filter: "done", word: "Done" },
};
const CHIP: Record<Tone, string> = { needs: "Needs you", error: "Error", working: "Working", done: "Done", quiet: "Idle", ended: "Ended" };

/** The state's icon in its tile. */
function StateIcon({ tone, shell }: { tone: Tone; shell?: boolean }) {
  if (tone === "working") return <LoaderCircle size={16} strokeWidth={2.4} className="w-spin" aria-hidden />;
  if (tone === "needs" || tone === "error") return <AlertCircle size={16} strokeWidth={2.2} aria-hidden />;
  if (tone === "done") return <Check size={16} strokeWidth={2.6} aria-hidden />;
  if (shell) return <SquareTerminal size={16} strokeWidth={2.2} aria-hidden />;
  return <Square size={13} strokeWidth={2.4} aria-hidden />;
}

/** The folder a path is in, of those given: the deepest one. */
function home(path: string, folders: string[]): string | undefined {
  let best: string | undefined;
  for (const f of folders) if (f && (path === f || path.startsWith(f + "/")) && (!best || f.length > best.length)) best = f;
  return best;
}

export function Work() {
  const { sessions, spaces, meta, online, heardAt, go, toast } = useStore();
  const { projects } = useDay();
  const [filter, setFilter] = useState<Filter>("all");
  const [shell, setShell] = useState(false);
  // A new project, or a folder for one without, over the screen.
  const [making, setMaking] = useState<null | { project?: Project }>(null);
  const [heard, setHeard] = useState<string | null>(null);
  const talk = useHoldToTalk((text) => setHeard((h) => (h ? h + " " + text : text)), toast);
  // "updated 2 min ago" and "Working · 4m" move on by themselves.
  const [, tick] = useState(0);
  useEffect(() => {
    const t = window.setInterval(() => tick((n) => n + 1), 30_000);
    return () => window.clearInterval(t);
  }, []);

  const groups = useMemo(() => {
    const byKey = new Map<string, Group>();
    const folders = [...(spaces?.spaces ?? []).map((s) => s.folder), ...projects.map((p) => p.folder)].filter(Boolean);
    const groupOf = (path: string, space?: Space): Group => {
      const folder = (space?.folder || home(path, folders) || path || "").replace(/\/+$/, "");
      const key = folder || space?.id || "loose";
      let g = byKey.get(key);
      if (!g) {
        const p = projects.find((x) => x.folder && x.folder.replace(/\/+$/, "") === folder) ?? projects.find((x) => x.folder && folder.startsWith(x.folder + "/"));
        g = { key, name: p?.name || space?.name || (folder ? baseName(folder) : "Without a window"), folder, color: p?.color || "", space, rows: [] };
        byKey.set(key, g);
      }
      if (space && !g.space) g.space = space;
      return g;
    };
    for (const sp of spaces?.spaces ?? []) groupOf(sp.folder, sp);
    for (const s of sessions) {
      const st = SESSION[s.state] ?? SESSION.idle;
      groupOf(s.cwd).rows.push({ key: s.id, tone: st.tone, rank: s.pinned ? st.rank - 0.5 : st.rank, at: s.updated_at, filter: st.filter, node: <SessionRow s={s} tone={st.tone} /> });
    }
    // Shells: every terminal that isn't a Claude session's own.
    const shells: { t: Terminal; space?: Space }[] = [];
    for (const sp of spaces?.spaces ?? []) for (const tab of sp.tabs) for (const t of tab.terminals) if (!t.session_id) shells.push({ t, space: sp });
    for (const t of spaces?.loose ?? []) if (!t.session_id) shells.push({ t });
    for (const { t, space } of shells) {
      const st = TERMINAL[t.status] ?? { tone: "quiet" as Tone, rank: 5, filter: "" as const, word: "" };
      const g = groupOf(t.cwd, space);
      g.rows.push({ key: t.terminal, tone: st.tone, rank: st.rank, at: 0, filter: st.filter, node: <ShellRow t={t} tone={st.tone} word={st.word} folder={g.folder} /> });
    }
    const out = [...byKey.values()];
    for (const g of out) g.rows.sort((a, b) => a.rank - b.rank || b.at - a.at);
    // The group with something that needs you first, then the one busy most lately.
    const first = (g: Group) => g.rows[0]?.rank ?? 9;
    const last = (g: Group) => Math.max(0, ...g.rows.map((r) => r.at));
    return out.sort((a, b) => first(a) - first(b) || last(b) - last(a) || a.name.localeCompare(b.name));
  }, [sessions, spaces, projects]);

  const counts = useMemo(() => {
    const c = { all: 0, needs: 0, working: 0, done: 0 };
    for (const g of groups)
      for (const r of g.rows) {
        if (r.tone !== "ended") c.all++;
        if (r.filter) c[r.filter]++;
      }
    return c;
  }, [groups]);
  const shown = groups
    .map((g) => ({ ...g, rows: g.rows.filter((r) => (filter === "all" ? true : r.filter === filter)) }))
    .filter((g) => g.rows.length > 0 || (filter === "all" && g.space));

  const machine = meta.machine || "Your laptop";
  const status =
    talk.state === "listening" ? (talk.tapped ? "Listening… tap the mic to stop" : "Listening… let go to stop")
    : talk.state === "hearing" ? "Turning that into text…"
    : online === false ? `${machine} · offline${heardAt ? ` · seen ${agoWords(heardAt)}` : ""}`
    : online === null && !heardAt ? `${machine} · connecting…`
    : `${machine} · ${meta.core === "ok" ? "online" : "Gwen Terminal is off"} · updated ${agoWords(heardAt)}`;
  const led = talk.state === "listening" ? "live" : online === false || online === null ? "" : meta.core === "ok" ? "on" : "warn";
  const chips: { id: Filter; label: string; dot?: string }[] = [
    { id: "all", label: "All" },
    { id: "needs", label: "Needs you", dot: "needs" },
    { id: "working", label: "Working", dot: "working" },
    { id: "done", label: "Done", dot: "done" },
  ];
  const empty = groups.every((g) => g.rows.length === 0);

  return (
    <>
      <header className="w-head">
        <h1 className="grow">Work</h1>
        <button className={`round ${talk.state === "listening" ? "rec" : ""}`} aria-label="Hold to talk to a session" {...talk.bind}>
          {talk.state === "hearing" ? <span className="spin" /> : <Mic size={19} />}
        </button>
        <button className="btn" onClick={() => setShell(true)}>
          <SquareTerminal size={16} aria-hidden /> Shell
        </button>
        <button className="btn primary" onClick={() => go({ name: "new" })}>
          <Plus size={16} strokeWidth={2.4} aria-hidden /> Claude
        </button>
        <div className="status" role="status">
          <span className={`led ${led}`} aria-hidden />
          <span className="clip">{status}</span>
        </div>
      </header>
      <div className="w-chips" role="group" aria-label="Show">
        {chips.map((c) => (
          <button key={c.id} className={`w-chip ${filter === c.id ? "on" : ""}`} aria-pressed={filter === c.id} onClick={() => setFilter(c.id)}>
            {c.dot && <span className={`w-dot ${c.dot}`} aria-hidden />}
            {c.label}
            {(c.id === "all" || counts[c.id] > 0) && <span className="w-n">{counts[c.id]}</span>}
          </button>
        ))}
      </div>
      <main className="page">
        <div className="w-list">
          {shown.map((g) => (
            <section key={g.key} aria-label={g.name}>
              <div className="w-proj">
                <span className="w-sq" style={g.color ? { background: g.color } : undefined} aria-hidden />
                <b className="clip">{g.name}</b>
                {g.folder && <span className="clip">{tilde(g.folder)}</span>}
              </div>
              {g.rows.length > 0 ? (
                <div className="w-card">{g.rows.map((r) => <div key={r.key}>{r.node}</div>)}</div>
              ) : (
                <p className="w-none">Nothing running here.</p>
              )}
            </section>
          ))}
          {shown.length === 0 && (
            <div className="empty">
              {online === null && !spaces ? (
                <span className="spin" />
              ) : empty ? (
                <>
                  <h2>Nothing running</h2>
                  <p>Start Claude in a project's folder, or open a shell. What runs in Gwen Terminal on the laptop shows here too.</p>
                  <button className="btn primary" onClick={() => go({ name: "new" })}>New Claude session</button>
                </>
              ) : (
                <p>Nothing {filter === "needs" ? "needs you" : filter === "working" ? "is working" : "has finished"} right now.</p>
              )}
            </div>
          )}
          <button className="btn full" onClick={() => setMaking({})} style={{ marginTop: 4 }}>
            <Plus size={16} strokeWidth={2.4} aria-hidden /> New project
          </button>
        </div>
      </main>
      {shell && <ShellSheet onClose={() => setShell(false)} onMake={(project) => (setShell(false), setMaking({ project }))} />}
      {making && <ProjectSheet project={making.project} onClose={() => setMaking(null)} />}
      {heard !== null && <VoiceSheet text={heard} onText={setHeard} talk={talk} onClose={() => setHeard(null)} />}
    </>
  );
}

/** One Claude session: its state, what it does or asks, and Review once it finished. */
function SessionRow({ s, tone }: { s: Session; tone: Tone }) {
  const { go, since } = useStore();
  const at = since(s);
  const stat = s.changes && s.changes.files > 0 ? s.changes : null;
  let line: ReactNode = sessionLine(s);
  if (s.state === "needs_you" && s.waiting?.kind === "permission" && s.waiting.summary) line = <>Asks to run <span className="mono">{s.waiting.summary}</span></>;
  else if (s.state === "working" && at) line = <>{s.step || "Working…"} · {lasted(at)}</>;
  else if (s.state === "done")
    line = (
      <>
        Finished {agoWords(s.updated_at)}
        {stat && (
          <>
            {" · "}
            {stat.files} {stat.files === 1 ? "file" : "files"} <span className="plus">+{stat.added}</span> <span className="minus">−{stat.removed}</span>
          </>
        )}
      </>
    );
  else if (s.state === "ended") line = `Ended ${agoWords(s.ended_at ?? s.updated_at)}`;
  else if (s.state === "idle") line = s.window ? `Idle ${lasted(s.updated_at)}` : `No window on the laptop · idle ${lasted(s.updated_at)}`;
  return (
    <button className={`w-row ${tone}`} onClick={() => go({ name: "session", id: s.id })}>
      <span className={`w-ic ${tone}`}><StateIcon tone={tone} /></span>
      <span className="w-text">
        <span className="w-name clip">
          {s.unread && <span className="w-unread" role="img" aria-label="Unread" />}
          {s.name}
        </span>
        <span className="w-line clip">{line}</span>
      </span>
      {s.state === "done" ? <span className="w-review">Review</span> : tone !== "quiet" && <span className={`w-tag ${tone}`}>{CHIP[tone]}</span>}
    </button>
  );
}

/** A shell (or any terminal that isn't a Claude session): what runs there, and where as a short prompt ("demo $", "demo/ui $"). */
function ShellRow({ t, tone, word, folder }: { t: Terminal; tone: Tone; word: string; folder: string }) {
  const { go } = useStore();
  const runs = t.app && t.app !== "shell" ? t.app : "";
  const place = folder && (t.cwd === folder || t.cwd.startsWith(folder + "/")) ? baseName(folder) + t.cwd.slice(folder.length) : tilde(t.cwd);
  return (
    <button className="w-row" onClick={() => go({ name: "terminal", id: t.terminal })}>
      <span className={`w-ic ${tone === "quiet" ? "shell" : tone}`}><StateIcon tone={tone} shell /></span>
      <span className="w-text">
        <span className="w-name clip">{t.name}</span>
        <span className="w-line mono clip">{runs ? `${runs} · ${place}` : place ? `${place} $` : "Shell"}</span>
      </span>
      {word && <span className={`w-tag ${tone}`}>{word}</span>}
    </button>
  );
}

/** Shell: pick a project's space and a terminal opens there, named by itself (gwen, gwen-2, …). */
/** Open a shell in a project's folder. Projects without a folder can get one here, and a new project can be made. */
function ShellSheet({ onClose, onMake }: { onClose: () => void; onMake: (project?: Project) => void }) {
  const { spaces, go, refresh, toast } = useStore();
  const { projects } = useDay();
  const folderless = projects.filter((p) => !p.folder);
  const [busy, setBusy] = useState("");
  const list = spaces?.spaces ?? [];
  const open = async (space: Space) => {
    setBusy(space.id);
    try {
      const { terminal } = await api.newTerminal(space.id);
      refresh();
      onClose();
      if (terminal) go({ name: "terminal", id: terminal });
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Couldn't open a terminal there.");
      setBusy("");
    }
  };
  return (
    <Sheet title="Open a shell in" onClose={onClose}>
      {list.length === 0 ? (
        <p className="note">No project has a folder yet. Give one a folder below, or make a new project.</p>
      ) : (
        list.map((sp) => {
          const p = projects.find((x) => x.folder && x.folder.replace(/\/+$/, "") === sp.folder.replace(/\/+$/, ""));
          return (
            <button key={sp.id} className="item" disabled={!!busy} onClick={() => open(sp)}>
              <span className="w-sq" style={p?.color ? { background: p.color } : undefined} aria-hidden />
              <span className="grow col" style={{ gap: 1 }}>
                <span className="clip">{p?.name || sp.name || baseName(sp.folder)}</span>
                {sp.folder && <span className="clip muted" style={{ fontSize: 12 }}>{tilde(sp.folder)}</span>}
              </span>
              {busy === sp.id && <span className="spin" />}
            </button>
          );
        })
      )}
      {folderless.map((p) => (
        <button key={p.id} className="item" onClick={() => onMake(p)}>
          <span className="w-sq" style={p.color ? { background: p.color } : undefined} aria-hidden />
          <span className="grow col" style={{ gap: 1 }}>
            <span className="clip">{p.name}</span>
            <span className="clip muted" style={{ fontSize: 12 }}>No folder yet · give it one</span>
          </span>
        </button>
      ))}
      <button className="item" onClick={() => onMake()}>
        <Plus size={18} aria-hidden /> New project
      </button>
    </Sheet>
  );
}

const ORDER: Partial<Record<SessionState, number>> = { done: 0, idle: 1, working: 2, error: 3 };
const BARS = [10, 18, 28, 14, 24, 32, 16, 22, 10];

/** Talking from Work: the heard text to check, the session it goes to, and Send (plan.md 3.14.13, the voice sheet). */
function VoiceSheet({ text, onText, talk, onClose }: { text: string; onText: (t: string) => void; talk: ReturnType<typeof useHoldToTalk>; onClose: () => void }) {
  const { sessions, toast } = useStore();
  // Sessions that can take a message now: one waiting on an answer must get the answer first. The one that just finished comes first.
  const targets = sessions.filter((s) => s.state in ORDER).sort((a, b) => ORDER[a.state]! - ORDER[b.state]! || b.updated_at - a.updated_at);
  const [to, setTo] = useState(targets[0]?.id ?? "");
  const [busy, setBusy] = useState(false);
  const key = useRef(idem());
  const target = targets.find((s) => s.id === to);
  const send = async () => {
    if (!target || !text.trim()) return;
    setBusy(true);
    try {
      const { result } = await api.prompt(target.id, text.trim(), key.current);
      noteSent(target.id, text);
      toast(result === "queued" ? `Queued for ${target.name}, behind what Claude is doing` : `Sent to ${target.name}`);
      onClose();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
      setBusy(false);
    }
  };
  const listening = talk.state === "listening";
  return (
    <Sheet label="Voice prompt" onClose={onClose}>
      <div className="voice">
        <div className="voice-head">
          <h2>Check before sending</h2>
          <span>
            <Monitor size={14} aria-hidden /> Heard on your laptop
          </span>
        </div>
        <label className="sr" htmlFor="heard">What you said</label>
        <textarea id="heard" rows={4} value={text} onChange={(e) => onText(e.target.value)} />
        {targets.length > 0 ? (
          <div className="targets" role="radiogroup" aria-label="Send to">
            {targets.map((s) => (
              <button key={s.id} role="radio" aria-checked={s.id === to} onClick={() => setTo(s.id)}>
                {s.name}
                {s.state === "working" && <span className="muted"> · working</span>}
              </button>
            ))}
          </div>
        ) : (
          <p className="muted" style={{ fontSize: 13 }}>No session can take a message right now. Answer the one waiting for you, or start a new one.</p>
        )}
        <div className="buttons">
          <button className="btn flex" style={{ height: 48, borderRadius: 12, fontSize: 15 }} onClick={onClose}>Discard</button>
          <button className="btn primary" style={{ flex: 2, minWidth: 0, height: 48, borderRadius: 12, fontSize: 15 }} disabled={busy || !target || !text.trim()} onClick={send}>
            {busy ? <span className="spin" /> : <span className="clip">Send to {target?.name ?? "…"}</span>}
          </button>
        </div>
        <div className="divider" />
        <div className="talk">
          <div className={`wave ${listening ? "on" : ""}`} aria-hidden>
            {BARS.map((h, i) => <span key={i} style={{ height: h }} />)}
          </div>
          <button className={`mic-big ${listening ? "rec" : ""}`} aria-label="Hold to talk again" {...talk.bind}>
            {talk.state === "hearing" ? <span className="spin" /> : <Mic size={30} strokeWidth={1.8} />}
          </button>
          <p>
            {listening ? (talk.tapped ? "Listening… tap the mic to stop." : "Listening… let go to stop.") : talk.state === "hearing" ? "Turning that into text…" : "Hold to talk and let go, or tap to start and again to stop. Up to 60 seconds."}
            <br />
            Nothing goes to Claude until you tap Send.
          </p>
        </div>
      </div>
    </Sheet>
  );
}
