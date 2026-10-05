-- +goose Up
-- file: internal/store/migrations/00007_methods.sql

-- Getting Things Done and the board: where a task stands, and who it waits on.
ALTER TABLE tasks ADD COLUMN stage TEXT NOT NULL DEFAULT 'todo'
    CHECK (stage IN ('inbox', 'todo', 'doing', 'waiting', 'someday'));
-- Eat the frog: how hard a task is, 1 (easy) to 3 (hard); null is unrated.
ALTER TABLE tasks ADD COLUMN effort INTEGER CHECK (effort BETWEEN 1 AND 3);
ALTER TABLE tasks ADD COLUMN delegated_to TEXT NOT NULL DEFAULT '';

-- SMART goals: the letters the goal's own columns do not already hold.
ALTER TABLE goals ADD COLUMN specific TEXT NOT NULL DEFAULT '';
ALTER TABLE goals ADD COLUMN measurable TEXT NOT NULL DEFAULT '';
ALTER TABLE goals ADD COLUMN assignable TEXT NOT NULL DEFAULT '';
ALTER TABLE goals ADD COLUMN realistic TEXT NOT NULL DEFAULT '';

-- The weekly review: one per week, keyed by its Monday.
CREATE TABLE weekly_reviews (
    id           TEXT PRIMARY KEY CHECK (length(id) = 36),
    week_start   TEXT NOT NULL CHECK (week_start GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    went_well    TEXT NOT NULL DEFAULT '',
    went_badly   TEXT NOT NULL DEFAULT '',
    energy       TEXT NOT NULL DEFAULT '',
    decisions    TEXT NOT NULL DEFAULT '',
    improvements TEXT NOT NULL DEFAULT '',
    checklist    INTEGER NOT NULL DEFAULT 0 CHECK (checklist >= 0),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    deleted_at   INTEGER,
    device_id    TEXT NOT NULL,
    rev          INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE UNIQUE INDEX weekly_reviews_week_live ON weekly_reviews (week_start) WHERE deleted_at IS NULL;

-- Biological prime time: energy check-ins, 1 (drained) to 5 (peak).
CREATE TABLE energy_logs (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    at         INTEGER NOT NULL,
    level      INTEGER NOT NULL CHECK (level BETWEEN 1 AND 5),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX energy_logs_at ON energy_logs (at) WHERE deleted_at IS NULL;

-- The assistant's conversations are LLM runs too.
CREATE TABLE llm_runs_new (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind       TEXT NOT NULL CHECK (kind IN ('breakdown', 'retro', 'day_plan', 'assistant')),
    subject_id TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('ok', 'failed', 'accepted', 'rejected')),
    output     TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

INSERT INTO llm_runs_new SELECT id, kind, subject_id, status, output, created_at, updated_at FROM llm_runs;
DROP TABLE llm_runs;
ALTER TABLE llm_runs_new RENAME TO llm_runs;
CREATE INDEX llm_runs_subject ON llm_runs (kind, subject_id, created_at);
