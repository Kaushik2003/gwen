# Gwen's data model, sync and memory: what the code does today, and the v2 changes it needs

**Scope.** I read the repo at commit `b029c77`, without writing to it. Every file:line below is in `/home/claude/gwen`. **[V]** means I read it in the code or a cited source. **[I]** means it is my own judgement. Code comments point to `docs/03-data-model.md`, `docs/04-api-contract.md` and `docs/07-integrations.md`, but there is no `docs/` directory in this checkout **[V]**. The code is the only ground truth here. `protocol/` holds only the Wayland `ext-idle-notify-v1.xml`. There is no sync protocol directory **[V]**.

## 0. Headline findings

1. **Gwen has no memory layer today.** Her only memory across chats is a JSON blob in `local_state` holding one attitude and a self-note of at most 600 characters (`internal/store/localstate.go:25-27`, `internal/llm/persona.go:18-20`) **[V]**. That blob is device-local and never synced **[V]**.
2. **Every turn sends the whole state again.** Each assistant turn serialises up to 150 open tasks, every active goal, 7 days of plan blocks, 7 days of worked time, the latest weekly review and the last 24 messages into one JSON user message (`internal/api/assistant.go:215-350`, `internal/llm/assistant.go:12-17`) **[V]**. There is no retrieval, no summarising and no prompt caching. The Anthropic request has no `cache_control` field (`internal/llm/providers.go:25-30, 53-55`) **[V]**.
3. **A chat is not stored as rows.** Each turn inserts a *new* `llm_runs` row holding the whole conversation as JSON (`assistant.go:72-75`, `store/llm.go:64-79`) **[V]**. The client remembers which run to continue in browser `localStorage` (`ui/src/components/AssistantChat.tsx:28-45`) **[V]**. Chats are therefore tied to one device and are not synced **[V]**.
4. **The Pi sync protocol is row-level last-writer-wins.** The tie-break is `(updated_at, device_id)` (`internal/store/sync.go:268`). Every pushed row must carry exactly the schema's column set (`sync.go:184-208`) **[V]**. There is no schema-version negotiation, so any added synced column breaks sync until the hub is rebuilt. `setup.md:347-350` documents this **[V]**.
5. **Hybrid search fits in the database Gwen already has.** The pinned `modernc.org/sqlite v1.59.0` ships a CGo-free sqlite-vec v0.1.9 as `modernc.org/sqlite/vec`, including `linux/arm64` for the Pi. Its SQLite is compiled with `-DSQLITE_ENABLE_FTS5` **[V]**. Vector plus BM25 search can live in the same `gwen.db` with no new driver.

## 1. Storage conventions

- **Only the daemon opens the database.** `gwend` is the only process that opens `<XDG_DATA_HOME>/gwen/gwen.db` (`store/db.go:1-2, 27`) **[V]**.
- **Connection settings:** WAL, `foreign_keys(1)`, `busy_timeout(5000)`, `synchronous(NORMAL)`, `_txlock=immediate`, and 4 open connections at most (`db.go:62-69`) **[V]**. The store refuses to open if the journal mode is not WAL (`db.go:84-86`) **[V]**.
- **Migrations:** goose v3.28.0 runs embedded `migrations/*.sql`, which are Up-only (`db.go:23-24`, `migrate.go:26-52`). Before migrating, the store backs up with `VACUUM INTO gwen.db.bak-<ver>` and keeps 2 backups (`migrate.go:17-20, 56-69`) **[V]**.
- **IDs:** UUIDv7, 36-character text, enforced by `CHECK (length(id) = 36)` (`model/model.go:104-106`) **[V]**.
- **Time:** instants are `INTEGER` Unix milliseconds, read back as UTC (`store/scan.go:14`). Local days are `TEXT 'YYYY-MM-DD'` checked by `GLOB` **[V]**. Durations are minutes or seconds in integer columns **[V]**.
- **Every table is `STRICT`.** Live-row uniqueness uses partial indexes `WHERE deleted_at IS NULL`. Enumerations are closed `CHECK (x IN (...))` constraints **[V]**. Adding a kind to `llm_runs` has already forced two table rebuilds (`00005_day_plans.sql:18-31`, `00007_methods.sql:51-64`) **[V]**.
- **The clock is injected.** `clock.Clock` (`internal/clock/clock.go:9-11`) is passed into `store.Open`, and `db.Now()` truncates to milliseconds (`db.go:111`) **[V]**.
- **Device identity:** `device_id` is a UUIDv7 created once and stored in `local_state` (`db.go:142-153`) **[V]**. No table describes what a device is **[V]**.

## 2. Schema (migrations 00001 to 00008)

| Table (migration) | Purpose | Key columns | Synced | Notes |
|---|---|---|---|---|
| `local_state` (1) | device key-value store | key, value, updated_at | no | closed key list, §4 |
| `projects` (1) | projects | name (unique among live rows, ignoring case), color `#rrggbb`, archived_at | yes | |
| `tasks` (1, extended in 2, 4, 6, 7) | tasks, steps, recurring templates and occurrences, quantity items | project_id, title ≤200, notes, status open/done, priority 1–4, due_day, estimate_minutes, done_at, goal_id, quantity, quantity_done, rrule, template_id, occurrence_day, parent_id (a step), start_day, start_minute, stage inbox/todo/doing/waiting/someday, effort 1–3, delegated_to | yes | `CHECK((status='done')=(done_at IS NOT NULL))` |
| `work_days` (1) | one clocked day | day, tz, clocked_in_at, clocked_out_at, target_seconds, note | yes | one open day at a time (unique partial index) |
| `segments` (1) | time-tracking segments | work_day_id, kind work/break_auto/break_manual, source user/activity/idle/lock/suspend/recovery/edit, project_id, task_id, started_at, ended_at, truncated | yes | one open segment at a time |
| `engine_events` (1) | audit trail of state transitions | INTEGER id, at, trigger, from_state, to_state, data JSON | no | pruned after 90 days (`cmd/gwend/daemon.go:29, 97`) |
| `goals` (2, extended in 4, 7) | quantity or task goals | kind, unit, target_quantity, minutes_per_unit, project_id, start_day, due_day, status, daily_minutes, SMART text (specific, measurable, assignable, realistic) | yes | |
| `commitments` (2) | recurring fixed blocks | rrule, start_minute, duration_minutes, counts_toward_target, active_from, active_until | yes | |
| `plan_items` (2) | planned blocks per day | day, task_id, planned_minutes, start_at, position, status planned/done/rolled/skipped, pinned, rolled_from_id, rollover_count | yes | |
| `gcal_links` (3) | plan item to Google event | plan_item_id, event_id, etag, synced_at | no | |
| `calendar_busy` (3, extended in 8) | cache of busy calendar time | calendar_id, event_id, start_at, end_at, fetched_at, title | no | replaced by range (`store/calendar.go:166-169`) |
| `llm_runs` (3, rebuilt in 5 and 7) | every LLM job and its JSON output | kind breakdown/retro/day_plan/assistant, subject_id, status ok/failed/accepted/rejected, output JSON | no | never pruned; assistant runs use subject_id = today |
| `hub_changes` (3) | hub-only change feed | seq AUTOINCREMENT, table_name, row_id, device_id | no | unique on (table_name, row_id) |
| `day_hours` (5) | per-day start and work-minutes override | day, start_minute, work_minutes | yes | |
| `weekly_reviews` (7) | the user's written weekly review | week_start (a Monday), went_well, went_badly, energy, decisions, improvements, checklist | yes | |
| `energy_logs` (7) | energy check-ins | at, level 1–5 | yes | |

`model.Task.IsStep()` means the task has a `parent_id` (`model.go:176`) **[V]**. The assistant filters steps out before sending tasks (`assistant.go:288`) **[V]**.

## 3. The sync envelope and the Pi hub protocol

**Envelope.** Every synced table ends with `created_at, updated_at, deleted_at, device_id, rev` (`sync.go:44`; `model.Envelope` at `model.go:108-115`) **[V]**.

**Synced tables.** `SyncTables` lists them in dependency order: projects, goals, tasks, commitments, work_days, segments, plan_items, day_hours, weekly_reviews, energy_logs (`sync.go:18-19`) **[V]**. `syncColumns` gives each table's exact column set (`sync.go:23-41`) **[V]**.

**Push** (`internal/sync/client.go:157-201`) **[V]**:
- The client reads every row in `SyncTables` with `updated_at >= watermark − 5 min` (`client.go:27, 163`; `sync.go` `Changed`).
- It sends batches of 500 rows (`client.go:26`) as `POST /sync/v1/push` with a bearer token.
- It then sets `sync_push_watermark` to the newest `updated_at` it sent.

**Hub apply** (`sync.go:210-250`) **[V]**:
- `checkRow` validates every row first. A single row with the wrong column set fails the whole push with `ErrInvalid` (400).
- Rows are then written in one transaction with `PRAGMA defer_foreign_keys = ON`, each inside a savepoint.
- `INSERT … ON CONFLICT(id) DO UPDATE … WHERE (excluded.updated_at, excluded.device_id) > (table.updated_at, table.device_id)` (`sync.go:266-268`).
- A row that breaks a constraint, such as a duplicate live project name or a second open work day, is skipped and counted as "ignored", with only a log line (`sync.go:280-283`).
- On the hub, each applied row replaces its `hub_changes` entry, which bumps its seq.

**Pull** (`sync.go:293-345`, `client.go:205-248`) **[V]**:
- Returns rows whose `hub_changes.seq > cursor` and whose last writer is not the requesting device, up to `PullLimit` 1000 (`hub.go:29`).
- The cursor is stored as `sync_pull_cursor`.
- Pulled pages are applied with `hub=false`. `OnApplied` turns them into SSE invalidations (`cmd/gwend/sync.go:20`).

**How conflicts are decided:**
- Last writer wins per whole row, using wall-clock `updated_at` **[V]**.
- `rev` is stored, and only checked to be ≥ 1. It plays no part in resolving conflicts **[V]**.
- Two devices editing different fields of the same row will lose one edit **[I]**.
- Clock skew between devices can make the older edit win **[I]**.

**Network and auth:**
- The hub is `gwend --hub`, serving plain HTTP over TCP (`cmd/gwend/hub.go:58-98`). There is no TLS in-process. `setup.md:249-276` relies on Tailscale for that **[V]**.
- Auth is a bearer token, or a 30-day HMAC cookie for the phone dashboard (`hub.go:86-115`) **[V]**.
- The hub serves `ReadOnlyHandler`: GET routes only, with no assistant chat (`internal/api/routes.go:113-142`). `setup.md:344` says "It is read-only… every change still happens on the laptop" **[V]**. **The phone cannot talk to Gwen today.**

**Schema version is never negotiated:**
- `GET /sync/v1/health` reports `schema_version` (`hub.go:165-167`), but the client never checks it **[V]**.
- A version mismatch shows up only as rejected pushes (`setup.md:347-350`) **[V]**.

## 4. Local key-value store

The keys form a closed list, and `SetLocal` rejects any other key (`localstate.go:12-35, 55-65`) **[V]**:

- `device_id` is written only by bootstrap.
- `heartbeat_at` and `shutdown_at` are Unix milliseconds, used for crash recovery (`daemon.go:198-206`).
- `gcal_calendar_id` and `gcal_sync_token`.
- `sync_push_watermark` and `sync_pull_cursor`.
- `planned_day`.
- `assistant_self` is the JSON of `wire.AssistantSelf{attitude, note, since}` (`internal/wire/methods.go:112-116`).

Configuration lives in TOML outside the database: `llm.assistant_name` and `llm.instructions` (`internal/config/config.go:101-111`). These are the user's standing instructions, appended to every persona prompt (`persona.go:96-100`). They are local to each device as well **[V]**.

## 5. Everything that acts as memory today

### 5.1 Persona and self

- **The persona** is `internal/llm/gwen.md`, 10,910 bytes, embedded at `persona.go:12-16` **[V]**. Its section 9 "Memory and Continuity" (`gwen.md:163-177`) tells her to remember ambitions, projects, deadlines and preferences "when reliable memory is available", and never to invent memories **[V]**. No such memory exists.
- **The self** has 8 attitudes (`persona.go:30-40`) and 12 moods (`assistant.go:36-37`) **[V]**.
- `PersonaPrompt` builds the system prompt in this order: persona, then the user's name for her, then "How you have chosen to treat them" with the attitude, the day it was chosen and the self-note, then `llm.instructions` (`persona.go:72-102`) **[V]**.
- Because the self-note sits *inside the system prompt*, every rewrite of it would break a cached prefix **[I]**.
- The model rewrites the note whole, at most 600 characters (`assistant.go:218-223, 307`). It is saved by `changeSelf` (`api/assistant.go:134-164`) and reset by `DELETE /v1/assistant/self` (`api/assistant.go:96-102`) **[V]**.
- `api/llm.go:20-33` `planner()` loads the self on every request and builds a fresh adapter around it (`llm/llm.go:52-91`) **[V]**.

### 5.2 Chat history

- `POST /v1/assistant/chat` accepts `{message, run_id}` (`wire/methods.go:85-88`) **[V]**.
- The handler loads the earlier run's `messages` (`api/assistant.go:166-183`), appends the user's turn, calls the model, applies the actions, and **creates a new run** holding the whole history (`api/assistant.go:33-82`) **[V]**.
- Storage therefore grows quadratically with conversation length **[I]**.
- There is no cap on an assistant conversation's length. The day-plan chat caps at `MaxUserMessages = 20` (`llm/dayplan.go:14`, `api/dayplan.go:94-96`) **[V]**.
- Only the newest 24 messages are sent (`api/assistant.go:346-348`). Older ones are dropped with no summary **[V]**.
- A message's stored fields are role, text, at, mood, attitude, and actions (as `ActionResult{type, ok, summary, error, task_id|project_id|goal_id}`) (`wire/methods.go:99-128`) **[V]**.
- There is no endpoint to list conversations (`routes.go:87-95`) **[V]**.

### 5.3 What the model sees on each turn

`llm.AssistantRequest` (`llm/assistant.go:41-55`) is built by `assistantRequest` (`api/assistant.go:215-350`) **[V]**:

| Field | What it holds |
|---|---|
| today, weekday, now | the date and current time |
| methods | eat_the_frog, prime_time |
| hours | today's start, end and capacity |
| busy | today's commitments and calendar events, with titles |
| free | today's free time from now on |
| projects | every live project, as refs p1… |
| goals | every active goal, as refs g1…, with done, remaining, pace and the SMART text |
| tasks | open tasks that are not steps, as refs t1…, capped at 150, notes truncated to 300 characters; each with its planned blocks for the next 7 days (`assistant.go:289-325`) |
| last_review | the latest weekly review |
| recent_days | worked and target minutes for 7 days |
| messages | the newest 24 |

**Context budget per turn:**
- The static system prompt is about 21.3 KB: `gwen.md` plus about 10.4 KB of assistant prompt and schema (`assistant.go:230-310`) **[V, measured]**.
- The dynamic payload can reach tens of thousands of characters: 150 tasks with up to 300 characters of notes each, plus 24 messages of up to 2,000 characters **[I]**.
- Both providers send one system prompt and **one** user message (`providers.go:54-55, 109-110`) **[V]**.
- The Claude Code CLI runs with `--tools ""` and `--no-session-persistence` (`llm/claudecode.go:81-82`) **[V]**.
- Actions are structured output with 8 types (`assistant.go:19-32`). No tool use exists **[V]**.

**Prompt caching.** Anthropic prompt caching is enabled by a `cache_control` field. Cache reads cost 0.1× base input, or 0.05× on Opus 5.5 and Sonnet 5.5, with a 5-minute default TTL ([docs](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)) **[V]**. Gwen sends no such field **[V]**.

### 5.4 Other things the model sees

| Job | Persona prepended? | Payload | Where the result is stored |
|---|---|---|---|
| assistant | yes | the table in 5.3 | `llm_runs(kind='assistant')` |
| day_plan | yes | ≤60 tasks with notes ≤500, a conversation of ≤20 user messages (`llm/dayplan.go:12-18`) | `llm_runs(kind='day_plan', subject=day)` |
| retro | yes | the week's days, project minutes, completed titles, goals, planned and done minutes, plus the review as `reflection` (`api/llm.go:146-196`) | `llm_runs(kind='retro', subject=week)`, Markdown |
| breakdown | no | the goal and ≤200 existing task titles (`api/llm.go:89-98`) | `llm_runs(kind='breakdown', subject=goal)` |

Persona flags are set at `prompts.go:148-153` **[V]**.

**Gaps between these jobs:**
- Retro outputs are never fed back to the assistant. The assistant sees only the user's review **[V]**.
- Energy logs feed only the energy report (`api/methods.go:108-112`), not any LLM job **[V]**.

### 5.5 Activity tracking

**Privacy contract.** `internal/activity` reports only idle/active, lock, suspend and resume. Its package doc states: "nothing about what the user types or which app is focused is ever read" (`activity.go:1-4`) **[V]**. Any app-awareness feature in v2 breaks that stated contract and must change it on purpose **[I]**.

**Persistence:** activity becomes `segments.source` (`activity`, `idle`, `lock`, `suspend`) and `engine_events` (90 days) **[V]**.

**`tracking.json` is not runtime data.** `assets/readme/tracking.json` is a sequence-diagram spec for the README (commit `e39d23e`) **[V]**.

## 6. Data a personal memory layer should connect to

- **Planning objects:** tasks (stage, effort, delegated_to, goal, project, recurrence, steps), goals (SMART text, pace), projects, commitments, plan_items (rollover_count shows procrastination), and day_hours **[V]**.
  - Hidden behavioural signals: rollover counts, skipped blocks, and tasks parked in `waiting` with a `delegated_to` person **[I]**.
- **Tracking:** work_days and segments give worked versus target time, per project and task, and break patterns. `StatsRepo.Days` and `Summary` derive these (`store/stats.go:26-40`) **[V]**.
- **Reflection:** weekly_reviews (user-written), retro Markdown (model-written), energy_logs **[V]**.
- **Calendar:** calendar_busy has titles, so it can feed people and meeting entities **[V]**.
- **People exist only as free text** in `tasks.delegated_to` **[V]**. There is no entity table.
- **Conversations:** the JSON in `llm_runs.output` is the only raw episodic material **[V]**.

## 7. Constraints for adding memory

1. **Sync is all or nothing per push** (`sync.go:220-225`) **[V]**.
   - Adding a synced table or column needs hub and laptops upgraded together (`setup.md:347-350`) **[V]**.
   - v2 should make `checkRow` tolerant (ignore unknown columns, default missing ones) and have the client compare `schema_version` before pushing **[I]**.
2. **Row-level last-writer-wins** suits *append-only or immutable* rows: memory items, messages, ledger mirrors **[I]**.
   - It is poor for shared mutable documents, so model edits as new rows that supersede old ones **[I]**.
   - Consider a hybrid logical clock in place of wall-clock `updated_at` for cross-device ordering **[I]**.
3. **Offline-first.** Every daemon owns a full database. The hub is a peer database, not an authority, except for its `hub_changes` seq **[V]**.
   - Anything that needs one winner, such as claiming a cross-device job, needs a hub-side compare-and-set endpoint. Last-writer-wins on a row cannot provide it **[I]**.
4. **Closed `CHECK` enums are costly.** Use open `TEXT` kinds validated in Go for extensible lists (memory kinds, entity kinds, program kinds), or accept table rebuilds **[I]**.
5. **Vector and full-text search**, all pure Go:
   - `modernc.org/sqlite/vec` has bundled sqlite-vec since v1.47.0 (2026-03-17). v1.59.0, released 2026-09-15, carries sqlite-vec v0.1.9 and pins `modernc.org/libc v1.75.7`, which matches Gwen's `go.mod:196` **[V]** ([CHANGELOG via module source](https://pkg.go.dev/modernc.org/sqlite), [vec pkg](https://pkg.go.dev/modernc.org/sqlite/vec)).
   - It is enabled by a blank import, which auto-registers `vec0` and the `vec_*` functions. It rules out `RegisterPageCache` **[V]**, which Gwen does not use **[V]**.
   - Platform files include `vec_linux_arm64.go` **[V]**.
   - The compile options include `SQLITE_ENABLE_FTS5`, `JSON1`, `RTREE`, `MATH_FUNCTIONS` and `PREUPDATE_HOOK` **[V]**.
   - pkg.go.dev flags the vec package as "not in the latest version of its module", although its listed versions run through v1.60.1. I could not resolve this, so pin carefully **[V/unresolved]**.
   - sqlite-vec is "pre-v1, so expect breaking changes" ([repo](https://github.com/asg017/sqlite-vec)) **[V]**. I believe v0.1.x `vec0` KNN is an exact scan **[I]**. At personal scale (10⁴–10⁵ vectors) that is fine **[I]**.
   - Alternatives:
     - The CGo and ncruces bindings do **not** work with modernc ([bindings](https://github.com/asg017/sqlite-vec-go-bindings)) **[V]**.
     - chromem-go: zero-dependency, exhaustive cosine search, MPL-2.0, about 39 ms for 100k docs on an i5-1135G7 ([repo](https://github.com/philippgille/chromem-go)) **[V]**.
     - coder/hnsw: an in-memory HNSW index with Export/Import, CC0-1.0 ([repo](https://github.com/coder/hnsw)) **[V]**.
     - sqlite.org's vec1: an IVFADC+OPQ ANN index at v0.7, "Testing is insufficient" ([doc](https://sqlite.org/vec1/doc/trunk/doc/vec1.md)) **[V]**.
   - **Recommendation:** keep `vec0` and FTS5 tables *derived and device-local*. Never sync them, and rebuild them from synced `memory_items` after pull via `OnApplied` **[I]**.
6. **Embeddings.** sherpa-onnx is used only for speech **[V]**. The existing OpenAI-compatible provider path for Ollama is the natural local embeddings source **[I]**.
7. **Privacy.** A tombstone (`deleted_at`) keeps the row body on the hub forever. A memory "forget" must blank `body` at the same time it sets `deleted_at` **[I]**.

## 8. What the v2 data model must change or add

The rules for every new table:
- `STRICT`, UUIDv7 ids, Unix-millisecond times.
- Every synced table carries the envelope and is listed in `SyncTables` / `syncColumns`.
- Derived data stays local.

| Table | Class | Purpose and essential columns |
|---|---|---|
| `devices` | synced | id = existing device_id, name, kind (laptop/pi/phone/cloud), tailnet_name, capabilities JSON (display, apps, claude_code, gpu), last_seen_at. Fills today's gap: device_id is an opaque string with no description |
| `threads` + `messages` | synced, append-only | They replace chat stored as `llm_runs` JSON. `messages`: thread_id, role, text, at, mood, attitude, actions JSON, run_id, origin device_id. This gives phone↔laptop continuity and per-message provenance. `llm_runs` stays for planner jobs |
| `self_versions` | synced, append-only | attitude, note, reason, since. Replaces the `assistant_self` key and keeps its history |
| `entities` | synced | kind (person/app/device/place/org/topic/project-alias), name, aliases JSON, attrs JSON. Promotes `delegated_to` and calendar titles into linkable people |
| `memory_items` | synced, append-only plus supersede | kind (open text: fact/preference/episode/lesson/procedure/self), body, structured JSON, subject_entity_id, scope (global or project:id), confidence, salience, sensitivity, content_hash. *Event time*: valid_from, valid_until. *Record time*: created_at, invalidated_at, superseded_by. *Provenance*: source_kind (message/run/task/review/import/user), source_id, source_device_id, extractor (model and prompt version) |
| `memory_links` | synced | memory_id, target_table, target_id, relation: links memories to tasks, goals, projects, entities and threads |
| `memory_backends` | device- or hub-local | memory_id, backend (`mem0`, `supermemory`, `local`), external_id, status, synced_at. This lets one Gwen memory be mirrored to Mem0 and Supermemory while Gwen's own table stays the source of truth |
| `memory_fts` (FTS5), `memory_vec` (vec0) | local, derived | rebuilt from `memory_items`; also hold the embedding model id |
| `briefs`, `rules` | synced | as in harness v1 §11.1 |
| `runs` | **synced summary** (v1 made it local) | add target_device_id and status. The phone must see "running on laptop, 3 approvals pending". Step detail stays local |
| `ledger` | local, append-only, hash-chained | as in v1. Mirror to the hub append-only for phone viewing |
| `dispatch` | hub-authoritative | from_device, to_device, kind, payload, status, lease_until. "Phone asks, laptop executes" needs a compare-and-set claim, not last-writer-wins |
| `places` | local | v1 shape (path, project_id, kind, remote) |
| `place_identities` | synced | git remote or repo id → project_id, device_id, path. Lets the phone or cloud know where a project lives |
| `shares` | synced | run_id, source_device, path, dest (tailscale-serve or cloud bucket), url, expires_at, revoked_at. Covers "take the folder to the cloud, give me access on my phone" |
| `programs` | synced catalog | name, version, digest sha256, manifest JSON (entrypoints, input/output schemas, required capabilities, context cost), source |
| `program_installs` | local | program_id, path, verified digest, status, enabled |

**Why bi-temporal.** Validity should be modelled on two timelines, after Zep/Graphiti. One timeline records when a fact held (`t_valid`/`t_invalid`). The other records when the system created or expired it (`t′created`/`t′expired`). A contradicting fact *invalidates* the old one rather than deleting it ([Zep, arXiv:2501.13956](https://arxiv.org/html/2501.13956v1)) **[V]**. Mem0 base has an ADD/UPDATE/DELETE/NOOP update phase whose DELETE removes contradicted memories. Mem0g instead marks relationships invalid "rather than physically removing them" ([Mem0, arXiv:2504.19413](https://arxiv.org/html/2504.19413v1)) **[V]**. Gwen should keep the invalidate-not-delete model in its own table whatever the backends do **[I]**.

**Context discipline in prompts:**
- Order the prompt as: static persona and job prompt (cacheable), then the program and tool catalogue (cacheable), then retrieved memories and the self, then the live state slice, then recent messages plus a rolling thread summary **[I]**.
- Replace the dump of 150 tasks with retrieval scoped to the turn **[I]**.

## 9. Integration points

1. **`internal/llm/llm.go:114-135` `ask()`.** Every LLM call passes through here. Hook in token accounting, the ledger, and memory injection. Add `cache_control` in `providers.go:25-30, 53-55`.
2. **`internal/llm/llm.go:18-25` `Planner` interface.** Add an agent or tool-use loop, or `memory.search` / `memory.write` tools. The current 8 actions are at `llm/assistant.go:19-32`.
3. **`internal/llm/assistant.go:41-55` `AssistantRequest`.** Add `memories`, `entities` and `thread_summary`. Shrink `Tasks` to a retrieved slice. The limits are at `:12-17`.
4. **`internal/llm/persona.go:72-102`.** Move the self-note out of the static prefix. Section 9 of `gwen.md:163-177` becomes true once memory exists.
5. **`internal/api/assistant.go:215-350` `assistantRequest`.** The place to swap "dump all" for retrieval. `:346-348` is the history window.
6. **`internal/api/assistant.go:59-76`.** Post-turn hook: write `messages`, queue memory extraction asynchronously, and write the ledger. `changeSelf` at `:134-164` moves to `self_versions`.
7. **`internal/api/llm.go:20-33` `planner()`.** Loads the self per request. Extend it to load the memory context.
8. **`internal/api/api.go:38-56` `Server`.** Add an optional `Memory` service, nil when disabled, following the existing pattern of `Calendar` and `Sync`.
9. **`cmd/gwend/daemon.go:124-155`.** Wiring. Run a consolidation and reflection goroutine like `keepPlan` (`cmd/gwend/keeper.go:20-32`).
10. **`internal/api/sse.go:36-41` `OnPublish`.** Observe `tasks_changed`, `goals_changed` and similar events to refresh links. Observers receive only the event *name*, so pass payloads or use `store.Changes` (`store/tasks.go:73-78`).
11. **`internal/store/events.go:81-113` `Repos`.** Add Memory, Entities, Threads/Messages and Devices repos.
12. **`internal/store/sync.go:18-44`.** Register new synced tables. `:184-208` is where `checkRow` should become tolerant, and `:255-290` is `applyRow`.
13. **`internal/sync/client.go:157-201`.** Add a schema-version check before push. `:225-233` with `cmd/gwend/sync.go:20` `OnApplied` re-embeds pulled memory rows.
14. **`internal/store/db.go:20`.** Add the blank import `modernc.org/sqlite/vec`. New migrations start at `00009_*.sql` and are backed up automatically (`migrate.go:46-50`).
15. **`internal/store/localstate.go:12-35`.** New local keys: embedding model version, consolidation watermark.
16. **`internal/api/routes.go:87-95`.** Add `/v1/threads`, `/v1/memory` and `/v1/entities`. `:113-142` `ReadOnlyHandler` must gain an authenticated chat and dispatch path for the phone.
17. **`ui/src/components/AssistantChat.tsx:28-45`.** Replace `localStorage` run tracking with server-side threads.
18. **`internal/activity/activity.go:1-4`.** The privacy contract to amend explicitly before any app or window awareness.

## 10. Risks I noticed along the way

- Assistant chats have no length cap and grow quadratically in storage, because `llm_runs` is never pruned **[V/I]**.
- A failed turn stores a `failed` run without the history (`api/assistant.go:62-64`). The UI then keeps the earlier run id **[V/I]**.
- Pushes that hit constraints are dropped silently, for example two devices creating the same project name. Only a log line records it (`sync.go:280-283`) **[V]**.
- Each daemon with Calendar enabled keeps its own `gcal_links`. Running Calendar on two laptops could duplicate events **[I]**.

## Sources
- https://pkg.go.dev/modernc.org/sqlite
- https://pkg.go.dev/modernc.org/sqlite/vec
- https://pkg.go.dev/modernc.org/sqlite/vec?tab=versions
- https://gorse.io/posts/sqlite-vec
- https://github.com/asg017/sqlite-vec
- https://github.com/asg017/sqlite-vec/releases/tag/v0.1.9
- https://github.com/asg017/sqlite-vec-go-bindings
- https://sqlite.org/vec1/doc/trunk/doc/vec1.md
- https://github.com/philippgille/chromem-go
- https://github.com/coder/hnsw
- https://arxiv.org/abs/2501.13956 (Zep)
- https://arxiv.org/html/2501.13956v1
- https://arxiv.org/html/2504.19413v1 (Mem0)
- https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- Repo files: `/home/claude/gwen` at commit b029c77 (paths and lines cited inline); v1 harness at `/mnt/project-files/gwen-harness/harness.md` §11