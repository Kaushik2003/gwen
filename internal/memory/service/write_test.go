package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/memory/files"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// learnFromChat makes one claim from a user message and projects it.
func (f *fx) learnFromChat() (model.Claim, model.ChatMessage) {
	f.t.Helper()
	f.m.facts = []llm.ExtractedFact{fact("user.employer", "Acme", "Works at Acme", "I work at Acme")}
	_, user, _ := f.turn("", "Remember that I work at Acme.", "Noted.")
	f.s.Tick(f.ctx)
	return f.only("user.employer"), user
}

// TestForgetReceipt erases a claim in the store and every lane, and says
// that the chat it came from still holds its words.
func TestForgetReceipt(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	c, _ := f.learnFromChat()
	ref := memory.Ref(memory.RefClaim, c.ID)
	require.True(t, f.vec.has(ref))
	require.True(t, f.mem0.has(ref))
	before := len(f.changes())

	rc, err := f.s.Forget(f.ctx, c.ID)
	require.NoError(t, err)
	require.Equal(t, 1, rc.Claims)
	require.Equal(t, []memory.LaneReceipt{
		{Lane: laneGwen, OK: true, Note: chatStillThere},
		{Lane: memory.LaneVec, OK: true},
		{Lane: memory.LaneMem0, OK: true, Note: mem0History},
		{Lane: memory.LaneFiles, OK: true},
	}, rc.Lanes)
	_, err = f.r.Memory.Claim(f.ctx, c.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.False(t, f.vec.has(ref))
	require.False(t, f.mem0.has(ref))
	require.Contains(t, f.mem0.deletes(), "m-"+c.ID, "Mem0 is asked by its own id")
	b, err := f.r.MemoryState.Backend(f.ctx, "mem0", c.ID)
	require.NoError(t, err)
	require.Equal(t, model.BackendDeleted, b.Status)
	require.NotContains(t, f.folder.last(), "Works at Acme")
	require.Equal(t, 1, f.folder.forgot, "git history keeps no forgotten words")
	evs := f.changes()
	require.Greater(t, len(evs), before)
	require.Equal(t, []string{c.ID}, evs[len(evs)-1].ClaimIDs)

	// A lane that fails says so; the claim is gone from Gwen's store anyway.
	d := f.remember("I like jazz")
	f.vec.set(func(l *fakeLane) { l.deleteErr = errors.New("index locked") })
	rc, err = f.s.Forget(f.ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, memory.LaneReceipt{Lane: laneGwen, OK: true}, rc.Lanes[0], "a note's words are gone with it")
	require.Equal(t, memory.LaneReceipt{Lane: memory.LaneVec, Note: "index locked"}, rc.Lanes[1])

	_, err = f.s.Forget(f.ctx, d.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestForgetAll erases everything in the store and the lanes; chats stay.
func TestForgetAll(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	f.learnFromChat()
	f.remember("I like jazz")
	path := filepath.Join(t.TempDir(), "lease.txt")
	require.NoError(t, os.WriteFile(path, []byte("The rent is 900 euros a month."), 0o600))
	res, err := f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: path})
	require.NoError(t, err)
	f.s.Tick(f.ctx)
	require.True(t, f.sm.has(res.ID))

	rc, err := f.s.ForgetAll(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 2, rc.Claims)
	require.Equal(t, 2, rc.Episodes, "the note and the document")
	lanes := make([]memory.LaneName, len(rc.Lanes))
	for i, l := range rc.Lanes {
		lanes[i] = l.Lane
		require.True(t, l.OK, l.Note)
	}
	require.Equal(t, []memory.LaneName{laneGwen, memory.LaneVec, memory.LaneMem0, memory.LaneSupermemory,
		memory.LaneFiles}, lanes)
	require.Empty(t, f.claims())
	require.Equal(t, 1, f.vec.cleared)
	require.Len(t, f.mem0.deletes(), 2, "both claims")
	require.Equal(t, []string{res.ID}, f.sm.deletes())
	require.Empty(t, f.folder.last())
	require.Equal(t, 1, f.folder.forgot)
	threads, err := f.r.Chats.List(f.ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, threads, 1, "chats are kept")
}

// TestResources reads a document, sends it to Supermemory and the vector
// lane, and forgets it again.
func TestResources(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	dir := t.TempDir()
	path := filepath.Join(dir, "lease.md")
	require.NoError(t, os.WriteFile(path, []byte("# Lease\n\nThe rent is 900 euros a month."), 0o600))
	res, err := f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: path})
	require.NoError(t, err)
	require.Equal(t, "lease.md", res.Title)
	require.Equal(t, path, res.Path)
	require.NotNil(t, res.EpisodeID)
	ep, err := f.r.Memory.Episode(f.ctx, *res.EpisodeID)
	require.NoError(t, err)
	require.Equal(t, model.EpisodeResource, ep.Kind)
	require.Equal(t, model.SpeakerExternal, ep.Speaker)
	require.Equal(t, model.ChannelDoc, ep.Channel)
	require.Equal(t, path, ep.Source)
	require.Contains(t, ep.Body, "900 euros")

	f.s.Tick(f.ctx)
	got := f.sm.items[res.ID]
	require.Contains(t, got.Text, "900 euros")
	require.Equal(t, "lease.md", got.Meta["title"])
	require.Equal(t, "resource", got.Kind)
	epRef := memory.Ref(memory.RefEpisode, *res.EpisodeID)
	require.True(t, f.vec.has(epRef))
	list, err := f.s.Resources(f.ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	titled, err := f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: path, Title: "My lease"})
	require.NoError(t, err)
	require.Equal(t, "My lease", titled.Title)

	rc, err := f.s.RemoveResource(f.ctx, res.ID)
	require.NoError(t, err)
	require.Equal(t, 1, rc.Episodes)
	require.Equal(t, []memory.LaneReceipt{{Lane: laneGwen, OK: true}, {Lane: memory.LaneSupermemory, OK: true},
		{Lane: memory.LaneVec, OK: true}, {Lane: memory.LaneFiles, OK: true}}, rc.Lanes)
	require.False(t, f.sm.has(res.ID))
	require.False(t, f.vec.has(epRef))
	_, err = f.r.Memory.Episode(f.ctx, *res.EpisodeID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.s.RemoveResource(f.ctx, res.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: filepath.Join(dir, "missing.txt")})
	require.ErrorIs(t, err, store.ErrInvalid)
	bin := filepath.Join(dir, "photo.bin")
	require.NoError(t, os.WriteFile(bin, []byte{1, 2, 3}, 0o600))
	_, err = f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: bin})
	require.ErrorIs(t, err, store.ErrInvalid)
	_, err = f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: dir})
	require.ErrorIs(t, err, store.ErrInvalid)
}

// TestRebuild rebuilds each derived lane from Gwen's store.
func TestRebuild(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	c, user := f.learnFromChat()
	note := f.remember("I like jazz")
	f.s.Tick(f.ctx)
	path := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("Gym on Tuesdays."), 0o600))
	res, err := f.s.AddResource(f.ctx, memory.AddResourceRequest{Path: path})
	require.NoError(t, err)
	f.s.Tick(f.ctx)

	require.NoError(t, f.s.Rebuild(f.ctx, memory.LaneVec))
	require.Equal(t, 1, f.vec.cleared)
	f.s.Tick(f.ctx)
	for _, ref := range []string{memory.Ref(memory.RefClaim, c.ID), memory.Ref(memory.RefClaim, note.ID),
		memory.Ref(memory.RefMessage, user.ID), memory.Ref(memory.RefEpisode, note.SourceID),
		memory.Ref(memory.RefEpisode, *res.EpisodeID)} {
		require.True(t, f.vec.has(ref), ref)
	}

	require.NoError(t, f.s.Rebuild(f.ctx, memory.LaneMem0))
	require.ElementsMatch(t, []string{"m-" + c.ID, "m-" + note.ID}, f.mem0.deletes())
	require.True(t, f.mem0.has(memory.Ref(memory.RefClaim, c.ID)))
	require.True(t, f.mem0.has(memory.Ref(memory.RefClaim, note.ID)))

	n := len(f.folder.projected)
	require.NoError(t, f.s.Rebuild(f.ctx, memory.LaneFiles))
	require.Len(t, f.folder.projected, n+1)

	f.sm.set(func(l *fakeLane) { l.items = map[string]memory.Record{} })
	require.NoError(t, f.s.Rebuild(f.ctx, memory.LaneSupermemory))
	require.True(t, f.sm.has(res.ID))

	f.setMemory(func(m *config.Memory) { m.EmbedEndpoint = "" })
	require.ErrorIs(t, f.s.Rebuild(f.ctx, memory.LaneVec), store.ErrInvalid)
	require.ErrorIs(t, f.s.Rebuild(f.ctx, memory.LaneClaims), store.ErrInvalid)
}

// TestProjectDropsHidden takes a claim out of the lanes once it is hidden
// or sensitive, and Mem0 never sees a sensitive claim.
func TestProjectDropsHidden(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	c, _ := f.learnFromChat()
	ref := memory.Ref(memory.RefClaim, c.ID)
	_, err := f.s.Suppress(f.ctx, c.ID, true)
	require.NoError(t, err)
	f.s.Tick(f.ctx)
	require.False(t, f.vec.has(ref))
	require.False(t, f.mem0.has(ref))
	_, err = f.s.Suppress(f.ctx, c.ID, false)
	require.NoError(t, err)
	f.s.Tick(f.ctx)
	require.True(t, f.vec.has(ref))
	require.True(t, f.mem0.has(ref))

	f.m.facts = []llm.ExtractedFact{{Key: "user.health", Subject: memory.SubjectUser, Value: jsonString("asthma"),
		Text: "Has asthma", Quote: "I have asthma", Sensitivity: model.SensitivitySensitive, Confidence: 0.9}}
	f.turn("", "Remember that I have asthma.", "Noted.")
	f.s.Tick(f.ctx)
	h := f.only("user.health")
	require.Equal(t, model.SensitivitySensitive, h.Sensitivity)
	href := memory.Ref(memory.RefClaim, h.ID)
	require.True(t, f.vec.has(href), "the vector lane decides about sensitive records itself")
	require.False(t, f.mem0.has(href), "a sensitive claim never goes to Mem0")
	require.NotContains(t, f.folder.last(), "Has asthma")
}

// TestReconcileMem0 drops Mem0 copies of claims no longer shown and queues
// the active ones it lacks.
func TestReconcileMem0(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	c, _ := f.learnFromChat()
	other := f.remember("I like jazz")
	f.s.Tick(f.ctx)
	// Behind the service's back: one claim rejected without its job, and one
	// copy lost.
	_, err := f.s.learner.Reject(f.ctx, c.ID)
	require.NoError(t, err)
	f.mem0.set(func(l *fakeLane) { delete(l.items, memory.Ref(memory.RefClaim, other.ID)) })
	now := f.clk.Now()
	require.NoError(t, f.r.MemoryState.SetBackend(f.ctx, model.MemoryBackend{Backend: "mem0", ItemID: other.ID,
		Status: model.BackendDeleted, UpdatedAt: now}))
	_, err = f.db.SQL().ExecContext(f.ctx, `DELETE FROM memory_jobs`)
	require.NoError(t, err)

	n, err := f.s.reconcile(f.ctx, f.s.lanesNow())
	require.NoError(t, err)
	require.Positive(t, n)
	require.Contains(t, f.mem0.deletes(), "m-"+c.ID)
	f.s.Tick(f.ctx)
	require.True(t, f.mem0.has(memory.Ref(memory.RefClaim, other.ID)), "the lost copy is stored again")
	require.False(t, f.vec.has(memory.Ref(memory.RefClaim, c.ID)))
}

// TestFolderChanges applies what the user changed in the memory folder: an
// edit corrects, a deleted line hides, and an added line is remembered.
func TestFolderChanges(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.allLanes()
	f.noModel = true
	a := f.remember("I like jazz")
	b := f.remember("I like cycling")
	f.s.Tick(f.ctx)
	f.folder.mu.Lock()
	f.folder.changes = []files.Change{
		{Kind: files.ChangeEdit, ClaimID: a.ID, Text: "I love jazz", Path: files.ProfileFile, Section: "About you"},
		{Kind: files.ChangeRemove, ClaimID: b.ID, Text: "I like cycling", Path: files.ProfileFile},
		{Kind: files.ChangeAdd, Text: "Globex", Path: files.ProfileFile, Section: "Works at"},
		{Kind: files.ChangeAdd, Text: "Loves hiking", Path: "people/anna.md", Subject: "Anna"},
		{Kind: files.ChangeEdit, ClaimID: "00000000-0000-4000-8000-00000000dead", Text: "gone", Path: files.ProfileFile},
	}
	f.folder.mu.Unlock()
	require.NoError(t, f.r.MemoryState.Enqueue(f.ctx, store.NewJob{Kind: model.JobFiles}))
	f.s.Tick(f.ctx)

	old, err := f.r.Memory.Claim(f.ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, model.ClaimSuperseded, old.Status)
	hidden, err := f.r.Memory.Claim(f.ctx, b.ID)
	require.NoError(t, err)
	require.True(t, hidden.Suppressed)
	var texts []string
	for _, c := range f.claims(model.ClaimActive) {
		texts = append(texts, c.Text)
	}
	require.ElementsMatch(t, []string{"I love jazz", "I like cycling", "Works at Globex", "About Anna: Loves hiking"},
		texts)
	last := f.folder.last()
	require.Contains(t, last, "Works at Globex", "the folder is written after the changes")
	require.Equal(t, 0, f.jobs()[model.JobFailed], "a change that cannot apply is skipped")
	require.True(t, slices.ContainsFunc(f.changes(), func(c wire.MemoryChanged) bool {
		return slices.Contains(c.ClaimIDs, b.ID)
	}))

	f.folder.mu.Lock()
	f.folder.scanErr = errors.New("permission denied")
	f.folder.mu.Unlock()
	require.NoError(t, f.r.MemoryState.Enqueue(f.ctx, store.NewJob{Kind: model.JobFiles}))
	f.s.Tick(f.ctx)
	fs := laneOf(f.s.Status(f.ctx), memory.LaneFiles)
	require.False(t, fs.OK)
	require.Equal(t, "permission denied", fs.Detail)
}

// TestAddedLine words a line the user added to the folder.
func TestAddedLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		ch   files.Change
		want string
	}{
		{files.Change{Text: "Globex", Path: files.ProfileFile, Section: "Works at"}, "Works at Globex"},
		{files.Change{Text: "Works at Globex", Path: files.ProfileFile, Section: "Works at"}, "Works at Globex"},
		{files.Change{Text: "Loves hiking", Path: "people/anna.md", Subject: "Anna"}, "About Anna: Loves hiking"},
		{files.Change{Text: "Anna loves hiking", Path: "people/anna.md", Subject: "Anna"}, "Anna loves hiking"},
		{files.Change{Text: " I take the bus ", Path: files.ProfileFile}, "I take the bus"},
	} {
		require.Equal(t, c.want, addedLine(c.ch))
	}
}

// TestResolver resolves refs for chats, with a claim's status, and lists
// what each message taught.
func TestResolver(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	c, user := f.learnFromChat()
	ref := memory.Ref(memory.RefClaim, c.ID)
	got, err := f.s.ResolveRef(f.ctx, ref)
	require.NoError(t, err)
	require.Equal(t, "Works at Acme", got.Text)
	require.Equal(t, model.ClaimActive, got.Status)
	require.NotEmpty(t, got.ThreadID)
	require.False(t, got.Gone)

	msg, err := f.s.ResolveRef(f.ctx, memory.Ref(memory.RefMessage, user.ID))
	require.NoError(t, err)
	require.Equal(t, "Remember that I work at Acme.", msg.Text)
	bad, err := f.s.ResolveRef(f.ctx, "claim:nope")
	require.NoError(t, err)
	require.True(t, bad.Gone)

	learned, err := f.s.Learned(f.ctx, []string{user.ID, "00000000-0000-4000-8000-000000000000"})
	require.NoError(t, err)
	require.Len(t, learned, 1)
	require.Equal(t, ref, learned[user.ID][0].Ref)
	require.Equal(t, model.ClaimActive, learned[user.ID][0].Status)

	_, err = f.s.Reject(f.ctx, c.ID)
	require.NoError(t, err)
	learned, err = f.s.Learned(f.ctx, []string{user.ID})
	require.NoError(t, err)
	require.Empty(t, learned, "a rejected claim is not shown as learned")
	got, err = f.s.ResolveRef(f.ctx, ref)
	require.NoError(t, err)
	require.Equal(t, model.ClaimRejected, got.Status)

	_, err = f.s.Forget(f.ctx, c.ID)
	require.NoError(t, err)
	got, err = f.s.ResolveRef(f.ctx, ref)
	require.NoError(t, err)
	require.True(t, got.Gone)
}

// TestWrites runs the user's changes through the learner and publishes
// each.
func TestWrites(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	c, _ := f.learnFromChat()
	n := len(f.changes())
	value := "Globex"
	edited, err := f.s.Edit(f.ctx, c.ID, memory.EditRequest{Value: &value})
	require.NoError(t, err)
	require.Equal(t, "Works at Globex", edited.Text)
	require.Len(t, f.changes(), n+1)
	require.Equal(t, []string{c.ID, edited.ID}, f.changes()[n].ClaimIDs)

	hidden, err := f.s.Suppress(f.ctx, edited.ID, true)
	require.NoError(t, err)
	require.True(t, hidden.Suppressed)
	views, err := f.s.Claims(f.ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive}, IncludeSuppressed: true})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "Works at", views[0].Label)
	exp, err := f.s.Explain(f.ctx, memory.Ref(memory.RefClaim, edited.ID))
	require.NoError(t, err)
	require.Equal(t, edited.ID, exp.Claim.ID)

	// A conflict, settled.
	f.m.contradict = func(req llm.ContradictRequest) (llm.ContradictOutput, error) {
		refs := map[string]string{}
		for _, c := range req.Claims {
			refs[c.Text] = c.Ref
		}
		return llm.ContradictOutput{Conflicts: []llm.Contradiction{{A: refs["Works at Globex"],
			B: refs["I work at Initech"], Reason: "not both"}}}, nil
	}
	_, err = f.s.Suppress(f.ctx, edited.ID, false)
	require.NoError(t, err)
	f.remember("I work at Initech")
	_, err = f.s.Sleep(f.ctx)
	require.NoError(t, err)
	cs, err := f.s.Conflicts(f.ctx, "")
	require.NoError(t, err)
	require.Len(t, cs, 1)
	cv, err := f.s.Resolve(f.ctx, cs[0].ID, model.ResolveKeepA)
	require.NoError(t, err)
	require.Equal(t, model.ConflictResolved, cv.Status)
	require.Equal(t, model.ClaimRejected, cv.B.Status)
	last := f.changes()[len(f.changes())-1]
	require.True(t, last.Conflicts)

	ents, err := f.s.Entities(f.ctx)
	require.NoError(t, err)
	require.Empty(t, ents)
	got, err := f.s.Get(f.ctx, "user.employer", nil, time.Time{})
	require.NoError(t, err)
	require.Equal(t, edited.ID, got.ID)
	p, err := f.s.Recall(f.ctx, memory.Query{Text: "where do I work"})
	require.NoError(t, err)
	require.NotEmpty(t, p.Hits)

	ex, err := f.s.Export(f.ctx)
	require.NoError(t, err)
	require.Len(t, ex.Claims, 3, "every status")
	require.Len(t, ex.Episodes, 2, "the edit and the note")
	require.Len(t, ex.Conflicts, 1)
	require.Equal(t, f.clk.Now().Truncate(time.Millisecond), ex.ExportedAt)
}

// TestPulled queues the lanes for rows another device wrote.
func TestPulled(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	claim, msg := "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"
	f.s.Pulled(f.ctx, store.Applied{Applied: 2, IDs: map[string][]string{"claims": {claim}, "chat_messages": {msg},
		"tasks": {"x"}}})
	rows, err := f.db.SQL().QueryContext(f.ctx, `SELECT kind, subject FROM memory_jobs ORDER BY kind, subject`)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var k, s string
		require.NoError(t, rows.Scan(&k, &s))
		got = append(got, k+" "+s)
	}
	require.Equal(t, []string{"files ", "profile 2026-10-01", "project claim:" + claim, "project msg:" + msg}, got)
}
