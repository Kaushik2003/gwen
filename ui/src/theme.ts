import { useCallback, useEffect, useState } from "react";

export type ThemeId = "ghost" | "tide" | "ember";

export const themes: { id: ThemeId; label: string; blurb: string; accent: string; canvas: string }[] = [
  { id: "ghost", label: "Ghost", blurb: "Gwen's own colours: obsidian black, a web-slinger's crimson.", accent: "#d72638", canvas: "#0d1012" },
  { id: "tide", label: "Tide", blurb: "The same shapes, cooler and quieter.", accent: "#3ec1af", canvas: "#0b0f14" },
  { id: "ember", label: "Ember", blurb: "Warmer graphite, an apricot accent.", accent: "#e9935a", canvas: "#110f0d" },
];

const storageKey = "gwen.theme";
const defaultTheme: ThemeId = "ghost";

function isThemeId(v: string | null): v is ThemeId {
  return v === "ghost" || v === "tide" || v === "ember";
}

/** Reads the saved theme, falling back to the default. Browser storage can throw or be empty
 * (private window, cleared site data), so a missing or unreadable value is just the default. */
export function storedTheme(): ThemeId {
  try {
    const v = localStorage.getItem(storageKey);
    return isThemeId(v) ? v : defaultTheme;
  } catch {
    return defaultTheme;
  }
}

/** Sets `data-theme` on the root element. Call once at startup, before the first paint, and again
 * whenever the theme changes; per-viewer only (plan.md §5.2 names a `[ui] theme` config key for a
 * synced version, which `internal/config` does not have yet). */
export function applyTheme(id: ThemeId) {
  document.documentElement.dataset.theme = id;
}

export function applyStoredTheme() {
  applyTheme(storedTheme());
}

export function useTheme(): [ThemeId, (id: ThemeId) => void] {
  const [theme, setTheme] = useState(storedTheme);
  useEffect(() => applyTheme(theme), [theme]);
  const set = useCallback((id: ThemeId) => {
    setTheme(id);
    try {
      localStorage.setItem(storageKey, id);
    } catch {
      // Private window or blocked storage: the theme still applies, just for this session.
    }
  }, []);
  return [theme, set];
}
