import { Check, ChevronDown, ChevronRight, Play, Plug, RotateCcw, ShieldCheck, Square, Trash2, X } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { App, onEvent, wire } from "../api";
import { Badge, Button, Checkbox, Empty, Panel, cx, type BadgeTone } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";

// Settings › What Gwen can do (plan.md 3.7, 3.8, W19): the connections and
// their capabilities with risk, approvals waiting for your click, the grants
// you remembered, and the runs Gwen made. Approvals are decided only here, by
// a click (plan.md section 7).

const riskTone: Record<string, BadgeTone> = { read: "neutral", local: "neutral", write: "working", external: "danger", destructive: "danger" };
const riskWords: Record<string, string> = {
  read: "Reads",
  local: "On this laptop",
  write: "Asks once per run",
  external: "Asks every time",
  destructive: "Not offered",
};
const stateWords: Record<string, string> = { connected: "Connected", needs_setup: "Needs setup", error: "Needs a look" };

/** A connection's state, in the colours of Settings › All connections. */
function StateChip({ state }: { state: string }) {
  const colors = { connected: "bg-success/14 text-success", error: "bg-warning/14 text-warning" }[state] ?? "bg-surface-2 text-ink-muted";
  const dot = { connected: "bg-success", error: "bg-warning" }[state] ?? "bg-off";
  return (
    <span className={cx("ml-auto inline-flex h-5 items-center gap-1.5 rounded-full px-2 text-[11px] font-medium", colors)}>
      <i className={cx("size-1.5 rounded-full", dot)} />
      {stateWords[state] ?? state}
    </span>
  );
}
const runTone: Record<string, BadgeTone> = { draft: "neutral", running: "working", waiting: "break", done: "accent", failed: "danger", cancelled: "neutral" };
const runWords: Record<string, string> = { draft: "Draft", running: "Running", waiting: "Waiting", done: "Done", failed: "Failed", cancelled: "Cancelled" };
const reasonWords: Record<string, string> = { queued: "queued behind two others", approval: "for your approval", interrupted: "Gwen restarted; resume or cancel it", reply: "for your reply" };
const stepMark: Record<string, string> = { pending: "·", running: "…", waiting: "?", done: "✓", failed: "✕", skipped: "–" };

/** A scope as people read it: a task, a folder, a project, a chat. */
function scopeWords(scope: string, d: ReturnType<typeof useDaemon>): string {
  const [kind, id] = [scope.slice(0, scope.indexOf(":")), scope.slice(scope.indexOf(":") + 1)];
  if (scope.startsWith("/")) return scope.replace(/^\/home\/[^/]+/, "~");
  if (kind === "task") return `the task “${d.tasks.find((t) => t.id === id)?.title ?? "a task"}”`;
  if (kind === "project") return `the project ${d.projects.find((p) => p.id === id)?.name ?? ""}`.trim();
  if (kind === "chat") return "this chat";
  return scope;
}

export default function AbilitiesSettings() {
  const d = useDaemon();
  const now = d.now();
  const [conns, setConns] = useState<wire.Connection[] | null>(null);
  const [pending, setPending] = useState<wire.Approval[]>([]);
  const [grants, setGrants] = useState<wire.Grant[]>([]);
  const [runs, setRuns] = useState<wire.Run[]>([]);
  const [unavailable, setUnavailable] = useState(false);

  const load = useCallback(() => {
    App.ListConnections().then(
      (l) => (setConns(l.connections), setUnavailable(false)),
      () => setUnavailable(true),
    );
    App.ListApprovals("pending").then((l) => setPending(l.approvals), () => setPending([]));
    App.ListGrants().then((l) => setGrants(l.grants), () => setGrants([]));
    App.ListRuns([], 20).then((l) => setRuns(l.runs), () => setRuns([]));
  }, []);
  useEffect(() => {
    load();
    const offs = ["run_changed", "approval_requested", "approval_decided", "connection_changed"].map((name) => onEvent(name, load));
    return () => offs.forEach((off) => off());
  }, [load]);

  if (unavailable)
    return (
      <Empty icon={Plug} title="Not running">
        This Gwen runs no harness, so it can't open links, read your calendar for a run, or ask for approvals.
      </Empty>
    );

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="text-[20px] font-semibold">What Gwen can do</h2>
        <p className="mt-1 text-ink-muted">
          Everything Gwen does for you goes through one of these, and is checked when it happens: reading and things on this laptop just run, a change asks you once per run, and anything
          that leaves the laptop asks every time.
        </p>
      </div>

      {pending.length > 0 && (
        <Panel title="Waiting for you" icon={ShieldCheck}>
          <ul className="flex flex-col gap-2.5">
            {pending.map((a) => (
              <ApprovalCard key={a.id} a={a} now={now} scope={(s) => scopeWords(s, d)} onDone={load} />
            ))}
          </ul>
        </Panel>
      )}

      <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(280px,1fr))]">
        {(conns ?? []).map((c) => (
          <section key={c.id} className="flex flex-col gap-2.5 rounded-xl border border-line bg-surface-1 px-4 py-3.5">
            <h3 className="flex items-center gap-2 text-[14px] font-semibold">
              {c.name}
              <StateChip state={c.state} />
            </h3>
            {c.message && <p className="text-[12px] leading-relaxed text-ink-muted">{c.message}</p>}
            <ul className="flex flex-col gap-1.5">
              {c.capabilities.map((cp) => (
                <li key={cp.name} className="flex items-center gap-2 text-[12.5px]">
                  <span className="min-w-0 flex-1 truncate" title={cp.name}>
                    {cp.title}
                  </span>
                  <Badge tone={riskTone[cp.risk] ?? "neutral"}>{riskWords[cp.risk] ?? cp.risk}</Badge>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>

      <Panel title="Remembered" icon={Check}>
        {grants.length === 0 ? (
          <p className="text-[12.5px] text-ink-subtle">Nothing yet. When you approve something, you can tell Gwen to remember it for that task, folder, or project, and it shows here.</p>
        ) : (
          <ul className="flex flex-col divide-y divide-line">
            {grants.map((g) => (
              <li key={g.id} className="flex items-center gap-3 py-2 text-[12.5px]">
                <span className="min-w-0 flex-1">
                  <span className="font-medium">{g.decision === "allow" ? "Allowed" : "Always asks"}:</span> {g.capability}
                  {g.pattern && <span className="font-mono text-ink-muted"> {g.pattern}</span>}
                  <span className="text-ink-muted"> for {scopeWords(g.scope, d)}</span>
                </span>
                <span className="text-[11.5px] text-ink-subtle">{formatAgo(g.created_at, now)}</span>
                <Button size="sm" icon={Trash2} onClick={() => App.DeleteGrant(g.id).then(load, d.fail)}>
                  Remove
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Panel title="Recent runs" icon={Play}>
        {runs.length === 0 ? (
          <p className="text-[12.5px] text-ink-subtle">No runs yet. A task's steps for when it starts run here when you start it.</p>
        ) : (
          <ul className="flex flex-col divide-y divide-line">
            {runs.map((r) => (
              <RunRow key={r.id} r={r} now={now} onDone={load} />
            ))}
          </ul>
        )}
      </Panel>
    </div>
  );
}

/** One approval: what it will do, exactly, and Approve or Reject; a click is the only way to decide. */
function ApprovalCard({ a, now, scope, onDone }: { a: wire.Approval; now: number; scope: (s: string) => string; onDone: () => void }) {
  const d = useDaemon();
  const [remember, setRemember] = useState("");
  const [busy, setBusy] = useState("");
  const [open, setOpen] = useState(false);
  const decide = async (decision: "approve" | "reject") => {
    setBusy(decision);
    try {
      await App.DecideApproval(a.id, wire.DecideApprovalRequest.createFrom({ decision, remember: decision === "approve" && remember ? remember : undefined }));
      onDone();
    } catch (e) {
      d.fail(e);
    } finally {
      setBusy("");
    }
  };
  return (
    <li className="flex flex-col gap-2 rounded-lg border border-line-strong bg-surface-2 px-3.5 py-3">
      <div className="flex items-center gap-2">
        <b className="min-w-0 flex-1 text-[13px] font-medium">{a.summary}</b>
        <Badge tone={riskTone[a.risk] ?? "neutral"}>{riskWords[a.risk] ?? a.risk}</Badge>
      </div>
      <div className="text-[11.5px] text-ink-subtle">
        {a.run_title ? `For “${a.run_title}”` : "From a chat"} · asked {formatAgo(a.created_at, now)} · {a.capability}
      </div>
      <button type="button" className="flex items-center gap-1 self-start text-[11.5px] text-ink-muted hover:text-ink" onClick={() => setOpen((o) => !o)}>
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />} Exactly what it will do
      </button>
      {open && <pre className="max-h-48 overflow-auto rounded-md bg-surface-1 px-2.5 py-2 font-mono text-[11.5px] whitespace-pre-wrap">{JSON.stringify(a.payload, null, 2)}</pre>}
      {a.grant_scopes.length > 0 && (
        <div className="flex flex-col gap-1">
          {a.grant_scopes.map((s) => (
            <div key={s} className="flex items-center gap-2 text-[12.5px]">
              <Checkbox square checked={remember === s} onChange={(on) => setRemember(on ? s : "")} label={`Don't ask again for ${scope(s)}`} />
              <button type="button" className="text-left text-ink-muted hover:text-ink" onClick={() => setRemember(remember === s ? "" : s)}>
                Don't ask again for {scope(s)}
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="flex gap-2">
        <Button size="sm" tone="primary" icon={Check} busy={busy === "approve"} disabled={!!busy} onClick={() => decide("approve")}>
          Approve
        </Button>
        <Button size="sm" icon={X} busy={busy === "reject"} disabled={!!busy} onClick={() => decide("reject")}>
          Reject
        </Button>
      </div>
    </li>
  );
}

/** One run, with its steps when opened, and Cancel or Resume. */
function RunRow({ r, now, onDone }: { r: wire.Run; now: number; onDone: () => void }) {
  const d = useDaemon();
  const [open, setOpen] = useState(false);
  const [detail, setDetail] = useState<wire.RunDetail | null>(null);
  useEffect(() => {
    if (open) App.GetRun(r.id).then(setDetail, () => setDetail(null));
  }, [open, r.id, r.updated_at]);
  const live = r.status === "running" || r.status === "waiting" || r.status === "draft";
  return (
    <li className="py-2">
      <div className="flex items-center gap-2.5 text-[12.5px]">
        <button type="button" className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={() => setOpen((o) => !o)}>
          {open ? <ChevronDown size={13} className="shrink-0 text-ink-subtle" /> : <ChevronRight size={13} className="shrink-0 text-ink-subtle" />}
          <span className="truncate font-medium">{r.title}</span>
          <span className="shrink-0 text-ink-subtle">{r.kind === "routine" ? "When it starts" : r.kind}</span>
        </button>
        <Badge tone={runTone[r.status] ?? "neutral"} title={r.reason}>
          {runWords[r.status] ?? r.status}
        </Badge>
        <span className="w-14 shrink-0 text-right text-[11.5px] text-ink-subtle">{formatAgo(r.updated_at, now)}</span>
        {r.status === "waiting" && r.reason === "interrupted" && (
          <Button size="sm" icon={RotateCcw} onClick={() => App.ResumeRun(r.id).then(onDone, d.fail)}>
            Resume
          </Button>
        )}
        {live && (
          <Button size="sm" icon={Square} onClick={() => App.CancelRun(r.id).then(onDone, d.fail)}>
            Cancel
          </Button>
        )}
      </div>
      {(r.reason || open) && (
        <div className="mt-1 pl-5 text-[11.5px] text-ink-subtle">
          {r.reason && <div className={cx(r.status === "failed" && "text-danger")}>{r.status === "waiting" || r.status === "draft" ? `Waiting ${reasonWords[r.reason] ?? r.reason}` : r.reason}</div>}
          {open &&
            detail?.steps.map((s) => (
              <div key={s.id} className="flex gap-2 py-0.5">
                <span className={cx("w-3 shrink-0 text-center font-mono", s.status === "failed" && "text-danger", s.status === "done" && "text-success")}>{stepMark[s.status] ?? "·"}</span>
                <span className="min-w-0 flex-1">
                  {s.title}
                  {s.error && <span className="text-danger"> · {s.error}</span>}
                </span>
              </div>
            ))}
        </div>
      )}
    </li>
  );
}
