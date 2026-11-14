import { Check, Globe, ShieldAlert, ShieldCheck, Trash2, X } from "lucide-react";
import { Fragment, useState, type ReactNode } from "react";
import type { wire } from "../api";
import { tildify, useHome } from "../shell/bits";
import { Badge, Button, cx, type BadgeTone } from "./ui";

// One approval waiting for you (plan.md 3.7, 3.16): what will happen, exactly, shown the way you read it (an edit as a
// diff, a command as its text, a fetch as its URL), how risky it is, and Allow or Deny. Where the approval offers it,
// Allow can be remembered for the chat or the project; external and destructive steps never offer it.

export type Decision = "approve" | "reject";

const riskTone: Record<string, BadgeTone> = { read: "neutral", local: "neutral", write: "neutral", external: "danger", destructive: "danger" };
const riskWords: Record<string, string> = {
  read: "Reads",
  local: "On this laptop",
  write: "Changes something",
  external: "Leaves this computer",
  destructive: "Can't be undone",
};

interface Payload {
  tool?: string;
  input?: Record<string, unknown>;
  folder?: string;
  why?: string;
}

const str = (v: unknown) => (typeof v === "string" ? v : "");

export function ApprovalCard({
  approval: a,
  onDecide,
  compact,
  projectName,
}: {
  approval: wire.Approval;
  /** Resolves once the daemon has the decision. */
  onDecide: (decision: Decision, remember?: string, note?: string) => Promise<unknown>;
  /** In the narrow side panel. */
  compact?: boolean;
  /** The chat's project, for "Always in <project>". */
  projectName?: string;
}) {
  const [busy, setBusy] = useState("");
  const [denying, setDenying] = useState(false);
  const [note, setNote] = useState("");
  const p = (a.payload ?? {}) as Payload;
  const scopes = a.grant_scopes ?? [];
  const chatScope = scopes.find((s) => s.startsWith("chat:"));
  const projectScope = scopes.find((s) => s.startsWith("project:"));
  const risky = a.risk === "external" || a.risk === "destructive";
  const [title, said = ""] = a.summary.split(" · ");
  // The badge already says the risk ("Leaves this computer: it ..."), so the line under the title gives only the reason.
  const reason = said.startsWith(`${riskWords[a.risk]}: `) ? said.slice(riskWords[a.risk].length + 2) : "";
  const why = reason ? `Because ${reason}.` : said;

  const decide = async (key: string, decision: Decision, remember?: string, text?: string) => {
    setBusy(key);
    try {
      await onDecide(decision, remember, text);
    } finally {
      setBusy("");
    }
  };

  return (
    <div
      role="group"
      aria-label={`Gwen asks: ${title}`}
      className={cx("flex flex-col gap-2.5 rounded-xl border px-3.5 py-3", risky ? "border-danger/35 bg-danger/[0.05]" : "border-info/35 bg-info/[0.05]")}
    >
      <div className="flex items-start gap-2.5">
        <span className={cx("mt-px grid size-6 shrink-0 place-items-center rounded-md", risky ? "bg-danger/15 text-danger" : "bg-info/15 text-info")}>
          {a.risk === "destructive" ? <Trash2 size={13} aria-hidden /> : a.risk === "external" ? <Globe size={13} aria-hidden /> : risky ? <ShieldAlert size={13} aria-hidden /> : <ShieldCheck size={13} aria-hidden />}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="min-w-0 text-[13px] font-medium break-words text-ink">{title}</span>
            <Badge tone={riskTone[a.risk] ?? "neutral"} className="h-[18px] px-1.5 text-[11px]">
              {riskWords[a.risk] ?? a.risk}
            </Badge>
          </div>
          {(p.why || why) && <p className="mt-0.5 text-[11.5px] text-ink-subtle">{p.why ? `Because ${p.why}.` : why}</p>}
        </div>
      </div>
      <PayloadView p={p} compact={compact} />
      {denying ? (
        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            decide("deny", "reject", undefined, note.trim() || undefined);
          }}
        >
          <input
            autoFocus
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Why not, or what to do instead (optional)"
            aria-label="Note for Gwen"
            className="field !h-8 !text-[12.5px]"
          />
          <div className="flex flex-wrap gap-1.5">
            <Button type="submit" size="sm" tone="destroy" icon={X} busy={busy === "deny"} disabled={!!busy}>
              Deny
            </Button>
            <Button size="sm" tone="ghost" onClick={() => setDenying(false)} disabled={!!busy}>
              Back
            </Button>
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-center gap-1.5">
          <Button size="sm" tone="primary" icon={Check} busy={busy === "allow"} disabled={!!busy} onClick={() => decide("allow", "approve")}>
            Allow
          </Button>
          {chatScope && (
            <Button size="sm" busy={busy === "chat"} disabled={!!busy} onClick={() => decide("chat", "approve", chatScope)}>
              Allow for this chat
            </Button>
          )}
          {projectScope && (
            <Button size="sm" busy={busy === "project"} disabled={!!busy} onClick={() => decide("project", "approve", projectScope)} title="Gwen won't ask about this again in this project">
              {compact || !projectName ? "Always here" : `Always in ${projectName}`}
            </Button>
          )}
          <Button size="sm" tone="ghost" icon={X} disabled={!!busy} onClick={() => setDenying(true)}>
            Deny
          </Button>
        </div>
      )}
    </div>
  );
}

/** What the approval will do, by its tool. */
function PayloadView({ p, compact }: { p: Payload; compact?: boolean }) {
  const home = useHome();
  const input = p.input ?? {};
  const path = (v: unknown) => {
    const full = str(v);
    return p.folder && full.startsWith(p.folder + "/") ? full.slice(p.folder.length + 1) : tildify(full, home);
  };
  const box = "max-h-56 overflow-auto rounded-lg border border-line bg-canvas/60 font-mono text-[11.5px] leading-[1.55]";
  let body: ReactNode = null;
  switch (p.tool) {
    case "Edit":
      body = <Diff path={path(input.file_path)} edits={[{ old: str(input.old_string), next: str(input.new_string) }]} className={box} />;
      break;
    case "MultiEdit": {
      const edits = Array.isArray(input.edits) ? (input.edits as Record<string, unknown>[]).map((e) => ({ old: str(e.old_string), next: str(e.new_string) })) : [];
      body = <Diff path={path(input.file_path)} edits={edits} className={box} />;
      break;
    }
    case "Write":
    case "NotebookEdit": {
      const text = str(input.content) || str(input.new_source);
      const lines = text.split("\n");
      body = (
        <div className={box}>
          <div className="border-b border-line px-2.5 py-1 text-ink-muted">{path(input.file_path || input.notebook_path)}</div>
          <pre className="px-2.5 py-1.5 whitespace-pre-wrap text-success">{lines.slice(0, 16).map((l) => "+ " + l).join("\n")}</pre>
          {lines.length > 16 && <div className="px-2.5 pb-1.5 text-ink-subtle">and {lines.length - 16} more lines</div>}
        </div>
      );
      break;
    }
    case "Bash":
      body = (
        <div className={box}>
          <pre className="px-2.5 py-1.5 break-all whitespace-pre-wrap text-ink">
            <span className="text-ink-subtle select-none">$ </span>
            {str(input.command)}
          </pre>
          {p.folder && !compact && <div className="border-t border-line px-2.5 py-1 text-[11px] text-ink-subtle">in {path(p.folder)}</div>}
        </div>
      );
      break;
    case "WebFetch":
      body = <div className={cx(box, "px-2.5 py-1.5 break-all text-ink")}>{str(input.url)}</div>;
      break;
    case "open":
      body = <div className={cx(box, "px-2.5 py-1.5 break-all text-ink")}>{str(input.url) || path(input.folder) || str(input.tab)}</div>;
      break;
    case "WebSearch":
      body = <div className={cx(box, "px-2.5 py-1.5 text-ink")}>{str(input.query)}</div>;
      break;
    case "Read":
    case "Glob":
    case "Grep":
      body = <div className={cx(box, "px-2.5 py-1.5 break-all text-ink")}>{path(input.file_path || input.path) || str(input.pattern)}</div>;
      break;
    case "session_start":
      body = (
        <div className={cx(box, "px-2.5 py-2 font-sans text-[12.5px] leading-relaxed whitespace-pre-wrap text-ink")}>
          {str(input.brief)}
          {input.sandbox === true && <div className="mt-1 text-[11px] text-ink-subtle">In the sandbox</div>}
        </div>
      );
      break;
    case "session_send":
      body = <div className={cx(box, "px-2.5 py-2 font-sans text-[12.5px] whitespace-pre-wrap text-ink")}>{str(input.text)}</div>;
      break;
    case "app_run":
      body = <AppRun input={input} className={box} />;
      break;
    default:
      if (Object.keys(input).length > 0) body = <pre className={cx(box, "px-2.5 py-1.5 whitespace-pre-wrap text-ink-muted")}>{JSON.stringify(input, null, 2)}</pre>;
  }
  return body;
}

const appNames: Record<string, string> = {
  GMAIL: "Gmail",
  SLACK: "Slack",
  GITHUB: "GitHub",
  GOOGLECALENDAR: "Google Calendar",
  NOTION: "Notion",
  GOOGLESHEETS: "Google Sheets",
  GOOGLEDRIVE: "Google Drive",
  GOOGLEDOCS: "Google Docs",
  LINEAR: "Linear",
  JIRA: "Jira",
};
// Who it goes to, then what it says; any other field after them.
const appKeys = ["to", "recipient_email", "recipient", "recipients", "extra_recipients", "cc", "bcc", "channel", "channel_id", "channel_name", "user", "user_id", "users",
  "owner", "repo", "issue_number", "pull_number", "calendar_id", "attendees", "page_id", "parent_id", "database_id", "spreadsheet_id",
  "subject", "title", "name", "summary", "text", "markdown_text", "body", "message", "content", "description", "comment"];

/** An app action (Composio) as you'd check it before it goes out: the app, the action, and every field it sends. */
function AppRun({ input, className }: { input: Record<string, unknown>; className: string }) {
  const [app = "", ...action] = str(input.tool).split("_");
  const args = (input.arguments && typeof input.arguments === "object" && !Array.isArray(input.arguments) ? input.arguments : {}) as Record<string, unknown>;
  const shown = (v: unknown) => v !== undefined && v !== null && v !== "" && !(Array.isArray(v) && v.length === 0);
  const keys = [...appKeys.filter((k) => k in args), ...Object.keys(args).filter((k) => !appKeys.includes(k))].filter((k) => shown(args[k]));
  const words = (v: unknown) =>
    typeof v === "string" ? v : Array.isArray(v) && v.every((x) => typeof x !== "object") ? v.join(", ") : typeof v === "object" ? JSON.stringify(v) : String(v);
  return (
    <div className={className}>
      {keys.length === 0 && (
        <div className="px-2.5 py-1.5 font-sans text-ink-muted">
          {appNames[app] ?? app.charAt(0) + app.slice(1).toLowerCase()} · {action.join(" ").toLowerCase()}, with nothing filled in
        </div>
      )}
      {keys.length > 0 && (
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 px-2.5 py-1.5 font-sans text-[12.5px] leading-relaxed">
          {keys.map((k) => (
            <Fragment key={k}>
              <dt className="text-ink-subtle">{k.replace(/_/g, " ")}</dt>
              <dd className="break-words whitespace-pre-wrap text-ink">{words(args[k])}</dd>
            </Fragment>
          ))}
        </dl>
      )}
    </div>
  );
}

/** An edit as a small diff: the old text's lines out, the new text's in. */
function Diff({ path, edits, className }: { path: string; edits: { old: string; next: string }[]; className: string }) {
  const minus = edits.reduce((n, e) => n + (e.old ? e.old.split("\n").length : 0), 0);
  const plus = edits.reduce((n, e) => n + (e.next ? e.next.split("\n").length : 0), 0);
  return (
    <div className={className}>
      <div className="sticky top-0 flex items-center gap-2 border-b border-line bg-surface-2 px-2.5 py-1">
        <span className="min-w-0 flex-1 truncate text-ink-muted">{path}</span>
        <span className="shrink-0 text-success">+{plus}</span>
        <span className="shrink-0 text-danger">−{minus}</span>
      </div>
      {edits.map((e, i) => (
        <div key={i} className={cx("py-1", i > 0 && "border-t border-line")}>
          {e.old &&
            e.old.split("\n").map((l, j) => (
              <div key={`o${j}`} className="bg-danger/10 px-2.5 whitespace-pre-wrap text-danger">
                <span className="select-none">− </span>
                {l}
              </div>
            ))}
          {e.next &&
            e.next.split("\n").map((l, j) => (
              <div key={`n${j}`} className="bg-success/10 px-2.5 whitespace-pre-wrap text-success">
                <span className="select-none">+ </span>
                {l}
              </div>
            ))}
        </div>
      ))}
    </div>
  );
}
