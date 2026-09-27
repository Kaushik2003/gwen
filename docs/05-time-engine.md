# Time engine

The rules that turn input activity and user commands into segments. This is the heart of the app;
every number the dashboard shows is a consequence of this file.

## Shape

`internal/timeengine` is a **pure** package: no I/O, no goroutines, no clock reads, no logging. It
receives inputs carrying their own timestamps and returns decisions. The daemon loop (W5) owns
everything with side effects. This is what lets the golden scenarios in
[11-testing.md](11-testing.md#golden-scenarios) run as table tests in microseconds.

```go
type Engine struct{ /* unexported */ }

func New(cfg Config, rec Recovered) *Engine

// Decide computes the result of an input without changing the engine.
// It returns ErrInvalidState for a command the current state does not allow.
func (e *Engine) Decide(in Input) (Decision, error)

// Accept makes a decision current. The loop calls it only after the
// decision's effects have been committed to the store.
func (e *Engine) Accept(d Decision)

// NextDeadline is the earliest instant at which the engine needs a Tick.
func (e *Engine) NextDeadline() (time.Time, bool)

func (e *Engine) Status() Snapshot

type Decision struct {
    Next    Snapshot // state, attribution, idle_since, snooze, nudge bookkeeping
    Effects []Effect // applied in order, in one store transaction
}
```

**Loop contract (W5).** For every input: `Decide` → run `Effects` in one transaction → on commit,
`Accept`, send notifications, emit SSE events. On a failed transaction, log at `Error`, do not call
`Accept`, and answer the API caller with `internal`. The engine's in-memory state therefore never
runs ahead of the database. After every input the loop re-arms one `Clock` timer at
`NextDeadline()` which delivers `Tick`.

### Inputs

| Input | Fields | Origin |
|---|---|---|
| `ClockIn` | `ProjectID, TaskID *string; At` | API, tray, notification action |
| `ClockOut` | `At` | API |
| `BreakStart` | `At` | API, notification action |
| `BreakEnd` | `At` | API, notification action |
| `Switch` | `ProjectID, TaskID *string; At` | API |
| `Snooze` | `At` | API, notification action |
| `Idle` | `Threshold time.Duration; At` | activity monitor |
| `Active` | `At` | activity monitor, the `back` notification action |
| `Locked`, `Unlocked`, `Suspend`, `Resume` | `At` | activity monitor (login1) |
| `Tick` | `At` | the loop's deadline timer |
| `ConfigChanged` | `Config` | `PATCH /v1/config` |

Every input first processes any deadline that has passed at or before its `At`, in time order,
exactly as if a `Tick` had arrived at that deadline. This matters after suspend, when timers fire
late. State-changing deadlines (hard idle, day rollover) always apply at their own instant. A
deadline that only sends a nudge and is more than 60 s older than `At` is skipped rather than fired,
and the next one is scheduled from `At` — so resuming after hours asleep produces no burst of stale
nudges.

### Effects

| Effect | Fields | Store action |
|---|---|---|
| `StartDay` | `Day, TZ string; TargetSeconds int; At` | Reopen the live work day for `Day` if one exists (clear `clocked_out_at`), else insert one. |
| `CloseDay` | `At` | Set `clocked_out_at` on the open work day. |
| `OpenSegment` | `Kind, Source string; ProjectID, TaskID *string; At` | Insert an open segment on the open work day. |
| `CloseSegment` | `At; Truncated bool` | Set `ended_at` on the open segment, honouring [invariant 7](03-data-model.md#invariants). |
| `Notify` | `Kind, Title, Body string; Actions []Action; At` | None; sent after commit per [07](07-integrations.md#notifications). |
| `Withdraw` | `Kind string` | None; closes that desktop notification if still shown. |
| `RecordTransition` | `Trigger, From, To string; At; Data map[string]any` | Insert into `engine_events`. |

Every change of `state`, and every day rollover, is one `RecordTransition` whose `Trigger` is the
input name in snake case (`clock_in`, `idle`, `tick`); a deadline caught up by another input records
`tick`. A decision that catches up deadlines can therefore hold several. Recovery records its result
with `Trigger` `recovery`.

## States

| State | Open segment | Meaning |
|---|---|---|
| `off` | none | Not clocked in. |
| `working` | `work` | Clocked in and present. |
| `idle_pending` | `work` | No input for at least the soft threshold. Time still counts. |
| `break_auto` | `break_auto` | Break the app decided on: long idle, lock, suspend, or recovery. |
| `break_manual` | `break_manual` | Break the user asked for. |

Alongside the state the engine holds: the **attribution** (`project_id`, `task_id`) that work
resumes on, `idle_since`, `snoozed_until`, whether the session is **locked**, and the nudge
bookkeeping described under [Nudges](#nudges).

## Thresholds

- **Soft** (`tracking.soft_idle`) — idle this long enters `idle_pending` and nudges.
- **Hard** (`tracking.hard_idle`) — idle this long converts to `break_auto` with retroactive reclaim.
- **Presence** — a fixed 5 s threshold, not configurable. The engine ignores `Idle` events for it.
  It exists so the monitor always reports `Active` within moments of the user returning from any
  break.

Defaults and validation are in [03-data-model.md](03-data-model.md#configuration).

The loop starts the monitor with thresholds `[5s, soft, hard]`. An `Idle` event means input stopped
at `idle_since = At − Threshold`. The engine clamps `idle_since` to no earlier than the open
segment's `started_at`.

The hard transition happens on whichever arrives first: an `Idle` event at the hard threshold, or
the engine's own deadline at `idle_since + hard`. The effects are identical, so a monitor that
misses the hard event cannot leave the engine stuck in `idle_pending`.

On `ConfigChanged` the engine adopts the new values immediately; `idle_since` is kept. The loop
restarts the monitor with the new thresholds.

### What the engine requires of the activity monitor

W2 must deliver these, whatever the backend:

1. `Idle{Threshold, At}` once per idle period per threshold, where `At − Threshold` is the last input
   instant. Polling backends compute `At` from the observed idle time, not the poll time.
2. `Active{At}` on the first input after any `Idle` event.
3. Idle measurement ignores idle inhibitors (video players must not count as presence). On Wayland
   this means `get_input_idle_notification` when the notifier's version is at least 2, and
   `get_idle_notification` otherwise.
4. `Locked`/`Unlocked` from the login1 session's `Lock`/`Unlock` signals and its `LockedHint`
   property; `Suspend`/`Resume` from `PrepareForSleep(true)`/`PrepareForSleep(false)`.
5. A login1 `delay` inhibitor for `sleep` is held while running and released when the loop calls the
   `Suspend` event's `Ack`, after committing the transition, so the suspend instant is written
   before the machine sleeps. The inhibitor is re-taken on `Resume`.

## Transitions

`at` is the input's `At`. **Close** and **Open** are `CloseSegment` and `OpenSegment`. Any command
not listed for a state returns `ErrInvalidState`. Any event not listed for a state is a no-op.

| From | Input | To | Effects |
|---|---|---|---|
| `off` | `ClockIn` | `working` | `StartDay` for [the day of](#day-boundaries) `at`; Open `work`/`user` with the given attribution at `at` |
| `off` | `Active`, `Unlocked`, `Resume` | `off` | `Notify clock_in`, subject to [Nudges](#nudges) |
| `working` | `Idle` soft | `idle_pending` | set `idle_since`; `Notify idle` |
| `working` | `Idle` hard | `break_auto` | [reclaim](#retroactive-reclaim) with source `idle`, no idle nudge |
| `working` | `Locked` | `break_auto` | Close at `at`; Open `break_auto`/`lock` at `at` |
| `working` | `Suspend` | `break_auto` | Close at `at`; Open `break_auto`/`suspend` at `at` |
| `working` | `BreakStart` | `break_manual` | Close at `at`; Open `break_manual`/`user` at `at` |
| `working` | `Switch` | `working` | if attribution differs: Close at `at`; Open `work`/`user` with new attribution at `at` |
| `idle_pending` | `Active` | `working` | `Withdraw idle` |
| `idle_pending` | `Idle` hard, or deadline `idle_since + hard` | `break_auto` | [reclaim](#retroactive-reclaim) with source `idle`; `Withdraw idle` |
| `idle_pending` | `Locked` | `break_auto` | reclaim with source `lock`; `Withdraw idle` |
| `idle_pending` | `Suspend` | `break_auto` | reclaim with source `suspend`; `Withdraw idle` |
| `idle_pending` | `BreakStart` | `break_manual` | Close at `idle_since`; Open `break_manual`/`user` at `idle_since`; `Withdraw idle` |
| `idle_pending` | `Switch` | `working` | `Withdraw idle`, then as `working` + `Switch` |
| `break_auto` | `Active` while not locked, or `Unlocked` | `working` | Close at `at`; Open `work`/`activity` with attribution at `at`; `Withdraw break_long` |
| `break_auto` | `BreakStart` | `break_manual` | Close at `at`; Open `break_manual`/`user` at `at`; `Withdraw break_long` |
| `break_auto`, `break_manual` | `BreakEnd` | `working` | Close at `at`; Open `work`/`user` with attribution at `at`; `Withdraw break_long` |
| `break_auto`, `break_manual` | `Switch` | unchanged | set attribution only |
| `break_manual` | `Active` | `break_manual` | `Notify break_active`, subject to [Nudges](#nudges) |
| `working`, `break_auto`, `break_manual` | `ClockOut` | `off` | Close at `at`; `CloseDay` at `at`; `Withdraw` all |
| `idle_pending` | `ClockOut` | `off` | Close at `idle_since`; `CloseDay` at `idle_since`; `Withdraw` all |
| any but `off` | `Snooze` | unchanged | `snoozed_until = at + nudge.snooze` |
| any but `off` | deadline: day rollover | see [Day boundaries](#day-boundaries) | |
| any | `Locked` / `Unlocked` | per rows above | also set / clear the locked flag |

`Active` while locked is ignored in every state: input reaching the lock screen is not presence.
`Unlocked` is the presence signal instead.

Clicking a notification action is presence, except for the two actions that *assert* the idle time
was not work: `BreakStart` and `ClockOut` from `idle_pending` end work at `idle_since`, not at the
click. That is deliberate — the user is confirming they stopped when input stopped.

## Retroactive reclaim

When idle reaches the hard threshold, the engine does not end work "now". It ends it at
`idle_since`, the last instant input was seen:

1. `CloseSegment` at `idle_since` — the open work segment's `ended_at` is `idle_since`, not the
   current time.
2. `OpenSegment` `break_auto` at `idle_since`, with the source given in the transition row.

Example, soft 3 m and hard 10 m: input stops at 10:00. At 10:03 the nudge fires and the timer is
still counting. If input returns at 10:07, nothing changes — the grace period was work. If nothing
returns by 10:10, the work segment ends at **10:00** and a break runs from 10:00. The dashboard's
worked total drops by the 10 minutes at that moment, and clients receive `state_changed` and
`day_changed` so they re-render.

If the reclaimed work segment would be shorter than 1000 ms it is tombstoned, per
[invariant 7](03-data-model.md#invariants).

## Switching

`Switch` changes what time is attributed to without stopping the clock. While working it splits the
open work segment at `at`. During a break it only changes the attribution work resumes on. A
`Switch` to the attribution already in effect is a no-op returning the current snapshot.

## Clock in and out

- `ClockIn` from `off` starts the work day for the day `at` belongs to. If that day already has a
  work day (the user clocked out earlier), it is reopened. The time between the earlier clock-out
  and now is a gap, not a break.
- A new work day gets `target_seconds` from `tracking.daily_target` and `tz` from the engine's
  location. A reopened day keeps its existing target.
- `ClockIn` attribution: both `null` means unassigned.
- `ClockOut` closes the open segment and the work day. Nudge bookkeeping is reset.

## Day boundaries

The **day an instant belongs to** is the local calendar date of `at − tracking.day_rollover`. With
the default `04:00`, 01:30 on the 16th belongs to the 15th, so late-night work stays on the day it
started.

While the state is not `off`, the engine keeps a deadline at the next rollover instant after the
work day's date: `day + 1` at `day_rollover` local time. When that deadline is processed:

1. Close the open segment and `CloseDay`, both **at the rollover instant**, not at the time the
   deadline was processed.
2. If the state was `working` or `idle_pending`, immediately `StartDay` for the new day and Open a
   `work`/`activity` segment at the rollover instant with the same attribution. State `working`.
3. Otherwise (a break) the state becomes `off`.

Rule 1 is why a laptop suspended over midnight produces a clean day boundary instead of a break
that runs into the next day.

## Nudges

| Kind | Fires | Actions | Phone |
|---|---|---|---|
| `idle` | Entering `idle_pending` via the soft threshold. Once per idle period. | `back`, `break`, `snooze` | yes |
| `break_long` | A break has lasted `nudge.break_reminder`, measured from the break segment's `started_at`; then every `nudge.repeat` until the break ends. | `end_break`, `snooze` | yes |
| `break_active` | In `break_manual`, the first `Active` after the break has lasted 60 s. At most once per break. | `end_break` | no |
| `clock_in` | In `off`, the first `Active`, `Unlocked`, or `Resume` on a local day for which the engine has not already prompted and has not clocked out. A `Resume` whose own deadline catch-up ended the day (a suspend across the rollover, G12) does not prompt; the next input does. | `clock_in` | no |

Rules:

- While `at < snoozed_until`, `idle` and `break_long` are dropped, not queued. A `break_long` whose
  time falls inside the snooze fires at `snoozed_until` instead, then resumes its repeat interval.
- `break_long` deadlines are part of `NextDeadline`.
- Leaving the state a nudge was about emits `Withdraw` for its kind, if one is showing. A nudge that
  would be sent and withdrawn within one decision is not sent.
- Action ids map to inputs: `back` → `Active`, `break` → `BreakStart`, `snooze` → `Snooze`,
  `end_break` → `BreakEnd`, `clock_in` → `ClockIn` with the last attribution. An action that yields
  `ErrInvalidState` is logged at `Debug` and dropped.
- Which channels a nudge uses, and its exact text, are owned by
  [07-integrations.md](07-integrations.md#notifications). The engine supplies `Kind`, and the `Title`
  and `Body` from that section's table.

## Crash recovery

The daemon can die at any moment — `kill -9`, OOM, power loss. It must never produce phantom hours
and never lose a closed segment.

**While running.** The loop writes `local_state.heartbeat_at` every 15 s, and also immediately on
`Suspend`. On a clean shutdown (`SIGTERM`, `SIGINT`) it writes `local_state.shutdown_at` and exits
without closing the open segment.

**At startup**, before the socket is bound, W5 loads a `Recovered` value from the store and the
engine applies these rules in `New`. Let `last_seen` be `shutdown_at` if set, else `heartbeat_at`,
else the open segment's `started_at` — and never earlier than the open segment's `started_at` when
there is one. Then `shutdown_at` is cleared.

1. **No open work day** → `off`.
2. **Open segment and `now − last_seen ≤ 60 s`** — a quick restart, e.g. a package upgrade. Keep the
   segment open and resume the state matching its kind: `work` → `working`, `break_auto` →
   `break_auto`, `break_manual` → `break_manual`. `idle_since` is not restored; the monitor re-arms.
3. **Open segment and `now − last_seen > 60 s`** — Close it at `last_seen`, with `truncated = 1`
   only if `shutdown_at` was not set. Continue with rule 4.
4. **Open work day, no open segment.** Let `gap_start` be the latest of `last_seen` and the most
   recent segment's `ended_at`.
   - If the day `now` belongs to differs from the work day's `day`, `CloseDay` at `gap_start` →
     `off`.
   - Otherwise Open `break_auto`/`recovery` at `gap_start` → `break_auto`. The first presence
     returns to `working`.

Rule 3 bounds the damage of any crash to 15 s of over-count. Recovery effects are returned from
`New` via `Engine.RecoveryEffects()` and committed by the loop before anything else; if that commit
fails, startup is fatal.

Recovery never touches closed segments and never runs outside startup.
