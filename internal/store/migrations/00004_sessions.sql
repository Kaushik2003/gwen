-- +goose Up
-- file: internal/store/migrations/00004_sessions.sql

ALTER TABLE goals ADD COLUMN daily_minutes INTEGER CHECK (daily_minutes BETWEEN 5 AND 1440);
ALTER TABLE tasks ADD COLUMN parent_id TEXT REFERENCES tasks (id);

CREATE INDEX tasks_parent ON tasks (parent_id) WHERE parent_id IS NOT NULL AND deleted_at IS NULL;
