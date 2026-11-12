import { ArrowLeft, MessageSquarePlus } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { api } from "./api";
import { Composer } from "./Composer";
import { useStore } from "./store";
import { TerminalView } from "./TerminalView";
import { Dot, Sheet, tilde } from "./ui";

/** Any terminal of Gwen Terminal, not only Claude: watch it, type into it, or compose a message for it. */
export function TerminalScreen({ terminal }: { terminal: string }) {
  const { terminals, spaces, sessions, go, back } = useStore();
  const [compose, setCompose] = useState(false);
  const t = terminals[terminal];

  // The other panes of the same tab: swipe the header, or tap one, to move between them.
  const panes = useMemo(() => {
    for (const sp of spaces?.spaces ?? []) for (const tab of sp.tabs) if (tab.terminals.some((x) => x.terminal === terminal)) return tab.terminals;
    return [];
  }, [spaces, terminal]);
  const open = (addr: string) => {
    const s = sessions.find((x) => x.terminal === addr);
    go(s ? { name: "session", id: s.id } : { name: "terminal", id: addr }, true);
  };
  const swipe = useRef<number | null>(null);
  const step = (dir: number) => {
    const i = panes.findIndex((p) => p.terminal === terminal);
    const next = panes[i + dir];
    if (i >= 0 && next) open(next.terminal);
  };

  return (
    <>
      <header
        className="bar"
        onTouchStart={(e) => (swipe.current = e.touches[0].clientX)}
        onTouchEnd={(e) => {
          const dx = e.changedTouches[0].clientX - (swipe.current ?? e.changedTouches[0].clientX);
          swipe.current = null;
          if (Math.abs(dx) > 70) step(dx < 0 ? 1 : -1);
        }}
      >
        <button className="icon-btn" aria-label="Back" onClick={back}><ArrowLeft size={21} /></button>
        <div className="grow col" style={{ gap: 1 }}>
          <h1 className="clip">{t?.name ?? terminal}</h1>
          {t?.cwd && <div className="sub clip mono">{tilde(t.cwd)}</div>}
        </div>
        <button className="icon-btn" aria-label="Compose a message" onClick={() => setCompose(true)}><MessageSquarePlus size={20} /></button>
      </header>
      {panes.length > 1 && (
        <nav className="panes" aria-label="Panes of this tab">
          {panes.map((p) => (
            <button key={p.terminal} className={p.terminal === terminal ? "on" : ""} onClick={() => open(p.terminal)}>
              {p.status && <Dot state={p.status === "blocked" ? "needs_you" : p.status} />}
              {p.name}
            </button>
          ))}
        </nav>
      )}
      <TerminalView terminal={terminal} />
      {compose && (
        <Sheet title="Compose" onClose={() => setCompose(false)}>
          <p className="small muted" style={{ padding: "0 16px 6px" }}>Sent as one paste, then Enter.</p>
          <Composer
            terminal={terminal}
            placeholder="Text, a photo, or hold the mic"
            send={async (text, key) => {
              await api.send(terminal, text, key);
              setCompose(false);
            }}
          />
        </Sheet>
      )}
    </>
  );
}
