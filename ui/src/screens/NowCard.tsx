import { ArrowUpRight, CalendarClock, Coffee, Play, Square, Timer } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { App, wire } from "../api";
import ProgressRing from "../components/ProgressRing";
import { Button, Dot, IconButton, cx, stateColor, stateLabel, unassignedColor } from "../components/ui";
import { useDaemon, useTick } from "../daemon";
import { clockFace, formatDate, formatDuration, formatTime, parseDuration } from "../format";
import { useTracking } from "../tracking";
import { Quit } from "../wailsjs/runtime/runtime";

/**
 * The tray's now card: a small bento of the day at a glance, dropped down from
 * the tray icon on a left click. Esc, clicking away, or another click on the
 * icon closes it.
 */
export default function NowCard() {
  const d = useDaemon();
  const t = useTracking();
  useTick(1000);
  const [plan, setPlan] = useState<wire.Plan | null>(null);
  const today = d.today();
  useEffect(() => {
    if (!d.up) return;
    let live = true;
    App.GetPlan(today).then(
      (p) => live && setPlan(p),
      () => live && setPlan(null),
    );
    return () => {
      live = false;
    };
  }, [d.up, today, d.planVersion, d.tasksVersion]);
  useCloseOnLeave();

  const st = d.status;
  const now = d.now();
  if (d.up === false || !st) {
    return (
      <Shell>
        <Tile className="grid flex-1 place-items-center text-center">
          <div>
            <div className="text-[15px] font-medium text-ink">{d.up === false ? "Gwen isn't running" : "Connecting…"}</div>
            {d.up === false && <p className="mt-1 text-xs text-ink-subtle">Start it from the tray menu.</p>}
          </div>
        </Tile>
      </Shell>
    );
  }

  const state = st.state;
  const color = stateColor[state];
  const worked = (st.today?.worked_ms ?? 0) + (t.working && st.today ? now - st.server_now_at : 0);
  const breakMs = (st.today?.break_ms ?? 0) + (t.onBreak && st.today ? now - st.server_now_at : 0);
  const target = (st.today?.target_seconds ?? 0) * 1000 || parseDuration(d.config?.tracking.daily_target ?? "8h");
  const session = st.open_segment ? now - st.open_segment.started_at : 0;
  const project = d.projects.find((p) => p.id === st.project_id);
  const task = d.tasks.find((x) => x.id === st.task_id);
  const met = target > 0 && worked >= target;

  // The block on now, else the next one still to come.
  const blocks = (plan?.items ?? []).filter((it) => it.status === "planned" && it.start_at != null).sort((a, b) => a.start_at! - b.start_at!);
  const current = blocks.find((it) => it.start_at! <= now && now < it.start_at! + it.planned_minutes * 60_000);
  const next = blocks.find((it) => it.start_at! > now);
  const shown = current ?? next;
  const left = blocks.filter((it) => it.start_at! + it.planned_minutes * 60_000 > now).length;

  return (
    <Shell>
      <div className="grid min-h-0 flex-1 grid-cols-6 grid-rows-[auto_auto_auto] gap-2">
        <Tile className="col-span-4 row-span-2 flex flex-col" tint={state === "off" ? undefined : color}>
          <div className="flex items-center gap-2 text-[12.5px] font-medium" style={{ color }}>
            <Dot color={color} live={state === "working"} />
            {stateLabel[state]}
            {state !== "off" && <span className="font-normal text-ink-subtle">since {formatTime(st.state_since_at)}</span>}
          </div>
          {state === "off" ? (
            <div className="mt-2 text-[30px] leading-tight font-semibold tracking-[-0.03em] text-ink-subtle">Off the clock</div>
          ) : (
            <div className={cx("mt-1.5 text-[44px] leading-none font-semibold tracking-[-0.04em] tabular-nums", t.onBreak ? "text-ink-subtle" : "text-ink")} title="Worked today">
              {clockFace(worked).slice(0, -3)}
              <span className="text-ink-faint">{clockFace(worked).slice(-3)}</span>
            </div>
          )}
          <div className="mt-auto min-w-0 pt-3">
            <div className="truncate text-[13.5px] font-medium text-ink" title={task?.title}>
              {task?.title ?? (state === "off" ? "Nothing tracked right now" : "No task")}
            </div>
            <div className="mt-0.5 flex items-center gap-1.5 text-xs text-ink-subtle">
              <Dot color={project?.color ?? unassignedColor} size={6} />
              <span className="truncate">{project?.name ?? "Unassigned"}</span>
            </div>
          </div>
        </Tile>

        <Tile className="col-span-2 flex flex-col justify-between">
          <Label>{formatDate(today)} · IST</Label>
          <div className="text-[22px] leading-none font-semibold tracking-[-0.03em] text-ink tabular-nums">{formatTime(now)}</div>
        </Tile>

        <Tile className="col-span-2 flex items-center gap-2.5">
          <ProgressRing fraction={target > 0 ? worked / target : 0} color={met ? stateColor.working : "var(--color-accent)"} size={42} stroke={4.5} label="Worked against the daily target">
            <span className="text-[10.5px] font-semibold text-ink tabular-nums">{Math.round(target > 0 ? (worked / target) * 100 : 0)}%</span>
          </ProgressRing>
          <div className="min-w-0">
            <Label>Target</Label>
            <div className="text-[13px] font-semibold whitespace-nowrap text-ink tabular-nums">{met ? "Met" : formatDuration(target - worked)}</div>
            {!met && <div className="text-[11px] whitespace-nowrap text-ink-faint">left of {formatDuration(target)}</div>}
          </div>
        </Tile>

        <Tile className="col-span-2 flex flex-col justify-between">
          <Label icon={t.onBreak ? Coffee : Timer}>{t.onBreak ? "This break" : "This stretch"}</Label>
          <div className="text-[17px] font-semibold text-ink tabular-nums">{state === "off" ? "—" : formatDuration(session)}</div>
          <div className="text-[11px] text-ink-faint tabular-nums">Breaks {formatDuration(breakMs)}</div>
        </Tile>

        <Tile className="col-span-4 flex min-w-0 flex-col justify-between">
          <Label icon={CalendarClock}>
            {current ? "On the plan now" : "Next up"}
            {left > 1 && <span className="ml-auto font-normal normal-case">{left - (current ? 1 : 0)} more today</span>}
          </Label>
          {shown ? (
            <div className="flex min-w-0 items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-[13.5px] font-medium text-ink" title={shown.task.title}>
                {shown.task.title}
              </span>
              <span className="shrink-0 text-xs text-ink-subtle tabular-nums">
                {current ? `until ${formatTime(shown.start_at! + shown.planned_minutes * 60_000)}` : `${formatTime(shown.start_at!)} · ${formatDuration(shown.planned_minutes * 60_000)}`}
              </span>
              {!t.isTracking(shown.task.id) && <IconButton icon={Play} label={`Start ${shown.task.title}`} tone="secondary" size="sm" onClick={() => t.startTask(shown.task)} />}
            </div>
          ) : (
            <div className="text-[13px] text-ink-subtle">Nothing else planned today.</div>
          )}
        </Tile>
      </div>

      <div className="flex shrink-0 gap-2">
        {state === "off" && (
          <Button tone="primary" icon={Play} className="flex-1" onClick={() => t.clockIn(d.lastAttribution.project_id, d.lastAttribution.task_id)}>
            Clock in
          </Button>
        )}
        {t.onBreak && (
          <Button tone="primary" icon={Play} className="flex-1" onClick={t.breakEnd}>
            End break
          </Button>
        )}
        {t.working && (
          <Button icon={Coffee} iconColor={stateColor.break_manual} className="flex-1" onClick={t.breakStart}>
            Break
          </Button>
        )}
        {state !== "off" && (
          <Button icon={Square} iconColor="var(--color-danger)" onClick={t.clockOut}>
            Clock out
          </Button>
        )}
        <Button icon={ArrowUpRight} onClick={() => App.OpenDashboard("").then(Quit, () => {})}>
          Dashboard
        </Button>
      </div>
    </Shell>
  );
}

/** Closes the card on Esc, and when focus leaves it after it once had it. */
function useCloseOnLeave() {
  const focused = useRef(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && Quit();
    const onFocus = () => (focused.current = true);
    const onBlur = () => focused.current && Quit();
    window.addEventListener("keydown", onKey);
    window.addEventListener("focus", onFocus);
    window.addEventListener("blur", onBlur);
    if (document.hasFocus()) focused.current = true;
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("focus", onFocus);
      window.removeEventListener("blur", onBlur);
    };
  }, []);
}

function Shell({ children }: { children: ReactNode }) {
  return <div className="flex h-full flex-col gap-2 overflow-hidden border border-line-strong bg-canvas p-2.5 select-none">{children}</div>;
}

function Tile({ className, tint, children }: { className?: string; tint?: string; children: ReactNode }) {
  return (
    <section
      className={cx("min-w-0 rounded-xl border border-line bg-surface-1 p-3", className)}
      style={tint ? { backgroundImage: `linear-gradient(140deg, color-mix(in srgb, ${tint} 14%, transparent), transparent 70%)` } : undefined}
    >
      {children}
    </section>
  );
}

function Label({ icon: Icon, children }: { icon?: typeof Timer; children: ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 text-[10.5px] font-medium tracking-[0.04em] text-ink-faint uppercase">
      {Icon && <Icon size={11} aria-hidden />}
      {children}
    </div>
  );
}
