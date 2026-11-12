import { useEffect, useState } from "react";
import { ApiError, api } from "./api";

/** A name for this phone from what the browser says about it; the person can change it. */
function guessName(): string {
  const ua = navigator.userAgent;
  const android = ua.match(/Android [\d.]+; ([^;)]+?)(?: Build|\))/);
  if (android) return android[1].trim();
  if (/iPhone/.test(ua)) return "iPhone";
  if (/iPad/.test(ua)) return "iPad";
  return "Phone";
}

/** The pairing page: the QR code on the laptop opens it with a one-time code after the #. */
export function Pair() {
  const [code, setCode] = useState(() => location.hash.replace(/^#/, "").trim());
  // Scanning a second code while this page is open only changes what follows the #.
  useEffect(() => {
    const changed = () => setCode(location.hash.replace(/^#/, "").trim());
    window.addEventListener("hashchange", changed);
    return () => window.removeEventListener("hashchange", changed);
  }, []);
  const [name, setName] = useState(guessName);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const pair = async () => {
    setBusy(true);
    setError("");
    try {
      await api.pair(code, name.trim() || "Phone");
      // Reload at the inbox, without the code in the address.
      location.replace("/");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Couldn't reach the laptop.");
      setBusy(false);
    }
  };

  return (
    <div className="pair">
      <img src="/icon-192.png" alt="" />
      <div className="col" style={{ gap: 6 }}>
        <h1>Pair this phone with Gwen</h1>
        <p className="muted">
          {code
            ? "This phone will be able to see and type into the terminals on your laptop, over your tailnet only."
            : "On the laptop, open Gwen › Settings › Phone and scan the QR code there with this phone's camera."}
        </p>
      </div>
      {code && (
        <>
          <div>
            <label className="label" htmlFor="device">What to call this phone</label>
            <input id="device" className="field" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
          </div>
          {error && <p className="error" role="alert">{error}</p>}
          <button className="btn primary full" disabled={busy} onClick={pair}>
            {busy ? <span className="spin" /> : "Pair"}
          </button>
        </>
      )}
    </div>
  );
}
