import { useMemo } from "react";
import { cx } from "../components/ui";
import { NavContext, useNav } from "../nav";
import Assistant from "../screens/Assistant";
import Board from "../screens/Board";
import Goals from "../screens/Goals";
import History from "../screens/History";
import InboxScreen, { type InboxView } from "../screens/Inbox";
import Memory from "../screens/Memory";
import Plan from "../screens/Plan";
import Projects from "../screens/Projects";
import Review from "../screens/Review";
import Settings from "../screens/Settings";
import Stats from "../screens/Stats";
import { Scroll } from "./bits";
import { screenLabel, subScreens, tabId, useTabs, type Tab } from "./tabs";
import { useWork } from "./work";

// Gwen's screens in the window (plan.md 3.15, boards 8 to 16): each place in
// the sidebar is one tab whose sub-screens are the old dashboard's screens,
// which keep reading the screen and param they always did.

type ScreenTab = Extract<Tab, { kind: "screen" }>;

/** The old screen id a hub's sub-screen is, for useNav().screen. */
const oldId: Record<string, string> = {
  "inbox/needs": "work",
  "inbox/captured": "inbox",
  "gwen/chat": "assistant",
  "gwen/memory": "memory",
  "plan/day": "plan",
  "plan/board": "board",
  "plan/goals": "goals",
  "review/week": "review",
  "review/stats": "stats",
  "review/history": "history",
};

export function HubScreen({ tab }: { tab: ScreenTab }) {
  const tabs = useTabs();
  const w = useWork();
  const outer = useNav();
  const subs = subScreens[tab.screen];
  const sub = tab.sub && subs.some((s) => s.id === tab.sub) ? tab.sub : tab.screen === "inbox" && w.needs.length === 0 ? "captured" : (subs[0]?.id ?? "");
  const key = `${tab.screen}/${sub}`;
  const nav = useMemo(() => ({ ...outer, screen: oldId[key] ?? tab.screen, param: tab.param ?? null }), [outer, key, tab.screen, tab.param]);

  return (
    <NavContext.Provider value={nav}>
      <div className="flex h-full flex-col">
        {subs.length > 0 && tab.screen !== "inbox" && (
          <div role="tablist" aria-label={screenLabel[tab.screen]} className="flex h-10 shrink-0 items-end gap-5 border-b border-chrome-line px-7">
            {subs.map((s) => {
              const on = s.id === sub;
              const count = tab.screen === "inbox" && s.id === "needs" ? w.needs.length : 0;
              return (
                <button
                  key={s.id}
                  type="button"
                  role="tab"
                  aria-selected={on}
                  onClick={() => tabs.update(tabId(tab), { ...tab, sub: s.id, param: null })}
                  className={cx("-mb-px flex h-10 items-center gap-1.5 border-b-2 text-[13px] font-medium", on ? "border-accent text-ink" : "border-transparent text-ink-subtle hover:text-ink")}
                >
                  {s.label}
                  {count > 0 && <span className="rounded-full bg-warning/15 px-1.5 text-[11px] font-semibold text-warning tabular-nums">{count}</span>}
                </button>
              );
            })}
          </div>
        )}
        <div className="min-h-0 flex-1">
          {/* Plan › Day scrolls itself, so Plan with Gwen can stand the full height beside it. */}
          {key === "plan/day" ? (
            <Body screen={tab.screen} sub={sub} />
          ) : (
            <Scroll wide={key === "plan/board" || key === "review/history"}>
              <Body screen={tab.screen} sub={sub} />
            </Scroll>
          )}
        </div>
      </div>
    </NavContext.Provider>
  );
}

function Body({ screen, sub }: { screen: ScreenTab["screen"]; sub: string }) {
  switch (`${screen}/${sub}`) {
    case "inbox/needs":
    case "inbox/captured":
      return <InboxHub view={sub as InboxView} />;
    case "gwen/chat":
      return <Assistant />;
    case "gwen/memory":
      return <Memory />;
    case "plan/day":
      return <Plan />;
    case "plan/board":
      return <Board />;
    case "plan/goals":
      return <Goals />;
    case "review/week":
      return <Review />;
    case "review/stats":
      return <Stats />;
    case "review/history":
      return <History />;
  }
  if (screen === "settings") return <Settings />;
  if (screen === "projects") return <Projects />;
  return null;
}

/** The Inbox switches between Needs you and Captured in its own header, so its views stay the tab's sub-screen. */
function InboxHub({ view }: { view: InboxView }) {
  const tabs = useTabs();
  return <InboxScreen view={view} onView={(v) => tabs.update("screen:inbox", { kind: "screen", screen: "inbox", sub: v, param: null })} />;
}
