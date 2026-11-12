# Setup

Everything needed to get Gwen running: building it, installing it, the first-run wizard, and the
optional integrations (phone push, Google Calendar, an LLM, and the Raspberry Pi hub). This file is
the one place setup steps are written down. When a setup step changes, update it here.

The specification behind these steps is [docs/09-packaging.md](docs/09-packaging.md) (build,
packages, service, wizard, Pi) and [docs/07-integrations.md](docs/07-integrations.md) (credentials,
ntfy, Calendar, LLM, sync hub).

## 1. Requirements

| Needed for | What | Fedora | Debian / Ubuntu |
|---|---|---|---|
| Everything | Go 1.26.4 or newer, GNU make, git | `sudo dnf install golang make git` | `sudo apt install golang make git` |
| The GUI (`gwen-ui`) | Node.js 24 with npm | `sudo dnf install nodejs npm` | from nodejs.org or NodeSource |
| The GUI (`gwen-ui`) | GTK 3 and WebKitGTK 4.1 headers | `sudo dnf install gtk3-devel webkit2gtk4.1-devel` | `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev` |
| Gwen Terminal ([step 8](#8-gwen-terminal)) | Rust (rustup), Zig 0.16, `just`, a Vulkan driver | see step 8 | see step 8 |
| Gwen on your phone ([step 9](#9-gwen-on-your-phone)) | Tailscale on the laptop and the phone; `bubblewrap` for sandboxed sessions | `sudo dnf install bubblewrap` | `sudo apt install bubblewrap` |

- If your distro's Go is older than `go.mod` asks for, `go` downloads the right toolchain on first
  use, provided `GOTOOLCHAIN` is left at its default, `auto`. That needs Go 1.21 or newer to start
  from. Debian 12's `golang` is older, so install Go from [go.dev/dl](https://go.dev/dl/) there.
- Every other tool (wails, nfpm, staticcheck, the Wayland scanner) is pinned in `go.mod` and runs
  through `go tool`. Do not install them separately.
- Check with `go version` and `node --version`. Node must report `v24`.

The desktop must be a Linux session with a systemd user instance. Idle detection works on Wayland
(KDE Plasma, GNOME, sway) and falls back to X11 methods. The tray uses StatusNotifierItem, which
KDE shows natively. GNOME needs the *AppIndicator and KStatusNotifierItem Support* extension.

## 2. Install

Pick one.

### From a package (rpm or deb)

```sh
make package                                   # dist/gwen-X.Y.Z-1.x86_64.rpm, dist/gwen_X.Y.Z-1_amd64.deb
sudo dnf install ./dist/gwen-*.x86_64.rpm      # Fedora
sudo apt install ./dist/gwen_*_amd64.deb       # Debian, Ubuntu
```

Then go to [step 3](#3-first-run). A non-technical user can open the rpm or deb from the software
center instead, then open **Gwen** from the app menu. The GUI's first-run screen does step 3 for
them.

Untagged builds are packaged as version `0.0.0`. For a release, tag `vX.Y.Z` and run
`make package VERSION=X.Y.Z`.

dnf skips a package whose version is already installed, so give each new build a higher version
(`make package VERSION=0.1.1`), or run `sudo dnf reinstall ./dist/gwen-*.x86_64.rpm`. The package
is built from the working tree, so uncommitted changes go into it.

### From source, for development

```sh
make install-dev
```

This builds everything into `bin/`, copies `gwend`, `gwen`, `gwen-tray`, `gwen-ui` and `gwen-stt` to
`~/.local/bin`, writes the user unit to `~/.config/systemd/user/gwend.service`, and reloads systemd.
It also writes the app menu's one **Gwen** entry, `~/.local/share/applications/gwen.desktop`, which
opens the Gwen window ([step 8](#8-gwen-terminal)) on Today, and hides the window's own entry.
On KDE Plasma it also installs the Gwen panel widget (see [the panel widget](#kde-panel-widget)).
`~/.local/bin` must be on your `PATH`. Fedora's default `~/.bashrc` already adds it.

The first `make build` takes a while: wails runs `npm ci` and the Vite build in `ui/` before it
compiles `gwen-ui`.

### Moving from a dev install to the package

A dev install takes precedence over the package: `~/.local/bin` comes before `/usr/bin` on `PATH`,
and `~/.config/systemd/user/gwend.service` overrides the packaged unit. Remove it first, or the dev
build keeps running after the package is installed:

```sh
systemctl --user disable --now gwend
pkill -x gwen-tray
rm ~/.local/bin/{gwend,gwen,gwen-tray,gwen-ui}
rm ~/.config/systemd/user/gwend.service
kpackagetool6 -t Plasma/Applet -r dev.gwen.panel   # KDE only; the package ships its own
systemctl --user daemon-reload
hash -r
```

Build and install the package as in [From a package](#from-a-package-rpm-or-deb), then start it
from the package. Re-running `gwen setup autostart` replaces a tray entry that points into
`~/.local/bin`:

```sh
gwen setup service
gwen setup autostart
```

Check it with `which gwen`, which should print `/usr/bin/gwen`, and
`systemctl --user status gwend`, which should show the unit loaded from
`/usr/lib/systemd/user/gwend.service`. Both installs use the same data and config, so nothing needs
to move.

### Updating a package install

Installing the package does not restart anything, so the daemon, the tray, and an open Talk window or now card keep
running the old build until you restart them. Give the build a version higher than the installed
one, which `rpm -q gwen` shows:

```sh
make check
make package VERSION=0.1.3                         # higher than the installed version
sudo dnf install ./dist/gwen-*.x86_64.rpm          # make package clears dist/ first, so this is the new build
systemctl --user restart gwend
pkill -x gwen-ui                                   # closes Talk or the now card, so they reopen as the new build
pkill -x gwen-tray; setsid gwen-tray >/dev/null 2>&1 &
```

On Debian or Ubuntu, install with `sudo apt install ./dist/gwen_*_amd64.deb` instead. Then open Gwen
again from the tray.

## 3. First run

```sh
gwen setup          # or: gwen setup --yes  to accept every default
```

The wizard prompts with defaults shown in brackets. Press Enter to keep a default. It runs these
steps in order:

| Step | Subcommand (re-runnable on its own) | What it does |
|---|---|---|
| Service | `gwen setup service` | Enables and starts `gwend` as a systemd user service, so it starts at every login. Waits for it to answer. |
| Tracking | `gwen setup tracking` | Daily target (default `8h`), nudge after idle (`3m`), count as break after (`10m`). |
| Tray | `gwen setup autostart` | Puts the tray in `~/.config/autostart/` and starts it now. |
| Phone | `gwen setup phone` | Turns on phone push. See [step 4](#4-phone-push-ntfy). |
| Summary | | Prints what is on. |

Then try it:

```sh
gwen project add Internship
gwen in --project Internship     # clock in
gwen status
gwen out                         # clock out
```

Open Gwen from the app menu (**Gwen**), or run `gwen open`. Gwen's screens live in the Gwen window,
which [step 8](#8-gwen-terminal) builds; until it is installed, `gwen open` says so. `gwen open SCREEN`
opens one screen: `today`, `gwen` (the chat), `tasks`, `inbox`, `board`, `projects`, `plan`, `goals`,
`review`, `weekly`, `stats`, `history`, `connections`, `memory`, or `settings`. In the window,
**Ctrl+Shift+G** goes back to Gwen's screens from your terminals.

## 4. Phone push (ntfy)

Nudges reach your phone through [ntfy](https://ntfy.sh).

1. Install the free **ntfy** app on the phone (Android: Play Store or F-Droid. iOS: App Store).
2. Run `gwen setup phone`. Accept `https://ntfy.sh` as the server, or give your Pi's ntfy (see
   [step 7](#push-server)). Flags: `--server URL`, `--fallback URL`, `--token TOKEN` for a
   server that needs an access token.
3. In the app, subscribe to every URL it prints, such as `https://ntfy.sh/gwen-abc…`.
4. It sends a test notification. Send another any time with `gwen notify test`.

Gwen does the same under **Settings → Phone**: scan its QR code with the ntfy app, press
**Send test**, and paste an access token there if your server needs one.

The topic is effectively a password: anyone who knows it can read your nudges. Do not share it.

## 5. Google Calendar (optional)

Gwen writes your plan to a calendar of its own and reads the events of the calendars you choose, so plans fit around them.
Google requires you to create your own OAuth client once:

1. In [Google Cloud Console](https://console.cloud.google.com/), create a project and enable the
   **Google Calendar API**.
2. Configure the **OAuth consent screen** as External, add yourself as a test user, then **publish
   the app to production**. It stays unverified, which is fine for personal use. If you skip
   publishing, Google signs you out every 7 days.
3. Create an **OAuth client** of type **Desktop app** and download its JSON.
4. Install the client file and sign in. In Gwen: **Settings → Google Calendar →
   Choose client file**, then **Connect**. Or from a terminal:

   ```sh
   gwen setup calendar --client-file ~/Downloads/client_secret_….json
   gwen cal connect      # opens the browser to sign in
   gwen cal status
   ```

`gwen setup calendar` with no flag only prints the steps above.

Once connected, choose which calendars count as busy in **Settings → Google Calendar → Busy
calendars**. Their events show on Today and Plan, and the planner works around them. Timed events
always count, even ones shown as free. All-day events count only when shown as busy.

If you connected before Gwen read events (it used to ask only for free/busy access), Gwen asks you
to connect again: press **Reconnect**, or run `gwen cal connect`.

## 6. LLM (optional)

An LLM can propose task breakdowns for goals, plan a day with you in a chat, and write weekly
retros. The planner works without it.

```sh
gwen setup llm
```

It prompts for the provider, then the model, then:

| Provider | Also asks for | Notes |
|---|---|---|
| `none` | nothing | Turns the adapter off. This is the default. |
| `anthropic` | API key | Paid per request. |
| `openai_compatible` | endpoint, API key | For example Ollama at `http://localhost:11434/v1`, which needs no key (press Enter). |
| `claude_code` | the `claude` command | Uses your Claude Pro or Max subscription through Claude Code. No key. |

For `claude_code`, install Claude Code first and sign in with `claude auth login`. The wizard
defaults the command to the absolute path of `claude` on your shell's `PATH`, because the daemon's
`PATH` can differ. It then checks the sign-in and says whether it is signed in.

Keys are stored in `~/.local/share/gwen/credentials/llm_api_key`, readable only by you. Then try:

```sh
gwen goal breakdown <goal>
gwen plan chat "start at 14:00, 3 hours, DSA first"
gwen retro
```

The same settings are in Gwen under **Settings → AI**, which checks the Claude Code
sign-in as soon as you pick it and, when the command is not Claude Code, offers the `claude` it
finds. Breakdowns and retros then run from the **Assistant** screen, or from a goal's card, and
the day-planning chat from **Plan with AI** on the Plan or Today screen.

Claude Code can take a minute or two to answer, so keep `llm.timeout` at the default `2m` or
longer.

With the Anthropic API, the part of each prompt that stays the same (Gwen's personality, your
instructions, and the job's rules) is cached, so a request within a few minutes of the last one
costs less and answers sooner. The daemon logs each request's tokens, read in full and from the
cache, as `llm request`.

Chats with Gwen are kept in `~/.local/share/gwen/gwen.db`, and earlier chats open from **Past
chats** on the Assistant screen. Deleting a chat there erases its words from the database. Before
each upgrade that changes the database, the daemon copies it to `gwen.db.bak-<version>` in the
same folder and keeps the last two copies, so a chat deleted after an upgrade can remain in those
copies until two later upgrades replace them. Delete the `.bak-` files to erase it at once.


### Memory

Gwen remembers what you tell her: facts about you and the people in your life, each kept with the
words it came from. It is on by default and learns in the background with the model above (no extra
setup). See and edit what she knows on the **Memory** screen, or with `gwen memory`; change how it
works in **Settings, Memory** (the `[memory]` section of the config):

- **Learn from chats**: with the assistant's model, with a local OpenAI-compatible model
  (`local_endpoint`, `local_model`), or off.
- **Semantic search** (optional): install [Ollama](https://ollama.com), run
  `ollama pull nomic-embed-text`, and switch it on (`embed_endpoint = "http://localhost:11434"`).
- **Readable memory folder** (optional): a copy in `~/Gwen/memory` you can read and edit; with `git`
  installed, your edits flow back.
- **Mem0 and Supermemory** (optional, off): give the URL of a server you run yourself on this machine
  (`mem0_url`, `supermemory_url`); API keys go in the credentials folder as `mem0_api_key` and
  `supermemory_api_key`. Start them with telemetry off (`MEM0_TELEMETRY=false`,
  `SUPERMEMORY_DISABLE_TELEMETRY=1`) and bound to `127.0.0.1`.
- Reading PDFs you add as documents needs `pdftotext` (`sudo dnf install poppler-utils`).

## 7. Raspberry Pi (optional)

A Pi 4 or 5 on 64-bit Raspberry Pi OS. It can host the push server, the sync hub, or both. In the
commands below, replace `PI_ADDRESS` with the Pi's address and `pi` with your login on the Pi. With
[Tailscale](#tailscale), the address is the Pi's name, such as `gwen-pi`, and works from anywhere.
Without it, use the Pi's LAN address, which works only on the home Wi-Fi.

### Prepare the Pi

1. Install the imager on the laptop: `sudo dnf install rpi-imager`, then open **Raspberry Pi
   Imager**.
2. Choose your Pi model, **Raspberry Pi OS (other) → Raspberry Pi OS Lite (64-bit)**, and the SD
   card.
3. Under **Edit settings**, set the hostname (for example `gwen-pi`), a username and password, your
   Wi-Fi, and on the **Services** tab **Enable SSH** with password authentication. Write the card.
4. Boot the Pi from the card and wait 2–3 minutes. Then, from the laptop:

```sh
ssh pi@gwen-pi.local                            # or the IP your router shows for the Pi
sudo apt update && sudo apt full-upgrade -y
uname -m                                        # must print aarch64
```

### Tailscale

Tailscale joins the laptop, the Pi, and the phone into one private network, so the phone and a
laptop away from home still reach the Pi. Nothing is opened to the internet. It is free for
personal use.

On the Pi, and again on the laptop:

```sh
curl -fsSL https://tailscale.com/install.sh | sh
sudo systemctl enable --now tailscaled
sudo tailscale up                               # prints a login link: open it and sign in
```

Use the same account on every device. On the phone, install the **Tailscale** app, sign in with that
account, and switch it on.

Then, in the [admin console](https://login.tailscale.com/admin/machines), open the Pi's **⋯** menu
and choose **Disable key expiry**, or the Pi drops off the network after 180 days.

Check from the laptop:

```sh
tailscale status                                # lists the Pi, the laptop, and the phone
tailscale ping gwen-pi                          # pong
```

If a name does not resolve, use the device's `100.x.y.z` address from `tailscale status` or the
phone app instead.

### Push server

Requires Docker on the Pi.

```sh
scp -r packaging/pi pi@PI_ADDRESS:~/gwen-pi
ssh pi@PI_ADDRESS
cd ~/gwen-pi
nano compose.yaml        # replace PI_ADDRESS in NTFY_BASE_URL with the Pi's address
docker compose up -d
```

Then, on the laptop:

```sh
gwen setup phone --server http://PI_ADDRESS:8080 --fallback https://ntfy.sh
```

In the phone app, subscribe to the topic on **both** servers. The phone only reaches the Pi while
it is on the home Wi-Fi or has the Tailscale app switched on. If your phone is usually away from home
and you do not use Tailscale, keep `https://ntfy.sh` as the primary server instead.

### Sync hub

The hub keeps a copy of your data, syncs between machines, and serves a read-only dashboard for the
phone.

On the laptop, build the arm64 daemon. Run `make ui` first so the hub dashboard is included.
Without `ui/node_modules`, the hub serves a placeholder page.

```sh
make ui
make build-hub                                  # bin/arm64/gwend
scp bin/arm64/gwend packaging/pi/gwen-hub.service pi@PI_ADDRESS:~
```

On the Pi:

```sh
sudo install -m 0755 ~/gwend /usr/local/bin/gwend
sudo useradd --system --home /var/lib/gwen gwen
sudo install -d -o gwen -m 0700 /var/lib/gwen /var/lib/gwen/credentials
openssl rand -hex 32 | sudo -u gwen tee /var/lib/gwen/credentials/sync_token
sudo chmod 0600 /var/lib/gwen/credentials/sync_token
sudo install -m 0644 ~/gwen-hub.service /etc/systemd/system/
sudo systemctl enable --now gwen-hub
```

The `tee` line prints the token. Keep it in a password manager: it opens all your data. Print it
again with `sudo cat /var/lib/gwen/credentials/sync_token`. Check the hub is up with
`systemctl status gwen-hub` (look for `active (running)`).

Copy the token, then on each laptop:

```sh
gwen setup sync --hub http://PI_ADDRESS:7777 --token <token>
gwen sync status
```

Or enter the hub address and token in Gwen under **Settings → Sync**, and press **Sync
now**. The first sync copies everything to the hub; after that the laptop syncs every 5 minutes.

The phone dashboard is at `http://PI_ADDRESS:7777/` (`http`, not `https`). With Tailscale switched on
in the phone app, that is `http://gwen-pi:7777/` from anywhere. Sign in with the token; it stays
signed in for 30 days. Add it to the home screen (Chrome: **⋮ → Add to Home screen**. Safari:
**Share → Add to Home Screen**) to open it like an app. It is read-only: it shows the plan, tasks,
goals, days, and stats, and every change still happens on the laptop.

Keep the hub on the same version as the laptops. When an update changes the database (`gwen
health` prints the schema), rebuild the hub with `make build-hub` and copy it over as above, then
`sudo systemctl restart gwen-hub`. Until then sync keeps working for what the hub knows: the laptop
holds back the newer tables (such as chats and memory) and sends them once the hub is updated.

```sh
make build-hub
scp bin/arm64/gwend pi@PI_ADDRESS:~
ssh pi@PI_ADDRESS 'sudo install -m 0755 ~/gwend /usr/local/bin/gwend && sudo systemctl restart gwen-hub'
```

### Preview the phone dashboard without a Pi

Run a hub on the laptop against a copy of your data. Your own daemon keeps running and is not
touched.

```sh
mkdir -p -m 0700 /tmp/gwen-hub/credentials
sqlite3 ~/.local/share/gwen/gwen.db ".backup /tmp/gwen-hub/gwen.db"   # safe copy while gwend runs
openssl rand -hex 32 | tee /tmp/gwen-hub/credentials/sync_token        # the sign-in token
(cd ui && npm run build:hub)
go build -o /tmp/gwen-hub/gwend ./cmd/gwend
/tmp/gwen-hub/gwend --hub --listen :7777 --data-dir /tmp/gwen-hub --config /tmp/gwen-hub/config.toml
```

Open `http://localhost:7777/` and sign in with the token. In Firefox, press **Ctrl+Shift+M** for a
phone-sized view. To see it on the real phone, open `http://LAPTOP_ADDRESS:7777/` there: the laptop's
Tailscale address (`tailscale ip -4`), or its Wi-Fi address (`ip -4 addr`) on the same Wi-Fi.
Press **Ctrl+C** to stop it, then `rm -r /tmp/gwen-hub`. The copy does not update; repeat the
`sqlite3` line and restart it to see newer data.

## 8. Gwen Terminal

Gwen Terminal is the Gwen window, Gwen's one desktop app. Its first space, **Gwen**, holds her
screens as tabs (Today, Gwen, Tasks, Plan, Review, Connections, Memory, Settings); every other space
holds your terminals and Claude Code sessions. It is Herdr's app ([herdr.dev](https://herdr.dev))
under Gwen's name: a thin fork kept in its own folder, next to **herdr-core**, the small background
program that keeps your terminals running when the window closes. Every Claude you start in it can be
seen and driven from your phone (step 9).

### Tools

```sh
# Rust, if you don't have it (rustup picks the exact version Herdr builds with by itself)
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Zig 0.16 exactly (newer versions don't build Herdr), into ~/.local
curl -fsSL https://ziglang.org/download/0.16.0/zig-x86_64-linux-0.16.0.tar.xz | tar -xJ -C ~/.local/share
ln -sfn ~/.local/share/zig-x86_64-linux-0.16.0/zig ~/.local/bin/zig

# just
sudo dnf install just          # Fedora; or: brew install just; or: cargo install just
```

Check with `zig version` (must say `0.16.0`) and `just --version`. Herdr's own tests also use
[bun](https://bun.sh), which `make term-update` needs.

### The fork

Once, get the fork and put Gwen's patch on it. If the folder already exists (it does on the machine
this was first built on), skip this.

```sh
git clone -o upstream https://github.com/herdrdev/herdr-app ~/Documents/coding/gwen-term
cd ~/Documents/coding/gwen-term
git submodule update --init --recursive --depth 1
git switch gwen        # the branch with Gwen's patch; it comes from wherever you keep the fork
```

The patch is a handful of small commits (Gwen's name and icon, its own settings folder
`~/.config/gwen-term`, and no calls to herdr.dev) plus one generated commit that writes "Gwen" over
"Herdr" in what you read. Everything else is Herdr, unchanged: layouts, mods, themes, and keys work
exactly as Herdr's do, and mods and layouts from Herdr's community gallery work as they are.

### Build and install

```sh
make term
```

This builds herdr-core at the release named in `term/core.version` and the fork, then installs
`gwen-term` in `~/.local/bin` and Gwen's own core as `~/.local/lib/gwen/herdr-core`, and adds
**Claude (sandbox)** to Gwen Terminal's agent menu. It puts Gwen's screens (the `gwen` mod) and her
three themes (Ghost, Tide, and Ember, in the theme picker beside Herdr's) in `~/.config/gwen-term`, and
adds **Ctrl+Shift+G** to its keys. The window's own app menu entry is hidden: **Gwen** in the app menu
(from `make install-dev` or the package) opens it. The first build takes about 20 minutes; later ones
a minute. If Gwen's core is running, it is restarted onto the new build and its terminals keep running.

`gwend` also puts the screens in place each time it starts, so a newer `gwend` brings its matching
screens without `make term`. The first time it sees the window open, it makes the **Gwen** space with
one tab per screen. Close a screen's tab and it comes back, in its place, when you open that screen
from the tray, the panel widget, or `gwen open`. When `gwend` is stopped, Gwen's tabs say so and offer
to start it; they reconnect by themselves when it is back. A Gwen tab splits beside a terminal like
any other tab.

Gwen Terminal is separate from a Herdr you run yourself. It has its own core (the session `gwen`), so
it shows none of your Herdr's spaces or terminals, your Herdr shows none of Gwen's, and `make term`
never touches your Herdr's `herdr-core` or `hc`. Your phone sees the sessions in Gwen Terminal only.
To look at Gwen's core from a shell: `HC_SESSION=gwen ~/.local/lib/gwen/herdr-core ls`. A folder other than `~/Documents/coding/gwen-term` is `make term TERM_DIR=/path`.

To take Herdr's newer fixes:

```sh
make term-update     # rebases the patch on Herdr, regenerates the wording, runs Herdr's own checks
make term            # builds and installs the result
```

If Herdr changed a line the patch touches, `make term-update` stops and says where; resolve it in the
fork's folder, then run it again. Nothing is installed until the checks pass.

Gwen Terminal never contacts herdr.dev: it has no update checks, downloads, feedback, or usage reports.
A mod you add from Herdr's gallery runs with your full access and nobody reviews gallery code, so add
only mods that you or an agent have read.

## 9. Gwen on your phone

From your phone you can see every Claude session and terminal in Gwen Terminal, type into them, answer
Claude's permission requests, review what changed, start new sessions, and talk instead of typing. It
travels over your tailnet only. A paired phone can type into any terminal on the laptop, so treat it
like the laptop itself, unlocked.

1. **Tailscale** runs on the laptop and on the phone, signed in to the same account.
2. **HTTPS certificates** are on for your tailnet: open
   [login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns) and turn on **HTTPS
   Certificates**. Without them the phone's browser refuses the page and the microphone. The laptop's
   tailnet name then appears in the public certificate logs, as with any HTTPS certificate.
3. Open **Gwen › Settings › Phone** and turn on **Use Gwen from my phone**. It asks for your password
   once, because Tailscale needs root to publish. From a terminal the same is `gwen remote on`.
4. Turn on **Let Gwen see my Claude sessions**. This adds Gwen's hooks to `~/.claude/settings.json`,
   keeps everything else in the file, and saves the original once as `settings.json.gwen-backup`.
   (`gwen remote hooks on`.)
5. Press **Pair a phone** and scan the QR code with the phone's camera. The code works once, for ten
   minutes. On the phone, use the browser's **Add to Home screen** to keep Gwen as an app.

`gwen remote status` shows each of these and what is missing. `gwen remote devices` lists paired
phones and `gwen remote revoke <id>` unpairs one.

**Sandbox.** A session started as *Sandbox* (from the phone, or **Claude (sandbox)** in Gwen Terminal,
or `gwen sandbox` in a project folder) only sees its own folder and your toolchains. It cannot see
the rest of your home folder, your SSH keys, your keyring, your desktop, your other terminals, or
your other Claude conversations, so it cannot push over SSH. Nothing it writes takes effect outside
by itself:

- It has a private Claude folder of its own, per project, under `~/.local/share/gwen/sandbox/`.
  Of your real `~/.claude` it sees only your settings, instructions, commands, agents, skills and
  plugins, read-only, so it cannot add a hook, a command or an MCP server to your own Claude.
- It signs in separately. Run `gwen sandbox-login` once (or paste the token from `claude setup-token`
  into **Settings › Phone › Sandbox sign-in**). That token can only talk to the model, so a sandboxed
  Claude has none of your Claude connectors either. Without it, each sandboxed project asks you to
  `/login` the first time.
- Your tool caches (Go modules, npm, pip, cargo) are used read-only with a private layer on top, so
  it cannot leave something in a cache that your own builds would later trust.
- The project's own git hooks and git config are read-only in the sandbox, because git runs them by
  itself the next time you use the repository. So `git commit` works there, but `git config` and
  `git remote add` do not.

The one thing a sandbox cannot do for you is vouch for what Claude wrote *inside* the project: its
code, scripts and build files run with your access when you run them later. Review Changes first.

**Voice** uses the speech model from **Settings › AI › Voice**; install it there first. The phone
records while you hold the mic, the laptop turns it into text, and the text waits in the message box
for you to send.

**Pushes** when a session needs you use the ntfy setup of [step 4](#4-phone-push-ntfy). They name the
folder only.

## Where things live

| What | Path |
|---|---|
| Config | `~/.config/gwen/config.toml` (view with `gwen config get`, change with `gwen config set KEY VALUE`) |
| Database | `~/.local/share/gwen/gwen.db` |
| Secrets | `~/.local/share/gwen/credentials/` (mode `0700`; never in the config file) |
| Daemon log | `~/.local/state/gwen/gwend.log`, and `journalctl --user -u gwend` |
| API socket | `$XDG_RUNTIME_DIR/gwen/gwend.sock` |
| Phone and hook sockets | `$XDG_RUNTIME_DIR/gwen/remote.sock` (published by Tailscale Serve while the phone remote is on) and `hook.sock` (Claude Code's hooks) |
| Gwen Terminal | `~/.config/gwen-term/` (settings, keys, layouts, mods, themes; Gwen's screens are `mods/gwen`, which `gwend` replaces with its own copy at start); the fork in `~/Documents/coding/gwen-term`; herdr-core's source in `~/.cache/gwen/herdr-core` |
| Gwen's voice | `~/.config/gwen/speech.json`: the provider and voice chosen in **Settings → AI**, which the Talk window uses too |
| User unit | `/usr/lib/systemd/user/gwend.service` (package) or `~/.config/systemd/user/gwend.service` (dev) |
| Tray autostart | `~/.config/autostart/gwen-tray.desktop` |
| Now card placement (KDE) | Window rule `gwen-now-card` in `~/.config/kwinrulesrc`, and a KWin script the tray loads from `$XDG_RUNTIME_DIR/gwen/now-card.js` |
| Panel widget (KDE) | `/usr/share/plasma/plasmoids/dev.gwen.panel` (package) or `~/.local/share/plasma/plasmoids/dev.gwen.panel` (dev) |

### KDE panel widget

KDE's tray shows only an icon, so the timer lives in a panel widget: a dot in the state's colour, the
day's worked time ticking, and the task (or project) being tracked. Clicking it drops down the now
card, Plasma's own popup: the timer, the time, progress to the target, the current stretch, the next
plan block, an energy check-in, and Break, Clock out, Talk, and Open Gwen. Click the project under the timer
to switch project. A right click has the rest of the tray's commands.

The widget does everything the tray does, so while it is on a panel the tray stays out (`gwen-tray`
exits at start, saying so in the journal) and Gwen shows once. Remove the widget and the tray is back
at the next login, or start it now with `setsid gwen-tray &`.

To add it: right-click the panel, choose **Add Widgets…**, search for **Gwen**, and drag it next to
the tray. To move it later, right-click the panel, **Enter Edit Mode**, and drag it.

## Development loop

```sh
make build          # bin/gwend, bin/gwen, bin/gwen-tray, bin/gwen-ui
make check          # gofmt, vet, staticcheck, go test -race, docs-check, tsc. Must pass before a package is done.
make docs-check     # the spec's change protocol only
gwend --foreground  # run the daemon in a terminal with readable logs on stderr
make clean          # remove bin/ and dist/
```

Gwen's screens are `term/mods/gwen/`. After changing them, `go run ./cmd/gwend --install-term` copies
them into `~/.config/gwen-term/mods/gwen`, and an open Gwen window reloads them. `gwen-term check
~/.config/gwen-term/mods/gwen` loads the installed copy once and reports what's wrong. Check the
installed copy, not `term/mods/gwen`: the check writes editor type files into the folder it checks.

To run a fresh daemon from `bin/` in a terminal, stop the service first so the two do not fight
over the socket: `systemctl --user stop gwend`. After changing code, `make install-dev` and
`systemctl --user restart gwend` put the new build in place. If the package is installed, this
makes the dev build take precedence over it again. See
[Moving from a dev install to the package](#moving-from-a-dev-install-to-the-package).

`VERSION=1.2.3` overrides the version stamped into the binaries, which otherwise comes from
`git describe`.

CI (`.github/workflows/check.yml`) runs `make check` on Ubuntu 24.04 with Go from `go.mod` and
Node 24.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `gwen` exits with code 3, "daemon not running" | `systemctl --user status gwend`, then `journalctl --user -u gwend -e`. Or run `gwend --foreground` to see the error. |
| `gwen setup service`: "did not answer within 5 s" | Same as above: the log names the cause. |
| No tray icon on GNOME | Install the AppIndicator and KStatusNotifierItem Support extension, then `gwen setup autostart`. |
| `make build` fails on `webkit2gtk-4.1` or `gtk+-3.0` | Install the GUI headers from [step 1](#1-requirements). |
| Calendar disconnects every 7 days | The OAuth consent screen is still in testing. Publish it to production ([step 5](#5-google-calendar-optional)), then `gwen cal connect`. |
| `claude_code` provider is unavailable | Open **Settings → AI** in Gwen and press **Use it** on the `claude` it finds, or re-run `gwen setup llm` and give the full path to `claude`. Check `claude auth status`. |
| `gwen open`: "the Gwen window isn't installed" | Build it: [step 8](#8-gwen-terminal). |
| Gwen's tabs say "Gwen isn't running" | Press **Start Gwen** there, or `systemctl --user start gwend`. The tabs reconnect by themselves. |
| No **Gwen** space in the Gwen window | Run `gwen open`: it makes the space again. |
| AI requests fail with "timed out" | Raise the wait under **Settings → AI**, or `gwen config set llm.timeout 2m`. |
| Phone gets nothing | `gwen notify test`. Check the app is subscribed to the exact topic URL, and that the phone can reach the server. |
| Phone cannot open the hub dashboard | Switch the Tailscale app on. Use `http://`, not `https://`. If the name fails, use the Pi's `100.x.y.z` address from the app. |
| Hub dashboard is empty | The laptop has not synced yet: **Settings → Sync → Sync now**, or `gwen sync now`. |
| `gwen-hub` does not start | `sudo journalctl -u gwen-hub -n 30` on the Pi names the cause. |
| `make docs-check` fails on an extra file in `docs/` | The spec folder is a closed set of 13 files. Move the extra file out of `docs/`. |

## Uninstall

Your data is kept.

```sh
gwen setup service --disable
gwen setup autostart --disable
sudo dnf remove gwen            # or: sudo apt remove gwen
```

Data stays in `~/.local/share/gwen/` until you delete it by hand.













# 1. Build the package. Use a real version: 0.0.0-1 is already installed, so dnf would skip a rebuilt 0.0.0.
make check                       # setup.md says it must pass before packaging
make package VERSION=0.1.0       # → dist/gwen-0.1.0-1.x86_64.rpm

# 2. Remove the dev install
systemctl --user disable --now gwend
pkill -x gwen-tray
rm ~/.local/bin/{gwend,gwen,gwen-tray,gwen-ui}
rm ~/.config/systemd/user/gwend.service
systemctl --user daemon-reload
hash -r

# 3. Install (upgrades 0.0.0 → 0.1.0)
sudo dnf install ./dist/gwen-0.1.0-1.x86_64.rpm

# 4. Start it from the package
gwen setup service
gwen setup autostart

# 5. Check
which gwen                                    # /usr/bin/gwen
systemctl --user status gwend | grep Loaded   # /usr/lib/systemd/user/gwend.service
gwen status








make check
make package VERSION=0.1.2                       # use a higher number each time
sudo dnf install ./dist/gwen-0.1.2-1.x86_64.rpm  # upgrades the installed version to 0.1.2
systemctl --user restart gwend                    # the running daemon keeps the old binary until restarted
pkill -x gwen-tray; setsid gwen-tray >/dev/null 2>&1 &
systemctl --user restart plasma-plasmashell 