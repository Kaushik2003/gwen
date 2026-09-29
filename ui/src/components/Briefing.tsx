import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { App, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatDuration, formatTime } from "../format";
import { GoalBar, PaceChip, goalAmount } from "../screens/Goals";
import { Button } from "./ui";

/** Where the day the briefing was last dismissed is kept, per viewer. */
const dismissedKey = "gwen.briefing.dismissed";

function dismissedDay(): string | null {
  try {
    return localStorage.getItem(dismissedKey);
  } catch {
    return null;
  }
}

function dismiss(day: string) {
  try {
    localStorage.setItem(dismissedKey, day);
  } catch {
    // Without storage the briefing simply shows again next launch.
  }
}

const BriefingContext = createContext<() => void>(() => {});

/** Opens the briefing modal. */
export function useBriefing(): () => void {
  return useContext(BriefingContext);
}

/** Shows the briefing on the first open of each day, and whenever asked. */
export function BriefingProvider({ children }: { children: ReactNode }) {
  const d = useDaemon();
  const [open, setOpen] = useState(false);
  const today = d.up && d.status ? d.today() : null;

  useEffect(() => {
    if (today && dismissedDay() !== today) setOpen(true);
  }, [today]);

  const close = useCallback(() => {
    if (today) dismiss(today);
    setOpen(false);
  }, [today]);

  return (
    <BriefingContext.Provider value={() => setOpen(true)}>
      {children}
      {open && <BriefingModal onClose={close} />}
    </BriefingContext.Provider>
  );
}

function BriefingModal({ onClose }: { onClose: () => void }) {
  const d = useDaemon();
  const [b, setB] = useState<wire.Briefing | null>(null);
  useEffect(() => {
    App.Briefing().then(setB, d.fail);
  }, [d.planVersion, d.fail]);

  const behind = b?.goals.filter((g) => g.progress.pace === "behind") ?? [];
  const empty = b && b.pending.length + b.today.length + b.reminders.length + behind.length === 0;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="max-h-[85vh] w-[36rem] max-w-[92vw] overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-zinc-900"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold">Briefing{b ? ` · ${b.day}` : ""}</h2>
        {!b && <p className="mt-4 text-sm text-zinc-500">Loading…</p>}
        {empty && <p className="mt-4 text-sm text-zinc-500">Nothing is planned, pending, or due.</p>}

        {b && b.pending.length > 0 && (
          <Section title="Carried over">
            {b.pending.map((it) => (
              <Row key={it.id} left={it.task.title} right={`from ${it.day}`} />
            ))}
          </Section>
        )}
        {b && b.today.length > 0 && (
          <Section title="Today's plan">
            {b.today.map((it) => (
              <Row
                key={it.id}
                left={it.task.title}
                muted={it.status !== "planned"}
                right={`${it.start_at != null ? formatTime(it.start_at) + " · " : ""}${formatDuration(it.planned_minutes * 60_000)}`}
              />
            ))}
          </Section>
        )}
        {b && b.reminders.length > 0 && (
          <Section title="Reminders">
            {b.reminders.map((r) => (
              <Row key={r.task_id} left={r.title} right={r.message} warn={r.days_left < 0} />
            ))}
          </Section>
        )}
        {behind.length > 0 && (
          <Section title="Goals behind pace">
            {behind.map((g) => (
              <li key={g.id} className="flex flex-col gap-1 py-1.5 text-sm">
                <div className="flex items-center gap-2">
                  <span className="flex-1">{g.title}</span>
                  <span className="text-xs tabular-nums text-zinc-500">{goalAmount(g)}</span>
                  <PaceChip pace={g.progress.pace} />
                </div>
                <GoalBar g={g} />
              </li>
            ))}
          </Section>
        )}
        <div className="mt-6 flex justify-end">
          <Button tone="primary" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-5">
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-zinc-500">{title}</h3>
      <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">{children}</ul>
    </section>
  );
}

function Row({ left, right, muted, warn }: { left: string; right: string; muted?: boolean; warn?: boolean }) {
  return (
    <li className="flex items-center gap-3 py-1.5 text-sm">
      <span className={`flex-1 ${muted ? "text-zinc-400" : ""}`}>{left}</span>
      <span className={`text-xs ${warn ? "text-red-600 dark:text-red-400" : "text-zinc-500"}`}>{right}</span>
    </li>
  );
}
