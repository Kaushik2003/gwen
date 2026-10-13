import { Ban, Check, Clock, Globe, KeyRound, Lock, TriangleAlert, Undo2, type LucideIcon } from "lucide-react";
import { Badge, Button, Callout, Checkbox, Panel, type BadgeTone } from "./ui";

export type Reach = "local" | "account" | "world";
export type Undo = "reversible" | "delayed" | "none";
export type Decision = "allow_once" | "allow_brief" | "always_allow" | "deny";

export interface ApprovalPayload {
  id: string;
  label: string;
  checked: boolean;
}

/** One pending ask from the kernel. Fields mirror the Action model (harness.md §5.1): what, where it goes,
 * why, what is tainted, and the undo window — there is no API for this yet, so callers pass fixtures. */
export interface Approval {
  id: string;
  tool: string;
  summary: string;
  destination?: string;
  brief?: string;
  reach: Reach;
  undo: Undo;
  /** Set once the action has been approved and is waiting out its outbox window; switches the card to "Cancel". */
  undoSeconds?: number;
  taint?: string[];
  destTaint?: boolean;
  /** The exact standing rule "Always allow this" would create, shown under that button. */
  standingRuleHint?: string;
  /** Omit for a single-payload approval; several turns this into a batch with its own checkbox per row. */
  payloads?: ApprovalPayload[];
}

const reachIcon: Record<Reach, LucideIcon> = { local: Lock, account: KeyRound, world: Globe };
const reachLabel: Record<Reach, string> = { local: "Local", account: "Account", world: "World" };
const reachTone: Record<Reach, BadgeTone> = { local: "neutral", account: "accent", world: "danger" };

export function ApprovalCard({
  approval: a,
  onTogglePayload,
  onDecide,
  busy,
}: {
  approval: Approval;
  onTogglePayload?: (payloadId: string) => void;
  onDecide: (decision: Decision, payloadIds: string[]) => void;
  busy?: boolean;
}) {
  const ReachIcon = reachIcon[a.reach];
  const payloadIds = a.payloads?.filter((p) => p.checked).map((p) => p.id) ?? [a.id];
  const noneSelected = (a.payloads?.length ?? 0) > 0 && payloadIds.length === 0;

  if (a.undoSeconds != null) {
    return (
      <Panel title={a.summary} icon={ReachIcon} actions={<Badge tone={reachTone[a.reach]}>{reachLabel[a.reach]}</Badge>}>
        {a.destination && <p className="text-[13px] text-ink-subtle">{a.destination}</p>}
        <Callout
          tone="idle"
          icon={Clock}
          action={
            <Button tone="secondary" icon={Undo2} busy={busy} onClick={() => onDecide("deny", payloadIds)}>
              Cancel
            </Button>
          }
        >
          Sending in {a.undoSeconds}s. Cancel costs nothing once it's out, it's gone.
        </Callout>
      </Panel>
    );
  }

  return (
    <Panel title={a.summary} icon={ReachIcon} actions={<Badge tone={reachTone[a.reach]}>{reachLabel[a.reach]}</Badge>}>
      <div className="space-y-1 text-[13px] text-ink-subtle">
        {a.destination && <p>Where: {a.destination}</p>}
        {a.brief && <p>Why: {a.brief}</p>}
      </div>

      {(a.taint?.length || a.destTaint) && (
        <Callout tone={a.destTaint ? "danger" : "idle"} icon={TriangleAlert} className="mt-3">
          {a.destTaint
            ? "The destination only appears in untrusted content, so this always asks, whatever your rules say."
            : `Reads from outside your control: ${a.taint!.join(", ")}.`}
        </Callout>
      )}

      {a.payloads && a.payloads.length > 1 && (
        <ul className="mt-3 space-y-1.5 border-t border-line pt-3">
          {a.payloads.map((p) => (
            <li key={p.id} className="flex items-center gap-2.5">
              <Checkbox checked={p.checked} onChange={() => onTogglePayload?.(p.id)} label={p.label} />
              <span className="text-[13px] text-ink">{p.label}</span>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-line pt-3">
        <Button tone="danger" icon={Ban} busy={busy} onClick={() => onDecide("deny", payloadIds)}>
          Deny
        </Button>
        <div className="flex flex-wrap items-center gap-2">
          <Button tone="secondary" disabled={noneSelected} busy={busy} onClick={() => onDecide("allow_once", payloadIds)}>
            Allow once
          </Button>
          {a.brief && (
            <Button tone="secondary" disabled={noneSelected} busy={busy} onClick={() => onDecide("allow_brief", payloadIds)}>
              Allow for this brief
            </Button>
          )}
          {a.reach !== "world" && (
            <Button
              tone="primary"
              icon={Check}
              disabled={noneSelected}
              busy={busy}
              onClick={() => onDecide("always_allow", payloadIds)}
              title={a.standingRuleHint}
            >
              Always allow this
            </Button>
          )}
        </div>
      </div>
      {a.reach !== "world" && a.standingRuleHint && <p className="mt-1.5 text-right text-xs text-ink-faint">Creates: {a.standingRuleHint}</p>}
    </Panel>
  );
}
