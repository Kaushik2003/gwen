-- +goose Up
-- file: internal/store/migrations/00011_remote.sql

-- Gwen on the phone (plan.md 3.14). Both tables are device-local and never
-- synced: a paired phone and a Claude Code session belong to this laptop.

-- A paired phone. token_hash is the SHA-256, in hex, of the token its cookie
-- carries; the token itself is never stored.
CREATE TABLE remote_devices (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER,
    revoked_at INTEGER
) STRICT;

-- A Claude Code session in a herdr-core terminal. terminal is the core's
-- address for it ("t7"), '' once it is gone.
CREATE TABLE code_sessions (
    id TEXT PRIMARY KEY CHECK (length(id) = 13),
    terminal TEXT NOT NULL DEFAULT '',
    claude_session_id TEXT NOT NULL DEFAULT '',
    cwd TEXT NOT NULL,
    sandbox INTEGER NOT NULL DEFAULT 0 CHECK (sandbox IN (0, 1)),
    args TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(args)),
    pid INTEGER,
    transcript_path TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('starting', 'working', 'needs_you', 'done', 'idle', 'error', 'ended')),
    need TEXT NOT NULL DEFAULT '' CHECK (need IN ('', 'permission', 'question', 'other')),
    step TEXT NOT NULL DEFAULT '',
    last_message TEXT NOT NULL DEFAULT '',
    permission_mode TEXT NOT NULL DEFAULT '',
    start_status TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(start_status)),
    touched TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(touched)),
    pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
    seen_at INTEGER,
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    ended_at INTEGER
) STRICT;
CREATE INDEX code_sessions_claude ON code_sessions (claude_session_id);
CREATE INDEX code_sessions_terminal ON code_sessions (terminal);
