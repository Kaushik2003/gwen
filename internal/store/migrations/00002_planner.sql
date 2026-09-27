-- +goose Up
-- file: internal/store/migrations/00002_planner.sql

CREATE TABLE goals (
    id               TEXT PRIMARY KEY CHECK (length(id) = 36),
    title            TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    kind             TEXT NOT NULL CHECK (kind IN ('quantity', 'tasks')),
    unit             TEXT NOT NULL DEFAULT '',
    target_quantity  INTEGER CHECK (target_quantity > 0),
    minutes_per_unit INTEGER CHECK (minutes_per_unit > 0),
    project_id       TEXT REFERENCES projects (id),
    start_day        TEXT NOT NULL CHECK (start_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    due_day          TEXT NOT NULL CHECK (due_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    status           TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'done', 'abandoned')),
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER,
    device_id        TEXT NOT NULL,
    rev              INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (due_day >= start_day),
    CHECK ((kind = 'quantity') = (target_quantity IS NOT NULL)),
    CHECK ((kind = 'quantity') = (minutes_per_unit IS NOT NULL))
) STRICT;

CREATE INDEX goals_active ON goals (due_day) WHERE status = 'active' AND deleted_at IS NULL;

ALTER TABLE tasks ADD COLUMN goal_id TEXT REFERENCES goals (id);
ALTER TABLE tasks ADD COLUMN quantity INTEGER CHECK (quantity > 0);
ALTER TABLE tasks ADD COLUMN quantity_done INTEGER CHECK (quantity_done >= 0);
ALTER TABLE tasks ADD COLUMN rrule TEXT;
ALTER TABLE tasks ADD COLUMN template_id TEXT REFERENCES tasks (id);
ALTER TABLE tasks ADD COLUMN occurrence_day TEXT CHECK (occurrence_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]');

CREATE INDEX tasks_goal ON tasks (goal_id) WHERE goal_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX tasks_occurrence ON tasks (template_id, occurrence_day)
    WHERE template_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE commitments (
    id                   TEXT PRIMARY KEY CHECK (length(id) = 36),
    title                TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    project_id           TEXT REFERENCES projects (id),
    rrule                TEXT NOT NULL,
    start_minute         INTEGER CHECK (start_minute BETWEEN 0 AND 1439),
    duration_minutes     INTEGER NOT NULL CHECK (duration_minutes BETWEEN 1 AND 1440),
    counts_toward_target INTEGER NOT NULL DEFAULT 1 CHECK (counts_toward_target IN (0, 1)),
    active_from          TEXT NOT NULL CHECK (active_from GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    active_until         TEXT CHECK (active_until GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    deleted_at           INTEGER,
    device_id            TEXT NOT NULL,
    rev                  INTEGER NOT NULL CHECK (rev >= 1),
    CHECK (active_until >= active_from)
) STRICT;

CREATE TABLE plan_items (
    id              TEXT PRIMARY KEY CHECK (length(id) = 36),
    day             TEXT NOT NULL CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    task_id         TEXT NOT NULL REFERENCES tasks (id),
    planned_minutes INTEGER NOT NULL CHECK (planned_minutes > 0),
    start_at        INTEGER,
    position        INTEGER NOT NULL CHECK (position >= 0),
    status          TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'done', 'rolled', 'skipped')),
    pinned          INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
    rolled_from_id  TEXT REFERENCES plan_items (id),
    rollover_count  INTEGER NOT NULL DEFAULT 0 CHECK (rollover_count >= 0),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    deleted_at      INTEGER,
    device_id       TEXT NOT NULL,
    rev             INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX plan_items_day ON plan_items (day, position) WHERE deleted_at IS NULL;
CREATE INDEX plan_items_task ON plan_items (task_id) WHERE deleted_at IS NULL;
