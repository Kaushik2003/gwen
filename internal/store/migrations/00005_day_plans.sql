-- +goose Up
-- file: internal/store/migrations/00005_day_plans.sql

CREATE TABLE day_hours (
    id           TEXT PRIMARY KEY CHECK (length(id) = 36),
    day          TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    start_minute INTEGER CHECK (start_minute BETWEEN 0 AND 1439),
    work_minutes INTEGER CHECK (work_minutes BETWEEN 0 AND 1440),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    deleted_at   INTEGER,
    device_id    TEXT NOT NULL,
    rev          INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE UNIQUE INDEX day_hours_day_live ON day_hours (day) WHERE deleted_at IS NULL;

CREATE TABLE llm_runs_new (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind       TEXT NOT NULL CHECK (kind IN ('breakdown', 'retro', 'day_plan')),
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
