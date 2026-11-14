import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { App, onEvent, wire } from "../api";
import { useDaemon } from "../daemon";
import { useTerminals } from "../work/terminals";
import type { main } from "../wailsjs/go/models";

// What the workbench shares (plan.md 3.15): code sessions from gwend, the
// terminals of Gwen's herdr-core, which project each belongs to, the phone's
// state, and the one Needs you list.

/** One thing that waits for you, wherever it came from. */
export type Need =
  | { kind: "session"; id: string; session: wire.CodeSession; project: string | null }
  | { kind: "done"; id: string; session: wire.CodeSession; project: string | null }
  | { kind: "nudge"; id: string; nudge: wire.Nudge }
  /** An approval Gwen waits on: in one of her chats (thread_id) or in a run (plan.md 3.7, 3.16). */
  | { kind: "approval"; id: string; approval: wire.Approval };

/** A terminal in a project: a Claude session's or a shell. */
export interface Term {
  address: string;
  name: string;
  cwd: string;
  status: string;
  claude: boolean;
  session: wire.CodeSession | null;
  project: string | null;
  task: string;
}

interface Work {
  sessions: wire.CodeSession[];
  terminals: Term[];
  core: "ok" | "down" | "loading";
  coreMessage: string;
  remote: wire.RemoteStatus | null;
  nudges: wire.Nudge[];
  needs: Need[];
  /** The project whose folder holds this path, the deepest one. */
  projectOf: (path: string) => string | null;
  /** The live terminals of a project, sessions first. */
  termsOf: (project: string) => Term[];
  /** Recent ended sessions of a project, for Resume. */
  endedOf: (project: string) => wire.CodeSession[];
  sessionById: (id: string) => wire.CodeSession | undefined;
  refreshTerminals: () => void;
  refreshSessions: () => void;
}

const WorkContext = createContext<Work | null>(null);

export function useWork(): Work {
  const w = useContext(WorkContext);
  if (!w) throw new Error("useWork outside WorkProvider");
  return w;
}

function within(path: string, folder: string): boolean {
  if (!folder || !path) return false;
  const f = folder.endsWith("/") ? folder.slice(0, -1) : folder;
  return path === f || path.startsWith(f + "/");
}

export function WorkProvider({ children }: { children: ReactNode }) {
  const d = useDaemon();
  const terms = useTerminals();
  const [sessions, setSessions] = useState<wire.CodeSession[]>([]);
  const [remote, setRemote] = useState<wire.RemoteStatus | null>(null);
  const [nudges, setNudges] = useState<wire.Nudge[]>([]);
  const [approvals, setApprovals] = useState<wire.Approval[]>([]);

  const refreshSessions = useCallback(() => {
    App.CodeSessions().then(
      (l) => setSessions(l.sessions ?? []),
      () => {},
    );
  }, []);

  useEffect(() => {
    refreshSessions();
    const offs = [
      onEvent("connected", refreshSessions),
      onEvent("code_session", (s: wire.CodeSession) => {
        if (!s?.id) return;
        setSessions((list) => {
          const at = list.findIndex((x) => x.id === s.id);
          return at < 0 ? [s, ...list] : list.map((x, i) => (i === at ? s : x));
        });
      }),
      onEvent("code_session_removed", (r: { id: string }) => setSessions((list) => list.filter((x) => x.id !== r?.id))),
    ];
    return () => offs.forEach((off) => off());
  }, [refreshSessions]);

  useEffect(() => {
    const load = () =>
      App.RemoteStatus().then(
        (r) => setRemote(r),
        () => setRemote(null),
      );
    load();
    const offs = [onEvent("remote_changed", load), onEvent("connected", load)];
    return () => offs.forEach((off) => off());
  }, []);

  useEffect(() => {
    const load = () =>
      // Not as the panel card: the window also lists the nudges that came while you were in it.
      App.Nudges(false).then(
        (l) => setNudges((l.nudges ?? []).filter((n) => !n.preview)),
        () => setNudges([]),
      );
    load();
    const offs = [onEvent("nudge_fired", load), onEvent("state_changed", load), onEvent("connected", load)];
    const timer = window.setInterval(load, 60_000);
    return () => {
      offs.forEach((off) => off());
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    const load = () =>
      App.ListApprovals("pending").then(
        (l) => setApprovals(l.approvals ?? []),
        () => setApprovals([]),
      );
    load();
    const offs = [onEvent("approval_requested", load), onEvent("approval_decided", load), onEvent("connected", load)];
    return () => offs.forEach((off) => off());
  }, []);

  const value = useMemo<Work>(() => {
    const folders = d.projects.filter((p) => p.folder).sort((a, b) => b.folder.length - a.folder.length);
    const projectOf = (path: string) => folders.find((p) => within(path, p.folder))?.id ?? null;
    const byTerminal = new Map(sessions.filter((s) => s.terminal && s.state !== "ended").map((s) => [s.terminal, s]));

    const terminals: Term[] = terms.list
      .filter((t: main.TermInfo) => t.live)
      .map((t: main.TermInfo) => {
        const session = byTerminal.get(t.address) ?? (t.session_id ? (sessions.find((s) => s.id === t.session_id) ?? null) : null);
        const project = t.project && d.projects.some((p) => p.id === t.project) ? t.project : projectOf(session?.cwd || t.cwd);
        const claude = t.claude || !!session;
        // A session is named after its folder, which its project already says: its task, or "Claude", says more.
        const task = d.tasks.find((x) => x.id === t.task);
        const raw = session?.name || t.name;
        const folderName = (session?.cwd || t.cwd).split("/").pop();
        const name = task?.title ?? (claude && (raw === folderName || raw === d.projects.find((p) => p.id === project)?.name) ? "Claude" : raw);
        return { address: t.address, name, cwd: t.cwd, status: t.status, claude, session, project, task: t.task };
      });

    const termsOf = (project: string) =>
      terminals.filter((t) => t.project === project).sort((a, b) => Number(b.claude) - Number(a.claude) || a.address.localeCompare(b.address, undefined, { numeric: true }));
    const endedOf = (project: string) => sessions.filter((s) => s.state === "ended" && projectOf(s.cwd) === project);

    const needs: Need[] = [
      ...approvals.map((a): Need => ({ kind: "approval", id: `approval:${a.id}`, approval: a })),
      ...sessions.filter((s) => s.state === "needs_you").map((s): Need => ({ kind: "session", id: `session:${s.id}`, session: s, project: projectOf(s.cwd) })),
      ...sessions.filter((s) => s.state === "done" && s.unread).map((s): Need => ({ kind: "done", id: `done:${s.id}`, session: s, project: projectOf(s.cwd) })),
      ...nudges.map((n): Need => ({ kind: "nudge", id: `nudge:${n.kind}:${n.at}`, nudge: n })),
    ];

    return {
      sessions,
      terminals,
      core: terms.core,
      coreMessage: terms.message,
      remote,
      nudges,
      needs,
      projectOf,
      termsOf,
      endedOf,
      sessionById: (id: string) => sessions.find((s) => s.id === id),
      refreshTerminals: terms.refresh,
      refreshSessions,
    };
  }, [d.projects, d.tasks, sessions, terms.list, terms.core, terms.message, terms.refresh, remote, nudges, approvals, refreshSessions]);

  return <WorkContext.Provider value={value}>{children}</WorkContext.Provider>;
}
