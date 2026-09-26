# Planner

v2. Turns goals, tasks and commitments into a concrete plan for a day, carries unfinished work
forward, and tells the user what to start now. **Fully deterministic**: same inputs, same plan. No
network and no LLM; the v3 LLM adapter only proposes tasks that then enter this pipeline like any
other ([07-integrations.md](07-integrations.md#llm-adapter)).

## Shape

`internal/planner` is pure, like the time engine: plain functions over values, no store access, no
clock reads. W10 loads inputs from the store, calls these, and writes the results in one
transaction.

```go
func Occurrences(rule string, anchor, from, to civil.Day) ([]civil.Day, error)
func Progress(g Goal, tasks []Task, today civil.Day) GoalProgress
func Capacity(in CapacityInput) CapacityResult
func Urgency(t Task, day civil.Day, progress map[string]GoalProgress, rolloverCount int) float64
func Rollover(items []PlanItem, tasks map[string]Task, today civil.Day) RolloverResult
func Generate(in GenerateInput) []PlanItemDraft
func Reminders(in ReminderInput) []Reminder
```

`civil.Day` is a small value type defined in `internal/planner` (`struct{ Y int; M time.Month; D int }`)
with `String()` returning `YYYY-MM-DD`; day arithmetic follows
[03-data-model.md](03-data-model.md#time-representation). **Today** always means the day the current
instant belongs to per [05-time-engine.md](05-time-engine.md#day-boundaries).

## Recurrence

`tasks.rrule` and `commitments.rrule` accept a strict subset of RFC 5545 RRULE, parsed by a
hand-written parser in `internal/planner/rrule.go`. Parts are `;`-separated `KEY=VALUE`, in any
order.

| Part | Allowed |
|---|---|
| `FREQ` | Required. `DAILY`, `WEEKLY`, or `MONTHLY`. |
| `INTERVAL` | Optional positive integer, default 1. |
| `BYDAY` | `WEEKLY` only. Comma list of `MO TU WE TH FR SA SU`. Default: the anchor's weekday. |
| `BYMONTHDAY` | `MONTHLY` only, and required there. Integer 1–28. |
| `UNTIL` | Optional date in `YYYYMMDD` form. Inclusive. |

Anything else — `COUNT`, `BYSETPOS`, times in `UNTIL`, `BYMONTHDAY` above 28 — is rejected, and the
API returns `invalid_request` with `details.field = "rrule"`.

**Anchor.** A commitment's anchor is `active_from`. A task template's anchor is the day its
`created_at` belongs to.

**Matching** a day `d ≥ anchor`:

- `DAILY`: `days(anchor, d) % INTERVAL == 0`.
- `WEEKLY`: `weekday(d) ∈ BYDAY` and `weeks(monday(anchor), monday(d)) % INTERVAL == 0`, where
  `monday(x)` is the Monday on or before `x`.
- `MONTHLY`: `d.D == BYMONTHDAY` and `months(anchor, d) % INTERVAL == 0`.

### Task templates and occurrences

A task with an `rrule` is a **template**: it is never planned, completed, or listed unless asked for
(`?templates=true`). For each matching day, an **occurrence** task is materialized: a copy of the
template's `project_id`, `title`, `notes`, `priority`, `estimate_minutes`, `goal_id`, with
`template_id` set and `occurrence_day` = the matching day.

- An occurrence's `due_day` is the day before the template's next matching day, capped at `UNTIL`.
  So a daily occurrence is due the same day and a weekly one is due before the next week's
  occurrence.
- Occurrences are materialized for a day `D` when a plan is generated for `D`. The unique index
  `tasks_occurrence` makes this idempotent.
- An occurrence is **expired** once today is past its `due_day` while it is still open. Expired
  occurrences are soft-deleted during [Rollover](#rollover). Missed recurring work does not pile
  up; for goals, the shortfall flows into the next occurrence's quantity instead.
- Editing a template affects only occurrences materialized afterwards.

## Goals and progress

Two kinds, per `goals.kind`:

- **`quantity`** — "300 problems". Creating one also creates, in the same transaction, its
  **session template**: a task with `rrule = "FREQ=DAILY"`, `title` = the goal's title,
  `goal_id` set, `project_id` from the goal, `priority = 3`. The user can edit that template's
  `rrule` (e.g. weekdays only). Deleting the goal soft-deletes the template and its open
  occurrences.
- **`tasks`** — a goal measured by completing a set of tasks the user (or the v3 LLM) links to it.
  No template.

`Progress` computes, for today `T`:

| Value | `quantity` goal | `tasks` goal |
|---|---|---|
| `target` | `target_quantity` | count of live non-template tasks with this `goal_id` |
| `done_quantity` | `SUM(quantity_done)` over done live tasks with this `goal_id` | count of those tasks that are done |
| `remaining_quantity` | `max(target − done, 0)` | same |

- `days_total` = days from `start_day` to `due_day` inclusive.
- `days_elapsed` = days from `start_day` to `T` inclusive, clamped to `[0, days_total]`.
- `sessions_left` = for `quantity` goals, the number of session-template matching days from `T` to
  `due_day` inclusive; for `tasks` goals, days from `T` to `due_day` inclusive. If `T > due_day`,
  `sessions_left = 0`.
- `required_per_day` = `remaining / sessions_left`, or `remaining` when `sessions_left = 0`.
- `actual_per_day` = `done / max(days_elapsed, 1)`.
- `expected_by_now` = `target × days_elapsed / days_total`.
- `pace`: with `ratio = done / expected_by_now` (`ratio = 1` when `expected_by_now = 0`): `ahead`
  when `ratio ≥ 1.1`, `behind` when `ratio < 0.9`, otherwise `on_track`.
- `projected_finish_day` = `T + ceil(remaining / actual_per_day) − 1` days when `actual_per_day > 0`
  and `remaining > 0`; `T` when `remaining = 0`; otherwise `null`.

When a task completion brings `remaining` to 0, the goal's `status` becomes `done` in the same
transaction. Reopening a task that makes `remaining > 0` again sets a `done` goal back to `active`.
`abandoned` is only ever set by the user.

**Session quantity.** A session occurrence materialized for day `D` gets
`quantity = max(1, ceil(remaining / sessions_left))` computed with `T = D`, and
`estimate_minutes = quantity × minutes_per_unit`. If `remaining = 0` no occurrence is created. On
completion, `quantity_done` defaults to `quantity` unless the request supplies it.

Worked example — 300 problems, 30 min each, 2026-09-15 to 2026-12-15, daily: 92 sessions, so the
first session is `ceil(300/92) = 4` problems, 120 minutes. Solve only 3 that day and the next day's
session is `ceil(297/91) = 4` again; fall further behind and it grows.

## Capacity

Minutes available for planned work on day `D`.

1. **Window**: `[planner.day_start, planner.day_end)` local on `D`.
2. **Commitment occurrences** on `D`: commitments whose `rrule` matches `D` and whose
   `[active_from, active_until]` contains `D`. A commitment with `start_minute` occupies the
   interval `[start_minute, start_minute + duration_minutes)`; one without is **floating**.
3. **Busy** (v3 only, else empty): `calendar_busy` intervals intersecting the window on `D`.
4. **Free intervals** `F` = the window minus fixed commitment intervals minus busy intervals, and
   when `D` is today, minus `[day_start, now rounded up to the next 5 minutes)`.
5. `slot_minutes` = total length of `F` − the durations of all floating commitments.
6. `target_left` = `target_minutes` − the durations of commitments with `counts_toward_target = 1`.
   `target_minutes` is the open work day's `target_seconds / 60` when `D` is today and a work day
   exists, else `tracking.daily_target`. When `D` is today, also subtract minutes already worked
   today on work segments whose project is **not** the `project_id` of a counting commitment (that
   time is already represented by the commitment).
7. `capacity_minutes = max(0, min(target_left, slot_minutes) − planner.buffer)`.

For the motivating case — 8 h target, a counting 5 h internship commitment on weekdays, 09:00–23:00
window, 30 m buffer — a fresh weekday has `min(480 − 300, 840 − 300) − 30 = 150` minutes of
capacity.

## Urgency

For each candidate task on day `D`:

| Factor | Definition |
|---|---|
| `due` | `clamp(1 − days_until_due / 14, 0, 1)`, where `days_until_due` is days from `D` to `due_day` (negative when overdue, so it clamps to 1). `0` when `due_day` is null. |
| `pace` | For a task with a `goal_id` whose goal is active: `clamp((required_per_day − actual_per_day) / required_per_day, 0, 1)`, and `0` when `required_per_day = 0`. Otherwise `0`. |
| `prio` | `(priority − 1) / 3`. |
| `age` | `clamp(rollover_count / 5, 0, 1)`, with `rollover_count` as defined in [Generating a plan](#generating-a-plan). |

```text
urgency = 0.40·due + 0.25·pace + 0.20·prio + 0.15·age
```

The weights are constants in `internal/planner`, not configuration. Ties break by `due_day`
ascending with nulls last, then `priority` descending, then `created_at` ascending, then `id`.

## Rollover

Runs for today before any plan read or generation for today, and is idempotent. For every live plan
item with `status = 'planned'` and `day` before today:

1. Task done → item `done`.
2. Task deleted, or an expired occurrence → item `skipped`; the expired occurrence is soft-deleted.
3. Otherwise → item `rolled`.

The **pending** list in the briefing is, for each still-open task, its most recent `rolled` item —
provided the task has no plan item with a later `day`.

## Generating a plan

`POST /v1/plan/generate` for `D`, and implicitly when `GET /v1/plan` or `GET /v1/briefing` finds no
live plan items for today. `D` before today is `invalid_request`.

1. Run [Rollover](#rollover) if `D` is today. Materialize occurrences for `D`.
2. Keep every item on `D` that is pinned or not `planned`. Soft-delete the rest.
3. `capacity_left = capacity_minutes − Σ planned_minutes of pinned planned items`.
4. **Pool**: live, open, non-template tasks, excluding occurrences whose `occurrence_day` is after
   `D` or whose `due_day` is before `D`, and excluding tasks that already have a kept item on `D`.
5. For each task, `remaining_minutes` = `estimate_minutes` (60 when null) minus all-time tracked
   minutes on the task; when that is below 25, use 25.
6. For each task, `rollover_count` = the most recent earlier item's `rollover_count + 1` if that item
   is `rolled`, else 0. Sort the pool by urgency, descending.
7. Walk the pool. For each task, while it has fewer than 2 blocks on `D`, `remaining_minutes > 0`
   and `capacity_left ≥ 25`: `block = min(remaining_minutes, 90, capacity_left)` rounded down to a
   multiple of 5; stop if `block < 25`; create an item with `planned_minutes = block`, the task's
   `rollover_count`, and `rolled_from_id` = that most recent rolled item when there is one.
   Subtract `block` from both counters.
8. **Positions**: kept items first, ordered by `start_at` with nulls last, then new items in
   creation order, numbered from 0.
9. **Slotting** (sets `start_at` on new items): walk new items in position order and place each at
   the earliest instant in `F` where it fits entirely, after removing intervals held by pinned
   items with a `start_at`. Leave a 10-minute gap after any 90-minute block. An item that fits
   nowhere keeps `start_at = null` and stays planned.

Constants — block 25 to 90 minutes, 5-minute rounding, 2 blocks per task per day, 60-minute default
estimate, 10-minute gap — live in `internal/planner`, not configuration.

**Completion.** Completing a task marks all its `planned` items `done`. Reopening a task sets its
`done` items on today back to `planned`.

**User edits** through `PATCH /v1/plan/items/{id}` that change `start_at`, `position`, or
`planned_minutes` also set `pinned = 1`, so regeneration never undoes a manual choice.

## Reminders

Computed for the briefing over live, open, non-template tasks with a `due_day`. Rules are tried in
order; a task gets at most the first that matches.

| Rule | Condition | `message` |
|---|---|---|
| overdue | `days_left < 0` | `Overdue by {n} day` / `Overdue by {n} days` |
| due today | `days_left = 0` | `Due today` |
| due tomorrow | `days_left = 1` | `Due tomorrow` |
| start now | `2 ≤ days_left ≤ 14` and `remaining_minutes > 0.25 × avail` | `Start now — due in {n} days, about {h}h of work left` |
| not started | `2 ≤ days_left ≤ 7`, no tracked time, and no item on today | `Not started — due in {n} days` |

- `days_left` = days from today to `due_day`. `remaining_minutes` as in step 5 above.
- `avail` = sum of `capacity_minutes` for each day from today to the day before `due_day`.
- `{h}` is `remaining_minutes / 60` rounded to one decimal place.
- At most 5 reminders, ordered by urgency for today.

## Briefing

`GET /v1/briefing` returns, for today: `pending` per [Rollover](#rollover), `today` = today's plan
items after implicit generation, `reminders` per [Reminders](#reminders), and `goals` = active goals
with progress, ordered by `due_day`. The desktop `clock_in` nudge summarizes it; the text is in
[07-integrations.md](07-integrations.md#notifications).
