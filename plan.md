# Gwen plan: from tracker to assistant

Status: ready for W15 to W23. Last updated 2026-10-10.

This file is the single plan for Gwen's next phase. It replaces `wish.md` and the `docs/` folder that was
deleted in commit `6879749` (still readable with `git show 6879749^:docs/<file>`). Work packages edit this
file in place when a decision changes. Do not add other planning files.

Contents: [1 Goals](#1-what-you-asked-for) · [2 Today's code](#2-where-gwen-is-today) ·
[3 Architecture](#3-architecture) · [4 Products](#4-products-and-services) · [5 UI and UX](#5-ui-and-ux) ·
[6 Implementation](#6-implementation-plan) · [7 Safety](#7-safety-and-privacy-rules)

---

## 1. What you asked for

| You asked for | What we build | Package |
|---|---|---|
| Google Calendar appointments show in the timeline as tasks | **Agenda**: one list of calendar events, plan blocks, and commitments, each with a checkbox | W17 |
| A notification 5 minutes before something starts | **Cues**: reminders before, at, and after the start, that survive restarts and sleep | W18 |
| Gwen talks to you first and tells you to start | **Presence**: a small card that speaks, with Start, Later, Skip, and Talk | W20 |
| Use the new design file, change the colours, better UX | One design system, three new palettes, a calmer layout with 8 screens | W16, W21 |
| A harness Gwen can use to do tasks for you | **Runs**: steps, a log, approvals, and permission checks inside the daemon | W19, W21 |
| DSA: notify, then open the browser with the problem links | **Routines**: "when it starts" steps on any task or event | W19, W21 |
| Read my last 5 emails, suggest replies, I send or Gwen sends | **Email review** run with Gmail, drafts, and send-on-click | W22 |
| Coding tasks: discuss the plan, file paths, which folder is which project | **Coding run** with Claude Code, project folders, and a git worktree per run | W23 |
| Connect more apps later without redesigning | **Connectors** with typed capabilities, and MCP for the long tail | W19, W24 |

The idea that ties these together: Gwen should not only remind you. She should know what is next, help you
start it, prepare what you need, and later do parts of the work with your OK. One timeline feeds one
reminder engine, which feeds one way of reaching you, which can start one kind of run. Each new app is a
connector that plugs into that chain.

---

## 2. Where Gwen is today

### What already works and gets reused

- **Daemon.** `gwend` owns the SQLite database, serves HTTP and Server-Sent Events on its Unix socket, and
  runs timed work on an injected `Clock` (for example `keepPlan` every 15 minutes, `cmd/gwend/keeper.go`).
- **Calendar sync.** Every 10 minutes Gwen reads the next 14 days of your chosen calendars into
  `calendar_busy` and writes your plan to a calendar of its own (`internal/gcal/sync.go`).
- **Notifications.** Desktop notifications go over D-Bus with buttons, and clicks come back as
  `ActionInvoked` (`internal/notify/desktop.go:93`). Phone pushes go through ntfy. But the only nudges are
  idle, long break, typing during a break, and clock in (`internal/timeengine/nudges.go`).
- **Voice.** `gwen-ui` hosts speech in and out. It runs as separate processes per window: the dashboard,
  `--now` (now card), and `--talk` (`cmd/gwen-ui/main.go`). The KDE panel widget polls `gwen panel` every
  3 seconds and launches these windows (`packaging/plasma/dev.gwen.panel/contents/ui/main.qml:119`). Voice
  models are large (Kokoro 354 MB, Parakeet 131 MB) and are freed after 3 minutes without use.
- **LLM.** One `Planner` interface with four jobs (`internal/llm/llm.go:19`). Each call returns one JSON reply
  checked against a schema. The assistant can make 8 kinds of change, which the daemon checks and applies.
  Claude Code is called with every tool turned off (`--tools ""`, `internal/llm/claudecode.go:81`), so it
  acts as a plain model today, not as an agent.
- **UI tokens.** All colours are CSS variables in one `@theme` block in `ui/src/index.css`. Only 9 files
  use hard-coded hex values. A new palette is mostly a one-file change.

### Why your appointments do not show as tasks

1. **Sync works.** Your database holds 18 events from your main calendar between 5 and 23 October, and 23
   October is the end of the 14 day window, so the last sync finished. The second id in `busy_calendars` is
   either your main calendar under its address (sync counts each event once on purpose,
   `internal/gcal/sync.go:331`) or a calendar with no events in the window. That part is fine.
2. **Events are stored only as busy time.** They flow from `calendar_busy` to `Plan.Events`
   (`internal/store/plan.go:961`) to `eventBlocks` (`ui/src/plan.ts:69`) and end up as a grey, hatched block
   in the timeline's Plan lane (`ui/src/components/Timeline.tsx:99`). Your daily 13:30 to 14:00 event is a
   30 minute sliver on a 14 hour lane with 11 px text.
3. **Everything else reads plan items only.** Today's plan list (`ui/src/screens/Today.tsx:107`), the now card
   (`ui/src/screens/NowCard.tsx:61`), the panel's next block (`cmd/gwen/panel.go`), the briefing, and the
   nudges ignore events. So an appointment gets no row, no checkbox, no reminder, and is never "next up".
4. **Some events are dropped.** Events outside the planner window (`day_start` to `day_end`) are cut off, and
   all-day events marked "free" (birthdays, holidays) are skipped.

W17 fixes this by making calendar events first-class entries of the agenda.

### Fix before new work starts

- **`make check` fails.** The `docs-check` target calls `scripts/docs-check.sh`, which was deleted with
  `docs/`. CI runs `make check`, so no package could pass its definition of done. (W15)
- **Two design files** that differ only in letter case: `ui/DESIGN.md` (the Linear-style system in use) and
  `ui/design.md` (the new workshop system). That breaks checkouts on macOS and Windows and confuses agents.
  They merge into one `ui/DESIGN.md`. (W16)
- **Stale links.** 108 source files point at `docs/*.md` files that no longer exist. No sweep: each package
  fixes or removes the stale links in the files it edits anyway.
- **`wish.md`** is superseded by this file. It is untracked, so delete it when you are ready.
- **Google client secret** sits in the repo root (git-ignored). W22 moves Google secrets and tokens into the
  system secret store.

---

## 3. Architecture

### 3.1 Principles

1. **One core, many surfaces.** `gwend` owns state, scheduling, permissions, and execution. The dashboard,
   panel, tray, CLI, and cue card only show things and send requests.
2. **Pure decisions, thin edges.** Like the time engine and the planner, the agenda builder, the cue engine,
   and the permission policy are pure functions of their inputs and the clock. Adapters do the I/O. That
   keeps them testable with `clock.NewFake` and golden tests.
3. **The model proposes, Gwen checks, you decide.** Model output is data. It never calls a tool directly,
   except inside a Claude Code run, where every permission request goes back through Gwen.
4. **Every side effect is a typed capability** with a risk level, checked by the daemon at the moment of the
   call, never only in the UI or in a prompt.
5. **Everything is recorded.** Cues and runs keep a log you can read in the dashboard.
6. **Light when idle.** No new always-on process. Windows open for a cue or a run and close again.

### 3.2 The five layers

```text
 sources     Google Calendar     Gwen tasks and plan     commitments     (later: email labels, issues)
                    \                     |                    /
 1 Agenda        one timeline of entries; every entry has a subject ref           internal/agenda
                                          |
 2 Cues          when to remind: lead, start, follow-up; deduped in cue_log       internal/cue
                                          |
 3 Presence      how to reach you: notification, phone, spoken cue card           gwen-ui --cue
                                          |
 4 Harness       runs: routine, email review, coding; steps, approvals, log       internal/harness
                                          |
 5 Connectors    typed capabilities: browser.open, gmail.*, code.*, mcp.*          internal/connect
```

The cue engine decides **when** something is due. Presence decides **how** to tell you. Gwen's persona
decides **what** to say. None of them can approve an action. Only you can, in the daemon's approval flow.

### 3.3 Subjects: one way to point at things

Reminders, links, routines, and runs attach to a **subject**. A subject ref is a string:

| Ref | Example | Notes |
|---|---|---|
| `task:<uuid>` | `task:5f0c...` | a Gwen task; an occurrence inherits from its template (`tasks.template_id`) |
| `event:<calendar_id>/<event_id>` | `event:primary/abc_20261010T080000Z` | one instance of a calendar event |
| `series:<calendar_id>/<recurring_event_id>` | `series:primary/abc` | every instance of a repeating event |
| `commitment:<uuid>@<YYYY-MM-DD>` | `commitment:91d2...@2026-10-10` | one day of a commitment |
| `goal:<uuid>` | `goal:7aa1...` | a goal, for runs started from it |

New package `internal/subject` parses and prints refs (`Parse`, `Ref.String`, `Ref.Kind`, `Ref.Parent`).
Lookups always try the instance first, then its parent: an event instance falls back to its series, a task
occurrence to its template, a commitment day to the commitment. So "DSA every day at 19:00" is set up once.
Refs go in query strings URL-encoded, never in URL paths.

### 3.4 Agenda (W17)

The agenda is a read model, built on request by a pure function in `internal/agenda`:

```go
func Build(day civil.Day, in Input) []Entry // plan entries + calendar events + commitments + marks, by start
```

An `Entry` has: `ref`, `kind` (`event`, `block`, `commitment`), `title`, `start_at`, `end_at`, `all_day`,
`source` (calendar name or project name), `state` (`upcoming`, `now`, `done`, `skipped`, `missed`),
`task_id` (blocks only), `link` (the event's Google Calendar page), and `has_routine`.

- **Storage.** `calendar_busy` becomes `calendar_events` and keeps more of each event (section 3.9). It stays
  a device-local cache: each device reads Google itself and it never syncs to the hub.
- **Which events.** The calendars in `busy_calendars` (shown in the UI as "Calendars on your agenda"). Timed
  events always show and always count as busy, as today. All-day events always show in an all-day strip and
  count as busy only when marked busy. Cancelled events and invitations you declined never show.
- **Checking things off.** Blocks use the task's own status. Events and commitments use `agenda_marks`
  (`done` or `skipped`), which syncs between devices because event ids are the same on every device.
- **The planner does not change.** It reads `calendar_events` rows with `busy = 1`, exactly as it reads
  `calendar_busy` now.
- **Where it shows.** Today's list (events and blocks in time order, with checkboxes), the timeline (events as
  solid blocks with a title and a minimum width of 8 px, plus the all-day strip), the now card, the panel's
  "next", and the briefing.

### 3.5 Cues (W18)

A cue is a reminder tied to an entry's start time. Three kinds:

| Kind | When | Applies to | Default |
|---|---|---|---|
| `lead` | before the start | all timed entries | 5 minutes before |
| `start` | at the start | all timed entries | on |
| `follow_up` | after the start, if you have not started | blocks and entries with a routine | 10 minutes after, once |

- **Rules.** Defaults come from the `[cues]` config section. Overrides per subject live in `cue_rules` and
  inherit like everything else (instance, then series or template).
- **Engine.** `internal/cue` is pure:
  `func Due(in Input) (due []Cue, next time.Time)`. Input: agenda entries from `now - grace` to `now + 36h`,
  rules, config, the ids already in `cue_log`, the started set, presence facts, and `now`.
- **Stable ids.** A cue id is a UUIDv5 (`uuid.NewSHA1`, already a dependency) of `<subject ref>|<start_at ms>|<kind>`. If an event moves, its start
  changes, so it gets a new cue and the old one expires. Restarts and syncs never duplicate a cue.
- **Loop.** `cmd/gwend/cues.go` arms one clock timer at `next`. It also re-evaluates after `plan_changed`,
  `agenda_changed`, `config_changed`, a resume from suspend, and every 5 minutes as a safety net.
- **Delivery.** Insert the cue into `cue_log` first (`INSERT OR IGNORE`), then hand it to presence. If Gwen
  was off or asleep, a cue up to `grace` (10 minutes) late is still shown and marked late. Older ones are
  logged as `missed` and the entry shows "missed" on Today.
- **Actions.** `start`, `snooze` (default 5 minutes, then the cue is due again), `skip` (marks the entry
  skipped for that day), `open` (opens the dashboard on the entry).
- **Started means** tracking is on the entry's task, or the entry is done, or a run for it is running, or you
  pressed Start. Started, done, and skipped entries get no more cues.
- **Start does real things.** For a block it switches tracking to the task. For an event it records the start.
  If the subject has a routine, it also starts a routine run (W19 registers that as a start hook).

### 3.6 Presence: how Gwen reaches you (W18 and W20)

| Situation | What happens |
|---|---|
| At the desk, active, speech allowed | desktop notification with buttons, and the cue card opens and speaks |
| At the desk, but Do Not Disturb, speech off, or another event is in progress | desktop notification only |
| Away (clocked out, on a break, or idle past `soft_idle`) | phone push through ntfy, and the desktop notification waits |
| Quiet hours | no sound or speech; lead and start cues for calendar events still show silently; follow-ups are dropped |

- Do Not Disturb comes from the `Inhibited` property of `org.freedesktop.Notifications`. Where the
  notification server does not offer that property, Gwen treats Do Not Disturb as off.
- At most one spoken cue every 10 minutes. Extra cues inside that window are shown, not spoken.
- **Who opens the card.** `gwend` never launches windows. It runs as a systemd service and also runs headless on
  the Pi. Instead, `gwen panel` output gets a `cue` field, and the KDE panel widget (or `gwen-tray`, on other
  desktops) runs `gwen-ui --cue <id>` once per new cue. Without either, you still get the notification.
- **The cue card** (`gwen-ui --cue`): a 360 by 200 frameless window at the bottom right, always on top, never
  taking keyboard focus. It shows Gwen's sprite, one line, and buttons **Start**, **5 min**, **Skip**, and
  **Talk**. It speaks the line with your chosen voice and closes after an action or after 2 minutes. Talk
  opens the talk window with the cue as the first message, so you can say "move it to 8" or "give me a hint".
- **What she says.** Every cue carries a short template line (`Cue.Text`, fast and offline), for example
  "DSA starts in 5 minutes." and "It's 7. Time for DSA. Want me to open today's problem?". The notification
  uses it too (W18). With `personal_lines = true`, the LLM writes the spoken line in Gwen's current attitude,
  with a 4 second limit and the template as fallback (W20).

### 3.7 Harness: runs (W19)

A **run** is one piece of work Gwen does with you. Run kinds plug into a registry:

```go
type Kind interface {
	Name() string                                         // "routine", "email_review", "code"
	Begin(ctx context.Context, r Run) ([]StepSpec, error) // the first steps; a step can add more
	Step(ctx context.Context, r Run, s Step, x Exec) (StepResult, error)
}

// Exec is the only way a kind touches the world.
type Exec interface {
	Invoke(ctx context.Context, capability string, input any) (json.RawMessage, error) // policy checked here
	Ask(ctx context.Context, a ApprovalRequest) (Decision, error)                       // the run waits for you
	Log(kind string, data any)                                                          // the run's event log
}
```

**Run states** (`runs.status`), with the only allowed moves:

| From | To |
|---|---|
| `draft` | `running`, `cancelled` |
| `running` | `waiting`, `done`, `failed`, `cancelled` |
| `waiting` (approval, your reply, or interrupted) | `running`, `cancelled` |

Steps go `pending`, `running`, then `done`, `failed`, `skipped`, or `waiting`.

- **Restarts.** On start, `gwend` moves every `running` run to `waiting` with the reason `interrupted`. A step
  with a side effect is never re-run on its own. You press Resume, and the kind decides how to continue.
- **Limits.** At most 2 runs execute at once; others wait in `draft` with the reason `queued`. Cancel stops the
  context, and a Claude Code process gets SIGINT.

**Permission policy** (`internal/connect/policy.go`, pure):

| Risk | Examples | Default |
|---|---|---|
| `read` | list emails, read a calendar | allowed |
| `local` | open a URL or folder, start tracking, speak | allowed |
| `write` | create a Gmail draft, edit files in a run's worktree | ask once per run |
| `external` | send an email, push code, post anywhere | ask every time, cannot be pre-approved |
| `destructive` | delete mail or files | not offered in this phase |

`grants` can relax `read`, `local`, and `write` for one subject or project folder. Nothing can relax
`external`. Approvals happen only by a click or key press in the dashboard or on a card. Voice can start
things but never approves a send.

**Approvals.** A step calls `Ask`, which stores an `approvals` row with a plain summary and the exact payload
(recipients, subject, and body for an email; a command for a tool; a change summary for code). Gwen sends
`approval_requested`, shows it on the Work screen and in a notification, and the step waits. Pending approvals
expire after 24 hours.

### 3.8 Connectors and capabilities (W19, extended by W22 to W24)

```go
type Connector interface {
	ID() string                    // "desktop", "calendar", "gmail", "claude_code", "mcp:<name>"
	Status(ctx context.Context) Status // connected, needs_setup, or error, with a message for the UI
	Capabilities() []Capability
}

type Capability struct {
	Name   string // "browser.open"
	Risk   Risk
	Input  any    // a Go struct; its JSON Schema is derived for validation and for the LLM
	Invoke func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}
```

| Capability | Risk | Package |
|---|---|---|
| `browser.open` (one or more URLs, default browser) | local | W19 |
| `folder.open` | local | W19 |
| `tracking.start` (project or task) | local | W19 |
| `gwen.say` (a line on the cue card) | local | W20 |
| `calendar.read` | read | W19 (wraps the existing sync cache) |
| `gmail.list_recent`, `gmail.read` | read | W22 |
| `gmail.create_draft` | write | W22 |
| `gmail.send_draft` | external | W22 |
| `code.plan` | read | W23 |
| `code.execute` | write | W23 |

To add an app: write a connector, add it to the list in `internal/connect/registry.go`, and add a card to the
Connections screen. Run kinds ask for capabilities by name and never import a connector package.

### 3.9 Data model additions

Migration numbers are fixed per package so parallel work never collides. Synced tables use the existing
envelope (`id`, `created_at`, `updated_at`, `deleted_at`, `device_id`, `rev`) and are added to
`store.SyncTables`.

```sql
-- 00009_agenda.sql (W17)
ALTER TABLE calendar_busy RENAME TO calendar_events;
DROP INDEX calendar_busy_range;
CREATE INDEX calendar_events_range ON calendar_events (start_at, end_at);
ALTER TABLE calendar_events ADD COLUMN series_id TEXT NOT NULL DEFAULT '';
ALTER TABLE calendar_events ADD COLUMN all_day   INTEGER NOT NULL DEFAULT 0 CHECK (all_day IN (0, 1));
ALTER TABLE calendar_events ADD COLUMN busy      INTEGER NOT NULL DEFAULT 1 CHECK (busy IN (0, 1));
ALTER TABLE calendar_events ADD COLUMN location  TEXT NOT NULL DEFAULT '';
ALTER TABLE calendar_events ADD COLUMN html_link TEXT NOT NULL DEFAULT '';
CREATE TABLE agenda_marks (  -- synced
    id TEXT PRIMARY KEY CHECK (length(id) = 36), subject TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('done', 'skipped')), <envelope>
) STRICT;
CREATE UNIQUE INDEX agenda_marks_subject ON agenda_marks (subject) WHERE deleted_at IS NULL;

-- 00010_cues.sql (W18)
CREATE TABLE cue_rules (  -- synced; NULL means "use the default", 0 means "off"
    id TEXT PRIMARY KEY CHECK (length(id) = 36), subject TEXT NOT NULL,
    lead_minutes INTEGER CHECK (lead_minutes BETWEEN 0 AND 1440), at_start INTEGER CHECK (at_start IN (0, 1)),
    follow_up_minutes INTEGER CHECK (follow_up_minutes BETWEEN 0 AND 240), speak INTEGER CHECK (speak IN (0, 1)),
    <envelope>
) STRICT;
CREATE UNIQUE INDEX cue_rules_subject ON cue_rules (subject) WHERE deleted_at IS NULL;
CREATE TABLE cue_log (  -- device-local
    id TEXT PRIMARY KEY CHECK (length(id) = 36), subject TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('lead', 'start', 'follow_up')), due_at INTEGER NOT NULL,
    fired_at INTEGER NOT NULL, state TEXT NOT NULL CHECK (state IN ('shown', 'late', 'missed', 'acted', 'snoozed', 'expired')),
    action TEXT NOT NULL DEFAULT '', snoozed_until INTEGER, channels TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(channels))
) STRICT;

-- 00011_runs.sql (W19)
CREATE TABLE resources (  -- synced; links and folders attached to a subject
    id TEXT PRIMARY KEY CHECK (length(id) = 36), subject TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('url', 'folder', 'file', 'note')), title TEXT NOT NULL DEFAULT '',
    value TEXT NOT NULL, position INTEGER NOT NULL CHECK (position >= 0), <envelope>
) STRICT;
CREATE TABLE routines (  -- synced; "when it starts" steps for a subject
    id TEXT PRIMARY KEY CHECK (length(id) = 36), subject TEXT NOT NULL,
    steps TEXT NOT NULL CHECK (json_valid(steps)),           -- [{"capability": "...", "input": {...}}]
    auto_start INTEGER NOT NULL DEFAULT 0 CHECK (auto_start IN (0, 1)), <envelope>
) STRICT;
CREATE UNIQUE INDEX routines_subject ON routines (subject) WHERE deleted_at IS NULL;
CREATE TABLE project_dirs (  -- device-local: folders differ per machine
    project_id TEXT PRIMARY KEY REFERENCES projects (id), path TEXT NOT NULL,
    test_command TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE runs (  -- device-local, like everything below
    id TEXT PRIMARY KEY CHECK (length(id) = 36), kind TEXT NOT NULL, subject TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL, brief TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'running', 'waiting', 'done', 'failed', 'cancelled')),
    reason TEXT NOT NULL DEFAULT '', external_ref TEXT NOT NULL DEFAULT '', result TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(result)),
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, started_at INTEGER, ended_at INTEGER
) STRICT;
CREATE TABLE run_steps (
    id TEXT PRIMARY KEY CHECK (length(id) = 36), run_id TEXT NOT NULL REFERENCES runs (id), position INTEGER NOT NULL,
    title TEXT NOT NULL, capability TEXT NOT NULL DEFAULT '', risk TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'waiting', 'done', 'failed', 'skipped')),
    input TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input)), output TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output)),
    error TEXT NOT NULL DEFAULT '', started_at INTEGER, ended_at INTEGER
) STRICT;
CREATE TABLE run_events (
    seq INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL REFERENCES runs (id), step_id TEXT,
    kind TEXT NOT NULL, data TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data)), at INTEGER NOT NULL
) STRICT;
CREATE TABLE approvals (
    id TEXT PRIMARY KEY CHECK (length(id) = 36), run_id TEXT NOT NULL REFERENCES runs (id), step_id TEXT NOT NULL,
    capability TEXT NOT NULL, risk TEXT NOT NULL, summary TEXT NOT NULL, payload TEXT NOT NULL CHECK (json_valid(payload)),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
    created_at INTEGER NOT NULL, decided_at INTEGER
) STRICT;
CREATE TABLE grants (
    id TEXT PRIMARY KEY CHECK (length(id) = 36), capability TEXT NOT NULL, scope TEXT NOT NULL,  -- a subject ref or a folder
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'ask')), created_at INTEGER NOT NULL, expires_at INTEGER
) STRICT;
```

Runs stay on the machine that ran them, because they touch that machine's folders and browser. Email bodies,
tokens, and file contents are never stored in these tables (section 7).

### 3.10 API and events

New endpoints follow the existing style (`internal/api/routes.go`): `/v1/...`, JSON bodies, and the existing
error codes. Every endpoint gets a `client.API` method and a `clienttest.Fake` entry.

| Area | Endpoints | Package |
|---|---|---|
| Agenda | `GET /v1/agenda?day=`, `PUT /v1/agenda/marks` (subject, status), `DELETE /v1/agenda/marks?subject=` | W17 |
| Cues | `GET /v1/cues?state=active`, `POST /v1/cues/{id}/act` (start, snooze, skip, open), `GET /v1/cue-rules?subject=`, `PUT /v1/cue-rules` | W18 |
| Presence | `GET /v1/cues/{id}/line` (the line to show and speak) | W20 |
| Subjects | `GET/PUT /v1/resources?subject=`, `GET/PUT /v1/routines?subject=` | W19 |
| Projects | `GET/PUT /v1/projects/{id}/dir` (folder and test command on this machine) | W19 |
| Runs | `GET /v1/runs`, `POST /v1/runs`, `GET /v1/runs/{id}` (with steps and log), `POST /v1/runs/{id}/start`, `/cancel`, `/resume`, `/message` | W19 |
| Approvals | `GET /v1/approvals?status=pending`, `POST /v1/approvals/{id}` (decision, optional edited payload), `GET /v1/approvals/{id}/wait` | W19, wait in W23 |
| Connections | `GET /v1/connections`, `POST /v1/connections/{id}/connect`, `DELETE /v1/connections/{id}` | W19, Gmail in W22 |

New SSE events: `agenda_changed`, `cue_fired`, `cue_updated`, `run_changed` (run id and status),
`approval_requested`, `approval_decided`, `connection_changed`.

### 3.11 The three workflows, end to end

**DSA practice (routine run, no LLM needed)**

1. Once: open the DSA event (or task) on Today, then **When it starts**. Add steps with pickers: open links
   (today's problem list, your notes page), open a folder (`~/dsa`), start tracking the DSA project.
2. 18:55: lead cue, "DSA starts in 5 minutes." 19:00: start cue with **Start**.
3. Start creates a routine run. Each step runs and is logged. Links open in your default browser through the
   desktop portal. If something fails, the card says so ("Couldn't open the browser"). The task is not
   marked done just because a link opened.
4. 19:10: if you have not started, Gwen follows up once: "DSA was due 10 minutes ago. Open it now, or move it?"

LeetCode has no official API, so problem links are stored as resources, not fetched from LeetCode.

**Email review (run kind `email_review`)**

1. You ask in chat ("go through my last 5 emails") or press **Review email** on the Work screen. The assistant
   gets one new action type, `start_run`, so chat can start a run.
2. Read: Gwen fetches the newest N inbox messages (default 5, at most 20), skipping sent and spam.
3. Redact (pure, local): card numbers (Luhn check), CVV codes near card words, IBANs, bank account and routing
   numbers, and tax ids become `[redacted]`. Bodies are trimmed to 4,000 characters and quoted history removed.
4. Triage (new LLM job `EmailTriage`, structured output, works on every provider): per message a 2 sentence
   summary, needs a reply (yes, no, maybe), why, urgency, a draft reply, and questions for you.
5. Discuss: edit a draft directly, or ask Gwen to change it ("shorter", "say I'm free Tuesday").
6. For each message you choose: **Save as draft** (creates a Gmail draft and shows Open in Gmail) or
   **Send** (an approval card shows To, Cc, Subject, and body; one click sends it). Nothing is sent without
   that click. Your first Save as draft click is the run's one `write` approval.
7. If an email contains a card number, the card shows a warning: delete the message and point the sender to the
   official payment page. The suggested reply does the same and never repeats the number.

**Coding task (run kind `code`)**

1. On the Projects screen each project gets **Folder on this computer** (a folder picker) and **Test command**
   (with detected suggestions such as `make check`, `go test ./...`, `npm test`).
2. You write the task with a short brief: what, why, and how you will know it is done. Then **Hand to Gwen**.
3. Plan: Gwen creates a git worktree for the run on a new branch `gwen/<short id>` under
   `~/.local/share/gwen/worktrees/<run id>`, then runs Claude Code in plan mode (read only). The worktree
   starts from your last commit, so uncommitted changes are not in it; the run view says so. The plan comes
   back as JSON: summary, files with paths and reasons, steps, test plan, and questions.
4. Discuss: your replies resume the same Claude Code session, still in plan mode, until you approve. Approving
   the plan is the run's one `write` approval.
5. Execute: Claude Code resumes with edits allowed inside the worktree only. Any permission outside the allowed
   list becomes an approval card in Gwen. Live progress streams into the run log.
6. Check: Gwen runs the test command itself in the worktree and reports the real exit code. It does not trust
   the model's claim.
7. Result: change summary, test result, and branch name, with **Open folder** and **Discard**. You merge it
   yourself. Gwen never pushes in this phase.

Claude Code calls (flags checked against the installed 2.1.296; W23 records a fixture of real output):

```sh
# plan (cwd = the run's worktree)
claude -p --output-format json --json-schema "$PLAN_SCHEMA" --session-id "$SESSION" \
  --permission-mode plan --setting-sources "" --strict-mcp-config --append-system-prompt "$RULES" "$BRIEF"
# discuss: same flags with --resume "$SESSION" and the user's reply as the prompt
# execute
claude -p --output-format stream-json --verbose --resume "$SESSION" --permission-mode acceptEdits \
  --setting-sources "" --strict-mcp-config --mcp-config "$GWEN_MCP" \
  --allowedTools "Read Edit Write Glob Grep Bash($TEST_COMMAND) Bash(git status) Bash(git diff:*)" \
  --disallowedTools "WebFetch WebSearch Bash(git push:*)" \
  --permission-prompts host --permission-prompt-tool mcp__gwen__approve "Carry out the approved plan."
```

`$GWEN_MCP` starts `gwen mcp`, a new stdio MCP server (official Go SDK) with two tools: `approve` (creates an
approval for the run and waits for your decision, then returns
`{"behavior":"allow","updatedInput":...}` or `{"behavior":"deny","message":"..."}`) and `report` (adds a
progress note to the run log). Runs reuse the binary discovery and `systemd-run --user --scope` wrapping of
`internal/llm/claudecode.go`, and strip API keys from the environment as that file does. Limits: 45 minutes
per execute phase, `--max-budget-usd 5` when an API key is in use.

### 3.12 Processes and memory

| Process | When it runs | Change |
|---|---|---|
| `gwend` | always | adds the cue loop, the run executor, and connector clients (Go, no models loaded) |
| `gwen-ui` dashboard | when you open it | new screens |
| `gwen-ui --cue` | a few seconds to 2 minutes per cue | new |
| `gwen-ui --talk` | when you talk | can start from a cue |
| `claude` and `gwen mcp` | only during a coding run | new |
| panel widget or `gwen-tray` | always (already) | launches the cue card |

---

## 4. Products and services

| Need | Choice | Why | Not chosen |
|---|---|---|---|
| Calendar | Google Calendar API v3 | already connected | |
| Email | Gmail API v1 (`google.golang.org/api/gmail/v1`, same module as Calendar) with scopes `gmail.readonly` and `gmail.compose` | reads mail, creates drafts, and sends drafts. Both scopes are "restricted", which is fine for a personal app: Google shows an unverified-app warning and allows up to 100 users | IMAP (needs app passwords); Composio or Zapier (a third party would hold your mail token) |
| Coding agent | Claude Code CLI, already installed and logged in with your plan | edits, search, and tests are already solved; plan mode and resumable sessions fit "discuss first" | our own code agent loop; Aider or OpenHands (another stack to maintain) |
| Tool protocol | MCP, official Go SDK `github.com/modelcontextprotocol/go-sdk` v1.7.0 | Gwen serves the approval tool to Claude Code now, and can use any MCP server later | custom JSON-RPC |
| Browser | your default browser through the XDG desktop portal `OpenURI` (D-Bus, `godbus` is already a dependency), with `xdg-open` as fallback | works from a systemd service on Wayland and raises the window properly | driving a browser for simple link opening |
| Browser automation (later, W26) | Playwright MCP, opt-in | real multi-step web tasks | always-on headless Chrome |
| Secrets | system secret store through `github.com/zalando/go-keyring` (on Plasma 6 this is ksecretd), with the current 0600 files as fallback for the Pi | tokens out of plain files | plain files for new tokens |
| Notifications | freedesktop notifications over D-Bus (have it), plus the `Inhibited` property for Do Not Disturb; ntfy for the phone (have it), with a "view" button that opens a link | already working | |
| Notes (later, W25) | a plain Markdown folder (works with Obsidian) | local, no account | Notion API (cloud and another OAuth) |
| Voice | unchanged: Fish Audio by default, Kokoro offline, Parakeet for speech to text | | |
| LLM | unchanged providers. Email triage works on all of them. Coding needs Claude Code | | |
| Run engine | SQLite tables and Go code in `gwend` | one user, one machine | Temporal, queues, LangChain, n8n, Pipedream |

New Go modules for this phase: `github.com/modelcontextprotocol/go-sdk` (W23) and
`github.com/zalando/go-keyring` (W22). Nothing else.

Before connecting Gmail, turn on 2-Step Verification for that Google account, because Gwen will be able to
read your mail.

---

## 5. UI and UX

### 5.1 What we take from `ui/design.md`

| Keep | Change | Drop |
|---|---|---|
| explicit UI states from real app state (runtime, audio, character, task) | the palette (see 5.2) | "Lab" as a main screen; diagnostics move to Settings |
| character stage rules: fixed bounds, still frame when idle, state from the real pipeline | modes become Gwen, Today and Tasks, Work | code editor and terminal panels (Claude Code edits code; Gwen shows summaries) |
| task and activity panel rules: real states only, no fake progress | | telemetry charts for models we don't run |
| approvals before side effects, event rows for tool activity | | |
| 24 px status bar, compact panel headers, Inter plus JetBrains Mono for paths and times | | |
| accessibility, reduced motion, no fake data | | |

It does not cover what an assistant needs most, so `ui/DESIGN.md` gains sections for the agenda, the cue card,
the approval card, the run view, the Connections screen, and the subject drawer. The merged file replaces both
current files (W16).

### 5.2 Palette

Three dark themes. Tokens keep their current names in `ui/src/index.css`, and each theme overrides them under
`[data-theme="..."]`. You switch in **Settings, Appearance** (stored as `[ui] theme`). Tracking colours
(working, idle, break, off) stay the same in every theme because they match the tray icons. Project colours are
data and stay as they are.

| Token | Ghost (default) | Tide | Ember |
|---|---|---|---|
| canvas | `#0F0E13` | `#0B0F14` | `#110F0D` |
| surface-1 | `#16151C` | `#111821` | `#191613` |
| surface-2 | `#1D1B24` | `#172029` | `#201C18` |
| surface-3 | `#25222D` | `#1E2833` | `#28231E` |
| surface-4 | `#2D2A36` | `#26313D` | `#302A24` |
| line | `#2F2B38` | `#26303B` | `#322C26` |
| line-strong | `#443F50` | `#384656` | `#473F37` |
| ink | `#F3F0F5` | `#E9EFF5` | `#F4EFE9` |
| ink-muted | `#C7C0CD` | `#B6C2CE` | `#CDC4B9` |
| ink-subtle | `#958D9E` | `#8392A1` | `#998F84` |
| ink-faint (disabled only) | `#6C6575` | `#5E6C7A` | `#70665D` |
| accent | `#E0679A` rose | `#3EC1AF` teal | `#E9935A` apricot |
| accent-hover / pressed | `#EB86B0` / `#C44F80` | `#62D5C5` / `#2BA393` | `#F1AB7A` / `#CD7942` |
| accent-soft | `#3A1E2C` | `#12302C` | `#3B2617` |
| on-accent (text on buttons) | `#1B0D14` | `#04130F` | `#1C0F06` |
| focus | `#F2A5C5` | `#86E2D6` | `#F6C29A` |
| success / warning | `#5CC58E` / `#E9BE5C` | `#86C66A` / `#E9B65A` | `#6CC58C` / `#E6D16A` |
| danger / info | `#F2694F` / `#62C3D0` | `#EF6B6B` / `#7FA9EE` | `#F0656A` / `#79B2E6` |

Measured contrast (WCAG): ink on canvas 16.6 to 17.0, ink-muted on surface-1 9.9 to 10.5, ink-subtle on
surface-1 5.6 to 5.7, text on accent buttons 5.9 to 8.6, accent on canvas 6.0 to 8.7. All body text passes AA.

**Why Ghost is the default:** it comes from Gwen's own art (black suit, white hood, magenta-pink lining), so the
app and the character look like one thing. It is warm without being loud, and its danger colour is an
orange-red that reads differently from the rose accent. If you want something cooler, pick Tide.

### 5.3 Navigation

Eleven items become eight. Screens that belong together become tabs.

| Nav item | Contains | Package |
|---|---|---|
| **Today** | agenda list, timeline, now and next, to-dos, Gwen's latest line | W17, then W21 |
| **Gwen** | the companion chat (renamed from Assistant in the nav), character stage, Talk button | W16 |
| **Tasks** | tabs: Inbox, Board, Projects | W16 |
| **Plan** | tabs: Day plan, Goals | W16 |
| **Work** | runs (needs you, running, finished) and approvals, with a badge count | W21 |
| **Review** | tabs: Weekly, Stats, History | W16 |
| **Connections** | Google Calendar, Gmail, Claude Code, phone, sync hub; later MCP servers | W16, then W22, W23 |
| **Settings** | General, Reminders, Voice, AI, Personality, Appearance, Diagnostics | W16, W18 |

The status bar (24 px) shows: tracking state and time, the next cue ("DSA at 19:00"), approvals waiting, voice
state, and the daemon connection.

### 5.4 Key screens

```text
Today ─────────────────────────────────────────────────────────────────────────────────
 Sat 10 Oct                       Working 3h 12m  ·  next: DSA practice 19:00
 [all day] Holiday
 [timeline: events solid and titled · plan blocks in project colours · line at now]
 Agenda                                          │ Gwen
 13:30  [ ] Standup             Calendar         │ (sprite)  "DSA in 25 minutes. Want me to
 14:00  [ ] Write plan.md       Gwen · 1h        │            open today's problem when it starts?"
 19:00  [ ] DSA practice        Calendar · ▶     │            [Set it up]  [Not now]
 To-dos                                          │ Needs you: 1 approval
```

```text
Cue card (bottom right, does not take focus)
 ┌──────────────────────────────────────┐
 │ (sprite)  DSA practice · 19:00        │
 │ "Time for DSA. I'll open today's      │
 │  problem and your notes."             │
 │ [Start]   [5 min]   [Skip]   [Talk]   │
 └──────────────────────────────────────┘
```

```text
Work › Email review · 5 messages · waiting for you
 1  Interview slot (recruiter)     needs reply   [Edit draft]  [Save as draft]  [Send]
 2  CI failed (GitHub)             no reply      "an automatic notice"
 3  Invoice question               maybe         [Edit draft]  [Save as draft]  [Send]
 Log: read 5 messages · triage done · 1 draft saved
```

**Subject drawer** (opens from any agenda row, task, or event): notes, links and folders, **When it starts**
(routine steps chosen with pickers), reminder overrides, and past runs.

### 5.5 Interaction rules

- One primary action per card. The primary button uses the accent; everything else is secondary or quiet.
- Gwen never steals focus and never covers the composer or a dialog.
- Spoken lines stay under 20 words.
- Anything that leaves your computer shows exactly what will happen before you click.
- No raw syntax in the UI: pickers for times, repeats, durations, folders, and routine steps. The test command is
  the one free text field, with detected suggestions.
- Empty states say what to do next ("No runs yet. Hand a task to Gwen from its menu.").

---

## 6. Implementation plan

### 6.1 Rules for every package

1. **Scope.** Change only the paths under your package's **Owns**. Shared registries accept additive lines at the
   end of the relevant block, never edits to other lines: `internal/api/routes.go`, `internal/wire/events.go`,
   `internal/client/client.go` and `clienttest`, `internal/store/sync.go` (`SyncTables`), `cmd/gwend/daemon.go`
   (wiring), `cmd/gwen-ui/app.go` (bindings), `ui/src/api.ts`, `ui/src/App.tsx` (nav list),
   `ui/src/components/StatusBar.tsx`, `ui/src/screens/Settings.tsx` (section list), and
   `ui/src/screens/Connections.tsx` (card list).
2. **No new decisions.** If this file does not answer something, stop and report the gap. Do not invent one.
3. **Conventions** (from the old `CONVENTIONS.md`): instants are `int64` Unix ms UTC at storage and wire
   boundaries; never call `time.Now()` outside `main`, take a `Clock`; wrap errors with `%w`; sentinel errors in
   the owning package; tests use `clock.NewFake` and never sleep; `gofmt -s`, vet, staticcheck.
4. **Modules.** Only the two in section 4, added by the package that needs them first.
5. **GUI first.** Every feature works from the dashboard. A CLI command is optional, never the only way.
6. **UI files** are hand-formatted at about 200 columns. Never run prettier with default settings.
7. **setup.md** is updated in the same change when setup steps change.
8. **Live checks** never touch your installed `gwend`. Use a throwaway daemon with a short `XDG_RUNTIME_DIR`
   and `env -u INVOCATION_ID`.
9. **Stale links.** Fix or remove `docs/` links in any file you edit.
10. **Done means:** everything under Delivers works with no placeholders; `make check` passes; every acceptance
    item has a test or a recorded manual check; the diff stays inside your paths; your summary lists any gaps.

### 6.2 Order

```text
Wave 0   W15 housekeeping
Wave 1   W16 design system and shell      |  W17 agenda
Wave 2   W18 cues                         |  W19 harness core
Wave 3   W20 presence (needs W18)         |  W21 subject drawer and Work screen (needs W16, W19)
Wave 4   W22 Gmail review                 |  W23 coding runner           (both need W21)
Later    W24 MCP connectors · W25 notes folder · W26 browser automation
```

Packages in the same wave run in parallel on their own branches (`w17-agenda` and so on). Merge in the order
listed. After Wave 1 your appointments are on Today. After Wave 2 you get the 5 minute reminder. After Wave 3
Gwen speaks first and the DSA routine opens your browser. After Wave 4 she can review email and take coding
tasks.

### 6.3 Work packages

Each package is one prompt (template in 6.4).

#### W15: Housekeeping

- **Owns:** `Makefile`, `.github/workflows/check.yml`, `README.md`.
- **Delivers:** remove the `docs-check` target and its call from `check`; point the Makefile header comment at
  this file; link `plan.md` from the README's Development section.
- **Acceptance:** `make check` passes on a clean checkout.

#### W16: Design system and app shell

- **Owns:** `ui/DESIGN.md`, `ui/design.md` (deleted after the merge), `ui/src/index.css`, `ui/src/App.tsx`,
  `ui/src/nav.ts`, `ui/src/components/ui.tsx`, new `ui/src/components/StatusBar.tsx`, new
  `ui/src/screens/Connections.tsx`, new `ui/src/settings/Appearance.tsx`, the hard-coded colours in `ui/src/components/{tags,BrandMark,Heatmap}.tsx`,
  `ui/src/components/{chart,color}.ts`, `ui/src/screens/{Board,History,Settings}.tsx`, and a `[ui]` section in
  `internal/config` with its wire and patch fields.
- **Delivers:** the merged design file (5.1); three themes (5.2) with `ghost` as default; the 8 item navigation with
  tabs (5.3) except Work, which W21 adds; the status bar with tracking, voice, and connection state; a Connections
  screen showing Google Calendar, phone, sync hub, and AI provider status (it reuses the existing settings parts);
  Settings, Appearance and Settings, Diagnostics (new `ui/src/settings/Diagnostics.tsx`: daemon health from
  `GET /v1/health` and integration status); Assistant renamed to Gwen in the nav.
- **Acceptance:** `tsc` passes; no hex colours outside `index.css` except project colour data (grep check);
  offscreen WebKitGTK snapshots of Today, Gwen, and Settings in all three themes; tab state survives switching
  screens.

#### W17: Agenda

- **Owns:** migration `00009_agenda.sql`; `internal/subject` (new); `internal/agenda` (new);
  `internal/store/{calendar,agenda}.go`; `internal/gcal/sync.go`; `internal/api/agenda.go`;
  `internal/wire/agenda.go`; `internal/client/agenda.go`; `cmd/gwen/{agenda,panel}.go`; `ui/src/agenda.ts`;
  `ui/src/plan.ts`; `ui/src/screens/{Today,NowCard,DayDetail}.tsx`; `ui/src/components/Timeline.tsx`;
  `packaging/plasma/dev.gwen.panel/contents/ui/NowCard.qml`.
- **Delivers:** section 3.4 in full. Sync stores series id, all-day, busy, location, and link, and keeps free
  all-day events with `busy = 0`. The planner reads only `busy = 1`. `agenda_changed` fires when a sync changes
  events or a mark changes. The Today list shows events with a checkbox and a calendar badge. The timeline draws
  events as solid titled blocks plus an all-day strip. The now card, the panel's next item, and the briefing
  include events. `gwen agenda` prints the day.
- **Acceptance:** golden tests for `agenda.Build`: an event crossing the day boundary with `day_rollover` at 02:00;
  a multi-day all-day event; a cancelled instance removed; a declined invitation hidden; an event before
  `day_start` still listed; instance ids stable across syncs. A sync test against the fake Google endpoint
  (`Options.Endpoint`). An API test for marks. Manual: your 13:30 event shows as a row you can check off.

#### W18: Cues

- **Owns:** `internal/cue` (new); migration `00010_cues.sql`; `internal/store/cues.go`; `cmd/gwend/cues.go`;
  `internal/notify` (the `Inhibited` check, cue kinds, their buttons, and phone routing); the `[cues]` section in
  `internal/config`; `internal/api/cues.go`; `internal/wire/cues.go`; `internal/client/cues.go`;
  `ui/src/settings/Reminders.tsx`; `cmd/gwen/cues.go`.
- **Config:**

  ```toml
  [cues]
    lead = "5m"           # "0" turns it off
    at_start = true
    follow_up = "10m"     # "0" turns it off; blocks and routines only
    speak = true
    personal_lines = false
    quiet_start = "23:00"
    quiet_end = "08:00"
    grace = "10m"
  ```

- **Delivers:** sections 3.5 and the notification half of 3.6, including the template lines (`Cue.Text`).
  Desktop notifications with Start, 5 min, and Skip.
  Clicks arrive through the existing action channel. The start hook list (`api.Server.StartHooks`) that W19
  registers into. The status bar's next reminder. Settings, Reminders, with duration pickers.
- **Acceptance:** template line tests per cue kind; table tests for `cue.Due`: lead, start, and follow-up times; snooze; no cues for done or
  skipped entries; a restart inside `grace` delivers late, beyond it logs missed; a moved event expires the old
  cue; a cancelled event expires its cues; quiet hours; Do Not Disturb; away goes to the phone; a DST change. A
  loop test with the fake clock. Manual: an event 7 minutes ahead notifies at minus 5.

#### W19: Harness core

- **Owns:** `internal/harness` (new); `internal/connect` (new: registry, policy, the desktop connector with
  `browser.open`, `folder.open`, `tracking.start`, and the calendar connector with `calendar.read`); migration
  `00011_runs.sql`; `internal/store/{runs,resources,routines,projectdirs}.go`;
  `internal/api/{runs,approvals,resources,routines,connections}.go`; matching `internal/wire` and
  `internal/client` files; `cmd/gwen/runs.go`.
- **Delivers:** sections 3.7 and 3.8, the routine run kind, the start hook (Start runs the subject's routine),
  restart recovery, cancel, the 2 run limit, approvals with SSE, project folder endpoints, connections listing.
  URLs open through the portal's `OpenURI` with `xdg-open` as fallback, one call per URL.
- **Acceptance:** tests for every allowed and refused state move; policy table tests (`external` can never be
  granted); a running run becomes `waiting` after a restart; cancel stops a step; a routine run with a fake opener
  logs each step and reports a failed open honestly; an approval round trip through `clienttest`; a test that
  scans run events and logs for token-like strings and finds none.

#### W20: Presence

- **Owns:** `cmd/gwen-ui/cue.go` (the `--cue` window); `ui/src/screens/CueCard.tsx`; `ui/src/screens/Talk.tsx`
  (start from a cue); `cmd/gwen/panel.go` (the `cue` field); `packaging/plasma/dev.gwen.panel/contents/ui/main.qml`;
  `cmd/gwen-tray`; `internal/llm/line.go` and the new `Planner.Line` method for every provider;
  `internal/api/presence.go`; the `gwen.say` capability.
- **Delivers:** section 3.6. The card as described, one card per cue (single instance id `dev.gwen.ui.cue`), the
  spoken line through the speech registry, the 10 minute speech limit, and optional personal lines.
- **Acceptance:** `Line` timeout falls back to the template; `gwen panel` shows
  a cue only while it is active and may be spoken; Talk starts with the cue line. Manual on KDE: an event 6
  minutes ahead opens a speaking card at minus 5 and Start opens the DSA links.

#### W21: Subject drawer and Work screen

- **Owns:** `ui/src/components/{SubjectDrawer,RoutineEditor,RunView,ApprovalCard}.tsx`; `ui/src/screens/Work.tsx`;
  `ui/src/screens/Today.tsx` (in this wave); `ui/src/screens/Projects.tsx` (folder and test command);
  `cmd/gwen-ui/host.go` (folder dialog and test command detection).
- **Delivers:** the drawer from every agenda row and task, the routine editor with pickers, the Work screen with
  runs and approvals, the Work nav item with its badge, the status bar's approvals count, and on Today the Gwen
  panel (the active cue's `Text` with its buttons) and the "Needs you" line.
- **Acceptance:** `tsc`; snapshots of the drawer, Work, and an approval card; the DSA routine set up and run
  from the GUI against a throwaway daemon.

#### W22: Gmail review

- **Owns:** `internal/gmail` (the connector); `internal/secret` (new: secret store with file fallback);
  `internal/config/credentials.go` (moves existing credentials into the secret store once, deleting each file only
  after a successful write); `internal/redact` (new, pure); `internal/llm/triage.go` and the new
  `Planner.EmailTriage` method; the `email_review` run kind in `internal/harness/email`; the `start_run` assistant
  action; Gmail connect in `internal/api/connections.go` and a new `cmd/gwen-ui/gmail.go`; `ui/src/components/EmailReview.tsx`;
  the Gmail card on Connections; the Gmail section of `setup.md`.
- **Delivers:** section 3.11, email review. Gmail uses its own token from a separate consent with
  `gmail.readonly` and `gmail.compose` on the same Google client. Disconnect deletes the local token and links to
  Google's permissions page for full revocation.
- **Acceptance:** redaction tests (card numbers with spaces and dashes, CVV after "cvv", IBAN, routing plus
  account; no false positives on phone numbers, dates, or order numbers that fail Luhn); connector tests against
  an `httptest` fake; triage parse and validate tests; sending needs an approval every time; drafts appear only
  after you choose; no body text in the database or logs. Manual: review your last 5 emails, save one draft, send
  one test email to yourself.

#### W23: Coding runner

- **Owns:** `internal/coderun` (the `code` run kind and Claude Code process handling); `internal/gitwt` (worktree
  create and remove); `cmd/gwen/mcp.go` (`gwen mcp`); `GET /v1/approvals/{id}/wait` in
  `internal/api/approvals.go`; `ui/src/components/CodeRun.tsx`; the Claude Code card on Connections (installed
  version and path).
- **Delivers:** section 3.11, coding task, with the exact calls listed there.
- **Acceptance:** tests with a fake `claude` script that replays recorded JSON and stream-json output; an approval
  round trip through `gwen mcp`; worktree create and cleanup on a temp repo; Gwen's own test run decides the
  reported result. Manual: a small change in a scratch repo, from brief to branch.

#### Later packages

These get their full sections here when they start: **W24 MCP connectors** (add an MCP server by command or URL
on Connections; its tools become capabilities with risk `write` or higher until you lower them), **W25 notes
folder** (read and write Markdown in a chosen folder, for DSA notes and run summaries), **W26 browser automation**
(Playwright MCP, opt-in).

### 6.4 How to write each prompt

Keep every prompt short. The decisions live in this file, so the prompt only points at them:

```text
You are implementing work package W17 (Agenda) of Gwen.
Read plan.md sections 3.3, 3.4, 3.9 (00009), 3.10 (Agenda row), 6.1, and the W17 entry in 6.3.
Then read the code under W17's Owns and implement everything under Delivers.
Change only those paths, plus additive lines in the shared registries named in 6.1.
If plan.md does not answer a question, stop and report it. Do not decide it yourself.
Run make check. For live checks, use a throwaway gwend, never the installed one.
Finish with: what you built, test results, and any gaps in plan.md.
Branch: w17-agenda.
```

Use the same shape for every package: its number and name, the sections to read, the branch name. Run one wave
at a time, merge in the listed order, run `make check` on main, and try the result yourself before the next wave.

---

## 7. Safety and privacy rules

These bind every package.

1. **Outside content is data.** Email text, web pages, calendar descriptions, and repository files can never
   change permissions, add recipients, or approve anything. Prompts mark such text as quoted data.
2. **The daemon enforces permissions** at the moment of each capability call (3.7), never only the UI or the
   prompt.
3. **Anything external needs a click every time.** No standing permission for `external` in this phase. Voice
   cannot approve.
4. **No secrets in prompts, logs, run events, SSE, or the UI.** Tokens live in the secret store. Never put
   credentials in config files, chat messages, notes, or spreadsheets.
5. **Email privacy.** Redact before any model sees the text (3.11). The database keeps message ids, sender, and
   subject for display, never bodies. Draft text is kept only while the run is open; afterwards only "saved" or
   "sent" and the Gmail id remain. If an email contains payment details, Gwen tells you to delete it and send the
   sender to the official payment page, and never repeats the details.
6. **Coding runs** work in a separate git worktree with a fixed list of allowed tools, no web tools, and no push.
   Gwen checks the result by running your test command itself.
7. **Honest status.** Never show "connected", "sent", "opened", or "done" before the system confirms it. Missing
   values say "Not available", never a made-up number.
8. **Google account.** Keep the OAuth app published to production (as `setup.md` explains) so tokens don't expire
   every 7 days, and keep 2-Step Verification on.
