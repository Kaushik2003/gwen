import { Plus, SquareTerminal } from "lucide-react";
import { useState } from "react";
import { ApiError, api, type Terminal } from "./api";
import { TerminalRow } from "./Inbox";
import { useStore } from "./store";
import { tilde } from "./ui";

/** The same spaces and tabs as Gwen Terminal on the laptop (plan.md 3.14.13). */
export function Spaces() {
  const { spaces, sessions, go, toast, refresh } = useStore();
  const [busy, setBusy] = useState("");
  const open = (t: Terminal) => {
    const s = sessions.find((x) => x.id === t.session_id);
    go(s ? { name: "session", id: s.id } : { name: "terminal", id: t.terminal });
  };
  const add = async (space: string) => {
    setBusy(space);
    try {
      const { terminal } = await api.newTerminal(space);
      refresh();
      if (terminal) go({ name: "terminal", id: terminal });
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Couldn't open a terminal there.");
    } finally {
      setBusy("");
    }
  };

  const none = !spaces || (spaces.spaces.length === 0 && spaces.loose.length === 0);
  return (
    <>
      <header className="bar">
        <div className="grow">
          <h1>Spaces</h1>
          <div className="sub">As on the laptop</div>
        </div>
      </header>
      <main className="page">
        {none && (
          <div className="empty">
            <SquareTerminal size={28} />
            <h2>No spaces open</h2>
            <p>Open Gwen on the laptop, or start a session from here and it opens for you.</p>
            <button className="btn primary" onClick={() => go({ name: "new" })}>New session</button>
          </div>
        )}
        {spaces?.spaces.map((sp) => (
          <section key={sp.id}>
            <div className="section">
              <span className="grow clip" style={{ textTransform: "none", letterSpacing: 0, fontSize: 14, color: "var(--fg)" }}>{sp.name || tilde(sp.folder)}</span>
              <button className="btn sm" disabled={busy === sp.id} onClick={() => add(sp.id)} aria-label={`New terminal in ${sp.name}`}>
                {busy === sp.id ? <span className="spin" style={{ width: 14, height: 14 }} /> : <Plus size={15} />} Terminal
              </button>
            </div>
            {sp.folder && <div className="small muted mono clip" style={{ padding: "0 14px 6px" }}>{tilde(sp.folder)}</div>}
            {sp.tabs.map((tab) =>
              tab.terminals.map((t, i) => (
                <div key={t.terminal} style={i > 0 ? { marginTop: -1 } : undefined}>
                  <TerminalRow t={tab.terminals.length > 1 || tab.name !== t.name ? { ...t, name: `${tab.name} · ${t.name}` } : t} onOpen={() => open(t)} roots={[]} />
                </div>
              )),
            )}
            {sp.tabs.every((tab) => tab.terminals.length === 0) && <div className="small muted" style={{ padding: "6px 14px 12px" }}>No terminals in this space.</div>}
          </section>
        ))}
        {spaces && spaces.loose.length > 0 && (
          <section>
            <div className="section">Without a window</div>
            {spaces.loose.map((t) => <TerminalRow key={t.terminal} t={t} onOpen={() => open(t)} roots={[]} />)}
          </section>
        )}
      </main>
    </>
  );
}
