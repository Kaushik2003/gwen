# Data model

The SQLite schema, the configuration file, and the rules that keep stored time honest. The SQL
blocks under the three schema sections are the literal contents of the migration files; copy them
verbatim.

## Principles

**Segments, not durations.** Time is stored as intervals (`segments`) and every total is a `SUM`
over them at query time. No table stores a precomputed "hours worked". This is what makes
[retroactive reclaim](05-time-engine.md#retroactive-reclaim) and after-the-fact editing safe: you
change an interval and every total is automatically correct.

**The daemon is the only writer.** No client opens the database file.

**Soft delete everywhere that syncs.** Synced tables are never hard-deleted; a delete sets
`deleted_at`. Every read of a synced table filters `deleted_at IS NULL` unless it is the sync code.

## Time representation

| Meaning | SQL type | Column naming | Go type |
|---|---|---|---|
| Instant | `INTEGER`, Unix milliseconds UTC | `*_at` | `time.Time` via `time.UnixMilli` |
| Local calendar date | `TEXT`, `YYYY-MM-DD` | `day`, `*_day` | `string` validated with `time.Parse("2006-01-02", s)` |
| Time of day | `INTEGER`, minutes after local midnight | `*_minute` | `int` |
| Duration | `INTEGER` | `*_minutes` or `*_seconds` | `time.Duration` |

Rules:

- Instants are truncated to the millisecond before storage. Never store a formatted timestamp.
- A local calendar date is used only where a human calendar day is genuinely meant: `work_days.day`,
  `tasks.due_day`, `tasks.occurrence_day`, `goals.start_day`, `goals.due_day`, `plan_items.day`,
  `commitments.active_from`, `commitments.active_until`. No other column holds a date.
- `work_days` records its IANA zone in `tz`, so a day tracked in one zone is still reported
  correctly after travel. All other date columns are interpreted in the device's current zone.
- Local-day arithmetic uses `time.Date(y, m, d+n, 0, 0, 0, 0, loc)`. Never add `24*time.Hour` —
  that is wrong twice a year across DST.
- Which work day an instant belongs to is decided by the time engine, not by the calendar; see
  [05-time-engine.md](05-time-engine.md#day-boundaries).

## Identifiers

Every synced row has a UUIDv7 primary key generated in Go with `uuid.NewV7()`, stored as the
36-character lowercase canonical `TEXT` form. UUIDv7 is time-ordered, so primary-key order is
roughly creation order and index locality stays good. Local-only tables use `INTEGER PRIMARY KEY`
or a natural key.

## Sync envelope

Every synced table (`projects`, `tasks`, `work_days`, `segments`, `goals`, `commitments`,
`plan_items`) ends with the same six columns:

| Column | Rule |
|---|---|
| `created_at` | Set once on insert. |
| `updated_at` | Set on every write, including the soft delete. |
| `deleted_at` | `NULL` while live; the delete instant once tombstoned. Tombstones are never purged. |
| `device_id` | The `device_id` of the device that made the latest write. |
| `rev` | Starts at 1, incremented by 1 on every write. |

Conflict resolution between devices is last-writer-wins on the tuple `(updated_at, device_id)`,
compared lexicographically; the greater tuple wins the whole row. `rev` is not part of conflict
resolution — it serves optimistic concurrency on the local API (see
[04-api-contract.md](04-api-contract.md#conventions)).

Local-only tables (`local_state`, `engine_events`, and the v3 integration caches) carry no envelope
and never leave the device.

## Invariants

Enforced by the schema where SQLite can express them, and by the store layer inside the write
transaction where it cannot. A write that would violate one returns `store.ErrConflict`, except
rule 7 which returns `store.ErrInvalid`.

1. At most one live work day is open (`clocked_out_at IS NULL`). *Schema.*
2. At most one live segment is open (`ended_at IS NULL`), and it belongs to the open work day.
   *Schema for the first half, store for the second.*
3. A closed work day has no open segment. *Store.*
4. Live segments of one work day never overlap. Touching is allowed (`a.ended_at = b.started_at`).
   *Store.*
5. Every live segment lies inside its work day's `[clocked_in_at, clocked_out_at]`. A segment edit
   that falls outside widens the bounds in the same transaction rather than failing. *Store.*
6. Break segments carry no project or task. *Schema.*
7. No live closed segment is shorter than 1000 ms. Closing a segment that would be shorter
   tombstones it instead; an API edit that would produce one is rejected. *Store.*
8. A work segment's `task_id`, when set, refers to a task whose `project_id` equals the segment's
   `project_id`, or whose `project_id` is `NULL`. *Store.*
9. Gaps between segments are allowed and count as neither work nor break. They arise only from user
   edits and from clocking back in on a day that was already clocked out.

## Derived totals

Every total in the API and dashboard is defined by one of these. The open segment counts up to
`:now`.

```sql
-- worked ms for a work day, by project (NULL project = "Unassigned")
SELECT project_id, SUM(COALESCE(ended_at, :now) - started_at) AS worked_ms
FROM segments
WHERE work_day_id = :work_day_id AND kind = 'work' AND deleted_at IS NULL
GROUP BY project_id;

-- worked and break ms for each day in a range (inclusive local dates)
SELECT d.day,
       d.target_seconds,
       COALESCE(SUM(CASE WHEN s.kind = 'work' THEN COALESCE(s.ended_at, :now) - s.started_at END), 0) AS worked_ms,
       COALESCE(SUM(CASE WHEN s.kind <> 'work' THEN COALESCE(s.ended_at, :now) - s.started_at END), 0) AS break_ms
FROM work_days d
LEFT JOIN segments s ON s.work_day_id = d.id AND s.deleted_at IS NULL
WHERE d.day BETWEEN :from_day AND :to_day AND d.deleted_at IS NULL
GROUP BY d.id
ORDER BY d.day;
```

- **Target met:** `worked_ms >= target_seconds * 1000`.
- **Current streak:** the number of consecutive calendar days, walking backwards, on which the
  target was met. The walk starts at today if today's target is already met, otherwise at
  yesterday. A calendar day with no work day breaks the streak.
- **Longest streak:** the longest such run in all history.
- **Heatmap value:** `worked_ms` per `day`; days without a work day are `0`.
- Time on a task is the same query as by project, grouped by `task_id`.

## Migrations

- Engine: `pressly/goose/v3`, SQL files embedded with `//go:embed migrations/*.sql` in
  `internal/store`.
- Naming: `internal/store/migrations/NNNNN_name.sql`, five-digit sequence.
- **Forward-only.** Files contain only a `-- +goose Up` section. There are no down migrations —
  SQLite cannot drop columns that carry foreign keys, so a down path would be a lie.
- **A shipped migration is never edited.** Changes go in a new file.
- Before applying any pending migration, the daemon writes a backup with
  `VACUUM INTO '<data dir>/gwen.db.bak-<current version>'`, keeping the two most recent backups.
  Rollback is restoring that file.
- Migrations run at daemon startup, before the socket is bound. Failure is fatal.

| File | Phase | Owner |
|---|---|---|
| `00001_init.sql` | v1 | W0 |
| `00002_planner.sql` | v2 | W10 |
| `00003_integrations.sql` | v3 | W12 |

### Connection

One `*sql.DB` for the process, opened with this DSN form (modernc driver):

```text
file:<data dir>/gwen.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_txlock=immediate
```

`SetMaxOpenConns(4)`. `_txlock=immediate` makes every transaction take the write lock at `BEGIN`,
so two writers queue on `busy_timeout` instead of failing mid-transaction with `SQLITE_BUSY`.

## Schema v1

```sql
-- +goose Up
-- file: internal/store/migrations/00001_init.sql

CREATE TABLE local_state (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE projects (
    id          TEXT PRIMARY KEY CHECK (length(id) = 36),
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    color       TEXT NOT NULL CHECK (color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'),
    archived_at INTEGER,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER,
    device_id   TEXT NOT NULL,
    rev         INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE UNIQUE INDEX projects_name_live ON projects (name COLLATE NOCASE) WHERE deleted_at IS NULL;

CREATE TABLE tasks (
    id               TEXT PRIMARY KEY CHECK (length(id) = 36),
    project_id       TEXT REFERENCES projects (id),
    title            TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    notes            TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
    priority         INTEGER NOT NULL DEFAULT 2 CHECK (priority BETWEEN 1 AND 4),
    due_day          TEXT CHECK (due_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    estimate_minutes INTEGER CHECK (estimate_minutes > 0),
    done_at          INTEGER,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER,
    device_id        TEXT NOT NULL,
    rev              INTEGER NOT NULL CHECK (rev >= 1),
    CHECK ((status = 'done') = (done_at IS NOT NULL))
) STRICT;

CREATE INDEX tasks_project ON tasks (project_id) WHERE deleted_at IS NULL;
CREATE INDEX tasks_open_due ON tasks (due_day) WHERE status = 'open' AND deleted_at IS NULL;

CREATE TABLE work_days (
    id             TEXT PRIMARY KEY CHECK (length(id) = 36),
    day            TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    tz             TEXT NOT NULL CHECK (length(tz) > 0),
    clocked_in_at  INTEGER NOT NULL,
    clocked_out_at INTEGER,
    target_seconds INTEGER NOT NULL CHECK (target_seconds >= 0),
    note           TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    deleted_at     INTEGER,
    device_id      TEXT NOT NULL,
    rev            INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (clocked_out_at >= clocked_in_at)
) STRICT;

CREATE UNIQUE INDEX work_days_day_live ON work_days (day) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX work_days_one_open ON work_days ((clocked_out_at IS NULL))
    WHERE clocked_out_at IS NULL AND deleted_at IS NULL;

CREATE TABLE segments (
    id          TEXT PRIMARY KEY CHECK (length(id) = 36),
    work_day_id TEXT NOT NULL REFERENCES work_days (id),
    kind        TEXT NOT NULL CHECK (kind IN ('work', 'break_auto', 'break_manual')),
    source      TEXT NOT NULL CHECK (source IN ('user', 'activity', 'idle', 'lock', 'suspend', 'recovery', 'edit')),
    project_id  TEXT REFERENCES projects (id),
    task_id     TEXT REFERENCES tasks (id),
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    truncated   INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER,
    device_id   TEXT NOT NULL,
    rev         INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (ended_at >= started_at),
    CHECK (kind = 'work' OR (project_id IS NULL AND task_id IS NULL)),
    CHECK (truncated = 0 OR ended_at IS NOT NULL)
) STRICT;

CREATE INDEX segments_day ON segments (work_day_id, started_at) WHERE deleted_at IS NULL;
CREATE INDEX segments_project ON segments (project_id, started_at) WHERE deleted_at IS NULL;
CREATE INDEX segments_task ON segments (task_id) WHERE task_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX segments_one_open ON segments ((ended_at IS NULL))
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE TABLE engine_events (
    id         INTEGER PRIMARY KEY,
    at         INTEGER NOT NULL,
    trigger    TEXT NOT NULL,
    from_state TEXT NOT NULL,
    to_state   TEXT NOT NULL,
    data       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data))
) STRICT;

CREATE INDEX engine_events_at ON engine_events (at);
```

`segments.source` records what *started* the segment:

| Value | Meaning |
|---|---|
| `user` | An explicit command: clock in, start or end a break, switch project. |
| `activity` | Input resumed after an automatic break. |
| `idle` | The hard idle threshold was crossed. |
| `lock` | The session locked. |
| `suspend` | The machine suspended. |
| `recovery` | Opened at startup after an unclean shutdown. |
| `edit` | Created by hand through the segment API. |

`engine_events` is an audit trail of every state transition: one row per transition, `trigger` and
the state names as defined in [05-time-engine.md](05-time-engine.md#states), and `data` a JSON
object of trigger-specific detail. Rows older than 90 days are deleted at daemon startup.

## Schema v2

```sql
-- +goose Up
-- file: internal/store/migrations/00002_planner.sql

CREATE TABLE goals (
    id               TEXT PRIMARY KEY CHECK (length(id) = 36),
    title            TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    kind             TEXT NOT NULL CHECK (kind IN ('quantity', 'tasks')),
    unit             TEXT NOT NULL DEFAULT '',
    target_quantity  INTEGER CHECK (target_quantity > 0),
    minutes_per_unit INTEGER CHECK (minutes_per_unit > 0),
    project_id       TEXT REFERENCES projects (id),
    start_day        TEXT NOT NULL CHECK (start_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    due_day          TEXT NOT NULL CHECK (due_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    status           TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'done', 'abandoned')),
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER,
    device_id        TEXT NOT NULL,
    rev              INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (due_day >= start_day),
    CHECK ((kind = 'quantity') = (target_quantity IS NOT NULL)),
    CHECK ((kind = 'quantity') = (minutes_per_unit IS NOT NULL))
) STRICT;

CREATE INDEX goals_active ON goals (due_day) WHERE status = 'active' AND deleted_at IS NULL;

ALTER TABLE tasks ADD COLUMN goal_id TEXT REFERENCES goals (id);
ALTER TABLE tasks ADD COLUMN quantity INTEGER CHECK (quantity > 0);
ALTER TABLE tasks ADD COLUMN quantity_done INTEGER CHECK (quantity_done >= 0);
ALTER TABLE tasks ADD COLUMN rrule TEXT;
ALTER TABLE tasks ADD COLUMN template_id TEXT REFERENCES tasks (id);
ALTER TABLE tasks ADD COLUMN occurrence_day TEXT CHECK (occurrence_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]');

CREATE INDEX tasks_goal ON tasks (goal_id) WHERE goal_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX tasks_occurrence ON tasks (template_id, occurrence_day)
    WHERE template_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE commitments (
    id                   TEXT PRIMARY KEY CHECK (length(id) = 36),
    title                TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    project_id           TEXT REFERENCES projects (id),
    rrule                TEXT NOT NULL,
    start_minute         INTEGER CHECK (start_minute BETWEEN 0 AND 1439),
    duration_minutes     INTEGER NOT NULL CHECK (duration_minutes BETWEEN 1 AND 1440),
    counts_toward_target INTEGER NOT NULL DEFAULT 1 CHECK (counts_toward_target IN (0, 1)),
    active_from          TEXT NOT NULL CHECK (active_from GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    active_until         TEXT CHECK (active_until GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    deleted_at           INTEGER,
    device_id            TEXT NOT NULL,
    rev                  INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (active_until >= active_from)
) STRICT;

CREATE TABLE plan_items (
    id              TEXT PRIMARY KEY CHECK (length(id) = 36),
    day             TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    task_id         TEXT NOT NULL REFERENCES tasks (id),
    planned_minutes INTEGER NOT NULL CHECK (planned_minutes > 0),
    start_at        INTEGER,
    position        INTEGER NOT NULL CHECK (position >= 0),
    status          TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'done', 'rolled', 'skipped')),
    pinned          INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
    rolled_from_id  TEXT REFERENCES plan_items (id),
    rollover_count  INTEGER NOT NULL DEFAULT 0 CHECK (rollover_count >= 0),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    deleted_at      INTEGER,
    device_id       TEXT NOT NULL,
    rev             INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX plan_items_day ON plan_items (day, position) WHERE deleted_at IS NULL;
CREATE INDEX plan_items_task ON plan_items (task_id) WHERE deleted_at IS NULL;
```

Column semantics for these tables — what `rrule` may contain, how `quantity_done` feeds pace, what
`pinned` protects — are owned by [06-planner.md](06-planner.md). A task with a non-`NULL` `rrule` is
a **template**; the rows generated from it have `template_id` and `occurrence_day` set.

## Schema v3

```sql
-- +goose Up
-- file: internal/store/migrations/00003_integrations.sql

CREATE TABLE gcal_links (
    plan_item_id TEXT PRIMARY KEY REFERENCES plan_items (id),
    event_id     TEXT NOT NULL UNIQUE,
    etag         TEXT NOT NULL,
    synced_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE calendar_busy (
    calendar_id TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    start_at    INTEGER NOT NULL,
    end_at      INTEGER NOT NULL,
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (calendar_id, event_id),
    CHECK (end_at > start_at)
) STRICT;

CREATE INDEX calendar_busy_range ON calendar_busy (start_at, end_at);

CREATE TABLE llm_runs (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind       TEXT NOT NULL CHECK (kind IN ('breakdown', 'retro')),
    subject_id TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('ok', 'failed', 'accepted', 'rejected')),
    output     TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX llm_runs_subject ON llm_runs (kind, subject_id, created_at);

CREATE TABLE hub_changes (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    row_id     TEXT NOT NULL,
    device_id  TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX hub_changes_row ON hub_changes (table_name, row_id);
```

All four are local-only. `hub_changes` is written only by a daemon running as the sync hub. Their
semantics are owned by [07-integrations.md](07-integrations.md#google-calendar),
[07-integrations.md](07-integrations.md#llm-adapter), and
[07-integrations.md](07-integrations.md#sync-hub).

## Local state keys

`local_state` holds device-local values. The key list is closed; adding a key is a spec change.

| Key | Value | Written by |
|---|---|---|
| `device_id` | UUIDv7, generated on first start, never changes | store bootstrap |
| `heartbeat_at` | Unix ms of the last engine heartbeat | engine loop, see [05](05-time-engine.md#crash-recovery) |
| `shutdown_at` | Unix ms of the last clean shutdown; removed at startup | engine loop, see [05](05-time-engine.md#crash-recovery) |
| `gcal_calendar_id` | ID of the dedicated Gwen calendar | gcal (v3) |
| `gcal_sync_token` | Calendar API `nextSyncToken` | gcal (v3) |
| `sync_push_watermark` | Unix ms, see [07](07-integrations.md#sync-hub) | sync (v3) |
| `sync_pull_cursor` | Opaque hub cursor | sync (v3) |

## Configuration

TOML at `~/.config/gwen/config.toml`, mode `0600`. This table is the single owner of every key, its
type, and its default; other docs define what the values *do*.

| Key | Type | Default | Behaviour defined in |
|---|---|---|---|
| `tracking.daily_target` | duration | `"8h"` | [05](05-time-engine.md#clock-in-and-out) |
| `tracking.soft_idle` | duration | `"3m"` | [05](05-time-engine.md#thresholds) |
| `tracking.hard_idle` | duration | `"10m"` | [05](05-time-engine.md#thresholds) |
| `tracking.day_rollover` | `"HH:MM"` | `"04:00"` | [05](05-time-engine.md#day-boundaries) |
| `nudge.desktop` | bool | `true` | [07](07-integrations.md#notifications) |
| `nudge.phone` | bool | `false` | [07](07-integrations.md#notifications) |
| `nudge.break_reminder` | duration | `"15m"` | [05](05-time-engine.md#nudges) |
| `nudge.repeat` | duration | `"10m"` | [05](05-time-engine.md#nudges) |
| `nudge.snooze` | duration | `"10m"` | [05](05-time-engine.md#nudges) |
| `ntfy.server` | URL | `"https://ntfy.sh"` | [07](07-integrations.md#notifications) |
| `ntfy.fallback_server` | URL or `""` | `""` | [07](07-integrations.md#notifications) |
| `ntfy.topic` | string | `""` | [07](07-integrations.md#notifications) |
| `planner.day_start` | `"HH:MM"` | `"09:00"` | [06](06-planner.md#capacity) |
| `planner.day_end` | `"HH:MM"` | `"23:00"` | [06](06-planner.md#capacity) |
| `planner.buffer` | duration | `"30m"` | [06](06-planner.md#capacity) |
| `calendar.enabled` | bool | `false` | [07](07-integrations.md#google-calendar) |
| `calendar.name` | string | `"Gwen"` | [07](07-integrations.md#google-calendar) |
| `calendar.busy_calendars` | list of string | `["primary"]` | [07](07-integrations.md#google-calendar) |
| `llm.provider` | `"none"`, `"anthropic"`, `"openai_compatible"` | `"none"` | [07](07-integrations.md#llm-adapter) |
| `llm.model` | string | `"claude-sonnet-5"` | [07](07-integrations.md#llm-adapter) |
| `llm.endpoint` | URL or `""` | `""` | [07](07-integrations.md#llm-adapter) |
| `llm.timeout` | duration | `"30s"` | [07](07-integrations.md#llm-adapter) |
| `sync.hub_url` | URL or `""` | `""` | [07](07-integrations.md#sync-hub) |
| `sync.interval` | duration | `"5m"` | [07](07-integrations.md#sync-hub) |
| `log.level` | `"debug"`, `"info"`, `"warn"`, `"error"` | `"info"` | [CONVENTIONS](CONVENTIONS.md#logging) |

Durations are Go `time.ParseDuration` strings. Loading rules, implemented in `internal/config`:

- A missing file means all defaults; the daemon then writes the defaults to disk with mode `0600`.
- A partial file overlays the defaults key by key.
- **Unknown keys are an error** (`toml.MetaData.Undecoded()` non-empty), naming the key. A typo
  must not silently fall back to a default.
- Validation failures are errors naming the key: any duration `<= 0`; `soft_idle >= hard_idle`;
  `day_start >= day_end`; malformed `HH:MM`; an unknown enum value.
- Secrets are never stored in this file. Tokens and API keys live in the credentials directory; see
  [07-integrations.md](07-integrations.md#credentials). `ntfy.topic` is the one sensitive value
  here and is redacted in logs.
- Writes (from `PATCH /v1/config`) are atomic: write `config.toml.tmp`, `fsync`, rename.
