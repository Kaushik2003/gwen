// Settings › AI: which provider answers, its model, and a live sign-in check
// for Claude Code.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { api, gwen } from "../../lib/gwen.js";
import { host } from "../../lib/host.js";
import { parseDuration } from "../../lib/format.js";
import { planName } from "../../lib/ai.js";
import { TextField } from "../../lib/kit.js";
import { cardGrid, choiceCard } from "./form.js";
import { hasCredential, loadCredentials } from "./credential.js";

const providers = [
  { id: "claude_code", title: "Claude Code", body: "Your Claude Pro or Max plan, through the Claude Code app. No API key.", icon: "sparkles" },
  { id: "anthropic", title: "Anthropic API", body: "Pay per request, with an API key from the Anthropic Console.", icon: "key-round" },
  { id: "openai_compatible", title: "OpenAI-compatible", body: "Ollama, LM Studio, or any server with a /v1 API, on this computer or the Pi.", icon: "arrow-left-right" },
  { id: "none", title: "Off", body: "No AI features.", icon: "circle-off" },
];

export default class AI {
  constructor(app) {
    this.app = app;
    this.loadedFrom = null; // the configuration the draft was taken from
    this.provider = "none";
    this.timeout = "2m";
    this.model = new TextField({ placeholder: "sonnet", onChange: (v, cx) => cx.notify() });
    this.endpoint = new TextField({ placeholder: "http://localhost:11434/v1", onChange: (v, cx) => cx.notify() });
    this.command = new TextField({ placeholder: "claude", onChange: (v, cx) => cx.notify(), onSubmit: (v, cx) => this.check(cx) });
    this.key = new TextField({ masked: true, onChange: (v, cx) => cx.notify() });
    this.claude = null;
    this.checking = false;
    this.saving = false;
  }

  /** Takes the saved settings into the draft when they changed under it. */
  sync(cx) {
    const llm = gwen.config?.llm;
    if (!llm) return;
    const stamp = JSON.stringify([llm.provider, llm.model, llm.endpoint, llm.command, llm.timeout]);
    if (this.loadedFrom === stamp) return;
    const first = this.loadedFrom === null;
    this.loadedFrom = stamp;
    this.provider = llm.provider ?? "none";
    this.model.set(llm.model ?? "");
    this.endpoint.set(llm.endpoint ?? "");
    this.command.set(llm.command ?? "claude");
    this.timeout = llm.timeout ?? "2m";
    if (first) cx.spawn(async (cx) => {
      await loadCredentials();
      cx.notify();
    });
    if (this.provider === "claude_code") this.check(cx, llm.command ?? "claude");
  }

  /** Asks the program whether it is a signed-in Claude Code. */
  check(cx, command) {
    const cmd = (command ?? this.command.value).trim() || "claude";
    this.checking = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        this.claude = await host.claudeCode(cmd);
      } catch (e) {
        gwen.fail(e);
      }
      this.checking = false;
      cx.notify();
    });
  }

  pick(id, cx) {
    this.provider = id;
    if (id === "claude_code" && !this.claude) this.check(cx);
    cx.notify();
  }

  /** The command to store: as gwen setup llm does, the full path, because the daemon's PATH may not be this window's. */
  commandToSave() {
    return this.claude?.found && this.claude.is_claude ? this.claude.command : this.command.value.trim();
  }

  dirty() {
    const llm = gwen.config?.llm ?? {};
    return (
      this.provider !== llm.provider ||
      this.model.value.trim() !== (llm.model ?? "") ||
      this.timeout !== llm.timeout ||
      (this.provider === "openai_compatible" && this.endpoint.value.trim() !== (llm.endpoint ?? "")) ||
      (this.provider === "claude_code" && this.commandToSave() !== llm.command) ||
      this.key.value.trim() !== ""
    );
  }

  save(cx) {
    if (this.saving) return;
    this.saving = true;
    const provider = this.provider;
    const patch = { provider, model: this.model.value.trim(), timeout: this.timeout };
    if (provider === "openai_compatible") patch.endpoint = this.endpoint.value.trim();
    if (provider === "claude_code") patch.command = this.commandToSave();
    const key = this.key.value.trim();
    this.app.spawn(cx, async () => {
      const ok = await gwen.act(() => api.PatchConfig({ llm: patch }));
      if (ok && key && provider !== "claude_code" && provider !== "none") {
        try {
          await api.SetCredential("llm_api_key", key);
          this.key.set("");
          await loadCredentials();
        } catch (e) {
          gwen.fail(e);
          this.saving = false;
          return;
        }
      }
      this.saving = false;
      if (ok) gwen.toast(provider === "none" ? "Turned AI off" : "Saved AI settings", "success");
    });
  }

  claudeState(ui) {
    const st = this.claude;
    if (this.checking || !st) return h_flex().items_center().gap(8).child(ui.spinner(14)).child(ui.muted("Asking Claude Code whether it is signed in…", { size: 13 }));
    if (st.is_claude && st.logged_in)
      return ui.callout({ tone: "ok", icon: "circle-check", text: `${st.plan ? `Signed in with your Claude ${planName(st.plan)} plan.` : `Signed in (${st.auth_method || "Claude Code"}).`} ${st.command}` });
    if (st.is_claude)
      return ui.callout({
        tone: "warn",
        icon: "circle-alert",
        text: "Claude Code is installed but not signed in. Run claude auth login in a terminal once, then check again.",
        action: ui.button(
          "ai-copy-login",
          "Copy command",
          (e, cx) => {
            cx.write_to_clipboard("claude auth login");
            gwen.toast("Copied. Paste it into a terminal, sign in, then check again.", "success");
          },
          { small: true, icon: "copy" },
        ),
      });
    return ui.callout({
      tone: "danger",
      icon: "circle-alert",
      text: `${st.found ? `${st.command} is not Claude Code.` : `Claude Code was not found as ${st.command}.`} ${st.suggested ? `Found Claude Code at ${st.suggested}.` : "Install Claude Code, or give the full path to claude, then check again."}`,
      action: st.suggested
        ? ui.button(
            "ai-use-suggested",
            "Use it",
            (e, cx) => {
              this.command.set(st.suggested);
              this.check(cx, st.suggested);
            },
            { small: true, tone: "primary" },
          )
        : null,
    });
  }

  render(ui) {
    const t = ui.t;
    const llm = gwen.config?.llm;
    const p = this.provider;
    const panel = ui
      .panel({ title: "AI", icon: "sparkles" })
      .child(ui.muted("Optional. The assistant chats with you and adds, moves, and reschedules your tasks as you ask, plans your day, breaks goals into tasks, and writes a weekly retro from your numbers.", { size: 13 }))
      .child(
        cardGrid(
          providers.map((x) =>
            choiceCard(ui, `ai-prov-${x.id}`, {
              icon: x.icon,
              title: x.title,
              body: x.body,
              on: x.id === p,
              badges: [x.id === llm?.provider ? ui.badge("In use") : null],
              onClick: (e, cx) => this.pick(x.id, cx),
            }),
          ),
        ),
      );
    if (p === "claude_code")
      panel
        .child(ui.field("Claude Code program", h_flex().gap(8).child(this.command.el(ui, { mono: true })).child(ui.button("ai-check", "Check", (e, cx) => this.check(cx), { busy: this.checking })), "The claude command, or its full path."))
        .child(this.claudeState(ui));
    if (p === "openai_compatible") panel.child(ui.field("Endpoint", this.endpoint.el(ui, { mono: true }), "Ends in /v1. Ollama on this computer is http://localhost:11434/v1."));
    if (p !== "none") {
      if (p === "claude_code")
        panel.child(
          ui.field(
            "Model",
            h_flex()
              .flex_wrap()
              .items_center()
              .gap(8)
              .child(this.model.el(ui, { w: 260 }))
              .children(
                ["sonnet", "opus", "haiku"].map((m) =>
                  ui.button(
                    `ai-model-${m}`,
                    m,
                    (e, cx) => {
                      this.model.set(m);
                      cx.notify();
                    },
                    { small: true, tone: this.model.value === m ? "primary" : "secondary" },
                  ),
                ),
              ),
            "An alias such as sonnet or opus, or a full model name.",
          ),
        );
      else panel.child(ui.field("Model", this.model.el(ui), p === "anthropic" ? "A model name from the Anthropic docs." : "As the server names it, such as llama3.1."));
      if (p === "anthropic" || p === "openai_compatible") {
        const has = hasCredential("llm_api_key");
        panel.child(
          ui.field(
            "API key",
            this.key.el(ui),
            has ? "A key is saved. Type a new one to replace it." : p === "openai_compatible" ? "Leave empty if the server needs none, as Ollama does." : "Stored on this computer, readable only by you.",
          ),
        );
      }
      const short = p === "claude_code" && parseDuration(this.timeout) < 60_000;
      panel.child(
        ui.field(
          "Wait for an answer up to",
          h_flex()
            .items_center()
            .gap(12)
            .child(
              ui.durationInput(
                "ai-timeout",
                this.timeout,
                (v, cx) => {
                  this.timeout = v;
                  cx.notify();
                },
                { units: ["m", "s"] },
              ),
            )
            .when(short, (e) =>
              e.child(
                ui.button(
                  "ai-2m",
                  "Use 2 minutes",
                  (ev, cx) => {
                    this.timeout = "2m";
                    cx.notify();
                  },
                  { small: true, tone: "ghost" },
                ),
              ),
            ),
          short ? "Short for Claude Code, which often needs a minute or two." : "Claude Code often needs a minute or two for a long breakdown.",
        ),
      );
    }
    const dirty = this.dirty();
    const ready = this.claude?.is_claude && this.claude.logged_in;
    panel.child(
      h_flex()
        .flex_wrap()
        .items_center()
        .gap(8)
        .pt(12)
        .border_t(1)
        .border_color(t.border)
        .when(p === "claude_code" && this.claude?.is_claude && !this.claude.logged_in && !this.checking, (e) => e.child(ui.text("You can save now and sign in later.", { size: 12, color: ui.tone("warn") })))
        .child(div().flex_1())
        .when(llm?.provider !== "none" && !dirty && (p !== "claude_code" || ready), (e) => e.child(ui.button("ai-open", "Open the assistant", () => this.app.go("gwen"), { icon: "sparkles" })))
        .child(ui.button("ai-save", "Save changes", (e, cx) => this.save(cx), { tone: "primary", busy: this.saving, disabled: !dirty })),
    );
    return panel;
  }
}
