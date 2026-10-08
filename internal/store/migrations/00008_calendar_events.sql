-- +goose Up
-- file: internal/store/migrations/00008_calendar_events.sql

-- Busy time is read from the events themselves, so the plan can show what
-- each block is. Rows cached before this hold no title; the next sync
-- replaces them.
ALTER TABLE calendar_busy ADD COLUMN title TEXT NOT NULL DEFAULT '';
