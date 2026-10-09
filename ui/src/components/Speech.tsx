import { useEffect, useRef, useState } from "react";
import { App, apiError, isHub } from "../api";
import type { main } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";

/** Where this viewer's provider, voice for each provider, and read-aloud choice are kept. */
const providerKey = "gwen.speech.provider";
const voiceKey = (provider: string) => `gwen.speech.voice.${provider}`;
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

/** Every useSpeech, told when this viewer picks a provider or voice, so all the speaker buttons follow. */
const pickers = new Set<() => void>();

function pick(key: string, value: string) {
  save(key, value);
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
 * Gwen's voice (cmd/gwen-ui/speech.go): the provider and voice this viewer
 * chose, whether that provider is ready, and say, which speaks text and
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
  const [, setPicked] = useState(0); // re-reads the choices when they change
  const live = useRef(true);
  // What to do when each of this hook's utterances ends, and the ends that
  // came before say heard the id, with their error.
  const ends = useRef(new Map<number, (error: string) => void>());
  const early = useRef(new Map<number, string>());
  const heard = useRef(0); // the utterance sounding now, for say

  const refresh = () => App.SpeechStatus().then((s) => live.current && setStatus(s), () => {});

  useEffect(() => {
    if (isHub) return;
    live.current = true;
    refresh();
    const picked = () => setPicked((n) => n + 1);
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
  const provider = providers.find((p) => p.id === load(providerKey)) ?? providers[0] ?? null;
  const saved = provider && load(voiceKey(provider.id));
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
    /** The provider this viewer chose, else the default; null until known. */
    provider,
    setProvider: (id: string) => pick(providerKey, id),
    progress: provider?.installing ? progress : null,
    ready: !!status?.player && !!provider?.ready,
    sounding,
    preparing,
    error,
    voice,
    setVoice: (v: string) => provider && pick(voiceKey(provider.id), v),
    say,
    stop: () => App.StopSaying(),
    install: () => App.InstallSpeech(provider?.id ?? ""),
    remove: () => App.RemoveSpeech(provider?.id ?? ""),
    /** Reads the status again, such as after a key is saved. */
    refresh,
  };
}
