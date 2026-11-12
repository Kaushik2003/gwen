# Gwen's cross-device execution layer: running work on the laptop, a cloud box and the phone over Tailscale

## 0. Summary

- **gwend can join the tailnet itself.** It can embed `tsnet`, which makes the daemon its own Tailscale node with an HTTPS listener. For each request, `WhoIs` tells gwend which user and which device is calling, and the tailnet policy file can attach Gwen-specific permissions to that answer ("app capabilities"). None of this needs a reverse proxy or a login system.
- **Brain and workers are separate.** The always-on node (the Pi hub, with the cloud box as warm standby) owns scheduling, the run log and approvals. The laptop is a worker for desktop-app work and local files. The cloud box is a worker running stacyvm sandboxes. The phone is a client: talking, reviewing and approving.
- **Code moves between machines as `git bundle` files** sent over gwend's own tailnet API. This needs no SSH keys, no git server and no sync daemon. Results come back as a branch.
- **Run history becomes an append-only log, one per node.** Because each node only appends to its own log, there are no conflicts. It rides on the existing envelope sync, and Litestream covers disaster recovery.
- **The phone surface is a PWA served over the tailnet**, using a passkey for approvals. ntfy stays the push channel, and Claude Code Remote Control covers live coding sessions.

---

## 1. Tailscale building blocks

| Block | What it gives Gwen | Status, limits (VERIFIED unless marked) |
|---|---|---|
| **tsnet** (`tailscale.com/tsnet` v1.104.1, published 7 Oct 2026, BSD-3-Clause) | gwend becomes its own tailnet node, with its own hostname, separate from the system `tailscaled` | `Server` fields include `Hostname`, `Dir`, `Ephemeral`, `AuthKey`, `ClientSecret` (an OAuth secret that mints keys), `ClientID`/`IDToken`/`Audience` (workload identity federation), `ControlURL`, `AdvertiseTags`. Methods include `ListenTLS`, `ListenService`, `ListenFunnel`, `ListenSSH`, `LocalClient`, `HTTPClient`, `Dial`, `Up`. It uses a userspace network stack. |
| **Caller identity** | gwend knows which user and which device sent each request | `LocalClient().WhoIs(ctx, remoteAddr)` returns `Node`, `UserProfile` and `CapMap`, and the docs say "Node and UserProfile are never nil" on success. golink, Tailscale's own app, does exactly this: `s.lc.WhoIs(r.Context(), r.RemoteAddr)` and then `tailcfg.UnmarshalCapJSON[capabilities](whois.CapMap, "tailscale.com/cap/golink")`. |
| **Grants + app capabilities** | Gwen's permissions written in the tailnet policy file | A grant has `src`, `dst`, and `ip` and/or `app`. App capabilities are named `"domain/capName"`, and their parameters are "opaque JSON objects" that the application interprets itself. Grants are recommended over the older ACL syntax. |
| **Serve** | Expose a local port on the tailnet only | Adds `Tailscale-User-Login`, `Tailscale-User-Name` and `Tailscale-User-Profile-Pic` headers. These are "not populated for tagged devices or for Funnel traffic". From v1.92, `--accept-app-caps` also forwards a `Tailscale-App-Capabilities` header. |
| **Services** | A stable `svc:name` address that can move between hosts | The host must be tagged, devices need Tailscale v1.86.0 or later, and the service is approved through `autoApprovers`. tsnet supports it via `ListenService(name, ServiceModeHTTP{...})`. |
| **Funnel** | Exposes a node to the public internet | Beta. Ports 443, 8443 and 10000 only, with bandwidth limits you can't configure. **Gwen should not use it**, because nothing she does needs the public internet (INFERRED). |
| **HTTPS certificates** | Real TLS, which phone browsers require for the microphone, passkeys and service workers | Let's Encrypt via a DNS-01 challenge, valid 90 days. Machine names are published in public Certificate Transparency logs, so nodes should get neutral names. `ts.net` is on the Public Suffix List (VERIFIED in the list file), so a passkey's relying-party ID can be the full host name or `<tailnet>.ts.net`. |
| **Tags** | Server-style identity for the hub, workers and sandboxes | A tag replaces user ownership, and key expiry is off by default on tagged nodes. Tailscale advises against tagging end-user devices. Tagged sources cannot use SSH `check` mode. |
| **Auth keys / OAuth clients** | Unattended joining of workers | Key types: one-off, reusable, ephemeral, pre-approved, tagged. Expiry is 1 to 90 days. An OAuth client with the `auth_keys` scope must carry tags. Its secret can be passed directly as an auth key with `?ephemeral=…&preauthorized=…` (ephemeral defaults to true). Access tokens last 1 hour. |
| **Taildrop** | Sending files between devices | Alpha, "limited to sending files between your personal devices", and "You cannot use Taildrop to send files to and from nodes you have tagged". **This rules it out for worker → phone.** |
| **Taildrive** | Folder shares over WebDAV at `100.100.100.100:8080` | Alpha, needs v1.64.0+. Linux, macOS and Windows can share; iOS and Android can only access. Permissions use the `tailscale.com/cap/drive` app capability (`shares`, `access: "ro"`), and tagged hosts work. A possible read-only "browse files" path. |
| **Tailscale SSH** | Shell access for the owner to servers, plus the "check" model | `accept` or `check` actions. `checkPeriod` ranges from 1 minute to 168 hours, default 12 hours. Supported on Linux and macOS (open-source build). |
| **Pricing (Personal, free)** | | Up to 6 users, unlimited user devices, 50 tagged resources, 1,000 minutes per month for ephemeral nodes (an ephemeral node present for more than 4 hours counts as a tagged resource instead). Non-commercial use only. |
| **Headscale** (v0.29.4, 23 Sep; BSD-3) | A self-hosted control server | Grants since v0.29.0. Supports tags, ACLs, SSH, MagicDNS, Taildrop and Taildrive. **Funnel and Serve are not supported.** The features page doesn't mention HTTPS certificates. The maintainers target "a single tailnet" for personal use. |
| **Wake-on-LAN** | Waking the sleeping laptop | Tailscale's source has a `TS_WAKE_MAC` setting, and a commit adds a handler that lets the control server ask a node to send a WoL packet. I found **no documented user-facing feature**. The community pattern is a device on the same LAN, such as a Pi, sending the magic packet. |

**What this means for the design (INFERRED)**
- The 1,000 ephemeral minutes rule out making every sandbox a tailnet node. The stacyvm host should be one tagged node.
- `tsnet.ControlURL` keeps a later move to Headscale possible, as long as Gwen relies on `ListenTLS`/`WhoIs` and not on Serve. I could not verify whether tsnet's automatic certificates work under Headscale. Start on the hosted Tailscale service.

---

## 2. Phone surface

| Channel | Strengths | Weaknesses |
|---|---|---|
| **PWA served by gwend over the tailnet** (recommended primary) | Full user interface: chat, run timeline, diff review, approvals. The tailnet HTTPS certificate gives the browser the "secure context" that the microphone, passkeys and service workers require. Identity comes from `WhoIs` with no login screen. | The Tailscale app must be connected on the phone. iOS Web Push only works for apps added to the Home Screen, on iOS/iPadOS 16.4+, and the permission request must follow a user tap (VERIFIED, WebKit). |
| **ntfy** (already in use) | Pushes work today. Up to **three action buttons** per notification (`view`, `http`, `copy`; `broadcast` is Android-only). | On iOS, a self-hosted server needs `upstream-base-url` (ntfy.sh) for instant delivery. Only the message ID goes upstream, and the phone shows "New message" until it fetches the real text. Without it, delivery "can take hours". The docs don't say whether an `http` action is sent by the phone or by ntfy, so **use only `view` deep links into the PWA for approvals** (INFERRED). |
| **Claude app + Claude Code Remote Control** | Pro/Max subscriptions. The local session "makes outbound HTTPS requests only and never opens inbound ports". You can attach files from the phone, and a session survives the machine sleeping. Optional "Trusted Devices" setting. | Covers coding sessions only, and Claude Code's own permission prompts sit outside Gwen's kernel unless the kernel is served to Claude Code as its permission tool (as v1 proposes). Ideal for "let me steer the cloud coding run from my phone". |
| **Telegram bot** | Works with no tailnet, and long polling (`getUpdates`) means no inbound port | Bot downloads are limited to 20 MB and uploads to 50 MB. The FAQ doesn't say whether bot chats are end-to-end encrypted; I believe they are not (INFERRED). Use it as a fallback only, never for approvals. |
| **Signal (signal-cli)** | End-to-end encrypted. Can be linked as a secondary device. Has a JSON-RPC daemon. | Unofficial, GPL-3.0, needs JRE 25, and "releases older than three months may not work". |
| **Matrix (mautrix-go)** | Go library (MPL-2.0) with end-to-end encryption, cross-signing and key backup | Needs a homeserver to run. Heavier than its value for one user (INFERRED). |

**Voice from the phone**
- **Web Speech API.** On Chrome, "audio is sent to a web service", so it doesn't work offline. MDN now documents `processLocally`, `available()` and `install()` for on-device recognition, but the feature is "not Baseline" (not supported across major browsers).
- **Recommended (INFERRED):** push-to-talk in the PWA. `getUserMedia` (secure context only) and MediaRecorder record the clip, upload Opus to the brain, and the brain sends it to whichever node runs Parakeet: the laptop if awake, otherwise the cloud box. Kokoro TTS audio streams back. The phone's own keyboard dictation is the zero-effort fallback.

**Reviewing code on the phone**
1. **A diff view in the PWA.** diff2html (MIT) turns `git diff` output into line-by-line or side-by-side views with syntax highlighting. Line comments become follow-up instructions for the run. This is best for "review and approve".
2. **code-server** (MIT; needs about 1 GB RAM and 2 vCPU) on the worker, exposed only on the tailnet as a Tailscale Service. Useful for "open it in VS Code" on a tablet; cramped on a phone (INFERRED).
3. **GitHub Mobile.** Push the branch and open a PR, for places that have a `remote` (matches v1's Push branch / Open PR actions).
4. **Claude Remote Control** to keep steering the agent itself.

---

## 3. Moving work between machines

| Method | Fit for Gwen |
|---|---|
| **`git bundle` over gwend's tailnet API** (recommended) | "Bundles are used for the 'offline' transfer of Git objects without an active 'server'". Incremental bundles (`old..new`) need only the base to exist on the receiving side, and `git bundle verify` checks prerequisites. It goes over gwend's tsnet connection, which is already authenticated by `WhoIs`, so no SSH keys and no git daemon (VERIFIED for git; the design is INFERRED). |
| `git push` to a bare repo over Tailscale SSH | Works, but a tagged node only gets SSH `accept` rules and needs a server account to log into. That adds keys and accounts for no gain over bundles. |
| rsync over Tailscale SSH | For places that aren't repos (`notes`, `docs`). One-shot, with filter files. |
| Mutagen | Live two-way sync over SSH or into Docker. Last release **v0.18.1, Feb 2025**, which is stale by now. Only worth it for interactive remote development. |
| Syncthing | Peer-to-peer and encrypted. The official Android app was **discontinued (announced 20 Oct 2024, last release Dec 2024)**; forks exist. Live syncing of `.git` folders is risky (INFERRED). |
| rclone | Object storage and cloud drives. Good for large datasets, not for code. |
| Taildrive | A read-only share from the worker that the phone can browse. Alpha. |

**Snapshotting work in progress (INFERRED technique).** On the laptop, gwend builds a commit through a temporary index file. It sets `GIT_INDEX_FILE` to a scratch file, runs `git add -A`, `git write-tree` and `git commit-tree`, and points `refs/gwen/runs/<id>/base` at the result. This captures uncommitted changes and untracked files that aren't gitignored, without touching the owner's own index, working tree or stash. Before anything leaves the laptop:
- the place's `protected` globs and `.env*` are left out;
- a secret scan (for example gitleaks) runs, and a hit blocks the transfer (v1 hard floor #6);
- credentials never travel. The worker gets sentinel values, and its own egress proxy injects the real secrets (v1 §5.6).

**Bringing results back.** The worker commits to `refs/gwen/runs/<id>/result` and sends back an incremental bundle (`base..result`). The laptop applies it as branch `gwen/<slug>` (v1 §7.3). Merging stays a separate, approved action.

**Durable task state**
- **Today (VERIFIED from the repo):** synced rows carry an `Envelope{CreatedAt, UpdatedAt, DeletedAt, DeviceID, Rev}`. The Pi hub serves `/sync/v1/push` and `/sync/v1/pull`, with a cursor and `PullLimit = 1000`. That is a row-level, last-writer-wins model.
- **Proposal (INFERRED):** model runs as an append-only `run_events` table keyed by `(node_id, seq)` with UUIDv7 IDs, chained with hashes like v1's ledger. Each node only appends to its own log, so replication can't conflict, and the existing push/pull carries it unchanged. The `runs` and `tasks` tables become projections rebuilt from the log.
- **Leases:** a `run_leases(run_id, node_id, epoch, expires_at)` table written only by the brain. Workers include the `epoch` (a fencing token) with every result, and the hub rejects stale epochs. This prevents two nodes from both finishing the same run.
- **Disaster recovery:** Litestream (**v0.5.17, 31 Aug 2026**, Apache-2.0) replicates the hub's database. v0.5.17 adds an experimental VFS for read-only access to replicas, so the cloud box could serve a read-only "status" view while the Pi is down.
- **Alternatives I would not adopt now:** cr-sqlite (MIT; latest v0.16.3, dated 17 Jan with no year shown, so maintenance is uncertain) isn't needed, because append-only logs are already conflict-free. Electric describes itself as "a read-path sync engine for Postgres", which doesn't fit SQLite.

---

## 4. Cloud execution options

| Option | Isolation | Lifetime / persistence | GPU | Cost (as published) | Joining the tailnet |
|---|---|---|---|---|---|
| **Own box + stacyvm** (baseline) | stacyvm: Firecracker ("dedicated kernel per sandbox", host-guest traffic over vsock only) or Docker with runc/gVisor/Kata. Firecracker "requires read/write access to `/dev/kvm`", so it needs bare metal or nested virtualization; gVisor runs without it. | Sandboxes are disposable; "persistent volumes across sandboxes" is planned. Snapshot restore of about 28 ms is a roadmap figure. Multi-node through remote workers with mTLS. | no | Flat monthly. Hetzner's page lists specs (for example CPX22: 2 vCPU, 4 GB, 80 GB) but showed no prices when I read it. | The host is one tagged node. Sandboxes stay off the tailnet. |
| **E2B** | microVMs; the infra repo (Apache-2.0) is tagged firecracker/kvm and is self-hostable on GCP (AWS in beta) | Hobby: 1 h sessions, 20 concurrent, $100 one-time credit. Pro: $150/month, 24 h sessions. | — | 2 vCPU $0.000028/s + 4 GiB $0.0000180/s ≈ **$0.17/h** (my arithmetic) | stacyvm already has an E2B provider, so it can burst through stacyvm. |
| **Modal** | gVisor by default, or a VM runtime | Default lifetime 5 min, up to 24 h. Filesystem snapshots carry state beyond that. | yes (gVisor runtime only) | Sandbox rates are about 3x standard: $0.00003942 per core-second + $0.00000667 per GiB-second. 1 core (2 vCPU) with 4 GiB ≈ **$0.24/h** (my arithmetic). $30/month free on Starter. | An official example runs Tailscale in the container with a SOCKS5/HTTP proxy on `localhost:1080`, using an ephemeral key. |
| **Vercel Sandbox** | Firecracker; "VPN clients" are allowed as privileged processes | Hobby 45 min, Pro 24 h per session. Persistent by default. 64 GB disk. | no | $0.128 per Active-CPU hour (time waiting on I/O isn't billed) + $0.0212 per GB-hour of memory. 2 vCPU/4 GB at 10% CPU ≈ **$0.11/h** | It can run tailscaled. |
| **Daytona** | "dedicated kernel"; claims start in "under 90ms" | Auto-stop after 15 min, auto-archive after 7 days | — | — | The **open-source repo is "no longer maintained" as of June 2026**. Avoid depending on it. |
| **Fly Machines** | — | A stopped Machine starts in "well under a second"; creating one takes "low double digit seconds". Optional volumes. | — | — | (not checked) |
| **Cloudflare Sandbox SDK** | Containers (microVMs) behind a Worker. Needs Workers Paid. | Snapshots; can mount an R2 bucket; `enableInternet: false` | — | — | Tailscale isn't mentioned in the docs (unverified) |

**Recommendation (INFERRED)**
- **The cloud box running stacyvm is the default worker:** always on, flat cost, persistent worktrees on the host's disk, mounted into disposable sandboxes.
- **Burst:** E2B through stacyvm's provider. Modal only for GPU jobs.
- **Managed sandboxes never join as long-lived nodes.** If one must reach the hub, it joins as an ephemeral `tag:gwen-burst` node using an OAuth client secret, and grants let it reach only the hub's worker API.

---

## 5. Scheduling and security

**Where the brain lives (INFERRED)**
- Put it on the **Pi hub**: always on, low power, already the sync authority, and on the home LAN so it can send Wake-on-LAN packets. The brain mostly makes remote LLM calls, so the Pi's CPU is enough.
- The **cloud box is the warm standby**: a Litestream replica, and it can be promoted by hand if the house is offline.

**Placement: filter, then score**
- Each node's gwend advertises a manifest of capabilities: `desktop:wayland`, `apps:[spotify,code,…]`, `places:[…]`, `voice:stt`, `kvm`, `stacyvm`, `gpu`, `always_on`, `battery`, `metered`.
- **Filter on hard requirements:**
  - desktop-app actions run only on the laptop;
  - "data must not leave the device" means laptop or Pi only;
  - GPU means Modal or the laptop.
- **Score on:** whether the node is online (`ipnstate.PeerStatus.Online`, and `LastSeen` when offline, from `LocalClient().Status()`), data locality, expected duration versus laptop battery and sleep, and cost.
- The LLM may only choose among the feasible nodes and must explain its choice. Placement stays deterministic and auditable.

**When the laptop is involved**
- While a run holds the laptop, gwend takes a logind inhibitor lock (`sleep`+`idle`, block mode; `handle-lid-switch` is also possible, block mode only) through the D-Bus `Inhibit()` call. The lock is released when the returned file descriptor is closed (VERIFIED, systemd).
- **If the laptop is asleep:**
  1. If the Pi is on the LAN and the laptop is wired, send a WoL packet. Laptops on Wi-Fi usually can't be woken this way (INFERRED).
  2. If the task can use a **warm mirror**, run from it. A warm mirror is a place snapshot bundled to the cloud box after each commit or every N minutes while the laptop is awake.
  3. Otherwise queue the task and send an ntfy push: "Laptop asleep. Open the lid, or run from the mirror taken 2 hours ago?"

**Security model**
- **Identity:** every request is identified by `WhoIs` (`UserProfile` plus `Node.StableID`).
- **Only the owner's own devices can approve.** Tagged nodes have no user identity, so workers can never approve. Permissions are an app capability (placeholder name `gwen.<your-domain>/cap/gwen`) whose parameters gwend reads with `UnmarshalCapJSON`.
- **Enrollment:** a `devices` table binds each StableNodeID to a passkey credential.
- **Approvals (INFERRED design, extending v1's "payload-bound" approvals):**
  - The approval card's WebAuthn challenge is `H(random nonce ‖ payload hash ‖ run id ‖ epoch)`. A passkey assertion with user verification therefore proves the owner, on this phone, approved *this exact payload*. This replaces v1's "signed single-use link".
  - Tiers: the phone with a passkey can approve `local`/`account`/`world` actions. Rule changes and installs need a passkey assertion less than 5 minutes old, mirroring SSH `check` mode.
  - Voice keeps v1's limits.
  - ntfy buttons only open the PWA (`view`). They never approve on their own.
- **Tailnet hygiene:**
  - Turn on device approval and keep key expiry on user devices.
  - Workers get tagged pre-approved keys.
  - Grants restrict workers to the hub's worker port.
  - Consider Tailnet Lock later.

---

## 6. What this means for Gwen

### Topology

| Node | Tailnet identity | Role | Runs |
|---|---|---|---|
| Phone | owner's own device (Tailscale app) | client | PWA (chat, voice push-to-talk, diff review, passkey approvals), ntfy app, Claude app |
| Laptop (system) | owner's own device | human use | Taildrop, owner's SSH |
| Laptop `gwend` | tsnet node `gwen-laptop`, `tag:gwen-node` | worker | local places, desktop/app automation, local STT/TTS, snapshot/apply bundles, inhibitor locks |
| Pi `gwend` | tsnet node `gwen`, `tag:gwen-hub` | **brain** | scheduler, run-event log, leases, approvals, PWA, ntfy publisher, WoL sender, sync hub |
| Cloud box | system tailscaled + tsnet `gwen-cloud`, `tag:gwen-worker` | worker | stacyvm (Firecracker if `/dev/kvm`, else gVisor), persistent worktrees, Claude Code (subscription, `claude remote-control`), code-server as `svc:gwen-code`, Litestream replica |
| Managed sandbox (optional) | ephemeral `tag:gwen-burst` | burst | joins only if needed, through an OAuth client secret |

### Policy sketch

The email address and capability domain are placeholders.

```json
{
  "tagOwners": {"tag:gwen-hub":["autogroup:admin"],"tag:gwen-node":["autogroup:admin"],
                "tag:gwen-worker":["autogroup:admin"],"tag:gwen-burst":["tag:gwen-hub"]},
  "grants": [
    {"src":["owner@example.com"],"dst":["tag:gwen-hub"],"ip":["tcp:443"],
     "app":{"gwen.example.com/cap/gwen":[{"role":"owner"}]}},
    {"src":["owner@example.com"],"dst":["svc:gwen-code"],"ip":["tcp:443"]},
    {"src":["tag:gwen-node","tag:gwen-worker","tag:gwen-burst"],"dst":["tag:gwen-hub"],"ip":["tcp:7443"],
     "app":{"gwen.example.com/cap/gwen":[{"role":"worker"}]}},
    {"src":["tag:gwen-hub"],"dst":["tag:gwen-node","tag:gwen-worker"],"ip":["tcp:7443"]}
  ],
  "ssh": [{"action":"check","src":["owner@example.com"],"dst":["tag:gwen-worker","tag:gwen-hub"],
           "users":["autogroup:nonroot"],"checkPeriod":"12h"}]
}
```

There is deliberately no laptop ↔ worker grant. Bundles are relayed by the hub, or a direct `ip` grant can be added later for speed.

### The phone scenario, end to end

1. The owner opens the `https://gwen.<tailnet>.ts.net` PWA and says: "take `~/code/foo` to the cloud, fix the failing tests, let me review." The audio goes to the brain, which sends it to an STT node.
2. `WhoIs` identifies the owner's enrolled phone (role `owner`).
3. The brain plans: the place `foo` is on the laptop, the work needs `stacyvm`, so it runs on `gwen-cloud`. Laptop status: Online.
4. If the laptop is offline, the brain tries WoL, falls back to the warm mirror, or queues the task and pushes a choice to the phone.
5. The brain asks the laptop for a snapshot. The laptop builds the temporary-index commit, excludes protected files, runs the secret scan, and streams `bundle(base)` to the hub, which records `run_events`.
6. The hub forwards the bundle to the cloud box under a lease with epoch 1. The worker unbundles it into a persistent worktree and starts a stacyvm sandbox on its default profile, with secrets injected by the egress proxy.
7. The engine runs in the sandbox: Claude Code (with Remote Control enabled, so the owner can watch from the Claude app) or Gwen's own loop. Events stream to the hub over SSE.
8. Tests run. The worker commits `refs/gwen/runs/<id>/result` and sends back `bundle(base..result)` along with test output, stamped with epoch 1.
9. The hub publishes an ntfy `view` notification: "Run done, 4 files changed, tests pass", linking to `/runs/<id>/review`.
10. The phone shows a diff2html view. "Open in VS Code" opens `svc:gwen-code` on the worktree. Line comments start a follow-up run on the same lease chain.
11. The owner taps **Approve: create branch `gwen/fix-tests`**. The passkey assertion is bound to (result SHA, target branch, epoch). The hub verifies it and appends an approval event.
12. When the laptop is online, it fetches the result bundle and creates the branch, without merging. Optionally the brain pushes the branch and opens a PR, which is a `world` action needing approval or a standing rule.
13. The worker garbage-collects the sandbox and keeps the worktree as the next warm mirror.

### Minimal pieces to build first

1. **`internal/tailnet`:** tsnet inside gwend (hub mode first), `ListenTLS` on 443, `WhoIs` middleware producing a `Caller{User, StableNodeID, Caps}`, plus an enrolled-devices table.
2. **`run_events` log and leases with epochs** on the existing envelope push/pull. Projections for runs and tasks.
3. **Node manifest and a filter-then-score placer.** Start with three rules: desktop-only, place-local, cloud-capable.
4. **Bundle transport:** snapshot via the temporary index, secret scan, incremental bundles, apply as a `gwen/<slug>` branch.
5. **A stacyvm worker adapter:** create a sandbox, mount the worktree, exec the engine, collect the result. Use stacyvm's REST API (`/api/v1`, `/sandboxes`, `/exec`).
6. **PWA v0:** chat, run timeline, diff2html review, passkey enrollment and payload-bound approvals, ntfy `view` deep links.
7. **Later:** a logind inhibitor during laptop runs, WoL from the Pi, warm mirrors, Litestream to the cloud box, code-server as a Service, push-to-talk voice routing, Telegram fallback.

---

## Sources

- https://tailscale.com/kb/1244/tsnet
- https://pkg.go.dev/tailscale.com/tsnet
- https://pkg.go.dev/tailscale.com/client/local
- https://pkg.go.dev/tailscale.com/client/tailscale/apitype
- https://pkg.go.dev/tailscale.com/ipn/ipnstate
- https://raw.githubusercontent.com/tailscale/golink/main/golink.go
- https://github.com/tailscale/golink
- https://tailscale.com/kb/1324/grants
- https://tailscale.com/docs/reference/syntax/grants
- https://tailscale.com/kb/1312/serve
- https://tailscale.com/kb/1223/funnel
- https://tailscale.com/docs/features/tsnet/how-to/register-service.md
- https://tailscale.com/kb/1153/enabling-https
- https://tailscale.com/kb/1068/tags
- https://tailscale.com/kb/1085/auth-keys
- https://tailscale.com/kb/1215/oauth-clients
- https://tailscale.com/kb/1106/taildrop
- https://tailscale.com/kb/1369/taildrive
- https://tailscale.com/kb/1080/cli
- https://tailscale.com/kb/1193/tailscale-ssh
- https://tailscale.com/kb/1196/security-hardening
- https://tailscale.com/pricing
- https://git.quad4.io/Mirrors/tailscale/src/commit/fc8b6d9c6a6c510227b21abf00bdfd24e53ab176/hostinfo/wol.go
- https://hackernoon.com/make-a-dedicated-wake-on-lan-server-with-tailscale
- https://github.com/juanfont/headscale
- https://github.com/juanfont/headscale/releases
- https://headscale.net/stable/about/features/
- https://raw.githubusercontent.com/publicsuffix/list/main/public_suffix_list.dat
- https://www.w3.org/TR/webauthn-3/
- https://developer.mozilla.org/en-US/docs/Web/API/SpeechRecognition
- https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getUserMedia
- https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/
- https://docs.ntfy.sh/config/#ios-instant-notifications
- https://docs.ntfy.sh/publish/#action-buttons
- https://code.claude.com/docs/en/remote-control
- https://core.telegram.org/bots/faq
- https://github.com/AsamK/signal-cli
- https://github.com/mautrix/go
- https://github.com/rtfpessoa/diff2html
- https://github.com/coder/code-server
- https://git-scm.com/docs/git-bundle
- https://github.com/mutagen-io/mutagen/releases
- https://syncthing.net/
- https://forum.syncthing.net/t/discontinuing-syncthing-android/23002
- https://github.com/benbjohnson/litestream/releases
- https://github.com/vlcn-io/cr-sqlite
- https://github.com/vlcn-io/cr-sqlite/releases
- https://electric.ax/docs/intro
- https://github.com/StacyOS/stacyvm
- https://github.com/firecracker-microvm/firecracker/blob/main/docs/getting-started.md
- https://gvisor.dev/docs/
- https://e2b.dev/pricing
- https://github.com/e2b-dev/infra
- https://modal.com/docs/guide/sandbox
- https://modal.com/pricing
- https://modal.com/docs/examples/modal_tailscale
- https://vercel.com/docs/vercel-sandbox
- https://vercel.com/docs/sandbox/pricing
- https://www.daytona.io/docs/en/
- https://www.daytona.io/docs/en/persistence
- https://github.com/daytonaio/daytona
- https://docs.fly.io/machines/overview
- https://developers.cloudflare.com/sandbox/
- https://www.hetzner.com/cloud/regular-performance/
- https://systemd.io/INHIBITOR_LOCKS/
- Repo files read: `/home/claude/gwen/internal/model/model.go`, `/home/claude/gwen/internal/sync/hub.go`; v1 harness: `/mnt/project-files/gwen-harness/harness.md`