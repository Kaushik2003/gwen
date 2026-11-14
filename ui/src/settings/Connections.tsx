import { useEffect, useState, type ReactNode } from "react";
import { App, onEvent, wire } from "../api";
import { Button, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";

// Settings › All connections (plan.md 3.15, board 15): what Gwen is
// connected to, one card each, and what plan.md has planned, shown dashed.

type Tone = "ok" | "warn" | "off";

const planned: { name: string; pkg: string; text: string }[] = [
  { name: "Gmail", pkg: "W22", text: "Review your newest emails, draft replies, send only with your OK." },
  { name: "Slack", pkg: "W28", text: "Action points from channels you pick, handed to a session." },
  { name: "MCP servers", pkg: "W24", text: "More tools for Gwen, asked before they change anything." },
  { name: "Notes folder", pkg: "W25", text: "Markdown notes Gwen reads and writes, Obsidian friendly." },
  { name: "Other computers", pkg: "W30", text: "Their terminals and agents join your Inbox over the tailnet." },
  { name: "Home", pkg: "W31", text: "Lights and devices through Home Assistant on the Pi." },
];

export default function ConnectionsOverview({ go }: { go: (section: string) => void }) {
  const d = useDaemon();
  const [cal, setCal] = useState<wire.CalendarStatus | null>(null);
  const [remote, setRemote] = useState<wire.RemoteStatus | null>(null);
  const [sync, setSync] = useState<wire.SyncStatus | null>(null);

  useEffect(() => {
    const load = () => {
      App.CalendarStatus().then(setCal, () => setCal(null));
      App.RemoteStatus().then(setRemote, () => setRemote(null));
      App.SyncStatus().then(setSync, () => setSync(null));
    };
    load();
    const offs = [onEvent("integration_changed", load), onEvent("remote_changed", load)];
    return () => offs.forEach((off) => off());
  }, []);

  const c = d.config;
  if (!c) return null;
  const now = d.now();
  const provider = c.llm.provider;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="text-[20px] font-semibold">Connections</h2>
        <p className="mt-1 text-ink-muted">What Gwen is connected to. Each card opens its own section.</p>
      </div>
      <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(270px,1fr))]">
        <Card
          title="Google Calendar"
          tone={cal?.connected ? (cal.last_error ? "warn" : "ok") : "off"}
          state={cal?.connected ? (cal.last_error ? "Needs a look" : "Connected") : "Not connected"}
          action={<Button size="sm" onClick={() => go("calendar")}>Calendar options</Button>}
        >
          {cal?.connected
            ? cal.last_error
              ? cal.last_error
              : `Last synced ${cal.last_sync_at ? formatAgo(cal.last_sync_at, now) : "not yet"}. Your plan goes to the calendar "${c.calendar.name}".`
            : "Busy time from your calendars, and your plan written back to one of them."}
        </Card>
        <Card
          title="Gwen on your phone"
          tone={remote?.published ? "ok" : remote?.enabled ? "warn" : "off"}
          state={remote?.published ? "On" : remote?.enabled ? "Not published" : "Off"}
          action={<Button size="sm" onClick={() => go("phone")}>{remote?.devices ? "Phone settings" : "Pair a phone"}</Button>}
        >
          {remote?.published
            ? `Over your tailnet only. ${remote.devices} phone${remote.devices === 1 ? "" : "s"} paired. Pushes when a session needs you and you're away.`
            : "Start and answer Claude sessions from your phone, over your tailnet only."}
        </Card>
        <Card
          title="Claude sessions"
          tone={remote?.hooks ? (remote.sandbox && !remote.sandbox_signed_in ? "warn" : "ok") : "off"}
          state={remote?.hooks ? (remote.sandbox && !remote.sandbox_signed_in ? "1 step left" : "Seen") : "Hooks off"}
          action={<Button size="sm" onClick={() => go("phone")}>Session settings</Button>}
        >
          {remote?.hooks
            ? remote.sandbox && !remote.sandbox_signed_in
              ? "Gwen sees your Claude sessions and their permission requests. Sandboxed sessions need one sign-in."
              : "Gwen sees your Claude sessions, their permission requests, and what they changed."
            : remote?.hooks_note || "Turn on Claude Code's hooks so Gwen can see your sessions."}
        </Card>
        <Card title="AI" tone={provider === "none" ? "off" : "ok"} state={provider === "none" ? "Off" : "Ready"} action={<Button size="sm" onClick={() => go("ai")}>AI settings</Button>}>
          {provider === "none" ? "Chat, planning, goal breakdown, and the retro need a model." : `${provider}${c.llm.model ? `, ${c.llm.model}` : ""}. Used for chat, planning, goal breakdown, and the retro.`}
        </Card>
        <Card
          title="Phone nudges"
          tone={c.nudge.phone && c.ntfy.topic ? "ok" : "off"}
          state={c.nudge.phone && c.ntfy.topic ? "On" : "Off"}
          action={<Button size="sm" onClick={() => go("phone")}>{c.nudge.phone ? "Nudge settings" : "Turn on"}</Button>}
        >
          Idle and long-break nudges on your phone through ntfy, with a private topic.
        </Card>
        <Card
          title="Sync hub"
          tone={sync?.configured ? (sync.last_error ? "warn" : "ok") : "off"}
          state={sync?.configured ? (sync.last_error ? "Needs a look" : "In step") : "Not set up"}
          action={<Button size="sm" onClick={() => go("sync")}>Sync settings</Button>}
        >
          {sync?.configured
            ? sync.last_error
              ? sync.last_error
              : `Your Raspberry Pi. Sent ${sync.last_push_at ? formatAgo(sync.last_push_at, now) : "not yet"}, received ${sync.last_pull_at ? formatAgo(sync.last_pull_at, now) : "not yet"}.`
            : "Keep a copy on your Raspberry Pi, with a read-only dashboard of Plan, Goals, History, and Stats."}
        </Card>
      </div>
      <div className="mt-1 text-[12px] text-ink-subtle">Planned, from plan.md</div>
      <div className="grid gap-2.5 [grid-template-columns:repeat(auto-fit,minmax(200px,1fr))]">
        {planned.map((p) => (
          <div key={p.name} className="flex flex-col gap-1 rounded-xl border border-dashed border-line-strong px-3.5 py-3">
            <b className="flex items-center gap-2 text-[13px] font-medium">
              {p.name}
              <span className="ml-auto rounded-[5px] bg-surface-2 px-1.5 font-mono text-[10.5px] text-ink-muted">{p.pkg}</span>
            </b>
            <span className="text-[11.5px] leading-snug text-ink-subtle">{p.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function Card({ title, tone, state, action, children }: { title: string; tone: Tone; state: string; action: ReactNode; children: ReactNode }) {
  const colors = { ok: "bg-success/14 text-success", warn: "bg-warning/14 text-warning", off: "bg-surface-2 text-ink-muted" }[tone];
  const dot = { ok: "bg-success", warn: "bg-warning", off: "bg-off" }[tone];
  return (
    <div className="flex flex-col gap-2 rounded-xl border border-line bg-surface-1 px-4 py-3.5">
      <h3 className="flex items-center gap-2 text-[14px] font-semibold">
        {title}
        <span className={cx("ml-auto inline-flex h-5 items-center gap-1.5 rounded-full px-2 text-[11px] font-medium", colors)}>
          <i className={cx("size-1.5 rounded-full", dot)} />
          {state}
        </span>
      </h3>
      <p className="flex-1 text-[12px] leading-relaxed text-ink-muted">{children}</p>
      <div className="flex gap-1.5">{action}</div>
    </div>
  );
}
