-- +goose Up
-- file: internal/store/migrations/00012_project_dirs.sql

-- A project's folder on this laptop (plan.md 3.15): the workbench opens its
-- terminals, files, and history there. Device-local and never synced, since
-- folders differ per machine.
CREATE TABLE project_dirs (
    project_id TEXT PRIMARY KEY REFERENCES projects (id),
    path TEXT NOT NULL,
    test_command TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
) STRICT;
