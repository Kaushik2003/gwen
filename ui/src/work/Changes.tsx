import { Check, ExternalLink, GitBranch, GitCommitHorizontal, LoaderCircle, RefreshCw, X } from "lucide-react";
import { useMemo, useRef, useState, type FormEvent } from "react";
import { apiError } from "../api";
import { Button, Callout, Empty, IconButton, Modal, cx } from "../components/ui";
import { baseName, count, dirName, git, notRepo, parseDiff, statusLook, useLoad, type FileChange } from "./repo";

// The side panel's Changes (plan.md 3.15, board 2): the uncommitted files with their status and
// +/−, the picked file's diff, and a commit that first shows exactly which files go in.

export default function Changes({ folder, onOpenFile }: { folder: string; onOpenFile?: (path: string) => void }) {
  const work = useLoad(`changes:${folder}`, () => git.commit(folder, ""));
  const [pick, setPick] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [needMessage, setNeedMessage] = useState(false);
  const [confirm, setConfirm] = useState<{ files: FileChange[]; message: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [commitError, setCommitError] = useState("");
  const [committed, setCommitted] = useState("");
  const input = useRef<HTMLInputElement>(null);

  const files = work.data?.changes ?? [];
  const file = files.find((f) => f.path === pick) ?? files[0] ?? null;
  const branch = work.data?.refs.find((r) => r.kind === "branch")?.name;

  const ask = (e: FormEvent) => {
    e.preventDefault();
    if (!message.trim()) {
      setNeedMessage(true);
      input.current?.focus();
      return;
    }
    setCommitError("");
    setConfirm({ files, message: message.trim() });
  };
  const commit = () => {
    if (!confirm) return;
    setBusy(true);
    setCommitError("");
    // A rename commits both names: the new file and the old one's removal.
    const paths = confirm.files.flatMap((f) => (f.old_path ? [f.path, f.old_path] : [f.path]));
    git.commitFiles(folder, confirm.message, paths).then(
      (short) => {
        setBusy(false);
        setConfirm(null);
        setMessage("");
        setCommitted(short);
        setPick(null);
        work.reload();
      },
      (err) => {
        setBusy(false);
        setCommitError(apiError(err).message);
      },
    );
  };

  if (work.loading && !work.data)
    return (
      <p className="flex items-center gap-2 px-4 py-4 text-[13px] text-ink-subtle">
        <LoaderCircle size={14} className="animate-spin" aria-hidden />
        Looking for changes…
      </p>
    );
  if (work.error && !work.data)
    return notRepo(work.error) ? (
      <Empty icon={GitBranch} title="Not a git folder" className="mx-4 my-4 py-8">
        This project's folder has no git repository, so there is nothing to commit.
      </Empty>
    ) : (
      <Callout tone="danger" className="m-4" action={<IconButton icon={RefreshCw} size="sm" label="Try again" onClick={work.reload} />}>
        {work.error.message}
      </Callout>
    );

  const added = work.data?.added ?? 0;
  const removed = work.data?.removed ?? 0;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-2 px-4 pt-3 pb-2.5 text-xs text-ink-muted">
        <GitBranch size={12} aria-hidden className="shrink-0 text-ink-subtle" />
        <span className="min-w-0 truncate font-mono">{branch ?? "detached HEAD"}</span>
        <span className="ml-auto shrink-0">
          {files.length} {files.length === 1 ? "file" : "files"}
          {(added > 0 || removed > 0) && " · "}
          <Counts added={added} removed={removed} />
        </span>
        <IconButton icon={RefreshCw} size="sm" label="Refresh" onClick={work.reload} className="-mr-2 !size-6" />
      </div>

      {committed && (
        <Callout tone="working" icon={Check} className="mx-4 mb-2" action={<IconButton icon={X} size="sm" label="Dismiss" onClick={() => setCommitted("")} className="!size-6" />}>
          Committed <span className="font-mono">{committed}</span>.
        </Callout>
      )}

      {files.length === 0 ? (
        <Empty icon={GitCommitHorizontal} title="Nothing to commit" className="mx-4 mb-4 py-8">
          Every file matches the last commit.
        </Empty>
      ) : (
        <>
          <div className="max-h-[40%] shrink-0 overflow-y-auto px-2" role="list" aria-label="Uncommitted files">
            {files.map((f) => {
              const look = statusLook[f.status];
              const on = f === file;
              return (
                <button
                  key={f.path}
                  type="button"
                  role="listitem"
                  aria-current={on ? "true" : undefined}
                  onClick={() => setPick(f.path)}
                  title={f.old_path ? `${f.path}, renamed from ${f.old_path}` : f.path}
                  className={cx("flex h-[30px] w-full items-center gap-2 rounded-[7px] px-2 text-left", on ? "bg-surface-2 text-ink" : "text-ink/85 hover:bg-surface-1")}
                >
                  <span className={cx("w-4 shrink-0 font-mono text-[11px]", look?.text)} title={look?.label}>
                    {f.status}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-mono text-xs" dir="rtl">
                    {/* Right to left keeps the file name in view when the path is cut. */}
                    <bdi>{f.path}</bdi>
                  </span>
                  <span className="shrink-0 font-mono text-[11px]">{f.binary ? <span className="text-ink-subtle">bin</span> : <Counts added={f.added} removed={f.removed} />}</span>
                </button>
              );
            })}
          </div>
          {file && <ChangeDiff key={file.path} folder={folder} file={file} onOpenFile={onOpenFile} />}
        </>
      )}

      {files.length > 0 && (
        <form onSubmit={ask} className="mt-auto px-4 pt-3 pb-3.5">
          <label htmlFor={`commit-${folder}`} className="sr-only">
            Commit message
          </label>
          <div className="flex gap-2">
            <input
              id={`commit-${folder}`}
              ref={input}
              value={message}
              onChange={(e) => {
                setMessage(e.target.value);
                setNeedMessage(false);
              }}
              placeholder="Commit message"
              aria-invalid={needMessage || undefined}
              className={cx("field !h-8 flex-1", needMessage && "!border-danger")}
            />
            <Button type="submit" size="md" className="!h-8">
              Commit
            </Button>
          </div>
          {needMessage && <p className="mt-1.5 text-xs text-danger">Write a message first: what these changes do.</p>}
        </form>
      )}

      {confirm && (
        <Modal
          title={`Commit ${confirm.files.length} ${confirm.files.length === 1 ? "file" : "files"}?`}
          description={
            <>
              To <span className="font-mono">{branch ?? "the detached HEAD"}</span> with the message “{confirm.message}”. Exactly these files go in; anything else you change stays out.
            </>
          }
          onClose={() => !busy && setConfirm(null)}
          footer={
            <>
              <Button tone="ghost" onClick={() => setConfirm(null)} disabled={busy}>
                Cancel
              </Button>
              <Button tone="primary" icon={GitCommitHorizontal} busy={busy} onClick={commit} autoFocus>
                Commit {confirm.files.length} {confirm.files.length === 1 ? "file" : "files"}
              </Button>
            </>
          }
        >
          <ul className="flex flex-col gap-0.5 rounded-lg border border-line bg-canvas p-2">
            {confirm.files.map((f) => (
              <li key={f.path} className="flex items-center gap-2 px-1 py-0.5 text-[12.5px]">
                <span className={cx("w-4 shrink-0 font-mono text-[11px]", statusLook[f.status]?.text)}>{f.status}</span>
                <span className="min-w-0 flex-1 font-mono text-xs break-all text-ink">
                  {f.path}
                  {f.old_path && <span className="text-ink-subtle"> (from {f.old_path})</span>}
                </span>
                <span className="shrink-0 text-xs text-ink-subtle">{f.status === "U" ? "New file" : (statusLook[f.status]?.label ?? f.status)}</span>
              </li>
            ))}
          </ul>
          {commitError && (
            <Callout tone="danger" className="mt-3">
              The commit failed: {commitError}
            </Callout>
          )}
        </Modal>
      )}
    </div>
  );
}

function Counts({ added, removed }: { added: number; removed: number }) {
  return (
    <span className="font-mono">
      {added > 0 && <span className="text-success">+{count(added)}</span>}
      {added > 0 && removed > 0 && " "}
      {removed > 0 && <span className="text-danger">−{count(removed)}</span>}
    </span>
  );
}

/** One file's uncommitted diff, compact: no line numbers, long lines wrapped. */
function ChangeDiff({ folder, file, onOpenFile }: { folder: string; file: FileChange; onOpenFile?: (path: string) => void }) {
  const diff = useLoad(`diff:${folder}\n${file.path}`, () => git.diff(folder, "", file.path));
  const parsed = useMemo(() => (diff.data ? parseDiff(diff.data.text) : null), [diff.data]);
  const what =
    file.status === "U" || file.status === "A" ? "new file" : file.status === "D" ? "deleted" : file.old_path ? `renamed from ${baseName(file.old_path)}` : dirName(file.path) || "changed";
  return (
    <div className="mx-4 mt-3 flex min-h-[120px] flex-1 flex-col overflow-hidden rounded-[10px] border border-line bg-canvas font-mono text-[11.5px] leading-[1.65]">
      <div className="flex shrink-0 items-center gap-2 border-b border-chrome-line py-1 pr-1 pl-2.5 text-[11px] text-ink-subtle">
        <span className="min-w-0 truncate">
          <span className="text-ink-muted">{baseName(file.path)}</span> · {what}
        </span>
        <span className="flex-1" />
        {onOpenFile && file.status !== "D" && <IconButton icon={ExternalLink} size="sm" label={`Open ${baseName(file.path)}`} className="!size-6" onClick={() => onOpenFile(file.path)} />}
      </div>
      <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto py-1 [tab-size:4]" role="region" aria-label={`Changes in ${file.path}`}>
        {diff.loading && !diff.data ? (
          <p className="px-2.5 py-1 font-sans text-xs text-ink-subtle">Reading the diff…</p>
        ) : diff.error && !diff.data ? (
          <p className="px-2.5 py-1 font-sans text-xs text-danger">{diff.error.message}</p>
        ) : !parsed || parsed.binary || file.binary ? (
          <p className="px-2.5 py-1 font-sans text-xs text-ink-subtle">A binary file; no text to compare.</p>
        ) : !parsed.hunks.length ? (
          <p className="px-2.5 py-1 font-sans text-xs text-ink-subtle">{parsed.renamedFrom ? "Renamed; the text is the same." : parsed.modeChange ? `Mode changed: ${parsed.modeChange}.` : "No line changes."}</p>
        ) : (
          <>
            {parsed.hunks.map((h, i) => (
              <div key={i}>
                {(i > 0 || h.lines[0]?.old !== 1 || parsed.hunks.length > 1) && !parsed.isNew && (
                  <div className="truncate px-2.5 pt-1 text-[11px] text-info/70">
                    {h.header} {h.context}
                  </div>
                )}
                {h.lines.map((l, j) => (
                  <div
                    key={j}
                    className={cx(
                      "px-2.5 break-words whitespace-pre-wrap [overflow-wrap:anywhere]",
                      l.kind === "add" ? "bg-success/8 text-ink" : l.kind === "del" ? "bg-danger/8 text-ink" : "text-ink-muted",
                    )}
                  >
                    <span className={cx("inline-block w-3 select-none", l.kind === "add" ? "text-success" : l.kind === "del" ? "text-danger" : "")}>{l.kind === "add" ? "+" : l.kind === "del" ? "−" : " "}</span>
                    {l.text}
                  </div>
                ))}
              </div>
            ))}
            {diff.data?.truncated && <p className="px-2.5 pt-1 font-sans text-xs text-ink-subtle">The diff goes on; open the file to see it all.</p>}
          </>
        )}
      </div>
    </div>
  );
}
