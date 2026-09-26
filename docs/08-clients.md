# Clients

The three user-facing programs: CLI (W6), tray (W7), GUI (W8). All three are thin: they call
`client.API` from `internal/client` and render the results. **No client computes a total, decides a
state transition, or talks to anything but the daemon socket** — with two exceptions, both
file-level and both owned by `gwen setup` ([09-packaging.md](09-packaging.md#setup)): writing
credentials files and managing the systemd unit and autostart entry.

## Shared behaviour

- **Daemon not running** (`client.ErrDaemonNotRunning`): every client shows
  `Gwen isn't running. Start it with: systemctl --user start gwend` (CLI, exit code 3), a
  "Start Gwen" action (tray), or the first-run screen (GUI).
- **Live updates**: tray and GUI hold one `GET /v1/events` stream and follow the reconnect and
  refetch rules in [04-api-contract.md](04-api-contract.md#events).
- **Ticking timers** are computed locally as `local_now − skew − open_segment.started_at`, where
  `skew = local_now − server_now_at` sampled at the last Status. Clients re-render once per second
  (GUI) or every 30 s (tray).
- **Formatting**: durations `7h 32m`, `45m`, or `30s` below one minute; times `HH:MM` 24-hour local;
  dates `YYYY-MM-DD`. The Go helpers are in `internal/client/format.go`; the GUI mirrors them in
  TypeScript.
- **Short ids**: lists show the last 8 hex characters of an id. UUIDv7 prefixes are timestamps and
  collide; suffixes do not. Anywhere a client accepts an id it accepts either the full id or a
  unique suffix of at least 8 characters, resolved against the relevant list.
- **Conflict** (`conflict` on a `rev` mismatch): refetch, show `Changed elsewhere — reloaded`, and do
  not retry automatically.

## CLI

`cmd/gwen`, Cobra. Global flags: `--json` prints the raw wire JSON instead of human output;
`--socket PATH` overrides the socket path. Exit codes: `0` success, `1` API error (prints
`message`), `2` usage error, `3` daemon not running.

**Project and task arguments** accept an id, a short id, or — for projects only — a case-insensitive
exact name among live, non-archived projects. `--project none` means unassigned.

**Time arguments**: `HH:MM` is a local time on the day given by `--day` (default: today per the
config's `tracking.day_rollover`); durations use Go syntax (`7h30m`).

### v1 commands

| Command | Endpoint |
|---|---|
| `gwen status` | `GET /v1/status` |
| `gwen health` | `GET /v1/health` |
| `gwen in [--project P] [--task T]` | `POST /v1/day/clock-in` |
| `gwen out` | `POST /v1/day/clock-out` |
| `gwen break` | `POST /v1/break/start` |
| `gwen back` | `POST /v1/break/end` |
| `gwen switch [--project P] [--task T]` | `POST /v1/session/switch` |
| `gwen snooze` | `POST /v1/nudge/snooze` |
| `gwen today` | `GET /v1/days/{today}` |
| `gwen log [--from D] [--to D]` | `GET /v1/days` (default: last 7 days) |
| `gwen day show DAY` | `GET /v1/days/{day}` |
| `gwen day set DAY [--target DUR] [--note TEXT]` | `PATCH /v1/days/{day}` |
| `gwen seg add --day D --kind K --start HH:MM --end HH:MM [--project P] [--task T]` | `POST /v1/segments` |
| `gwen seg edit ID [--kind K] [--start HH:MM] [--end HH:MM] [--project P] [--task T]` | `PATCH /v1/segments/{id}` |
| `gwen seg split ID HH:MM` | `POST /v1/segments/{id}/split` |
| `gwen seg rm ID` | `DELETE /v1/segments/{id}` |
| `gwen project ls [--archived \| --all]` | `GET /v1/projects` |
| `gwen project show P` | `GET /v1/projects/{id}` |
| `gwen project add NAME [--color HEX]` | `POST /v1/projects` |
| `gwen project edit P [--name N] [--color HEX]` | `PATCH /v1/projects/{id}` |
| `gwen project archive P` / `unarchive P` | `PATCH /v1/projects/{id}` with `archived` |
| `gwen project rm P` | `DELETE /v1/projects/{id}` |
| `gwen task ls [--project P] [--status S] [--due-before D]` | `GET /v1/tasks` |
| `gwen task show T` | `GET /v1/tasks/{id}` |
| `gwen task add TITLE [--project P] [--priority N] [--due D] [--estimate DUR] [--notes TEXT]` | `POST /v1/tasks` |
| `gwen task edit T [same flags plus --title]` | `PATCH /v1/tasks/{id}` |
| `gwen task done T` | `POST /v1/tasks/{id}/complete` |
| `gwen task reopen T` | `POST /v1/tasks/{id}/reopen` |
| `gwen task rm T` | `DELETE /v1/tasks/{id}` |
| `gwen stats [--from D] [--to D]` | `GET /v1/stats/summary` (default: last 30 days) |
| `gwen stats heatmap [--year Y]` | `GET /v1/stats/heatmap` |
| `gwen config get [KEY]` | `GET /v1/config` |
| `gwen config set KEY VALUE` | `PATCH /v1/config` |
| `gwen notify test` | `POST /v1/notify/test` |
| `gwen watch` | `GET /v1/events`, one line per event |
| `gwen setup ...` | owned by [09-packaging.md](09-packaging.md#setup) |

`--day` resolution, `gwen today`, and `HH:MM` parsing read `tracking.day_rollover` from
`GET /v1/config`. `gwen seg ...` resolves a short segment id against the segments of `--day`.

**`gwen status` output**, the reference for human formatting:

```text
Working · 3h 12m on Internship (task 9f3a1c2e)
Today   5h 47m worked · 38m break · target 8h (72%)
```

`off` prints `Not clocked in.`; `idle_pending` prints `Idle since HH:MM (still counting)`.
`gwen stats heatmap` prints one row per weekday with one character per week: `·` for zero, then
`░ ▒ ▓ █` for up to 25 %, 50 %, 75 %, and above 75 % of the target.

### v2 commands

| Command | Endpoint |
|---|---|
| `gwen goal ls [--status S]` | `GET /v1/goals` |
| `gwen goal show G` | `GET /v1/goals/{id}` |
| `gwen goal add TITLE --due D [--start D] [--quantity N --unit U --per-unit DUR] [--project P]` | `POST /v1/goals` (`kind` is `quantity` when `--quantity` is given, else `tasks`) |
| `gwen goal edit G [flags] [--status S]` | `PATCH /v1/goals/{id}` |
| `gwen goal rm G` | `DELETE /v1/goals/{id}` |
| `gwen commit ls` | `GET /v1/commitments` |
| `gwen commit add TITLE --rrule R --duration DUR [--at HH:MM] [--project P] [--no-count] [--from D] [--until D]` | `POST /v1/commitments` |
| `gwen commit edit C [flags]` | `PATCH /v1/commitments/{id}` |
| `gwen commit rm C` | `DELETE /v1/commitments/{id}` |
| `gwen plan [DAY]` | `GET /v1/plan` |
| `gwen plan gen [DAY]` | `POST /v1/plan/generate` |
| `gwen plan move ITEM --at HH:MM` / `--position N` / `--minutes N` | `PATCH /v1/plan/items/{id}` |
| `gwen plan skip ITEM` / `unskip ITEM` | `PATCH /v1/plan/items/{id}` with `status` |
| `gwen brief` | `GET /v1/briefing` |

v2 adds to v1 commands: `task add/edit --goal G --rrule R --quantity N`, `task ls --goal G
--templates`, and `task done T --qty N`.

### v3 commands

| Command | Endpoint |
|---|---|
| `gwen cal status` | `GET /v1/calendar/status` |
| `gwen cal connect` | `POST /v1/calendar/auth/start`, then `xdg-open` the URL |
| `gwen cal sync` | `POST /v1/calendar/sync` |
| `gwen goal breakdown G [--instructions TEXT]` | `POST /v1/goals/{id}/breakdown` |
| `gwen llm show RUN` | `GET /v1/llm/runs/{id}` |
| `gwen llm accept RUN --pick 0,2,5` | `POST /v1/llm/runs/{id}/accept` |
| `gwen llm reject RUN` | `POST /v1/llm/runs/{id}/reject` |
| `gwen retro [--week D]` | `POST /v1/retro` (default: the Monday of last week) |
| `gwen sync status` | `GET /v1/sync/status` |
| `gwen sync now` | `POST /v1/sync/now` |

## Tray

`cmd/gwen-tray`, `fyne.io/systray`. Starts from the autostart entry, holds the event stream, and never
blocks on the daemon.

**Icon** — four embedded 22 px and 44 px PNGs from `packaging/icons/tray/`: `off` grey, `working`
green, `idle` amber (for `idle_pending`), `break` blue (both break states). When the daemon is not
running, the `off` icon is shown.

**Tooltip** — `Working · 3h 12m · Internship`, `On break · 12m`, `Not clocked in`, or `Gwen isn't
running`.

**Menu**, top to bottom, with visibility per state:

| Item | Shown when | Action |
|---|---|---|
| Status line (disabled) | always | tooltip text |
| `Clock in` | `off` | `POST /v1/day/clock-in` with the last used attribution |
| `Start break` | `working`, `idle_pending`, `break_auto` | `POST /v1/break/start` |
| `End break` | `break_auto`, `break_manual` | `POST /v1/break/end` |
| `Switch project` ▸ | not `off` | submenu: `Unassigned` plus up to 10 live non-archived projects by name, current one checked; `POST /v1/session/switch` |
| `Snooze nudges` | not `off` | `POST /v1/nudge/snooze` |
| `Clock out` | not `off` | `POST /v1/day/clock-out` |
| separator | | |
| `Open dashboard` | always | exec `gwen-ui` |
| `Start Gwen` | daemon not running | exec `systemctl --user start gwend.service` |
| `Quit tray` | always | exit the tray only |

`systray` cannot remove items, so the project submenu is a fixed pool of 11 items shown and hidden as
`projects_changed` arrives. "Last used attribution" is the Status `project_id`/`task_id` from the
most recent non-`off` Status the tray saw, kept in memory only.

## GUI

`cmd/gwen-ui` (Go host) and `ui/` (React). Wails v2, built with the `webkit2_41` tag.

**Host.** Binds one struct, `App`, whose exported methods mirror `client.API` one to one with the
same names and wire types, except `Events`; Wails generates the TypeScript bindings into
`ui/src/wailsjs/`, which are committed. The Wails project file is `cmd/gwen-ui/wails.json`, with
`frontend:dir` `../../ui`, `frontend:install` `npm ci`, `frontend:build` `npm run build`, and
`outputfilename` `gwen-ui`. Vite builds into `ui/dist/`, which package `ui` embeds from
`ui/embed.go` (`//go:embed all:dist`) for the host to serve. The host also subscribes to
`/v1/events` and re-emits each event with `runtime.EventsEmit(ctx, "gwen:" + name, data)`. Extra
host-only methods: `NewTopic()`, `SetCredential(name, value)`, `OpenURL(url)`,
`EnableService()`, `SetAutostart(bool)`. Single instance via Wails `SingleInstanceLock` with unique
id `dev.gwen.ui`; a second launch focuses the first window.

**Window** 1100×720, minimum 900×600, follows the system light/dark preference. Navigation is a left
sidebar; the selected screen is React state.

### v1 screens

| Screen | Contents | Endpoints |
|---|---|---|
| **First run** | Shown instead of everything else while the daemon is not running: explains Gwen, `Start and run at login` button (`EnableService`), tray autostart toggle, retry. | `GET /v1/health` |
| **Today** | Large ticking timer coloured by state; state label; project and task pickers that switch; Clock in/out, Start/End break, Snooze buttons with the same visibility as the tray; progress ring for worked vs target; a horizontal timeline of today's segments coloured by project (breaks hatched); per-project list; a banner for each `warnings` entry. | `GET /v1/status` via the stream, `GET /v1/days/{day}`, `GET /v1/projects`, `GET /v1/tasks`, tracking commands |
| **History** | Range picker (default last 7 days); stacked bars of worked time per day by project; clicking a day opens **Day detail**: segment table with inline edit of kind, project, task, start and end; split; delete; `Add segment` dialog; target and note fields. | `GET /v1/days`, `GET/PATCH /v1/days/{day}`, segment endpoints |
| **Stats** | Range tabs Week, Month, Year, Custom; cards for worked, average per tracked day, days target met, current and longest streak; project donut; year heatmap with year selector. | `GET /v1/stats/summary`, `GET /v1/stats/heatmap` |
| **Projects & tasks** | Project list with colour swatch, rename, colour, archive, delete; task list filtered by project and status, with add, edit, complete, reopen, delete. | project and task endpoints |
| **Settings** | A form per config section with inline validation errors from `details.key`; **Phone** section showing a QR code of `{ntfy.server}/{ntfy.topic}`, `New topic`, and `Send test`; daemon version. | `GET/PATCH /v1/config`, `POST /v1/notify/test`, `GET /v1/health` |

### v2 screens

| Screen | Contents | Endpoints |
|---|---|---|
| **Plan** | Day picker; capacity bar (planned vs capacity); commitments as fixed blocks; ordered items with start time, task, minutes, pin and skip; drag to reorder (`position`, which pins); `Regenerate`. | plan endpoints, `GET /v1/commitments` |
| **Goals** | Goals with progress bar, pace chip, projected finish; add and edit dialog; session template `rrule` editor for quantity goals; commitments list with add, edit, delete. | goal and commitment endpoints, `PATCH /v1/tasks/{id}` |
| **Briefing** | Modal on the first GUI open of each day (dismissal day kept in `localStorage`) and from Today: pending, today's plan, reminders, goals behind pace. | `GET /v1/briefing` |

### v3 additions

| Where | Contents | Endpoints |
|---|---|---|
| Settings → **Calendar** | Status, `Connect` (auth start, then `OpenURL`), `Sync now`, busy-calendar list. | calendar endpoints, `PATCH /v1/config` |
| Settings → **AI** | Provider, model, endpoint, API key field (`SetCredential("llm_api_key", …)`). | `PATCH /v1/config` |
| Settings → **Sync** | Hub URL, token field (`SetCredential("sync_token", …)`), status, `Sync now`. | sync endpoints, `PATCH /v1/config` |
| Goal detail → **Break down** | Instructions box; result as a checklist; `Add selected` and `Discard`. | breakdown and run endpoints |
| Stats → Week → **Retro** | Generate and render the markdown as plain paragraphs and lists. | `POST /v1/retro` |

**Hub build** (`VITE_GWEN_TARGET=hub`): only History, Day detail (read-only), Stats, Plan
(read-only), and Goals (read-only) are included, calling the hub over `fetch` instead of Wails
bindings; see [07-integrations.md](07-integrations.md#read-only-dashboard).
