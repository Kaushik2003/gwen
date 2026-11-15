import { useSyncExternalStore } from "react";

// Tabs with unsaved edits (file tabs) and how to save each: the tab bar's ●, and the question
// before such a tab closes. Each file tab registers itself while it has edits.

const saves = new Map<string, () => Promise<boolean>>();
let snapshot: ReadonlySet<string> = new Set();
const listeners = new Set<() => void>();

/** Marks a tab as having unsaved edits (save resolves true once they are written), or clears it with null. */
export function setUnsaved(id: string, save: (() => Promise<boolean>) | null) {
  const had = saves.has(id);
  if (save) saves.set(id, save);
  else saves.delete(id);
  if (had !== !!save) {
    snapshot = new Set(saves.keys());
    for (const l of listeners) l();
  }
}

/** How to save a tab's edits, or undefined when it has none. */
export const unsavedSave = (id: string) => saves.get(id);

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** The ids of the tabs with unsaved edits. */
export function useUnsaved(): ReadonlySet<string> {
  return useSyncExternalStore(subscribe, () => snapshot);
}
