// Tasks: the inbox everything is captured into, the board of every task by
// stage, and the projects with their task lists. Each is a tab inside this one.

import { api, gwen } from "../lib/gwen.js";
import Inbox from "./tasks/inbox.js";
import Board from "./tasks/board.js";
import Projects from "./tasks/projects.js";

export default class Tasks {
  constructor(app) {
    this.app = app;
    this.goals = []; // every goal: the forms offer the active ones, the lists name them
    this.loaded = {}; // the stamp each piece was loaded at
    this.inbox = new Inbox(app, this);
    this.board = new Board(app, this);
    this.projects = new Projects(app, this);
  }

  get subs() {
    return [
      { id: "inbox", label: "Inbox", icon: "inbox", count: () => gwen.tasks.filter((t) => t.stage === "inbox" && !t.parent_id).length },
      { id: "board", label: "Board", icon: "columns-3" },
      { id: "projects", label: "Projects", icon: "folder-kanban" },
    ];
  }

  /** The sub-screen on show. */
  get current() {
    return this[this.app.sub] ?? this.inbox;
  }

  get activeGoals() {
    return this.goals.filter((g) => g.status === "active");
  }

  /**
   * Fetches a piece again when its stamp changed, and keeps the answer only
   * if nothing newer was asked for meanwhile.
   */
  load(cx, key, stamp, fetch, apply) {
    if (this.loaded[key] === stamp) return;
    this.loaded[key] = stamp;
    cx.spawn(async (cx) => {
      try {
        const out = await fetch();
        if (this.loaded[key] === stamp) apply(out);
      } catch (e) {
        gwen.fail(e);
      }
      cx.notify();
    });
  }

  sync(cx) {
    const v = gwen.versions;
    this.load(cx, "goals", `${v.goals}/${v.tasks}`, () => api.ListGoals("all"), (l) => (this.goals = l.goals ?? []));
    this.current.sync?.(cx);
  }

  /** Another screen asked for this one, or a tab inside it was picked: "matrix" opens the board as the matrix. */
  show(sub, param, cx) {
    if (sub === "board" && param === "matrix") this.board.view = "matrix";
    this.sync(cx);
  }

  render(ui, cx) {
    return this.current.render(ui, cx);
  }
}
