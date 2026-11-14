import { ChevronDown, ChevronRight, Info } from "lucide-react";
import { useEffect, useState } from "react";
import { ApiError, api, type Change, type Changes, type Diff, type Session } from "./api";
import type { Ask } from "./Composer";
import { useStore } from "./store";

/** Chips above the composer on the Changes tab: each only fills the composer, for you to edit and send (plan.md 3.14.13). */
export const ASKS: Ask[] = [
  { label: "Run the tests", text: "Run the tests and tell me what fails." },
  { label: "Explain this diff", text: "Explain what you changed in this diff, file by file." },
  { label: "Commit it", text: "Commit these changes with a clear message." },
];

/** How many diff lines show before "… more lines". */
const FIRST = 10;

interface Line { kind: "add" | "del" | "ctx" | "hunk"; no: number | null; text: string }

/** A unified diff as numbered lines, without git's header lines: the new file's line numbers, the old ones for removed lines. */
function lines(diff: string): Line[] {
  const out: Line[] = [];
  let oldNo = 0, newNo = 0, inHunk = false;
  for (const l of diff.split("\n")) {
    const h = l.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
    if (h) {
      oldNo = Number(h[1]);
      newNo = Number(h[2]);
      inHunk = true;
      out.push({ kind: "hunk", no: null, text: l });
    } else if (!inHunk) continue;
    else if (l.startsWith("+")) out.push({ kind: "add", no: newNo++, text: l });
    else if (l.startsWith("-")) out.push({ kind: "del", no: oldNo++, text: l });
    else if (l.startsWith("\\")) out.push({ kind: "ctx", no: null, text: l });
    else if (l.startsWith("diff ")) inHunk = false;
    else {
      oldNo++;
      out.push({ kind: "ctx", no: newNo++, text: l || " " });
    }
  }
  while (out.length && out[out.length - 1].kind === "ctx" && out[out.length - 1].text === " ") out.pop();
  return out;
}

/** One file's diff, under its row. */
function FileDiff({ s, file }: { s: Session; file: Change }) {
  const { upsert, toast } = useStore();
  const [diff, setDiff] = useState<Diff | null>(null);
  const [failed, setFailed] = useState("");
  const [all, setAll] = useState(false);
  useEffect(() => {
    api.diff(s.id, file.path).then(setDiff, (e) => setFailed(e instanceof ApiError ? e.message : "Couldn't read that file's changes."));
  }, [s.id, file.path]);
  const laptop = async () => {
    try {
      upsert(await api.control(s.id, "open_on_laptop"));
      toast("Opened on the laptop");
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "That didn't go through.");
    }
  };
  if (failed) return <div className="diff"><div className="note">{failed}</div></div>;
  if (!diff) return <div className="diff"><div className="note"><span className="spin sm" /> Reading the diff…</div></div>;
  if (diff.binary) return <div className="diff"><div className="note">A binary file, {Math.max(1, Math.round(diff.size / 1024))} KB. There is nothing to show line by line.</div></div>;
  const ls = lines(diff.diff);
  const shown = all ? ls : ls.slice(0, FIRST);
  return (
    <div className="diff" role="region" aria-label={`Changes in ${file.path}`}>
      {shown.map((l, i) =>
        l.kind === "hunk" ? (
          <div key={i} className="ln hunk">{l.text}</div>
        ) : (
          <div key={i} className={`ln ${l.kind}`}>
            <span className="no">{l.no ?? ""}</span>
            <span className="tx">{l.text}</span>
          </div>
        ),
      )}
      {ls.length === 0 && <div className="note">No line changes, only the file's mode or name.</div>}
      {!all && ls.length > FIRST && <button className="more" onClick={() => setAll(true)}>… {ls.length - FIRST} more lines</button>}
      {diff.truncated && (all || ls.length <= FIRST) && (
        <div className="note">
          <span className="grow">This diff is long; the first 2,000 lines are shown.</span>
          <button className="btn xs" onClick={laptop}>Open on the laptop</button>
        </div>
      )}
    </div>
  );
}

const TAGS: Record<string, [string, string]> = { added: ["new", "new"], untracked: ["new", "new"], deleted: ["deleted", "gone"], renamed: ["renamed", ""] };

/** What the folder has changed since its last commit; tap a file to see its diff right under it. */
export function ChangesTab({ s }: { s: Session }) {
  const [changes, setChanges] = useState<Changes | null>(null);
  const [failed, setFailed] = useState("");
  const [open, setOpen] = useState<Record<string, boolean>>({});

  useEffect(() => {
    let live = true;
    api.changes(s.id).then(
      (c) => live && (setChanges(c), setFailed("")),
      (e) => live && setFailed(e instanceof ApiError ? e.message : "Couldn't read the changes."),
    );
    return () => {
      live = false;
    };
    // The step changes each time Claude touches something, which is when the list may have changed.
  }, [s.id, s.state, s.step]);

  if (failed) return <main className="page"><div className="empty"><p>{failed}</p></div></main>;
  if (!changes) return <main className="page"><div className="empty"><span className="spin" /></div></main>;
  if (changes.files.length === 0) {
    return (
      <main className="page">
        <div className="empty">
          <h2>No changes</h2>
          <p>{changes.git ? "Everything matches the last commit." : "Claude hasn't edited a file in this session."}</p>
        </div>
      </main>
    );
  }
  const mine = changes.files.filter((f) => !f.before_session);
  const earlier = changes.files.filter((f) => f.before_session);
  const added = mine.reduce((n, f) => n + f.added, 0);
  const removed = mine.reduce((n, f) => n + f.removed, 0);
  const row = (f: Change) => {
    const [tag, tone] = TAGS[f.status] ?? ["", ""];
    const isOpen = !!open[f.path];
    const inner = (
      <>
        {changes.git && (isOpen ? <ChevronDown size={12} strokeWidth={2.4} aria-hidden /> : <ChevronRight size={12} strokeWidth={2.4} aria-hidden />)}
        <span className="path"><bdi>{f.path}</bdi></span>
        {tag && <span className={`tag ${tone}`}>{tag}</span>}
        {f.before_session && <span className="tag">before session</span>}
        {f.binary ? (
          <span className="nums muted">binary</span>
        ) : (
          changes.git && (
            <span className="nums">
              {(f.added > 0 || f.status !== "deleted") && <span className="plus">+{f.added}</span>}
              {f.removed > 0 && <> <span className="minus">−{f.removed}</span></>}
            </span>
          )
        )}
      </>
    );
    if (!changes.git) return <div key={f.path} className="file">{inner}</div>;
    return [
      <button key={f.path} className="file" aria-expanded={isOpen} onClick={() => setOpen((o) => ({ ...o, [f.path]: !o[f.path] }))}>
        {inner}
      </button>,
      isOpen && <FileDiff key={f.path + "/diff"} s={s} file={f} />,
    ];
  };
  // An opened file gets a box of its own with its diff; the closed files around it share one.
  const boxes: Change[][] = [];
  for (const f of [...mine, ...earlier]) {
    const last = boxes[boxes.length - 1];
    if (open[f.path] || !last || open[last[0].path]) boxes.push([f]);
    else last.push(f);
  }
  return (
    <main className="page">
      <div className="changes">
        {!changes.git && (
          <div className="info" role="note">
            <Info size={16} aria-hidden />
            <span>This folder isn't a git repository, so Gwen lists the files Claude edited, without what changed in them.</span>
          </div>
        )}
        {earlier.length > 0 && (
          <div className="info" role="note">
            <Info size={16} aria-hidden />
            <span>This folder already had uncommitted changes when the session started. They are included below, marked “before session”.</span>
          </div>
        )}
        <div className="totals">
          <span>
            {mine.length} {mine.length === 1 ? "file" : "files"}
            {changes.git && mine.length > 0 && (
              <>
                {" · "}
                <span className="plus">+{added}</span> <span className="minus">−{removed}</span>
              </>
            )}
          </span>
          {changes.git && <span>{changes.head ? `vs last commit ${changes.head}` : "no commits yet"}</span>}
        </div>
        {boxes.map((b) => (
          <div key={b[0].path} className="files">
            {b.map(row)}
          </div>
        ))}
      </div>
    </main>
  );
}
