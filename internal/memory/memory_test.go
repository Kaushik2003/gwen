package memory

import (
	"encoding/json"
	"testing"

	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

func TestParseRef(t *testing.T) {
	id := model.NewID()
	for ref, want := range map[string][2]string{
		"claim:" + id:           {RefClaim, id},
		"msg:" + id:             {RefMessage, id},
		"ep:" + id:              {RefEpisode, id},
		"doc:" + id + "#c12":    {RefResource, id},
		"data:worked_last_week": {RefData, "worked_last_week"},
	} {
		kind, got, ok := ParseRef(ref)
		require.True(t, ok, ref)
		require.Equal(t, want, [2]string{kind, got}, ref)
	}
	for _, ref := range []string{"", "claim", "claim:", "claim:short", "task:" + id, id} {
		_, _, ok := ParseRef(ref)
		require.False(t, ok, ref)
	}
	require.Equal(t, "claim:"+id, Ref(RefClaim, id))
}

func TestSpec(t *testing.T) {
	s, ok := Spec("user.employer")
	require.True(t, ok)
	require.Equal(t, model.CardinalityOne, s.Cardinality)
	require.Equal(t, SubjectUser, s.Subject)

	s, ok = Spec("pref.music.focus")
	require.True(t, ok)
	require.Equal(t, model.CardinalityMany, s.Cardinality)
	require.Equal(t, "Focus", s.Label)

	s, ok = Spec("person.nickname")
	require.True(t, ok)
	require.Equal(t, SubjectEntity, s.Subject)
	require.Equal(t, EntityPerson, EntityKindOf("person.nickname"))
	require.Equal(t, "", EntityKindOf("user.home"))

	s, ok = Spec("user.health.allergy")
	require.True(t, ok)
	require.True(t, s.Sensitive)

	for _, k := range []string{"", "user", "employer", "secret.password", "User.employer", "user..x", "a.b.c.d.e.f.g"} {
		_, ok := Spec(k)
		require.False(t, ok, k)
	}
}

func TestQuoteAndHash(t *testing.T) {
	require.True(t, ContainsQuote("I’ve  started at\nGlobex — finally!", "I've started at Globex"))
	require.True(t, ContainsQuote("Mein Name ist Jürgen.", "ist jürgen"))
	require.True(t, ContainsQuote("東京で働いています", "東京で"))
	require.False(t, ContainsQuote("I work at Acme", "I work at Globex"))
	require.False(t, ContainsQuote("anything", " . "))

	a := ClaimHash(nil, "user.employer", json.RawMessage(`"Acme"`))
	require.Equal(t, a, ClaimHash(nil, "user.employer", json.RawMessage(`"  acme "`)))
	require.NotEqual(t, a, ClaimHash(nil, "user.employer", json.RawMessage(`"Globex"`)))
	sub := model.NewID()
	require.NotEqual(t, a, ClaimHash(&sub, "user.employer", json.RawMessage(`"Acme"`)))
}

func TestTokensAndCut(t *testing.T) {
	require.Equal(t, 0, Tokens(""))
	require.Equal(t, 3, Tokens("hello world"))
	require.Equal(t, 9, Tokens("東京で働いています"))
	require.Equal(t, "hello world", Cut("hello world", 20))
	require.Equal(t, "the quick brown…", Cut("the quick brown fox jumps", 18))
}

func TestTrust(t *testing.T) {
	require.Equal(t, 0, Tier(model.TrustOwnerEdit))
	require.Equal(t, 4, Tier(model.TrustExternal))
	require.Equal(t, -1, Tier("nope"))
	require.Equal(t, model.SensitivitySensitive, MoreSensitive(model.SensitivityPersonal, model.SensitivitySensitive))
	require.Equal(t, model.SensitivityPersonal, MoreSensitive(model.SensitivityPersonal, model.SensitivityPublic))
}
