import { BellRing, CalendarClock, CornerDownRight, Target, type LucideIcon } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { App, isHub, wire } from "../api";
import { useDaemon } from "../daemon";
import { formatDate, formatDuration, formatLongDate, formatTime } from "../format";
import { useNav } from "../nav";
import { GoalMeter, PaceChip, goalAmount } from "./goal";
import { Button, Modal, cx } from "./ui";

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
  const today = !isHub && d.up && d.status ? d.today() : null; // the hub serves no briefing

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

function greeting(): string {
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

function BriefingModal({ onClose }: { onClose: () => void }) {
  const d = useDaemon();
  const { go } = useNav();
  const [b, setB] = useState<wire.Briefing | null>(null);
  useEffect(() => {
    App.Briefing().then(setB, d.fail);
  }, [d.planVersion, d.fail]);

  const behind = b?.goals.filter((g) => g.progress.pace === "behind") ?? [];
  const empty = b && b.pending.length + b.today.length + b.reminders.length + behind.length === 0;
  const planned = b?.today.filter((it) => it.status !== "skipped").reduce((sum, it) => sum + it.planned_minutes, 0) ?? 0;
  return (
    <Modal
      title={greeting()}
      description={b ? `${formatLongDate(b.day)}${planned ? `. ${formatDuration(planned * 60_000)} planned.` : "."}` : "Loading your day…"}
      onClose={onClose}
      footer={
        <>
          <Button
            icon={CalendarClock}
            onClick={() => {
              onClose();
              go("plan");
            }}
          >
            Open plan
          </Button>
          <Button tone="primary" onClick={onClose} autoFocus>
            Close
          </Button>
        </>
      }
    >
      {empty && <p className="text-sm text-ink-subtle">Nothing is planned, carried over, or due. A clear day.</p>}
      <div className="flex flex-col gap-5">
        {b && b.pending.length > 0 && (
          <Section title="Carried over" icon={CornerDownRight}>
            {b.pending.map((it) => (
              <Row key={it.id} left={it.task.title} right={`from ${formatDate(it.day)}`} />
            ))}
          </Section>
        )}
        {b && b.today.length > 0 && (
          <Section title="Today's plan" icon={CalendarClock}>
            {b.today.map((it) => (
              <Row
                key={it.id}
                left={it.task.title}
                muted={it.status !== "planned"}
                right={`${it.start_at != null ? formatTime(it.start_at) + "  " : ""}${formatDuration(it.planned_minutes * 60_000)}`}
              />
            ))}
          </Section>
        )}
        {b && b.reminders.length > 0 && (
          <Section title="Reminders" icon={BellRing}>
            {b.reminders.map((r) => (
              <Row key={r.task_id} left={r.title} right={r.message} warn={r.days_left < 0} />
            ))}
          </Section>
        )}
        {behind.length > 0 && b && (
          <Section title="Goals behind pace" icon={Target}>
            {behind.map((g) => (
              <li key={g.id} className="flex flex-col gap-2 py-2.5">
                <div className="flex items-center gap-3 text-sm">
                  <span className="min-w-0 flex-1 truncate text-ink">{g.title}</span>
                  <span className="text-xs text-ink-subtle tabular-nums">{goalAmount(g)}</span>
                  <PaceChip pace={g.progress.pace} />
                </div>
                <GoalMeter g={g} today={b.day} />
              </li>
            ))}
          </Section>
        )}
      </div>
    </Modal>
  );
}

function Section({ title, icon: Icon, children }: { title: string; icon: LucideIcon; children: ReactNode }) {
  return (
    <section>
      <h3 className="mb-1 flex items-center gap-2 text-[13px] font-semibold text-ink-muted">
        <Icon size={14} className="text-ink-subtle" aria-hidden />
        {title}
      </h3>
      <ul className="divide-y divide-line">{children}</ul>
    </section>
  );
}

function Row({ left, right, muted, warn }: { left: string; right: string; muted?: boolean; warn?: boolean }) {
  return (
    <li className="flex items-center gap-3 py-2 text-sm">
      <span className={cx("min-w-0 flex-1 truncate", muted ? "text-ink-faint line-through" : "text-ink")}>{left}</span>
      <span className={cx("shrink-0 text-xs whitespace-pre tabular-nums", warn ? "text-danger" : "text-ink-subtle")}>{right}</span>
    </li>
  );
}
