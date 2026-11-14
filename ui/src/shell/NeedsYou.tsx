import { Bell, Check, ShieldAlert, ShieldQuestion } from "lucide-react";
import { useState } from "react";
import { App, wire } from "../api";
import { Button, Empty, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { agentChatTab, useTabs } from "./tabs";
import { useWork, type Need } from "./work";

// The one Needs you list (plan.md 3.15): approvals Gwen waits on (3.16),
// sessions waiting on you or finished unread, and nudges. Each row answers in
// place, or opens where it can be answered.

export default function NeedsYou({ limit, compact }: { limit?: number; compact?: boolean }) {
  const w = useWork();
  const shown = limit ? w.needs.slice(0, limit) : w.needs;
  if (w.needs.length === 0)
    return compact ? (
      <p className="flex items-center gap-2.5 px-4 py-3 text-[12.5px] text-ink-subtle">
        <Check size={14} className="shrink-0 text-success" aria-hidden />
        <span>
          <span className="font-medium text-ink-muted">Nothing needs you.</span> When a session asks for something or finishes, or Gwen nudges you, it shows here.
        </span>
      </p>
    ) : (
      <Empty icon={Check} title="Nothing needs you">
        When a session asks for something or finishes, or Gwen nudges you, it shows here.
      </Empty>
    );
  return (
    <ul className="flex flex-col">
      {shown.map((n) => (
        <Row key={n.id} need={n} compact={compact} />
      ))}
      {limit && w.needs.length > limit && <li className="border-t border-chrome-line px-4 py-2 text-[12px] text-ink-subtle">{w.needs.length - limit} more in the Inbox</li>}
    </ul>
  );
}

function Row({ need, compact }: { need: Need; compact?: boolean }) {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const [busy, setBusy] = useState("");
  const [note, setNote] = useState("");

  const project = (id: string | null) => d.projects.find((p) => p.id === id);
  const openSession = (s: wire.CodeSession, p: string | null) => {
    if (s.terminal) tabs.open({ kind: "term", project: p ?? "", terminal: s.terminal });
    App.CodeMark(s.id, wire.CodeMark.createFrom({ seen: true })).catch(() => {});
  };

  let icon, tint, title, detail, actions;
  if (need.kind === "session") {
    const s = need.session;
    const wait = s.waiting;
    const where = project(need.project)?.name ?? s.name;
    icon = <ShieldAlert size={15} aria-hidden />;
    tint = "bg-warning/12 text-warning";
    title = (
      <>
        <b className="font-semibold">Claude in {where}</b>{" "}
        <span className="text-ink-muted">{wait?.kind === "question" ? "asks" : wait?.tool ? `wants to use ${wait.tool}` : "needs you"}</span>
      </>
    );
    detail = wait?.question || wait?.summary || s.step || s.last_message;
    const allow = wait?.held ? wait.options.find((o) => o.kind === "allow") : undefined;
    const deny = wait?.held ? wait.options.find((o) => o.kind === "deny") : undefined;
    actions = (
      <>
        {allow && (
          <Button
            size="sm"
            tone="primary"
            busy={busy === allow.key}
            onClick={async () => {
              setBusy(allow.key);
              const r = await App.CodeAnswer(s.id, wire.CodeAnswer.createFrom({ key: allow.key })).catch((e) => ({ ok: false, message: String(e) }));
              setBusy("");
              if (!r.ok) setNote(r.message);
              w.refreshSessions();
            }}
          >
            {allow.label || "Allow once"}
          </Button>
        )}
        {deny && !compact && (
          <Button
            size="sm"
            busy={busy === deny.key}
            onClick={async () => {
              setBusy(deny.key);
              await App.CodeAnswer(s.id, wire.CodeAnswer.createFrom({ key: deny.key })).catch(() => {});
              setBusy("");
              w.refreshSessions();
            }}
          >
            {deny.label || "Deny"}
          </Button>
        )}
        <Button size="sm" onClick={() => openSession(s, need.project)} disabled={!s.terminal}>
          Open
        </Button>
      </>
    );
    if (wait?.note && !note) detail = `${detail ? detail + " · " : ""}${wait.note}`;
  } else if (need.kind === "approval") {
    const a = need.approval;
    icon = <ShieldQuestion size={15} aria-hidden />;
    tint = "bg-accent/12 text-accent-hover";
    title = a.thread_id ? (
      <>
        <b className="font-semibold">Gwen</b> <span className="text-ink-muted">needs your OK</span>
      </>
    ) : (
      <>
        <b className="font-semibold">{a.run_title || "A run"}</b> <span className="text-ink-muted">needs your OK</span>
      </>
    );
    detail = a.summary.split(" · ")[0];
    actions = (
      <Button
        size="sm"
        tone="secondary"
        onClick={() =>
          a.thread_id
            ? tabs.open(agentChatTab(a.thread_id))
            : tabs.open({ kind: "screen", screen: "settings", param: "abilities" })
        }
      >
        Review
      </Button>
    );
  } else if (need.kind === "done") {
    const s = need.session;
    icon = <Check size={15} strokeWidth={2.4} aria-hidden />;
    tint = "bg-success/12 text-success";
    title = (
      <>
        <b className="font-semibold">Claude in {project(need.project)?.name ?? s.name}</b> <span className="text-ink-muted">finished</span>
      </>
    );
    detail = s.last_message || s.step;
    actions = (
      <>
        <Button size="sm" onClick={() => openSession(s, need.project)} disabled={!s.terminal}>
          Review
        </Button>
        {!compact && (
          <Button size="sm" tone="ghost" onClick={() => App.CodeMark(s.id, wire.CodeMark.createFrom({ seen: true })).then(w.refreshSessions, () => {})}>
            Mark read
          </Button>
        )}
      </>
    );
  } else {
    const n = need.nudge;
    icon = <Bell size={15} aria-hidden />;
    tint = "bg-break/12 text-break";
    title = <b className="font-semibold">{n.title}</b>;
    detail = n.body;
    actions = (
      <>
        {n.actions.slice(0, compact ? 1 : 3).map((a, i) => (
          <Button key={a.id} size="sm" tone={i === 0 ? "secondary" : "ghost"} onClick={() => d.run(() => App.ActOnNudge(n.kind, a.id))}>
            {a.label}
          </Button>
        ))}
      </>
    );
  }

  return (
    <li className="flex items-center gap-3 border-t border-chrome-line px-4 py-[11px] first:border-t-0">
      <span className={cx("grid size-[30px] shrink-0 place-items-center rounded-lg", tint)}>{icon}</span>
      <div className="min-w-0 flex-1">
        <div className="truncate">{title}</div>
        {(detail || note) && (
          <div className={cx("mt-0.5 truncate text-[11.5px]", need.kind === "session" ? "font-mono text-ink-muted" : "text-ink-subtle")} title={note || detail || undefined}>
            {note || detail}
          </div>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-1.5">{actions}</div>
    </li>
  );
}
