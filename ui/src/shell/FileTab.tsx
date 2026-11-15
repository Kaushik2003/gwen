import { LoaderCircle } from "lucide-react";
import { lazy, Suspense } from "react";
import { Empty } from "../components/ui";
import { useDaemon } from "../daemon";
import FileTree from "../work/FileTree";
import { tabId, useTabs, type Tab } from "./tabs";

// A file's tab (plan.md 3.15, board 5). Opened without a file it is the
// project's Files: the tree, and the file you pick from it. The editor
// (CodeMirror) loads the first time a file opens, not with the window.

const FileView = lazy(() => import("../work/FileView"));

export default function FileTab({ tab, active }: { tab: Extract<Tab, { kind: "file" }>; active: boolean }) {
  const d = useDaemon();
  const tabs = useTabs();
  const project = d.projects.find((p) => p.id === tab.project);
  if (!project?.folder)
    return (
      <Empty title="No folder" className="py-16">
        This project has no folder on this laptop. Choose one on its home tab.
      </Empty>
    );
  const open = (path: string) => tabs.open({ kind: "file", project: tab.project, path });
  if (!tab.path)
    return (
      <div className="flex h-full">
        <div className="w-[320px] shrink-0 border-r border-chrome-line bg-chrome">
          <FileTree folder={project.folder} onOpen={open} />
        </div>
        <Empty title={`${project.name}'s files`} className="flex-1 py-16">
          Pick a file to open it. Changed files carry a letter: U new, M changed, D deleted.
        </Empty>
      </div>
    );
  return (
    <Suspense
      fallback={
        <p className="flex items-center gap-2 p-5 text-[13px] text-ink-subtle">
          <LoaderCircle size={14} className="animate-spin" aria-hidden />
          Opening the editor…
        </p>
      }
    >
      <FileView folder={project.folder} path={tab.path} tabKey={tabId(tab)} active={active} onOpenHistory={() => tabs.open({ kind: "history", project: tab.project })} />
    </Suspense>
  );
}
