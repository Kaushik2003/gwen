import { ChevronDown, ChevronRight, ChevronsDownUp, File as FileIcon, Filter, Folder, FolderOpen, GitBranch, LoaderCircle, RefreshCw, Search, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { Callout, Empty, IconButton, cx } from "../components/ui";
import { baseName, buildTree, changedDirs, dirName, git, langChip, statusLook, useLoad, type TreeFile, type TreeNode } from "./repo";

// The Files panel's tree (plan.md 3.15, board 5): git's file list with each file's status, folders
// with their changed-file counts, open folders pinned while their contents scroll past.

const ROW = 23; // px; also each pinned folder's offset from the top
const MAX_PINNED = 6; // deeper folders scroll normally rather than eat the panel
const MAX_RESULTS = 200;

export default function FileTree({ folder, selected, onOpen }: { folder: string; selected?: string; onOpen: (path: string) => void }) {
  const tree = useLoad(`tree:${folder}`, () => git.tree(folder));
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [filter, setFilter] = useState("");
  const [changedOnly, setChangedOnly] = useState(false);
  const seeded = useRef("");
  const list = useRef<HTMLDivElement>(null);
  const files = tree.data?.files;

  // Changed folders start open (once per folder), and so do the open file's.
  useEffect(() => {
    if (!files || seeded.current === folder) return;
    seeded.current = folder;
    const open = changedDirs(files);
    for (let d = dirName(selected ?? ""); d; d = dirName(d)) open.add(d);
    setExpanded(open);
  }, [files, folder, selected]);
  useEffect(() => {
    if (!selected) return;
    setExpanded((prev) => {
      const next = new Set(prev);
      for (let d = dirName(selected); d; d = dirName(d)) next.add(d);
      return next.size === prev.size ? prev : next;
    });
  }, [selected]);
  // Bring the open file into view once it is drawn, once per file (not on every refresh).
  const scrolled = useRef("");
  useEffect(() => {
    if (!selected || !files || scrolled.current === folder + "\n" + selected) return;
    const id = requestAnimationFrame(() => {
      const row = list.current?.querySelector(`[data-path="${CSS.escape(selected)}"]`);
      if (!row) return;
      scrolled.current = folder + "\n" + selected;
      row.scrollIntoView({ block: "center" });
    });
    return () => cancelAnimationFrame(id);
  }, [selected, files, folder, expanded]);

  const shown = useMemo(() => (files && changedOnly ? files.filter((f) => f.status) : files), [files, changedOnly]);
  const root = useMemo(() => (shown ? buildTree(shown) : null), [shown]);
  const results = useMemo(() => (files && filter.trim() ? search(files, filter) : null), [files, filter]);
  const openDir = changedOnly ? (p: string) => !expanded.has(`-${p}`) : (p: string) => expanded.has(p);
  const toggle = (p: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      const key = changedOnly ? `-${p}` : p; // in "changed only" every folder is open unless closed
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  const selectedDir = selected ? dirName(selected) : null;

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const rows = Array.from(list.current?.querySelectorAll<HTMLButtonElement>("[data-tree-row]") ?? []);
    const i = rows.indexOf(document.activeElement as HTMLButtonElement);
    if (i < 0) return;
    const row = rows[i];
    const focus = (n: number) => rows[Math.max(0, Math.min(rows.length - 1, n))]?.focus();
    const path = row.dataset.path ?? "";
    if (e.key === "ArrowDown") focus(i + 1);
    else if (e.key === "ArrowUp") focus(i - 1);
    else if (e.key === "Home") focus(0);
    else if (e.key === "End") focus(rows.length - 1);
    else if (e.key === "ArrowRight" && row.dataset.dir === "closed") toggle(path);
    else if (e.key === "ArrowRight" && row.dataset.dir === "open") focus(i + 1);
    else if (e.key === "ArrowLeft" && row.dataset.dir === "open") toggle(path);
    else if (e.key === "ArrowLeft") focus(rows.findIndex((r) => r.dataset.path === dirName(path)));
    else return;
    e.preventDefault();
  };

  const node = (n: TreeNode): ReactNode => {
    const guides = Array.from({ length: n.depth }, (_, k) => {
      const ancestor = n.path.split("/").slice(0, k + 1).join("/");
      return <span key={k} aria-hidden className={cx("absolute inset-y-0 w-px", ancestor === selectedDir ? "bg-line-strong" : "bg-surface-3")} style={{ left: 12 + 14 * k }} />;
    });
    if (!n.dir) return <FileRow key={n.path} file={n.file!} name={n.name} depth={n.depth} guides={guides} selected={n.path === selected} onOpen={onOpen} />;
    const open = openDir(n.path);
    const pinned = open && n.depth < MAX_PINNED;
    return (
      <div key={n.path}>
        <button
          type="button"
          data-tree-row
          data-path={n.path}
          data-dir={open ? "open" : "closed"}
          aria-expanded={open}
          onClick={() => toggle(n.path)}
          className={cx("relative flex w-full items-center gap-[7px] rounded-md pr-2.5 text-left text-[12.5px] whitespace-nowrap text-ink/85 hover:bg-surface-2", pinned && "sticky bg-chrome")}
          style={{ height: ROW, paddingLeft: 6 + 14 * n.depth, ...(pinned ? { top: n.depth * ROW, zIndex: 20 - n.depth } : {}) }}
        >
          {guides}
          {open ? <ChevronDown size={12} strokeWidth={2.2} className="shrink-0 text-ink-subtle" aria-hidden /> : <ChevronRight size={12} strokeWidth={2.2} className="shrink-0 text-ink-subtle" aria-hidden />}
          {open ? <FolderOpen size={15} strokeWidth={1.8} className="shrink-0 text-[#c9b9a6]" aria-hidden /> : <Folder size={15} strokeWidth={1.8} className="shrink-0 text-ink-muted" aria-hidden />}
          <span className={cx("min-w-0 truncate", open && "font-medium")}>{n.name}</span>
          {n.changed > 0 && (
            <span className="ml-auto shrink-0 font-mono text-[11px] font-medium text-warning" title={`${n.changed} changed ${n.changed === 1 ? "file" : "files"} inside`}>
              ●{n.changed}
            </span>
          )}
        </button>
        {open && n.children.map(node)}
      </div>
    );
  };

  const changedCount = files?.reduce((n, f) => n + (f.status ? 1 : 0), 0) ?? 0;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-1 pt-2.5 pr-2.5 pb-1.5 pl-4">
        <span className="min-w-0 truncate font-semibold">{baseName(tree.data?.root ?? folder) || folder}</span>
        {tree.data?.git && (
          <span className="ml-1.5 inline-flex min-w-0 items-center gap-[5px] font-mono text-[11.5px] text-ink-subtle">
            <GitBranch size={12} aria-hidden className="shrink-0" />
            <span className="truncate">{tree.data.branch || "detached HEAD"}</span>
          </span>
        )}
        <span className="flex-1" />
        <IconButton icon={Filter} size="sm" label={changedOnly ? "Show all files" : "Show changed files only"} active={changedOnly} onClick={() => setChangedOnly((v) => !v)} disabled={!tree.data?.git} />
        <IconButton icon={ChevronsDownUp} size="sm" label="Collapse all folders" onClick={() => setExpanded(new Set(changedOnly ? (root ? allDirs(root).map((d) => `-${d}`) : []) : []))} />
        <IconButton icon={RefreshCw} size="sm" label="Refresh" onClick={tree.reload} />
      </div>
      <div className="px-2.5 pb-2">
        <label className="flex h-8 items-center gap-2 rounded-lg border border-line bg-canvas px-2.5 focus-within:border-accent">
          <Search size={14} className="shrink-0 text-ink-subtle" aria-hidden />
          <span className="sr-only">Go to file</span>
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && results?.[0]) onOpen(results[0].path);
              else if (e.key === "Escape") setFilter("");
              else if (e.key === "ArrowDown") list.current?.querySelector<HTMLButtonElement>("[data-tree-row]")?.focus();
              else return;
              e.preventDefault();
            }}
            placeholder="Go to file"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-ink-faint"
          />
          {filter && (
            <button type="button" aria-label="Clear" onClick={() => setFilter("")} className="text-ink-subtle hover:text-ink">
              <X size={13} aria-hidden />
            </button>
          )}
        </label>
      </div>

      <div ref={list} onKeyDown={onKey} className="relative min-h-0 flex-1 overflow-y-auto px-2 pb-2" aria-label={`Files in ${baseName(folder)}`} role="navigation">
        {tree.loading && !tree.data ? (
          <p className="flex items-center gap-2 px-2 py-3 text-[13px] text-ink-subtle">
            <LoaderCircle size={14} className="animate-spin" aria-hidden />
            Reading the folder…
          </p>
        ) : tree.error && !tree.data ? (
          <Callout tone="danger" className="mx-1 mt-1" action={<IconButton icon={RefreshCw} size="sm" label="Try again" onClick={tree.reload} />}>
            {tree.error.message}
          </Callout>
        ) : results ? (
          results.length === 0 ? (
            <p className="px-2 py-3 text-[13px] text-ink-subtle">No file matches “{filter.trim()}”.</p>
          ) : (
            <>
              {results.slice(0, MAX_RESULTS).map((f) => (
                <FileRow key={f.path} file={f} name={baseName(f.path)} dir={dirName(f.path)} depth={0} selected={f.path === selected} onOpen={onOpen} />
              ))}
              {results.length > MAX_RESULTS && <p className="px-2 py-2 text-xs text-ink-subtle">{results.length - MAX_RESULTS} more; type more of the name.</p>}
            </>
          )
        ) : root && root.children.length ? (
          root.children.map(node)
        ) : (
          <Empty className="mx-1 mt-1 py-8" title={changedOnly ? "Nothing changed" : "No files here"}>
            {changedOnly ? "Every file matches the last commit." : "This folder is empty, or git ignores everything in it."}
          </Empty>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-3 border-t border-chrome-line px-4 pt-2 pb-2.5 text-[11.5px] text-ink-subtle">
        {tree.data?.git ? (
          <>
            <span>
              <b className="font-mono font-medium text-success">U</b> new
            </span>
            <span>
              <b className="font-mono font-medium text-warning">M</b> changed
            </span>
            <span>
              <b className="font-mono font-medium text-danger">D</b> deleted
            </span>
            <span className="ml-auto">{tree.data.truncated ? `First ${tree.data.files.length.toLocaleString("en-US")} files` : changedCount ? `${changedCount} changed` : "Ignored files hidden"}</span>
          </>
        ) : tree.data ? (
          <span>Not a git folder · .git and node_modules hidden{tree.data.truncated ? ` · first ${tree.data.files.length.toLocaleString("en-US")} files` : ""}</span>
        ) : null}
      </div>
    </div>
  );
}

function FileRow({
  file,
  name,
  dir,
  depth,
  guides,
  selected,
  onOpen,
}: {
  file: TreeFile;
  name: string;
  dir?: string;
  depth: number;
  guides?: ReactNode;
  selected: boolean;
  onOpen: (path: string) => void;
}) {
  const chip = langChip(file.path);
  const look = statusLook[file.status];
  return (
    <button
      type="button"
      data-tree-row
      data-path={file.path}
      aria-current={selected ? "page" : undefined}
      title={file.old_path ? `${file.path}, renamed from ${file.old_path}` : file.path}
      onClick={() => onOpen(file.path)}
      className={cx(
        "relative flex w-full items-center gap-[7px] rounded-md pr-2.5 text-left text-[12.5px] whitespace-nowrap",
        selected ? "bg-surface-3 font-medium text-ink shadow-[inset_0_0_0_1px_var(--color-line-strong)]" : "hover:bg-surface-2",
        !selected && (file.status === "U" || file.status === "A" ? "text-success" : file.status === "D" ? "text-danger/90" : "text-ink/85"),
      )}
      style={{ height: ROW, paddingLeft: dir !== undefined ? 8 : 25 + 14 * depth }}
    >
      {guides}
      {chip ? (
        <span aria-hidden className="inline-flex h-3.5 w-[17px] shrink-0 items-center justify-center rounded-[3px] font-mono text-[8.5px] font-semibold" style={{ background: chip[1], color: chip[2] }}>
          {chip[0]}
        </span>
      ) : (
        <FileIcon size={14} strokeWidth={1.8} className="shrink-0 text-ink-faint" aria-hidden />
      )}
      <span className={cx("min-w-0 truncate", file.status === "D" && "line-through decoration-danger/60")}>{name}</span>
      {dir && <span className="min-w-0 truncate text-[11.5px] font-normal text-ink-faint">{dir}</span>}
      {look ? (
        <span className={cx("ml-auto shrink-0 font-mono text-[11px] font-medium", look.text)} title={look.label}>
          {file.status}
          <span className="sr-only">, {look.label.toLowerCase()}</span>
        </span>
      ) : (
        selected && <span className="ml-auto shrink-0 text-[11px] font-normal text-ink-subtle">open</span>
      )}
    </button>
  );
}

function allDirs(n: TreeNode, out: string[] = []): string[] {
  for (const c of n.children) {
    if (c.dir) {
      out.push(c.path);
      allDirs(c, out);
    }
  }
  return out;
}

/** Files whose path holds every word of the query, names that match first, then shorter paths. */
function search(files: TreeFile[], query: string): TreeFile[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
  const hits: { f: TreeFile; score: number }[] = [];
  for (const f of files) {
    const p = f.path.toLowerCase();
    if (!terms.every((t) => p.includes(t))) continue;
    const name = baseName(p);
    const score = (terms.every((t) => name.includes(t)) ? 0 : 1000) + (name.startsWith(terms[0]) ? 0 : 500) + p.length;
    hits.push({ f, score });
  }
  return hits.sort((a, b) => a.score - b.score).map((h) => h.f);
}
