package store_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

// thread starts a chat with one message from the user and its reply.
func (f *fx) thread(text string) (model.ChatThread, []model.ChatMessage) {
	f.t.Helper()
	th, msgs, err := f.r.Chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, text, at("09:00")),
		say(model.ChatAssistant, "Noted.", at("09:01")),
	})
	require.NoError(f.t, err)
	return th, msgs
}

func TestObservations(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state := f.r.MemoryState
	th, msgs := f.thread("Plan my week")
	_, err := state.Observation(f.ctx, th.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	o := model.ChatObservation{ThreadID: th.ID, ThroughID: msgs[0].ID, ThroughAt: at("09:00"),
		Log: "- 2026-09-15 [high] Asked to plan the week"}
	require.NoError(t, state.SetObservation(f.ctx, o))
	got, err := state.Observation(f.ctx, th.ID)
	require.NoError(t, err)
	o.UpdatedAt = fxNow
	require.Equal(t, o, got)

	f.clk.Advance(time.Minute)
	o.ThroughID, o.ThroughAt, o.Log, o.Reflections = msgs[1].ID, at("09:01"), "- 2026-09-15 [high] Condensed", 1
	require.NoError(t, state.SetObservation(f.ctx, o))
	got, err = state.Observation(f.ctx, th.ID)
	require.NoError(t, err)
	o.UpdatedAt = fxNow.Add(time.Minute)
	require.Equal(t, o, got, "replaced")

	o.ThreadID = model.NewID()
	require.ErrorIs(t, state.SetObservation(f.ctx, o), store.ErrNotFound)
	gone, goneMsgs := f.thread("Delete this")
	require.NoError(t, f.r.Chats.Delete(f.ctx, gone.ID))
	err = state.SetObservation(f.ctx, model.ChatObservation{ThreadID: gone.ID, ThroughID: goneMsgs[0].ID,
		ThroughAt: at("09:00"), Log: "- 2026-09-15 [high] Delete this"})
	require.ErrorIs(t, err, store.ErrNotFound, "a deleted chat gets no log back")
}

func TestNotes(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state := f.r.MemoryState
	th, msgs := f.thread("Where do I work?")
	other, otherMsgs := f.thread("Gym?")

	notes, err := state.Notes(f.ctx, th.ID)
	require.NoError(t, err)
	require.Empty(t, notes)
	n := model.ChatNote{MessageID: msgs[0].ID, ThreadID: th.ID, Note: "[m1] Works at Acme",
		Refs: map[string]string{"m1": "claim:" + model.NewID()}}
	require.NoError(t, state.AddNote(f.ctx, n))
	require.NoError(t, state.AddNote(f.ctx, model.ChatNote{MessageID: otherMsgs[0].ID, ThreadID: other.ID,
		Note: "Nothing in memory answers this."}))
	notes, err = state.Notes(f.ctx, th.ID)
	require.NoError(t, err)
	n.CreatedAt = fxNow
	require.Equal(t, map[string]model.ChatNote{msgs[0].ID: n}, notes)
	notes, err = state.Notes(f.ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, map[string]string{}, notes[otherMsgs[0].ID].Refs, "no refs is an empty map")

	require.ErrorIs(t, state.AddNote(f.ctx, n), store.ErrConflict, "a message has one note")
	n.MessageID, n.ThreadID = model.NewID(), model.NewID()
	require.ErrorIs(t, state.AddNote(f.ctx, n), store.ErrNotFound)
}

func TestProfileCards(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state := f.r.MemoryState
	_, err := state.LatestProfile(f.ctx)
	require.ErrorIs(t, err, store.ErrNotFound)
	for _, day := range []string{"", "2026-9-20", "2026-02-30", "today"} {
		_, err := state.AddProfile(f.ctx, model.ProfileCard{Day: day, Text: "x"})
		userErr(t, err, store.ErrInvalid, "day")
	}

	var last model.ProfileCard
	for i := range 32 {
		f.clk.Advance(time.Minute)
		c, err := state.AddProfile(f.ctx, model.ProfileCard{Day: "2026-09-20", Text: "card " + strconv.Itoa(i),
			Refs: map[string]string{"p1": "claim:" + model.NewID()}})
		require.NoError(t, err)
		require.Len(t, c.ID, 36)
		require.Equal(t, fxNow.Add(time.Duration(i+1)*time.Minute), c.CreatedAt)
		last = c
	}
	got, err := state.LatestProfile(f.ctx)
	require.NoError(t, err)
	require.Equal(t, last, got)
	require.Equal(t, 30, f.count(`SELECT count(*) FROM profile_cards`), "the newest 30 are kept")
	require.Zero(t, f.count(`SELECT count(*) FROM profile_cards WHERE text IN ('card 0', 'card 1')`))

	empty, err := state.AddProfile(f.ctx, model.ProfileCard{Day: "2026-09-21"})
	require.NoError(t, err)
	got, err = state.LatestProfile(f.ctx)
	require.NoError(t, err)
	require.Equal(t, empty.ID, got.ID, "the newest is the latest, even on the same instant")
	require.Equal(t, map[string]string{}, got.Refs)
	require.Equal(t, "", got.Text)
}

// job reads a job's row.
func (f *fx) job(id int64) model.MemoryJob {
	f.t.Helper()
	var (
		j                         model.MemoryJob
		payload                   string
		runAfter, created, update int64
	)
	require.NoError(f.t, f.db.SQL().QueryRowContext(f.ctx, `SELECT id, kind, subject, payload, state, attempts,
		run_after, last_error, created_at, updated_at FROM memory_jobs WHERE id = ?`, id).Scan(&j.ID, &j.Kind,
		&j.Subject, &payload, &j.State, &j.Attempts, &runAfter, &j.LastError, &created, &update))
	j.Payload = []byte(payload)
	j.RunAfter, j.CreatedAt, j.UpdatedAt = time.UnixMilli(runAfter).UTC(), time.UnixMilli(created).UTC(),
		time.UnixMilli(update).UTC()
	return j
}

func TestJobs(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state := f.r.MemoryState
	require.NoError(t, state.Enqueue(f.ctx))
	_, err := state.NextJob(f.ctx, fxNow)
	require.ErrorIs(t, err, store.ErrNotFound, "none queued")

	claim := "claim:" + model.NewID()
	require.NoError(t, state.Enqueue(f.ctx,
		store.NewJob{Kind: model.JobExtract},
		store.NewJob{Kind: model.JobProject, Subject: claim, Payload: map[string]string{"why": "new"},
			RunAfter: fxNow.Add(-time.Hour)},
		store.NewJob{Kind: model.JobFiles, RunAfter: fxNow.Add(time.Hour)},
	))
	counts, err := state.JobCounts(f.ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]int{model.JobPending: 3}, counts)

	f.clk.Advance(time.Second)
	project, err := state.NextJob(f.ctx, fxNow)
	require.NoError(t, err)
	require.Equal(t, model.JobProject, project.Kind, "the job due first goes first")
	require.Equal(t, claim, project.Subject)
	require.JSONEq(t, `{"why": "new"}`, string(project.Payload))
	require.Equal(t, model.JobRunning, project.State)
	require.Equal(t, 1, project.Attempts)
	require.Equal(t, fxNow.Add(time.Second), project.UpdatedAt)
	require.Equal(t, project, f.job(project.ID), "returned as stored")

	_, err = state.NextJob(f.ctx, fxNow, model.JobFiles, model.JobObserve)
	require.ErrorIs(t, err, store.ErrNotFound, "the files job is not due yet")
	files, err := state.NextJob(f.ctx, fxNow.Add(2*time.Hour), model.JobFiles)
	require.NoError(t, err)
	require.Equal(t, model.JobFiles, files.Kind, "only the kinds asked for")

	require.NoError(t, state.Enqueue(f.ctx, store.NewJob{Kind: model.JobProject, Subject: claim}))
	counts, err = state.JobCounts(f.ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]int{model.JobPending: 1, model.JobRunning: 2}, counts,
		"a job of the same kind and subject is running, so none is added")

	require.NoError(t, state.FinishJob(f.ctx, project.ID))
	require.Equal(t, model.JobDone, f.job(project.ID).State)
	require.ErrorIs(t, state.FinishJob(f.ctx, project.ID), store.ErrConflict, "it is not running")
	require.ErrorIs(t, state.FinishJob(f.ctx, 9999), store.ErrNotFound)
	require.NoError(t, state.Enqueue(f.ctx, store.NewJob{Kind: model.JobProject, Subject: claim}))
	counts, err = state.JobCounts(f.ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]int{model.JobPending: 2, model.JobRunning: 1, model.JobDone: 1}, counts,
		"once it is done, the same kind and subject can be queued again")

	retry := fxNow.Add(10 * time.Minute)
	long := strings.Repeat("é", 1500)
	require.NoError(t, state.FailJob(f.ctx, files.ID, long, &retry))
	got := f.job(files.ID)
	require.Equal(t, model.JobPending, got.State)
	require.Equal(t, retry, got.RunAfter)
	require.Equal(t, strings.Repeat("é", 1000), got.LastError, "the error is cut to 1000 characters")
	require.ErrorIs(t, state.FailJob(f.ctx, files.ID, "again", nil), store.ErrConflict)

	extract, err := state.NextJob(f.ctx, fxNow, model.JobExtract)
	require.NoError(t, err)
	require.NoError(t, state.FailJob(f.ctx, extract.ID, "model unavailable", nil))
	got = f.job(extract.ID)
	require.Equal(t, model.JobFailed, got.State)
	require.Equal(t, "model unavailable", got.LastError)
	_, err = state.NextJob(f.ctx, fxNow, model.JobExtract)
	require.ErrorIs(t, err, store.ErrNotFound, "a failed job is not run again")

	again, err := state.NextJob(f.ctx, retry, model.JobFiles)
	require.NoError(t, err)
	require.Equal(t, files.ID, again.ID)
	require.Equal(t, 2, again.Attempts)
	n, err := state.RequeueRunning(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, model.JobPending, f.job(files.ID).State)

	n, err = state.PruneJobs(f.ctx, fxNow)
	require.NoError(t, err)
	require.Zero(t, n, "done after before is kept")
	n, err = state.PruneJobs(f.ctx, fxNow.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, n)
	counts, err = state.JobCounts(f.ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]int{model.JobPending: 2, model.JobFailed: 1}, counts)
}

func TestBackends(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state := f.r.MemoryState
	a, b := model.NewID(), model.NewID()
	_, err := state.Backend(f.ctx, "mem0", a)
	require.ErrorIs(t, err, store.ErrNotFound)

	synced := fxNow.Add(-time.Hour)
	ba := model.MemoryBackend{Backend: "mem0", ItemID: a, ExternalID: "m-1", Status: model.BackendSynced,
		ContentHash: "h1", SyncedAt: &synced}
	require.NoError(t, state.SetBackend(f.ctx, ba))
	require.NoError(t, state.SetBackend(f.ctx, model.MemoryBackend{Backend: "mem0", ItemID: b,
		Status: model.BackendPending}))
	require.NoError(t, state.SetBackend(f.ctx, model.MemoryBackend{Backend: "supermemory", ItemID: a,
		ExternalID: "s-1", Status: model.BackendFailed, Error: "HTTP 500"}))
	got, err := state.Backend(f.ctx, "mem0", a)
	require.NoError(t, err)
	ba.UpdatedAt = fxNow
	require.Equal(t, ba, got)

	f.clk.Advance(time.Minute)
	ba.Status, ba.ExternalID, ba.SyncedAt = model.BackendDeleted, "m-2", nil
	require.NoError(t, state.SetBackend(f.ctx, ba))
	got, err = state.Backend(f.ctx, "mem0", a)
	require.NoError(t, err)
	ba.UpdatedAt = fxNow.Add(time.Minute)
	require.Equal(t, ba, got, "replaced")

	all, err := state.Backends(f.ctx, "mem0", "")
	require.NoError(t, err)
	require.Len(t, all, 2)
	pending, err := state.Backends(f.ctx, "mem0", model.BackendPending)
	require.NoError(t, err)
	require.Equal(t, []string{b}, ids(pending, func(b model.MemoryBackend) string { return b.ItemID }))

	got, err = state.BackendByExternal(f.ctx, "mem0", "m-2")
	require.NoError(t, err)
	require.Equal(t, a, got.ItemID)
	_, err = state.BackendByExternal(f.ctx, "mem0", "s-1")
	require.ErrorIs(t, err, store.ErrNotFound, "external ids are per lane")
	_, err = state.BackendByExternal(f.ctx, "mem0", "")
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, state.SetBackend(f.ctx, model.MemoryBackend{Backend: "mem0", ItemID: a, Status: "gone"}),
		store.ErrInvalid)
}

func TestClaimUsage(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state := f.r.MemoryState
	a, b, c := model.NewID(), model.NewID(), model.NewID()
	usage, err := state.Usage(f.ctx, nil)
	require.NoError(t, err)
	require.Empty(t, usage)

	require.NoError(t, state.TouchClaims(f.ctx, nil, fxNow))
	require.NoError(t, state.TouchClaims(f.ctx, []string{a, a, b}, at("09:00")))
	require.NoError(t, state.TouchClaims(f.ctx, []string{a}, at("10:00")))
	require.NoError(t, state.TouchClaims(f.ctx, []string{a}, at("08:00")))
	usage, err = state.Usage(f.ctx, []string{a, b, c})
	require.NoError(t, err)
	require.Equal(t, map[string]store.ClaimUsage{
		a: {Uses: 3, LastUsedAt: at("10:00")},
		b: {Uses: 1, LastUsedAt: at("09:00")},
	}, usage, "one use per call; the last use is the latest; a claim never used is absent")

	many := make([]string, 1200)
	for i := range many {
		many[i] = model.NewID()
	}
	many[1100] = b
	usage, err = state.Usage(f.ctx, many)
	require.NoError(t, err)
	require.Equal(t, map[string]store.ClaimUsage{b: {Uses: 1, LastUsedAt: at("09:00")}}, usage)
}

func TestResources(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	state, mem := f.r.MemoryState, f.r.Memory
	docs, err := state.Resources(f.ctx)
	require.NoError(t, err)
	require.Empty(t, docs)

	text := model.Episode{Channel: model.ChannelDoc, Speaker: model.SpeakerExternal,
		Body: "The tenant pays rent to Mrs Featherstonehaugh on the first", At: at("08:00"),
		Sensitivity: model.SensitivityPersonal}
	signed := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	lease, err := state.AddResource(f.ctx, model.Resource{Title: "Lease Thistlewood.pdf",
		Path: "/home/me/Thistlewood.pdf", Mime: "application/pdf", Size: 2048, DocumentAt: &signed}, text)
	require.NoError(t, err)
	require.Len(t, lease.ID, 36)
	require.NotNil(t, lease.EpisodeID)
	require.Equal(t, fxNow, lease.AddedAt)
	require.Equal(t, sha(text.Body), lease.ContentHash, "the text's hash when none is given")
	ep, err := mem.Episode(f.ctx, *lease.EpisodeID)
	require.NoError(t, err)
	require.Equal(t, model.EpisodeResource, ep.Kind)
	require.Equal(t, "Lease Thistlewood.pdf", ep.Title, "titled as the document")
	require.Equal(t, lease.ID, ep.Source)
	got, err := state.Resource(f.ctx, lease.ID)
	require.NoError(t, err)
	require.Equal(t, lease, got, "returned as stored")

	f.clk.Advance(time.Minute)
	notes, err := state.AddResource(f.ctx, model.Resource{Title: "Notes", Path: "/home/me/notes.md", Size: 5,
		ContentHash: "file-hash"}, model.Episode{Kind: model.EpisodeNote, Title: "My notes", Source: "/home/me/notes.md",
		Channel: model.ChannelDoc, Speaker: model.SpeakerUser, Body: "Bring snacks", At: at("08:00"),
		Sensitivity: model.SensitivityPublic})
	require.NoError(t, err)
	require.Equal(t, "file-hash", notes.ContentHash)
	ep, err = mem.Episode(f.ctx, *notes.EpisodeID)
	require.NoError(t, err)
	require.Equal(t, model.EpisodeResource, ep.Kind, "a resource's text is always a resource episode")
	require.Equal(t, "My notes", ep.Title)
	require.Equal(t, "/home/me/notes.md", ep.Source)
	docs, err = state.Resources(f.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{notes.ID, lease.ID}, ids(docs, func(r model.Resource) string { return r.ID }),
		"newest first")

	hits, err := mem.SearchEpisodes(f.ctx, "who gets the rent?", 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, *lease.EpisodeID, hits[0].ID)

	_, err = state.AddResource(f.ctx, model.Resource{Title: "Broken", Path: "/x", Size: 1},
		model.Episode{Channel: model.ChannelDoc, Body: "no speaker", At: at("08:00"),
			Sensitivity: model.SensitivityPublic})
	userErr(t, err, store.ErrInvalid, "speaker")
	_, err = state.AddResource(f.ctx, model.Resource{Title: "Negative", Path: "/x", Size: -1}, text)
	require.ErrorIs(t, err, store.ErrInvalid)
	require.Equal(t, 2, f.count(`SELECT count(*) FROM resources`), "a failed add adds nothing")
	require.Equal(t, 2, f.count(`SELECT count(*) FROM episodes`))

	require.NoError(t, state.RemoveResource(f.ctx, lease.ID, store.LedgerItem{}))
	_, err = state.Resource(f.ctx, lease.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, state.RemoveResource(f.ctx, lease.ID, store.LedgerItem{}), store.ErrNotFound)
	docs, err = state.Resources(f.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{notes.ID}, ids(docs, func(r model.Resource) string { return r.ID }))
	_, err = mem.Episode(f.ctx, *lease.EpisodeID)
	require.ErrorIs(t, err, store.ErrNotFound, "its text is erased")
	hits, err = mem.SearchEpisodes(f.ctx, "rent", 10)
	require.NoError(t, err)
	require.Empty(t, hits)
	require.Equal(t, 1, f.count(`SELECT count(*) FROM resources WHERE id = ? AND title = '' AND path = ''
		AND deleted_at IS NOT NULL`, lease.ID))
	for _, word := range []string{"Featherstonehaugh", "Thistlewood"} {
		require.False(t, f.dbFileHas(word), word)
	}
	require.True(t, f.dbFileHas("Bring snacks"))
	ledger := f.ledger()
	require.Len(t, ledger, 1)
	require.JSONEq(t, `{"action": "resource.remove", "resource": "`+lease.ID+`", "hash": "`+lease.ContentHash+`"}`,
		string(ledger[0].Data))
}
