// Settings › AI › Voice input and Voice output. The speech models live in
// gwen-ui, which answers through host.* without opening a window; talking
// itself happens in the Talk window, which this section opens.

import { div } from "gpui-kit";
import { v_flex, h_flex } from "gpui-base";
import { gwen } from "../../lib/gwen.js";
import { host } from "../../lib/host.js";
import { cardGrid, choiceCard, downloadProgress } from "./form.js";
import { Credential, loadCredentials } from "./credential.js";

export default class Voice {
  constructor(app, cx) {
    this.app = app;
    this.voice = null; // voice input's status
    this.speech = null; // Gwen's voice's status
    this.error = { voice: "", speech: "" };
    this.busy = { voice: false, speech: false };
    this.saying = false;
    this.keys = {}; // provider id -> Credential for its API key
    this.started = false;
    // While anything downloads, its progress is read again every two seconds.
    cx.timer.every(2000, (cx) => {
      if (this.voice?.installing || this.speech?.providers?.some((p) => p.installing)) this.load(cx);
    });
  }

  get name() {
    return gwen.config?.llm?.assistant_name || "Gwen";
  }

  sync(cx) {
    if (this.started) return;
    this.started = true;
    this.load(cx);
  }

  load(cx) {
    cx.spawn(async (cx) => {
      try {
        this.voice = await host.voiceStatus();
        this.error.voice = "";
      } catch (e) {
        this.error.voice = String(e?.message ?? e);
      }
      try {
        this.speech = await host.speechStatus();
        this.error.speech = "";
        for (const p of this.speech.providers ?? [])
          if (p.key && !this.keys[p.id]) this.keys[p.id] = new Credential(this.app, p.key, `${p.name} API key`, { hint: `From your ${p.name} account, kept in a file only you can read.`, onSaved: (cx) => this.load(cx) });
        await loadCredentials();
      } catch (e) {
        this.error.speech = String(e?.message ?? e);
      }
      cx.notify();
    });
  }

  /** Runs a host step for one panel, showing its failure there. */
  step(which, cx, body) {
    this.busy[which] = true;
    this.error[which] = "";
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        await body(cx);
      } catch (e) {
        this.error[which] = String(e?.message ?? e);
      }
      this.busy[which] = false;
      this.load(cx);
    });
  }

  installVoice(cx) {
    this.step("voice", cx, async () => {
      this.load(cx); // shows the download as it starts
      await host.installVoice();
      gwen.toast("Voice input is ready", "success");
    });
  }

  removeVoice(cx) {
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm({ title: "Remove the voice model?", body: `Frees ${this.voice?.download_mb ?? 100} MB. You can download it again any time.`, confirm: "Remove", danger: true }, cx);
      if (ok)
        this.step("voice", cx, async () => {
          await host.removeVoice();
          gwen.toast("Removed the voice model", "success");
        });
    });
  }

  /** The provider she speaks with: the one chosen, else the first. */
  current() {
    const ps = this.speech?.providers ?? [];
    return ps.find((p) => p.id === this.speech?.provider) ?? ps[0] ?? null;
  }

  voiceOf(p) {
    const id = this.speech?.voices?.[p.id] ?? "";
    return p.voices.some((v) => v.id === id) ? id : p.voices[0]?.id;
  }

  /**
   * Makes provider the one she speaks with, and voice its voice unless "".
   * gwen-ui keeps the choice, so the Talk window speaks the same way.
   */
  choose(provider, voice, cx) {
    if (this.speech) {
      // Shown at once; the status read after saving confirms it.
      this.speech.provider = provider;
      if (voice) this.speech.voices = { ...(this.speech.voices ?? {}), [provider]: voice };
    }
    this.step("speech", cx, () => host.chooseSpeech(provider, voice));
  }

  installSpeech(p, cx) {
    this.step("speech", cx, async () => {
      this.load(cx);
      await host.installSpeech(p.id);
      gwen.toast(`${this.name} can talk now`, "success");
    });
  }

  removeSpeech(p, cx) {
    this.app.spawn(cx, async (cx) => {
      const ok = await this.app.ui.confirm({ title: `Remove ${this.name}'s offline voice?`, body: `Frees ${p.download_mb ?? 300} MB. You can download it again any time.`, confirm: "Remove", danger: true }, cx);
      if (ok)
        this.step("speech", cx, async () => {
          await host.removeSpeech(p.id);
          gwen.toast(`Removed ${this.name}'s offline voice`, "success");
        });
    });
  }

  hear(p, cx) {
    if (this.saying) {
      host.stopSaying().catch(() => {});
      return;
    }
    this.saying = true;
    this.error.speech = "";
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        await host.say(`Hi, I'm ${this.name}. Tell me what's on your mind, and I'll sort out your day with you.`, p.id, this.voiceOf(p), 1);
      } catch (e) {
        this.error.speech = `${this.name} couldn't speak: ${String(e?.message ?? e)}`;
      }
      this.saying = false;
      cx.notify();
    });
  }

  voicePanel(ui) {
    const panel = ui.panel({ title: "Voice input", icon: "mic" }).child(
      ui.muted("Talk to Gwen instead of typing. NVIDIA's Parakeet model turns your speech into text right on this computer, in English. No audio leaves it, and it only takes memory while you talk.", { size: 13 }),
    );
    const st = this.voice;
    if (!st) return panel.child(this.error.voice ? ui.callout({ tone: "danger", icon: "circle-alert", text: `Voice input could not be checked: ${this.error.voice}` }) : ui.muted("Checking…", { size: 13 }));
    if (!st.recorder) panel.child(ui.callout({ tone: "warn", icon: "circle-alert", text: "Voice input records through PipeWire's recorder, which isn't installed. Install the pipewire-utils package, then reopen Settings." }));
    else if (st.installing) panel.child(downloadProgress(ui, st.progress, st.download_mb));
    else if (!st.installed)
      panel.child(
        h_flex()
          .items_center()
          .gap(12)
          .child(ui.button("vi-download", "Download voice model", (e, cx) => this.installVoice(cx), { tone: "primary", icon: "download", busy: this.busy.voice }))
          .child(ui.dim(`${st.download_mb} MB, once`)),
      );
    else
      panel
        .child(
          ui.callout({
            tone: "ok",
            icon: "circle-check",
            text: "Ready. Press Talk in the Gwen tab, or on the panel widget, and speak.",
            action: ui.button("vi-remove", "Remove", (e, cx) => this.removeVoice(cx), { small: true, tone: "ghost", icon: "trash-2", disabled: this.busy.voice }),
          }),
        )
        .child(
          h_flex()
            .items_center()
            .gap(12)
            .child(ui.button("vi-test", "Test it in Talk", (e, cx) => this.app.spawn(cx, () => host.talk()), { icon: "mic" }))
            .child(ui.dim("Opens the Talk window: say a sentence, and your words appear as you go.")),
        );
    if (this.error.voice) panel.child(ui.callout({ tone: "danger", icon: "circle-alert", text: this.error.voice }));
    return panel;
  }

  speechPanel(ui) {
    const name = this.name;
    const panel = ui
      .panel({ title: "Voice output", icon: "volume-2" })
      .child(ui.muted(`Let ${name} talk back, in a voice from the internet or one that runs on this computer. With voice input as well, Talk makes it a spoken conversation.`, { size: 13 }));
    const st = this.speech;
    const p = this.current();
    if (!st || !p) return panel.child(this.error.speech ? ui.callout({ tone: "danger", icon: "circle-alert", text: `${name}'s voice could not be checked: ${this.error.speech}` }) : ui.muted("Checking…", { size: 13 }));
    if (!st.player) panel.child(ui.callout({ tone: "warn", icon: "circle-alert", text: `${name}'s voice plays through PipeWire's player, which isn't installed. Install the pipewire-utils package, then reopen Settings.` }));
    panel.child(
      cardGrid(
        st.providers.map((x) =>
          choiceCard(ui, `vo-prov-${x.id}`, {
            icon: x.online ? "cloud" : "hard-drive",
            title: x.name,
            body: x.about,
            on: x.id === p.id,
            badges: [x.ready ? ui.badge("Ready", { tone: "ok", icon: "circle-check" }) : null],
            onClick: (e, cx) => this.choose(x.id, "", cx),
          }),
        ),
      ),
    );
    const key = p.key ? this.keys[p.id] : null;
    if (key)
      panel.child(
        v_flex()
          .gap(6)
          .child(key.el(ui))
          .when(Boolean(p.key_url), (e) => e.child(h_flex().child(ui.button("vo-get-key", "Get a key", (ev, cx) => cx.open_url(p.key_url), { small: true, tone: "ghost", icon: "external-link" })))),
      );
    if (p.download_mb > 0) {
      if (p.installing) panel.child(downloadProgress(ui, p.progress, p.download_mb, `${name}'s voice`));
      else if (!p.installed)
        panel.child(
          h_flex()
            .items_center()
            .gap(12)
            .child(ui.button("vo-download", `Download ${name}'s voice`, (e, cx) => this.installSpeech(p, cx), { tone: "primary", icon: "download", disabled: !st.player, busy: this.busy.speech }))
            .child(ui.dim(`${p.download_mb} MB, once`)),
        );
      else
        panel.child(
          ui.callout({
            tone: "ok",
            icon: "circle-check",
            text: "Downloaded. Press Talk to have a conversation.",
            action: ui.button("vo-remove", "Remove", (e, cx) => this.removeSpeech(p, cx), { small: true, tone: "ghost", icon: "trash-2", disabled: this.busy.speech }),
          }),
        );
    }
    if (p.ready && st.player)
      panel.child(
        h_flex()
          .items_end()
          .gap(14)
          .p(14)
          .rounded(8)
          .border_1()
          .border_color(ui.t.border)
          .child(ui.face(this.saying ? "happy" : "smiling", 64))
          .child(
            div()
              .flex_1()
              .min_w(0)
              .child(
                ui.field(
                  "Her voice",
                  ui.select(
                    "vo-voice",
                    p.voices.map((v) => ({ value: v.id, label: v.name })),
                    this.voiceOf(p),
                    (v, cx) => this.choose(p.id, v, cx),
                  ),
                ),
              ),
          )
          .child(ui.button("vo-hear", this.saying ? "Stop" : "Hear her", (e, cx) => this.hear(p, cx), { icon: this.saying ? "square" : "volume-2" })),
      );
    if (this.error.speech) panel.child(ui.callout({ tone: "danger", icon: "circle-alert", text: this.error.speech }));
    return panel;
  }

  render(ui) {
    return [this.voicePanel(ui), this.speechPanel(ui)];
  }
}
