package store_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

func say(role, text string, at time.Time) model.ChatMessage {
	return model.ChatMessage{Role: role, Text: text, At: at}
}

func TestChatThreads(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	chats := f.r.Chats

	empty, err := chats.List(f.ctx, 10, 0)
	require.NoError(t, err)
	require.Empty(t, empty)

	exam, msgs, err := chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, "  I have an exam on Friday,\nhelp me prepare", at("09:00")),
		{Role: model.ChatAssistant, Text: "Added three study blocks.", At: at("09:01"), Mood: "happy",
			Actions: []byte(`[{"type":"create_task","ok":true,"summary":"Added \"Revise\""}]`)},
	})
	require.NoError(t, err)
	require.Len(t, exam.ID, 36)
	require.Equal(t, "I have an exam on Friday,", exam.Title, "titled by the first line of the first message")
	require.Equal(t, 2, exam.Messages)
	require.Equal(t, at("09:01"), exam.LastAt)
	require.Len(t, msgs, 2)
	require.Equal(t, exam.ID, msgs[1].ThreadID)
	require.Equal(t, "happy", msgs[1].Mood)

	gym, _, err := chats.Append(f.ctx, "", []model.ChatMessage{say(model.ChatUser, "Skipped the gym again", at("10:00"))})
	require.NoError(t, err)
	_, _, err = chats.Append(f.ctx, exam.ID, []model.ChatMessage{
		say(model.ChatUser, "Which chapters first?", at("11:00")),
		say(model.ChatAssistant, "Start with chapter 4.", time.Time{}),
	})
	require.NoError(t, err)

	list, err := chats.List(f.ctx, 10, 0)
	require.NoError(t, err)
	require.Equal(t, []string{exam.ID, gym.ID}, []string{list[0].ID, list[1].ID}, "the most recently active first")
	require.Equal(t, 4, list[0].Messages)
	require.Equal(t, fxNow, list[0].LastAt, "a message without a time is said now")
	one, err := chats.List(f.ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, one, 1)
	next, err := chats.List(f.ctx, 1, 1)
	require.NoError(t, err)
	require.Equal(t, gym.ID, next[0].ID, "the next page")
	none, err := chats.List(f.ctx, 1, 2)
	require.NoError(t, err)
	require.Empty(t, none)

	got, err := chats.Messages(f.ctx, exam.ID)
	require.NoError(t, err)
	require.Len(t, got, 4)
	require.Equal(t, "Start with chapter 4.", got[3].Text)
	require.JSONEq(t, `[]`, string(got[2].Actions), "no actions is an empty list")
	require.JSONEq(t, `[{"type":"create_task","ok":true,"summary":"Added \"Revise\""}]`, string(got[1].Actions))

	_, _, err = chats.Append(f.ctx, model.NewID(), []model.ChatMessage{say(model.ChatUser, "hi", at("12:00"))})
	require.ErrorIs(t, err, store.ErrNotFound)
	_, _, err = chats.Append(f.ctx, gym.ID, []model.ChatMessage{say("system", "x", at("12:00"))})
	require.ErrorIs(t, err, store.ErrInvalid)
	_, _, err = chats.Append(f.ctx, gym.ID, nil)
	require.ErrorIs(t, err, store.ErrInvalid)
	_, _, err = chats.Append(f.ctx, gym.ID, []model.ChatMessage{{Role: model.ChatAssistant, Text: "x",
		Actions: []byte(`{"not":"a list"}`)}})
	require.ErrorIs(t, err, store.ErrInvalid, "actions are a JSON list")
	again, err := chats.Messages(f.ctx, gym.ID)
	require.NoError(t, err)
	require.Len(t, again, 1, "a failed append adds nothing")
}

func TestChatTitle(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	long := "Plan my whole week around the internship, the two exams, the gym, and the reading group that meets on Thursdays"
	th, _, err := f.r.Chats.Append(f.ctx, "", []model.ChatMessage{say(model.ChatUser, long, at("09:00"))})
	require.NoError(t, err)
	require.Equal(t, "Plan my whole week around the internship, the two exams, the gym, and the…", th.Title)
	require.LessOrEqual(t, len([]rune(th.Title)), store.MaxChatTitle)
}

func TestChatSearch(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	chats := f.r.Chats
	exam, msgs, err := chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, "I have a physics exam on Friday", at("09:00")),
		say(model.ChatAssistant, "I planned revision sessions for the exam.", at("09:01")),
	})
	require.NoError(t, err)
	gym, _, err := chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, "I keep skipping the gym", at("10:00")),
		say(model.ChatAssistant, "Running counts too.", at("10:01")),
	})
	require.NoError(t, err)

	hits, err := chats.Search(f.ctx, "How did my exams go?", 10, nil)
	require.NoError(t, err)
	require.Len(t, hits, 2, "exams finds exam: words are stemmed")
	require.Equal(t, exam.Title, hits[0].ThreadTitle)
	for _, h := range hits {
		require.Equal(t, exam.ID, h.ThreadID)
	}

	hits, err = chats.Search(f.ctx, "physics exam", 10, nil)
	require.NoError(t, err)
	require.Equal(t, msgs[0].ID, hits[0].ID, "the message with more of the words ranks first")

	hits, err = chats.Search(f.ctx, "exam", 10, []string{msgs[0].ID})
	require.NoError(t, err)
	require.Len(t, hits, 1, "skipped messages are left out")
	require.Equal(t, msgs[1].ID, hits[0].ID)

	hits, err = chats.Search(f.ctx, "run", 10, nil)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, gym.ID, hits[0].ThreadID)

	for _, q := range []string{"", "ok", "and the you", `"exam" OR NEAR(`, "*"} {
		hits, err = chats.Search(f.ctx, q, 10, nil)
		require.NoError(t, err, q)
		if q == `"exam" OR NEAR(` {
			require.Len(t, hits, 2, "query syntax in the text is read as words")
		} else {
			require.Empty(t, hits, q)
		}
	}

	hits, err = chats.Search(f.ctx, "Résumé café", 10, nil)
	require.NoError(t, err)
	require.Empty(t, hits)
	_, _, err = chats.Append(f.ctx, gym.ID, []model.ChatMessage{say(model.ChatUser, "Update my resume at the cafe", at("11:00"))})
	require.NoError(t, err)
	hits, err = chats.Search(f.ctx, "Résumé café", 10, nil)
	require.NoError(t, err)
	require.Len(t, hits, 1, "accents do not matter")
}

func TestDeleteChat(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	chats := f.r.Chats
	th, _, err := chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, "My secret plan for the zanzibar startup"+strings.Repeat(", and then the next step", 300), at("09:00")),
		say(model.ChatAssistant, "Your secret is safe.", at("09:01")),
	})
	require.NoError(t, err)
	_, run, err := chats.AppendTurn(f.ctx, th.ID, []model.ChatMessage{say(model.ChatUser, "Is my secret plan good?", at("09:02"))},
		func(id string, msgs []model.ChatMessage) any {
			return map[string]any{"thread_id": id, "messages": []map[string]string{{"role": "user", "text": msgs[0].Text}}}
		})
	require.NoError(t, err)
	require.Equal(t, th.ID, run.SubjectID)
	require.Equal(t, model.RunAssistant, run.Kind)
	require.Contains(t, string(run.Output), "Is my secret plan good?")
	busy, _, err := chats.Append(f.ctx, "", []model.ChatMessage{say(model.ChatUser, "Another chat", at("10:00"))})
	require.NoError(t, err)
	for i := range 60 {
		_, _, err := chats.Append(f.ctx, busy.ID, []model.ChatMessage{say(model.ChatUser, strings.Repeat("filler ", 40+i), at("10:01"))})
		require.NoError(t, err)
	}
	other, err := f.r.LLMRuns.Create(f.ctx, model.RunAssistant, model.NewID(), model.RunOK,
		map[string]any{"messages": []map[string]string{{"role": "user", "text": "another secret"}}})
	require.NoError(t, err)

	require.NoError(t, chats.Delete(f.ctx, th.ID))
	require.ErrorIs(t, chats.Delete(f.ctx, th.ID), store.ErrNotFound)
	_, err = chats.Get(f.ctx, th.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = chats.Messages(f.ctx, th.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	list, err := chats.List(f.ctx, 10, 0)
	require.NoError(t, err)
	require.Equal(t, []string{busy.ID}, []string{list[0].ID})
	hits, err := chats.Search(f.ctx, "secret", 10, nil)
	require.NoError(t, err)
	require.Empty(t, hits)

	var words int
	require.NoError(t, f.db.SQL().QueryRow(`SELECT count(*) FROM chat_messages WHERE text LIKE '%secret%'`).Scan(&words))
	require.Zero(t, words, "the words are erased, not just hidden")
	require.NoError(t, f.db.SQL().QueryRow(`SELECT count(*) FROM chat_threads WHERE title LIKE '%secret%'`).Scan(&words))
	require.Zero(t, words)
	var tombstones int
	require.NoError(t, f.db.SQL().QueryRow(`SELECT count(*) FROM chat_messages WHERE deleted_at IS NOT NULL AND rev = 2`).
		Scan(&tombstones))
	require.Equal(t, 3, tombstones)
	require.NoError(t, f.db.SQL().QueryRow(`SELECT count(*) FROM chat_messages_fts WHERE chat_messages_fts MATCH 'secret'`).
		Scan(&words))
	require.Zero(t, words, "nor are they left in the search index")

	var file string
	require.NoError(t, f.db.SQL().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file))
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(raw), "secret plan"), "nor left in the file's free space")
	require.False(t, strings.Contains(string(raw), "secret is safe"), "secret is safe")
	require.False(t, strings.Contains(string(raw), "zanzibar"), "nor in the search index")
	require.True(t, strings.Contains(string(raw), "another secret"), "the check can see words that are there")

	got, err := f.r.LLMRuns.Get(f.ctx, run.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"thread_id": "`+th.ID+`", "messages": []}`, string(got.Output), "its turns' runs keep no words")
	kept, err := f.r.LLMRuns.Get(f.ctx, other.ID)
	require.NoError(t, err)
	require.Contains(t, string(kept.Output), "another secret", "other runs are untouched")
}
