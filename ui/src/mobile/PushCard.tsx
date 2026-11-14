import { Bell, BellOff, Smartphone } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "./api";
import { useStore } from "./store";

// Phone notifications (plan.md 3.14.11): one tap turns them on. The phone asks for permission, subscribes in the service
// worker, and hands the subscription to the laptop, which pushes while you are away from it: a session needs you,
// finished, or stopped, or Gwen waits for your OK. Once on, the card is gone; a renewed subscription is sent again quietly.

type Phase = "hidden" | "off" | "blocked" | "install";

const DISMISSED = "gwen.push.dismissed";

const supported = () => window.isSecureContext && "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
const iPhone = () => /iPhone|iPad|iPod/.test(navigator.userAgent);
const standalone = () => matchMedia("(display-mode: standalone)").matches || (navigator as { standalone?: boolean }).standalone === true;

function keyBytes(key: string): Uint8Array<ArrayBuffer> {
  const s = atob(key.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (key.length % 4)) % 4));
  return Uint8Array.from(s, (c) => c.charCodeAt(0));
}

// What to try when the browser refuses to subscribe, in the order that fixes it most often.
function subscribeHelp(e: unknown): string {
  const name = e instanceof DOMException ? e.name : "";
  if (name === "NotAllowedError") return "Notifications are blocked for this site. Allow them in the browser's site settings, then try again.";
  if (name === "NotSupportedError") return "This browser can't do push notifications. Use Chrome, or add Gwen to the Home Screen.";
  const brave = (navigator as { brave?: unknown }).brave !== undefined;
  return (
    "Your phone couldn't reach Google's push service. " +
    (brave ? "In Brave, turn on Settings > Privacy > Use Google services for push messaging. " : "") +
    "Check that Play Services is up to date, that the phone is online without a VPN or Private DNS filter other than Tailscale, then try again."
  );
}

function dismissed(): boolean {
  try {
    return localStorage.getItem(DISMISSED) === "1";
  } catch {
    return false;
  }
}

export function PushCard() {
  const { toast } = useStore();
  const [phase, setPhase] = useState<Phase>("hidden");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let live = true;
    (async () => {
      if (!supported()) {
        if (iPhone() && !standalone() && !dismissed()) setPhase("install");
        return;
      }
      const reg = await navigator.serviceWorker.ready;
      const [sub, st] = await Promise.all([reg.pushManager.getSubscription(), api.pushState()]);
      if (!live) return;
      if (Notification.permission === "granted" && sub) {
        // On already: make sure the laptop has this phone's current subscription.
        if (!st.subscribed || st.endpoint !== sub.endpoint) await api.pushSubscribe(sub.toJSON());
        return;
      }
      if (dismissed()) return;
      setPhase(Notification.permission === "denied" ? "blocked" : "off");
    })().catch(() => {});
    return () => {
      live = false;
    };
  }, []);

  const turnOn = async () => {
    setBusy(true);
    try {
      const allowed = await Notification.requestPermission();
      if (allowed !== "granted") {
        setPhase(allowed === "denied" ? "blocked" : "off");
        return;
      }
      const [reg, st] = await Promise.all([navigator.serviceWorker.ready, api.pushState()]);
      const key = keyBytes(st.key);
      let sub = await reg.pushManager.getSubscription();
      if (sub) {
        // A subscription made for another key (the laptop's was made again) can't be used.
        const had = sub.options.applicationServerKey ? new Uint8Array(sub.options.applicationServerKey) : null;
        if (!had || had.length !== key.length || had.some((b, i) => b !== key[i])) {
          await sub.unsubscribe();
          sub = null;
        }
      }
      try {
        sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
      } catch (e) {
        // Chrome says only "Registration failed - push service error": the phone could not reach Google's push service (FCM).
        throw new Error(subscribeHelp(e));
      }
      await api.pushSubscribe(sub.toJSON());
      await api.pushTest();
      setPhase("hidden");
      toast("Notifications are on: a test one is on its way");
    } catch (e) {
      toast(e instanceof Error ? e.message : "Notifications couldn't be turned on");
    } finally {
      setBusy(false);
    }
  };

  const later = () => {
    try {
      localStorage.setItem(DISMISSED, "1");
    } catch {
      // Private mode: it shows again next time.
    }
    setPhase("hidden");
  };

  if (phase === "hidden") return null;
  const Icon = phase === "blocked" ? BellOff : phase === "install" ? Smartphone : Bell;
  return (
    <section className="card" aria-label="Notifications">
      <div className="card-name">
        <Icon size={17} aria-hidden />
        {phase === "install" ? "Notifications need Gwen on your Home Screen" : phase === "blocked" ? "Notifications are blocked" : "Get notified when something needs you"}
      </div>
      <p style={{ margin: 0, fontSize: 13.5, lineHeight: "19px", color: "var(--text-2)" }}>
        {phase === "install"
          ? "In Safari, tap Share, then Add to Home Screen, and open Gwen from there."
          : phase === "blocked"
            ? "Allow notifications for this site in your browser's settings, then come back."
            : "While you're away from the laptop: a session needs you, finishes, or stops, or Gwen waits for your OK."}
      </p>
      <div style={{ display: "flex", gap: 8 }}>
        {phase === "off" && (
          <button className="btn primary" disabled={busy} onClick={turnOn}>
            {busy ? "Turning on…" : "Turn on"}
          </button>
        )}
        <button className="btn" onClick={later}>
          {phase === "off" ? "Not now" : "OK"}
        </button>
      </div>
    </section>
  );
}
