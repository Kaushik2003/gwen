# Agent memory and context engineering in 2026: what Gwen's memory design should take from the field

Legend: **[V]** means I read it in the cited source. **[I]** means it is my own judgement. All scores are self-reported by the authors unless I say otherwise.

## 0. Summary

- **Benchmark scores are near saturation, and they don't compare well.** Top claims are now LongMemEval about 92 to 95.6% and LoCoMo about 89 to 93.6% (Agent Zero Memory, Mastra OM, Hindsight, Maximem Synap) [V]. The same system scores very differently from one paper to the next. Zep on LoCoMo is reported as 59.22, 65.99, 75.14 and 79.09 [V: MemOS, Mem0 via Zep, Zep, MIRIX papers]. The LoCoMo answer key is about 6.4% wrong [V].
- **The hard problems are still unsolved.** These are selective forgetting and contradiction handling (≤7% on multi-hop FactConsolidation; contradiction resolution is BEAM's weakest ability), and memory used for action across sessions (MemoryArena success rates 0.12 to 0.23) [V].
- **A naive memory layer loses to full context on frontier models.** With GPT-5-mini, full context scores 92.85 on LoCoMo against 57.68 for flat-fact Mem0 [V]. Memory pays off for weaker or local models (Hindsight 20B: 39.0 to 83.6 on LongMemEval) and on cost after about 10 turns [V].
- **Converging pattern among the best systems:**
  - verbatim episodes as ground truth;
  - typed, bi-temporal derived facts;
  - several retrieval channels fused with Reciprocal Rank Fusion (RRF);
  - background consolidation;
  - a stable, cache-friendly prefix plus just-in-time tools;
  - provenance with abstention.

  Memory stored as files you can read and edit, often backed by git, is also spreading: Letta MemFS, Claude's memory tool, EverMemOS's Markdown + SQLite stack [V].

---

## 1. Systems

| System | Core idea and memory types | Storage | Update and consolidation | Retrieval | Time handling | License / maturity | Published results |
|---|---|---|---|---|---|---|---|
| **Zep / Graphiti** | Temporal knowledge graph. Three layers: episodes (raw, "non-lossy"), entities and edges, communities [V] | Neo4j 5.26, FalkorDB 1.1.2, Neptune. Kuzu is deprecated because upstream is unmaintained [V] | An LLM checks new edges against related old ones. On a conflict it sets `t_invalid` on the old edge instead of deleting it [V] | Cosine + BM25 + breadth-first graph search. Rerank with RRF, MMR, mention frequency, node distance or cross-encoder [V] | **Bi-temporal**: `t_valid/t_invalid` (true in the world) and `t'_created/t'_expired` (when the system learned it) [V] | Graphiti Apache-2.0, about 29k stars, v0.29.2 (8 Jun 2026). Zep Community Edition is no longer updated; Zep cloud is commercial [V] | LongMemEval_S: gpt-4o 60.2 to **71.2**; context 115k to 1.6k tokens; latency 28.9 s to 2.58 s [V]. Weak models "did not fully understand" the temporal data [V] |
| **Letta (MemGPT)** | OS-style virtual context. Core memory blocks always in context, archival and recall memory outside it [V]. In 2026, "Context Repositories": memory is a git-backed folder of Markdown files with frontmatter. Root files load into the prompt; subfolders are disclosed progressively [V] | Local git repo, pushed to the cloud for hosted agents. No vector index by default [V] | Sleep-time agent or "dreaming" subagents work in **git worktrees** and merge back. A defragmentation skill keeps memory to 15 to 25 files [V] | grep and file tools, with optional search add-ons [V] | Not first-class [I] | Apache-2.0, about 25k stars. Development moved to letta-code; the V1 server is archived [V] | Filesystem agent scored 74.0% on LoCoMo with GPT-4o mini [V]. Sleep-time compute paper: about 5× less test-time compute, accuracy +13% to +18%, 2.5× lower cost per query, on stateful math tasks [V] |
| **LangMem** | Semantic, episodic and procedural memory. Procedural memory means **rewriting the prompt** (metaprompt / gradient / prompt_memory) [V] | LangGraph BaseStore [V] | Background manager extracts and consolidates [V] | Search tools [V] | None noted | MIT, 1.6k stars, low activity [V] | LoCoMo 58.1 (cited by Memanto) vs 78.05 (MIRIX paper): the harness changes the score [V] |
| **Cognee** | Pipeline that builds a knowledge graph from documents; operations are remember / recall / improve / forget [V] | Graph + vector + relational. A Postgres-only setup is a demo [V] | `improve` folds session knowledge into the graph [V] | Routed hybrid search [V] | Not first-class [I] | Apache-2.0, 31.2k stars, v1.6.1 (24 Sep 2026). Runs with no LLM key using GLiNER plus local embeddings [V] | BEAM 100K: 0.79; 10M: 0.67 (exploratory, with a reproduction gap) [V] |
| **MemOS** | "Memory OS" covering text, KV-cache (activation) and parameter memory. MemCube metadata holds origin signature, access control, TTL, priority and a version chain [V] | Neo4j + Qdrant. The local plugin uses **SQLite FTS5 + vectors** [V] | MemScheduler moves memory between types; lifecycle runs Generated → Activated → Merged → Archived [V] | Hybrid [V] | Timestamps [V] | Apache-2.0, 11.1k stars [V] | Paper (gpt-4o-mini): LoCoMo 75.80, LongMemEval 77.8. README (own harness): 88.83 / 89.20, BEAM-10M 56.75 [V] |
| **A-MEM** | Zettelkasten notes (content, time, keywords, tags, context, links). New notes **evolve** the context of older linked notes [V] | Vector store [V] | The LLM decides links and evolution [V] | Embedding match plus linked "box" expansion [V] | Timestamp only [V] | NeurIPS 2025; code on GitHub, license not verified | LoCoMo F1 (4o-mini): temporal **45.85** vs MemGPT 25.52, using 2,520 vs 16,977 tokens [V] |
| **HippoRAG 2** | Hippocampal-indexing analogy: an open knowledge graph plus passage nodes, Personalized PageRank, and an LLM "recognition memory" filter [V] | Files / vLLM [V] | Incremental indexing and deletion are tested [V] | Query-to-triple matching, then PageRank [V] | None | MIT, 4.0k stars, ICML 2025 [V] | Average F1 59.8 vs NV-Embed-v2 57.0. GraphRAG 49.6, LightRAG **6.6**, RAPTOR 48.8 [V] |
| **MIRIX** | Six types: Core, Episodic, Semantic, Procedural, Resource, **Knowledge Vault** (verbatim secrets with a sensitivity level). A meta-manager routes input to six manager agents [V] | Paper: SQLite. Repo now: PostgreSQL [V] | Managers update in parallel [V] | "Active retrieval": generate a topic, then top-10 from each type, tagged by source [V] | Episodic timestamps [V] | Apache-2.0, 3.4k stars [V] | LoCoMo 85.38 vs full-context 87.52 (gpt-4.1-mini). ScreenshotVQA 59.5 vs RAG 44.1 at 99.9% less storage [V] |
| **MemoryOS** | Short-, mid- and long-term personal memory. FIFO dialogue chains, segmented pages [V] | — | Promotion between tiers [V] | — | — | Open source (BAI-LAB) [V] | LoCoMo: +49.11% F1 on average over baselines (4o-mini) [V] |
| **Memobase** | "Memory for User, not Agent." A **profile** (topic/sub_topic) plus an event timeline [V] | Postgres + Redis [V] | Batch buffer flushes at about 1,024 tokens or after about 1 hour idle [V] | SQL reads under 100 ms [V] | Event timeline [V] | Apache-2.0, 2.8k stars [V] | MemOS table: LoCoMo 72.01; LongMemEval 72.4 with the **best knowledge-update score, 89.7** [V] |
| **Hindsight** | Four networks: world facts, the agent's own experiences, opinions with confidence, observations (entity summaries). Operations: retain / recall / reflect [V] | Postgres + pgvector; embedded mode available [V] | Reflection forms and updates opinions [V] | **Four parallel channels**: semantic, BM25, graph spreading-activation, temporal. Then RRF, cross-encoder, token budget [V] | Each fact has an occurrence interval *and* a mention time; relative dates become absolute [V] | MIT, 44.6k stars, Ollama, MCP endpoint per bank [V] | LongMemEval: 20B open model **39.0 (full context) to 83.6**; up to 91.4. LoCoMo up to 89.61 [V] |
| **EverMemOS** | Engram-inspired: MemCells (episode, atomic facts, time-bounded **"Foresight"**), then MemScenes, then profiles [V] | Local Markdown + SQLite + LanceDB [V] | Semantic consolidation [V] | Agentic retrieval guided by MemScenes [V] | Foresight carries validity windows [V] | Apache-2.0, 13.4k stars [V] | Claims state of the art; I did not verify the numbers |
| **Mastra Observational Memory (OM)** | **No retrieval.** An Observer compresses old messages into dated, prioritised observations; a Reflector condenses and drops superseded ones. The prefix stays stable, so it can be cached [V] | Text log | Two token thresholds [V] | None (always in context) [V] | Dated observations [V] | Open source per Mastra; license not verified | LongMemEval_S **94.87** (gpt-5-mini), 84.23 (gpt-4o). Multi-session 87.2 is the hardest category. About 6× compression [V] |
| **Agent Zero Memory** (Aug 2026) | Three views over provenance-tagged history: event timeline, entity–event graph, curated documentary facts. "Citation lock" with abstention [V] | — | Curation [V] | Three agentic searches, each embedding + BM25 fused by RRF (k=60) [V] | Timeline [V] | No code found | LongMemEval 95.60, LoCoMo 93.60. An AI review notes the margins (+0.73 / +1.10) are within standard error and the baselines came from different harnesses [V] |

**Others:**
- **LightRAG**: MIT, 39.3k stars; incremental insert, deletion, citations [V]. Weak on QA in the HippoRAG 2 comparison [V].
- **Memary**: MIT, 2.6k stars, maintenance unclear [V].
- **Second Me**: L0 raw data, L1 natural-language memory, L2 memory in model weights (Qwen2.5-7B, SFT + DPO). It was evaluated on one staff member's data: 132 notes and 62 todos [V]. Too thin to adopt [I].
- **Memanto**: 13 typed categories, vector only. LongMemEval 89.8. Raising retrieval from 10 to 40 chunks gave **+20.4 points**, the largest single gain in its ablation [V].
- **MemPalace** (Apr 2026): 96.6% is *retrieval recall*, not QA accuracy. The critics say it comes from verbatim storage plus default ChromaDB embeddings; its palace features *lowered* accuracy, to 89.4 and 84.2 [V; one critique is by competitor Vectorize].
- **Maximem Synap**: LongMemEval 92%, LoCoMo 93.2% [V].
- **Learned memory managers**:
  - Memory-R1 uses RL over ADD/UPDATE/DELETE/NOOP, trained on 152 QA pairs [V].
  - AgeMem (2026) trains store/retrieve/update/summarize/discard as RL actions [V via survey].

---

## 2. Benchmarks: what they measure and what still fails

| Benchmark | What it is | State of the art and failures |
|---|---|---|
| **LoCoMo** (2024) | About 300 turns and about 9K tokens on average, up to 35 sessions [V]. Zep says 16 to 26K [V] | Short enough to fit in context. Audit (Apr 2026): **99 of 1,540 answers wrong (6.4%)**, so the realistic ceiling is about 93.6%. The gpt-4o-mini judge accepted **62.81%** of vague wrong answers. No knowledge-update category [V] |
| **LongMemEval** (ICLR 2025) | 500 questions. S is about 115k tokens; M is about 1.5M tokens over 500 sessions. Five abilities: information extraction, multi-session reasoning, temporal reasoning, knowledge updates, abstention [V] | Original paper: ChatGPT scored 57.7% vs 91.8% when reading offline. Design findings: store **individual rounds, not sessions**; fact-augmented keys gave recall +9.4% and QA +5.4%; time-aware query expansion gave temporal recall +6.8 to 11.3%; Chain-of-Note gave up to +10 points [V]. Best now about 95%. Multi-session reasoning (about 87%) is the hardest category [V] |
| **MemoryAgentBench** (ICLR 2026) | Input arrives incrementally. Four competencies: accurate retrieval, test-time learning, long-range understanding, **selective forgetting** [V] | RAG wins retrieval; long context wins learning and long-range understanding. **Every method scores ≤7% on multi-hop FactConsolidation** [V] |
| **BEAM** (ICLR 2026) | 100 conversations of 100K to 10M tokens; 2,000 questions; 10 abilities [V] | 1M-context models average 0.19 to 0.26. LIGHT (episodic + working memory + scratchpad) improves on the best baseline by 3.5 to 12.69%. **Contradiction resolution is weakest for every method** [V] |
| **MemoryArena** (Feb 2026, ICML 2026) | Tasks whose sessions depend on each other: shopping, group travel, progressive search, formal reasoning [V] | Success rates 0.12 to 0.23. **Group travel is 0 for every method.** Memory systems do not reliably beat long context, and they are slower (99 to 133 s per subtask) [V] |
| **HaluMem** (Nov 2025) | Tests extraction, updating and QA as separate stages; more than 1M tokens [V] | Hallucinations **build up during extraction and updating**, then carry into QA [V] |
| **PersonaMem** | More than 180 histories, up to 60 sessions [V] | Frontier models score about 50%. Tracking how a profile changes over time is the hard part [V] |

**Does full context beat memory?** Pollertlam & Kornsuwannawit (Mar 2026) compared GPT-5-mini reading the full history against Mem0 flat facts [V]:

| Benchmark | Full context (GPT-5-mini) | Mem0 flat facts |
|---|---|---|
| LoCoMo | 92.85 | 57.68 |
| LongMemEval | 82.40 | 49.00 |
| PersonaMem v2 | 69.75 | 62.48 |

- Memory becomes cheaper after about 10 turns at 100k tokens, even with caching [V].
- Chroma's context-rot study points the other way. Every model family did better on the focused LongMemEval prompt (about 300 tokens) than on the full one (about 113k) [V].
- **[I] My reading:** memory beats full context when two things hold. The reader is weak, or the history is beyond about 100k tokens. And the memory layer keeps verbatim evidence instead of only lossy facts.

---

## 3. Context engineering for long agent runs

- **Context rot** (Chroma, 14 Jul 2025; 18 models) [V]:
  - Performance drops with length, and faster when the needle is semantically far from the question.
  - A single distractor already hurts.
  - Shuffled haystacks did *better* than coherent ones.
  - On ambiguity, Claude tended to abstain rather than guess.
- **Lost in the middle** (Liu et al., 2023): models recall best from the start and end of the context and worst from the middle [V].
- **Anthropic's context engineering guidance** (29 Sep 2025) [V]:
  - Use the smallest high-signal set of tokens; context is an "attention budget".
  - Tools should be token-efficient and not overlap.
  - Prefer **just-in-time retrieval through lightweight identifiers** (paths, queries) over pre-loading, with a hybrid approach like Claude Code: CLAUDE.md loads up front, grep and glob run on demand.
  - For long runs: compaction, then tool-result clearing (the "lightest touch"), then structured note-taking, then sub-agents that return 1,000 to 2,000-token digests.
- **Claude API features** [V]:
  - Context editing (beta `context-management-2025-06-27`):
    - `clear_tool_uses_20250919` takes `trigger` (default 100k input tokens), `keep` (default 3), `clear_at_least`, `exclude_tools` and `clear_tool_inputs`.
    - `clear_thinking_20251015` manages old thinking blocks.
  - Server-side compaction `compact_20260112` is the recommended primary strategy.
  - Memory tool `memory_20250818`:
    - Runs **client-side**: Claude asks for `view/create/str_replace/insert/delete/rename` under `/memories`, and your code executes it.
    - The API auto-injects "ASSUME INTERRUPTION".
    - Your handler must block path traversal.
  - Measured on Anthropic's internal agentic-search evaluation: memory + context editing **+39%**, context editing alone **+29%**, **84% fewer tokens** on a 100-turn web-search task.
- **Manus lessons** (18 Jul 2025) [V]:
  - KV-cache hit rate is the key metric. On Sonnet, cached input costs $0.30/MTok vs $3/MTok uncached, and Manus's input:output ratio is about 100:1.
  - Keep the prefix stable (no timestamps at the top). Keep context append-only and serialise deterministically.
  - **Mask tools rather than removing them.**
  - Treat the **filesystem as context**, with *restorable* compression: drop the page, keep the URL.
  - **Recite a todo.md** (tasks average about 50 tool calls) to fight goal drift.
  - Keep failures in the context. Vary the format so the agent doesn't copy its own past patterns.
- **Sub-agents** (Anthropic's multi-agent research system) [V]:
  - The multi-agent setup scored +90.2% over a single agent on the internal evaluation, but used about 15× the tokens of chat.
  - Token usage explains 80% of BrowseComp variance.
  - Sub-agents compress by working in their own context windows.
- **Observational memory as a cache-friendly memory** [V]: Mastra keeps one stable prefix (observations) and appends messages. This avoids retrieval that changes every turn and breaks the cache.

**[I] What this means for the owner's "program files" idea.** The evidence supports executable capabilities that sit in context only as a name and signature, and that return **small handles** (paths, IDs, digests) rather than raw output. That follows from Manus's restorable compression, Anthropic's advice on token-efficient tools, and tool-result clearing.

---

## 4. Cognitive-science framing: which mechanisms show measured gains

| Mechanism | How the systems implement it | Measured evidence |
|---|---|---|
| Working memory | Core blocks (MemGPT/Letta), `system/` files, LIGHT's recent-turn buffer | Indirect. LIGHT beats 1M-context baselines by 3.5 to 12.69% [V] |
| Episodic (verbatim) | Zep episodes, LongMemEval round-level storage | **Positive.** Round granularity beats sessions; verbatim storage drives MemPalace's recall [V] |
| Semantic (extracted facts) | Mem0, Memanto, Hindsight world facts | **Mixed.** Facts alone help multi-session reasoning but hurt overall accuracy (LongMemEval). Flat facts are about 33 points below full context (2603.04814). Extraction is where hallucinations start (HaluMem) [V] |
| Procedural | Voyager code skills, Agent Workflow Memory (AWM), ReasoningBank, LangMem prompt rewriting | **Strongest evidence.** Voyager: tech-tree milestones up to 15.3× faster. AWM: +24.6% on Mind2Web, +51.1% on WebArena (relative). ReasoningBank (learns from failures too): +8.3% WebArena, +4.6% SWE-bench Verified, about 3 fewer steps [V] |
| Reflection | Generative Agents, Reflexion, Hindsight opinions | **Positive.** Generative Agents TrueSkill 29.89 full vs 26.88 without reflection. Reflexion 91% vs 80% pass@1 on HumanEval [V] |
| Retrieval scoring (recency × importance × relevance) | Generative Agents: decay 0.995 per game-hour, importance rated 1 to 10, reflection when importance sums past 150 [V] | Not ablated on its own [I] |
| Consolidation / "sleep" | Letta sleep-time agents and dreaming, EverMemOS, Mastra Reflector | Gains shown only on stateful reasoning tasks (5×, +13 to 18%). A March 2026 survey finds no measured gains on memory benchmarks [V] |
| Forgetting curves | MemoryBank (Ebbinghaus decay) | **Unproven.** The evaluation is qualitative, and every system fails selective forgetting [V] |
| Hippocampal indexing | HippoRAG 2 | Modest: +2.8 average F1 over strong dense retrieval [V] |

---

## 5. What a personal assistant needs that the benchmarks don't measure

- **Provenance.** Zep links each fact back to its episodes. MemCube carries an origin signature and a version chain. Agent Zero only answers from evidence it actually read, and abstains otherwise [V].
- **Memories the user can see and edit.**
  - Simon Willison prefers Claude's memory over ChatGPT's because it appears as visible tool calls; the Team/Enterprise memory summary is editable and scoped per project [V].
  - Letta MemFS and Claude's memory tool make memory plain files you can diff [V].
- **Privacy and local-first.**
  - The Ink & Switch local-first ideals: fast, multi-device, offline, collaboration, longevity, private by default, user ownership; CRDTs as the foundation [V].
  - Cognee runs with no LLM key, EverMemOS keeps a local Markdown/SQLite/LanceDB stack, and the MemOS local plugin uses SQLite with FTS5 [V].
- **Structured vs unstructured data.**
  - Memobase's typed profile has the best knowledge-update score. MIRIX's Knowledge Vault tags each entry with a sensitivity level [V].
  - [I] Calendar, tasks and contacts should stay as authoritative typed tables that memory *links to*. They should not be re-extracted as text facts.
- **"As of when?"** Zep separates when something was true from when the system learned it. Hindsight separates when it happened from when it was mentioned. LongMemEval's time-aware query expansion helps temporal recall [V].
- **Contradictions.** These are the weakest ability on BEAM and MemoryAgentBench [V].
- **Security.**
  - MINJA poisons an agent's memory through queries alone, with a 98.2% injection success rate and a 76.8% attack success rate.
  - Sanitising embeddings and detecting poison at the prompt level both failed in that paper [V].
  - An assistant that reads email and the web is exposed to this by default [I].

---

## 6. Design principles for Gwen's memory

1. **Keep one canonical, append-only, verbatim episode log in gwend's SQLite, and treat everything else as a rebuildable projection.** Facts, the graph, profiles, embeddings, and any Mem0 or Supermemory index all count as projections. *Evidence:* Zep's episodes are its non-lossy store; storing rounds verbatim wins on LongMemEval; flat facts lose about 33 points; hallucinations build up in extraction (HaluMem). This is how to do the "hybrid of Mem0 + Supermemory + our own" safely: the external engines become **swappable indexers fed from Gwen's log**, never the source of truth [I].

2. **Do multi-channel retrieval fused by RRF, in-process.**
   - Channels: FTS5 BM25, sqlite-vec, entity/graph hops, temporal filters, plus Mem0 and Supermemory results as extra ranked lists.
   - `modernc.org/sqlite` (already v1.59.0 in Gwen's go.mod) ships sqlite-vec support without cgo [V].
   - *Evidence:* Zep, Hindsight and Agent Zero all fuse several channels; removing any one channel cost 1.2 to 1.8 points [V].
   - **Retrieve generously and let the reader filter.** Memanto gained +20.4 points by going from 10 to 40 chunks [V].

3. **Make facts bi-temporal and keep both kinds of time.**
   - Store `valid_from/valid_to` and `recorded_at/superseded_at` (Zep) [V].
   - Also store *occurrence* vs *mention* time, and resolve relative dates to absolute ones at write time (Hindsight) [V].
   - Expose an `as_of` parameter on every memory query [I].

4. **Supersede facts instead of deleting them, and treat contradictions as objects.** When a new fact conflicts with an old one, create a `conflict` record. Resolve it automatically only above a confidence bar; otherwise ask the user. *Evidence:* every system fails selective forgetting and contradiction resolution [V].

5. **Use a two-tier context.**
   - **Tier 1** is a stable, prompt-cached prefix: persona, the user profile, and a compact dated observation log (Mastra OM: 94.87% on LongMemEval, and cacheable) [V].
   - **Tier 2** is just-in-time retrieval tools for the long tail (Anthropic) [V].
   - Never put volatile content ahead of the cached prefix (Manus) [V].

6. **Put provenance on every derived item, lock answers to citations, and abstain when evidence is missing.** Each memory carries `source_episode_ids`, `extractor_model`, `confidence` and `trust_tier`. Gwen answers "I don't have that" rather than guessing (Agent Zero, MemCube) [V].

7. **Keep personal data typed.**
   - Calendar, tasks, contacts and places stay in their own tables. Memories reference them by ID.
   - Use a small, fixed set of memory kinds. The harness's fact / preference / person / episode / lesson set is close; add `observation` and `commitment` [I].
   - *Evidence:* Memobase's profile wins knowledge updates; tracking profile changes is the hard part in PersonaMem [V].

8. **Run sleep-time consolidation as a separate low-priority worker.** It can run on the Pi hub or an idle laptop, on a local model. It produces **reviewable diffs**: dedupe, profile refresh, contradiction scan, reflection, and distilling procedures. *Evidence:* Letta sleep-time agents and worktree-based dreaming; the sleep-time compute numbers; Generative Agents' reflection ablation [V].

9. **Make procedural memory executable ("program files") and nearly free in context.**
   - In context, each program file is a name, signature and one-line contract. Its body runs outside the model. Results come back as handles.
   - Learn procedures from failures too (ReasoningBank), and abstract the steps into workflows (AWM) [V].
   - *Evidence:* procedural memory has the strongest measured gains of any memory type (Voyager, AWM, ReasoningBank) [V].

10. **Give each agent run its own working memory.**
    - Each run gets a scratchpad / todo.md that it recites (Manus).
    - Clear tool results and compact. On the Anthropic provider, use `clear_tool_uses_20250919`, `compact_20260112` and `memory_20250818`, which together gave +39% and 84% fewer tokens [V].
    - Re-implement the same semantics in gwend for Ollama and OpenAI-compatible providers [I].
    - Use sub-agents only where isolating their context is worth roughly 15× the tokens [V].

11. **Show memory as human-readable files.** Project memory into a Markdown/git view, in the style of Letta MemFS or the Claude memory tool. User edits flow back as user-authored episodes with the highest trust level [V/I].

12. **Gate memory writes by trust tier.**
    - Content from email, the web or imported documents cannot create durable facts or procedures without corroboration or the user's approval. It sits in quarantine until then.
    - Every memory write goes through the harness ledger.
    - *Evidence:* MINJA's 98.2% injection success rate, and the defenses tested in that paper failed [V].

13. **Use local-first sync.**
    - Episodes are append-only, so they replicate trivially. Fact edits use per-field last-writer-wins with hybrid logical clocks, or CRDTs [I; grounded in Ink & Switch, V].
    - Each device recomputes projections itself, with a pinned embedding-model version [I].

14. **Let the model and the history size decide the policy.**
    - For a frontier model and less than about 100k tokens of relevant history, load the history directly. For local models or longer histories, use memory.
    - *Evidence:* full context beats naive memory on GPT-5-mini; memory lifts a 20B model from 39.0 to 83.6 [V].

15. **Build Gwen's own evaluation harness instead of trusting leaderboards.**
    - Run these with one fixed judge and confidence intervals: LongMemEval-S, a BEAM slice, MemoryAgentBench's FactConsolidation, a MemoryArena-style multi-session task, and Gwen-specific evaluations (calendar-grounded questions, "as-of" queries, contradictions, poisoning).
    - *Evidence:* the LoCoMo audit, MemPalace's overfitting, Zep's four different scores, and the standard-error critique of Agent Zero [V].

---

## 7. Open problems where Gwen could innovate

1. **Selective forgetting and contradiction handling across multiple hops.** It is ≤7% everywhere [V]. A bi-temporal graph combined with explicit conflict objects that the user can resolve is not something any system ships [I].
2. **Memory for action across sessions.** MemoryArena's group-travel task is 0% for every method [V]. Gwen's briefs, runs and outbox could carry task state that the agent re-reads, not only facts [I].
3. **Memory that is cache-aware.** Nobody publishes a design that combines a stable cached prefix with just-in-time retrieval without breaking the cache. Gwen could add "append-only memory deltas" at the tail of the context [I].
4. **Extraction you can check.** HaluMem shows errors pile up at write time [V]. Gwen could check each extracted fact against its verbatim source before committing it [I].
5. **Reflection that doesn't reinforce its own mistakes.** The March 2026 survey names confidence decay and external validation as open problems [V].
6. **Defending against memory poisoning in a personal assistant** that reads untrusted email and web content [V/I].
7. **Executable procedural memory with provenance and evaluations.** This is the owner's "program files" idea. Voyager showed the gain, but no memory system unifies skills, capabilities, versioning, evaluations and a near-zero context footprint [I].
8. **Bi-temporal reasoning by small local models.** Zep reports that weak models misread its temporal data [V].
9. **Prospective memory**: commitments, reminders and EverMemOS-style "Foresight", tied to the calendar [V/I].
10. **Consistency of derived memory across devices.** CRDTs cover rows, but not graphs or embeddings computed by LLMs [I].
11. **Multimodal personal memory** (screen, voice). MIRIX's ScreenshotVQA is the only serious data point I found [V].
12. **Evaluations for latency, token efficiency and context rot** in memory benchmarks. These are explicitly missing [V].

---

## Sources

- https://arxiv.org/html/2501.13956v1
- https://github.com/getzep/graphiti
- https://www.getzep.com/blog/announcing-a-new-direction-for-zeps-open-source-strategy/
- https://blog.getzep.com/lies-damn-lies-statistics-is-mem0-really-sota-in-agent-memory/
- https://github.com/letta-ai/letta
- https://www.letta.com/blog/context-repositories
- https://docs.letta.com/letta-code/memfs
- https://docs.letta.com/guides/agents/architectures/sleeptime
- https://letta.com/blog/sleep-time-compute
- https://arxiv.org/abs/2504.13171
- https://www.letta.com/blog/benchmarking-ai-agent-memory
- https://arxiv.org/abs/2310.08560
- https://www.langchain.com/blog/langmem-sdk-launch
- https://github.com/langchain-ai/langmem
- https://github.com/topoteretes/cognee
- https://arxiv.org/pdf/2507.03724v4
- https://github.com/MemTensor/MemOS
- https://arxiv.org/abs/2502.12110
- https://arxiv.org/pdf/2502.12110v11
- https://arxiv.org/html/2502.14802v1
- https://github.com/OSU-NLP-Group/HippoRAG
- https://arxiv.org/html/2507.07957v1
- https://github.com/Mirix-AI/MIRIX
- https://arxiv.org/abs/2506.06326v1
- https://github.com/memodb-io/memobase
- https://github.com/HKUDS/LightRAG
- https://github.com/kingjulio8238/memary
- https://arxiv.org/html/2503.08102v1
- https://arxiv.org/html/2512.12818v1
- https://github.com/vectorize-io/hindsight
- https://arxiv.org/abs/2601.02163v1
- https://github.com/EverMind-AI/EverMemOS
- https://mastra.ai/research/observational-memory
- https://arxiv.org/abs/2608.29606
- https://pith.science/paper/2608.29606
- https://arxiv.org/html/2604.22085v1
- https://arxiv.org/abs/2604.21284
- https://vectorize.io/articles/mempalace-review
- https://arxiv.org/abs/2607.21503
- https://arxiv.org/abs/2508.19828v4
- https://arxiv.org/abs/2402.17753
- https://penfieldlabs.substack.com/p/we-audited-locomo-64-of-the-answer
- https://arxiv.org/html/2410.10813v2
- https://arxiv.org/html/2507.05257v2
- https://iclr.cc/virtual/2026/poster/10010781
- https://arxiv.org/html/2510.27246v1
- https://arxiv.org/html/2602.16313v1
- https://arxiv.org/abs/2511.03506v2
- https://arxiv.org/abs/2504.14225v1
- https://arxiv.org/html/2603.04814
- https://arxiv.org/pdf/2603.07670
- https://www.trychroma.com/research/context-rot
- https://arxiv.org/abs/2307.03172v3
- https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- https://platform.claude.com/docs/en/build-with-claude/context-editing
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool
- https://claude.com/resources/articles/context-management
- https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- https://www.anthropic.com/engineering/multi-agent-research-system
- https://arxiv.org/abs/2304.03442
- https://arxiv.org/pdf/2304.03442
- https://arxiv.org/abs/2305.16291
- https://arxiv.org/html/2409.07429v1
- https://arxiv.org/abs/2509.25140
- https://research.google/blog/reasoningbank-enabling-agents-to-learn-from-experience/
- https://ojs.aaai.org/index.php/AAAI/article/view/29946
- https://arxiv.org/html/2503.03704v5
- https://simonwillison.net/2025/Sep/12/claude-memory/
- https://inkandswitch.com/local-first/
- https://blog.acolyer.org/2019/11/20/local-first-software/
- https://gorse.io/posts/sqlite-vec