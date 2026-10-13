import { Check, CircleAlert, Ellipsis, Eye, EyeOff, FileText, LoaderCircle, MessageSquare, NotebookPen, Pencil, ThumbsDown, Trash2, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { App, apiError, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";
import { historyLine, kindLabel, plainValue, receiptText, shortDate, threadParam, toConfirm, trustLabel, trustMeans, useMemoryVersion, whenText } from "../memory";
import { useNav } from "../nav";
import { useConfirm, useToast } from "./feedback";
import { Badge, Button, Callout, Field, IconButton, Input, Modal, TextArea, cx } from "./ui";

/** The assistant's name, as the chat shows it. */
export function useAssistantName(): string {
  return useDaemon().config?.llm.assistant_name || "Gwen";
}

/** What can be done to a claim, from a row's menu, the review list, a chat chip, or the detail; each resolves true when it was done. */
export interface ClaimActions {
  keep: (c: wire.MemoryClaim) => Promise<boolean>;
  reject: (c: wire.MemoryClaim) => Promise<boolean>;
  hide: (c: wire.MemoryClaim, hidden: boolean) => Promise<boolean>;
  forget: (c: wire.MemoryClaim) => Promise<boolean>;
  /** Opens the editor; render editor for it to show. */
  edit: (c: wire.MemoryClaim) => void;
  editor: ReactNode;
}

/** The claim actions with their toasts and confirmations; onChanged runs after any of them changed something. */
export function useClaimActions(onChanged?: () => void): ClaimActions {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const name = useAssistantName();
  const [editing, setEditing] = useState<wire.MemoryClaim | null>(null);
  const changed = (ok: unknown) => {
    if (ok) onChanged?.();
    return !!ok;
  };
  return {
    keep: async (c) => {
      const ok = await d.act(() => App.ConfirmClaim(c.id));
      if (ok) notify(`${name} will keep this: ${c.text}`, "success");
      return changed(ok);
    },
    reject: async (c) => {
      const ok = await d.act(() => App.RejectClaim(c.id));
      if (ok) notify(`Got it. ${name} won't hold on to that, and won't learn it again.`, "success");
      return changed(ok);
    },
    hide: async (c, hidden) => {
      const ok = await d.act(() => App.HideClaim(c.id, wire.HideClaimRequest.createFrom({ hidden })));
      if (ok) notify(hidden ? `${name} won't bring this up. She still knows it.` : `${name} may bring this up again.`, "success");
      return changed(ok);
    },
    forget: async (c) => {
      const sure = await confirm({
        title: "Forget this for good?",
        body: (
          <>
            <span className="text-ink">{c.text}</span>
            <span className="mt-2 block">
              {name} erases it everywhere she keeps it, and won't learn it again from the same words. This can't be undone. To keep it but stop her mentioning it, use Don't bring this up
              instead.
            </span>
          </>
        ),
        confirm: "Forget",
        danger: true,
      });
      if (!sure) return false;
      const r = await d.act(() => App.ForgetClaim(c.id));
      if (r) notify(receiptText(r, "Forgotten.", name), "success");
      return changed(r);
    },
    edit: setEditing,
    editor: editing && (
      <EditClaim
        claim={editing}
        onClose={() => setEditing(null)}
        onSaved={() => {
          setEditing(null);
          onChanged?.();
        }}
      />
    ),
  };
}

/** Fixes a claim: the value of a one-valued plain fact, else the whole statement. */
function EditClaim({ claim, onClose, onSaved }: { claim: wire.MemoryClaim; onClose: () => void; onSaved: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const name = useAssistantName();
  const value = plainValue(claim);
  const [text, setText] = useState(value ?? claim.text);
  const [saving, setSaving] = useState(false);
  const changed = text.trim() !== "" && text.trim() !== (value ?? claim.text);

  async function save() {
    if (!changed) return;
    setSaving(true);
    const req = value != null ? wire.EditClaimRequest.createFrom({ text: "", value: text.trim() }) : wire.EditClaimRequest.createFrom({ text: text.trim() });
    const r = await d.act(() => App.EditClaim(claim.id, req));
    setSaving(false);
    if (!r) return;
    const now = (r.claims ?? []).map((c) => c.text);
    notify(now.length ? `Saved. ${name} now remembers: ${now.join("; ")}.` : "Saved.", "success");
    onSaved();
  }

  return (
    <Modal
      title={`Fix what ${name} knows`}
      description={value != null ? `The old value is kept as history, so ${name} knows it changed.` : `${name} remembers it the way you write it here.`}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" busy={saving} disabled={!changed} onClick={save}>
            Save
          </Button>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save();
        }}
      >
        {value != null ? (
          <Field label={claim.subject ? `${claim.subject}: ${claim.label}` : claim.label}>
            <Input value={text} onChange={(e) => setText(e.target.value)} maxLength={200} autoFocus />
          </Field>
        ) : (
          <Field label={`What ${name} should remember`}>
            <TextArea
              rows={3}
              value={text}
              onChange={(e) => setText(e.target.value)}
              maxLength={500}
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  save();
                }
              }}
            />
          </Field>
        )}
      </form>
    </Modal>
  );
}

/** Who a claim rests on, in words: You set it, You said it, Gwen noticed, Gwen inferred, From a document. */
export function TrustBadge({ trust }: { trust: string }) {
  const name = useAssistantName();
  const label = trustLabel(trust, name);
  if (!label) return null;
  return (
    <Badge tone={trust === "owner_edit" || trust === "owner_said" ? "neutral" : "accent"} title={trustMeans(trust, name)}>
      {label}
    </Badge>
  );
}

/** The badges of a claim's state: not sure while disputed, hidden, a past value, or waiting for you. */
export function StateBadges({ claim }: { claim: wire.MemoryClaim }) {
  const name = useAssistantName();
  return (
    <>
        {claim.status === "candidate" && <Badge tone="idle">To confirm</Badge>}
        {claim.status === "quarantined" && <Badge tone="idle">To confirm</Badge>}
        {claim.status === "superseded" && <Badge>Past value</Badge>}
        {claim.status === "rejected" && <Badge tone="danger">Not true</Badge>}
        {claim.disputed && (
          <Badge tone="idle" icon={CircleAlert} title={`${name} holds something else that disagrees with this.`}>
            Not sure
          </Badge>
        )}
        {claim.hidden && (
          <Badge icon={EyeOff} title={`${name} knows it but won't bring it up.`}>
            Hidden
          </Badge>
        )}
    </>
  );
}

/** Where something came from; with a chat, it opens that chat on the Assistant screen. */
export function SourceChip({ source, threadId, onOpen }: { source: string; threadId?: string; onOpen?: () => void }) {
  const { go } = useNav();
  if (!source) return null;
  const Icon = source.startsWith("document") ? FileText : source.startsWith("chat") ? MessageSquare : NotebookPen;
  const look = "inline-flex max-w-full min-w-0 items-center gap-1 rounded-md border border-line bg-surface-2 px-1.5 py-0.5 text-[11px] text-ink-subtle";
  const body = (
    <>
      <Icon size={11} className="shrink-0" aria-hidden />
      <span className="truncate">{source}</span>
    </>
  );
  if (!threadId) return <span className={look}>{body}</span>;
  return (
    <button
      type="button"
      title="Open this chat"
      onClick={(e) => {
        e.stopPropagation();
        onOpen?.();
        go("assistant", threadParam(threadId));
      }}
      className={cx(look, "transition-colors hover:border-line-3 hover:text-ink")}
    >
      {body}
    </button>
  );
}

/** A claim row's menu: Edit, Don't bring this up or Bring it up again, Not true, Forget. */
export function ClaimMenu({ claim, actions }: { claim: wire.MemoryClaim; actions: ClaimActions }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        setOpen(false);
      }
    };
    window.addEventListener("mousedown", away);
    window.addEventListener("keydown", key, true);
    return () => {
      window.removeEventListener("mousedown", away);
      window.removeEventListener("keydown", key, true);
    };
  }, [open]);
  const item = (icon: LucideIcon, label: string, run: () => void, danger = false) => {
    const Icon = icon;
    return (
      <button
        type="button"
        role="menuitem"
        onClick={() => {
          setOpen(false);
          run();
        }}
        className={cx("flex h-8 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] transition-colors hover:bg-surface-4", danger ? "text-danger" : "text-ink-muted hover:text-ink")}
      >
        <Icon size={14} aria-hidden />
        {label}
      </button>
    );
  };
  return (
    <div ref={box} className="relative" onClick={(e) => e.stopPropagation()}>
      <IconButton icon={Ellipsis} label="More" size="sm" active={open} onClick={() => setOpen(!open)} aria-haspopup="menu" aria-expanded={open} />
      {open && (
        <div role="menu" className="animate-pop absolute top-8 right-0 z-30 flex w-56 flex-col rounded-lg border border-line-strong bg-surface-3 p-1 shadow-xl shadow-black/60">
          {item(Pencil, "Edit", () => actions.edit(claim))}
          {claim.hidden ? item(Eye, "Bring it up again", () => actions.hide(claim, false)) : item(EyeOff, "Don't bring this up", () => actions.hide(claim, true))}
          {item(ThumbsDown, "Not true", () => actions.reject(claim))}
          <div className="my-1 h-px bg-line" />
          {item(Trash2, "Forget", () => actions.forget(claim), true)}
        </div>
      )}
    </div>
  );
}

/**
 * Why Gwen holds something: the words it rests on, the messages around
 * them, the values it had before, and what can be done about it. refId is a
 * ref (claim:, msg:, ep:, doc:) or a claim id; threadId is the chat it came
 * from, when the caller knows it.
 */
export default function MemoryDetail({ refId, threadId, onClose }: { refId: string; threadId?: string; onClose: () => void }) {
  const name = useAssistantName();
  const version = useMemoryVersion();
  const ref = refId.includes(":") ? refId : `claim:${refId}`;
  const [x, setX] = useState<wire.MemoryExplanation | null>(null);
  const [error, setError] = useState<{ gone: boolean; message: string } | null>(null);
  const [reload, setReload] = useState(0);
  const actions = useClaimActions(() => setReload((n) => n + 1));

  useEffect(() => {
    let live = true;
    App.ExplainMemory(ref).then(
      (e) => {
        if (!live) return;
        setX(e);
        setError(null);
      },
      (e) => {
        if (!live) return;
        const err = apiError(e);
        setError({ gone: err.code === "not_found", message: err.message });
      },
    );
    return () => {
      live = false;
    };
  }, [ref, version, reload]);

  const claim = x?.claim ?? null;
  const kind = ref.split(":")[0];
  const quote = x?.quote || claim?.quote || "";
  const sourceMessage = messageIdOf(claim ? claim.source_ref : ref);
  const history = (x?.history ?? []).filter((h) => h.id !== claim?.id);
  const chat = claim?.thread_id || threadId || "";
  const title = claim ? claim.text : x ? kindLabel(kind) : error ? "Not in memory" : "Loading";

  async function run(done: Promise<boolean>, close = false) {
    if ((await done) && close) onClose();
  }

  const footer = claim && claim.status !== "rejected" && (
    <>
      {toConfirm(claim.status) && (
        <Button tone="primary" icon={Check} onClick={() => run(actions.keep(claim))}>
          Keep
        </Button>
      )}
      <Button icon={Pencil} onClick={() => actions.edit(claim)}>
        {toConfirm(claim.status) ? "Fix" : "Edit"}
      </Button>
      {claim.status === "active" && (
        <Button icon={claim.hidden ? Eye : EyeOff} onClick={() => run(actions.hide(claim, !claim.hidden))}>
          {claim.hidden ? "Bring it up again" : "Don't bring this up"}
        </Button>
      )}
      <Button icon={ThumbsDown} onClick={() => run(actions.reject(claim), true)}>
        Not true
      </Button>
      <div className="flex-1" />
      <Button tone="danger" icon={Trash2} onClick={() => run(actions.forget(claim), true)}>
        Forget
      </Button>
    </>
  );

  return (
    <>
    <Modal
      title={title}
      size="lg"
      onClose={onClose}
      description={
        claim && (
          <span className="mt-1 flex flex-wrap items-center gap-1.5">
            <TrustBadge trust={claim.trust} />
            <StateBadges claim={claim} />
            <span className="text-xs text-ink-faint">{whenText(claim)}</span>
          </span>
        )
      }
      footer={footer || undefined}
    >
      {error ? (
        error.gone ? (
          <p className="text-sm text-ink-subtle">{name} has forgotten this, or it was never kept.</p>
        ) : (
          <Callout tone="danger" icon={CircleAlert}>
            {error.message}
          </Callout>
        )
      ) : !x ? (
        <p className="flex items-center gap-2 text-[13px] text-ink-subtle">
          <LoaderCircle size={14} className="animate-spin" aria-hidden />
          Looking it up…
        </p>
      ) : (
        <div className="flex flex-col gap-5">
          {claim?.subject && (
            <p className="-mt-1 text-[13px] text-ink-subtle">
              About <span className="text-ink">{claim.subject}</span>
            </p>
          )}
          {quote && (
            <Section title="In the words it came from">
              <blockquote className="border-l-2 border-accent/50 pl-3 text-[13px] leading-relaxed text-ink-muted italic">“{quote}”</blockquote>
            </Section>
          )}
          {(claim?.source || chat) && (
            <Section title="Where it came from">
              <div className="flex flex-wrap items-center gap-2">
                <SourceChip source={claim?.source || "a chat"} threadId={chat || undefined} onOpen={onClose} />
                {claim && <span className="text-xs text-ink-faint">learned {shortDate(claim.recorded_at)}</span>}
              </div>
            </Section>
          )}
          {x.context && x.context.length > 0 && (
            <Section title="Around it">
              <div className="flex flex-col gap-2 rounded-lg border border-line bg-surface-2/40 p-3">
                {x.context.map((m, i) => (
                  <ContextLine key={m.id ?? i} message={m} name={name} highlight={!!m.id && m.id === sourceMessage} />
                ))}
              </div>
            </Section>
          )}
          {x.episode && (
            <Section title={x.episode.kind === "resource" ? "The document" : "The note"}>
              <div className="rounded-lg border border-line bg-surface-2/40 p-3">
                {x.episode.title && <div className="mb-1 text-[13px] font-medium text-ink">{x.episode.title}</div>}
                <p className="max-h-48 overflow-y-auto text-[13px] leading-relaxed whitespace-pre-line text-ink-muted">{cut(x.episode.body, 1200)}</p>
                <div className="mt-2 text-[11px] text-ink-faint">{shortDate(x.episode.at)}</div>
              </div>
            </Section>
          )}
          {history.length > 0 && (
            <Section title="History">
              <ul className="flex flex-col gap-1.5">
                {history.map((h) => (
                  <li key={h.id} className="text-[13px] text-ink-muted">
                    <span className="text-ink-subtle">{h.status === "rejected" ? "Not true: " : h.status === "superseded" ? "Before: " : "Also: "}</span>
                    {historyLine(h)}
                  </li>
                ))}
              </ul>
            </Section>
          )}
          {claim?.status === "rejected" && <Callout>You said this isn't true, so {name} doesn't use it.</Callout>}
        </div>
      )}
    </Modal>
    {actions.editor}
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-xs font-medium tracking-[0.02em] text-ink-subtle">{title}</h3>
      {children}
    </section>
  );
}

/** One message around a claim's source; the source itself stands out. */
function ContextLine({ message: m, name, highlight }: { message: wire.AssistantMessage; name: string; highlight: boolean }) {
  const user = m.role === "user";
  return (
    <div className={cx("rounded-md px-2.5 py-1.5 text-[13px] leading-relaxed", highlight ? "bg-accent/12 text-ink shadow-[inset_0_0_0_1px_var(--color-accent)]" : "text-ink-muted")}>
      <span className={cx("mr-2 text-[11px] font-medium", user ? "text-accent-hover" : "text-ink-subtle")}>{user ? "You" : name}</span>
      <span className="whitespace-pre-line">{cut(m.text, 600)}</span>
      {m.at > 0 && <span className="ml-2 text-[11px] text-ink-faint tabular-nums">{formatTime(m.at)}</span>}
    </div>
  );
}

/** The message id of a ref such as "msg:<id>", or null for any other ref. */
function messageIdOf(ref: string): string | null {
  return ref.startsWith("msg:") ? ref.slice("msg:".length) : null;
}

function cut(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n).trimEnd()}…` : s;
}
