import { Power, RotateCcw } from "lucide-react";
import { useEffect, useState } from "react";
import BrandMark from "../components/BrandMark";
import { App, apiError } from "../api";
import { Button, Callout, ToggleRow } from "../components/ui";
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
      <div className="w-full max-w-120">
        <BrandMark className="size-12" />
        <h1 className="mt-6 text-headline font-semibold text-ink">Gwen isn't running yet</h1>
        <p className="mt-3 text-[15px] leading-relaxed text-ink-muted">
          Gwen clocks your working day, notices when you step away, nudges you back, and turns your goals into daily plans. Everything stays on this computer.
        </p>
        <p className="mt-3 text-[15px] leading-relaxed text-ink-subtle">
          Start its background service so it can track your time. It starts again every time you log in, and closing this window never stops it.
        </p>
        <div className="mt-7 flex flex-wrap items-center gap-2">
          <Button tone="primary" size="lg" icon={Power} onClick={start} busy={busy}>
            {busy ? "Starting…" : "Start Gwen"}
          </Button>
          <Button size="lg" icon={RotateCcw} onClick={() => d.refresh()}>
            Try again
          </Button>
        </div>
        <div className="mt-7 border-t border-line pt-5">
          <ToggleRow title="Show the tray icon at login" description="Clock in, take breaks, and clock out from the panel." checked={tray} onChange={toggleTray} />
        </div>
        {error && (
          <Callout tone="danger" className="mt-5">
            {error}
          </Callout>
        )}
      </div>
    </div>
  );
}
