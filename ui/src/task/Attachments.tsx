import { CalendarDays, ChevronRight, ExternalLink, FileText, GitPullRequest, Link2, Mail, NotebookText, Paperclip, Plug, RefreshCw, Search, StickyNote, X, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { App, wire } from "../api";
import { Button, IconButton, Input, TextArea, cx } from "../components/ui";
import { useToast } from "../components/feedback";
import { useDaemon } from "../daemon";
import { formatDate, formatDay, formatTime } from "../format";
import { forgetConnectors, listConnectors, searchConnector } from "./connectorCache";

// What a task comes with for context, in the one task form: a link, a note, a file on this computer, or an item
// from an app through Composio (an email, an event, an issue, a page), kept with its text so an agent handed the
// task can read it. None of it is needed to make a task; it is there when the task needs it.

export const appNames: Record<string, string> = { gmail: "Gmail", googlecalendar: "Google Calendar", github: "GitHub", notion: "Notion" };
const appIcons: Record<string, LucideIcon> = { gmail: Mail, googlecalendar: CalendarDays, github: GitPullRequest, notion: NotebookText };
const kindIcons: Record<string, LucideIcon> = { link: Link2, note: StickyNote, file: FileText };
/** What each app's search finds, for its placeholder. */
const appFinds: Record<string, string> = { gmail: "Search your mail", googlecalendar: "Search your events", github: "Search issues and pull requests", notion: "Search your pages" };

/** An attachment not saved yet: on a task still being made, it is added once the task exists. */
export type PendingAttachment = wire.CreateAttachmentRequest & { key: string; snippet?: string };

function iconOf(a: { kind: string; app: string }): LucideIcon {
  return a.kind === "app" ? (appIcons[a.app] ?? Plug) : (kindIcons[a.kind] ?? Paperclip);
}

/** A task's attachments, loaded and kept current; null while loading. */
export function useAttachments(taskId: string | null | undefined): wire.Attachment[] | null {
  const d = useDaemon();
  const [list, setList] = useState<wire.Attachment[] | null>(null);
  useEffect(() => {
    if (!taskId) {
      setList([]);
      return;
    }
    let live = true;
    App.ListAttachments(taskId).then(
      (l) => live && setList(l.attachments),
      () => live && setList([]),
    );
    return () => {
      live = false;
    };
  }, [taskId, d.tasksVersion]);
  return list;
}

/** The attachments as context for an agent's brief: links and files by where they are, notes and app items with their text. */
export function attachmentsBrief(list: wire.Attachment[]): string {
  if (list.length === 0) return "";
  // Each text is cut so a few long emails still leave the brief a sensible size.
  const each = Math.max(1500, Math.floor(24000 / list.length));
  const cut = (s: string) => (s.length > each ? `${s.slice(0, each)}\n[… cut; ${s.length - each} more characters]` : s);
  const parts = list.map((a) => {
    const head =
      a.kind === "link"
        ? `- Link: ${a.title} <${a.url}>`
        : a.kind === "file"
          ? `- File: ${a.path}`
          : a.kind === "note"
            ? `- Note: ${a.title}`
            : `- ${appNames[a.app] ?? a.app}: ${a.title}${a.url ? ` <${a.url}>` : ""}${a.fetched_at ? ` (as of ${formatDate(formatDay(new Date(a.fetched_at)), false)})` : ""}`;
    const body = a.kind === "note" || a.kind === "app" ? a.body.trim() : "";
    return body ? `${head}\n\n${cut(body).replace(/^/gm, "    ")}` : head;
  });
  return `Context attached to this task:\n\n${parts.join("\n\n")}`;
}

/**
 * The form's Attachments: what is attached, and four ways to add more. On a saved task each change is saved
 * at once; on a new one they wait in pending until the task is made.
 */
export function AttachmentsField({ taskId, pending, onPending }: { taskId: string | null; pending: PendingAttachment[]; onPending: (p: PendingAttachment[]) => void }) {
  const d = useDaemon();
  const saved = useAttachments(taskId);
  const [adding, setAdding] = useState<"link" | "note" | "app" | null>(null);
  const [busy, setBusy] = useState(false);

  async function add(req: wire.CreateAttachmentRequest, snippet?: string) {
    if (!taskId) {
      onPending([...pending, { ...req, key: `${Date.now()}-${pending.length}`, snippet }]);
      return true;
    }
    setBusy(true);
    const ok = await d.act(() => App.CreateAttachment(taskId, wire.CreateAttachmentRequest.createFrom(req)));
    setBusy(false);
    return !!ok;
  }
  async function pickFile() {
    const path = await App.PickFile().catch((e) => {
      d.fail(e);
      return "";
    });
    if (path) await add({ kind: "file", path, title: "", url: "", app: "", ref: "", body: "" });
  }

  const items = [...(saved ?? []).map((a) => ({ saved: a, pending: null })), ...pending.map((p) => ({ saved: null, pending: p }))];
  const appKeys = new Set(items.map((x) => (x.saved ? `${x.saved.app}:${x.saved.ref}` : `${x.pending!.app}:${x.pending!.ref}`)));

  return (
    <div className="flex flex-col gap-2">
      {items.length > 0 && (
        <ul className="overflow-hidden rounded-lg border border-line">
          {items.map((x, i) =>
            x.saved ? (
              <SavedRow key={x.saved.id} a={x.saved} first={i === 0} />
            ) : (
              <PendingRow key={x.pending!.key} p={x.pending!} first={i === 0} onRemove={() => onPending(pending.filter((p) => p.key !== x.pending!.key))} />
            ),
          )}
        </ul>
      )}
      <div className="flex flex-wrap items-center gap-1">
        <Button size="sm" tone={adding === "link" ? "secondary" : "ghost"} icon={Link2} onClick={() => setAdding(adding === "link" ? null : "link")}>
          Link
        </Button>
        <Button size="sm" tone={adding === "note" ? "secondary" : "ghost"} icon={StickyNote} onClick={() => setAdding(adding === "note" ? null : "note")}>
          Note
        </Button>
        <Button size="sm" tone="ghost" icon={FileText} onClick={pickFile} disabled={busy}>
          File
        </Button>
        <Button size="sm" tone={adding === "app" ? "secondary" : "ghost"} icon={Plug} onClick={() => setAdding(adding === "app" ? null : "app")}>
          From an app
        </Button>
      </div>
      {adding === "link" && <LinkAdder busy={busy} onAdd={async (url, title) => (await add({ kind: "link", url, title, path: "", app: "", ref: "", body: "" })) && setAdding(null)} onCancel={() => setAdding(null)} />}
      {adding === "note" && <NoteAdder busy={busy} onAdd={async (body) => (await add({ kind: "note", body, title: "", url: "", path: "", app: "", ref: "" })) && setAdding(null)} onCancel={() => setAdding(null)} />}
      {adding === "app" && (
        <AppPicker
          attached={appKeys}
          onPick={(it) => add({ kind: "app", app: it.app, ref: it.ref, title: it.title, url: it.url, path: "", body: "" }, it.snippet)}
          onClose={() => setAdding(null)}
        />
      )}
    </div>
  );
}

/** A saved attachment: open it, read what was kept, refresh an app item, or remove it. */
function SavedRow({ a, first }: { a: wire.Attachment; first: boolean }) {
  const d = useDaemon();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const Icon = iconOf(a);
  const hasText = (a.kind === "note" || a.kind === "app") && !!a.body;
  const sub =
    a.kind === "app"
      ? [appNames[a.app] ?? a.app, a.fetched_at ? `kept ${formatDate(formatDay(new Date(a.fetched_at)), false)}, ${formatTime(a.fetched_at)}` : null].filter(Boolean).join(" · ")
      : a.kind === "link"
        ? a.url
        : a.kind === "file"
          ? a.path
          : null;
  async function refresh() {
    setBusy(true);
    await d.act(() => App.RefreshAttachment(a.id));
    setBusy(false);
  }
  return (
    <li className={cx(!first && "border-t border-chrome-line")}>
      <div className="flex items-center gap-2.5 py-1.5 pr-2 pl-3">
        <Icon size={14} className="shrink-0 text-ink-subtle" aria-hidden />
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] text-ink" title={a.title}>
            {a.title}
          </div>
          {sub && <div className="truncate text-[11.5px] text-ink-subtle">{sub}</div>}
        </div>
        {hasText && <IconButton size="sm" icon={ChevronRight} label={open ? "Hide the text" : "Show the text"} active={open} onClick={() => setOpen(!open)} className={cx(open && "[&_svg]:rotate-90")} />}
        {a.url && <IconButton size="sm" icon={ExternalLink} label={`Open ${a.title}`} onClick={() => d.act(() => App.OpenURL(a.url))} />}
        {a.kind === "file" && <IconButton size="sm" icon={ExternalLink} label={`Open ${a.title}`} onClick={() => d.act(() => App.OpenFile(a.path))} />}
        {a.kind === "app" && <IconButton size="sm" icon={RefreshCw} label="Fetch it again" onClick={refresh} disabled={busy} className={cx(busy && "[&_svg]:animate-spin")} />}
        <IconButton size="sm" icon={X} label={`Remove ${a.title}`} onClick={() => d.act(() => App.DeleteAttachment(a.id))} />
      </div>
      {open && hasText && <pre className="mx-3 mb-2 max-h-56 overflow-y-auto rounded-md bg-surface-2 px-3 py-2 font-sans text-[12px] leading-relaxed whitespace-pre-wrap text-ink-muted">{a.body}</pre>}
    </li>
  );
}

function PendingRow({ p, first, onRemove }: { p: PendingAttachment; first: boolean; onRemove: () => void }) {
  const Icon = iconOf(p);
  const title = p.title || (p.kind === "link" ? p.url : p.kind === "file" ? p.path.split("/").pop() : p.kind === "note" ? p.body.split("\n")[0] : p.ref);
  const sub = p.kind === "app" ? [appNames[p.app] ?? p.app, "its text is kept when you save"].join(" · ") : p.kind === "link" ? p.url : p.kind === "file" ? p.path : null;
  return (
    <li className={cx("flex items-center gap-2.5 py-1.5 pr-2 pl-3", !first && "border-t border-chrome-line")}>
      <Icon size={14} className="shrink-0 text-ink-subtle" aria-hidden />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] text-ink">{title}</div>
        {sub && <div className="truncate text-[11.5px] text-ink-subtle">{sub}</div>}
      </div>
      <IconButton size="sm" icon={X} label={`Remove ${title}`} onClick={onRemove} />
    </li>
  );
}

function LinkAdder({ busy, onAdd, onCancel }: { busy: boolean; onAdd: (url: string, title: string) => void; onCancel: () => void }) {
  const [url, setUrl] = useState("");
  const [title, setTitle] = useState("");
  const valid = /^https?:\/\/\S+\.\S+/.test(url.trim());
  const submit = () => valid && onAdd(url.trim(), title.trim());
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-line bg-surface-2/50 p-2.5">
      <Input autoFocus value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…" aria-label="Link" onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), submit())} />
      <div className="flex items-center gap-2">
        <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="What it is (optional)" aria-label="What the link is" className="flex-1" onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), submit())} />
        <Button size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" tone="primary" onClick={submit} disabled={!valid} busy={busy}>
          Attach
        </Button>
      </div>
    </div>
  );
}

function NoteAdder({ busy, onAdd, onCancel }: { busy: boolean; onAdd: (body: string) => void; onCancel: () => void }) {
  const [body, setBody] = useState("");
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-line bg-surface-2/50 p-2.5">
      <TextArea autoFocus rows={3} value={body} onChange={(e) => setBody(e.target.value)} placeholder="Background, decisions, what done looks like…" aria-label="Note" className="resize-y" />
      <div className="flex justify-end gap-2">
        <Button size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" tone="primary" onClick={() => body.trim() && onAdd(body.trim())} disabled={!body.trim()} busy={busy}>
          Attach
        </Button>
      </div>
    </div>
  );
}

/** Find something in an app and attach it: pick the app, search (or see the latest), attach any result. */
function AppPicker({ attached, onPick, onClose }: { attached: Set<string>; onPick: (it: wire.ConnectorItem) => Promise<boolean>; onClose: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [connectors, setConnectors] = useState<wire.Connector[] | null>(null);
  const [app, setApp] = useState<string>("");
  const [q, setQ] = useState("");
  const [items, setItems] = useState<wire.ConnectorItem[] | null>(null);
  const [error, setError] = useState("");
  const [searching, setSearching] = useState(false);
  const [adding, setAdding] = useState<string | null>(null);
  const run = useRef(0);

  const [connecting, setConnecting] = useState(false);
  const loadConnectors = (fresh?: boolean) =>
    listConnectors(fresh).then(
      (l) => {
        setConnectors(l.connectors);
        setApp((cur) => cur || l.connectors.find((c) => c.status === "active")?.app || l.connectors[0]?.app || "");
      },
      (e) => setError(String(e?.message ?? e)),
    );
  useEffect(() => {
    loadConnectors();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  async function connect(app: string) {
    forgetConnectors();
    setConnecting(true);
    const link = await d.act(() => App.LinkConnector(app));
    if (link) {
      try {
        await App.OpenURL(link.url);
        notify(`${appNames[app] ?? "The app"}'s sign-in is open in your browser. Come back here and press Check again when you're done.`);
      } catch (e) {
        d.fail(e);
      }
    }
    setConnecting(false);
  }

  const current = connectors?.find((c) => c.app === app);
  async function search(query: string, fresh?: boolean) {
    if (!current || current.status !== "active") return;
    const n = ++run.current;
    setSearching(true);
    setError("");
    try {
      const r = await searchConnector(app, query, fresh);
      if (n === run.current) setItems(r.items);
    } catch (e) {
      if (n === run.current) {
        setItems(null);
        setError(String((e as { message?: string })?.message ?? e));
      }
    } finally {
      if (n === run.current) setSearching(false);
    }
  }
  // Switching app shows its latest items.
  useEffect(() => {
    setItems(null);
    setQ("");
    if (current?.status === "active") search("");
  }, [app, current?.status]); // eslint-disable-line react-hooks/exhaustive-deps

  const unavailable = connectors != null && connectors.every((c) => c.status === "unavailable");
  return (
    <div className="flex flex-col gap-2.5 rounded-lg border border-line bg-surface-2/50 p-2.5">
      <div className="flex items-center gap-1.5">
        <div role="tablist" aria-label="App" className="flex min-w-0 flex-1 flex-wrap gap-1">
          {(connectors ?? []).map((c) => {
            const Icon = appIcons[c.app] ?? Plug;
            return (
              <button
                key={c.app}
                type="button"
                role="tab"
                aria-selected={app === c.app}
                onClick={() => setApp(c.app)}
                className={cx(
                  "inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[12.5px]",
                  app === c.app ? "border-line-3 bg-surface-4 text-ink" : "border-transparent text-ink-muted hover:bg-surface-3 hover:text-ink",
                )}
              >
                <Icon size={13} aria-hidden />
                {appNames[c.app] ?? c.name}
                {c.status !== "active" && <span className="size-1.5 rounded-full bg-idle" title={c.status === "expired" ? "Needs reconnecting" : "Not connected"} />}
              </button>
            );
          })}
        </div>
        <IconButton size="sm" icon={X} label="Close the app search" onClick={onClose} />
      </div>
      {connectors == null && !error && <p className="px-1 text-[12px] text-ink-subtle">Checking your apps…</p>}
      {unavailable && <p className="px-1 text-[12px] leading-relaxed text-ink-subtle">Attaching from apps needs Composio on this computer. Links, notes, and files work without it.</p>}
      {current && !unavailable && current.status !== "active" && (
        <div className="flex flex-wrap items-center gap-2 px-1">
          <p className="min-w-0 flex-1 text-[12px] leading-relaxed text-ink-subtle">
            {appNames[current.app]} {current.status === "expired" ? "was connected, but Composio's access to it has expired." : "isn't connected to Composio yet."} Connect opens its sign-in in your browser; come back here when it's done.
          </p>
          <Button size="sm" tone="primary" icon={ExternalLink} busy={connecting} onClick={() => connect(current.app)}>
            Connect
          </Button>
          <Button size="sm" icon={RefreshCw} onClick={() => { forgetConnectors(); loadConnectors(true); }}>
            Check again
          </Button>
        </div>
      )}
      {current?.status === "active" && (
        <>
          <div className="flex items-center gap-1.5">
            <label className="relative flex min-w-0 flex-1 items-center">
              <Search size={14} className="pointer-events-none absolute left-2.5 text-ink-subtle" aria-hidden />
              <input
                autoFocus
                value={q}
                onChange={(e) => setQ(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    search(q.trim());
                  }
                }}
                placeholder={`${appFinds[app] ?? "Search"}, then press Enter`}
                aria-label={appFinds[app] ?? "Search"}
                className="field !h-8 w-full !pl-8 !text-[13px]"
              />
            </label>
            <IconButton icon={RefreshCw} size="sm" label="Look again" onClick={() => search(q.trim(), true)} disabled={searching} />
          </div>
          {searching && <p className="px-1 text-[12px] text-ink-subtle">Looking in {appNames[app]}…</p>}
          {!searching && items && items.length === 0 && <p className="px-1 text-[12px] text-ink-subtle">Nothing found{q.trim() ? ` for “${q.trim()}”` : ""}.</p>}
          {!searching && items && items.length > 0 && (
            <ul className="max-h-64 overflow-y-auto rounded-md border border-line bg-surface-1">
              {items.map((it, i) => {
                const key = `${it.app}:${it.ref}`;
                const done = attached.has(key);
                return (
                  <li key={key} className={cx("flex items-start gap-2.5 px-3 py-2", i > 0 && "border-t border-chrome-line")}>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-[13px] text-ink" title={it.title}>
                        {it.title || "(no title)"}
                      </div>
                      <div className="truncate text-[11.5px] text-ink-subtle">{[it.when ? `${formatDate(formatDay(new Date(it.when)), false)}` : null, it.snippet].filter(Boolean).join(" · ")}</div>
                    </div>
                    <Button
                      size="sm"
                      tone={done ? "ghost" : "secondary"}
                      disabled={done || adding != null}
                      busy={adding === key}
                      onClick={async () => {
                        setAdding(key);
                        await onPick(it);
                        setAdding(null);
                      }}
                    >
                      {done ? "Attached" : "Attach"}
                    </Button>
                  </li>
                );
              })}
            </ul>
          )}
        </>
      )}
      {error && <p className="px-1 text-[12px] leading-relaxed text-danger">{error}</p>}
    </div>
  );
}
