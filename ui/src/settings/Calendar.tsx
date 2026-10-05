import { CalendarDays, CircleCheck, ExternalLink, FileJson, Plus, RefreshCw, X } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { App, apiError, wire } from "../api";
import { useToast } from "../components/feedback";
import { Badge, Button, Callout, Input, Panel, ToggleRow } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatAgo } from "../format";
import { useHasCredential } from "./Credential";

/** Settings → Calendar (docs/08-clients.md#v3-additions), set up end to end without the CLI. */
export default function CalendarSettings() {
  const d = useDaemon();
  const notify = useToast();
  const enabled = d.config?.calendar.enabled ?? false;
  const [status, setStatus] = useState<wire.CalendarStatus | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const [hasClient] = useHasCredential("google_client.json", version);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    App.CalendarStatus().then(
      (s) => {
        setStatus(s);
        setUnavailable(null);
      },
      (e) => {
        const err = apiError(e);
        setStatus(null);
        if (err.code === "unavailable") setUnavailable(err.message);
        else d.fail(e);
      },
    );
  }, [d.integrationVersion, enabled, version, d.fail]);

  async function chooseClient() {
    setBusy("client");
    try {
      if (await App.ChooseGoogleClient()) {
        setVersion((v) => v + 1);
        notify("Installed the OAuth client. Now connect your Google account.", "success");
      }
    } catch (e) {
      d.fail(e);
    }
    setBusy(null);
  }
  async function connect() {
    setBusy("connect");
    const auth = await d.act(() => App.CalendarAuthStart());
    if (auth) await d.act(() => App.OpenURL(auth.auth_url));
    setBusy(null);
  }
  async function syncNow() {
    setBusy("sync");
    const s = await d.act(() => App.CalendarSync());
    setBusy(null);
    if (s) {
      setStatus(s);
      if (!s.last_error) notify("Synced with Google Calendar", "success");
    }
  }

  const connected = enabled && status?.connected;
  return (
    <>
      <Panel title="Google Calendar" icon={CalendarDays}>
        <p className="-mt-1 mb-4 text-[13px] leading-relaxed text-ink-subtle">
          Gwen writes your plan to a calendar of its own and reads busy times from your other calendars, so plans fit around meetings.
        </p>

        {connected && status ? (
          <div className="flex flex-wrap items-center gap-3 rounded-lg border border-line bg-surface-2 px-4 py-3">
            <CircleCheck size={18} className="text-working" aria-hidden />
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium text-ink">Connected</div>
              <div className="text-xs text-ink-subtle">{status.last_sync_at != null ? `Last synced ${formatAgo(status.last_sync_at, Date.now())}` : "Not synced yet"}</div>
            </div>
            <Button size="sm" onClick={connect} busy={busy === "connect"}>
              Reconnect
            </Button>
            <Button size="sm" tone="primary" icon={RefreshCw} onClick={syncNow} busy={busy === "sync"}>
              Sync now
            </Button>
          </div>
        ) : hasClient === false ? (
          <ol className="flex flex-col gap-4 text-sm">
            <Step n={1}>
              Google needs every app to have its own sign-in client, so you make one once. In Google Cloud Console, create a project and turn on the Google Calendar API.
              <span className="mt-2 block">
                <Button size="sm" icon={ExternalLink} onClick={() => d.act(() => App.OpenURL("https://console.cloud.google.com/apis/library/calendar-json.googleapis.com"))}>
                  Open Google Cloud Console
                </Button>
              </span>
            </Step>
            <Step n={2}>Set up the OAuth consent screen as External, add yourself as a test user, then publish the app. Unverified is fine for personal use; unpublished, Google signs you out every 7 days.</Step>
            <Step n={3}>Create an OAuth client of type Desktop app and download its JSON file.</Step>
            <Step n={4}>
              Choose that file here.
              <span className="mt-2 block">
                <Button size="sm" tone="primary" icon={FileJson} onClick={chooseClient} busy={busy === "client"}>
                  Choose client file
                </Button>
              </span>
            </Step>
            <Step n={5}>Connect, and sign in with Google in the browser window that opens.</Step>
          </ol>
        ) : hasClient && !enabled ? (
          <Callout
            action={
              <Button size="sm" tone="primary" onClick={() => d.act(() => App.PatchConfig({ calendar: { enabled: true } }))}>
                Turn on
              </Button>
            }
          >
            The calendar is off. Your OAuth client is installed, so turning it on is all it takes.
          </Callout>
        ) : (
          hasClient && (
            <div className="flex flex-col gap-3">
              <Callout
                tone="accent"
                action={
                  <Button size="sm" tone="primary" onClick={connect} busy={busy === "connect"}>
                    Connect Google Calendar
                  </Button>
                }
              >
                Your OAuth client is installed. Connect, and sign in with Google in the browser window that opens.
              </Callout>
              {unavailable && <p className="text-xs text-ink-subtle">{unavailable.replace(/;\s*run gwen.*$/i, "")}</p>}
            </div>
          )
        )}

        {status?.last_error && (
          <Callout tone="danger" className="mt-4">
            {status.last_error}
          </Callout>
        )}

        {hasClient && (
          <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
            <span className="text-xs text-ink-subtle">Using a different Google Cloud project? Install its client file instead.</span>
            <Button size="sm" icon={FileJson} onClick={chooseClient} busy={busy === "client"}>
              Replace client file
            </Button>
          </div>
        )}
      </Panel>
      {hasClient && <CalendarOptions />}
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

/** Which calendar Gwen writes to and which ones count as busy. */
function CalendarOptions() {
  const d = useDaemon();
  const notify = useToast();
  const cal = d.config!.calendar;
  const [name, setName] = useState(cal.name);
  const [busy, setBusy] = useState<string[]>(cal.busy_calendars ?? []);
  const [adding, setAdding] = useState("");
  useEffect(() => {
    setName(cal.name);
  }, [cal.name]);
  useEffect(() => {
    setBusy(cal.busy_calendars ?? []);
  }, [cal.busy_calendars]);

  const changed = name !== cal.name || busy.join("\n") !== (cal.busy_calendars ?? []).join("\n");
  async function save() {
    if (await d.act(() => App.PatchConfig({ calendar: { name: name.trim(), busy_calendars: busy } }))) notify("Saved calendar settings", "success");
  }
  function add() {
    const v = adding.trim();
    if (v && !busy.includes(v)) setBusy([...busy, v]);
    setAdding("");
  }

  return (
    <Panel title="Calendar options">
      <div className="flex flex-col divide-y divide-line">
        <div className="pb-3.5">
          <ToggleRow
            title="Use Google Calendar"
            description="Off stops syncing but keeps your sign-in."
            checked={cal.enabled}
            onChange={(on) => d.act(() => App.PatchConfig({ calendar: { enabled: on } }))}
          />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3.5">
          <div className="min-w-0 flex-1 basis-56">
            <div className="text-sm font-medium text-ink">Gwen's calendar</div>
            <div className="mt-0.5 text-[13px] text-ink-subtle">The calendar your plan is written to. Gwen creates it.</div>
          </div>
          <Input value={name} onChange={(e) => setName(e.target.value)} className="w-56" aria-label="Gwen's calendar" />
        </div>
        <div className="py-3.5">
          <div className="text-sm font-medium text-ink">Busy calendars</div>
          <div className="mt-0.5 text-[13px] text-ink-subtle">Their events take time out of the plan. primary is your main calendar; others go by their calendar ID.</div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            {busy.map((b) => (
              <Badge key={b} className="h-7 gap-1.5 pr-1 pl-2.5 text-[13px]">
                {b}
                <button type="button" aria-label={`Remove ${b}`} onClick={() => setBusy(busy.filter((x) => x !== b))} className="grid size-5 place-items-center rounded-full hover:bg-surface-4">
                  <X size={12} aria-hidden />
                </button>
              </Badge>
            ))}
            <form
              className="flex gap-1.5"
              onSubmit={(e) => {
                e.preventDefault();
                add();
              }}
            >
              <Input value={adding} onChange={(e) => setAdding(e.target.value)} placeholder="Calendar ID" className="h-7 w-52 text-[13px]" aria-label="Calendar to add" />
              <Button size="sm" icon={Plus} type="submit" disabled={!adding.trim()}>
                Add
              </Button>
            </form>
          </div>
        </div>
      </div>
      <div className="mt-2 flex justify-end border-t border-line pt-4">
        <Button tone="primary" onClick={save} disabled={!changed || !name.trim()}>
          Save changes
        </Button>
      </div>
    </Panel>
  );
}
