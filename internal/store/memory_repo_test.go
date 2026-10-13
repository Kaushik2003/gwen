package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func note(body string, when time.Time) model.Episode {
	return model.Episode{Kind: model.EpisodeNote, Channel: model.ChannelUI, Speaker: model.SpeakerUser, Body: body,
		At: when, Sensitivity: model.SensitivityPersonal}
}

// fact is an active claim about the user said in a chat message.
func fact(key, text, value string) model.Claim {
	return model.Claim{ID: model.NewID(), Key: key, Value: []byte(value), Text: text,
		Cardinality: model.CardinalityOne, Status: model.ClaimActive, Trust: model.TrustOwnerSaid,
		Sensitivity: model.SensitivityPersonal, Confidence: 0.9, MentionedAt: at("09:00"),
		SourceKind: model.SourceMessage, SourceID: model.NewID(), Span: text, Channel: model.ChannelChat,
		Extractor: "test#1", ContentHash: sha(key + "\x00" + value)}
}

// apply applies a change that must succeed.
func (f *fx) apply(ch store.MemoryChange) {
	f.t.Helper()
	require.NoError(f.t, f.r.Memory.Apply(f.ctx, ch))
}

// count counts the rows of a query.
func (f *fx) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	require.NoError(f.t, f.db.SQL().QueryRowContext(f.ctx, query, args...).Scan(&n))
	return n
}

// dbFileHas reports whether the database file holds s, the write-ahead log
// included.
func (f *fx) dbFileHas(s string) bool {
	f.t.Helper()
	var file string
	require.NoError(f.t, f.db.SQL().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file))
	for _, path := range []string{file, file + "-wal"} {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(f.t, err)
		if strings.Contains(string(raw), s) {
			return true
		}
	}
	return false
}

func (f *fx) ledger() []model.LedgerEntry {
	f.t.Helper()
	rows, err := f.r.Memory.Ledger(f.ctx, 0, 0)
	require.NoError(f.t, err)
	return rows
}

func ids[T any](xs []T, id func(T) string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = id(x)
	}
	return out
}

func claimIDs(cs []model.Claim) []string { return ids(cs, func(c model.Claim) string { return c.ID }) }

func TestEpisodes(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem := f.r.Memory

	for _, tc := range []struct {
		field string
		edit  func(e *model.Episode)
	}{
		{"kind", func(e *model.Episode) { e.Kind = "" }},
		{"speaker", func(e *model.Episode) { e.Speaker = "bot" }},
		{"sensitivity", func(e *model.Episode) { e.Sensitivity = "secret" }},
		{"at", func(e *model.Episode) { e.At = time.Time{} }},
	} {
		e := note("x", at("09:00"))
		tc.edit(&e)
		_, err := mem.AddEpisode(f.ctx, e)
		userErr(t, err, store.ErrInvalid, tc.field)
	}
	bad := note("x", at("09:00"))
	bad.Channel = ""
	_, err := mem.AddEpisode(f.ctx, bad)
	require.ErrorIs(t, err, store.ErrInvalid, "the schema checks the channel")

	dentist := note("Remember that my dentist is Dr Okafor on Elm Street", at("09:00"))
	dentist.Title = "Dentist"
	ep, err := mem.AddEpisode(f.ctx, dentist)
	require.NoError(t, err)
	require.Len(t, ep.ID, 36)
	require.Equal(t, sha(dentist.Body), ep.ContentHash, "the hash is memory.Hash of the body")
	require.Equal(t, model.Envelope{CreatedAt: fxNow, UpdatedAt: fxNow, DeviceID: f.db.DeviceID(), Rev: 1}, ep.Envelope)
	got, err := mem.Episode(f.ctx, ep.ID)
	require.NoError(t, err)
	require.Equal(t, ep, got, "returned as stored")

	mine := note("I moved to Lisbon in March", at("10:00"))
	mine.ID = model.NewID()
	lisbon, err := mem.AddEpisode(f.ctx, mine)
	require.NoError(t, err)
	require.Equal(t, mine.ID, lisbon.ID, "the caller's id is kept")
	late, err := mem.AddEpisode(f.ctx, note("Gym on Tuesdays", at("11:00")))
	require.NoError(t, err)

	rows, err := mem.EpisodesAfter(f.ctx, 0, 0)
	require.NoError(t, err)
	require.Equal(t, []string{ep.ID, lisbon.ID, late.ID}, ids(rows, func(r store.EpisodeRow) string { return r.ID }))
	require.Less(t, rows[0].Rowid, rows[1].Rowid)
	next, err := mem.EpisodesAfter(f.ctx, rows[0].Rowid, 1)
	require.NoError(t, err)
	require.Equal(t, []string{lisbon.ID}, ids(next, func(r store.EpisodeRow) string { return r.ID }))

	hits, err := mem.SearchEpisodes(f.ctx, "Who is my dentist?", 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, ep.ID, hits[0].ID)
	require.Less(t, hits[0].Rank, 0.0, "FTS5 ranks are negative, lower is better")
	hits, err = mem.SearchEpisodes(f.ctx, "dentist", 0)
	require.NoError(t, err)
	require.Empty(t, hits)

	between, err := mem.EpisodesBetween(f.ctx, at("09:00"), at("11:00"), 0)
	require.NoError(t, err)
	require.Equal(t, []string{lisbon.ID, ep.ID}, ids(between, func(e model.Episode) string { return e.ID }),
		"newest first, the end excluded")
	between, err = mem.EpisodesBetween(f.ctx, at("09:00"), at("12:00"), 1)
	require.NoError(t, err)
	require.Equal(t, []string{late.ID}, ids(between, func(e model.Episode) string { return e.ID }))

	before := len(f.ledger())
	require.NoError(t, mem.EraseEpisode(f.ctx, ep.ID, store.LedgerItem{}))
	_, err = mem.Episode(f.ctx, ep.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, mem.EraseEpisode(f.ctx, ep.ID, store.LedgerItem{}), store.ErrNotFound)
	hits, err = mem.SearchEpisodes(f.ctx, "dentist Okafor", 10)
	require.NoError(t, err)
	require.Empty(t, hits)
	rows, err = mem.EpisodesAfter(f.ctx, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Zero(t, f.count(`SELECT count(*) FROM episodes_fts WHERE episodes_fts MATCH 'okafor'`))
	require.Equal(t, 1, f.count(`SELECT count(*) FROM episodes WHERE id = ? AND title = '' AND body = ''
		AND deleted_at IS NOT NULL AND rev = 2 AND content_hash = ?`, ep.ID, ep.ContentHash),
		"a tombstone with its words erased and its hash kept")
	require.False(t, f.dbFileHas("Okafor"), "the words are gone from the file")
	require.True(t, f.dbFileHas("Lisbon"), "the check can see words that are there")

	ledger := f.ledger()
	require.Len(t, ledger, before+1)
	require.JSONEq(t, `{"action": "episode.erase", "episode": "`+ep.ID+`", "hash": "`+ep.ContentHash+`"}`,
		string(ledger[len(ledger)-1].Data))
	require.NoError(t, mem.EraseEpisode(f.ctx, late.ID, store.LedgerItem{Data: map[string]string{"why": "asked"}}))
	ledger = f.ledger()
	require.JSONEq(t, `{"why": "asked"}`, string(ledger[len(ledger)-1].Data), "the caller's data is kept")
}

func TestMemoryReadsMessages(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem, chats := f.r.Memory, f.r.Chats
	exam, msgs, err := chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, "I have an exam on Friday", at("09:00")),
		say(model.ChatAssistant, "Three study blocks then.", at("09:01")),
		say(model.ChatUser, "Physics, chapter 4", at("09:02")),
	})
	require.NoError(t, err)
	gym, gymMsgs, err := chats.Append(f.ctx, "", []model.ChatMessage{say(model.ChatUser, "Gym tonight", at("10:00"))})
	require.NoError(t, err)
	gone, goneMsgs, err := chats.Append(f.ctx, "", []model.ChatMessage{say(model.ChatUser, "Delete me", at("09:30"))})
	require.NoError(t, err)
	require.NoError(t, chats.Delete(f.ctx, gone.ID))

	rows, err := mem.MessagesAfter(f.ctx, 0, 0)
	require.NoError(t, err)
	require.Equal(t, []string{msgs[0].ID, msgs[1].ID, msgs[2].ID, gymMsgs[0].ID},
		ids(rows, func(r store.MessageRow) string { return r.ID }), "a deleted chat's messages are left out")
	require.Equal(t, exam.Title, rows[0].ThreadTitle)
	require.Equal(t, gym.Title, rows[3].ThreadTitle)
	require.Equal(t, "Three study blocks then.", rows[1].Text)
	require.Less(t, rows[1].Rowid, rows[2].Rowid)
	rows, err = mem.MessagesAfter(f.ctx, rows[1].Rowid, 1)
	require.NoError(t, err)
	require.Equal(t, []string{msgs[2].ID}, ids(rows, func(r store.MessageRow) string { return r.ID }))

	before, err := mem.MessagesBefore(f.ctx, exam.ID, at("09:02"), 4)
	require.NoError(t, err)
	require.Equal(t, []string{msgs[0].ID, msgs[1].ID}, ids(before, func(m model.ChatMessage) string { return m.ID }),
		"oldest first")
	before, err = mem.MessagesBefore(f.ctx, exam.ID, at("09:02"), 1)
	require.NoError(t, err)
	require.Equal(t, []string{msgs[1].ID}, ids(before, func(m model.ChatMessage) string { return m.ID }),
		"the newest n before")
	before, err = mem.MessagesBefore(f.ctx, exam.ID, at("09:02"), 0)
	require.NoError(t, err)
	require.Empty(t, before)

	hit, err := mem.Message(f.ctx, msgs[2].ID)
	require.NoError(t, err)
	require.Equal(t, exam.Title, hit.ThreadTitle)
	require.Equal(t, "Physics, chapter 4", hit.Text)
	_, err = mem.Message(f.ctx, goneMsgs[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	between, err := mem.MessagesBetween(f.ctx, at("09:00"), at("10:00"), 0)
	require.NoError(t, err)
	require.Equal(t, []string{msgs[2].ID, msgs[1].ID, msgs[0].ID},
		ids(between, func(h model.ChatHit) string { return h.ID }), "newest first, the end excluded")
	require.Equal(t, exam.Title, between[0].ThreadTitle)
	between, err = mem.MessagesBetween(f.ctx, at("09:00"), at("11:00"), 2)
	require.NoError(t, err)
	require.Equal(t, []string{gymMsgs[0].ID, msgs[2].ID}, ids(between, func(h model.ChatHit) string { return h.ID }))
}

func TestClaimsAndEntities(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem := f.r.Memory

	anna := model.Entity{ID: model.NewID(), Kind: "person", Name: "Anna", Aliases: []string{"Annie", "Sis"}}
	acme := model.Entity{ID: model.NewID(), Kind: "org", Name: "Acme"}
	employer := fact("user.employer", "Works at Acme as a backend intern", `"Acme"`)
	employer.RecordedAt = at("09:00")
	employer.ValidFrom = tp(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	home := fact("user.home", "Lives in Lisbon", `"Lisbon"`)
	home.RecordedAt, home.Suppressed = at("09:01"), true
	birthday := fact("person.birthday", "Anna's birthday is 3 May", `"2026-05-03"`)
	birthday.SubjectID, birthday.RecordedAt = &anna.ID, at("09:02")
	health := fact("user.health", "Has asthma", `"asthma"`)
	health.Sensitivity, health.RecordedAt = model.SensitivitySensitive, at("09:03")
	food := fact("pref.food", "Likes ramen", `"ramen"`)
	food.Status, food.Trust, food.Cardinality, food.RecordedAt = model.ClaimCandidate, model.TrustInferred,
		model.CardinalityMany, at("09:04")
	underscore := fact("user.a_b", "Underscore key", `{"n": 1}`)
	underscore.RecordedAt = at("09:05")
	lookalike := fact("user.aXb", "Lookalike key", `1`)
	lookalike.RecordedAt = at("09:06")
	future := fact("user.school", "Starts at TU Delft in October", `"TU Delft"`)
	future.RecordedAt, future.ValidFrom = at("09:07"), tp(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	past := fact("user.city", "Lived in Porto", `"Porto"`)
	past.RecordedAt, past.ValidTo = at("09:08"), tp(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	past.ValidFrom = tp(time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC))
	all := []model.Claim{employer, home, birthday, health, food, underscore, lookalike, future, past}
	f.apply(store.MemoryChange{Entities: []model.Entity{anna, acme}, Claims: all})

	got, err := mem.Claim(f.ctx, employer.ID)
	require.NoError(t, err)
	require.Equal(t, model.Envelope{CreatedAt: fxNow, UpdatedAt: fxNow, DeviceID: f.db.DeviceID(), Rev: 1}, got.Envelope)
	require.Equal(t, f.db.DeviceID(), got.SourceDeviceID, "the source device is this one when not given")
	require.Nil(t, got.SubjectID)
	require.JSONEq(t, `"Acme"`, string(got.Value))
	require.Equal(t, *employer.ValidFrom, *got.ValidFrom)
	require.Nil(t, got.ValidTo)
	require.Equal(t, employer.RecordedAt, got.RecordedAt)
	require.Equal(t, employer.MentionedAt, got.MentionedAt)
	got.Envelope, got.SourceDeviceID = employer.Envelope, employer.SourceDeviceID
	require.Equal(t, employer, got, "every column reads back")
	_, err = mem.Claim(f.ctx, model.NewID())
	require.ErrorIs(t, err, store.ErrNotFound)
	hidden, err := mem.Claim(f.ctx, home.ID)
	require.NoError(t, err)
	require.True(t, hidden.Suppressed)
	about, err := mem.Claim(f.ctx, birthday.ID)
	require.NoError(t, err)
	require.Equal(t, anna.ID, *about.SubjectID)

	for _, tc := range []struct {
		name string
		f    store.ClaimFilter
		want []model.Claim
	}{
		{"everything live but suppressed, newest recorded first", store.ClaimFilter{},
			[]model.Claim{past, future, lookalike, underscore, food, health, birthday, employer}},
		{"suppressed too", store.ClaimFilter{IncludeSuppressed: true},
			[]model.Claim{past, future, lookalike, underscore, food, health, birthday, home, employer}},
		{"by status", store.ClaimFilter{Statuses: []string{model.ClaimCandidate}}, []model.Claim{food}},
		{"by statuses", store.ClaimFilter{Statuses: []string{model.ClaimCandidate, model.ClaimRejected}},
			[]model.Claim{food}},
		{"by key prefix", store.ClaimFilter{KeyPrefix: "person."}, []model.Claim{birthday}},
		{"_ in a prefix is not a wildcard", store.ClaimFilter{KeyPrefix: "user.a_"}, []model.Claim{underscore}},
		{"% in a prefix is not a wildcard", store.ClaimFilter{KeyPrefix: "user.%"}, nil},
		{"about the user", store.ClaimFilter{SubjectSet: true, KeyPrefix: "user.e"}, []model.Claim{employer}},
		{"about an entity", store.ClaimFilter{SubjectSet: true, Subject: &anna.ID}, []model.Claim{birthday}},
		{"not sensitive", store.ClaimFilter{ExcludeSensitive: true, KeyPrefix: "user.h", IncludeSuppressed: true},
			[]model.Claim{home}},
		{"valid now", store.ClaimFilter{ValidAt: &fxNow, KeyPrefix: "user."},
			[]model.Claim{lookalike, underscore, health, employer}},
		{"valid in August", store.ClaimFilter{ValidAt: tp(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)), KeyPrefix: "user.c"},
			[]model.Claim{past}},
		{"a page", store.ClaimFilter{Limit: 2, Offset: 1}, []model.Claim{future, lookalike}},
		{"an offset alone", store.ClaimFilter{Offset: 7}, []model.Claim{employer}},
	} {
		list, err := mem.Claims(f.ctx, tc.f)
		require.NoError(t, err, tc.name)
		require.Equal(t, claimIDs(tc.want), claimIDs(list), tc.name)
	}

	// The newest valid_from (or, without one, mention) comes first.
	first := fact("user.employer", "Worked at Initech", `"Initech"`)
	first.MentionedAt, first.Status, first.SupersededBy = at("08:00"), model.ClaimSuperseded, &employer.ID
	first.ValidFrom = tp(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	unstated := fact("user.employer", "Works at Globex", `"Globex"`)
	unstated.MentionedAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	f.apply(store.MemoryChange{Claims: []model.Claim{first, unstated}})
	byKey, err := mem.ClaimsByKey(f.ctx, "user.employer", nil)
	require.NoError(t, err)
	require.Equal(t, []string{unstated.ID, employer.ID, first.ID}, claimIDs(byKey))
	byKey, err = mem.ClaimsByKey(f.ctx, "user.employer", nil, model.ClaimSuperseded)
	require.NoError(t, err)
	require.Equal(t, []string{first.ID}, claimIDs(byKey))
	require.NotNil(t, byKey[0].SupersededAt, "a claim stored superseded is superseded now")
	byKey, err = mem.ClaimsByKey(f.ctx, "person.birthday", nil)
	require.NoError(t, err)
	require.Empty(t, byKey, "nil is the user, not any subject")
	byKey, err = mem.ClaimsByKey(f.ctx, "person.birthday", &anna.ID)
	require.NoError(t, err)
	require.Equal(t, []string{birthday.ID}, claimIDs(byKey))

	byHash, err := mem.ClaimsByHash(f.ctx, employer.ContentHash)
	require.NoError(t, err)
	require.Equal(t, []string{employer.ID}, claimIDs(byHash))
	from, err := mem.ClaimsFrom(f.ctx, model.SourceMessage, food.SourceID)
	require.NoError(t, err)
	require.Equal(t, []string{food.ID}, claimIDs(from))

	found, err := mem.SearchClaims(f.ctx, "Is she still interning?", 10)
	require.NoError(t, err)
	require.Equal(t, []string{employer.ID}, ids(found, func(h store.ClaimHit) string { return h.ID }),
		"interning finds intern: words are stemmed")
	found, err = mem.SearchClaims(f.ctx, "Where did they work?", 10)
	require.NoError(t, err)
	require.Len(t, found, 3, "Works and Worked")
	found, err = mem.SearchClaims(f.ctx, "employer", 10)
	require.NoError(t, err)
	require.Len(t, found, 3, "the key is searched too")
	require.Less(t, found[0].Rank, 0.0)
	found, err = mem.SearchClaims(f.ctx, "ramen", 10)
	require.NoError(t, err)
	require.Equal(t, []string{food.ID}, ids(found, func(h store.ClaimHit) string { return h.ID }), "any status")

	counts, err := mem.CountClaims(f.ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]int{model.ClaimActive: 9, model.ClaimCandidate: 1, model.ClaimSuperseded: 1}, counts)

	e, err := mem.Entity(f.ctx, anna.ID)
	require.NoError(t, err)
	require.Equal(t, anna.Aliases, e.Aliases)
	require.Equal(t, int64(1), e.Rev)
	people, err := mem.Entities(f.ctx, "person")
	require.NoError(t, err)
	require.Equal(t, []string{anna.ID}, ids(people, func(e model.Entity) string { return e.ID }))
	everyone, err := mem.Entities(f.ctx, "")
	require.NoError(t, err)
	require.Equal(t, []string{acme.ID, anna.ID}, ids(everyone, func(e model.Entity) string { return e.ID }), "by name")
	require.Equal(t, []string{}, everyone[0].Aliases, "no aliases is an empty list")
	for _, name := range []string{"anna", "ANNA", "annie", "SIS"} {
		found, err := mem.FindEntities(f.ctx, name)
		require.NoError(t, err)
		require.Equal(t, []string{anna.ID}, ids(found, func(e model.Entity) string { return e.ID }), name)
	}
	for _, name := range []string{"Ann", "", "Bob"} {
		found, err := mem.FindEntities(f.ctx, name)
		require.NoError(t, err)
		require.Empty(t, found, name)
	}

	// An entity whose id exists is renamed and given the aliases.
	f.clk.Advance(time.Minute)
	f.apply(store.MemoryChange{Entities: []model.Entity{{ID: anna.ID, Kind: "person", Name: "Anna Silva",
		Aliases: []string{"Anna"}}}})
	e, err = mem.Entity(f.ctx, anna.ID)
	require.NoError(t, err)
	require.Equal(t, "Anna Silva", e.Name)
	require.Equal(t, []string{"Anna"}, e.Aliases)
	require.Equal(t, int64(2), e.Rev)
	require.Equal(t, fxNow, e.CreatedAt)
	require.Equal(t, fxNow.Add(time.Minute), e.UpdatedAt)
	found2, err := mem.FindEntities(f.ctx, "sis")
	require.NoError(t, err)
	require.Empty(t, found2)
	_, err = mem.Entity(f.ctx, model.NewID())
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestApplyUpdatesClaims(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem := f.r.Memory
	old := fact("user.employer", "Works at Initech", `"Initech"`)
	old.ValidFrom = tp(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cur := fact("user.employer", "Works at Acme", `"Acme"`)
	f.apply(store.MemoryChange{Claims: []model.Claim{old, cur},
		Ledger: []store.LedgerItem{{Data: map[string]any{"action": "claim.commit", "claim": old.ID}}}})

	f.clk.Advance(time.Minute)
	now := fxNow.Add(time.Minute)
	until := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: old.ID, WasStatus: model.ClaimActive,
		SupersededBy: &cur.ID, ValidTo: store.Nullable[time.Time]{Set: true, Value: &until}}}})
	got, err := mem.Claim(f.ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, model.ClaimSuperseded, got.Status)
	require.Equal(t, cur.ID, *got.SupersededBy)
	require.Equal(t, now, *got.SupersededAt)
	require.Equal(t, until, *got.ValidTo)
	require.Equal(t, int64(2), got.Rev)
	require.Equal(t, now, got.UpdatedAt)

	// Made current again, it is superseded by nothing.
	active := model.ClaimActive
	conf, on := 0.4, true
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: old.ID, Status: &active, Confidence: &conf,
		Suppressed: &on, ValidTo: store.Nullable[time.Time]{Set: true}}}})
	got, err = mem.Claim(f.ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, model.ClaimActive, got.Status)
	require.Nil(t, got.SupersededBy)
	require.Nil(t, got.SupersededAt)
	require.Nil(t, got.ValidTo, "a set nil clears valid_to")
	require.InDelta(t, 0.4, got.Confidence, 1e-9)
	require.True(t, got.Suppressed)
	require.Equal(t, int64(3), got.Rev)
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: old.ID, WasStatus: model.ClaimActive}}})
	got, err = mem.Claim(f.ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Rev, "an update that changes nothing writes nothing")

	// A failed precondition rolls the whole change back.
	ledgerRows, jobs := len(f.ledger()), f.count(`SELECT count(*) FROM memory_jobs`)
	rejected := model.ClaimRejected
	newEntity := model.Entity{ID: model.NewID(), Kind: "person", Name: "Bea"}
	newClaim := fact("user.name", "Is called Sam", `"Sam"`)
	change := func(u store.ClaimUpdate) store.MemoryChange {
		return store.MemoryChange{Entities: []model.Entity{newEntity}, Claims: []model.Claim{newClaim},
			Updates: []store.ClaimUpdate{u},
			Links: []model.MemoryLink{{FromKind: "claim", FromID: newClaim.ID, ToKind: "entity", ToID: newEntity.ID,
				Relation: "about"}},
			Ledger: []store.LedgerItem{{Data: map[string]string{"action": "claim.reject"}}},
			Jobs:   []store.NewJob{{Kind: model.JobProject, Subject: "claim:" + newClaim.ID}}}
	}
	err = mem.Apply(f.ctx, change(store.ClaimUpdate{ID: cur.ID, WasStatus: model.ClaimCandidate, Status: &rejected}))
	userErr(t, err, store.ErrConflict, "")
	require.Contains(t, err.Error(), "the claim changed while this was decided")
	err = mem.Apply(f.ctx, change(store.ClaimUpdate{ID: model.NewID(), Status: &rejected}))
	require.ErrorIs(t, err, store.ErrNotFound)
	err = mem.Apply(f.ctx, change(store.ClaimUpdate{ID: cur.ID, Status: strp("bogus")}))
	require.ErrorIs(t, err, store.ErrInvalid, "the schema checks the status")
	superseded := model.ClaimSuperseded
	err = mem.Apply(f.ctx, change(store.ClaimUpdate{ID: cur.ID, Status: &superseded}))
	require.ErrorIs(t, err, store.ErrInvalid, "superseded needs superseded_by")
	_, err = mem.Entity(f.ctx, newEntity.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "nothing was written")
	_, err = mem.Claim(f.ctx, newClaim.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	links, err := mem.LinksFrom(f.ctx, newClaim.ID)
	require.NoError(t, err)
	require.Empty(t, links)
	require.Len(t, f.ledger(), ledgerRows)
	require.Equal(t, jobs, f.count(`SELECT count(*) FROM memory_jobs`))
	got, err = mem.Claim(f.ctx, cur.ID)
	require.NoError(t, err)
	require.Equal(t, model.ClaimActive, got.Status)

	bad := fact("user.name", "Broken", `{"not json"`)
	userErr(t, mem.Apply(f.ctx, store.MemoryChange{Claims: []model.Claim{bad}}), store.ErrInvalid, "value")
	bad.Value = nil
	userErr(t, mem.Apply(f.ctx, store.MemoryChange{Claims: []model.Claim{bad}}), store.ErrInvalid, "value")
	bad.Value, bad.Cardinality = []byte(`"x"`), "several"
	require.ErrorIs(t, mem.Apply(f.ctx, store.MemoryChange{Claims: []model.Claim{bad}}), store.ErrInvalid)
	dup := fact("user.name", "Duplicate", `"x"`)
	dup.ID = cur.ID
	require.ErrorIs(t, mem.Apply(f.ctx, store.MemoryChange{Claims: []model.Claim{dup}}), store.ErrConflict)

	// Forgetting erases the words and keeps the hash.
	secret := fact("user.side_project", "Builds Zanzibarco on weekends", `"Zanzibarco"`)
	f.apply(store.MemoryChange{Claims: []model.Claim{secret}})
	found, err := mem.SearchClaims(f.ctx, "zanzibarco", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: secret.ID, Forget: true}},
		Ledger: []store.LedgerItem{{Data: map[string]string{"action": "claim.forget", "claim": secret.ID,
			"hash": secret.ContentHash}}}})
	_, err = mem.Claim(f.ctx, secret.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	byHash, err := mem.ClaimsByHash(f.ctx, secret.ContentHash)
	require.NoError(t, err)
	require.Len(t, byHash, 1, "a forgotten claim keeps its hash, so it is not learned again")
	require.Equal(t, "", byHash[0].Text)
	require.Equal(t, "null", string(byHash[0].Value))
	require.Equal(t, "", byHash[0].Span)
	require.NotNil(t, byHash[0].DeletedAt)
	found, err = mem.SearchClaims(f.ctx, "zanzibarco", 10)
	require.NoError(t, err)
	require.Empty(t, found)
	require.Zero(t, f.count(`SELECT count(*) FROM claims_fts WHERE claims_fts MATCH 'zanzibarco'`))
	require.False(t, f.dbFileHas("Zanzibarco"), "nor left in the file")
	require.True(t, f.dbFileHas("Initech"))
	err = mem.Apply(f.ctx, store.MemoryChange{Updates: []store.ClaimUpdate{{ID: secret.ID, Suppressed: &on}}})
	require.ErrorIs(t, err, store.ErrNotFound, "a forgotten claim cannot change")
	from, err := mem.ClaimsFrom(f.ctx, model.SourceMessage, secret.SourceID)
	require.NoError(t, err)
	require.Empty(t, from)
	seq, err := mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Zero(t, seq, "forgetting keeps the chain")
}

func strp(s string) *string { return &s }

func TestApplyConflictsLinksAndJobs(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem := f.r.Memory
	a := fact("user.employer", "Works at Acme", `"Acme"`)
	b := fact("user.employer", "Works at Initech", `"Initech"`)
	c := fact("user.birthday", "Born on 3 May", `"2000-05-03"`)
	d := fact("user.birthday", "Born on 4 May", `"2000-05-04"`)
	work := model.Conflict{ID: model.NewID(), ClaimA: a.ID, ClaimB: b.ID, Reason: "two employers",
		Sensitivity: model.SensitivityPersonal}
	born := model.Conflict{ID: model.NewID(), ClaimA: c.ID, ClaimB: d.ID, Sensitivity: model.SensitivityPersonal}
	f.apply(store.MemoryChange{Claims: []model.Claim{a, b, c, d}, Conflicts: []model.Conflict{work, born}})

	got, err := mem.Conflict(f.ctx, work.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictOpen, got.Status, "a new conflict is open")
	require.Equal(t, "two employers", got.Reason)
	require.Nil(t, got.Resolution)
	require.Nil(t, got.ResolvedAt)
	require.Equal(t, int64(1), got.Rev)
	open, err := mem.Conflicts(f.ctx, model.ConflictOpen)
	require.NoError(t, err)
	require.Len(t, open, 2)
	for _, id := range []string{a.ID, b.ID} {
		of, err := mem.OpenConflictsOf(f.ctx, id)
		require.NoError(t, err)
		require.Equal(t, []string{work.ID}, ids(of, func(c model.Conflict) string { return c.ID }))
	}

	f.clk.Advance(time.Minute)
	keepA, both := model.ResolveKeepA, model.ResolveBoth
	f.apply(store.MemoryChange{ConflictUpdates: []store.ConflictUpdate{
		{ID: work.ID, Status: model.ConflictResolved, Resolution: &keepA},
		{ID: born.ID, Status: model.ConflictDismissed},
	}})
	got, err = mem.Conflict(f.ctx, work.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictResolved, got.Status)
	require.Equal(t, keepA, *got.Resolution)
	require.Equal(t, fxNow.Add(time.Minute), *got.ResolvedAt)
	require.Equal(t, int64(2), got.Rev)
	got, err = mem.Conflict(f.ctx, born.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictDismissed, got.Status)
	require.Nil(t, got.ResolvedAt)
	of, err := mem.OpenConflictsOf(f.ctx, a.ID)
	require.NoError(t, err)
	require.Empty(t, of)
	open, err = mem.Conflicts(f.ctx, model.ConflictOpen)
	require.NoError(t, err)
	require.Empty(t, open)
	every, err := mem.Conflicts(f.ctx, "")
	require.NoError(t, err)
	require.Len(t, every, 2)

	err = mem.Apply(f.ctx, store.MemoryChange{ConflictUpdates: []store.ConflictUpdate{{ID: model.NewID(),
		Status: model.ConflictDismissed}}})
	require.ErrorIs(t, err, store.ErrNotFound)
	err = mem.Apply(f.ctx, store.MemoryChange{ConflictUpdates: []store.ConflictUpdate{{ID: born.ID,
		Status: model.ConflictResolved}}})
	require.ErrorIs(t, err, store.ErrInvalid, "resolved needs a resolution")
	err = mem.Apply(f.ctx, store.MemoryChange{ConflictUpdates: []store.ConflictUpdate{{ID: born.ID,
		Status: model.ConflictDismissed, Resolution: &both}}})
	require.ErrorIs(t, err, store.ErrInvalid, "only a resolved conflict has a resolution")
	_, err = mem.Conflict(f.ctx, model.NewID())
	require.ErrorIs(t, err, store.ErrNotFound)

	msg := model.NewID()
	evidence := model.MemoryLink{ID: model.NewID(), FromKind: "claim", FromID: a.ID, ToKind: "message", ToID: msg,
		Relation: "evidence"}
	again := evidence
	again.ID = model.NewID()
	aboutAcme := model.MemoryLink{FromKind: "claim", FromID: a.ID, ToKind: "entity", ToID: model.NewID(),
		Relation: "about"}
	f.apply(store.MemoryChange{Links: []model.MemoryLink{evidence, again, aboutAcme}})
	links, err := mem.LinksFrom(f.ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, links, 2, "a link that is already live is kept")
	require.Equal(t, evidence.ID, links[0].ID)
	require.Equal(t, int64(1), links[0].Rev)
	to, err := mem.LinksTo(f.ctx, "message", msg)
	require.NoError(t, err)
	require.Equal(t, []string{evidence.ID}, ids(to, func(l model.MemoryLink) string { return l.ID }))
	err = mem.Apply(f.ctx, store.MemoryChange{Links: []model.MemoryLink{{FromKind: "task", FromID: a.ID,
		ToKind: "message", ToID: msg, Relation: "evidence"}}})
	require.ErrorIs(t, err, store.ErrInvalid, "a link the schema rejects fails rather than being skipped")

	f.apply(store.MemoryChange{Jobs: []store.NewJob{
		{Kind: model.JobProject, Subject: "claim:" + a.ID},
		{Kind: model.JobProject, Subject: "claim:" + a.ID, Payload: map[string]int{"n": 1}},
		{Kind: model.JobFiles, Payload: map[string]int{"n": 2}, RunAfter: fxNow.Add(time.Hour)},
	}})
	type job struct {
		kind, subject, payload, state string
		runAfter                      int64
	}
	var jobs []job
	rows, err := f.db.SQL().QueryContext(f.ctx,
		`SELECT kind, subject, payload, state, run_after FROM memory_jobs ORDER BY id`)
	require.NoError(t, err)
	for rows.Next() {
		var j job
		require.NoError(t, rows.Scan(&j.kind, &j.subject, &j.payload, &j.state, &j.runAfter))
		jobs = append(jobs, j)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	now := fxNow.Add(time.Minute).UnixMilli()
	require.Equal(t, []job{
		{model.JobProject, "claim:" + a.ID, "{}", model.JobPending, now},
		{model.JobFiles, "", `{"n":2}`, model.JobPending, fxNow.Add(time.Hour).UnixMilli()},
	}, jobs, "a pending job of the same kind and subject is kept; no payload is {}; no run_after is now")
	err = mem.Apply(f.ctx, store.MemoryChange{Jobs: []store.NewJob{{Kind: model.JobFiles, Payload: []int{1}}}})
	userErr(t, err, store.ErrInvalid, "payload")
	err = mem.Apply(f.ctx, store.MemoryChange{Jobs: []store.NewJob{{Kind: ""}}})
	require.ErrorIs(t, err, store.ErrInvalid, "a job needs a kind")
}

func TestLedgerChain(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem := f.r.Memory
	seq, err := mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Zero(t, seq, "an empty ledger verifies")
	require.Empty(t, f.ledger())

	run := model.NewID()
	f.apply(store.MemoryChange{Ledger: []store.LedgerItem{
		{Data: map[string]any{"action": "claim.commit", "claim": "c1"}},
		{Kind: "kernel", RunID: &run},
		{Data: struct {
			Action string `json:"action"`
			N      int    `json:"n"`
		}{"claim.supersede", 2}},
	}})
	f.clk.Advance(time.Second)
	f.apply(store.MemoryChange{Ledger: []store.LedgerItem{{Data: json.RawMessage(`{"action": "conflict.open"}`)}}})

	rows := f.ledger()
	require.Len(t, rows, 4)
	want := []struct{ kind, run, data string }{
		{model.LedgerMemory, "", `{"action":"claim.commit","claim":"c1"}`},
		{"kernel", run, `{}`},
		{model.LedgerMemory, "", `{"action":"claim.supersede","n":2}`},
		{model.LedgerMemory, "", `{"action":"conflict.open"}`},
	}
	prev := strings.Repeat("0", 64)
	for i, e := range rows {
		require.Equal(t, want[i].kind, e.Kind, i)
		require.Equal(t, want[i].data, string(e.Data), i)
		if want[i].run == "" {
			require.Nil(t, e.RunID, i)
		} else {
			require.Equal(t, want[i].run, *e.RunID, i)
		}
		require.Equal(t, prev, e.PrevHash, "each row covers the one before")
		require.Equal(t, sha(prev+"\n"+e.Kind+"\n"+want[i].run+"\n"+strconv.FormatInt(e.At.UnixMilli(), 10)+"\n"+
			string(e.Data)), e.Hash, i)
		prev = e.Hash
	}
	require.Equal(t, fxNow, rows[0].At)
	require.Equal(t, fxNow.Add(time.Second), rows[3].At)
	page, err := mem.Ledger(f.ctx, rows[0].Seq, 2)
	require.NoError(t, err)
	require.Equal(t, []int64{rows[1].Seq, rows[2].Seq}, []int64{page[0].Seq, page[1].Seq})

	for _, data := range []any{"words", []string{"a"}, 3} {
		err := mem.Apply(f.ctx, store.MemoryChange{Ledger: []store.LedgerItem{{Data: data}}})
		userErr(t, err, store.ErrInvalid, "data")
	}
	require.Len(t, f.ledger(), 4)
	seq, err = mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Zero(t, seq)

	_, err = f.db.SQL().ExecContext(f.ctx, `UPDATE ledger SET data = '{}' WHERE seq = ?`, rows[1].Seq)
	require.ErrorContains(t, err, "append-only")
	_, err = f.db.SQL().ExecContext(f.ctx, `DELETE FROM ledger WHERE seq = ?`, rows[3].Seq)
	require.ErrorContains(t, err, "append-only")

	// With the guard taken away, a changed row no longer verifies.
	_, err = f.db.SQL().ExecContext(f.ctx, `DROP TRIGGER ledger_no_update`)
	require.NoError(t, err)
	_, err = f.db.SQL().ExecContext(f.ctx, `UPDATE ledger SET data = '{"action":"claim.forget"}' WHERE seq = ?`,
		rows[2].Seq)
	require.NoError(t, err)
	seq, err = mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Equal(t, rows[2].Seq, seq)
	_, err = f.db.SQL().ExecContext(f.ctx, `UPDATE ledger SET data = ?, prev_hash = ? WHERE seq = ?`,
		string(rows[2].Data), strings.Repeat("f", 64), rows[2].Seq)
	require.NoError(t, err)
	seq, err = mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Equal(t, rows[2].Seq, seq, "a row must chain to the one before")
}

func TestEraseAll(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem, state := f.r.Memory, f.r.MemoryState
	th, msgs, err := f.r.Chats.Append(f.ctx, "", []model.ChatMessage{
		say(model.ChatUser, "I work at Quuxcorp now", at("09:00")),
		say(model.ChatAssistant, "Noted.", at("09:01")),
	})
	require.NoError(t, err)
	anna := model.Entity{ID: model.NewID(), Kind: "person", Name: "Annabeth Quill", Aliases: []string{"Quillie"}}
	work := fact("user.employer", "Works at Quuxcorp", `"Quuxcorp"`)
	work.SourceID = msgs[0].ID
	gone := fact("user.home", "Lives in Wobbleton", `"Wobbleton"`)
	sister := fact("person.relation", "Annabeth is their sister", `"sister"`)
	sister.SubjectID = &anna.ID
	f.apply(store.MemoryChange{Entities: []model.Entity{anna}, Claims: []model.Claim{work, gone, sister},
		Conflicts: []model.Conflict{{ClaimA: work.ID, ClaimB: sister.ID, Reason: "Quuxcorp or not",
			Sensitivity: model.SensitivityPersonal}},
		Links: []model.MemoryLink{{FromKind: "claim", FromID: sister.ID, ToKind: "entity", ToID: anna.ID,
			Relation: "about"}},
		Jobs: []store.NewJob{{Kind: model.JobProject, Subject: "claim:" + work.ID}}})
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: gone.ID, Forget: true}}})
	_, err = mem.AddEpisode(f.ctx, note("My locker code story at Flibberton gym", at("10:00")))
	require.NoError(t, err)
	_, err = state.AddResource(f.ctx, model.Resource{Title: "Lease Grimsditch.pdf", Path: "/home/me/lease.pdf",
		Size: 10}, model.Episode{Channel: model.ChannelDoc, Speaker: model.SpeakerExternal, Body: "Rent is due monthly",
		At: at("10:00"), Sensitivity: model.SensitivityPersonal})
	require.NoError(t, err)
	require.NoError(t, state.AddNote(f.ctx, model.ChatNote{MessageID: msgs[0].ID, ThreadID: th.ID,
		Note: "[m1] Works at Quuxcorp", Refs: map[string]string{"m1": "claim:" + work.ID}}))
	require.NoError(t, state.SetObservation(f.ctx, model.ChatObservation{ThreadID: th.ID, ThroughID: msgs[1].ID,
		ThroughAt: at("09:01"), Log: "- 2026-09-15 [high] Works at Quuxcorp"}))
	_, err = state.AddProfile(f.ctx, model.ProfileCard{Day: "2026-09-20", Text: "- [p1] Works at Quuxcorp",
		Refs: map[string]string{"p1": "claim:" + work.ID}})
	require.NoError(t, err)
	require.NoError(t, state.SetBackend(f.ctx, model.MemoryBackend{Backend: "mem0", ItemID: work.ID,
		ExternalID: "x1", Status: model.BackendSynced}))
	require.NoError(t, state.TouchClaims(f.ctx, []string{work.ID}, fxNow))

	erased, err := mem.EraseAll(f.ctx, store.LedgerItem{Data: map[string]string{"by": "test"}})
	require.NoError(t, err)
	require.Equal(t, store.Erased{Claims: 2, Episodes: 2, Entities: 1, Resources: 1, Conflicts: 1, Links: 1}, erased,
		"live rows are counted; the claim forgotten before is not")

	list, err := mem.Claims(f.ctx, store.ClaimFilter{IncludeSuppressed: true})
	require.NoError(t, err)
	require.Empty(t, list)
	counts, err := mem.CountClaims(f.ctx)
	require.NoError(t, err)
	require.Empty(t, counts)
	byHash, err := mem.ClaimsByHash(f.ctx, work.ContentHash)
	require.NoError(t, err)
	require.Len(t, byHash, 1, "hashes stay")
	entities, err := mem.Entities(f.ctx, "")
	require.NoError(t, err)
	require.Empty(t, entities)
	conflicts, err := mem.Conflicts(f.ctx, "")
	require.NoError(t, err)
	require.Empty(t, conflicts)
	links, err := mem.LinksFrom(f.ctx, sister.ID)
	require.NoError(t, err)
	require.Empty(t, links)
	eps, err := mem.EpisodesAfter(f.ctx, 0, 0)
	require.NoError(t, err)
	require.Empty(t, eps)
	notes, err := state.Notes(f.ctx, th.ID)
	require.NoError(t, err)
	require.Empty(t, notes)
	_, err = state.Observation(f.ctx, th.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = state.LatestProfile(f.ctx)
	require.ErrorIs(t, err, store.ErrNotFound)
	jobs, err := state.JobCounts(f.ctx)
	require.NoError(t, err)
	require.Empty(t, jobs)
	backends, err := state.Backends(f.ctx, "mem0", "")
	require.NoError(t, err)
	require.Empty(t, backends)
	usage, err := state.Usage(f.ctx, []string{work.ID})
	require.NoError(t, err)
	require.Empty(t, usage)
	docs, err := state.Resources(f.ctx)
	require.NoError(t, err)
	require.Empty(t, docs)
	require.Zero(t, f.count(`SELECT count(*) FROM resources`), "resources are deleted")
	require.Zero(t, f.count(`SELECT count(*) FROM entities WHERE name <> '' OR aliases <> '[]'`))
	require.Zero(t, f.count(`SELECT count(*) FROM conflicts WHERE reason <> ''`))

	ledger := f.ledger()
	require.JSONEq(t, `{"action": "memory.erase_all", "by": "test", "claims": 2, "episodes": 2, "entities": 1,
		"conflicts": 1, "links": 1, "resources": 1}`, string(ledger[len(ledger)-1].Data))
	seq, err := mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Zero(t, seq)

	kept, err := f.r.Chats.Messages(f.ctx, th.ID)
	require.NoError(t, err)
	require.Len(t, kept, 2, "chats are kept")
	for _, word := range []string{"Annabeth", "Quillie", "Wobbleton", "Flibberton", "Grimsditch", "Rent is due",
		"Works at Quuxcorp", "Quuxcorp or not"} {
		require.False(t, f.dbFileHas(word), word)
	}
	require.True(t, f.dbFileHas("I work at Quuxcorp now"), "the chat's words are still there")
	again, err := mem.EraseAll(f.ctx, store.LedgerItem{})
	require.NoError(t, err)
	require.Equal(t, store.Erased{}, again)
}

func TestMemoryRepos(t *testing.T) {
	t.Parallel()
	db := testutil.NewDB(t)
	r := store.NewRepos(db)
	require.NotNil(t, r.Memory)
	require.NotNil(t, r.MemoryState)
	require.NotNil(t, r.Self)
}

// storedNote returns the note stored with a message.
func (f *fx) storedNote(threadID, messageID string) model.ChatNote {
	f.t.Helper()
	notes, err := f.r.MemoryState.Notes(f.ctx, threadID)
	require.NoError(f.t, err)
	n, ok := notes[messageID]
	require.True(f.t, ok, "a note for %s", messageID)
	return n
}

// Forgetting a claim takes its words out of everything made from it, in the
// same change: notes, profile cards, conflicts, and observation logs.
func TestForgetErasesWordsEverywhere(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	mem, state := f.r.Memory, f.r.MemoryState
	a, aMsgs := f.thread("Where do I work?")
	b, bMsgs := f.thread("Remind me what I do")

	job := fact("user.employer", "Works at Quuxcorp as a gardener", `"Quuxcorp"`)
	job.Span = "I garden at Quuxcorp"
	tea := fact("pref.drink", "Likes green tea", `"green tea"`)
	other := fact("user.employer", "Works at Flimco", `"Flimco"`)
	before := fact("user.employer", "Worked at Dorpville", `"Dorpville"`)
	keepA := model.ResolveKeepA
	open := model.Conflict{ID: model.NewID(), ClaimA: job.ID, ClaimB: other.ID, Reason: "Quuxcorp or Flimco",
		Sensitivity: model.SensitivityPersonal}
	settled := model.Conflict{ID: model.NewID(), ClaimA: before.ID, ClaimB: job.ID, Reason: "Quuxcorp twice",
		Sensitivity: model.SensitivityPersonal, Status: model.ConflictResolved, Resolution: &keepA}
	unrelated := model.Conflict{ID: model.NewID(), ClaimA: tea.ID, ClaimB: other.ID, Reason: "tea at Flimco",
		Sensitivity: model.SensitivityPersonal}
	f.apply(store.MemoryChange{Claims: []model.Claim{job, tea, other, before},
		Conflicts: []model.Conflict{open, settled, unrelated}})

	jobRef, teaRef := "claim:"+job.ID, "claim:"+tea.ID
	aNote := model.ChatNote{MessageID: aMsgs[0].ID, ThreadID: a.ID,
		Note: "What you remember:\n[m1] Works at Quuxcorp as a gardener (since Jun 2026)\n[m2] Likes green tea\n[m10] x",
		Refs: map[string]string{"m1": jobRef, "m2": teaRef, "m10": "msg:" + model.NewID()}}
	bNote := model.ChatNote{MessageID: bMsgs[0].ID, ThreadID: b.ID, Note: "[m4] Works at Quuxcorp as a gardener",
		Refs: map[string]string{"m4": jobRef}}
	require.NoError(t, state.AddNote(f.ctx, aNote))
	require.NoError(t, state.AddNote(f.ctx, bNote))
	_, err := state.AddProfile(f.ctx, model.ProfileCard{Day: "2026-09-19", Text: "- [p1] Likes green tea",
		Refs: map[string]string{"p1": teaRef}})
	require.NoError(t, err)
	_, err = state.AddProfile(f.ctx, model.ProfileCard{Day: "2026-09-20",
		Text: "- [p1] Works at Quuxcorp as a gardener\n- [p2] Likes green tea", Refs: map[string]string{"p1": jobRef,
			"p2": teaRef}})
	require.NoError(t, err)
	require.NoError(t, state.SetObservation(f.ctx, model.ChatObservation{ThreadID: a.ID, ThroughID: aMsgs[1].ID,
		ThroughAt: at("09:01"), Log: "- 2026-09-15 [high] Said they started at QUUXCORP\n" +
			"- 2026-09-15 [low] Talked about tea\n- 2026-09-15 [high] Said: i garden at quuxcorp, mostly"}))
	require.NoError(t, state.SetObservation(f.ctx, model.ChatObservation{ThreadID: b.ID, ThroughID: bMsgs[1].ID,
		ThroughAt: at("09:01"), Log: "- 2026-09-15 [high] works at quuxcorp as a gardener\n- 2026-09-15 [low] ok"}))

	f.clk.Advance(time.Minute)
	now := fxNow.Add(time.Minute)
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: job.ID, Forget: true}}})

	n := f.storedNote(a.ID, aMsgs[0].ID)
	require.Equal(t, "What you remember:\n[m1] (forgotten)\n[m2] Likes green tea\n[m10] x", n.Note,
		"only the forgotten claim's line loses its words")
	require.Equal(t, aNote.Refs, n.Refs, "the refs stay")
	require.Equal(t, "[m4] (forgotten)", f.storedNote(b.ID, bMsgs[0].ID).Note, "in every chat")

	card, err := state.LatestProfile(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "2026-09-19", card.Day, "a card citing the claim is deleted")
	require.Equal(t, 1, f.count(`SELECT count(*) FROM profile_cards`))

	got, err := mem.Conflict(f.ctx, open.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictDismissed, got.Status)
	require.Equal(t, "", got.Reason)
	require.Equal(t, now, got.UpdatedAt)
	require.Equal(t, int64(2), got.Rev)
	got, err = mem.Conflict(f.ctx, settled.ID)
	require.NoError(t, err)
	require.Equal(t, model.ConflictResolved, got.Status, "a settled conflict stays settled")
	require.Equal(t, "", got.Reason)
	require.Equal(t, int64(2), got.Rev)
	got, err = mem.Conflict(f.ctx, unrelated.ID)
	require.NoError(t, err)
	require.Equal(t, "tea at Flimco", got.Reason)
	require.Equal(t, int64(1), got.Rev)
	ledger := f.ledger()
	require.Len(t, ledger, 1)
	require.JSONEq(t, `{"action": "conflict.dismiss", "conflict": "`+open.ID+`", "reason": "claim forgotten"}`,
		string(ledger[0].Data))

	o, err := state.Observation(f.ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, "- 2026-09-15 [low] Talked about tea", o.Log, "lines holding the value or the span go")
	require.Equal(t, now, o.UpdatedAt)
	o, err = state.Observation(f.ctx, b.ID)
	require.NoError(t, err)
	require.Equal(t, "- 2026-09-15 [low] ok", o.Log, "a line holding the text goes")

	for _, word := range []string{"Quuxcorp", "quuxcorp", "QUUXCORP", "gardener", "I garden at"} {
		require.False(t, f.dbFileHas(word), word)
	}
	require.True(t, f.dbFileHas("Likes green tea"))
	seq, err := mem.VerifyLedger(f.ctx)
	require.NoError(t, err)
	require.Zero(t, seq)
}

// Short words are not looked for in observation logs: they would drop lines
// that only happen to hold them.
func TestForgetKeepsLinesForShortWords(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	th, msgs := f.thread("Hi")
	short := fact("user.city", "In Rome", `"Rome"`)
	short.Span = "Rome"
	tiny := fact("user.nick", "Is Al", `"Al"`)
	tiny.Span = "I'm Al"
	f.apply(store.MemoryChange{Claims: []model.Claim{short, tiny}})
	log := "- 2026-09-15 [high] Flying to Rome\n- 2026-09-15 [low] Al said hi"
	require.NoError(t, f.r.MemoryState.SetObservation(f.ctx, model.ChatObservation{ThreadID: th.ID,
		ThroughID: msgs[1].ID, ThroughAt: at("09:01"), Log: log}))
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: tiny.ID, Forget: true}}})
	o, err := f.r.MemoryState.Observation(f.ctx, th.ID)
	require.NoError(t, err)
	require.Equal(t, log, o.Log, "a value under 4, a span under 8, and a text under 12 characters drop nothing")
	f.apply(store.MemoryChange{Updates: []store.ClaimUpdate{{ID: short.ID, Forget: true}}})
	o, err = f.r.MemoryState.Observation(f.ctx, th.ID)
	require.NoError(t, err)
	require.Equal(t, "- 2026-09-15 [low] Al said hi", o.Log, "a value of 4 does")
	require.Equal(t, fxNow, o.UpdatedAt)
}
