# Packaging and deployment

How Gwen is built, packaged as rpm and deb, installed by a non-technical user, started at login,
and deployed to the Raspberry Pi.

## Build

A single `Makefile` at the repository root. Build tools are Go `tool` directives in `go.mod`, run
with `go tool <name>`, so their versions are pinned with everything else (see
[02-architecture.md](02-architecture.md#dependencies)).

| Target | Does |
|---|---|
| `make build` | `bin/gwend`, `bin/gwen`, `bin/gwen-tray` with `CGO_ENABLED=0`; then `bin/gwen-ui` via `go tool wails build -tags webkit2_41` in `cmd/gwen-ui/` ([08-clients.md](08-clients.md#gui)) |
| `make ui` | `npm ci && npm run build` in `ui/` |
| `make build-hub` | `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` build of `bin/arm64/gwend` |
| `make check` | `gofmt -s -l` (fails on any output), `go vet ./...`, `go tool staticcheck ./...`, `go test -race ./...` (all with `-tags webkit2_41`; vet and staticcheck also with `manual`), `make docs-check`, and `npx tsc --noEmit` in `ui/` when `ui/node_modules` exists |
| `make docs-check` | `scripts/docs-check.sh`, the checks in [README.md](README.md#what-make-docs-check-verifies) |
| `make package` | `make build`, then `go tool nfpm package` for rpm and deb into `dist/`, plus `dist/SHA256SUMS` |
| `make install-dev` | `make build`, copy binaries to `~/.local/bin`, install the dev user unit, `systemctl --user daemon-reload` |
| `make clean` | remove `bin/` and `dist/` |

**Version.** `VERSION` defaults to `git describe --tags --always --dirty` with the leading `v`
stripped, and is injected into every binary with `-ldflags "-X main.version=$(VERSION)"`. `gwend`
passes it to `GET /v1/health`.

**Release.** Tag `vX.Y.Z`, run `make package VERSION=X.Y.Z`, attach `dist/*` to the GitHub release.

## Daemon flags

| Flag | Default | Meaning |
|---|---|---|
| `--foreground` | off | Also log human-readable text to stderr. |
| `--data-dir DIR` | `$XDG_DATA_HOME/gwen` | Database and credentials location. |
| `--config FILE` | `$XDG_CONFIG_HOME/gwen/config.toml` | Config file. |
| `--hub` | off | Run as the sync hub; see [07-integrations.md](07-integrations.md#sync-hub). |
| `--listen ADDR` | `:7777` | Hub listen address; only valid with `--hub`. |

## Packages

`packaging/nfpm.yaml` produces both formats from one file. Its `version` is `${VERSION}`, which
`make package` sets from `VERSION`, falling back to `0.0.0` for an untagged build.

| Field | Value |
|---|---|
| `name` | `gwen` |
| `arch` | `amd64` |
| `license` | `MIT` |
| `description` | `Personal time tracker and study planner` |
| rpm `depends` | `webkit2gtk4.1`, `gtk3` |
| deb `depends` | `libwebkit2gtk-4.1-0`, `libgtk-3-0` |

Installed files:

| Path | Source |
|---|---|
| `/usr/bin/gwend`, `/usr/bin/gwen`, `/usr/bin/gwen-tray`, `/usr/bin/gwen-ui` | `bin/` |
| `/usr/lib/systemd/user/gwend.service` | `packaging/gwend.service` |
| `/usr/share/applications/gwen.desktop` | `packaging/gwen.desktop` |
| `/usr/share/gwen/gwen-tray.desktop` | `packaging/gwen-tray.desktop` (copied to autostart by setup, never installed there) |
| `/usr/share/icons/hicolor/scalable/apps/gwen.svg` and `48x48`, `128x128` PNGs | `packaging/icons/` |

The postinstall script prints one line — `Run "gwen setup" or open Gwen from your app menu to
finish.` — and does nothing else. Packages never enable services or autostart for anyone; that is a
per-user choice made in setup.

**Installing** for a non-technical user: download the rpm (Fedora) or deb (Ubuntu, Debian) and open
it with the software center, or `sudo dnf install ./gwen-X.Y.Z-1.x86_64.rpm`. Then open **Gwen**
from the app menu; the GUI's first-run screen does the rest.

Flatpak and AppImage are not built. Idle detection needs the host Wayland socket and the login1
system bus, and a sandboxed app cannot install a systemd user unit, so a Flatpak would need so many
holes that it would add nothing.

## Service

`packaging/gwend.service`:

```ini
[Unit]
Description=Gwen time tracker
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStart=/usr/bin/gwend
Restart=on-failure
RestartSec=5
MemoryHigh=60M
MemoryMax=100M

[Install]
WantedBy=graphical-session.target
```

It starts at graphical login, not at boot: idle detection needs the user's session, and there is
nothing to track before login. `PartOf` stops it at logout, which triggers the clean-shutdown path
in [05-time-engine.md](05-time-engine.md#crash-recovery).

`make install-dev` writes the same unit to `~/.config/systemd/user/gwend.service` with
`ExecStart=%h/.local/bin/gwend`.

`packaging/gwen-tray.desktop` is a standard autostart entry: `Exec=gwen-tray`, `X-GNOME-Autostart-enabled=true`,
`NoDisplay=true`. `packaging/gwen.desktop` launches `gwen-ui` with `Categories=Utility;Office;` and
`Icon=gwen`.

## Setup

`gwen setup`, implemented by W9 in `cmd/gwen/setup*.go`. Plain line-based prompts on stdin with the
default shown in brackets; `--yes` accepts every default without prompting. Every step is also its
own subcommand, safe to re-run.

| Step | Subcommand | Does |
|---|---|---|
| 1. Service | `gwen setup service [--disable]` | `systemctl --user enable --now gwend.service` (or `disable --now`), then waits up to 5 s for `GET /v1/health`. |
| 2. Tracking | `gwen setup tracking` | Prompts `tracking.daily_target`, `soft_idle`, `hard_idle`; `PATCH /v1/config`. |
| 3. Tray | `gwen setup autostart [--disable]` | Copies `/usr/share/gwen/gwen-tray.desktop` to `~/.config/autostart/` (or removes it) and starts `gwen-tray` now. |
| 4. Phone | `gwen setup phone [--server URL] [--fallback URL] [--token TOKEN]` | Generates `ntfy.topic` if empty; sets the servers and `nudge.phone = true`; writes `credentials/ntfy_token` when given; prints the subscribe URL and app install instructions; runs `POST /v1/notify/test`. |
| 5. Summary | — | Prints what is enabled and how to open the dashboard. |

v3 adds three subcommands, not part of the default run:

- `gwen setup calendar --client-file PATH` — prints the Google Cloud steps from
  [07-integrations.md](07-integrations.md#google-calendar), copies the file to
  `credentials/google_client.json`, sets `calendar.enabled = true`.
- `gwen setup llm` — prompts provider, model, endpoint, and key; writes `credentials/llm_api_key`;
  patches config.
- `gwen setup sync --hub URL --token TOKEN` — writes `credentials/sync_token`, sets `sync.hub_url`,
  then runs `POST /v1/sync/now`.

The GUI's first-run and Settings screens perform the same file and systemd operations through the
host methods in [08-clients.md](08-clients.md#gui).

**Uninstalling** keeps data: `gwen setup service --disable`, `gwen setup autostart --disable`, then
remove the package. Data lives on in `~/.local/share/gwen/` until deleted by hand.

## Raspberry Pi

A Raspberry Pi 4 or 5 running 64-bit Raspberry Pi OS with Docker installed. Everything lives in
`packaging/pi/`.

### Push server (v1)

`packaging/pi/compose.yaml`:

```yaml
services:
  ntfy:
    image: binwiederhier/ntfy:v2.11.0
    command: serve
    ports:
      - "8080:80"
    environment:
      NTFY_BASE_URL: "http://PI_ADDRESS:8080"
      NTFY_CACHE_FILE: /var/cache/ntfy/cache.db
    volumes:
      - ./ntfy-cache:/var/cache/ntfy
    restart: unless-stopped
```

Deploy: copy the directory to the Pi, replace `PI_ADDRESS` with the Pi's LAN address, and run
`docker compose up -d`. Then on the laptop: `gwen setup phone --server http://PI_ADDRESS:8080
--fallback https://ntfy.sh`, and in the phone's ntfy app subscribe to the printed topic on **both**
servers.

**Reachability.** The phone receives from the Pi only while it can reach the Pi — on the home Wi-Fi,
or anywhere if both devices are on the same Tailscale network (then use the Pi's Tailscale address).
The fallback covers the laptop failing to reach the Pi, not the phone being away. A user whose phone
is usually off the home network and who does not use Tailscale should set `ntfy.sh` as the primary
server.

### Sync hub (v3)

1. `make build-hub`, then copy `bin/arm64/gwend` to `/usr/local/bin/gwend` on the Pi.
2. Create a system user and data directory: `sudo useradd --system --home /var/lib/gwen gwen`,
   `sudo install -d -o gwen -m 0700 /var/lib/gwen/credentials`.
3. Generate the token: `openssl rand -hex 32 | sudo -u gwen tee /var/lib/gwen/credentials/sync_token`,
   then `chmod 0600` it.
4. Install `packaging/pi/gwen-hub.service` to `/etc/systemd/system/` and
   `sudo systemctl enable --now gwen-hub`.

`packaging/pi/gwen-hub.service`:

```ini
[Unit]
Description=Gwen sync hub
After=network-online.target
Wants=network-online.target

[Service]
User=gwen
ExecStart=/usr/local/bin/gwend --hub --listen :7777 --data-dir /var/lib/gwen --config /var/lib/gwen/config.toml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

On the laptop: `gwen setup sync --hub http://PI_ADDRESS:7777 --token <token>`. The phone dashboard
is then at `http://PI_ADDRESS:7777/`.
