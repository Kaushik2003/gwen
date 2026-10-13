package store

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

// The assistant runs of a v8 database, which each held the whole
// conversation so far, become one thread per conversation, and each run
// keeps only its own turn.
func TestChatsMigrationImportsConversations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	v8 := openTest(t, dir, migrationsUpTo(t, 8))
	require.Equal(t, int64(8), v8.SchemaVersion())

	msg := func(role, text string, at int64, extra string) string {
		return `{"role": "` + role + `", "text": "` + text + `", "at": ` + strconv.FormatInt(at, 10) + extra + `}`
	}
	u1 := msg("user", "Add my essay at ten", 1000, "")
	a1 := msg("assistant", "Done.", 2000, `, "mood": "smug", "attitude": "angry", "actions": [{"type": "create_task", "ok": true, "summary": "Added \"Essay\""}]`)
	u2 := msg("user", "What next?", 3000, "")
	a2 := msg("assistant", "The essay.", 4000, `, "mood": "smiling"`)
	u3 := msg("user", "Plan my gym week", 5000, "")
	a3 := msg("assistant", "Three sessions.", 6000, "")
	const (
		r1     = "00000000-0000-7000-8000-000000000001"
		r2     = "00000000-0000-7000-8000-000000000002"
		r3     = "00000000-0000-7000-8000-000000000003"
		failed = "00000000-0000-7000-8000-000000000004"
		plan   = "00000000-0000-7000-8000-000000000005"
		broken = "00000000-0000-7000-8000-000000000006"
	)
	for _, r := range []struct{ id, kind, status, output string }{
		{r1, "assistant", "ok", `{"messages": [` + u1 + `, ` + a1 + `]}`},
		{r2, "assistant", "ok", `{"messages": [` + u1 + `, ` + a1 + `, ` + u2 + `, ` + a2 + `]}`},
		{r3, "assistant", "ok", `{"messages": [` + u3 + `, ` + a3 + `]}`},
		{failed, "assistant", "failed", `{"error": "HTTP 500"}`},
		{plan, "day_plan", "ok", `{"messages": [` + u3 + `], "items": []}`},
		{broken, "assistant", "ok", `{"messages": [{"role": "system", "text": "x"}]}`},
	} {
		_, err := v8.SQL().Exec(`INSERT INTO llm_runs (id, kind, subject_id, status, output, created_at, updated_at)
			VALUES (?, ?, '2026-09-15', ?, ?, ?, ?)`, r.id, r.kind, r.status, r.output, len(r.output), len(r.output))
		require.NoError(t, err)
	}
	require.NoError(t, v8.Close())

	db, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.Equal(t, int64(10), db.SchemaVersion())
	chats := NewChatRepo(db)

	threads, err := chats.List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, threads, 2, "one thread per conversation")
	require.Equal(t, r3, threads[0].ID, "a thread has the id of its conversation's last run")
	require.Equal(t, "Plan my gym week", threads[0].Title)
	require.Equal(t, r2, threads[1].ID)
	require.Equal(t, "Add my essay at ten", threads[1].Title)
	require.Equal(t, time.UnixMilli(1000).UTC(), threads[1].CreatedAt)
	require.Equal(t, time.UnixMilli(4000).UTC(), threads[1].LastAt)

	msgs, err := chats.Messages(ctx, r2)
	require.NoError(t, err)
	require.Len(t, msgs, 4)
	require.Equal(t, []string{"Add my essay at ten", "Done.", "What next?", "The essay."},
		[]string{msgs[0].Text, msgs[1].Text, msgs[2].Text, msgs[3].Text})
	require.Equal(t, "smug", msgs[1].Mood)
	require.Equal(t, "angry", msgs[1].Attitude)
	require.JSONEq(t, `[{"type": "create_task", "ok": true, "summary": "Added \"Essay\""}]`, string(msgs[1].Actions))
	require.JSONEq(t, `[]`, string(msgs[3].Actions))

	runs := NewLLMRunRepo(db)
	for id, want := range map[string]struct{ thread, output string }{
		r1:     {r2, `{"thread_id": "` + r2 + `", "messages": [` + u1 + `, ` + a1 + `]}`},
		r2:     {r2, `{"thread_id": "` + r2 + `", "messages": [` + u2 + `, ` + a2 + `]}`},
		r3:     {r3, `{"thread_id": "` + r3 + `", "messages": [` + u3 + `, ` + a3 + `]}`},
		failed: {"2026-09-15", `{"error": "HTTP 500"}`},
		plan:   {"2026-09-15", `{"messages": [` + u3 + `], "items": []}`},
		broken: {"2026-09-15", `{"messages": [{"role": "system", "text": "x"}]}`},
	} {
		run, err := runs.Get(ctx, id)
		require.NoError(t, err)
		require.Equal(t, want.thread, run.SubjectID, id)
		require.JSONEq(t, want.output, string(run.Output), id)
	}

	hits, err := chats.Search(ctx, "essay", 10, nil)
	require.NoError(t, err)
	require.Len(t, hits, 2, "imported messages are searchable")

	// Deleting an imported chat leaves its words nowhere in the database,
	// though the backup taken before the upgrade still has them.
	require.NoError(t, chats.Delete(ctx, r2))
	raw, err := os.ReadFile(filepath.Join(dir, DatabaseFile))
	require.NoError(t, err)
	require.False(t, strings.Contains(string(raw), "Add my essay at ten"), "Add my essay at ten")
	require.False(t, strings.Contains(string(raw), "The essay."), "The essay.")
	require.True(t, strings.Contains(string(raw), "Plan my gym week"), "Plan my gym week")
	backup, err := os.ReadFile(filepath.Join(dir, backupPrefix+"8"))
	require.NoError(t, err)
	require.True(t, strings.Contains(string(backup), "Add my essay at ten"), "Add my essay at ten")

	// Once is enough: a new run of the old shape is not imported again.
	_, err = db.SQL().Exec(`INSERT INTO llm_runs (id, kind, subject_id, status, output, created_at, updated_at)
		VALUES (?, 'assistant', '', 'ok', ?, 1, 1)`, model.NewID(), `{"messages": [`+u3+`]}`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	again, err := Open(ctx, dir, clock.NewFake(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	t.Cleanup(func() { again.Close() })
	threads, err = NewChatRepo(again).List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, threads, 1)
}
