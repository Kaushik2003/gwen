import { useEffect, useState } from "react";
import { App, apiError, wire } from "../api";
import { Button, Card, Input } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatTime } from "../format";

/** Settings → Calendar (docs/08-clients.md#v3-additions). */
export default function CalendarSettings() {
  const d = useDaemon();
  const [status, setStatus] = useState<wire.CalendarStatus | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [busy, setBusy] = useState((d.config?.calendar.busy_calendars ?? []).join(", "));
  const [syncing, setSyncing] = useState(false);

  useEffect(() => {
    App.CalendarStatus().then(
      (s) => {
        setStatus(s);
        setUnavailable(null);
      },
      (e) => {
        const err = apiError(e);
        if (err.code === "unavailable") setUnavailable(err.message);
        else d.fail(e);
      },
    );
  }, [d.integrationVersion, d.config?.calendar.enabled, d.fail]);
  useEffect(() => setBusy((d.config?.calendar.busy_calendars ?? []).join(", ")), [d.config?.calendar.busy_calendars]);

  async function connect() {
    const auth = await d.act(() => App.CalendarAuthStart());
    if (auth) await d.act(() => App.OpenURL(auth.auth_url));
  }
  async function syncNow() {
    setSyncing(true);
    const s = await d.act(() => App.CalendarSync());
    setSyncing(false);
    if (s) setStatus(s);
  }
  async function saveBusy() {
    const list = busy.split(",").map((s) => s.trim()).filter(Boolean);
    await d.act(() => App.PatchConfig({ calendar: { busy_calendars: list } }));
  }

  return (
    <Card title="Calendar">
      {unavailable ? (
        <p className="text-sm text-zinc-500">
          Google Calendar is not set up: {unavailable}. It needs your own Google OAuth client; run{" "}
          <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">gwen setup calendar</code> for the steps.
        </p>
      ) : (
        status && (
          <div className="flex flex-col gap-3 text-sm">
            <div className="flex items-center gap-2">
              <span
                className={`inline-block h-2.5 w-2.5 rounded-full ${status.connected ? "bg-emerald-500" : "bg-zinc-400"}`}
              />
              <span className="flex-1">
                {status.connected ? "Connected to Google Calendar" : "Not connected yet"}
                {status.last_sync_at != null && (
                  <span className="text-zinc-500"> · last sync {formatTime(status.last_sync_at)}</span>
                )}
              </span>
              <Button onClick={connect}>{status.connected ? "Reconnect" : "Connect"}</Button>
              <Button tone="primary" onClick={syncNow} disabled={!status.connected || syncing}>
                {syncing ? "Syncing…" : "Sync now"}
              </Button>
            </div>
            {status.last_error && <p className="text-xs text-red-600 dark:text-red-400">{status.last_error}</p>}
            <label className="flex flex-col gap-1">
              <span className="text-zinc-600 dark:text-zinc-400">Busy calendars (their events reduce planning time)</span>
              <div className="flex gap-2">
                <Input className="flex-1" value={busy} onChange={(e) => setBusy(e.target.value)} placeholder="primary" />
                <Button onClick={saveBusy}>Save</Button>
              </div>
            </label>
          </div>
        )
      )}
    </Card>
  );
}
