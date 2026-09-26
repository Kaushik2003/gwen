# Product

## What Gwen is

A personal time-tracking and study-planning app for Linux desktops. It answers three questions for
one person, honestly, every day:

1. **How much did I actually work today, and on what?** Not "how long was the app open" — how long
   was I genuinely at the keyboard, attributed to a project.
2. **What should I be doing right now?** Derived from real deadlines and real remaining capacity,
   not a wish list.
3. **Am I on pace for the things I committed to?** "300 DSA problems in 3 months" is a rate, and
   the app knows whether you are meeting it.

The motivating situation: the user works an internship roughly 5 hours a day, wants to put in 8+
productive hours total across several projects, takes breaks throughout, and is running long-horizon
self-set challenges alongside. Nothing off the shelf both tracks the clock honestly *and* plans
against goals, so Gwen does both against one source of truth.

## Principles

**Honest by default.** Idle time is not work. The app would rather under-count than flatter you —
that is the entire reason [retroactive reclaim](05-time-engine.md#retroactive-reclaim) exists. A
tracker that inflates your hours is worse than no tracker.

**Never lose data to a crash.** A killed daemon, a suspend, or a reboot must never produce phantom
hours and must never silently drop a segment. See
[05-time-engine.md](05-time-engine.md#crash-recovery).

**Works offline, always.** No network, no account, no API key is required for anything in v1 or v2.
Cloud pieces — phone push, calendar, LLM, sync — are strictly additive and every one of them
degrades to a working local app when unavailable.

**Light enough to forget about.** The always-on daemon must stay under 40 MB RSS. The GUI is
launched on demand and closing it changes nothing about tracking.

**Local-first.** Your data is a SQLite file you own and can copy. The sync hub is a replica, never
the authority.

## Users

There is one primary user: the person who installed it, on their own machine. Gwen is single-user
and single-tenant by design — there is no sharing, no team view, no multi-account support, and the
data model does not carry an owner column.

Secondary consideration: the app should be installable by a non-technical Linux user via a package
manager, with a `gwen setup` wizard doing the rest. See [09-packaging.md](09-packaging.md).

## User stories

### v1 — tracking, CRUD, stats

- Clock in for the day and see a running timer attributed to a project.
- Switch the active project or task without stopping the clock.
- Step away briefly and come back to find no time lost.
- Step away for a long time and come back to find the app correctly stopped counting *at the moment
  I stopped typing*, not when I returned, and started a break instead.
- Get a desktop notification when I have been idle a few minutes, with buttons to say "I'm back",
  "start a break", or "snooze".
- Get that same nudge on my phone when I have walked away from the machine.
- Manually start and end a break.
- Lock my screen and have it counted as a break immediately, with no grace period.
- Clock out and see the day's totals: worked, on break, per project, versus my 8-hour target.
- Create, rename, archive, and colour projects.
- Create tasks under a project, mark them done, and see time attributed to them.
- Fix the record afterwards: edit a segment's times or project, split one, delete one, or add a
  segment I forgot to track.
- See a dashboard: today at a glance, a week view, per-project breakdown, a year heatmap, and
  streaks.

### v2 — planner

- Define a goal with a target and a deadline ("300 DSA problems by 2026-12-12").
- Declare fixed commitments ("internship, 5h, weekdays").
- Get a daily plan that fits real remaining capacity, ordered by urgency.
- See at clock-in what is still pending from yesterday, carried forward automatically.
- Be told a task is due soon and should start now, before it becomes a problem.
- See whether each goal is ahead of or behind its required pace.
- Define recurring tasks ("contribute to open source, weekly").

### v3 — calendar, AI, sync

- Have my plan appear as blocks in Google Calendar, and have Gwen avoid double-booking over events
  already there.
- Hand Gwen a big goal in a sentence and get a proposed task breakdown I can edit and accept.
- Get a written end-of-week retrospective on where the time actually went.
- Open the dashboard from my phone, and keep my history when I reinstall my laptop.

## Non-goals

Stated so no agent builds them:

- **Multi-user, teams, sharing, or an employer-facing view.** This is a personal tool. Nothing about
  surveillance-by-someone-else is in scope.
- **Screenshots, keystroke logging, or window-title capture.** The activity signal is
  *input-or-no-input*, nothing more. Reading what the user is typing or which app they are in is
  out of scope permanently, not just for v1.
- **Mobile or Windows/macOS apps.** Linux desktop only. The phone's role is receiving push
  notifications and, in v3, viewing a read-only dashboard in a browser.
- **Pomodoro enforcement.** Gwen reports and nudges; it never blocks apps or locks you out.
- **Invoicing, billing, hourly rates, or client reporting.**
- **A general-purpose chat assistant.** The LLM has two narrow jobs, both defined in
  [07-integrations.md](07-integrations.md#llm-adapter).

## Glossary

Used precisely and consistently throughout these docs and in code.

| Term | Meaning |
|---|---|
| **Work day** | One local calendar day that has been clocked into. Owns all segments for that day. |
| **Segment** | A contiguous interval of a single kind (`work`, `break_auto`, `break_manual`). The atomic unit of time. All totals are sums over segments. |
| **Open segment** | A segment with no `ended_at`. At most one exists at a time. |
| **Clock in / out** | Starting and ending a work day. Distinct from starting and ending a break. |
| **Activity** | Keyboard or pointer input reaching the compositor. The only activity signal Gwen uses. |
| **Idle** | No activity for some duration. |
| **Soft threshold** | Idle duration that triggers a nudge but still counts as work. |
| **Hard threshold** | Idle duration that converts the idle period into a break retroactively. |
| **Grace period** | The span between soft and hard thresholds. Returning within it costs nothing. |
| **Retroactive reclaim** | Rewriting the open work segment's end back to the instant input stopped, when the hard threshold is crossed. |
| **Nudge** | A notification prompting a return to work. |
| **Goal** | A long-horizon target with a quantity and a deadline. |
| **Task** | A unit of work, optionally under a goal and a project. |
| **Commitment** | A recurring fixed block of time that consumes capacity before planning. |
| **Plan item** | A task scheduled to a specific day by the planner. |
| **Capacity** | Minutes available for planned work on a day, after commitments and buffer. |
| **Rollover** | Carrying an unfinished plan item forward to a later day. |

## Phases

Each phase is independently useful and shippable. Package contents are in
[10-build-plan.md](10-build-plan.md).

**v1 — Tracking.** The daily driver. Clock in/out, idle detection, automatic breaks, desktop and
phone notifications, projects and tasks CRUD, segment editing, stats dashboard, CLI, tray, and
installable packaging. After v1 the app is genuinely used every day; everything after is additive.

**v2 — Planner.** Goals, commitments, deterministic daily scheduling, rollover, urgency ranking,
recurrence. No network, no AI.

**v3 — Integrations.** Google Calendar two-way sync, the LLM adapter, and the Raspberry Pi sync
hub with a phone-accessible dashboard.
