-- +goose Up
-- file: internal/store/migrations/00001_init.sql

CREATE TABLE local_state (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE projects (
    id          TEXT PRIMARY KEY CHECK (length(id) = 36),
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    color       TEXT NOT NULL CHECK (color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'),
    archived_at INTEGER,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER,
    device_id   TEXT NOT NULL,
    rev         INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE UNIQUE INDEX projects_name_live ON projects (name COLLATE NOCASE) WHERE deleted_at IS NULL;

CREATE TABLE tasks (
    id               TEXT PRIMARY KEY CHECK (length(id) = 36),
    project_id       TEXT REFERENCES projects (id),
    title            TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    notes            TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
    priority         INTEGER NOT NULL DEFAULT 2 CHECK (priority BETWEEN 1 AND 4),
    due_day          TEXT CHECK (due_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    estimate_minutes INTEGER CHECK (estimate_minutes > 0),
    done_at          INTEGER,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER,
    device_id        TEXT NOT NULL,
    rev              INTEGER NOT NULL CHECK (rev >= 1),
    CHECK ((status = 'done') = (done_at IS NOT NULL))
) STRICT;

CREATE INDEX tasks_project ON tasks (project_id) WHERE deleted_at IS NULL;
CREATE INDEX tasks_open_due ON tasks (due_day) WHERE status = 'open' AND deleted_at IS NULL;

CREATE TABLE work_days (
    id             TEXT PRIMARY KEY CHECK (length(id) = 36),
    day            TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    tz             TEXT NOT NULL CHECK (length(tz) > 0),
    clocked_in_at  INTEGER NOT NULL,
    clocked_out_at INTEGER,
    target_seconds INTEGER NOT NULL CHECK (target_seconds >= 0),
    note           TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    deleted_at     INTEGER,
    device_id      TEXT NOT NULL,
    rev            INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (clocked_out_at >= clocked_in_at)
) STRICT;

CREATE UNIQUE INDEX work_days_day_live ON work_days (day) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX work_days_one_open ON work_days ((clocked_out_at IS NULL))
    WHERE clocked_out_at IS NULL AND deleted_at IS NULL;

CREATE TABLE segments (
    id          TEXT PRIMARY KEY CHECK (length(id) = 36),
    work_day_id TEXT NOT NULL REFERENCES work_days (id),
    kind        TEXT NOT NULL CHECK (kind IN ('work', 'break_auto', 'break_manual')),
    source      TEXT NOT NULL CHECK (source IN ('user', 'activity', 'idle', 'lock', 'suspend', 'recovery', 'edit')),
    project_id  TEXT REFERENCES projects (id),
    task_id     TEXT REFERENCES tasks (id),
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    truncated   INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER,
    device_id   TEXT NOT NULL,
    rev         INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (ended_at >= started_at),
    CHECK (kind = 'work' OR (project_id IS NULL AND task_id IS NULL)),
    CHECK (truncated = 0 OR ended_at IS NOT NULL)
) STRICT;

CREATE INDEX segments_day ON segments (work_day_id, started_at) WHERE deleted_at IS NULL;
CREATE INDEX segments_project ON segments (project_id, started_at) WHERE deleted_at IS NULL;
CREATE INDEX segments_task ON segments (task_id) WHERE task_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX segments_one_open ON segments ((ended_at IS NULL))
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE TABLE engine_events (
    id         INTEGER PRIMARY KEY,
    at         INTEGER NOT NULL,
    trigger    TEXT NOT NULL,
    from_state TEXT NOT NULL,
    to_state   TEXT NOT NULL,
    data       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data))
) STRICT;

CREATE INDEX engine_events_at ON engine_events (at);
