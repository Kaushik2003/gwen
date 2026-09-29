import QRCode from "qrcode";
import { useEffect, useState } from "react";
import { App, apiError, wire } from "../api";
import { Button, Card, Input, Label, Select } from "../components/ui";
import { useDaemon } from "../daemon";
import AISettings from "../settings/AI";
import CalendarSettings from "../settings/Calendar";
import SyncSettings from "../settings/Sync";

type Kind = "text" | "bool" | { options: string[] };
interface Field {
  key: string;
  label: string;
  kind: Kind;
  hint?: string;
}

const tracking: Field[] = [
  { key: "daily_target", label: "Daily target", kind: "text", hint: "e.g. 8h or 7h30m" },
  { key: "soft_idle", label: "Idle prompt after", kind: "text" },
  { key: "hard_idle", label: "Automatic break after", kind: "text" },
  { key: "day_rollover", label: "Day starts at", kind: "text", hint: "HH:MM" },
];
const nudge: Field[] = [
  { key: "desktop", label: "Desktop notifications", kind: "bool" },
  { key: "phone", label: "Phone notifications", kind: "bool" },
  { key: "break_reminder", label: "Break reminder after", kind: "text", hint: "0s turns it off" },
  { key: "repeat", label: "Repeat every", kind: "text" },
  { key: "snooze", label: "Snooze for", kind: "text" },
];
const ntfy: Field[] = [
  { key: "server", label: "Server", kind: "text" },
  { key: "fallback_server", label: "Fallback server", kind: "text", hint: "empty for none" },
  { key: "topic", label: "Topic", kind: "text" },
];
const log: Field[] = [{ key: "level", label: "Level", kind: { options: ["debug", "info", "warn", "error"] } }];

export default function Settings() {
  const d = useDaemon();
  const [health, setHealth] = useState<wire.Health | null>(null);
  const [autostart, setAutostart] = useState(false);
  useEffect(() => {
    App.Health().then(setHealth, d.fail);
    App.Autostart().then(setAutostart);
  }, [d.fail]);
  if (!d.config) return null;
  const c = d.config;
  return (
    <div className="flex max-w-3xl flex-col gap-4">
      <Section title="Tracking" name="tracking" fields={tracking} values={c.tracking} />
      <Section title="Nudges" name="nudge" fields={nudge} values={c.nudge} />
      <Section title="Phone" name="ntfy" fields={ntfy} values={c.ntfy}>
        <Phone config={c} />
      </Section>
      <CalendarSettings />
      <AISettings />
      <SyncSettings />
      <Section title="Log" name="log" fields={log} values={c.log} />
      <Card title="About">
        <label className="mb-3 flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={autostart}
            onChange={async (e) => {
              const on = e.target.checked;
              try {
                await App.SetAutostart(on);
                setAutostart(on);
              } catch (err) {
                d.fail(err);
              }
            }}
          />
          Start the tray icon at login
        </label>
        <div className="text-sm text-zinc-500">
          Dashboard <Version /> · daemon {health?.version ?? "…"} · schema {health?.schema_version ?? "…"}
        </div>
      </Card>
    </div>
  );
}

function Version() {
  const [v, setV] = useState("");
  useEffect(() => {
    App.Version().then(setV);
  }, []);
  return <>{v}</>;
}

/** One config section: a draft of its values, saved as a PATCH of the changed keys. */
function Section({ title, name, fields, values, children }: { title: string; name: string; fields: Field[]; values: object; children?: React.ReactNode }) {
  const d = useDaemon();
  const current = values as Record<string, unknown>;
  const [draft, setDraft] = useState<Record<string, unknown>>(current);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  useEffect(() => setDraft(values as Record<string, unknown>), [values]);

  const changed = fields.filter((f) => draft[f.key] !== current[f.key]);
  async function save() {
    const patch: Record<string, unknown> = {};
    for (const f of changed) patch[f.key] = draft[f.key];
    setSaving(true);
    try {
      await App.PatchConfig({ [name]: patch });
      setErrors({});
    } catch (e) {
      const err = apiError(e);
      const key = typeof err.details.key === "string" ? err.details.key : "";
      if (key.startsWith(name + ".")) {
        setErrors({ [key.slice(name.length + 1)]: err.message.replace(`${key}: `, "") });
      } else {
        d.fail(e);
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card
      title={title}
      actions={
        <>
          {changed.length > 0 && <Button onClick={() => (setDraft(current), setErrors({}))}>Revert</Button>}
          <Button tone="primary" disabled={changed.length === 0 || saving} onClick={save}>
            Save
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        {fields.map((f) => (
          <Label key={f.key} text={f.label} error={errors[f.key]}>
            <FieldInput field={f} value={draft[f.key]} onChange={(v) => setDraft({ ...draft, [f.key]: v })} />
            {f.hint && !errors[f.key] && <span className="text-xs text-zinc-500">{f.hint}</span>}
          </Label>
        ))}
      </div>
      {children}
    </Card>
  );
}

function FieldInput({ field, value, onChange }: { field: Field; value: unknown; onChange: (v: unknown) => void }) {
  if (field.kind === "bool") {
    return <input type="checkbox" className="self-start" checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />;
  }
  if (typeof field.kind === "object") {
    return (
      <Select value={String(value)} onChange={(e) => onChange(e.target.value)}>
        {field.kind.options.map((o) => (
          <option key={o}>{o}</option>
        ))}
      </Select>
    );
  }
  return <Input value={String(value ?? "")} onChange={(e) => onChange(e.target.value)} />;
}

function Phone({ config }: { config: wire.Config }) {
  const d = useDaemon();
  const url = `${config.ntfy.server.replace(/\/+$/, "")}/${config.ntfy.topic}`;
  const [qr, setQr] = useState("");
  const [result, setResult] = useState<wire.NotifyTestResult | null>(null);
  const [testing, setTesting] = useState(false);
  useEffect(() => {
    QRCode.toDataURL(url, { margin: 1, width: 180 }).then(setQr, () => setQr(""));
  }, [url]);

  return (
    <div className="mt-4 flex items-start gap-5 border-t border-zinc-200 pt-4 dark:border-zinc-800">
      {qr && <img src={qr} alt={url} className="h-[180px] w-[180px] rounded bg-white" />}
      <div className="flex flex-col gap-3 text-sm">
        <p>
          Subscribe to <code className="break-all">{url}</code> in the ntfy app to get nudges on your phone. Anyone who knows the topic can
          read them, so keep it private.
        </p>
        <div className="flex gap-2">
          <Button
            onClick={async () => {
              const topic = await App.NewTopic();
              await d.act(() => App.PatchConfig({ ntfy: { topic } }));
            }}
          >
            New topic
          </Button>
          <Button
            disabled={testing}
            onClick={async () => {
              setTesting(true);
              setResult((await d.act(() => App.NotifyTest())) ?? null);
              setTesting(false);
            }}
          >
            Send test
          </Button>
        </div>
        {result && (
          <ul className="text-xs">
            <li>Desktop: {result.desktop}</li>
            <li>Phone: {result.phone}</li>
          </ul>
        )}
      </div>
    </div>
  );
}
