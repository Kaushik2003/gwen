// What Gwen's screens do on this computer rather than through gwend: start
// the service, pick files, open folders, run `gwen setup` steps. Every
// program runs with a fixed argument list; nothing typed into a screen is
// ever handed to a shell.

import { randomBytes } from "crypto";
import { exec, home, pickFolder } from "herdr";

/** Runs a program and returns its trimmed output; throws its message when it fails. */
async function run(command, args, options) {
  let r;
  try {
    r = await exec(command, args, options);
  } catch (e) {
    throw new Error(`${command} could not start: ${String(e?.message ?? e)}`);
  }
  if (r.code !== 0) throw new Error((r.stderr || r.stdout || `${command} failed`).trim().split("\n").pop());
  return r.stdout.trim();
}

/** Whether a program is installed. */
async function has(command) {
  try {
    return (await exec("sh", ["-c", 'command -v "$1" >/dev/null', "sh", command])).code === 0;
  } catch {
    return false;
  }
}

/** The gwen command line, which is installed beside gwend. */
const gwenBin = async () => ((await has("gwen")) ? "gwen" : `${home()}/.local/bin/gwen`);

/** Runs `gwen-ui host <args>` and returns its JSON answer. */
async function uiHost(args, timeout = 30000) {
  const bin = (await has("gwen-ui")) ? "gwen-ui" : `${home()}/.local/bin/gwen-ui`;
  const out = await run(bin, ["host", ...args], { timeout });
  return out ? JSON.parse(out) : null;
}

export const host = {
  /** Starts gwend now and at every login, as `gwen setup service` does. */
  startService: () => run("systemctl", ["--user", "enable", "--now", "gwend.service"], { timeout: 20000 }),

  /** Whether the tray starts at login. */
  async autostart() {
    try {
      return (await exec("sh", ["-c", 'test -e "${XDG_CONFIG_HOME:-$HOME/.config}/autostart/gwen-tray.desktop"'])).code === 0;
    } catch {
      return false;
    }
  },

  /** Adds or removes the tray from autostart, as `gwen setup autostart` does. */
  async setAutostart(on) {
    await run(await gwenBin(), ["setup", "autostart", ...(on ? [] : ["--disable"])], { timeout: 20000 });
  },

  /**
   * Asks for a file in the desktop's own dialog. Resolves with its path, or
   * null when the dialog was closed. filter: such as "*.json".
   */
  async pickFile(title, filter) {
    let r;
    if (await has("kdialog")) r = await exec("kdialog", ["--title", title, "--getopenfilename", home(), filter ?? "*"], { timeout: 600000 });
    else if (await has("zenity")) r = await exec("zenity", ["--file-selection", "--title", title, ...(filter ? ["--file-filter", filter] : [])], { timeout: 600000 });
    else throw new Error("This desktop has no file dialog Gwen can use (kdialog or zenity).");
    const path = r.stdout.trim();
    return r.code === 0 && path ? path : null;
  },

  /** Asks where to save a file. Resolves with the path, or null when closed. */
  async pickSave(title, name) {
    let r;
    if (await has("kdialog")) r = await exec("kdialog", ["--title", title, "--getsavefilename", `${home()}/${name}`], { timeout: 600000 });
    else if (await has("zenity")) r = await exec("zenity", ["--file-selection", "--save", "--confirm-overwrite", "--title", title, "--filename", `${home()}/${name}`], { timeout: 600000 });
    else throw new Error("This desktop has no file dialog Gwen can use (kdialog or zenity).");
    const path = r.stdout.trim();
    return r.code === 0 && path ? path : null;
  },

  /** Asks for a folder. Resolves with its path, or null. */
  pickFolder: (prompt) => pickFolder({ prompt }),

  /** Opens a folder or file with the desktop's default program. */
  openPath: (path) => run("xdg-open", [path], { timeout: 10000 }),

  /**
   * Installs the Google OAuth client JSON from a file dialog and turns the
   * calendar on, as `gwen setup calendar --client-file` does. False when the
   * dialog was closed.
   */
  async chooseGoogleClient() {
    const path = await host.pickFile("Choose the OAuth client file from Google Cloud Console", "*.json");
    if (!path) return false;
    await run(await gwenBin(), ["setup", "calendar", "--client-file", path, "--yes"], { timeout: 30000 });
    return true;
  },

  /** Saves everything Gwen remembers as JSON where the person chooses. Resolves with the path, or null. */
  async saveMemoryExport() {
    const path = await host.pickSave("Save what Gwen remembers", "gwen-memory.json");
    if (!path) return null;
    await run(await gwenBin(), ["memory", "export", "--out", path], { timeout: 60000 });
    return path;
  },

  /**
   * Whether command is a signed-in Claude Code: {command, found, is_claude,
   * logged_in, plan, auth_method, detail, suggested}, as `gwen setup llm` checks.
   */
  async claudeCode(command) {
    const st = { command, found: false, is_claude: false, logged_in: false, plan: "", auth_method: "", detail: "", suggested: "" };
    const resolve = async (name) => {
      try {
        const r = await exec("sh", ["-c", 'command -v "$1"', "sh", name]);
        return r.code === 0 ? r.stdout.trim() : "";
      } catch {
        return "";
      }
    };
    const path = await resolve(command);
    if (!path) st.detail = `${command} was not found`;
    else {
      st.command = path;
      st.found = true;
      try {
        const r = await exec(path, ["auth", "status", "--json"], { timeout: 30000 });
        const auth = JSON.parse(r.stdout);
        if (typeof auth.loggedIn === "boolean") {
          st.is_claude = true;
          st.logged_in = auth.loggedIn;
          st.auth_method = auth.authMethod ?? "";
          st.plan = String(auth.subscriptionType ?? "").toLowerCase();
        } else st.detail = "it printed no sign-in status";
      } catch (e) {
        st.detail = String(e?.message ?? e);
      }
    }
    if (!st.is_claude) {
      const claude = (await resolve("claude")) || (await resolve(`${home()}/.local/bin/claude`));
      if (claude && claude !== st.command) st.suggested = claude;
    }
    return st;
  },

  /** Opens the Talk window: a spoken conversation with Gwen (gwen-ui --talk). */
  async talk() {
    const bin = (await has("gwen-ui")) ? "gwen-ui" : `${home()}/.local/bin/gwen-ui`;
    // Detached, so it outlives this call: the window stays until it is hung up.
    await run("sh", ["-c", 'setsid -f "$1" --talk >/dev/null 2>&1', "sh", bin], { timeout: 10000 });
  },

  /**
   * Publishes Gwen to the tailnet, or stops, through `tailscale serve` as
   * root: the desktop asks for the password once (plan.md 3.14.4).
   */
  async tailscaleServe(args) {
    if (!(await has("tailscale"))) throw new Error("Tailscale isn't installed on this laptop.");
    if (!(await has("pkexec"))) throw new Error("Gwen can't ask for your password here. In a terminal, run: gwen remote on");
    let r;
    try {
      r = await exec("pkexec", ["tailscale", "serve", ...args], { timeout: 120000 });
    } catch (e) {
      throw new Error(`Tailscale could not be started: ${String(e?.message ?? e)}`);
    }
    if (r.code === 0) return;
    if (r.code === 126 || r.code === 127) throw new Error("Nothing changed: the password prompt was closed.");
    throw new Error(`Tailscale didn't accept that: ${(r.stderr || r.stdout).trim().split("\n").pop()}`);
  },

  // Voice input and Gwen's voice. The speech models are linked into gwen-ui,
  // which also answers these without opening a window (`gwen-ui host ...`).

  /** Voice input: {installed, installing, recorder, download_mb, progress: {done, total} | null}. */
  voiceStatus: () => uiHost(["voice-status"]),
  /** Downloads the speech-to-text model, about 100 MB. Resolves when it is done; voiceStatus().progress moves meanwhile. */
  installVoice: () => uiHost(["voice-install"], 600000),
  removeVoice: () => uiHost(["voice-remove"]),
  /**
   * Gwen's voice: {player, providers: [{id, name, about, online, voices: [{id, name}], download_mb,
   * installed, installing, key, key_url, has_key, ready, progress: {done, total} | null}]}. The first provider is the default.
   */
  speechStatus: () => uiHost(["speech-status"]),
  /** Downloads a provider's voice model. Resolves when done; speechStatus() shows progress meanwhile. */
  installSpeech: (provider) => uiHost(["speech-install", provider], 600000),
  removeSpeech: (provider) => uiHost(["speech-remove", provider]),
  /** Makes provider the one she speaks with, and voice its voice unless "", for this window and Talk alike. */
  chooseSpeech: (provider, voice) => uiHost(["speech-choose", provider, ...(voice ? [voice] : [])]),
  /** Says text aloud with a provider's voice at speed (1 is normal), cutting off whatever was being said. Resolves when she has finished. */
  say: (text, provider, voice, speed) => uiHost(["say", provider, voice, String(speed ?? 1), text], 300000),
  /** Cuts off whatever is being said. */
  stopSaying: () => uiHost(["stop-saying"]),

  /** A fresh ntfy topic: "gwen-" and 24 lowercase base32 characters. */
  newTopic() {
    const alphabet = "abcdefghijklmnopqrstuvwxyz234567";
    const bytes = randomBytes(24);
    let out = "gwen-";
    for (const b of bytes) out += alphabet[b % 32];
    return out;
  },

  run,
  has,
  gwenBin,
};
