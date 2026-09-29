import { useState } from "react";
import { BriefingProvider } from "./components/Briefing";
import { DaemonProvider, useDaemon } from "./daemon";
import FirstRun from "./screens/FirstRun";
import Goals from "./screens/Goals";
import History from "./screens/History";
import Plan from "./screens/Plan";
import Projects from "./screens/Projects";
import Settings from "./screens/Settings";
import Stats from "./screens/Stats";
import Today from "./screens/Today";

const screens = [
  { id: "today", label: "Today", view: Today },
  { id: "plan", label: "Plan", view: Plan },
  { id: "goals", label: "Goals", view: Goals },
  { id: "history", label: "History", view: History },
  { id: "stats", label: "Stats", view: Stats },
  { id: "projects", label: "Projects & tasks", view: Projects },
  { id: "settings", label: "Settings", view: Settings },
] as const;

type ScreenId = (typeof screens)[number]["id"];

function Shell() {
  const d = useDaemon();
  const [screen, setScreen] = useState<ScreenId>("today");
  if (d.up === null) return <div className="p-8 text-zinc-500">Connecting to Gwen…</div>;
  if (!d.up) return <FirstRun />;
  const View = screens.find((s) => s.id === screen)!.view;
  return (
    <BriefingProvider>
      <div className="flex h-full">
        <nav className="flex w-52 shrink-0 flex-col gap-1 border-r border-zinc-200 bg-white p-3 dark:border-zinc-800 dark:bg-zinc-900">
          <div className="mb-4 flex items-center gap-2 px-2 pt-1">
            <span className="inline-block h-6 w-6 rounded-md bg-emerald-500" />
            <span className="text-lg font-semibold">Gwen</span>
          </div>
          {screens.map((s) => (
            <button
              key={s.id}
              onClick={() => setScreen(s.id)}
              className={`rounded-md px-3 py-2 text-left text-sm ${
                screen === s.id
                  ? "bg-emerald-50 font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                  : "text-zinc-600 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
              }`}
            >
              {s.label}
            </button>
          ))}
        </nav>
        <main className="flex-1 overflow-y-auto p-6">
          {d.notice && (
            <div className="mb-4 flex items-center justify-between rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900 ring-1 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900">
              <span>{d.notice}</span>
              <button className="ml-4 text-xs underline" onClick={() => d.setNotice(null)}>
                Dismiss
              </button>
            </div>
          )}
          <View />
        </main>
      </div>
    </BriefingProvider>
  );
}

export default function App() {
  return (
    <DaemonProvider>
      <Shell />
    </DaemonProvider>
  );
}
