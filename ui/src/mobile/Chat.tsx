import { Check, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api, type Session, type TranscriptEvent } from "./api";
import { Markdown } from "./Markdown";
import { sentHere } from "./store";
import { clock, sessionLine } from "./ui";

/** How a tool's step reads: "Edited src/main.go", "Ran npm run build". */
const VERB: Record<string, string> = {
  Read: "Read", Edit: "Edited", MultiEdit: "Edited", Write: "Wrote", NotebookEdit: "Edited", Bash: "Ran", Grep: "Searched for", Glob: "Looked for",
  WebFetch: "Fetched", WebSearch: "Searched the web for", Task: "Asked an agent:", Agent: "Asked an agent:", TodoWrite: "Updated the to-do list",
};
const FAMILY: Record<string, string> = { Read: "read", Edit: "edit", MultiEdit: "edit", Write: "edit", NotebookEdit: "edit", Grep: "search", Glob: "search" };

type Row = { one: TranscriptEvent } | { many: TranscriptEvent[]; label: string };

/** A run of tool steps, folded the way the mockups show it: "Read 4 files", the first edit, then "Edited 3 more files". */
function fold(events: TranscriptEvent[]): Row[] {
  const out: Row[] = [];
  for (let i = 0; i < events.length; ) {
    const fam = events[i].ok ? FAMILY[events[i].tool] : undefined;
    let j = i + 1;
    if (fam) while (j < events.length && events[j].ok && FAMILY[events[j].tool] === fam) j++;
    const run = events.slice(i, j);
    i = j;
    if (run.length > 1 && fam === "read") out.push({ many: run, label: `Read ${run.length} files` });
    else if (run.length > 1 && fam === "search") out.push({ many: run, label: `Searched ${run.length} times` });
    else if (run.length > 2 && fam === "edit") out.push({ one: run[0] }, { many: run.slice(1), label: `Edited ${run.length - 1} more files` });
    else for (const e of run) out.push({ one: e });
  }
  return out;
}

const Nums = ({ added, removed }: { added: number; removed: number }) =>
  added > 0 || removed > 0 ? (
    <span className="nums">
      <span className="plus">+{added}</span> <span className="minus">−{removed}</span>
    </span>
  ) : null;

function Step({ e, sub }: { e: TranscriptEvent; sub?: boolean }) {
  const [open, setOpen] = useState(false);
  const verb = VERB[e.tool] ?? e.tool;
  const inner = (
    <>
      {e.ok ? <Check size={14} strokeWidth={2.4} aria-label="Done" /> : <X size={14} strokeWidth={2.4} aria-label="Failed" />}
      <span className="grow clip">
        {verb} {e.text && <span className="what">{e.text}</span>}
        {!e.ok && !open && <> · failed{e.output ? ", tap to see why" : ""}</>}
      </span>
      <Nums added={e.added} removed={e.removed} />
    </>
  );
  const cls = `step ${e.ok ? "" : "bad"} ${sub ? "sub" : ""}`;
  if (!e.output) return <div className={cls}>{inner}</div>;
  return (
    <>
      <button className={cls} aria-expanded={open} onClick={() => setOpen((o) => !o)}>{inner}</button>
      {open && <div className="step-out"><pre>{e.output}</pre></div>}
    </>
  );
}

function Many({ r }: { r: { many: TranscriptEvent[]; label: string } }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button className="step" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <Check size={14} strokeWidth={2.4} aria-hidden />
        <span className="grow clip">{r.label}</span>
      </button>
      {open && r.many.map((e, i) => <Step key={`${e.cursor}-${i}`} e={e} sub />)}
    </>
  );
}

function Steps({ events }: { events: TranscriptEvent[] }) {
  return (
    <div className="steps">
      {fold(events).map((r, i) => ("one" in r ? <Step key={i} e={r.one} /> : <Many key={i} r={r} />))}
    </div>
  );
}

/** The conversation, read from Claude Code's transcript on the laptop; `ask` (what Claude waits for) comes last. */
export function Chat({ s, ask }: { s: Session; ask?: ReactNode }) {
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
  const asking = !!ask;
  useEffect(() => {
    const el = box.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [events, s.step, asking]);

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

  // Tool steps next to each other share one bordered list.
  const blocks: ReactNode[] = [];
  const list = events ?? [];
  for (let i = 0; i < list.length; ) {
    const e = list[i];
    if (e.kind === "tool") {
      let j = i;
      while (j < list.length && list[j].kind === "tool") j++;
      blocks.push(<Steps key={`t${e.cursor}-${i}`} events={list.slice(i, j)} />);
      i = j;
      continue;
    }
    const k = `${e.cursor}-${i}`;
    if (e.kind === "you") {
      blocks.push(
        <div key={k} className="you">
          <div className="bubble">{e.text}</div>
          {e.at > 0 && <span className="when">{sentHere(s.id, e.text, e.at) ? "From your phone · " : ""}{clock(e.at)}</span>}
        </div>,
      );
    } else if (e.kind === "claude") blocks.push(<div key={k} className="claude"><Markdown text={e.text} /></div>);
    else blocks.push(<div key={k} className="notice">{e.text}</div>);
    i++;
  }

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
      ) : events.length === 0 && !ask ? (
        <div className="empty">
          <h2>Nothing said yet</h2>
          <p>{s.state === "ended" ? "This conversation has no messages." : "Write the first message below, or watch the Terminal tab."}</p>
        </div>
      ) : (
        <div className="chat">
          {before !== null && <button className="btn xs" style={{ alignSelf: "center" }} onClick={older}>Earlier messages</button>}
          {blocks}
          {s.state === "working" && <div className="working"><span className="spin sm" /> {sessionLine(s)}</div>}
          {ask}
        </div>
      )}
    </main>
  );
}
