# Mem0 and Supermemory for Gwen's memory layer: architecture, benchmarks and failure modes

Claims are tagged **[V]** when I read them in a source and **[I]** when they are my own judgement. Dates are as of 2026-10-12.

---

## 1. Mem0

### 1.1 Architecture: the 2025 paper vs the 2026 code

**The paper (arXiv 2504.19413, 28 Apr 2025)** [V]
- **Extraction:** one LLM call over the new message pair, a rolling summary S and the last m=10 messages.
- **Update:** for each candidate fact, fetch the top s=10 similar memories. The LLM then picks ADD, UPDATE, DELETE or NOOP via a tool call.
- **Mem0g (graph variant):**
  - adds an entity extractor and a relationship generator that writes triplets to Neo4j;
  - outdated relations are marked invalid rather than deleted;
  - retrieval is entity-centric subgraph expansion plus triplet similarity.
- **Models:** GPT-4o-mini for every LLM step.
- **Footprint:** about 7k memory tokens per conversation for Mem0, about 14k for Mem0g, and over 600k for Zep.
- **Gaps in the paper itself:** Mem0g scored lower than plain Mem0 on multi-hop (47.19 vs 51.15). The abstract claims a 91% p95 latency cut, but §4.3 says 92% for Mem0 and 85% for Mem0g.

**Mem0 OSS v2.0.0 (spring 2026), algorithm described 16 Apr 2026** [V]. It is a different system from the paper:
- **ADD-only.** One LLM call extracts facts. UPDATE and DELETE are gone. "New facts are stored alongside old ones."
- **Graph memory removed from OSS.** About 4,000 lines of Neo4j/Memgraph/Kuzu/AGE/Neptune driver code were deleted. `enable_graph` and `graph_store` are gone, and `relations` is no longer returned. Graph memory (now "entity linking") is a Platform feature, on Pro and above.
- **Hybrid retrieval.**
  - Semantic, BM25 and entity scores are combined additively (`ENTITY_BOOST_WEIGHT = 0.5`).
  - BM25 and entity scores only re-rank semantic candidates.
  - Without the spaCy `en_core_web_sm` model (and fastembed, for Qdrant), search falls back to semantic-only.
  - The `top_k` default dropped from 100 to 20.
- **Dedup is an MD5 hash of the exact text.** Near-duplicates are not caught.

**The add() path, read from `mem0/memory/main.py` on `main`** [V]:
1. Load the last 10 messages from the SQLite history DB.
2. Embed the input and fetch the top 10 existing memories. Their UUIDs are mapped to integers, labelled in the code as "anti-hallucination".
3. **One LLM call** with `json_object` output.
4. Embed the extracted facts in a batch.
5. MD5 dedup against those 10 neighbours plus the current batch.
6. Insert, with payload fields `data`, `hash`, `created_at`, `updated_at`, `role`, `actor_id`, `attributed_to` and the scope ids.
7. Write ADD rows to the SQLite history.
8. spaCy NER, entity embedding, and an upsert into a `{collection}_entities` collection. This step lists up to `top_k=10000` entity rows per add.

So each add costs 1 LLM call, about 3 embedding batches and a spaCy pass. [I] The 10,000-row entity listing on every add looks like a scaling hazard once a user has a large entity set. **Search makes no LLM call by default.** It runs 1 embedding, a vector search, a keyword search and NER on the query; an optional reranker can be added.

**Scoping and memory types** [V]
- Scope is `user_id`, `agent_id` and `run_id`, which in v2 must go inside `filters`, plus metadata.
- `memory_type="procedural_memory"` still exists, but the code carries "TODO: Remove procedural-memory support in a future breaking release."

**Time, forgetting and multimodal input**
- **OSS** [V]: only `expiration_date` (YYYY-MM-DD; expired memories are hidden). The source code itself raises "The timestamp parameter is not supported by the OSS Memory SDK", and does the same for `reference_date` and `decay`.
- **Platform only** [V]:
  - **Temporal reasoning:** event-date boosting, `timestamp` on add, `reference_date` on search.
  - **Memory Decay:** access-based multiplier from 0.3× to 1.5×. It only biases ranking and never filters; off by default.
  - **Dream:**
    - Merge: duplicates are hidden but kept.
    - Supersede: the older fact is marked superseded; `latest_only=true` returns only current facts.
    - Synthesis: weekly on Pro, needs at least 20 memories.
  - [I] Contradiction handling, the most important correctness feature, is effectively a paid feature.
- **Multimodal** [V]: vision messages go through an LLM describer (`enable_vision`). The OSS code shows no document-connector ecosystem.
- **User profiles** [V]: SDK v2.2.0 added profile generate/get/sample methods to the client, which targets the Platform.

### 1.2 Deployment
- **License and versions** [V]: Apache-2.0, about 66.8k stars. Latest Python SDK is 2.2.1 (25 Sep 2026); Node SDK is 3.3.1.
- **Self-hosted server** [V]:
  - FastAPI.
  - Compose brings up the `mem0` API on host port 8888, `pgvector/pgvector:pg17` on 8432, and a Next.js dashboard on 3000.
  - Auth is on by default (JWT or `m0sk_` API keys).
  - REST routes: `/memories`, `/search` (with `explain`), `/memories/{id}/history`, `/entities`.
  - Compose passes `MEM0_TELEMETRY=${MEM0_TELEMETRY:-true}`.
- **Library** [V]:
  - Defaults: local Qdrant at `/tmp/qdrant` and SQLite at `~/.mem0/history.db`.
  - About 30 vector stores, including qdrant, pgvector, chroma, faiss, redis and milvus. There is no sqlite-vec option.
  - LLMs include `ollama`, `lmstudio`, `vllm`, `litellm`, `anthropic` and `openai`.
  - Embedders include `ollama`, `huggingface` and `fastembed`.
  - **A fully local setup is therefore possible.** The README defaults to OpenAI `gpt-5-mini` and `text-embedding-3-small`.
- **MCP:**
  - OpenMemory, the local MCP server, has been **sunset**. A maintainer replied to an unauthenticated cross-user read/write/delete report (#6080, June 2026): "we have sunsetted openmemory, so we won't be taking issue or pr." [V]
  - The current MCP is **hosted only** at `mcp.mem0.ai/mcp`, with 11 tools (add/search/get/update/delete memories, entities, events). [V]
- **Clients** [V]: Python and TS SDKs and a CLI. **There is no official Go SDK.** `plexusone/mem0-go` v0.1.0 is a third-party, MIT, ogen-generated client covering hosted and FOSS backends.
- **Hosted pricing** [V]:
  - Hobby: free, 10k adds / 1k retrievals per month.
  - Starter: $19.
  - Pro: $249, 500k adds / 50k retrievals; includes graph memory, Dream and analytics.
  - Enterprise: custom; on-prem and audit logs.

### 1.3 Benchmarks and disputes
- **The paper's LoCoMo J scores** [V]:

  | System | Single-hop | Multi-hop | Open-domain | Temporal |
  |---|---|---|---|---|
  | Mem0 | 67.13 | 51.15 | 72.93 | 55.51 |
  | Mem0g | 65.71 | 47.19 | 75.71 | 58.13 |

  Search p50/p95 was 0.148/0.200 s for Mem0 and 0.476/0.657 s for Mem0g. Total p95 was 1.44 s for Mem0 vs 17.1 s for full context.
- **Zep's rebuttal (6 May 2025)** [V]:
  - Mem0 misconfigured Zep: both speakers were given the user role, timestamps were put into message text instead of `created_at`, and searches ran sequentially.
  - Corrected Zep score: 75.14 vs Mem0g 68.44 and Mem0 66.88.
  - **Mem0's own full-context baseline (72.90) beat both Mem0 variants.**
  - Zep also argued LoCoMo is too short (16–26k tokens), has no knowledge-update tests, and has broken category-5 and image questions.
- **LoCoMo audit (Penfield, 8 Apr 2026)** [V]: 6.4% of the answer key is wrong (99 of 1,540). The GPT-4o-mini judge accepted 62.81% of intentionally wrong but on-topic answers. The audit cites failed reproductions, including **Mem0 #3944**: about 0.20 on the Mem0 Platform, with a fabricated memory dated January 2026 for a 2023 conversation.
- **2026 self-reported numbers (Platform, "includes proprietary optimizations"; OSS users should "expect similar direction but not identical numbers")** [V]:
  - LoCoMo 92.5 (old algorithm 71.4).
  - LongMemEval 94.4 (old 67.8).
  - BEAM-1M 64.1; BEAM-10M 48.6. Contradiction resolution on BEAM is 35.7 at 1M and 32.5 at 10M.
  - Under 7k tokens per retrieval; p50 latency 0.88–1.09 s.
- **Independent results:**
  - **MemoryAgentBench** (Hu, Wang, McAuley; ICLR 2026) [V]: Mem0 overall 21.1 vs MemGPT 28.3. Selective forgetting (conflict resolution) 10.0, with **FactConsolidation multi-hop at 2.0**. Long-range understanding 20.7.
  - **HaluMem** (Nov 2025) [V]:
    - Extraction recall 42.91% on the Medium set and **3.23% on Long**.
    - Correct memory updates 25.50% on Medium and **1.45% on Long** (98.51% omission).
    - Dialogue ingestion took **2,768 minutes** on Medium, against 41.66 minutes for retrieval.
  - These used the pre-2026 algorithm.

### 1.4 Failure modes in the issue tracker
- **Stale facts outrank new ones** [V]:
  - **#4956**: under ADD-only, "I work at Company A" and "I now work at Company B" coexist, and `score_and_rank` has no recency term. A bi-temporal fix was approved in principle; PR #4969 was closed unmerged and #5218 has conflicts.
  - **#4896**: "my name is LGY" and then "LGS" are both stored. Closed with "the behavior is by design in v3."
- **The community is patching around these gaps** [V]:
  - **#5352** lists "time-blindness, semantic conflicts, and missing CRUD" and links a weekly LLM hygiene script. On a 90-fact store its first run removed 8 duplicates and merged 6 more.
  - A Hermes-agent plugin exists for the same purpose.
- **Concurrency** [V]: **#6531**, a TOCTOU race in the TS OSS `add()`, creates permanent duplicates under parallel adds. The reporter says a Python-side equivalent was fixed in #6516.
- **Agent hallucinations become memories** [I]: v2 deliberately stores "agent-confirmed facts" with the same weight as user facts [V], so assistant hallucinations can become memories. HaluMem measured QA hallucination at around 19% [V].
- **Telemetry is on by default** [V]:
  - PostHog via `MEM0_TELEMETRY` (default "True"), read at import time. #2062 reports setting it in-script did not stop traffic.
  - Payload includes OS version, kernel release, CPU and Python version, plus a persistent anonymous id.
  - Hot-path events are sampled at 10%; lifecycle events always fire.
  - The OSS client also displays upsell "notices" when a store passes 2,000 memories or `top_k ≥ 50`.
- **Missing model features** [V]:
  - **No provenance link from a memory to its source message.** The payload has `role` and `actor_id` but no message or chunk id.
  - **No per-memory ACLs.** Scope ids only.
  - **No structured state.**
  - **Procedural memory is slated for removal.**
- **Security** [V]: memory-injection research (MemGhost, July 2026) reports payloads transferring to a Mem0 backend. The mitigations it recommends are provenance tags, approval gates for externally sourced writes, and write logs.

### 1.5 Using it from a Go daemon [I]
- **Options:**
  - Run the official server as a Docker sidecar (three containers including Postgres and the dashboard) and call REST.
  - Or write a ~150-line Python sidecar around `Memory` with Ollama and Qdrant-local or pgvector.
- **Before running either:**
  - Set `MEM0_TELEMETRY=false` in the environment, not in code.
  - Pin the version: v2 broke the API and score semantics.
  - Keep the history DB path inside Gwen's data dir.
- **Resource use is not published.** Expect a Python process with spaCy and fastembed plus Postgres.

---

## 2. Supermemory

### 2.1 Architecture
- **Pipeline** [V]: Queued → Extracting (text, OCR, transcription, page fetch) → Chunking → Embedding → Indexing → Done.
- **"Dreaming" turns content into memories:**
  - `dynamic` (default) groups related documents. **Extraction may continue after status "done"**, and a fresh namespace "can show an empty profile for minutes."
  - `instant` processes one document and costs an extra operation.
- **Outputs per namespace** [V]: document chunks (evidence), memories (facts) and a profile.
- **Memory model** [V]:
  - The LongMemEval report says atomic memories are generated per semantic chunk, with **the source chunk kept as evidence**.
  - Relations are **updates** (a correction or state change), **extends** (adds detail) and **derives** (an inference).
  - `isLatest` keeps retrieval on the current fact while history stays in the graph.
- **Versioned edits and time fields** [V]: `PATCH /v4/memories` creates a version chain (`version`, `parentMemoryId`, `rootMemoryId`) and accepts:
  - `forgetAfter` and `forgetReason` (scheduled forgetting);
  - `temporalContext {documentDate, eventDate}`.
- **Forgetting** [V]: the docs describe type-dependent decay. Facts persist until updated, preferences "strengthen with repetition", episodes "decay unless significant", and temporary facts expire.
- **Search parameters** [V]:
  - `searchMode` = hybrid (default), memories or chunks.
  - `threshold` default 0.3 (was 0.6 in v4).
  - `rerank` = none, order (about +100 ms) or aggregate.
  - `rewriteQuery`, metadata `filter`, `limit` up to 100.
  - `include` = documents, related or forgotten.
- **Profiles** [V]: `POST /ns/{namespace}/profile` returns a static part (durable facts) and a dynamic part (recent context), with optional buckets (up to 50). The profile takes no query, so you call search in parallel.
- **Scoping** [V]:
  - The namespace (formerly `containerTag`) is a "hard isolation boundary" carried in the URL.
  - Metadata gives soft filters.
  - Scoped API keys.
  - "Per-tag access" on the Scale plan.
- **Connectors** [V]: Google Drive, Gmail, Notion, OneDrive, GitHub (webhooks plus 4-hour polling), Granola (manual), and a web crawler (7-day recrawl). **All platform only.**
- **SMFS** [V]: a FUSE mount on Linux (MIT/Apache) that exposes a container as files: `grep` runs semantic search, and `profile.md` is generated on read. The vendor reports cutting Claude Code's total tokens from 72M to 24M on 110 xAFS questions. [I] This is the closest existing thing to the owner's "load directly, don't bloat context" idea.

### 2.2 Deployment
- **License** [V]: the repo is MIT (about 31k stars), and the SDKs are Apache-2.0. **The engine is not open source.** The `supermemory-server` binary "is built from a separate, non-public codebase," and is free under a **"lite" license capped at 10,000 documents** (release server-v0.0.7). Local mode is limited to one org, one machine and one process.
- **Local mode** (launched 10 Jun 2026; latest server-v0.0.8, 17 Aug) [V]:
  - A single bun-compiled binary: linux-x64 298 MB, linux-arm64 269 MB.
  - Embedded encrypted store and graph engine.
  - Local ONNX embeddings by default (`bge-base-en-v1.5`, 768d), or OpenAI, OpenAI-compatible or Google.
  - Bring-your-own LLM, including Ollama, vLLM or llama.cpp.
  - API on `:6767`. Telemetry is on unless `SUPERMEMORY_DISABLE_TELEMETRY=1`.
  - URL ingestion calls a hosted reader service.
  - **Not in local mode:** connectors and MCP. Extraction uses "your model" in local mode and "proprietary models" on the platform.
- **MCP** [V]: hosted at `mcp.supermemory.ai/mcp` with tools `memory`, `recall` and `context`.
- **Clients** [V]: TS and Python SDKs. **There is no Go SDK.** Plain REST works.
- **Pricing** [V]:
  - Free $0 ($5 of credits), Pro $19, Max $100 (adds Gmail), Scale $399 (self-host option, S3 and crawler), Enterprise (air-gapped).
  - Overage: $5 per 1M "SM tokens" of memory (text), $1 per 1M for SuperRAG, and $5 per 1M search queries.

### 2.3 Benchmarks and disputes
- **LongMemEval-S, earlier report** [V]: GPT-4o 81.6, GPT-5 84.6, Gemini-3-Pro 85.2. The Hindsight paper cites these, and they come from Supermemory's own technical report.
- **Current research page (May 2026)** [V]: GPT-4o **97%**, with every category at 95–100%. The table is labelled "Recall@20 with aggregation" yet presented as LLM-judge accuracy, and the GPT-5 and Gemini rows are unchanged at 84.6 and 85.2. [I] The table is internally inconsistent: a weaker model scoring 12 points higher points to a different, aggregated setup. Do not treat 97% as production accuracy.
- **ASMR, 22 Mar 2026** [V]:
  - The "~99%" headline was 98.6%, counting an answer correct if **any of 8 prompt variants** got it right. A 12-agent majority-vote variant scored 97.2%.
  - Supermemory said it was "not our main production engine," which sits at about 85%.
  - A critic (Boschi, who has a stake via Hindsight) cites Supermemory's follow-up: about 70 s latency, over 10k tokens per query, and 12 agents.
- **No published LoCoMo number** [V], only "#1" claims.
- **Independent result: HaluMem** [V]:
  - Supermemory was the only system that did not collapse on the Long set: recall 53.02%, QA 53.77%.
  - It over-extracts: 77,134 memories at **29.71% accuracy**, with the **worst false-memory resistance** (FMR 36.86%).
  - Update correctness is only 16–17%.
  - Ingestion took **1,672 minutes** on Long.
- **Vendor latency** [V]: search 187 ms server-side and 356 ms end to end (its own telemetry).

### 2.4 Failure modes in the self-hosted server's issues [V]
- **#1740:** memories containing **no ASCII characters (e.g., pure CJK) are silently never indexed** (254 of 4,845 in the reporter's store). Also, `limit` over 100 returns zero results.
- **#1413:**
  - Snapshots re-serialized the whole DB every 10 s, holding about 60% of a core at 142 MB.
  - v0.0.7 skips idle ticks, but every tick that has writes still dumps the whole DB.
- **#1649:** a mid-file edit re-embeds nearly all chunks, about 21 CPU-minutes on 2 vCPUs.
- **Security and data loss:**
  - **#1441:** binds all interfaces. The docs call v0.0.8 auth "unsafe when exposed to untrusted networks."
  - **#1420:** the data dir defaults to the current working directory, so launching from elsewhere silently creates an empty store and a new key.
  - **#1547:** custom-id collisions overwrite files.
  - **#1315:** the linux-x64 binary broke ingestion because of an unbundled dependency.
- **API and config bugs:**
  - **#1704:** search returns nothing when `containerTag` is omitted.
  - **#1695:** profile buckets are never populated.
  - **#1336:** embedding env vars are ignored.
- **#1653:** extracted memories can only be reached through top-K search. The store is encrypted with no documented export path, though a reply points to `POST /v4/memories/list`. [I] That makes lock-in a real risk.
- **#1348 (governance request):** asks for PII redaction, poisoning defense and an audit trail, none of which exist yet.

### 2.5 Using it from a Go daemon [I]
- Run `supermemory-server` as a systemd user service with an explicit absolute data dir.
- Bind it only to loopback: put it in a network namespace or bwrap with a proxy, because of #1441.
- Talk to it with a hand-written Go REST client covering `/v4/search`, `/ns/{ns}/profile`, `/v3/documents` and `PATCH /v4/memories`.
- It is a closed binary, so pin the version and verify its sha256.
- **Two hard limits apply:** the 10,000-document cap, and no connectors or MCP when run locally. The Raspberry Pi hub could run the arm64 build, but CPU spikes during snapshots and re-embedding are documented.

---

## 3. Comparison

| Dimension | Mem0 (OSS 2.2.x / Platform) | Supermemory (local server / Platform) |
|---|---|---|
| Engine license | Apache-2.0, fully open [V] | Closed binary; free "lite" up to 10k docs [V] |
| LLM calls per write | 1 extraction call plus embeddings and spaCy [V] | Extraction plus async "dreaming"; count unpublished [V] |
| Update semantics | OSS ADD-only + MD5 dedup; Supersede/Merge platform only [V] | updates/extends/derives, `isLatest`, version chain [V] |
| Temporal | OSS: `expiration_date` only; event dates, `reference_date`, decay on Platform [V] | `documentDate`/`eventDate`, `forgetAfter`, time-aware search [V] |
| Graph | Removed from OSS in v2; Platform "entity linking" (Pro+) [V] | Embedded graph engine, local included [V] |
| Retrieval | Semantic + BM25 + entity, no recency term [V] | Hybrid memories + chunks, rerank, query rewrite [V] |
| Provenance | No memory-to-source pointer [V] | Source chunk kept as evidence [V] |
| Profiles | Platform profile API (SDK 2.2.0) [V] | Static/dynamic profile, buckets [V] |
| Documents / multimodal | Vision-to-text only [V] | PDF/OCR/audio/video; URLs via hosted reader [V] |
| Connectors | None in OSS [I] | 7, platform only [V] |
| Procedural memory | Deprecated, marked for removal [V] | None explicit [I] |
| Structured state (tasks/plans) | No [I] | No [I] |
| Per-memory ACL | No; scope ids only [V] | Namespace-level; per-tag on Scale [V] |
| MCP | Hosted only; OpenMemory sunset [V] | Hosted only; not in local mode [V] |
| Go client | Third-party only [V] | None; REST [V] |
| Local footprint | Python + Postgres/pgvector (or Qdrant) + spaCy [V] | ~270–300 MB binary, ONNX embeddings [V] |
| Telemetry default | On (PostHog) [V] | On [V] |
| Independent evidence | MemoryAgentBench 21.1 overall; HaluMem update recall 1.45% on Long [V] | HaluMem: best on Long, but 29.7% extraction accuracy and worst FMR [V] |
| Self-reported LongMemEval | 94.4 (Platform) [V] | 81.6 to 85.2 production; 97 / 98.6 under aggregation [V] |

---

## 4. Gaps Gwen must fill herself

[I] Judgements built on the verified evidence above.

1. **System of record.** Neither product can be Gwen's source of truth:
   - Mem0 OSS cannot express "this stopped being true."
   - Supermemory's store is encrypted, closed and capped at 10k documents, with export only recently patched in.

   **Gwen should own an append-only event log plus a bi-temporal fact table in her SQLite**, with valid-from/valid-to, recorded-at, superseded-by and source-event-id. Mem0 and Supermemory then act as replaceable *derived indexes* fed from the outbox. That way, running both does not create two competing versions of the truth.
2. **Contradiction and update resolution.** Updates are the weakest point everywhere: HaluMem update correctness is 1–47%, and MemoryAgentBench multi-hop conflict resolution is 2.0. Gwen needs:
   - deterministic supersession for keyed attributes such as `user.employer` or `user.city`, with no LLM vote;
   - an LLM only for proposing keys;
   - a consolidation pass like Mem0's "Dream" that runs locally.
3. **Provenance and trust tiers on every memory.** Record who said it, from which channel (voice, phone, email, tool output), and an evidence pointer. That is what defends against MemGhost-style injection. Tie writes to the harness permission kernel: external-source writes go to quarantine until approved, and every write is logged in the ledger.
4. **Procedural memory as executable "program files".** Both systems store prose facts, and Mem0 is removing procedural support. The owner's program-file idea belongs here: a procedure is code plus a manifest plus a few-line description. Only the description is retrievable into context; invoking it executes outside the context window.
5. **Structured working state.** Tasks, plans, open loops and device and file locations should be typed SQLite tables queried by tools, not vectors.
6. **Context budgeting.** The useful pattern is Supermemory's profile + search split and SMFS's "memory as files": a small always-on profile, query-time recall and grep-able files. Gwen should own the token budget per turn rather than trust a vendor's top-K.
7. **Local-first, multilingual and private by default:**
   - Set both telemetry switches off.
   - Bind sidecars to loopback only.
   - Use absolute data dirs.
   - Test non-ASCII text (#1740).
   - Run Ollama/ONNX embeddings, with sqlite-vec in Gwen's own DB as the baseline index.
8. **A Go-native interface.** Neither vendor ships an official Go client. Define one `MemoryBackend` interface in gwend with `Remember`, `Recall`, `Supersede`, `Forget` and `Profile`, a REST adapter for each sidecar, and fused retrieval that falls back to Gwen-only results.
9. **Write-path cost control.** Ingestion dominates: 2,768 minutes for Mem0 and 1,672 for Supermemory in HaluMem. Batch extraction at idle or on the Pi hub, run cheap local NER and dedup before any LLM call, and never put extraction in the reply's critical path.
10. **Her own evaluation.** Vendor numbers on LoCoMo and LongMemEval are disputed or come from aggregated runs. Keep a small regression suite drawn from the owner's real conversations: knowledge updates, time questions, multi-hop, injection attempts and CJK text. Run it on every backend change.

---

## Sources
- https://arxiv.org/abs/2504.19413 ; https://arxiv.org/pdf/2504.19413v1
- https://www.getzep.com/blog/lies-damn-lies-statistics-is-mem0-really-sota-in-agent-memory/
- https://penfieldlabs.substack.com/p/we-audited-locomo-64-of-the-answer
- https://mem0.ai/blog/mem0-the-token-efficient-memory-algorithm
- https://mem0.ai/blog/ai-memory-benchmarks
- https://pypi.org/project/mem0ai/
- https://github.com/mem0ai/mem0 ; https://github.com/mem0ai/mem0/releases
- https://newreleases.io/project/github/mem0ai/mem0/release/v2.0.0
- https://docs.mem0.ai/migration/oss-v2-to-v3.md
- https://docs.mem0.ai/platform/features/graph-memory.md
- https://docs.mem0.ai/platform/features/temporal-reasoning.md
- https://docs.mem0.ai/platform/features/dream
- https://docs.mem0.ai/platform/features/memory-decay
- https://docs.mem0.ai/open-source/features/rest-api.md ; https://docs.mem0.ai/open-source/overview.md
- https://docs.mem0.ai/platform/mem0-mcp
- https://mem0.ai/pricing
- https://raw.githubusercontent.com/mem0ai/mem0/main/mem0/memory/main.py
- https://raw.githubusercontent.com/mem0ai/mem0/main/mem0/memory/telemetry.py
- https://raw.githubusercontent.com/mem0ai/mem0/main/mem0/memory/notices.py
- https://raw.githubusercontent.com/mem0ai/mem0/main/mem0/utils/factory.py
- https://raw.githubusercontent.com/mem0ai/mem0/main/server/docker-compose.yml
- https://github.com/mem0ai/mem0/issues/3944 ; /issues/4896 ; /issues/4956 ; /issues/5352 ; /issues/6531 ; /issues/6080 ; /issues/2062
- https://pkg.go.dev/github.com/plexusone/mem0-go
- https://arxiv.org/html/2511.03506v2 (HaluMem)
- https://arxiv.org/html/2507.05257v2 ; https://iclr.cc/virtual/2026/poster/10010781 (MemoryAgentBench)
- https://arxiv.org/pdf/2512.12818 (Hindsight)
- https://labs.cloudsecurityalliance.org/wp-content/uploads/2026/07/CSA_research_note_memghost_ai_agent_memory_injection_20260726-csa-styled.pdf
- https://github.com/supermemoryai/supermemory ; https://github.com/supermemoryai ; https://github.com/supermemoryai/supermemory/releases ; https://github.com/supermemoryai/supermemory/issues
- https://github.com/supermemoryai/supermemory/issues/1413 ; /issues/1740 ; /issues/1653
- https://supermemory.ai/docs/self-hosting/overview.md ; https://supermemory.ai/docs/self-hosting/local-vs-enterprise.md ; https://supermemory.ai/docs/deployment/self-hosting
- https://supermemory.ai/changelog/local.md
- https://supermemory.ai/docs/concepts/how-it-works ; https://supermemory.ai/docs/concepts/graph-memory.md
- https://supermemory.ai/docs/api-reference/content-management/update-a-memory-creates-new-version.md
- https://supermemory.ai/docs/user-profiles.md ; https://supermemory.ai/docs/search ; https://supermemory.ai/docs/api-reference/search.md
- https://supermemory.ai/docs/connectors/overview
- https://supermemory.ai/pricing
- https://supermemory.ai/research ; https://supermemory.ai/research/longmembench ; https://supermemory.ai/research/longmembench.md
- https://supermemory.ai/compare/supermemory-vs-mem0/
- https://aihola.com/article/supermemory-99-longmemeval-agentic-memory
- https://buttondown.com/nicoloboschi/archive/thats-not-how-you-do-business/
- https://smfs.ai
- https://arxiv.org/abs/2602.08316 (SWE-ContextBench; the abstract does not name Mem0 or Supermemory, so the vendor's comparison numbers drawn from it are unverified)