import { CalendarClock, CircleAlert, CircleCheck, Eye, KeyRound, LoaderCircle, ScrollText, Settings as SettingsIcon, Sparkles, Target } from "lucide-react";
import { useEffect, useId, useState, type ReactNode } from "react";
import { App, wire } from "../api";
import { RetroFlow, planName } from "../components/ai";
import Breakdown from "../components/Breakdown";
import { goalAmount } from "../components/goal";
import { Button, IconButton, Modal, cx } from "../components/ui";
import { useDaemon } from "../daemon";
import { formatDate } from "../format";
import { useNav } from "../nav";
import { Card, StateGlyph } from "../shell/bits";
import { useTabs } from "../shell/tabs";
import { useWork } from "../shell/work";
import type { main } from "../wailsjs/go/models";

/**
 * With Gwen: beside the chat, the AI she runs on and the jobs that have their own flow — plan your day,
 * break a goal into tasks, the weekly retro — and the sessions she watches. Small cards, each sized by
 * what it holds, in one column beside the chat or two under it; the long flows open over the screen.
 */
export default function WithGwen({ className }: { className?: string }) {
  return (
    <aside aria-label="With Gwen" className={cx("min-w-0 gap-3 @3xl:columns-2 @5xl:columns-1 [&>*]:mb-3 [&>*]:break-inside-avoid [&>*:last-child]:mb-0", className)}>
      <ProviderCard />
      <PlanCard />
      <BreakdownCard />
      <RetroCard />
      <WatchingCard />
    </aside>
  );
}

const say = "text-[12.5px] leading-relaxed text-ink-subtle";

/** Which provider answers and whether it can right now; the way to its settings, offered first when something needs fixing. */
function ProviderCard() {
  const d = useDaemon();
  const { go } = useNav();
  const llm = d.config?.llm;
  const [claude, setClaude] = useState<main.ClaudeCodeStatus | null>(null);
  const [hasKey, setHasKey] = useState<boolean | null>(null);

  useEffect(() => {
    setClaude(null);
    setHasKey(null);
    if (llm?.provider === "claude_code") App.ClaudeCode(llm.command).then(setClaude, () => setClaude(null));
    if (llm?.provider === "anthropic" || llm?.provider === "openai_compatible") App.HasCredential("llm_api_key").then(setHasKey, () => setHasKey(null));
  }, [llm?.provider, llm?.command]);

  const settings = () => go("settings", "ai");
  if (!llm)
    return (
      <Card title="AI" icon={Sparkles}>
        <Pending>Reading your AI settings…</Pending>
      </Card>
    );

  if (llm.provider === "none")
    return (
      <Card title="No AI provider yet" icon={Sparkles}>
        <p className={say}>Use your Claude subscription through Claude Code, an Anthropic API key, or a local model such as Ollama.</p>
        <Button size="sm" tone="primary" icon={SettingsIcon} className="mt-3" onClick={settings}>
          Set up AI
        </Button>
      </Card>
    );

  let state: ReactNode;
  let fix = false;
  if (llm.provider === "claude_code") {
    fix = !!claude && !(claude.is_claude && claude.logged_in);
    state = !claude ? (
      <Pending>Checking Claude Code…</Pending>
    ) : claude.is_claude && claude.logged_in ? (
      <Status ok>{claude.plan ? `Signed in with your Claude ${planName(claude.plan)} plan` : "Signed in"}</Status>
    ) : claude.is_claude ? (
      <Status>Claude Code is not signed in</Status>
    ) : (
      <Status danger>{claude.found ? `${claude.command} is not Claude Code` : `Claude Code was not found at ${claude.command}`}</Status>
    );
  } else {
    fix = llm.provider === "anthropic" && hasKey === false;
    state = (
      <div className="flex flex-col gap-1">
        {llm.provider === "openai_compatible" &&
          (llm.endpoint ? <p className="truncate font-mono text-[12px] text-ink-muted">{llm.endpoint}</p> : <p className="text-[12.5px] text-idle">No endpoint set</p>)}
        {hasKey === null ? (
          <Pending>Checking for an API key…</Pending>
        ) : hasKey ? (
          <Status ok>API key saved</Status>
        ) : (
          llm.provider === "anthropic" && <Status>No API key saved</Status>
        )}
      </div>
    );
  }
  // The card's title is the provider; its model goes under it, so a long one never pushes Settings out of the header.
  return (
    <Card title={providerKind[llm.provider] ?? "AI"} icon={llm.provider === "claude_code" ? Sparkles : KeyRound} actions={!fix && <IconButton size="sm" icon={SettingsIcon} label="AI settings" onClick={settings} />}>
      {llm.model && <p className="mb-1 truncate font-mono text-[12px] text-ink-muted">{llm.model}</p>}
      {state}
      {fix && (
        <Button size="sm" tone="primary" icon={SettingsIcon} className="mt-3" onClick={settings}>
          Fix in settings
        </Button>
      )}
    </Card>
  );
}

const providerKind: Record<string, string> = { claude_code: "Claude Code", anthropic: "Anthropic API", openai_compatible: "OpenAI-compatible API" };

function Status({ ok, danger, children }: { ok?: boolean; danger?: boolean; children: ReactNode }) {
  return (
    <p className={cx("flex items-start gap-1.5 text-[12.5px] leading-relaxed", ok ? "text-ink-muted" : danger ? "text-danger" : "text-idle")}>
      {ok ? <CircleCheck size={13} className="mt-[3px] shrink-0 text-success" aria-hidden /> : <CircleAlert size={13} className="mt-[3px] shrink-0" aria-hidden />}
      <span className="min-w-0">{children}</span>
    </p>
  );
}

function Pending({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-center gap-2 text-[12.5px] text-ink-subtle" role="status">
      <LoaderCircle size={13} className="animate-spin" aria-hidden />
      {children}
    </p>
  );
}

/** Plan your day: talked through on Plan › Day, where the blocks show on the timeline. */
function PlanCard() {
  const { go } = useNav();
  return (
    <Card
      title="Plan your day"
      icon={CalendarClock}
      actions={
        <Button size="sm" icon={Sparkles} onClick={() => go("plan", "chat")}>
          Plan today
        </Button>
      }
    >
      <p className={say}>She fits your tasks around your calendar and your hours, and you apply what you like.</p>
    </Card>
  );
}

/** Break a goal into tasks: pick an active goal here; the breakdown, with its options, opens over the screen. */
function BreakdownCard() {
  const d = useDaemon();
  const { go } = useNav();
  const group = useId();
  const [goals, setGoals] = useState<wire.Goal[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [goalId, setGoalId] = useState<string | null>(null);
  const [open, setOpen] = useState<wire.Goal | null>(null);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    let live = true;
    App.ListGoals("active").then(
      (l) => {
        if (!live) return;
        setGoals(l.goals);
        setFailed(false);
      },
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [d.goalsVersion, d.tasksVersion, retry]);

  const goal = goals?.find((g) => g.id === goalId) ?? goals?.[0] ?? null;
  return (
    <Card title="Break a goal into tasks" icon={Target}>
      {goals === null ? (
        failed ? (
          <div className="flex items-center gap-3">
            <p className={cx(say, "flex-1")}>Couldn't load your goals.</p>
            <Button size="sm" onClick={() => setRetry((n) => n + 1)}>
              Try again
            </Button>
          </div>
        ) : (
          <Pending>Loading your goals…</Pending>
        )
      ) : goals.length === 0 || !goal ? (
        <>
          <p className={say}>No active goals yet. Set one, and she breaks it into tasks you pick from.</p>
          <Button size="sm" icon={Target} className="mt-3" onClick={() => go("goals")}>
            Create a goal
          </Button>
        </>
      ) : (
        <>
          <div role="radiogroup" aria-label="Goal to break down" className="-mx-1.5 flex max-h-48 flex-col gap-0.5 overflow-y-auto">
            {goals.map((g) => (
              <label key={g.id} className={cx("flex cursor-pointer items-start gap-2.5 rounded-lg px-1.5 py-1.5 transition-colors", g.id === goal.id ? "bg-surface-2" : "hover:bg-surface-2/60")}>
                <input type="radio" name={group} checked={g.id === goal.id} onChange={() => setGoalId(g.id)} className="mt-[3px] shrink-0 accent-accent" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] text-ink">{g.title}</span>
                  <span className="block truncate text-[11.5px] text-ink-subtle tabular-nums">
                    {goalAmount(g)}
                    {g.due_day && ` · by ${formatDate(g.due_day, false)}`}
                  </span>
                </span>
              </label>
            ))}
          </div>
          <Button size="sm" icon={Sparkles} className="mt-2.5" onClick={() => setOpen(goal)}>
            Break it down
          </Button>
        </>
      )}
      {open && <Breakdown goal={open} onClose={() => setOpen(null)} />}
    </Card>
  );
}

/** The weekly retro, written from a tracked week; picking the week and writing it open over the screen. */
function RetroCard() {
  const [open, setOpen] = useState(false);
  return (
    <Card
      title="Weekly retro"
      icon={ScrollText}
      actions={
        <Button size="sm" icon={ScrollText} onClick={() => setOpen(true)}>
          Write a retro
        </Button>
      }
    >
      <p className={say}>What went well, what slipped, and what to try next, from your tracked week.</p>
      {open && (
        <Modal title="Weekly retro" size="lg" onClose={() => setOpen(false)}>
          <RetroFlow />
        </Modal>
      )}
    </Card>
  );
}

/** How much a session wants you, first to last. */
function urgency(s: wire.CodeSession): number {
  if (s.state === "needs_you") return 0;
  if (s.state === "error") return 1;
  if (s.state === "working" || s.state === "starting") return 2;
  if (s.state === "done") return s.unread ? 3 : 4;
  return 5;
}

/** A session's state in words. */
function stateWords(s: wire.CodeSession): string {
  switch (s.state) {
    case "needs_you":
      return "needs you";
    case "error":
      return "stopped with an error";
    case "working":
      return "is working";
    case "starting":
      return "is starting";
    case "done":
      return "finished";
    default:
      return "is waiting for you";
  }
}

/** She's watching: the Claude sessions running now, the one that needs you first. Each opens its terminal. */
function WatchingCard() {
  const d = useDaemon();
  const w = useWork();
  const tabs = useTabs();
  const live = w.sessions.filter((s) => s.state !== "ended").sort((a, b) => urgency(a) - urgency(b) || b.updated_at - a.updated_at);
  const shown = live.slice(0, 6);
  return (
    <Card title="She's watching" icon={Eye} count={live.length} bodyClassName="">
      {live.length === 0 ? (
        <p className={cx(say, "px-4 py-3")}>No sessions are running. When Claude works in one of your projects, she follows it here and tells you when it needs you.</p>
      ) : (
        <ul>
          {shown.map((s, i) => {
            const project = w.projectOf(s.cwd);
            const where = d.projects.find((p) => p.id === project)?.name;
            const what = s.name || where || "A session";
            return (
              <li key={s.id} className={cx(i > 0 && "border-t border-chrome-line")}>
                <button
                  type="button"
                  disabled={!s.terminal}
                  title={where && s.name ? `${s.name} in ${where}` : what}
                  onClick={() => tabs.open({ kind: "term", project: project ?? "", terminal: s.terminal })}
                  className="flex w-full items-center gap-2.5 px-4 py-2.5 text-left transition-colors hover:bg-surface-2/60 disabled:cursor-default disabled:hover:bg-transparent"
                >
                  <span className="grid w-3.5 shrink-0 place-items-center">
                    {s.state === "idle" ? <span title="Waiting" className="size-[7px] rounded-full border border-ink-faint" /> : <StateGlyph state={s.state} unread={s.unread} />}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-[13px] text-ink-muted">
                    <span className="text-ink">{what}</span> {stateWords(s)}
                  </span>
                </button>
              </li>
            );
          })}
          {live.length > shown.length && <li className="border-t border-chrome-line px-4 py-2 text-[12px] text-ink-subtle">and {live.length - shown.length} more</li>}
        </ul>
      )}
    </Card>
  );
}
