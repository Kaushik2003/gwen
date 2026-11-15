import { useCallback } from "react";
import { askGwen } from "../agent/agent";
import { useDaemon } from "../daemon";
import { usePanel } from "../shell/SidePanel";

/** Asks Gwen something in a new chat and opens the side panel on it. */
export function useAskGwen(): (text: string) => void {
  const d = useDaemon();
  const panel = usePanel();
  return useCallback(
    (text: string) => {
      panel.setTab(false, "gwen");
      if (!panel.open) panel.toggle();
      askGwen(text).catch(d.fail);
    },
    [panel, d.fail],
  );
}
