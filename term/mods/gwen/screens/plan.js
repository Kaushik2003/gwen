// Plan: the day plan with its AI planning chat, and the goals with their
// commitments. Each is a tab inside this one.

import DayPlan from "./plan/day.js";
import Goals from "./plan/goals.js";

export default class Plan {
  constructor(app, cx) {
    this.app = app;
    this.day = new DayPlan(app, cx);
    this.goals = new Goals(app, cx);
  }

  get subs() {
    return [
      { id: "plan", label: "Day plan", icon: "calendar-clock" },
      { id: "goals", label: "Goals", icon: "target" },
    ];
  }

  get current() {
    return this.app.sub === "goals" ? this.goals : this.day;
  }

  get tick() {
    return Boolean(this.current.tick);
  }

  sync(cx) {
    this.current.sync(cx);
  }

  /** "chat" opens the day plan on today with the AI chat open. */
  show(sub, param, cx) {
    this.sync(cx);
    if (sub === "plan") this.day.show(param, cx);
  }

  render(ui, cx) {
    return this.current.render(ui, cx);
  }
}
