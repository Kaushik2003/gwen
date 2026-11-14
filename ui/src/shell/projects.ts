import type { wire } from "../api";

// Projects nest one level (a sub-project sits under one project): helpers that read the tree.

/** The project a sub-project sits under, if any. */
export function parentOf(p: wire.Project): string | undefined {
  return p.parent_id || undefined;
}

export interface ProjectNode {
  project: wire.Project;
  children: wire.Project[];
}

/** Top-level projects in the given order, each with its sub-projects; a sub-project whose parent is gone stands on its own. */
export function projectTree(projects: wire.Project[]): ProjectNode[] {
  const ids = new Set(projects.map((p) => p.id));
  const top = projects.filter((p) => !parentOf(p) || !ids.has(parentOf(p)!));
  return top.map((project) => ({ project, children: projects.filter((c) => c !== project && parentOf(c) === project.id) }));
}

/** Every live project as a picker choice, sub-projects under their parent as "Parent › Child". */
export function projectChoices(projects: wire.Project[]): { id: string; label: string }[] {
  const live = projects.filter((p) => !p.archived_at);
  return projectTree(live).flatMap((n) => [{ id: n.project.id, label: n.project.name }, ...n.children.map((c) => ({ id: c.id, label: `${n.project.name} › ${c.name}` }))]);
}

/** A project's name with its parent's in front, for a sub-project. */
export function projectPath(p: wire.Project | undefined, projects: wire.Project[]): string {
  if (!p) return "";
  const parent = projects.find((x) => x.id === parentOf(p));
  return parent ? `${parent.name} › ${p.name}` : p.name;
}
