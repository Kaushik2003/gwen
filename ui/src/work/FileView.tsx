import { ExternalLink, FileWarning, GitCommitHorizontal, History as HistoryIcon, LoaderCircle, UserRoundSearch } from "lucide-react";
import { Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { apiError } from "../api";
import { Button, Callout, Empty, cx } from "../components/ui";
import { baseName, bytes, git, highlight, langName, relTime, tokClass, useLoad, type BlameLine } from "./repo";

// A file's tab (plan.md 3.15, board 5): read-only text with line numbers, syntax colours, gutter
// marks for the uncommitted lines (else the lines of the last commit that touched the file), and
// blame for the line you pick. Long lines scroll sideways here, and only here.

const LH = 20; // px per line
const OVERSCAN = 30; // lines drawn above and below the visible ones
const GUTTER = 50; // mark, then the line number

export default function FileView({ folder, path, onOpenHistory }: { folder: string; path: string; onOpenHistory?: () => void }) {
  const key = `${folder}\n${path}`;
  const file = useLoad(`file:${key}`, () => git.file(folder, path));
  const marks = useLoad(`marks:${key}`, () => git.marks(folder, path));
  const [showLarge, setShowLarge] = useState(false);
  const [blameOn, setBlameOn] = useState(false);
  const [cur, setCur] = useState<number | null>(null);
  const [blame, setBlame] = useState<{ line: number; data?: BlameLine; error?: string } | null>(null);
  const [openError, setOpenError] = useState("");
  const scroller = useRef<HTMLDivElement>(null);
  const [view, setView] = useState({ top: 0, height: 800 });

  useEffect(() => {
    setShowLarge(false);
    setCur(null);
    setBlame(null);
    setOpenError("");
    scroller.current?.scrollTo(0, 0);
  }, [key]);

  const data = file.data;
  const readable = !!data && !data.binary && (!data.truncated || showLarge);
  const lines = useMemo(() => (readable ? highlight(data.text, data.lang) : []), [readable, data]);
  const cols = useMemo(() => (readable ? longestLine(data.text) : 0), [readable, data]);
  const gutter = useMemo(() => {
    const kinds = new Map<number, string>();
    const dels = new Set<number>();
    for (const m of marks.data?.lines ?? []) {
      if (m.kind === "del") dels.add(m.line);
      else kinds.set(m.line, m.kind);
    }
    return { kinds, dels };
  }, [marks.data]);

  // Blame follows the picked line while it is on, a moment after the line settles.
  useEffect(() => {
    if (!blameOn || cur == null) return;
    let live = true;
    const t = setTimeout(() => {
      setBlame({ line: cur });
      git.blame(folder, path, cur).then(
        (data) => live && setBlame({ line: cur, data }),
        (e) => live && setBlame({ line: cur, error: apiError(e).message }),
      );
    }, 120);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [blameOn, cur, folder, path]);

  const measure = useCallback(() => {
    const el = scroller.current;
    if (el) setView((v) => (v.top === el.scrollTop && v.height === el.clientHeight ? v : { top: el.scrollTop, height: el.clientHeight }));
  }, []);
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [measure, readable]);

  const pick = (line: number) => {
    setCur(line);
    const el = scroller.current;
    if (!el) return;
    const y = (line - 1) * LH + 10;
    if (y < el.scrollTop) el.scrollTop = y;
    else if (y + LH > el.scrollTop + el.clientHeight) el.scrollTop = y + LH - el.clientHeight;
  };
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const n = lines.length;
    if (!n) return;
    if (e.key === "ArrowDown") pick(Math.min(n, (cur ?? 0) + 1));
    else if (e.key === "ArrowUp") pick(Math.max(1, (cur ?? 2) - 1));
    else if (e.key === "Escape") setCur(null);
    else return;
    e.preventDefault();
  };
  const openInEditor = () => {
    setOpenError("");
    git.open(folder, path).catch((e) => setOpenError(apiError(e).message));
  };

  const parts = path.split("/");
  const first = Math.max(0, Math.floor(view.top / LH) - OVERSCAN);
  const last = Math.min(lines.length, Math.ceil((view.top + view.height) / LH) + OVERSCAN);
  const m = marks.data;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-[42px] flex-wrap items-center gap-x-2.5 gap-y-1.5 border-b border-chrome-line bg-chrome py-1.5 pr-3.5 pl-[18px]">
        <nav aria-label="Path" className="flex min-w-0 items-center gap-1.5 text-[12.5px] text-ink-muted">
          <span className="shrink-0">{baseName(folder)}</span>
          {parts.map((p, i) => (
            <Fragment key={i}>
              <span aria-hidden className="text-ink-faint">
                /
              </span>
              <span className={cx(i === parts.length - 1 ? "font-medium text-ink" : "truncate")} aria-current={i === parts.length - 1 ? "page" : undefined}>
                {p}
              </span>
            </Fragment>
          ))}
        </nav>
        {data && (
          <span className="text-xs whitespace-nowrap text-ink-subtle">
            {data.binary ? `Binary · ${bytes(data.size)}` : `${langName(data.lang)} · ${data.lines.toLocaleString("en-US")} ${data.lines === 1 ? "line" : "lines"}${data.truncated ? " shown" : ""}`}
          </span>
        )}
        <span className="flex-1" />
        {m?.source === "commit" && (
          <span title={`${m.short} ${m.subject}`} className="inline-flex h-[26px] max-w-full items-center gap-1.5 rounded-full bg-success/12 px-2.5 text-xs whitespace-nowrap text-success">
            <span className="size-1.5 shrink-0 rounded-full bg-success" />
            <span className="truncate">
              Marking changes from <span className="font-mono">{m.short}</span> · {relTime(m.when)}
            </span>
          </span>
        )}
        {m?.source === "uncommitted" && (
          <span className="inline-flex h-[26px] items-center gap-1.5 rounded-full bg-warning/12 px-2.5 text-xs whitespace-nowrap text-warning">
            <span className="size-1.5 rounded-full bg-warning" />
            Uncommitted changes
          </span>
        )}
        <Button size="sm" icon={UserRoundSearch} aria-pressed={blameOn} className={cx(blameOn && "!border-accent/60 !bg-accent/15")} onClick={() => setBlameOn((v) => !v)} disabled={!readable} title="Click a line to see who last changed it">
          Blame
        </Button>
        {onOpenHistory && (
          <Button size="sm" icon={HistoryIcon} onClick={onOpenHistory}>
            History
          </Button>
        )}
        <Button size="sm" icon={ExternalLink} onClick={openInEditor}>
          Open in editor
        </Button>
      </div>
      {openError && (
        <Callout tone="danger" className="m-3 mb-0">
          Couldn't open the file: {openError}
        </Callout>
      )}

      {file.loading && !data ? (
        <p className="flex items-center gap-2 p-5 text-[13px] text-ink-subtle">
          <LoaderCircle size={14} className="animate-spin" aria-hidden />
          Reading {baseName(path)}…
        </p>
      ) : file.error && !data ? (
        <Empty icon={FileWarning} title="Can't show this file" className="m-5" action={onOpenHistory && <Button size="sm" icon={HistoryIcon} onClick={onOpenHistory}>Open history</Button>}>
          {file.error.message}
        </Empty>
      ) : data?.binary ? (
        <Empty icon={FileWarning} title="A binary file" className="m-5" action={<Button size="sm" icon={ExternalLink} onClick={openInEditor}>Open in its app</Button>}>
          {baseName(path)} is {bytes(data.size)} of binary data; Gwen shows text files.
        </Empty>
      ) : data?.truncated && !showLarge ? (
        <Empty
          icon={FileWarning}
          title="A large file"
          className="m-5"
          action={
            <>
              <Button size="sm" icon={ExternalLink} onClick={openInEditor}>
                Open in editor
              </Button>
              <Button size="sm" tone="ghost" onClick={() => setShowLarge(true)}>
                Show the first {data.lines.toLocaleString("en-US")} lines
              </Button>
            </>
          }
        >
          {baseName(path)} is {bytes(data.size)}, more than the 1 MB Gwen reads at once.
        </Empty>
      ) : data && lines.length === 0 ? (
        <Empty title="An empty file" className="m-5" />
      ) : (
        <div className="relative flex min-h-0 flex-1 bg-canvas">
          <div
            ref={scroller}
            role="region"
            tabIndex={0}
            aria-label={`${path}${cur ? `, line ${cur}` : ""}`}
            onScroll={measure}
            onKeyDown={onKey}
            className="min-w-0 flex-1 overflow-auto font-mono text-[12.5px] text-ink/85 [tab-size:4] focus-visible:outline-offset-[-2px]"
          >
            <div className="relative" style={{ height: lines.length * LH + 22, width: `calc(${cols}ch + ${GUTTER + 48}px)`, minWidth: "100%" }}>
              <div className="absolute inset-x-0 top-2.5" style={{ transform: `translateY(${first * LH}px)` }}>
                {lines.slice(first, last).map((toks, k) => {
                  const n = first + k + 1;
                  const kind = gutter.kinds.get(n);
                  const isCur = n === cur;
                  return (
                    <div key={n} onClick={() => pick(n)} className={cx("relative flex items-center", isCur && "bg-surface-1")} style={{ height: LH }}>
                      <span className={cx("sticky left-0 z-10 flex h-full shrink-0 items-center", isCur ? "bg-surface-1" : "bg-canvas")} style={{ width: GUTTER }}>
                        <span className={cx("mr-[3px] h-full w-[3px] shrink-0", kind === "add" ? "bg-success" : kind === "mod" ? "bg-warning" : "")} title={kind === "add" ? "Added" : kind === "mod" ? "Changed" : undefined} />
                        <span className={cx("flex-1 pr-[14px] text-right tabular-nums select-none", isCur ? "text-ink" : "text-ink-faint")}>{n}</span>
                        {(gutter.dels.has(n) || (n === 1 && gutter.dels.has(0))) && (
                          <span
                            title="Lines deleted here"
                            className={cx("absolute left-0 z-20 size-0 border-y-4 border-l-[6px] border-y-transparent border-l-danger", n === 1 && gutter.dels.has(0) && !gutter.dels.has(1) ? "-top-1" : "-bottom-1")}
                          />
                        )}
                      </span>
                      <span className="pl-1 whitespace-pre">
                        {toks.map((t, i) =>
                          t.c ? (
                            <span key={i} className={tokClass[t.c]}>
                              {t.t}
                            </span>
                          ) : (
                            t.t
                          ),
                        )}
                        {isCur && blameOn && <BlameNote blame={blame?.line === n ? blame : null} />}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
          <Overview lines={lines.length} marks={gutter} view={view} scroller={scroller} />
        </div>
      )}
    </div>
  );
}

function BlameNote({ blame }: { blame: { data?: BlameLine; error?: string } | null }) {
  const b = blame?.data;
  const text = !blame
    ? "Looking…"
    : blame.error
      ? blame.error
      : b?.uncommitted
        ? "Not committed yet"
        : b
          ? `${b.short} · ${b.author} · ${relTime(b.when)} · ${b.summary}`
          : "Looking…";
  return (
    <span aria-live="polite" className="pl-7 font-sans text-xs text-ink-faint italic">
      {b && !b.uncommitted && <GitCommitHorizontal size={12} className="mr-1 inline align-[-2px]" aria-hidden />}
      {text}
    </span>
  );
}

/** The strip at the right edge: where the marked lines are in the whole file, and what is in view. */
function Overview({
  lines,
  marks,
  view,
  scroller,
}: {
  lines: number;
  marks: { kinds: Map<number, string>; dels: Set<number> };
  view: { top: number; height: number };
  scroller: RefObject<HTMLDivElement | null>;
}) {
  const runs = useMemo(() => {
    const out: { from: number; to: number; kind: string }[] = [];
    for (const [n, kind] of [...marks.kinds].sort((a, b) => a[0] - b[0])) {
      const last = out[out.length - 1];
      if (last && last.kind === kind && last.to === n - 1) last.to = n;
      else out.push({ from: n, to: n, kind });
    }
    for (const n of marks.dels) out.push({ from: n + 0.5, to: n + 0.5, kind: "del" });
    return out;
  }, [marks]);
  if (lines < 2) return null;
  const total = lines * LH;
  const pct = (n: number) => `${Math.min(100, Math.max(0, ((n - 1) / lines) * 100))}%`;
  return (
    <div
      aria-hidden
      onClick={(e) => {
        const el = scroller.current;
        if (!el) return;
        const r = e.currentTarget.getBoundingClientRect();
        el.scrollTop = ((e.clientY - r.top) / r.height) * el.scrollHeight - el.clientHeight / 2;
      }}
      className="relative w-3 shrink-0 cursor-pointer border-l border-chrome-line bg-chrome-deep"
    >
      {view.height < total && (
        <span
          className="absolute inset-x-px rounded-[2px] bg-ink/6 shadow-[inset_0_0_0_1px_rgb(242_241_237/0.1)]"
          style={{ top: `${(view.top / (total + 22)) * 100}%`, height: `${Math.max(2, (view.height / (total + 22)) * 100)}%` }}
        />
      )}
      {runs.map((r, i) => (
        <span
          key={i}
          className={cx("absolute inset-x-[3px] rounded-[1px]", r.kind === "add" ? "bg-success" : r.kind === "mod" ? "bg-warning" : "bg-danger")}
          style={{ top: pct(r.from), height: `max(3px, ${((r.to - r.from + 1) / lines) * 100}%)` }}
        />
      ))}
    </div>
  );
}

/** The widest line in characters, tabs counted as four. */
function longestLine(text: string): number {
  let max = 0;
  let start = 0;
  while (start <= text.length) {
    let end = text.indexOf("\n", start);
    if (end < 0) end = text.length;
    let w = end - start;
    if (w > max) {
      const line = text.slice(start, end);
      w = line.length + (line.split("\t").length - 1) * 3;
      max = Math.max(max, w);
    }
    start = end + 1;
  }
  return max;
}
