# Gwen program files: a design for executable skills that load into gwend

Labels: **[V]** means verified in a cited source, either by me in this pass or in the attached research reports. **[I]** means my own design judgement.

## 0. Decisions in one screen

- **What a program file is.** A program file is a directory, identified by its content hash. It holds:
  - `program.toml`: a typed signature, a capability ceiling, budgets and provenance;
  - TypeScript code;
  - replay tests;
  - an optional prose note of 150 tokens or fewer.

  The kernel reads authority only from the TOML. It never runs code to learn what a program may do. [I]
- **Primary runtime.** TypeScript is transformed in-process by esbuild, then runs on **QuickJS-ng inside wazero** (`fastschema/qjs`) in gwend's own process. It has zero ambient authority. The only door out is `gwen.*` host functions, and each one is a `kernel.Invoke`. [I]
- **Fallback runtime.** The same TS source runs in a **Deno subprocess under bwrap, with gwend as Deno's permission broker**. This tier is also where probation and imported programs run. [I]
- **What the agent sees.** About 10 one-line signatures, plus four meta-tools: `find`, `describe`, `run(code)` and `call`. Large results come back as **handles**, never as data. [I]
- **One interface for everything.** MCP servers, CLIs, wasm helpers and app bridges (MPRIS, KWin, VS Code, UNO, AT-SPI, the computer-use loop) all become `gwen.*` capabilities. Programs compose them, and MCP survives only as hidden plumbing. [I]
- **How programs are born.** New programs are distilled from runs that worked, never imagined. They are replay-tested in both runtimes, gated by the kernel evaluator, approved by their ceiling, and kept versioned, signed and retirable. [I]
  - SkillsBench found that self-generated text skills cost −1.3pp on average. ASI found programmatic skills checked by execution beat text skills by 11.3%. [V]
- **Cross-device.** Programs are location-transparent; host functions are location-bound. A program running on the Pi can call `place.*` on the laptop and `sandbox.*` on a stacyvm cloud node. Each node's kernel decides its own calls. [I]

Why this is a real change from SKILL.md:
- Agent Skills scripts already keep their *code* out of context. But they have no input or output schema, no per-call gate, and no output control; whatever they print is what the model sees. [V]
- Code-as-action plus handles is where the large savings come from. Anthropic measured 150,000 → 2,000 tokens (98.7%) [V]. Cloudflare covers more than 2,500 endpoints in about 1,000 tokens [V].
- Anthropic's post already sketches "persist their own code as reusable functions" in `./skills/*.ts` with a SKILL.md [V]. Gwen adds what that sketch lacks: types, capabilities, tests and governance.

---

## 1. File format and manifest

```
~/.local/share/gwen/programs/            # git repo; tag per name@version
  spotify.make_playlist_and_play/
    program.toml   main.ts   lib/*.ts|*.wasm   tests/*.test.ts   tests/fixtures/*.trace.json   NOTES.md
```

- **Identity.** A program is identified by the sha256 of a canonical tar of its directory.
- **Transport.** It moves between nodes as `.gwp`, which is that tar plus the hub's signature.

**Signature as a type string [I].** `input`/`output` are TypeScript-shaped type strings, for example `"{ mood: string, minutes?: int(10..240) }"`. A small Go parser compiles them to JSON Schema. The grammar covers primitives, `int(a..b)`, `T[]`, object fields with `?`, unions of string literals, and `Handle<kind>`.

From one source the loader gets three things:
- **validation** of arguments and return values;
- a **generated `types.d.ts`**, which is what `describe` returns;
- the **catalog line**, about 25 to 40 tokens.

It is compact enough for a model to write correctly, and unlike `.d.ts` → schema it needs no TypeScript compiler. Claude's `input_examples` raised accuracy on complex parameters from 72% to 90% [V], so `[[example]]` entries are part of the manifest too.

**`uses` is the ceiling [I].** It lists host functions, optionally narrowed by resource: `http.fetch@api.spotify.com`, `sandbox.open@cloud`, `fs.read@place:foo`.
- At load time, `uses` must be a subset of the installed capability registry.
- At runtime, every host call must match `uses` ∩ the brief's scope before policy even runs.
- `auth` names secrets. Their values are injected by the broker inside the host function and never enter the guest. Cloudflare's Code Mode works the same way: its supervisor holds the keys [V].

---

## 2. Runtime choice

| Option | Authority model | Fit for LLM-written code | Status | Role |
|---|---|---|---|---|
| **QuickJS-ng in wazero** (`fastschema/qjs`) | Filesystem and network off "unless explicitly configured"; `MemoryLimit`, `MaxExecutionTime`, `MaxStackSize`; Go exposed only through `SetFunc`/`SetAsyncFunc`; `NewPool` [V] | ES2023, modules, async/await [V]; TS after esbuild `api.Transform` [V] | MIT; latest tag **v0.0.6**; about 610 stars [V]. Young | **Primary.** Promoted programs and agent snippets |
| **Deno subprocess + bwrap** | With `DENO_PERMISSION_BROKER_PATH` set, "all `--allow-*` and `--deny-*` flags are ignored", every check goes to the broker, and a broker failure terminates the process [V]. `--allow-run`/`--allow-ffi` count as allow-all [V], so bwrap stays underneath | Full TS and npm | Mature | **Fallback** and **probation tier**. Same source, `gwen.*` shimmed over `/run/gwen/bridge.sock` |
| wazero / Extism wasm | Import list = capability list; no I/O by default [V]. Extism `AllowedHosts`/`AllowedPaths` [V]; Go SDK latest tag v1.7.1 [V] | Compiled languages, so a slow write/test loop | wazero v1.12.0; no Component Model in pure Go [V] | `lib/*.wasm` **compute helpers** (parsers, diffing), callable from TS |
| goja | Only injected globals [V] | README: "Full ECMAScript 5.1", "Most of ES6 … still work in progress", says nothing about async [V] | MIT, 7.1k stars; one goroutine per Runtime [V] | Reserve only, if qjs is abandoned |
| Starlark | Deterministic, step-limited [V] | "No need for classes, exceptions"; `while` only with `-recursion` [V]. Python habits fail [I] | BSD-3 | Not for programs; possibly for trigger predicates |
| Yaegi, Go `plugin`, Node/Bun | Yaegi: whatever you `Use()` is exposed. `plugin`: no unloading. Node's permission model "does not protect against malicious code" [V] | High | – | Rejected |

**Why TypeScript and not Python [I].**
- No pure-Go, in-process, capability-sandboxed Python exists at qjs's level.
- TS gives a `.d.ts` interface the model reads natively. Cloudflare and Anthropic both generate TS APIs for code mode [V].

Python still runs as `runtime="proc"` (T2), for the cases that need its ecosystem.

**Keeping the fallback honest [I].** Every program's tests run in both qjs and Deno before promotion. If qjs regresses, a program flips to the Deno tier without a rewrite.

**Defence in depth [I].** Code that was LLM-written or imported, and not yet trusted, runs out of process. Only stable programs run inside gwend.

---

## 3. Host API and the kernel path

The `gwen` global is all a guest ever sees.

```ts
gwen.http.fetch({url, method?, query?, json?, auth?})   gwen.fs.read|list|write|edit({place, path…})
gwen.shell.run({profile, argv})                          gwen.place.snapshot|apply_bundle(...)
gwen.app.<adapter>.<verb>(...)   // mpris, kwin, vscode, office, atspi, input, browser, cu
gwen.mcp.<server>.<tool>(...)    gwen.sandbox.open|put_bundle|exec|get_bundle(...)
gwen.memory.recall|note(...)     gwen.llm.json({tier, prompt, input?: Handle, schema})
gwen.prog["name"](args)          gwen.handle.put|peek|slice(...)   gwen.state.get|set(...)
gwen.kv.get|put (program-private) gwen.clock.now|today|sleep   gwen.notify.push   gwen.share.publish   gwen.log
```

Every host call goes through the same path [I]:

1. **Ceiling check.** Match `Fn@resource` against `uses` ∩ brief scope. A miss returns `E_UNDECLARED`; it is logged as a program bug and never asks the owner.
2. **Budget.** Count against host calls, wall time, LLM tokens and network bytes.
3. **Handle resolution.** `{"$h":"h_…"}` arguments become data on the host side. The call's taint becomes the union of the run's taint and the handles' taint.
4. **`kernel.Invoke`.** The call carries `Caller: "agent→repo.review_diff_on_phone@1.2.0→text.render_diff@1.0.3"`, the program's status (probation or stable), the brief and the lease epoch. The capability's own classifier builds the `Action`; the v1 policy, hard floor, `DestTaint`, outbox and ledger apply unchanged.
5. **When the decision is `ask`.** The guest's `await` stays suspended for up to `approval_wait` (default 10 minutes). After that the invocation ends `pending` and is resumed later (§6).
6. **Return.** A large result becomes a handle; taint labels travel with it.

`gwen.llm.json` lets a program use a small model on data that **never enters the main context**: picking tracks, triaging 200 diff hunks, extracting fields. That is the main lever for long-context work [I].

---

## 4. What the agent sees

**Pinned index [I].** About 10 pinned lines, chosen per brief template and by use. Everything else stays deferred. Claude's docs say tool-selection accuracy "degrades once you exceed 30–50 available tools" [V].

```
spotify.make_playlist_and_play({mood, minutes?, seed?, device?}) → {playlist, tracks, playing, via} · make+play a mood playlist
repo.review_diff_on_phone({place, base?, editor?, ttl_h?}) → {url, files, additions, deletions} · send working diff to phone
```

**Meta-tools:**

| Tool | What it does |
|---|---|
| `programs.find(q)` | BM25 plus embeddings over summaries and signatures; returns lines |
| `programs.describe(names)` | `.d.ts` slices plus examples |
| `run(code)` | A TS snippet against `gwen.*` and `gwen.prog.*`; loops, joins and filters happen there |
| `call(name, args)` | A plain single call. Programmatic tool calling showed no gain and about 8% more cost on sequential τ²-bench work [V], so single steps stay direct |

**What comes back to the model:**
- the return value, schema-checked and at most `return_bytes`;
- one status line;
- log output of at most 1 KB.

Anything larger becomes `{h, kind, bytes|rows, schema, preview(3), taint}`. The model can `handle.peek(h, {query|rows|lines})`, or pass `h` into the next snippet. Handles are unguessable, scoped to one run, expire with the run plus 24 hours, and are content-addressed so another node can fetch them through the hub [I].

**Long runs [I].** Programs and snippets write to `gwen.state`, a structured per-brief object. Each turn the harness renders the brief, the state (bounded), the index and the last observation, and drops old tool transcripts. This is the SKILL.state pattern [V]. Programs are what keep the state small and accurate.

**Claude Code as an engine [I].** `gwen mcp` exposes exactly these four meta-tools, so MCP exists only at the edge, for engines that require it.

---

## 5. Composition and wrapping

**Programs calling programs [I].**
- A caller declares the edge (`requires = ["text.render_diff@^1"]`) and calls `gwen.prog["text.render_diff"](…)`.
- The callee runs in its **own** qjs instance with its **own** ceiling ∩ brief scope, so it is isolated by memory and authority.
- Data passes as JSON or handles, which is an object-capability model.
- The loader computes the **transitive ceiling**. The caller's approval card shows the total reach, so the owner approves everything a call could do.
- Recursion depth is limited to 4. Cycles are rejected at load.

**MCP servers [I].**
- A `runtime="mcp"` package names the command and its bwrap profile.
- The loader starts the server, runs `tools/list`, and registers each tool as capability `mcp.<server>.<tool>`, with a signature generated from `inputSchema`/`outputSchema` and the kernel's cautious default class. Annotations are hints only, because clients MUST treat them as untrusted [V].
- The agent never sees MCP schemas, and large results become handles.

**App adapters** are native Go capability families inside gwend:
- `app.mpris.*` and `app.kwin.*` over godbus;
- `app.vscode.*` over the companion extension's 0600 socket, the Claude Code IDE design [V];
- `app.office.*` through unoserver (T2);
- `app.atspi.*` and `app.input.*` through the RemoteDesktop portal [V];
- `app.browser.*` through agent-browser or the Playwright CLI (T2).

**Computer use is one capability [I].** `app.cu.do({app, goal, until})` runs the vision loop in a sub-context, with screenshots, `zoom` and around 10 s per step [V]. Only a result line and a recorded trace come back. When the same verb has gone through `cu` or AT-SPI N times, Gwen proposes compiling the trace into a deterministic adapter program, the Stagehand and Terminator replay pattern [V].

**CLIs and Python** use `runtime="proc"`: one JSON line in, one out, with host calls over the bridge socket, under the manifest's bwrap profile [I].

---

## 6. Lifecycle

**States [I]:** `draft → candidate → probation → stable → deprecated → archived`, plus `quarantined`.

1. **Trigger.**
   - A `run(code)` snippet succeeds and a similar host-call set has been seen at least twice;
   - the owner says "remember how to do this";
   - the adapter ladder reaches its compile threshold; or
   - a new MCP server or CLI arrives. Wrapping generates bindings only, no new code.
2. **Spec from the trace.**
   - The recorded host calls become `uses`: exactly the functions and resources observed, widened only by the owner.
   - Observed I/O becomes the type strings and examples.
3. **Code.** The strong model writes `main.ts` and tests in a scratch place, through `code.delegate`. LATM found tool-making needs the strong model while a cheap model can use the result [V].
4. **Tests.**
   - Replayed traces, plus two or three generated edge cases.
   - Every test asserts the **complete effect set** with `t.effects`. An extra host call fails the test, which is a security property, not only a correctness one.
   - They run in qjs and Deno with mocked host functions only: zero real effects.
5. **Eval.**
   - The kernel evaluator, which Gwen cannot edit, replays held-out briefs with the program available and without it.
   - It must not regress (the VALVE gate gave an 11x smaller library [V]), and it records tokens and steps saved.
6. **Promotion card.** Shows the signature, the ceiling as a plain table, the transitive reach, a diff against the previous version, test and eval results, and provenance.
   - The owner approves the **ceiling**, and the code is shown.
   - Probation runs in T2, and effectful calls ask the first three times.
   - A patch release with the same or a narrower ceiling and green tests auto-promotes with a notice. Widening the ceiling always needs approval.

**Versioning and rollback [I].**
- Changing the signature bumps the major version; callers pin `@^major`.
- Rollback repoints `stable` to the previous hash, in one click.
- Every version stays in the git repo and in a `programs` table holding the hash, status, ceiling and statistics.

**Provenance [I].** A signed record in the ledger: `{hash, author: gwen|owner|import(url@commit), model, from_run, parent, tests_hash, eval_id, approval_id}`, signed by the hub key. Nodes refuse unsigned packages. Imports start `quarantined` in T2; the ClawHavoc campaign planted about 1,200 malicious skills [V].

**Durable execution [I].**
- Every host call is journaled by `(invocation, seq)`, and effectful calls carry `idem = H(program hash, invocation, seq, args hash)`.
- `Resume` re-executes the program. Journaled calls return their recorded result instead of re-running.
- Clock and randomness come from the host, so replay is deterministic.
- This one mechanism gives three things: resume after approval, resume after a crash, and **moving a paused invocation to another node**.

**Health and retirement [I].**
- Per version, Gwen tracks success rate, schema violations, error codes, owner corrections and tokens returned.
- Adapters export a read-only `canary`, which runs daily. Replay tests catch logic regressions but not upstream drift.
  - Example: Spotify's February 2026 changes removed `POST /users/{id}/playlists`, renamed `/tracks` to `/items`, and cut search `limit` from 50 to 10 [V]. Any older program would break; a canary catches it and opens a repair draft.
- When the error threshold is crossed, the program falls back to the previous version, or is quarantined and removed from the index with a growth-backlog item added.
- Programs unused for 90 days are archived.
- Near-duplicates are merged with a reviewer that runs both programs' tests (ToolLibGen/TroVE) [V].

---

## 7. Cross-device execution [I]

- **Where calls run.** Each capability has a `Home()`:
  - **laptop:** `place.*`, `app.*`, `fs@place`;
  - **cloud:** `sandbox.*@cloud`;
  - **hub (Pi):** `notify`, `share`, `memory`, `llm`.
- **Where the program runs.** The placer picks the node that minimises remote host calls, filtered by which nodes are online (tsnet `Status`) and by `placement.prefer`.
- **Remote calls.** A host call homed elsewhere goes as RPC over tsnet to that node's `gwend /kernel/v1/call`, carrying the caller chain, brief and lease epoch. **The receiving kernel classifies and decides again.** Both ledgers record the call.
- **stacyvm.** A T2 program on the cloud runs in a stacyvm sandbox. Its host calls travel over the bridge socket to the cloud gwend. This needs stacyvm changes #4 (archive in and out), #7 (bridge) and #13 (warm pools) [V gaps].
- **Laptop asleep.** A laptop-homed call ends the invocation `pending` and sends the "open lid / use mirror / queue" ntfy choice from the cross-device report. Journal resume continues the invocation on any node later.

---

## 8. What stays prose

Prose is for **judgement**; code is for **anything a test can check** [I].

- **Stays prose:** the persona, attitudes, and the playbook (routing, review style, tone).
- **Goes to memory:** taste and preferences, such as "no vocals while coding". Programs *query* memory; they don't hard-code it.
- **`NOTES.md` (150 tokens or fewer per program):** when to use it and pitfalls that can't be checked. Loaded only on `describe` or after a failure.
- **Short prose recipes:** procedures where every step needs the model's decision. Code mode doesn't pay off there (τ²-bench), so these call programs step by step.
- **Never prose:** how to call a program. That is the signature.

---

## 9. Example: `spotify.make_playlist_and_play`

```toml
name = "spotify.make_playlist_and_play"
version = "1.0.0"
runtime = "ts"
entry = "main.ts"
summary = "Pick tracks for a mood, make a private playlist, start playback here"
input  = "{ mood: string, minutes?: int(10..240), seed?: string[], device?: 'laptop'|'any' }"
output = "{ playlist: { id: string, uri: string, name: string }, tracks: int, unresolved: int, playing: bool, via: 'mpris'|'webapi' }"
effect = { reach = "account", undo = "reversible" }      # private playlist + local playback
uses = ["http.fetch@api.spotify.com", "app.mpris.has", "app.mpris.open_uri", "app.mpris.status",
        "llm.json", "memory.recall", "kv.*", "clock.*"]
auth = ["spotify"]                                       # scopes: playlist-modify-private, user-modify-playback-state
[placement]
prefer = "laptop"
[budget]
wall_ms = 45000
host_calls = 120
mem_mb = 64
llm_tokens = 6000
return_bytes = 1024
[[example]]
args = { mood = "deep focus, no vocals", minutes = 90 }
[provenance]
author = "gwen"
from_run = "run_01JA…"
status = "probation"
tests = "sha256:…"
policy_flag = "spotify-voice-clause"   # owner must accept; see NOTES.md
```

```ts
// main.ts
import { chunks, expect, ProgramError, title } from "./lib/util";
const API = "https://api.spotify.com/v1";
type Pick = { artist: string; title: string };

export default async function main(a: Args): Promise<Result> {
  const minutes = a.minutes ?? 60;
  const taste = await gwen.memory.recall("music taste, dislikes", { k: 5 });
  // Picks are made by a small model inside the runtime; they never reach the main context.
  // Spotify removed Recommendations for new apps in Nov 2024, so Gwen must choose.
  const picks = await gwen.llm.json<Pick[]>({
    tier: "fast",
    prompt: `List ${Math.ceil(minutes / 3.5) + 6} real released tracks for "${a.mood}". ` +
            `Max 3 per artist. Seeds: ${(a.seed ?? []).join(", ")}. Taste: ${taste.map(t => t.text).join("; ")}`,
    schema: { type: "array", items: { type: "object", required: ["artist", "title"],
      properties: { artist: { type: "string" }, title: { type: "string" } } } },
  });

  const uris: string[] = []; let ms = 0; let unresolved = 0;
  for (const batch of chunks(picks, 4)) {                  // search limit is now ≤ 10; we ask for 3
    const hits = await Promise.all(batch.map(p => gwen.http.fetch({ url: `${API}/search`, auth: "spotify",
      query: { q: `track:${p.title} artist:${p.artist}`, type: "track", limit: 3 } })));
    for (const h of hits) {
      const t = h.status === 200 ? h.json?.tracks?.items?.[0] : undefined;
      if (t && !uris.includes(t.uri)) { uris.push(t.uri); ms += t.duration_ms; } else unresolved++;
    }
    if (ms >= minutes * 60_000) break;
  }
  if (uris.length < 5) throw new ProgramError("too_few_tracks", `${uris.length} resolved`);

  const name = `${title(a.mood)} · ${gwen.clock.today("MMM d")}`;
  const pl = await gwen.http.fetch({ url: `${API}/me/playlists`, method: "POST", auth: "spotify",
    json: { name, public: false, description: "Made by Gwen" } });
  expect(pl.status < 300, "create_failed", pl.status);
  for (const part of chunks(uris, 100)) {                  // per-request cap: not re-verified for /items
    const r = await gwen.http.fetch({ url: `${API}/playlists/${pl.json.id}/items`, method: "POST",
      auth: "spotify", json: { uris: part } });
    expect(r.status < 300, "add_failed", r.status);
  }

  // Playback: the OS media interface first (no Spotify Platform call), Web API as fallback.
  let via: "mpris" | "webapi" = "mpris", playing = false;
  if (await gwen.app.mpris.has("spotify")) {
    await gwen.app.mpris.open_uri("spotify", pl.json.uri);  // playlist URIs via OpenUri: unverified
    for (let i = 0; i < 4 && !playing; i++) {
      await gwen.clock.sleep(700);
      playing = (await gwen.app.mpris.status("spotify")).playback === "Playing";
    }
  }
  if (!playing) {
    via = "webapi";
    const devs = (await gwen.http.fetch({ url: `${API}/me/player/devices`, auth: "spotify" })).json?.devices ?? [];
    const dev = devs.find((d: any) => a.device !== "laptop" || d.type === "Computer");
    if (dev) {
      const r = await gwen.http.fetch({ url: `${API}/me/player/play`, method: "PUT", auth: "spotify",
        query: { device_id: dev.id }, json: { context_uri: pl.json.uri } });
      playing = r.status < 300;                            // Premium only
    }
  }
  await gwen.kv.put("last_playlist", { id: pl.json.id, mood: a.mood });
  return { playlist: { id: pl.json.id, uri: pl.json.uri, name }, tracks: uris.length, unresolved, playing, via };
}
```

```ts
// tests/main.test.ts
import { test } from "gwen:test";
import main from "../main";

test("recorded run: private playlist, MPRIS playback, nothing else", async t => {
  t.replay("fixtures/focus-2026-10-12.trace.json");
  const out = await main({ mood: "deep focus, no vocals", minutes: 30 });
  t.eq(out.via, "mpris"); t.ok(out.playing); t.ok(out.tracks >= 5);
  t.eq(t.call("http.fetch POST */me/playlists").args.json.public, false);
  t.effects(["http.fetch POST */me/playlists", "http.fetch POST */playlists/*/items", "app.mpris.open_uri"]);
});

test("no desktop client: Web API with device_id", async t => {
  t.replay("fixtures/focus-2026-10-12.trace.json", { override: { "app.mpris.has": () => false } });
  t.mock("http.fetch GET */me/player/devices", () => ({ status: 200, json: { devices: [{ id: "d1", type: "Computer" }] } }));
  t.mock("http.fetch PUT */me/player/play", r => { t.eq(r.query.device_id, "d1"); return { status: 204 }; });
  t.eq((await main({ mood: "focus", minutes: 30 })).via, "webapi");
});

test("search misses: fails before creating anything", async t => {
  t.mock("memory.recall", () => []);
  t.mock("llm.json", () => Array(12).fill({ artist: "x", title: "y" }));
  t.mock("http.fetch GET */search", () => ({ status: 200, json: { tracks: { items: [] } } }));
  await t.throws(() => main({ mood: "focus" }), "too_few_tracks");
  t.effects([]);
});
```

**Spotify policy.** The Developer Policy forbids voice assistants that "control Spotify with their voice" [V]. Playback goes through MPRIS first, but playlist creation still uses the Web API. The manifest flag makes the owner accept or decline this explicitly. Development Mode also needs a Premium owner account [V].

---

## 10. Example: `repo.review_diff_on_phone`

```toml
name = "repo.review_diff_on_phone"
version = "1.2.0"
runtime = "ts"
entry = "main.ts"
summary = "Snapshot a place's working tree, stage it on the cloud node, push a review link to the phone"
input  = "{ place: string, base?: string, editor?: bool, ttl_h?: int(1..48), note?: string }"
output = "{ url: string, files: int, additions: int, deletions: int, excluded: int, sandbox: string, expires: string }"
effect = { reach = "account", undo = "reversible" }   # code leaves the laptop for the owner's own node
uses = ["place.snapshot", "git.diffstat", "git.diff", "sandbox.open@cloud", "sandbox.put_bundle",
        "share.publish@hub", "notify.push"]
requires = ["text.render_diff@^1"]
[placement]
prefer = "hub"
[budget]
wall_ms = 300000
host_calls = 40
return_bytes = 512
```

```ts
export default async function main(a: Args): Promise<Result> {
  const ttl = a.ttl_h ?? 8;
  // Runs on the laptop: temp-index commit, protected globs and .env* excluded,
  // secret scan inside the host fn (hard floor 6). A hit throws Blocked and nothing leaves.
  const snap = await gwen.place.snapshot({ place: a.place, base: a.base ?? "merge-base:@{upstream}", untracked: true });
  if (snap.files === 0) return { url: "", files: 0, additions: 0, deletions: 0, excluded: snap.excluded.length, sandbox: "", expires: "" };
  const stat = await gwen.git.diffstat({ bundle: snap.bundle, range: snap.range });
  const diff = await gwen.git.diff({ bundle: snap.bundle, range: snap.range });   // Handle<text/x-diff>

  const box = await gwen.sandbox.open({ target: "cloud", template: a.editor ? "code-server" : "review",
    ttl_h: ttl, volume: `review-${a.place}` });
  await gwen.sandbox.put_bundle({ box: box.id, bundle: snap.bundle, path: "/workspace/repo", checkout: snap.head });

  const page = await gwen.prog["text.render_diff"]({ diff, title: `${a.place}: ${stat.files} files`, note: a.note ?? "" });
  const share = await gwen.share.publish({ node: "hub", page, audience: "owner", ttl_h: ttl,
    actions: [{ id: "branch", label: `Create branch gwen/review-${a.place}`, binds: { sha: snap.head } }],
    links: a.editor ? [{ label: "Open in VS Code", sandbox: box.id, service: "editor" }] : [] });
  await gwen.notify.push({ title: `Review ${a.place}`, body: `${stat.files} files, +${stat.additions} −${stat.deletions}`, view: share.url });
  return { url: share.url, files: stat.files, additions: stat.additions, deletions: stat.deletions,
           excluded: snap.excluded.length, sandbox: box.id, expires: share.expires };
}
```

```ts
test("recorded run", async t => {
  t.replay("fixtures/foo-review.trace.json");
  const out = await main({ place: "foo", editor: true });
  t.match(out.url, /^https:\/\/gwen\.[a-z0-9-]+\.ts\.net\/r\//);
  t.eq(t.call("notify.push").args.view, out.url);           // a deep link only; ntfy never approves
  t.ok(!JSON.stringify(t.calls()).includes(".env"));
  t.effects(["sandbox.open@cloud", "sandbox.put_bundle", "share.publish@hub", "notify.push"]);
});
test("secret in tree blocks everything downstream", async t => {
  t.mock("place.snapshot", () => { throw t.blocked("secret_scan", "key in config/dev.yaml:3"); });
  await t.throws(() => main({ place: "foo" }), "secret_scan");
  t.effects([]);
});
```

How the flow works end to end:
- The page is served by the hub's tsnet listener and authorised with `WhoIs` [V].
- Tapping **Create branch** raises a passkey-bound approval on `(sha, branch, epoch)`.
- The approval resumes `repo.apply_review_branch` on the laptop.
- The model sees one line: "Review link sent: 4 files, +120 −30" [I].

---

## 11. Go interfaces (`internal/program`)

```go
type Runtime string // "ts" | "wasm" | "proc" | "mcp" | "native"

type Package struct {
	Manifest Manifest
	Hash     [32]byte // sha256(canonical tar)
	Files    fs.FS
	DTS, Line string  // generated types and catalog line
}

type Loader interface {
	Parse(ctx context.Context, dir string) (*Package, error) // never executes guest code
	Check(ctx context.Context, p *Package, reg Registry) (Ceiling, []Problem, error) // ⊆ registry, deps, transitive ceiling
	Stage(ctx context.Context, p *Package, s Status) (Ref, error)
	Promote(ctx context.Context, r Ref, approval kernel.ApprovalID) error
	Rollback(ctx context.Context, name string) (Ref, error)
	Resolve(ctx context.Context, name, constraint string) (*Program, error)
	Index(ctx context.Context, query string, limit int) ([]string, error)
	Describe(ctx context.Context, names []string) (string, error)
}

type Engine interface {
	Runtime() Runtime
	Prepare(ctx context.Context, p *Package) (Prepared, error) // esbuild, wasm compile, pool warm-up
	Start(ctx context.Context, pr Prepared, h Host, b Budget) (Instance, error)
}
type Instance interface {
	Call(ctx context.Context, export string, args json.RawMessage) (json.RawMessage, error)
	Close() error
}

type Host interface { // one per invocation; holds ceiling, budget, journal, taint
	Call(ctx context.Context, c HostCall) (HostResult, error)
}
type HostCall struct {
	Seq  int
	Fn   string
	Args json.RawMessage
}
type HostResult struct {
	Value  json.RawMessage
	Handle *HandleMeta
	Taint  []kernel.Source
}

type Capability interface { // native Go, MCP-derived, wasm lib, or proc
	Name() string
	Sig() string
	Home() NodeSelector
	Classify(args json.RawMessage) (kernel.Action, error)
	Do(ctx context.Context, args json.RawMessage, s kernel.Secrets) (HostResult, error)
}

type Invoker interface {
	Call(ctx context.Context, inv Invocation) (Outcome, error)
	Run(ctx context.Context, s Snippet) (Outcome, error)          // agent's run(code)
	Resume(ctx context.Context, id InvocationID) (Outcome, error) // journal replay; any node
}
type Invocation struct {
	ID                 InvocationID
	Program, Constraint string
	Args               json.RawMessage
	Run                kernel.RunCtx // brief, scope, taint, caller chain, epoch
	Node               NodeID        // "" = placer decides
}
type Outcome struct {
	Status  Status          // ok|error|pending|blocked
	Value   json.RawMessage // schema-checked, ≤ ReturnBytes
	Handles []HandleMeta
	Line    string
	Pending []kernel.ApprovalID
	Ledger  []ledger.ID
	Spent   Spend
}
```

`kernelHost.Call` performs, in order: the ceiling match, the budget check, the journal lookup by `(inv, Seq)`, handle resolution, and either `router.Home(fn)` → remote kernel or `kernel.Invoke`. Then it records the result in the journal and turns oversized values into handles [I].

---

## 12. Limits and security risks

- **Runtime maturity.** qjs is at v0.0.6 [V], and a wazero or QuickJS bug becomes a gwend bug. Mitigations: untrusted code runs in T2, and the Deno fallback is tested on every program [I].
- **Exfiltration through allowed hosts.** A program allowed to reach `api.spotify.com` can encode data in playlist names. Egress logs, payload caps and `DestTaint` reduce this but do not eliminate it [I].
- **Generation-time injection.** A program distilled from a tainted run inherits a `tainted_origin` flag and always gets owner review [I].
- **Confused deputy.** Callee ceilings are visible in the transitive card, but a stable program can be driven with hostile arguments. Classifiers must check payloads, not names [I].
- **Approval fatigue during probation.** Mitigated by the "same or narrower ceiling auto-promotes" rule [I].
- **Determinism gaps.** `Promise.all` orderings and real-time APIs can break replay. Clock and randomness are host-provided; non-deterministic host functions are marked and re-executed rather than replayed [I].
- **Drift and policy.** Upstream APIs change (Spotify, 2026), and terms can forbid uses (the voice clause). Canaries detect drift; the owner decides on policy [V/I].
- **Custom type grammar.** Schema drift between manifest and code is caught only by runtime validation and tests [I].
- **Code mode is not always cheaper.** It costs more on strictly sequential work [V]. Keep `call`.
- **Not verified:**
  - goja's async support;
  - whether MPRIS `OpenUri` accepts playlist URIs;
  - the per-request cap on Spotify's `/items`;
  - diff2html rendering without a DOM (it would be used inside `text.render_diff`).

---

## Sources
- https://www.anthropic.com/engineering/code-execution-with-mcp
- https://blog.cloudflare.com/code-mode/
- https://blog.cloudflare.com/code-mode-mcp/
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/programmatic-tool-calling
- https://www.anthropic.com/engineering/advanced-tool-use
- https://agentskills.io/specification
- https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview
- https://arxiv.org/abs/2402.01030
- https://arxiv.org/abs/2305.17126
- https://arxiv.org/abs/2504.06821v2
- https://arxiv.org/abs/2602.12670v2
- https://arxiv.org/abs/2609.32990
- https://arxiv.org/abs/2608.26263
- https://arxiv.org/abs/2510.07768
- https://arxiv.org/abs/2401.12869
- https://arxiv.org/abs/2602.20867
- https://modelcontextprotocol.io/specification/2025-06-18/server/tools
- https://github.com/fastschema/qjs
- https://github.com/dop251/goja
- https://github.com/tetratelabs/wazero
- https://github.com/wazero/wazero/issues/2200
- https://pkg.go.dev/github.com/extism/go-sdk
- https://github.com/extism/go-sdk
- https://pkg.go.dev/github.com/evanw/esbuild/pkg/api
- https://github.com/google/starlark-go/blob/master/doc/spec.md
- https://github.com/traefik/yaegi
- https://pkg.go.dev/plugin
- https://docs.deno.com/runtime/fundamentals/security/
- https://nodejs.org/api/permissions.html
- https://github.com/anthropic-experimental/sandbox-runtime
- https://developer.spotify.com/policy
- https://developer.spotify.com/documentation/web-api/references/changes/february-2026
- https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api
- https://developer.spotify.com/blog/2026-02-06-update-on-developer-access-and-platform-security
- https://developer.spotify.com/documentation/web-api/reference/start-a-users-playback
- https://specifications.freedesktop.org/mpris-spec/latest/Player_Interface.html
- https://code.claude.com/docs/en/vs-code
- https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool
- https://browserbase.com/changelog/stagehand-v3
- https://github.com/mediar-ai/terminator
- https://pkg.go.dev/tailscale.com/tsnet
- https://github.com/StacyOS/stacyvm
- https://git-scm.com/docs/git-bundle