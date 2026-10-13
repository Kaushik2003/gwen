import type { LucideIcon } from "lucide-react";
import type { ComponentType } from "react";
import { useNav } from "../nav";
import { Segmented } from "./ui";

export interface GroupTab {
  id: string;
  label: string;
  icon?: LucideIcon;
  view: ComponentType;
}

/** Groups several of the app's existing screens under one nav slot (plan.md §5.3: Tasks, Plan,
 * Review). The tab shown is `active` (the real nav.screen, a leaf id such as "board" or "goals"),
 * not separate state, so a deep link such as go("board", "matrix") still lands on the right tab
 * with param untouched: this component never reads or writes param, only which view mounts. */
export function TabGroup({ tabs, active }: { tabs: GroupTab[]; active: string }) {
  const { go } = useNav();
  const current = tabs.find((t) => t.id === active) ?? tabs[0];
  const View = current.view;
  return (
    <div className="flex flex-col gap-5">
      <Segmented
        label="Section"
        value={current.id}
        onChange={(id) => go(id)}
        options={tabs.map((t) => ({ value: t.id, label: t.label, icon: t.icon }))}
      />
      <View key={current.id} />
    </div>
  );
}
