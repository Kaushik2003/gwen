-- +goose Up
-- file: internal/store/migrations/00006_task_starts.sql

ALTER TABLE tasks ADD COLUMN start_day TEXT CHECK (start_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]');
ALTER TABLE tasks ADD COLUMN start_minute INTEGER CHECK (start_minute BETWEEN 0 AND 1439);
