import { CalendarClock, CircleAlert, CircleCheck, KeyRound, LoaderCircle, ScrollText, Settings as SettingsIcon, Sparkles, Target } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { App, wire } from "../api";
import { BreakdownFlow, RetroFlow, planName } from "../components/ai";
import AssistantChat from "../components/AssistantChat";
import { GoalMeter, goalAmount } from "../components/goal";
import { Button, Empty, PageHeader, Panel, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDate } from "../format";
import { useNav } from "../nav";
import type { main } from "../wailsjs/go/models";

/** The LLM's three jobs in one place, with whether it is ready (docs/08-clients.md#v3-additions). */
export default function Assistant() {
  const d = useDaemon();
  const { go, param } = useNav();
  const [goals, setGoals] = useState<wire.Goal[] | null>(null);
  const [goalId, setGoalId] = useState<string | null>(null);
  // Another screen can open the chat with a message ready to send.
  const [initial, setInitial] = useState<string | null>(param);

  useEffect(() => {
    App.ListGoals("active").then((l) => setGoals(l.goals), d.fail);
  }, [d.goalsVersion, d.tasksVersion, d.fail]);

  const goal = goals?.find((g) => g.id === goalId) ?? goals?.[0] ?? null;
  return (
    <div className="flex flex-col gap-5">
      <PageHeader title="Assistant" subtitle="Talk to your assistant in plain words: it adds, moves, reschedules, and reassigns your tasks, plans your day, and sets SMART goals with you." />
      <ProviderStatus />
      {d.config?.llm.provider !== "none" && <AssistantChat initial={initial} onInitialUsed={() => setInitial(null)} />}

      <Panel title="Plan your day" icon={CalendarClock}>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
          <p className="min-w-64 flex-1 text-[13px] leading-relaxed text-ink-subtle">
            Talk the day through: when you start, how long you have, what matters most. The assistant proposes blocks from your open tasks, shown on the timeline, and you apply them when they look right.
          </p>
          <Button tone="primary" icon={Sparkles} onClick={() => go("plan", "chat")}>
            Plan today with AI
          </Button>
        </div>
      </Panel>

      <Panel title="Break a goal into tasks" icon={Sparkles}>
        {goals && goals.length === 0 ? (
          <Empty
            icon={Target}
            title="No active goals"
            action={
              <Button icon={Target} onClick={() => go("goals")}>
                Create a goal
              </Button>
            }
          >
            Create a goal first, then come back to have it broken into tasks.
          </Empty>
        ) : (
          goal && (
            <div className="flex flex-col gap-5">
              <div role="radiogroup" aria-label="Goal" className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {goals!.map((g) => {
                  const on = g.id === goal.id;
                  return (
                    <button
                      key={g.id}
                      type="button"
                      role="radio"
                      aria-checked={on}
                      onClick={() => setGoalId(g.id)}
                      className={cx(
                        "flex flex-col gap-2 rounded-lg border p-3 text-left transition-colors",
                        on ? "border-accent bg-accent/10" : "border-line-strong bg-surface-2 hover:border-line-3",
                      )}
                    >
                      <span className="flex items-center gap-2">
                        <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink">{g.title}</span>
                        <span className="text-xs text-ink-subtle">by {formatDate(g.due_day, false)}</span>
                      </span>
                      <GoalMeter g={g} today={d.today()} height={4} />
                      <span className="text-xs text-ink-subtle tabular-nums">{goalAmount(g)}</span>
                    </button>
                  );
                })}
              </div>
              <BreakdownFlow key={goal.id} goal={goal} />
            </div>
          )
        )}
      </Panel>

      <Panel title="Weekly retro" icon={ScrollText}>
        <RetroFlow />
      </Panel>
    </div>
  );
}

/** Which provider answers, and whether it can right now. */
function ProviderStatus() {
  const d = useDaemon();
  const { go } = useNav();
  const llm = d.config?.llm;
  const [claude, setClaude] = useState<main.ClaudeCodeStatus | null>(null);
  const [hasKey, setHasKey] = useState<boolean | null>(null);

  useEffect(() => {
    setClaude(null);
    if (llm?.provider === "claude_code") App.ClaudeCode(llm.command).then(setClaude, () => setClaude(null));
    if (llm?.provider === "anthropic" || llm?.provider === "openai_compatible") App.HasCredential("llm_api_key").then(setHasKey, () => setHasKey(null));
  }, [llm?.provider, llm?.command]);

  if (!llm) return null;
  const settings = (fix: boolean) => (
    <Button size="sm" tone={fix ? "primary" : "secondary"} icon={SettingsIcon} onClick={() => go("settings", "ai")}>
      {fix ? "Fix in settings" : "AI settings"}
    </Button>
  );

  if (llm.provider === "none")
    return (
      <Bar icon={<Sparkles size={18} className="text-accent-hover" />} title="No AI provider yet" action={<Button tone="primary" size="sm" onClick={() => go("settings", "ai")}>Set up AI</Button>}>
        Use your Claude subscription through Claude Code, an Anthropic API key, or a local model such as Ollama.
      </Bar>
    );

  if (llm.provider === "claude_code") {
    const state: [ReactNode, string, string] = !claude
      ? [<LoaderCircle size={14} className="animate-spin" />, "Checking Claude Code…", "text-ink-subtle"]
      : claude.is_claude && claude.logged_in
        ? [<CircleCheck size={14} />, claude.plan ? `Signed in with your Claude ${planName(claude.plan)} plan` : "Signed in", "text-working"]
        : claude.is_claude
          ? [<CircleAlert size={14} />, "Claude Code is not signed in", "text-idle"]
          : [<CircleAlert size={14} />, claude.found ? `${claude.command} is not Claude Code` : `Claude Code was not found at ${claude.command}`, "text-danger"];
    return (
      <Bar icon={<Sparkles size={18} className="text-accent-hover" />} title={`Claude Code${llm.model ? `, ${llm.model}` : ""}`} action={settings(!!claude && !(claude.is_claude && claude.logged_in))}>
        <span className={cx("inline-flex items-center gap-1.5", state[2])}>
          {state[0]}
          {state[1]}
        </span>
      </Bar>
    );
  }

  const name = llm.provider === "anthropic" ? "Anthropic API" : "OpenAI-compatible API";
  return (
    <Bar icon={<KeyRound size={18} className="text-accent-hover" />} title={`${name}${llm.model ? `, ${llm.model}` : ""}`} action={settings(llm.provider === "anthropic" && hasKey === false)}>
      {llm.provider === "openai_compatible" && <span className="mr-3">{llm.endpoint || "No endpoint set"}</span>}
      {hasKey === false && llm.provider === "anthropic" ? (
        <span className="inline-flex items-center gap-1.5 text-idle">
          <CircleAlert size={14} />
          No API key saved
        </span>
      ) : (
        hasKey && (
          <span className="inline-flex items-center gap-1.5 text-working">
            <CircleCheck size={14} />
            API key saved
          </span>
        )
      )}
    </Bar>
  );
}

function Bar({ icon, title, action, children }: { icon: ReactNode; title: string; action: ReactNode; children: ReactNode }) {
  return (
    <div className="lift flex flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border border-line bg-surface-1 px-5 py-4">
      <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-accent/12">{icon}</span>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold text-ink">{title}</div>
        <div className="mt-0.5 text-[13px] text-ink-subtle">{children}</div>
      </div>
      {action}
    </div>
  );
}
