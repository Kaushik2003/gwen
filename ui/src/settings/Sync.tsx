import { useEffect, useState } from "react";
import { App, apiError, wire } from "../api";
import { Button, Card, Input, Label } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";

/** Settings → Sync (docs/08-clients.md#v3-additions). */
export default function SyncSettings() {
  const d = useDaemon();
  const [hubURL, setHubURL] = useState(d.config?.sync.hub_url ?? "");
  const [token, setToken] = useState("");
  const [status, setStatus] = useState<wire.SyncStatus | null>(null);
  const [syncing, setSyncing] = useState(false);

  useEffect(() => setHubURL(d.config?.sync.hub_url ?? ""), [d.config?.sync.hub_url]);
  useEffect(() => {
    App.SyncStatus().then(setStatus, (e) => {
      if (apiError(e).code === "unavailable") setStatus(null);
      else d.fail(e);
    });
  }, [d.integrationVersion, d.config?.sync.hub_url, d.fail]);

  async function save() {
    if (!(await d.act(() => App.PatchConfig({ sync: { hub_url: hubURL.trim().replace(/\/+$/, "") } })))) return;
    if (token.trim()) {
      try {
        await App.SetCredential("sync_token", token.trim());
        setToken("");
      } catch (e) {
        d.fail(e);
        return;
      }
    }
    App.SyncStatus().then(setStatus, () => setStatus(null));
  }
  async function syncNow() {
    setSyncing(true);
    const s = await d.act(() => App.SyncNow());
    setSyncing(false);
    if (s) setStatus(s);
  }
  const when = (ms?: number | null) => (ms != null ? formatTime(ms) : "never");

  return (
    <Card title="Sync">
      <p className="mb-3 text-xs text-zinc-500">
        Copies your data to the hub on your Raspberry Pi, where your phone can see it and a new install can restore it.
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Label text="Hub URL">
          <Input value={hubURL} onChange={(e) => setHubURL(e.target.value)} placeholder="http://raspberrypi:7777" />
        </Label>
        <Label text="Sync token (leave empty to keep the current one)">
          <Input type="password" value={token} onChange={(e) => setToken(e.target.value)} autoComplete="off" />
        </Label>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-3 text-sm">
        <Button onClick={save}>Save</Button>
        <Button tone="primary" onClick={syncNow} disabled={!status?.configured || syncing}>
          {syncing ? "Syncing…" : "Sync now"}
        </Button>
        {status?.configured ? (
          <span className="text-zinc-500">
            Last push {when(status.last_push_at)} · last pull {when(status.last_pull_at)}
          </span>
        ) : (
          <span className="text-zinc-500">Not set up yet.</span>
        )}
      </div>
      {status?.last_error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{status.last_error}</p>}
    </Card>
  );
}
