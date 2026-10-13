# Program files: research for Gwen's executable skills

Legend: **[V]** means I read it in the cited source. **[I]** means it is my own judgement or inference. "Not verified" means I could not confirm it.

## 0. Bottom line

The owner is right that SKILL.md is mostly text. The research from 2023 to 2026 points the same way.

- **Three things save context.**
  1. **Code as the action.** One snippet can make many tool calls. Intermediate results stay in the runtime.
  2. **Deferred discovery.** The agent sees short names and signatures, not full schemas.
  3. **Executable skills checked by tests.** These beat text skills.
- **Unverified self-written skills tend not to help.** SkillsBench found self-generated *text* skills averaged −1.3pp. Programmatic skills checked by execution (ASI) beat text skills by 11.3%. Validation gates (VALVE) kept the library 11x smaller.
- **Runtime fit.** Go can host such programs in-process with real capability security. The most practical stack is wazero, with QuickJS-in-wasm or Extism on top. The WASM Component Model is not available in pure Go.
- **Proposed design.** A "program file" is:
  - a typed signature plus a capability manifest;
  - code that runs with zero ambient authority;
  - tests recorded from real runs.
- **How the agent uses it.** The agent writes small TypeScript snippets that call programs as functions. Every host call goes through the permission kernel. Only a size-capped, schema-checked return value reaches the model.

---

## 1. Code as the action: what saves context, and by how much

| Source | What saves context | Measured gain |
|---|---|---|
| **CodeAct** (Wang et al., ICML 2024; arXiv 2402.01030) | Actions are Python code, so the agent can loop, branch and compose. | Up to 20% higher success rate than Text/JSON actions, across 17 LLMs **[V]**. smolagents' README cites this paper for "30% fewer steps" **[V]**. I did not find that figure in the abstract. |
| **Anthropic, "Code execution with MCP"** (4 Nov 2025) | Each MCP server becomes a folder of typed TS files (`servers/google-drive/...`). The agent reads only the files it needs. Data is filtered in code, e.g. 10,000 rows filtered and only 5 logged. Intermediate results never reach the model. Sensitive fields can be tokenized before the model sees them. | 150,000 → 2,000 tokens, a 98.7% cut **[V]**. The post also lists costs: you need a sandbox, resource limits and monitoring **[V]**. |
| **Cloudflare Code Mode** (26 Sep 2025) | MCP schemas become a generated TypeScript API (`declare const codemode`). The model writes TS. A fresh V8 isolate runs each snippet and is then discarded. Global `fetch()`/`connect()` throw. The only external access is through bindings. The supervisor holds the API keys. | The post gives no benchmarks **[V]**. |
| **Cloudflare Code Mode MCP** (20 Feb 2026) | Only two tools: `search(code)` runs JS against the OpenAPI spec, which never enters context; `execute(code)` calls `cloudflare.request()`. | About 1,000 tokens covers more than 2,500 endpoints, versus 1.17M tokens for an ordinary MCP server, a 99.9% cut. The page's meta description says "over 2 million", so the source is inconsistent **[V]**. |
| **Claude tool search** (beta 24 Nov 2025; current docs) | `defer_loading: true` keeps tool schemas out of context until a regex or BM25 search finds them. You can plug in your own retrieval with `tool_reference` blocks. | 77K → 8.7K tokens (about 85%). Opus 4 went from 49% to 74%, Opus 4.5 from 79.5% to 88.1% on MCP evaluations **[V]**. The docs say selection accuracy "degrades once you exceed 30–50 available tools". Up to 10,000 deferred tools are allowed **[V]**. |
| **Claude programmatic tool calling** | A tool marked `allowed_callers: ["code_execution_20260120"]` is callable from Python in a container. Results go back to the code, not the model. | Launch post: 43,588 → 27,297 tokens (−37%); about 200KB of raw data reduced to about 1KB; GIA 46.5 → 51.2 **[V]**. Current docs: +11% average on BrowseComp/DeepSearchQA with 24% fewer input tokens; about 38% fewer on a 75-tool benchmark; on τ²-bench (sequential calls) no gain and about 8% *more* cost **[V]**. The docs say `allowed_callers` "is not a security boundary" **[V]**. |
| **smolagents CodeAgent** | Python code actions. | Its docs say `LocalPythonExecutor` "is not a security sandbox" and recommend E2B, Modal, Docker or Blaxel **[V]**. |

**What actually saves context [I]:**

1. **Intermediate data stays in the runtime.** Variables and handles replace pasting results back into the model. This is where the large ratios (98.7%, 99.9%) come from.
2. **Fewer inference passes.** One snippet replaces N round-trips. PTC: "20+ tool calls … eliminates 19+ inference passes" **[V]**.
3. **Interface disclosure is lazy.** The agent sees a signature line, not a schema.

The τ²-bench result matters. When calls are strictly sequential and each one needs the model's judgement, code mode costs more. So Gwen should keep plain direct calls for single steps.

## 2. Agents that make their own tools and skill libraries

| Work | Representation | How tools were validated | Result / lesson |
|---|---|---|---|
| **Voyager** (2023) | JS skill code in Minecraft, indexed by an embedding of the skill's *description*, top-5 retrieval **[V]** | GPT-4 acts as a critic that judges whether the program achieved the task. Environment feedback and execution errors also feed back **[V]** | 3.3x more items, 2.3x more distance, tech-tree milestones up to 15.3x faster **[V]** |
| **LATM** (2023) | Python function written by GPT-4 and used by GPT-3.5 | Unit tests on 3 validation samples. Only 2 of 60 trials were corrected from errors. A dispatcher picks the right tool 95% ±2% of the time **[V]** | Dyck language: 92.2 (LATM with GPT-3.5) vs 20.4 (CoT) vs 63.6 (GPT-4 CoT). GPT-3.5 as the *maker* succeeded 0/5 on two tasks: tool-making needs a strong model **[V]** |
| **CREATOR** (2023) | Separates creating a tool from using it | Not stated in the abstract | MATH, TabMWP, plus a 2K-question Creation Challenge **[V]** |
| **CRAFT** (ICLR 2024) | Toolsets per task | Validate solutions → abstract → **deduplicate** → retrieve **[V]** | "Substantial improvements" over baselines. The abstract gives no number **[V]** |
| **TroVE** (2024) | Toolbox of functions | Grow, use and periodically **trim** the toolbox **[V]** | Toolboxes 79–98% smaller; human verification 31% faster and 13% more accurate **[V]** |
| **ToolMaker** (2025) | Turns repos from papers into tools | Closed-loop self-correction; benchmark of 15 tasks with more than 100 unit tests **[V]** | 80% of tasks done correctly **[V]** |
| **Alita** (2025) | Generates task-specific MCPs from open-source code | Details not verified: arXiv rate-limited my fetch of the full text | GAIA 75.15% pass@1, 87.27% pass@3 **[V]** |
| **SkillWeaver** (2025) | Web agents turn practice into APIs | Propose, practise, then hone **[V]** | +31.8% on WebArena, +39.8% on real sites (relative). Skills from strong agents lift weaker agents by up to 54.3% **[V]** |
| **AWM** (2024) | Workflows stored as *text* | Induced offline or online | +24.6% Mind2Web, +51.1% WebArena (relative) **[V]** |
| **ASI** (2025) | Skills stored as *programs* | "Programmatic verification during induction", which it credits for most of the gain **[V]**. I did not verify the exact mechanism | +23.5% vs a static agent and **+11.3% vs text skills**; 10.7–15.3% fewer steps **[V]** |
| **ToolLibGen** (Oct 2025) | Clusters many one-off tools and refactors them into aggregated tools, with a reviewer agent | The reviewer checks that the merged tool keeps all original behaviour **[V]** | Names the bloat problem: a growing, unstructured library makes retrieval ambiguous **[V]** |
| **DGM** (2025) | The agent rewrites its own code | Benchmark-gated, with a sandbox and human oversight **[V]** | SWE-bench 20.0 → 50.0%, Polyglot 14.2 → 30.7% **[V]** |
| **AlphaEvolve** (May 2025) | Evolves programs | Automated evaluators; only applies where results can be "automatically verified" **[V]** | Recovers 0.7% of Google's compute; 23% kernel speedup **[V]** |
| **SkillsBench** (Feb 2026) | SKILL.md bundles | Deterministic verifiers | Curated skills: 33.9 → 50.5% (+16.6pp, v2). Focused skills with ≤3 modules beat exhaustive bundles. **Self-generated skills: −1.3pp on average.** Codex + GPT-5.2 fell 5.6pp; only Opus 4.6 gained (+1.4pp) (v1 text) **[V]** |
| **VALVE** (Sep 2026) | Text skill bank with a **validation gate** | Accepts an update only if it passes a holdout validation **[V]** | Stable over more than 1,000 SWE tasks; +14.9 points average; 75% less drawdown; library 11x more compact than ungated evolution **[V]** |
| **CoEvoSkills** (2026) | Multi-file skill packages | A surrogate verifier evolves alongside the skill generator **[V]** | Beats 5 baselines on SkillsBench; the abstract gives no numbers **[V]** |
| **SKILL.state** (Aug 2026) | Each step sees only the skill spec, a mutable structured state and the latest observation; reasoning is dropped after a validated state update **[V]** | – | Prompt size stays bounded. The abstract gives no numbers **[V]** |
| **SoK: Agentic Skills** (Feb 2026) | Survey | – | The "ClawHavoc" campaign planted about 1,200 malicious skills in a marketplace and stole keys and credentials **[V]** |

**Lessons [I]:**

1. **Verification decides the value.** Skills written from memory without execution, as in SkillsBench's self-generated condition, hurt slightly. Skills distilled from runs that succeeded and were checked (Voyager, ASI, LATM, VALVE) help.
2. **Programs beat prose** when the environment is scriptable (ASI's +11.3%).
3. **Libraries rot without trimming, merging and gating** (TroVE, CRAFT, ToolLibGen, VALVE's 11x).
4. **Making tools needs the strong model; using them can be cheap** (LATM). For Gwen: Claude writes programs, and a local model can call them.
5. **Imported skills are a supply-chain risk** (ClawHavoc).

## 3. Anthropic Agent Skills: where SKILL.md stops

What is loaded, and when **[V]**:

- **Level 1:** name and description, about 100 tokens per skill, always in the system prompt.
- **Level 2:** the SKILL.md body, under 5k tokens recommended, read when the skill triggers.
- **Level 3:** extra files cost nothing until accessed. Reference files enter context when read. Scripts run through bash: "the script code itself never enters context", only its output does.

Limits by surface **[V]**:

- **API:** no network access and no package installs at runtime.
- **Claude Code:** skills have "the same network access as any other program on the user's computer".

The open spec (agentskills.io) **[V]**:

- `scripts/` holds "executable code"; supported languages "depend on the agent implementation".
- There is **no input or output schema** for scripts.
- `allowed-tools` is marked experimental.
- Keep SKILL.md under 500 lines.

**Where it stops [I]:**

- A script has no typed signature. The agent learns how to call it by reading prose.
- Output is whatever the script prints. Its size and shape are not controlled.
- A script runs as a whole process with the shell's ambient authority. Nothing gates it per call.
- Scripts cannot be composed as functions in memory. Chaining goes through bash and text.
- There are no tests or provenance in the format.

Scripts already keep their *code* out of context. What SKILL.md lacks is a typed interface and capability control. The owner's "program files" fill exactly that gap.

## 4. In-process runtimes Gwen (Go) could use

| Runtime | Maturity / license | Capability model | Familiar to LLMs | Hot reload | Notes |
|---|---|---|---|---|---|
| **wazero** | v1.12.0 (28 May 2026), Apache-2.0, pure Go, no CGO; Wasm Core 1.0/2.0 **[V]** | **No authority by default.** No filesystem unless `FSConfig` grants paths; stdout discarded; stdin returns EOF; clocks are fake until supplied **[V]**. All I/O goes through host functions you write. | Indirect (code is compiled to wasm) | Compile, then instantiate per call; easy to swap **[I]** | Compiler mode is "10x or more" faster than the interpreter **[V]**. Supports only `wasi_snapshot_preview1` **[V]**. Component Model: "definitely no until it is standardized in the w3c recommendation" (mathetake, issue #2200) **[V]** |
| **Extism Go SDK** | BSD-3-Clause, built on wazero; pkg.go.dev showed v1.7.1 (2 Mar 2025) with a newer version available; the exact latest is not verified | Manifest fields `AllowedHosts`, `AllowedPaths`, `Memory{MaxPages, MaxHttpResponseBytes, MaxVarBytes}`, `Timeout`, `Config`; host functions via `NewHostFunctionWithStack` **[V]** | Plugins can be written in many languages; XTP bindgen generates typed bindings for TS, Go, Python, Rust, C#, Zig, C++ **[V]** | Yes **[I]** | JS PDK: QuickJS-ng plus Wizer, up to ES2020, "no event loop, no I/O", synchronous `fetch` **[V]**. Maintainers: no Component Model support "in any near-term future" **[V]** |
| **fastschema/qjs** (QuickJS-ng in wazero) | MIT, Go ≥1.22, no releases tagged **[V]** | `MemoryLimit`, `MaxExecutionTime`, `MaxStackSize`; filesystem and network off unless allowed; Go functions exposed with `SetFunc`/`SetAsyncFunc` **[V]** | High (JS, ES2023, async/await) **[V]** | Yes | Self-reported benchmark: 746ms vs goja 1.127s **[V]**. Young project **[I]** |
| **goja** | MIT, about 7.1k stars **[V]** | Only the globals you inject; `vm.Interrupt()` for timeouts **[V]** | Medium: ES5.1, with "most ES6 … still in progress" **[V]** | Yes | Not goroutine-safe; no event loop **[V]**. LLM-written modern TS needs downlevelling **[I]** |
| **esbuild Go API** | MIT, v0.28.2 seen **[V]** | – | – | – | `api.Transform` turns TS into JS in-process **[V]**. Lets Gwen accept TS source **[I]** |
| **Starlark-go** | BSD-3-Clause, module updated 5 Oct 2026 **[V]** | Deterministic (no clocks or randomness); values frozen after module load; recursion off by default; `SetMaxExecutionSteps` and `Cancel` **[V]** | High (Python-like) | Yes | Best for deterministic recipes and policy glue; no library ecosystem **[I]** |
| **Yaegi** (Go interpreter) | Apache-2.0, labelled "alpha" **[V]** | `unsafe` and `syscall` not exported by default; whatever you `Use()` is exposed **[V]**. No sandbox claim | High (Go) | Yes | Go modules not supported; slow on CPU-heavy code **[V]**. Exposing stdlib gives full OS access, so too risky for self-written code **[I]** |
| **Risor v2** (`deepnoodle-ai/risor/v2`) | Apache-2.0, 909 stars **[V]** | Environment starts empty; "secure by default"; no imports **[V]** | Medium (JS-like syntax) | Yes | Small community **[I]** |
| **gopher-lua** | MIT, Lua 5.1 **[V]** | `SkipOpenLibs`; `SetContext` for cancellation **[V]** | Medium | Yes | Not goroutine-safe **[V]** |
| **expr** | MIT **[V]** | Side-effect free, "always terminate" **[V]** | High | – | Use it for guards and policy predicates, not programs **[I]** |
| **Go `plugin`** | stdlib | None. Plugins run in-process with full rights | High | **No unloading**; toolchain, build tags and flags must match exactly or it crashes; Linux, FreeBSD and macOS only **[V]** | Reject **[I]** |
| **Deno subprocess** | – | `--allow-read=/path`, `--allow-net=host`, `--deny-*`; a refused operation throws `NotCapable`. **`DENO_PERMISSION_BROKER_PATH` sends every permission decision to an external process** **[V]** | Highest (TS + npm) | Yes | `--allow-run` and `--allow-ffi` count as allow-all; untrusted code also needs OS sandboxing **[V]** |
| **Node / Bun** | – | Node's permission model "does not protect against malicious code" (a "seat belt") **[V]**. A third-party design note says only Deno has a real permission model and bun runs with full authority **[V]**. I did not check this in Bun's own docs | Highest | – | Not a security boundary **[I]** |
| **srt** (Anthropic sandbox-runtime) | Apache-2.0, beta **[V]** | bubblewrap, no network namespace, proxy with a domain allowlist, seccomp blocks Unix sockets, writes allow-only **[V]** | Any language | – | Wraps any subprocess (Python, CLIs) **[V]** |

**WASI and Component Model status [V]:**

- WASI 0.3.0 shipped 11 June 2026 and 0.3.1 on 11 August 2026, with native `async func`, `stream<T>` and `future<T>`.
- Wasmtime 46+ supports it.
- None of this works in wazero.

So typed wasm interfaces in pure Go mean JSON over linear memory (the Extism style) plus JSON Schema, not WIT **[I]**. Wasmtime-via-cgo would be needed for WIT. I did not verify wasmtime-go's component support.

Go 1.24 added `go:wasmexport` and `-buildmode=c-shared` reactors for `wasip1` **[V]**. So Gwen can compile Go helpers into wazero modules **[I]**.

## 5. Typed program interfaces

- **MCP 2025-06-18** **[V]**:
  - Tools carry `inputSchema` and an optional `outputSchema`.
  - When a tool declares `outputSchema`, its `structuredContent` MUST conform, and clients SHOULD validate it.
  - Clients MUST treat tool annotations as untrusted unless the server is trusted.
- **Code Mode** turns schemas into TS `interface`s with JSDoc comments **[V]**. Anthropic's post uses one TS file per tool **[V]**.
- **Claude tool definitions** take `input_examples` for conventions a schema can't express: 72 → 90% on complex parameters **[V]**.
- **Gwen's catalog line [I].** Generate one TypeScript signature line per program from its JSON Schema and keep it to 25–40 tokens. Example:

  `spotify.queuePlaylist(q:{mood:string;minutes?:number}):{playlistId:string;tracks:number;playing:boolean} // build+play a playlist`

  The full `.d.ts` file (with JSDoc and examples) stays on disk for the agent to read on demand.

## 6. Implications for Gwen

### 6.1 What a program file is [I]

A git-tracked directory under `~/.local/share/gwen/programs/<ns>.<name>/`:

```
program.toml     manifest (Gwen already depends on BurntSushi/toml)
main.ts          source (TS; esbuild-transform in-process) | main.wasm | main.star | run.py
types.d.ts       generated from manifest schemas; what `describe()` returns
tests/*.json     recorded host-call traces + expected outputs (replayable)
NOTES.md         optional SKILL.md-style prose: pitfalls, when-to-use; loaded only on failure or ask
```

```toml
name = "spotify.queuePlaylist"   version = "0.3.1"   runtime = "js"   # js | wasm | star | proc | remote
summary = "Build a playlist for a mood and start playback"
input  = { type="object", required=["mood"], properties={ mood={type="string"}, minutes={type="integer"} } }
output = { type="object", properties={ playlistId={type="string"}, tracks={type="integer"}, playing={type="boolean"} } }
calls  = ["spotify.search", "spotify.playlist.create", "spotify.player.play", "kv.get", "kv.put"]  # host functions only
effect = "reversible-write"      # kernel tool class
budget = { wall_ms=20000, mem_mb=64, host_calls=200, return_bytes=2048 }
[provenance] author="gwen"  from_run="brief_0192"  status="probation"  tests_sha="…"  approved_by=""
```

Compared with SKILL.md, this adds:

- a typed signature;
- a declared and enforced capability set;
- an effect class the kernel understands;
- budgets;
- tests;
- provenance.

### 6.2 Runtime tiers [I]

| Tier | Runtime | Use for | Isolation | Cost |
|---|---|---|---|---|
| **T0 (default)** | QuickJS-in-wazero (qjs), with goja as fallback; TS via esbuild | Glue and composition; most programs Gwen writes herself | Linear-memory sandbox; **no** I/O except `gwen.*` host functions | In-process, per-call instance, ms-scale (not measured) |
| **T0′** | Starlark | Deterministic recipes, triggers, policy-adjacent logic | Deterministic, step-limited | Lowest |
| **T1** | wazero/Extism wasm modules (Go via `wasip1`, Rust, Extism JS PDK) | Compute helpers: parsers, file formats, diffing | Same as T0; `AllowedPaths` and `AllowedHosts` cut to the manifest | Needs a compile step, so slower for Gwen to iterate on |
| **T2** | Subprocess under bwrap/srt, or **Deno with gwend as the permission broker** | Real ecosystems: Python libs, Playwright, LibreOffice UNO, `ydotool`, CLIs | OS sandbox from the v1 profiles plus the egress proxy | Process startup; full ambient authority unless sandboxed |
| **T3** | Remote: stacyvm (Firecracker, gVisor or Docker providers; REST, exec, file ops; Apache-2.0; early, 9 stars **[V]**) or the Claude code-execution container | Heavy, untrusted or cloud jobs; the "laptop to cloud" flow | microVM | Network, plus stacyvm's own maturity risk |

**Trade-offs [I]:**

- **WASM** gives the strongest authority story: an import list *is* the capability list. But WIT/components are not available in Go, and compiled languages slow down Gwen's write-test loop.
- **Embedded JS** is what LLMs write best, and what Code Mode and Anthropic validated. It has the same zero-authority property *if* it runs inside wazero (qjs). goja is pure Go but runs in Gwen's own memory, so a goja bug is a gwend bug.
- **Subprocess** gives the full ecosystem but needs OS sandboxing. Deno's permission broker lets the kernel answer every `read`, `net` and `run` request. That is the cleanest T2 route.

Start with T0 + T2 (Deno/bwrap). Add T1 only when a program needs heavy compute.

### 6.3 How the agent invokes and composes them [I]

The model gets three meta-tools, adapted from Code Mode and Cloudflare's search/execute:

1. **`programs.find(query)`** returns catalog lines only. Retrieval is BM25 plus embeddings over summaries and signatures, as with Claude's `tool_reference`. Keep about 10 always-on lines and defer the rest, since accuracy drops past 30–50 tools.
2. **`programs.run(code)`** takes a TS snippet. In it, every program and host primitive is an async function on `gwen` (`await gwen.spotify.queuePlaylist({mood:"focus"})`). Loops, filtering and joins happen in the snippet.
3. **Direct call `program(name, args)`** for single sequential steps. This avoids the τ²-bench cost penalty.

Programs can call other programs. The kernel checks the callee's caps against the *intersection* of the caller's manifest and the brief scope, so a program never widens what its caller may do.

### 6.4 Keeping results out of context [I]

- **Return budget.** `return_bytes` is enforced. Anything larger becomes a **handle** (`h:run/8f2/rows`) in a per-brief blackboard; the model sees `{handle, count, schema, 3-row preview}`. Later snippets take the handle as input. This follows Anthropic's "filter in code, log five rows".
- **Validated output.** The output is checked against the `output` schema. A violation returns a compact error code, while the full trace goes to the ledger, not the model.
- **Private data.** PII fields are tokenized before the model sees them and restored on the way into host calls, following the Anthropic post.
- **Long briefs.** Use the SKILL.state pattern: the model sees the brief spec, a structured state object maintained by programs, and the latest observation, instead of a growing transcript.

### 6.5 How Gwen writes and verifies new programs [I]

The loop combines ASI, LATM, VALVE and TroVE:

1. **Distil, don't imagine.** Program candidates come only from `run(code)` snippets that succeeded in real briefs. Gwen parameterizes the snippet and derives the schemas from observed I/O. SkillsBench's −1.3pp warns against skills written from memory.
2. **Record and replay.** Every host call in the source run is recorded and becomes a test with mocked host functions. Tests run in T0 with **no** side effects. Add 2–3 edge cases that Claude generates.
3. **Maker vs. user.** Claude (subscription/CLI) writes and repairs programs. Local models may *call* them, following LATM's strong-maker, cheap-user split.
4. **Probation, then promotion.** The first N live uses are observed or require approval. Promotion needs the holdout to pass, a VALVE-style gate. The owner sees a diff and test report in the UI.
5. **Library hygiene.** A weekly job:
   - trims unused programs (TroVE);
   - merges near-duplicates with a reviewer check (ToolLibGen/CRAFT);
   - bumps the version on any signature change (callers pin `name@major`).
6. **Imports are untrusted.** SKILL.md imports and MCP servers become `runtime="proc"` (T2) or are wrapped as generated TS bindings (Code Mode). They never get T0 trust without tests and review (ClawHavoc).

### 6.6 Permission kernel integration [I]

- **One door.** Every `gwen.*` host function is a gateway call. The manifest's `calls` is a static allowlist checked when the program loads. The pure policy decides each call using the payload, the brief scope and the effect class. Payload-bound approvals and the hash-chained ledger apply unchanged.
- **No ambient authority.** In T0/T1 there is no `fetch`, filesystem or env. Network goes through a `http.fetch` host function behind the egress proxy. Secrets are injected by the secrets broker in the host function and never enter the sandbox, as in Code Mode.
- **Limits are enforced, not advisory.** Budgets (wall time, memory, host-call count) are enforced by the runtime. Claude's `allowed_callers` is explicitly not a security boundary, so Gwen's equivalent must be.
- **Apps become host-function families.** Examples: `dbus.call` (MPRIS for Spotify playback, KDE), `vscode.*` (via an extension bridge), `office.*` (UNO), `ui.*` (AT-SPI). Programs then wrap these into tasks like "make a focus playlist and play it". This links to the owner's "embedded apps" ask; the app bridges themselves belong to another track.

### 6.7 Open risks and gaps

- qjs is young. Keep goja behind the same interface as a fallback **[I]**.
- No Component Model in Go means hand-rolled JSON ABI and schema drift risk **[I]**.
- Not verified here:
  - ASI's exact verification mechanism;
  - Alita's MCP Box internals (arXiv rate-limited the full-text fetch);
  - goja's async/await status;
  - Bun permissions from Bun's own docs;
  - the newest Extism Go SDK version.

---

## Sources

- https://arxiv.org/abs/2402.01030
- https://github.com/xingyaoww/code-act
- https://www.anthropic.com/engineering/code-execution-with-mcp
- https://blog.cloudflare.com/code-mode/
- https://blog.cloudflare.com/code-mode-mcp/
- https://www.anthropic.com/engineering/advanced-tool-use
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/programmatic-tool-calling
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool
- https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview
- https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills
- https://agentskills.io/specification
- https://github.com/huggingface/smolagents
- https://huggingface.co/docs/smolagents/tutorials/secure_code_execution
- https://arxiv.org/abs/2305.16291
- https://voyager.minedojo.org/
- https://arxiv.org/abs/2305.17126
- https://arxiv.org/html/2305.17126v2
- https://arxiv.org/abs/2305.14318
- https://arxiv.org/abs/2309.17428
- https://arxiv.org/abs/2401.12869
- https://arxiv.org/abs/2502.11705
- https://arxiv.org/abs/2505.20286
- https://arxiv.org/abs/2504.07079
- https://arxiv.org/abs/2409.07429
- https://arxiv.org/abs/2504.06821v2
- https://arxiv.org/abs/2510.07768
- https://huggingface.co/papers/2505.22954
- https://deepmind.google/discover/blog/alphaevolve-a-gemini-powered-coding-agent-for-designing-advanced-algorithms/
- https://arxiv.org/abs/2507.21046
- https://arxiv.org/abs/2602.12670v2
- https://arxiv.org/html/2602.12670v1
- https://arxiv.org/abs/2602.20867
- https://arxiv.org/abs/2604.01687
- https://arxiv.org/abs/2605.17734
- https://arxiv.org/abs/2608.26263
- https://arxiv.org/abs/2609.32990
- https://modelcontextprotocol.io/specification/2025-06-18/server/tools
- https://github.com/tetratelabs/wazero
- https://pkg.go.dev/github.com/tetratelabs/wazero
- https://wazero.io/specs/
- https://github.com/wazero/wazero/issues/2200
- https://wasi.dev/releases/wasi-p3
- https://github.com/extism/go-sdk
- https://pkg.go.dev/github.com/extism/go-sdk
- https://extism.org/docs/concepts/plug-in-system
- https://github.com/extism/js-pdk
- https://github.com/extism/extism/issues/666
- https://github.com/dylibso/xtp-bindgen
- https://github.com/fastschema/qjs
- https://github.com/dop251/goja
- https://pkg.go.dev/github.com/evanw/esbuild/pkg/api
- https://github.com/traefik/yaegi
- https://github.com/google/starlark-go
- https://github.com/google/starlark-go/blob/master/doc/spec.md
- https://pkg.go.dev/go.starlark.net/starlark
- https://github.com/risor-io/risor
- https://github.com/expr-lang/expr
- https://github.com/yuin/gopher-lua
- https://pkg.go.dev/plugin
- https://go.dev/doc/go1.24
- https://docs.deno.com/runtime/fundamentals/security/
- https://nodejs.org/api/permissions.html
- https://github.com/dezren39/nix/issues/299
- https://github.com/anthropic-experimental/sandbox-runtime
- https://github.com/StacyOS/stacyvm
- https://notesbylex.com/self-generated-skills-dont-help