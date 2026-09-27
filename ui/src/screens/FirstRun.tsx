import { useEffect, useState } from "react";
import { App, apiError } from "../api";
import { Button, Card } from "../components/ui";
import { useDaemon } from "../daemon";

/** Shown instead of everything else while the daemon is not running. */
export default function FirstRun() {
  const d = useDaemon();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tray, setTray] = useState(false);

  useEffect(() => {
    App.Autostart().then(setTray);
  }, []);

  async function start() {
    setBusy(true);
    setError(null);
    try {
      await App.EnableService();
      await d.refresh();
    } catch (e) {
      setError(apiError(e).message);
    } finally {
      setBusy(false);
    }
  }

  async function toggleTray(on: boolean) {
    try {
      await App.SetAutostart(on);
      setTray(on);
    } catch (e) {
      setError(apiError(e).message);
    }
  }

  return (
    <div className="flex h-full items-center justify-center p-6">
      <Card className="w-[34rem]">
        <div className="mb-4 flex items-center gap-3">
          <span className="inline-block h-10 w-10 rounded-lg bg-emerald-500" />
          <h1 className="text-2xl font-semibold">Welcome to Gwen</h1>
        </div>
        <p className="mb-3 text-sm text-zinc-600 dark:text-zinc-300">
          Gwen clocks your working day, notices when you step away, nudges you back, and turns your goals into daily
          plans. Everything stays on this computer.
        </p>
        <p className="mb-5 text-sm text-zinc-600 dark:text-zinc-300">
          Gwen isn't running yet. Start its background service so it can track your time; it starts again every time
          you log in, and closing this window never stops it.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Button tone="primary" onClick={start} disabled={busy}>
            {busy ? "Starting…" : "Start and run at login"}
          </Button>
          <Button onClick={() => d.refresh()}>Retry</Button>
          <label className="ml-auto flex items-center gap-2 text-sm">
            <input type="checkbox" checked={tray} onChange={(e) => toggleTray(e.target.checked)} />
            Show the tray icon at login
          </label>
        </div>
        {error && <p className="mt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}
      </Card>
    </div>
  );
}
