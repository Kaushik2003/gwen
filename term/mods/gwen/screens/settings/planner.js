// Settings, Planner: the hours Gwen plans tasks into.

import { ConfigForm } from "./form.js";

const fields = [
  { key: "day_start", label: "Plans start at", kind: { type: "time" } },
  { key: "day_end", label: "Plans end at", kind: { type: "time" } },
  { key: "buffer", label: "Keep free each day", hint: "Plans leave this much of the day unplanned.", kind: { type: "duration" } },
];

export default class Planner extends ConfigForm {
  constructor(app) {
    super(app, { title: "Planner", description: "The hours Gwen plans tasks into.", name: "planner", fields });
  }
}
