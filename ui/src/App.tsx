import {
  CalendarClock,
  ChartColumn,
  ClipboardCheck,
  Coffee,
  Columns3,
  FolderKanban,
  History as HistoryIcon,
  Inbox as InboxIcon,
  Play,
  Settings as SettingsIcon,
  Sparkles,
  Square,
  Sun,
  Target,
  type LucideIcon,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState, type ComponentType } from "react";
import { App as Host, isHub, onEvent } from "./api";
import BrandMark from "./components/BrandMark";
import { BriefingProvider } from "./components/Briefing";
import { FeedbackProvider } from "./components/feedback";
import { Button, Dot, IconButton, Splash, cx, stateColor } from "./components/ui";
import { DaemonProvider, useDaemon, useTick } from "./daemon";
import { clockFace, formatDuration } from "./format";
import { NavContext, useNav } from "./nav";
import Assistant from "./screens/Assistant";
import Board from "./screens/Board";
import FirstRun from "./screens/FirstRun";
import Goals from "./screens/Goals";
import History from "./screens/History";
import Inbox from "./screens/Inbox";
import Login from "./screens/Login";
import Plan from "./screens/Plan";
import Projects from "./screens/Projects";
import Review from "./screens/Review";
import Settings from "./screens/Settings";
import Stats from "./screens/Stats";
import Today from "./screens/Today";
import { useTracking } from "./tracking";

interface Screen {
  id: string;
  label: string;
  icon: LucideIcon;
  view: ComponentType;
}

const allScreens: Screen[] = [
  { id: "today", label: "Today", icon: Sun, view: Today },
  { id: "assistant", label: "Assistant", icon: Sparkles, view: Assistant },
  { id: "inbox", label: "Inbox", icon: InboxIcon, view: Inbox },
  { id: "board", label: "Board", icon: Columns3, view: Board },
  { id: "plan", label: "Plan", icon: CalendarClock, view: Plan },
  { id: "goals", label: "Goals", icon: Target, view: Goals },
  { id: "review", label: "Weekly review", icon: ClipboardCheck, view: Review },
  { id: "history", label: "History", icon: HistoryIcon, view: History },
  { id: "stats", label: "Stats", icon: ChartColumn, view: Stats },
  { id: "projects", label: "Projects & tasks", icon: FolderKanban, view: Projects },
  { id: "settings", label: "Settings", icon: SettingsIcon, view: Settings },
];

/** The hub's read-only dashboard shows only these (docs/08-clients.md#v3-additions). */
const hubScreens = new Set(["history", "stats", "plan", "goals"]);
const screens = isHub ? allScreens.filter((s) => hubScreens.has(s.id)) : allScreens;

function Shell() {
  const d = useDaemon();
  const [nav, setNav] = useState<{ screen: string; param: string | null }>({ screen: screens[0].id, param: null });
  const go = useCallback((screen: string, param: string | null = null) => setNav({ screen, param }), []);
  const main = useRef<HTMLElement>(null);
  useEffect(() => {
    main.current?.scrollTo(0, 0);
  }, [nav.screen]);
  // The tray opens the dashboard on a screen: at launch, or in the running window.
  useEffect(() => {
    if (isHub) return;
    const open = (screen: string) => screens.some((s) => s.id === screen) && go(screen);
    Host.StartScreen().then(open, () => {});
    return onEvent("navigate", open);
  }, [go]);

  if (d.up === null) return <Splash>Connecting to Gwen…</Splash>;
  if (isHub && d.locked) return <Login />;
  if (!d.up) return isHub ? <Splash>The hub is not reachable. Check that the Pi is on.</Splash> : <FirstRun />;
  const current = screens.find((s) => s.id === nav.screen) ?? screens[0];
  const View = current.view;
  return (
    <NavContext.Provider value={{ ...nav, go }}>
      <BriefingProvider>
        <div className="flex h-full flex-col md:flex-row">
          <Sidebar current={current.id} />
          <main ref={main} className="min-w-0 flex-1 overflow-y-auto">
            <div className="@container mx-auto w-full max-w-290 px-4 py-5 md:px-8 md:py-7">
              <View key={current.id} />
            </div>
          </main>
        </div>
      </BriefingProvider>
    </NavContext.Provider>
  );
}

function Sidebar({ current }: { current: string }) {
  const d = useDaemon();
  const settings = screens.find((s) => s.id === "settings");
  const inboxCount = d.tasks.filter((t) => t.stage === "inbox" && !t.parent_id).length;
  return (
    <nav
      aria-label="Screens"
      className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-line bg-canvas px-3 py-2 md:w-58 md:flex-col md:items-stretch md:gap-0.5 md:overflow-x-visible md:overflow-y-auto md:border-r md:border-b-0 md:px-3 md:py-4"
    >
      <div className="mr-2 flex shrink-0 items-center gap-2.5 px-2 md:mr-0 md:mb-5">
        <BrandMark className="size-6" />
        <span className="text-[15px] font-semibold tracking-[-0.01em]">Gwen</span>
      </div>
      {screens
        .filter((s) => s !== settings)
        .map((s) => (
          <NavItem key={s.id} screen={s} selected={current === s.id} count={s.id === "inbox" ? inboxCount : 0} />
        ))}
      <div className="hidden md:block md:flex-1" />
      {!isHub && <StatusDock />}
      {settings && <NavItem screen={settings} selected={current === settings.id} />}
    </nav>
  );
}

function NavItem({ screen, selected, count = 0 }: { screen: Screen; selected: boolean; count?: number }) {
  const { go } = useNav();
  const Icon = screen.icon;
  return (
    <button
      type="button"
      onClick={() => go(screen.id)}
      aria-current={selected ? "page" : undefined}
      className={cx(
        "flex h-8 shrink-0 items-center gap-2.5 rounded-md px-2.5 text-left text-[13.5px] font-medium transition-colors",
        selected ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-subtle hover:bg-surface-1 hover:text-ink",
      )}
    >
      <Icon size={16} aria-hidden className={selected ? "text-ink" : "text-ink-faint"} />
      {screen.label}
      {count > 0 && <span className="ml-auto rounded-full bg-accent/20 px-1.5 text-[11px] font-semibold text-accent-hover tabular-nums">{count}</span>}
    </button>
  );
}

const dockLabel: Record<string, string> = {
  off: "Not clocked in",
  working: "Working",
  idle_pending: "Idle",
  break_auto: "On break",
  break_manual: "On break",
};

/** The tray's controls on every screen: state, the day's worked timer, and the next action. */
function StatusDock() {
  const d = useDaemon();
  const t = useTracking();
  const { go } = useNav();
  useTick(1000);
  const st = d.status;
  if (!st) return null;
  const color = stateColor[st.state];
  const now = d.now();
  // The day's worked time goes on from its total, so a break or a switch never restarts it.
  const worked = (st.today?.worked_ms ?? 0) + (t.working && st.today ? now - st.server_now_at : 0);
  const breakFor = t.onBreak && st.open_segment ? now - st.open_segment.started_at : 0;
  const project = d.projects.find((p) => p.id === st.project_id);
  const task = d.tasks.find((x) => x.id === st.task_id);
  return (
    <div className="lift mb-3 hidden rounded-xl border border-line bg-surface-1 p-3 md:block">
      <button type="button" onClick={() => go("today")} className="block w-full text-left" title="Open Today">
        <span className="flex items-center gap-2">
          <Dot color={color} live={st.state === "working"} />
          <span className="text-[13px] font-medium" style={{ color }}>
            {dockLabel[st.state]}
            {t.onBreak && ` · ${formatDuration(breakFor)}`}
          </span>
          {st.state !== "off" && (
            <span className={cx("ml-auto text-[13px] font-semibold tabular-nums", t.onBreak ? "text-ink-subtle" : "text-ink")} title="Worked today">
              {clockFace(worked)}
            </span>
          )}
        </span>
        {st.state !== "off" && (
          <span className="mt-1 block truncate text-xs text-ink-subtle">{task?.title ?? project?.name ?? "Unassigned"}</span>
        )}
      </button>
      <div className="mt-3 flex gap-1.5">
        {st.state === "off" && (
          <Button tone="primary" size="sm" icon={Play} className="flex-1" onClick={() => t.clockIn(d.lastAttribution.project_id, d.lastAttribution.task_id)}>
            Clock in
          </Button>
        )}
        {t.onBreak && (
          <Button tone="primary" size="sm" icon={Play} className="flex-1" onClick={t.breakEnd}>
            End break
          </Button>
        )}
        {t.working && (
          <Button size="sm" icon={Coffee} iconColor={stateColor.break_manual} className="flex-1" onClick={t.breakStart}>
            Break
          </Button>
        )}
        {st.state !== "off" && <IconButton icon={Square} label="Clock out" tone="secondary" size="sm" onClick={t.clockOut} />}
      </div>
    </div>
  );
}

export default function App() {
  return (
    <FeedbackProvider>
      <DaemonProvider>
        <Shell />
      </DaemonProvider>
    </FeedbackProvider>
  );
}
