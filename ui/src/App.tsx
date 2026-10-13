import {
  BookMarked,
  Cable,
  CalendarClock,
  ChartColumn,
  ClipboardCheck,
  Coffee,
  Columns3,
  FolderKanban,
  History as HistoryIcon,
  Inbox as InboxIcon,
  ListChecks,
  Play,
  Settings as SettingsIcon,
  Sparkles,
  Square,
  Sun,
  Target,
  Workflow,
  type LucideIcon,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState, type ComponentType } from "react";
import { App as Host, isHub, onEvent } from "./api";
import BrandMark from "./components/BrandMark";
import { BriefingProvider } from "./components/Briefing";
import { TabGroup } from "./components/TabGroup";
import { FeedbackProvider } from "./components/feedback";
import { Button, Dot, IconButton, Splash, cx, stateColor } from "./components/ui";
import { DaemonProvider, useDaemon, useTick } from "./daemon";
import { clockFace, formatDuration } from "./format";
import { NavContext, useNav } from "./nav";
import Assistant from "./screens/Assistant";
import Board from "./screens/Board";
import Connections from "./screens/Connections";
import FirstRun from "./screens/FirstRun";
import Goals from "./screens/Goals";
import History from "./screens/History";
import Inbox from "./screens/Inbox";
import Login from "./screens/Login";
import Memory from "./screens/Memory";
import NowCard from "./screens/NowCard";
import Plan from "./screens/Plan";
import Projects from "./screens/Projects";
import Review from "./screens/Review";
import Settings from "./screens/Settings";
import Stats from "./screens/Stats";
import Talk from "./screens/Talk";
import Today from "./screens/Today";
import Work from "./screens/Work";
import { useTracking } from "./tracking";

interface Screen {
  id: string;
  label: string;
  icon: LucideIcon;
  view: ComponentType;
}

interface NavGroup {
  id: string;
  label: string;
  icon: LucideIcon;
  tabs: Screen[];
}

// Leaf screens. Each keeps the id it has always had, since the tray (cmd/gwen-tray/main.go) and
// the KDE panel (packaging/plasma/.../main.qml) hardcode "assistant", "inbox", and "board" to open
// the dashboard on them, and several in-app deep links (go("board", "matrix"), go("plan", "chat"),
// go("goals")) do the same. Grouping below is purely a sidebar and layout change; every one of
// those calls still resolves to the same screen it always did.
const todayScreen: Screen = { id: "today", label: "Today", icon: Sun, view: Today };
const assistantScreen: Screen = { id: "assistant", label: "Gwen", icon: Sparkles, view: Assistant };
const inboxScreen: Screen = { id: "inbox", label: "Inbox", icon: InboxIcon, view: Inbox };
const boardScreen: Screen = { id: "board", label: "Board", icon: Columns3, view: Board };
const projectsScreen: Screen = { id: "projects", label: "Projects", icon: FolderKanban, view: Projects };
const planScreen: Screen = { id: "plan", label: "Day plan", icon: CalendarClock, view: Plan };
const goalsScreen: Screen = { id: "goals", label: "Goals", icon: Target, view: Goals };
const workScreen: Screen = { id: "work", label: "Work", icon: Workflow, view: Work };
const weeklyScreen: Screen = { id: "review", label: "Weekly", icon: ClipboardCheck, view: Review };
const statsScreen: Screen = { id: "stats", label: "Stats", icon: ChartColumn, view: Stats };
const historyScreen: Screen = { id: "history", label: "History", icon: HistoryIcon, view: History };
const connectionsScreen: Screen = { id: "connections", label: "Connections", icon: Cable, view: Connections };
const memoryScreen: Screen = { id: "memory", label: "Memory", icon: BookMarked, view: Memory };
const settingsScreen: Screen = { id: "settings", label: "Settings", icon: SettingsIcon, view: Settings };

/** The desktop nav, grouped to plan.md §5.3's eight items: Today, Gwen, Tasks, Plan, Work, Review,
 * Connections, Settings (pinned separately below, as before). Memory has no slot in that table and
 * plan.md does not say where it goes, so it keeps its own slot rather than losing its nav entry on
 * a guess; that gap is still open. */
const navGroups: NavGroup[] = [
  { id: "today", label: "Today", icon: Sun, tabs: [todayScreen] },
  { id: "assistant", label: "Gwen", icon: Sparkles, tabs: [assistantScreen] },
  { id: "tasks", label: "Tasks", icon: ListChecks, tabs: [inboxScreen, boardScreen, projectsScreen] },
  { id: "plan", label: "Plan", icon: CalendarClock, tabs: [planScreen, goalsScreen] },
  { id: "work", label: "Work", icon: Workflow, tabs: [workScreen] },
  { id: "review", label: "Review", icon: ClipboardCheck, tabs: [weeklyScreen, statsScreen, historyScreen] },
  { id: "connections", label: "Connections", icon: Cable, tabs: [connectionsScreen] },
  { id: "memory", label: "Memory", icon: BookMarked, tabs: [memoryScreen] },
];
const settingsGroup: NavGroup = { id: "settings", label: "Settings", icon: SettingsIcon, tabs: [settingsScreen] };
const allGroups = [...navGroups, settingsGroup];

function resolveGroup(screenId: string): { group: NavGroup; tab: Screen } | null {
  for (const g of allGroups) {
    const tab = g.tabs.find((t) => t.id === screenId);
    if (tab) return { group: g, tab };
  }
  return null;
}

/** The sync hub's read-only dashboard keeps its own flat four screens (docs/08-clients.md#v3-
 * additions), unchanged by and untouched by the grouping above. */
const hubScreens: Screen[] = [
  { id: "plan", label: "Plan", icon: CalendarClock, view: Plan },
  { id: "goals", label: "Goals", icon: Target, view: Goals },
  { id: "history", label: "History", icon: HistoryIcon, view: History },
  { id: "stats", label: "Stats", icon: ChartColumn, view: Stats },
];
const hubGroups: NavGroup[] = hubScreens.map((s) => ({ id: s.id, label: s.label, icon: s.icon, tabs: [s] }));

function Shell() {
  const d = useDaemon();
  const [nav, setNav] = useState<{ screen: string; param: string | null }>({ screen: isHub ? hubScreens[0].id : "today", param: null });
  const go = useCallback((screen: string, param: string | null = null) => setNav({ screen, param }), []);
  const main = useRef<HTMLElement>(null);
  useEffect(() => {
    main.current?.scrollTo(0, 0);
  }, [nav.screen]);
  // The tray opens the dashboard on a screen: at launch, or in the running window.
  useEffect(() => {
    if (isHub) return;
    const open = (screen: string) => resolveGroup(screen) !== null && go(screen);
    Host.StartScreen().then(open, () => {});
    return onEvent("navigate", open);
  }, [go]);

  if (d.up === null) return <Splash>Connecting to Gwen…</Splash>;
  if (isHub && d.locked) return <Login />;
  if (!d.up) return isHub ? <Splash>The hub is not reachable. Check that the Pi is on.</Splash> : <FirstRun />;

  if (isHub) {
    const current = hubScreens.find((s) => s.id === nav.screen) ?? hubScreens[0];
    const View = current.view;
    return (
      <NavContext.Provider value={{ ...nav, go }}>
        <BriefingProvider>
          <div className="flex h-full flex-col md:flex-row">
            <Sidebar current={current.id} groups={hubGroups} />
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

  const resolved = resolveGroup(nav.screen) ?? resolveGroup("today")!;
  const View = resolved.tab.view;
  // Today is a dashboard, not prose: it earns a wider stage than the rest of the app, since
  // it's where plan.md §5.3 expects people to live. Everything else keeps the narrower,
  // reading-comfortable cap.
  const wide = resolved.tab.id === "today";
  return (
    <NavContext.Provider value={{ ...nav, go }}>
      <BriefingProvider>
        <div className="flex h-full flex-col md:flex-row">
          <Sidebar current={resolved.group.id} groups={navGroups} settings={settingsGroup} />
          <main ref={main} className="min-w-0 flex-1 overflow-y-auto">
            <div className={cx("@container mx-auto w-full px-4 py-5 md:px-8 md:py-7", wide ? "max-w-420" : "max-w-290")}>
              {resolved.group.tabs.length > 1 ? <TabGroup tabs={resolved.group.tabs} active={resolved.tab.id} /> : <View key={resolved.tab.id} />}
            </div>
          </main>
        </div>
      </BriefingProvider>
    </NavContext.Provider>
  );
}

function Sidebar({ current, groups, settings }: { current: string; groups: NavGroup[]; settings?: NavGroup }) {
  const d = useDaemon();
  const { go } = useNav();
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
      {groups.map((g) => (
        <NavItem key={g.id} label={g.label} icon={g.icon} selected={current === g.id} count={g.id === "tasks" ? inboxCount : 0} onClick={() => go(g.tabs[0].id)} />
      ))}
      <div className="hidden md:block md:flex-1" />
      {!isHub && <StatusDock />}
      {settings && <NavItem label={settings.label} icon={settings.icon} selected={current === settings.id} onClick={() => go(settings.tabs[0].id)} />}
    </nav>
  );
}

function NavItem({ label, icon: Icon, selected, count = 0, onClick }: { label: string; icon: LucideIcon; selected: boolean; count?: number; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={selected ? "page" : undefined}
      className={cx(
        "flex h-8 shrink-0 items-center gap-2.5 rounded-md px-2.5 text-left text-[13.5px] font-medium transition-colors",
        selected ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-subtle hover:bg-surface-1 hover:text-ink",
      )}
    >
      <Icon size={16} aria-hidden className={selected ? "text-ink" : "text-ink-faint"} />
      {label}
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
  // The tray's now card and the widget's talk window run the same frontend
  // in their own small windows.
  const [kind, setKind] = useState<"dashboard" | "now" | "talk" | null>(isHub ? "dashboard" : null);
  useEffect(() => {
    if (!isHub)
      Promise.all([Host.IsNowCard(), Host.IsTalk()]).then(
        ([now, talk]) => setKind(now ? "now" : talk ? "talk" : "dashboard"),
        () => setKind("dashboard"),
      );
  }, []);
  if (kind === null) return null;
  return (
    <FeedbackProvider>
      <DaemonProvider>{kind === "now" ? <NowCard /> : kind === "talk" ? <Talk /> : <Shell />}</DaemonProvider>
    </FeedbackProvider>
  );
}
