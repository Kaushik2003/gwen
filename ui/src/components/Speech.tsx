import { useEffect, useRef, useState } from "react";
import { App, apiError, isHub } from "../api";
import type { main } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";

/**
 * Where the old dashboard kept this viewer's provider and voice for each
 * provider. They live in gwen-ui now (SpeechStatus.provider and voices), so
 * Settings in the Gwen window and this window agree; what is still here is
 * moved there once.
 */
const providerKey = "gwen.speech.provider";
const voiceKey = (provider: string) => `gwen.speech.voice.${provider}`;
/** Whether replies are read aloud, which stays this viewer's own. */
const aloudKey = "gwen.speech.aloud";

function load(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function save(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // storage is a convenience
  }
}

function drop(key: string) {
  try {
    localStorage.removeItem(key);
  } catch {
    // storage is a convenience
  }
}

/** Moves the old dashboard's choices into gwen-ui, once, when nothing is chosen there yet. */
let moving: Promise<void> | null = null;
function moveOldChoices(s: main.SpeechStatus): Promise<void> {
  moving ??= (async () => {
    const old = load(providerKey);
    if (s.provider || !old) return;
    for (const p of s.providers) {
      const v = load(voiceKey(p.id));
      if (v && p.voices.some((x) => x.id === v)) await App.ChooseSpeech(p.id, v).catch(() => {});
    }
    if (s.providers.some((p) => p.id === old)) await App.ChooseSpeech(old, "").catch(() => {});
    drop(providerKey);
    s.providers.forEach((p) => drop(voiceKey(p.id)));
  })();
  return moving;
}

/** Every useSpeech, told when a provider or voice is picked, so all the speaker buttons follow. */
const pickers = new Set<() => void>();

async function pick(provider: string, voice: string): Promise<void> {
  await App.ChooseSpeech(provider, voice);
  pickers.forEach((f) => f());
}

/** Whether replies are read aloud, as this viewer last chose. */
export function useAloud(): [boolean, (on: boolean) => void] {
  const [aloud, setAloud] = useState(() => load(aloudKey) === "on");
  return [
    aloud,
    (on) => {
      save(aloudKey, on ? "on" : "off");
      setAloud(on);
    },
  ];
}

/**
 * Gwen's voice (cmd/gwen-ui/speech.go): the provider and voice chosen in
 * Settings, whether that provider is ready, and say, which speaks text and
 * calls back when it is done. Providers describe themselves, so nothing here
 * knows one from another. sounding is the utterance that can be heard now,
 * 0 for none; preparing is one asked for that is not yet heard.
 */
export function useSpeech() {
  const [status, setStatus] = useState<main.SpeechStatus | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [sounding, setSounding] = useState(0);
  const [preparing, setPreparing] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const live = useRef(true);
  // What to do when each of this hook's utterances ends, and the ends that
  // came before say heard the id, with their error.
  const ends = useRef(new Map<number, (error: string) => void>());
  const early = useRef(new Map<number, string>());
  const heard = useRef(0); // the utterance sounding now, for say

  const refresh = () =>
    App.SpeechStatus().then(
      async (s) => {
        if (!s.provider && load(providerKey)) {
          await moveOldChoices(s);
          s = await App.SpeechStatus();
        }
        if (live.current) setStatus(s);
      },
      () => {},
    );

  useEffect(() => {
    if (isHub) return;
    live.current = true;
    refresh();
    const picked = () => void refresh();
    pickers.add(picked);
    const offs = [
      EventsOn("speech:changed", refresh),
      EventsOn("speech:progress", setProgress),
      EventsOn("speech:start", (id: number) => {
        heard.current = id;
        setPreparing((p) => (p === id ? 0 : p));
        setSounding(id);
      }),
      EventsOn("speech:end", ({ id, error }: { id: number; error: string }) => {
        if (heard.current === id) heard.current = 0;
        setPreparing((p) => (p === id ? 0 : p));
        setSounding((s) => (s === id ? 0 : s));
        const end = ends.current.get(id);
        if (!end) {
          if (early.current.size > 50) early.current.clear(); // the others' utterances
          early.current.set(id, error);
          return;
        }
        ends.current.delete(id);
        if (error) setError(error);
        end(error);
      }),
    ];
    return () => {
      live.current = false;
      pickers.delete(picked);
      offs.forEach((off) => off());
    };
  }, []);

  // Speech does not outlive the screen it was started on.
  useEffect(() => () => void (ends.current.size > 0 && App.StopSaying()), []);

  const providers = status?.providers ?? [];
  const provider = providers.find((p) => p.id === status?.provider) ?? providers[0] ?? null;
  const saved = provider && status?.voices?.[provider.id];
  const voice = provider?.voices.find((v) => v.id === saved)?.id ?? provider?.voices[0]?.id ?? "";

  /** Says text, cutting off anything before; onEnd gets "" once it was said or cut off, else why not. Resolves to its id, or 0 when it could not start. */
  async function say(text: string, onEnd?: (error: string) => void): Promise<number> {
    setError(null);
    if (!provider) return 0;
    try {
      const id = await App.Say(text, provider.id, voice, 1);
      const end = onEnd ?? (() => {});
      if (early.current.has(id)) {
        const error = early.current.get(id)!;
        early.current.delete(id);
        if (error) setError(error);
        end(error);
      } else {
        ends.current.set(id, end);
        if (heard.current !== id) setPreparing(id);
      }
      return id;
    } catch (e) {
      setError(apiError(e).message);
      return 0;
    }
  }

  return {
    status,
    /** The provider chosen in Settings, else the default; null until known. */
    provider,
    setProvider: (id: string) => pick(id, "").catch((e) => setError(apiError(e).message)),
    progress: provider?.installing ? progress : null,
    ready: !!status?.player && !!provider?.ready,
    sounding,
    preparing,
    error,
    voice,
    setVoice: (v: string) => provider && pick(provider.id, v).catch((e) => setError(apiError(e).message)),
    say,
    stop: () => App.StopSaying(),
    install: () => App.InstallSpeech(provider?.id ?? ""),
    remove: () => App.RemoveSpeech(provider?.id ?? ""),
    /** Reads the status again, such as after a key is saved. */
    refresh,
  };
}
