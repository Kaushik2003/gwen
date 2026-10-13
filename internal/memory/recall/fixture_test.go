package recall

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// fxNow is a Tuesday afternoon.
var fxNow = time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)

type fx struct {
	t      *testing.T
	ctx    context.Context
	clk    *clock.Fake
	r      store.Repos
	router *Router
}

func newFx(t *testing.T) *fx {
	t.Helper()
	clk := clock.NewFake(fxNow)
	db := testutil.NewDBWithClock(t, clk)
	repos := store.NewRepos(db)
	router := New(Options{Repo: repos.Memory, State: repos.MemoryState, Chats: repos.Chats, Clock: clk,
		Loc: time.UTC})
	patient(router)
	return &fx{t: t, ctx: context.Background(), clk: clk, r: repos, router: router}
}

// patient gives every lane time enough under the race detector; a lane the
// router does not know keeps the default.
func patient(r *Router) {
	for k := range r.deadlines {
		r.deadlines[k] = 20 * time.Second
	}
	r.wait = 30 * time.Second
}

func day(d int, hms ...string) time.Time {
	t := time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC)
	if len(hms) > 0 {
		h, err := time.Parse("15:04", hms[0])
		if err != nil {
			panic(err)
		}
		t = time.Date(2026, 10, d, h.Hour(), h.Minute(), 0, 0, time.UTC)
	}
	return t
}

// chat stores a thread whose turns alternate, the user's first; the first
// text is its title.
func (f *fx) chat(at time.Time, texts ...string) (model.ChatThread, []model.ChatMessage) {
	f.t.Helper()
	msgs := make([]model.ChatMessage, len(texts))
	for i, text := range texts {
		role := model.ChatUser
		if i%2 == 1 {
			role = model.ChatAssistant
		}
		msgs[i] = model.ChatMessage{Role: role, Text: text, At: at.Add(time.Duration(i) * time.Minute)}
	}
	thread, stored, err := f.r.Chats.Append(f.ctx, "", msgs)
	require.NoError(f.t, err)
	return thread, stored
}

// claim stores a claim, active and said by the user unless edit says
// otherwise.
func (f *fx) claim(key, text string, edit ...func(c *model.Claim)) model.Claim {
	f.t.Helper()
	value, err := json.Marshal(text)
	require.NoError(f.t, err)
	c := model.Claim{ID: model.NewID(), Key: key, Value: value, Text: text, Cardinality: model.CardinalityOne,
		Status: model.ClaimActive, Trust: model.TrustOwnerSaid, Sensitivity: model.SensitivityPersonal,
		Confidence: 0.9, SourceKind: model.SourceUserUI, Span: text, Channel: model.ChannelUI, Extractor: "test",
		RecordedAt: f.clk.Now()}
	for _, e := range edit {
		e(&c)
	}
	c.ContentHash = memory.ClaimHash(c.SubjectID, c.Key, c.Value)
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Claims: []model.Claim{c}}))
	got, err := f.r.Memory.Claim(f.ctx, c.ID)
	require.NoError(f.t, err)
	return got
}

func (f *fx) entity(kind, name string, aliases ...string) model.Entity {
	f.t.Helper()
	e := model.Entity{ID: model.NewID(), Kind: kind, Name: name, Aliases: aliases}
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Entities: []model.Entity{e}}))
	return e
}

func (f *fx) episode(e model.Episode) model.Episode {
	f.t.Helper()
	if e.Kind == "" {
		e.Kind = model.EpisodeNote
	}
	if e.Channel == "" {
		e.Channel = model.ChannelUI
	}
	if e.Speaker == "" {
		e.Speaker = model.SpeakerUser
	}
	if e.Sensitivity == "" {
		e.Sensitivity = model.SensitivityPersonal
	}
	got, err := f.r.Memory.AddEpisode(f.ctx, e)
	require.NoError(f.t, err)
	return got
}

func (f *fx) conflict(a, b model.Claim) model.Conflict {
	f.t.Helper()
	k := model.Conflict{ID: model.NewID(), ClaimA: a.ID, ClaimB: b.ID, Reason: "both cannot hold",
		Sensitivity: model.SensitivityPersonal, Status: model.ConflictOpen}
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Conflicts: []model.Conflict{k}}))
	return k
}

func (f *fx) update(u store.ClaimUpdate) {
	f.t.Helper()
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Updates: []store.ClaimUpdate{u}}))
}

// recall runs a query with time enough for every lane.
func (f *fx) recall(q memory.Query) memory.Packet {
	f.t.Helper()
	if q.Budget.Deadline == 0 {
		q.Budget.Deadline = 30 * time.Second
	}
	p, err := f.router.Recall(f.ctx, q)
	require.NoError(f.t, err)
	return p
}

func hitRefs(hits []memory.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Ref
	}
	return out
}

func candRefs(cs []cand) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.hit.Ref
	}
	return out
}

func claimRef(c model.Claim) string { return memory.Ref(memory.RefClaim, c.ID) }

func msgRef(m model.ChatMessage) string { return memory.Ref(memory.RefMessage, m.ID) }

func epRef(e model.Episode) string { return memory.Ref(memory.RefEpisode, e.ID) }

func tp(t time.Time) *time.Time { return &t }

// fakeLane is an extra lane with fixed hits, or one that fails or hangs.
type fakeLane struct {
	name memory.LaneName
	hits []memory.Hit
	err  error
	hang bool

	mu    sync.Mutex
	asked []memory.Query
}

func (l *fakeLane) Name() memory.LaneName { return l.name }

func (l *fakeLane) Search(ctx context.Context, q memory.Query) ([]memory.Hit, error) {
	l.mu.Lock()
	l.asked = append(l.asked, q)
	l.mu.Unlock()
	if l.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return l.hits, l.err
}

// fakeData is a data lane with fixed hits; it keeps the window it was asked
// for.
type fakeData struct {
	hits   []memory.Hit
	window *memory.TimeRange
}

func (d *fakeData) Search(_ context.Context, _ memory.Query, w *memory.TimeRange) ([]memory.Hit, error) {
	d.window = w
	return d.hits, nil
}
