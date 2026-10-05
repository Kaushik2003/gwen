import { CircleCheck, Cloud, ExternalLink, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { App, apiError, wire } from "../api";
import DurationInput from "../components/DurationInput";
import { useToast } from "../components/feedback";
import { Badge, Button, Callout, Input, Panel } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";
import { useHasCredential } from "./Credential";

/** Settings → Sync (docs/08-clients.md#v3-additions). */
export default function SyncSettings() {
  const d = useDaemon();
  const notify = useToast();
  const sync = d.config?.sync;
  const [hubURL, setHubURL] = useState(sync?.hub_url ?? "");
  const [every, setEvery] = useState(sync?.interval ?? "5m");
  const [token, setToken] = useState("");
  const [hasToken, setHasToken] = useHasCredential("sync_token");
  const [status, setStatus] = useState<wire.SyncStatus | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setHubURL(sync?.hub_url ?? "");
  }, [sync?.hub_url]);
  useEffect(() => {
    setEvery(sync?.interval ?? "5m");
  }, [sync?.interval]);
  useEffect(() => {
    App.SyncStatus().then(setStatus, (e) => {
      if (apiError(e).code === "unavailable") setStatus(null);
      else d.fail(e);
    });
  }, [d.integrationVersion, sync?.hub_url, d.fail]);

  async function save() {
    setSaving(true);
    const ok = await d.act(() => App.PatchConfig({ sync: { hub_url: hubURL.trim().replace(/\/+$/, ""), interval: every } }));
    if (ok && token.trim()) {
      try {
        await App.SetCredential("sync_token", token.trim());
        setToken("");
        setHasToken(true);
      } catch (e) {
        d.fail(e);
        setSaving(false);
        return;
      }
    }
    setSaving(false);
    if (!ok) return;
    notify("Saved sync settings", "success");
    App.SyncStatus().then(setStatus, () => setStatus(null));
  }
  async function syncNow() {
    setSyncing(true);
    const s = await d.act(() => App.SyncNow());
    setSyncing(false);
    if (s) {
      setStatus(s);
      if (!s.last_error) notify("Synced with the hub", "success");
    }
  }
  const now = Date.now();
  const changed = hubURL.trim().replace(/\/+$/, "") !== (sync?.hub_url ?? "") || every !== sync?.interval || token.trim() !== "";

  return (
    <Panel title="Sync with your hub" icon={Cloud}>
      <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">
        Copies your data to the hub on your Raspberry Pi, where your phone can open the dashboard and a new install can restore everything.
      </p>
      {status?.configured ? (
        <div className="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-line bg-surface-2 px-4 py-3">
          <CircleCheck size={18} className={status.last_error ? "text-ink-faint" : "text-working"} aria-hidden />
          <div className="min-w-0 flex-1 text-xs text-ink-subtle">
            <div className="text-sm font-medium text-ink">{sync?.hub_url}</div>
            Sent {status.last_push_at != null ? formatAgo(status.last_push_at, now) : "never"}, received {status.last_pull_at != null ? formatAgo(status.last_pull_at, now) : "never"}
          </div>
          <Button size="sm" icon={ExternalLink} onClick={() => d.act(() => App.OpenURL(`${sync?.hub_url}/`))}>
            Open hub dashboard
          </Button>
          <Button size="sm" tone="primary" icon={RefreshCw} onClick={syncNow} busy={syncing}>
            Sync now
          </Button>
        </div>
      ) : (
        <Callout className="mb-4">Not set up yet. Run the hub on your Pi, then give its address and token here.</Callout>
      )}
      {status?.last_error && (
        <Callout tone="danger" className="mb-4">
          {status.last_error}
        </Callout>
      )}
      <div className="flex flex-col divide-y divide-line">
        <Row label="Hub address" hint="Such as http://raspberrypi:7777.">
          <Input value={hubURL} onChange={(e) => setHubURL(e.target.value)} placeholder="http://raspberrypi:7777" className="w-72 max-w-full" aria-label="Hub address" spellCheck={false} />
        </Row>
        <Row
          label="Sync token"
          hint={
            hasToken ? (
              <Badge tone="working" icon={CircleCheck}>
                Saved
              </Badge>
            ) : (
              "The token the hub printed when it was set up."
            )
          }
        >
          <Input type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder={hasToken ? "Type a new one to replace it" : "Paste it here"} autoComplete="off" className="w-72 max-w-full" aria-label="Sync token" />
        </Row>
        <Row label="Sync every">
          <DurationInput value={every} onChange={setEvery} units={["h", "m"]} label="Sync every" />
        </Row>
      </div>
      <div className="mt-2 flex justify-end border-t border-line pt-4">
        <Button tone="primary" onClick={save} busy={saving} disabled={!changed}>
          Save changes
        </Button>
      </div>
    </Panel>
  );
}

function Row({ label, hint, children }: { label: string; hint?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3.5">
      <div className="min-w-0 flex-1 basis-48">
        <div className="text-sm font-medium text-ink">{label}</div>
        {hint && <div className="mt-1 text-[13px] text-ink-subtle">{hint}</div>}
      </div>
      {children}
    </div>
  );
}
