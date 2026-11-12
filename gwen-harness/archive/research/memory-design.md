# Gwen memory and context layer v2: design

Legend: **[V]** means the claim was verified in a cited source, either by the three input reports or by me. **[I]** means it is my own judgement. **[H]** means it is a hypothesis Gwen's eval harness must test before the design relies on it. During this pass I re-checked two load-bearing facts myself:
- Mem0's 2026 algorithm is ADD-only and describes no recency term in scoring.
- Supermemory's local mode has no connectors, uses bring-your-own-key extraction, and is limited to "one machine, one process".

## 0. Thesis

**Gwen owns the truth, and Mem0 and Supermemory are lenses over it.**
- Gwen's SQLite holds two authoritative stores:
  - an append-only, verbatim **episode log**;
  - a keyed, bi-temporal **claims** table with provenance.
- Mem0 and Supermemory are **projections** fed from an outbox. They are rebuildable, removable, and never the source of truth.

That one decision contains most of the risk of running three memory systems (§12). It also follows the evidence:
- Verbatim episodes win on LongMemEval, and flat facts are about 33 points below full context [V].
- Mem0 OSS cannot express "this stopped being true" (#4956) [V].
- Supermemory's store is a closed, encrypted binary with a 10k-document lite cap [V].

There are four innovations, each aimed at "context is the biggest thing":
1. **Answer by value, not by chunks.** Most personal questions resolve to a keyed claim lookup that returns a value plus a handle, not 20 retrieved passages.
2. **Memory as an API for program files.** The owner's program files call `mem.Get` and `mem.Recall` in code, so memory is consumed outside the model's context and only results come back.
3. **A cache-stable context compiler.**
   - Tiers: a daily-stable prefix, then a per-thread observation log, then an append-only **memory delta at the tail**, then just-in-time handles.
   - This fills the gap the field report names: no published design combines a stable cached prefix with just-in-time retrieval [V/I].
4. **Every lane earns its place by ablation.** Mem0 and Supermemory are evaluated per lane on a personal benchmark, and a lane that does not move accuracy beyond its confidence interval is switched off.

## 1. Division of labour: who is strongest at what

| Need | Mem0 OSS 2.2.x (local sidecar) | Supermemory local server | Gwen-native |
|---|---|---|---|
| Fact extraction from chat | One tuned LLM call per add [V]. **Role: a second-opinion extractor** that adds recall. Its weakness is recall itself: HaluMem shows 42.91% on Medium and 3.23% on Long [V] | Over-extracts: 29.71% accuracy, worst false-memory resistance (FMR 36.86%) [V] | **Primary extractor**: round-level, keyed, absolute dates, cited span |
| Fast conversational recall | Search makes no LLM call; semantic + BM25 + entity boost; paper p50 search 0.148 s [V]. **Role: a recall lane** | Hybrid memories + chunks [V] | FTS5 + sqlite-vec in `gwen.db` [V] |
| Documents and resources (PDF, OCR, audio, pages) | Vision-to-text only [V] | **Strongest**: extract → chunk → embed, chunk kept as evidence; `updates`/`extends`/`derives` relations, `isLatest`; the only system not to collapse on HaluMem Long (recall 53.02%) [V]. **Role: the resource lane** | Local FTS over file paths and metadata |
| Profile | Platform only [V] | Static/dynamic profile [V]. **Role: a candidate source** for the nightly profile card | **Profile card** compiled from claims |
| Time and supersession | OSS has `expiration_date` only; no recency term (#4956) [V] | `documentDate`/`eventDate`, `forgetAfter`, version chain [V] | **Authoritative bi-temporal claims** |
| Provenance | No memory-to-source pointer [V] | Chunk evidence [V] | Span-level source on every item |
| Typed personal data, procedures, run state, ACLs, sync, Go API | None [V] | None [V] | All of these |
| Connectors (Gmail, Drive, Notion) | None in OSS [I] | Platform only [V] | Optional hosted Supermemory in "cloud-ok" mode |

## 2. Memory types and where each lives

| Type | Store (authority) | Synced? | Reaches the model as |
|---|---|---|---|
| **Working** (per turn, per run) | Run dir `runs/<id>/todo.md`, `notes.md`, tool outputs stored as artifacts with handles | Run summary synced; detail local | Recited todo (Manus); handles; cleared tool results [V] |
| **Episodic** | `threads` + `messages` (append-only, round-level) and run traces; replaces chats stored as `llm_runs` JSON blobs [V] | Yes | Observation log (Mastra-style); `mem.expand(handle)` for verbatim text |
| **Semantic and profile** | `claims` (bi-temporal, keyed) + `entities` + `memory_links` | Yes | Profile card in the prefix; values + handles in the delta |
| **Temporal facts** | `claims.valid_from/valid_to` + `recorded_at/superseded_at`, plus `occurred_at` vs `mentioned_at` | Yes | `as_of` on every query |
| **Procedural** | `programs` catalog + program files on disk, in git | Catalog synced | Name + signature + one line in the cached catalog; the body executes outside context |
| **Resource / document** | Raw file path + content hash in `resources`; chunks and derived memories in Supermemory | Metadata synced; index local | Cited chunk snippets via handles |
| **Structured personal data** | Existing typed tables: tasks, goals, plan_items, segments, calendar_busy, weekly_reviews, energy_logs [V] | As today | Fixed query programs, never re-extracted as text facts |
| **Self** | `self_versions` (append-only), moved out of the system prompt | Yes | Inside the daily profile-card block |
| **Conflicts and quarantine** | `conflicts`, plus `claims.status` ∈ candidate / active / quarantined / rejected / superseded | Yes | Review cards for the user; never injected as fact |

## 3. Architecture

```
 utterance / tool result / file / typed-table event
        │
        ▼ (online, <10 ms, no LLM)
 ┌─────────────── gwend ─────────────────────────────────────────────────────┐
 │ kernel gateway ── memory.* actions ── policy (taint, sensitivity) ─ ledger │
 │      │                                                                     │
 │ EPISODE LOG (messages, runs, resources) ──► FTS5   (local, derived)        │
 │      │                                                                     │
 │      ▼ outbox jobs (idle-aware)                                            │
 │ EXTRACTOR (local model) ─┐                                                 │
 │ Mem0 add(infer) shadow ──┼─► VERIFIER (span check, entailment, speaker,    │
 │ Supermemory profile ─────┘    trust tier) ─► KEYED SUPERSESSION ─► CLAIMS  │
 │                                    │                   │       (bi-temporal)│
 │                                    └─► CONFLICTS / QUARANTINE ─► user cards │
 │ CLAIMS ──outbox──► projections: sqlite-vec · Mem0 (infer=false) ·          │
 │                    memory-files view (md/git)                              │
 │ RESOURCES ──outbox──► Supermemory local (docs, chunks, relations)          │
 │                                                                            │
 │ SLEEP WORKER (idle laptop / Pi): dedupe · contradiction scan · profile card│
 │   · observation reflector · reflection · program-draft distillation ·     │
 │   reconcile projections · expire                                           │
 │                                                                            │
 │ READ: Router ─► lanes in parallel w/ deadlines:                            │
 │   claims-key │ FTS5 │ vec │ entity-hop │ time-window │ SQL programs │      │
 │   programs │ Mem0 │ Supermemory                                            │
 │   ─► weighted RRF ─► as_of/supersession/trust/sensitivity filter ─► dedupe │
 │   ─► (rerank) ─► packer (budget) ─► CONTEXT COMPILER                       │
 │      [prefix: persona·catalog·profile card]                                │
 │      [observation log] [turns] [memory delta @tail]                        │
 └────────────────────────────────────────────────────────────────────────────┘
   sidecars: bwrap, loopback-only, egress denied (except the Ollama socket)
```

## 4. Write path

**Online (request path, no LLM call):**
- Append the round to `messages` and insert it into FTS5.
- Enqueue embedding and extraction jobs in the outbox.
- Explicit "remember" or "forget" requests and edits made in the UI or the memory files become **T0** claims immediately.
- Typed-table changes (task done, calendar event) are recorded as `memory_links` events, not claims. They are already authoritative [I].
- This removes extraction from the reply's critical path. HaluMem ingestion took 2,768 minutes for Mem0 and 1,672 for Supermemory [V].

**Near-line (seconds to minutes, idle-aware, batched):**
- **Batching.** Buffer rounds until about 1,024 tokens or about 1 h idle, the Memobase pattern [V].
- **Extraction.** One local-model call (Ollama) per batch produces **candidates**: `{key, subject_entity, value, text, occurred_at, valid_from, valid_to, cited_span}`.
  - Keys come from an open-but-validated vocabulary such as `user.employer`, `person:<id>.birthday`, `pref.music.focus`, each with single or multi cardinality.
  - Relative dates are resolved to absolute ones at write time (Hindsight) [V].
  - Fact-augmented keys raised LongMemEval recall by 9.4% [V].
- **Verifier** (the "checkable extraction" open problem; HaluMem shows hallucinations build up at write time [V]):
  1. The cited span must exist verbatim in the episode.
  2. An entailment check: a small model call judges whether the span supports the claim [H: cost against false-memory rate].
  3. **Speaker rule.** A claim supported only by Gwen's own text never becomes active without the user's affirmation. Mem0 v2 deliberately stores "agent-confirmed facts" at full weight, and HaluMem measured QA hallucination around 19% [V].
  4. The trust tier is assigned (§7).
- **Commit.**
  - For a single-valued key with an existing active claim:
    - If the new `valid_from` ≥ the old one, the old claim is **deterministically superseded**: `valid_to` and `superseded_by` are set, and no LLM vote is taken.
    - If the order is ambiguous, a `conflict` row is created.
  - Multi-valued keys append.
  - **Only active claims are projected**, so poisoned or unverified content never reaches Mem0 or the vector index [I].
- **Mem0 as second-opinion extractor** [H].
  - In shadow mode, Mem0 `add` runs with inference in a `run_id`-scoped scratch space.
  - Facts it finds that Gwen's extractor missed become candidates and pass through the same verifier.
  - The union may raise recall while the verifier protects precision. Keep this only if GwenBench recall improves beyond its confidence interval, because it costs one extra LLM call per batch.

**Projection writes** (outbox, idempotent, keyed by claim UUIDv7):
- **Mem0:**
  - Stores the verified claim text without inference, with metadata `{gwen_claim_id, key, valid_from}` [I: confirm the `infer` and `metadata` parameters on the pinned version].
  - When Gwen supersedes or forgets a claim, the outbox deletes the matching Mem0 row.
- **Supermemory** receives only **resources**: files the user marks, PDFs, voice-meeting transcripts, pages kept from runs.
  - Gwen sets `temporalContext.documentDate`.
  - It uses UUIDv7 custom ids, because custom-id collisions overwrite files (#1547) [V].

**Sleep consolidation** (idle laptop, or the Pi for cheap jobs; produces a reviewable diff, as Letta's dreaming does [V]):
- **Near-duplicate clustering:** merge into one claim carrying several sources. Mem0 OSS dedup is exact MD5 only [V].
- **Contradiction scan** across entities and keys, producing conflicts.
- **Profile-card recompile**, versioned and at most once a day so the cache prefix stays stable.
- **Observation reflector:** condenses the observation log and drops superseded entries (Mastra) [V].
- **Reflection insights** at tier T3 with confidence decay.
- **Lessons and draft program files** from finished runs (AWM, ReasoningBank: +24.6% on Mind2Web, +8.3% on WebArena [V]), gated by evals as in harness §9.5.
- **Supermemory's profile output** is read as candidate claims, which must verify against Supermemory's chunks.
- **Projection reconciliation** and **expiry**.

The sleep-time compute evidence is on stateful reasoning tasks only, so treat consolidation's benefit to memory as [H] [V].

## 5. Time, contradictions, forgetting

**Bi-temporal claims** (Zep/Graphiti [V], plus Hindsight's occurrence vs mention time [V]):
- `valid_from` / `valid_to`: when the claim was true in the world.
- `recorded_at` / `superseded_at`: when Gwen learned it and when she retired it.
- `occurred_at` / `mentioned_at`.
- Every read takes `as_of`, defaulting to now.
- **Time-aware query expansion.** Temporal phrases in the query become a window filter. On LongMemEval this raised temporal recall by 6.8 to 11.3% [V].

**Contradictions are objects.**
- Every system scores ≤7% on multi-hop FactConsolidation, and contradiction resolution is BEAM's weakest ability [V].
- A conflict row carries both claims, both sources, and a proposed resolution.
- It auto-resolves only when the key is single-valued and the timestamps are ordered.
- Everything else goes to a batched "Gwen isn't sure" card, which takes one tap to resolve.
- Until resolved, both claims are injected with a `disputed` marker, or Gwen abstains [I].

**Forgetting comes in five kinds:**
1. **Supersede**: the truth changed. History is kept.
2. **Expire**: `valid_to`, or Supermemory's `forgetAfter` on resources, for temporary facts such as "in Berlin this week".
3. **Rank decay**: salience × recency × access. It affects ranking only and never deletes. Forgetting curves are unproven [V].
4. **User hard delete**:
   - Tombstone the row, **blank its body**, delete it from every projection, and return a `ForgetReceipt` listing each lane's confirmation.
   - For Supermemory, confirm the delete by listing through `POST /v4/memories/list`. Export and delete paths there are weak (#1653) [V].
   - The ledger stores only the claim id and content hash, so a hard delete does not break the hash chain [I].
5. **Retrieval-only "don't bring this up"**: a flag that suppresses the claim from injection.

Raw episodes are never forgotten automatically. Storage is cheap, and verbatim episodes are what everything else is rebuilt from [I].

## 6. Read path: router, fusion, budgets, context compiler

**Router.** A deterministic intent classifier (rules plus embedding nearest-prototype, no LLM call) [H: accuracy] picks lanes:

| Intent | Lanes (in priority order) |
|---|---|
| Lookup ("where do I work", "Alice's birthday") | claims-key (an exact hit returns a **value**), entity-hop |
| Episodic ("what did we decide Tuesday") | time-window + FTS5 + vec over episodes and observation logs |
| Resource ("what does the lease say") | Supermemory chunks, local FTS |
| Fuzzy preference | vec over claims, Mem0, Supermemory memories |
| Personal data ("hours on Gwen last week") | fixed SQL query programs over the typed tables (no LLM-written SQL) |
| How-to | programs catalog |
| Multi-hop | decompose into sub-queries [H: one LLM call, used only when flagged] |
| Unknown | fan out to all local lanes; sidecars if within budget |

**Fusion and filtering:**
- Weighted RRF (k=60) across lanes. Zep, Hindsight and Agent Zero all fuse several channels; removing any one channel cost 1.2 to 1.8 points [V].
- Then filter:
  - map every Mem0 hit to its claim through `memory_backends` and drop it if that claim is superseded or forgotten. This neutralises #4956;
  - apply `as_of` validity;
  - apply the trust tier and sensitivity rules (§7).
- Dedupe by claim id, then content hash, then near-duplicate cosine [H: threshold].
- Optional local cross-encoder rerank [H].

**Retrieve wide, inject narrow.**
- Pull about 40 candidates. Memanto gained +20.4 points going from 10 to 40 chunks [V].
- Pack them into a token budget with a precedence order: claim **values**, then one-line gists with handles, then short verbatim snippets.
- This reconciles that gain with context rot: focused prompts beat about 113k-token prompts in every model family [V].

**Context compiler.** Gwen today sends the whole state every turn: up to 150 tasks, 24 messages, and no `cache_control` [V]. The compiler replaces that with four tiers:

| Tier | Content | Budget (defaults, tuned by eval) [H] | Cache behaviour |
|---|---|---|---|
| P0 prefix | persona, program/tool catalog (names + one line), **profile card** + current self | ≤ 800 tokens for the card | Stable for a day; `cache_control` breakpoint. Cache reads cost 0.1× base [V] |
| P1 observation log | dated, prioritised observations of the thread and today's state (Mastra OM: 94.87 on LongMemEval_S [V]) | ≤ 3k | Second breakpoint; changes only when the Observer runs |
| P2 turns | recent verbatim rounds | rolling | Append-only |
| P3 **memory delta** | router output for *this* message, attached to the latest user turn | ≤ 1.2k frontier / ≤ 2k local | Appended at the **tail**, so it never invalidates P0 or P1 |

**Just-in-time access.**
- The model gets `mem.recall`, `mem.get`, `mem.expand`, `mem.timeline` and `mem.why`, which return **handles** such as `c:01J…`, `ep:01J…#r3` and `doc:<id>#c12`, not dumps.
- This follows Anthropic's lightweight identifiers [V] and Manus's restorable compression [V].

**Long runs.**
- Each run has a recited `todo.md` and a scratchpad.
- On Anthropic: tool-result clearing (`clear_tool_uses_20250919`), compaction (`compact_20260112`) and the client-side memory tool (`memory_20250818`). Memory plus context editing gave +39% and 84% fewer tokens [V].
- On Ollama and the OpenAI-compatible providers, gwend reimplements the same semantics [I].
- At run end, the trace becomes an episode.

**Model-adaptive policy.**
- For a frontier reader with less than about 100k tokens of relevant history, prefer verbatim recall. Full context scored 92.85 vs Mem0 flat facts 57.68 on LoCoMo with GPT-5-mini [V].
- Small local models get heavier memory. Hindsight's 20B model went from 39.0 to 83.6 [V].

**Citation lock.** An answer about the user's past must cite at least one handle above a threshold. Otherwise Gwen says she doesn't know (Agent Zero, MemCube) [V].

**Memory consumed by programs.** A program file such as `plan-week` calls `mem.Get(ctx, "user.work_hours", now)` and `mem.Recall(...)` directly through the host API. The model sees only the program's small result. This is the owner's "load directly, don't add context" principle applied to memory [I]. It is the closest analogue to SMFS's vendor-reported cut from 72M to 24M tokens [V, vendor claim].

## 7. Provenance, trust, permissions, user editing

**Provenance on every claim:** `source_kind`, `source_id`, `span`, `source_device_id`, `channel` (voice, chat, phone, email, tool), `extractor` (model plus prompt version), `confidence`, `trust_tier`, `sensitivity`.

**Trust tiers:**

| Tier | Source | Effect |
|---|---|---|
| T0 | user edits or explicit commands | committed directly |
| T1 | user utterances | normal path |
| T2 | Gwen's tool observations of typed data | normal path |
| T3 | Gwen's inference or reflection | carries a decaying confidence |
| T4 | external content: email, web, documents, Supermemory-derived memories | **quarantined** |

T4 content can never become an active claim without corroboration from a T0 or T1 source, or the user's approval. MINJA reached 98.2% injection success, and the defences tested in that paper failed [V]. Retrieved T4 hits **taint the run**, using the harness's taint labels.

**The permission kernel governs memory.** `memory.*` calls are kernel Actions:
- **Reads** are local and allowed, with one rule against the "lethal trifecta" (private data + untrusted content + a way out). In a tainted run, or one with world-reach tools in scope, items with sensitivity ≥ `sensitive` appear only as handles, and expanding them needs approval [I, from harness §2.4 and §5.7].
- **Writes** are local and reversible. A write from a tainted run goes to quarantine.
- **Supersede and soft-forget** are allowed and recorded in the ledger.
- **Hard delete and export** are allowed only from user surfaces, like rules.
- **Sidecar processes** run under a `memory-sidecar` sandbox profile: loopback only, egress denied.
- **Every claim state transition** is a ledger row.

**User editing:**
- Claims are projected to `~/Gwen/memory/*.md`: `profile.md`, `people/alice.md`, `topics/…`. The folder is a git repository, in the style of Letta MemFS and Claude's memory tool [V].
- Edits flow back as T0 episodes.
- UI cards offer "Gwen learned X, from chat on 3 Oct: keep / fix / forget".
- `mem.why(handle)` shows the source span.

## 8. Privacy, local-first, cross-device

**Never leaves the machine in any mode:**
- raw voice;
- episodes and claims with sensitivity ≥ `sensitive`, and their embeddings;
- secrets, which belong to the secrets broker, never memory (MIRIX's vault idea [V], made stricter).

**Local-first mode (the default):**
- **Mem0 OSS** (Apache-2.0) [V]:
  - Ollama for the LLM and embeddings, local Qdrant. No Postgres, which keeps the footprint small [I].
  - `MEM0_TELEMETRY=false` set in the environment. #2062 reports that setting it in-script did not stop traffic [V].
- **Supermemory local:**
  - local ONNX embeddings (`bge-base-en-v1.5`), Ollama for the LLM;
  - `SUPERMEMORY_DISABLE_TELEMETRY=1`;
  - an absolute data dir, because #1420 shows a cwd default silently creating an empty store [V];
  - loopback only, because #1441 binds all interfaces [V];
  - URL ingestion is disabled because it calls a hosted reader [V].
- The sandbox egress proxy denies all traffic from the sidecars, so a telemetry flag that is ignored still leaks nothing [I].

**Cloud-ok mode** (opt-in, per namespace):
- Hosted Supermemory, but only for its connectors (Gmail, Drive, Notion; platform only [V]).
- Ingested content is tier T4.
- A hosted LLM (Claude) may extract from items with sensitivity ≤ `personal`.

**Sync:**
- **Synced, append-only rows**, which fit the existing row-level last-writer-wins [V]: messages, claims (a supersede is a new row), conflicts, entities, links, `self_versions`, observation logs, the programs catalog.
- **Local projections**, rebuilt via `OnApplied` with a pinned embedding-model id [V/I]: FTS5, vec, the Mem0 and Supermemory stores, `memory_backends`.
- **Prerequisites:** schema-version negotiation and a tolerant `checkRow` [V].
- **Memory home.** The laptop is the "memory home" that runs the sidecars.
- **Phone queries** go to the hub. The hub runs the pure-Go lanes over synced rows: claims-key, FTS5 and entity. sqlite-vec has `linux/arm64` support [V].
  - When the laptop is reachable on the tailnet, the hub dispatches the full recall to it.
  - Otherwise it answers in degraded mode, and says so [I].

## 9. Failure handling

- **Per-lane deadlines** [H]: about 150 ms for local lanes and about 500 ms for sidecars. A circuit breaker opens after N failures.
- The **Gwen-only path** (claims, FTS5, vec) is always available and is the floor.
- `Packet.Degraded` lists the missing lanes, and the UI shows memory health.
- **Writes never block on sidecars.** The outbox retries with idempotency keys.
- A **nightly reconciliation** compares claim ids against each projection and repairs drift.
- **"Rebuild lane from log"** is a supported operation, so a corrupted, capped or abandoned vendor store costs reindex time, not data.
- **Supermemory's 10k-document cap** [V]: a counter warns at 80%. Above the cap, resources fall back to local-only indexing [I].
- **Known engine bugs:**
  - Non-ASCII text is silently not indexed (#1740) [V]. Gwen's FTS5 lane covers it, and GwenBench tests it.
  - Mem0 TS `add()` has a TOCTOU duplicate race (#6531) [V]. The outbox serialises writes per scope.

## 10. Go interface

```go
package memory

type Tier uint8      // T0..T4
type Sensitivity uint8 // public, personal, sensitive, secret
type LaneName string // "claims","fts","vec","entity","time","sql","programs","mem0","supermemory"

type TimeRange struct{ From, To time.Time }

type Provenance struct {
	SourceKind, SourceID, Span, DeviceID, Channel, Extractor string
	Tier        Tier
	Sensitivity Sensitivity
}

type Claim struct {
	ID, Key, SubjectEntity, Text string
	Value                      json.RawMessage
	Valid                      TimeRange // world time
	RecordedAt, SupersededAt   time.Time // system time
	SupersededBy, Status       string
	Confidence                 float64
	Prov                       Provenance
}

type RunCtx struct {
	RunID          string
	Tainted        bool
	WorldReach     bool
	ModelProfile   string // "frontier","local-small"
}

type Budget struct {
	Tokens, Candidates int
	Deadline           time.Duration
}

type Query struct {
	Text     string
	AsOf     time.Time
	Window   *TimeRange
	Entities []string
	Intent   string // optional override; router classifies otherwise
	Budget   Budget
	Run      RunCtx
}

type Hit struct {
	Handle   string // "c:<id>", "ep:<id>#r3", "doc:<id>#c12"
	Gist     string
	Value    json.RawMessage // set for keyed claims
	Score    float64
	Lanes    []LaneName
	Valid    TimeRange
	Prov     Provenance
	Disputed bool
}

type Packet struct {
	Hits     []Hit
	Tokens   int
	Abstain  bool
	Degraded []LaneName
}

type Router interface {
	Recall(ctx context.Context, q Query) (Packet, error)
	Get(ctx context.Context, key string, asOf time.Time, run RunCtx) (Claim, error)
	Expand(ctx context.Context, handle string, maxTokens int, run RunCtx) (string, error)
	Why(ctx context.Context, handle string) (Provenance, error)
}

type Writer interface {
	Observe(ctx context.Context, ep Episode) error                 // append-only, online
	Propose(ctx context.Context, c []Candidate) ([]string, error)  // -> verifier
	Resolve(ctx context.Context, conflictID string, choice Resolution) error
	Supersede(ctx context.Context, oldID string, next Claim, reason string) error
	Forget(ctx context.Context, r ForgetRequest) (ForgetReceipt, error)
}

type Lane interface {
	Name() LaneName
	Search(ctx context.Context, q Query) ([]Hit, error)
	Health(ctx context.Context) error
}

type Projection interface { // Mem0, Supermemory, vec, memory-files
	Lane
	Upsert(ctx context.Context, recs []Record) error // idempotent on Record.ID
	Delete(ctx context.Context, ids []string) error
	Rebuild(ctx context.Context, since time.Time) error
}

type Compiler interface {
	Compile(ctx context.Context, t Turn) (Prompt, error) // P0..P3 with cache breakpoints
}
```

Wiring points:
- `Server.Memory`, which is nil when disabled, following the `Calendar` and `Sync` pattern [V].
- Ingestion hooks after `api/assistant.go:59-76` [V].
- The compiler replaces `assistantRequest` at `:215-350` [V].
- The `cache_control` breakpoints go into `providers.go`.

## 11. Evaluation

**Public benchmarks:**
- Run with one fixed judge, three seeds and bootstrap confidence intervals.
- **LongMemEval-S**, especially the knowledge-update, temporal and abstention categories [V].
- **MemoryAgentBench FactConsolidation**, for selective forgetting [V].
- **HaluMem stage metrics**: extraction and update accuracy and FMR. These measure the verifier directly [V].
- A **BEAM** 100K slice, for contradiction [V].
- **LoCoMo** only with the Penfield-corrected key, because 6.4% of the original answer key is wrong [V].

**GwenBench** (personal, owner-confirmed, about 200 items [H]):
- knowledge updates ("I moved");
- `as_of` questions;
- multi-hop across the typed tables ("which project was I stuck on when I last met Alice");
- abstention;
- email and web poisoning attempts;
- CJK and non-ASCII text;
- program and procedure recall;
- phone-degraded mode.

**Efficiency metrics, gated alongside accuracy:**
- tokens injected per answered question;
- prefix cache hit rate (on Sonnet, cached input costs $0.30 vs $3 per MTok, per Manus [V]);
- p50 and p95 recall latency;
- write-path cost per day;
- poisoned-write acceptance rate.

**Lane ablation is the gate for each vendor.** Run GwenBench with each lane off. A lane whose removal stays within its confidence interval gets disabled.

## 12. Risks of three memory systems, and containment

| Risk | Containment |
|---|---|
| **Cost** (double extraction, ingestion hours) | One primary extractor per batch. Mem0 projection without inference. Mem0's own extraction runs only in shadow mode and stays only if eval-justified. Supermemory gets resources only. Batching at idle. Kernel token budgets |
| **Consistency** (three versions of the truth) | Gwen's claims are authoritative. Projections are idempotent on claim ids, filtered by supersession on read, reconciled nightly, and rebuildable from the log |
| **Latency** | Parallel fan-out with deadlines. The prefix never depends on sidecars. Lookups are answered by the claims-key lane in-process |
| **Duplication** | Mapping through `memory_backends`, then content hash, then near-duplicate clustering in fusion and in sleep consolidation |
| **Poisoning amplified across stores** | Only active, verified claims are projected. T4 is quarantined. Retrieved T4 hits taint the run |
| **Lock-in and closed binary** | Supermemory holds no unique data: raw resources stay on disk. Pin the version and check its sha256 [I] |
| **Telemetry and network exposure** | Environment flags, plus sandbox with egress denied, loopback-only binding and absolute data dirs |
| **Version drift** (Mem0 v2 broke the API and score semantics [V]) | Pinned versions, contract tests per adapter, a third-party Go client avoided in favour of a hand-written REST adapter |
| **Ops footprint** (Python + spaCy + Qdrant, plus a 270–300 MB binary [V]) | Sidecars are optional and run on the laptop only. Phase gating: Gwen-native ships first |

## 13. Phased build order

1. **M0 Foundations:**
   - threads/messages, claims, entities, links, conflicts; FTS5 + `modernc.org/sqlite/vec`;
   - move the self-note out of the prefix, add `cache_control`;
   - replace the 150-task dump with scoped retrieval;
   - GwenBench v0 and a LongMemEval-S harness.
   - *Exit:* a token and cache baseline is recorded.
2. **M1 Write path:**
   - extractor, verifier, keyed supersession, quarantine, the ledger hooked into the kernel;
   - memory cards, the memory-files view, `mem.why`.
3. **M2 Context compiler:**
   - the P0–P3 tiers, observation log, handles and JIT tools, run working memory, clearing and compaction.
   - *Exit:* accuracy holds with fewer tokens per turn.
4. **M3 Supermemory resource lane:**
   - sandboxed; resources only; ablation run.
5. **M4 Mem0 lane:**
   - projection, recall lane and shadow extractor; ablation; keep or disable each part.
6. **M5 Sleep worker:**
   - profile card, reflection, program-draft distillation, reconciliation.
7. **M6 Cross-device:**
   - schema negotiation, hub degraded lanes, dispatch to the memory home;
   - optional hosted Supermemory connectors in cloud-ok mode.

## Sources
- https://arxiv.org/abs/2504.19413 (Mem0) ; https://mem0.ai/blog/mem0-the-token-efficient-memory-algorithm ; https://mem0.ai/blog/ai-memory-benchmarks
- https://github.com/mem0ai/mem0 ; /issues/4956 ; /issues/6531 ; /issues/2062 ; /issues/3944 ; https://raw.githubusercontent.com/mem0ai/mem0/main/mem0/memory/main.py
- https://www.getzep.com/blog/lies-damn-lies-statistics-is-mem0-really-sota-in-agent-memory/
- https://supermemory.ai/docs/self-hosting/local-vs-enterprise.md ; https://supermemory.ai/changelog/local.md ; https://supermemory.ai/docs/concepts/how-it-works ; https://supermemory.ai/docs/api-reference/content-management/update-a-memory-creates-new-version.md ; https://supermemory.ai/docs/user-profiles.md ; https://supermemory.ai/docs/connectors/overview ; https://supermemory.ai/research/longmembench
- https://github.com/supermemoryai/supermemory/issues/1740 ; /issues/1441 ; /issues/1420 ; /issues/1547 ; /issues/1653 ; https://github.com/supermemoryai/supermemory/releases ; https://smfs.ai
- https://arxiv.org/html/2511.03506v2 (HaluMem) ; https://arxiv.org/html/2507.05257v2 (MemoryAgentBench) ; https://arxiv.org/html/2510.27246v1 (BEAM) ; https://arxiv.org/html/2410.10813v2 (LongMemEval) ; https://arxiv.org/html/2603.04814 (full context vs memory)
- https://penfieldlabs.substack.com/p/we-audited-locomo-64-of-the-answer
- https://arxiv.org/html/2501.13956v1 (Zep) ; https://arxiv.org/html/2512.12818v1 (Hindsight) ; https://mastra.ai/research/observational-memory ; https://github.com/memodb-io/memobase ; https://arxiv.org/abs/2507.07957 (MIRIX) ; https://arxiv.org/abs/2608.29606 (Agent Zero Memory) ; https://arxiv.org/abs/2604.21284 (Memanto)
- https://www.letta.com/blog/context-repositories ; https://letta.com/blog/sleep-time-compute ; https://arxiv.org/abs/2409.07429 (AWM) ; https://arxiv.org/abs/2509.25140 (ReasoningBank)
- https://arxiv.org/abs/2503.03704 (MINJA) ; https://labs.cloudsecurityalliance.org/wp-content/uploads/2026/07/CSA_research_note_memghost_ai_agent_memory_injection_20260726-csa-styled.pdf
- https://www.trychroma.com/research/context-rot ; https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents ; https://platform.claude.com/docs/en/build-with-claude/context-editing ; https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool ; https://platform.claude.com/docs/en/build-with-claude/prompt-caching ; https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- https://pkg.go.dev/modernc.org/sqlite/vec ; https://github.com/asg017/sqlite-vec ; https://inkandswitch.com/local-first/
- Local: /mnt/project-files/gwen-harness/harness.md (§2.4, §3.3, §5.1, §5.7, §9, §11) ; /home/claude/gwen at b029c77 (files cited in the data-model report)