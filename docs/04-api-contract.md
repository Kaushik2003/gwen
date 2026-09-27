# API contract

The complete local API served by `gwend` on its Unix socket. **This contract is frozen**: client
packages build against it before the daemon exists, so an endpoint, field, or event not listed here
does not exist. Changing it follows [README.md](README.md#change-protocol) and is additive within
`/v1`.

## Conventions

- Transport is described in [02-architecture.md](02-architecture.md#transport). Requests use the
  host `gwend`, e.g. `http://gwend/v1/status`.
- Bodies are JSON, UTF-8, `Content-Type: application/json`. Request bodies are decoded with
  `DisallowUnknownFields`; an unknown field is `invalid_request`. Commands with no parameters take
  `{}`.
- **Wire field names and units are the column names and units from
  [03-data-model.md](03-data-model.md#time-representation).** Instants are `int64` ms (`*_at`),
  dates are `"YYYY-MM-DD"`. Values computed at request time are durations in milliseconds named
  `*_ms`.
- Nullable fields are always present, with `null`. Fields are never omitted.
- Envelope fields on the wire: `id`, `created_at`, `updated_at`, `rev`. `deleted_at` and
  `device_id` are never sent; deleted rows are never returned.
- **Optimistic concurrency:** every `PATCH` body accepts an optional `rev`. If present and not equal
  to the stored `rev`, the response is `conflict` and nothing is written.
- `PATCH` fields are all optional; an absent field is unchanged, and `null` sets a nullable field to
  null.
- Status codes: `200` for reads, commands and updates; `201` with the created object for `POST`
  creates; `204` with no body for `DELETE`. `DELETE` is always a soft delete.
- Lists are returned as `{"<plural>": [...]}`, unpaginated. Range queries take `from` and `to`
  local dates, inclusive, at most 366 days apart; violating that is `invalid_request`.
- Go types for every object and request body live in `internal/wire`, one struct per object named
  exactly as the headings below (`wire.Status`, `wire.Segment`, `wire.ClockInRequest`).
  `internal/client` exposes one method per endpoint on the `client.API` interface.

## Errors

Every non-2xx response has this body:

```json
{"error": {"code": "invalid_state", "message": "cannot start a break while off", "details": {"state": "off"}}}
```

| `code` | HTTP | When | Go source |
|---|---|---|---|
| `invalid_request` | 400 | Malformed JSON, unknown field, failed validation. `details.field` names the field. | decode errors, `store.ErrInvalid`, `config` validation |
| `not_found` | 404 | The id or day does not exist or is deleted. | `store.ErrNotFound` |
| `conflict` | 409 | `rev` mismatch, uniqueness violation, overlapping segments. | `store.ErrConflict` |
| `invalid_state` | 422 | The command is not allowed in the current engine state. `details.state` is the state. | `timeengine.ErrInvalidState` |
| `unavailable` | 503 | The feature is not configured (calendar disabled, LLM provider `none`, no sync hub). | `api.ErrUnavailable` |
| `internal` | 500 | Anything else. Logged at `Error` with the cause; `message` is always `"internal error"`. | everything unmapped |

`message` is a lowercase human-readable sentence safe to show directly in a client. The mapping
function in `internal/api` is the only code that produces these responses.

Client-side, `internal/client` returns `*client.APIError{Status, Code, Message, Details}` for these,
and `client.ErrDaemonNotRunning` when the socket is missing or refuses the connection.

## Objects

### Status

The engine snapshot. Clients render the ticking timer from `open_segment.started_at` and their own
clock; they never poll for it.

| Field | Type | Meaning |
|---|---|---|
| `state` | string | One of the states in [05-time-engine.md](05-time-engine.md#states). |
| `state_since_at` | int64 | When the current state was entered. |
| `work_day` | WorkDay or null | The open work day. |
| `open_segment` | Segment or null | The open segment. |
| `project_id` | string or null | Project of the open work segment, or the one work will resume on. |
| `task_id` | string or null | Same, for the task. |
| `idle_since_at` | int64 or null | Last input instant; set only in `idle_pending`. |
| `snoozed_until_at` | int64 or null | Nudges suppressed until this instant. |
| `today` | DaySummary or null | Totals for the open work day, computed at `server_now_at`. |
| `server_now_at` | int64 | Daemon clock at response time. Clients compute skew from it. |
| `activity_backend` | string | `wayland`, `swayidle`, `dbus_screensaver`, `xprintidle`, or `none`. |
| `warnings` | string[] | Degradation notices from [02-architecture.md](02-architecture.md#failure-and-degradation); empty when healthy. |

### WorkDay

`id`, `day`, `tz`, `clocked_in_at`, `clocked_out_at`, `target_seconds`, `note`, `created_at`,
`updated_at`, `rev`.

### Segment

`id`, `work_day_id`, `kind`, `source`, `project_id`, `task_id`, `started_at`, `ended_at`,
`truncated` (bool), `created_at`, `updated_at`, `rev`.

### DaySummary

| Field | Type | Meaning |
|---|---|---|
| `day` | string | Local date. |
| `target_seconds` | int | From the work day. |
| `worked_ms` | int64 | Per [03-data-model.md](03-data-model.md#derived-totals). |
| `break_ms` | int64 | Same. |
| `target_met` | bool | Same. |
| `by_project` | ProjectTotal[] | Ordered by `worked_ms` descending. |

**ProjectTotal:** `project_id` (null for unassigned), `name` (`"Unassigned"` when null), `color`
(`"#6b7280"` when null), `worked_ms`. Name and color are resolved even for deleted projects, so
history always renders.

### Project

`id`, `name`, `color`, `archived_at`, `created_at`, `updated_at`, `rev`.

When `color` is omitted on create, the daemon assigns the first colour in this palette not used by
a live project, cycling from the start when all are used: `#3b82f6`, `#10b981`, `#f59e0b`,
`#ef4444`, `#8b5cf6`, `#ec4899`, `#14b8a6`, `#f97316`.

### Task

`id`, `project_id`, `title`, `notes`, `status`, `priority`, `due_day`, `estimate_minutes`,
`done_at`, `created_at`, `updated_at`, `rev`, plus the computed `tracked_ms` (all-time work on this
task). From v2 also `goal_id`, `quantity`, `quantity_done`, `rrule`, `template_id`,
`occurrence_day`; in v1 responses these are present and `null`.

### Config

A JSON object mirroring the TOML structure in
[03-data-model.md](03-data-model.md#configuration) exactly: `{"tracking": {"soft_idle": "3m", ...},
"nudge": {...}, ...}`. Durations stay strings.

## Endpoints — v1

Implemented by W5. Every tracking command runs through the engine, returns the resulting `Status`,
and returns `invalid_state` when [05-time-engine.md](05-time-engine.md#transitions) does not allow
it.

### Health and status

| Method and path | Body | Returns |
|---|---|---|
| `GET /v1/health` | — | `{"ok": true, "version": "1.0.0", "schema_version": 1, "pid": 4242}` |
| `GET /v1/status` | — | Status |

### Tracking commands

| Method and path | Body | Effect |
|---|---|---|
| `POST /v1/day/clock-in` | `ClockInRequest {project_id, task_id}` (both nullable) | Starts or reopens today's work day. |
| `POST /v1/day/clock-out` | `{}` | Closes the open segment and the work day. |
| `POST /v1/break/start` | `{}` | Starts a manual break. |
| `POST /v1/break/end` | `{}` | Ends any break and resumes work. |
| `POST /v1/session/switch` | `SwitchRequest {project_id, task_id}` (both nullable) | Changes attribution; see [05](05-time-engine.md#switching). |
| `POST /v1/nudge/snooze` | `{}` | Suppresses nudges for `nudge.snooze`. |

`project_id` or `task_id` that does not exist is `not_found`; a task whose project differs from
`project_id` is `invalid_request`. When `task_id` is set and `project_id` is null, the task's
project is used.

### Days and segments

| Method and path | Body or query | Returns |
|---|---|---|
| `GET /v1/days` | `?from=&to=` | `{"days": [DaySummary]}` — only days with a work day, ascending. |
| `GET /v1/days/{day}` | — | `{"work_day": WorkDay, "summary": DaySummary, "segments": [Segment]}`, segments by `started_at`. |
| `PATCH /v1/days/{day}` | `{target_seconds, note, rev}` | WorkDay |
| `POST /v1/segments` | `CreateSegmentRequest {day, kind, project_id, task_id, started_at, ended_at}` | `201` Segment |
| `PATCH /v1/segments/{id}` | `{kind, project_id, task_id, started_at, ended_at, rev}` | Segment |
| `POST /v1/segments/{id}/split` | `{"at": int64}` | `{"segments": [Segment, Segment]}` |
| `DELETE /v1/segments/{id}` | — | `204` |

Rules:

- `POST /v1/segments` creates a closed segment with `source = "edit"`; `ended_at` is required. If no
  work day exists for `day`, a closed one is created with `tz` set to the device's zone, `target_seconds`
  from `tracking.daily_target`, and bounds equal to the segment.
- The open segment belongs to the engine. `PATCH`, `split`, and `DELETE` on it return
  `invalid_state`; use `session/switch` instead.
- `split` requires `at` strictly inside the segment with both halves at least 1000 ms; otherwise
  `invalid_request`. The first half keeps the id.
- Every write here enforces [03-data-model.md](03-data-model.md#invariants); an overlap is
  `conflict`. For overlap the open segment runs on indefinitely, and no segment written here may end
  after the daemon's clock (`invalid_request`), so an edit can never collide with where the engine
  will close the open segment.
- A `task_id` given without a `project_id` takes the task's project, as in the tracking commands.
- Changing `kind` to a break kind clears `project_id` and `task_id`.

### Projects

| Method and path | Body or query | Returns |
|---|---|---|
| `GET /v1/projects` | `?archived=false` (default), `true`, or `all` | `{"projects": [Project]}` by name, case-insensitive |
| `POST /v1/projects` | `{name, color}` (`color` optional) | `201` Project |
| `GET /v1/projects/{id}` | — | Project |
| `PATCH /v1/projects/{id}` | `{name, color, archived, rev}` (`archived` is a bool) | Project |
| `DELETE /v1/projects/{id}` | — | `204`; also soft-deletes its tasks in the same transaction |

A duplicate live name, case-insensitive, is `conflict`. Deleting or archiving the project of the
open segment is `invalid_state`.

### Tasks

| Method and path | Body or query | Returns |
|---|---|---|
| `GET /v1/tasks` | `?project_id=&status=open` (default), `done`, `all`; `&due_before=YYYY-MM-DD` | `{"tasks": [Task]}` |
| `POST /v1/tasks` | `{project_id, title, notes, priority, due_day, estimate_minutes}` | `201` Task |
| `GET /v1/tasks/{id}` | — | Task |
| `PATCH /v1/tasks/{id}` | same fields as create, plus `rev` | Task |
| `POST /v1/tasks/{id}/complete` | `{}` (v2 adds optional `quantity_done`) | Task |
| `POST /v1/tasks/{id}/reopen` | `{}` | Task |
| `DELETE /v1/tasks/{id}` | — | `204` |

- List order: `due_day` ascending with nulls last, then `priority` descending, then `created_at`
  ascending. `due_before` is exclusive.
- Create defaults: `notes` `""`, `priority` `2`, the rest `null`.
- `complete` and `reopen` are idempotent: applying either to a task already in that status returns
  `200` with the task unchanged.
- Deleting the task of the open segment is `invalid_state`.

### Stats

| Method and path | Query | Returns |
|---|---|---|
| `GET /v1/stats/summary` | `?from=&to=` | StatsSummary |
| `GET /v1/stats/heatmap` | `?year=2026` | `{"year": 2026, "days": [{"day", "worked_ms"}]}` — days with `worked_ms > 0` only, ascending |

**StatsSummary:** `from`, `to`, `worked_ms`, `break_ms`, `days_tracked`, `days_target_met`,
`avg_worked_ms` (over tracked days, `0` when none), `by_project` (ProjectTotal[]),
`current_streak`, `longest_streak`. Streaks are all-time regardless of range, as defined in
[03-data-model.md](03-data-model.md#derived-totals).

### Config

| Method and path | Body | Returns |
|---|---|---|
| `GET /v1/config` | — | Config |
| `PATCH /v1/config` | Partial Config, nested | Config |

Validation per [03-data-model.md](03-data-model.md#configuration); a failure is `invalid_request`
with `details.key` set to the dotted key. A successful patch is applied to the running engine
immediately and emits `config_changed`.

### Notifications

| Method and path | Body | Returns |
|---|---|---|
| `POST /v1/notify/test` | `{}` | `{"desktop": "ok", "phone": "disabled"}` |

Sends a `Gwen test` / `Notifications are working.` notification through each backend, ignoring
snooze and nudge state. Each field is `"ok"`, `"disabled"` (turned off in config, or no topic), or
the backend's error message.

## Endpoints — v2

Implemented by W10. Computation rules are owned by [06-planner.md](06-planner.md).

| Method and path | Body or query | Returns |
|---|---|---|
| `GET /v1/goals` | `?status=active` (default), `done`, `abandoned`, `all` | `{"goals": [Goal]}` by `due_day` |
| `POST /v1/goals` | `{title, kind, unit, target_quantity, minutes_per_unit, project_id, start_day, due_day}` | `201` Goal |
| `GET /v1/goals/{id}` | — | Goal |
| `PATCH /v1/goals/{id}` | create fields plus `status`, `rev` | Goal |
| `DELETE /v1/goals/{id}` | — | `204` |
| `GET /v1/commitments` | — | `{"commitments": [Commitment]}` |
| `POST /v1/commitments` | `{title, project_id, rrule, start_minute, duration_minutes, counts_toward_target, active_from, active_until}` | `201` Commitment |
| `PATCH /v1/commitments/{id}` | create fields plus `rev` | Commitment |
| `DELETE /v1/commitments/{id}` | — | `204` |
| `GET /v1/plan` | `?day=YYYY-MM-DD` | Plan |
| `POST /v1/plan/generate` | `{"day": "YYYY-MM-DD"}` | Plan |
| `PATCH /v1/plan/items/{id}` | `{start_at, position, pinned, planned_minutes, status, rev}` (`status` only `planned` or `skipped`) | PlanItem |
| `GET /v1/briefing` | — | Briefing |

v2 also extends `GET /v1/tasks` with `&goal_id=` and `&templates=true` (templates are excluded
unless set), and `POST`/`PATCH /v1/tasks` accept `goal_id`, `quantity`, `rrule`.

- **Goal:** all `goals` columns on the wire, plus `progress`: `{done_quantity, remaining_quantity,
  required_per_day, actual_per_day, pace, projected_finish_day}` where `pace` is `ahead`,
  `on_track`, or `behind`. For `kind = "tasks"` the quantities count tasks.
- **Commitment:** all `commitments` columns on the wire; `counts_toward_target` is a bool.
- **Plan:** `{day, capacity_minutes, planned_minutes, items: [PlanItem]}`, items by `position`.
- **PlanItem:** all `plan_items` columns on the wire (`pinned` a bool), plus `task` (embedded Task).
- **Briefing:** `{day, pending: [PlanItem], today: [PlanItem], reminders: [Reminder], goals: [Goal]}`.
  **Reminder:** `{task_id, title, due_day, days_left, message}`.

## Endpoints — v3

Implemented by W12, W13, and W14. Behaviour is owned by [07-integrations.md](07-integrations.md).
All return `unavailable` when their feature is not configured.

| Method and path | Body | Returns |
|---|---|---|
| `GET /v1/calendar/status` | — | CalendarStatus |
| `POST /v1/calendar/auth/start` | `{}` | `{"auth_url": "https://accounts.google.com/..."}` |
| `POST /v1/calendar/sync` | `{}` | CalendarStatus, after the sync completes |
| `POST /v1/goals/{id}/breakdown` | `{"instructions": "string"}` | LlmRun |
| `POST /v1/retro` | `{"week_start": "YYYY-MM-DD"}` | LlmRun |
| `GET /v1/llm/runs/{id}` | — | LlmRun |
| `POST /v1/llm/runs/{id}/accept` | `{"indexes": [0, 2]}` | `{"tasks": [Task]}` |
| `POST /v1/llm/runs/{id}/reject` | `{}` | LlmRun |
| `GET /v1/sync/status` | — | SyncStatus |
| `POST /v1/sync/now` | `{}` | SyncStatus, after the sync completes |

- **CalendarStatus:** `{enabled, connected, calendar_id, last_sync_at, last_error}`.
- **LlmRun:** all `llm_runs` columns on the wire, `output` as a JSON object whose shape per `kind` is
  defined in [07-integrations.md](07-integrations.md#llm-adapter).
- **SyncStatus:** `{configured, last_push_at, last_pull_at, last_error}`.

The sync hub's own network API is not part of this socket API; it is specified in
[07-integrations.md](07-integrations.md#sync-hub).

## Events

`GET /v1/events` is a Server-Sent Events stream (`Content-Type: text/event-stream`).

- Frame format: `id: <n>\nevent: <name>\ndata: <json>\n\n`, with `n` increasing from 1 per daemon
  run.
- **The first frame on every connection is `state_changed` with the current Status.** A client
  therefore never needs a separate `GET /v1/status` to initialise.
- There is no replay. After a reconnect a client treats all cached lists as stale and refetches.
- A `: ping` comment is sent every 25 s. Clients reconnect on disconnect with backoff 1 s doubling
  to 30 s.
- Events are invalidations: apart from `state_changed`, `nudge_fired`, and `config_changed` they
  carry identifiers, and the client refetches what it displays.

| Event | `data` | Emitted when |
|---|---|---|
| `state_changed` | Status | Any engine transition, snooze, or attribution change. |
| `day_changed` | `{"day": "YYYY-MM-DD"}` | Any work day or segment of that day is written, by the engine or an edit. |
| `nudge_fired` | `{"kind", "at", "title", "body"}` | A nudge is sent; `kind` per [05](05-time-engine.md#nudges). |
| `projects_changed` | `{}` | Any project write. |
| `tasks_changed` | `{"task_ids": [string]}` | Any task write. |
| `config_changed` | Config | A successful config patch. |
| `plan_changed` | `{"day": "YYYY-MM-DD"}` | v2: a plan for that day is generated or edited. |
| `goals_changed` | `{}` | v2: any goal or commitment write. |
| `integration_changed` | `{"calendar": CalendarStatus, "sync": SyncStatus}` | v3: after any calendar or sync attempt. |
