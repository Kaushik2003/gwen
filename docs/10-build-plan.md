# Build plan

The work packages that build Gwen, what each owns, and the order they land in. Each package is
sized for one agent working alone in its own branch.

## Rules

- **Ownership is exclusive.** Every path in the repository is owned by exactly one package at a
  time. A package changes only paths it owns. Paths not yet created belong to the package that
  lists them.
- **Read list.** Each package reads the docs in its row of [README.md](README.md#read-only-what-your-work-package-needs),
  plus its own section here and its acceptance criteria in [11-testing.md](11-testing.md).
- **Definition of done**, for every package:
  1. Everything under *Delivers* is implemented, with no placeholders
     ([CONVENTIONS.md](CONVENTIONS.md#rules-for-agents)).
  2. `make check` passes.
  3. Its acceptance criteria in [11-testing.md](11-testing.md#acceptance-criteria) pass; each
     automated criterion has a test.
  4. `git diff --stat main...HEAD` shows only owned paths.
  5. The final summary lists any spec gap found, per
     [README.md](README.md#change-protocol).
- **`go.mod` and `go.sum`.** W0 adds every v1 module and tool from
  [02-architecture.md](02-architecture.md#dependencies) with `go get`. No other v1 package edits
  them. W5 runs `go mod tidy` once, as its last commit. In v2 and v3 the phase's first package adds
  that phase's modules.
- **Branches** are named `wN-short-name`. Merge order is the dependency order below.

## Dependency graph

```text
v1   W0 ──┬── W1 ──┐
          ├── W2 ──┤
          ├── W3 ──┼── W5 ── v1 release gate
          ├── W4 ──┘          ▲
          ├── W6 ─────────────┤
          ├── W7 ─────────────┤
          ├── W8 ─────────────┤
          └── W9 ─────────────┘
v2   v1 ── W10 ── W11
v3   v2 ── W12 (migration commit) ──┬── W12 rest
                                    ├── W13
                                    └── W14
```

W1–W4 and W6–W9 run fully in parallel after W0. W6–W8 build against the frozen
[04-api-contract.md](04-api-contract.md) using `clienttest.Fake`, and are verified against the real
daemon at the [v1 release gate](11-testing.md#v1-release-gate).

## v1

### W0 — Skeleton

Lands first; everything depends on it.

**Owns:** `go.mod`, `go.sum`, `Makefile`, `.gitignore`, `.github/workflows/check.yml`, root
`README.md`, `scripts/`, `internal/clock/`, `internal/config/`, `internal/model/`, `internal/wire/`,
`internal/client/`, `internal/testutil/`, and in `internal/store/` only `db.go`, `migrate.go`,
`errors.go`, `localstate.go`, `migrations/00001_init.sql`.

**Delivers:**

- Module `github.com/kzark/gwen`; all v1 modules and tools in `go.mod`.
- `Makefile` with every target in [09-packaging.md](09-packaging.md#build). `package` fails until W9
  lands; that is expected.
- CI workflow running `make check` on push and pull request.
- `clock`: `Clock`, `Timer`, the real clock, and `NewFake(t0)` with `Advance(d)`.
- `config`: load, defaults, overlay, validation, atomic save, `NewTopic()`, and `LogValue()`
  redaction, per [03-data-model.md](03-data-model.md#configuration).
- `model`: structs for every v1 table and string constants for segment `kind` and `source`.
- `wire`: every v1 object and request body in [04-api-contract.md](04-api-contract.md), plus
  `Millis(time.Time) int64` and `Time(int64) time.Time`.
- `client`: the `API` interface with one method per v1 endpoint; the Unix-socket HTTP
  implementation; `APIError`; `ErrDaemonNotRunning`; an SSE reader; `format.go` with the duration,
  time, and short-id helpers described in [08-clients.md](08-clients.md#shared-behaviour).
- `client/clienttest`: `Fake`, an in-memory `client.API` whose responses tests script and whose
  received requests tests inspect.
- `store`: `Open` with the DSN and pool settings, goose migration with the pre-migration backup,
  the transaction helper, `ErrNotFound`, `ErrConflict`, `ErrInvalid`, `local_state` get and set, and
  `device_id` bootstrap, per [03-data-model.md](03-data-model.md#migrations).
- `testutil`: `NewDB(t)` returning a migrated temp-file store, and fixed reference instants.
- `scripts/docs-check.sh` as already present in the repository.

### W1 — Time engine

**Owns:** `internal/timeengine/`. **Depends on:** W0.

**Delivers:** the engine exactly as specified in [05-time-engine.md](05-time-engine.md): inputs,
effects, `Decide`, `Accept`, `NextDeadline`, `Status`, recovery in `New` with
`RecoveryEffects()`, the nudge rules, and the day boundary rules. `ErrInvalidState`.

### W2 — Activity monitor

**Owns:** `internal/activity/`, `protocol/`. **Depends on:** W0.

**Delivers:**

- The `ActivityMonitor` port and `Event` type from
  [02-architecture.md](02-architecture.md#activityactivitymonitor).
- Backends `waylandIdle`, `swayidleProc`, `dbusScreensaver`, `xprintidle`, probed in that order by
  `activity.New(ctx)`, and a `none` monitor that emits only login1 events. `Name()` values are the
  `activity_backend` strings in [04-api-contract.md](04-api-contract.md#status).
- The vendored `protocol/ext-idle-notify-v1.xml` and Go bindings generated from it with
  `go tool go-wayland-scanner`, committed under `internal/activity/extidle/`.
- The login1 source: lock, unlock, suspend, resume, and the sleep delay inhibitor.
- Every guarantee in [05-time-engine.md](05-time-engine.md#what-the-engine-requires-of-the-activity-monitor).

### W3 — Store repositories

**Owns:** `internal/store/` except W0's files. **Depends on:** W0.

**Delivers:** `ProjectRepo`, `TaskRepo`, `WorkDayRepo`, `SegmentRepo`, `StatsRepo` interfaces and
SQLite implementations covering every read and write the v1 endpoints need; the engine effect
operations `StartDay`, `CloseDay`, `OpenSegment`, `CloseSegment` taking a transaction; all
[invariants](03-data-model.md#invariants) with their errors; envelope maintenance (`updated_at`,
`rev`, `device_id`); the [derived totals](03-data-model.md#derived-totals) and streaks;
`engine_events` insert and prune.

### W4 — Notifications

**Owns:** `internal/notify/`. **Depends on:** W0.

**Delivers:** `Notifier` port, `Dispatcher`, the desktop and ntfy backends, `Actions()` channel,
`Withdraw`, and the test-notification method used by `POST /v1/notify/test`, all per
[07-integrations.md](07-integrations.md#notifications).

### W5 — Daemon and API

**Owns:** `cmd/gwend/`, `internal/api/`. **Depends on:** W1, W2, W3, W4.

**Delivers:**

- `gwend` main: flags from [09-packaging.md](09-packaging.md#daemon-flags) (except `--hub` and
  `--listen`, which W14 adds), logging setup, config load, store open and migrate, recovery commit, socket bind with
  the stale-socket rule from [02-architecture.md](02-architecture.md#failure-and-degradation),
  graceful shutdown with `shutdown_at`.
- The engine loop from [05-time-engine.md](05-time-engine.md#shape): input channel, deadline timer,
  15 s heartbeat, monitor restart on config change, notification action mapping, SSE emission.
- `internal/api`: every v1 route, the error mapping, and the SSE hub, per
  [04-api-contract.md](04-api-contract.md).
- `go mod tidy` as the final commit.

### W6 — CLI

**Owns:** `cmd/gwen/` except `cmd/gwen/setup*.go`. **Depends on:** W0.

**Delivers:** every v1 command in [08-clients.md](08-clients.md#v1-commands) with its flags, human
output, `--json`, and exit codes. Root command wiring that registers `newSetupCmd()`, which W9
defines.

### W7 — Tray

**Owns:** `cmd/gwen-tray/`, `packaging/icons/tray/`. **Depends on:** W0.

**Delivers:** the tray in [08-clients.md](08-clients.md#tray), with a pure
`menuFor(status *wire.Status, daemonUp bool, projects []wire.Project, now time.Time) Menu` function
that decides visibility, labels, checked project, icon, and tooltip, where `projects` is the live
list and `now` the skew-corrected current instant; and the four tray icons.

### W8 — GUI

**Owns:** `cmd/gwen-ui/`, `ui/`. **Depends on:** W0.

**Delivers:** the Wails host with `App` bindings, event forwarding, single-instance lock, and host
methods; the v1 screens in [08-clients.md](08-clients.md#v1-screens); `ui/package.json` with the
frontend dependencies from [02-architecture.md](02-architecture.md#dependencies) pinned to exact
versions.

### W9 — Packaging and setup

**Owns:** `packaging/` except `packaging/icons/tray/`, `cmd/gwen/setup*.go`. **Depends on:** W0.

**Delivers:** everything in [09-packaging.md](09-packaging.md) from Packages through Setup, the Pi
push-server files, and the app icon (`gwen.svg` plus 48 and 128 px PNGs: a rounded square in
`#10b981` with a white stopwatch glyph). `newSetupCmd()` with every v1 subcommand; the three v3
subcommands are added by W12–W14.

## v2

Starts after the v1 release gate passes.

### W10 — Planner backend

**Owns:** `internal/planner/`, and within v2 may edit `internal/model/`, `internal/store/`,
`internal/api/`, `internal/wire/`, `internal/client/`, `cmd/gwend/`. **Depends on:** v1.

**Delivers:** everything in [06-planner.md](06-planner.md); migration `00002_planner.sql`; store
repositories for goals, commitments, and plan items, plus the v2 task columns; every v2 endpoint in
[04-api-contract.md](04-api-contract.md#endpoints--v2) with `plan_changed` and `goals_changed`
events; v2 wire types and client methods; the briefing body for the `clock_in` nudge.

### W11 — Planner clients

**Owns:** within v2, `cmd/gwen/` and `ui/`, `cmd/gwen-ui/`. **Depends on:** W10.

**Delivers:** the v2 CLI commands and v2 GUI screens in [08-clients.md](08-clients.md).

## v3

Starts after v2. W12's first merge is migration `00003_integrations.sql` alone; W13 and W14 start
from that merge. The three then run in parallel. Each owns new files named for its feature — for
example `internal/api/calendar.go`, `internal/client/calendar.go`, `cmd/gwen/cal.go`,
`ui/src/settings/Calendar.tsx`. The only shared files are route registration in
`internal/api/routes.go`, the `client.API` interface, the GUI navigation, and `cmd/gwen/setup.go`;
packages merge in the order W12, W13, W14, and the later package resolves conflicts in those files
only.

### W12 — Google Calendar

**Owns:** `internal/gcal/`, migration `00003_integrations.sql`, and calendar-named files as above.

**Delivers:** [07-integrations.md](07-integrations.md#google-calendar) in full; the calendar
endpoints; `gwen cal` commands; `gwen setup calendar`; the Settings → Calendar panel; `busy` input
wired into capacity.

### W13 — LLM adapter

**Owns:** `internal/llm/`, and LLM-named files as above.

**Delivers:** [07-integrations.md](07-integrations.md#llm-adapter) in full; breakdown, retro, and
run endpoints; `gwen goal breakdown`, `gwen llm`, `gwen retro`; `gwen setup llm`; the AI settings
panel, Break down dialog, and Retro view.

### W14 — Sync hub

**Owns:** `internal/sync/`, `cmd/gwend/hub*.go`, `packaging/pi/gwen-hub.service`, and sync-named
files as above.

**Delivers:** [07-integrations.md](07-integrations.md#sync-hub) in full, including hub mode, the hub
API, login, read-only API subset and dashboard serving; the `VITE_GWEN_TARGET=hub` UI build; sync
endpoints; `gwen sync`; `gwen setup sync`; the Sync settings panel; the hub deployment files in
[09-packaging.md](09-packaging.md#sync-hub-v3).
