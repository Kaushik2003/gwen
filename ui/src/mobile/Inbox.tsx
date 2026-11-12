import { Pin, Search, SquareTerminal, X } from "lucide-react";
import { useMemo, useRef, useState, type PointerEvent } from "react";
import { ApiError, api, type Session, type SessionState, type Terminal } from "./api";
import { useStore } from "./store";
import { Dot, ago, sessionLine, tilde } from "./ui";

// The inbox is home, as in Herdr's phone app: every agent, sorted by what needs you (plan.md 3.14.13).
const GROUPS: { title: string; states: SessionState[] }[] = [
  { title: "Needs you", states: ["needs_you", "error"] },
  { title: "Done", states: ["done"] },
  { title: "Working", states: ["working", "starting"] },
  { title: "Idle", states: ["idle"] },
  { title: "Ended", states: ["ended"] },
];

export function Inbox() {
  const { sessions, meta, go, spaces, terminals, online } = useStore();
  const [searching, setSearching] = useState(false);
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();

  const groups = useMemo(() => {
    const match = (s: Session) => !q || `${s.name} ${s.cwd} ${s.branch} ${s.step} ${s.last_message}`.toLowerCase().includes(q);
    return GROUPS.map((g) => ({
      ...g,
      // Pinned first, then the newest.
      items: sessions.filter((s) => g.states.includes(s.state) && match(s)).sort((a, b) => Number(b.pinned) - Number(a.pinned) || b.updated_at - a.updated_at),
    })).filter((g) => g.items.length > 0);
  }, [sessions, q]);

  // Searching also finds plain terminals, which have no card of their own.
  const found = useMemo(
    () => (q ? Object.values(terminals).filter((t) => !t.session_id && `${t.name} ${t.cwd}`.toLowerCase().includes(q)) : []),
    [terminals, q],
  );

  return (
    <>
      <header className="bar">
        {searching ? (
          <>
            <input className="field grow" autoFocus placeholder="Find a session or terminal" value={query} onChange={(e) => setQuery(e.target.value)} style={{ padding: "8px 11px" }} />
            <button className="icon-btn" aria-label="Close search" onClick={() => (setSearching(false), setQuery(""))}>
              <X size={20} />
            </button>
          </>
        ) : (
          <>
            <div className="grow">
              <h1>Inbox</h1>
              <div className="sub clip">{meta.machine || "Your laptop"}</div>
            </div>
            <button className="icon-btn" aria-label="Search" onClick={() => setSearching(true)}>
              <Search size={20} />
            </button>
          </>
        )}
      </header>
      <main className="page">
        {groups.map((g) => (
          <section key={g.title}>
            <div className="section">
              {g.title} <span className="count">{g.items.length}</span>
            </div>
            {g.items.map((s) => (
              <SessionCard key={s.id} s={s} />
            ))}
          </section>
        ))}
        {found.length > 0 && (
          <section>
            <div className="section">Terminals</div>
            {found.map((t) => (
              <TerminalRow key={t.terminal} t={t} onOpen={() => go({ name: "terminal", id: t.terminal })} roots={[]} />
            ))}
          </section>
        )}
        {groups.length === 0 && found.length === 0 && (
          <div className="empty">
            {q ? (
              <p>Nothing matches “{query}”.</p>
            ) : online === null && !spaces ? (
              <span className="spin" />
            ) : (
              <>
                <h2>No Claude sessions yet</h2>
                <p>Start Claude in Gwen on the laptop and it shows up here, or start one from this phone.</p>
                <button className="btn primary" onClick={() => go({ name: "new" })}>
                  New session
                </button>
              </>
            )}
          </div>
        )}
      </main>
    </>
  );
}

export function TerminalRow({ t, onOpen, helper, roots }: { t: Terminal; onOpen: () => void; helper?: boolean; roots: string[] }) {
  return (
    <button className={`card ${helper ? "helper" : ""}`} onClick={onOpen}>
      {t.status ? <Dot state={t.status === "blocked" ? "needs_you" : t.status} /> : <SquareTerminal size={15} className="muted" style={{ marginTop: 3, flex: "none" }} />}
      <div className="grow col" style={{ gap: 1 }}>
        <span className="title clip" style={helper ? { fontSize: 14, fontWeight: 500 } : undefined}>{t.name}</span>
        {t.cwd && <span className="line clip mono" style={{ fontSize: 12 }}>{tilde(t.cwd, roots)}</span>}
      </div>
    </button>
  );
}

const SWIPE = 84;

/** One agent. Swipe right to pin it and mark it read; swipe left to press Esc. */
function SessionCard({ s }: { s: Session }) {
  const { go, upsert, toast } = useStore();
  const [dx, setDx] = useState(0);
  const start = useRef<{ x: number; y: number; moved: boolean } | null>(null);

  const down = (e: PointerEvent) => {
    start.current = { x: e.clientX, y: e.clientY, moved: false };
  };
  const move = (e: PointerEvent) => {
    const st = start.current;
    if (!st) return;
    const x = e.clientX - st.x;
    if (!st.moved && Math.abs(x) < 12) return;
    if (!st.moved && Math.abs(e.clientY - st.y) > Math.abs(x)) {
      start.current = null; // a scroll, not a swipe
      return;
    }
    st.moved = true;
    const live = s.state !== "ended";
    setDx(Math.max(live ? -140 : 0, Math.min(140, x)));
  };
  const up = async () => {
    const st = start.current;
    start.current = null;
    const x = dx;
    setDx(0);
    if (!st?.moved) return;
    try {
      if (x >= SWIPE) {
        upsert(await api.mark(s.id, { pinned: !s.pinned, seen: true }));
        toast(s.pinned ? "Unpinned" : "Pinned");
      } else if (x <= -SWIPE) {
        await api.control(s.id, "interrupt");
        toast("Pressed Esc");
      }
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
    }
  };

  return (
    <>
      <div className="card-wrap">
        {dx > 0 && <div className="card-under right">{s.pinned ? "Unpin" : "Pin"} · mark read</div>}
        {dx < 0 && <div className="card-under left">Press Esc</div>}
        <button
          className="card"
          style={{ transform: dx ? `translateX(${dx}px)` : undefined, transition: dx ? "none" : "transform 0.18s" }}
          onPointerDown={down}
          onPointerMove={move}
          onPointerUp={up}
          onPointerCancel={() => ((start.current = null), setDx(0))}
          onClick={(e) => {
            if (dx !== 0) return e.preventDefault();
            go({ name: "session", id: s.id });
          }}
        >
          <Dot state={s.state} />
          <div className="grow col" style={{ gap: 2 }}>
            <div className="row" style={{ gap: 7 }}>
              <span className="title clip">{s.name}</span>
              {s.pinned && <Pin size={12} className="muted" style={{ flex: "none" }} />}
              {s.sandbox && <span className="tag">Sandbox</span>}
              <span className="grow" />
              <span className="meta">{ago(s.ended_at ?? s.updated_at)}</span>
              {s.unread && <span className="unread" aria-label="Unread" />}
            </div>
            <span className={`line ${s.state === "needs_you" ? "clip2" : "clip"}`} style={s.state === "needs_you" ? { color: "var(--fg)" } : undefined}>
              {sessionLine(s)}
            </span>
            {s.branch && <span className="line clip mono" style={{ fontSize: 11.5 }}>{s.branch}</span>}
          </div>
        </button>
      </div>
      {s.helpers.map((h) => (
        <TerminalRow key={h.terminal} t={h} helper roots={[]} onOpen={() => go({ name: "terminal", id: h.terminal })} />
      ))}
    </>
  );
}
