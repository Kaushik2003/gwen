import { useCallback, useEffect, useRef, useState } from "react";
import { App, apiError, type ApiError } from "../api";
import { repo } from "../wailsjs/go/models";

// A project's files, history and changes (plan.md 3.15, boards 2, 5 and 6): typed calls into
// internal/repo through the Wails bindings, and the pure helpers the workbench views share.

export type Tree = repo.Tree;
export type TreeFile = repo.TreeFile;
export type FileText = repo.File;
export type Marks = repo.Marks;
export type Mark = repo.Mark;
export type BlameLine = repo.BlameLine;
export type Log = repo.Log;
export type Commit = repo.Commit;
export type Ref = repo.Ref;
export type CommitDetail = repo.CommitDetail;
export type FileChange = repo.FileChange;
export type DiffText = repo.DiffText;

export interface LogQuery {
  limit?: number;
  skip?: number;
  search?: string;
  author?: string;
  path?: string;
}

export const git = {
  tree: (folder: string) => App.RepoTree(folder),
  file: (folder: string, path: string) => App.RepoFile(folder, path),
  marks: (folder: string, path: string) => App.RepoMarks(folder, path),
  blame: (folder: string, path: string, line: number) => App.RepoBlame(folder, path, line),
  log: (folder: string, q: LogQuery) => App.RepoLog(folder, repo.LogQuery.createFrom({ limit: 0, skip: 0, search: "", author: "", path: "", ...q })),
  /** hash "" is the uncommitted work. */
  commit: (folder: string, hash: string) => App.RepoCommit(folder, hash),
  /** hash "" is the uncommitted work. */
  diff: (folder: string, hash: string, path: string) => App.RepoDiff(folder, hash, path),
  /** Commits exactly these paths; returns the new short commit id. */
  commitFiles: (folder: string, message: string, paths: string[]) => App.RepoCommitFiles(folder, message, paths),
  open: (folder: string, path: string) => App.OpenPath(folder, path),
};

/** The folder has no git repository (internal/repo ErrNotRepo). */
export const notRepo = (e: ApiError | null) => e?.code === "not_repo";

export interface Loaded<T> {
  data: T | undefined;
  error: ApiError | null;
  loading: boolean;
  /** Fetches again, keeping what is shown until the answer comes. */
  reload: () => void;
}

/**
 * Loads data for a key, drops answers that arrive after the key changed, and with focus set
 * fetches again quietly whenever the window gains focus (files change outside Gwen).
 */
export function useLoad<T>(key: string, load: () => Promise<T>, focus = true): Loaded<T> {
  const [state, setState] = useState<{ key: string; data?: T; error: ApiError | null; loading: boolean }>({ key, error: null, loading: true });
  const seq = useRef(0);
  const loader = useRef(load);
  loader.current = load;
  const run = useCallback(
    (quiet: boolean) => {
      const n = ++seq.current;
      if (!quiet) setState({ key, error: null, loading: true });
      loader.current().then(
        (data) => n === seq.current && setState({ key, data, error: null, loading: false }),
        (e) => n === seq.current && setState((s) => ({ key, data: s.key === key ? s.data : undefined, error: apiError(e), loading: false })),
      );
    },
    [key],
  );
  useEffect(() => run(false), [run]);
  useEffect(() => {
    if (!focus) return;
    const onFocus = () => run(true);
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [run, focus]);
  const stale = state.key !== key;
  return { data: stale ? undefined : state.data, error: stale ? null : state.error, loading: stale || state.loading, reload: useCallback(() => run(true), [run]) };
}

// ---------------------------------------------------------------------------------------------
// Paths, statuses, numbers, time.

export const baseName = (p: string) => p.slice(p.lastIndexOf("/") + 1);
export const dirName = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "");

/** A status letter's colour and meaning: U and A green, M amber, D red, R blue. */
export const statusLook: Record<string, { text: string; label: string }> = {
  U: { text: "text-success", label: "New file, not committed" },
  A: { text: "text-success", label: "Added" },
  M: { text: "text-warning", label: "Changed" },
  D: { text: "text-danger", label: "Deleted" },
  R: { text: "text-info", label: "Renamed" },
  C: { text: "text-info", label: "Copied" },
  T: { text: "text-warning", label: "Type changed" },
};

/** "1,017" below ten thousand, "29.9k" above. */
export function count(n: number): string {
  return n < 10_000 ? n.toLocaleString("en-US") : `${(n / 1000).toFixed(1).replace(/\.0$/, "")}k`;
}

/** "just now", "30 min ago", "3 hours ago", "yesterday", "4 weeks ago", "2 years ago". */
export function relTime(ms: number, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 45) return "just now";
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return h === 1 ? "an hour ago" : `${h} hours ago`;
  const d = Math.round(h / 24);
  if (d === 1) return "yesterday";
  if (d < 7) return `${d} days ago`;
  if (d < 30) return Math.round(d / 7) === 1 ? "a week ago" : `${Math.round(d / 7)} weeks ago`;
  const mo = Math.round(d / 30.44);
  if (mo < 12) return mo <= 1 ? "a month ago" : `${mo} months ago`;
  const y = Math.round(d / 365.25);
  return y <= 1 ? "a year ago" : `${y} years ago`;
}

/** "Fri 13 Nov 2026, 12:38 am". */
export function longTime(ms: number): string {
  const d = new Date(ms);
  const day = d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", year: "numeric" });
  const time = d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" }).toLowerCase();
  return `${day}, ${time}`;
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

const langNames: Record<string, string> = {
  go: "Go", ts: "TypeScript", tsx: "TSX", js: "JavaScript", jsx: "JSX", json: "JSON", md: "Markdown", css: "CSS", html: "HTML", xml: "XML",
  sh: "Shell", py: "Python", rs: "Rust", toml: "TOML", yaml: "YAML", sql: "SQL", c: "C", cpp: "C++", java: "Java", kotlin: "Kotlin",
  swift: "Swift", ruby: "Ruby", lua: "Lua", proto: "Protobuf", ini: "Config", make: "Makefile", docker: "Dockerfile", gomod: "Go module",
  ignore: "Ignore list", diff: "Diff", text: "Text",
};
export const langName = (lang: string) => langNames[lang] ?? "Plain text";

/** The small coloured chip in front of a file's name: its language in two or three letters. */
const chips: Record<string, [string, string, string]> = {
  go: ["go", "rgba(0,173,216,.15)", "#6fd3eb"],
  ts: ["ts", "rgba(49,120,198,.2)", "#8cbcf4"],
  tsx: ["tsx", "rgba(49,120,198,.2)", "#8cbcf4"],
  js: ["js", "rgba(240,219,79,.14)", "#f2dc6f"],
  jsx: ["jsx", "rgba(240,219,79,.14)", "#f2dc6f"],
  json: ["{}", "rgba(242,179,61,.14)", "#f2c58a"],
  md: ["md", "rgba(175,162,160,.14)", "#cfc6c3"],
  css: ["css", "rgba(156,140,240,.16)", "#c9bff7"],
  html: ["<>", "rgba(233,147,90,.16)", "#f1ab7a"],
  xml: ["<>", "rgba(233,147,90,.16)", "#f1ab7a"],
  py: ["py", "rgba(77,155,219,.16)", "#8ec1ec"],
  rs: ["rs", "rgba(222,165,132,.16)", "#e9b593"],
  sh: ["sh", "rgba(79,184,138,.15)", "#8fd6b2"],
  yaml: ["yml", "rgba(175,162,160,.12)", "#b9afac"],
  toml: ["tml", "rgba(175,162,160,.12)", "#b9afac"],
  ini: ["cfg", "rgba(175,162,160,.12)", "#b9afac"],
  sql: ["sql", "rgba(238,132,150,.14)", "#f2a5b3"],
  gomod: ["go", "rgba(0,173,216,.1)", "#6fd3eb"],
  make: ["mk", "rgba(175,162,160,.12)", "#b9afac"],
  docker: ["dk", "rgba(77,155,219,.16)", "#8ec1ec"],
};
export function langChip(path: string): [string, string, string] | null {
  return chips[langOf(path)] ?? null;
}

/** The same names as internal/repo Lang, for paths the tree lists. */
export function langOf(path: string): string {
  const base = baseName(path).toLowerCase();
  const byName: Record<string, string> = { makefile: "make", gnumakefile: "make", dockerfile: "docker", "go.mod": "gomod", "go.work": "gomod", ".gitignore": "ignore" };
  if (byName[base]) return byName[base];
  const ext = base.includes(".") ? base.slice(base.lastIndexOf(".") + 1) : "";
  const byExt: Record<string, string> = {
    go: "go", ts: "ts", mts: "ts", cts: "ts", tsx: "tsx", js: "js", mjs: "js", cjs: "js", jsx: "jsx", json: "json", jsonc: "json", md: "md",
    markdown: "md", css: "css", scss: "css", html: "html", htm: "html", vue: "html", svelte: "html", xml: "xml", svg: "xml", sh: "sh", bash: "sh",
    zsh: "sh", fish: "sh", py: "py", rs: "rs", toml: "toml", yaml: "yaml", yml: "yaml", sql: "sql", c: "c", h: "c", cc: "cpp", cpp: "cpp",
    hpp: "cpp", java: "java", kt: "kotlin", swift: "swift", rb: "ruby", lua: "lua", proto: "proto", ini: "ini", conf: "ini", cfg: "ini",
    desktop: "ini", service: "ini", diff: "diff", patch: "diff",
  };
  return byExt[ext] ?? "";
}

// ---------------------------------------------------------------------------------------------
// The file tree, from the flat list git gives.

export interface TreeNode {
  name: string;
  path: string; // "" for the root
  dir: boolean;
  depth: number; // 0 for the root's children
  children: TreeNode[];
  file?: TreeFile;
  changed: number; // changed files at or below this node
}

/** Builds folders from the paths: folders first, then files, each in natural order. */
export function buildTree(files: TreeFile[]): TreeNode {
  const root: TreeNode = { name: "", path: "", dir: true, depth: -1, children: [], changed: 0 };
  const dirs = new Map<string, TreeNode>([["", root]]);
  for (const f of files) {
    const parts = f.path.split("/");
    let parent = root;
    for (let i = 0; i < parts.length - 1; i++) {
      const path = parts.slice(0, i + 1).join("/");
      let d = dirs.get(path);
      if (!d) {
        d = { name: parts[i], path, dir: true, depth: i, children: [], changed: 0 };
        dirs.set(path, d);
        parent.children.push(d);
      }
      parent = d;
    }
    parent.children.push({ name: parts[parts.length - 1], path: f.path, dir: false, depth: parts.length - 1, children: [], file: f, changed: f.status ? 1 : 0 });
  }
  const finish = (n: TreeNode): number => {
    if (!n.dir) return n.changed;
    n.children.sort((a, b) => (a.dir !== b.dir ? (a.dir ? -1 : 1) : a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" })));
    n.changed = n.children.reduce((sum, c) => sum + finish(c), 0);
    return n.changed;
  };
  finish(root);
  return root;
}

/** The folders that hold changed files, which start open. */
export function changedDirs(files: TreeFile[]): Set<string> {
  const out = new Set<string>();
  for (const f of files) if (f.status) for (let d = dirName(f.path); d && !out.has(d); d = dirName(d)) out.add(d);
  return out;
}

// ---------------------------------------------------------------------------------------------
// Unified diffs.

export interface DiffLine {
  kind: "ctx" | "add" | "del";
  text: string;
  old: number | null;
  new: number | null;
  noEol?: boolean;
}

export interface DiffHunk {
  header: string; // "@@ -78,14 +78,6 @@"
  context: string; // the function git names after it
  lines: DiffLine[];
}

export interface ParsedDiff {
  hunks: DiffHunk[];
  added: number;
  removed: number;
  binary: boolean;
  isNew: boolean;
  isDeleted: boolean;
  renamedFrom: string;
  modeChange: string; // "100644 → 100755"
}

const hunkRe = /^(@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@)\s?(.*)$/;

export function parseDiff(text: string): ParsedDiff {
  const out: ParsedDiff = { hunks: [], added: 0, removed: 0, binary: false, isNew: false, isDeleted: false, renamedFrom: "", modeChange: "" };
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop();
  let hunk: DiffHunk | null = null;
  let oldNo = 0;
  let newNo = 0;
  let oldMode = "";
  for (const raw of lines) {
    const line = raw.endsWith("\r") ? raw.slice(0, -1) : raw;
    const h = hunkRe.exec(line);
    if (h) {
      hunk = { header: h[1], context: h[4], lines: [] };
      out.hunks.push(hunk);
      oldNo = Number(h[2]);
      newNo = Number(h[3]);
      continue;
    }
    if (line.startsWith("diff --git ")) {
      if (out.hunks.length) break; // one file only
      continue;
    }
    if (!hunk) {
      if (line.startsWith("new file mode")) out.isNew = true;
      else if (line.startsWith("deleted file mode")) out.isDeleted = true;
      else if (line.startsWith("rename from ")) out.renamedFrom = line.slice(12);
      else if (line.startsWith("Binary files ") || line === "GIT binary patch") out.binary = true;
      else if (line.startsWith("old mode ")) oldMode = line.slice(9);
      else if (line.startsWith("new mode ")) out.modeChange = `${oldMode} → ${line.slice(9)}`;
      continue;
    }
    const sign = line[0];
    if (sign === "+") {
      hunk.lines.push({ kind: "add", text: line.slice(1), old: null, new: newNo++ });
      out.added++;
    } else if (sign === "-") {
      hunk.lines.push({ kind: "del", text: line.slice(1), old: oldNo++, new: null });
      out.removed++;
    } else if (sign === "\\") {
      const last = hunk.lines[hunk.lines.length - 1];
      if (last) last.noEol = true;
    } else {
      hunk.lines.push({ kind: "ctx", text: line.slice(1), old: oldNo++, new: newNo++ });
    }
  }
  return out;
}

export interface SplitSide {
  kind: "ctx" | "add" | "del";
  text: string;
  num: number;
}
export interface SplitRow {
  left: SplitSide | null;
  right: SplitSide | null;
}

/** Pairs a hunk's lines side by side: a run of deletions faces the additions that follow it. */
export function splitRows(lines: DiffLine[]): SplitRow[] {
  const rows: SplitRow[] = [];
  let i = 0;
  while (i < lines.length) {
    const l = lines[i];
    if (l.kind === "ctx") {
      rows.push({ left: { kind: "ctx", text: l.text, num: l.old! }, right: { kind: "ctx", text: l.text, num: l.new! } });
      i++;
      continue;
    }
    const dels: DiffLine[] = [];
    const adds: DiffLine[] = [];
    while (i < lines.length && lines[i].kind === "del") dels.push(lines[i++]);
    while (i < lines.length && lines[i].kind === "add") adds.push(lines[i++]);
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) {
      const d = dels[k];
      const a = adds[k];
      rows.push({ left: d ? { kind: "del", text: d.text, num: d.old! } : null, right: a ? { kind: "add", text: a.text, num: a.new! } : null });
    }
  }
  return rows;
}

// ---------------------------------------------------------------------------------------------
// The history graph: a classic lane assignment over commits in topological order.

export const laneColors = ["#3ec1af", "#9c8cf0", "#e9935a", "#4d9bdb", "#ee8496", "#86c66a", "#f2c94c", "#7cc4f2"];
/** The uncommitted work's colour, and its dashed line down to HEAD. */
export const uncommittedColor = "#f2b33d";
export const laneColor = (c: number) => (c < 0 ? uncommittedColor : laneColors[c % laneColors.length]);

export interface GraphLine {
  from: number; // lane at the start
  to: number; // lane at the end
  color: number; // -1: the uncommitted work's
  /** top: from the row's top edge to its node; bottom: from the node to the bottom edge; through: past the node. */
  part: "top" | "bottom" | "through";
  dashed?: boolean;
}

export interface GraphRow {
  lane: number;
  color: number;
  merge: boolean;
  lines: GraphLine[];
  width: number; // lanes in use in this row
}

type Lane = { hash: string; color: number; dashed: boolean } | null;

/**
 * Gives every commit a lane. A lane waits for the next commit it expects (a parent of the commit
 * above); a commit takes the first lane waiting for it, the other lanes waiting for it join it,
 * its first parent continues in its lane and each further parent gets a lane of its own unless
 * one already waits for it. head, when given, is HEAD's commit, which the uncommitted row above
 * the list joins with a dashed line.
 */
export function graphRows(commits: Commit[], head?: string): GraphRow[] {
  let lanes: Lane[] = head ? [{ hash: head, color: -1, dashed: true }] : [];
  let next = 0;
  const free = () => {
    const i = lanes.indexOf(null);
    return i < 0 ? lanes.length : i;
  };
  return commits.map((c) => {
    const before = lanes.slice();
    let lane = before.findIndex((l) => l?.hash === c.hash);
    let color: number;
    if (lane < 0) {
      lane = free();
      color = next++;
    } else {
      color = before[lane]!.color;
      if (color < 0) color = next++;
    }
    const lines: GraphLine[] = [];
    before.forEach((l, j) => {
      if (l?.hash === c.hash) {
        lines.push({ from: j, to: lane, color: l.color, part: "top", dashed: l.dashed });
        lanes[j] = null;
      }
    });
    if (lane >= lanes.length) lanes.length = lane + 1;
    lanes[lane] = null;
    if (c.parents.length) {
      lanes[lane] = { hash: c.parents[0], color, dashed: false };
      lines.push({ from: lane, to: lane, color, part: "bottom" });
    }
    for (const p of c.parents.slice(1)) {
      let k = lanes.findIndex((l) => l?.hash === p);
      if (k < 0) {
        k = free();
        lanes[k] = { hash: p, color: next++, dashed: false };
      }
      lines.push({ from: lane, to: k, color: lanes[k]!.color, part: "bottom" });
    }
    before.forEach((l, j) => {
      if (l && l.hash !== c.hash) lines.push({ from: j, to: j, color: l.color, part: "through", dashed: l.dashed });
    });
    while (lanes.length && lanes[lanes.length - 1] === null) lanes.pop();
    lanes = lanes.slice();
    const width = Math.max(before.length, lanes.length, lane + 1);
    return { lane, color, merge: c.parents.length > 1, lines, width };
  });
}

// ---------------------------------------------------------------------------------------------
// A small syntax highlighter: comments, strings, numbers, keywords, types, and calls.

export type TokClass = "k" | "t" | "f" | "s" | "c" | "n";
export interface Tok {
  t: string;
  c?: TokClass;
}

/** Token colours, from the approved mockup. */
export const tokClass: Record<TokClass, string> = {
  k: "text-[#ee8496]",
  t: "text-[#7cc4f2]",
  f: "text-[#86e2d6]",
  s: "text-[#f2c58a]",
  c: "text-[#958a88] italic",
  n: "text-[#f6c29a]",
};

interface Spec {
  line?: string[]; // line comment starts
  block?: [string, string][]; // block comments
  multi?: string[]; // strings that may span lines, closed by the same text
  quotes?: string; // one-line strings
  keywords: Set<string>;
  consts?: Set<string>;
  types?: Set<string>;
  capsAreTypes?: boolean; // Capitalised names are types
  noCase?: boolean; // keywords in any case (SQL)
  keys?: boolean; // "name:" or "name =" at a line's start is a key (YAML, TOML, INI)
}

const words = (s: string) => new Set(s.split(" "));
const cKeywords = "if else for while do switch case default break continue return goto struct union enum typedef static const extern sizeof void volatile inline";
const specs: Record<string, Spec> = {
  go: {
    line: ["//"], block: [["/*", "*/"]], multi: ["`"], quotes: `"'`, capsAreTypes: true,
    keywords: words("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var"),
    consts: words("true false nil iota"),
    types: words("bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr any comparable"),
  },
  js: {
    line: ["//"], block: [["/*", "*/"]], multi: ["`"], quotes: `"'`, capsAreTypes: true,
    keywords: words(
      "abstract as async await break case catch class const continue debugger declare default delete do else enum export extends finally for from function get if implements import in instanceof interface keyof let new of package private protected public readonly return satisfies set static super switch throw try type typeof var void while with yield",
    ),
    consts: words("true false null undefined this NaN Infinity"),
    types: words("string number boolean any unknown never object symbol bigint void"),
  },
  py: {
    line: ["#"], multi: [`"""`, "'''"], quotes: `"'`, capsAreTypes: true,
    keywords: words("and as assert async await break class continue def del elif else except finally for from global if import in is lambda match case nonlocal not or pass raise return try while with yield"),
    consts: words("True False None self cls"),
    types: words("int float str bool bytes list dict set tuple object"),
  },
  rs: {
    line: ["//"], block: [["/*", "*/"]], quotes: `"`, capsAreTypes: true,
    keywords: words("as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return static struct super trait type unsafe use where while"),
    consts: words("true false self Self None Some Ok Err"),
    types: words("i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool char str String Vec Option Result Box"),
  },
  c: {
    line: ["//"], block: [["/*", "*/"]], quotes: `"'`, capsAreTypes: true,
    keywords: words(`${cKeywords} class namespace template typename public private protected virtual override new delete try catch throw using import package final fun val var let func guard self extends implements interface`),
    consts: words("true false null nullptr NULL this nil"),
    types: words("int char short long float double unsigned signed bool size_t auto String"),
  },
  sh: {
    line: ["#"], quotes: `"'`,
    keywords: words("if then else elif fi for while until do done case esac function in return export local readonly set unset shift exit source alias"),
    consts: words("true false"),
  },
  sql: {
    line: ["--"], block: [["/*", "*/"]], quotes: `'"`, noCase: true,
    keywords: words(
      "select from where and or not insert into values update set delete create table index view drop alter add column primary key foreign references join left right inner outer on as group by order having limit offset distinct union all case when then else end begin commit rollback if exists default unique check null is in like between returning with",
    ),
    consts: words("true false"),
    types: words("integer int text real blob varchar boolean timestamp date numeric"),
  },
  css: { block: [["/*", "*/"]], quotes: `"'`, keywords: words("important"), consts: words("inherit initial unset none auto") },
  json: { quotes: `"`, keywords: new Set(), consts: words("true false null") },
  yaml: { line: ["#"], quotes: `"'`, keys: true, keywords: new Set(), consts: words("true false null yes no on off") },
  toml: { line: ["#", ";"], quotes: `"'`, keys: true, keywords: new Set(), consts: words("true false") },
  make: { line: ["#"], quotes: `"'`, keys: true, keywords: words("ifeq ifneq ifdef ifndef else endif include define endef export") },
  docker: { line: ["#"], quotes: `"'`, keywords: words("FROM RUN CMD COPY ADD ENV ARG WORKDIR EXPOSE ENTRYPOINT USER VOLUME LABEL AS HEALTHCHECK SHELL") },
  html: { block: [["<!--", "-->"]], quotes: `"'`, keywords: new Set() },
};
const specFor: Record<string, string> = {
  go: "go", gomod: "go", ts: "js", tsx: "js", js: "js", jsx: "js", py: "py", rs: "rs", c: "c", cpp: "c", java: "c", kotlin: "c", swift: "c",
  proto: "c", sh: "sh", sql: "sql", css: "css", json: "json", yaml: "yaml", toml: "toml", ini: "toml", make: "make", docker: "docker",
  html: "html", xml: "html", ruby: "py", lua: "sql",
};

const wordRe = /[A-Za-z_$][\w$]*/y;
const numRe = /(?:0[xX][\da-fA-F_]+|0[bB][01_]+|\d[\d_]*(?:\.\d+)?(?:[eE][+-]?\d+)?)[a-zA-Z]*/y;
const keyRe = /^(\s*-?\s*)([\w.$@/-]+|"[^"]*"|'[^']*')(\s*[:=])/;
const tagRe = /<\/?[A-Za-z][\w:-]*|\/?>/y;

/** Splits text into lines of tokens. Unknown languages come back as plain lines. */
export function highlight(text: string, lang: string): Tok[][] {
  const lines = text.split("\n");
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop();
  const plain = (l: string) => [{ t: l.endsWith("\r") ? l.slice(0, -1) : l }];
  if (lang === "md") return lines.map((l) => (/^#{1,6}\s/.test(l) ? [{ t: l, c: "k" as const }] : /^(```|~~~)/.test(l) ? [{ t: l, c: "c" as const }] : plain(l)));
  if (lang === "diff") return lines.map((l) => [{ t: l, c: l[0] === "+" ? ("f" as const) : l[0] === "-" ? ("k" as const) : l.startsWith("@@") ? ("t" as const) : undefined }]);
  const spec = specs[specFor[lang] ?? ""];
  if (!spec) return lines.map(plain);
  let open: { end: string; c: TokClass } | null = null;
  return lines.map((raw) => {
    const line = raw.endsWith("\r") ? raw.slice(0, -1) : raw;
    const toks: Tok[] = [];
    let buf = "";
    const push = (t: string, c?: TokClass) => {
      if (!c) return void (buf += t);
      if (buf) toks.push({ t: buf });
      buf = "";
      if (t) toks.push({ t, c });
    };
    let i = 0;
    if (spec.keys && !open) {
      const k = keyRe.exec(line);
      if (k && !spec.line?.some((p) => line.trimStart().startsWith(p))) {
        push(k[1]);
        push(k[2], "k");
        push(k[3]);
        i = k[0].length;
      } else if (/^\s*\[[^\]]*\]\s*$/.test(line)) {
        return [{ t: line, c: "t" as const }];
      }
    }
    while (i < line.length) {
      if (open) {
        const j = line.indexOf(open.end, i);
        if (j < 0) {
          push(line.slice(i), open.c);
          break;
        }
        push(line.slice(i, j + open.end.length), open.c);
        i = j + open.end.length;
        open = null;
        continue;
      }
      if (spec.line?.some((p) => line.startsWith(p, i) && (p !== "#" || i === 0 || /\s/.test(line[i - 1])))) {
        push(line.slice(i), "c");
        break;
      }
      const blk = spec.block?.find(([a]) => line.startsWith(a, i));
      if (blk) {
        push(blk[0], "c");
        i += blk[0].length;
        open = { end: blk[1], c: "c" };
        continue;
      }
      const ml = spec.multi?.find((d) => line.startsWith(d, i));
      if (ml) {
        push(ml, "s");
        i += ml.length;
        open = { end: ml, c: "s" };
        continue;
      }
      const ch = line[i];
      if (spec.quotes?.includes(ch)) {
        let j = i + 1;
        while (j < line.length && line[j] !== ch) j += line[j] === "\\" ? 2 : 1;
        const s = line.slice(i, j + 1);
        // A JSON key is the string before a colon.
        const key = lang === "json" && /^\s*:/.test(line.slice(j + 1));
        push(s, key ? "t" : "s");
        i = j + 1;
        continue;
      }
      if (lang === "html" || lang === "xml") {
        tagRe.lastIndex = i;
        const tag = tagRe.exec(line);
        if (tag) {
          push(tag[0], "k");
          i += tag[0].length;
          continue;
        }
      }
      const prev = i > 0 ? line[i - 1] : " ";
      if (ch >= "0" && ch <= "9" && !/[\w$.]/.test(prev)) {
        numRe.lastIndex = i;
        const n = numRe.exec(line);
        if (n) {
          push(n[0], "n");
          i += n[0].length;
          continue;
        }
      }
      wordRe.lastIndex = i;
      const w = /[\w$]/.test(prev) ? null : wordRe.exec(line);
      if (w) {
        const word = w[0];
        const lower = spec.noCase ? word.toLowerCase() : word;
        let j = i + word.length;
        while (line[j] === " ") j++;
        const c: TokClass | undefined = spec.keywords.has(lower)
          ? "k"
          : spec.consts?.has(word)
            ? "n"
            : spec.types?.has(lower)
              ? "t"
              : line[j] === "("
                ? "f"
                : spec.capsAreTypes && /^[A-Z][a-z0-9]/.test(word)
                  ? "t"
                  : undefined;
        push(word, c);
        i += word.length;
        continue;
      }
      push(ch);
      i++;
    }
    if (buf) toks.push({ t: buf });
    return toks;
  });
}
