import { TriangleAlert, X } from "lucide-react";
import { DayProvider } from "./day";
import { Gwen } from "./Gwen";
import { NewSession } from "./NewSession";
import { Pair } from "./Pair";
import { SessionScreen } from "./SessionScreen";
import { Provider, tabScreens, useStore } from "./store";
import { TabBar } from "./TabBar";
import { TerminalScreen } from "./TerminalScreen";
import { Today } from "./Today";
import { ago, clock } from "./ui";
import { Work } from "./Work";

export function App() {
  return (
    <Provider>
      <DayProvider>
        <Shell />
      </DayProvider>
    </Provider>
  );
}

/** Every screen (the phone redo): Today, Work and Gwen with the tab bar; a session, a terminal and New open over them. */
function Shell() {
  const { route, go, online, meta, heardAt, alert, dismissAlert, toastText } = useStore();
  if (route.name === "pair") return <Pair />;
  const tabbed = (tabScreens as readonly string[]).includes(route.name);
  return (
    <div className={tabbed ? "app tabbed" : "app"}>
      {online === false ? (
        <div className="banner" role="status">
          <TriangleAlert size={16} aria-hidden />
          <span>
            Can't reach <b>{meta.machine || "the laptop"}</b>. It may be asleep, or Tailscale is off on this phone.
            {heardAt > 0 && <> Showing what it said {Date.now() - heardAt < 86_400_000 ? `at ${clock(heardAt)}` : `on ${ago(heardAt)}`}.</>}
          </span>
        </div>
      ) : (
        meta.core !== "ok" && meta.message && (
          <div className="banner" role="status">
            <TriangleAlert size={16} aria-hidden />
            <span>{meta.message}</span>
          </div>
        )
      )}
      {route.name === "today" && <Today />}
      {route.name === "work" && <Work />}
      {route.name === "gwen" && <Gwen key={route.id ?? ""} chat={route.id} />}
      {route.name === "new" && <NewSession />}
      {route.name === "session" && <SessionScreen key={route.id} id={route.id!} />}
      {route.name === "terminal" && <TerminalScreen key={route.id} terminal={route.id!} />}
      {/* Above whatever sits at the bottom of the screen, never over the header's buttons. */}
      <div className="float">
        {toastText && <div className="toast" role="status">{toastText}</div>}
        {alert && (
          <div className={`alert ${alert.urgent ? "urgent" : ""}`} role="status">
            <button className="grow row" style={{ minHeight: 44, textAlign: "left" }} onClick={() => (dismissAlert(), go({ name: "session", id: alert.id }))}>
              <span className={`led ${alert.urgent ? "warn" : "on"}`} aria-hidden />
              <span className="grow clip">
                <b>{alert.name}</b> {alert.text}
              </span>
              <span className="open">Open</span>
            </button>
            <button className="ghost soft" aria-label="Dismiss" onClick={dismissAlert}>
              <X size={18} />
            </button>
          </div>
        )}
      </div>
      {tabbed && <TabBar />}
    </div>
  );
}
