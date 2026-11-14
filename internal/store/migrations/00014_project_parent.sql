-- +goose Up
-- file: internal/store/migrations/00014_project_parent.sql

-- A project can sit under one other project, one level deep: the parent only
-- groups its sub-projects. Synced with the rest of the row. No foreign key: a
-- synced sub-project may arrive before its parent, and the store reads a
-- parent that is missing, deleted, or itself a sub-project as none.
ALTER TABLE projects ADD COLUMN parent_id TEXT CHECK (length(parent_id) = 36 AND parent_id <> id);

CREATE INDEX projects_parent ON projects (parent_id) WHERE parent_id IS NOT NULL AND deleted_at IS NULL;
