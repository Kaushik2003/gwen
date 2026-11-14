import { Check } from "lucide-react";
import { Panel, cx } from "../components/ui";
import { themes } from "../theme";
import { useTheme } from "../theme";

/** Lives only in this browser (plan.md §5.2's `[ui] theme` would sync it; internal/config has no
 * `[ui]` section yet), so it is read and written through theme.ts's localStorage helpers directly
 * rather than ConfigForm's PATCH /v1/config round trip. */
export default function AppearanceSettings() {
  const [theme, setTheme] = useTheme();
  return (
    <Panel title="Appearance" bodyClassName="grid gap-3 @sm:grid-cols-3">
      {themes.map((t) => {
        const on = t.id === theme;
        return (
          <button
            key={t.id}
            type="button"
            aria-pressed={on}
            onClick={() => setTheme(t.id)}
            className={cx(
              "flex flex-col gap-3 rounded-lg border p-3.5 text-left transition-colors",
              on ? "border-accent bg-accent/10" : "border-line-strong bg-surface-2 hover:border-line-3",
            )}
          >
            <span className="flex items-center justify-between">
              <span className="flex overflow-hidden rounded-md border border-line" style={{ width: 56, height: 28 }}>
                <span className="flex-1" style={{ backgroundColor: t.canvas }} />
                <span className="flex-1" style={{ backgroundColor: t.accent }} />
              </span>
              {on && (
                <span className="grid size-5 place-items-center rounded-full bg-accent text-on-accent">
                  <Check size={12} strokeWidth={3} aria-hidden />
                </span>
              )}
            </span>
            <span>
              <span className="block text-sm font-medium text-ink">{t.label}</span>
              <span className="block text-[13px] text-ink-subtle">{t.blurb}</span>
            </span>
          </button>
        );
      })}
    </Panel>
  );
}
