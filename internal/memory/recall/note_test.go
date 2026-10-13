package recall

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func (f *fx) resource(title, body string) model.Resource {
	f.t.Helper()
	res, err := f.r.MemoryState.AddResource(f.ctx, model.Resource{Title: title, Path: "/docs/" + title,
		Mime: "application/pdf", Size: int64(len(body))}, model.Episode{Channel: model.ChannelDoc,
		Speaker: model.SpeakerExternal, Body: body, At: day(1), Sensitivity: model.SensitivityPersonal})
	require.NoError(f.t, err)
	return res
}

func fromMessage(m model.ChatMessage) func(c *model.Claim) {
	return func(c *model.Claim) {
		c.SourceKind, c.SourceID, c.Channel = model.SourceMessage, m.ID, model.ChannelChat
	}
}

func TestNote(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, weekend := f.chat(day(3), "Weekend plans", "Have fun!")
	acme := f.claim("user.employer", "Works at Acme", fromMessage(weekend[0]), func(c *model.Claim) {
		c.ValidFrom = tp(day(1, "00:00"))
	})
	_, interview := f.chat(day(2), "Acme interview", "How did the Acme interview go?")
	badge := f.episode(model.Episode{Body: "Acme badge is on the fridge", At: day(5)})
	handbook := f.resource("Handbook.pdf", "Acme staff get Fridays off.")
	maybe := f.claim("user.role", "Manager at Acme", func(c *model.Claim) {
		c.Status, c.Trust = model.ClaimCandidate, model.TrustInferred
	})
	outside := f.claim("user.fact", "Acme pays well", func(c *model.Claim) {
		c.Status, c.Trust, c.Cardinality = model.ClaimQuarantined, model.TrustExternal, model.CardinalityMany
	})
	sm := &fakeLane{name: memory.LaneSupermemory, hits: []memory.Hit{{Ref: "doc:" + handbook.ID + "#c1",
		Kind: "resource", Text: "Fridays are off for all staff at Acme.", Source: "document “Handbook.pdf”"}}}
	data := &fakeData{hits: []memory.Hit{{Ref: "data:acme_hours", Text: "Worked 31 hours at Acme last week"}}}
	r := New(Options{Repo: f.r.Memory, State: f.r.MemoryState, Chats: f.r.Chats, Data: data,
		Lanes: []memory.Lane{sm}, Clock: f.clk, Loc: time.UTC})
	patient(r)

	note, err := r.Note(f.ctx, memory.NoteRequest{Text: "Tell me about Acme", ThreadID: "",
		Exclude: []string{msgRef(interview[1])}, RefStart: 5, AsOf: fxNow, Profile: memory.ProfileFrontier})
	require.NoError(t, err)
	lines := strings.Split(note.Text, "\n")
	require.Len(t, lines, 7, note.Text)
	require.Equal(t, "What you remember that may bear on this (cite [mN] when you rely on one):", lines[0])
	require.Equal(t, "[m5] Works at Acme (since Oct 2026; they said it in chat “Weekend plans”, 3 Oct)", lines[1])
	require.ElementsMatch(t, []string{
		"2 Oct, chat “Acme interview”, they said: “Acme interview”",
		"5 Oct, note they wrote: “Acme badge is on the fridge”",
	}, []string{body(lines[2]), body(lines[3])}, "messages and episodes next")
	require.Equal(t, "[m8] Worked 31 hours at Acme last week", lines[4], "then data")
	require.ElementsMatch(t, []string{
		"from the document “Handbook.pdf” (written by others): “Acme staff get Fridays off.”",
		"from the document “Handbook.pdf” (written by others): “Fridays are off for all staff at Acme.”",
	}, []string{body(lines[5]), body(lines[6])}, "then documents")
	for i, line := range lines[1:] {
		require.True(t, strings.HasPrefix(line, "[m"+strconv.Itoa(5+i)+"] "), "refs from RefStart in order")
	}
	require.Len(t, note.Refs, 6)

	want := map[string]string{"m5": claimRef(acme), "m8": "data:acme_hours"}
	for k, v := range want {
		require.Equal(t, v, note.Refs[k], k)
	}
	refs := map[string]bool{}
	for _, v := range note.Refs {
		refs[v] = true
	}
	require.True(t, refs[msgRef(interview[0])])
	require.True(t, refs[epRef(badge)])
	require.True(t, refs["doc:"+handbook.ID+"#c1"])
	require.False(t, refs[msgRef(interview[1])], "excluded")
	require.False(t, refs[claimRef(maybe)], "never a candidate claim")
	require.False(t, refs[claimRef(outside)], "never a quarantined claim")
	for k, v := range note.Refs {
		require.Contains(t, note.Text, "["+k+"] ", v)
	}
	require.NotEmpty(t, note.Packet.Hits)
	require.Equal(t, memory.IntentAny, note.Packet.Intent)

	usage, err := f.r.MemoryState.Usage(f.ctx, []string{acme.ID, maybe.ID})
	require.NoError(t, err)
	require.Equal(t, map[string]store.ClaimUsage{acme.ID: {Uses: 1, LastUsedAt: fxNow}}, usage,
		"the claims in the note are counted as used")
}

func TestNoteLines(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, msgs := f.chat(time.Date(2025, 12, 30, 9, 0, 0, 0, time.UTC), "Old plans for the cabin", "The cabin sounds lovely")
	gwen := f.episode(model.Episode{Body: "Cabin keys are with the neighbour", At: day(6), Speaker: model.SpeakerGwen})
	edit := f.episode(model.Episode{Kind: model.EpisodeEdit, Channel: model.ChannelFile, Body: "Cabin is in Wales",
		At: day(7)})
	ended := f.claim("user.plan", "Plans to rent the cabin", func(c *model.Claim) {
		c.Cardinality = model.CardinalityMany
		c.ValidFrom = tp(time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC))
		c.ValidTo = tp(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	})
	until := f.claim("user.plan", "Keeps the cabin booking", func(c *model.Claim) {
		c.Cardinality, c.ValidTo = model.CardinalityMany, tp(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	})
	wifi := f.episode(model.Episode{Body: "No wifi at the cabin", At: day(6)})
	noted := f.claim("user.fact", "The cabin has no wifi", func(c *model.Claim) {
		c.Cardinality, c.SourceKind, c.SourceID = model.CardinalityMany, model.SourceEpisode, wifi.ID
	})

	note, err := f.router.Note(f.ctx, memory.NoteRequest{Text: "cabin", AsOf: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	require.Contains(t, note.Text, "Plans to rent the cabin (Nov 2025 to Mar 2026; they set it themselves)")
	require.Contains(t, note.Text, "Keeps the cabin booking (until Dec 2026; they set it themselves)")
	require.Contains(t, note.Text, "The cabin has no wifi (they wrote it in a note in the dashboard, 6 Oct)")
	require.Contains(t, note.Text, "30 Dec 2025, chat “Old plans for the cabin”, they said: “Old plans for the cabin”")
	require.Contains(t, note.Text, "30 Dec 2025, chat “Old plans for the cabin”, you said: “The cabin sounds lovely”")
	require.Contains(t, note.Text, "6 Oct, note you wrote: “Cabin keys are with the neighbour”")
	require.Contains(t, note.Text, "7 Oct, edit they made: “Cabin is in Wales”")
	require.Contains(t, note.Text, "6 Oct, note they wrote: “No wifi at the cabin”")
	require.Len(t, note.Refs, 8)
	require.Equal(t, "[m1] ", note.Text[strings.Index(note.Text, "\n")+1:][:5], "refs start at 1 by default")
	for _, ref := range []string{claimRef(ended), claimRef(until), claimRef(noted), msgRef(msgs[0]), epRef(edit),
		epRef(gwen)} {
		require.Contains(t, note.Refs, refKey(note.Refs, ref))
	}

	long := strings.Repeat("word ", 100)
	_, chat := f.chat(day(8), "Long one about the cabin", long+"cabin")
	note, err = f.router.Note(f.ctx, memory.NoteRequest{Text: "cabin", Exclude: []string{msgRef(msgs[0])}})
	require.NoError(t, err)
	for _, line := range strings.Split(note.Text, "\n") {
		if strings.Contains(line, "Long one") && strings.Contains(line, "you said") {
			quote := line[strings.Index(line, "you said: “")+len("you said: “") : len(line)-len("”")]
			require.LessOrEqual(t, len([]rune(quote)), 300, "message text is cut to 300 characters")
			require.True(t, strings.HasSuffix(quote, "…"))
		}
	}
	require.NotEmpty(t, refKey(note.Refs, msgRef(chat[1])))
}

// body is a note line without its ref.
func body(line string) string {
	_, rest, _ := strings.Cut(line, "] ")
	return rest
}

// refKey is the note ref (m3) of a memory ref, "" when absent.
func refKey(refs map[string]string, ref string) string {
	for k, v := range refs {
		if v == ref {
			return k
		}
	}
	return ""
}

func TestNoteDisputed(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	a := f.claim("user.school", "Studies at Leeds")
	b := f.claim("user.school", "Studies at York")
	f.conflict(a, b)
	note, err := f.router.Note(f.ctx, memory.NoteRequest{Text: "where do I study", RefStart: 3})
	require.NoError(t, err)
	ka, kb := refKey(note.Refs, claimRef(a)), refKey(note.Refs, claimRef(b))
	require.ElementsMatch(t, []string{"m3", "m4"}, []string{ka, kb})
	require.Contains(t, note.Text, "Studies at Leeds (they set it themselves; disputed: conflicts with ["+kb+"])")
	require.Contains(t, note.Text, "Studies at York (they set it themselves; disputed: conflicts with ["+ka+"])")

	c := &cand{claim: &a, conflicts: []model.Conflict{{ClaimA: a.ID, ClaimB: b.ID}}}
	require.Equal(t, "disputed", disputed(c, map[string]int{claimRef(a): 1}), "the other is not in the note")
}

func TestNoteBudget(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	for i := range 40 {
		f.claim("user.fact", fmt.Sprintf("Acme note %02d: %s", i, strings.Repeat("lorem ipsum ", 35)), func(c *model.Claim) {
			c.Cardinality = model.CardinalityMany
		})
	}
	sizes := map[string]int{}
	for _, profile := range []string{memory.ProfileFrontier, memory.ProfileLocal} {
		note, err := f.router.Note(f.ctx, memory.NoteRequest{Text: "acme note", Profile: profile, RefStart: 1})
		require.NoError(t, err)
		budget := map[string]int{memory.ProfileFrontier: 1200, memory.ProfileLocal: 2000}[profile]
		require.LessOrEqual(t, memory.Tokens(note.Text), budget)
		require.Greater(t, memory.Tokens(note.Text), budget-150, "it stops only when the next line would not fit")
		lines := strings.Split(note.Text, "\n")
		require.Len(t, note.Refs, len(lines)-1)
		for i := range note.Refs {
			n, err := strconv.Atoi(i[1:])
			require.NoError(t, err)
			require.True(t, n >= 1 && n <= len(note.Refs), "refs are numbered without gaps")
			require.True(t, strings.HasPrefix(lines[n], "["+i+"] "))
		}
		sizes[profile] = len(note.Refs)
	}
	require.Greater(t, sizes[memory.ProfileLocal], sizes[memory.ProfileFrontier])
	require.Less(t, sizes[memory.ProfileLocal], 40)
}

func TestNoteAbstainAndEmpty(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	note, err := f.router.Note(f.ctx, memory.NoteRequest{Text: "where do I live?"})
	require.NoError(t, err)
	require.Equal(t, "What you remember that may bear on this (cite [mN] when you rely on one):\n"+
		"Nothing in memory answers this. If they ask about it, say you don't remember.", note.Text)
	require.Empty(t, note.Refs)
	require.True(t, note.Packet.Abstain)

	note, err = f.router.Note(f.ctx, memory.NoteRequest{Text: "hello there"})
	require.NoError(t, err)
	require.Empty(t, note.Text)
	require.Empty(t, note.Refs)
}

func TestViews(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	thread, weekend := f.chat(day(3), "Weekend plans", "Have fun!")
	_, old := f.chat(time.Date(2025, 12, 30, 9, 0, 0, 0, time.UTC), "Old chat", "ok")
	cli := f.episode(model.Episode{Channel: model.ChannelCLI, Body: "I take the 8:10 train", At: day(4)})
	ui := f.episode(model.Episode{Body: "Gym on Tuesdays", At: day(4)})
	edit := f.episode(model.Episode{Kind: model.EpisodeEdit, Channel: model.ChannelFile, Body: "- Lives in Porto",
		At: day(5)})
	lease := f.resource("Lease.pdf", "The lease ends in May")
	anna := f.entity(memory.EntityPerson, "Anna")

	from := func(kind, id string) func(c *model.Claim) {
		return func(c *model.Claim) { c.SourceKind, c.SourceID = kind, id }
	}
	many := func(c *model.Claim) { c.Cardinality = model.CardinalityMany }
	claims := []model.Claim{
		f.claim("user.employer", "Works at Acme", fromMessage(weekend[0])),
		f.claim("user.fact", "Said something last year", many, fromMessage(old[0])),
		f.claim("user.routine", "Takes the 8:10 train", many, from(model.SourceEpisode, cli.ID)),
		f.claim("user.routine", "Gym on Tuesdays", many, from(model.SourceEpisode, ui.ID)),
		f.claim("user.home", "Lives in Porto", from(model.SourceEpisode, edit.ID)),
		f.claim("user.fact", "Lease ends in May", many, from(model.SourceEpisode, *lease.EpisodeID)),
		f.claim("user.name", "Is called Sam"),
		f.claim("user.work_hours", "Works 9 to 5", from(model.SourceData, "")),
		f.claim("user.pet", "Has a cat", many, from(model.SourceMem0, model.NewID())),
		f.claim("user.fact", "Owns a flat", many, from(model.SourceSuperM, "")),
		f.claim("user.ambition", "Wants to travel", many, from(model.SourceClaim, model.NewID())),
		f.claim("person.birthday", "Anna's birthday is 3 May", func(c *model.Claim) { c.SubjectID = &anna.ID }),
		f.claim("user.hobby.climbing", "Climbs on Sundays", many, fromMessage(model.ChatMessage{ID: model.NewID()})),
		f.claim("user.fact", "Saw it in a note", many, from(model.SourceEpisode, model.NewID())),
	}
	f.conflict(claims[0], claims[1])

	views, err := f.router.Views(f.ctx, claims)
	require.NoError(t, err)
	type row struct{ Subject, Label, Source, ThreadID string }
	got := make([]row, len(views))
	for i, v := range views {
		require.Equal(t, claims[i], v.Claim)
		require.Equal(t, i <= 1, v.Disputed, v.Text)
		got[i] = row{v.Subject, v.Label, v.Source, v.ThreadID}
	}
	require.Equal(t, []row{
		{"", "Works at", "chat “Weekend plans”, 3 Oct", thread.ID},
		{"", "About you", "chat “Old chat”, 30 Dec 2025", old[0].ThreadID},
		{"", "Routine", "note in the terminal, 4 Oct", ""},
		{"", "Routine", "note in the dashboard, 4 Oct", ""},
		{"", "Lives in", "your edit, 5 Oct", ""},
		{"", "About you", "document “Lease.pdf”", ""},
		{"", "Name", "your edit", ""},
		{"", "Usual working hours", "your tracked data", ""},
		{"", "Pet", "chat (found by Mem0)", ""},
		{"", "About you", "your documents (Supermemory)", ""},
		{"", "Wants to", "other memories", ""},
		{"Anna", "Birthday", "your edit", ""},
		{"", "Climbing", "a chat since deleted", ""},
		{"", "About you", "a note since erased", ""},
	}, got)

	views, err = f.router.Views(f.ctx, nil)
	require.NoError(t, err)
	require.Empty(t, views)
}

func TestGet(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	globex := f.claim("user.employer", "Works at Globex", func(c *model.Claim) { c.ValidFrom = tp(june) })
	acme := f.claim("user.employer", "Works at Acme", func(c *model.Claim) {
		c.Status, c.SupersededBy = model.ClaimSuperseded, &globex.ID
		c.ValidFrom, c.ValidTo = tp(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), tp(june)
	})
	got, err := f.router.Get(f.ctx, "user.employer", nil, time.Time{})
	require.NoError(t, err)
	require.Equal(t, globex.ID, got.ID)
	got, err = f.router.Get(f.ctx, "user.employer", nil, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, acme.ID, got.ID, "what held then")
	_, err = f.router.Get(f.ctx, "user.employer", nil, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.router.Get(f.ctx, "user.home", nil, time.Time{})
	require.ErrorIs(t, err, store.ErrNotFound)

	anna := f.entity(memory.EntityPerson, "Anna")
	hers := f.claim("person.birthday", "Anna's birthday is 3 May", func(c *model.Claim) { c.SubjectID = &anna.ID })
	got, err = f.router.Get(f.ctx, "person.birthday", &anna.ID, time.Time{})
	require.NoError(t, err)
	require.Equal(t, hers.ID, got.ID)
	_, err = f.router.Get(f.ctx, "person.birthday", nil, time.Time{})
	require.ErrorIs(t, err, store.ErrNotFound, "not the user's")

	f.update(store.ClaimUpdate{ID: globex.ID, Suppressed: testutil.Ptr(true)})
	_, err = f.router.Get(f.ctx, "user.employer", nil, time.Time{})
	require.ErrorIs(t, err, store.ErrNotFound, "a suppressed claim is not brought up")
}

func TestExplain(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	thread, msgs := f.chat(day(3), "Weekend plans", "Sounds good", "I start at Acme on Monday", "Congrats!",
		"thanks", "bye", "see you")
	acme := f.claim("user.employer", "Works at Acme", fromMessage(msgs[2]), func(c *model.Claim) {
		c.Span = "I start at Acme on Monday"
	})
	initech := f.claim("user.employer", "Works at Initech", func(c *model.Claim) {
		c.Status, c.SupersededBy = model.ClaimSuperseded, &acme.ID
		c.MentionedAt = day(1)
	})
	link := model.MemoryLink{ID: model.NewID(), FromKind: "claim", FromID: acme.ID, ToKind: "thread", ToID: thread.ID,
		Relation: "about"}
	require.NoError(t, f.r.Memory.Apply(f.ctx, store.MemoryChange{Links: []model.MemoryLink{link}}))

	ex, err := f.router.Explain(f.ctx, claimRef(acme))
	require.NoError(t, err)
	require.Equal(t, claimRef(acme), ex.Ref)
	require.Equal(t, acme.ID, ex.Claim.ID)
	require.Equal(t, "chat “Weekend plans”, 3 Oct", ex.Claim.Source)
	require.Equal(t, thread.ID, ex.Claim.ThreadID)
	require.Equal(t, "I start at Acme on Monday", ex.Quote)
	require.Equal(t, []string{msgs[0].ID, msgs[1].ID, msgs[2].ID, msgs[3].ID, msgs[4].ID},
		messageIDs(ex.Context), "the words around the source")
	require.Nil(t, ex.Episode)
	require.Len(t, ex.History, 2)
	require.Equal(t, acme.ID, ex.History[0].ID, "newest first")
	require.Equal(t, initech.ID, ex.History[1].ID)
	require.Len(t, ex.Links, 1)
	require.Equal(t, thread.ID, ex.Links[0].ToID)

	ex, err = f.router.Explain(f.ctx, msgRef(msgs[0]))
	require.NoError(t, err)
	require.Equal(t, "Weekend plans", ex.Quote)
	require.Equal(t, []string{msgs[0].ID, msgs[1].ID, msgs[2].ID}, messageIDs(ex.Context))
	require.Nil(t, ex.Claim)

	ep := f.episode(model.Episode{Body: "Gym on Tuesdays", At: day(4)})
	noted := f.claim("user.routine", "Gym on Tuesdays", func(c *model.Claim) {
		c.Cardinality, c.SourceKind, c.SourceID = model.CardinalityMany, model.SourceEpisode, ep.ID
	})
	ex, err = f.router.Explain(f.ctx, claimRef(noted))
	require.NoError(t, err)
	require.Equal(t, ep.ID, ex.Episode.ID)
	require.Empty(t, ex.Context)

	ex, err = f.router.Explain(f.ctx, epRef(ep))
	require.NoError(t, err)
	require.Equal(t, "Gym on Tuesdays", ex.Quote)
	require.Equal(t, ep.ID, ex.Episode.ID)

	lease := f.resource("Lease.pdf", "The lease ends in May")
	ex, err = f.router.Explain(f.ctx, "doc:"+lease.ID+"#c3")
	require.NoError(t, err)
	require.Equal(t, "The lease ends in May", ex.Episode.Body)

	_, err = f.router.Explain(f.ctx, "nonsense")
	require.ErrorIs(t, err, store.ErrInvalid)
	f.update(store.ClaimUpdate{ID: acme.ID, Forget: true})
	_, err = f.router.Explain(f.ctx, claimRef(acme))
	require.ErrorIs(t, err, store.ErrNotFound, "a forgotten claim")
}

func messageIDs(ms []model.ChatMessage) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func TestResolve(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	long := strings.Repeat("Acme ", 40)
	thread, msgs := f.chat(day(3), "Weekend plans", long)
	acme := f.claim("user.employer", "Works at Acme", fromMessage(msgs[0]))
	note := f.episode(model.Episode{Body: "Gym on Tuesdays", At: day(4)})
	titled := f.episode(model.Episode{Title: "Bike", Body: "The bike lock code is not written here", At: day(4)})
	lease := f.resource("Lease.pdf", "The lease ends in May")

	resolve := func(ref string) wire.MemoryRef {
		t.Helper()
		got, err := f.router.Resolve(f.ctx, ref)
		require.NoError(t, err)
		return got
	}
	require.Equal(t, wire.MemoryRef{Ref: claimRef(acme), Kind: "claim", Text: "Works at Acme",
		Source: "chat “Weekend plans”, 3 Oct", ThreadID: thread.ID}, resolve(claimRef(acme)))
	got := resolve(msgRef(msgs[1]))
	require.Equal(t, "message", got.Kind)
	require.LessOrEqual(t, len([]rune(got.Text)), 120)
	require.True(t, strings.HasSuffix(got.Text, "…"))
	require.Equal(t, "chat “Weekend plans”, 3 Oct", got.Source)
	require.Equal(t, thread.ID, got.ThreadID)
	require.Equal(t, wire.MemoryRef{Ref: epRef(note), Kind: "episode", Text: "Gym on Tuesdays",
		Source: "note in the dashboard, 4 Oct"}, resolve(epRef(note)))
	require.Equal(t, "Bike", resolve(epRef(titled)).Text, "an episode's title when it has one")
	require.Equal(t, wire.MemoryRef{Ref: "doc:" + lease.ID, Kind: "resource", Text: "Lease.pdf",
		Source: "document “Lease.pdf”"}, resolve("doc:"+lease.ID))
	require.Equal(t, wire.MemoryRef{Ref: "ep:" + *lease.EpisodeID, Kind: "resource", Text: "Lease.pdf",
		Source: "document “Lease.pdf”"}, resolve("ep:"+*lease.EpisodeID))
	require.Equal(t, wire.MemoryRef{Ref: "data:week_hours", Kind: "data", Text: "week_hours",
		Source: "your tracked data"}, resolve("data:week_hours"))

	f.update(store.ClaimUpdate{ID: acme.ID, Forget: true})
	require.Equal(t, wire.MemoryRef{Ref: claimRef(acme), Kind: "claim", Gone: true}, resolve(claimRef(acme)))
	require.NoError(t, f.r.Chats.Delete(f.ctx, thread.ID))
	require.Equal(t, wire.MemoryRef{Ref: msgRef(msgs[1]), Kind: "message", Gone: true}, resolve(msgRef(msgs[1])))
	require.NoError(t, f.r.Memory.EraseEpisode(f.ctx, note.ID, store.LedgerItem{}))
	require.Equal(t, wire.MemoryRef{Ref: epRef(note), Kind: "episode", Gone: true}, resolve(epRef(note)))
	require.NoError(t, f.r.MemoryState.RemoveResource(f.ctx, lease.ID, store.LedgerItem{}))
	require.Equal(t, wire.MemoryRef{Ref: "doc:" + lease.ID + "#c1", Kind: "resource", Gone: true},
		resolve("doc:"+lease.ID+"#c1"))

	_, err := f.router.Resolve(f.ctx, "claim:short")
	require.ErrorIs(t, err, store.ErrInvalid)
}
