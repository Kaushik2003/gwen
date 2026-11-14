import { CalendarRange, Target } from "lucide-react";
import { useMemo } from "react";
import type { wire } from "../api";
import { RadialBars } from "./liquid-charts/liquid/charts/radial";
import { Timeline } from "./liquid-charts/liquid/charts/time";
import type { ChartConfig } from "./liquid-charts/liquid/core";
import { daysBetween } from "../format";
import { goalAmount, goalFraction, evenPaceMark } from "./goal";
import { Panel } from "./ui";

const dayMs = 86_400_000;
const noProject = "#8a8f98";

/** A "YYYY-MM-DD" day as UTC midnight, which is how the Timeline reads its axis. */
function utcDay(day: string): number {
  const [y, m, d] = day.split("-").map(Number);
  return Date.UTC(y, m - 1, d);
}

/**
 * Active goals at a glance: a roadmap of each goal's dates with its progress poured in, and one ring track per goal
 * against where an even pace would have it today. Both colour a goal by its project.
 */
export default function GoalsGlance({ goals, projects, today }: { goals: wire.Goal[]; projects: wire.Project[]; today: string }) {
  const { config, tasks, bars } = useMemo(() => {
    const config: ChartConfig = {};
    const keyOf = (g: wire.Goal) => g.project_id ?? "none";
    for (const g of goals) {
      const p = projects.find((x) => x.id === g.project_id);
      config[keyOf(g)] = { label: p?.name ?? "No project", colors: { dark: [p?.color ?? noProject], light: [p?.color ?? noProject] } };
    }
    const tasks = goals.map((g) => ({
      id: g.id,
      name: g.title,
      group: config[keyOf(g)].label as string,
      series: keyOf(g),
      start: utcDay(g.start_day),
      // The timeline's end is exclusive: the day after the due day.
      end: utcDay(g.due_day) + dayMs,
      progress: goalFraction(g),
      note: goalAmount(g),
    }));
    // A goal is its own ring track, so each needs its own key and colour in the config.
    const bars = goals.map((g) => {
      const p = projects.find((x) => x.id === g.project_id);
      const mark = evenPaceMark(g, today);
      return {
        key: g.id,
        label: g.title,
        color: p?.color ?? noProject,
        value: Math.round(goalFraction(g) * 100),
        target: mark === null ? undefined : Math.round(mark * 100),
        measure: Math.max(daysBetween(today, g.due_day), 0),
      };
    });
    return { config, tasks, bars };
  }, [goals, projects, today]);

  const ringConfig: ChartConfig = Object.fromEntries(bars.map((b) => [b.key, { label: b.label, colors: { dark: [b.color], light: [b.color] } }]));

  // Rings have nothing to show until a goal has some progress; the roadmap then keeps the whole row.
  const started = bars.some((b) => b.value > 0);

  return (
    <div className={started ? "grid items-start gap-4 @4xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]" : undefined}>
      <Panel title="Roadmap" icon={CalendarRange}>
        <Timeline
          tasks={tasks}
          config={config}
          today={utcDay(today)}
          theme="dark"
          label="Active goals from their start to their due day, each filled to its progress. Use Left and Right to move between goals."
        />
      </Panel>
      {started && (
        <Panel title="Pace" icon={Target}>
          <RadialBars
            data={bars.map(({ key, value, target, measure }) => ({ key, value, target, measure }))}
            config={ringConfig}
            max={100}
            format={{ kind: "number", unit: "%" }}
            targetLabel="Even pace today"
            heightMeaning="second-measure"
            measureLabel="Days left"
            measureFormat={{ kind: "number" }}
            theme="dark"
            label="Each active goal's progress against where an even pace would have it today. Use the arrow keys to read each goal."
          />
        </Panel>
      )}
    </div>
  );
}
