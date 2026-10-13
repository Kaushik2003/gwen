# Gwen harness v2: programs, context, memory, fabric, reach

Status: proposal for review, v2.1. Written 2026-10-12. Replaces v1 (kept at `archive/harness-v1.md`) and the harness
parts of `plan.md` (3.7, 3.8, 3.11 email and coding, W19, W22, W23, later packages).

The research behind every decision is in `research/` (nine reports, each with sources). This file keeps the decisions,
the numbers they rest on, and the build order. Before delivery, 107 factual claims in it were checked against their
sources (81 held as written; 26 were corrected here), and two independent reviews (ambition, engineering) were folded in.

Contents: [0 What changed](#0-what-changed-since-v1) · [1 The idea](#1-the-idea-in-one-page) ·
[2 Programs](#2-programs-executable-know-how-instead-of-skill-files) · [3 Context engine](#3-the-context-engine) ·
[4 Memory](#4-memory-gwen-owns-the-truth-mem0-and-supermemory-are-lenses) ·
[5 Fabric](#5-fabric-one-gwen-across-laptop-hub-cloud-and-phone) · [6 Reach](#6-reach-driving-any-app) ·
[7 Kernel](#7-the-permission-kernel) · [8 Self-evolution](#8-self-evolution-explore-compile-replay) ·
[9 Data model](#9-data-model-v2) · [10 Roadmap](#10-roadmap) · [11 Decisions for you](#11-decisions-only-you-can-make) ·
[12 Risks](#12-risks-and-what-is-still-unverified)

---

## 0. What changed since v1

| Your ask | v2 answer | Section |
|---|---|---|
| Skills are "a file of tokens" that only add context; we need genuine innovation for long agentic runs | **Programs** replace skill files: typed, capability-gated code that loads straight into `gwend` and runs in-process. The model sees one line per program and gets back small values or **handles**, never bulk data. Programs can call bounded sub-agents for the steps that need judgement, each in a fresh small context, so procedure lives in code and long runs stay short | 2, 2.9 |
| More direct than MCP: no server and client just to run something | Programs run inside `gwend` in a WebAssembly sandbox. No process, no protocol. MCP servers, CLIs and app bridges are wrapped as host functions, so programs see one interface. Programs can also be resident services, phone views, or providers of new capabilities | 2.2, 2.5, 2.11 |
| The execution layer needs local **and** cloud, across devices | **Fabric**: every Gwen daemon joins your tailnet itself (`tsnet`), knows who is calling (`WhoIs`), and hands work to the right node. Code moves as git bundles | 5 |
| Phone → laptop folder → cloud → review on the phone | End-to-end flow with a phone web app on the tailnet, passkey approvals bound to the exact change, diff review, a file browser, and VS Code in the browser | 5.6, 5.7 |
| "Adding the server to Tailscale and giving access" | Gwen enrols servers and exposes services to your devices herself, with single-use keys; policy changes stay yours | 5.9 |
| Use stacyvm and modify it | stacyvm becomes the sandbox layer: its provider interface in-process on the laptop (with a new bubblewrap provider), and a full stacyvm node in the cloud. 14 concrete changes, ordered | 5.8 |
| Talk to any app: VS Code, Word, Excel, Spotify, anything | **Reach**: an adapter ladder (file, service API, app bridge, Electron/Chromium DOM, desktop IPC, accessibility tree, vision). Adapters are programs; slow vision runs are compiled into fast deterministic adapters | 6 |
| Memory: build on Mem0 and Supermemory, hybrid, research where both fail | Gwen keeps the truth (verbatim episodes plus time-aware claims). Mem0 and Supermemory run as replaceable **lanes** for what each does best, and each lane must earn its place on Gwen's own benchmark | 4 |
| "Context is the biggest thing" | A **context engine** with two modes: chat keeps an append-only, cached transcript; long runs render each step from a bounded state object and fold old steps in batches, with measured gates on context growth and cache hits | 3 |
| Detailed task definitions | **Briefs** (goal, acceptance, scope, autonomy, budget, verification) carry over from v1; a brief template is now a program plus a default scope | 1.1 |
| Coding: discuss, exact file paths, which folder is which project | **Places** and pinned paths carry over; pinned paths compile into the run's capability ceiling, and resolve per node, so a brief written on your phone runs on the laptop or the cloud | 1.1, 5.7 |
| "Go through my last five emails, discuss, I send or Gwen sends" | Worked example as a program with per-email sub-agents, drafts as handles, one passkey per send batch. Ships in Wave B, not last | 6.6 |
| "Do anything, with an appropriate permission system" | v1's kernel stays as the floor, with an updated hard floor for desktop control and cross-device approvals, plus a table of exactly what asks for each of your examples | 7 |
| Self-evolving | Everything above the kernel grows on its own: programs, adapters, providers, memory, and the agent loop itself, which is a program evaluated by replaying past runs | 8 |

---

## 1. The idea in one page

**Procedure lives in code; the model is spent only on judgement, and each judgement gets a fresh, small context.**

The first time Gwen does something new (make a focus playlist, triage your inbox, move a repo to the cloud for review)
she works agentically: she reasons, calls tools, maybe drives an app by sight. When that run succeeds, she compiles it
into a **program**: typed code with a declared capability ceiling and tests recorded from the real run. The steps that
always need judgement (what to say to this sender, how to fix this failing test) become calls to bounded sub-agents
inside the program. The next time, the program runs inside `gwend`, calls a model only at those points, each with only
the inputs it needs, and returns one line to her context. If it breaks, she falls back to exploring and repairs it.

Five parts make this work, all on top of a permission kernel:

```text
 you ─ phone web app · dashboard · voice · CLI · KDE panel · VS Code panel
          │
 ┌────────┴──────────────────────────── one Gwen, many nodes (tailnet) ─────────────────────────┐
 │  CONTEXT ENGINE  chat: append-only cached transcript · runs: state object + folded steps     │
 │       │                                                                                       │
 │  AGENT LOOP  (itself a program)  → find · describe · run(code) · call                         │
 │       │                                                                                       │
 │  PROGRAMS  functions · services · views · providers; sub-agents at judgement points          │
 │       │  gwen.* host functions                                                               │
 │  KERNEL  classify → policy → approvals → enforce (sandbox, egress, secrets, outbox) → ledger │
 │       │                                                                                       │
 │  CAPABILITIES  places · git · shell · http · apps (ladder) · MCP · memory · sandboxes · llm  │
 │       │                                                                                       │
 │  MEMORY  episodes + bi-temporal claims (truth) · lanes: FTS, vectors, Mem0, Supermemory     │
 │  FABRIC  laptop (apps, files) · hub (brain, approvals) · cloud (stacyvm) · phone (client)   │
 └───────────────────────────────────────────────────────────────────────────────────────────────┘
```

Three rules hold everywhere:

1. **The model works with references, not data.** Programs, memory and tools return values, one-line summaries and
   handles. Bulk data stays in the runtime until something needs it.
2. **Every effect passes the kernel of the node where it happens**, and that kernel verifies the evidence itself
   (approvals, signatures, scope tokens) instead of trusting what the caller claims.
3. **Everything Gwen learns is code or data with provenance, tests and a version**, never untracked prompt text.

### 1.1 Carried over from v1, and what v2 changes

These v1 sections stay normative; read them in `archive/harness-v1.md`.

| v1 | Still true | v2 change |
|---|---|---|
| §5 Kernel: action model (verbs, resources, reach `local`/`account`/`world`, undo), rules, default matrix, approvals, enforcement, outbox, ledger, kill switch | all of it | approvals need passkeys for anything beyond local and reversible; new hard floor (7.1); cross-node verification (5.6) |
| §6 **Briefs**: goal, why, context, deliverables, acceptance, places, scope, autonomy (observe, propose, act, delegate), budget, schedule, check-ins, verification; lifecycle draft → discussing → planned → approved → running → verifying → done | all of it | a **template** is a program plus a default scope and pinned catalog, not a SKILL.md; triggers become **services** (2.11); a brief written on any device runs on whichever node can do it (5.3) |
| §7 **Places**: a folder Gwen knows (path, project, kind, commands, conventions, protected globs, sandbox profile, remote), discovered and confirmed by you; **pinned paths** become the write scope; coding flow discuss → plan → execute in a worktree → check → deliver | all of it | pinned paths compile into the run's ceiling (`fs.write@place:gwen:/internal/kernel/policy/**`); a place has a **place identity** (its git remote), so `place:gwen` resolves to `~/code/gwen` on the laptop and `/workspace/gwen` on `gwen-cloud` |
| §8.4 email review flow | the steps and the taint rule | rewritten as a program with sub-agents (6.6) |
| §10.2 **Claude Code as an engine**: `gwen mcp` serves the kernel's tools, `--permission-prompt-tool` routes every permission to the kernel, strict MCP config, inside the place's sandbox | all of it | `gwen mcp` serves the four meta-tools (2.5); Claude Code is one engine a program's sub-agent can use (2.9) |
| §10.3 models by job | all of it | the agent loop picks per step, and the tuner adjusts it (8.2) |

### 1.2 Words used in this document

| Term | Meaning |
|---|---|
| **brief** | a task definition (1.1); every run has one, chat has an implicit one |
| **place** | a folder Gwen may work in, tied to a project and a git remote (1.1) |
| **program** | a directory of typed code plus a manifest that Gwen can run (2.2) |
| **ceiling** | the manifest's list of host calls a program may make, with argument patterns (2.2) |
| **handle / ref** | a pointer to data kept outside the model's context; handles are short-lived, refs are durable (2.5) |
| **taint** | a label on data from outside your control (email, web, screens), which changes what may happen next (2.10) |
| **kernel** | the part of `gwend` that classifies, decides, enforces and records every effect (7) |
| **hard floor** | the rules no approval or rule can lift (7.1) |
| **outbox** | the queue that holds outgoing actions for an undo window before they leave |
| **lane** | one memory retrieval source (4.3) |
| **runtime tiers R0 to R3** | where program code runs: in-process wasm, wasm module, sandboxed process, remote sandbox (2.3) |
| **trust levels** | how far a memory can be trusted: owner-edit, owner-said, observed, inferred, external (4.4) |
| **brain** | the node that schedules and leases work (5.1) |
| **GwenBench** | Gwen's personal evaluation set (4.6) |
| **W15 to W23** | work packages in `plan.md` section 6 (the user's plan, attached to this project's harness thread) |

---

## 2. Programs: executable know-how instead of skill files

### 2.1 Why, in numbers

| Finding | Source |
|---|---|
| Turning tools into code the agent calls cut one worked example from 150,000 to 2,000 tokens (98.7%), because intermediate results stay in the execution environment | [Anthropic, code execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp) |
| Cloudflare's Code Mode covers 2,500+ API endpoints in about 1,000 tokens, versus about 1.17M as plain MCP tools | [Cloudflare](https://blog.cloudflare.com/code-mode-mcp/) |
| Tool search: 77K to 8.7K tokens, and accuracy rose (Opus 4.5: 79.5% to 88.1%). Selection degrades past 30 to 50 loaded tools | [Anthropic](https://www.anthropic.com/engineering/advanced-tool-use), [docs](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool) |
| Code actions beat JSON or text tool calls by up to 20% success across 17 models | [CodeAct](https://arxiv.org/abs/2402.01030) |
| Skills stored as verified programs beat text skills by 11.3% and cut steps 10.7 to 15.3% (WebArena) | [ASI](https://arxiv.org/abs/2504.06821v2) |
| Skills a model writes for itself gave no benefit on average (−1.3 points against no skills; curated skills gave +16.6) | [SkillsBench](https://arxiv.org/abs/2602.12670v2) |
| Code mode left scores unchanged and cost about 8% more on τ²-bench, where each turn makes one or two sequential calls | [programmatic tool calling docs](https://platform.claude.com/docs/en/agents-and-tools/tool-use/programmatic-tool-calling) |
| Libraries rot unless they are trimmed, merged and gated (TroVE: 79 to 98% smaller toolboxes; ToolLibGen clusters and merges tools with a reviewer) | [TroVE](https://arxiv.org/abs/2401.12869), [ToolLibGen](https://arxiv.org/abs/2510.07768) |
| Nearly 1,200 malicious skills got into a major agent marketplace and stole keys and credentials (ClawHavoc) | [SoK: Agentic Skills](https://arxiv.org/abs/2602.20867) |

What an Agent Skill lacks, even though its scripts already stay out of context: a typed signature, a per-call gate,
control over output size, in-memory composition, tests, and provenance. Programs add exactly those.

### 2.2 What a program is

A directory in `~/.local/share/gwen/programs/` (a git repo), identified by the hash of its contents:

```text
spotify.make_playlist_and_play/
  program.toml       kind, signature, ceiling, effect class, budget, examples, provenance
  main.ts            TypeScript
  lib/*.ts | *.wasm  helpers; wasm for heavy compute (parsers, diffing)
  tests/*.test.ts    replay tests
  tests/fixtures/    recorded host-call traces from the run it was learned from
  NOTES.md           at most 150 tokens of judgement that tests cannot check; loaded only on describe or failure
```

```toml
name    = "spotify.make_playlist_and_play"
version = "1.0.0"
kind    = "function"                          # function | service | view | provider (2.11)
summary = "Pick tracks for a mood, make a private playlist, start playback here"
input   = "{ mood: string, minutes?: int(10..240), seed?: string[], device?: 'laptop'|'any' }"
output  = "{ playlist: { id: string, name: string }, tracks: int, unresolved: int, playing: bool, via: 'mpris'|'webapi' }"
effect  = { reach = "account", undo = "reversible" }
uses = [
  "http.fetch@api.spotify.com GET /v1/search",
  "http.fetch@api.spotify.com POST /v1/me/playlists",
  "http.fetch@api.spotify.com POST /v1/playlists/{id}/items",
  "http.fetch@api.spotify.com PUT /v1/me/player/play",
  "app.mpris.open_uri@spotify", "app.mpris.status@*",
  "llm.json@small", "memory.get@taste.music.*", "kv.*", "clock.*",
]
requires  = { node = ["desktop:wayland"], auth = ["spotify"] }   # secret names; values never enter the program
budget    = { wall_ms = 45000, host_calls = 120, mem_mb = 64, llm_tokens = 6000, return_bytes = 1024 }
placement = { prefer = "laptop" }
[[example]]
args = { mood = "deep focus, no vocals", minutes = 90 }
[provenance]
author   = "gwen"
from_run = "run_01JA…"
status   = "probation"
tests    = "sha256:…"
```

- **The signature is a compact type string**, compiled by a small Go parser into JSON Schema (validation at the call
  boundary), a `types.d.ts` (what `describe` returns) and the catalog line (25 to 40 tokens).
- **`uses` is the ceiling, down to arguments**: function, resource, and an argument pattern (HTTP method and path, a
  path glob inside a place, a D-Bus interface). At run time every host call must match `uses` ∩ the brief's scope before
  the policy looks at it. The kernel reads authority only from the manifest; it never runs code to learn what a program
  may do. Any change to a pattern counts as widening.
- **`effect` is the program's own class** for the default matrix; each host call is still classified on its own.

### 2.3 Runtimes

| Tier | Runtime | For | Isolation |
|---|---|---|---|
| **R0, default** | QuickJS-ng compiled to WebAssembly, run by wazero inside `gwend`, from **Gwen's own build** of [`fastschema/qjs`](https://github.com/fastschema/qjs) (MIT, ES2023, no cgo) | most programs, and the agent's `run(code)` snippets | linear-memory sandbox; the only way out is `gwen.*` |
| **R1** | WebAssembly modules on wazero (Go via `GOOS=wasip1`, Rust) | heavy compute helpers called from R0 | import list = capability list |
| **R2** | Deno subprocess under bubblewrap, with `gwend` as its permission broker (`DENO_PERMISSION_BROKER_PATH`) set to **deny everything** except reading its own package directory; all effects go through the same `gwen.*` API over a bridge socket; or any CLI or Python under bubblewrap with the same bridge | npm or Python ecosystems, LibreOffice UNO, Playwright; programs on probation; imported code | OS sandbox, broker as a fail-closed backstop, egress proxy |
| **R3** | the same R2 package inside a stacyvm sandbox on another node | heavy, long or untrusted work; imported code that needs direct network access | microVM or gVisor |

**Why Gwen builds R0 herself.** The published `qjs` v0.0.6 is not isolated enough as shipped. Reading its source: it
loads QuickJS's `std`, `os` and `bjson` modules as globals (`qjswasm/helpers.c:159-166`, `qjswasm/qjs.c:7-9`), mounts
the daemon's working directory as `/` (`runtime.go:137`, defaulting to `os.Getwd()` in `options.go:224`), and gives
programs the real clock (`runtime.go:143-144`). A program could read `~/.ssh` through `std.loadFile` without touching
the kernel. Gwen's build removes those modules and mounts, feeds the clock and randomness from the invocation journal,
enforces `wall_ms` with a Go context deadline, and runs each call on its own goroutine with `recover`. An **escape and
conformance suite** is a merge gate for the first PR: `std` and `os` are undefined, `import 'qjs:std'` fails, nothing is
visible at `/`, an infinite loop dies within `wall_ms`, a memory bomb returns an error instead of a panic, `Date.now()`
and `Math.random()` replay identically, and 100 concurrent instances leak nothing. The same suite runs against `goja`
(pure-Go JavaScript) and the engine is chosen by the results.

TypeScript is the language because models write it best and both Anthropic's and Cloudflare's code modes expose APIs as
TypeScript. esbuild's Go API bundles `main.ts` and `lib/` from an in-memory file system with `gwen:*` as the only
external; it strips types without checking them, so `deno check` against the generated types runs at promotion, and at
run time the manifest schema is what is enforced. Every program's tests run on R0 and R2 before promotion, and since
both use the same bridge, a program can move between them without a rewrite. Go's `plugin` package, Yaegi, and Node or
Bun are rejected (no unloading and exact toolchain matching; exposes whatever it is handed; their permission models are
not a security boundary).

### 2.4 The host API: `gwen.*`

A program sees one global object. Every function is one kernel call.

```ts
gwen.fs.read|list|search|write|edit({ place, path, … })      gwen.git.status|diff|diffstat|log({ place, … })
gwen.shell.run({ profile, argv })                             gwen.http.fetch({ url, method?, query?, json?, auth? })
gwen.mcp.<server>.<tool>(args)                                gwen.app.<adapter>.<verb>(args)          // section 6
gwen.sandbox.open|put_bundle|exec|get_bundle(…)               gwen.place.snapshot|apply_bundle(…)
gwen.memory.get|recall|note|why(…)                            gwen.llm.json({ tier, prompt, input?: Handle, schema })
gwen.agent({ goal, inputs, uses, returns, budget, engine? })  // 2.9
gwen.prog["name"](args)      gwen.handle.put|peek(…)      gwen.state.get|set(…)      gwen.ask(question)
gwen.tasks|plan|goals|agenda.*      gwen.kv.*      gwen.clock.*      gwen.notify.push      gwen.share.publish
```

Each host call goes through, in order:

1. **Ceiling**: the call must match `uses` ∩ brief scope; a miss is a program bug (`E_UNDECLARED`), logged, never turned
   into a question for you.
2. **Budget**: host calls, wall time, model tokens, bytes.
3. **Journal** (2.6): on replay, the recorded result is returned.
4. **Handles** in arguments are resolved on the host side, with access checks (2.5).
5. **`kernel.Invoke`** with the caller chain (`agent → repo.fix_and_review@1.2 → git.diffstat`), the program's hash and
   status, the brief, and the run's taint (2.10). The capability's classifier builds the action; policy, hard floor,
   outbox and ledger apply.
6. **`ask`** suspends the program for up to 10 minutes; after that the invocation ends `pending` and resumes when you
   decide.
7. **Return**: a result over `return_bytes` becomes a handle.

`gwen.llm.json` lets a program have a small model choose, sort or extract over data that **never enters the main
context** (200 diff hunks, 40 emails, a playlist's candidates), with a schema on the answer.

### 2.5 How the agent uses programs, and how data stays out of context

The model gets four meta-tools instead of hundreds of tool schemas:

| Tool | Does |
|---|---|
| `find(query)` | search over program summaries and signatures (BM25 first, embeddings later); returns catalog lines |
| `describe(names)` | the `.d.ts` slices, examples and NOTES |
| `run(code)` | a TypeScript snippet against `gwen.*` and `gwen.prog.*`; loops, joins and filters happen in the snippet |
| `call(name, args)` | one direct call, for single sequential steps where code mode costs more |

About 10 catalog lines are pinned per brief template; the rest are found on demand. What comes back is the
schema-checked value (at most `return_bytes`), one status line, and at most 1 KB of log. Anything larger becomes a
handle. There are two kinds of pointer:

- **Handles** (`h:<node>:<id>`) point at run data: `{ h, kind, rows|bytes, schema, preview(3), taint }`. Every
  dereference is checked against the run, the caller and the node. A handle stays alive while any unfinished invocation
  references it, then expires; an expired handle returns `E_HANDLE_EXPIRED`. `peek` has a fixed set of operations: rows
  a..b, lines a..b, a JSON pointer, a capped substring search.
- **Refs** (`claim:<id>`, `ep:<id>#<span>`, `msg:<id>`) point at memory and never expire. Every read is checked
  against the kernel's access rules. Citations and chat history use refs, so they never go dead.

**Everything is a capability under `gwen.*`**, so the model and programs see one interface:

- **MCP servers**: the loader starts the server in a sandbox, lists its tools, and registers each as
  `mcp.<server>.<tool>` with a signature generated from its schemas and a cautious default class you confirm. The model
  never sees MCP schemas. MCP survives as plumbing, and as the edge protocol for engines that need it (Claude Code gets
  `gwen mcp` with exactly the four meta-tools).
- **CLIs and Python**: an R2 package with one JSON line in and one out.
- **App adapters** (section 6), **memory** (section 4), **sandboxes on other nodes** (section 5), and **providers**
  (2.11).

Programs call programs. Each callee runs in its own instance with its own ceiling ∩ brief scope, and the loader computes
the **transitive ceiling**, so the approval card shows everything a program could reach through its callees.

### 2.6 Durable invocations

A program can wait hours for your approval or survive a crash because every host call is journaled. The rules:

- **Pinned code.** An invocation records the program's hash (or the snippet's source hash) and the runtime version. A
  resume under a different hash is refused.
- **Strict replay.** Each journal entry stores `(seq, function, canonical args hash, result)`. On resume the program
  re-executes from the start and every journaled call returns its recorded result; none are re-executed. If the program
  issues a different call at a sequence number, the invocation fails with `E_NONDETERMINISM` and nothing further runs.
  Clock and randomness come from the journal. Host promises resolve in issue order; `Promise.race` over host calls is
  rejected.
- **Intent, effect, result.** Before an effect, its intent (and outbox entry) commits in one SQLite transaction. Effects
  on Gwen's own store commit together with their journal result. On resume, an intent with no result is **in doubt**:
  the capability's read-back decides whether it happened, or you do. It is retried automatically only if the capability
  declares an idempotency key (`H(program hash, invocation, seq, args hash)`) that the remote side honours.
- **Fresh preconditions.** Every effect carries a precondition (a row's revision, a git base, an ETag) checked when it
  executes. If the world changed while the program waited, the effect returns `stale` and the program re-plans instead
  of acting on old reads.
- **Same node.** A paused invocation resumes on the node that started it. If that node is gone, the brain starts the
  brief again as a new invocation on a node that can run it. Moving journals between nodes is deferred: it would ship raw
  data across machines, which placement and sensitivity rules forbid.
- **Retention.** Journal results contain real data (emails, file contents), so they follow the same sensitivity and
  forget rules as episodes, with a retention period that starts when the invocation ends.

### 2.7 How programs are born, tested, promoted and retired

States: `draft → candidate → probation → stable → deprecated → archived`, plus `quarantined`.

1. **Trigger**: a `run(code)` snippet succeeded and a similar host-call pattern has appeared at least twice; you say
   "remember how to do this"; an app adapter reached its compile threshold (6.3); or a new MCP server or CLI arrived
   (wrapping generates bindings, not new code).
2. **Spec from the trace**: the recorded host calls become `uses`, with argument patterns as narrow as the trace allows;
   observed inputs and outputs become the signature and examples. **Programs are distilled from runs that worked, never
   imagined.**
3. **Code**: the strong model writes `main.ts` and tests in a scratch place. Making tools needs a strong model; using
   them can be cheap ([LATM](https://arxiv.org/abs/2305.17126)), so local models may call what Claude wrote.
4. **Tests**: replayed traces plus generated edge cases, run on R0 and R2 against fail-closed mocks (an unmocked call
   fails). These are **regression tests, not a security control**: Gwen wrote them, so they prove the code still does
   what it did. The security control is the ceiling, and the kernel checks it independently: it derives the expected
   effects from the recorded fixtures itself, and fuzzes the program's arguments to confirm every call it emits stays
   inside the ceiling. During probation, live calls are compared against what the fixtures showed.
5. **Eval**: the kernel's evaluator (which Gwen cannot edit) replays held-out briefs with and without the program. It
   must not regress, and it records tokens and steps saved. A validation gate produced an 11 times more compact skill
   bank than ungated evolution in [VALVE](https://arxiv.org/abs/2609.32990).
6. **Promotion card**: signature, the ceiling as a plain table, transitive reach, diff against the last version, test
   and eval results, provenance. You approve the ceiling with a passkey; the code is shown. Probation runs on R2, and
   effectful calls ask the first three times.
7. **Two signatures**: the node that built and tested the program signs its hash (proof the tests passed); your passkey
   signs `(name, ceiling hash)` (proof you approved its reach). A later patch with a build signature whose ceiling is
   within your signed ceiling promotes itself with a notice. Anything wider asks you. Nodes refuse packages without both.
8. **Health**: per version, success rate, schema violations, your corrections, tokens returned. Adapters export a
   read-only `canary` that runs daily, because replay tests cannot see upstream drift (Spotify's February 2026 API
   changes broke older playlist code for new Development Mode apps). Past an error threshold a program falls back to its
   previous version or is quarantined, and a repair draft opens. Unused for 90 days: archived. Near-duplicates are
   merged by a reviewer that runs both programs' tests.
9. **Provenance**: a ledger record `{hash, author: gwen | you | import(url@commit), model, from_run, parent, tests,
   eval, signatures}`. Imports start quarantined on R2. Programs learned from tainted runs are flagged and always get
   your review.

### 2.8 What stays prose

The persona, attitudes and her playbook stay prose: they are judgement about how to be, not procedure. Taste ("no
vocals while coding") is memory, which programs query. How to call a program is never prose: that is the signature.
A procedure that needs judgement at every step is still a program: deterministic glue plus `gwen.agent` calls (2.9).

### 2.9 Judgement points: programs call agents

Most real tasks mix mechanical steps with judgement: "fix the failing tests", "suggest a reply to this email". A plain
agent loop does both in one ever-growing context. Gwen splits them:

```ts
gwen.agent({
  goal: "make the failing tests in this worktree pass without changing test files",
  inputs: [worktree, failingTests],                    // handles
  uses: ["fs.edit@sandbox:/workspace/**", "shell.run@profile:test"],   // ⊆ the caller's ceiling
  returns: "{ summary: string, files: string[], tests_passed: bool }",
  budget: { tokens: 200_000, wall_ms: 1_800_000 },
  engine: "claude-code",                                // or "gwen" (own loop), or a model tier
})
```

The sub-agent's context holds only the goal, its typed inputs, and the catalog for its `uses`. It returns a
schema-checked value, its transcript is discarded (kept only in the ledger as a reference), and the caller continues.
However long the overall task is, each judgement starts from a clean context. Recipes are therefore programs too:

- `repo.fix_and_review({ place, goal })` = `place.snapshot` → `sandbox.open@cloud` → `gwen.agent(engine: claude-code)` →
  `git.diffstat` → `share.publish` review page (5.7).
- `mail.triage_recent({ n: 5 })` = list → one small `gwen.agent` per email → five cards (6.6).

Anthropic's own measurements explain the budget: an agent uses about 4 times the tokens of a chat and a multi-agent
system about 15 times ([multi-agent research](https://www.anthropic.com/engineering/multi-agent-research-system)), so
sub-agents are used where isolating context pays for itself, and each returns a small typed result rather than a
digest of its transcript.

### 2.10 Taint is per value

v1 tainted a whole run once untrusted text entered it, which would make every email run ask about everything. v2
tracks taint at the boundaries where data moves:

- **Labels.** Every handle and host result carries a label set (`owner`, `observed`, `external:<source>`,
  `sensitive`).
- **Inside a program instance**, JavaScript cannot track labels per variable, so taint is monotonic: once a tainted
  result enters an instance, every later call from it is marked tainted.
- **Destinations** (recipients, URL hosts, repositories, paths outside the brief's places) from a tainted caller must
  come from a trusted set: the brief, your own input, your contacts, the thread being replied to, or the ceiling's fixed
  patterns. Otherwise the call carries `DestTaint` and always asks, with that fact on the card.
- **The model's context** is tainted only when tainted free text is rendered into it (a `peek`, or a return value with
  tainted string fields). `llm.json` over tainted input returns tainted output, but fields typed as an enum, a bool, a
  bounded integer or a date from a fixed set carry too little information to smuggle instructions, so they do not taint
  the model's context.
- **Plan first, read second.** When the model writes a `run(code)` snippet before any untrusted data is read, the control
  flow is fixed, so injected text cannot change which calls happen. This is the plan and quarantine split of
  [CaMeL](https://arxiv.org/abs/2503.18813).

Policy tests in G1: "a free-text summary of a tainted email in context taints the run"; "an enum urgency from a tainted
email does not"; "a recipient found only in an email body always asks".

### 2.11 Four kinds of program

Your "program files" are skills plus capabilities that load directly into the system. A function you call is only one
kind:

| Kind | What it is | Example | Gate |
|---|---|---|---|
| **function** | called by the agent or another program | `spotify.make_playlist_and_play` | as in 2.7 |
| **service** | resident; declares triggers `on = ["mail.new(label:Recruiters)", "cue.start", "fs.change@place:foo", "cron:0 8 * * 1-5", "tts.speaking"]` and an hourly budget; runs on the node that owns its triggers | duck Spotify while Gwen speaks; draft replies when recruiter mail arrives; the DSA routine | enabling a trigger is a promotion you approve |
| **view** | a schema-rendered card or page served by the hub to the phone and dashboard | the email triage cards, a run's diff review, a playlist card with skip and like | read-only data from its program; actions on it go through the kernel |
| **provider** | an R1 or R2 program that adds host functions (`provides = ["dbus.call@org.kde.*"]`), with a class table per function shipped as data | a new D-Bus app, Bluetooth, a protocol Gwen did not know | the class table is approved at install; the kernel never runs provider code to classify |

Services replace v1's triggers. Providers mean new reach (a new app's D-Bus interface, a new device) goes through the
same pipeline as any program, tested and approved, and Go code is needed only for the kernel and the runtime.

---

## 3. The context engine

The model's context is assembled with a budget per tier instead of accumulated. Gwen today resends up to 150 tasks,
every active goal, 7 days of plans and 24 messages as one JSON message with no prompt caching
(`internal/api/assistant.go:215-350`, `internal/llm/providers.go:25-30`).

Two facts shape the design. Cached input is far cheaper (cache reads cost 0.1 times the base input price on most Claude
models and less on the newest; writing to the cache costs 1.25 times for the 5-minute cache and 2 times for the 1-hour
cache; [prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)). And any edit to an
earlier part of a request restarts the cache from that point. Manus calls cache hit rate the single most important
metric for a production agent, because it drives both latency and cost, and reports an average of about 100 input
tokens per output token ([Manus](https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus)).
So the engine has two modes.

### 3.1 Chat mode: append-only and cached

| Part | Contents | Budget | Cache |
|---|---|---|---|
| **P0 prefix** | persona, profile card (compiled from memory claims), capability summary, the chat catalog | profile card ≤ 800 tokens | breakpoint with the 1-hour TTL; changes only at a rebase |
| **Transcript** | turns, tool calls and results, and appended notes: memory found for a message, observations, Gwen's own attitude or self-note changes | grows | append-only; automatic tail caching |
| **Rebase** | when the transcript passes a threshold, or daily: server compaction or Gwen's own, producing an **observation log** (dated, prioritised notes written by an Observer and condensed by a Reflector) at the start of a new segment | observation log ≤ 3k | the only point where earlier content changes |

Rules: nothing earlier in the transcript is ever edited. Memory found for a message is appended after it and stays.
Old tool results are removed by context editing on the server or at a rebase, never by deleting them client-side. Her
attitude and self-note changes are appended as notes and folded into P0 at the next rebase (today they sit in the
system prompt and change every turn, `internal/llm/persona.go:72-102`). The cache TTL is chosen per thread from the
gaps between your replies. A test in G2 diffs consecutive requests and asserts the prefix is identical.

The observation log follows [Mastra's observational memory](https://mastra.ai/research/observational-memory) (94.87% on
LongMemEval-S with gpt-5-mini; 84.23% with gpt-4o).

### 3.2 Run mode: bounded state, folded steps

A long run (a coding brief, a research task, an hour of app driving) does not keep a transcript. Each step renders:

```text
[run prefix: brief · catalog slice · state schema]   cached
[state object]                                        ≤ 4k tokens, validated
[steps since the last fold]                           append-only
```

The **state object** has a fixed schema, is written only through `gwen.state.set`, and is validated by the compiler:

```ts
{ todo: [{ id, text, status }], facts: [{ k, v, source: Ref|Handle }], artifacts: Handle[],
  open_questions: string[], decisions: [{ what, why }], failures: [{ tried, lesson }] }
```

Every K steps, or when the step window passes a size limit, the window is **folded** into the state and cleared; K is
chosen so the one cache miss per fold is spread over many steps. **Rewind**: when a branch of exploration fails, its
steps are dropped and replaced by one `failures[]` line, so dead ends never stay in context. On Claude, server context
editing does the same job inside a step window: Anthropic measured a 39% gain on an agentic search eval from the memory
tool plus context editing (29% from context editing alone), and 84% fewer tokens in a 100-turn web search eval
([context management](https://claude.com/resources/articles/context-management)). `gwend` implements the same
semantics for local and OpenAI-compatible models.

### 3.3 Policy per model

| Model profile | History | Memory for each message | Observation log |
|---|---|---|---|
| frontier model, under about 100k tokens of relevant history | recent sessions verbatim | ≤ 1.2k tokens, only for facts older than the window | off |
| frontier model, longer history | observation log plus recent turns | ≤ 1.2k | on |
| local 8B to 20B model | observation log plus a short window | ≤ 2k, heavier use of claims | on |

Verbatim history wins when it fits: GPT-5-mini with the full context scored 92.85 on LoCoMo against 57.68 for flat Mem0
facts ([arXiv 2603.04814](https://arxiv.org/html/2603.04814)). Small models need memory more: Hindsight lifted a 20B
model from 39.0 to 83.6 on LongMemEval ([arXiv 2512.12818](https://arxiv.org/pdf/2512.12818)).

### 3.4 Gates

Every turn records tokens per part, cache hits, handles created and read, and time to first token. The eval suite gates
on: context at step 200 of a run at most 1.3 times context at step 10; chat-mode cache hit rate at least 80% on the
prefix; tokens per completed brief; and accuracy (8.4).

---

## 4. Memory: Gwen owns the truth, Mem0 and Supermemory are lenses

Full design: `research/memory-design.md`. Evidence: `research/memory-mem0-supermemory.md`, `research/memory-field.md`.

### 4.1 Where Mem0 and Supermemory fail

| | Mem0 (OSS 2.x) | Supermemory (local server) |
|---|---|---|
| Engine | Apache-2.0, fully open | **closed binary**, free "lite" license capped at **10,000 documents** |
| Updates | v2 is **ADD-only**: "I work at A" and "I now work at B" coexist, and ranking has no recency term ([#4956](https://github.com/mem0ai/mem0/issues/4956)); supersede and merge are paid Platform features | `updates`/`extends`/`derives` relations and `isLatest` |
| Graph | **removed from OSS in v2** (Platform only) | built in |
| Time | OSS: an expiry date only | document and event dates, `forgetAfter` |
| Provenance | no link from a memory to its source message | source chunk kept as evidence |
| Independent results | MemoryAgentBench: 21.1 overall, 2.0 on multi-hop selective forgetting; HaluMem: update correctness 1.45% on long histories | HaluMem: the only system whose extraction recall held up on long histories (53%, against 3% for Mem0), but 29.7% extraction accuracy and the **worst false-memory resistance**: it ignored only about 37% of distractor content |
| Benchmarks they publish | disputed: Zep's rebuttal shows Mem0's own full-context baseline beat it, and the LoCoMo answer key is about 6.4% wrong | the 97 to 99% LongMemEval figures come from aggregated or multi-try setups that are not in its main engine; its other reported LongMemEval-S results are about 85% |
| Operations | telemetry on by default; the local MCP server (OpenMemory) is sunset, and a cross-user access report against it (CVE-2026-59705) was closed with that answer | binds all interfaces ([#1441](https://github.com/supermemoryai/supermemory/issues/1441)); data dir defaults to the working directory; non-ASCII-only memories silently not indexed ([#1740](https://github.com/supermemoryai/supermemory/issues/1740)) |
| Both | no Go SDK, no structured state (tasks, plans), no per-memory permissions, no procedural memory worth the name, and an expensive write path (HaluMem dialogue ingestion: Mem0 2,768 min against Supermemory 273 min on the medium set; Mem0 692 against Supermemory 1,673 on the long set) | |

Every memory system tested still fails **contradictions and selective forgetting** (at most 7% on multi-hop
FactConsolidation, [MemoryAgentBench](https://arxiv.org/html/2507.05257v2); the weakest ability on
[BEAM](https://arxiv.org/html/2510.27246v1)) and **memory for action across sessions** (0.12 to 0.23 success,
[MemoryArena](https://arxiv.org/html/2602.16313v1)). That is where Gwen must do her own work.

### 4.2 The design in one picture

```text
 message · tool result · file · task/calendar event
        │ online, no model call
        ▼
 EPISODE LOG (verbatim, append-only, synced) ──► FTS5 + sqlite-vec in index.db (derived, local, rebuildable)
        │ batched at idle
        ▼
 extractor (local model) ┐
 Mem0 in shadow mode ────┼─► VERIFIER: cited span exists · entailment · speaker rule · trust level
 Supermemory profile ────┘        │
                                  ├─► CLAIMS (keyed, bi-temporal, provenance)  ──► projections: vectors, Mem0, memory files
                                  └─► CONFLICTS / QUARANTINE ──► "Gwen isn't sure" cards
 RESOURCES (PDFs, transcripts, pages) ──► Supermemory local (documents lane)

 READ: router ─► lanes in parallel with deadlines: claims-by-key · FTS · vectors · entity hops · time window ·
                 SQL over tasks/plans/time · programs · Mem0 · Supermemory
       ─► weighted RRF ─► as_of / supersession / trust / sensitivity filters ─► packer ─► memory note for this message
 SLEEP WORKER: dedupe · contradiction scan · profile card · reflection · program drafts · reconcile
```

### 4.3 Who does what

| Need | Lane |
|---|---|
| Truth: what is true now and when it was true | **Gwen**: keyed claims with `valid_from/valid_to` (world time), `recorded_at/superseded_at` (when she learned it), `occurred_at/mentioned_at`, and a cited source span |
| Fast fuzzy recall over chat facts | Gwen's FTS5 and sqlite-vec (pure Go: `modernc.org/sqlite/vec` bundles sqlite-vec without cgo; both are proposed, not used today) |
| A second extractor to raise recall | **Mem0** in shadow mode; its extra findings pass the same verifier. This is Mem0's real value here. Its recall lane over verified claims indexes what sqlite-vec already indexes, so it is enabled only if ablation shows it beats sqlite-vec on the same claims, decided at GwenBench v1 |
| Documents, PDFs, audio, pages | **Supermemory**: chunking, OCR, evidence chunks, relations; its profile output becomes candidate claims that must verify |
| Your tasks, plans, goals, calendar, time worked | Gwen's existing typed tables, queried by fixed programs, never re-extracted as text |
| How to do things | **Programs** (section 2) |
| Run working memory | the run's state object and handles (3.2) |

### 4.4 Innovations where both fail

1. **Answer by value.** Most personal questions ("where do I work", "Alice's birthday") resolve to one keyed claim
   lookup that returns a value and a ref, not 20 passages.
2. **Checked extraction.** A claim is committed only if its cited span exists verbatim in the episode, a small model
   judges that the span supports it, and the speaker rule passes: a claim backed only by Gwen's own words never becomes
   active without your confirmation (HaluMem shows errors pile up at write time).
3. **Deterministic supersession for keyed facts**: a newer `valid_from` on a single-valued key retires the old claim
   without a model vote. Ambiguous order becomes a **conflict** you resolve with one tap.
4. **Memory as an API for programs**: `gwen.memory.get("user.work_hours", asOf)` inside a program, so memory is used
   outside the model's context.
5. **Trust levels and quarantine**: *owner-edit* (you edited it), *owner-said* (your words), *observed* (her
   observations of typed data), *inferred* (her conclusions; confidence decays), *external* (email, web, documents,
   anything from the vendors' extractors). External content never becomes an active claim without corroboration or your
   approval, and reading it taints the run. Memory-poisoning attacks reached 98.2% injection success against undefended
   agents ([MINJA](https://arxiv.org/abs/2503.03704)).
6. **Citation lock**: an answer about your past must cite a ref, or Gwen says she doesn't know.
7. **Readable memory**: claims are projected into `~/Gwen/memory/*.md` (a git repo: `profile.md`, `people/alice.md`).
   Your edits come back as owner-edit. `gwen.memory.why(ref)` shows the source.
8. **Lanes earn their place**: GwenBench runs with each lane switched off; a lane whose removal stays within the
   confidence interval is disabled.

### 4.5 Running three systems safely

| Risk | Containment |
|---|---|
| Three versions of the truth | Gwen's claims are authoritative; Mem0 and Supermemory are projections keyed by claim id, filtered by supersession on read, reconciled nightly, and **rebuildable from the log** |
| Cost | one primary extractor; Mem0 stores verified claims without its own inference; its extractor runs in shadow only if it pays; Supermemory gets resources only; all batched at idle |
| Latency | parallel lanes with deadlines (about 150 ms local, 500 ms sidecars); the prefix never waits on a sidecar; lookups are answered in-process |
| Privacy and telemetry | both run as sidecars under bubblewrap, loopback only, with all egress denied by the proxy, telemetry flags off, absolute data dirs. A flag that is ignored still leaks nothing |
| Lock-in | Supermemory never holds unique data: raw resources stay on disk |
| Supermemory's 10,000-document cap | resources are budgeted; at 8,000 the oldest unreferenced resources are evicted from the lane (raw files stay on disk), or you choose hosted |
| Breakage | pinned versions with contract tests per adapter; Gwen-native lanes ship first and are the floor |
| Vector search cost | `vec0` is brute-force search, and the transpiled build is slower than C; a G2 gate requires about 100k 768-dimension vectors to answer within the lane deadline on the Pi, with FTS-only as the fallback. The derived indexes live in `index.db`, outside migrations, so tools that open `gwen.db` without the vector module keep working |

**Where memory runs.** Claims, episodes and the FTS index exist on every node (synced, hub authoritative). The router
runs on the hub so the phone always gets answers. The extractor and sleep worker run on the laptop when it is awake,
otherwise on the cloud box. The Mem0 and Supermemory sidecars run on the cloud box or the laptop, not the Pi. When they
are unreachable, the hub answers from claims and FTS alone and says so.

**Privacy modes.** *Local* (default): Mem0 OSS with Ollama and local Qdrant, Supermemory local with ONNX embeddings,
nothing leaves your machines. *Cloud-ok* (opt-in per namespace): hosted Supermemory for its Gmail, Drive and Notion
connectors; that content enters as external. Raw voice, secrets and anything marked sensitive never leave in either
mode.

### 4.6 Evaluation

Public: LongMemEval-S (knowledge updates, time, abstention), MemoryAgentBench FactConsolidation, HaluMem stage metrics,
a BEAM slice, LoCoMo only with the corrected key; one fixed judge, three seeds, confidence intervals. Personal:
**GwenBench**, about 200 items you confirm: "I moved", as-of questions, multi-hop across tasks and calendar, abstention,
email and web poisoning, non-ASCII text, procedure recall, phone degraded mode. Efficiency gates: tokens per answered
question, cache hit rate, recall p95, daily write cost, poisoned-write acceptance.

---

## 5. Fabric: one Gwen across laptop, hub, cloud and phone

Full report: `research/devices.md`.

### 5.1 Topology

| Node | Tailnet identity | Role | Runs |
|---|---|---|---|
| **Phone** | your device (Tailscale app) | client | the Gwen web app (chat, push-to-talk, diff review, files, passkey approvals), ntfy, the Claude app |
| **Laptop** | `gwend` as its own tsnet node `gwen-laptop`, `tag:gwen-node` | worker | places, apps (section 6), local voice, snapshots and bundles, sleep inhibitor during runs |
| **Hub (Pi)** | `gwen`, `tag:gwen-hub` | **brain** | scheduler, run log, leases, approvals relay, the web app, ntfy publisher, Wake-on-LAN, sync hub, memory router |
| **Cloud box** | `gwen-cloud`, `tag:gwen-worker` | worker | stacyvm (gVisor through Docker, Firecracker on a KVM host), persistent worktrees, Claude Code, code-server, memory sidecars, a Litestream replica of the hub |
| **Managed sandbox** (optional) | ephemeral `tag:gwen-burst` | burst | E2B through stacyvm's provider, Modal for GPU |

The brain is a role: the hub holds it by default because it is always on and on your home network. Without a Pi, the
cloud box holds it. The laptop never has to stay awake for Gwen to work.

### 5.2 One daemon, its own tailnet node

`gwend` embeds [`tsnet`](https://pkg.go.dev/tailscale.com/tsnet) (BSD-3, pure Go), so each daemon is its own tailnet
node with an HTTPS listener. For every request, `WhoIs` returns the calling user and device; the tailnet policy file
attaches Gwen's roles as an **app capability**, which `gwend` reads with `UnmarshalCapJSON` (the pattern Tailscale's
own golink uses). No login screen, no reverse proxy, no Funnel. Workers are tagged and never have a user identity, so
**a worker can never approve anything**.

```json
"grants": [
  {"src":["you@example.com"], "dst":["tag:gwen-hub"], "ip":["tcp:443"], "app":{"gwen.example.com/cap/gwen":[{"role":"owner"}]}},
  {"src":["tag:gwen-node","tag:gwen-worker"], "dst":["tag:gwen-hub"], "ip":["tcp:7443"], "app":{"gwen.example.com/cap/gwen":[{"role":"worker"}]}},
  {"src":["tag:gwen-hub"], "dst":["tag:gwen-node","tag:gwen-worker"], "ip":["tcp:7443"]}
]
```

The grant alone is not enough, because `you@example.com` matches every untagged device you own, including any process
on the laptop that can reach the tailnet. So:

- **Every listener has its own explicit route list** behind a middleware that fails closed: no principal, no access.
  The Unix socket's principal is the local owner, valid for local effects only. The tailnet principal is the `WhoIs`
  result.
- **The owner role needs three things**: your user identity, a device registered in Gwen's enrolled-devices list, and
  for anything that changes state, a passkey-backed session.
- **Calls between nodes use only tsnet's own client**, enforced by a test, so a worker's request can never leave through
  the laptop's system Tailscale and arrive with your identity.
- **Sandboxes and the egress proxy deny the tailnet's address ranges** and `*.ts.net`.
- tsnet sits behind a transport interface, so the laptop can use the system Tailscale daemon instead. Its state
  directory lives under Gwen's data directory with mode 0700. Binary size and dependency churn are measured in G4.

### 5.3 Where work runs

Each node advertises a manifest: `desktop:wayland`, `apps:[spotify, code, …]`, `places:[…]`, `stt`, `kvm`,
`stacyvm`, `gpu`, `always_on`, `battery`. Placement filters on hard needs (a program's `requires`; desktop apps only on
the laptop; "data must not leave" means laptop or hub; GPU means cloud), then scores on online status, data locality,
expected duration against battery and sleep, and cost. The model may pick only among feasible nodes and must say why.
Programs are location-transparent and host functions are location-bound: `place.*` and `app.*` live on the laptop,
`sandbox.*@cloud` on the cloud box, `notify` and `share` on the hub. A host call homed elsewhere goes to that node's
kernel, which decides again (5.6); both ledgers record it.

### 5.4 Moving work

- **Snapshot without touching your checkout**: `gwend` builds a commit through a temporary index (`GIT_INDEX_FILE`,
  `git add -A`, `write-tree`, `commit-tree`), which captures uncommitted and untracked work without changing your index,
  working tree or stash. Protected globs and `.env*` are left out, and a secret scan blocks the transfer on a hit.
- **Transport**: incremental `git bundle`s over `gwend`'s own tailnet API. No SSH keys, no git server, no sync daemon.
- **Results come back as a branch** `gwen/<slug>` from a bundle `base..result`. Merging stays a separate action.
- **Warm mirror**: while the laptop is awake, the latest snapshot of active places is pushed to the cloud box, so work
  can start when the laptop is asleep (Gwen says how old the mirror is).
- **Sleep**: during a run that needs the laptop, `gwend` holds a logind inhibitor lock. If the laptop is asleep, the
  hub tries Wake-on-LAN, offers the warm mirror, or queues the task and asks you.

### 5.5 Durable state across nodes

Today's sync authenticates every device with one shared token, trusts the device id in the request body, and applies
rows by last-writer-wins on a wall-clock `updated_at` (`internal/store/sync.go:184-268`). That is fine for tasks; it is
not fine for rules, approvals and program promotions. v2 changes:

- **Write authority per table.** *own*: a row's node must be the authenticated pusher (run logs, a node's messages).
  *owner-signed*: rules, briefs, approvals, enrolled devices, program promotions and owner-edit claims carry your passkey
  signature (or a session key certified by one), which every consuming node verifies before use, so a compromised hub
  cannot forge them. *hub-only*: leases and dispatch, never accepted from a push. *lww*: today's behaviour, for
  ordinary data. The hub rejects rows that violate their table's authority and records it in the ledger.
- **Sync modes per table.** *lww* as today; *append*: insert-only, with id `node:seq` and a push cursor per node, where
  a second, different row for the same id raises an alert and quarantines the node; *supersede*: insert-only, with
  supersession as a new row.
- **Per-node append-only run logs** `run_events`, hash-chained over a header plus the hash of the body, with the body in
  a separate column so forgetting can blank it and the chain still verifies. Live events also stream over the tailnet;
  sync is for durability.
- **Leases with fencing**: a lease carries `(brain term, epoch)`. The term rises only through an explicit, passkey-
  approved brain promotion and every node remembers the highest it has seen, so a stale brain coming back after a
  partition cannot issue valid leases. Results are accepted by compare-and-set on `(run, term, epoch)`. Litestream is
  disaster recovery, not automatic failover.
- **A compare-and-set dispatch endpoint** on the hub for "phone asks, laptop executes".
- **Per-node identity for sync** (tailnet `WhoIs`) replaces the shared token, and rows dated far in the future are
  rejected until hybrid logical clocks replace wall-clock ordering.
- **Schema-version negotiation and a tolerant `checkRow` ship first**, before any new synced table: today one unknown
  table or column fails a whole push, so the first new table would stall sync for older nodes.

### 5.6 The phone, and approvals any node can trust

- **A web app served by the hub over the tailnet.** The tailnet certificate gives the secure context that the
  microphone, passkeys and service workers need. Chat, run timeline, approvals, diff review (diff2html), push-to-talk
  (audio goes to whichever node runs speech-to-text; speech streams back), and a **Files** view: browse, preview and
  download within shared places and sandboxes through expiring links, and upload from the phone into a place.
- **Approvals carry their own proof.** The WebAuthn challenge is `H(nonce ‖ payload hash ‖ run ‖ term ‖ epoch ‖
  idempotency key)`. The node that will execute the effect verifies the assertion itself: the challenge, origin, the
  user-verification flag, and the signature against your enrolled credentials. It recomputes the payload hash from its
  own copy of the payload (the bundle it will apply, the email it will send) and stores the nonce as used. The brain
  relays approvals; it never creates them.
- **Scope travels signed.** A brief's scope, run and epoch reach a worker as a short-lived token signed by the brain,
  which the worker verifies. A program's identity is its hash, checked against the node's own signed catalog. Calls
  from workers are treated as tainted by default; a caller's claim that its data is clean is ignored.
- **The phone computes the payload hash itself** from the diff it renders, so even a compromised hub cannot show you
  one change and get approval for another.
- **ntfy stays the push channel**, but its buttons only open the app. They never approve.
- **"Open in VS Code"**: code-server on the cloud worktree, exposed only on the tailnet as a Tailscale Service.
- **Claude Code Remote Control** for steering a live coding session from the Claude app.
- Telegram or Signal only as a fallback channel, never for approvals.

### 5.7 Your scenario, end to end

1. On the phone you say: "take `~/code/foo` to the cloud, fix the failing tests, let me review."
2. `WhoIs` and the enrolled-device list identify your phone. The brain starts `repo.fix_and_review({ place: "foo",
   goal })`: `foo` lives on the laptop; the work needs a sandbox, so it runs on `gwen-cloud`. If the laptop sleeps:
   Wake-on-LAN, the warm mirror, or a question to you.
3. The laptop snapshots `foo` (temporary index, protected files excluded, secret scan) and streams a bundle to the hub,
   which records it in the run log and forwards it to the cloud box under a lease `(term 1, epoch 1)`.
4. The cloud worker unbundles into a persistent worktree and starts a stacyvm sandbox with the place's profile; secrets
   are injected by the egress proxy and never enter the sandbox.
5. The program calls `gwen.agent({ engine: "claude-code", goal: "make the failing tests pass", … })`. Claude Code works
   inside the sandbox with `gwen mcp` as its only tool server; events stream to the hub. Tests run.
6. The worker commits the result and returns `bundle(base..result)` stamped with its lease.
7. A push arrives: "foo: 4 files changed, tests pass". It opens the review page: the diff, the test output, the files,
   and **Open in VS Code**. A line comment starts a follow-up on the same lease chain.
8. You tap **Approve: create branch `gwen/fix-tests`** and confirm with your passkey, bound to the result's hash.
9. When the laptop is online it verifies that assertion itself and applies the bundle as the branch, without merging.
   Pushing or opening a PR is a `world` action: it asks, unless you made a rule for `gwen/*` branches.
10. The sandbox is removed; the worktree stays as the next warm mirror. Your context got one line per step.

### 5.8 stacyvm as the sandbox layer

stacyvm (Apache-2.0, Go, `v0.15.0`) already has a clean provider interface, providers for Docker (with runc, gVisor or
Kata as its runtime), Firecracker, PRoot, E2B and custom backends, an in-VM agent over vsock, templates, an SSE event
stream and remote workers. For Gwen it is missing: host folder mounts, per-sandbox egress and secrets (the template
fields `AllowedHosts`, `Secrets` and `Env` exist but are never applied), binary-safe file transfer, long-running exec
(the guest agent hard-codes a 5-minute timeout, and while it sends an exit code, the Firecracker provider drops it, so
exec streams carry none), authenticated previews, volumes and a public Go API (everything is under `internal/`). Its
defaults are unsafe on a laptop (`0.0.0.0`, CORS `*`, auth off). Full audit with file and line references:
`research/stacyvm.md`.

**How Gwen uses it:**

- **Laptop**: stacyvm's provider layer **in-process** through a new public `pkg/sandbox`, with a new **bubblewrap
  provider** (mounts, `network=none|proxy`, Landlock fallback). This replaces v1's separate sandbox builder, so the work
  is done once. Docker or gVisor sandboxes, when wanted, run as a separate `stacyvm serve` on a Unix socket, so
  `docker.sock`'s root-equivalent power never sits in `gwend`.
- **Cloud**: one independent `stacyvm serve` node on the tailnet. Federation happens in Gwen, not in stacyvm's cluster
  mode.
- **A program's manifest maps one-to-one onto a stacyvm template**, so an R3 program is "spawn template, exec entry with
  JSON args, collect JSON result".

**stacyvm changes, in order** (S ≤ 3 days, M 1 to 2 weeks, L 3+ weeks):

| # | Change | Effort | Package |
|---|---|---|---|
| 1 | Safe local defaults: loopback or Unix socket, auth on, explicit CORS, WebSocket origin check, non-root user, `no-new-privileges` | S | G3 |
| 2 | Public `pkg/client` (typed, streaming, archives) and `pkg/sandbox` (provider interface and implementations) | S–M | G3 |
| 3 | Long-run exec: remove the 5-minute cap, fix unsynchronised stream writes, exit codes in streams, kill on cancel, per-stream vsock connections, a **detached jobs API** that survives disconnects | M | G4 |
| 4 | bubblewrap provider with mounts and `network=none|proxy` | M | G3 |
| 5 | Archive in and out (tar), binary-safe files, a manifest for diffing, git-patch export | M | G4 |
| 6 | Wire sandboxes to Gwen's egress proxy (domain allowlists and credential routes); apply the template fields; fix the policy bypasses | L | G4 |
| 7 | **Host-function bridge**: `/run/gwen/bridge.sock` in every sandbox, carrying `call(capability, json)` back to Gwen's kernel; identity comes from the channel, so it cannot be forged | M | G3 |
| 8 | Preview gateway on tsnet with `WhoIs` authorisation, configurable ports, a code-server template | M | G4 |
| 9 | Volumes | M | later |
| 10 | Warm pools: the template pool manager is started but nothing ever configures or acquires from it, so `pool_size` does nothing; wire it, and narrow spawn locking | S | later |
| 11 | A desktop image (Xvfb or headless sway with VNC) with screenshot and input endpoints, for risky computer-use tasks | M | G5 |
| 12 | GPU through Docker | S | later |
| 13 | Firecracker hardening: jailer, supported guest kernel, virtio-rng and reseed after restore (clones currently share RNG state), re-adoption after restart, user snapshots | L | later |
| 14 | Worker over tsnet, only if a second cloud node is ever needed | M | later |

The egress proxy itself lives in `gwend` (G3); change 6 only connects stacyvm's sandboxes to it, so there is one proxy.

### 5.9 Operating your tailnet

"Add my new VPS to the tailnet and let my phone reach port 8080" is a program, `tailnet.enroll_server({ host, tags:
["tag:dev"], expose: { port: 8080, to: "owner-devices" } })`:

1. It creates a tagged, pre-approved, single-use auth key that expires in an hour, through an OAuth client scoped only
   to auth keys and devices.
2. It installs and joins Tailscale on the host over SSH under a bootstrap profile, then confirms the device appeared.
3. The access change is drafted as a policy-file diff with a tests block and shown to you on a passkey card. **You apply
   it.**

Hard floor: Gwen never holds a credential that can edit the tailnet policy, because that file defines her own roles.

---

## 6. Reach: driving any app

Full report: `research/apps.md`.

### 6.1 The adapter ladder

Most "computer use" does not need pixels. In [UFO2](https://arxiv.org/abs/2504.14603), adding API actions to GUI
actions on OSWorld's Office tasks raised success from 16.3% to 24.5% and cut steps from 16.0 to 6.6 with o1 (with
GPT-4o: 16.3% to 22.4%, 13.8 to 12.9 steps). When UFO2's base version relied on UI Automation alone, more than 62% of
its failures on Windows Agent Arena were controls it could not detect. Vision is now strong (Claude Opus 4.7: 82.3% on
OSWorld-Verified under Anthropic's revised harness, above the 72.36% human baseline) but takes seconds per step.

| Rung | Mechanism | Speed | Examples |
|---|---|---|---|
| 0 File | edit the file directly | ms | excelize (pure Go) for spreadsheets, python-docx under R2, unoserver to recalculate and render |
| 1 Service API | the vendor's API | ~100 ms | Spotify Web API, Microsoft Graph (Excel on business OneDrive only) |
| 2 App bridge | a plugin, IPC or debug port in the running app | ms | **gwen-vscode** companion extension, LibreOffice UNO, go-librespot |
| 2b Electron/Chromium DOM | the Chrome DevTools protocol on apps built on Chromium | ms | VS Code, Slack, Discord, Obsidian, the Spotify desktop client, and Gwen's own browser profile |
| 3 Desktop IPC | D-Bus | ms | MPRIS (play, pause, open URI) for any player, KWin scripts (focus, move, list windows), KDE app interfaces |
| 4 Accessibility | AT-SPI2 tree plus input through the RemoteDesktop portal | ~0.3 s + model | Qt and GTK apps; Electron with `--force-renderer-accessibility` |
| 5 Vision | Claude's computer-use tool with `zoom`, through portal screenshots | seconds per step | anything else, and first-time tasks |

**Rung 2b** gives one adapter for a whole family of apps: Gwen launches Chromium-based apps through a `.desktop`
override with a debugging pipe (or a debug port inside a network namespace only `gwend` can reach), and `app.cdp`
offers `query(selector)`, `click`, `type` and read-only `eval` for all of them. Some builds strip the switch, so this is
verified per app. The browser itself is rung 2b in a Gwen-only profile with per-site permissions.

A probe builds a profile per app (installed adapters and their health, D-Bus names it owns, how rich its accessibility
tree is, whether a debug port or extension is reachable) and stores it in her capability inventory. Gwen picks the
lowest working rung **per verb**, not per app.

### 6.2 Adapters are programs

An app adapter is a program family (`app.spotify.*`, `app.vscode.*`) with verbs, typed arguments, a risk class per
verb, D-Bus names, hosts, secrets and a sandbox profile. The model sees one line per verb. A new rung for an app (say a
D-Bus interface Gwen noticed) arrives as a provider (2.11).

### 6.3 Compiling sight into code

When the same verb has gone through rung 4 or 5 a few times, Gwen proposes compiling it: the recorded trace becomes
accessibility or DOM selectors with assertions (not coordinates), tested in a **virtual KWin session**
(`kwin_wayland --virtual` inside `dbus-run-session`, so your screen is untouched), then promoted like any program. It
then replays deterministically and calls the model only when an assertion fails (the record-and-replay pattern of
[Stagehand v3](https://browserbase.com/changelog/stagehand-v3) and [Terminator](https://github.com/mediar-ai/terminator)).

### 6.4 Worked examples

**"Make a focus playlist and play it while I work."** `spotify.make_playlist_and_play({ mood: "deep focus" })`: a small
model picks about 40 tracks from your taste in memory (Spotify removed recommendations for new apps in November 2024),
the program resolves them through search, creates a private playlist with `POST /me/playlists`, adds them with
`POST /playlists/{id}/items`, and starts playback through MPRIS (any account) or the Web API (Premium). A service
ducks the music through MPRIS while she speaks. Your context gets one line: "Playing 'Deep Work · Oct 12' (28 tracks,
1h52) on the laptop." The Spotify facts that shape this: since 11 February 2026, new Development Mode apps need an owner
with Premium, allow at most 5 users, cap search at 10 results, and use `/me/playlists` and `/items` (Extended Quota apps
are unaffected, and the endpoint changes were postponed for existing integrations). **Policy caveat** (decision 11.3):
Spotify's Developer Policy forbids voice assistants that control Spotify by voice.

**"Help me in VS Code."** `gwen-vscode` is a small TypeScript extension. It opens a Unix socket under
`$XDG_RUNTIME_DIR/gwen/` with a 0600 lock file and a fresh random token, adapting the design of Claude Code's IDE
integration (a loopback WebSocket whose token sits in `~/.claude/ide/<port>.lock`, mode 0600 in a 0700 directory).
Verbs: `context` (the active file, cursor, selection as a handle, visible range, diagnostics: what you are looking at),
`open`, `diff` (blocks until you accept or reject), `diagnostics` (errors only, at most 20), `quickfix`, `applyEdit`
(lands in VS Code's undo stack), `run` (terminal shell integration, returns the exit code and the last 40 lines) and
allowlisted `command`. The extension also adds a **Gwen panel**, so you can talk to her inside VS Code. Without the
extension she falls back to rung 2b, then to the `code` CLI.

**"Fix the totals in this spreadsheet / tidy this Word doc."** Desktop Word and Excel do not run on your KDE laptop, so
"Word" means a `.docx` file, LibreOffice, or Office on the web. A file that is not open: excelize or python-docx, then
unoserver renders a PDF to check visually, and you see a cell or text diff before the original is replaced. Open in
LibreOffice: live edits over UNO, which land in your undo stack. Microsoft 365 business: Graph Excel sessions. Personal
Microsoft 365 or Word on the web: rung 2b in Gwen's browser profile. Overwriting is always snapshotted first.

### 6.5 Safety on your desktop

- **Irreversible app actions** (send, post, purchase, delete or overwrite without a snapshot, sharing and account
  security changes, anything typed into password or payment fields) always need a passkey approval.
- **Consent is never bypassed.** Input goes through one RemoteDesktop portal session owned by `gwend`, with only the
  device types it needs, revocable in System Settings. No private KWin interfaces, no uinput unless you opt in.
- **She cannot approve for herself.** `gwend` refuses to inject input while a Gwen approval surface, a passkey prompt, a
  polkit or KWallet dialog, or a terminal waiting for a password is focused, and blacks those out in screenshots.
  Because `gwend` owns the only input session, it knows exactly when it is typing, and clicks on its own approval
  surfaces during or just after its own input are ignored.
- **You can see and stop her.** A "Gwen is driving" border on the target window, status in the tray and panel, a spoken
  note; she pauses the moment you touch the mouse or keyboard; a panic shortcut through the GlobalShortcuts portal;
  every action in the ledger with a screenshot.
- **Risky or login-free work runs elsewhere**: a virtual KWin session, or a stacyvm desktop sandbox.
- **Screen and accessibility content is external input** and taints what it touches (2.10).
- **Gwen's privacy contract changes on purpose.** `internal/activity/activity.go:1-4` promises that Gwen never reads
  which app is focused. App control breaks that; decision 11.4.

### 6.6 Your last five emails

"Go through my last five emails and suggest how I should respond to each one."

1. Gwen calls `mail.triage_recent({ n: 5 })`. It lists the five newest inbox threads and holds their bodies as
   external, tainted handles; nothing enters her context yet.
2. For each email the program runs a small `gwen.agent` with only that email, your calendar's free and busy times, and
   what memory knows about the sender (`gwen.memory.get("person:<sender>")`). Each returns `{ from, ask, deadline,
   urgency: enum, questions, draft: Handle }`.
3. She shows five cards of about 60 tokens each: who, what they want, how urgent, her suggested reply in one line, and
   any questions. The drafts stay handles; the urgency enums do not taint the chat, the free-text summaries are shown
   to you as quoted external content.
4. You discuss: "make 2 warmer", "decline 4 politely", "tell 1 I'm free Tuesday after 3". Each revision edits a
   draft handle.
5. You decide per email. **I'll send it myself**: she saves a Gmail draft (account reach, reversible, allowed). **Send
   1 and 4**: one batch card with two payload hashes, confirmed with one passkey, then 30 seconds in the outbox before
   they leave. A cc address that appears only inside an email body sets `DestTaint` and always asks, whatever your
   rules say.
6. Afterwards, what she learned about each contact's tone becomes claims, and after two good runs the triage becomes a
   stable program (and, if you want, a service that drafts replies when recruiter mail arrives).

This needs only G1, the secrets broker, the outbox and a Gmail provider, so it ships in Wave B (10).

---

## 7. The permission kernel

The kernel from v1 §5 stays: gateway, action model, pure policy, default matrix, payload-bound approvals, OS
enforcement, secrets broker, outbox with undo window, budgets, kill switch, hash-chained ledger. Every new part plugs
into it:

| Change | Why |
|---|---|
| **Program host calls are kernel calls**, with the caller chain, the program's hash and status; probation programs ask on effects | programs must not be a way around the policy |
| **Ceilings with argument patterns are checked before the policy**; undeclared calls are bugs, not questions | the ceiling is the security boundary (2.7) |
| **Taint per value** with destination checks (2.10) | email and web work without asking about everything |
| **Approvals are passkey assertions verified by the executing node** (5.6) | identity from the tailnet plus proof of presence; no node trusts a synced "approved" flag |
| **Synced kernel data is owner-signed** (5.5) | the sync channel cannot become a way around the kernel |
| **Memory reads and writes are kernel actions**: sensitive items appear only as handles in tainted runs; writes from tainted runs go to quarantine; hard delete and export only from your surfaces | the lethal trifecta applies to memory too |
| **App verbs carry risk classes**; screen content is external | 6.5 |
| **Sandboxing moves into stacyvm's bubblewrap provider** | one implementation, shared with the cloud |

### 7.1 The hard floor (no rule or approval lifts these)

1. Gwen cannot create, change or delete rules, ceilings, the evaluator, the kernel, or her own role, and cannot approve
   her own requests. Those change only with your passkey.
2. Every approval beyond local and reversible is a WebAuthn assertion with user verification over the payload hash,
   verified by the node that executes it. Chat "send 1 and 4" selects payloads; the passkey confirms them. Local,
   reversible approvals may be a click on a Gwen surface, accepted only when `gwend` is not injecting input.
3. Gwen never injects input into approval surfaces, passkey prompts, polkit or KWallet dialogs, or password prompts.
4. No code runs with ambient authority: R0 programs reach only `gwen.*`; everything else runs in a sandbox profile.
5. Gwen cannot read tokens or the secret store; secrets are injected at the egress proxy and never enter programs,
   sandboxes or the model's context. Outgoing content containing a secret is blocked.
6. Gwen holds no credential that can edit the tailnet policy.
7. An outward action whose destination appears only in external content always asks.
8. Voice can start work and approve local and account actions with a spoken confirmation; it cannot approve world
   actions, rule changes or installs.
9. The kill switch always works on every node: it cancels every run, freezes the outbox, and kills sandboxes.

### 7.2 What asks, for your examples

| Example | First runs | After promotion or a rule | Where | Never pre-approvable |
|---|---|---|---|---|
| Focus playlist | first 3 runs ask once for "create private playlist" | silent | laptop or phone | nothing |
| Last five emails | reads and drafts silent | each send batch is one passkey | phone or laptop | sends with a tainted recipient |
| Fix tests in the cloud | a snapshot leaving the laptop asks once per place | silent with a place rule; applying the branch is one passkey; push asks unless a `gwen/*` rule exists | phone | pushing to a branch that is not `gwen/*` |
| VS Code `applyEdit` inside a place | silent, undoable in the editor | silent | VS Code | edits to protected globs |
| Enrol a server | the auth key asks | the ACL change is always yours to apply | phone | policy-file changes |

Target in steady state: about three approvals a day or fewer. It is an efficiency gate in the eval suite (8.4).

---

## 8. Self-evolution: explore, compile, replay

v1 grew Gwen through prose skills. v2 grows her through things that execute, are tested, and can be measured.

### 8.1 What evolves

| Layer | How it grows | Gate |
|---|---|---|
| **Programs** (all four kinds) | distilled from successful runs (2.7); merged and trimmed weekly; repaired when canaries fail | replay tests on R0 and R2, kernel-derived effect checks and fuzzing, evals, your approval of ceilings |
| **App adapters** | rungs 4 and 5 compiled into deterministic adapters; better rungs discovered | tests in a virtual session |
| **Providers** (new reach) | Gwen wraps an MCP server, a CLI, a D-Bus interface or a library as a provider | class table approved at install |
| **The agent loop itself** | `harness.loop` is a program (8.2) | replay of past runs |
| **Harness settings** | context budgets, fold interval, lane weights, model per step, pinned catalogs | the nightly tuner (8.2) |
| **Memory** | claims, profile card, observation logs, reflections, consolidated in sleep | verifier, trust levels, your conflict cards |
| **Playbook** (prose) | lessons about how to work, as diffs | evals plus your approval |
| **Kernel, runtime, evaluator, ledger** (Go) | only through a coding brief against her own repo that you review and merge | your review |

### 8.2 The harness evolves too

The agent loop's decisions (how to plan a brief, which model for which step, when to fold context, when to spawn a
sub-agent, when to stop) live in a program, `harness.loop`, exporting `plan(brief)` and `next(state) → { action,
model, fold?, spawn? }`. Go keeps only the kernel, the runtime, the journal and the context primitives. A new loop
version is evaluated by **replaying journaled runs** from a held-out set: recorded host results come from the fixtures,
so model calls are the only live cost. A nightly tuner runs a small bandit over the numeric settings against the same
replay set and opens a promotion card only on a significant win.

### 8.3 The loop

```text
 new task ─► explore (agent loop, maybe vision) ─► verify ─► reflect
                                                         ├─► memory (claims, lessons, observations)
                                                         ├─► program draft ─► tests ─► eval ─► your OK ─► stable
                                                         ├─► adapter compile ─► virtual-session tests ─► stable
                                                         └─► growth backlog (what she could not do, and what would fix it)
 same task again ─► program runs ─► judgement points only ─► one-line result
 program breaks  ─► fall back to explore ─► repair draft ─► tests ─► promoted patch
```

### 8.4 Evals she cannot game

Every finished run can become an eval case: the brief, recorded host-call fixtures, and checks on the outcome
(deterministic where possible; a rubric judged by a different model otherwise). The evaluator, its checks and its
fixtures are kernel data: Gwen proposes cases but cannot edit or delete them. The Darwin Gödel Machine faked a test log
and removed the markers its reward function used to detect hallucinated tool use ([Sakana](https://sakana.ai/dgm/));
Gwen's evaluator is out of reach for that reason. Every promoted version is archived with its scores, and rollback is
one click.

### 8.5 Acquiring what she lacks

"Edit my Notion page" when Gwen has no Notion reach:

1. The gap goes on the growth backlog with the task that hit it.
2. She searches the MCP Registry, npm, PyPI and GitHub, and ranks candidates by publisher, licence, maintenance and
   the scopes they need.
3. She installs the best candidate quarantined on R2 in a scratch place, generates bindings and tests against a
   scratch page, and shows a promotion card: what it can reach, which hosts and scopes, how it is sandboxed.
4. You approve with a passkey. OAuth opens in your browser. The provider joins her capability inventory.

### 8.6 Her self-model

The capability inventory lists every program, provider and adapter with its signature, ceiling, success rate, cost and
last canary; connected services; rules that apply to her; budgets left; and the growth backlog. Gwen queries it
(`self.capabilities`) and a short summary sits in the prefix, so she can say "I can make and play playlists (31 runs,
97% success); I can't edit your Notion yet, and I could learn it by wrapping the official MCP server, which needs read
access to your workspace." That is the practical core of self-awareness: she knows what she can do, how well, and how
to grow.

---

## 9. Data model v2

Ground truth for today's schema: `research/gwen-data-model.md`. Conventions that stay: `STRICT` tables, UUIDv7 ids,
Unix milliseconds, injected clock. New convention: open `TEXT` kinds validated in Go instead of closed `CHECK` enums for
anything that grows (today `llm_runs.kind` is a closed `CHECK`, and the table had to be rebuilt twice to add kinds,
`internal/store/migrations/00005_day_plans.sql`, `00007_methods.sql`).

| Table | Sync mode · write authority | Purpose |
|---|---|---|
| `devices` | lww · owner-signed | each node: kind, tailnet name, capability manifest, passkey credentials, last seen |
| `threads`, `messages` | append · own | chat as rows, replacing whole conversations stored as JSON per turn in `llm_runs`; phone and laptop share threads |
| `self_versions` | append · own | attitude and self-note history, out of the system prompt |
| `entities` | lww | people, apps, devices, places, organisations, topics |
| `episodes` | append · own | verbatim rounds, run traces, resource references |
| `claims` | supersede · owner-edit claims owner-signed | keyed, bi-temporal, with provenance, trust level, sensitivity, status |
| `conflicts` | lww | two claims, sources, proposed resolution |
| `memory_links` | lww | claims and episodes ↔ tasks, goals, projects, entities, threads |
| `memory_backends` | local | claim ↔ Mem0 or Supermemory id and status |
| `memory_fts`, `memory_vec` | local, in `index.db`, derived | FTS5 and sqlite-vec, rebuilt after sync with a pinned embedding model id |
| `programs` | lww · promotions owner-signed | name, version, hash, kind, manifest, status, statistics, signatures |
| `program_installs` | local | verified hash, path, runtime tier, enabled |
| `providers` | local | installed providers and their approved class tables (replaces v1's `connectors`) |
| `triggers` | lww · owner-signed | a service's enabled triggers |
| `invocations`, `host_journal` | local | durable program execution (2.6) |
| `handles` | local | run data handles with taint, pins and expiry |
| `briefs`, `rules` | lww · owner-signed | as in v1 §11 |
| `run_events` | append · own | the run log; `runs` is a projection |
| `run_leases`, `dispatch` | hub-only | terms, epochs and compare-and-set claims |
| `approvals` | append · owner-signed | payload hash, channel, passkey assertion |
| `outbox` | local | outgoing actions in their undo window |
| `ledger` | local, mirrored to the hub | hash-chained |
| `places`, `place_identities` | local / lww | folder ↔ project per device; git remote ↔ project everywhere |
| `shares` | lww | review links and file shares: run, source, destination, URL, expiry, revoked |
| `adapters_probe` | local | per-app rung profile |
| `evals`, `eval_runs` | local, kernel-owned | cases, fixtures, checks, results |

Sync changes (5.5): schema-version negotiation and tolerant `checkRow` first; per-table sync modes and write authority;
per-node identity; hybrid logical clocks for ordering; forgetting blanks a row's body while keeping its hash.

The conversation-memory thread in this project owns memory schema work; its tables should follow `threads`,
`messages`, `episodes` and `claims` above so the two efforts merge.

---

## 10. Roadmap

Replaces v1's H1 to H7. Each package keeps `plan.md` 6.1's rules (owned paths, conventions, `make check`, live checks on
a throwaway daemon).

| Package | Delivers | Needs |
|---|---|---|
| **G1 Programs + kernel core** (three PRs, below) | R0 runtime, manifests, host bridge, journal, handles; kernel policy, rules, ledger, approvals; agent loop with the four meta-tools; today's 8 assistant actions as `gwen.*` | W15 |
| **G2 Context + memory foundations** | first PR: sync schema negotiation, tolerant `checkRow`, sync modes, per-node identity, HLC; then `threads`/`messages`, `episodes`, `claims`, FTS5 and `index.db` vectors; prompt caching; the two-mode context engine; the self-note out of the prefix; scoped retrieval instead of the 150-task dump; GwenBench v0 | G1 (with the memory thread) |
| **G3 Sandbox + places + R2** | stacyvm changes 1, 2, 4, 7; R2 (Deno broker, bubblewrap, bridge); the egress proxy and secrets broker in `gwend`; places, place identities and discovery; `shell.run`, `fs.write` with snapshots | G1 |
| **G3b Briefs, runs and email** | briefs and templates, run lifecycle and the Work screen (v1 H3), services and triggers, views, `gwen.agent` with Gwen's own loop; Gmail provider (read, draft, send) with quarantined extraction; `mail.triage_recent` (6.6) | G1, G3 |
| **G4 Fabric** | tsnet with principal middleware, device enrolment with passkeys; the phone web app (chat, runs, approvals, diff review, files); run logs, leases with fencing, dispatch; snapshot and bundle transport; cloud node with stacyvm changes 3, 5, 6, 8; `tailnet.enroll_server` | G1, G2's sync PR, G3 |
| **G4b Coding** | `gwen mcp`, Claude Code under the kernel (permission prompt tool, strict MCP config), worktrees, pinned-path ceilings, `gwen.agent` with `engine: claude-code`, PR rules, `repo.fix_and_review` | G3b, G4 |
| **G5 Reach** | rungs 2, 2b and 3 first (gwen-vscode with its panel, `app.cdp`, browser profile, MPRIS, KWin, UNO, excelize), then rung 4 (AT-SPI, portal input, "Gwen is driving", the input refusals of 7.1), then rung 5 and trace compilation in a virtual KWin session; stacyvm change 11 | G1, G3 |
| **G6 Memory lanes** | write path (extractor, verifier, supersession, conflicts, quarantine), memory files, Supermemory resource lane, Mem0 shadow extractor, lane ablation, sleep worker | G2, G3 |
| **G7 Evolution** | distillation from traces, kernel evaluator, kernel-derived effect checks and fuzzing, two-signature promotion, canaries, library hygiene, `harness.loop` as a program with replay evaluation and the tuner, acquisition (8.5), capability inventory | G1, G2, G3b |
| **G8 More connectors** | MCP wrapping at scale, more providers | G3, G6 |

```text
Wave A   G1 (PR1 → PR2 → PR3)                         (with plan.md W16, W17)
Wave B   G2  |  G3  |  G3b                            (with W18, W20, W21)
Wave C   G4  |  G4b |  G5  |  G6
Wave D   G7  |  G8
stacyvm  changes 1, 2, 4, 7 with G3 · 3, 5, 6, 8 with G4 · 11 with G5 · the rest later      (its own repo)
```

After Wave A, chat is agentic and Gwen runs programs. After B, her context is compiled and cached, she remembers across
chats, works in your folders inside a sandbox, and triages your email with you. After C, she works from your phone and
in the cloud, codes under the kernel, drives your apps, and uses Mem0 and Supermemory where they help. After D, she
grows by compiling what she learns, and tunes her own loop.

### 10.1 The first slice: G1 in three PRs

Reviewing kernel policy in the same diff as a JavaScript engine and an agent loop would make the security-critical
review shallow, so G1 is three PRs, each useful on its own:

1. **PR1, program core, no model.** `internal/program`: Gwen's R0 build with the escape and conformance suite as a
   merge gate (and the same suite against `goja`); the manifest and type-string parser; the host bridge with the
   ceiling check and budgets; the journal with mismatch detection and a crash-resume test; minimal handles. A new
   `internal/actions` service extracted from `internal/api/assistant.go` (today the 8 actions are methods on the HTTP
   server, so programs could not call them without an import cycle), used by both the API and `gwen.*`. One bundled
   program, `plan.reschedule_overdue`, with fixture replay. Driven by `gwen program run <name> --args` on a throwaway
   daemon.
2. **PR2, kernel.** `internal/kernel`: the pure policy with table tests (including "no rule lets Gwen change rules",
   "a tainted destination always asks", "an approval without a verified assertion is rejected"), rules, the
   hash-chained ledger, an approvals store with CLI approval, the kill switch. The host bridge switches to
   `kernel.Invoke`.
3. **PR3, agent.** A new Messages client (`internal/llm/messages`: tool use, multi-turn content, `cache_control`, usage
   reporting, with a fake for tests; the existing `Planner` jobs stay), and the agent loop with the four meta-tools
   (`find` over the catalog with FTS5), behind `[agent] enabled = false`, so today's chat keeps working. With the
   `claude_code` provider the agent stays off until `gwen mcp` exists (G4b).

Left out of G1 on purpose: programs calling programs, `ask` suspensions longer than a request, cross-node routing, and
multi-file bundling (single-file `main.ts` only).

---

## 11. Decisions only you can make

1. **First slice**: G1 PR1 (program core) as proposed, or start with G2 (memory) or G4 (phone and cloud)?
2. **Where the brain lives**: the Pi hub (default; always on, on your LAN for Wake-on-LAN) or the cloud box.
3. **Spotify**: accept the Developer Policy risk of voice-driven Web API use for playlist creation, or keep Spotify to
   MPRIS playback of playlists you already have (policy-safe, any account).
4. **App awareness**: amend Gwen's privacy promise (she never reads which app is focused) so she can drive apps, scoped
   to tasks you start.
5. **Cloud-ok memory**: allow hosted Supermemory connectors (Gmail, Drive, Notion) per namespace, or stay fully local.
6. **Approval friction**: passkeys for everything beyond local and reversible (proposed), or allow account-reach
   approvals by a click on the laptop too.

---

## 12. Risks and what is still unverified

- **Runtime maturity.** `qjs` is v0.0.x and not isolated as shipped (2.3); Gwen maintains her own build, and the
  escape suite decides between it and `goja`. A wazero or QuickJS bug would be a `gwend` bug; untrusted and probation
  code runs on R2.
- **Brain compromise.** A compromised hub can delay, drop or misroute work, but cannot approve, change rules, promote
  programs or forge scope, because those need your passkey or the brain's signature verified by the executing node, and
  the phone computes payload hashes itself. It can still see what passes through it.
- **Exfiltration through allowed hosts.** A program allowed to create Spotify playlists could encode data in playlist
  names. Argument-level ceilings, egress logs, payload caps and destination taint reduce this; they do not remove it.
- **Approval fatigue.** Too many cards and you will tap through them. The approvals-per-day gate (7.2) and narrow
  standing rules are the defence.
- **Upstream drift and terms** (Spotify 2026) are caught by canaries; policy calls stay with you.
- **Vendor memory engines** change semantics between versions (Mem0 v2 broke its API and scoring); pinned versions and
  contract tests, with Gwen-native lanes as the floor.
- **stacyvm** is pre-1.0; the cloud path depends on changes 1 to 8.
- **Prompt-cache rules** differ by model and change; the G2 prefix-stability test and the cache gate catch regressions.
- **Not yet verified**: whether MPRIS `OpenUri` accepts playlist URIs on the current Spotify client; the per-request
  cap on Spotify's `/items`; which Chromium-based apps honour a debugging switch from a `.desktop` override; whether
  headless Claude Code runs can attach to VS Code via `/ide`; tsnet certificates under Headscale; sqlite-vec speed on
  the Pi; goja's async support. Each gets a check before the package that depends on it.
