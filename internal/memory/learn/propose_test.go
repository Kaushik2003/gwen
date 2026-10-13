package learn

import (
	"errors"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

func TestProposeCommitsAUserFact(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "I just started working at Acme as a backend intern", t0.Add(-time.Hour))
	res := f.propose(from(fact("user.employer", "Acme", "Works at Acme", "working at Acme"), msg))
	c := f.only("user.employer")
	require.Equal(t, Result{Claims: []string{c.ID}, Learned: 1}, res)
	require.Equal(t, model.ClaimActive, c.Status)
	require.Equal(t, model.TrustOwnerSaid, c.Trust)
	require.Equal(t, model.CardinalityOne, c.Cardinality)
	require.Equal(t, model.SensitivityPersonal, c.Sensitivity)
	require.JSONEq(t, `"Acme"`, string(c.Value))
	require.Equal(t, "working at Acme", c.Span)
	require.Equal(t, msg.ID, c.SourceID)
	require.Equal(t, model.SourceMessage, c.SourceKind)
	require.Equal(t, msg.At, c.MentionedAt)
	require.Equal(t, "fake/model#m1", c.Extractor)
	require.Equal(t, memory.ClaimHash(nil, "user.employer", jv("Acme")), c.ContentHash)
	require.Equal(t, []string{"claim.commit"}, f.actions())
	require.Equal(t, []string{"project claim:" + c.ID, "files "}, f.jobs())
}

func TestProposeChecksKeyAndSubject(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "My sister Anna works at Acme and her birthday is 3 May", t0)
	anna := fact("person.relation", "sister", "Anna is their sister", "My sister Anna")
	anna.Subject = "person:Anna"
	f.propose(from(anna, msg))
	entities, err := f.r.Memory.Entities(f.ctx, "")
	require.NoError(t, err)
	require.Len(t, entities, 1)
	refs := map[string]model.Entity{"e1": entities[0]}

	bad := []llm.ExtractedFact{
		fact("nonsense", "x", "Nonsense", "Anna"),             // not a key
		fact("Weird.Key!", "x", "Weird", "Anna"),              // not a key
		fact("person.birthday", "3 May", "Birthday", "3 May"), // about the user, but a person key
		{Key: "user.employer", Subject: "person:Anna", Value: jv("Acme"), Text: "Works at Acme", Quote: "works at Acme"},
		{Key: "person.employer", Subject: "org:Acme", Value: jv("Acme"), Text: "Works at Acme", Quote: "works at Acme"},
		{Key: "org.fact", Subject: "e1", Value: jv("x"), Text: "Anna is an org", Quote: "Anna"}, // e1 is a person
		{Key: "person.fact", Subject: "Anna", Value: jv("x"), Text: "Anna", Quote: "Anna"},      // not kind:Name
	}
	for _, b := range bad {
		c := from(b, msg)
		c.Entities = refs
		res := f.propose(c)
		require.Zero(t, res.Learned, "%+v", b)
	}
	// A ref and a name that matches, ignoring case, find the same person.
	byRef := llm.ExtractedFact{Key: "person.birthday", Subject: "e1", Value: jv("2026-05-03"),
		Text: "Anna's birthday is 3 May", Quote: "her birthday is 3 May"}
	byName := llm.ExtractedFact{Key: "person.employer", Subject: "person:anna", Value: jv("Acme"),
		Text: "Anna works at Acme", Quote: "Anna works at Acme"}
	c1, c2 := from(byRef, msg), from(byName, msg)
	c1.Entities = refs
	res := f.propose(c1, c2)
	require.Equal(t, 2, res.Learned)
	for _, c := range f.claims() {
		require.Equal(t, entities[0].ID, *c.SubjectID)
		links, err := f.r.Memory.LinksFrom(f.ctx, c.ID)
		require.NoError(t, err)
		require.Len(t, links, 1)
		require.Equal(t, "about", links[0].Relation)
		require.Equal(t, entities[0].ID, links[0].ToID)
	}
	entities, err = f.r.Memory.Entities(f.ctx, "")
	require.NoError(t, err)
	require.Len(t, entities, 1)
}

func TestProposeChecksTheValue(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "I like jazz", t0)
	for _, v := range []string{`null`, `"  "`, `{bad`, ``} {
		fct := fact("pref.music", "", "Likes jazz", "like jazz")
		fct.Value = []byte(v)
		require.Zero(t, f.propose(from(fct, msg)).Learned, v)
	}
	empty := fact("pref.music", "jazz", "   ", "like jazz")
	require.Zero(t, f.propose(from(empty, msg)).Learned)
	padded := fact("pref.music", "  jazz ", "Likes jazz", "like jazz")
	f.propose(from(padded, msg))
	require.JSONEq(t, `"jazz"`, string(f.only("pref.music").Value))
}

func TestProposeScreensSecrets(t *testing.T) {
	f := newFx(t)
	text := "my card is 4111 1111 1111 1111, my password is hunter22, iban DE89 3704 0044 0532 0130 00, " +
		"key sk-abcdefghijklmnop1234 and pin: 4821"
	msg := f.say("", model.ChatUser, text, t0)
	for _, q := range []string{"my card is 4111 1111 1111 1111", "my password is hunter22",
		"iban DE89 3704 0044 0532 0130 00", "key sk-abcdefghijklmnop1234", "pin: 4821"} {
		require.Zero(t, f.propose(from(fact("user.fact", "x", "Has a secret", q), msg)).Learned, q)
	}
	marked := fact("user.fact", "card", "Has a card", "my card")
	marked.Sensitivity = "secret"
	require.Zero(t, f.propose(from(marked, msg)).Learned)
	require.Empty(t, f.claims())
	// The health keys are sensitive whatever the model says.
	ill := f.say("", model.ChatUser, "I have asthma", t0)
	health := fact("user.health", "asthma", "Has asthma", "I have asthma")
	health.Sensitivity = "public"
	f.propose(from(health, ill))
	require.Equal(t, model.SensitivitySensitive, f.only("user.health").Sensitivity)
}

func TestHasSecret(t *testing.T) {
	for s, want := range map[string]bool{
		"4111111111111111":                      true,
		"4111-1111-1111-1111":                   true,
		"call me on +1 415 555 2671":            false,
		"4111 1111 1111 1112":                   false, // fails the Luhn check
		"GB82 WEST 1234 5698 7654 32":           true,
		"GB82 WEST 1234 5698 7654 33":           false, // wrong check digits
		"my password is hunter2":                true,
		"Passwort: geheim123":                   true,
		"the wifi password is in the drawer":    false,
		"my password isn't working":             false,
		"uses a password manager":               false,
		"pin the tab":                           false,
		"PIN 4821":                              true,
		"ghp_abcdefghijklmnopqrstuvwxyz0123":    true,
		"xoxb-1234-abcdefgh":                    true,
		"AKIAIOSFODNN7EXAMPLE":                  true,
		"d41d8cd98f00b204e9800998ecf8427e":      true,
		"born on 2001-04-03, moved in 2019":     false,
		"Works at Acme as a backend intern":     false,
		"/home/kzark/Documents/coding/projects": false,
	} {
		require.Equal(t, want, hasSecret(s), s)
	}
}

func TestProposeNeedsTheQuote(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "I work at Acme", t0)
	require.Zero(t, f.propose(from(fact("user.employer", "Globex", "Works at Globex", "work at Globex"), msg)).Learned)
	require.Zero(t, f.propose(from(fact("user.employer", "Acme", "Works at Acme", ""), msg)).Learned)
	// Words compared after folding case, quotes, and spaces.
	require.Equal(t, 1, f.propose(from(fact("user.employer", "Acme", "Works at Acme", "i  WORK at acme."), msg)).Learned)
}

func TestProposeTrustBySpeaker(t *testing.T) {
	f := newFx(t)
	thread := f.say("", model.ChatUser, "I love hiking", t0)
	gwen := f.say(thread.ThreadID, model.ChatAssistant, "You seem to enjoy climbing too", t0.Add(time.Minute))
	f.propose(from(fact("pref.likes", "hiking", "Likes hiking", "love hiking"), thread))
	f.propose(from(fact("pref.likes", "climbing", "Likes climbing", "enjoy climbing"), gwen))
	doc := Candidate{Fact: fact("pref.likes", "sailing", "Likes sailing", "sailing"), SourceKind: model.SourceEpisode,
		SourceID: model.NewID(), SourceText: "He likes sailing", Speaker: model.SpeakerExternal,
		Channel: model.ChannelDoc, At: t0}
	f.propose(doc)
	edit := doc
	edit.Fact = fact("pref.likes", "chess", "Likes chess", "chess")
	edit.SourceText, edit.Trust = "chess", model.TrustOwnerEdit
	f.propose(edit)
	got := map[string][2]string{}
	for _, c := range f.claims() {
		got[c.Text] = [2]string{c.Status, c.Trust}
	}
	require.Equal(t, map[string][2]string{
		"Likes hiking":   {model.ClaimActive, model.TrustOwnerSaid},
		"Likes climbing": {model.ClaimCandidate, model.TrustInferred},
		"Likes sailing":  {model.ClaimQuarantined, model.TrustExternal},
		"Likes chess":    {model.ClaimActive, model.TrustOwnerEdit},
	}, got)
}

func TestProposeRemembersWhatWasRejected(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "I work at Acme", t0)
	cand := from(fact("user.employer", "Acme", "Works at Acme", "work at Acme"), msg)
	f.propose(cand)
	c := f.only("user.employer")
	_, err := f.l.Reject(f.ctx, c.ID)
	require.NoError(t, err)
	// The same words do not teach it again.
	require.Zero(t, f.propose(cand).Learned)
	// Nor once it is forgotten.
	f.clk.Advance(time.Minute)
	again := f.say(msg.ThreadID, model.ChatUser, "I work at Acme, really", t0.Add(time.Minute))
	f.propose(from(fact("user.employer", "Acme", "Works at Acme", "work at Acme"), again))
	latest := f.claims(model.ClaimActive)
	require.Len(t, latest, 1)
	_, err = f.l.Forget(f.ctx, latest[0].ID)
	require.NoError(t, err)
	require.Zero(t, f.propose(from(fact("user.employer", "Acme", "Works at Acme", "work at Acme"), again)).Learned)
}

func TestProposeCorroboratesACandidate(t *testing.T) {
	f := newFx(t)
	gwen := f.say("", model.ChatAssistant, "So you live in Berlin?", t0)
	f.propose(from(fact("user.home", "Berlin", "Lives in Berlin", "live in Berlin"), gwen))
	cand := f.only("user.home")
	require.Equal(t, model.ClaimCandidate, cand.Status)
	user := f.say(gwen.ThreadID, model.ChatUser, "Yes, I live in Berlin", t0.Add(time.Minute))
	res := f.propose(from(fact("user.home", "Berlin", "Lives in Berlin", "I live in Berlin"), user))
	require.Equal(t, []string{cand.ID}, res.Claims)
	require.Zero(t, res.Learned)
	c := f.only("user.home")
	require.Equal(t, cand.ID, c.ID)
	require.Equal(t, model.ClaimActive, c.Status)
	links, err := f.r.Memory.LinksFrom(f.ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, "evidence", links[0].Relation)
	require.Equal(t, "message", links[0].ToKind)
	require.Equal(t, user.ID, links[0].ToID)
	require.Equal(t, []string{"claim.commit", "claim.activate"}, f.actions())
	// Said again by the user, it is only more evidence.
	again := f.say(gwen.ThreadID, model.ChatUser, "I live in Berlin, as I said", t0.Add(2*time.Minute))
	res = f.propose(from(fact("user.home", "Berlin", "Lives in Berlin", "live in Berlin"), again))
	require.Empty(t, res.Claims)
	links, err = f.r.Memory.LinksFrom(f.ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, links, 2)
	// The same fact twice in one call is one claim.
	user2 := f.say(gwen.ThreadID, model.ChatUser, "I speak German", t0.Add(3*time.Minute))
	lang := fact("user.language", "German", "Speaks German", "speak German")
	require.Equal(t, 1, f.propose(from(lang, user2), from(lang, user2)).Learned)
}

func TestProposeChecksEntailment(t *testing.T) {
	f := newFx(t)
	f.verify = true
	msg := f.say("", model.ChatUser, "I like jazz and rock, maybe techno, and I own a cat", t0)
	sure := fact("pref.music", "jazz", "Likes jazz", "like jazz")
	partly := fact("pref.music", "rock", "Likes rock", "rock")
	weak := fact("pref.music", "techno", "Likes techno", "maybe techno")
	weak.Confidence = 0.4
	wrong := fact("user.pet", "dog", "Has a dog", "own a cat")
	edit := from(fact("user.pet", "cat", "Has a cat", "own a cat"), msg)
	edit.Trust = model.TrustOwnerEdit
	f.m.verify = func(req llm.VerifyRequest) (llm.VerifyOutput, error) {
		require.Len(t, req.Checks, 4)
		require.Equal(t, llm.VerifyCheck{Ref: "v1", Claim: "Likes jazz", Quote: "like jazz", Speaker: "user",
			Context: msg.Text}, req.Checks[0])
		return llm.VerifyOutput{Verdicts: []llm.Verdict{{Ref: "v1", Supported: "yes"}, {Ref: "v2", Supported: "partly"},
			{Ref: "v3", Supported: "partly"}, {Ref: "v4", Supported: "no"}}}, nil
	}
	res := f.propose(from(sure, msg), from(partly, msg), from(weak, msg), from(wrong, msg), edit)
	require.Equal(t, 3, res.Learned)
	got := map[string]float64{}
	for _, c := range f.claims() {
		got[c.Text] = c.Confidence
	}
	require.InDeltaMapValues(t, map[string]float64{"Likes jazz": 0.9, "Likes rock": 0.54, "Has a cat": 1}, got, 1e-9)

	// A failed check fails the whole proposal.
	f.m.verify = func(llm.VerifyRequest) (llm.VerifyOutput, error) { return llm.VerifyOutput{}, errors.New("down") }
	_, err := f.l.Propose(f.ctx, []Candidate{from(fact("user.pet", "fish", "Has a fish", "a cat"), msg)})
	require.ErrorContains(t, err, "down")
	require.Len(t, f.claims(), 3)

	// A fact with no quote takes the words the check found, which must be
	// the source's.
	f.m.verify = func(req llm.VerifyRequest) (llm.VerifyOutput, error) {
		return llm.VerifyOutput{Verdicts: []llm.Verdict{{Ref: "v1", Supported: "yes", Quote: "own a cat"},
			{Ref: "v2", Supported: "yes", Quote: "own a dog"}}}, nil
	}
	shadow := from(fact("user.pet", "a cat", "Owns a cat", ""), msg)
	shadow.Extractor = "mem0"
	madeUp := from(fact("user.pet", "dog", "Owns a dog", ""), msg)
	require.Equal(t, 1, f.propose(shadow, madeUp).Learned)
	var mem0 model.Claim
	for _, c := range f.claims() {
		if c.Extractor == "mem0" {
			mem0 = c
		}
	}
	require.Equal(t, "own a cat", mem0.Span)
	require.Equal(t, model.ClaimActive, mem0.Status)
}

func TestProposeSupersedesOneValuedClaims(t *testing.T) {
	cases := []struct {
		name                 string
		oldFrom, newFrom     string
		oldSaid, newSaid     time.Time
		oldStatus, newStatus string
		conflict             bool
	}{
		{"newer in both supersedes", "", "", t0.Add(-48 * time.Hour), t0, model.ClaimSuperseded, model.ClaimActive, false},
		{"older in both is a backfill", "2026-09-01", "2026-01-01", t0, t0.Add(-time.Hour), model.ClaimActive,
			model.ClaimSuperseded, false},
		{"orders disagree", "2026-09-01", "2026-01-01", t0.Add(-time.Hour), t0, model.ClaimActive, model.ClaimActive,
			true},
		{"both equal", "", "", t0, t0, model.ClaimActive, model.ClaimActive, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx(t)
			old := f.say("", model.ChatUser, "I work at Acme", tc.oldSaid)
			oldFact := fact("user.employer", "Acme", "Works at Acme", "work at Acme")
			oldFact.ValidFrom = tc.oldFrom
			f.propose(from(oldFact, old))
			oldClaim := f.only("user.employer")
			f.clk.Advance(time.Minute)
			msg := f.say(old.ThreadID, model.ChatUser, "I work at Globex", tc.newSaid)
			newFact := fact("user.employer", "Globex", "Works at Globex", "work at Globex")
			newFact.ValidFrom = tc.newFrom
			res := f.propose(from(newFact, msg))
			require.Equal(t, tc.conflict, res.Conflicts)
			var newClaim model.Claim
			for _, c := range f.claims() {
				if c.ID != oldClaim.ID {
					newClaim = c
				}
			}
			oldClaim = f.claim(oldClaim.ID)
			require.Equal(t, tc.oldStatus, oldClaim.Status)
			require.Equal(t, tc.newStatus, newClaim.Status)
			conflicts, err := f.r.Memory.Conflicts(f.ctx, model.ConflictOpen)
			require.NoError(t, err)
			switch {
			case tc.conflict:
				require.Len(t, conflicts, 1)
				require.Equal(t, oldClaim.ID, conflicts[0].ClaimA)
				require.Equal(t, newClaim.ID, conflicts[0].ClaimB)
				require.Equal(t, model.SensitivityPersonal, conflicts[0].Sensitivity)
				require.Contains(t, f.actions(), "conflict.open")
			case tc.oldStatus == model.ClaimSuperseded:
				require.Empty(t, conflicts)
				require.Equal(t, newClaim.ID, *oldClaim.SupersededBy)
				require.Equal(t, tc.newSaid, *oldClaim.ValidTo)
				require.Equal(t, []string{"claim.commit", "claim.commit", "claim.supersede"}, f.actions())
				require.ElementsMatch(t, []string{oldClaim.ID, newClaim.ID}, res.Claims)
			default:
				require.Empty(t, conflicts)
				require.Equal(t, oldClaim.ID, *newClaim.SupersededBy)
				require.Equal(t, day("2026-09-01"), *newClaim.ValidTo)
				require.Nil(t, oldClaim.ValidTo)
			}
		})
	}
}

func TestProposeStoresDates(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "I was in Rome from 2 to 9 August", t0)
	trip := fact("user.plan", "Rome", "Was in Rome", "in Rome from 2 to 9 August")
	trip.ValidFrom, trip.ValidTo, trip.OccurredAt = "2026-08-02", "2026-08-09", "2026-08-02T10:00:00Z"
	f.propose(from(trip, msg))
	c := f.only("user.plan")
	require.Equal(t, day("2026-08-02"), *c.ValidFrom)
	require.Equal(t, day("2026-08-10"), *c.ValidTo, "the last day is stored exclusive")
	require.Equal(t, day("2026-08-02").Add(10*time.Hour), *c.OccurredAt)
}

func TestProposeManyValuedClaims(t *testing.T) {
	f := newFx(t)
	msg := f.say("", model.ChatUser, "I like jazz and I like rock", t0)
	f.propose(from(fact("pref.music", "jazz", "Likes jazz", "like jazz"), msg),
		from(fact("pref.music", "rock", "Likes rock", "like rock"), msg))
	require.Len(t, f.claims(model.ClaimActive), 2)
	require.Empty(t, f.actionsOf("conflict.open"))
	// A fact that updates a known many-valued claim supersedes it.
	var rock model.Claim
	for _, c := range f.claims() {
		if c.Text == "Likes rock" {
			rock = c
		}
	}
	later := f.say(msg.ThreadID, model.ChatUser, "these days I prefer punk over rock", t0.Add(time.Hour))
	punk := fact("pref.music", "punk", "Prefers punk to rock", "prefer punk over rock")
	punk.Updates = "k1"
	cand := from(punk, later)
	cand.Known = map[string]string{"k1": rock.ID}
	res := f.propose(cand)
	require.Len(t, res.Claims, 2)
	rock = f.claim(rock.ID)
	require.Equal(t, model.ClaimSuperseded, rock.Status)
	require.Len(t, f.claims(model.ClaimActive), 2)
}

// actionsOf lists the ledger rows of one action.
func (f *fx) actionsOf(action string) []string {
	var out []string
	for _, a := range f.actions() {
		if a == action {
			out = append(out, a)
		}
	}
	return out
}
