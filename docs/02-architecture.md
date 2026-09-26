# Architecture

Everything in this file is decided. Rationale is included so you can trust the decision, not so you
can revisit it.

## Processes

Four binaries. The split exists because the always-on process must be small and dependency-free,
while the dashboard wants a browser engine — putting both in one binary would mean paying for
WebKit 24 hours a day.

| Binary | Role | CGo | Runs |
|---|---|---|---|
| `gwend` | Daemon. Owns the database, activity monitor, time engine, scheduler, notifier, and all integrations. The only writer. | **no** | always, systemd user unit |
| `gwen` | CLI. Thin client. | no | on invocation |
| `gwen-tray` | Tray icon and menu: current state, clock in/out, break. | no | session autostart |
| `gwen-ui` | Wails v2 dashboard (WebKitGTK + React). | yes | on demand |

`gwend` being CGo-free is load-bearing: it is what lets `GOOS=linux GOARCH=arm64 go build` produce a
Raspberry Pi binary with no cross-compiler toolchain, which the v3 sync hub needs. Do not introduce
a CGo dependency into `gwend`, `gwen`, or `gwen-tray`. This is why the SQLite driver is
`modernc.org/sqlite` (pure Go) and not `mattn/go-sqlite3`.

The three clients share one Go client package (`internal/client`) and never touch the database
directly. **The daemon is the sole owner of the database file.**

In v3 the same `gwend` binary also runs on the Raspberry Pi as `gwend --hub`, the sync hub, with no
activity monitor or engine; see [07-integrations.md](07-integrations.md#sync-hub).

## Transport

HTTP/1.1 with JSON bodies over a **Unix domain socket** at `$XDG_RUNTIME_DIR/gwen/gwend.sock`, mode
`0600`.

No TCP port is opened. Filesystem permissions are the entire authentication model — there are no
tokens, no passwords, and no auth middleware in v1 or v2, because anything that can read the socket
already runs as the user. (The v3 sync hub is a separate network service with its own bearer-token
auth; see [07-integrations.md](07-integrations.md#sync-hub).)

Routing uses stdlib `net/http` method-and-pattern routing (`mux.HandleFunc("GET /v1/status", ...)`).
No third-party router.

Live updates use **Server-Sent Events** on `/v1/events`. Clients do not poll. The running timer
ticks client-side from the open segment's `started_at`, so a ticking clock costs zero IPC. Full
surface in [04-api-contract.md](04-api-contract.md).

## Repository layout

```text
gwen/
├── cmd/
│   ├── gwend/              daemon entrypoint, wiring, signal handling
│   ├── gwen/               CLI (Cobra)
│   ├── gwen-tray/          tray
│   └── gwen-ui/            Wails host process
├── internal/
│   ├── api/                HTTP handlers, SSE hub, error mapping
│   ├── activity/           ActivityMonitor port + backends
│   ├── client/             Go client for the socket API
│   ├── clock/              Clock interface, real + fake
│   ├── config/             config.toml load/save/defaults
│   ├── model/              domain structs shared by all layers
│   ├── notify/             Notifier port + desktop/ntfy backends
│   ├── store/              SQLite, migrations, repositories
│   │   └── migrations/     embedded .sql files
│   ├── timeengine/         pure state machine
│   ├── wire/               JSON types for the API, shared by api and client
│   ├── planner/            deterministic scheduler          (v2)
│   ├── llm/                Planner adapters                 (v3)
│   ├── gcal/               Google Calendar sync             (v3)
│   ├── sync/               sync hub client                  (v3)
│   └── testutil/           shared fixtures
├── ui/                     Wails frontend (React + TS + Tailwind)
├── packaging/              nfpm.yaml, systemd unit, .desktop, icons
├── protocol/               vendored Wayland protocol XML
├── scripts/                docs-check.sh and build helpers
├── docs/                   this specification
└── Makefile
```

## Ports and adapters

Four ports isolate everything platform-specific or network-facing. Each has exactly one interface,
defined in the package named below, with the real backends behind it. This is what keeps the time
engine testable and what lets the app degrade gracefully when a backend is missing.

### `activity.ActivityMonitor`

Emits activity and session-lifecycle events. **The single hardest platform problem in this app.**

```go
type ActivityMonitor interface {
    // Start emits events until ctx is cancelled. thresholds are the idle
    // durations the caller wants to be notified at, ascending.
    Start(ctx context.Context, thresholds []time.Duration) (<-chan Event, error)
    Name() string
}

type Event struct {
    Kind      EventKind // Idle, Active, Locked, Unlocked, Suspend, Resume
    Threshold time.Duration // set on Idle, the threshold that fired
    At        time.Time
}
```

Backends are probed at startup in this order; the first that initialises wins and the choice is
logged at `Info`:

1. **`waylandIdle`** — in-process Wayland client speaking `ext_idle_notifier_v1`. Primary path on
   KWin, Mutter, and sway. The protocol XML is vendored at
   `protocol/ext-idle-notify-v1.xml`.
2. **`swayidleProc`** — spawns `swayidle -w` and parses its callbacks, if the binary is present.
3. **`dbusScreensaver`** — polls `org.freedesktop.ScreenSaver.GetSessionIdleTime` every 10s. X11
   only.
4. **`xprintidle`** — polls the `xprintidle` binary every 10s. X11 last resort.

**Verified on the target machine:** KDE Plasma 6.7.5 on Wayland returns `NotSupported` for
`GetSessionIdleTime`, so backend 3 does *not* work there — but KWin does advertise
`ext_idle_notifier_v1` version 2. Backend 1 is the real path on the development machine; the others
exist for portability.

Lock, unlock, suspend and resume come from `org.freedesktop.login1` over the system bus and are
merged into the same channel regardless of which idle backend won. `login1` is always active.

If every backend fails, the daemon still runs: it logs a `Warn`, tracks time manually, and disables
automatic break conversion. Tracking never hard-fails because idle detection is unavailable.

### `notify.Notifier`

```go
type Notifier interface {
    Notify(ctx context.Context, n Notification) error
    Withdraw(ctx context.Context, kind string) error // no-op where the backend cannot retract
    Name() string
}
```

Two backends run in parallel, not as fallbacks — desktop and phone should both fire. Failure of one
is logged at `Warn` and never blocks the other or the state machine. See
[07-integrations.md](07-integrations.md#notifications).

### `llm.Planner` (v3)

Optional enrichment. The deterministic planner in `internal/planner` is always present and always
authoritative; the LLM adapter only proposes, and its failures never affect tracking or planning.
What each LLM job does on failure is defined in
[07-integrations.md](07-integrations.md#llm-adapter).

### `store` repositories

One interface per aggregate (`ProjectRepo`, `TaskRepo`, `WorkDayRepo`, `SegmentRepo`, `StatsRepo`).
Concrete SQLite implementations only; the interfaces exist so the time engine and API can be tested
against fakes, not to support another database.

## Concurrency model

The daemon has **one goroutine that owns all mutable state** — the engine loop in `cmd/gwend`. It
selects over the activity event channel, the API command channel, the heartbeat ticker, and
context cancellation. Every state transition happens there, serially.

This is deliberate and is not negotiable: it means the time engine needs no locks, transitions can
never interleave, and the state machine can be tested by feeding it a slice of events. API handlers
do not mutate state directly — they send a command onto the channel and wait for the reply.

Reads (stats, lists, history) go straight to the store from the handler goroutine; SQLite in WAL
mode handles concurrent readers fine.

## Failure and degradation

| Failure | Behaviour |
|---|---|
| Idle backends all unavailable | Track manually, no auto-break, `Warn` at startup and in `GET /v1/status` |
| Desktop notification bus missing | Log `Warn`, continue; phone still fires |
| ntfy unreachable | Log `Warn`, continue; desktop still fires. No retry queue in v1. |
| Database unwritable at startup | **Fatal.** Exit non-zero with a clear message; the daemon must not run without durable storage. |
| Daemon killed mid-segment | Segment recovered and truncated on next start — [05-time-engine.md](05-time-engine.md#crash-recovery) |
| Socket already in use | If a live daemon answers `GET /v1/health`, exit non-zero. If it is stale, remove and bind. |

## Dependencies

The complete allowed list. **Adding anything requires a spec change first**
([README.md](README.md#change-protocol)).

| Module | Used by | For |
|---|---|---|
| `modernc.org/sqlite` | store | pure-Go SQLite driver, keeps `gwend` CGo-free |
| `github.com/pressly/goose/v3` | store | embedded migrations |
| `github.com/google/uuid` | model | UUIDv7 identifiers |
| `github.com/spf13/cobra` | cmd/gwen | CLI |
| `github.com/BurntSushi/toml` | config | config file |
| `github.com/godbus/dbus/v5` | notify, activity | desktop notifications, login1 |
| `github.com/rajveermalviya/go-wayland` | activity | Wayland client for idle protocol |
| `fyne.io/systray` | cmd/gwen-tray | StatusNotifierItem tray |
| `gopkg.in/natefinch/lumberjack.v2` | cmd/gwend | log rotation |
| `github.com/stretchr/testify` | tests | assertions |
| `github.com/wailsapp/wails/v2` | cmd/gwen-ui | desktop GUI shell |
| `google.golang.org/api` | gcal (v3) | official Calendar SDK |
| `golang.org/x/oauth2` | gcal (v3) | OAuth2 |
| `honnef.co/go/tools/cmd/staticcheck` | build tool | lint |
| `github.com/wailsapp/wails/v2/cmd/wails` | build tool | GUI build |
| `github.com/goreleaser/nfpm/v2/cmd/nfpm` | build tool | rpm and deb packages |
| `github.com/rajveermalviya/go-wayland/cmd/go-wayland-scanner` | build tool | Go bindings for the idle protocol, committed |

Build tools are recorded as `tool` directives in `go.mod` and never linked into a binary.
Everything else — routing, JSON, HTTP, SSE, logging, time — is stdlib.

Frontend (`ui/`, pinned in `package.json`): React, TypeScript, Vite, Tailwind, Recharts, and
`qrcode` (the phone-setup QR code). No component library, no router, no state-management library;
React state and the SSE stream are sufficient.

## Filesystem

XDG base directories, resolved with the standard env-var-then-default rules.

| Path | Contents |
|---|---|
| `~/.config/gwen/config.toml` | user configuration |
| `~/.local/share/gwen/gwen.db` | SQLite database (WAL mode) |
| `~/.local/share/gwen/credentials/` | secrets, mode `0700`; see [07-integrations.md](07-integrations.md#credentials) |
| `~/.local/state/gwen/gwend.log` | rotated daemon log |
| `$XDG_RUNTIME_DIR/gwen/gwend.sock` | API socket, mode `0600` |
