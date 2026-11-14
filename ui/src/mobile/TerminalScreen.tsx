import { ChevronLeft } from "lucide-react";
import { useMemo, useRef, useState, type TouchEvent } from "react";
import { useDay } from "./day";
import { useStore } from "./store";
import { TerminalView, type PaneSpot } from "./TerminalView";
import { Sheet, tilde } from "./ui";

/**
 * The panes of the tab a terminal is in: swipe the header to move between them, or pick one on the tab map. A pane that runs a
 * Claude session opens as that session, on its Terminal tab.
 */
export function usePanes(terminal: string) {
  const { spaces, sessions, go } = useStore();
  const [map, setMap] = useState(false);
  const tab = useMemo(() => {
    for (const sp of spaces?.spaces ?? []) for (const t of sp.tabs) if (t.terminals.some((x) => x.terminal === terminal)) return t;
    return null;
  }, [spaces, terminal]);
  const panes = tab?.terminals ?? [];
  const at = panes.findIndex((p) => p.terminal === terminal);
  const open = (addr: string) => {
    const s = sessions.find((x) => x.terminal === addr && x.state !== "ended");
    go(s ? { name: "session", id: s.id, tab: "terminal" } : { name: "terminal", id: addr }, true);
  };
  const from = useRef<number | null>(null);
  const swipe = {
    onTouchStart: (e: TouchEvent) => (from.current = e.touches[0].clientX),
    onTouchEnd: (e: TouchEvent) => {
      const dx = e.changedTouches[0].clientX - (from.current ?? e.changedTouches[0].clientX);
      from.current = null;
      const next = panes[at + (dx < 0 ? 1 : -1)];
      if (Math.abs(dx) > 70 && at >= 0 && next) open(next.terminal);
    },
  };
  const spot: PaneSpot | undefined = panes.length > 1 && at >= 0 ? { at: at + 1, of: panes.length, onMap: () => setMap(true) } : undefined;
  const sheet = map && tab && (
    <Sheet title={`Tab map · ${tab.name}`} onClose={() => setMap(false)}>
      <p className="note">The panes of this tab, as they sit on the laptop. Tap one to watch it.</p>
      <div className="pad">
        <div className="tiles" style={{ gridTemplateColumns: `repeat(${Math.min(panes.length, 3)}, minmax(0, 1fr))` }}>
          {panes.map((p, i) => (
            <button key={p.terminal} className="tile" aria-current={i === at ? "true" : undefined} onClick={() => (setMap(false), i !== at && open(p.terminal))}>
              <span className="shot" style={i === at ? { borderColor: "var(--accent)" } : undefined} aria-hidden><span /></span>
              <span className="name">
                <span className={`tdot ${p.status === "blocked" ? "needs" : p.status}`} aria-hidden />
                <span>{p.name}{i === at ? " · here" : ""}</span>
              </span>
            </button>
          ))}
        </div>
      </div>
    </Sheet>
  );
  return { swipe, spot, sheet };
}

/** A terminal's state as its header chip says it. */
const CHIPS: Record<string, [string, string]> = {
  working: ["Running", "working"], blocked: ["Needs you", "needs"], error: ["Error", "error"], done: ["Done", "done"], idle: ["Idle", "quiet"],
};

/** Any terminal of Gwen Terminal, not only Claude: watch it and type into it a line at a time, or straight in (the phone redo, P4). */
export function TerminalScreen({ terminal }: { terminal: string }) {
  const { terminals, spaces, back } = useStore();
  const { projects } = useDay();
  const t = terminals[terminal];
  const { swipe, spot, sheet } = usePanes(terminal);
  const space = spaces?.spaces.find((sp) => sp.tabs.some((tab) => tab.terminals.some((x) => x.terminal === terminal)));
  const folder = space?.folder ?? "";
  const project = projects.find((p) => p.folder && folder && p.folder.replace(/\/+$/, "") === folder.replace(/\/+$/, ""));
  const [word, tone] = CHIPS[t?.status ?? ""] ?? ["", ""];

  return (
    <>
      <header className="head" {...swipe}>
        <button className="ghost" aria-label="Back" onClick={back}>
          <ChevronLeft size={22} />
        </button>
        <div className="grow col" style={{ gap: 2, minWidth: 0 }}>
          <span className="title clip">{t?.name ?? terminal}</span>
          <span className="sub clip">
            {project && <span className="sq" style={{ background: project.color || undefined }} aria-hidden />}
            {[project?.name || space?.name, t?.cwd && tilde(t.cwd)].filter(Boolean).join(" · ")}
          </span>
        </div>
        {word && <span className={`state-chip ${tone}`}>{word}</span>}
      </header>
      <TerminalView terminal={terminal} pane={spot} />
      {sheet}
    </>
  );
}
