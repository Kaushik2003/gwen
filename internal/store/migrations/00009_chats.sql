-- +goose Up
-- file: internal/store/migrations/00009_chats.sql

-- The assistant's conversations, a row per message, so they can be listed,
-- searched, and recalled. Until now each turn's llm run held the whole
-- conversation again. These tables are not synced yet; their rows carry the
-- envelope for when they are. A deleted thread keeps tombstones whose words
-- are erased.
CREATE TABLE chat_threads (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    title      TEXT NOT NULL CHECK (length(title) <= 200),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE TABLE chat_messages (
    id         TEXT PRIMARY KEY CHECK (length(id) = 36),
    thread_id  TEXT NOT NULL REFERENCES chat_threads (id),
    role       TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    text       TEXT NOT NULL,
    at         INTEGER NOT NULL,
    mood       TEXT NOT NULL DEFAULT '',
    attitude   TEXT NOT NULL DEFAULT '',
    actions    TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(actions) AND json_type(actions) = 'array'),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    device_id  TEXT NOT NULL,
    rev        INTEGER NOT NULL CHECK (rev >= 1)
) STRICT;

CREATE INDEX chat_messages_thread ON chat_messages (thread_id, at);

-- Every message's words, for search and recall. The index holds the text of
-- chat_messages by rowid; the triggers keep it in step.
CREATE VIRTUAL TABLE chat_messages_fts USING fts5 (
    text,
    content = 'chat_messages',
    content_rowid = 'rowid',
    tokenize = 'porter unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER chat_messages_fts_insert AFTER INSERT ON chat_messages BEGIN
    INSERT INTO chat_messages_fts (rowid, text) VALUES (new.rowid, new.text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER chat_messages_fts_delete AFTER DELETE ON chat_messages BEGIN
    INSERT INTO chat_messages_fts (chat_messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER chat_messages_fts_update AFTER UPDATE OF text ON chat_messages BEGIN
    INSERT INTO chat_messages_fts (chat_messages_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
    INSERT INTO chat_messages_fts (rowid, text) VALUES (new.rowid, new.text);
END;
-- +goose StatementEnd
