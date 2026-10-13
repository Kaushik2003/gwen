-- +goose Up
-- file: internal/store/migrations/00010_memory.sql

-- Gwen's memory (gwen-harness/harness.md section 4). Chat stays in
-- chat_threads and chat_messages, cited as msg:<id>. What she learns from it
-- are claims: keyed, true over a span of world time, recorded with the words
-- they came from and how far they can be trusted. Evidence that is not chat
-- (a note typed in the dashboard, an edit of a memory file, a document) is an
-- episode, cited as ep:<id>. Rows that sync carry the envelope; derived and
-- device-local rows do not. Kinds that grow are open TEXT checked in Go.

-- What a reply rested on from memory, and the claims the user asked her to
-- stop bringing up: refs only, resolved when shown, so forgetting a claim
-- leaves none of its words here.
ALTER TABLE chat_messages ADD COLUMN memory TEXT NOT NULL DEFAULT '{}'
    CHECK (json_valid(memory) AND json_type(memory) = 'object');

-- People, organisations, places, and topics that claims are about.
CREATE TABLE entities (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind       TEXT NOT NULL CHECK (length(kind) BETWEEN 1 AND 40),
    name       TEXT NOT NULL CHECK (length(name) <= 200),
    aliases    TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(aliases) AND json_type(aliases) = 'array'),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX entities_name ON entities (lower(name)) WHERE deleted_at IS NULL;

-- Verbatim evidence besides chat, append-only. Forgetting one erases its
-- words and keeps the row.
CREATE TABLE episodes (
    id           TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind         TEXT NOT NULL CHECK (length(kind) BETWEEN 1 AND 40),
    channel      TEXT NOT NULL CHECK (length(channel) BETWEEN 1 AND 40),
    speaker      TEXT NOT NULL CHECK (speaker IN ('user', 'gwen', 'external')),
    title        TEXT NOT NULL DEFAULT '' CHECK (length(title) <= 200),
    body         TEXT NOT NULL,
    source       TEXT NOT NULL DEFAULT '' CHECK (length(source) <= 1000),
    at           INTEGER NOT NULL,
    sensitivity  TEXT NOT NULL CHECK (sensitivity IN ('public', 'personal', 'sensitive')),
    content_hash TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    deleted_at   INTEGER,
    device_id    TEXT NOT NULL,
    rev          INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX episodes_at ON episodes (at) WHERE deleted_at IS NULL;

-- What Gwen holds true about the user and the people and things in their
-- life. valid_from and valid_to are world time (NULL is open); recorded_at
-- and superseded_at are when she learned and retired it. A forgotten claim is
-- a tombstone whose words are erased; its content_hash stays so the same fact
-- is not learned again from the same words.
CREATE TABLE claims (
    id               TEXT PRIMARY KEY CHECK (length(id) = 36),
    key              TEXT NOT NULL CHECK (length(key) BETWEEN 1 AND 120),
    subject_id       TEXT,
    value            TEXT NOT NULL CHECK (json_valid(value)),
    text             TEXT NOT NULL CHECK (length(text) <= 500),
    cardinality      TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
    status           TEXT NOT NULL CHECK (status IN ('candidate', 'active', 'quarantined', 'rejected', 'superseded')),
    trust            TEXT NOT NULL CHECK (trust IN ('owner_edit', 'owner_said', 'observed', 'inferred', 'external')),
    sensitivity      TEXT NOT NULL CHECK (sensitivity IN ('public', 'personal', 'sensitive')),
    confidence       REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    valid_from       INTEGER,
    valid_to         INTEGER CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from),
    occurred_at      INTEGER,
    mentioned_at     INTEGER NOT NULL,
    recorded_at      INTEGER NOT NULL,
    superseded_at    INTEGER,
    superseded_by    TEXT,
    suppressed       INTEGER NOT NULL DEFAULT 0 CHECK (suppressed IN (0, 1)),
    source_kind      TEXT NOT NULL CHECK (length(source_kind) BETWEEN 1 AND 40),
    source_id        TEXT NOT NULL DEFAULT '',
    span             TEXT NOT NULL DEFAULT '' CHECK (length(span) <= 1000),
    source_device_id TEXT NOT NULL,
    channel          TEXT NOT NULL CHECK (length(channel) BETWEEN 1 AND 40),
    extractor        TEXT NOT NULL DEFAULT '' CHECK (length(extractor) <= 200),
    content_hash     TEXT NOT NULL,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER,
    device_id        TEXT NOT NULL,
    rev              INTEGER NOT NULL CHECK (rev >= 1),
    CHECK ((status = 'superseded') = (superseded_by IS NOT NULL))
) STRICT;

CREATE INDEX claims_key ON claims (key, subject_id) WHERE deleted_at IS NULL;
CREATE INDEX claims_status ON claims (status, recorded_at) WHERE deleted_at IS NULL;
CREATE INDEX claims_source ON claims (source_kind, source_id);
CREATE INDEX claims_subject ON claims (subject_id) WHERE deleted_at IS NULL;
CREATE INDEX claims_hash ON claims (content_hash);

-- Two claims that cannot both be true, until the user says which is.
CREATE TABLE conflicts (
    id          TEXT PRIMARY KEY CHECK (length(id) = 36),
    claim_a     TEXT NOT NULL CHECK (length(claim_a) = 36),
    claim_b     TEXT NOT NULL CHECK (length(claim_b) = 36),
    reason      TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 500),
    sensitivity TEXT NOT NULL CHECK (sensitivity IN ('public', 'personal', 'sensitive')),
    status      TEXT NOT NULL CHECK (status IN ('open', 'resolved', 'dismissed')),
    resolution  TEXT CHECK (resolution IN ('keep_a', 'keep_b', 'both', 'neither')),
    resolved_at INTEGER,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER,
    device_id   TEXT NOT NULL,
    rev         INTEGER NOT NULL CHECK (rev >= 1),
    CHECK ((status = 'resolved') = (resolution IS NOT NULL))
) STRICT;

CREATE INDEX conflicts_open ON conflicts (status) WHERE deleted_at IS NULL;

-- What a claim or an episode is about or rests on: further evidence, the
-- task, goal, project, or entity it concerns.
CREATE TABLE memory_links (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    from_kind  TEXT NOT NULL CHECK (from_kind IN ('claim', 'episode')),
    from_id    TEXT NOT NULL CHECK (length(from_id) = 36),
    to_kind    TEXT NOT NULL CHECK (length(to_kind) BETWEEN 1 AND 40),
    to_id      TEXT NOT NULL CHECK (length(to_id) BETWEEN 1 AND 100),
    relation   TEXT NOT NULL CHECK (length(relation) BETWEEN 1 AND 40),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE UNIQUE INDEX memory_links_unique ON memory_links (from_id, to_kind, to_id, relation) WHERE deleted_at IS NULL;
CREATE INDEX memory_links_to ON memory_links (to_kind, to_id) WHERE deleted_at IS NULL;

-- How the assistant chose to treat the user, every version, append-only. The
-- newest live row is the current one; a reset is a version with neither.
CREATE TABLE self_versions (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    attitude   TEXT NOT NULL DEFAULT '' CHECK (length(attitude) <= 40),
    note       TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 600),
    since      INTEGER,
    reason     TEXT NOT NULL CHECK (reason IN ('chat', 'reset', 'import')),
    thread_id  TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX self_versions_created ON self_versions (created_at) WHERE deleted_at IS NULL;

-- The words of claims and episodes, for search. Like chat_messages_fts, the
-- indexes hold the text by rowid and triggers keep them in step.
CREATE VIRTUAL TABLE claims_fts USING fts5 (
    text,
    key,
    content = 'claims',
    content_rowid = 'rowid',
    tokenize = 'porter unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER claims_fts_insert AFTER INSERT ON claims BEGIN
    INSERT INTO claims_fts (rowid, text, key) VALUES (new.rowid, new.text, new.key);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER claims_fts_delete AFTER DELETE ON claims BEGIN
    INSERT INTO claims_fts (claims_fts, rowid, text, key) VALUES ('delete', old.rowid, old.text, old.key);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER claims_fts_update AFTER UPDATE OF text, key ON claims BEGIN
    INSERT INTO claims_fts (claims_fts, rowid, text, key) VALUES ('delete', old.rowid, old.text, old.key);
    INSERT INTO claims_fts (rowid, text, key) VALUES (new.rowid, new.text, new.key);
END;
-- +goose StatementEnd

CREATE VIRTUAL TABLE episodes_fts USING fts5 (
    title,
    body,
    content = 'episodes',
    content_rowid = 'rowid',
    tokenize = 'porter unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER episodes_fts_insert AFTER INSERT ON episodes BEGIN
    INSERT INTO episodes_fts (rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER episodes_fts_delete AFTER DELETE ON episodes BEGIN
    INSERT INTO episodes_fts (episodes_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER episodes_fts_update AFTER UPDATE OF title, body ON episodes BEGIN
    INSERT INTO episodes_fts (episodes_fts, rowid, title, body) VALUES ('delete', old.rowid, old.title, old.body);
    INSERT INTO episodes_fts (rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
-- +goose StatementEnd

-- Everything below is this device's own and never synced.

-- The observation log of a chat: dated notes on what was said before the
-- messages the assistant still sees verbatim, written by the Observer and
-- condensed by the Reflector.
CREATE TABLE chat_observations (
    thread_id   TEXT PRIMARY KEY REFERENCES chat_threads (id),
    through_id  TEXT NOT NULL CHECK (length(through_id) = 36),
    through_at  INTEGER NOT NULL,
    log         TEXT NOT NULL,
    reflections INTEGER NOT NULL DEFAULT 0 CHECK (reflections >= 0),
    updated_at  INTEGER NOT NULL
) STRICT;

-- The memory note attached to a user message when it was sent, kept so the
-- conversation the model sees later is the same text, byte for byte.
CREATE TABLE chat_notes (
    message_id TEXT PRIMARY KEY CHECK (length(message_id) = 36),
    thread_id  TEXT NOT NULL CHECK (length(thread_id) = 36),
    note       TEXT NOT NULL,
    refs       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(refs) AND json_type(refs) = 'object'),
    created_at INTEGER NOT NULL
) STRICT;

CREATE INDEX chat_notes_thread ON chat_notes (thread_id);

-- The profile card: what Gwen keeps in mind about the user every day,
-- compiled from active claims at most once a day.
CREATE TABLE profile_cards (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    day        TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    text       TEXT NOT NULL,
    refs       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(refs) AND json_type(refs) = 'object'),
    created_at INTEGER NOT NULL
) STRICT;

CREATE INDEX profile_cards_created ON profile_cards (created_at);

-- Work the memory does in the background: extraction, embedding, projection
-- to Mem0 and Supermemory, the memory files, consolidation. One pending job
-- per kind and subject.
CREATE TABLE memory_jobs (
    id         INTEGER PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (length(kind) BETWEEN 1 AND 40),
    subject    TEXT NOT NULL DEFAULT '',
    payload    TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
    state      TEXT NOT NULL CHECK (state IN ('pending', 'running', 'done', 'failed')),
    attempts   INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    run_after  INTEGER NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE UNIQUE INDEX memory_jobs_pending ON memory_jobs (kind, subject) WHERE state IN ('pending', 'running');
CREATE INDEX memory_jobs_due ON memory_jobs (state, run_after);

-- Where a claim or a resource lives in a lane outside gwen.db, so the lane can
-- be reconciled, filtered by supersession, and rebuilt.
CREATE TABLE memory_backends (
    backend      TEXT NOT NULL CHECK (length(backend) BETWEEN 1 AND 40),
    item_id      TEXT NOT NULL CHECK (length(item_id) = 36),
    external_id  TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL CHECK (status IN ('pending', 'synced', 'deleted', 'failed')),
    content_hash TEXT NOT NULL DEFAULT '',
    error        TEXT NOT NULL DEFAULT '',
    synced_at    INTEGER,
    updated_at   INTEGER NOT NULL,
    PRIMARY KEY (backend, item_id)
) STRICT;

CREATE INDEX memory_backends_external ON memory_backends (backend, external_id);

-- How often a claim was put in front of the model, for ranking.
CREATE TABLE memory_usage (
    claim_id     TEXT PRIMARY KEY CHECK (length(claim_id) = 36),
    uses         INTEGER NOT NULL CHECK (uses >= 0),
    last_used_at INTEGER NOT NULL
) STRICT;

-- Documents the user gave Gwen to remember. The text she can read is an
-- episode; the file stays where it is.
CREATE TABLE resources (
    id           TEXT PRIMARY KEY CHECK (length(id) = 36),
    title        TEXT NOT NULL CHECK (length(title) <= 200),
    path         TEXT NOT NULL,
    mime         TEXT NOT NULL DEFAULT '',
    size         INTEGER NOT NULL CHECK (size >= 0),
    content_hash TEXT NOT NULL,
    episode_id   TEXT CHECK (episode_id IS NULL OR length(episode_id) = 36),
    document_at  INTEGER,
    added_at     INTEGER NOT NULL,
    deleted_at   INTEGER
) STRICT;

-- The ledger of harness v1 section 11: append-only and hash-chained, each
-- row's hash covering the one before. Memory writes the first rows (kind
-- 'memory'); the kernel adds its kinds later. Rows hold ids and content
-- hashes, never the words, so forgetting never breaks the chain.
CREATE TABLE ledger (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id    TEXT,
    kind      TEXT NOT NULL CHECK (length(kind) BETWEEN 1 AND 40),
    data      TEXT NOT NULL CHECK (json_valid(data)),
    at        INTEGER NOT NULL,
    prev_hash TEXT NOT NULL,
    hash      TEXT NOT NULL
) STRICT;

-- +goose StatementBegin
CREATE TRIGGER ledger_no_update BEFORE UPDATE ON ledger BEGIN
    SELECT RAISE(ABORT, 'the ledger is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER ledger_no_delete BEFORE DELETE ON ledger BEGIN
    SELECT RAISE(ABORT, 'the ledger is append-only');
END;
-- +goose StatementEnd
