import { CalendarClock, ChartColumn, History as HistoryIcon, Target, type LucideIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type ComponentType } from "react";
import { App as Host, isHub } from "./api";
import BrandMark from "./components/BrandMark";
import { FeedbackProvider } from "./components/feedback";
import { Splash, cx } from "./components/ui";
import { DaemonProvider, useDaemon } from "./daemon";
import { NavContext } from "./nav";
import Goals from "./screens/Goals";
import History from "./screens/History";
import Login from "./screens/Login";
import NowCard from "./screens/NowCard";
import Plan from "./screens/Plan";
import Stats from "./screens/Stats";
import Talk from "./screens/Talk";

// Gwen's screens live in the Gwen window now (plan.md 3.15). This frontend
// draws what stays outside it: the tray's now card and the Talk window in
// gwen-ui, and the sync hub's read-only pages on the Pi.

interface Screen {
  id: string;
  label: string;
  icon: LucideIcon;
  view: ComponentType;
}

/** The sync hub's read-only dashboard (docs/08-clients.md#v3-additions). */
const hubScreens: Screen[] = [
  { id: "plan", label: "Plan", icon: CalendarClock, view: Plan },
  { id: "goals", label: "Goals", icon: Target, view: Goals },
  { id: "history", label: "History", icon: HistoryIcon, view: History },
  { id: "stats", label: "Stats", icon: ChartColumn, view: Stats },
];

function Hub() {
  const d = useDaemon();
  const [nav, setNav] = useState<{ screen: string; param: string | null }>({ screen: hubScreens[0].id, param: null });
  const go = useCallback((screen: string, param: string | null = null) => setNav({ screen, param }), []);
  const main = useRef<HTMLElement>(null);
  useEffect(() => {
    main.current?.scrollTo(0, 0);
  }, [nav.screen]);

  if (d.up === null) return <Splash>Connecting to Gwen…</Splash>;
  if (d.locked) return <Login />;
  if (!d.up) return <Splash>The hub is not reachable. Check that the Pi is on.</Splash>;

  const current = hubScreens.find((s) => s.id === nav.screen) ?? hubScreens[0];
  const View = current.view;
  return (
    <NavContext.Provider value={{ ...nav, go }}>
      <div className="flex h-full flex-col md:flex-row">
        <nav
          aria-label="Screens"
          className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-line bg-canvas px-3 py-2 md:w-58 md:flex-col md:items-stretch md:gap-0.5 md:overflow-x-visible md:overflow-y-auto md:border-r md:border-b-0 md:px-3 md:py-4"
        >
          <div className="mr-2 flex shrink-0 items-center gap-2.5 px-2 md:mr-0 md:mb-5">
            <BrandMark className="size-6" />
            <span className="text-[15px] font-semibold tracking-[-0.01em]">Gwen</span>
          </div>
          {hubScreens.map((s) => {
            const Icon = s.icon;
            const selected = s.id === current.id;
            return (
              <button
                key={s.id}
                type="button"
                onClick={() => go(s.id)}
                aria-current={selected ? "page" : undefined}
                className={cx(
                  "flex h-8 shrink-0 items-center gap-2.5 rounded-md px-2.5 text-left text-[13.5px] font-medium transition-colors",
                  selected ? "bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--color-line)]" : "text-ink-subtle hover:bg-surface-1 hover:text-ink",
                )}
              >
                <Icon size={16} aria-hidden className={selected ? "text-ink" : "text-ink-faint"} />
                {s.label}
              </button>
            );
          })}
        </nav>
        <main ref={main} className="min-w-0 flex-1 overflow-y-auto">
          <div className="@container mx-auto w-full max-w-290 px-4 py-5 md:px-8 md:py-7">
            <View key={current.id} />
          </div>
        </main>
      </div>
    </NavContext.Provider>
  );
}

export default function App() {
  // The tray's now card and the widget's talk window run the same frontend
  // in their own small windows; the hub serves it to a browser.
  const [kind, setKind] = useState<"hub" | "now" | "talk" | null>(isHub ? "hub" : null);
  useEffect(() => {
    if (!isHub)
      Promise.all([Host.IsNowCard(), Host.IsTalk()]).then(
        ([now]) => setKind(now ? "now" : "talk"),
        () => setKind("talk"),
      );
  }, []);
  if (kind === null) return null;
  return (
    <FeedbackProvider>
      <DaemonProvider>{kind === "now" ? <NowCard /> : kind === "talk" ? <Talk /> : <Hub />}</DaemonProvider>
    </FeedbackProvider>
  );
}
