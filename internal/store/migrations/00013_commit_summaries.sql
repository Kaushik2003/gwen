-- +goose Up
-- file: internal/store/migrations/00013_commit_summaries.sql

-- Gwen's short summary of a commit, made from the Claude conversation that
-- made it when you ask for it in History (plan.md 3.15). Keyed by Claude's own
-- session id and the short commit id git printed there, so it outlives the
-- code session that ran it. Device-local and never synced, like
-- code_sessions: the conversation lives on this laptop.
CREATE TABLE commit_summaries (
    claude_session_id TEXT NOT NULL,
    commit_short TEXT NOT NULL,
    summary TEXT NOT NULL,
    made_at INTEGER NOT NULL,
    PRIMARY KEY (claude_session_id, commit_short)
) STRICT;
