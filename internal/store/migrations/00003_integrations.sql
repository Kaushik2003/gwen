-- +goose Up
-- file: internal/store/migrations/00003_integrations.sql

CREATE TABLE gcal_links (
    plan_item_id TEXT PRIMARY KEY REFERENCES plan_items (id),
    event_id     TEXT NOT NULL UNIQUE,
    etag         TEXT NOT NULL,
    synced_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE calendar_busy (
    calendar_id TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    start_at    INTEGER NOT NULL,
    end_at      INTEGER NOT NULL,
    fetched_at  INTEGER NOT NULL,
    PRIMARY KEY (calendar_id, event_id),
    CHECK (end_at > start_at)
) STRICT;

CREATE INDEX calendar_busy_range ON calendar_busy (start_at, end_at);

CREATE TABLE llm_runs (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    kind       TEXT NOT NULL CHECK (kind IN ('breakdown', 'retro')),
    subject_id TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('ok', 'failed', 'accepted', 'rejected')),
    output     TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX llm_runs_subject ON llm_runs (kind, subject_id, created_at);

CREATE TABLE hub_changes (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    row_id     TEXT NOT NULL,
    device_id  TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX hub_changes_row ON hub_changes (table_name, row_id);
