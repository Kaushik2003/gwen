import { Bot, ShieldCheck } from "lucide-react";
import { autonomyChoices, useAgentStatus } from "../agent/agent";
import { Badge, Button, Panel } from "../components/ui";

// Settings › Gwen › Agent (plan.md 3.16): whether her agent can run here, and what asks under each autonomy. The
// switch and the default autonomy are the [agent] keys above this; remembered permissions live in What Gwen can do.

const rows: { what: string; cautious: string; balanced: string; trusted: string }[] = [
  { what: "Reading code, git, and your tasks", cautious: "Free", balanced: "Free", trusted: "Free" },
  { what: "Changing tasks, plans and goals", cautious: "Asks", balanced: "Free, with Undo", trusted: "Free" },
  { what: "Handing a job to a Claude session", cautious: "Asks", balanced: "Once per chat", trusted: "Free" },
  { what: "What Claude may do in that session", cautious: "Asks you", balanced: "Asks you", trusted: "Auto mode" },
  { what: "Fetching from the web", cautious: "Asks", balanced: "Once per chat", trusted: "Once per chat" },
  { what: "Reading your apps (Gmail, Slack, …)", cautious: "Free", balanced: "Free", trusted: "Free" },
  { what: "Sending or changing in your apps", cautious: "Every time", balanced: "Every time", trusted: "Every time" },
  { what: "Opening a link in your browser", cautious: "Every time", balanced: "Every time", trusted: "Every time" },
  { what: "Deleting tasks or goals", cautious: "Every time", balanced: "Every time", trusted: "Every time" },
];

export default function AgentSettings({ onRemembered }: { onRemembered: () => void }) {
  const status = useAgentStatus();
  return (
    <Panel
      title="What she may do"
      icon={Bot}
      actions={
        status && (
          <Badge tone={status.available ? "working" : status.enabled ? "idle" : "neutral"}>
            {status.available ? (status.live > 0 ? `Running in ${status.live} ${status.live === 1 ? "chat" : "chats"}` : "Ready") : status.enabled ? "Can't run" : "Off"}
          </Badge>
        )
      }
    >
      <div className="flex flex-col gap-4">
        {status && !status.available && status.reason && <p className="text-[12.5px] text-ink-muted">{status.reason}</p>}
        <p className="text-[12.5px] leading-relaxed text-ink-muted">
          Gwen runs your projects: she reads the code, plans with you, and hands every change, command and test to a Claude session in the project's folder, which she
          follows and reports on. Claude's own questions reach Needs you and your phone. In Auto mode, Claude Code's classifier approves routine work and asks only about
          risky steps.
        </p>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[30rem] text-left text-[12px]">
            <thead>
              <tr className="text-ink-subtle">
                <th className="py-1.5 pr-3 font-medium" />
                {autonomyChoices.map((c) => (
                  <th key={c.value} className="px-2 py-1.5 font-medium">
                    {c.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.what} className="border-t border-chrome-line">
                  <td className="py-1.5 pr-3 text-ink-muted">{r.what}</td>
                  {[r.cautious, r.balanced, r.trusted].map((v, i) => (
                    <td key={i} className={v === "Never" ? "px-2 py-1.5 text-danger" : v === "Free" ? "px-2 py-1.5 text-ink-subtle" : "px-2 py-1.5 text-ink"}>
                      {v}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button size="sm" icon={ShieldCheck} onClick={onRemembered}>
            Remembered permissions
          </Button>
          <span className="text-[12px] text-ink-subtle">“Allow for this chat” and “Always in this project” show there; remove any of them.</span>
        </div>
      </div>
    </Panel>
  );
}
