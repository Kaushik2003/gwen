import type { wire } from "../api";
import { Empty } from "../components/ui";
import { useDaemon } from "../daemon";
import History from "../work/History";
import { startClaude } from "./ProjectHome";
import { useTabs } from "./tabs";
import { useWork } from "./work";

// A project's history (plan.md 3.15, board 6): commits, then a commit's
// files, then its code.

export default function HistoryTab({ project: id }: { project: string }) {
  const d = useDaemon();
  const tabs = useTabs();
  const w = useWork();
  const project = d.projects.find((p) => p.id === id);
  if (!project?.folder)
    return (
      <Empty title="No folder" className="py-16">
        This project has no folder on this laptop. Choose one on its home tab.
      </Empty>
    );
  const openTerm = (proj: string, terminal: string) => tabs.open({ kind: "term", project: proj, terminal });
  // The session that made a commit: its tab when it still runs, else the conversation resumed.
  const openSession = async (link: wire.CommitSession) => {
    const live = link.session_id ? w.sessionById(link.session_id) : undefined;
    if (live?.terminal) return openTerm(id, live.terminal);
    await startClaude(project.folder, id, { resume: link.claude_session_id, sandbox: link.sandbox }, openTerm);
  };
  return <History folder={project.folder} onOpenFile={(path) => tabs.open({ kind: "file", project: id, path })} onOpenSession={openSession} />;
}
