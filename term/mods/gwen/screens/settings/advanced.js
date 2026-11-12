// Settings, Advanced: how much the background service writes to its log.

import { ConfigForm } from "./form.js";

const fields = [
  {
    key: "level",
    label: "Log detail",
    hint: "How much the background service writes to its log, which journalctl --user -u gwend shows.",
    kind: {
      type: "select",
      options: [
        { value: "debug", label: "Everything (debug)" },
        { value: "info", label: "Normal (info)" },
        { value: "warn", label: "Warnings" },
        { value: "error", label: "Errors only" },
      ],
    },
  },
];

export default class Advanced extends ConfigForm {
  constructor(app) {
    super(app, { title: "Advanced", name: "log", fields });
  }
}
