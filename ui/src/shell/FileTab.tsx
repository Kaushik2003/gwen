import { Empty } from "../components/ui";
import { useDaemon } from "../daemon";
import FileTree from "../work/FileTree";
import FileView from "../work/FileView";
import { useTabs, type Tab } from "./tabs";

// A file's tab (plan.md 3.15, board 5). Opened without a file it is the
// project's Files: the tree, and the file you pick from it.

export default function FileTab({ tab }: { tab: Extract<Tab, { kind: "file" }> }) {
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
          Pick a file to read it. Changed files carry a letter: U new, M changed, D deleted.
        </Empty>
      </div>
    );
  return <FileView folder={project.folder} path={tab.path} onOpenHistory={() => tabs.open({ kind: "history", project: tab.project })} />;
}
