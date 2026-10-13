package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// chat stores a chat of alternating user and assistant messages, said a
// minute apart from 07:00, and returns its id and the messages as stored.
func chat(t *testing.T, ts *testServer, texts ...string) (string, []model.ChatMessage) {
	t.Helper()
	msgs := make([]model.ChatMessage, len(texts))
	for i, text := range texts {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		msgs[i] = model.ChatMessage{Role: role, Text: text, At: testutil.At("07:00").Add(time.Duration(i) * time.Minute),
			Actions: []byte("[]")}
	}
	th, stored, err := ts.Repos.Chats.Append(ts.ctx, "", msgs)
	require.NoError(t, err)
	return th.ID, stored
}

func TestTranscriptRendersTheSameBytes(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, nil)
	taskID := "99999999-9999-4999-8999-999999999999"
	acts, err := json.Marshal([]wire.ActionResult{{Type: "create_task", OK: true, Summary: `Added "Essay" <draft>`,
		TaskID: &taskID}, {Type: "update_task", OK: false, Summary: "update task", Error: "no such task"}})
	require.NoError(t, err)
	th, stored, err := ts.Repos.Chats.Append(ts.ctx, "", []model.ChatMessage{
		{Role: llm.RoleUser, Text: "Add my essay", At: testutil.At("07:00"), Actions: []byte("[]")},
		{Role: llm.RoleAssistant, Text: "Done & dusted.", At: testutil.At("07:01"), Mood: "smug", Actions: acts},
		{Role: llm.RoleUser, Text: "Thanks", At: testutil.At("07:02"), Actions: []byte("[]")},
		{Role: llm.RoleAssistant, Text: "Any time.", At: testutil.At("07:03"), Actions: []byte("[]")},
	})
	require.NoError(t, err)
	note := "What you remember that may bear on this (cite [mN] when you rely on one):\n[m1] Works at Acme"
	require.NoError(t, ts.Repos.MemoryState.AddNote(ts.ctx, model.ChatNote{MessageID: stored[0].ID, ThreadID: th.ID,
		Note: note, Refs: map[string]string{"m1": "claim:" + claimA, "m7": "msg:" + stored[2].ID}}))

	first, err := ts.transcript(ts.ctx, th.ID)
	require.NoError(t, err)
	require.Equal(t, []llm.Turn{
		{Role: llm.RoleUser, Text: "Add my essay\n\n<memory>\n" + note + "\n</memory>"},
		{Role: llm.RoleAssistant, Text: `{"reply":"Done & dusted.","mood":"smug","did":["Added \"Essay\" <draft>",` +
			`"failed: update task"]}`},
		{Role: llm.RoleUser, Text: "Thanks"},
		{Role: llm.RoleAssistant, Text: `{"reply":"Any time.","mood":""}`},
	}, first.turns)
	require.Equal(t, 8, first.refStart, "after the highest ref the chat's notes used")
	require.Empty(t, first.log)

	// Built again from the same rows, the turns are the same bytes, so the prompt cache holds.
	again, err := ts.transcript(ts.ctx, th.ID)
	require.NoError(t, err)
	require.Equal(t, first.turns, again.turns)
	for i := range first.turns {
		require.True(t, first.turns[i].Text == again.turns[i].Text)
	}
	require.Equal(t, map[string]bool{taskID: true}, namedTasks(first.sent))

	// A chat longer than the window sends its newest messages.
	texts := make([]string, llm.MaxAssistantMessages+6)
	for i := range texts {
		texts[i] = fmt.Sprintf("message %d", i)
	}
	long, _ := chat(t, ts, texts...)
	tr, err := ts.transcript(ts.ctx, long)
	require.NoError(t, err)
	require.Len(t, tr.turns, llm.MaxAssistantMessages)
	require.Equal(t, "message 6", tr.turns[0].Text)
	require.Equal(t, 1, tr.refStart)

	empty, err := ts.transcript(ts.ctx, "")
	require.NoError(t, err)
	require.Empty(t, empty.turns)
	require.Equal(t, 1, empty.refStart)
}

func TestTranscriptCutsOverAtTheObservationLog(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	ts := newTestServer(t, mem)
	texts := make([]string, 40)
	for i := range texts {
		texts[i] = fmt.Sprintf("message %d", i)
	}
	th, stored := chat(t, ts, texts...)

	// Without a log, the newest messages.
	tr, err := ts.transcript(ts.ctx, th)
	require.NoError(t, err)
	require.Len(t, tr.turns, llm.MaxAssistantMessages)
	require.Empty(t, tr.log)

	// With one, every message after the last it covers, however many.
	mem.obs[th] = model.ChatObservation{ThreadID: th, ThroughID: stored[9].ID, Log: "- 2026-09-15 [high] An essay."}
	tr, err = ts.transcript(ts.ctx, th)
	require.NoError(t, err)
	require.Len(t, tr.turns, 30)
	require.Equal(t, "message 10", tr.turns[0].Text)
	require.Equal(t, "- 2026-09-15 [high] An essay.", tr.log)
	require.Equal(t, messageRefs(stored[10:]), messageRefs(tr.sent))

	// A log through a message it cannot find is not used.
	mem.obs[th] = model.ChatObservation{ThreadID: th, ThroughID: "gone", Log: "- old"}
	tr, err = ts.transcript(ts.ctx, th)
	require.NoError(t, err)
	require.Len(t, tr.turns, llm.MaxAssistantMessages)
	require.Empty(t, tr.log)

	require.Equal(t, "## What you keep in mind about them\n\n- [p1] Works at Acme\n\n"+
		"## Earlier in this conversation (your observations)\n\n- 2026-09-15 [high] An essay.",
		memoryContext("- [p1] Works at Acme\n", "- 2026-09-15 [high] An essay."))
	require.Equal(t, "## Earlier in this conversation (your observations)\n\n- x", memoryContext("", "- x"))
	require.Empty(t, memoryContext(" ", ""))
}

func TestChooseTasks(t *testing.T) {
	t.Parallel()
	today := civil.MustParse("2026-09-15")
	day := func(n int) *string { s := today.AddDays(n).String(); return &s }
	task := func(i int) model.Task {
		return model.Task{ID: fmt.Sprintf("t%03d", i), Title: fmt.Sprintf("Chore %d", i), Priority: 1,
			Stage: model.StageTodo, Envelope: model.Envelope{CreatedAt: testutil.T0.Add(time.Duration(i) * time.Minute)}}
	}

	// At most assistantTasks: all of them, as they are.
	few := make([]model.Task, assistantTasks)
	for i := range few {
		few[i] = task(i)
	}
	require.Equal(t, few, chooseTasks(few, nil, today, []string{"anything"}, nil))

	var tasks []model.Task
	for i := range 120 {
		tasks = append(tasks, task(i))
	}
	tasks[100].Stage = model.StageDoing
	tasks[101].DueDay = day(-3) // overdue
	tasks[102].DueDay = day(7)  // due within a week
	tasks[103].DueDay = day(30) // later
	tasks[104].Title, tasks[104].Notes = "Chapter review", "Finish the essays for school"
	tasks[105].Title = "Buy groceries"
	blocks := map[string][]llm.AssistantBlock{tasks[106].ID: {{Day: today.AddDays(2).String(), Minutes: 30}}}
	for i := 107; i < 117; i++ {
		tasks[i].Priority = 4
	}
	for i := 80; i < 100; i++ { // 20 inbox tasks; the newest are the highest numbers
		tasks[i].Stage = model.StageInbox
	}
	named := map[string]bool{tasks[118].ID: true}
	said := []string{"I wrote my essay", "about school", "how is the essay going?"}

	got := chooseTasks(tasks, blocks, today, said, named)
	require.Len(t, got, assistantTasks)
	ids := make([]string, len(got))
	for i, t := range got {
		ids[i] = t.ID
	}
	require.True(t, slices.IsSorted(ids), "the chosen keep the list's order")
	has := func(i int) bool { return slices.Contains(ids, tasks[i].ID) }
	for _, i := range []int{100, 101, 102, 104, 106, 118} {
		require.True(t, has(i), "task %d: doing, due, related, planned, or named", i)
	}
	for i := 85; i < 100; i++ {
		require.True(t, has(i), "the 15 newest inbox tasks")
	}
	for i := 107; i < 117; i++ {
		require.True(t, has(i), "then the most important")
	}
	require.False(t, has(80), "an old inbox task waits")
	require.False(t, has(105), "nothing brings up the groceries")
	require.True(t, has(103), "among the rest, a due day goes before none")
	require.True(t, has(0), "the rest fill up by priority, then the list's order")
	require.True(t, has(27))
	require.False(t, has(28))

	require.Equal(t, []string{"essay", "school", "box", "note", "class", "plann", "use"},
		taskWords("the essays for school: boxes, notes, classes, planned, uses, and a to-do"))
}

func TestCitesAndForget(t *testing.T) {
	t.Parallel()
	notes := map[string]model.ChatNote{"u1": {Refs: map[string]string{"m1": "claim:" + claimA, "m2": "msg:x"}}}
	note := memory.Note{Refs: map[string]string{"m3": "ep:" + claimGone, "m4": "claim:" + claimGone}}
	card := model.ProfileCard{Refs: map[string]string{"p1": "claim:" + claimB}}
	table := refTable(notes, note, card)
	require.Equal(t, []string{"claim:" + claimA, "claim:" + claimB, "ep:" + claimGone},
		citedRefs([]string{"m1", "p1", "m9", "m3", "m1"}, table), "unknown refs dropped, repeats once")
	require.Nil(t, citedRefs(nil, table))
	require.Equal(t, []string{claimGone, claimB}, forgetClaims([]string{"m4", "m2", "m3", "p1", "p9", "m4"}, table),
		"only refs naming claims")
	require.Equal(t, 3, nextNoteRef(notes))
	require.Equal(t, 1, nextNoteRef(nil))
}

func TestRecallNoteWithoutMemory(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, nil)
	long := strings.Repeat("essay ", 100)
	_, stored := chat(t, ts, "My essay is due Friday", "Then start\nit tonight.", long)
	chat(t, ts, "Groceries")
	n, err := ts.recallNote(ts.ctx, "when is the essay due?", []string{stored[2].ID}, 4)
	require.NoError(t, err)
	require.Equal(t, noteHeader+"\n"+
		"[m4] 15 Sep, chat “My essay is due Friday”, they said: “My essay is due Friday”", n.Text)
	require.Equal(t, map[string]string{"m4": "msg:" + stored[0].ID}, n.Refs)

	n, err = ts.recallNote(ts.ctx, "essay", nil, 1)
	require.NoError(t, err)
	lines := strings.Split(n.Text, "\n")
	require.Len(t, lines, 2+1)
	require.True(t, strings.HasPrefix(lines[2], "[m2] 15 Sep, chat “My essay is due Friday”, they said: “essay essay"))
	require.True(t, strings.HasSuffix(lines[2], "…”"), "cut to fit")
	require.LessOrEqual(t, len([]rune(lines[2])), recallChars+80)

	n, err = ts.recallNote(ts.ctx, "nothing matches", nil, 1)
	require.NoError(t, err)
	require.Empty(t, n.Text)
	require.Equal(t, "2 Jan 2025", ts.shortDate(time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC), testutil.T0))
}

func TestAssistantTurnWithMemory(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	ts := newTestServer(t, mem)
	for i := range assistantTasks + 5 {
		_, _, err := ts.Repos.Tasks.Create(ts.ctx, store.NewTask{Title: fmt.Sprintf("Chore %d", i)})
		require.NoError(t, err)
	}

	// The first message of a chat: no note, no card, nothing before it.
	ts.planner.answer("Hi!", nil, nil)
	run, err := ts.c.AssistantChat(ts.ctx, wire.AssistantChatRequest{Message: "Hello"})
	require.NoError(t, err)
	var out wire.AssistantOutput
	require.NoError(t, json.Unmarshal(run.Output, &out))
	thread := out.ThreadID
	req := ts.planner.last()
	require.Empty(t, req.Memory)
	require.Empty(t, req.Context)
	require.Empty(t, req.History)
	require.Equal(t, "Hello", req.Message)
	require.Len(t, req.Tasks, assistantTasks)
	require.Equal(t, 5, req.MoreTasks)
	require.Equal(t, memory.NoteRequest{Text: "Hello", RefStart: 1, Exclude: []string{}, AsOf: ts.now(),
		Profile: memory.ProfileFrontier}, mem.noteReqs[0])
	notes, err := ts.Repos.MemoryState.Notes(ts.ctx, thread)
	require.NoError(t, err)
	require.Empty(t, notes, "an empty note is not kept")
	require.Len(t, mem.turns, 1)
	require.Equal(t, "Hello", mem.turns[0].User.Text)
	require.Equal(t, "Hi!", mem.turns[0].Reply.Text)

	// The next: the card and the log are the context, the note goes with the message.
	msgs, err := ts.Repos.Chats.Messages(ts.ctx, thread)
	require.NoError(t, err)
	mem.obs[thread] = model.ChatObservation{ThreadID: thread, ThroughID: msgs[0].ID, Log: "- 2026-09-15 [low] Said hello."}
	mem.card = model.ProfileCard{Day: "2026-09-15", Text: "- [p1] Lives in Pune", Refs: map[string]string{"p1": "claim:" + claimB}}
	mem.note = memory.Note{Text: noteHeader + "\n[m1] Works at Acme\n[m2] a message", Refs: map[string]string{
		"m1": "claim:" + claimA, "m2": "msg:" + msgs[0].ID}}
	ts.planner.answer("You work at Acme, and I won't bring up Pune.", []string{"m1", "p1", "m8"}, []string{"p1", "m2"})
	run, err = ts.c.AssistantChat(ts.ctx, wire.AssistantChatRequest{Message: "Where do I work?", ThreadID: &thread})
	require.NoError(t, err)
	req = ts.planner.last()
	require.Equal(t, "## What you keep in mind about them\n\n- [p1] Lives in Pune\n\n"+
		"## Earlier in this conversation (your observations)\n\n- 2026-09-15 [low] Said hello.", req.Context)
	require.Equal(t, []llm.Turn{{Role: llm.RoleAssistant, Text: `{"reply":"Hi!","mood":"smiling"}`}}, req.History,
		"the turns the log does not cover")
	require.Equal(t, mem.note.Text, req.Memory)
	nr := mem.noteReqs[1]
	require.Equal(t, thread, nr.ThreadID)
	require.Equal(t, []string{"msg:" + msgs[1].ID}, nr.Exclude, "what the model sees verbatim")

	out = wire.AssistantOutput{}
	require.NoError(t, json.Unmarshal(run.Output, &out))
	reply := out.Messages[1]
	require.Equal(t, []wire.MemoryRef{
		{Ref: "claim:" + claimA, Kind: "claim", Text: "resolved claim:" + claimA, Source: "the fake"},
		{Ref: "claim:" + claimB, Kind: "claim", Text: "resolved claim:" + claimB, Source: "the fake"},
	}, reply.Cites, "cites through the note and the card; a ref never sent is dropped")
	require.Len(t, reply.Forget, 1)
	require.Equal(t, "claim:"+claimB, reply.Forget[0].Ref)
	turn := mem.turns[1]
	require.Equal(t, []string{claimB}, turn.Forget, "only claims are forgotten")
	require.Equal(t, []string{"claim:" + claimA, "claim:" + claimB}, turn.Reply.Cites)
	require.Equal(t, []string{"claim:" + claimB}, turn.Reply.Forget)
	notes, err = ts.Repos.MemoryState.Notes(ts.ctx, thread)
	require.NoError(t, err)
	kept := notes[turn.User.ID]
	require.Equal(t, mem.note.Text, kept.Note)
	require.Equal(t, mem.note.Refs, kept.Refs)

	// Later, a ref of an earlier note still names what it named; new refs go on from there.
	mem.note = memory.Note{Text: noteHeader + "\n[m3] Lives in Pune", Refs: map[string]string{"m3": "claim:" + claimB}}
	mem.card = model.ProfileCard{}
	delete(mem.obs, thread)
	ts.planner.answer("Still Acme.", []string{"m1"}, nil)
	_, err = ts.c.AssistantChat(ts.ctx, wire.AssistantChatRequest{Message: "And still?", ThreadID: &thread})
	require.NoError(t, err)
	require.Equal(t, 3, mem.noteReqs[2].RefStart)
	require.Equal(t, []string{"claim:" + claimA}, mem.turns[2].Reply.Cites)
	req = ts.planner.last()
	require.Empty(t, req.Context, "no card and no log")
	require.Len(t, req.History, 4)
	require.Equal(t, "Where do I work?\n\n<memory>\n"+noteHeader+"\n[m1] Works at Acme\n[m2] a message\n</memory>",
		req.History[2].Text, "the note is sent again with its message")

	// The chat shows the refs resolved, and what was learned from a message.
	mem.learned = map[string][]wire.MemoryRef{turn.User.ID: {{Ref: "claim:" + claimA, Kind: "claim",
		Text: "Works at Acme", Status: wire.ClaimCandidate}}}
	detail, err := ts.c.GetThread(ts.ctx, thread)
	require.NoError(t, err)
	require.Len(t, detail.Messages, 6)
	require.Equal(t, wire.ClaimCandidate, detail.Messages[2].Learned[0].Status)
	require.Nil(t, detail.Messages[0].Learned)
	require.Len(t, detail.Messages[3].Cites, 2)
	require.Len(t, detail.Messages[3].Forget, 1)
	require.Nil(t, detail.Messages[1].Cites)

	// A memory that does not resolve refs leaves them to the store.
	ts.Memory = plainMemory{mem}
	detail, err = ts.c.GetThread(ts.ctx, thread)
	require.NoError(t, err)
	require.Equal(t, wire.MemoryRef{Ref: "claim:" + claimA, Kind: "claim", Gone: true}, detail.Messages[3].Cites[0],
		"a claim the store does not hold is gone")
}

func TestAssistantTurnSurvivesAFailingNote(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	mem.noteErr = fmt.Errorf("recall timed out")
	ts := newTestServer(t, mem)
	ts.planner.answer("Hi!", []string{"m1"}, []string{"m1"})
	run, err := ts.c.AssistantChat(ts.ctx, wire.AssistantChatRequest{Message: "Hello"})
	require.NoError(t, err)
	require.Equal(t, wire.RunOK, run.Status)
	require.Empty(t, ts.planner.last().Memory)
	require.Empty(t, mem.turns[0].Forget)
	require.Nil(t, mem.turns[0].Reply.Cites)
}

func TestStoreResolver(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, nil)
	th, stored := chat(t, ts, "I work at Acme", "Noted.")
	at := testutil.T0
	claim := model.Claim{ID: claimA, Key: "user.employer", Value: []byte(`"Acme"`), Text: "Works at Acme",
		Cardinality: model.CardinalityOne, Status: model.ClaimCandidate, Trust: model.TrustOwnerSaid,
		Sensitivity: model.SensitivityPersonal, Confidence: 0.9, MentionedAt: at, RecordedAt: at,
		SourceKind: model.SourceMessage, SourceID: stored[0].ID, Span: "I work at Acme", Channel: model.ChannelChat,
		Extractor: "test", SourceDeviceID: ts.DB.DeviceID(), ContentHash: memory.Hash("acme")}
	rejected := claim
	rejected.ID, rejected.Status, rejected.ContentHash = claimB, model.ClaimRejected, memory.Hash("acme2")
	require.NoError(t, ts.Repos.Memory.Apply(ts.ctx, store.MemoryChange{Claims: []model.Claim{claim, rejected}}))

	r := storeResolver{repos: ts.Repos}
	got, err := r.ResolveRef(ts.ctx, "claim:"+claimA)
	require.NoError(t, err)
	require.Equal(t, wire.MemoryRef{Ref: "claim:" + claimA, Kind: "claim", Text: "Works at Acme",
		Source: "chat “I work at Acme”", ThreadID: th, Status: model.ClaimCandidate}, got)
	got, err = r.ResolveRef(ts.ctx, "msg:"+stored[1].ID)
	require.NoError(t, err)
	require.Equal(t, wire.MemoryRef{Ref: "msg:" + stored[1].ID, Kind: "message", Text: "Noted.",
		Source: "chat “I work at Acme”", ThreadID: th}, got)
	got, err = r.ResolveRef(ts.ctx, "ep:"+claimGone)
	require.NoError(t, err)
	require.True(t, got.Gone)
	got, err = r.ResolveRef(ts.ctx, "nonsense")
	require.NoError(t, err)
	require.True(t, got.Gone)

	learned, err := r.Learned(ts.ctx, []string{stored[0].ID, stored[1].ID})
	require.NoError(t, err)
	require.Len(t, learned, 1)
	require.Equal(t, []wire.MemoryRef{{Ref: "claim:" + claimA, Kind: "claim", Text: "Works at Acme",
		Source: "this message", Status: model.ClaimCandidate}}, learned[stored[0].ID], "a rejected claim is not shown")

	// Deleting the chat forgets what was learned from it.
	require.NoError(t, ts.Repos.Chats.Delete(ts.ctx, th))
	got, err = r.ResolveRef(ts.ctx, "claim:"+claimA)
	require.NoError(t, err)
	require.Equal(t, wire.MemoryRef{Ref: "claim:" + claimA, Kind: "claim", Gone: true}, got)
}
