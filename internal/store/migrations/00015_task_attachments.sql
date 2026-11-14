-- +goose Up
-- file: internal/store/migrations/00015_task_attachments.sql

-- What a task comes with for context: a link, a note, a file on this
-- machine, or an item from an app through Composio (an email, an event, an
-- issue, a page) with the text snapshot taken when it was attached, so an
-- agent handed the task can read it without asking the app again. Synced
-- with the task, except files: a path means nothing on another device. No
-- foreign key, like projects.parent_id: the store checks the task on write.
CREATE TABLE task_attachments (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    task_id    TEXT NOT NULL CHECK (length(task_id) = 36),
    kind       TEXT NOT NULL CHECK (kind IN ('link', 'note', 'file', 'app')),
    title      TEXT NOT NULL,
    url        TEXT NOT NULL DEFAULT '',
    path       TEXT NOT NULL DEFAULT '',
    app        TEXT NOT NULL DEFAULT '',
    ref        TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL DEFAULT '',
    fetched_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX task_attachments_task ON task_attachments (task_id, created_at) WHERE deleted_at IS NULL;
