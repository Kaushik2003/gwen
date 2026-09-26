# Testing

How Gwen is tested, the scenarios that pin down the time engine, and the acceptance criteria that
define done for every work package.

## Strategy

Tooling and style rules are in [CONVENTIONS.md](CONVENTIONS.md#testing). By layer:

| Layer | Approach |
|---|---|
| `timeengine`, `planner`, `config`, rrule | Pure table tests on values. No store, no clock. |
| `store` | Real SQLite in a temp file per test via `testutil.NewDB(t)`. |
| `api` and the loop | Real engine and store; fake activity monitor, fake notifier, fake clock; requests over a real Unix socket in `t.TempDir()`. |
| `activity` | Parsers and signal translation behind small interfaces with fakes; compositor behaviour by a manual test. |
| `notify` | ntfy against `httptest.Server`; desktop against a fake D-Bus connection interface. |
| `gcal`, `llm`, `sync` | Against `httptest.Server` fakes of the remote APIs. |
| CLI | Golden tests: command line in, captured request and printed output out, against `clienttest.Fake`. |
| Tray | The pure `menuFor` function; systray wiring by a manual test. |
| GUI | `tsc --noEmit`, `wails build`, and the manual checklist in its acceptance criteria. |

Tests never touch the network, never sleep, and pass under `-race`. Manual tests live behind the
`manual` build tag and print instructions; `make check` does not run them.

## Golden scenarios

Engine tests for W1. Unless a scenario says otherwise: zone `UTC`, date 2026-09-15, config
`soft_idle 3m`, `hard_idle 10m`, `break_reminder 15m`, `repeat 10m`, `snooze 10m`,
`day_rollover 04:00`, `daily_target 8h`. `P`, `Q`, `R` are project ids. Times are `HH:MM[:SS]` on
that date. **Idle(T) @ X** is an `Idle` event with threshold `T` and `At = X`.

Each scenario asserts the ordered effects, the final state, and — after folding the effects into a
list of segments — the segments shown.

| # | Inputs | Expected |
|---|---|---|
| G1 grace return | ClockIn(P) @09:00; Idle(3m) @10:03; Active @10:07; ClockOut @12:00 | `idle_pending` at 10:03 with `Notify idle`; `Withdraw idle` at 10:07. Segments: work P 09:00–12:00. |
| G2 hard reclaim | ClockIn(P) @09:00; Idle(3m) @10:03; Idle(10m) @10:10; Active @10:40; ClockOut @12:00 | work P 09:00–10:00; break_auto/idle 10:00–10:40; work/activity P 10:40–12:00. |
| G3 reclaim by deadline | As G2 but Tick @10:10 instead of Idle(10m) | Identical to G2. `NextDeadline` at 10:03 returns 10:10. |
| G4 missed soft | ClockIn(P) @09:00; Idle(10m) @10:10 | No `Notify idle`. work P 09:00–10:00; break_auto/idle from 10:00. |
| G5 lock during idle | ClockIn(P) @09:00; Idle(3m) @10:03; Locked @10:05; Active @10:20; Unlocked @10:30 | work 09:00–10:00; break_auto/lock 10:00–10:30; `Active` at 10:20 ignored; work/activity from 10:30. |
| G6 lock while working | ClockIn(P) @09:00; Locked @11:00 | work 09:00–11:00; break_auto/lock from 11:00. |
| G7 manual break | ClockIn(P) @09:00; BreakStart @12:00; Active @12:00:30; Active @12:02; Active @12:03; Tick @12:15; Tick @12:25; BreakEnd @12:30 | No nudge at 12:00:30; `Notify break_active` at 12:02 only; `Notify break_long` at 12:15 and 12:25; at 12:30 `Withdraw break_long` and work/user P from 12:30. |
| G8 snooze | G2 inputs through 10:10; Snooze @10:12; Tick @10:15; Tick @10:22; Tick @10:32 | Nothing at 10:15; `break_long` at 10:22 and 10:32. `NextDeadline` after the snooze is 10:22. |
| G9 break from idle | ClockIn(P) @09:00; Idle(3m) @10:03; BreakStart @10:05 | work 09:00–10:00; break_manual/user from 10:00. |
| G10 clock out from idle | ClockIn(P) @09:00; Idle(3m) @10:03; ClockOut @10:05 | work 09:00–10:00; `CloseDay` at 10:00; `off`. |
| G11 suspend and resume | ClockIn(P) @09:00; Suspend @13:00; Resume @14:00; Active @14:00:05 | work 09:00–13:00; break_auto/suspend 13:00–14:00:05; no transition at `Resume`; work/activity from 14:00:05. |
| G12 suspend over rollover | ClockIn(P) @09:00; Suspend @23:30; Resume @2026-09-16 08:00; Active @2026-09-16 08:00:03 | break_auto/suspend 23:30 → 2026-09-16 04:00; `CloseDay` at 04:00; `off`; no `break_long` nudge at any point; `Notify clock_in` at 08:00:03. |
| G13 rollover while working | ClockIn(P) @20:00; Tick @2026-09-16 04:00 | work 20:00 → 04:00 on day 2026-09-15; `StartDay 2026-09-16`; work/activity P from 04:00; `working`. |
| G14 switching | ClockIn(P) @09:00; Switch(Q) @10:00; Switch(Q) @10:30; BreakStart @11:00; Switch(R) @11:10; BreakEnd @11:20 | work P 09:00–10:00; work Q 10:00–11:00; second switch has no effects; break_manual 11:00–11:20 unchanged by `Switch(R)`; work/user R from 11:20. |
| G15 invalid commands | `BreakStart`, `BreakEnd`, `ClockOut`, `Snooze`, `Switch` in `off`; `ClockIn` and `BreakEnd` in `working`; `BreakStart` in `break_manual` | Each returns `ErrInvalidState`; state unchanged. |
| G16 sub-second reclaim | ClockIn(P) @09:00:00; Idle(3m) @09:03:00.500; Tick @09:10:00.500 | `CloseSegment` at 09:00:00.500 and `OpenSegment break_auto` at 09:00:00.500. (The store test for invariant 7 covers the tombstone.) |
| G17 DST | Zone `America/New_York`. ClockIn(P) @2026-10-31 22:00 EDT; Tick at 2026-11-01 01:30 EST | Still `working` on day 2026-10-31; `NextDeadline` is 2026-11-01 04:00 EST = 09:00 UTC. |
| G18 decide is pure | Any scenario: call `Decide` twice on the same input before `Accept` | Equal decisions; `Status()` unchanged until `Accept`. |

### Recovery scenarios

`now` is 2026-09-15 11:30 unless stated.

| # | Stored state | Expected from `New` |
|---|---|---|
| R1 crash mid-work | Open day; open work P from 09:00; `heartbeat_at` 10:59:45; no `shutdown_at` | Close at 10:59:45, truncated; Open break_auto/recovery at 10:59:45; `break_auto`. |
| R2 quick restart | As R1 but `heartbeat_at` 11:29:50 | No effects; `working`. |
| R3 clean shutdown, next day | Open day 2026-09-14; open work from 18:00; `shutdown_at` 2026-09-14 22:00 | Close at 22:00, not truncated; `CloseDay` at 22:00; `off`. |
| R4 open day, no segment | Open day; last segment ended 10:00; `heartbeat_at` 10:00:10 | Open break_auto/recovery at 10:00:10; `break_auto`. |
| R5 nothing open | No open day | No effects; `off`. |
| R6 stale heartbeat | Open work from 11:00; `heartbeat_at` from the day before | Close at 11:00 (clamped to `started_at`), truncated; Open break_auto/recovery at 11:00. |

## Acceptance criteria

### W0

- `make check` passes on the skeleton.
- `store.Open` on an empty directory yields `journal_mode = wal`, `foreign_keys = 1`, and the
  migrated v1 schema; `device_id` is created once and unchanged after reopening.
- With a test migration filesystem containing a second migration, a backup file is written before
  it applies.
- Config: missing file gives defaults and writes the file with mode `0600`; a partial file
  overlays; an unknown key, `soft_idle >= hard_idle`, and a bad `HH:MM` each fail naming the key;
  save then load round-trips; `LogValue` output never contains the topic.
- Client: an `httptest` server on a Unix socket returning the error envelope yields an `APIError`
  with the code; a missing socket yields `ErrDaemonNotRunning`; the SSE reader handles multi-line
  data, `id:`, and `: ping` comments.
- `var _ client.API = (*clienttest.Fake)(nil)` compiles. The fake clock fires timers only on
  `Advance`.

### W1

- Every row of the [transitions table](05-time-engine.md#transitions) is exercised by at least one
  test case, and G1–G18 and R1–R6 pass.
- `NextDeadline` is correct in each state: hard deadline, `break_long`, snooze-shifted
  `break_long`, rollover.
- Statement coverage of `internal/timeengine` is at least 95 %.

### W2

- With fake probers, `activity.New` selects the first that initialises and falls through on errors
  in the documented order; with none, returns the `none` monitor.
- `swayidle` output parsing and `xprintidle` parsing are table-tested; polling backends compute
  `At` from the observed idle time with a fake clock.
- login1 signals translate to `Locked`, `Unlocked`, `Suspend`, `Resume` through a fake bus; the
  sleep inhibitor is released after `Suspend` is acknowledged.
- Manual, on KDE Plasma Wayland: `go test -tags manual -run TestManualMonitor ./internal/activity`
  prints `Idle` at 5 s and at a 20 s test threshold, `Active` on input, lock and unlock events, and
  suspend and resume events; with a video playing and no input, `Idle` still fires.

### W3

- Each [invariant](03-data-model.md#invariants) has a test that attempts a violation and gets the
  documented error or behaviour.
- Fixture — work P 09:00–10:00, break 10:00–10:30, open work P from 10:30, `now` 11:00 — gives
  `worked_ms` 5,400,000 and `break_ms` 1,800,000.
- Streaks: target met today; met yesterday but not yet today; a day without a work day breaks the
  run; longest streak across history.
- Every write increments `rev` and sets `updated_at` and `device_id`; deleting a project
  soft-deletes its tasks; task list ordering matches the contract.
- Statement coverage of `internal/store` is at least 80 %.

### W4

- ntfy: method, path, headers, and body match the spec; fallback on a 5xx and on a refused
  connection; no fallback on a 4xx; the bearer header is sent only when the token file exists; a
  captured log contains no topic.
- Desktop: `Notify` arguments including `replaces_id` reuse, `expire_timeout`, and actions only with
  the capability; `ActionInvoked` becomes an `ActionEvent`; `Withdraw` calls `CloseNotification`.
- A backend that blocks does not delay the other; it is abandoned at the timeout.
- Manual: an `idle` nudge on KDE shows three buttons and each delivers its action.

### W5

- Every v1 endpoint has a happy-path test and a test per error code it documents.
- The first SSE frame is `state_changed`; editing a segment emits `day_changed`; creating a project
  emits `projects_changed`.
- With the store failing inside a decision's transaction, the engine state is unchanged and the
  caller gets `internal`.
- Recovery effects commit before the socket accepts connections. The socket has mode `0600`. A
  stale socket file is replaced; a live daemon on the socket makes a second start exit non-zero.
- `SIGTERM` writes `shutdown_at`. The heartbeat is written every 15 s of fake time.
- `engine_events` rows older than 90 days are removed at startup.

### W6

- Every command in the v1 table of [08-clients.md](08-clients.md#v1-commands) has a golden test
  asserting the request sent and the output printed.
- Exit codes 0, 1, 2, and 3 are each produced by a test. An ambiguous short id exits 2 with
  `ambiguous id`.
- `--json` prints the wire JSON unchanged.

### W7

- `menuFor` is table-tested for all five states and for daemon-down: visible items, labels,
  checked project, icon, and tooltip.
- Manual: on KDE, the icon appears, changes colour with state within 1 s, and each menu action
  works.

### W8

- `npx tsc --noEmit` is clean and `make build` produces `bin/gwen-ui`.
- Manual checklist against a running daemon: first-run screen appears when the daemon is stopped
  and `Start and run at login` works; each v1 screen loads its data; each action on it succeeds and
  the screen updates from the event stream without a manual refresh; closing the window leaves
  tracking unaffected; a second launch focuses the first window.

### W9

- `make package` produces an rpm and a deb, and `rpm -qlp` lists exactly the installed files in
  [09-packaging.md](09-packaging.md#packages).
- In a `fedora:44` container, `dnf install` of the rpm resolves its dependencies and succeeds.
- `systemd-analyze --user verify packaging/gwend.service` reports nothing.
- Every setup subcommand is tested against `clienttest.Fake` with a fake command runner in place of
  `systemctl`; `--yes` completes without reading stdin.

### W10

- rrule: daily with interval, weekly with `BYDAY` and interval, monthly, `UNTIL`, and each rejected
  form.
- The worked examples in [06-planner.md](06-planner.md) produce exactly the numbers stated: session
  quantity 4 and 120 minutes; capacity 150 minutes.
- Urgency weights and tie-breaks; generation block rules (minimum, maximum, two blocks, rounding,
  slotting, gap, pinned items kept); each rollover rule; each reminder rule.
- A goal becomes `done` when its last unit completes and returns to `active` on reopen.
- `00002_planner.sql` applies to a v1 database containing data, and v1 endpoints behave unchanged.
- Every v2 endpoint has a happy-path and error-path test.

### W11

- Every v2 CLI command has a golden test. `tsc` is clean. The manual checklist covers Plan, Goals,
  and Briefing as in W8.

### W12

- Against a fake Calendar API: first sync creates the calendar; push inserts, patches, and deletes;
  a moved event pins the item and updates `start_at`; a cancelled event skips the item; `410`
  triggers a full resync; busy rows are replaced for the range; failures back off.
- The OAuth callback validates `state` and exchanges the code with the PKCE verifier against a fake
  token endpoint.
- Manual: connecting a real Google account shows the Gwen calendar with today's plan.

### W13

- Request shapes for both providers, including no `Authorization` header when no key file exists.
- Each breakdown validation rule rejects; failures store `failed` runs; a failed retro stores a
  `rules` summary with `status = "ok"`; `accept` creates the selected tasks in one transaction;
  provider `none` returns `unavailable`.

### W14

- Push: newer row wins, older row ignored, equal `updated_at` decided by `device_id`; a
  constraint-violating row is skipped and counted.
- Pull: pages by cursor and excludes the requester's own writes. Client watermark overlap re-pushes
  without duplicates.
- Wrong token returns 401; `/login` sets the cookie; the arm64 build succeeds.
- Manual: deployed on a Pi, the phone opens the dashboard and sees today's totals.

## v1 release gate

Run on the development machine after W5–W9 are merged. Every step must pass.

1. `make install-dev`, then `gwen setup --yes`, then `gwen setup phone` with a real topic.
2. `gwen config set tracking.soft_idle 10s` and `gwen config set tracking.hard_idle 30s`.
3. `gwen project add Internship`, then `gwen in --project Internship`.
4. Take hands off the keyboard and mouse for 40 s. At about 10 s a desktop notification with three
   buttons appears and the phone receives a push. At 30 s the tray turns blue.
5. `gwen day show` lists a work segment ending within 1 s of when input stopped and a
   `break_auto`/`idle` segment starting at that same instant.
6. Touch the mouse: the tray turns green within 1 s and a `work`/`activity` segment opens.
7. The `segments` table queried with `sqlite3` matches `gwen day show --json`.
8. `systemctl --user kill --signal=SIGKILL gwend && systemctl --user stop gwend`, wait 90 s,
   `systemctl --user start gwend`. `gwen status` shows `break_auto`, and the last work segment is
   `truncated` with an end within 15 s of the kill.
9. The GUI Today screen agrees with `gwen status` and ticks every second.
10. After an hour of normal use, `ps -o rss= -C gwend` reports under 40 MB.
