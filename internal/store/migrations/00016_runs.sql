-- +goose Up
-- file: internal/store/migrations/00016_runs.sql

-- The harness (plan.md 3.7 to 3.9, W19). Resources and routines belong to a
-- subject (internal/subject) and sync. Runs, their steps, log, approvals,
-- and grants stay on the machine that ran them, because they touch its
-- folders and browser. Email bodies, tokens, and file contents are never
-- stored here (section 7).

-- Links, folders, files, and notes attached to a subject.
CREATE TABLE subject_resources (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    subject    TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('url', 'folder', 'file', 'note')),
    title      TEXT NOT NULL DEFAULT '',
    value      TEXT NOT NULL,
    position   INTEGER NOT NULL CHECK (position >= 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;
CREATE INDEX subject_resources_subject ON subject_resources (subject, position) WHERE deleted_at IS NULL;

-- "When it starts": the steps a subject's routine run takes.
CREATE TABLE routines (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    subject    TEXT NOT NULL,
    steps      TEXT NOT NULL CHECK (json_valid(steps)),
    auto_start INTEGER NOT NULL DEFAULT 0 CHECK (auto_start IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;
CREATE UNIQUE INDEX routines_subject ON routines (subject) WHERE deleted_at IS NULL;

CREATE TABLE runs (
    id           TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind         TEXT NOT NULL,
    subject      TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL,
    brief        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL CHECK (status IN ('draft', 'running', 'waiting', 'done', 'failed', 'cancelled')),
    reason       TEXT NOT NULL DEFAULT '',
    external_ref TEXT NOT NULL DEFAULT '',
    result       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(result)),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    started_at   INTEGER,
    ended_at     INTEGER
) STRICT;
CREATE INDEX runs_status ON runs (status, updated_at);

CREATE TABLE run_steps (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    run_id     TEXT NOT NULL REFERENCES runs (id),
    position   INTEGER NOT NULL,
    title      TEXT NOT NULL,
    capability TEXT NOT NULL DEFAULT '',
    risk       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL CHECK (status IN ('pending', 'running', 'waiting', 'done', 'failed', 'skipped')),
    input      TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input)),
    output     TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output)),
    error      TEXT NOT NULL DEFAULT '',
    started_at INTEGER,
    ended_at   INTEGER
) STRICT;
CREATE INDEX run_steps_run ON run_steps (run_id, position);

CREATE TABLE run_events (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id  TEXT NOT NULL REFERENCES runs (id),
    step_id TEXT,
    kind    TEXT NOT NULL,
    data    TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data)),
    at      INTEGER NOT NULL
) STRICT;
CREATE INDEX run_events_run ON run_events (run_id, seq);

-- A step waiting for your decision: a run's (run_id) or, from W32, a chat's
-- (thread_id), exactly one of them. step_id is a run_steps or agent_steps
-- id. pattern narrows what a remembered grant covers (a command's prefix);
-- grant_scopes are the scopes "remember" may pick.
CREATE TABLE approvals (
    id           TEXT PRIMARY KEY CHECK (length(id) = 36),
    run_id       TEXT REFERENCES runs (id),
    thread_id    TEXT,
    step_id      TEXT NOT NULL,
    capability   TEXT NOT NULL,
    risk         TEXT NOT NULL,
    pattern      TEXT NOT NULL DEFAULT '',
    summary      TEXT NOT NULL,
    payload      TEXT NOT NULL CHECK (json_valid(payload)),
    grant_scopes TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(grant_scopes)),
    status       TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
    created_at   INTEGER NOT NULL,
    decided_at   INTEGER,
    CHECK ((run_id IS NULL) <> (thread_id IS NULL))
) STRICT;
CREATE INDEX approvals_status ON approvals (status, created_at);

-- A grant relaxes (allow) or tightens (ask) a capability, with a pattern
-- or none, for one scope: a subject ref, a folder, project:<id>, or
-- chat:<id>. Nothing can grant an external capability.
CREATE TABLE grants (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    capability TEXT NOT NULL,
    pattern    TEXT NOT NULL DEFAULT '',
    scope      TEXT NOT NULL,
    decision   TEXT NOT NULL CHECK (decision IN ('allow', 'ask')),
    created_at INTEGER NOT NULL,
    expires_at INTEGER
) STRICT;
CREATE UNIQUE INDEX grants_scope ON grants (capability, pattern, scope);
