# stacyvm as Gwen's execution layer: what it does today, where it falls short, and how to integrate it

**What I studied.** I read the clone at commit `74f342e` (2026-06-04). `git ls-remote` shows that commit is still remote HEAD, and the latest tag is `v0.15.0`. stacyvm is licensed Apache-2.0 (`LICENSE`). Its module is `github.com/StacyOs/stacyvm`, on `go 1.25.5`, with 25 direct and 120 indirect requirements (`go.mod`). Gwen is on `go 1.26.4` with `modernc.org/sqlite v1.59.0`; stacyvm uses `v1.45.0`. These are compatible.

**Labels.** **[V]** means I verified it in the cited file or URL. **[I]** means it is my inference or judgement.

**Effort scale** (one developer who knows the code): **S** is 3 days or less, **M** is 1 to 2 weeks, **L** is 3 weeks or more.

---

## 1. What stacyvm does today

### 1.1 Processes and components
- **One binary, `stacyvm`** (Cobra CLI) [V].
  - `serve` runs the whole stack in one process: the chi HTTP API, the orchestrator, a "local worker", the provider registry, the store and an in-memory event bus (`cmd/stacyvm/cmd_serve.go:60-308`).
  - `worker` runs a remote worker: a heartbeat to the control plane plus an optional inbound RPC server.
  - The binary also includes `tui`, `web-ui`, `doctor`, `db backup`, `upgrade rehearse` and `support bundle` (`README.md:536-551`).
- **Guest agent `stacyvm-agent`** [V]. It is a Linux-only binary. Inside a Firecracker VM it runs as PID 1, mounts `/proc`, `/sys` and `/dev`, seeds entropy, and serves on vsock port 1024 (`cmd/stacyvm-agent/main.go:30-79`).
- **Other clients** [V]:
  - Python and TypeScript SDKs, which are thin REST wrappers (`README.md:378`).
  - A Next.js dashboard embedded with `//go:embed`.
  - A Bubble Tea TUI.
  - A Wails desktop app (see §1.7).
- **Events** [V]. An SSE stream at `/api/v1/events`. A Postgres LISTEN/NOTIFY bridge is used only in Postgres mode (`cmd_serve.go:61-73`).

### 1.2 Lifecycle and the provider contract
- **States** [V]: `creating, running, idle, unhealthy, expired, destroyed, error` (`internal/orchestrator/types.go:7-15`).
- **TTL** [V]. A reaper goroutine expires sandboxes. The default TTL is `30m` and `max_ttl` is `24h` (`internal/config/config.go:281,288`). `ExtendTTL` sets `expires = now + extra` (`manager.go:2725-2746`).
- **Provider interface** [V] (`internal/providers/provider.go:71-119`):
  - `Name`, `Spawn`, `Exec`, `ExecStream`.
  - Eight file operations: write, read, list, delete, move, chmod, stat, glob.
  - `Status`, `Destroy`, `ConsoleLog`, `Healthy`.
  - Optional extras: `SnapshotLister`, `RuntimeSandboxLister` and `StatsReporter` (`:130-152`).
- **SpawnOptions is minimal** [V]: `Image, MemoryMB, VCPUs, DiskSizeMB, Metadata` (`:16-22`). There are no mounts, network, env, secrets, ports or devices.
- **Exec modes** [V]. `shell` runs through `sh -c`. `argv` runs without a shell (`provider.go:169-186`).
- **Spawn is serialized globally** [V]. `admissionMu` is locked in `acquireSpawnAdmission` and released by a `defer` at the end of `Spawn` (`manager.go:478-483`, `1847-1868`). Image pulls and VM boots therefore run one at a time.

### 1.3 Exec, files, preview, and the in-sandbox agent protocol
**REST surface** [V] (`internal/api/routes/sandboxes.go:42-80`):
- Sync exec.
- NDJSON streaming exec (`stream:true`).
- WebSocket exec at `/exec/ws`, which accepts any origin (`:696`).
- File endpoints, `/logs` and `/stats`.

**Streams carry no exit code** [V]. The stream chunk type is `{stream, data}` only (`provider.go:38-41`). The Firecracker stream path also drops the agent's `ExitCode` (`firecracker.go:641-705`). There is no stdin or PTY support; `later.txt` lists "Full bidirectional PTY" as a follow-up.

**Files are text-only JSON** [V]. `FileWriteRequest.Content` is a `string` (`types.go:63-67`). Reads load the whole file into memory (`manager.go:1094`). The repo's own SDK test reads binary output by running `base64` through exec (`sdk/python/tests/test_pdf_gen.py:131`). The Docker provider uses tar internally for each file (`docker.go:405-470`), but no archive API is exposed.

**Preview** [V]. This is Docker-only and port 3000 only. The Docker provider adds Traefik labels for `Host(3000-<id>.<domain>)` (`docker.go:226-232`). Previews have no authentication: anyone who can reach Traefik and knows the 8-hex-character ID gets through. The bundled compose file runs Traefik with `--api.insecure=true` (`docker-compose.yml`). SDK preview URLs are hard-coded to `http://` (`sdk/js/src/sandbox.ts:117-121`). The doc says Firecracker preview is "Pending" (`docs/live-preview-architecture.md:49-52`).

**Agent protocol** [V]. Each message is a 4-byte big-endian length prefix followed by JSON (`internal/agentproto/io.go`), with a 64 MB maximum message size. There are 12 methods: ping, exec, exec_stream, 8 file operations and shutdown (`protocol.go:9-22`). File content travels as `[]byte`, so this layer is binary-safe.

**Agent bugs that matter for long agent runs** [V]:
- Both `exec` and `exec_stream` hard-code `context.WithTimeout(..., 5*time.Minute)` (`main.go:203,256`).
- Two goroutines write stream frames to one `bufio.Writer` with no mutex (`:302-318`). Each frame is written in two calls (`io.go`), so frames can interleave [I].
- The writer is flushed only after the handler returns (`:161`), so "live" output is batched until the 4 KB buffer fills [I].
- The host keeps one vsock connection per VM and holds `connMu` for the whole stream (`firecracker.go:655-705`). Every other operation on that VM blocks while a stream runs.

### 1.4 Templates, images, snapshots, pools
**Templates** [V]. These are stored specs (`orchestrator/templates.go:13-28`). The schema already has `Setup`, `AllowedHosts`, `Env`, `Secrets{Name, InjectAt}` and `PoolSize`. However, `ToSpawnRequest` drops every one of those fields (`:105-115`): they are persisted and never applied.

**Warm pools are dead code** [V]. `PoolManager` is started (`cmd_serve.go:238-240`), but `Configure`/`Acquire` (`pool.go:52-78`) are never called anywhere.

**"Environments"** [V]. This is an image builder. It generates a Dockerfile from a base image plus apt and pip packages, builds it with Docker, and can push to a registry (`internal/environments/manager.go:287-330,405`).

**Firecracker rootfs** [V]. Built by `docker export`, then `mkfs.ext4` inside a `docker run --privileged` container, then `resize2fs -M` (`internal/providers/rootfs_builder.go:115-213`). Building it needs Docker and privilege.

**Snapshots are per-image "golden" snapshots only** [V]:
- The first cold boot of an image starts a background boot, pause and Full snapshot (`firecracker.go:133-298,577-590`).
- Later spawns restore from it, with relative drive and vsock paths (`:302-394`; claimed 28 ms in `docs/snapshot-restore.md`).
- There is no API to snapshot or restore a user's running sandbox.
- The documented benchmark guest kernel is 4.14 (`docs/snapshot-restore.md:33`). Firecracker's kernel policy lists only 5.10, 6.1 and 6.18 [V].
- Clones restored from one snapshot share RNG state. Firecracker recommends virtio-rng and a manual reseed for guest kernels before 5.18 [V]. stacyvm configures no `/entropy` device and does not reseed after restore [V]. At first boot, the agent credits 4096 bits of entropy from a time-and-PID-seeded LCG (`main.go:84-113`) [V]. Treat this as a security bug [I].

**Pool mode** [V]. N logical sandboxes share one container, each under `/workspace/<id>`. Path scoping applies to the file API only (`manager.go:1539-1564`); exec is not confined. The README says the hardening knobs "have no effect today" (`README.md:362`).

### 1.5 Remote workers, auth, policy
**Worker RPC** [V]. JSON over HTTP `/rpc` with lease fencing (`internal/workerproto/protocol.go`, `docs/worker-rpc-contract.md`). Optional mTLS with TLS 1.2 minimum (`internal/worker/tls.go`).

Limits in the worker RPC path [V]:
- The worker server sets `WriteTimeout: 30s` (`worker/rpc.go:602-611`).
- The client's non-streaming calls use `Timeout: 30s` and read at most 1 MiB of response (`rpc_client.go:355,364`).
- **Consequence [I]:** remote exec longer than 30 s, remote file reads over 1 MiB, and remote spawns that pull a slow image will fail.
- The advertised `rpc_url` is the literal `--listen` address (`worker/runtime.go:119-131`), so `0.0.0.0:7430` is advertised as-is. There is no advertise-URL flag.
- Placement picks the least-loaded online worker that supports the provider. There are no selectors or labels (`orchestrator/scheduler.go:26-80`).

**Auth** [V]. Options are an API key, an admin key, OIDC JWT (RS256 or ES256/384/512, via JWKS), the roles viewer/api-operator/admin/tenant_admin, and HMAC-signed worker tokens with rotation and revocation (`README.md:637-662,709-735`).

The defaults are unsafe for a laptop [V]:
- `server.host: 0.0.0.0`, `cors_allowed_origins: ["*"]` and `auth.enabled: false` (`config.go:211,214,297`).
- The server listens on TCP only, through `ListenAndServe` (`internal/api/server.go:302-310`).
- Combined with the WebSocket's `OriginPatterns: "*"`, any web page the user visits could drive the API [I].

**Policy** [V]. Tenant allow/deny globs for `image`, `provider` and `network_mode` (`middleware/policy.go:50-57`). Gaps:
- `SpawnRequest` has no `network_mode` field (`types.go:34-44`), so the network rule never fires [V].
- The `/templates/{name}/spawn` route skips `PolicyEnforcer` entirely (`routes/templates.go:170-199` versus `routes/sandboxes.go:54-58`) [V].
- Egress is set globally per provider: Docker `network_mode` defaults to `bridge` (`config.go:257`). There is no per-sandbox allowlist and no secret injection [V].

**Resource limits** [V]. Memory and vCPU per spawn, Docker `pids_limit` 256, `cap_drop ALL`, optional `read_only_rootfs`, and a seccomp profile path. There is no `no-new-privileges` option (`docker.go:250-263,873-878`). The container user defaults to the image's own user (`config.go:263`), which is usually root.

### 1.6 Persistence
- **Store** [V]. SQLite (modernc, pure Go, `SetMaxOpenConns(1)`) or Postgres (pgx) (`internal/store/sqlite.go:18-24`). It holds sandboxes, templates, quotas, tenants, policies, workers, leases and audit records.
- **Exec logs** [V]. The full stdout and stderr of every sync exec are inserted into `exec_logs` (`sqlite.go:605-611`).
- **No volumes** [V]. There is no volume table, mount or bind anywhere; persistent volumes are on the roadmap (`README.md:1025`).
- **Restarts** [V]. `Reconcile` re-adopts Docker containers through labels (`docker.go:702`). Firecracker keeps its VMs only in an in-memory map and has no `ListRuntimeSandboxes`. After a stacyvm restart, those records are marked destroyed (`manager.go:176-266`), and the VM processes and directories are orphaned [I].

### 1.7 What `desktop/` is
It is a separate Wails v2.12.0 module (`desktop/go.mod`). It shows the dashboard and runs the API daemon in-process, by importing `internal/api`, `internal/orchestrator`, `internal/providers` and others from inside the repo tree (`desktop/daemon.go:10-16`) [V].

**There is no GUI desktop, VNC or noVNC inside sandboxes.** A grep for vnc, xvfb, x11 and wayland finds nothing [V].

### 1.8 Maturity by provider

| Provider | Status per `docs/public-support-matrix.md:102-111` [V] | What the code says [V] |
|---|---|---|
| mock | Supported, dev only | Runs commands on the host with host env, no isolation (`mock.go:84-108`) |
| Docker/runc | Supported | CI runs a Docker runtime certification. Unit conformance needs `STACYVM_DOCKER_INTEGRATION=1`. On timeout the Go call returns, but the exec'd process is not killed (`docker.go:289-350`) [I] |
| Docker + gVisor/Kata | Host-certified | Only a `runtime` string; not exercised in CI |
| Firecracker | Host-certified | CI runs unit tests only; conformance needs `/dev/kvm` plus environment variables (`firecracker_test.go:116-140`). **No network interface is configured. A repo-wide grep finds no `network-interfaces`, jailer or `/entropy`** (the docs promise networking, `docs/deployment.md:197`). Rootfs is mounted read-write (`firecracker.go:517`) despite README's "read-only rootfs". Production guidance says to start Firecracker through `jailer` [V, prod-host-setup] |
| PRoot | Experimental | ptrace-based, not an isolation boundary |
| E2B | Preview | 5 of 8 file operations return "not supported" (`e2b.go:203-220`) |
| Custom HTTP | Preview | Tested against an in-process fake |
| Multi-worker, OIDC, tenants | Preview | CI smoke tests run remote workers with the mock provider (`scripts/smoke-remote-worker.sh:144,297`) |

The codebase has 50 test files and 406 `Test*` functions [V]. `deprecated/` holds only the old Vite web UI [V].

---

## 2. Can it be used as a Go library?

**Today it is server-only for any external module** [V]. All engine code is under `internal/`. Go's internal rule blocks `github.com/kzark/gwen` from importing it. The only public packages are `tui`, `web` and `docs`. The `tui/client.go` REST client is unexported (`apiClient`), and `cmd/stacyvm/http_client.go` holds a second private copy. **There is no Go SDK** [V].

**Paths that would work** [I]:
1. **Add public facade packages inside the stacyvm tree.** The `desktop/` module proves code under the stacyvm path may import `internal/`. The owner controls the repo, so this is S.
2. **Export the provider layer.** `internal/providers` imports only `agentproto`, the Docker client and zerolog [V]. It is a clean seam that can be lifted out without pulling in the orchestrator, viper, pgx or cobra.

Embedding the full server would pull the Docker client, pgx, viper and swaggo into gwend. It would also give gwend a second SQLite database and stacyvm's goroutines [I].

---

## 3. Fit for Gwen's needs

| Need | Today | Verdict [I] |
|---|---|---|
| (a) Local sandbox for shell and program runs on the laptop | Docker needs the root-equivalent `docker.sock`. Nothing can bind-mount a host folder. PRoot and mock do not isolate. Firecracker has no network and needs KVM plus a privileged rootfs build | **Poor.** Running `bwrap` directly is better for "run in my repo" (millisecond start, direct binds, unprivileged). stacyvm wins only for "different image or untrusted code" |
| (b) Cloud workers on a tailnet for heavy or long tasks | Single-node `serve` on a cloud VM works over TCP plus an API key. Host `tailscaled` needs no code change. Cluster mode is held back by the 30 s / 1 MiB RPC limits, the `rpc_url` advertising and missing placement selectors. The agent's 5-minute cap affects Firecracker | **Usable as one independent cloud node. Do not use cluster mode yet** |
| (c) Moving a laptop folder in and back | Per-file, text-only JSON. No tar, no diff, files held in memory | **Missing** |
| (d) Preview or code-server for the phone | Docker only, port 3000 only, unauthenticated Host routing through Traefik. `*.localhost` does not resolve on a phone | **Weak.** Needs configurable ports, an authenticating gateway and a tailnet listener |
| (e) Headless GUI apps for computer use | Nothing | **Missing.** Buildable as an image plus endpoints |
| (f) Per-sandbox egress allowlist and secret injection | Schema fields exist (`AllowedHosts`, `Secrets`) but are unwired. Network mode is global | **Missing** |
| (g) Snapshot and restore for durable long tasks | Golden per-image snapshots only. Firecracker VMs are lost on restart. Docker containers are re-adopted. TTL is capped at `24h` by default | **Missing.** The practical durability path is Docker plus volumes plus resumable job state |

---

## 4. Gaps and concrete modifications

| # | Change | Why | Files | Effort |
|---|---|---|---|---|
| 1 | **Safe local defaults**: bind 127.0.0.1 or a Unix socket, auth on, explicit CORS, WebSocket origin check, non-root container user, `no-new-privileges` | Defaults expose host-adjacent exec | `internal/config/config.go:211-214,263,297`, `internal/api/server.go:302-320` (`Handler()` is already exported, `:312`), `routes/sandboxes.go:696`, `providers/docker.go:873-878` | S |
| 2 | **Public Go surface**: a `pkg/client` typed client with WebSocket/NDJSON streaming and archive upload, plus `pkg/sandbox` re-exporting the provider interface and implementations | Lets gwend use stacyvm without REST glue or `internal/` | new `pkg/…`; replace `tui/client.go`, `cmd/stacyvm/http_client.go` | S–M |
| 3 | **Long-run exec correctness**: remove the agent's 5-minute cap; add a write mutex and a flush per frame; add `Done`/`ExitCode` to `StreamChunk`; set per-stream write deadlines through `http.ResponseController`; kill Docker exec processes on cancel (pid file plus `kill`); give each Firecracker stream its own vsock connection; add a **detached jobs API** (start, then attach or poll, surviving client disconnect, with bounded log ring and no full-stdout DB rows) | Gwen's long tasks and phone-driven tasks must outlive HTTP requests | `cmd/stacyvm-agent/main.go:161,203,256,302-330`, `providers/provider.go:38-41`, `providers/firecracker.go:641-705,1074-1084`, `providers/docker.go:289-400`, `api/routes/sandboxes.go:357-377`, `api/server.go:155-160`, `store/sqlite.go:605-611` | M |
| 4 | **Folder in and out**: tar stream `PUT/GET /sandboxes/{id}/archive?path=`; binary-safe file API; `GET /manifest` returning `{path,size,mtime,sha256}` for diffing; a git-patch export so changes come back as a reviewable diff for Gwen's approval and outbox | Covers laptop to cloud to phone to laptop | `orchestrator/types.go:63-67`, `routes/sandboxes.go:394-446`, `agentproto/protocol.go` (new `archive_put/archive_get`, chunked), `providers/docker.go` (`CopyToContainer`/`CopyFromContainer` already tar), `worker/rpc_client.go:364` | M |
| 5 | **bwrap "lite" provider**: `SpawnOptions` gains `Mounts []{Host,Guest,RO}` and `Network{none\|proxy}`; exec runs `bwrap --unshare-all --die-with-parent` with read-only `/usr` and Landlock (ABI 4 or later adds TCP bind and connect rules) as fallback | One provider contract for Gwen's local runs; replaces harness v1's separate `internal/sandbox` builder | new `internal/providers/bwrap.go` (start from `mock.go`/`proot.go:172-196`), `provider.go:16-22`, `cmd_serve.go:75-170`, `config.go`, `provider_conformance_test.go` | M |
| 6 | **Egress proxy and secrets**: an HTTP CONNECT and SOCKS5 proxy per sandbox with domain allowlists; "credential routes" (sandbox calls `http://gwen/anthropic/...`, the proxy adds the key and goes upstream over HTTPS, so no TLS interception is needed); wire `Template.AllowedHosts/Secrets/Env/Setup`; fix the policy `network_mode` check and the template-route bypass. Plumbing per provider: bwrap with no netns and a bind-mounted proxy socket (the pattern `sandbox-runtime` uses); Docker on an `internal` network with only the proxy reachable; Firecracker with no NIC, where the agent forwards guest `127.0.0.1:3128` over vsock to host `v.sock_3128` | Gwen's threat model needs network rules the kernel enforces | new `internal/egress/`, `orchestrator/templates.go:105-115`, `middleware/policy.go:50-57`, `routes/templates.go:170-199`, `providers/*` | L |
| 7 | **Host-function bridge**: a socket inside every sandbox (`/run/gwen/bridge.sock`) carrying `call(capability, json)` to a registered `BridgeHandler`, which is Gwen's gateway, policy, approvals and ledger. Sandbox identity comes from the channel itself (a per-sandbox host UDS; Firecracker's guest-initiated vsock lands on `v.sock_<port>` [V]), so it cannot be forged | This is how "program files" call capabilities without holding secrets or tokens in context | `provider.go` (`SpawnOptions.Bridge`), `providers/bwrap.go`, `firecracker.go` (listen on `v.sock_1025`), `docker.go` (bind mount, needs #9), small in-sandbox `gwen` CLI | M |
| 8 | **Preview gateway**: configurable ports; a built-in Go reverse proxy instead of Traefik labels; `tsnet.ListenTLS` on the tailnet with `LocalClient().WhoIs` authorization [V, tsnet]; optional `Ephemeral` tagged tsnet node per long-lived share [V]; code-server template (MIT; minimum 1 GB RAM and 2 vCPUs [V]) | Phone access to previews and code review | `providers/docker.go:226-232`, new `internal/preview/`, `orchestrator/manager.go:2429-2448`, SDK `getPreviewUrl` | M |
| 9 | **Volumes**: named volumes in the store, `SpawnRequest.Volumes`; Docker volumes or binds, Firecracker extra virtio-block drives (relative paths for snapshots), bwrap binds | Durable workspaces between sandboxes | `store/store.go`, `migrations.go`, `postgres_migrations.go`, `types.go:34-44`, providers | M |
| 10 | **GUI desktop image plus computer-use endpoints**: Xvfb with x11vnc and noVNC (the stack Anthropic's demo uses, "X11 + VNC", ports 5900/6080 [V]) or sway-headless with wayvnc (wlroots only, "Gnome, KDE and Weston are not supported" [V]); add `screenshot` and `input` (xdotool) endpoints | Run LibreOffice, Chromium and similar in a sandbox. This cannot drive apps on the owner's real KDE desktop or Spotify; that belongs to a host-side app bridge [I] | `images/desktop/`, `agentproto/protocol.go`, `routes/sandboxes.go` | M |
| 11 | **Firecracker hardening**: vsock egress (from #6) instead of tap; jailer; a supported guest kernel (6.1 or 6.18); `/entropy` virtio-rng (Firecracker ≥1.4.0 [V]) plus post-restore reseed; persist VM metadata, `Setpgid`, and `ListRuntimeSandboxes` for re-adoption; user snapshots (pause, `/snapshot/create`, copy rootfs) | Makes microVMs usable for cloud long-runs | `providers/firecracker.go` (whole file), `rootfs_builder.go`, `cmd/stacyvm-agent/main.go:57-113` | L |
| 12 | **GPU**: `HostConfig.DeviceRequests` (`--gpus`, needs the NVIDIA Container Toolkit [V]). The Firecracker changelog lists no VFIO or GPU passthrough [V] | Heavy ML tasks on a GPU cloud node | `providers/docker.go:250-263`, template `gpu` | S |
| 13 | **Warm pools and spawn concurrency**: call `Configure` from `Template.PoolSize` and `Acquire` on template spawn; narrow `admissionMu` to bookkeeping only | Instant program-file invocation | `orchestrator/pool.go`, `routes/templates.go:185`, `manager.go:478-483` | S |
| 14 | **Worker over tsnet** (only if cluster mode is wanted later): `--advertise-url`, a tsnet listener, lift the 30 s / 1 MiB limits, placement labels | Multi-node later | `worker/runtime.go:119-131`, `worker/rpc.go:602-611`, `rpc_client.go:355,364`, `orchestrator/scheduler.go:26-80` | M |

---

## 5. Recommendation

### Integration shape: provider layer in-process, full server as a service, nothing remote-only [I]

**On the laptop, use stacyvm as a library at the provider layer.**
- Export providers through change #2 and add the bwrap provider (#5).
- gwend imports `pkg/sandbox` and runs every local command through it, behind Gwen's permission kernel. The kernel decides; the provider enforces mounts, network=proxy and the bridge.
- This replaces harness v1's own bwrap builder, so the work is not written twice.
- Only unprivileged providers run in-process: bwrap now, Landlock as fallback.

**On the laptop, Docker or gVisor sandboxes (optional) run as a sidecar.**
- Run `stacyvm serve` as a separate systemd service on a Unix socket with auth (#1).
- gwend talks to it through `pkg/client`.
- This keeps `docker.sock`'s root-equivalent power and any child VM processes out of gwend. It also means restarting gwend while developing it does not kill sandboxes.

**In the cloud, run one independent single-node `stacyvm serve`.**
- SQLite store, Docker with gVisor, Firecracker later on a KVM host.
- Join it to the tailnet through tsnet (#8). It is always on, so phone review works while the laptop sleeps.
- gwend keeps a registry of execution targets: `local-bwrap`, `local-docker` and `cloud`. Gwen's kernel picks the target per task. Federation happens in Gwen, not in stacyvm's enterprise cluster mode.
- Leave OIDC, tenants, Postgres and worker RPC dormant for now.

**Do not embed the whole server in gwend.** It would need `internal/` workarounds, bring heavy dependencies and a second database, and put root-equivalent privileges in gwend. Firecracker VMs would die with gwend. **Do not go remote-only either**: local runs need host folders, low latency and offline operation.

**The owner's example flow with these pieces.** The owner says on the phone: "move this repo to the cloud and let me review it".
1. gwend tars the folder, after the kernel strips secrets per policy.
2. It calls `PUT /archive` on the cloud node and spawns the `code-server` template with a workspace volume.
3. It sends a tailnet HTTPS link by ntfy; the preview gateway authorizes the phone with tsnet WhoIs.
4. Edits come back as a git patch through `GET /archive` or the manifest diff.
5. The patch goes to Gwen's approval and outbox before it is applied to the laptop folder.

**How this maps onto "program files".** A program file's manifest (code, entrypoint, declared hosts, secrets, mounts and capabilities) lines up one-to-one with a stacyvm Template whose schema fields already exist but are unwired (`templates.go:13-34`). Add the bridge (#7) for capability calls and warm pools (#13) for instant start. Then an agent invokes a program file as `exec entrypoint '{json args}'` and gets a compact JSON result back, which adds almost nothing to context and needs no MCP server or client pair [I].

### stacyvm changes, in order
1. Safe local defaults, a Unix socket listener and auth (#1). **S**
2. Public `pkg/client` and `pkg/sandbox` (#2). **S–M**
3. Long-run exec fixes and the detached jobs API (#3). **M**
4. bwrap provider with mounts and `network=none|proxy` (#5). **M**
5. Archive in and out, binary-safe files, manifest and patch export (#4). **M**
6. Egress proxy, credential routes, wiring of template fields, policy fixes (#6). **L**
7. Host-function bridge socket (#7). **M**
8. tsnet preview gateway, configurable ports, code-server template (#8). **M**
9. Volumes (#9). **M**
10. Warm pools and spawn concurrency (#13). **S**
11. Desktop and GUI image with screenshot and input endpoints (#10). **M**
12. GPU through Docker (#12). **S**
13. Firecracker hardening and user snapshots (#11). **L**
14. Worker-over-tsnet cluster mode (#14), only if a second cloud node is ever needed. **M**

---

## Sources
- stacyvm repository, commit `74f342e` (files cited inline): https://github.com/StacyOS/stacyvm
- tsnet package docs (embedding, Listen/ListenTLS/ListenFunnel/WhoIs, Server fields `Ephemeral`, `AdvertiseTags`, `ListenService`): https://pkg.go.dev/tailscale.com/tsnet
- Firecracker, randomness for snapshot clones: https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/random-for-clones.md
- Firecracker, production host setup (use the jailer): https://github.com/firecracker-microvm/firecracker/blob/main/docs/prod-host-setup.md
- Firecracker, guest kernel policy: https://github.com/firecracker-microvm/firecracker/blob/main/docs/kernel-policy.md
- Firecracker, vsock host and guest connection model: https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md
- Firecracker changelog (PCI in 1.13.0, virtio-rng in 1.4.0, no VFIO entries): https://github.com/firecracker-microvm/firecracker/blob/main/CHANGELOG.md
- Anthropic sandbox-runtime (bwrap with no netns, host proxies over Unix sockets, domain allowlists, Apache-2.0): https://github.com/anthropic-experimental/sandbox-runtime
- Landlock ABI versions: https://docs.kernel.org/userspace-api/landlock.html
- Anthropic computer-use demo (X11 + VNC, ports 5900/6080/8080/8501, isolation advice): https://raw.githubusercontent.com/anthropics/claude-quickstarts/main/computer-use-demo/README.md
- wayvnc (wlroots only, headless sessions, KDE not supported): https://github.com/any1/wayvnc
- code-server (MIT, 1 GB RAM and 2 vCPUs minimum): https://github.com/coder/code-server
- Docker GPU access (`--gpus`, NVIDIA Container Toolkit): https://docs.docker.com/engine/containers/gpu/
- modernc.org/sqlite driver docs: https://pkg.go.dev/modernc.org/sqlite