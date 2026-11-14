import { Sparkles, SquareTerminal, Sun } from "lucide-react";
import { useDay } from "./day";
import { useStore } from "./store";

/** The three screens at the bottom: Today (home), Work, and Gwen. Today counts what needs you. */
export function TabBar() {
  const { route, go, sessions } = useStore();
  const { approvals } = useDay();
  const needs = approvals.length + sessions.filter((s) => s.state === "needs_you").length;
  const tabs = [
    { name: "today" as const, label: "Today", icon: Sun, badge: needs },
    { name: "work" as const, label: "Work", icon: SquareTerminal, badge: 0 },
    { name: "gwen" as const, label: "Gwen", icon: Sparkles, badge: 0 },
  ];
  return (
    <nav className="tabbar" aria-label="Screens">
      {tabs.map((t) => (
        <button
          key={t.name}
          type="button"
          className={route.name === t.name ? "on" : undefined}
          aria-current={route.name === t.name ? "page" : undefined}
          onClick={() => route.name !== t.name && go({ name: t.name }, true)}
        >
          <t.icon size={22} strokeWidth={2} aria-hidden />
          {t.label}
          {t.badge > 0 && (
            <span className="tabbadge" aria-label={`${t.badge} need you`}>
              {t.badge}
            </span>
          )}
        </button>
      ))}
    </nav>
  );
}
