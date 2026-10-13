package recall

import (
	"errors"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

func (f *fx) lane(text string, run func(s query) ([]cand, error)) []string {
	f.t.Helper()
	cs, err := run(f.router.settle(memory.Query{Text: text}))
	require.NoError(f.t, err)
	return candRefs(cs)
}

func (f *fx) ents(s query) func() ([]model.Entity, error) {
	return func() ([]model.Entity, error) { return f.router.namedEntities(f.ctx, s.Text) }
}

func TestClaimsLane(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	claims := func(s query) ([]cand, error) { return f.router.claimsLane(f.ctx, s, f.ents(s)) }

	night := f.claim("pref.work", "Likes to work at night", func(c *model.Claim) { c.Cardinality = model.CardinalityMany })
	employer := f.claim("user.employer", "Employed by Acme Corp")
	role := f.claim("user.role", "A nurse on the night shift")
	home := f.claim("user.home", "Lives in Porto")
	require.Equal(t, []string{claimRef(employer), claimRef(role), claimRef(night)}, f.lane("Where do I work?", claims),
		"the key hits come first, then the claims that share words")
	require.Equal(t, []string{claimRef(home)}, f.lane("which city is home", claims))

	anna := f.entity(memory.EntityPerson, "Anna", "Annie")
	hers := f.claim("person.birthday", "Anna's birthday is 3 May", func(c *model.Claim) { c.SubjectID = &anna.ID })
	mine := f.claim("user.birthday", "Birthday is 9 June")
	got := f.lane("When is Annie's birthday?", claims)
	require.Equal(t, claimRef(hers), got[0], "a person named: their birthday")
	got = f.lane("when is my birthday", claims)
	require.Equal(t, claimRef(mine), got[0], "no one named: the user's")

	s := f.router.settle(memory.Query{Text: "work night", Budget: memory.Budget{Candidates: 2}})
	cs, err := claims(s)
	require.NoError(t, err)
	require.Len(t, cs, 2, "at most the candidates asked")
}

func TestChatsAndEpisodesLanes(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, msgs := f.chat(day(10), "Dentist on Friday", "Noted, the dentist on Friday.")
	f.chat(day(11), "Weekend plans", "Sounds fun")
	ep := f.episode(model.Episode{Body: "My dentist is Dr Okafor on Elm Street", At: day(9)})
	f.episode(model.Episode{Body: "Gym on Tuesdays", At: day(9)})

	chats := func(s query) ([]cand, error) { return f.router.chatsLane(f.ctx, s) }
	require.ElementsMatch(t, []string{msgRef(msgs[0]), msgRef(msgs[1])}, f.lane("who is my dentist", chats))
	cs, err := chats(f.router.settle(memory.Query{Text: "who is my dentist", Exclude: []string{msgRef(msgs[0])}}))
	require.NoError(t, err)
	require.Equal(t, []string{msgRef(msgs[1])}, candRefs(cs), "excluded messages are left out")
	require.Equal(t, kindMessage, cs[0].hit.Kind)
	require.Equal(t, model.TrustInferred, cs[0].hit.Trust, "Gwen's own words")

	episodes := func(s query) ([]cand, error) { return f.router.episodesLane(f.ctx, s) }
	require.Equal(t, []string{epRef(ep)}, f.lane("who is my dentist", episodes))
	cs, err = episodes(f.router.settle(memory.Query{Text: "who is my dentist", Exclude: []string{epRef(ep)}}))
	require.NoError(t, err)
	require.Empty(t, cs)
}

func TestEntityLane(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	entity := func(s query) ([]cand, error) { return f.router.entityLane(f.ctx, s, f.ents(s)) }
	anna := f.entity(memory.EntityPerson, "Anna", "Annie")
	acme := f.entity(memory.EntityOrg, "Acme Corp")
	cello := f.claim("person.fact", "Anna plays the cello", func(c *model.Claim) { c.SubjectID = &anna.ID })
	porto := f.claim("person.home", "Anna lives in Porto", func(c *model.Claim) { c.SubjectID = &anna.ID })
	hq := f.claim("org.fact", "Acme Corp is based in Leeds", func(c *model.Claim) { c.SubjectID = &acme.ID })
	f.claim("user.home", "Lives in Lisbon")
	f.claim("person.fact", "Anna sings", func(c *model.Claim) { c.SubjectID = &anna.ID; c.Status = model.ClaimCandidate })

	require.ElementsMatch(t, []string{claimRef(cello), claimRef(porto)}, f.lane("what does annie like?", entity),
		"an alias names her; her candidate claims are not asked for")
	require.ElementsMatch(t, []string{claimRef(cello), claimRef(porto), claimRef(hq)},
		f.lane("Is Anna still at acme corp?", entity))
	require.Empty(t, f.lane("Annabel and Acme", entity), "names are whole words")
	require.Empty(t, f.lane("what's the weather", entity))
}

func TestTimeLane(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	timeLane := func(s query) ([]cand, error) { return f.router.timeLane(f.ctx, s) }
	_, morning := f.chat(day(12, "10:00"), "Booked the dentist for Friday", "Good")
	_, evening := f.chat(day(12, "18:00"), "Watched a film", "Nice")
	f.chat(day(9), "The dentist again", "ok")
	milk := f.episode(model.Episode{Body: "Bought milk", At: day(12, "12:00")})
	learned := f.claim("user.fact", "Has a dentist appointment on Friday", func(c *model.Claim) {
		c.Cardinality, c.RecordedAt = model.CardinalityMany, day(12, "11:00")
	})
	f.claim("user.fact", "Owns a bike", func(c *model.Claim) {
		c.Cardinality, c.RecordedAt = model.CardinalityMany, day(13, "09:00")
	})

	require.Equal(t, []string{msgRef(evening[1]), msgRef(evening[0]), epRef(milk), claimRef(learned),
		msgRef(morning[1]), msgRef(morning[0])}, f.lane("what did we talk about yesterday", timeLane),
		"no words beyond the time: newest first")
	require.Equal(t, []string{claimRef(learned), msgRef(morning[0]), msgRef(evening[1]), msgRef(evening[0]),
		epRef(milk), msgRef(morning[1])}, f.lane("what did I say about the dentist yesterday", timeLane),
		"with words: the most in common first, then newest")

	s := f.router.settle(memory.Query{Text: "yesterday", Budget: memory.Budget{Candidates: 2}})
	cs, err := timeLane(s)
	require.NoError(t, err)
	require.Len(t, cs, 2)

	// The lane runs only when a window is named.
	p := f.recall(memory.Query{Text: "what did we talk about", Lanes: []memory.LaneName{memory.LaneTime}})
	require.Empty(t, p.Hits)
	p = f.recall(memory.Query{Text: "anything from yesterday?", Lanes: []memory.LaneName{memory.LaneTime}})
	require.Equal(t, memory.IntentEpisodic, p.Intent)
	require.Len(t, p.Hits, 6)
	require.Equal(t, []memory.LaneName{memory.LaneTime}, p.Hits[0].Lanes)
	window := &memory.TimeRange{From: day(5, "00:00"), To: day(6, "00:00")}
	p = f.recall(memory.Query{Text: "anything from yesterday?", Window: window, Lanes: []memory.LaneName{memory.LaneTime}})
	require.Empty(t, p.Hits, "the query's window wins")
}

func TestDataLane(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	data := &fakeData{hits: []memory.Hit{{Ref: "data:week_hours", Text: "Worked 31 hours last week"}}}
	r := New(Options{Repo: f.r.Memory, State: f.r.MemoryState, Chats: f.r.Chats, Data: data, Clock: f.clk,
		Loc: time.UTC})
	patient(r)
	p, err := r.Recall(f.ctx, memory.Query{Text: "how long did I work last week",
		Budget: memory.Budget{Deadline: time.Minute}})
	require.NoError(t, err)
	require.Equal(t, memory.IntentData, p.Intent)
	require.Equal(t, []memory.Hit{{Ref: "data:week_hours", Kind: kindData, Text: "Worked 31 hours last week",
		Source: "your tracked data", Trust: model.TrustObserved, Score: 3.0 / 61,
		Lanes: []memory.LaneName{memory.LaneData}}}, p.Hits)
	require.Equal(t, &memory.TimeRange{From: day(5, "00:00"), To: day(12, "00:00")}, data.window)
	require.False(t, p.Abstain, "data questions do not abstain")

	data.window = nil
	_, err = r.Recall(f.ctx, memory.Query{Text: "what does the lease say", Budget: memory.Budget{Deadline: time.Minute}})
	require.NoError(t, err)
	require.Nil(t, data.window, "a lane of weight 0 is not asked")
}

func TestFuse(t *testing.T) {
	t.Parallel()
	c := func(ref string) cand {
		return cand{hit: memory.Hit{Ref: ref, Score: 99, Lanes: []memory.LaneName{"x"}}}
	}
	s := query{intent: memory.IntentLookup, exclude: map[string]bool{"claim:x": true}}
	lanes := []lane{{name: memory.LaneClaims}, {name: memory.LaneChats}, {name: memory.LaneVec}}
	fused := fuse(s, lanes, [][]cand{
		{c("claim:a"), c("claim:x"), c("claim:b"), c("claim:a")},
		{c("claim:b"), c("msg:c")},
		nil,
	})
	got := map[string]memory.Hit{}
	var order []string
	for _, f := range fused {
		got[f.hit.Ref] = f.hit
		order = append(order, f.hit.Ref)
	}
	require.Equal(t, []string{"claim:b", "claim:a", "msg:c"}, order, "best first")
	require.InDelta(t, 2.0/61, got["claim:a"].Score, 1e-12, "rank 1 in claims; the duplicate is ignored")
	require.InDelta(t, 2.0/62+0.6/61, got["claim:b"].Score, 1e-12, "rank 2 in claims (x is excluded), 1 in chats")
	require.InDelta(t, 0.6/62, got["msg:c"].Score, 1e-12)
	require.Equal(t, []memory.LaneName{memory.LaneClaims, memory.LaneChats}, got["claim:b"].Lanes)
	require.NotContains(t, got, "claim:x")
}

func TestDeadlines(t *testing.T) {
	t.Parallel()
	r := New(Options{})
	require.Equal(t, 150*time.Millisecond, r.deadline(memory.LaneClaims))
	require.Equal(t, 150*time.Millisecond, r.deadline(memory.LaneTime))
	require.Equal(t, 400*time.Millisecond, r.deadline(memory.LaneVec))
	require.Equal(t, 500*time.Millisecond, r.deadline(memory.LaneMem0))
	require.Equal(t, 500*time.Millisecond, r.deadline(memory.LaneSupermemory))
	require.Equal(t, 500*time.Millisecond, r.deadline("other"))
	s := r.settle(memory.Query{Text: "x"})
	require.Equal(t, 600*time.Millisecond, s.Budget.Deadline)
	require.Equal(t, 40, s.Budget.Candidates)
}

func TestDegradedLanes(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	home := f.claim("user.home", "Lives in Porto")
	slow := &fakeLane{name: "slow", hang: true}
	broken := &fakeLane{name: memory.LaneMem0, err: errors.New("mem0 is down")}
	f.router.SetLanes([]memory.Lane{slow, nil, broken})
	f.router.deadlines["slow"] = 50 * time.Millisecond

	start := time.Now()
	p := f.recall(memory.Query{Text: "where do I live"})
	require.Less(t, time.Since(start), 10*time.Second)
	require.Equal(t, []memory.LaneName{"slow", memory.LaneMem0}, p.Degraded)
	require.Equal(t, []string{claimRef(home)}, hitRefs(p.Hits), "the other lanes still answer")
	require.False(t, p.Abstain)

	// The query's deadline bounds every lane.
	delete(f.router.deadlines, "slow")
	start = time.Now()
	p = f.recall(memory.Query{Text: "where do I live", Lanes: []memory.LaneName{"slow"},
		Budget: memory.Budget{Deadline: 30 * time.Millisecond}})
	require.Less(t, time.Since(start), 400*time.Millisecond, "well before the lane's own 500 ms")
	require.Equal(t, []memory.LaneName{"slow"}, p.Degraded)
	require.True(t, p.Abstain, "nothing answered")

	// A lane of Gwen's own that ran out of time proves nothing, so she does not
	// say she doesn't remember.
	f.router.deadlines[memory.LaneClaims] = time.Nanosecond
	p = f.recall(memory.Query{Text: "where is my passport", Lanes: []memory.LaneName{memory.LaneClaims}})
	require.Contains(t, p.Degraded, memory.LaneClaims)
	require.False(t, p.Abstain)
	delete(f.router.deadlines, memory.LaneClaims)

	f.router.SetLanes(nil)
	p = f.recall(memory.Query{Text: "where do I live", Lanes: []memory.LaneName{"slow", memory.LaneMem0}})
	require.Empty(t, p.Degraded, "lanes replaced")
	require.Empty(t, p.Hits)
}

func TestExtraLanes(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	home := f.claim("user.home", "Lives in Porto")
	maybe := f.claim("user.pet", "Has a cat", func(c *model.Claim) { c.Status = model.ClaimCandidate })
	lease, err := f.r.MemoryState.AddResource(f.ctx, model.Resource{Title: "Lease.pdf", Path: "/docs/lease.pdf",
		Mime: "application/pdf", Size: 10}, model.Episode{Channel: model.ChannelDoc, Speaker: model.SpeakerExternal,
		Body: "The lease ends in May 2027. Pets are allowed.", At: day(1), Sensitivity: model.SensitivityPersonal})
	require.NoError(t, err)

	mem0 := &fakeLane{name: memory.LaneMem0, hits: []memory.Hit{
		{Ref: claimRef(home), Text: "lives in porto (Mem0's words)", Score: 0.9},
		{Ref: claimRef(maybe), Score: 0.8},
		{Ref: "claim:" + model.NewID(), Score: 0.7},
		{Ref: "nonsense", Score: 0.6},
	}}
	sm := &fakeLane{name: memory.LaneSupermemory, hits: []memory.Hit{
		{Ref: "doc:" + lease.ID + "#c2", Kind: "resource", Text: "Pets are allowed.", Score: 0.9},
		{Ref: "doc:" + model.NewID() + "#c1", Kind: "resource", Text: "A removed document", Score: 0.8},
	}}
	f.router.SetLanes([]memory.Lane{mem0, sm})
	p := f.recall(memory.Query{Text: "does the lease allow pets",
		Lanes: []memory.LaneName{memory.LaneMem0, memory.LaneSupermemory}})
	require.Equal(t, memory.IntentResource, p.Intent)
	require.ElementsMatch(t, []string{claimRef(home), "doc:" + lease.ID + "#c2"}, hitRefs(p.Hits),
		"refs are read from the store; candidates, gone claims, and removed documents are left out")
	for _, h := range p.Hits {
		switch h.Kind {
		case kindClaim:
			require.Equal(t, "Lives in Porto", h.Text, "the claim as stored, not the lane's words")
			require.Equal(t, "your edit", h.Source)
			require.Equal(t, []memory.LaneName{memory.LaneMem0}, h.Lanes)
		case kindResource:
			require.Equal(t, "Pets are allowed.", h.Text)
			require.Equal(t, "document “Lease.pdf”", h.Source)
			require.Equal(t, model.TrustExternal, h.Trust)
		default:
			t.Fatalf("unexpected hit %+v", h)
		}
	}
	mem0.mu.Lock()
	require.Equal(t, 40, mem0.asked[0].Budget.Candidates)
	require.Equal(t, memory.IntentResource, mem0.asked[0].Intent)
	mem0.mu.Unlock()
}

func TestFilters(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	june, march := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	porto := f.claim("user.home", "Lives in Porto", func(c *model.Claim) { c.ValidFrom = tp(june) })
	lisbon := f.claim("user.home", "Lives in Lisbon", func(c *model.Claim) {
		c.Status, c.SupersededBy = model.ClaimSuperseded, &porto.ID
		c.ValidFrom, c.ValidTo = tp(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), tp(june)
	})

	t.Run("status and as of", func(t *testing.T) {
		p := f.recall(memory.Query{Text: "where do I live"})
		require.Equal(t, []string{claimRef(porto)}, hitRefs(p.Hits), "now: the active claim only")
		p = f.recall(memory.Query{Text: "where do I live", AsOf: march})
		require.Equal(t, []string{claimRef(lisbon)}, hitRefs(p.Hits), "in March: the claim that held then")
		p = f.recall(memory.Query{Text: "where do I live", AsOf: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
		require.Empty(t, p.Hits)
		require.True(t, p.Abstain)
	})

	t.Run("superseded with no end", func(t *testing.T) {
		globex := f.claim("user.employer", "Works at Globex")
		acme := f.claim("user.employer", "Works at Acme", func(c *model.Claim) {
			c.Status, c.SupersededBy, c.SupersededAt = model.ClaimSuperseded, &globex.ID, tp(june)
		})
		p := f.recall(memory.Query{Text: "where do I work", AsOf: march})
		require.ElementsMatch(t, []string{claimRef(globex), claimRef(acme)}, hitRefs(p.Hits),
			"it held until it was superseded")
		p = f.recall(memory.Query{Text: "where do I work", AsOf: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})
		require.Equal(t, []string{claimRef(globex)}, hitRefs(p.Hits))
	})

	t.Run("never recalled", func(t *testing.T) {
		pets := []model.Claim{
			f.claim("user.pet", "Has a cat named Miso", func(c *model.Claim) {
				c.Status, c.Trust = model.ClaimCandidate, model.TrustInferred
			}),
			f.claim("user.pet", "Has a dog named Rex", func(c *model.Claim) {
				c.Status, c.Trust = model.ClaimQuarantined, model.TrustExternal
			}),
			f.claim("user.pet", "Has a parrot named Kiwi", func(c *model.Claim) { c.Status = model.ClaimRejected }),
			f.claim("user.pet", "Has a hamster named Bo", func(c *model.Claim) { c.Suppressed = true }),
		}
		gone := f.claim("user.pet", "Has a goldfish named Finn")
		f.update(store.ClaimUpdate{ID: gone.ID, Forget: true})
		p := f.recall(memory.Query{Text: "cat dog parrot hamster goldfish named Miso Rex Kiwi Bo Finn"})
		require.Empty(t, p.Hits)
		visible := f.claim("user.pet", "Has a rabbit named Miso")
		p = f.recall(memory.Query{Text: "cat dog parrot hamster goldfish rabbit named Miso Rex Kiwi Bo Finn"})
		require.Equal(t, []string{claimRef(visible)}, hitRefs(p.Hits))
		require.Len(t, pets, 4)
	})

	t.Run("disputed", func(t *testing.T) {
		a := f.claim("user.school", "Studies at Leeds")
		b := f.claim("user.school", "Studies at York")
		f.conflict(a, b)
		calm := f.claim("user.study", "Studies chemistry")
		p := f.recall(memory.Query{Text: "where do I study"})
		require.ElementsMatch(t, []string{claimRef(a), claimRef(b), claimRef(calm)}, hitRefs(p.Hits))
		for _, h := range p.Hits {
			require.Equal(t, h.Ref != claimRef(calm), h.Disputed, h.Text)
		}
	})

	t.Run("sensitive in a tainted run", func(t *testing.T) {
		insulin := f.claim("user.health", "Takes insulin daily", func(c *model.Claim) {
			c.Sensitivity, c.Cardinality = model.SensitivitySensitive, model.CardinalityMany
		})
		ep := f.episode(model.Episode{Body: "Insulin pen is in the fridge", At: day(8),
			Sensitivity: model.SensitivitySensitive})
		p := f.recall(memory.Query{Text: "insulin"})
		require.ElementsMatch(t, []string{claimRef(insulin), epRef(ep)}, hitRefs(p.Hits))
		for _, h := range p.Hits {
			require.Contains(t, h.Text, "nsulin")
		}
		for _, run := range []memory.RunCtx{{Tainted: true}, {WorldReach: true}} {
			p = f.recall(memory.Query{Text: "insulin", Run: run})
			require.ElementsMatch(t, []string{claimRef(insulin), epRef(ep)}, hitRefs(p.Hits), "the refs are kept")
			for _, h := range p.Hits {
				require.Equal(t, "(sensitive: ask to see it)", h.Text)
				require.Nil(t, h.Value)
			}
		}
	})
}

func TestDedupeAndExclude(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, msgs := f.chat(day(10), "Works at  ACME corp", "Works at Acme Corp")
	acme := f.claim("user.employer", "Works at Acme Corp")
	p := f.recall(memory.Query{Text: "acme corp", Intent: memory.IntentAny})
	require.Equal(t, []string{claimRef(acme)}, hitRefs(p.Hits), "messages saying what the claim says are dropped")

	p = f.recall(memory.Query{Text: "acme corp", Intent: memory.IntentAny, Exclude: []string{claimRef(acme)}})
	require.Len(t, p.Hits, 1, "without the claim, one of the two messages that say the same is kept")
	require.Contains(t, []string{msgRef(msgs[0]), msgRef(msgs[1])}, p.Hits[0].Ref)
	require.Equal(t, "chat “Works at ACME corp”, 10 Oct", p.Hits[0].Source)
}

func TestAbstain(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	for text, want := range map[string]bool{
		"where do I live":                true,
		"what did we decide on Tuesday?": true,
		"hello there":                    false,
		"do i like jazz":                 false,
		"what does the lease say":        false,
	} {
		p := f.recall(memory.Query{Text: text})
		require.Equal(t, want, p.Abstain, text)
		require.Empty(t, p.Hits)
		require.NotNil(t, p.Hits)
	}

	sm := &fakeLane{name: memory.LaneSupermemory, hits: []memory.Hit{{Ref: "doc:" + model.NewID(), Text: "x"}}}
	f.router.SetLanes([]memory.Lane{sm})
	f.router.o.State = nil // documents are kept without a state to check them against
	p := f.recall(memory.Query{Text: "where do I live"})
	require.Len(t, p.Hits, 1)
	require.True(t, p.Abstain, "a document does not answer a question about the past")

	f.claim("user.home", "Lives in Porto")
	p = f.recall(memory.Query{Text: "where do I live"})
	require.False(t, p.Abstain)
	require.Equal(t, memory.IntentLookup, p.Intent)
}

func TestRecallBudget(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	for i := range 10 {
		f.claim("user.fact", "Acme fact number "+string(rune('a'+i)), func(c *model.Claim) {
			c.Cardinality = model.CardinalityMany
		})
	}
	p := f.recall(memory.Query{Text: "acme fact"})
	require.Len(t, p.Hits, 10)
	per := memory.Tokens(p.Hits[0].Text)
	p = f.recall(memory.Query{Text: "acme fact", Budget: memory.Budget{Tokens: 3*per + 1}})
	require.Len(t, p.Hits, 3, "stops before the budget would be exceeded")
	require.Equal(t, 3*per, p.Tokens)
}
