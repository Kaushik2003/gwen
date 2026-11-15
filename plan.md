# Gwen plan: from tracker to assistant

Status: W27 (Gwen Terminal and the phone) is built on branch `w27-gwen-mobile`; its real-phone check and test batch
are still to do (6.3, W27). The first W16/W16b (Gwen's screens as a mod inside Gwen Terminal) was rejected on
2026-11-13 and is replaced by a new W16 and W16b: one Gwen app, Gwen's own window with the Herdr workbench inside it
(3.15), designed first on the "Gwen, one app" canvas. The new W16 is built (not yet tried in the installed window), and
W16b is built, and the first W16's mod, app listener, and Gwen space are deleted. W15 and W17 to W23 are ready. W32
(Gwen's agent: the chat works in your projects, 3.16) is built on top of W19 and live-checked on a throwaway daemon;
not yet tried in the installed window. Last updated 2026-11-13.

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
| From my phone, by chat or voice: see my Claude Code sessions on the laptop, review their changes, give the next task, or start one in a folder; over my Wi-Fi or tailnet, not the Claude app | **Gwen Terminal and the phone**: Gwen Terminal (a thin fork of Herdr) on the laptop, and a phone app on your tailnet that sees its Claude sessions, types into them, shows their live terminal, and reviews their changes | W27 |
| Read Slack, find the actionable points, and hand them to an agent (Slack may be a cloud service) | **Slack inbox** | W28 (later) |
| Turn the chat into an agent that does what I say in a project | **Gwen's agent**: in a project she reads the code, plans, and hands every change and command to Claude sessions she follows and reports on; your policy decides what asks | W32 |

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
- **Who opens the card.** `gwend` never launches windows (one exception: it opens Gwen Terminal for a session you start from the phone, 3.14.5). It runs as a systemd service and also runs headless on
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
expire after 24 hours. An approval may offer **grant scopes**; when you pick one ("remember"), the approval also makes
a grant for that scope, which is never possible for `external` or `destructive`. Gwen's agent (3.16, W32) uses the
same approvals and grants, with a chat in place of a run.

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
-- 00012_project_dirs.sql (W16)
CREATE TABLE project_dirs (  -- device-local: folders differ per machine, never synced
    project_id TEXT PRIMARY KEY REFERENCES projects (id), path TEXT NOT NULL,
    test_command TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
) STRICT;

-- 00013_commit_summaries.sql (W16b)
CREATE TABLE commit_summaries (  -- device-local, like code_sessions
    claude_session_id TEXT NOT NULL, commit_short TEXT NOT NULL, summary TEXT NOT NULL,
    made_at INTEGER NOT NULL, PRIMARY KEY (claude_session_id, commit_short)
) STRICT;

-- 00014_project_parent.sql (sub-projects): one level deep, synced, no foreign key
ALTER TABLE projects ADD COLUMN parent_id TEXT CHECK (length(parent_id) = 36 AND parent_id <> id);

-- 00015_task_attachments.sql (task context): a link, note, file, or app item (Composio CLI) with its text
-- snapshot; synced except files. Full DDL in internal/store/migrations.

-- 00018_agenda.sql (W17)
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

-- 00010_memory.sql (memory layer)
-- Existing chat messages remain the conversation record. Memory stores cited
-- evidence and claims separately, and keeps derived or device-local state out
-- of sync.
ALTER TABLE chat_messages ADD COLUMN memory TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(memory));
CREATE TABLE entities (id TEXT PRIMARY KEY, kind TEXT NOT NULL, name TEXT NOT NULL, aliases TEXT NOT NULL DEFAULT '[]', <envelope>) STRICT;
CREATE TABLE episodes (id TEXT PRIMARY KEY, kind TEXT NOT NULL, channel TEXT NOT NULL, speaker TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '', body TEXT NOT NULL, source TEXT NOT NULL DEFAULT '', at INTEGER NOT NULL,
    sensitivity TEXT NOT NULL, content_hash TEXT NOT NULL, <envelope>) STRICT;
CREATE TABLE claims (id TEXT PRIMARY KEY, key TEXT NOT NULL, subject_id TEXT, value TEXT NOT NULL, text TEXT NOT NULL,
    cardinality TEXT NOT NULL, status TEXT NOT NULL, trust TEXT NOT NULL, sensitivity TEXT NOT NULL,
    confidence REAL NOT NULL, valid_from INTEGER, valid_to INTEGER, occurred_at INTEGER, mentioned_at INTEGER NOT NULL,
    recorded_at INTEGER NOT NULL, superseded_at INTEGER, superseded_by TEXT, suppressed INTEGER NOT NULL DEFAULT 0,
    source_kind TEXT NOT NULL, source_id TEXT NOT NULL DEFAULT '', span TEXT NOT NULL DEFAULT '',
    source_device_id TEXT NOT NULL, channel TEXT NOT NULL, extractor TEXT NOT NULL DEFAULT '', content_hash TEXT NOT NULL,
    <envelope>) STRICT;
CREATE TABLE conflicts (id TEXT PRIMARY KEY, claim_a TEXT NOT NULL, claim_b TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
    sensitivity TEXT NOT NULL, status TEXT NOT NULL, resolution TEXT, resolved_at INTEGER, <envelope>) STRICT;
CREATE TABLE memory_links (id TEXT PRIMARY KEY, from_kind TEXT NOT NULL, from_id TEXT NOT NULL, to_kind TEXT NOT NULL,
    to_id TEXT NOT NULL, relation TEXT NOT NULL, <envelope>) STRICT;
-- `claims_fts` and `episodes_fts` are FTS5 indexes. `chat_notes`,
-- `memory_jobs`, `memory_backends`, `memory_usage`, `resources`, and the
-- ledger are device-local. `index.db` holds the rebuildable vector index.

-- 00011_remote.sql (W27); both tables are device-local and never synced
CREATE TABLE remote_devices (
    id TEXT PRIMARY KEY CHECK (length(id) = 36), name TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL, last_seen_at INTEGER, revoked_at INTEGER
) STRICT;
CREATE TABLE code_sessions (
    id TEXT PRIMARY KEY CHECK (length(id) = 13), terminal TEXT NOT NULL DEFAULT '',
    claude_session_id TEXT NOT NULL DEFAULT '', cwd TEXT NOT NULL, sandbox INTEGER NOT NULL DEFAULT 0 CHECK (sandbox IN (0, 1)),
    args TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(args)), pid INTEGER, transcript_path TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('starting', 'working', 'needs_you', 'done', 'idle', 'error', 'ended')),
    need TEXT NOT NULL DEFAULT '' CHECK (need IN ('', 'permission', 'question', 'other')),
    step TEXT NOT NULL DEFAULT '', last_message TEXT NOT NULL DEFAULT '', permission_mode TEXT NOT NULL DEFAULT '',
    start_status TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(start_status)), touched TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(touched)),
    pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)), seen_at INTEGER,
    started_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, ended_at INTEGER
) STRICT;
CREATE INDEX code_sessions_claude ON code_sessions (claude_session_id);
CREATE INDEX code_sessions_terminal ON code_sessions (terminal);

-- 00019_cues.sql (W18)
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

-- 00016_runs.sql (W19)
CREATE TABLE subject_resources (  -- synced; links and folders attached to a subject
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
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    run_id TEXT REFERENCES runs (id), thread_id TEXT,     -- a run's approval, or a chat's (W32): exactly one is set
    step_id TEXT NOT NULL,                                -- run_steps.id, or agent_steps.id for a chat
    capability TEXT NOT NULL, risk TEXT NOT NULL, pattern TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL, payload TEXT NOT NULL CHECK (json_valid(payload)),
    grant_scopes TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(grant_scopes)),  -- the scopes "remember" may pick
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
    created_at INTEGER NOT NULL, decided_at INTEGER,
    CHECK ((run_id IS NULL) <> (thread_id IS NULL))
) STRICT;
CREATE TABLE grants (
    id TEXT PRIMARY KEY CHECK (length(id) = 36), capability TEXT NOT NULL, pattern TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL,  -- a subject ref, a folder, project:<id>, or chat:<id> (W32)
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'ask')), created_at INTEGER NOT NULL, expires_at INTEGER
) STRICT;

-- 00017_agent.sql (W32, built before W17 and W18)
ALTER TABLE chat_threads ADD COLUMN project_id TEXT CHECK (project_id IS NULL OR length(project_id) = 36);  -- synced; NULL: everywhere
CREATE TABLE agent_chats (  -- device-local: the Claude Code session behind a chat on this machine
    thread_id TEXT PRIMARY KEY, claude_session_id TEXT NOT NULL, cwd TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('idle', 'starting', 'thinking', 'waiting', 'error')),
    error TEXT NOT NULL DEFAULT '', usage TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(usage)),
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE agent_steps (  -- device-local
    id TEXT PRIMARY KEY CHECK (length(id) = 36), thread_id TEXT NOT NULL, tool_use_id TEXT NOT NULL DEFAULT '',
    turn INTEGER NOT NULL, position INTEGER NOT NULL, tool TEXT NOT NULL, capability TEXT NOT NULL, risk TEXT NOT NULL,
    title TEXT NOT NULL, input TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input)), output TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('running', 'waiting', 'done', 'failed', 'denied', 'cancelled')),
    approval_id TEXT, undo TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(undo)), started_at INTEGER NOT NULL, ended_at INTEGER
) STRICT;
CREATE INDEX agent_steps_thread ON agent_steps (thread_id, turn, position);
ALTER TABLE project_dirs ADD COLUMN agent_autonomy TEXT NOT NULL DEFAULT ''
    CHECK (agent_autonomy IN ('', 'cautious', 'balanced', 'trusted'));  -- '' follows [agent] autonomy
```

`chat_threads` sends `project_id` in its sync columns from W32 on. A hub running an older `gwend` drops it, so update
the hub too, as for sub-projects. Runs stay on the machine that ran them, because they touch that machine's folders
and browser. Email bodies,
tokens, and file contents are never stored in these tables (section 7).

### 3.10 API and events

New endpoints follow the existing style (`internal/api/routes.go`): `/v1/...`, JSON bodies, and the existing
error codes. Every endpoint gets a `client.API` method and a `clienttest.Fake` entry.

| Area | Endpoints | Package |
|---|---|---|
| Agenda | `GET /v1/agenda?day=`, `PUT /v1/agenda/marks` (subject, status), `DELETE /v1/agenda/marks?subject=` | W17 |
| Cues | `GET /v1/cues?state=active`, `POST /v1/cues/{id}/act` (start, snooze, skip, open), `GET /v1/cue-rules?subject=`, `PUT /v1/cue-rules` | W18 |
| Presence | `GET /v1/cues/{id}/line` (the line to show and speak) | W20 |
| Memory | `GET/PATCH/POST/DELETE /v1/memory/claims`; `GET /v1/memory/{status, conflicts, entities, profile, resources, explain, export}`; `POST /v1/memory/{remember, recall, sleep, rebuild, forget-all}` | developed |
| Subjects | `GET/PUT /v1/resources?subject=`, `GET/PUT /v1/routines?subject=` | W19 |
| Projects | `GET/PUT /v1/projects/{id}/dir` (folder and test command on this machine) | W19 |
| Runs | `GET /v1/runs`, `POST /v1/runs`, `GET /v1/runs/{id}` (with steps and log), `POST /v1/runs/{id}/start`, `/cancel`, `/resume`, `/message` | W19 |
| Approvals | `GET /v1/approvals?status=pending`, `POST /v1/approvals/{id}` (decision, optional edited payload, `remember`: one of the approval's grant scopes, refused for `external` and `destructive`), `GET /v1/approvals/{id}/wait`; `GET /v1/grants`, `DELETE /v1/grants/{id}` | W19, wait in W23 |
| Agent | `GET /v1/agent/status` (Claude Code found, version, signed in; live chats); `GET /v1/agent/chats?project=<id>\|everywhere\|all`, `POST /v1/agent/chats` (project or none), `GET /v1/agent/chats/{id}` (messages, steps, pending approvals, queue), `PATCH /v1/agent/chats/{id}` (title), `DELETE /v1/agent/chats/{id}`; `POST /v1/agent/chats/{id}/messages` (text and the open tab), `POST /v1/agent/chats/{id}/stop`; `POST /v1/agent/steps/{id}/undo`; on `agent.sock` only: `GET /v1/agent/tools`, `POST /v1/agent/tools/{name}` | W32 |
| Connections | `GET /v1/connections`, `POST /v1/connections/{id}/connect`, `DELETE /v1/connections/{id}` | W19, Gmail in W22 |
| Code sessions | `GET/POST /v1/code/sessions`, `GET /v1/code/sessions/{id}`, `GET .../transcript`, `GET .../changes`, `GET .../changes/diff?path=`, `POST .../prompt`, `/control`, `/answer`, `/mark`; `GET /v1/code/spaces`, `POST /v1/code/spaces/{id}/terminal`, `GET /v1/code/terminals/{t}/view` (WebSocket), `POST /v1/code/terminals/{t}/send`, `/put`; `GET /v1/code/folders?path=`, `GET /v1/code/recent-folders`, `GET /v1/code/conversations?dir=`, `POST /v1/code/transcribe`; `POST /v1/code/hook` (hook socket only); `GET /v1/code/events` (remote socket only) | W27 |
| Remote | `GET /v1/remote/status` (Tailscale, Serve, URL, certificates), `POST /v1/remote/pair-code`, `GET /v1/remote/devices`, `DELETE /v1/remote/devices/{id}`, `POST /v1/remote/hooks` (main socket only) | W27 |

New SSE events: `agenda_changed`, `cue_fired`, `cue_updated`, `run_changed` (run id and status),
`approval_requested`, `approval_decided`, `connection_changed`, `memory_changed`, `code_session_changed` (session id
and state), `remote_changed`, and from W32 `agent_changed` (chat id, state, queue length), `agent_text` (chat id,
message id, the new text; at most one every 50 ms per chat), and `agent_step` (chat id and the step).

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

### 3.12 Memory

Gwen remembers in her own store. A memory answer names the evidence it rests on, and you can inspect, edit,
hide, reject, export, or erase it. A model or an external memory service never becomes the authority for what
Gwen believes about you.

```text
chat message · note · resource · memory-file edit
                  │
                  ▼
      episodes: verbatim, append-only evidence
                  │ background learning
                  ▼
 claims: keyed facts with time, trust, source span, and status ──► conflicts for review
                  │
                  ├──► FTS5 and vector index       derived, local, rebuildable
                  ├──► Mem0                         optional shadow lane
                  ├──► Supermemory                  optional document lane
                  └──► ~/Gwen/memory/*.md           readable projection

question ──► claims, FTS, vectors, time window, and typed Gwen data ──► compact cited note
```

**Evidence and claims.** An episode preserves words Gwen may need to cite. A claim is a short, keyed statement
about a person, place, preference, or fact. It has a source reference (`msg:<id>` or `ep:<id>`), its supporting
span, a confidence, a sensitivity level, and a trust level: `owner_edit`, `owner_said`, `observed`, `inferred`,
or `external`. Existing tasks, plans, calendar events, and time records stay in their typed tables. Memory
links can point from an episode or claim to those records without copying them into prose.

**Time and contradictions.** `valid_from` and `valid_to` describe when a claim was true. `recorded_at` and
`superseded_at` describe when Gwen learned or retired it. A newer unambiguous value for a single-valued key
supersedes the older claim. When the order or meaning is unclear, Gwen creates an open conflict instead of
guessing. External and inferred claims stay candidates or quarantined until corroborated or confirmed. Gwen's
own chat reply cannot make an active claim without confirmation.

**Read path.** The router first looks up a keyed claim when it can identify one. It then queries FTS, the vector
lane, the relevant time window, and typed records in parallel. It filters results by supersession, trust,
sensitivity, and an optional "as of" time before it builds the memory note sent with a chat turn. The note has
short citations. If no source supports a past fact, Gwen says that she does not know it.

**Lanes and operations.** The native claims and FTS lanes work without any sidecar. The vector lane, Mem0,
Supermemory, and memory files are optional. Mem0 receives verified claims only and stays off unless its recall
improves Gwen's benchmark. Supermemory receives resources and document evidence, never unique data. Background
jobs learn, embed, project, consolidate, and rebuild. A failed optional lane does not prevent native recall.

**Privacy and control.** Sensitive claims do not sync. The user can review conflicts, explain any reference,
rebuild a derived lane, export all memory, and erase all memory. Erasure removes the words from episodes and
claims while preserving only the hashes required to avoid learning the same erased fact again. The editable
Markdown projection comes back as `owner_edit`. Any sidecar runs local and loopback-only with telemetry disabled;
hosted memory is an explicit opt-in for a namespace.

**Delivery checks.** The memory layer is evaluated with GwenBench cases for updates, "as of" questions,
abstention, cross-record recall, non-ASCII text, poisoned external content, and degraded mode. Track recall p95,
write cost, citation coverage, and poisoned-write acceptance. A derived lane stays enabled only when an ablation
shows a measurable improvement.

### 3.13 Processes and memory

| Process | When it runs | Change |
|---|---|---|
| `gwend` | always | adds the cue loop, the run executor, connector clients, and the memory worker; models load only for a memory job |
| `gwen-ui` | the Gwen window while it is open; the now card, the cue card, and Talk; `gwen-ui host` while Settings asks about voice | the Gwen window holds every screen and the workbench (3.15) |
| `gwen-ui --cue` | a few seconds to 2 minutes per cue | new |
| `gwen-ui --talk` | when you talk | can start from a cue |
| `claude` and `gwen mcp` | only during a coding run | new |
| panel widget or `gwen-tray` | always (already) | launches the cue card |
| herdr-core (`hc server`) | while the Gwen window is open or any terminal runs | new (W27), built from upstream source |
| `gwen-term` (Gwen Terminal) | only if you open it yourself | new (W27); no longer the Gwen app (3.15) |
| `gwen hook` | a few milliseconds per Claude Code event | new (W27) |
| `gwen-stt` | while you talk from the phone, and 3 minutes after | new (W27), loads Parakeet |

### 3.14 Gwen Terminal and Gwen on the phone (W27)

You start and drive Claude Code sessions on this laptop from your phone, by text or voice, watch their live
terminal, and review what they changed. On the laptop they run in **Gwen Terminal**, a thin fork of the open-source
Herdr app that keeps Herdr's whole interface, so every Claude you start there can be reached from the phone, and when
you sit down you keep going in the same tab. The phone app follows Herdr's phone app screen for screen (Inbox,
Terminal, Compose, Spaces) and adds what Gwen brings: Chat, Changes, permission answers, New session, and voice. The terminals live in **herdr-core**, which keeps them running when the window closes. Everything
travels over your tailnet. Nothing goes through Claude Code Remote Control, the Claude app, or any Herdr service
(Claude Code itself still talks to Anthropic's API, as it does today). Ghostty stays your personal terminal; Gwen
ignores it. Mockups: the "Gwen Phone Remote" canvas, `https://claude.ai/artifact/DVWYtw3MvBaGxjoTpBpvda` (private to
you). The text below decides; the canvas only shows it.

**3.14.1 Names**

Every app is called **Gwen** wherever you see it. Internal names keep the parts apart in code, folders, branches,
and this file:

| Internal name | What it is | What you see |
|---|---|---|
| `gwend` | the daemon | nothing |
| `gwen` | the CLI | `gwen` |
| `gwen-ui` | **the Gwen app**: with no flag the Gwen window (3.15), with `--now`, `--talk`, `--cue` the small windows | window title "Gwen"; the only "Gwen" in the launcher |
| `gwen-tray`, the panel widget | the tray | "Gwen tray" |
| `gwen-term` | a thin fork of `herdr-app` (3.14.3): Herdr's own window on Gwen's core. Since the new W16 nothing opens it; it stays as a second view of the same terminals | "Gwen Terminal" (its launcher entry is hidden) |
| `gwen-mobile` | the phone app | "Gwen" on the phone's home screen |
| `herdr-core` (`hc`) | the terminal server, built from upstream unchanged | nothing |
| `gwen-stt` | the speech-to-text helper | nothing |

**3.14.2 How the pieces fit**

```text
phone: gwen-mobile ── tailnet HTTPS ──▶ Tailscale Serve ──▶ gwend remote.sock
                                                              │
                                                              ▼
laptop: gwen-term (window) ◀── attach ──▶ herdr-core ◀── JSON-RPC ── gwend (sessions, transcripts, changes, voice)
                                            │ terminals: claude, shells                ▲
                                            └── Claude Code hooks (gwen hook) ── hook.sock
```

- herdr-core runs the terminals, `gwen-term` draws them, and `gwend` is one more client of herdr-core. `gwend` never
  draws a terminal.
- **The phone never reaches herdr-core.** Anyone who can use its socket effectively has a shell (herdr-core
  `docs/security.md`), so `gwend` exposes only the operations of 3.14.7, each with its own checks.
- Claude Code reports its own status (working, blocked, idle, done, error) to herdr-core through OSC 7501; 2.1.322
  checks for it at start. Hooks add what a status cannot carry: the Claude session id, permission details, the last
  message, and edited files.

**3.14.3 `gwen-term`: the thin fork**

- **Repository.** A separate repo, `~/Documents/coding/gwen-term`, cloned from `github.com/herdrdev/herdr-app` with
  that remote named `upstream`, and a branch `gwen` on top of `upstream/master`. A GitHub fork under your account (as
  `origin`) is made only when you ask. Gwen's repo never vendors it.
- **The patch stays small.** Each item is its own small commit, so a rebase conflict names its reason:
  1. App id, launcher entry, and installed command `gwen-term` (`Name=Gwen`, `GenericName=Terminal`), with the Gwen
     icon from `packaging/icons`. `Cargo.toml` is left exactly as upstream has it (Herdr's licence-notice test
     hashes it), so Cargo still names the binary `herdr-app`; `scripts/install-linux` installs it as `gwen-term`.
  2. App home `~/.config/gwen-term` (the `HERDR_APP_HOME` override still works).
  3. No calls to herdr.dev: update checks, the core download and update, update-notes media, feedback, and usage
     reports are all off behind one constant, `gwen::UPSTREAM_SERVICES` in `src/gwen.rs`.
  4. `LICENSE`, `THIRD-PARTY-NOTICES`, and `licenses/` stay, and the README's first lines say this is a modified
     Herdr (Apache-2.0 section 4).
  5. `scripts/gwen-rename`, and the one large commit it **generates**: user-visible "Herdr" becomes "Gwen" and the
     command in help and docs becomes `gwen-term` (about 330 lines in 46 files). Because it is generated, it never
     needs a conflict resolved: `make term-update` drops it, rebases the small commits, and runs the script again.
- **Kept unchanged, so community layouts and mods keep working:** the JavaScript module name `herdr`, the core
  document `herdr-app`, the `herdr.space` and `herdr.tab` labels, the `HERDR_*` environment variables, the layout and
  mod formats, and the generated `AGENTS.md` (its paths follow the new home).
- **Everything else is Herdr's** (3.14.12). Gwen's own files live in Gwen's repo under `term/` and are never patched
  into the fork: `core.version`, the `programs.json` entry of 3.14.12, and `themes.json` (Herdr's built-in theme
  colors, for the phone's terminal).
- **herdr-core is not forked, and Gwen has her own.** Gwen builds it from source at the newest release tag that
  `gwen-term` accepts, pins that tag in `term/core.version`, and installs it as `~/.local/lib/gwen/herdr-core`, never
  in `~/.local/bin`. It runs as the core **session `gwen`** (`HC_SESSION=gwen`): its own socket
  (`$XDG_RUNTIME_DIR/herdr-core/gwen.sock`), state, terminals, and `herdr-app` document. `gwen-term` sets both at
  startup (`gwen::own_core`). So Gwen Terminal and a Herdr you run for yourself never share terminals, spaces, or a
  core restart, and `make term` never touches your Herdr. `hc update` is never used.
- **Building** needs Rust (installed), Zig 0.16, and `just` (setup.md adds them). `make term` builds both and
  installs `gwen-term`, its desktop entry and icon, `hc`, and merges the `programs.json` entry (3.14.12). It refuses when the
  `gwen-term` checkout has uncommitted changes. `make term-update` fetches upstream, drops the generated commit, rebases the
  small ones, regenerates it, runs `just check` (Herdr's 78 script tests, 411 unit tests, and its notice check all
  pass on the `gwen` branch), and stops on any conflict for you to resolve. Both targets run `scripts/term.sh`. Updating while terminals run is safe: herdr-core keeps them.

**3.14.4 How the phone reaches the laptop**

- With `[remote] enabled = true`, `gwend` listens on a second Unix socket, `$XDG_RUNTIME_DIR/gwen/remote.sock`
  (mode 0600). Tailscale Serve publishes it to the tailnet only: `tailscale serve --bg --https=8443
  unix:<socket>`. The phone opens `https://<machine>.<tailnet>.ts.net:8443`. Port 8443 leaves anything you serve on
  443 alone. No Funnel, no TCP listener.
- **Turning it on** is a button in Settings › Phone. `gwen-ui` runs that command through `pkexec` (one password
  prompt; Tailscale needs root to serve a Unix socket); `gwen remote on` does the same from a terminal with `sudo`. Off runs `pkexec tailscale serve --https=8443 off`. `gwend`
  never runs `tailscale` as root; it only reads `tailscale status --json` and `tailscale serve status --json`.
- **HTTPS certificates** must be on in the tailnet (`CertDomains` in `tailscale status --json` is not empty). If
  not, the setup row says so and its button opens `https://login.tailscale.com/admin/dns`. The row also says that
  the laptop's name appears in public certificate transparency logs.
- **Who may call.** Every request on the remote socket passes all four checks, or gets 401 (fail closed):
  1. The peer of the connection (`SO_PEERCRED`) is uid 0, which is `tailscaled`. Any other peer is closed.
  2. `Tailscale-User-Login` equals the owner login, stored once when you first turn the remote on (from
     `Self.UserID` → `User[id].LoginName` in `tailscale status --json`) under the new local state key
     `remote_owner`. Serve strips copies of this header sent by the client and sends none for tagged devices.
  3. A `gwen_device` cookie whose SHA-256 matches a `remote_devices` row that is not revoked. The cookie is
     `Secure`, `HttpOnly`, `SameSite=Strict`, `Max-Age` 400 days. `last_seen_at` updates at most once a minute.
  4. Every POST, PUT, and DELETE carries the header `X-Gwen: 1`.
  `/pair` (the page and its POST) skips check 3 only.
- **Pairing.** Settings › Phone shows a QR code for `https://<host>:8443/pair#<code>`: 128 random bits in base32,
  single use, valid 10 minutes, one live code at a time. The phone posts the code with a device name (taken from the
  user agent, editable). `gwend` stores the SHA-256 of a new 256-bit token and sets the token as the cookie.
  **Revoke** deletes the row; that phone's next request gets 401 and shows the pair page.
- **Route lists.** The remote socket serves only the phone app (`ui/dist-mobile`), `/pair`, `/v1/code/*` except
  `/v1/code/hook` and the Gwen window's own `/v1/code/usage` and `/v1/code/commit-sessions` (3.15), and `GET
  /v1/code/events`. Everything else is 404. The main socket serves the same `/v1/code/*` routes for the dashboard and
  CLI (local owner), those window routes, and `/v1/remote/*`. The hook socket serves only
  `POST /v1/code/hook` (3.14.6).
- **Later.** No passkeys in W27. Harness G4 moves the transport to tsnet and replaces the device cookie with passkeys
  (`gwen-harness/harness.md` 5.2 and 5.6).

**3.14.5 Sessions on herdr-core**

- **Connection.** `internal/herdr` speaks herdr-core's JSON-RPC 2.0 on the socket of Gwen's own core
  (`$XDG_RUNTIME_DIR/herdr-core/gwen.sock`, or `[remote] core_socket`), in framed mode. It never connects to the
  default core, which belongs to a Herdr of your own. `hello` must report
  protocol generation 1 and a version at least the one in `term/core.version`; otherwise Settings › Phone says
  "Gwen Terminal's core is out of date. Run make term." `gwend` reconnects after 1, 2, 5, then every 10 seconds and
  takes a fresh snapshot after each reconnect. When a session is needed and no core runs, `gwend` starts one with
  `systemd-run --user --collect -p Type=oneshot -p RemainAfterExit=yes -- env HC_SESSION=gwen
  ~/.local/lib/gwen/herdr-core server start`, so it gets your desktop session's environment, not the service's, and
  the core it leaves in the background outlives the command and every window.
- **What a code session is.** Any live herdr-core terminal on this laptop whose status record reports `app:
  "claude-code"`, or whose foreground program's `argv[0]` base name is `claude`. `gwend` follows an unfiltered
  `events.subscribe`, gives each one a `cs_` id, writes `code_sessions`, and labels the terminal
  `gwen.session=<id>` (`terminal.label`), so it keeps the same card after `gwend` restarts. Claude running in
  Ghostty, or in any terminal outside herdr-core, is not seen.
- **States**, kept in memory and written to `code_sessions` on every change:

  | State | Set by |
  |---|---|
  | `starting` | the terminal was started for a session and has no status yet |
  | `working` | status `working` |
  | `needs_you` | status `blocked`. `need` says why: `permission` (a held or shown `PermissionRequest`), `question` (`AskUserQuestion`), or `other` |
  | `done` | status `done`, or `working` → `idle` while nobody looked. Becomes `idle` when you open it on the phone or send it a message |
  | `idle` | status `idle` |
  | `error` | status `error` |
  | `ended` | Claude's process is gone (status cleared and the foreground is no longer Claude), or the terminal finished or was removed |

  `step` is a short line from the last `PreToolUse`: `Running <command, first 80 characters>`, `Editing <path>`,
  `Reading <path>`, `Searching`, otherwise the tool name. `last_message` is the first 300 characters of `Stop`'s
  `last_assistant_message`. `/clear`, `/resume`, and compaction bring a `SessionStart` with a new Claude session id
  in the same terminal; the card stays and follows the new id.
- **Starting a session** (from the phone or the Gwen window), with **Open a window on the laptop** on (the default):
  `gwend` starts the terminal in herdr-core exactly as without a window (below), then runs `gwen-ui --open
  session:<id>`, which shows the session's tab in the Gwen window, starting the window if it is closed (3.15). This
  is the one exception to "`gwend` never launches windows" (3.6). Before W16b it ran `gwen-term do tab.new`.
- **Claude's own first question.** In a folder Claude Code has not been told to trust, it asks before it reports any
  status. A new session that is still silent after 4 seconds becomes `needs_you` (`other`) with "Answer in the
  Terminal tab or on the laptop", so it never sits at Starting.
- **Without a window** (the switch off): `terminal.run {argv, cwd, labels: {"gwen.session": <id>}}` in herdr-core.
  **Open on laptop** (the phone's ⋯ menu) runs `gwen-ui --open terminal:<address>` for its terminal, which opens or focuses its tab.
- **Ending.** The `end` control presses Escape when working, then sends `/exit`. If Claude's process is still alive
  10 seconds later, `gwend` sends it SIGTERM. The tab and its shell stay; closing the tab is yours.
- **Closing the window** leaves every terminal running. Closing a tab ends its terminals (Herdr's rule). Nothing ends
  a session on a timer.
- **Ended** sessions stay listed for 24 hours. **Resume** starts a new session in the same folder with the original
  flags, the same Normal or Sandbox (the Resume sheet can switch it), and `--resume <Claude session id>`. When a new
  session reports a Claude session id that an ended row already has, that ended row is dropped from the lists, so one
  conversation keeps one card.
- **One Claude per conversation.** Resume or Continue (3.14.7) for a conversation that is running in any session is
  refused with `running` and its session id, and the phone opens that card instead.
- **Restart.** After `gwend` restarts, the snapshot brings back every terminal labelled `gwen.session` under its old
  row; other Claude terminals get new rows. After a reboot, herdr-core gives kept tabs a fresh shell but never restarts
  their programs, so those sessions are `ended` and offer Resume.

**3.14.6 Hooks**

- Settings › Phone, **Let Gwen see my Claude sessions** (`POST /v1/remote/hooks {on}`, or `gwen remote hooks on`),
  adds one entry per event to the `hooks` of
  `~/.claude/settings.json`: `SessionStart`, `PreToolUse`, `PostToolUse` (matcher
  `Edit|Write|MultiEdit|NotebookEdit`), `PermissionRequest`, `Stop`, `SessionEnd`. Each entry is `{"type":
  "command", "command": "<absolute path of gwen> hook", "async": true}`, except `PermissionRequest`, which is
  synchronous with `"timeout": 900`.
- **Merging.** Read the file; keep every other top-level key in its order and every other hook entry unchanged;
  rewrite only `hooks`; skip an event that already has Gwen's exact command; write to a temporary file and rename.
  The first write keeps a copy at `settings.json.gwen-backup`. Off removes only entries whose command is
  `<gwen path> hook`. If the file is not valid JSON, change nothing and say so. If `disableAllHooks` is true, the
  switch shows "Claude Code hooks are turned off in your settings". The switch shows the file's real state and
  re-reads it whenever Settings › Phone opens.
- **`gwen hook`** exits 0 at once unless `HC_TERMINAL_ID` is set and `HC_TERMINAL_SOCKET` is Gwen's own core, so
  Claude outside Gwen Terminal, including in a Herdr of your own, costs nothing and is never reported. It reads
  the event from stdin (at most 1 MiB) and keeps only what Gwen uses: event name, session id, transcript path, cwd,
  permission mode, tool name, a summary of the tool input (command, first 200 characters, or file path),
  `last_assistant_message` (first 300 characters), `source`, `reason`, and, for `PermissionRequest` and
  `AskUserQuestion`, the full tool input up to 8 KiB and `permission_suggestions`. It never sends prompt text. It adds
  `HC_TERMINAL_ID` and its parent pid (Claude's pid), and posts to the hook socket with a 500 ms timeout. It prints
  nothing and exits 0, always, including when `gwend` is down. The one exception is `PermissionRequest` (3.14.7).
- **The hook socket**, `$XDG_RUNTIME_DIR/gwen/hook.sock` (mode 0600), is separate so that a sandbox can be given it
  without the main API (3.14.8).
- **Trust.** An event is accepted for terminal `t` only if the sending process (`SO_PEERCRED` pid) descends from the
  pid of `t`'s program in herdr-core's terminal record. Otherwise it is dropped. Each sending pid may post at most 50
  events per second; extra events are dropped.

**3.14.7 Driving a session from the phone**

- **Prompt**: `POST /v1/code/sessions/{id}/prompt {text, idem}`, text up to 32 KiB, becomes `terminal.send`.
  herdr-core pastes it and presses Enter only after you send or clear anything you are typing in that terminal at the
  laptop (at most 60 seconds after your last key), otherwise after about 3 seconds of quiet, and holds keys you press
  meanwhile. While it waits, the phone shows "Waiting for you to finish typing on the laptop".
  - Refused with `conflict` ("Answer Claude first") in `needs_you`. Gwen never forces a prompt into a blocked Claude.
  - The same `idem` (a client UUID) within 10 minutes returns the first result without sending again.
  - The answer is `sent` when the state was `idle` or `done`, or `queued` when it was `working` (Claude Code queues
    it).
- **Any terminal, as in Herdr's phone app.** The phone works on every terminal in Gwen Terminal, not only Claude:
  - `GET /v1/code/spaces` lists Gwen Terminal's spaces, their tabs, and each tab's panes with their terminals,
    names, and statuses. `gwend` reads them from herdr-core's `herdr-app` document (`doc.get`, then `doc` events).
    The format belongs to Herdr, so the parser is tested against a recorded document and skips what it doesn't know.
  - `POST /v1/code/spaces/{space}/terminal` opens a new shell there: from W16b a space is a project, and the shell
    starts in its folder with the label `gwen.project=<id>` (before, `gwen-term do tab.new --space <id>`).
  - `POST /v1/code/terminals/{t}/send {text, idem}` is Compose: the same `terminal.send` as a prompt, with the same
    `idem` rule, for any terminal.
  - `POST /v1/code/terminals/{t}/put?name=` (one file as the body, at most 8 MB, herdr-core's limit) is a photo or
    screenshot from Compose:
    `terminal.put` saves it beside the terminal and returns where it landed, and the phone adds that path to the text.
- **Controls**: `POST /v1/code/sessions/{id}/control {action}` with `interrupt` (`terminal.interrupt`), `cycle_mode`
  (Shift+Tab; the phone shows the mode from the last hook's `permission_mode` in words: Ask before edits, Accept
  edits, Plan, Auto), `compact` (sends `/compact`), `clear` (sends `/clear`; the phone confirms first), `end`
  (3.14.5), and `open_on_laptop` (3.14.5).
- **Inbox marks**: `POST /v1/code/sessions/{id}/mark {pinned, seen}` sets `pinned`, and `seen_at` to now.
- **Away** means the laptop is locked or has had no input for 2 minutes, from the activity monitor `gwend` already
  runs.
- **Permissions while you are away.** The `PermissionRequest` hook call stays open: `gwend` holds the request. (Claude
  Code shows its own dialog on the laptop at the same time; whichever answers first wins, and an answer at the laptop
  ends the held call.) The phone's answer becomes the hook's output, wrapped as `{"hookSpecificOutput":
  {"hookEventName": "PermissionRequest", "decision": …}}`: Allow once is `{"behavior": "allow"}`; Always allow adds
  `"updatedPermissions": [<the suggestion you picked>]`, with each entry of `permission_suggestions` shown in words;
  Deny is `{"behavior": "deny", "message": <your text, or "Denied from your phone">}`. If you come back to the
  laptop first, or 10 minutes pass, `gwend` answers the hook with no decision, so Claude Code shows its own dialog on
  the laptop, and the phone shows "Moved back to the laptop".
- **Permissions while you are at the laptop.** The hook returns at once with no decision and the dialog shows on the
  laptop. The phone can still answer: `gwend` reads the screen with `terminal.read`, builds the phone's buttons from
  the numbered option lines it finds (`1.`, `2.`, …), and presses the number you pick with `terminal.keys`. It reads
  again 2 seconds later; if the dialog is still there, the phone says "Didn't go through. Answer on the laptop."
- **Questions** (`AskUserQuestion`): for a single question with single choice, the phone lists the options from the
  tool input and presses the chosen option's number, checked the same way. Multi-select or several questions show
  "Answer in the Terminal tab or on the laptop".
- **Terminal view**: `GET /v1/code/terminals/{t}/view` upgrades to a WebSocket. `gwend` opens `terminal.attach
  {observe: true, resize_notifications: true, history: 1000, cols, rows}` and relays the stream: output bytes as
  binary messages, a resize as the text message `{"cols": c, "rows": r}`, and the end as `{"closed": reason}`. It
  returns credit to herdr-core as it writes to the phone; a phone more than 4 MiB behind gets a fresh paint from
  herdr-core instead of the backlog. The phone draws it with xterm.js at the laptop's grid size, fitted to the
  screen's width, with pinch to zoom, in Gwen Terminal's colors (3.14.13).
  - **Typing.** Tapping the view opens the phone's keyboard. What you type goes back over the same WebSocket as
    `{"text": "…"}` (at most 4 KiB a message and 50 messages a second) and is typed as it is, symbols and all.
    herdr-core takes typed bytes only from a viewer that may type, so on the first typed character `gwend` opens a
    second, typing attach whose size is the terminal's own grid (kept in step with the grid the observer is told):
    a viewer that asks for the size the terminal already has changes nothing, so the laptop's tab is never squeezed
    to phone width. `{"keys": [...]}` (at most 32 names) still presses named keys through `terminal.keys`.
  - **Key bar** above the keyboard: Ctrl (held for the next key), Esc, Tab, ⇧Tab, the four arrows, and Enter. Its
    keys go the same way as typed text, in order with it, encoded on the phone from the modes xterm.js follows:
    herdr-core holds keys pressed through its API until 3 seconds after anyone stops typing in that terminal, which
    would make Enter after a typed command wait. Reordering the bar comes with W29.
  - Each paired phone may hold 2 views open. If the first live check shows that Serve does not pass WebSocket
    upgrades, output goes over SSE with base64 chunks and keys over POST.
- **New session**: `POST /v1/code/sessions {dir, create, git_init, sandbox, prompt, window, resume}`. `dir` must
  resolve (after symlinks) inside one of `[remote] roots` (default `["~"]`), not inside a hidden folder or Gwen's own
  config or data folders. `create` makes only the last path element, and `git_init` runs `git init` in it. `prompt`
  becomes Claude's first message.
- **Continue a conversation**: `GET /v1/code/conversations?dir=` lists the newest 15 conversations in that folder:
  the transcripts in the `~/.claude/projects` folder whose first lines name `dir` as their `cwd` (found the same
  way as recent folders), each with its Claude session id, its name (the newest custom title in the transcript)
  or else its first message (first 120 characters), last active time, whether it last ran sandboxed (from
  `code_sessions`), and the session id when it is running now. `POST /v1/code/sessions` with `resume: <Claude
  session id>` starts it with `--resume`, in the mode it last ran in unless `sandbox` is given.
- **Folders**: `GET /v1/code/folders?path=` lists subfolders (no files, no hidden folders) with `git` and `branch`.
  `GET /v1/code/recent-folders` returns the working folder of the newest transcript in each `~/.claude/projects/*`
  folder (read from its first lines), the newest 12 that still exist inside a root.

**3.14.8 Sandbox (bubblewrap)**

Chosen per session: "How it runs" on New session (Normal is the default), or **Claude in a sandbox** in Gwen
Terminal's + menu, which runs `gwen sandbox [claude arguments]` in the new tab. **Normal** is exactly your own
`claude` with your own permission rules. **Sandbox** runs Claude under this command, then the arguments of 3.14.5:

```sh
bwrap --die-with-parent --unshare-pid --unshare-ipc --unshare-uts \
  --ro-bind /usr /usr --symlink usr/bin /bin --symlink usr/sbin /sbin --symlink usr/lib /lib --symlink usr/lib64 /lib64 \
  --ro-bind /etc /etc --ro-bind-try /opt /opt --proc /proc --dev /dev --tmpfs /tmp --tmpfs /var/tmp \
  --tmpfs /run --ro-bind-try /run/systemd/resolve /run/systemd/resolve \
  --tmpfs "$HOME" --bind "$DIR" "$DIR" \
  --bind "$STATE/claude" "$HOME/.claude" --setenv CLAUDE_CONFIG_DIR "$HOME/.claude" \
  <read-only from your real ~/.claude, each that exists: settings.json CLAUDE.md commands agents skills plugins keybindings.json output-styles rules> \
  --bind "$HOME/.claude/projects/<this folder>" "$HOME/.claude/projects/<this folder>" \
  --ro-bind "$DIR/.git/config" "$DIR/.git/config" --ro-bind "$DIR/.git/hooks" "$DIR/.git/hooks" \
  --ro-bind "$GWEN_BIN" "$GWEN_BIN" --dir "$XDG_RUNTIME_DIR/gwen" --bind "$HOOK_SOCK" "$HOOK_SOCK" \
  <read-only, each --ro-bind-try: ~/.local/bin ~/.local/share/claude ~/go ~/.cargo/bin ~/.rustup ~/.nvm ~/.bun ~/.deno ~/.gitconfig ~/.config/git> \
  <caches, each --overlay-src <yours> --overlay "$STATE/cache/<name>/upper" "$STATE/cache/<name>/work" <yours>:
     ~/go/pkg/mod ~/.cache/go-build ~/.npm ~/.cache/pip ~/.cargo/registry ~/.local/share/pnpm> \
  <each [remote] sandbox_extra entry, --ro-bind or --bind> \
  --unsetenv SSH_AUTH_SOCK --unsetenv DBUS_SESSION_BUS_ADDRESS --unsetenv WAYLAND_DISPLAY --unsetenv DISPLAY \
  --unsetenv HC_SOCKET --unsetenv HC_SESSION --setenv GWEN_SANDBOX 1 --chdir "$DIR" -- "$CLAUDE"
```

- The network stays on, because Claude Code needs Anthropic's API. Egress limits come with harness G3's proxy.
- The sandbox cannot reach herdr-core's socket (`/run` is empty inside, so `hc` there could only start a separate core
  of its own), the main API, SSH keys, the keyring, other folders, or your desktop. It can push no git commits over
  SSH, and image paste does not work in it.
- **Nothing a sandboxed session writes takes effect outside by itself.** `$STATE` is
  `~/.local/share/gwen/sandbox/<folder name>-<hash>`, private to that project folder.
  - **Claude's own folder** inside the sandbox is `$STATE/claude`, never your real `~/.claude`. Your settings,
    instructions, commands, agents, skills and plugins show read-only, so your hooks (Gwen's among them) still run,
    but a sandboxed Claude cannot add a hook, a command or an MCP server to your own Claude, cannot read your other
    conversations, and never sees your sign-in. Its `~/.claude.json` is a copy of yours made at each start, without
    your other projects, MCP servers and account, plus what earlier sandboxed sessions of this project stored; it
    never flows back. Only this folder's conversations live in your real `~/.claude/projects`, so the phone's Chat
    and Resume work across Normal and Sandbox.
  - **Sign-in is separate.** Claude Code rewrites its sign-in file by renaming, which cannot be shared file by file
    without sharing the whole folder, so a sandbox has its own: the token from `claude setup-token`, stored once as
    the credential `claude_sandbox_token` (`gwen sandbox-login`, or Settings › Phone) and passed in the environment,
    never on a command line. That token can only talk to the model, so the sandbox has no Claude connectors.
    Without it, each sandboxed project asks for `/login` once.
  - **Tool caches** are yours underneath, read-only, with a private writable layer on top (bubblewrap 0.11 or newer;
    older versions get a private empty cache), so nothing a session leaves there reaches your own builds.
  - **The project's `.git/config` and `.git/hooks`** are read-only: git runs them by itself the next time you use
    the repository, and Changes would not show them. `git commit` works; `git config` and `git remote add` do not.
  - What Claude writes in the project itself (code, scripts, build files) is yours to review in Changes before you
    run it; no sandbox can vouch for that.
- **Starting.** `gwend` starts a sandboxed session as `gwen sandbox <claude arguments>` in the folder, the same
  command the agent menu runs. `gwen sandbox` prepares `$STATE`, reads the sign-in, and replaces itself with the
  `bwrap` command above. Settings › Phone says all of this under the Sandbox option.

**3.14.9 Transcript and changes**

- **Transcript.** `gwend` reads the session's transcript file (the path from its hooks) and turns its lines into
  events: `you` (prompt text, images as "[image]"), `claude` (Markdown), `tool` (name, a one-line summary, ok or
  error, +/− counts for edits, and for a failed Bash call its last 40 output lines), and `notice` (compaction,
  interruption, error). Thinking and other tool output are left out. Unknown line types are skipped: the format
  belongs to Claude Code, so the parser is tested against recorded fixtures from 2.1.322.
  `GET /v1/code/sessions/{id}/transcript?before=<cursor>&limit=50` pages back, and `?after=<cursor>` gets new
  events; a cursor is a line's byte offset. `gwend` follows the file only while a phone or dashboard stream is open
  (it checks the size every second).
- **Changes.** In a git folder, `GET /v1/code/sessions/{id}/changes` lists each changed file against `HEAD` (or the
  empty tree before the first commit): path, status (added, modified, deleted, renamed, untracked), +/−, binary, and
  `before_session`, true when the file was already changed in the `git status` snapshot taken at `SessionStart`.
  `GET .../changes/diff?path=` returns the unified diff (`git diff --no-index /dev/null <path>` for untracked files),
  cut at 2,000 lines with `truncated: true`; binary files return their size only. Git runs with
  `GIT_OPTIONAL_LOCKS=0` and a 10 s timeout, so it never competes with Claude's own git calls. Outside git, the list
  is the files from `PostToolUse` edit events, without diffs.

**3.14.10 Voice**

- The phone records while you hold the mic button (`getUserMedia` and an AudioWorklet), converts to 16 kHz mono
  16-bit PCM in the browser, stops at 60 s, and posts it raw to `POST /v1/code/transcribe`.
- `gwend` hands it to `gwen-stt`, a new small cgo binary that uses `internal/voice` (Parakeet through sherpa-onnx, the
  same models folder as `gwen-ui`). Requests on its stdin are a 4-byte little-endian length plus the PCM; each reply
  is one JSON line, `{"text": "…"}` or `{"error": "…"}`. `gwend` starts it on first use and closes it after 3 minutes
  unused, so the model takes memory only while you talk, and `gwend` stays CGO-free.
- The text lands in a review sheet, editable. Voice never sends, approves, or answers anything by itself (section 7,
  rule 3). Without models: "Voice isn't set up on the laptop. Open Gwen › Settings › Voice."

**3.14.11 Alerts and keeping awake**

- When a session goes to `needs_you`, `error`, or `done`, and you are away, `gwend` sends one ntfy push through the
  existing `[ntfy]` settings. Title: the folder name. Message: "needs permission", "has a question", "needs you",
  "stopped with an error", or "finished". Click URL: `https://<host>:8443/s/<id>`. A push never contains a prompt,
  command, path, or code, because `ntfy.sh` is a public server. At most one push per session every 30 s. Switch:
  **Push to my phone when I'm away**.
- `[remote] keep_awake = "working"` (default; also `"working_lid"` and `"never"`): while any session is `working` or
  `needs_you`, `gwend` holds a logind `sleep:idle` block inhibitor, plus `handle-lid-switch` for `"working_lid"`. It
  releases it when no session needs it, or when UPower reports the battery discharging below 15%.
- **Offline.** The service worker serves the app shell. The app shows "Can't reach <machine>. It may be asleep, or
  Tailscale is off on this phone." with the last list it saw and when.

**3.14.12 Herdr's interface, and making it yours**

- **Gwen Terminal keeps Herdr's whole interface**: its default layout, themes, agents sidebar, spaces, tabs and panes,
  command palette, Settings window, welcome, and shipped mods. Gwen changes none of them, so Herdr's improvements to
  them arrive with `make term-update`. Gwen adds one entry to the personal `programs.json`, which puts **Claude
  (sandbox)** in Herdr's own agent menus (3.14.8): `"claude-sandbox": {"name": "Claude (sandbox)", "icon":
  "icons/claude.svg", "run": ["gwen", "sandbox"]}`. `make term` merges it in and keeps your other entries.
- **Customising works exactly like Herdr**, with plain files in `~/.config/gwen-term` that reload when saved:
  **layouts** (one JSON file per window: a tree of rows, columns, stacks, and blocks, where a block holds a terminal,
  the active tab's panes, or a mod), **mods** (a folder with `main.js` and `mod.json`: JavaScript that describes
  panels, menus, and dialogs, drawn natively by the app; it reads live data such as spaces, agents, and terminals,
  runs app commands, and can run programs and fetch), **themes**, **keys**, and **settings**. A broken edit keeps the
  last good version and reports the error. The generated `AGENTS.md` in that folder explains all of it, so you can
  ask an agent in Gwen Terminal to change the window for you.
- **Community layouts and mods work unchanged** (about 300 layouts and 560 mods in Herdr's gallery), because the fork
  keeps the `herdr` module and the file formats (3.14.3). To add one, ask an agent in Gwen Terminal: "Read
  ~/.config/gwen-term/AGENTS.md, then add this mod: <its gallery link>". **A mod runs with your full access, and
  nobody reviews gallery code**: add only mods you or an agent have read first.
- **Gwen's own panels come later, as mods opened in a tab** (`tab.new {mod}`) or an overlay, never by editing Herdr's
  layouts: a personal copy of a layout stops receiving Herdr's updates (Herdr's rule), so Gwen never makes one.
- **Layouts, themes, keys, and mods are set in Gwen Terminal's own Settings window and command palette.** The
  dashboard does not duplicate them.
- **On the phone**, W27's screens are fixed. W29 brings layouts, mods, and themes to `gwen-mobile` in the same style
  (6.3, Later packages). So that it can, each W27 phone screen is a self-contained component that takes its data as
  props: Inbox, Terminal, Spaces, Chat, Changes, New session.

**3.14.13 Screens**

The phone app follows Herdr's phone app screen for screen, then adds Gwen's own. It is a second Vite entry,
`ui/mobile.html` → `ui/dist-mobile`, embedded by `ui/embed_mobile.go`, and installable to the home screen (manifest and
icons from `packaging/icons`). It looks like Gwen Terminal: its chrome follows `ui/DESIGN.md`, which W27 rewrites to
Herdr's look (5.1), and both chrome and terminal use Gwen Terminal's active theme: the theme named in `~/.config/gwen-term/settings.json`, read from `themes/<name>.json` there or from
`term/themes.json` for Herdr's built-in ones.

1. **Inbox** (home, as in Herdr): every agent in Gwen Terminal, sorted Needs you, Done, Working, Idle, with Ended (24
   hours) last. A blue dot marks one you haven't read. Helpers an agent started sit under it. Swipe right to pin or
   mark read; swipe left to press Esc. Pull down, or tap search, to find any terminal. Gwen adds to each Claude card
   the step or last message, changed files with +/−, and Allow once and Deny on permission cards. A + button opens
   New session. While the app is open, a banner appears when an agent needs you or finishes.
2. **Terminal** (as in Herdr): the live terminal, typed into directly, with the key bar (3.14.7). Swiping the header
   moves to the next pane in the same tab; the tab map shows all of its panes live. **Compose** opens a sheet for
   longer input: type, paste text or a screenshot, attach photos, or hold the mic; it is sent as one paste plus
   Enter.
3. **Spaces** (as in Herdr): Gwen Terminal's spaces, each tab drawn as a small map of its panes. Open one to watch its
   panes, or start a new terminal there.
4. **A Claude session** has three tabs: **Terminal** (first, as in Herdr), **Chat**, and **Changes**; the phone
   remembers the tab you used last. Its header shows the folder, path, branch, and Normal or Sandbox, and a ⋯ menu
   has Interrupt, Change mode, Compact, Clear, Open on laptop, and End session. **Chat** shows the transcript, the
   permission or question card inline, and a composer with the mode chip, mic, and send. **Changes** shows the
   before-session banner, totals against the short `HEAD` hash, files with expandable diffs, and chips "Run the
   tests", "Explain this diff", "Commit it" that only fill the composer.
5. **New session**: recent folders, Browse folders…, New folder…; once a folder is picked, **Conversation** lists New
   conversation and its recent conversations (name or first message, last active, a Sandbox tag; one running now
   shows Open); How it runs (Normal or Sandbox, one line each); a first message with mic; the switch Open a window on
   the laptop; and Start session or Continue conversation.
6. **Voice sheet**: the heard text to edit, Discard, Send to <folder>, and the hold-to-talk button.
7. **Pair**: a name for this phone and Pair.

Side-by-side sidebar and terminal on large screens comes later. On the laptop: Gwen Terminal is Herdr's interface
(3.14.12), and the dashboard has **Settings › Phone** (setup checklist: Gwen Terminal installed, hooks on, HTTPS
certificates, published, a phone paired; the QR code; the switches and pickers of 3.14.14; paired devices with
Revoke).

**3.14.14 Configuration**

```toml
[remote]
enabled = false
port = 8443
roots = ["~"]
keep_awake = "working"      # working | working_lid | never
push = true
sandbox_extra = []          # ["~/data", "~/out:rw"]: read-only, or writable with :rw
core_socket = ""            # "" = herdr-core's default socket
```

Every key except `core_socket` has a control in Settings › Phone. The owner login and paired devices live in the
database, not here.

**3.15 One Gwen app: Gwen's window with the workbench inside (W16, W16b)**

Rewritten 2026-11-13. The first W16 put Gwen's screens inside Herdr's window as a mod, so Herdr became the shell and
Gwen a guest; you rejected it. Gwen is the product and Herdr's workbench is one of her parts: **one app, called
Gwen**, which is Gwen's own window (`gwen-ui`, Wails and React) with your projects, terminals, files and git history
inside it, on the same herdr-core that keeps terminals running. Mockups: the "Gwen, one app" canvas,
`https://claude.ai/artifact/L3afH9bp5wkSjdCRS8MvSa` (private to you). Board 18 ("Where every feature lives") is
the checklist: every feature of the old dashboard, the tray, the panel widget, the CLI, and the phone has a place.
The text below decides; the canvas shows it.

- **One model.** A **project** is a Gwen project and a workbench at once: its folder on this laptop (device-local,
  `project_dirs`, 3.9), its terminals and Claude sessions, its tasks and goal, its tracked time, and its git
  history. Today is a tab, not a separate app. Gwen is a side panel, a voice, and cue cards, not a screen you visit
  to find her. Everything that waits for you is one **Needs you** queue.
- **The window.** `gwen-ui` with no flag is the Gwen window, with one instance (`dev.gwen.ui`). `gwen-ui --open
  <target>` shows a target: a screen (`today`, `inbox`, `gwen`, `plan`, `board`, `goals`, `review`, `weekly`,
  `stats`, `history`, `memory`, `settings`, `settings/<section>`, and the old names `assistant`, `projects`,
  `tasks`, `connections`), `project:<id>`, `session:<cs_id>`, or `terminal:<address>`. A second launch hands its
  target to the running window and exits. `--screen NAME` still works. `gwen open [target]`, the tray, the panel
  widget, and the launcher entry all run it. The window keeps nothing of its own that matters: closing it leaves
  every terminal running in herdr-core.
- **Layout** (board 1). A 256 px **sidebar**, the **tab bar** and content, an optional **side panel** (340 px,
  Ctrl+\\ shows or hides it), and a **status bar** on every screen.
  - Sidebar, top to bottom: Gwen and search (Ctrl K); the **Now card** (tracking dot and state, timer, project ·
    task, Break or Back, Clock out, or Start when off; a click opens the Now popover of board 11: target ring,
    streak, project and task pickers, energy, snooze); **Today**, **Inbox** (with the Needs you count), **Gwen**,
    **Plan**, **Review**; **Projects**: each unarchived project that has a folder or time today, showing today's
    time or its next planned block, with its live sessions and terminals under it (a dot for needs you, a spinner
    for working, a tick for done), and + for a new project; at the bottom **Claude's usage** (the five-hour and
    weekly shares used, and a bar for the fuller one), **Settings**, and the phone's state. Claude Code gives these
    numbers only to a status line, so each session Gwen starts gets `--settings` with `gwen statusline` as its
    status line: it sends them over the hook socket (event `StatusLine`, the terminal's pid check) and then runs
    your own status line with the same input, so the terminal looks as before (`GET /v1/code/usage`, event
    `code_usage`). Nothing shows until a session Gwen started has reported; a reading over three hours old is faded.
  - Tabs: **Today** is first and cannot close. Any other tab is a screen, a project's home, a terminal (a Claude
    session or a shell), a file, or a project's history. The sidebar opens or focuses a tab. Ctrl W closes, Ctrl
    Tab and Ctrl Shift Tab cycle, Ctrl 1 to 9 jump. Open tabs and the panel's state are kept per window in the
    webview's storage (a convenience: losing them loses nothing).
  - Status bar: tracking state, timer and project; the next plan block or event; "N need you" (opens the Inbox);
    voice; the phone; Ctrl K.
  - Side panel: on Today **Gwen** and **Activity**; in a project **Changes**, **Files**, **Tasks**, and **Gwen**.
    The Gwen tab is the companion chat in compact form, with Talk; from W32 it is Gwen's agent, with the open
    project's chats (3.16).
- **Terminals.** herdr-core session `gwen` (3.14.3) runs them; the Gwen window draws them with `@xterm/xterm` in
  Geist Mono and the window's theme. `gwen-ui` is a client of the core, as Gwen Terminal was: it attaches as a viewer
  that may type, passes output to the page as Wails events, and passes typing and grid size back. `gwend` is
  unchanged by this. Opening a project with no terminal starts a shell in its folder (`terminal.run`, `keep`,
  labels `gwen.project=<id>`); **New session** starts a Claude session through `gwend` (window off) and opens its
  tab. A terminal tab can split into two terminals side by side. Closing a tab detaches; a shell tab offers to end
  its shell, and a session keeps running until you end it (3.14.5). When no core runs, `gwen-ui` starts it the way
  `gwend` does.
- **Inside a project** (board 2). Above a session's terminal a strip shows its **task** (a picker; the link is the
  terminal label `gwen.task=<id>`), the time on it, the branch, and Normal or Sandbox. The side panel's **Changes**
  lists uncommitted files with their status and +/−, shows a file's diff, and commits with a message after showing
  exactly which files go in.
- **Files** (board 5). The tree comes from `git ls-files -co --exclude-standard` (a plain walk without git, skipping
  `.git` and `node_modules`), with the open folders' rows pinned while you scroll, indent guides, git status letters
  (U new, A added, M changed, D deleted, R renamed), and the number of changed files on each folder. A file opens as
  a tab in an editor built in (CodeMirror 6, set up to feel like VS Code: line numbers, syntax colours, folding,
  multiple cursors, bracket matching and closing, find and replace, completion), with gutter marks for uncommitted
  lines (else the lines of the last commit that touched it) and blame for the cursor's line (commit, author, when,
  subject). Ctrl S or **Save** writes the file; the tab shows ● while unsaved and asks before closing with unsaved
  edits. It follows the file on disk the way Herdr's editor does: a change made elsewhere comes in at once while
  nothing is unsaved, and with unsaved edits it asks, Reload or Keep mine. A save writes only over the text it last
  read (`repo.SaveFile` checks the hash), so a change made meanwhile is never overwritten unseen. Binary files,
  files over 1 MB, and text that is not UTF-8 stay read-only; the open-in-app button uses the desktop's default app.
- **History** (board 6). Three columns, left to right: **commits** (graph lanes with merges, branch and tag
  chips, author, time, +/−; uncommitted work first; search, author, and file filters), the **commit** (message,
  author, time, parents, its session when one made it, files as a tree or a list with +/− bars), and the
  **diff** (unified or split, old and new line numbers, long lines wrapped, never cut). Fetch and push come later.
  **Its session:** Gwen reads the Claude transcripts of the project's folder and the folders inside it for the result
  of each Bash call that ran `git commit`, and takes the short id git printed (`[main 20dd8fb] …`); a commit that
  printed none (`-q`, or output cut off) is the one git logged in the minute before the result whose subject is in
  the command, when exactly one fits. The commit shows "Made in" the conversation's name or first prompt, with Open
  (its tab, while it runs) or Resume, and a Claude mark on its row. **Summarize** asks Gwen's AI for one or two
  sentences on what the commit changes and why, from the first prompt and the turns that led to it, and keeps them
  (`commit_summaries`, 3.9; `GET /v1/code/commit-sessions`, `POST /v1/code/commit-sessions/summary`, window only).
- **A project's home** (board 7): folder, branch, ahead and behind; time today and this week against its weekly
  target; sessions running and recent (Open, Resume, Transcript); its tasks by stage; its goal; recent commits; its
  defaults (new sessions Normal or Sandbox, weekly target, colour, folder).
- **Your time follows your tabs.** While tracking is on and the window has focus, a project's tab (home, terminal,
  file, or history) that stays in front for 30 seconds switches tracking to that project (`Switch`); the strip says
  "tracked while this tab has focus". Settings › Tracking › **Follow my tabs** turns it off (config
  `[tracking] follow_tabs = true`).
- **Needs you** is one list: sessions that need you or finished unread, nudges, and from W19 approvals. Today's
  Needs you card, the Inbox's first tab, the sidebar count, and the status bar show the same list, and each row
  answers in place (Allow once, Review changes, I took it).
- **Hand it to an agent** (board 3, W16b). An Inbox item or a task's menu starts a Claude session (normal or
  sandbox) in the task's project folder with the task's title and notes as the first message, and links the task.
- **Gwen's screens, placed** (boards 8 to 17). **Plan**: Day, Board (six stages and the Eisenhower matrix, the task
  form in the side panel), Goals (with commitments). **Inbox**: Needs you, Captured (with the one-at-a-time
  processing wizard). **Gwen**: Chat, Memory. **Review**: Week, Stats, History (a day's segments edited in place).
  **Settings**: Connections (Calendar, Phone, Claude sessions, AI, phone nudges, sync hub; planned ones shown
  dashed with their package), Your day (Tracking, Nudges, Planner, Methods), Gwen (AI, Voice, Personality,
  Memory), App (Appearance, Keys, Advanced, About). Today keeps board 1 on top; scrolling down brings the agenda,
  goals, and where today went (board 11). The morning briefing, Ctrl K (screens, projects, tasks, files,
  commands, "Ask Gwen"), and toasts with Undo are board 16.
- **Outside the window** (board 17). The now card, Talk, the cue card (W18, W20), the tray, and the panel widget
  stay; their "Open Gwen" runs `gwen-ui --open`.
- **Look.** `ui/DESIGN.md` is the spec: Ghost by default (Tide and Ember in Settings › Appearance), Geist for the
  interface and Geist Mono for code and terminals, tracking colours fixed (they match the tray). The terminal uses
  the window's palette.
- **Gwen Terminal** is no longer the Gwen app. The fork, `make term`, and Gwen's herdr-core stay (the core is the
  terminal server), and `gwen-term` still works as a second view of the same terminals, but nothing opens it. The
  `gwen` mod, the Gwen space, and `gwend`'s app listener (the first W16) are deleted in W16b. The phone's Spaces
  come from projects instead of Gwen Terminal's layout: one space per project with a folder, a tab per terminal
  inside that folder.

### 3.16 Gwen's agent: the chat works in your projects (W32)

Decided with you on 2026-11-13. Today the chat plans. Each reply is one JSON answer that can make 8 kinds of change to
tasks, plans, and goals. The context is the same on every screen, and Claude Code runs with every tool turned off
(section 2). W32 makes the chat an agent that runs your projects. In a project she reads the code, knows your git
state, plans with you, and hands every change and command to a Claude session she starts, follows, and reports on. Everywhere she still does everything the
planning assistant did. You decide what runs: Gwen's policy answers every permission request Claude Code makes, and
anything it does not allow by itself becomes an approval card in the chat.

| Question | Your choice |
|---|---|
| Engine | Claude Code headless on your subscription: one long-lived `claude -p` per active chat, with streaming JSON in and out, Gwen's tools over MCP, and Gwen as the permission host |
| Code changes | Gwen orchestrates only (changed by you on 2026-11-13 after trying the first build, which ran everything itself): her chat reads code and plans, and every change, command, test, or build goes to a Claude session she starts, follows, and reports on |
| Trusted jobs | A Trusted project's sessions run in Claude Code's auto mode: its classifier approves routine work and asks only about risky steps |
| Autonomy | Balanced by default. Each project can be set to Cautious or Trusted (the policy table below) |
| Chats | Any number of chats per project, plus chats that belong to no project ("everywhere"). The side panel follows your tab |

```text
 side panel · Gwen › Chat        POST /v1/agent/chats/{id}/messages        SSE agent_changed · agent_text · agent_step
        │
 gwend  internal/agent
        ├─ Manager   one Session per live chat (at most 3); idle ones stop and resume by session id
        ├─ Session   the claude process: user turns on stdin, events on stdout, can_use_tool → Policy
        ├─ Context   the system prompt when a process starts; a context note on every message
        ├─ Policy    pure: request → allow · ask · deny, with grants (3.9)
        └─ Tools     Gwen's tools, served on agent.sock
 claude -p (cwd = the project's folder) ──stdio── gwen mcp ──agent.sock── gwend
```

**The process.** These flags were checked against Claude Code 2.1.323 on 2026-11-13:

```sh
cd "$FOLDER" && GWEN_AGENT=1 claude -p --input-format stream-json --output-format stream-json --verbose \
  --include-partial-messages --session-id "$SESSION" --model "$MODEL" \
  --setting-sources "" --tools "Read,Glob,Grep,WebFetch,WebSearch" --strict-mcp-config --mcp-config "$MCP_JSON" \
  --allowedTools "mcp__gwen" \
  --permission-mode default --permission-prompts host --permission-prompt-tool stdio \
  --system-prompt-file "$PROMPT"
# later starts of the same chat: --resume "$SESSION" instead of --session-id
```

- **Where it runs.** `$FOLDER` is the project's folder (`project_dirs`). A chat for everywhere, or a project with no
  folder, runs in `~/.local/share/gwen/agent/everywhere`, an empty folder. `$MODEL` is `[llm] model`.
- **How it starts.** The process starts the way `internal/llm/claudecode.go` starts Claude Code: the same binary
  discovery, `systemd-run --user --scope`, and API keys removed from the environment. It also gets a parent-death
  signal. With `GWEN_AGENT=1` set, `gwen hook` and `gwen statusline` exit at once.
- **No settings files.** `--setting-sources ""` loads no settings file, so no allow rule, hook, or MCP server from
  your settings or from the repo can get past Gwen's policy. It also skips CLAUDE.md files (checked), so Context
  puts them into the prompt itself.
- **Protocol: Gwen's side.** Gwen first writes
  `{"type":"control_request","request_id":"init","request":{"subtype":"initialize","hooks":null}}`; without it the
  process waits forever (checked). A message is
  `{"type":"user","message":{"role":"user","content":[<context note>, <your text>]}}`.
- **Protocol: permissions.** Claude Code asks with a `control_request` whose request has the subtype `can_use_tool`
  and carries `tool_name`, `input`, `tool_use_id`, `blocked_path`, `description`, and `permission_suggestions`.
  Gwen answers with
  `{"type":"control_response","response":{"subtype":"success","request_id":…,"response":{"behavior":"allow","updatedInput":<the approved input>}}}`
  or with the behaviour `deny` and a `message`. The model reads that message as the tool's error (checked).
- **Protocol: stopping and reading.** Stop sends a `control_request` with the subtype `interrupt`. Gwen reads
  `system` `init` (session id, tools), `stream_event` text deltas, `assistant` messages (text and `tool_use`),
  `user` messages (`tool_result`), and `result`, which ends the turn with its duration, usage, and cost. W32
  records a fixture of a real session that has every one of these, an interrupt included.
- **Who asks.** Claude Code lets reads inside the folder, and its own read-only commands, through without asking
  (checked). Everything it would ask you about comes to Gwen as `can_use_tool`. Gwen's own tools (`mcp__gwen__*`)
  are allowed inside Claude Code and checked by `gwend` when the call arrives instead (Tools, below).
- **Lifetime.** A chat's process starts with the first message after the chat was idle, and stops (stdin closed)
  after `[agent] idle_stop` without a turn. The next message resumes the same session. At most `[agent] max_live`
  processes run: starting one more stops the one idle longest, and when none is idle the message waits in the
  chat's queue. A turn longer than `[agent] turn_timeout` is interrupted with a note.
- **Crashes and restarts.** If the process dies mid-turn, the chat shows the error with its last stderr lines, and
  its running steps become `cancelled`. The next message resumes the session. When `gwend` starts, every chat is
  `idle`, and steps left `running` become `cancelled` ("Gwen restarted").
- **Messages while she works** queue in `gwend` (at most 5, shown as queued) and go in order. Stop ends the turn,
  and its pending approvals expire.

**Context.**

- **The system prompt** is written to a 0600 file when the process starts, and stays the same while it runs (for
  the prompt cache). It holds:
  1. the persona, her name, and your instructions, as `internal/llm/persona.go` builds them, with the attitude and
     self-note in effect;
  2. the agent rules (`internal/agent/rules.md`, embedded):
     - she works for you, inside Gwen;
     - tasks, plans, and goals change only through Gwen's tools;
     - she makes small changes herself; work needing more than about ten edits or half an hour, or that you call a
       job for Claude, she hands to a session, after saying so;
     - she never retries a denied action another way;
     - she says what she changed;
     - she never pushes, publishes, or sends unless you asked for it in this chat;
  3. the profile card from memory (at most 800 tokens, as `internal/api/context.go` makes it);
  4. the chat's place: the project's name, sub-projects, folder, test command, git remote, and branch; for a chat
     for everywhere, every project with its folder;
  5. the CLAUDE.md files Claude Code would have read: `~/.claude/CLAUDE.md`, and in the folder `CLAUDE.md`,
     `.claude/CLAUDE.md`, and `CLAUDE.local.md`. Each is cut at 12,000 characters and all of them at 30,000, with a
     note where they were cut.
- **A context note** goes in front of every message: a `<gwen-context>` text block of at most 8,000 characters.
  - Now, and tracking: state, project, task, and the time on it.
  - What you have open. The window sends the active tab with the message (a screen, a file path, a terminal, or a
    project's history), plus the side panel's text selection when there is one.
  - In a project:
    - git: branch, ahead and behind, and the number of changed files (`internal/repo`, now used by `gwend` too);
    - its open tasks, at most 20: short id, title, stage, due date, estimate, and whether it is planned today;
    - its Claude sessions: name, state, step or last message, and task;
    - today's plan blocks for it.
  - Everywhere: every project with its open tasks and today's time; today's plan and agenda; Needs you; the 20 most
    urgent open tasks.
  - The memory note for the message, as today (at most 1,200 tokens).
  - What changed since her last message that she did not do: sessions that finished, tasks you completed.
  - A short id is the first 8 characters of an id. Tools accept any unique prefix of 8 or more characters.

**Tools.** Her chat has Claude Code's read-only tools only (Read, Glob, Grep, WebFetch, WebSearch), and her own
system prompt replaces Claude Code's, so she speaks as Gwen and cannot edit or run anything herself. Gwen adds hers over MCP through `gwen mcp`, a stdio server built on the
official Go SDK. `gwen mcp` reaches `gwend` only through `agent.sock` (`$XDG_RUNTIME_DIR/gwen/agent.sock`), which
serves nothing but the tool endpoints. It sends a token made for that process alone. The token is in the MCP config
file, a 0600 file in `$XDG_RUNTIME_DIR/gwen/agent/` that is deleted when the process ends.

| Tool | What it does | Capability |
|---|---|---|
| `context` | what is open and tracked right now: the context note, fresh | gwen.read |
| `tasks`, `task` | list tasks by project, stage, status, or text; read one task with its steps | gwen.read |
| `plan` | a day's plan blocks with their times, and the calendar's busy times that day | gwen.read |
| `goals`, `projects`, `time` | goals with progress; projects with folders; tracked time per project over a range | gwen.read |
| `sessions`, `session_transcript` | a project's Claude sessions and terminals; the last N turns of one | gwen.read |
| `recall` | memory search with cited claims | gwen.read |
| `task_create`, `task_update`, `task_complete`, `task_schedule`, `task_unschedule` | as the planning assistant does, plus steps; `task_update` covers every field, stage and project included | gwen.change |
| `project_create`, `goal_create`, `goal_update`, `track` | make projects and goals; start or switch tracking | gwen.change |
| `remember`, `set_attitude` | store a memory; set her attitude and self-note (`self_versions`) | gwen.change |
| `open` | open a URL or a folder, or a tab in the window: a file, terminal, project, or screen | gwen.change |
| `task_delete`, `goal_delete` | delete a task or a goal | destructive |
| `changes` | what a session changed since it started (files with +/−), or the project's uncommitted files; with a path, that file's diff | gwen.read |
| `session_start` | hand a job to a new Claude session for a project, given a title and a brief (below) | agent.session |
| `session_send` | send a message to a session that is idle or done; refused while it works or waits on a permission | agent.session |

- **`session_start`** starts the session in the project's folder (`remote.Service.StartInProject`: the phone's
  `remote.roots` limit does not apply to a folder you chose), normal or sandboxed (the project's default).
  - In a Trusted project it runs with `--permission-mode auto`; otherwise in Claude Code's own default mode, so its
    questions reach Needs you and the phone.
  - The title names its terminal (made into herdr's 1 to 32 of a-z, 0-9, `-`: "Fix add bug" is `fix-add-bug`).
  - It links the session to a task, as Hand it to an agent does: `gwend` sets the terminal label `gwen.task=<id>`
    through `internal/herdr`, and the task goes to Waiting on "Claude".
  - It opens no tab, so it never takes your focus. The chat shows the session.
- **Not offered:** answering a session's permission request, approving anything, changing grants, rules, settings, or
  project folders, and deleting projects.
- **Checks.** `gwend` checks each call to Gwen's tools with the same Policy and approvals that Claude Code's requests
  go through.
- **Undo.** Each change returns `{"ok", "summary", "change"}`, and the chat shows the summary with **Undo**: a
  created task is deleted, changed fields are restored, a schedule is removed.

**Policy** (`internal/agent/policy.go`, pure, with table tests). Each request becomes one capability, and the chat's
autonomy decides what happens. Her chat has no Edit, Write, or Bash, so in practice what reaches it is reads, Gwen's
tools, and handing off jobs; the edit and command rows stay for an engine that gives her those tools, and a job's own
permissions are Claude Code's (auto mode in Trusted):

| Capability | Covers | Risk | Cautious | Balanced | Trusted | Can be remembered for |
|---|---|---|---|---|---|---|
| `fs.read` | reads inside the folder, and Claude Code's read-only commands (Claude Code does not ask) | read | allow | allow | allow | |
| `gwen.read` | Gwen's read tools | read | allow | allow | allow | |
| `fs.read_outside` | reading outside the folder | read | ask | ask | allow | this project (that folder) |
| `web` | WebFetch, WebSearch | read | ask | ask once per chat | allow | this chat, this project |
| `gwen.change` | Gwen's change tools | local | ask | allow, with Undo | allow | |
| `fs.edit` | Edit, Write, NotebookEdit inside the folder | write | ask | ask once per chat | allow | this chat, this project |
| `shell.run` | a command Claude Code asks about | write | ask | ask | allow | this project (the command's prefix) |
| `agent.session` | starting or messaging a session | write | ask | ask once per chat | allow | this chat, this project |
| `external` | anything that leaves the machine in your name (list below) | external | ask | ask | ask | never |
| `destructive` | deleting things, and any write outside the folder (list below) | destructive | ask | ask | ask | never |
| floor | the hard limits (list below) | | deny | deny | deny | |

- **External:** `git push`, writes with `gh`, uploads and requests other than GET with `curl` or `wget`,
  `ssh`/`scp`/`rsync`/`sftp` to a host, `publish` with npm, pnpm, yarn, bun, or cargo, and `docker push`.
- **Destructive:** `rm`, `rmdir`, `unlink`, `shred`, `find -delete`, `git reset --hard`, `git clean`, `git checkout --`
  and `git restore` of files, `git branch -D`, `git stash drop` and `clear`, force pushes, `truncate`, `dd`, `mkfs`,
  recursive `chmod` and `chown`, `kill` and `pkill`, and `docker rm`, `rmi`, and `prune`; any write outside the
  folder; deleting tasks or goals.
- **The floor:**
  - `sudo`, `su`, `doas`, and `pkexec`;
  - any read or write of `~/.ssh`, `~/.gnupg`, `~/.claude.json`, `~/.claude/.credentials.json`, or Gwen's
    credentials, config, and runtime folders;
  - writing Claude Code's settings in `~/.claude/`, `.git/hooks`, `.git/config`, shell start-up files, or systemd
    units;
  - any command that names Gwen's sockets or the `gwen` binary.
- **Ask once per chat** asks the first time. After that the same capability is allowed in that chat without asking
  (a grant scoped `chat:<id>`).
- **Commands are split** at `;`, `&&`, `||`, `|`, `&`, newlines, `$(…)`, and backticks. Each part is classified, and
  the strongest class wins, in the order floor, destructive, external, `shell.run`. Quoting cannot hide an operator.
- **A command's prefix** is its first word, or its first two words when the first is `git`, `go`, `npm`, `pnpm`,
  `yarn`, `npx`, `bun`, `deno`, `node`, `cargo`, `make`, `python`, `python3`, `uv`, `pip`, `docker`, `gh`, or
  `kubectl`. An interpreter running a module keeps it (`python3 -m pytest`). Parts that are only `cd` are skipped.
- **Inline code** (`python -c`, `node -e`, `bash -c`, and the like) runs anything, so no grant can cover it.
- **Trusted runs only plain commands** without asking: one part, no expansions, quotes, escapes, globs, redirections,
  or helpers such as `eval`, `env`, `xargs`, or `base64`. Anything else asks even in Trusted, because the floor reads
  words and such a command can hide them (`cat ~/.s'sh'/key`).
- **A remembered `shell.run` grant** matches only a command that has one part, has no redirection, and has the
  grant's prefix. Anything compound asks, except in Trusted.
- **Grants** are W19's `grants` (3.9): scope `chat:<id>` or `project:<id>`, plus the pattern. They are made only by
  your answer on an approval card. Settings › Gwen › Agent lists them and removes them.
- **Autonomy** is `[agent] autonomy` by default. Each project can set its own in `project_dirs.agent_autonomy`, from
  its home's defaults or from Edit in Manage projects.
- **What this stops, and what it does not.** In Normal mode an approved command runs as you, so it can do anything
  you can, exactly as in Claude Code. The protection is that you see each command before it runs. The floor is a
  check on the words of what Claude asks, a guard against mistakes, not a wall: a script she wrote and then runs
  can do anything you can, and so can Trusted, which edits and runs plain commands without asking. Outside the
  sandbox, give Trusted only to projects whose code you would run yourself. `gwend.sock` has no token yet (any
  program of yours can call it), so only the sandbox keeps the agent away from it. A project whose
  default is Sandbox (after the sandbox sign-in, 3.14) runs Claude Code, and everything it starts, inside
  bubblewrap. Bubblewrap binds only the project folder and `agent.sock`: no other Gwen socket, no credentials, no
  home folder. Once you have signed the sandbox in, Sandbox is the recommended default.

**Approvals.**

- **The row.** A request that asks becomes a W19 `approvals` row and an `approval_requested` event, and the turn
  waits. The row holds the thread id, step id, capability, risk, a plain summary, the exact payload, and the grant
  scopes it offers.
- **The card** in the chat shows what will happen: an edit as a diff of the old and new text with its path, a command
  as its exact text with its folder, a fetch as its URL, and a session start as its brief and folder.
- **Buttons:** **Allow**, **Deny** (with an optional note, which Claude reads), and **Allow for this chat** or
  **Always in this project** where the table offers them. Allow answers with exactly the payload you saw
  (`updatedInput`).
- **Elsewhere.** Pending approvals also show in Needs you (Today, the Inbox, the count), with a link to the chat. A
  notification says "Gwen needs your OK in <project>" with what she asks; its Open button shows Gwen › Chat. It has
  no Allow button.
- **Guards.** `agent.sock` serves only Gwen's tools, so the agent cannot reach the approvals or grants endpoints. An
  unanswered approval expires when its turn times out, and Claude gets "No answer in time".

**Chats.**

- **What a chat is.** A chat is a `chat_threads` row (3.9) with its project, or with none for everywhere. A project
  has as many chats as you start.
- **The side panel's Gwen tab** shows the chats of the open tab's project and opens the one used last. Its header
  lists the others and has New chat. On screens outside a project it shows the chats for everywhere.
- **In the background.** A chat keeps working when you switch away. Its row shows a spinner while she works and a
  dot while she waits for you.
- **Gwen › Chat** lists every chat by project, everywhere first, on the left.
- **Names.** The first message names the chat (60 characters, editable).
- **Deleting a chat** stops its process and removes the chat.

**Following the jobs.** `gwend` remembers which sessions each chat started (the step's output ends with
`[session:<id>]`, which the chat's card reads) and calls `Manager.SessionsChanged` on every `code_session` event:
- **Finished:** a job that has worked and goes idle or done adds a "Claude finished" row and gives Gwen a turn of her
  own: a `<gwen-event>` with its last message, not stored as a message of yours. She checks the work (`changes`,
  `session_transcript`, reading files), reports in a few lines, and ticks the task off when it is truly done.
- **Needs you:** a "Claude needs you" row with Answer (its terminal). Only you answer it. A new folder's first job asks
  whether you trust the folder; answer it once.
- **Stopped or ended:** a quiet row.

**In the chat** (the side panel and Gwen › Chat):

- **Her text** streams in as she writes it, as Markdown.
- **Tool calls.** Each one is a quiet row under her text, grouped per turn ("Read 4 files"), that opens to the list:
  - "Edited `ui/src/x.tsx` +12 −3" opens the diff in Changes;
  - "Ran `go test ./...`, exit 0, 4.2 s" opens the last 40 lines of output;
  - "Created Fix board filter" has Undo;
  - a failed or denied call says so in plain words.
- **Approval cards** stay pinned above the composer while they wait.
- **Jobs.** A job she handed off is a card: its title and task, Claude's live state ("Working · Editing calc.py",
  "Needs you", "Finished"), how long it has run, its brief, and Open or Answer. A strip under the header lists the
  chat's jobs still going. Job news ("Claude finished", "Claude needs you") are rows in the timeline, followed by
  her report. The timeline is in time order, and her replies carry her face and name.
- **Chips** above the composer say what she will see with your next message: the project, and the open file or
  terminal. × leaves one out of that message.
- **The header** has the chat's name and switcher, the project's autonomy (a menu), and Stop while she works.
- **Voice.** Dictation fills the composer, and Read aloud reads her reply. Talk stays with the planning assistant
  (W33).
- **When the agent can't run** (`[agent] enabled = false`, a provider other than `claude_code`, or Claude Code missing
  or signed out), the Gwen tab is today's planning assistant, with one line saying why and how to turn the agent on.

**Records.**

- **Messages.** Your messages and her replies are `chat_messages`, as today, so memory keeps learning from chats
  (`Memory.ChatTurn`).
- **Steps.** Each tool call is an `agent_steps` row (device-local). It holds the tool, capability, risk, a plain
  title, the input (at most 16 KB, enough for the diff and the command), at most 2 KB of output text, the status, the
  approval, and the undo. It never holds a file's contents from a read.
- **The full record** of the conversation stays Claude Code's own transcript in `~/.claude/projects`.

**Not in W32:**

- the phone, and Talk with the agent (W33);
- Gwen starting work by herself: she reports on jobs she started, but starts new work only when you ask;
- a worktree per chat: W23's coding runs stay the isolated path;
- connectors: they arrive as more MCP servers (W22, W24);
- engines other than Claude Code (harness.md G1 PR3).

### 3.17 The agentic timeline: what waits on what

Asked for on 2026-11-14, built the same day. Today's "Your day" card becomes one picture of how the day hangs
together. It shows your plan, with arrows for what waits on what, your tracked time, your fixed time, and each Claude
session's day. Every task gets where it is heading now and, when something is wrong, a sentence saying why. Gwen reads
the same picture and fixes it.

**Dependencies.** A task waits on other tasks: `tasks.waits_on`, a JSON list of ids (00018_timeline.sql). It syncs
with the row (added at schema 18, so an older hub never gets it). The store refuses a task waiting on itself, a loop
(the error names the task that already waits), a repeating template, and more than 20. Ids of tasks that are gone are
dropped quietly. You set them in the task form ("Starts after"), in the timeline's panel, or through Gwen
(`task_wait`).

**The planner** (internal/planner/waits.go) sizes tasks most urgent first. A task waits its turn until everything open
it waits on is kept on the day or got blocks itself. One still waiting at the end stays off the day, so room never goes
to work that can't start. New blocks go after the blocks of what they wait on and start no earlier than those end.
Done, deleted, and missing tasks block nothing.

**Agent history.** Each state change of a Claude session writes a span (`agent_spans`: state, need, task, start, end),
kept on this machine and pruned with its session. A session keeps its task (`code_sessions.task_id`, from its
terminal's `gwen.task` label), so the link outlives the terminal.

**The day graph** (`GET /v1/days/{day}/graph`, internal/api/daygraph.go; the window, the phone, and Gwen read it):

- *Who is in it:* the plan's tasks; the tasks agents work on; open work kept off the day by what it waits on; what all
  of those wait on; and the open tasks that wait on them.
- *Where each task is heading:* the one you track ends at now plus what is left of its estimate. A planned one starts
  at its plan, or after what it waits on ends, or now if its time came and went. One with Claude working on it ends at
  now plus what is left. One where Claude waits on you has no end in sight.
- *States, worst first:* done, waiting on you (a session on it, or on what it waits on, asks you something), blocked
  (it waits on open work with no end in sight today: off the plan, with someone, out of planned time, or blocked
  itself), on it now, with Claude, not planned, running late (heading 10 or more minutes past its plan, or past the
  end of the day), on track. Each comes with a reason, such as "Starts after “API”, which now ends at 10:40 am: 40 min
  later than planned".
- *Issues* are the waiting, blocked, and late ones, worst first. The *longest chain* runs back from the latest-ending
  task that waits on something, through what ends last at each link.

**The window** (ui/src/timeline):

- The liquid Timeline has groups You (Plan, Busy, Tracked) and Agents (one row per session). A planned block fills as
  you work on it. A block pushed later by what it waits on moves; one you're on starts where you started it; one
  running long shows where its plan ended. Blocked work is red, work waiting on you amber; Claude working is blue,
  waiting on you amber.
- Focusing a bar lights its whole chain both ways (the liquid layout change in `relatedTasks`).
- The panel below reads the bar in focus. Hovering previews it and clicking keeps it. It shows the reason, planned
  against heading, the full "Waits on" and "Holds up" trees, and actions: add or remove what it waits on, Start
  (disabled while blocked), Done, Hand to Claude, Open or Answer the session, Edit, and Ask Gwen. Beside it is "In
  your way" with the longest chain.
- The zoom is Day (the day's work with air) or Around now (2 hours back, 3 ahead). "Sort out my day" opens a chat with
  Gwen.

**Gwen.** `day_graph` (read) and `task_wait` (a change, approved as task changes are, with undo); her rules say to read
the graph before sorting out a day and to make the smallest change that works.

**The phone.** Today gets "In your way" (the issues, and Ask Gwen to sort it out), and a plan row that is blocked, late,
or waiting on you says why.

**Not in it yet:** a week view (one graph per day only); dragging bars to reschedule; Gwen acting on the graph by
herself without being asked.

---

## 4. Products and services

| Need | Choice | Why | Not chosen |
|---|---|---|---|
| Calendar | Google Calendar API v3 | already connected | |
| Email | Gmail API v1 (`google.golang.org/api/gmail/v1`, same module as Calendar) with scopes `gmail.readonly` and `gmail.compose` | reads mail, creates drafts, and sends drafts. Both scopes are "restricted", which is fine for a personal app: Google shows an unverified-app warning and allows up to 100 users | IMAP (needs app passwords); Composio or Zapier (a third party would hold your mail token) |
| Coding agent | Claude Code CLI, already installed and logged in with your plan | edits, search, and tests are already solved; plan mode and resumable sessions fit "discuss first" | our own code agent loop; Aider or OpenHands (another stack to maintain) |
| Tool protocol | MCP, official Go SDK `github.com/modelcontextprotocol/go-sdk` v1.7.0 | Gwen serves her tools to her agent's Claude Code (`gwen mcp`, W32) and the approval tool to coding runs (W23), and can use any MCP server later | custom JSON-RPC |
| Gwen's agent (W32) | Claude Code headless (`claude -p` with stream-json in and out, `--permission-prompt-tool stdio`), on your subscription | file, search, shell, and subagent tools are already solved, and every permission request comes to Gwen as data she answers | Gwen's own agent loop on the Messages API (needs an API key, and her own file and shell tools; harness.md G1 PR3) |
| Browser | your default browser through the XDG desktop portal `OpenURI` (D-Bus, `godbus` is already a dependency), with `xdg-open` as fallback | works from a systemd service on Wayland and raises the window properly | driving a browser for simple link opening |
| Browser automation (later, W26) | Playwright MCP, opt-in | real multi-step web tasks | always-on headless Chrome |
| Secrets | system secret store through `github.com/zalando/go-keyring` (on Plasma 6 this is ksecretd), with the current 0600 files as fallback for the Pi | tokens out of plain files | plain files for new tokens |
| Notifications | freedesktop notifications over D-Bus (have it), plus the `Inhibited` property for Do Not Disturb; ntfy for the phone (have it), with a "view" button that opens a link | already working | |
| Notes (later, W25) | a plain Markdown folder (works with Obsidian) | local, no account | Notion API (cloud and another OAuth) |
| Voice | unchanged: Fish Audio by default, Kokoro offline, Parakeet for speech to text | | |
| LLM | unchanged providers. Email triage works on all of them. Coding needs Claude Code | | |
| Run engine | SQLite tables and Go code in `gwend` | one user, one machine | Temporal, queues, LangChain, n8n, Pipedream |
| Phone access (W27) | Tailscale Serve on the system `tailscaled`, publishing a Unix socket on port 8443 | tailnet-only HTTPS with a real certificate (the microphone needs one), the caller's identity in a header, nothing new running inside `gwend` | tsnet inside `gwend` (harness G4, a whole Tailscale node in the always-on daemon); Funnel; Claude Code Remote Control and the Claude app (you ruled them out) |
| Session host (W27) | herdr-core, built from upstream source at a pinned release | terminals outlive their window; an API to send, press keys, read, attach, and follow status; it waits for your unfinished typing before sending; Claude Code reports its status to it by itself | Gwen's own tmux (screen scraping and typing detection by hand); typing into Ghostty tabs (impossible: Linux blocks `TIOCSTI` and Ghostty has no input API) |
| Desktop terminal (W27) | `gwen-term`, a thin fork of herdr-app (Apache-2.0; Rust, GPUI, libghostty) | every Claude started in it is reachable from the phone; Herdr's community layouts and mods; upstream fixes arrive by rebase | Ghostty (no input API); writing a terminal of our own |
| Sandbox (W27) | bubblewrap, already installed | unprivileged, starts instantly, the first cut of harness G3's bubblewrap provider | Docker, stacyvm (later) |
| Desktop interface (W16) | Gwen's own window (Wails and React) with the workbench inside: terminals from herdr-core drawn with `@xterm/xterm`, files and git history from `git` | Gwen is the product, so her window is the shell; one design system with the phone; terminals still outlive the window | Gwen's screens as a mod inside Gwen Terminal (built, then rejected 2026-11-13: Herdr became the shell and the mod kit limited every screen) |
| Phone app (W27) | `gwen-mobile`, a web app built from `ui/`, installable to the home screen, following Herdr's phone app screen for screen | one codebase and design system with the dashboard; an Android wrapper can come later (W29) | Herdr's phone app (its source isn't published, and it only does terminals); a native app first |
| Terminals on the phone and laptop (W27, W16) | npm `@xterm/xterm` 5.5.0 | draws herdr-core's stream; MIT | a terminal of our own |
| QR code (W27) | npm `qrcode` 1.5.4 in the dashboard | small, MIT | a Go module |
| Fonts on the phone (W27) | npm `@fontsource-variable/geist` and `@fontsource-variable/geist-mono` 5.3.0 | Herdr's fonts, shipped inside the app so nothing loads from the network; OFL-1.1 | system fonts (the phone would not look like the laptop) |
| Terminal WebSocket (W27) | `github.com/gorilla/websocket` 1.5.3, already in the module graph through Wails | no new module to download | a hand-written upgrade |

New Go modules for this phase: `github.com/modelcontextprotocol/go-sdk` (W32, or W23 if it comes first) and
`github.com/zalando/go-keyring` (W22). Nothing else; W27 adds none (it uses `gorilla/websocket` directly, which was already an indirect requirement).

Before connecting Gmail, turn on 2-Step Verification for that Google account, because Gwen will be able to
read your mail.

---

## 5. UI and UX

### 5.1 One design system

The Gwen window and the phone share one design system, `ui/DESIGN.md`: the Ghost palette (5.2), Geist and Geist
Mono, a 256 px sidebar, 44 px tab bar, 30 px status bar, 7 to 12 px radii, hairline borders, and Herdr's workbench
conventions (sessions with live status, tabs, splits, Ctrl K) drawn in Gwen's style. These rules from the old
`ui/design.md` stay:

| Keep | Drop |
|---|---|
| explicit UI states from real app state (runtime, audio, character, task) | separate screens for things that belong together (a task, its session, and its time are one place) |
| character stage rules: fixed bounds, still frame when idle, state from the real pipeline | "Lab" as a main screen; diagnostics move to Settings |
| task and activity panel rules: real states only, no fake progress | telemetry charts for models we don't run |
| approvals before side effects, event rows for tool activity | |
| accessibility, reduced motion, no fake data | |

### 5.2 Palette

Ghost is the default. Settings › Appearance picks Ghost, Tide, or Ember for the Gwen window, its terminals, and the
phone (`term/themes/*.json` keep the same palettes for Gwen Terminal). Tracking colours
(working, idle, break, off) stay the same in every theme because they match the tray icons. Project colours are
data and stay as they are.

| Token | Ghost (default) | Tide | Ember |
|---|---|---|---|
| canvas | `#0D1012` | `#0B0F14` | `#110F0D` |
| surface-1 | `#141819` | `#111821` | `#191613` |
| surface-2 | `#1C2225` | `#172029` | `#201C18` |
| surface-3 | `#232A2E` | `#1E2833` | `#28231E` |
| surface-4 | `#2B3338` | `#26313D` | `#302A24` |
| line | `#2A3236` | `#26303B` | `#322C26` |
| line-strong | `#3C464D` | `#384656` | `#473F37` |
| ink | `#F2F1ED` | `#E9EFF5` | `#F4EFE9` |
| ink-muted | `#AFA2A0` | `#B6C2CE` | `#CDC4B9` |
| ink-subtle | `#958A88` | `#8392A1` | `#998F84` |
| ink-faint (disabled only) | `#696160` | `#5E6C7A` | `#70665D` |
| accent | `#D72638` crimson | `#3EC1AF` teal | `#E9935A` apricot |
| accent-hover / pressed | `#E2495A` / `#98111E` | `#62D5C5` / `#2BA393` | `#F1AB7A` / `#CD7942` |
| accent-soft | `#3F0D12` | `#12302C` | `#3B2617` |
| on-accent (text on buttons) | `#FFFFFF` | `#04130F` | `#1C0F06` |
| focus | `#F2707D` | `#86E2D6` | `#F6C29A` |
| success / warning | `#4FB88A` / `#F2B33D` | `#86C66A` / `#E9B65A` | `#6CC58C` / `#E6D16A` |
| danger / info | `#E5384A` / `#4D9BDB` | `#EF6B6B` / `#7FA9EE` | `#F0656A` / `#79B2E6` |

Measured contrast (WCAG): ink on canvas 16.6 to 16.9, ink-muted on surface-1 7.2 to 10.5, ink-subtle on
surface-1 5.3 to 5.7, text on accent buttons 4.9 to 8.6, accent on canvas 3.8 to 8.7. All body text passes AA;
Ghost's accent trades some of that headroom for staying a believable, restrained crimson rather than a pastel.

**Why Ghost is the default:** it's rebuilt around Gwen's `design.md` swatch board — obsidian black with a
Spider-Man-inspired red/blue duality, not the earlier rose Spider-Gwen lining. Crimson Silk stays the one
budgeted action colour; danger is a distinct, slightly brighter red (Scarlet Bikini, lightened) so a real
problem never looks like a button; and Aegean Sky supplies the one cool signal colour the app needs, instead
of a second warm accent. If you want the cooler Tide or warmer Ember palette instead, pick it in Settings.

### 5.3 Navigation

The sidebar, the tabs, and Ctrl K are the navigation (3.15):

| Sidebar | Contains | Package |
|---|---|---|
| **Now card** | tracking state and timer, project and task, Break, Clock out; opens the Now popover | W16 |
| **Today** | now and next, Needs you, to-dos, your day (plan, you, agents); below: agenda, goals, where today went | W16, then W17, W21 |
| **Inbox** | tabs: Needs you, Captured; capture, process, plan, start, hand off | W16, W16b |
| **Gwen** | tabs: Chat (companion, character stage, Talk), Memory | W16 |
| **Plan** | tabs: Day, Board, Goals | W16 |
| **Review** | tabs: Week, Stats, History | W16 |
| **Projects** | each project: its home, sessions, terminals, files, history | W16, W16b |
| **Settings** | groups: Connections, Your day, Gwen, App | W16, W18, W27 |

The status bar shows tracking state and time, the next block or cue, Needs you, voice, the phone, and Ctrl K.

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
Now      W27 Gwen Terminal and the phone (needs no other package; merges first)
Wave 0   W15 housekeeping                 |  W16 the Gwen window (needs W27)
Wave 1   W16b agents in the workbench     |  W17 agenda (needs W16)   |  W19 harness core
Wave 2   W18 cues                         |  W32 Gwen's agent (needs W16, W19)  |  W21 subject drawer and Work screen (needs W16, W19)
Wave 3   W20 presence (needs W18)         |  W22 Gmail review         |  W23 coding runner (both need W21)
Later    W24 MCP connectors · W25 notes folder · W26 browser automation · W28 Slack inbox · W29 phone layouts
         and mods · W30 devices · W31 home
```

**Screens live in `ui/src/` (3.15).** W17 to W23 build their views in the Gwen window: screens in
`ui/src/screens/`, workbench views in `ui/src/work/`, shared pieces in `ui/src/components/`, and the shell in
`ui/src/shell/`. The cue card and Talk stay small `gwen-ui` windows.

Packages in the same wave run in parallel on their own branches (`w17-agenda` and so on). Merge in the order
listed. After Wave 0 every screen and the workbench live in the Gwen window. After Wave 1 sessions open there too, and
your appointments are on Today. After Wave 2 you get the 5 minute reminder, the DSA routine opens your browser, and Gwen's chat works in your projects.
After Wave 3 Gwen speaks first, reviews email, and takes coding tasks. The memory layer already owns migration `00010`, W27 takes `00011`, and W16 takes `00012` (`project_dirs`), and W16b
`00013` (`commit_summaries`), sub-projects `00014` (`project_parent`), task attachments `00015` (`task_attachments`). W19 (built first) takes `00016` and W32 (built next) `00017`, then W17 `00018` and W18 `00019`, and use the memory layer's cited claims and episodes instead of adding another memory store. Migrations apply in
order, so a package built ahead of the numbers reserved before it takes the next free number when it is built, and
renumbers this file in the same change.

### 6.3 Work packages

Each package is one prompt (template in 6.4).

#### W15: Housekeeping

- **Owns:** `Makefile`, `.github/workflows/check.yml`, `README.md`.
- **Delivers:** remove the `docs-check` target and its call from `check`; point the Makefile header comment at
  this file; link `plan.md` from the README's Development section.
- **Acceptance:** `make check` passes on a clean checkout.

#### W16: The Gwen window

Needs W27. Replaces the first W16, which was rejected (3.15).

- **Owns:** `cmd/gwen-ui` (the window mode, `--open` and one instance, `term.go` for terminals, `repo.go` for files
  and history); `internal/repo` (new: git and files for the workbench); `ui/src/App.tsx`, `ui/src/shell/` (new),
  `ui/src/work/` (new), `ui/src/screens/`, `ui/src/settings/`, `ui/src/components/`, `ui/src/index.css`,
  `ui/DESIGN.md`; migration `00012_project_dirs.sql`, `internal/store/projectdirs.go`, a project's `folder` on the
  wire (`Project.folder`, set with `PatchProject`); the config key `[tracking] follow_tabs`; `cmd/gwen/open.go`; the
  "Open Gwen" actions of `cmd/gwen-tray` and `packaging/plasma/dev.gwen.panel`; `packaging/gwen.desktop`; the
  launcher lines of `Makefile`.
- **Delivers:** 3.15 except what W16b lists: the window, sidebar, tabs, status bar, side panel, and Ctrl K; every
  screen of the old dashboard in its new place (board 18), with everything it did; Today as board 1; terminals in
  project tabs with split; Files, History, and a project's home; Changes with commit; your time following your
  tabs.
- **Acceptance:** tests for `internal/repo` (the tree with git status, a path outside the folder refused, a log with a
  merge, a commit's files, a diff, blame, a folder without git) and for `project_dirs`; `tsc` passes; manual:
  every action of the old screens works (board 18 as the checklist), open a project, start a shell and a Claude
  session, type, resize, split, close the window and find the terminals still running; Files and History on this
  repo; tracking follows a tab after 30 s.

#### W16b: Agents in the workbench

Needs W16.

- **Owns:** `internal/remote` (sessions open in the Gwen window; Spaces from projects); the hand-off and task links
  in `ui/src/`; deleting `term/mods/gwen/`, `cmd/gwend/app.go`, `internal/api/poll.go`, and the Gwen space code
  (`internal/remote/gwenspace.go`).
- **Delivers:** "Open a window on the laptop" and "Open on laptop" open the session's tab in the Gwen window
  (3.14.5); the phone's Spaces are your projects; Hand it to an agent; a session's task link; nudges in the Needs
  you list; the first W16's mod and app listener removed.
- **Acceptance:** tests for Spaces from projects and for the open command `gwend` runs; manual: start a session from
  the phone and see its tab open on the laptop; hand an Inbox item to Claude and see the task linked.

#### W17: Agenda

- **Owns:** migration `00018_agenda.sql`; `internal/subject` (new); `internal/agenda` (new);
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

- **Owns:** `internal/cue` (new); migration `00019_cues.sql`; `internal/store/cues.go`; `cmd/gwend/cues.go`;
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
  `00016_runs.sql`; `internal/store/{runs,resources,routines}.go` (`project_dirs` is W16's);
  `internal/api/{runs,approvals,resources,routines,connections}.go`; matching `internal/wire` and
  `internal/client` files; `cmd/gwen/runs.go`.
- **Delivers:** sections 3.7 and 3.8, the routine run kind, the start hook (Start runs the subject's routine),
  restart recovery, cancel, the 2 run limit, approvals with SSE, project folder endpoints, connections listing.
  URLs open through the portal's `OpenURI` with `xdg-open` as fallback, one call per URL.
- **Acceptance:** tests for every allowed and refused state move; policy table tests (`external` can never be
  granted); a running run becomes `waiting` after a restart; cancel stops a step; a routine run with a fake opener
  logs each step and reports a failed open honestly; an approval round trip through `clienttest`; a test that
  scans run events and logs for token-like strings and finds none.
- **Done so far (2026-11-13):** built, tests not written yet (they come in one batch). `internal/subject` (only
  what W19 uses: parse, print, parents; W17 extends it); `internal/connect` (registry, the pure policy where every
  matching grant counts and an ask beats an allow, the desktop connector through the portal's `OpenURI` with
  `xdg-open` as fallback, the calendar connector over the busy cache); `internal/harness` (the engine with the state
  table, 2 running, queued drafts, cancel, Resume, restart recovery that also catches runs waiting on an approval,
  the routine kind, and `Approvals`, the approvals service W32 shares: `Ask` with a run or a thread,
  `Allowed` with an exact pattern, `Decide` with `remember`); migration `00016_runs.sql` with W32's approval and
  grant columns; `internal/store/{runs,resources}.go` (resources and routines in one file) and `projects.go`'s
  `Dir`/`SetDir`; all of `internal/api/runs.go` (runs, approvals, grants, resources, routines, project dir,
  connections, and the start hook on clock-in and switch); wire, client, and `clienttest`; `gwen run`
  (ls, show, cancel, resume, approvals, grants; it never decides an approval); Settings › What Gwen can do
  (connections with each capability's risk, approvals with Approve, Reject, and "Don't ask again", remembered
  grants with Remove, recent runs with their steps, Cancel, and Resume). Checked live on a throwaway `gwend`: a
  routine saved with a missing folder is refused; starting the task runs its routine; an ask grant made the first
  step wait; Approve with remember in the window ran it and replaced the grant; the calendar step failed honestly
  ("the calendar isn't connected") and the run ended failed, 1 of 2; a restart mid-approval left the run waiting
  (interrupted) with its approval expired, and Resume asked again, then Cancel. **Not yet:** the "When it starts"
  editor on a task or event (no window path to make a routine yet; W17/W21 screens), the queue and the 2 run limit
  live, `browser.open` and `folder.open` live (they would open real windows), and the chat on top (W32, 3.16).

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

- **Owns:** `ui/src/components/{SubjectDrawer,RoutineEditor,RunView}.tsx` (approval cards use W32's `ApprovalCard`); `ui/src/screens/Work.tsx`;
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
  create and remove); the `approve` and `report` tools of `gwen mcp` (W32's `cmd/gwen/mcp.go`, or the whole command if W23 comes first); `GET /v1/approvals/{id}/wait` in
  `internal/api/approvals.go`; `ui/src/components/CodeRun.tsx`; the Claude Code card on Connections (installed
  version and path).
- **Delivers:** section 3.11, coding task, with the exact calls listed there.
- **Acceptance:** tests with a fake `claude` script that replays recorded JSON and stream-json output; an approval
  round trip through `gwen mcp`; worktree create and cleanup on a temp repo; Gwen's own test run decides the
  reported result. Manual: a small change in a scratch repo, from brief to branch.

#### W27: Gwen Terminal and the phone

Built first, before Wave 0. It needs no other package. It spans two repos: Gwen's, and `gwen-term` (3.14.3).

- **Owns:** in `gwen-term`, the `gwen` branch and its five commits (3.14.3). In Gwen: `internal/herdr` (new: the
  herdr-core client, with plain and framed JSON-RPC, events with snapshots and `missed`, the attach relay with
  credit, and a fake core for tests); `internal/remote` (new: code sessions and their states, the hook event
  handling, the `settings.json` merge, `gwen-term` control, the transcript parser, changes, the bubblewrap command,
  pairing, away detection, the inhibitor, and the `gwen-stt` client); `cmd/gwend/remote.go` (the remote and hook
  sockets, their route lists and auth, the terminal WebSocket, wiring); `internal/api/{code,remote}.go`;
  `internal/wire/code.go`; `internal/client/code.go` and its `clienttest` entries; migration `00011_remote.sql`;
  `internal/store/{code_sessions,remote_devices}.go`; the local state key `remote_owner` in
  `internal/store/localstate.go`; the `[remote]` section in `internal/config`; `cmd/gwen/{hook,remote,sandbox}.go`
  (the hidden `gwen hook`, `gwen remote status|on|off|pair|devices|revoke|ls`, and `gwen sandbox`); `cmd/gwen-stt`
  (new); `cmd/gwen-ui/remote.go` (host methods: publish on and off through `pkexec`, hooks on and off); `term/`
  (new: `core.version`, the `programs.json` entry, `themes.json`); `ui/DESIGN.md` (rewritten to Herdr's look, 5.1); `ui/mobile.html`, `ui/src/mobile/` (the phone app), `ui/embed_mobile.go`, the second
  entry in `ui/vite.config.ts`, `build:mobile`, `qrcode`, and `@xterm/xterm` in `ui/package.json`;
  `ui/src/settings/Phone.tsx`; the `Makefile` (`term`, `term-update`, `gwen-stt` with the voice libraries,
  `build:mobile`, a `ui-stub` placeholder for `ui/dist-mobile`); `packaging/nfpm.yaml` (`gwen-stt`); the Phone and
  Gwen Terminal sections of `setup.md`.
- **Delivers:** section 3.14, and the W27 rows of 3.9, 3.10, 3.13, 4, and rule 12 of section 7.
- **Acceptance:** tests for: each socket's route list (everything outside it is 404); the auth table (no login
  header, another login, no cookie, a revoked cookie, a peer that is not uid 0, a POST without `X-Gwen`); a pairing
  code used twice and one used after 10 minutes; the herdr-core client against the fake core (the generation and
  version check, reconnecting with a fresh snapshot, `missed`, stream credit and the 4 MiB repaint, close reasons);
  which terminals count as code sessions (OSC 7501 `app`, foreground `argv[0]`, neither) and the state table from
  recorded event sequences, including `done` becoming `idle` when opened; the `gwen-term do` arguments (window on
  and off, an existing space, a new space, a sandbox, a resume) against a fake `gwen-term`; one conversation keeps
  one card after Resume, and resuming a running conversation is refused; the conversations list on recorded
  transcripts (custom title, first message, sandbox tag, running); the `settings.json` merge (foreign hooks and key
  order kept, only Gwen's entries removed, invalid JSON untouched); `gwen hook` exits at once without
  `HC_TERMINAL_ID`; the transcript parser on recorded 2.1.322 fixtures, including unknown line types; the held
  permission (away and the phone decides; you return and it is released without a decision; 10 minutes pass);
  prompt `idem` and the `conflict` refusal; key messages capped at 32 names and 50 a second; the spaces list on a
  recorded `herdr-app` document; `put` with a photo; the hook pid check; changes on temporary repos
  (untracked, binary, no commits yet, before-session, not git); a golden test of the bubblewrap argument list;
  `gwen-stt` framing with a fake helper. Tests use `clock.NewFake` and never sleep. In `gwen-term`, upstream's `just
  check` passes on the `gwen` branch. Manual, with a throwaway `gwend` (6.1 rule 8, with `core_socket` set to your
  herdr-core; Serve pointed at its socket for the check and back afterwards) and a phone on the tailnet: `make term`,
  then check that it looks and works like Herdr apart from its name, its files are in `~/.config/gwen-term`, Claude
  (sandbox) is in its agent menu, and it opens no connection to herdr.dev (`ss -tp` while it runs); add one community mod and one community layout and see them work; pair; start
  a session in a scratch folder with a window and one without; `hc get <terminal>` shows Claude's status; send a
  prompt by text, one by voice, and one while typing at the laptop (it waits for you); type in the phone's Terminal tab and
  check that the laptop's tab keeps its size; swipe between panes; open Spaces and start a terminal in one; attach a
  photo in Compose; answer a permission by phone while at the laptop and one while the screen is locked;
  review Changes; close the window, reopen Gwen Terminal, and keep going in the same tab; `/exit`, then Continue it
  from the phone's New session; in a sandboxed session `go build` works, `ls ~/Documents` shows nothing, `hc ls`
  shows none of your terminals, and `ssh -T git@github.com` fails.
- **Done so far (2026-11-12):** everything in Owns is built, except that the tests of Acceptance are not written yet
  (they come in one batch) and the view's SSE fallback is left out (the WebSocket works; it has not yet been tried
  through Tailscale Serve). Checked live with a throwaway `gwend` and herdr-core, real Claude Code 2.1.322, and a
  phone-sized browser behind a stand-in for Serve: `make term` and the fork's `just check`; the app looks like Herdr
  under Gwen's name and opens no network connection; a session with a window and one without; status, prompt and
  `idem`; a permission answered at the laptop's dialog and one held while away; Chat and Changes; typing and the key
  bar with the laptop's grid unchanged; Open on laptop and a new terminal in a space; the card kept across a `gwend`
  restart; the route lists and the auth checks; the sandbox (Go builds, no `~/Documents`, no SSH keys, no terminals
  of yours, SSH to GitHub fails). **Not yet checked:** a real phone over Tailscale (HTTPS certificates are off in the
  tailnet and the phone has been offline), voice from the phone, a photo in Compose, pushes, keep-awake, and a
  community mod and layout.
- **Branch:** `w27-gwen-mobile` in Gwen, `gwen` in `gwen-term`.

#### W32: Gwen's agent

Needs W16 and W19. Specced with you on 2026-11-13 (3.16).

- **Owns:**
  - daemon: `internal/agent` (new: manager, the stream-json process, policy, command classifier, the system prompt,
    `rules.md`); Gwen's tools in `internal/api/agenttools.go`, built on the planning assistant's own `applyAction`
    so both change tasks the same way (one implementation; `internal/agent` reaches them through its `Sources`
    interface, which `api.Server` implements, so there is no import cycle and no `internal/actions` package);
    `OpenWindow` and `LinkTask` in `internal/remote/gwenterm.go`; the approval notification and its Open action in
    `cmd/gwend/loop.go`;
  - store and wire: migration `00017_agent.sql`; `internal/store/agent.go`; the `project_id` of
    `internal/store/chats.go` and its sync column;
  - API: `internal/api/agent.go`, with the adapter that gives `internal/agent` the profile card and the memory note
    from `internal/api/context.go`; `internal/wire/agent.go`; `internal/client/agent.go` and its `clienttest`
    entries; `cmd/gwend/agent.go` (`agent.sock` and the wiring);
  - commands: `cmd/gwen/mcp.go` (`gwen mcp`, which W23 extends); `gwen hook` and `gwen statusline` exiting under
    `GWEN_AGENT`;
  - config and projects: the `[agent]` section of `internal/config`; `agent_autonomy` on the project wire and in
    `PatchProject`; the `agent.sock` bind in the sandbox profile (`internal/remote/sandbox.go`);
  - UI: `ui/src/agent/` (new: the chat, the chat list, step rows, the session card);
    `ui/src/components/ApprovalCard.tsx` (rewritten on W19's approvals, with props `approval: wire.Approval` and
    `onDecide(decision, remember)`; W21 uses it); the Gwen tab of `ui/src/shell/SidePanel.tsx`;
    `ui/src/screens/Assistant.tsx` (Gwen › Chat); approval rows in `ui/src/shell/NeedsYou.tsx`;
    `ui/src/settings/Agent.tsx`.
- **Config:**

  ```toml
  [agent]
    enabled = true
    autonomy = "balanced"   # cautious, balanced, or trusted; a project can set its own
    idle_stop = "10m"
    max_live = 3
    turn_timeout = "20m"
  ```

- **Delivers:** section 3.16.
- **Acceptance:**
  - Policy table tests for every row and mode: `external` and `destructive` can never be remembered, the floor
    denies in Trusted, and a remembered prefix never matches a compound command.
  - Command classifier tests:
    - `cd x && go test ./...` is `shell.run` with the prefix `go test`;
    - `git push` is external; `rm -rf build` is destructive;
    - `echo $(cat ~/.ssh/id_ed25519)` and `sudo` hit the floor;
    - `curl -X POST` is external;
    - quoting cannot hide an operator.
  - A fake `claude` script that replays the recorded fixture drives a chat through `clienttest`:
    - init and streamed text;
    - an allowed edit and a denied command;
    - an approval round trip with Allow for this chat;
    - a queued message, and Stop;
    - idle stop, then resume with the same session id;
    - a crash mid-turn, and a `gwend` restart.
  - `gwen mcp` calls succeed with the process token, and are refused without it and on the main socket.
  - Undo of a created task works. `tsc` passes.
  - Manual, on a throwaway daemon, in a copy of this repo:
    - "why does the board filter reset?" reads files without asking;
    - "fix it" asks once for edits, then changes the file;
    - "run the UI typecheck" asks for `npx tsc`, with Always in this project;
    - "hand the phone pairing task to Claude" starts a linked session that shows in the chat;
    - "push it" asks every time.
- **Done so far (2026-11-13):** built, not yet tried in the installed window.
  - **Daemon.** Live-checked on a throwaway daemon with Claude Code 2.1.323 (Haiku), in a scratch repo:
    - reads ran without asking;
    - an edit asked once, then not again in that chat;
    - `python3 -m pytest` asked; remembered for the project, it then ran without asking;
    - `task_create` ran over `gwen mcp` without asking, and Undo removed the task;
    - `session_start` asked; a Deny with a note reached Claude, which did the work itself;
    - `git push` asked, couldn't be remembered, and wasn't retried after Deny;
    - Stop ended a waiting approval; a killed process showed its error and the next message resumed the session, as
      did a `gwend` restart;
    - a chat for everywhere answered from its context note, and the old planning chats list as chats for everywhere.
  - **Hardening after two security reviews:** Trusted runs only plain commands without asking; paths are checked
    with symbolic links followed, so a link to `~/.ssh` hits the floor.
  - **Window.** Side panel, Gwen › Chat, approval cards, Needs you, Settings › Gwen › Agent, and the autonomy per
    project. Checked in the preview harness, including one live turn: card, Allow, result.
  - **Go side:** `make check` passes. The UI typecheck fails only on `ui/src/mobile`, which is mid-rework in another
    session.
  - **Orchestrator (evening, after you tried it):** her chat lost Bash and Edit and got her own prompt; jobs run in
    auto mode in Trusted projects; she follows them and reports. Live-checked with three real jobs in a scratch repo:
    no permission prompts in Trusted; each job's card showed its live state; one report per finished job.
  - **Not done:**
    - sandboxed agent chats: the daemon keeps no per-project Sandbox default yet, and the sandbox sign-in isn't done;
      this needs that setting and the `agent.sock` bind;
    - a live `session_start` and `session_send` (they start real sessions);
    - the stream-json fixture and the tests above, which come with the test batch;
    - a token on `gwend.sock` for the approvals and grants endpoints.

#### Later packages

These get their full sections here when they start: **W33 Gwen's agent on the phone** (the phone's Chat uses
the agent's chats; agent approvals in the phone's Inbox, where `write` ones can be answered and `external` and
`destructive` ones need the laptop; Talk with the agent), **W24 MCP connectors** (add an MCP server by command or URL
on Connections; its tools become capabilities with risk `write` or higher until you lower them), **W25 notes
folder** (read and write Markdown in a chosen folder, for DSA notes and run summaries), **W26 browser automation**
(Playwright MCP, opt-in), **W28 Slack inbox** (read the channels and DMs you pick, pull out actionable points, and
hand one to a Claude session from W27 or a run from W19; Slack is a cloud service, which is fine here), **W29 Make
the phone yours** (layouts, mods, and themes for `gwen-mobile` in Herdr's style: a layout is a JSON tree; a mod is
`main.js` plus `mod.json` and describes its view with a portable subset of Herdr's builder API (`div`, `h_flex`,
`v_flex`, text, `svg`, `image`, `Button`, `Switch`, `Input`, and the flex and style methods), which Gwen draws as
DOM; `mod.json` names its targets, so one mod can run in Gwen Terminal, on the phone, or both; phone mods get no
`exec` and no files, only Gwen capabilities through `gwend`, each declared in `mod.json` and approved when added; a
guide like Herdr's `AGENTS.md`, so you ask Gwen to change the phone; an Android wrapper of the same app if device
features need one), **W30 Devices** (your other computers join as herdr-core machines over SSH on the tailnet: their
terminals and agents appear in the same Inbox, and "pull these files from the Pi to my phone" copies them between
machines with rsync over SSH, and to the phone as a download; it grows into harness G4's fabric), **W31 Home**
(proposed: smart devices on your Wi-Fi, such as a bulb, by chat or voice, through a local Home Assistant on the Pi,
which supports most brands and Matter without a cloud; each device action is a Gwen capability with its own risk).

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
9. **Memory is untrusted evidence until checked.** External text and model output cannot create an active claim
   without the source span, a verifier, and corroboration or your confirmation. A claim cannot grant a permission
   or change a policy.
10. **Memory answers cite their source.** When Gwen states a fact about the past, the response includes a claim,
    message, or episode reference. If no cited evidence supports it, Gwen says that she does not know.
11. **Optional memory services stay optional.** Mem0, Supermemory, vector search, and memory files are derived
    lanes. They run locally and loopback-only by default, with telemetry disabled. Rebuilding or removing a lane
    cannot erase Gwen's source episodes or claims.
12. **Gwen on the phone.** Only your tailnet identity on a paired device gets in. Each socket serves a fixed route
    list and fails closed. The phone never reaches herdr-core; `gwend` exposes only the operations of 3.14.7. Gwen never
    answers a Claude Code permission by herself: every answer comes from your tap or from the laptop. Pushes carry no
    code, commands, paths, or prompts. Prompt text, transcripts, terminal output, and diffs are never logged.
    Sandboxed sessions get the hook socket only. Gwen Terminal makes no connection to herdr.dev. A paired phone can
    type into any terminal in Gwen Terminal, as if you sat at the laptop, so pairing a phone is as strong as handing
    over the unlocked laptop: Revoke is one tap in Settings › Phone.
