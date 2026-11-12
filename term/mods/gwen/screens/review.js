// Review: the weekly review, stats over any range, and the history of days
// with each day's detail. Each is a tab inside this one.

import Weekly from "./review/weekly.js";
import Stats from "./review/stats.js";
import History from "./review/history.js";

export default class Review {
  constructor(app, cx) {
    this.app = app;
    this.weekly = new Weekly(app, cx);
    this.stats = new Stats(app, cx);
    this.history = new History(app, cx);
  }

  get subs() {
    return [
      { id: "weekly", label: "Weekly", icon: "clipboard-check" },
      { id: "stats", label: "Stats", icon: "chart-column" },
      { id: "history", label: "History", icon: "history" },
    ];
  }

  get current() {
    return this[this.app.sub] ?? this.weekly;
  }

  get tick() {
    return Boolean(this.current.tick);
  }

  sync(cx) {
    this.current.sync(cx);
  }

  show(_sub, _param, cx) {
    this.sync(cx);
  }

  render(ui, cx) {
    return this.current.render(ui, cx);
  }
}
