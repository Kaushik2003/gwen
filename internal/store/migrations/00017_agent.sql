-- +goose Up
-- file: internal/store/migrations/00017_agent.sql

-- Gwen's agent (plan.md 3.16, W32): a chat belongs to a project, or to none
-- ("everywhere"), and that is synced with the chat. The Claude Code session
-- behind a chat, and the steps it took, stay on the machine that ran them,
-- because they touch its folders. A step keeps its input (at most 16 KB, for
-- the diff and the command) and at most 2 KB of output text, never a file's
-- contents from a read.
ALTER TABLE chat_threads ADD COLUMN project_id TEXT CHECK (project_id IS NULL OR length(project_id) = 36);

CREATE TABLE agent_chats (
    thread_id         TEXT PRIMARY KEY,
    claude_session_id TEXT NOT NULL,
    cwd               TEXT NOT NULL,
    state             TEXT NOT NULL CHECK (state IN ('idle', 'starting', 'thinking', 'waiting', 'error')),
    error             TEXT NOT NULL DEFAULT '',
    usage             TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(usage)),
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
) STRICT;

CREATE TABLE agent_steps (
    id          TEXT PRIMARY KEY CHECK (length(id) = 36),
    thread_id   TEXT NOT NULL,
    tool_use_id TEXT NOT NULL DEFAULT '',
    turn        INTEGER NOT NULL,
    position    INTEGER NOT NULL,
    tool        TEXT NOT NULL,
    capability  TEXT NOT NULL,
    risk        TEXT NOT NULL,
    title       TEXT NOT NULL,
    input       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input)),
    output      TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('running', 'waiting', 'done', 'failed', 'denied', 'cancelled')),
    approval_id TEXT,
    undo        TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(undo)),
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER
) STRICT;
CREATE INDEX agent_steps_thread ON agent_steps (thread_id, turn, position);
CREATE INDEX agent_steps_tool_use ON agent_steps (thread_id, tool_use_id) WHERE tool_use_id <> '';

-- '' follows [agent] autonomy.
ALTER TABLE project_dirs ADD COLUMN agent_autonomy TEXT NOT NULL DEFAULT ''
    CHECK (agent_autonomy IN ('', 'cautious', 'balanced', 'trusted'));
