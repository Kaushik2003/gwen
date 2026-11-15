import { isolateHistory } from "@codemirror/commands";
import type { Text } from "@codemirror/state";
import { EditorView, type ViewUpdate } from "@codemirror/view";
import { ExternalLink, FileWarning, History as HistoryIcon, LoaderCircle, Save, UserRoundSearch } from "lucide-react";
import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { apiError } from "../api";
import { Button, Callout, Empty, IconButton, cx } from "../components/ui";
import { setUnsaved } from "../shell/unsaved";
import { editable, editableExt, editorState, markedLines, minimalChange, separatorOf, setBlame, setMarks, type EditorHooks, type MarkKind } from "./editor";
import { baseName, bytes, changedOnDisk, git, langName, relTime, useLoad, type FileText } from "./repo";

// A file's tab (plan.md 3.15, board 5): the file in an editor (work/editor.ts), with gutter marks for the
// uncommitted lines (else the lines of the last commit that touched the file) and blame for the cursor's
// line. Ctrl+S or Save writes it. It follows the file on disk, as Herdr's editor does: a change made
// elsewhere (an agent, another editor) comes in at once while nothing here is unsaved, and with unsaved
// edits it asks: Reload, or Keep mine. A save writes only over the text it last read (repo.SaveFile's
// check), so a change made meanwhile is never overwritten unseen.

const POLL_MS = 1500; // how often the open tab looks for a change on disk

interface Saved {
  doc: Text; // the text as on disk, to tell unsaved edits from none
  hash: string; // "" when the file is read-only here
  stamp: string;
}

export default function FileView({ folder, path, tabKey, active = true, onOpenHistory }: { folder: string; path: string; tabKey: string; active?: boolean; onOpenHistory?: () => void }) {
  const key = `${folder}\n${path}`;
  const file = useLoad(`file:${key}`, () => git.file(folder, path));
  const marks = useLoad(`marks:${key}`, () => git.marks(folder, path));
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const saved = useRef<Saved | null>(null);
  const [dirty, setDirty] = useState(false);
  const [conflict, setConflict] = useState<FileText | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [openError, setOpenError] = useState("");
  const [blameOn, setBlameOn] = useState(false);
  const [pos, setPos] = useState({ line: 1, col: 1, lines: 0 });
  const [marked, setMarked] = useState<{ line: number; kind: MarkKind }[]>([]);
  const [scroll, setScroll] = useState({ top: 0, height: 0, total: 0 });
  const [ready, setReady] = useState(false);
  const hooks = useRef<EditorHooks>({ save: () => {}, update: () => {} });

  const data = file.data;
  const canEdit = !!data?.hash;

  /** Puts the file's text in the editor and takes it as the saved text: all of it the first time, else just what changed. */
  const load = useCallback((d: FileText) => {
    const v = view.current;
    if (!v) return;
    const sep = separatorOf(d.text);
    if (sep !== v.state.lineBreak) v.setState(editorState({ text: d.text, lang: d.lang, canEdit: !!d.hash }, hooks));
    else {
      const next = sep === "\r\n" ? d.text.split("\r\n").join("\n") : d.text;
      const c = minimalChange(v.state.doc.toString(), next);
      v.dispatch({
        changes: c.from === c.to && !c.insert ? undefined : { ...c, insert: sep === "\r\n" ? c.insert.split("\n").join("\r\n") : c.insert },
        effects: editable.reconfigure(editableExt(!!d.hash)),
      });
    }
    saved.current = { doc: v.state.doc, hash: d.hash, stamp: d.stamp };
    setDirty(false);
    setConflict(null);
    setPos((p) => ({ ...p, lines: v.state.doc.lines }));
  }, []);

  const save = useCallback(async (): Promise<boolean> => {
    const v = view.current;
    const s = saved.current;
    if (!v || !s?.hash || saving) return false;
    const doc = v.state.doc;
    if (doc.eq(s.doc)) return true;
    setSaving(true);
    setSaveError("");
    try {
      const d = await git.save(folder, path, v.state.sliceDoc(), s.hash);
      saved.current = { doc, hash: d.hash, stamp: d.stamp };
      // Typing after a save starts a new undo step, so undo comes back to the saved text.
      v.dispatch({ annotations: isolateHistory.of("full") });
      setDirty(!v.state.doc.eq(doc));
      setConflict(null);
      marks.reload();
      return true;
    } catch (e) {
      const err = apiError(e);
      if (changedOnDisk(err)) file.reload(); // shows Reload / Keep mine
      else setSaveError(err.message);
      return false;
    } finally {
      setSaving(false);
    }
  }, [folder, path, saving, file, marks]);

  hooks.current = {
    save: () => void save(),
    update: (u: ViewUpdate) => {
      const s = saved.current;
      if (u.docChanged && s) setDirty(!u.state.doc.eq(s.doc));
      if (u.docChanged || u.selectionSet) {
        const head = u.state.selection.main.head;
        const line = u.state.doc.lineAt(head);
        setPos({ line: line.number, col: head - line.from + 1, lines: u.state.doc.lines });
      }
      if (u.docChanged || u.transactions.some((tr) => tr.effects.some((e) => e.is(setMarks)))) setMarked(markedLines(u.state));
    },
  };

  // The editor, once there is text to show; a later read of the file goes through load.
  useEffect(() => {
    if (!data || data.binary || !host.current) return;
    const v = view.current;
    if (!v) {
      const nv = new EditorView({ parent: host.current, state: editorState({ text: data.text, lang: data.lang, canEdit: !!data.hash }, hooks) });
      view.current = nv;
      saved.current = { doc: nv.state.doc, hash: data.hash, stamp: data.stamp };
      setPos({ line: 1, col: 1, lines: nv.state.doc.lines });
      const onScroll = () => setScroll({ top: nv.scrollDOM.scrollTop, height: nv.scrollDOM.clientHeight, total: nv.scrollDOM.scrollHeight });
      nv.scrollDOM.addEventListener("scroll", onScroll, { passive: true });
      const ro = new ResizeObserver(onScroll);
      ro.observe(nv.scrollDOM);
      stopWatching.current = () => ro.disconnect();
      setReady(true);
      return;
    }
    const s = saved.current;
    if (s && (data.hash ? data.hash === s.hash : data.stamp === s.stamp)) {
      s.stamp = data.stamp; // the same text (a save of ours, or a touch)
      return;
    }
    if (s && !v.state.doc.eq(s.doc)) setConflict(data);
    else load(data);
  }, [data, load]);

  const stopWatching = useRef<() => void>(() => {});
  useEffect(
    () => () => {
      stopWatching.current();
      view.current?.destroy();
      view.current = null;
    },
    [],
  );

  // Gutter marks, as git last reported them; they move with your edits until the next save.
  useEffect(() => {
    const v = view.current;
    if (v && marks.data) v.dispatch({ effects: setMarks.of(marks.data.lines ?? []) });
  }, [marks.data, ready]);

  // Follow the file on disk while its tab is in front: a new stamp reads it again.
  useEffect(() => {
    if (!active) return;
    let live = true;
    const t = setInterval(() => {
      const s = saved.current;
      if (!s) return;
      git.stamp(folder, path).then(
        (stamp) => live && stamp !== s.stamp && stamp !== conflict?.stamp && file.reload(),
        () => {},
      );
    }, POLL_MS);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, [active, folder, path, file, conflict]);

  // Ctrl+S anywhere in the window while this tab is in front, not only with the cursor in the text.
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "s" && !e.defaultPrevented) {
        e.preventDefault();
        void saveRef.current();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active]);

  // The tab bar's ● and the question before the tab closes.
  const saveRef = useRef(save);
  saveRef.current = save;
  useEffect(() => {
    setUnsaved(tabKey, dirty ? () => saveRef.current() : null);
  }, [tabKey, dirty]);
  useEffect(() => () => setUnsaved(tabKey, null), [tabKey]);

  // Blame follows the cursor's line while it is on, a moment after the line settles; it reads the saved file.
  useEffect(() => {
    const v = view.current;
    if (!v) return;
    if (!blameOn || dirty) {
      v.dispatch({ effects: setBlame.of(blameOn && dirty ? { line: pos.line, text: "Save to see who changed this line" } : null) });
      return;
    }
    let live = true;
    const line = pos.line;
    const t = setTimeout(() => {
      view.current?.dispatch({ effects: setBlame.of({ line, text: "Looking…" }) });
      git.blame(folder, path, line).then(
        (b) => live && view.current?.dispatch({ effects: setBlame.of({ line, text: b.uncommitted ? "Not committed yet" : `${b.short} · ${b.author} · ${relTime(b.when)} · ${b.summary}` }) }),
        (e) => live && view.current?.dispatch({ effects: setBlame.of({ line, text: apiError(e).message }) }),
      );
    }, 120);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [blameOn, dirty, pos.line, folder, path]);

  const openInApp = () => {
    setOpenError("");
    git.open(folder, path).catch((e) => setOpenError(apiError(e).message));
  };

  const parts = path.split("/");
  const m = marks.data;
  const showEditor = !!data && !data.binary;

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
          {dirty && (
            <span className="text-ink-muted" title="Unsaved changes" aria-label="Unsaved changes">
              ●
            </span>
          )}
        </nav>
        {data && (
          <span className="text-xs whitespace-nowrap text-ink-subtle">
            {data.binary ? `Binary · ${bytes(data.size)}` : `${langName(data.lang)} · Ln ${pos.line}, Col ${pos.col} · ${pos.lines.toLocaleString("en-US")} ${pos.lines === 1 ? "line" : "lines"}`}
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
        <Button size="sm" icon={UserRoundSearch} aria-pressed={blameOn} className={cx(blameOn && "!border-accent/60 !bg-accent/15")} onClick={() => setBlameOn((v) => !v)} disabled={!showEditor} title="Shows who last changed the cursor's line">
          Blame
        </Button>
        {onOpenHistory && (
          <Button size="sm" icon={HistoryIcon} onClick={onOpenHistory}>
            History
          </Button>
        )}
        {canEdit && (
          <Button size="sm" tone={dirty ? "primary" : "secondary"} icon={Save} busy={saving} disabled={!dirty || saving} onClick={() => void save()} title="Save (Ctrl+S)">
            Save
          </Button>
        )}
        <IconButton icon={ExternalLink} label="Open in its app" size="sm" onClick={openInApp} />
      </div>
      {openError && (
        <Callout tone="danger" className="m-3 mb-0">
          Couldn't open the file: {openError}
        </Callout>
      )}
      {saveError && (
        <Callout tone="danger" className="m-3 mb-0" action={<Button size="sm" tone="ghost" onClick={() => setSaveError("")}>Dismiss</Button>}>
          Couldn't save {baseName(path)}: {saveError}
        </Callout>
      )}
      {conflict && (
        <Callout
          tone="accent"
          className="m-3 mb-0"
          action={
            <>
              <Button
                size="sm"
                onClick={() => {
                  load(conflict);
                  view.current?.focus();
                }}
              >
                Reload
              </Button>
              <Button
                size="sm"
                tone="ghost"
                onClick={() => {
                  // Keep mine: the next save writes over the new text on disk.
                  if (saved.current) saved.current = { ...saved.current, hash: conflict.hash, stamp: conflict.stamp };
                  setConflict(null);
                  view.current?.focus();
                }}
              >
                Keep mine
              </Button>
            </>
          }
        >
          {baseName(path)} changed on disk while you have unsaved changes here. Reload takes the new text and drops yours; Keep mine lets your next save write over it.
        </Callout>
      )}
      {data && !data.binary && !canEdit && (
        <Callout className="m-3 mb-0">
          {data.truncated
            ? `${baseName(path)} is ${bytes(data.size)}, more than the 1 MB Gwen reads at once: this is its start, read-only, since saving a cut-off copy would lose the rest.`
            : `${baseName(path)} isn't UTF-8 text, so it is read-only here: saving would change bytes you didn't touch.`}
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
        <Empty icon={FileWarning} title="A binary file" className="m-5" action={<Button size="sm" icon={ExternalLink} onClick={openInApp}>Open in its app</Button>}>
          {baseName(path)} is {bytes(data.size)} of binary data; Gwen edits text files.
        </Empty>
      ) : null}
      <div className={cx("relative min-h-0 flex-1 bg-canvas", !showEditor && "hidden")}>
        <div ref={host} className="absolute inset-0 right-3" />
        <Overview lines={pos.lines} marks={marked} scroll={scroll} view={view} />
      </div>
    </div>
  );
}

/** The strip at the right edge: where the marked lines are in the whole file, and what is in view. */
function Overview({ lines, marks, scroll, view }: { lines: number; marks: { line: number; kind: MarkKind }[]; scroll: { top: number; height: number; total: number }; view: { current: EditorView | null } }) {
  const runs = useMemo(() => {
    const out: { from: number; to: number; kind: MarkKind }[] = [];
    for (const { line, kind } of [...marks].sort((a, b) => a.line - b.line)) {
      if (kind === "del") {
        out.push({ from: line + 0.5, to: line + 0.5, kind });
        continue;
      }
      const last = out[out.length - 1];
      if (last && last.kind === kind && last.to >= line - 1) last.to = line;
      else out.push({ from: line, to: line, kind });
    }
    return out;
  }, [marks]);
  const pct = (n: number) => `${Math.min(100, Math.max(0, ((n - 1) / Math.max(1, lines)) * 100))}%`;
  return (
    <div
      aria-hidden
      onClick={(e) => {
        const el = view.current?.scrollDOM;
        if (!el) return;
        const r = e.currentTarget.getBoundingClientRect();
        el.scrollTop = ((e.clientY - r.top) / r.height) * el.scrollHeight - el.clientHeight / 2;
      }}
      className="absolute inset-y-0 right-0 w-3 cursor-pointer border-l border-chrome-line bg-chrome-deep"
    >
      {scroll.total > scroll.height && scroll.height > 0 && (
        <span
          className="absolute inset-x-px rounded-[2px] bg-ink/6 shadow-[inset_0_0_0_1px_rgb(242_241_237/0.1)]"
          style={{ top: `${(scroll.top / scroll.total) * 100}%`, height: `${Math.max(2, (scroll.height / scroll.total) * 100)}%` }}
        />
      )}
      {lines > 1 &&
        runs.map((r, i) => (
          <span
            key={i}
            className={cx("absolute inset-x-[3px] rounded-[1px]", r.kind === "add" ? "bg-success" : r.kind === "mod" ? "bg-warning" : "bg-danger")}
            style={{ top: pct(r.from), height: `max(3px, ${((r.to - r.from + 1) / lines) * 100}%)` }}
          />
        ))}
    </div>
  );
}
