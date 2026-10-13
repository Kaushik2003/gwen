import { BookMarked, Check, CircleAlert, CircleHelp, FilePlus2, FileText, LoaderCircle, Pencil, Plus, Search, Settings as SettingsIcon, Sparkles, ThumbsDown, Trash2, Users, X, type LucideIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { App, apiError, wire, type ApiError } from "../api";
import { useConfirm, useToast } from "../components/feedback";
import MemoryDetail, { ClaimMenu, SourceChip, StateBadges, TrustBadge, useAssistantName, useClaimActions, type ClaimActions } from "../components/MemoryDetail";
import { Badge, Button, Callout, Dot, Empty, Field, IconButton, Input, Modal, PageHeader, Panel, TextArea, ToggleRow, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";
import { cardSections, kindLabel, laneLabel, laneState, rememberedText, receiptText, sectionOf, shortDate, sizeText, useMemoryVersion, whenText, type Section } from "../memory";
import { useNav } from "../nav";

/** Everything the screen shows, loaded together. */
interface Loaded {
  claims: wire.MemoryClaim[];
  candidates: wire.MemoryClaim[];
  quarantined: wire.MemoryClaim[];
  conflicts: wire.MemoryConflict[];
  status: wire.MemoryStatus;
  resources: wire.MemoryResource[];
  card: wire.ProfileCard | null;
}

function query(status: string, limit: number): wire.MemoryClaimQuery {
  return wire.MemoryClaimQuery.createFrom({ status, key: "", subject: "", q: "", hidden: true, limit, offset: 0 });
}

/** Memory: what Gwen knows about you and the people in your life, what she is unsure of, and why she holds each thing. */
export default function Memory() {
  const d = useDaemon();
  const { go } = useNav();
  const name = useAssistantName();
  const version = useMemoryVersion();
  const [reload, setReload] = useState(0);
  const refetch = useCallback(() => setReload((n) => n + 1), []);
  const actions = useClaimActions(refetch);
  const [past, setPast] = useState(false);
  const [data, setData] = useState<Loaded | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [filter, setFilter] = useState("");
  const [recall, setRecall] = useState<{ query: string; result: wire.RecallResult | null } | null>(null);
  const [remembering, setRemembering] = useState(false);
  const [detail, setDetail] = useState<string | null>(null);
  const enabled = d.config?.memory.enabled ?? true;

  useEffect(() => {
    if (!enabled) return;
    let live = true;
    Promise.all([
      App.ListClaims(query(past ? "all" : "active", 500)),
      App.ListClaims(query("candidate", 100)),
      App.ListClaims(query("quarantined", 100)),
      App.ListConflicts(""),
      App.MemoryStatus(),
      App.ListResources(),
      App.MemoryProfile().catch(() => null),
    ]).then(
      ([claims, candidates, quarantined, conflicts, status, resources, card]) => {
        if (!live) return;
        setData({
          claims: (claims.claims ?? []).filter((c) => c.status === "active" || c.status === "superseded"),
          candidates: candidates.claims ?? [],
          quarantined: quarantined.claims ?? [],
          conflicts: conflicts.conflicts ?? [],
          status,
          resources: resources.resources ?? [],
          card,
        });
        setError(null);
      },
      (e) => {
        if (!live) return;
        const err = apiError(e);
        if (err.code === "daemon_not_running" || err.code === "unauthorized") d.fail(e);
        setError(err);
      },
    );
    return () => {
      live = false;
    };
  }, [enabled, past, version, reload, d.fail]);

  async function runRecall(q: string) {
    if (!q.trim()) return setRecall(null);
    setRecall({ query: q.trim(), result: null });
    try {
      const result = await App.RecallMemory(wire.RecallRequest.createFrom({ query: q.trim(), as_of: "", lanes: [] }));
      setRecall({ query: q.trim(), result });
    } catch (e) {
      setRecall(null);
      d.fail(e);
    }
  }

  const words = filter.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const matches = (c: wire.MemoryClaim) => words.every((w) => `${c.text} ${c.label} ${c.subject} ${c.source}`.toLowerCase().includes(w));
  const shown = useMemo(() => {
    const by: Record<Section, wire.MemoryClaim[]> = { you: [], people: [], other: [] };
    for (const c of data?.claims ?? []) if (matches(c)) by[sectionOf(c)].push(c);
    // Past values after the ones that hold now.
    for (const s of Object.values(by)) s.sort((a, b) => Number(a.status === "superseded") - Number(b.status === "superseded"));
    return by;
  }, [data, filter]);

  const header = (
    <PageHeader
      title={`What ${name} knows`}
      subtitle={`What ${name} has learned about you and the people in your life, and why. Fix what's wrong, hide what she shouldn't bring up, or forget it for good.`}
      actions={
        enabled &&
        !error && (
          <Button tone="primary" icon={Plus} onClick={() => setRemembering(true)}>
            Remember something
          </Button>
        )
      }
    />
  );

  const off = !enabled || error?.code === "unavailable";
  if (off)
    return (
      <div className="flex flex-col gap-5">
        {header}
        <Callout
          tone="idle"
          icon={CircleAlert}
          action={
            <Button size="sm" tone="primary" icon={SettingsIcon} onClick={() => go("settings", "memory")}>
              Memory settings
            </Button>
          }
        >
          {!enabled ? `Memory is off, so ${name} doesn't learn or recall anything about you. Turn it on in Settings, Memory.` : sentence(error!.message)}
        </Callout>
      </div>
    );

  if (error)
    return (
      <div className="flex flex-col gap-5">
        {header}
        <Callout
          tone="danger"
          icon={CircleAlert}
          action={
            <Button size="sm" onClick={refetch}>
              Try again
            </Button>
          }
        >
          {name}'s memory couldn't be read: {error.message}
        </Callout>
      </div>
    );

  const review = data ? data.conflicts.length + data.candidates.length + data.quarantined.length : 0;
  const nothing = data && data.claims.length === 0 && review === 0;
  return (
    <div className="flex flex-col gap-5">
      {header}
      <div className="grid items-start gap-5 @4xl:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="flex min-w-0 flex-col gap-5">
          <SearchBox name={name} value={filter} onChange={setFilter} onRecall={runRecall} onClear={() => setRecall(null)} />
          {recall && <RecallPanel name={name} recall={recall} onOpen={setDetail} onClose={() => setRecall(null)} />}

          {!data ? (
            <p className="flex items-center gap-2 px-1 text-[13px] text-ink-subtle">
              <LoaderCircle size={14} className="animate-spin" aria-hidden />
              Reading what {name} knows…
            </p>
          ) : nothing ? (
            <Empty
              icon={BookMarked}
              title={`${name} hasn't learned anything about you yet.`}
              action={
                <>
                  <Button icon={Sparkles} onClick={() => go("assistant")}>
                    Open the assistant
                  </Button>
                  <Button tone="primary" icon={Plus} onClick={() => setRemembering(true)}>
                    Remember something
                  </Button>
                </>
              }
            >
              Tell her about yourself in the Assistant, or use Remember something.
            </Empty>
          ) : (
            <>
              {review > 0 && <ReviewPanel name={name} data={data} actions={actions} onOpen={setDetail} onChanged={refetch} />}
              <ClaimList title="About you" claims={shown.you} actions={actions} onOpen={setDetail} filtered={words.length > 0} />
              <ClaimList title="People" icon={Users} claims={shown.people} actions={actions} onOpen={setDetail} filtered={words.length > 0} grouped />
              <ClaimList title="Other" claims={shown.other} actions={actions} onOpen={setDetail} filtered={words.length > 0} grouped />
            </>
          )}
        </div>
        {data && <SidePanel name={name} data={data} past={past} onPast={setPast} onChanged={refetch} />}
      </div>

      {remembering && <RememberModal name={name} onClose={() => setRemembering(false)} onDone={refetch} />}
      {detail && <MemoryDetail refId={detail} onClose={() => setDetail(null)} />}
      {actions.editor}
    </div>
  );
}

/** The daemon's message as a sentence. */
function sentence(s: string): string {
  const t = s.charAt(0).toUpperCase() + s.slice(1);
  return /[.!?]$/.test(t) ? t : `${t}.`;
}

/** Filters the lists as you type; Enter asks what she would recall. */
function SearchBox({ name, value, onChange, onRecall, onClear }: { name: string; value: string; onChange: (v: string) => void; onRecall: (q: string) => void; onClear: () => void }) {
  return (
    <div className="flex flex-col gap-1.5">
      <form
        className="relative"
        onSubmit={(e) => {
          e.preventDefault();
          onRecall(value);
        }}
      >
        <Search size={15} className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-ink-faint" aria-hidden />
        <Input value={value} onChange={(e) => onChange(e.target.value)} placeholder={`Search what ${name} knows`} aria-label={`Search what ${name} knows`} className="w-full !pr-10 !pl-9" />
        {value && (
          <IconButton
            icon={X}
            label="Clear"
            size="sm"
            className="absolute top-1/2 right-1 -translate-y-1/2"
            onClick={() => {
              onChange("");
              onClear();
            }}
          />
        )}
      </form>
      <p className="px-1 text-xs text-ink-faint">Typing filters the lists below. Press Enter to see what {name} would recall if you asked her about it.</p>
    </div>
  );
}

/** What a recall found for the search: what she would have in mind if you asked her. */
function RecallPanel({ name, recall, onOpen, onClose }: { name: string; recall: { query: string; result: wire.RecallResult | null }; onOpen: (ref: string) => void; onClose: () => void }) {
  const r = recall.result;
  const hits = r?.hits ?? [];
  return (
    <Panel title={`${name} would recall`} icon={Sparkles} actions={<IconButton icon={X} label="Close" size="sm" onClick={onClose} />}>
      <p className="-mt-1 mb-3 text-[13px] text-ink-subtle">If you asked her about “{recall.query}”.</p>
      {!r ? (
        <p className="flex items-center gap-2 text-[13px] text-ink-subtle">
          <LoaderCircle size={14} className="animate-spin" aria-hidden />
          Thinking back…
        </p>
      ) : hits.length === 0 || r.abstain ? (
        <p className="text-[13px] text-ink-subtle">Nothing comes to mind for that.</p>
      ) : (
        <ul className="-mx-2 flex flex-col">
          {hits.map((h) => (
            <li key={h.ref}>
              <button type="button" onClick={() => onOpen(h.ref)} className="flex w-full flex-col gap-1 rounded-lg px-2 py-2 text-left hover:bg-surface-2">
                <span className="text-sm text-ink">{h.text}</span>
                <span className="flex flex-wrap items-center gap-1.5">
                  {h.kind === "claim" ? <TrustBadge trust={h.trust} /> : <Badge>{kindLabel(h.kind)}</Badge>}
                  {h.disputed && <Badge tone="idle">Not sure</Badge>}
                  <SourceChip source={h.source} />
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {r && (r.degraded ?? []).length > 0 && <p className="mt-3 text-xs text-ink-faint">Didn't answer in time: {(r.degraded ?? []).map(laneLabel).join(", ")}.</p>}
    </Panel>
  );
}

/** What needs you: conflicts, what she thinks, and what came from a document. */
function ReviewPanel({ name, data, actions, onOpen, onChanged }: { name: string; data: Loaded; actions: ClaimActions; onOpen: (id: string) => void; onChanged: () => void }) {
  return (
    <Panel title="To review" icon={CircleHelp} actions={<span className="text-xs text-ink-subtle">She doesn't rely on these until you say</span>}>
      <div className="flex flex-col gap-4">
        {data.conflicts.map((c) => (
          <ConflictCard key={c.id} name={name} conflict={c} onOpen={onOpen} onChanged={onChanged} />
        ))}
        {data.candidates.length > 0 && <Candidates title={`${name} thinks`} claims={data.candidates} actions={actions} onOpen={onOpen} />}
        {data.quarantined.length > 0 && <Candidates title="From a document" claims={data.quarantined} actions={actions} onOpen={onOpen} />}
      </div>
    </Panel>
  );
}

/** Two things that can't both be true, and the four ways out. */
function ConflictCard({ name, conflict: c, onOpen, onChanged }: { name: string; conflict: wire.MemoryConflict; onOpen: (id: string) => void; onChanged: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [busy, setBusy] = useState("");
  async function resolve(resolution: string, said: string) {
    setBusy(resolution);
    const ok = await d.act(() => App.ResolveConflict(c.id, wire.ResolveConflictRequest.createFrom({ resolution })));
    setBusy("");
    if (ok) {
      notify(said, "success");
      onChanged();
    }
  }
  const side = (label: string, x: wire.MemoryClaim) => (
    <div className="flex min-w-0 flex-col gap-1.5 rounded-lg border border-line bg-surface-2 p-3">
      <span className="text-[11px] font-medium text-ink-faint">{label}</span>
      <button type="button" onClick={() => onOpen(x.id)} className="text-left text-sm text-ink hover:underline hover:underline-offset-2">
        {x.text}
      </button>
      <span className="flex flex-wrap items-center gap-1.5">
        <SourceChip source={x.source} threadId={x.thread_id || undefined} />
        <span className="text-xs text-ink-faint">{shortDate(x.mentioned_at || x.recorded_at)}</span>
      </span>
    </div>
  );
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-idle/25 bg-idle/[0.04] p-3.5">
      <div className="flex items-center gap-2 text-sm font-medium text-ink">
        <CircleHelp size={15} className="text-idle" aria-hidden />
        {name} isn't sure which is right
      </div>
      <div className="grid gap-2 @xl:grid-cols-2">
        {side("The first", c.a)}
        {side("The second", c.b)}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" busy={busy === "keep_a"} onClick={() => resolve("keep_a", `${name} will go with: ${c.a.text}`)}>
          The first
        </Button>
        <Button size="sm" busy={busy === "keep_b"} onClick={() => resolve("keep_b", `${name} will go with: ${c.b.text}`)}>
          The second
        </Button>
        <Button size="sm" busy={busy === "both"} onClick={() => resolve("both", `${name} will keep both.`)}>
          Both are true
        </Button>
        <Button size="sm" busy={busy === "neither"} onClick={() => resolve("neither", `${name} will drop both.`)}>
          Neither
        </Button>
      </div>
    </div>
  );
}

/** Claims waiting for you: Keep, Fix, or Not true. */
function Candidates({ title, claims, actions, onOpen }: { title: string; claims: wire.MemoryClaim[]; actions: ClaimActions; onOpen: (id: string) => void }) {
  return (
    <section className="flex flex-col gap-1">
      <h3 className="px-1 text-xs font-medium tracking-[0.02em] text-ink-subtle">{title}</h3>
      <ul className="flex flex-col divide-y divide-line rounded-lg border border-line">
        {claims.map((c) => (
          <li key={c.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2.5">
            <button type="button" onClick={() => onOpen(c.id)} className="flex min-w-0 flex-1 basis-64 flex-col gap-1 text-left">
              <span className="text-sm text-ink">{c.text}</span>
              <span className="flex flex-wrap items-center gap-1.5">
                <SourceChip source={c.source} threadId={c.thread_id || undefined} />
                <span className="text-xs text-ink-faint">{whenText(c)}</span>
              </span>
            </button>
            <span className="flex shrink-0 gap-1.5">
              <Button size="sm" tone="primary" icon={Check} onClick={() => actions.keep(c)}>
                Keep
              </Button>
              <Button size="sm" icon={Pencil} onClick={() => actions.edit(c)}>
                Fix
              </Button>
              <Button size="sm" icon={ThumbsDown} onClick={() => actions.reject(c)}>
                Not true
              </Button>
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** One list of what she knows; grouped lists are by who or what each is about. */
function ClaimList({
  title,
  icon,
  claims,
  actions,
  onOpen,
  filtered,
  grouped,
}: {
  title: string;
  icon?: LucideIcon;
  claims: wire.MemoryClaim[];
  actions: ClaimActions;
  onOpen: (id: string) => void;
  filtered: boolean;
  grouped?: boolean;
}) {
  if (claims.length === 0) return null;
  const groups: [string, wire.MemoryClaim[]][] = [];
  if (grouped) {
    const by = new Map<string, wire.MemoryClaim[]>();
    for (const c of claims) by.set(c.subject || "Someone", [...(by.get(c.subject || "Someone") ?? []), c]);
    groups.push(...[...by.entries()].sort(([a], [b]) => a.localeCompare(b)));
  } else groups.push(["", claims]);
  return (
    <Panel title={title} icon={icon} actions={filtered && <span className="text-xs text-ink-subtle">{claims.length} matching</span>} bodyClassName="px-2 pt-1 pb-3">
      <div className="flex flex-col gap-3">
        {groups.map(([who, list]) => (
          <div key={who}>
            {who && <h3 className="px-3 pt-2 pb-1 text-[13px] font-medium text-ink-muted">{who}</h3>}
            <ul className="flex flex-col">
              {list.map((c) => (
                <ClaimRow key={c.id} claim={c} actions={actions} onOpen={onOpen} />
              ))}
            </ul>
          </div>
        ))}
      </div>
    </Panel>
  );
}

/** A thing she knows: the statement, since when, where from, how sure; a click opens why. */
function ClaimRow({ claim: c, actions, onOpen }: { claim: wire.MemoryClaim; actions: ClaimActions; onOpen: (id: string) => void }) {
  const pastValue = c.status === "superseded";
  return (
    <li className="group flex cursor-pointer items-start gap-2 rounded-lg px-3 py-2.5 hover:bg-surface-2" onClick={() => onOpen(c.id)}>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <button type="button" className={cx("text-left text-sm", pastValue || c.hidden ? "text-ink-subtle" : "text-ink")}>
          {c.text}
        </button>
        <span className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-ink-faint">{whenText(c)}</span>
          <SourceChip source={c.source} threadId={c.thread_id || undefined} />
          <TrustBadge trust={c.trust} />
          <StateBadges claim={c} />
        </span>
      </div>
      {!pastValue && <ClaimMenu claim={c} actions={actions} />}
    </li>
  );
}

/** The side panel: the view, today's card, learning and its sources, and documents. */
function SidePanel({ name, data, past, onPast, onChanged }: { name: string; data: Loaded; past: boolean; onPast: (on: boolean) => void; onChanged: () => void }) {
  const d = useDaemon();
  const { go } = useNav();
  const notify = useToast();
  const confirm = useConfirm();
  const [adding, setAdding] = useState(false);
  const st = data.status;
  const learn = d.config?.memory.learn ?? "assistant";
  const card = cardSections(data.card?.text ?? st.profile?.text ?? "");
  const waiting = (st.jobs?.pending ?? 0) + (st.jobs?.running ?? 0);
  const now = Date.now();
  const lanes = (st.lanes ?? []).filter((l) => l.enabled || ["vec", "files", "mem0", "supermemory"].includes(l.lane));

  async function addDocument() {
    try {
      const path = await App.PickDocument();
      if (!path) return;
      setAdding(true);
      const r = await d.act(() => App.AddResource(wire.AddResourceRequest.createFrom({ path, title: "" })));
      if (r) {
        notify(`${name} read “${r.title}”. What she learns from it waits for you under To review.`, "success");
        onChanged();
      }
    } catch (e) {
      d.fail(e);
    } finally {
      setAdding(false);
    }
  }

  async function removeDocument(doc: wire.MemoryResource) {
    const ok = await confirm({
      title: `Remove “${doc.title}”?`,
      body: `${name} forgets the document's words and what she learned only from it. The file itself stays where it is.`,
      confirm: "Remove",
      danger: true,
    });
    if (!ok) return;
    const r = await d.act(() => App.RemoveResource(doc.id));
    if (r) {
      notify(receiptText(r, `Removed “${doc.title}”.`, name), "success");
      onChanged();
    }
  }

  return (
    <Panel className="@4xl:sticky @4xl:top-0" bodyClassName="p-0">
      <div className="flex flex-col divide-y divide-line">
        <div className="px-5 py-4">
          <ToggleRow title="Show past values" description="Things that used to be true, such as a job you left." checked={past} onChange={onPast} />
        </div>

        <SideSection title={`What ${name} keeps in mind today`}>
          {card.length === 0 ? (
            <p className="text-[13px] text-ink-subtle">Nothing yet. Once she knows you a little, a short summary appears here. She reads it before every chat.</p>
          ) : (
            <div className="flex flex-col gap-3">
              {card.map((s) => (
                <div key={s.heading}>
                  {s.heading && <div className="mb-1 text-xs font-medium text-ink-subtle">{s.heading}</div>}
                  <ul className="flex flex-col gap-0.5 text-[13px] leading-relaxed text-ink-muted">
                    {s.lines.map((l, i) => (
                      <li key={i}>{l}</li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </SideSection>

        <SideSection title="Learning">
          <p className="text-[13px] leading-relaxed text-ink-muted">
            {learn === "off"
              ? `Learning from chats is off. ${name} only keeps what you tell her to remember.`
              : !st.learning
                ? `Learning is paused: the model she learns with isn't working. Check Settings, Memory.`
                : `${name} learns from your chats ${learn === "local" ? "with a local model" : "with the assistant's model"}.`}
            {st.last_learned_at != null && ` Last learned ${formatAgo(st.last_learned_at, now)}.`}
            {waiting > 0 && ` ${waiting} ${waiting === 1 ? "thing" : "things"} waiting.`}
            {st.last_slept_at != null && ` Tidied up ${formatAgo(st.last_slept_at, now)}.`}
          </p>
          {lanes.length > 0 && (
            <ul className="mt-3 flex flex-col gap-1.5">
              {lanes.map((l) => {
                const s = laneState(l);
                return (
                  <li key={l.lane} className="flex items-center gap-2 text-[13px]" title={s.words}>
                    <Dot color={s.color} />
                    <span className="whitespace-nowrap text-ink-muted">{laneLabel(l.lane)}</span>
                    <span className="ml-auto truncate text-xs text-ink-faint">{s.short}</span>
                  </li>
                );
              })}
            </ul>
          )}
          <Button size="sm" tone="ghost" icon={SettingsIcon} className="-ml-2.5 mt-2" onClick={() => go("settings", "memory")}>
            Memory settings
          </Button>
        </SideSection>

        <SideSection title="Documents">
          {data.resources.length === 0 ? (
            <p className="text-[13px] text-ink-subtle">Give {name} a document, such as notes about your course, and she remembers what's in it.</p>
          ) : (
            <ul className="flex flex-col gap-1">
              {data.resources.map((r) => (
                <li key={r.id} className="flex items-center gap-2">
                  <FileText size={14} className="shrink-0 text-ink-faint" aria-hidden />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] text-ink" title={r.path}>
                      {r.title}
                    </span>
                    <span className="text-[11px] text-ink-faint">
                      {sizeText(r.size)} · added {shortDate(r.added_at)}
                    </span>
                  </span>
                  <IconButton icon={Trash2} label={`Remove ${r.title}`} size="sm" onClick={() => removeDocument(r)} />
                </li>
              ))}
            </ul>
          )}
          <Button size="sm" icon={FilePlus2} className="mt-3" busy={adding} onClick={addDocument}>
            Add a document
          </Button>
        </SideSection>
      </div>
    </Panel>
  );
}

function SideSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="px-5 py-4">
      <h2 className="mb-2 text-[13px] font-semibold text-ink">{title}</h2>
      {children}
    </section>
  );
}

/** Remember something: words typed here are kept as facts you set. */
function RememberModal({ name, onClose, onDone }: { name: string; onClose: () => void; onDone: () => void }) {
  const d = useDaemon();
  const notify = useToast();
  const [text, setText] = useState("");
  const [saving, setSaving] = useState(false);
  async function save() {
    if (!text.trim() || saving) return;
    setSaving(true);
    const r = await d.act(() => App.Remember(wire.RememberRequest.createFrom({ text: text.trim() }), ""));
    setSaving(false);
    if (!r) return;
    notify(rememberedText(r.claims ?? [], name), (r.claims ?? []).length ? "success" : "info");
    onDone();
    if ((r.claims ?? []).length) onClose();
  }
  return (
    <Modal
      title="Remember something"
      description={`Write it the way you'd tell ${name}. She keeps each fact in it as something you set yourself.`}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button tone="primary" icon={BookMarked} busy={saving} disabled={!text.trim()} onClick={save}>
            Remember
          </Button>
        </>
      }
    >
      <Field label="What to remember" hint={saving ? `${name} is reading it…` : `${4000 - text.length} characters left. Enter saves, Shift+Enter starts a new line.`}>
        <TextArea
          rows={4}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              save();
            }
          }}
          maxLength={4000}
          autoFocus
          disabled={saving}
          placeholder="My sister Mira's birthday is 3 March. I work at Acme as a designer. I hate meetings before 10."
        />
      </Field>
    </Modal>
  );
}
