// Gwen: talk to her in plain words. She adds, moves, reschedules and
// reassigns tasks as she answers, and each reply lists what it changed.
// Chats are kept: Past chats opens or deletes an earlier one. Talk opens the
// Talk window for a spoken conversation.

import { div } from "gpui-kit";
import { v_flex, h_flex, ScrollHandle } from "gpui-base";
import { api, apiError, gwen } from "../lib/gwen.js";
import { host } from "../lib/host.js";
import { formatAgo, formatTime } from "../lib/format.js";
import { attitudeOf } from "../lib/llm.js";
import { threadOf } from "../lib/memory.js";
import { MemoryDetail } from "../lib/memory-detail.js";
import { aiFailed, aiProgress, aiUnavailable, planName, providerName } from "../lib/ai.js";
import { TextField, alpha } from "../lib/kit.js";
import { hasCredential, loadCredentials } from "./settings/credential.js";

/** Things to say with one click. */
const starters = [
  "What should I do first today?",
  "Process my inbox",
  "I have an exam on Friday, help me prepare",
  "Move my hardest task to my prime time",
  "Sort my tasks with the Eisenhower matrix",
  "Turn my goal into a SMART goal",
];

/** How many past chats are listed at a time. */
const pastPage = 50;

/** The face of an assistant turn: the one she chose, else one for what she did. */
function turnMood(m) {
  if (m.mood) return m.mood;
  const actions = m.actions ?? [];
  return actions.some((a) => !a.ok) ? "shocked" : actions.length ? "happy" : "smiling";
}

export default class Chat {
  constructor(app) {
    this.app = app;
    this.thread = null; // {id, messages}
    this.past = null; // {threads, more} while the earlier chats are listed instead
    this.asking = null; // {message, since}
    this.failure = null;
    this.unavailable = null;
    this.refused = null;
    this.attitude = "";
    this.claude = undefined; // Claude Code's sign-in, once asked: undefined not asked, null asking
    this.claudeFor = "";
    this.loaded = {};
    this.started = false;
    // Why she remembers what a chip names, and Keep, Not true and Forget on it.
    this.detail = new MemoryDetail(app, { onChanged: (cx) => this.reload(cx) });
    this.scroll = ScrollHandle.new();
    this.input = new TextField({ multiline: true, rows: 2, placeholder: "Tell Gwen what's on your mind", onSubmit: (v, cx) => this.send(v, cx) });
  }

  get tick() {
    return this.asking !== null;
  }

  get name() {
    return gwen.config?.llm?.assistant_name || "Gwen";
  }

  remember(id) {
    try {
      if (id) localStorage.setItem("thread", id);
      else localStorage.removeItem("thread");
    } catch {}
  }

  sync(cx) {
    this.detail.sync(cx);
    const llm = gwen.config?.llm;
    if (!this.started) {
      this.started = true;
      cx.spawn(async (cx) => {
        this.attitude = (await api.AssistantSelf().catch(() => null))?.attitude ?? "";
        let id = null;
        try {
          id = localStorage.getItem("thread");
        } catch {}
        if (id && !this.thread) {
          try {
            const t = await api.GetThread(id);
            if (!this.thread) this.setThread({ id: t.thread.id, messages: t.messages ?? [] });
          } catch (e) {
            // A chat that is gone is forgotten; one the daemon could not read now is kept for next time.
            if (apiError(e).code === "not_found") this.remember(null);
          }
        }
        cx.notify();
      });
    }
    // Whether the provider can answer right now.
    const key = `${llm?.provider}/${llm?.command}`;
    if (llm && this.claudeFor !== key) {
      this.claudeFor = key;
      this.claude = llm.provider === "claude_code" ? null : undefined;
      cx.spawn(async (cx) => {
        if (llm.provider === "claude_code") this.claude = await host.claudeCode(llm.command).catch(() => undefined);
        else await loadCredentials();
        cx.notify();
      });
    }
    // What she learned or forgot changes the chips: the open chat is read again, unless a turn is on its way.
    const stamp = gwen.versions.memory;
    if (this.loaded.memory !== stamp) {
      const first = this.loaded.memory === undefined;
      this.loaded.memory = stamp;
      if (!first) this.reload(cx);
    }
  }

  reload(cx) {
    const id = this.thread?.id;
    if (!id || this.asking) return;
    cx.spawn(async (cx) => {
      const t = await api.GetThread(id).catch(() => null);
      if (t && this.thread?.id === t.thread.id && (t.messages ?? []).length >= this.thread.messages.length) this.thread = { id, messages: t.messages ?? [] };
      cx.notify();
    });
  }

  /** Another screen opened the chat: on one of the chats ("thread:<id>"), or with a message ready to send. */
  show(_sub, param, cx) {
    if (!param) return;
    const thread = threadOf(param);
    if (thread) this.open(thread, cx);
    else {
      this.input.set(param);
      this.input.focus();
    }
  }

  setThread(thread) {
    this.thread = thread;
    this.remember(thread?.id ?? null);
    this.toEnd();
  }

  /** Shows the newest message. */
  toEnd() {
    const n = (this.thread?.messages.length ?? 0) + (this.asking ? 1 : 0);
    if (n > 0) this.scroll.scroll_to_item(n); // the list starts with a spacer, so item n is the last turn
  }

  send(message, cx) {
    const m = String(message ?? "").trim();
    if (!m || this.asking) return;
    this.asking = { message: m, since: Date.now() };
    this.past = null; // what is sent goes to the chat on screen
    this.input.set("");
    this.failure = this.unavailable = this.refused = null;
    this.toEnd();
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const r = await api.AssistantChat({ message: m, thread_id: this.thread?.id ?? null });
        if (r.status === "failed") {
          this.failure = r.output?.error ?? "The assistant did not answer.";
          this.input.set(m);
        } else {
          const out = r.output ?? {};
          const turn = out.messages ?? [];
          this.asking = null;
          this.setThread({ id: out.thread_id, messages: [...(this.thread?.id === out.thread_id ? this.thread.messages : []), ...turn] });
          const reply = turn.at(-1);
          if (reply?.attitude) this.attitude = reply.attitude;
        }
      } catch (e) {
        const err = apiError(e);
        this.input.set(m);
        if (err.code === "unavailable") this.unavailable = err.message;
        else if (err.code === "invalid_request") this.refused = err.message;
        else gwen.fail(e);
      }
      this.asking = null;
      this.input.focus();
      cx.notify();
    });
  }

  startOver(cx) {
    this.setThread(null);
    this.past = null;
    this.failure = this.refused = null;
    this.input.set("");
    cx.notify();
  }

  /** Lists the earlier chats in place of this one, or goes back to it. */
  browse(cx) {
    if (this.past) {
      this.past = null;
      cx.notify();
      return;
    }
    this.app.spawn(cx, async () => {
      const { threads } = await api.ListThreads(pastPage, 0);
      this.past = { threads: threads ?? [], more: (threads ?? []).length === pastPage };
    });
  }

  /** Lists the next page of older chats under those listed. */
  older(cx) {
    if (!this.past) return;
    this.app.spawn(cx, async () => {
      const { threads } = await api.ListThreads(pastPage, this.past.threads.length);
      const seen = new Set(this.past.threads.map((t) => t.id));
      this.past = { threads: [...this.past.threads, ...(threads ?? []).filter((t) => !seen.has(t.id))], more: (threads ?? []).length === pastPage };
    });
  }

  open(id, cx) {
    cx.spawn(async (cx) => {
      try {
        const t = await api.GetThread(id);
        this.past = null;
        this.failure = this.refused = null;
        this.setThread({ id: t.thread.id, messages: t.messages ?? [] });
      } catch (e) {
        if (apiError(e).code === "not_found") gwen.toast("That chat has been deleted.");
        else gwen.fail(e);
      }
      cx.notify();
    });
  }

  /** Deletes a chat: its words are erased, and she no longer recalls them. */
  forgetChat(t, cx) {
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm(
        {
          title: `Delete "${t.title || "Untitled chat"}"?`,
          body: `Its messages are erased, and ${this.name} no longer remembers them. What she changed in your tasks stays, and so does her note to herself.`,
          confirm: "Delete chat",
          danger: true,
        },
        cx,
      );
      if (!ok) return;
      await api.DeleteThread(t.id);
      if (this.past) this.past = { ...this.past, threads: this.past.threads.filter((x) => x.id !== t.id) };
      if (this.thread?.id === t.id) this.setThread(null);
    });
  }

  // Drawing ------------------------------------------------------------------

  turn(ui, m, key, o = {}) {
    const t = ui.t;
    const user = m.role === "user";
    const actions = m.actions ?? [];
    const switched = attitudeOf(m.attitude);
    const col = v_flex().max_w("85%").min_w(0).gap(7);
    if (user) col.items_end();
    col.child(
      div()
        .px(13)
        .py(9)
        .rounded(12)
        .text_size(13)
        .line_height("20px")
        .when(user, (e) => e.bg(alpha(t.accent, 0.15)).text_color(t.foreground))
        .when(!user, (e) => e.border_1().border_color(t.border).bg(t.raised).text_color(t.foreground))
        .when(Boolean(o.pending), (e) => e.opacity(0.7))
        .child(m.text),
    );
    if (switched)
      col.child(
        h_flex()
          .id(`${key}-att`)
          .items_center()
          .gap(7)
          .px(4)
          .tooltip(switched.means)
          .child(ui.icon("drama", 13, t.accent))
          .child(ui.muted(`${this.name} is ${switched.label.toLowerCase()} with you now`, { size: 12 })),
      );
    if (actions.length)
      col.child(
        v_flex()
          .gap(4)
          .children(
            actions.map((a) =>
              h_flex()
                .items_start()
                .gap(7)
                .px(9)
                .py(5)
                .rounded(6)
                .border_1()
                .border_color(alpha(ui.tone(a.ok ? "ok" : "danger"), 0.25))
                .bg(alpha(ui.tone(a.ok ? "ok" : "danger"), 0.08))
                .child(div().pt(2).child(ui.icon(a.ok ? "circle-check" : "circle-x", 13, ui.tone(a.ok ? "ok" : "danger"))))
                .child(div().min_w(0).text_size(12).line_height("17px").text_color(a.ok ? t.foreground : t.muted).child(`${a.summary}${!a.ok && a.error ? `, not done: ${a.error}` : ""}`)),
            ),
          ),
      );
    const chips = this.detail.chips(ui, key, m);
    if (chips) col.child(chips);
    if (m.at > 0 && !o.pending) col.child(div().px(4).text_size(11).text_color(t.dim).child(formatTime(m.at)));
    const row = h_flex().w_full().items_start().gap(10).px(20).py(7);
    if (user) return row.justify_end().child(col);
    return row.child(div().flex_none().child(ui.face(turnMood(m), 44))).child(col);
  }

  /** Which provider answers, and whether it can right now: shown only when something needs fixing. */
  providerNotice(ui) {
    const llm = gwen.config?.llm;
    if (!llm) return null;
    const fix = (id, label) => ui.button(id, label, () => this.app.go("settings", "ai"), { small: true, tone: "primary", icon: "settings" });
    if (llm.provider === "none")
      return ui.callout({ tone: "accent", icon: "sparkles", text: "No AI provider yet. Use your Claude subscription through Claude Code, an Anthropic API key, or a local model such as Ollama.", action: fix("chat-setup", "Set up AI") });
    if (llm.provider === "claude_code") {
      const c = this.claude;
      if (!c || (c.is_claude && c.logged_in)) return null;
      const text = c.is_claude ? "Claude Code is not signed in." : c.found ? `${c.command} is not Claude Code.` : `Claude Code was not found at ${c.command}.`;
      return ui.callout({ tone: c.is_claude ? "warn" : "danger", icon: "circle-alert", text, action: fix("chat-fix", "Fix in settings") });
    }
    if (llm.provider === "anthropic" && hasCredential("llm_api_key") === false) return ui.callout({ tone: "warn", icon: "key-round", text: "No API key saved for the Anthropic API.", action: fix("chat-key", "Fix in settings") });
    return null;
  }

  providerLine() {
    const llm = gwen.config?.llm;
    const c = this.claude;
    if (llm?.provider === "claude_code" && c?.is_claude && c.logged_in && c.plan) return `Claude Code, your ${planName(c.plan)} plan · changes your tasks and plan as you talk`;
    return `${providerName(llm)} · changes your tasks and plan as you talk`;
  }

  pastChats(ui) {
    const t = ui.t;
    const past = this.past;
    if (past.threads.length === 0)
      return [div().px(20).py(40).flex().justify_center().child(ui.muted(`No chats yet. What you say to ${this.name} is kept here, and she remembers it in later chats.`, { size: 13 }))];
    const now = Date.now();
    const rows = [div().px(20).pb(4).child(ui.muted(`${this.name} remembers what was said in these chats when it comes up again. Deleting one erases it.`, { size: 12 }))];
    for (const th of past.threads) {
      const on = th.id === this.thread?.id;
      rows.push(
        div().px(20).py(2).child(
          h_flex()
            .items_center()
            .gap(8)
            .px(12)
            .py(8)
            .rounded(8)
            .border_1()
            .border_color(on ? alpha(t.accent, 0.4) : t.border)
            .bg(on ? alpha(t.accent, 0.06) : t.raised)
            .child(
              v_flex()
                .id(`chat-past-${th.id}`)
                .flex_1()
                .min_w(0)
                .gap(2)
                .cursor_pointer()
                .on_click((e, cx) => this.open(th.id, cx))
                .child(div().truncate().text_size(13).text_color(t.foreground).child(th.title || "Untitled chat"))
                .child(ui.dim(`${formatAgo(th.last_at, now)} · ${th.messages} ${th.messages === 1 ? "message" : "messages"}`, { size: 11 })),
            )
            .child(ui.iconButton(`chat-past-del-${th.id}`, "trash-2", (e, cx) => this.forgetChat(th, cx), { tip: "Delete this chat", size: 24 })),
        ),
      );
    }
    if (past.more) rows.push(h_flex().justify_center().py(6).child(ui.button("chat-older", "Show older chats", (e, cx) => this.older(cx), { small: true, tone: "ghost" })));
    return rows;
  }

  render(ui) {
    const t = ui.t;
    const llm = gwen.config?.llm;
    const name = this.name;
    const messages = this.thread?.messages ?? [];
    const last = messages.filter((m) => m.role === "assistant").at(-1);
    const mood = this.asking ? "thinking" : this.failure || this.unavailable ? "crying" : this.refused ? "annoyed" : last ? turnMood(last) : "smiling";
    const att = attitudeOf(this.attitude);
    const busy = this.asking !== null;

    const header = h_flex()
      .flex_none()
      .w_full()
      .items_center()
      .gap(12)
      .px(20)
      .h(64)
      .border_b(1)
      .border_color(t.border)
      .child(ui.face(mood, 46))
      .child(
        v_flex()
          .flex_1()
          .min_w(0)
          .gap(2)
          .child(
            h_flex()
              .items_center()
              .gap(8)
              .child(div().text_size(14).font_semibold().child(name))
              .when(Boolean(att), (e) => e.child(ui.badge(att.label, { tone: this.attitude === "angry" ? "danger" : "accent", tip: att.means }))),
          )
          .child(ui.muted(this.providerLine(), { size: 12, truncate: true })),
      )
      .child(ui.button("chat-talk", "Talk", (e, cx) => this.app.spawn(cx, () => host.talk()), { small: true, icon: "audio-lines", tip: "A spoken conversation with her, in the Talk window" }))
      .child(ui.iconButton("chat-past", "history", (e, cx) => this.browse(cx), { tip: "Past chats", active: Boolean(this.past), disabled: busy }))
      .when(Boolean(this.thread || this.failure), (e) => e.child(ui.button("chat-new", "New chat", (ev, cx) => this.startOver(cx), { small: true, tone: "ghost", icon: "message-square-plus", disabled: busy })));

    let items;
    if (llm?.provider === "none") items = [];
    else if (this.past) items = this.pastChats(ui);
    else if (messages.length === 0 && !busy)
      items = [
        v_flex()
          .w_full()
          .items_center()
          .gap(14)
          .py(28)
          .px(20)
          .child(ui.face("winking", 132))
          .child(
            div()
              .max_w(460)
              .text_size(13)
              .line_height("20px")
              .text_color(t.muted)
              .child(`Talk to ${name} like you would to a person. Tell her about new work, ask what to do next, or say "move the report to tomorrow afternoon": she adds, moves, reschedules, and reassigns tasks for you, and tells you what she changed.`),
          )
          .child(
            h_flex()
              .max_w(600)
              .flex_wrap()
              .justify_center()
              .gap(8)
              .children(
                starters.map((s, i) =>
                  div()
                    .id(`chat-start-${i}`)
                    .px(12)
                    .h(28)
                    .flex()
                    .items_center()
                    .rounded(14)
                    .border_1()
                    .border_color(t.input)
                    .bg(t.raised)
                    .text_size(12)
                    .text_color(t.muted)
                    .cursor_pointer()
                    .hover((st) => st.text_color(t.foreground).bg(t.hover))
                    .on_click((e, cx) => this.send(s, cx))
                    .child(s),
                ),
              ),
          )
          .child(
            h_flex()
              .pt(6)
              .flex_wrap()
              .justify_center()
              .gap(8)
              .child(ui.button("chat-plan", "Plan today with AI", () => this.app.go("plan", "plan", "chat"), { small: true, tone: "ghost", icon: "calendar-clock" }))
              .child(ui.button("chat-goal", "Break a goal into tasks", () => this.app.go("goals"), { small: true, tone: "ghost", icon: "target" }))
              .child(ui.button("chat-retro", "Weekly retro", () => this.app.go("stats"), { small: true, tone: "ghost", icon: "scroll-text" })),
          ),
      ];
    else {
      items = messages.map((m, i) => this.turn(ui, m, `chat-m-${m.id ?? i}`));
      if (busy) items.push(this.turn(ui, { role: "user", text: this.asking.message, at: this.asking.since }, "chat-pending", { pending: true }));
    }

    // Why something stopped, above the box.
    const notices = [];
    const provider = this.providerNotice(ui);
    if (provider) notices.push(provider);
    if (busy) notices.push(aiProgress(ui, this.asking.since, `${name} is thinking`));
    if (this.unavailable) notices.push(aiUnavailable(ui, this.app, this.unavailable, "chat-unavailable"));
    if (this.refused)
      notices.push(ui.callout({ tone: "neutral", icon: "circle-alert", text: `${this.refused[0].toUpperCase()}${this.refused.slice(1)}.`, action: ui.button("chat-refused-new", "New chat", (e, cx) => this.startOver(cx), { small: true }) }));
    if (this.failure) notices.push(aiFailed(ui, this.app, this.failure, "chat-failed"));

    const canSend = Boolean(this.input.value.trim()) && !busy && llm?.provider !== "none";
    const composer = v_flex()
      .flex_none()
      .w_full()
      .gap(10)
      .px(20)
      .py(14)
      .border_t(1)
      .border_color(t.border)
      .children(notices)
      .child(
        h_flex()
          .items_end()
          .gap(8)
          .child(this.input.el(ui, { h: 62 }))
          .child(ui.button("chat-send", "", (e, cx) => this.send(this.input.value, cx), { tone: "primary", icon: "send-horizontal", disabled: !canSend, tip: "Send" })),
      )
      .child(ui.dim("Enter sends, Shift+Enter starts a new line. Talk opens a spoken conversation."));

    return v_flex()
      .size_full()
      .min_h(0)
      .child(header)
      .child(
        div()
          .id("chat-scroll")
          .flex_1()
          .min_h(0)
          .w_full()
          .overflow_y_scrollbar()
          .track_scroll(this.scroll)
          .child(div().h(10))
          .children(items)
          .child(div().h(10)),
      )
      .child(composer);
  }
}
