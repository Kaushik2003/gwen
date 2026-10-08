import QRCode from "qrcode";
import { BellRing, Brain, CalendarClock, CalendarDays, CircleCheck, Cloud, Copy, Info, RefreshCw, Send, Smartphone, Sparkles, Timer, Wrench, type LucideIcon } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { App, apiError, wire } from "../api";
import DurationInput from "../components/DurationInput";
import { useConfirm, useToast } from "../components/feedback";
import TimeInput from "../components/TimeInput";
import { Badge, Button, Callout, Input, PageHeader, Panel, Select, ToggleRow, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { useNav } from "../nav";
import AISettings from "../settings/AI";
import CalendarSettings from "../settings/Calendar";
import { CredentialField } from "../settings/Credential";
import MethodsSettings from "../settings/Methods";
import PersonalitySettings from "../settings/Personality";
import SyncSettings from "../settings/Sync";
import VoiceSettings from "../settings/Voice";
import { ClipboardSetText } from "../wailsjs/runtime/runtime";

type Kind =
  | { type: "duration"; units?: ("h" | "m" | "s")[] }
  | { type: "time" }
  | { type: "bool" }
  | { type: "text"; placeholder?: string }
  | { type: "select"; options: { value: string; label: string }[] };

interface FieldDef {
  key: string;
  label: string;
  hint?: ReactNode;
  kind: Kind;
}

const tracking: FieldDef[] = [
  { key: "daily_target", label: "Daily target", hint: "Worked time that makes a full day.", kind: { type: "duration" } },
  { key: "soft_idle", label: "Ask if you're away after", hint: "No keyboard or mouse for this long sends a nudge. The time still counts as work.", kind: { type: "duration", units: ["m", "s"] } },
  { key: "hard_idle", label: "Count it as a break after", hint: "Away this long, and the time since you left becomes a break.", kind: { type: "duration", units: ["m", "s"] } },
  { key: "day_rollover", label: "A new day starts at", hint: "Work after midnight counts toward the day before until this time.", kind: { type: "time" } },
];
const nudge: FieldDef[] = [
  { key: "desktop", label: "Desktop notifications", kind: { type: "bool" } },
  { key: "phone", label: "Phone notifications", hint: "Through the ntfy app. Set it up under Phone.", kind: { type: "bool" } },
  { key: "break_reminder", label: "Remind me to take a break after", hint: "Of work without one. Zero turns it off.", kind: { type: "duration" } },
  { key: "repeat", label: "Repeat an unanswered nudge every", kind: { type: "duration", units: ["m", "s"] } },
  { key: "snooze", label: "Snooze nudges for", kind: { type: "duration" } },
];
const ntfy: FieldDef[] = [
  { key: "server", label: "Server", hint: "ntfy.sh, or your own on the Pi.", kind: { type: "text", placeholder: "https://ntfy.sh" } },
  { key: "fallback_server", label: "Fallback server", hint: "Tried when the first one fails. Empty for none.", kind: { type: "text", placeholder: "http://raspberrypi:8080" } },
  { key: "topic", label: "Topic", hint: "Works like a password: anyone who knows it can read your nudges.", kind: { type: "text" } },
];
const planner: FieldDef[] = [
  { key: "day_start", label: "Plans start at", kind: { type: "time" } },
  { key: "day_end", label: "Plans end at", kind: { type: "time" } },
  { key: "buffer", label: "Keep free each day", hint: "Plans leave this much of the day unplanned.", kind: { type: "duration" } },
];
const log: FieldDef[] = [
  {
    key: "level",
    label: "Log detail",
    hint: "How much the background service writes to its log, which journalctl --user -u gwend shows.",
    kind: {
      type: "select",
      options: [
        { value: "debug", label: "Everything (debug)" },
        { value: "info", label: "Normal (info)" },
        { value: "warn", label: "Warnings" },
        { value: "error", label: "Errors only" },
      ],
    },
  },
];

const sections: { id: string; label: string; icon: LucideIcon }[] = [
  { id: "tracking", label: "Tracking", icon: Timer },
  { id: "nudges", label: "Nudges", icon: BellRing },
  { id: "phone", label: "Phone", icon: Smartphone },
  { id: "planner", label: "Planner", icon: CalendarClock },
  { id: "methods", label: "Methods", icon: Brain },
  { id: "calendar", label: "Google Calendar", icon: CalendarDays },
  { id: "ai", label: "AI", icon: Sparkles },
  { id: "sync", label: "Sync", icon: Cloud },
  { id: "advanced", label: "Advanced", icon: Wrench },
  { id: "about", label: "About", icon: Info },
];

export default function Settings() {
  const d = useDaemon();
  const { param } = useNav();
  const [section, setSection] = useState(param && sections.some((s) => s.id === param) ? param : "tracking");
  useEffect(() => {
    if (param && sections.some((s) => s.id === param)) setSection(param);
  }, [param]);
  if (!d.config) return null;
  const c = d.config;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader title="Settings" />
      <div className="grid items-start gap-6 @3xl:grid-cols-[12.5rem_minmax(0,1fr)]">
        <nav aria-label="Settings sections" className="flex flex-wrap gap-1 @3xl:sticky @3xl:top-0 @3xl:flex-col @3xl:flex-nowrap @3xl:gap-0.5">
          {sections.map((s) => {
            const Icon = s.icon;
            const on = s.id === section;
            return (
              <button
                key={s.id}
                type="button"
                onClick={() => setSection(s.id)}
                aria-current={on ? "page" : undefined}
                className={cx(
                  "flex h-8 shrink-0 items-center gap-2.5 rounded-md px-2.5 text-left text-[13.5px] font-medium transition-colors",
                  on ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-subtle hover:bg-surface-1 hover:text-ink",
                )}
              >
                <Icon size={15} aria-hidden className={on ? "text-ink" : "text-ink-faint"} />
                {s.label}
              </button>
            );
          })}
        </nav>
        <div className="@container flex max-w-3xl min-w-0 flex-col gap-5">
          {section === "tracking" && <ConfigForm title="Tracking" description="How Gwen counts your day." name="tracking" fields={tracking} values={c.tracking} />}
          {section === "nudges" && <ConfigForm title="Nudges" description="When Gwen taps you on the shoulder." name="nudge" fields={nudge} values={c.nudge} />}
          {section === "phone" && <PhoneSettings config={c} />}
          {section === "planner" && <ConfigForm title="Planner" description="The hours Gwen plans tasks into." name="planner" fields={planner} values={c.planner} />}
          {section === "calendar" && <CalendarSettings />}
          {section === "methods" && <MethodsSettings />}
          {section === "ai" && (
            <>
              <AISettings />
              <VoiceSettings />
              <PersonalitySettings />
            </>
          )}
          {section === "sync" && <SyncSettings />}
          {section === "advanced" && <ConfigForm title="Advanced" name="log" fields={log} values={c.log} />}
          {section === "about" && <About />}
        </div>
      </div>
    </div>
  );
}

/** One config section: a draft of its values, saved as a PATCH of the changed keys. */
function ConfigForm({ title, description, name, fields, values }: { title: string; description?: string; name: string; fields: FieldDef[]; values: object }) {
  const d = useDaemon();
  const notify = useToast();
  const current = values as Record<string, unknown>;
  const [draft, setDraft] = useState<Record<string, unknown>>(current);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    setDraft(values as Record<string, unknown>);
  }, [values]);

  const changed = fields.filter((f) => draft[f.key] !== current[f.key]);
  async function save() {
    const patch: Record<string, unknown> = {};
    for (const f of changed) patch[f.key] = draft[f.key];
    setSaving(true);
    try {
      await App.PatchConfig({ [name]: patch });
      setErrors({});
      notify(`Saved ${title.toLowerCase()} settings`, "success");
    } catch (e) {
      const err = apiError(e);
      const key = typeof err.details.key === "string" ? err.details.key : "";
      if (key.startsWith(name + ".")) setErrors({ [key.slice(name.length + 1)]: err.message.replace(`${key}: `, "") });
      else d.fail(e);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Panel title={title}>
      {description && <p className="-mt-1 mb-2 text-[13px] text-ink-subtle">{description}</p>}
      <div className="flex flex-col divide-y divide-line">
        {fields.map((f) => (
          <SettingRow key={f.key} field={f} value={draft[f.key]} error={errors[f.key]} onChange={(v) => setDraft({ ...draft, [f.key]: v })} />
        ))}
      </div>
      <div className="mt-4 flex items-center justify-end gap-2 border-t border-line pt-4">
        {changed.length > 0 && (
          <Button
            tone="ghost"
            onClick={() => {
              setDraft(current);
              setErrors({});
            }}
          >
            Revert
          </Button>
        )}
        <Button tone="primary" disabled={changed.length === 0} busy={saving} onClick={save}>
          Save changes
        </Button>
      </div>
    </Panel>
  );
}

function SettingRow({ field: f, value, error, onChange }: { field: FieldDef; value: unknown; error?: string; onChange: (v: unknown) => void }) {
  if (f.kind.type === "bool")
    return (
      <div className="py-3.5">
        <ToggleRow title={f.label} description={f.hint} checked={Boolean(value)} onChange={onChange} />
      </div>
    );
  let control: ReactNode;
  switch (f.kind.type) {
    case "duration":
      control = <DurationInput value={String(value ?? "0s")} onChange={onChange} units={f.kind.units} label={f.label} />;
      break;
    case "time":
      control = <TimeInput value={String(value ?? "")} onCommit={onChange} aria-label={f.label} className="w-24" />;
      break;
    case "select":
      control = (
        <Select value={String(value)} onChange={(e) => onChange(e.target.value)} aria-label={f.label} className="w-52">
          {f.kind.options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      );
      break;
    default:
      control = <Input value={String(value ?? "")} placeholder={f.kind.placeholder} onChange={(e) => onChange(e.target.value)} aria-label={f.label} className="w-72 max-w-full" spellCheck={false} />;
  }
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3.5">
      <div className="min-w-0 flex-1 basis-56">
        <div className="text-sm font-medium text-ink">{f.label}</div>
        {f.hint && <div className="mt-0.5 text-[13px] leading-relaxed text-ink-subtle">{f.hint}</div>}
        {error && <div className="mt-1 text-xs text-danger">{error}</div>}
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  );
}

function PhoneSettings({ config }: { config: wire.Config }) {
  const d = useDaemon();
  const notify = useToast();
  const confirm = useConfirm();
  const url = `${config.ntfy.server.replace(/\/+$/, "")}/${config.ntfy.topic}`;
  const [qr, setQr] = useState("");
  const [result, setResult] = useState<wire.NotifyTestResult | null>(null);
  const [testing, setTesting] = useState(false);
  useEffect(() => {
    QRCode.toDataURL(url, { margin: 1, width: 360, color: { dark: "#010102", light: "#ffffff" } }).then(setQr, () => setQr(""));
  }, [url]);

  async function newTopic() {
    const ok = await confirm({
      title: "Make a new topic?",
      body: "Your phone stops getting nudges until you subscribe to the new topic in the ntfy app. Do this if the old one may have leaked.",
      confirm: "Make new topic",
    });
    if (!ok) return;
    const topic = await App.NewTopic();
    if (await d.act(() => App.PatchConfig({ ntfy: { topic } }))) notify("New topic made. Subscribe to it on your phone.", "success");
  }

  return (
    <>
      <Panel title="Nudges on your phone" icon={Smartphone}>
        {!config.nudge.phone && (
          <Callout
            tone="idle"
            className="mb-4"
            action={
              <Button size="sm" onClick={() => d.act(() => App.PatchConfig({ nudge: { phone: true } }))}>
                Turn on
              </Button>
            }
          >
            Phone notifications are off.
          </Callout>
        )}
        <div className="flex flex-col gap-6 @lg:flex-row @lg:items-start">
          {qr && <img src={qr} alt={`QR code for ${url}`} className="size-44 shrink-0 rounded-xl bg-white p-2" />}
          <ol className="flex min-w-0 flex-col gap-4 text-sm">
            <Step n={1}>Install the free ntfy app. Android: Play Store or F-Droid. iOS: App Store.</Step>
            <Step n={2}>
              Tap + and subscribe to this topic, or scan the code.
              <span className="mt-2 flex items-center gap-2">
                <code className="min-w-0 truncate rounded-md border border-line bg-surface-2 px-2 py-1 text-xs text-ink-muted">{url}</code>
                <Button
                  size="sm"
                  icon={Copy}
                  onClick={() => {
                    ClipboardSetText(url);
                    notify("Copied the topic link", "success");
                  }}
                >
                  Copy
                </Button>
              </span>
            </Step>
            <Step n={3}>
              Send a test to check that it arrives.
              <span className="mt-2 flex flex-wrap items-center gap-2">
                <Button
                  size="sm"
                  icon={Send}
                  busy={testing}
                  onClick={async () => {
                    setTesting(true);
                    setResult((await d.act(() => App.NotifyTest())) ?? null);
                    setTesting(false);
                  }}
                >
                  Send test
                </Button>
                {result && (
                  <>
                    <Badge tone={result.desktop === "sent" ? "working" : "neutral"}>Desktop: {result.desktop}</Badge>
                    <Badge tone={result.phone === "sent" ? "working" : "neutral"}>Phone: {result.phone}</Badge>
                  </>
                )}
              </span>
            </Step>
          </ol>
        </div>
        <div className="mt-5 flex justify-end border-t border-line pt-4">
          <Button icon={RefreshCw} onClick={newTopic}>
            Make a new topic
          </Button>
        </div>
      </Panel>
      <ConfigForm title="ntfy server" name="ntfy" fields={ntfy} values={config.ntfy} />
      <Panel title="Access token">
        <CredentialField name="ntfy_token" label="ntfy access token" hint="Only for a server that needs one, such as your own with access control." />
      </Panel>
    </>
  );
}

function Step({ n, children }: { n: number; children: ReactNode }) {
  return (
    <li className="flex gap-3">
      <span className="grid size-6 shrink-0 place-items-center rounded-full bg-surface-3 text-xs font-semibold text-ink-muted">{n}</span>
      <span className="min-w-0 flex-1 pt-0.5 leading-relaxed text-ink-muted">{children}</span>
    </li>
  );
}

function About() {
  const d = useDaemon();
  const [health, setHealth] = useState<wire.Health | null>(null);
  const [version, setVersion] = useState("");
  const [autostart, setAutostart] = useState(false);
  useEffect(() => {
    App.Health().then(setHealth, d.fail);
    App.Version().then(setVersion);
    App.Autostart().then(setAutostart);
  }, [d.fail]);
  return (
    <Panel title="About">
      <div className="flex flex-col divide-y divide-line">
        <div className="py-3.5">
          <ToggleRow
            title="Show the tray icon at login"
            description="The tray holds the same controls as the status box in the sidebar. On KDE, the Gwen panel widget takes its place when it is on a panel."
            checked={autostart}
            onChange={async (on) => {
              try {
                await App.SetAutostart(on);
                setAutostart(on);
              } catch (e) {
                d.fail(e);
              }
            }}
          />
        </div>
        <dl className="grid grid-cols-[10rem_1fr] gap-y-2 py-3.5 text-sm">
          <dt className="text-ink-subtle">Dashboard</dt>
          <dd className="text-ink tabular-nums">{version || "…"}</dd>
          <dt className="text-ink-subtle">Background service</dt>
          <dd className="flex items-center gap-2 text-ink tabular-nums">
            {health?.version ?? "…"}
            {health?.ok && (
              <Badge tone="working" icon={CircleCheck}>
                Running
              </Badge>
            )}
          </dd>
          <dt className="text-ink-subtle">Database schema</dt>
          <dd className="text-ink tabular-nums">{health?.schema_version ?? "…"}</dd>
          <dt className="text-ink-subtle">Activity detection</dt>
          <dd className="text-ink">{d.status?.activity_backend ?? "…"}</dd>
        </dl>
      </div>
    </Panel>
  );
}
