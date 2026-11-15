-- +goose Up
-- file: internal/store/migrations/00018_timeline.sql

-- The agentic timeline (plan.md 3.17).
--
-- A task can wait on other tasks: waits_on is a JSON array of their ids, in
-- the order they were added. It syncs with the rest of the row. No foreign
-- key, like parent_id: a synced task may arrive before what it waits on, and
-- readers skip ids that are missing, deleted, or done. The store refuses a
-- write that would make a loop.
ALTER TABLE tasks ADD COLUMN waits_on TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(waits_on));

-- The task a Claude session works on, kept from its terminal's gwen.task
-- label so it outlives the terminal; '' for none.
ALTER TABLE code_sessions ADD COLUMN task_id TEXT NOT NULL DEFAULT '';

-- When each Claude session was in each state, for drawing what agents did
-- over a day. One row per state a session entered; ended_at is NULL on the
-- state it is in now. Kept on this machine, with its sessions.
CREATE TABLE agent_spans (
    id         INTEGER PRIMARY KEY,
    session_id TEXT NOT NULL,
    state      TEXT NOT NULL,
    need       TEXT NOT NULL DEFAULT '',
    task_id    TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    ended_at   INTEGER
) STRICT;
CREATE INDEX agent_spans_session ON agent_spans (session_id, started_at);
CREATE INDEX agent_spans_time ON agent_spans (started_at);
