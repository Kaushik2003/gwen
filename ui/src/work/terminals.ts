import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { apiError, isHub } from "../api";
import * as Host from "../wailsjs/go/main/App";
import { main } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";

// The terminals of Gwen's herdr-core, session `gwen` (plan.md 3.15): the Gwen window draws them itself, as a client of the core.
// gwen-ui sends `term:changed` whenever terminals come, go, or change status, title, labels, or program, and when the core comes
// or goes; the list is fetched again then and whenever the window gets the focus.

export type CoreState = "ok" | "down" | "loading";

export interface Terminals {
  /** Every live terminal on this laptop, by address. */
  list: main.TermInfo[];
  core: CoreState;
  /** Why there are no terminals when the core is down, as a sentence. */
  message: string;
  refresh(): void;
}

/** The label that links a terminal to a task. */
export const TASK_LABEL = "gwen.task";

export function useTerminals(): Terminals {
  const [state, setState] = useState<{ list: main.TermInfo[]; core: CoreState; message: string }>({ list: [], core: "loading", message: "" });
  const latest = useRef(0);

  const refresh = useCallback(() => {
    if (isHub) {
      setState({ list: [], core: "down", message: "Terminals are on the laptop." });
      return;
    }
    // Answers can come back out of order; only the newest counts.
    const n = ++latest.current;
    Host.TermList().then(
      (l) => {
        if (n === latest.current) setState({ list: l.terminals ?? [], core: l.core === "ok" ? "ok" : "down", message: l.message ?? "" });
      },
      (e) => {
        if (n === latest.current) setState((s) => ({ ...s, core: "down", message: apiError(e).message }));
      },
    );
  }, []);

  useEffect(() => {
    refresh();
    if (isHub) return;
    const off = EventsOn("term:changed", refresh);
    window.addEventListener("focus", refresh);
    return () => {
      off();
      window.removeEventListener("focus", refresh);
    };
  }, [refresh]);

  return useMemo(() => ({ ...state, refresh }), [state, refresh]);
}

/** Starts the login shell in a folder, in a project; it starts Gwen's terminal server first when none runs. */
export function startShell(cwd: string, project: string, name?: string): Promise<main.TermInfo> {
  return Host.TermStart(main.TermStartRequest.createFrom({ cwd, project, name: name ?? "", argv: [] }));
}

/** Ends a shell terminal and its program. A terminal with a Claude session is refused: end the session instead. */
export function endTerminal(address: string): Promise<void> {
  return Host.TermEnd(address);
}

/** Links a terminal to a task, or unlinks it with "". */
export function linkTask(address: string, taskId: string | ""): Promise<void> {
  return Host.TermLabel(address, { [TASK_LABEL]: taskId });
}
