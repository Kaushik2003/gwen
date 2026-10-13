import { Code2, Mail, type LucideIcon } from "lucide-react";
import { type ReactNode } from "react";
import { Badge, PageHeader, Panel } from "../components/ui";
import { useDaemon } from "../daemon";
import CalendarSettings from "../settings/Calendar";
import SyncSettings from "../settings/Sync";
import { PhoneSettings } from "./Settings";

/** What Gwen can reach, and how (plan.md §5.3). Calendar, phone, and the sync hub are real
 * connectors, so this reuses their Settings modules rather than duplicating their logic. Gmail and
 * Claude Code are gwen-harness/harness.md connectors (§8) with no backend yet: a plain status card,
 * not an empty state, since there is nothing here to fill in from this side. */
export default function Connections() {
  const d = useDaemon();
  if (!d.config) return null;
  return (
    <div className="flex flex-col gap-5">
      <PageHeader title="Connections" />
      <CalendarSettings />
      <PhoneSettings config={d.config} />
      <SyncSettings />
      <NotBuilt icon={Mail} title="Gmail">
        Email review (<code>gwen-harness/harness.md</code> §8.4) needs the kernel and the Gmail connector. Not built yet.
      </NotBuilt>
      <NotBuilt icon={Code2} title="Claude Code">
        Running Claude Code under Gwen's permission kernel (<code>gwen-harness/harness.md</code> §10.2 / §7.3) is not built yet.
      </NotBuilt>
    </div>
  );
}

function NotBuilt({ icon: Icon, title, children }: { icon: LucideIcon; title: string; children: ReactNode }) {
  return (
    <Panel title={title} icon={Icon} actions={<Badge tone="neutral">Not built yet</Badge>}>
      <p className="text-[13px] text-ink-subtle">{children}</p>
    </Panel>
  );
}
