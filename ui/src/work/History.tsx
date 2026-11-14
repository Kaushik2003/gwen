import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  ChevronUp,
  Copy,
  ExternalLink,
  FileCode2,
  Folder,
  GitBranch,
  GitCommitHorizontal,
  History as HistoryIcon,
  LoaderCircle,
  RefreshCw,
  Search,
  Sparkles,
  Tag,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent, type ReactNode } from "react";
import { App, apiError, wire, type ApiError } from "../api";
import { faceUrl } from "../components/Face";
import { Button, Callout, Empty, IconButton, Segmented, cx } from "../components/ui";
import { ClaudeMark } from "../shell/bits";
import { ClipboardSetText } from "../wailsjs/runtime/runtime";
import {
  baseName,
  count,
  dirName,
  git,
  graphRows,
  laneColor,
  longTime,
  notRepo,
  parseDiff,
  relTime,
  splitRows,
  statusLook,
  uncommittedColor,
  useLoad,
  type Commit,
  type CommitDetail,
  type DiffLine,
  type FileChange,
  type GraphRow,
  type Log,
  type Ref,
} from "./repo";

// A project's history (plan.md 3.15, board 6), three columns: the commits (graph lanes, refs,
// author, time, +/−, the uncommitted work first, filters), the picked commit (message, author,
// the Claude session that made it with Gwen's summary, parents, files as a tree or a list), and
// the picked file's diff, which takes the most room and wraps long lines rather than cut them.

const PAGE = 200;
const ROW_H = 54;
const NODE_Y = 19;
const LANE_W = 12;
const MAX_LANES = 12;

type FileMode = "tree" | "list";

interface LogState {
  key: string;
  commits: Commit[];
  more: boolean;
  first: Log | null; // the first page: branch, upstream, uncommitted
  error: ApiError | null;
  loading: boolean;
  loadingMore: boolean;
}

export default function History({
  folder,
  onOpenFile,
  onOpenSession,
}: {
  folder: string;
  onOpenFile?: (path: string) => void;
  /** Opens the session's tab when it runs, else resumes the conversation. */
  onOpenSession?: (link: wire.CommitSession) => Promise<void>;
}) {
  const [searchText, setSearchText] = useState("");
  const [search, setSearch] = useState("");
  const [author, setAuthor] = useState("");
  const [path, setPath] = useState("");
  const [sel, setSel] = useState<string | null>(null); // a commit, "" for the uncommitted work
  const [pick, setPick] = useState<{ sel: string; path: string } | null>(null);
  const [mode, setMode] = useState<FileMode>("tree");
  const [authors, setAuthors] = useState<string[]>([]);
  const key = JSON.stringify([folder, search, author, path]);
  const [log, setLog] = useState<LogState>({ key, commits: [], more: false, first: null, error: null, loading: true, loadingMore: false });
  const seq = useRef(0);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const t = setTimeout(() => setSearch(searchText.trim()), 300);
    return () => clearTimeout(t);
  }, [searchText]);
  useEffect(() => {
    setSel(null);
    setAuthors([]);
  }, [folder]);

  const fetchLog = useCallback(
    (skip: number, limit: number, quiet: boolean) => {
      const n = ++seq.current;
      setLog((s) => (skip > 0 ? { ...s, loadingMore: true } : quiet && s.key === key ? s : { key, commits: [], more: false, first: null, error: null, loading: true, loadingMore: false }));
      git.log(folder, { limit, skip, search, author, path }).then(
        (l) => {
          if (n !== seq.current) return;
          setLog((s) => ({
            key,
            commits: skip > 0 ? [...s.commits, ...l.commits] : l.commits,
            more: l.more,
            first: skip > 0 ? s.first : l,
            error: null,
            loading: false,
            loadingMore: false,
          }));
          if (!author) setAuthors((prev) => mergeAuthors(prev, l.commits));
        },
        (e) => n === seq.current && setLog((s) => ({ ...s, key, error: apiError(e), loading: false, loadingMore: false })),
      );
    },
    [folder, search, author, path, key],
  );
  useEffect(() => fetchLog(0, PAGE, false), [fetchLog]);
  const links = useCommitSessions(folder);
  const loaded = log.key === key ? log.commits.length : 0;
  useEffect(() => {
    const onFocus = () => fetchLog(0, Math.min(1000, Math.max(PAGE, loaded)), true);
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [fetchLog, loaded]);

  const current = log.key === key ? log : null;
  const commits = current?.commits ?? [];
  const uncommitted = current?.first?.uncommitted;
  const filtered = !!(search || author);
  const headCommit = commits.find((c) => c.refs.some((r) => r.kind === "head"));
  const rows = useMemo(() => (filtered ? null : graphRows(commits, uncommitted && headCommit ? headCommit.hash : undefined)), [commits, filtered, uncommitted, headCommit]);
  const lanes = Math.min(MAX_LANES, Math.max(1, ...(rows ?? []).map((r) => r.width)));

  // Pick the uncommitted work, else the newest commit, when nothing (or something gone) is picked.
  useEffect(() => {
    if (!current || current.loading) return;
    const ids = uncommitted ? ["", ...commits.map((c) => c.hash)] : commits.map((c) => c.hash);
    if (sel === null || (sel === "" && !uncommitted)) setSel(ids[0] ?? null);
  }, [current, commits, uncommitted, sel]);

  const detail = useLoad(`commit:${folder}\n${sel ?? "-"}`, () => (sel === null ? Promise.resolve(null) : git.commit(folder, sel)), sel === "");
  const changes = useMemo(() => ordered(detail.data?.changes ?? [], mode), [detail.data, mode]);
  const file = (sel !== null && pick?.sel === sel && changes.find((c) => c.path === pick.path)) || changes[0] || null;
  const pickFile = (p: string) => sel !== null && setPick({ sel, path: p });

  const onListKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const ids = uncommitted ? ["", ...commits.map((c) => c.hash)] : commits.map((c) => c.hash);
    const i = ids.indexOf(sel ?? "\0");
    const next = ids[Math.max(0, Math.min(ids.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))];
    if (next === undefined) return;
    setSel(next);
    listRef.current?.querySelector<HTMLButtonElement>(`[data-hash="${next}"]`)?.focus();
  };

  const first = current?.first;
  // The list is one tab stop: the picked row, or the first when the pick is not in the list.
  const tabStop = sel === "" && uncommitted ? "" : commits.some((c) => c.hash === sel) ? sel : uncommitted ? "" : commits[0]?.hash;
  if (current?.error && notRepo(current.error))
    return (
      <Empty icon={HistoryIcon} title="No git history here" className="m-6">
        This project's folder isn't a git repository, so it has no commits to show.
      </Empty>
    );

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-12 flex-wrap items-center gap-2 border-b border-chrome-line bg-chrome px-3.5 py-2">
        {first && (
          <span className="inline-flex h-[30px] items-center gap-[7px] rounded-[7px] border border-line-strong bg-surface-1 px-2.5" title={first.upstream ? `Tracking ${first.upstream}` : undefined}>
            <GitBranch size={13} aria-hidden className="text-ink-subtle" />
            <span className="font-mono text-xs">{first.branch || "detached HEAD"}</span>
          </span>
        )}
        {!!first?.ahead && (
          <span className="inline-flex h-6 items-center gap-[5px] rounded-full bg-surface-2 px-2.5 text-xs text-ink-muted" title={`Ahead of ${first.upstream}`}>
            <ArrowUp size={12} aria-hidden />
            {first.ahead} to push
          </span>
        )}
        {!!first?.behind && (
          <span className="inline-flex h-6 items-center gap-[5px] rounded-full bg-surface-2 px-2.5 text-xs text-ink-muted" title={`Behind ${first.upstream}`}>
            <ArrowDown size={12} aria-hidden />
            {first.behind} to pull
          </span>
        )}
        <label className="flex h-[30px] min-w-[180px] flex-[1_1_240px] items-center gap-2 rounded-lg border border-line bg-canvas px-2.5 focus-within:border-accent">
          <Search size={14} className="shrink-0 text-ink-subtle" aria-hidden />
          <span className="sr-only">Search commit messages</span>
          <input
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setSearchText("")}
            placeholder="Search commit messages"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-ink-faint"
          />
          {searchText && (
            <button type="button" aria-label="Clear the search" onClick={() => setSearchText("")} className="text-ink-subtle hover:text-ink">
              <X size={13} aria-hidden />
            </button>
          )}
        </label>
        <label className="contents">
          <span className="sr-only">Author</span>
          <select value={author} onChange={(e) => setAuthor(e.target.value)} className="field !h-[30px] max-w-44 !text-[13px]">
            <option value="">Everyone</option>
            {[...new Set([...authors, ...(author ? [author] : [])])].map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
        <PathFilter folder={folder} value={path} onChange={setPath} />
        <IconButton icon={RefreshCw} size="sm" label="Refresh" onClick={() => fetchLog(0, Math.min(1000, Math.max(PAGE, loaded)), true)} />
      </div>

      <div className="flex min-h-0 flex-1">
        <section aria-label="Commits" className="flex w-[30%] max-w-[460px] min-w-[270px] flex-col border-r border-chrome-line">
          <div className="flex h-9 shrink-0 items-center gap-2 border-b border-chrome-line px-3.5 text-xs text-ink-subtle">
            <b className="text-[12.5px] font-semibold text-ink">Commits</b>
            {first?.branch && !filtered && !path && <span className="truncate">on {first.branch} and every branch</span>}
            {(filtered || path) && <span className="truncate">matching the filters</span>}
            <span className="ml-auto shrink-0">newest first</span>
          </div>
          <div ref={listRef} role="listbox" aria-label="Commits" onKeyDown={onListKey} className="min-h-0 flex-1 overflow-y-auto">
            {!current || (current.loading && !commits.length) ? (
              <p className="flex items-center gap-2 p-4 text-[13px] text-ink-subtle">
                <LoaderCircle size={14} className="animate-spin" aria-hidden />
                Reading the history…
              </p>
            ) : current.error && !commits.length ? (
              <Callout tone="danger" className="m-3" action={<IconButton icon={RefreshCw} size="sm" label="Try again" onClick={() => fetchLog(0, PAGE, false)} />}>
                {current.error.message}
              </Callout>
            ) : (
              <>
                {uncommitted && (
                  <CommitRow
                    hash=""
                    selected={sel === ""}
                    tabbable={tabStop === ""}
                    onSelect={setSel}
                    graph={<UncommittedNode lanes={lanes} joins={!!rows && !!headCommit} />}
                    subject={<span className="truncate font-semibold text-[#f2d9a6]">Uncommitted changes</span>}
                    meta={
                      <>
                        {uncommitted.files} {uncommitted.files === 1 ? "file" : "files"} · <Counts added={uncommitted.added} removed={uncommitted.removed} />
                      </>
                    }
                  />
                )}
                {commits.map((c, i) => (
                  <CommitRow
                    key={c.hash}
                    hash={c.hash}
                    selected={sel === c.hash}
                    tabbable={tabStop === c.hash}
                    onSelect={setSel}
                    graph={rows ? <GraphCell row={rows[i]} lanes={lanes} filled={c === headCommit} /> : <PlainNode />}
                    subject={
                      <>
                        <RefChips refs={c.refs} />
                        {links.of(c.hash) && <ClaudeMark size={11} className="opacity-80" />}
                        <span className={cx("truncate", sel === c.hash && "font-medium text-ink")}>{c.subject}</span>
                      </>
                    }
                    meta={
                      <>
                        <span className="truncate">{c.author}</span> · <span className="shrink-0">{relTime(c.when)}</span> ·{" "}
                        {c.parents.length > 1 && !c.files ? <span>merge</span> : <Counts added={c.added} removed={c.removed} />}
                      </>
                    }
                  />
                ))}
                {!uncommitted && !commits.length && (
                  <Empty title={filtered || path ? "No commits match" : "No commits yet"} className="m-3 py-8">
                    {filtered || path ? "Try other words, someone else, or any file." : "Commit some work and it shows here."}
                  </Empty>
                )}
                {current.more && (
                  <div className="p-3">
                    <Button size="sm" tone="ghost" className="w-full" busy={current.loadingMore} onClick={() => fetchLog(commits.length, PAGE, true)}>
                      Load more
                    </Button>
                  </div>
                )}
              </>
            )}
          </div>
        </section>

        <section aria-label="Commit" className="flex w-[25%] max-w-[400px] min-w-[250px] flex-col border-r border-chrome-line bg-chrome-deep">
          <CommitColumn
            sel={sel}
            detail={detail.data ?? null}
            error={detail.error}
            loading={detail.loading}
            changes={changes}
            file={file}
            mode={mode}
            setMode={setMode}
            onPick={pickFile}
            onSelect={setSel}
            made={sel ? links.of(sel) : undefined}
            folder={folder}
            onOpenSession={onOpenSession}
            onSummary={links.setSummary}
          />
        </section>

        <section aria-label={file ? `Changes in ${file.path}` : "Diff"} className="flex min-w-0 flex-1 flex-col bg-canvas">
          {sel !== null && file ? (
            <DiffColumn
              key={`${sel}\n${file.path}`}
              folder={folder}
              hash={sel}
              file={file}
              onOpenFile={onOpenFile}
              onStep={(d) => {
                const i = changes.indexOf(file);
                const next = changes[i + d];
                if (next) pickFile(next.path);
              }}
              canStep={[changes.indexOf(file) > 0, changes.indexOf(file) < changes.length - 1]}
            />
          ) : (
            <Empty title={detail.loading && sel !== null ? "Loading…" : "No file picked"} className="m-6">
              {detail.data && !changes.length ? "This commit changes no files." : "Pick a file of the commit to see what changed."}
            </Empty>
          )}
        </section>
      </div>
    </div>
  );
}

/**
 * A message body as paragraphs: git messages are wrapped at about 72 columns, which reads badly
 * in a narrow column, so single line breaks inside a paragraph become spaces. Lists, indented
 * lines, and blank lines keep their breaks.
 */
function reflow(body: string): string {
  return body.replace(/([^\n])\n(?=[^\n\s*\-+>#|\d])/g, "$1 ");
}

function mergeAuthors(prev: string[], commits: Commit[]): string[] {
  const set = new Set(prev);
  for (const c of commits) set.add(c.author);
  return set.size === prev.length ? prev : [...set].sort((a, b) => a.localeCompare(b));
}

/** The files in the order they are shown: grouped by folder in the tree, by path in the list. */
function ordered(changes: FileChange[], mode: FileMode): FileChange[] {
  const out = [...changes];
  if (mode === "list") return out.sort((a, b) => a.path.localeCompare(b.path));
  return out.sort((a, b) => dirName(a.path).localeCompare(dirName(b.path)) || baseName(a.path).localeCompare(baseName(b.path)));
}

// ---------------------------------------------------------------------------------------------
// The commit list.

function CommitRow({ hash, selected, tabbable, onSelect, graph, subject, meta }: { hash: string; selected: boolean; tabbable: boolean; onSelect: (h: string) => void; graph: ReactNode; subject: ReactNode; meta: ReactNode }) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      tabIndex={tabbable ? 0 : -1}
      data-hash={hash}
      onClick={() => onSelect(hash)}
      className={cx(
        "flex w-full items-stretch gap-1.5 border-b border-chrome-line pr-3 pl-1.5 text-left text-ink/85",
        selected ? "bg-surface-2 shadow-[inset_0_0_0_1px_var(--color-line-strong)]" : "hover:bg-surface-1",
      )}
      style={{ height: ROW_H }}
    >
      {graph}
      <span className="flex min-w-0 flex-1 flex-col justify-center gap-[3px]">
        <span className="flex items-center gap-1.5 overflow-hidden text-[13px] whitespace-nowrap">{subject}</span>
        <span className="flex items-center gap-1.5 overflow-hidden text-[11.5px] whitespace-nowrap text-ink-subtle">{meta}</span>
      </span>
    </button>
  );
}

function Counts({ added, removed }: { added: number; removed: number }) {
  if (!added && !removed) return <span>no line changes</span>;
  return (
    <span className="shrink-0 font-mono text-[11px]">
      {added > 0 && <span className="text-success">+{count(added)}</span>}
      {added > 0 && removed > 0 && " "}
      {removed > 0 && <span className="text-danger">−{count(removed)}</span>}
    </span>
  );
}

function RefChips({ refs }: { refs: Ref[] }) {
  const chips: { label: string; kind: string; title: string }[] = [];
  for (let i = 0; i < refs.length; i++) {
    const r = refs[i];
    if (r.kind === "head" && refs[i + 1]?.kind === "branch") {
      chips.push({ label: refs[i + 1].name, kind: "head", title: `HEAD: ${refs[i + 1].name} is checked out` });
      i++;
    } else chips.push({ label: r.name, kind: r.kind, title: r.kind === "head" ? "HEAD, detached" : r.kind === "remote" ? `Remote branch ${r.name}` : r.kind === "tag" ? `Tag ${r.name}` : `Branch ${r.name}` });
  }
  const look: Record<string, string> = {
    head: "bg-[#3ec1af]/16 text-[#8fe0d3]",
    branch: "bg-surface-3 text-ink-muted",
    remote: "bg-surface-3 text-ink-subtle",
    tag: "bg-warning/14 text-warning",
  };
  return (
    <>
      {chips.slice(0, 3).map((c) => (
        <span key={c.kind + c.label} title={c.title} className={cx("inline-flex h-[18px] max-w-36 shrink-0 items-center gap-1 rounded-full px-1.5 font-mono text-[10.5px]", look[c.kind])}>
          {c.kind === "tag" && <Tag size={9} aria-hidden />}
          <span className="truncate">{c.label}</span>
        </span>
      ))}
      {chips.length > 3 && (
        <span className="shrink-0 font-mono text-[10.5px] text-ink-subtle" title={chips.slice(3).map((c) => c.label).join(", ")}>
          +{chips.length - 3}
        </span>
      )}
    </>
  );
}

const laneX = (l: number) => 9 + l * LANE_W;
const svgWidth = (lanes: number) => laneX(lanes - 1) + 9;

function GraphCell({ row, lanes, filled }: { row: GraphRow; lanes: number; filled: boolean }) {
  const x = laneX(row.lane);
  const color = laneColor(row.color);
  return (
    <svg width={svgWidth(lanes)} height={ROW_H} viewBox={`0 0 ${svgWidth(lanes)} ${ROW_H}`} fill="none" aria-hidden className="shrink-0 overflow-hidden">
      {row.lines.map((l, i) => {
        const x1 = laneX(l.from);
        const x2 = laneX(l.to);
        const d =
          l.part === "through"
            ? `M${x1} 0V${ROW_H}`
            : l.part === "top"
              ? x1 === x2
                ? `M${x1} 0V${NODE_Y}`
                : `M${x1} 0V2C${x1} 10 ${x2} 13 ${x2} ${NODE_Y}`
              : x1 === x2
                ? `M${x1} ${NODE_Y}V${ROW_H}`
                : `M${x1} ${NODE_Y}C${x2} ${NODE_Y + 3} ${x2} ${NODE_Y + 10} ${x2} ${NODE_Y + 17}V${ROW_H}`;
        return <path key={i} d={d} stroke={laneColor(l.color)} strokeWidth={2} strokeOpacity={l.part === "through" ? 0.6 : 1} strokeDasharray={l.dashed ? "3 3" : undefined} />;
      })}
      {filled ? (
        <circle cx={x} cy={NODE_Y} r={5.5} fill={color} />
      ) : (
        <>
          <circle cx={x} cy={NODE_Y} r={4.5} fill="var(--color-canvas)" stroke={color} strokeWidth={2} />
          {row.merge && <circle cx={x} cy={NODE_Y} r={1.6} fill={color} />}
        </>
      )}
    </svg>
  );
}

function UncommittedNode({ lanes, joins }: { lanes: number; joins: boolean }) {
  return (
    <svg width={svgWidth(lanes)} height={ROW_H} fill="none" aria-hidden className="shrink-0">
      {joins && <path d={`M9 ${NODE_Y + 6}V${ROW_H}`} stroke={uncommittedColor} strokeWidth={2} strokeDasharray="3 3" />}
      <circle cx={9} cy={NODE_Y} r={5} stroke={uncommittedColor} strokeWidth={2} strokeDasharray="2.6 2" />
    </svg>
  );
}

function PlainNode() {
  return (
    <svg width={18} height={ROW_H} fill="none" aria-hidden className="shrink-0">
      <circle cx={9} cy={NODE_Y} r={3.5} fill="var(--color-ink-faint)" />
    </svg>
  );
}

/** The file filter: a path of the folder, with suggestions from its files once focused. */
function PathFilter({ folder, value, onChange }: { folder: string; value: string; onChange: (p: string) => void }) {
  const [text, setText] = useState(value);
  const [files, setFiles] = useState<string[] | null>(null);
  const id = useMemo(() => `paths-${Math.random().toString(36).slice(2)}`, []);
  useEffect(() => setText(value), [value]);
  useEffect(() => setFiles(null), [folder]);
  const suggestions = useMemo(() => {
    if (!files) return [];
    const t = text.trim().toLowerCase();
    return files.filter((f) => !t || f.toLowerCase().includes(t)).slice(0, 50);
  }, [files, text]);
  const apply = (p: string) => onChange(p.trim().replace(/^\.\/+/, "").replace(/\/+$/, ""));
  return (
    <label className="flex h-[30px] w-52 min-w-0 items-center gap-1.5 rounded-lg border border-line-strong bg-surface-2 px-2.5 focus-within:border-accent">
      <FileCode2 size={13} className="shrink-0 text-ink-subtle" aria-hidden />
      <span className="sr-only">Only commits that touch this file or folder</span>
      <input
        value={text}
        list={id}
        onFocus={() => files === null && git.tree(folder).then((t) => setFiles(t.files.map((f) => f.path)), () => setFiles([]))}
        onChange={(e) => {
          setText(e.target.value);
          if (files?.includes(e.target.value)) apply(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") apply(text);
          else if (e.key === "Escape") {
            setText("");
            apply("");
          }
        }}
        onBlur={() => text !== value && apply(text)}
        placeholder="Any file"
        spellCheck={false}
        className="min-w-0 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-ink"
      />
      <datalist id={id}>
        {suggestions.map((f) => (
          <option key={f} value={f} />
        ))}
      </datalist>
      {value && (
        <button type="button" aria-label="Any file" onClick={() => apply("")} className="text-ink-subtle hover:text-ink">
          <X size={13} aria-hidden />
        </button>
      )}
    </label>
  );
}

// ---------------------------------------------------------------------------------------------
// The commit column.

function CommitColumn({
  sel,
  detail,
  error,
  loading,
  changes,
  file,
  mode,
  setMode,
  onPick,
  onSelect,
  made,
  folder,
  onOpenSession,
  onSummary,
}: {
  sel: string | null;
  detail: CommitDetail | null;
  error: ApiError | null;
  loading: boolean;
  changes: FileChange[];
  file: FileChange | null;
  mode: FileMode;
  setMode: (m: FileMode) => void;
  onPick: (path: string) => void;
  onSelect: (hash: string) => void;
  made?: wire.CommitSession;
  folder: string;
  onOpenSession?: (link: wire.CommitSession) => Promise<void>;
  onSummary: (link: wire.CommitSession, summary: string) => void;
}) {
  const [full, setFull] = useState(false);
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    setFull(false);
    setCopied(false);
  }, [sel]);
  if (sel === null) return <Empty title="No commit picked" className="m-4" />;
  if (!detail) {
    return error ? (
      <Callout tone="danger" className="m-3">
        {error.message}
      </Callout>
    ) : (
      <p className="flex items-center gap-2 p-4 text-[13px] text-ink-subtle">
        <LoaderCircle size={14} className="animate-spin" aria-hidden />
        {loading ? "Reading the commit…" : ""}
      </p>
    );
  }
  const working = sel === "";
  const branch = detail.refs.find((r) => r.kind === "branch")?.name;
  const longBody = detail.body.split("\n").length > 4 || detail.body.length > 360;
  const max = Math.max(1, ...changes.map((c) => c.added + c.removed));
  return (
    <>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-chrome-line px-3.5 text-xs text-ink-subtle">
        <b className="text-[12.5px] font-semibold text-ink">{working ? "Uncommitted" : "Commit"}</b>
        {!working && (
          <span className="ml-auto inline-flex items-center gap-1.5 font-mono text-ink-muted">
            {detail.short}
            <IconButton
              icon={Copy}
              size="sm"
              label={copied ? "Copied" : "Copy the commit id"}
              className="!size-6"
              onClick={() => ClipboardSetText(detail.hash).then(() => setCopied(true), () => setCopied(false))}
            />
          </span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-3 px-3.5 pt-3.5 pb-2.5">
          <h2 className="text-[15px] leading-[1.35] font-semibold break-words">{working ? "Uncommitted changes" : detail.subject}</h2>
          {working ? (
            <p className="text-xs text-ink-muted">
              {branch ? (
                <>
                  On <span className="font-mono">{branch}</span>, not committed yet.
                </>
              ) : (
                "Not committed yet."
              )}{" "}
              Commit them from the Changes panel.
            </p>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
                <span aria-hidden className="inline-flex size-[22px] items-center justify-center rounded-full bg-surface-3 text-[10px] font-semibold text-ink">
                  {(detail.author.trim()[0] ?? "?").toUpperCase()}
                </span>
                <span title={detail.author_email}>{detail.author}</span>
                <span className="text-ink-subtle">· {longTime(detail.when)}</span>
              </div>
              {made && <MadeBy key={`${made.claude_session_id} ${made.commit}`} link={made} folder={folder} subject={detail.subject} onOpen={onOpenSession} onSummary={onSummary} />}
              {detail.parents.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5 text-xs text-ink-subtle">
                  {detail.parents.length > 1 ? "Parents" : "Parent"}
                  {detail.parents.map((p) => (
                    <button key={p} type="button" onClick={() => onSelect(p)} className="rounded-md bg-surface-2 px-1.5 font-mono text-[11.5px] text-ink-muted hover:bg-surface-3 hover:text-ink" title="Show this commit">
                      {p.slice(0, 7)}
                    </button>
                  ))}
                </div>
              )}
              {detail.body && (
                <div className="text-xs leading-normal text-ink-muted">
                  <p className={cx("break-words whitespace-pre-wrap", !full && longBody && "line-clamp-4")}>{reflow(detail.body)}</p>
                  {longBody && (
                    <button type="button" onClick={() => setFull((v) => !v)} className="mt-1 text-ink-muted underline decoration-line-3 underline-offset-2 hover:text-ink">
                      {full ? "Less" : "Full message"}
                    </button>
                  )}
                </div>
              )}
            </>
          )}
        </div>
        <div className="sticky top-0 z-10 flex items-center gap-2 border-y border-chrome-line bg-chrome-deep px-3.5 py-2 text-xs text-ink-subtle">
          <b className="text-[12.5px] font-semibold text-ink">
            {changes.length} {changes.length === 1 ? "file" : "files"}
          </b>
          <Counts added={detail.added} removed={detail.removed} />
          <Segmented
            size="sm"
            className="ml-auto"
            label="Show files as"
            value={mode}
            onChange={setMode}
            options={[
              { value: "tree", label: "Tree" },
              { value: "list", label: "List" },
            ]}
          />
        </div>
        <div className="flex flex-col gap-px px-2 pt-0.5 pb-3">
          {changes.map((c, i) => {
            const dir = dirName(c.path);
            const header = mode === "tree" && dir !== dirName(changes[i - 1]?.path ?? "\0") && (
              <div key={`d:${dir}`} className="flex h-6 items-center gap-1.5 px-2 font-mono text-[11.5px] text-ink-subtle">
                <Folder size={13} strokeWidth={1.8} aria-hidden className="shrink-0" />
                <span className="truncate">{dir || "/"}</span>
              </div>
            );
            return [header, <FileRow key={c.path} change={c} tree={mode === "tree"} selected={file === c} max={max} onPick={onPick} />];
          })}
        </div>
      </div>
    </>
  );
}

function FileRow({ change: c, tree, selected, max, onPick }: { change: FileChange; tree: boolean; selected: boolean; max: number; onPick: (p: string) => void }) {
  const status = c.status === "U" ? "U" : c.status;
  const look = statusLook[status];
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={() => onPick(c.path)}
      title={c.old_path ? `${c.path}, renamed from ${c.old_path}` : c.path}
      className={cx(
        "flex h-7 w-full items-center gap-2 rounded-[7px] pr-2.5 text-left text-[12.5px]",
        tree && dirName(c.path) ? "pl-[26px]" : "pl-2.5",
        selected ? "bg-surface-3 font-medium text-ink shadow-[inset_0_0_0_1px_var(--color-line-strong)]" : "text-ink/85 hover:bg-surface-1",
      )}
    >
      <span className={cx("w-3 shrink-0 text-center font-mono text-[11px] font-medium", look?.text)} title={look?.label}>
        {status}
      </span>
      <span className="min-w-0 flex-1 truncate">
        {tree ? baseName(c.path) : <>{dirName(c.path) && <span className="text-ink-subtle">{dirName(c.path)}/</span>}{baseName(c.path)}</>}
      </span>
      <ChangeBar added={c.added} removed={c.removed} max={max} binary={c.binary} />
      <span className="min-w-[52px] shrink-0 text-right font-mono text-[11px] font-normal whitespace-nowrap">{c.binary ? <span className="text-ink-subtle">bin</span> : <Counts added={c.added} removed={c.removed} />}</span>
    </button>
  );
}

/** Five squares: how big the change is (on a log scale against the commit's biggest), green for added, red for removed. */
function ChangeBar({ added, removed, max, binary }: { added: number; removed: number; max: number; binary: boolean }) {
  const total = added + removed;
  const filled = binary || !total ? 0 : Math.max(1, Math.min(5, Math.ceil((5 * Math.log(total + 1)) / Math.log(max + 1))));
  const green = total ? Math.round((filled * added) / total) : 0;
  return (
    <span aria-hidden className="inline-flex shrink-0 gap-0.5">
      {[0, 1, 2, 3, 4].map((i) => (
        <i key={i} className={cx("block size-1.5 rounded-[1.5px]", i < green ? "bg-success" : i < filled ? "bg-danger" : "bg-line")} />
      ))}
    </span>
  );
}

// ---------------------------------------------------------------------------------------------
// The diff column.

function DiffColumn({
  folder,
  hash,
  file,
  onOpenFile,
  onStep,
  canStep,
}: {
  folder: string;
  hash: string;
  file: FileChange;
  onOpenFile?: (path: string) => void;
  onStep: (d: 1 | -1) => void;
  canStep: [boolean, boolean];
}) {
  const [split, setSplit] = useState<"unified" | "split">(() => (localStorage.getItem("gwen.history.diff") === "split" ? "split" : "unified"));
  const diff = useLoad(`diff:${folder}\n${hash}\n${file.path}`, () => git.diff(folder, hash, file.path), hash === "");
  const parsed = useMemo(() => (diff.data ? parseDiff(diff.data.text) : null), [diff.data]);
  const setMode = (m: "unified" | "split") => {
    setSplit(m);
    try {
      localStorage.setItem("gwen.history.diff", m);
    } catch {
      // storage is a convenience
    }
  };
  const dir = dirName(file.path);
  return (
    <>
      <div className="flex min-h-9 shrink-0 flex-wrap items-center gap-2 border-b border-chrome-line bg-chrome-deep py-1 pr-2.5 pl-3.5">
        <span className="min-w-0 truncate font-mono text-[12.5px] text-ink" title={file.path}>
          {dir && <span className="text-ink-subtle">{dir}/</span>}
          {baseName(file.path)}
        </span>
        {file.old_path && <span className="truncate font-mono text-[11.5px] text-ink-subtle">from {file.old_path}</span>}
        {!file.binary && <Counts added={file.added} removed={file.removed} />}
        <span className="flex-1" />
        <Segmented
          size="sm"
          label="Diff layout"
          value={split}
          onChange={setMode}
          options={[
            { value: "unified", label: "Unified" },
            { value: "split", label: "Split" },
          ]}
        />
        <IconButton icon={ChevronUp} size="sm" label="Previous file" disabled={!canStep[0]} onClick={() => onStep(-1)} className="!size-[26px]" />
        <IconButton icon={ChevronDown} size="sm" label="Next file" disabled={!canStep[1]} onClick={() => onStep(1)} className="!size-[26px]" />
        {onOpenFile && (
          <Button size="sm" icon={ExternalLink} className="!h-[26px]" disabled={file.status === "D"} title={file.status === "D" ? "The file is deleted" : undefined} onClick={() => onOpenFile(file.path)}>
            Open file
          </Button>
        )}
      </div>
      <div role="region" aria-label="Diff" className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto pb-3 [tab-size:4]">
        {diff.loading && !diff.data ? (
          <p className="flex items-center gap-2 p-4 text-[13px] text-ink-subtle">
            <LoaderCircle size={14} className="animate-spin" aria-hidden />
            Reading the diff…
          </p>
        ) : diff.error && !diff.data ? (
          <Callout tone="danger" className="m-3">
            {diff.error.message}
          </Callout>
        ) : !parsed || parsed.binary || file.binary ? (
          <Empty icon={GitCommitHorizontal} title="A binary file" className="m-5">
            Gwen compares text; this file {file.status === "A" || file.status === "U" ? "was added" : file.status === "D" ? "was deleted" : "changed"} as binary data.
          </Empty>
        ) : !parsed.hunks.length ? (
          <Empty title="No line changes" className="m-5">
            {parsed.renamedFrom ? `Renamed from ${parsed.renamedFrom}; the text is the same.` : parsed.modeChange ? `The file mode changed: ${parsed.modeChange}.` : parsed.isNew ? "An empty new file." : "Nothing to compare."}
          </Empty>
        ) : (
          <>
            {diff.data?.truncated && (
              <Callout tone="idle" className="m-3">
                This diff is longer than 512 KB; it shows the beginning.
              </Callout>
            )}
            {parsed.hunks.map((h, i) => (
              <div key={i} className={cx(i > 0 && "mt-2.5")}>
                <div className="flex gap-2 overflow-hidden border-y border-info/10 bg-info/6 px-3.5 py-1.5 font-mono text-[11.5px] whitespace-nowrap text-info/75">
                  {h.header}
                  {h.context && <b className="truncate font-normal text-info">{h.context}</b>}
                </div>
                {chunks(h.lines).map((part, k) => (
                  <div key={k} style={lazy(part.length)}>
                    {split === "split" ? <SplitLines lines={part} /> : part.map((l, j) => <UnifiedLine key={j} line={l} />)}
                  </div>
                ))}
              </div>
            ))}
          </>
        )}
      </div>
    </>
  );
}

const CHUNK = 200;
function chunks(lines: DiffLine[]): DiffLine[][] {
  const out: DiffLine[][] = [];
  for (let i = 0; i < lines.length; i += CHUNK) out.push(lines.slice(i, i + CHUNK));
  return out;
}
/** Off-screen chunks skip layout and paint until scrolled to. */
const lazy = (n: number): CSSProperties => ({ contentVisibility: "auto", containIntrinsicSize: `auto ${n * 20}px` });

const rowLook = {
  ctx: { row: "", sign: "", text: "text-ink/80" },
  add: { row: "bg-success/12", sign: "text-success", text: "text-ink" },
  del: { row: "bg-danger/12", sign: "text-danger", text: "text-ink" },
};
const textCls = "min-w-0 pr-3.5 break-words whitespace-pre-wrap [overflow-wrap:anywhere]";

function UnifiedLine({ line: l }: { line: DiffLine }) {
  const look = rowLook[l.kind];
  return (
    <div className={cx("grid grid-cols-[40px_40px_18px_minmax(0,1fr)] font-mono text-xs leading-5", look.row)}>
      <span className={cx("pr-2 text-right text-ink-faint tabular-nums select-none", l.kind === "del" && "text-danger/80")}>{l.old ?? ""}</span>
      <span className={cx("pr-2 text-right text-ink-faint tabular-nums select-none", l.kind === "add" && "text-success/80")}>{l.new ?? ""}</span>
      <span className={cx("text-center select-none", look.sign || "text-ink-faint")}>{l.kind === "add" ? "+" : l.kind === "del" ? "−" : ""}</span>
      <span className={cx(textCls, look.text)}>
        {l.text}
        {l.noEol && <NoEol />}
      </span>
    </div>
  );
}

function SplitLines({ lines }: { lines: DiffLine[] }) {
  return (
    <>
      {splitRows(lines).map((r, i) => (
        <div key={i} className="grid grid-cols-2 font-mono text-xs leading-5">
          {[r.left, r.right].map((s, k) =>
            s ? (
              <div key={k} className={cx("grid grid-cols-[40px_18px_minmax(0,1fr)]", rowLook[s.kind].row, k === 1 && "border-l border-chrome-line")}>
                <span className={cx("pr-2 text-right text-ink-faint tabular-nums select-none", s.kind === "del" && "text-danger/80", s.kind === "add" && "text-success/80")}>{s.num}</span>
                <span className={cx("text-center select-none", rowLook[s.kind].sign)}>{s.kind === "add" ? "+" : s.kind === "del" ? "−" : ""}</span>
                <span className={cx(textCls, rowLook[s.kind].text)}>{s.text}</span>
              </div>
            ) : (
              <div key={k} className={cx("bg-surface-1/60", k === 1 && "border-l border-chrome-line")} />
            ),
          )}
        </div>
      ))}
    </>
  );
}

function NoEol() {
  return (
    <span className="ml-2 font-sans text-[11px] text-ink-faint italic" title="No newline at end of file">
      no newline at end
    </span>
  );
}

/**
 * The commits Claude conversations made in a folder (plan.md 3.15): `of` finds the one for a
 * commit by its full id, `setSummary` puts Gwen's new summary in place. Read again when the
 * window comes back into focus, since a session may have committed meanwhile.
 */
function useCommitSessions(folder: string) {
  const [links, setLinks] = useState<wire.CommitSession[]>([]);
  useEffect(() => {
    let live = true;
    const load = () =>
      App.CommitSessions(folder).then(
        (r) => live && setLinks(r.commits ?? []),
        () => live && setLinks([]),
      );
    load();
    window.addEventListener("focus", load);
    return () => {
      live = false;
      window.removeEventListener("focus", load);
    };
  }, [folder]);
  const of = useCallback((hash: string) => links.find((l) => l.commit && hash.startsWith(l.commit)), [links]);
  const setSummary = useCallback(
    (link: wire.CommitSession, summary: string) =>
      setLinks((ls) => ls.map((l) => (l.claude_session_id === link.claude_session_id && l.commit === link.commit ? { ...l, summary } : l))),
    [],
  );
  return { of, setSummary };
}

/** The Claude session that made a commit, with Open or Resume, and Gwen's summary or a button to ask for it. */
function MadeBy({
  link,
  folder,
  subject,
  onOpen,
  onSummary,
}: {
  link: wire.CommitSession;
  folder: string;
  subject: string;
  onOpen?: (link: wire.CommitSession) => Promise<void>;
  onSummary: (link: wire.CommitSession, summary: string) => void;
}) {
  const [busy, setBusy] = useState<"" | "open" | "summary">("");
  const [error, setError] = useState("");
  const running = !!link.session_id;
  const name = link.title || link.first_prompt || "a Claude session";
  const about = [link.sandbox && "sandboxed", running && "running now", link.title && link.first_prompt].filter(Boolean).join(" · ");

  async function run(what: "open" | "summary", fn: () => Promise<void>) {
    setBusy(what);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(apiError(e).message);
    } finally {
      setBusy("");
    }
  }
  const summarize = () =>
    run("summary", async () => {
      const r = await App.SummarizeCommit(wire.SummarizeCommit.createFrom({ dir: folder, claude_session_id: link.claude_session_id, commit: link.commit, subject }));
      onSummary(link, r.summary);
    });

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2.5 rounded-[9px] border border-line bg-surface-1 px-2.5 py-2">
        <ClaudeMark size={15} />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12.5px] font-medium" title={name}>
            Made in {name}
          </span>
          <span className="block truncate text-[11.5px] text-ink-subtle" title={about || undefined}>
            Claude session{about ? ` · ${about}` : ""}
          </span>
        </span>
        {onOpen && (
          <Button size="sm" busy={busy === "open"} onClick={() => run("open", () => onOpen(link))} title={running ? "Show its terminal" : "Start Claude again in this conversation"}>
            {running ? "Open" : "Resume"}
          </Button>
        )}
      </div>
      {link.summary ? (
        <div className="flex items-start gap-2.5 rounded-[9px] bg-surface-2 px-2.5 py-2">
          <img src={faceUrl("smiling")} alt="Gwen" className="size-[22px] shrink-0 rounded-full bg-[#F2F1ED] object-cover" />
          <p className="text-[12.5px] leading-[1.45] break-words text-ink">{link.summary}</p>
        </div>
      ) : (
        <Button size="sm" tone="ghost" icon={Sparkles} className="self-start" busy={busy === "summary"} onClick={summarize} title="Gwen reads the conversation and sums up this commit">
          Summarize
        </Button>
      )}
      {error && <p className="text-xs text-danger">{error}</p>}
    </div>
  );
}
