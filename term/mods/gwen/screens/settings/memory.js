// Settings › Memory: whether and how Gwen learns, where memory is kept,
// optional services, privacy, export, and forgetting everything.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { home } from "herdr";
import { api, apiError, gwen } from "../../lib/gwen.js";
import { host } from "../../lib/host.js";
import { countWords, laneState, receiptText } from "../../lib/memory.js";
import { TextField } from "../../lib/kit.js";
import { hasCredential, loadCredentials } from "./credential.js";
import { chip, stacked } from "./connect-parts.js";

/** Where semantic search finds its embedding model when it is turned on: Ollama on this computer. */
const ollama = "http://localhost:11434";
/** The readable memory folder when it is turned on. */
const defaultFolder = "~/Gwen/memory";
/** What Forget everything asks you to type. */
const forgetPhrase = "forget everything";
/** The settings typed as text, each with its own field. */
const textKeys = ["local_endpoint", "local_model", "embed_model", "mem0_url", "supermemory_url"];

function learnHint(learn, name) {
  if (learn === "local") return "A model on this computer or your Pi reads your chats for facts. Your chats don't leave your devices to be learned from.";
  if (learn === "off") return `${name} only keeps what you tell her to remember, on the Memory screen.`;
  return "The model that answers in the assistant also reads your chats for facts, a little after each one.";
}

export default class MemorySettings {
  constructor(app) {
    this.app = app;
    this.draft = null;
    this.draftFrom = null;
    this.errors = {};
    this.status = null;
    this.loaded = null;
    this.saving = false;
    this.rebuilding = false;
    this.exporting = false;
    this.text = {};
    const placeholders = { local_endpoint: "http://localhost:11434/v1", local_model: "llama3.1", embed_model: "nomic-embed-text", mem0_url: "http://localhost:8888", supermemory_url: "http://127.0.0.1:6767" };
    for (const k of textKeys)
      this.text[k] = new TextField({
        placeholder: placeholders[k],
        onChange: (v, cx) => {
          this.draft[k] = v;
          cx.notify();
        },
      });
    this.keys = { mem0: new TextField({ masked: true, onChange: (v, cx) => cx.notify() }), supermemory: new TextField({ masked: true, onChange: (v, cx) => cx.notify() }) };
  }

  get name() {
    return gwen.config?.llm?.assistant_name || "Gwen";
  }

  sync(cx) {
    const saved = gwen.config?.memory;
    if (!saved) return;
    const from = JSON.stringify(saved);
    if (this.draftFrom !== from) {
      this.draftFrom = from;
      this.revert();
    }
    const stamp = `${gwen.versions.memory}/${from}`;
    if (this.loaded !== stamp) {
      this.loaded = stamp;
      cx.spawn(async (cx) => {
        await loadCredentials();
        this.status = await api.MemoryStatus().catch(() => null);
        cx.notify();
      });
    }
  }

  revert() {
    this.draft = { ...gwen.config.memory };
    for (const k of textKeys) this.text[k].set(this.draft[k] ?? "");
    this.keys.mem0.set("");
    this.keys.supermemory.set("");
    this.errors = {};
  }

  changed() {
    const saved = gwen.config.memory;
    return Object.keys(saved).filter((k) => (typeof this.draft[k] === "string" ? this.draft[k].trim() : this.draft[k]) !== saved[k]);
  }

  dirty() {
    return this.changed().length > 0 || this.keys.mem0.value.trim() !== "" || this.keys.supermemory.value.trim() !== "";
  }

  set(patch, cx) {
    Object.assign(this.draft, patch);
    for (const k of textKeys) if (k in patch) this.text[k].set(patch[k] ?? "");
    cx.notify();
  }

  lane(id) {
    return this.status?.lanes?.find((l) => l.lane === id);
  }

  save(cx) {
    this.saving = true;
    const changed = this.changed();
    this.app.spawn(cx, async () => {
      try {
        if (changed.length) {
          const patch = {};
          for (const k of changed) patch[k] = typeof this.draft[k] === "string" ? this.draft[k].trim() : this.draft[k];
          await api.PatchConfig({ memory: patch });
        }
        for (const [cred, field] of [
          ["mem0_api_key", this.keys.mem0],
          ["supermemory_api_key", this.keys.supermemory],
        ]) {
          if (!field.value.trim()) continue;
          await api.SetCredential(cred, field.value.trim());
          field.set("");
        }
        await loadCredentials();
        this.errors = {};
        gwen.toast("Saved memory settings", "success");
      } catch (e) {
        const err = apiError(e);
        const key = typeof err.details.key === "string" ? err.details.key : "";
        if (key.startsWith("memory.")) this.errors = { [key.slice("memory.".length)]: err.message.replace(`${key}: `, "") };
        else gwen.fail(e);
      } finally {
        this.saving = false;
      }
    });
  }

  rebuild(cx) {
    this.rebuilding = true;
    this.app.spawn(cx, async () => {
      if (await gwen.run(() => api.RebuildMemory({ lane: "vec" }))) gwen.toast(`Rebuilding the search index. ${this.name} works as usual meanwhile.`, "success");
      this.rebuilding = false;
    });
  }

  pickFolder(cx) {
    this.app.spawn(cx, async (cx) => {
      const dir = await host.pickFolder("Use this folder");
      if (dir) this.set({ files_dir: dir }, cx);
    });
  }

  exportMemory(cx) {
    this.exporting = true;
    this.app.spawn(cx, async () => {
      try {
        const path = await host.saveMemoryExport();
        if (path) gwen.toast(`Saved ${this.name}'s memory to ${path}. Only you can read the file.`, "success");
      } finally {
        this.exporting = false;
      }
    });
  }

  /** Erasing all of memory asks you to type the words first. */
  forgetAll(cx) {
    const ui = this.app.ui;
    const name = this.name;
    let busy = false;
    const typed = new TextField({ placeholder: forgetPhrase, onChange: (v, cx) => cx.notify(), onSubmit: (v, cx) => go(cx) });
    const ready = () => typed.value.trim().toLowerCase() === forgetPhrase;
    const go = (cx) => {
      if (!ready() || busy) return;
      busy = true;
      this.app.spawn(cx, async (cx) => {
        const r = await gwen.act(() => api.ForgetAllMemory(forgetPhrase));
        busy = false;
        if (!r) return;
        const counted = countWords(r);
        gwen.toast(receiptText(r, counted ? `Forgot everything: ${counted}.` : "Forgot everything.", name), "success");
        ui.closeDialog(cx, id);
      });
    };
    const id = ui.openDialog(
      {
        title: "Forget everything?",
        width: 440,
        render: (ui) =>
          v_flex()
            .gap(14)
            .child(ui.muted(`${name} erases every fact, note, and document she holds, in every place she keeps them. This can't be undone. Your chats and tasks stay.`, { size: 13 }))
            .child(ui.field(`To confirm, type “${forgetPhrase}”`, typed.el(ui))),
        actions: (ui) => [ui.button("mf-cancel", "Cancel", (e, cx) => ui.closeDialog(cx, id), { tone: "ghost" }), ui.button("mf-forget", "Forget everything", (e, cx) => go(cx), { tone: "danger", busy, disabled: !ready() })],
      },
      cx,
    );
    typed.focus();
  }

  laneLine(ui, lane) {
    if (!lane) return null;
    const s = laneState(lane);
    return h_flex().items_center().gap(8).child(ui.dot(ui.tone(s.color), 8)).child(ui.dim(s.words));
  }

  service(ui, title, key, cred, lane) {
    const has = hasCredential(cred);
    const keyField = this.keys[key];
    return v_flex()
      .gap(6)
      .child(
        h_flex()
          .items_center()
          .gap(8)
          .child(div().w(100).flex_none().text_size(13).font_medium().text_color(ui.t.muted).child(title))
          .child(this.text[`${key}_url`].el(ui, { mono: true }))
          .child(keyField.el(ui))
          .child(div().w(16).flex_none().when(has === true, (e) => e.child(div().id(`ms-${key}-has`).tooltip("API key saved").child(ui.icon("circle-check", 15, ui.tone("ok")))))),
      )
      .child(div().pl(108).child(ui.dim(has ? "A key is saved: type in the second box to replace it." : "The second box is the API key, if it needs one.")))
      .when(Boolean(this.errors[`${key}_url`]), (e) => e.child(div().pl(108).child(ui.text(this.errors[`${key}_url`], { size: 12, color: ui.tone("danger") }))))
      .when(Boolean(lane), (e) => e.child(div().pl(108).child(this.laneLine(ui, lane))));
  }

  render(ui) {
    if (!gwen.config || !this.draft) return ui.splash("Loading…", { busy: true });
    const t = ui.t;
    const name = this.name;
    const saved = gwen.config.memory;
    const d = this.draft;
    const semantic = d.embed_endpoint !== "";
    const folder = d.files_dir !== "";
    const err = (k) => (this.errors[k] ? ui.text(this.errors[k], { size: 12, color: ui.tone("danger") }) : null);
    const dirty = this.dirty();

    const panel = ui
      .panel({ title: "Memory", icon: "book-marked" })
      .child(ui.muted(`${name} learns about you and the people in your life from your chats and from what you ask her to remember, and brings it up when it helps. You can see and fix all of it on the Memory screen.`, { size: 13 }))
      .child(ui.toggleRow("ms-enabled", "Memory", `Off, ${name} learns nothing new and recalls nothing. What she knows is kept until you forget it.`, d.enabled, (on, cx) => this.set({ enabled: on }, cx)));

    const learn = v_flex()
      .gap(10)
      .child(
        h_flex().child(
          ui.segmented(
            "ms-learn",
            [
              { value: "assistant", label: "Using the assistant's model" },
              { value: "local", label: "Using a local model" },
              { value: "off", label: "Off" },
            ],
            d.learn,
            (v, cx) => this.set({ learn: v }, cx),
          ),
        ),
      );
    if (d.learn === "local")
      learn.child(
        h_flex()
          .items_start()
          .gap(12)
          .child(div().flex_1().min_w(0).child(ui.field("Server", this.text.local_endpoint.el(ui, { mono: true }), this.errors.local_endpoint ?? "Ollama on this computer is http://localhost:11434/v1.")))
          .child(div().flex_1().min_w(0).child(ui.field("Model", this.text.local_model.el(ui), this.errors.local_model ?? "As the server names it, such as llama3.1."))),
      );
    panel.child(stacked(ui, "Learn from chats", learnHint(d.learn, name), learn, { error: this.errors.learn }));
    panel.child(ui.toggleRow("ms-verify", "Double-check each fact", "Before keeping a fact, a second look checks it against your own words. A little slower, with fewer mistakes.", d.verify, (on, cx) => this.set({ verify: on }, cx)));

    const sem = v_flex().gap(10);
    if (semantic) sem.child(ui.field("Embedding model", this.text.embed_model.el(ui, { w: 280 }), this.errors.embed_model || this.errors.embed_endpoint || "Install it once with Ollama, such as nomic-embed-text."));
    if (semantic && saved.embed_endpoint)
      sem.child(
        h_flex()
          .items_center()
          .gap(12)
          .child(this.laneLine(ui, this.lane("vec")) ?? div())
          .child(ui.button("ms-rebuild", "Rebuild index", (e, cx) => this.rebuild(cx), { small: true, icon: "refresh-cw", busy: this.rebuilding })),
      );
    panel.child(
      stacked(
        ui,
        "Semantic search",
        `${name} finds memories by meaning as well as by words, with an embedding model running in Ollama on this computer.`,
        v_flex()
          .gap(10)
          .child(h_flex().child(ui.toggle("ms-semantic", semantic, (on, cx) => this.set({ embed_endpoint: on ? saved.embed_endpoint || ollama : "" }, cx))))
          .child(sem),
      ),
    );

    const files = v_flex().gap(10).child(h_flex().child(ui.toggle("ms-folder", folder, (on, cx) => this.set({ files_dir: on ? saved.files_dir || defaultFolder : "" }, cx))));
    if (folder)
      files.child(
        h_flex()
          .flex_wrap()
          .items_center()
          .gap(8)
          .child(chip(ui, d.files_dir))
          .child(ui.button("ms-folder-change", "Change", (e, cx) => this.pickFolder(cx), { small: true, icon: "folder-search" }))
          .when(Boolean(saved.files_dir) && saved.files_dir === d.files_dir, (e) =>
            e.child(
              ui.button(
                "ms-folder-open",
                "Open folder",
                (ev, cx) => this.app.spawn(cx, () => host.openPath(saved.files_dir.replace(/^~(?=\/|$)/, home()))),
                { small: true, icon: "folder-open" },
              ),
            ),
          ),
      );
    const fe = err("files_dir");
    if (fe) files.child(fe);
    if (folder && saved.files_dir) files.child(this.laneLine(ui, this.lane("files")) ?? div());
    panel.child(stacked(ui, "Readable memory folder", `${name} also writes what she knows as plain text files you can read. Edit them, and she learns from your changes.`, files));

    panel.child(
      stacked(
        ui,
        "Optional memory services",
        "Mem0 and Supermemory can hold a copy for recall. Leave the address empty to not use one.",
        v_flex()
          .gap(12)
          .child(this.service(ui, "Mem0", "mem0", "mem0_api_key", saved.mem0_url ? this.lane("mem0") : null))
          .child(this.service(ui, "Supermemory", "supermemory", "supermemory_api_key", saved.supermemory_url ? this.lane("supermemory") : null)),
      ),
    );
    panel.child(
      stacked(
        ui,
        "Privacy",
        d.privacy === "local"
          ? "Mem0 and Supermemory must run on this computer. Nothing leaves it."
          : "Mem0 and Supermemory may be hosted services. Facts about your health, contacts, and other sensitive things still never leave this computer.",
        h_flex().child(
          ui.segmented(
            "ms-privacy",
            [
              { value: "local", label: "Keep everything on my devices" },
              { value: "cloud_ok", label: "Allow hosted connectors" },
            ],
            d.privacy,
            (v, cx) => this.set({ privacy: v }, cx),
          ),
        ),
        { error: this.errors.privacy, divider: false },
      ),
    );
    panel.child(
      h_flex()
        .justify_end()
        .gap(8)
        .pt(12)
        .border_t(1)
        .border_color(t.border)
        .when(dirty, (e) =>
          e.child(
            ui.button(
              "ms-revert",
              "Revert",
              (ev, cx) => {
                this.revert();
                cx.notify();
              },
              { tone: "ghost" },
            ),
          ),
        )
        .child(ui.button("ms-save", "Save changes", (e, cx) => this.save(cx), { tone: "primary", disabled: !dirty, busy: this.saving })),
    );

    const data = ui
      .panel({ title: "Your memory data", icon: "shield-alert" })
      .child(ui.row("Export memory", `Save everything ${name} remembers to a file on this computer, readable only by you.`, ui.button("ms-export", "Export memory", (e, cx) => this.exportMemory(cx), { icon: "download", busy: this.exporting })))
      .child(ui.row("Forget everything", `Erase everything ${name} has learned, everywhere she keeps it. Your chats and tasks stay.`, ui.button("ms-forget", "Forget everything", (e, cx) => this.forgetAll(cx), { tone: "danger", icon: "trash-2" }), { divider: false }));
    return [panel, data];
  }
}
