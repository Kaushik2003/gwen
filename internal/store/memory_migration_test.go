package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

// 00010_memory.sql applies to a v9 database holding chats, whose messages
// read back with no cites and nothing to forget.
func TestMemoryMigrationOnV9Chats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	v9 := openTest(t, dir, migrationsUpTo(t, 9))
	require.Equal(t, int64(9), v9.SchemaVersion())
	const (
		thread = "00000000-0000-7000-8000-000000000001"
		user   = "00000000-0000-7000-8000-000000000002"
		reply  = "00000000-0000-7000-8000-000000000003"
	)
	for _, q := range []string{
		`INSERT INTO chat_threads (id, title, created_at, updated_at, device_id, rev)
			VALUES ('` + thread + `', 'Where I work', 1000, 1000, 'dev', 1)`,
		`INSERT INTO chat_messages (id, thread_id, role, text, at, created_at, updated_at, device_id, rev)
			VALUES ('` + user + `', '` + thread + `', 'user', 'I work at Acme', 1000, 1000, 1000, 'dev', 1)`,
		`INSERT INTO chat_messages (id, thread_id, role, text, at, mood, actions, created_at, updated_at, device_id, rev)
			VALUES ('` + reply + `', '` + thread + `', 'assistant', 'Noted.', 2000, 'happy', '[]', 2000, 2000, 'dev', 1)`,
	} {
		_, err := v9.SQL().Exec(q)
		require.NoError(t, err)
	}
	require.NoError(t, v9.Close())

	db, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(10), db.SchemaVersion())
	for _, table := range []string{"entities", "episodes", "claims", "conflicts", "memory_links", "self_versions",
		"chat_observations", "chat_notes", "profile_cards", "memory_jobs", "memory_backends", "memory_usage",
		"resources", "ledger"} {
		require.True(t, tableExists(t, db.SQL(), table), table)
	}
	chats := NewChatRepo(db)
	msgs, err := chats.Messages(ctx, thread)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	for _, m := range msgs {
		require.Nil(t, m.Cites, m.ID)
		require.Nil(t, m.Forget, m.ID)
	}
	require.Equal(t, "I work at Acme", msgs[0].Text)
	require.Equal(t, "happy", msgs[1].Mood)
	var memory string
	require.NoError(t, db.SQL().QueryRow(`SELECT memory FROM chat_messages WHERE id = ?`, user).Scan(&memory))
	require.Equal(t, "{}", memory)

	rows, err := NewMemoryRepo(db).MessagesAfter(ctx, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2, "the memory reads the chats from before it")
	require.Equal(t, "Where I work", rows[0].ThreadTitle)

	_, stored, err := chats.Append(ctx, thread, []model.ChatMessage{{Role: model.ChatAssistant, Text: "Acme it is.",
		Cites: []string{"msg:" + user}}})
	require.NoError(t, err)
	msgs, err = chats.Messages(ctx, thread)
	require.NoError(t, err)
	require.Equal(t, stored[0].ID, msgs[2].ID)
	require.Equal(t, []string{"msg:" + user}, msgs[2].Cites)
}

// selfIn opens a v9 database in dir, keeps value as assistant_self (none
// when ""), and opens it again at the newest version.
func selfIn(t *testing.T, dir, value string) *DB {
	t.Helper()
	ctx := context.Background()
	v9 := openTest(t, dir, migrationsUpTo(t, 9))
	if value != "" {
		require.NoError(t, SetLocal(ctx, v9.SQL(), KeyAssistantSelf, value, t0))
	}
	require.NoError(t, v9.Close())
	db, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(10), db.SchemaVersion())
	return db
}

// How the assistant chose to treat the user moves from local_state to
// self_versions once, and the old key goes.
func TestSelfImport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	const note = "They want it blunt and short"
	db := selfIn(t, dir, `{"attitude": "smug", "note": "`+note+`", "since": 1757926800000}`)

	self := NewSelfRepo(db)
	v, err := self.Current(ctx)
	require.NoError(t, err)
	require.Equal(t, model.SelfImport, v.Reason)
	require.Equal(t, "smug", v.Attitude)
	require.Equal(t, note, v.Note)
	require.Equal(t, time.UnixMilli(1757926800000).UTC(), *v.Since)
	require.Nil(t, v.ThreadID)
	require.Equal(t, db.DeviceID(), v.DeviceID)
	_, err = GetLocal(ctx, db.SQL(), KeyAssistantSelf)
	require.ErrorIs(t, err, ErrNotFound, "the old key is deleted")
	_, err = GetLocalTime(ctx, db.SQL(), KeySelfImported)
	require.NoError(t, err)

	// Once is enough: a value written again under the old key stays there.
	require.NoError(t, SetLocal(ctx, db.SQL(), KeyAssistantSelf, `{"attitude": "angry"}`, t0))
	require.NoError(t, db.Close())
	again, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { again.Close() })
	history, err := NewSelfRepo(again).History(ctx, 0)
	require.NoError(t, err)
	require.Len(t, history, 1)
	_, err = GetLocal(ctx, again.SQL(), KeyAssistantSelf)
	require.NoError(t, err)
}

func TestSelfImportNothingToKeep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, value := range map[string]string{
		"none":       "",
		"empty":      `{"attitude": "", "note": "", "since": null}`,
		"unreadable": `{"attitude": `,
	} {
		db := selfIn(t, t.TempDir(), value)
		_, err := NewSelfRepo(db).Current(ctx)
		require.ErrorIs(t, err, ErrNotFound, name)
		_, err = GetLocal(ctx, db.SQL(), KeyAssistantSelf)
		require.True(t, errors.Is(err, ErrNotFound), name)
		_, err = GetLocal(ctx, db.SQL(), KeySelfImported)
		require.NoError(t, err, name)
	}

	// A long note from before the limit is cut, not refused.
	db := selfIn(t, t.TempDir(), `{"attitude": "soft", "note": "`+strings.Repeat("é", 700)+`"}`)
	v, err := NewSelfRepo(db).Current(ctx)
	require.NoError(t, err)
	require.Len(t, []rune(v.Note), 600)
	require.Nil(t, v.Since)
}

// A new database has nothing to import and is marked so at once.
func TestSelfImportNewDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir(), clock.NewFake(t0))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	at, err := GetLocalTime(ctx, db.SQL(), KeySelfImported)
	require.NoError(t, err)
	require.Equal(t, t0, at.UTC())
}
