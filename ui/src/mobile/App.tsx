import { Inbox as InboxIcon, LayoutGrid, Plus } from "lucide-react";
import { Inbox } from "./Inbox";
import { NewSession } from "./NewSession";
import { Pair } from "./Pair";
import { SessionScreen } from "./SessionScreen";
import { Spaces } from "./Spaces";
import { Provider, useStore } from "./store";
import { TerminalScreen } from "./TerminalScreen";
import { ago, clock } from "./ui";

export function App() {
  return (
    <Provider>
      <Shell />
    </Provider>
  );
}

function Shell() {
  const { route, go, sessions, online, meta, heardAt } = useStore();
  if (route.name === "pair") return <Pair />;
  const needs = sessions.filter((s) => s.state === "needs_you").length;
  const home = route.name === "inbox" || route.name === "spaces";
  return (
    <div className="app">
      {online === false && (
        <div className="banner warn" role="status">
          Can't reach <b>{meta.machine || "the laptop"}</b>. It may be asleep, or Tailscale is off on this phone.
          {heardAt > 0 && <> Showing what it said {Date.now() - heardAt < 86_400_000 ? `at ${clock(heardAt)}` : `on ${ago(heardAt)}`}.</>}
        </div>
      )}
      {online !== false && meta.core !== "ok" && meta.message && <div className="banner warn" role="status">{meta.message}</div>}
      {route.name === "inbox" && <Inbox />}
      {route.name === "spaces" && <Spaces />}
      {route.name === "new" && <NewSession />}
      {route.name === "session" && <SessionScreen key={route.id} id={route.id!} />}
      {route.name === "terminal" && <TerminalScreen key={route.id} terminal={route.id!} />}
      {home && (
        <nav className="tabs">
          <button className={route.name === "inbox" ? "on" : ""} onClick={() => go({ name: "inbox" }, true)}>
            <span style={{ position: "relative" }}>
              <InboxIcon size={21} />
              {needs > 0 && <span className="badge">{needs}</span>}
            </span>
            Inbox
          </button>
          <button onClick={() => go({ name: "new" })} aria-label="New session">
            <Plus size={21} />
            New
          </button>
          <button className={route.name === "spaces" ? "on" : ""} onClick={() => go({ name: "spaces" }, true)}>
            <LayoutGrid size={21} />
            Spaces
          </button>
        </nav>
      )}
    </div>
  );
}
