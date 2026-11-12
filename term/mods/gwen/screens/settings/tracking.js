// Settings, Tracking: how Gwen counts your day.

import { ConfigForm } from "./form.js";

const fields = [
  { key: "daily_target", label: "Daily target", hint: "Worked time that makes a full day.", kind: { type: "duration" } },
  { key: "soft_idle", label: "Ask if you're away after", hint: "No keyboard or mouse for this long sends a nudge. The time still counts as work.", kind: { type: "duration", units: ["m", "s"] } },
  { key: "hard_idle", label: "Count it as a break after", hint: "Away this long, and the time since you left becomes a break.", kind: { type: "duration", units: ["m", "s"] } },
  { key: "day_rollover", label: "A new day starts at", hint: "Work after midnight counts toward the day before until this time.", kind: { type: "time" } },
];

export default class Tracking extends ConfigForm {
  constructor(app) {
    super(app, { title: "Tracking", description: "How Gwen counts your day.", name: "tracking", fields });
  }
}
