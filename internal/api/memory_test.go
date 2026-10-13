package api

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestMemoryRoutesWithoutMemory(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, nil)
	calls := map[string]error{}
	_, calls["status"] = ts.c.MemoryStatus(ts.ctx)
	_, calls["claims"] = ts.c.ListClaims(ts.ctx, wire.MemoryClaimQuery{})
	_, calls["remember"] = ts.c.Remember(ts.ctx, wire.RememberRequest{Text: "I live in Pune"}, "")
	_, calls["forget all"] = ts.c.ForgetAllMemory(ts.ctx, client.ForgetAllConfirm)
	calls["rebuild"] = ts.c.RebuildMemory(ts.ctx, wire.RebuildRequest{Lane: "vec"})
	for name, err := range calls {
		ae := apiCode(t, err, wire.CodeUnavailable)
		require.Equal(t, memoryOff, ae.Message, name)
	}
	// Self is not memory: it works without one.
	hist, err := ts.c.AssistantSelfHistory(ts.ctx, 0)
	require.NoError(t, err)
	require.Empty(t, hist.Versions)
}

func TestMemoryDisabledIsUnavailable(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	mem.err = fmt.Errorf("%w: no model to learn with", memory.ErrDisabled)
	ts := newTestServer(t, mem)
	_, err := ts.c.SleepMemory(ts.ctx)
	ae := apiCode(t, err, wire.CodeUnavailable)
	require.Equal(t, memoryOff, ae.Message)
}

func TestMemoryClaims(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	ts := newTestServer(t, mem)

	list, err := ts.c.ListClaims(ts.ctx, wire.MemoryClaimQuery{})
	require.NoError(t, err)
	require.Len(t, list.Claims, 2)
	a := list.Claims[0]
	require.Equal(t, claimA, a.ID)
	require.Equal(t, "Works at", a.Label)
	require.JSONEq(t, `"Acme"`, string(a.Value))
	require.Equal(t, "msg:77777777-7777-4777-8777-777777777777", a.SourceRef)
	require.Equal(t, "I work at Acme", a.Quote)
	require.Equal(t, "chat “Work”, 15 Sep", a.Source)
	require.Equal(t, wire.Millis(testutil.T0.Add(-30*24*time.Hour)), *a.ValidFrom)
	require.True(t, list.Claims[1].Disputed)
	require.Empty(t, list.Claims[1].SourceRef, "an edit has no source to point at")
	f := mem.filters[0]
	require.Equal(t, []string{model.ClaimActive}, f.Statuses, "active by default")
	require.False(t, f.SubjectSet)
	require.False(t, f.IncludeSuppressed)
	require.Equal(t, DefaultClaims, f.Limit)

	_, err = ts.c.ListClaims(ts.ctx, wire.MemoryClaimQuery{Status: "all", Key: "user.", Subject: "user", Hidden: true,
		Limit: 5, Offset: 2})
	require.NoError(t, err)
	f = mem.filters[1]
	require.Empty(t, f.Statuses, "all is every status")
	require.Equal(t, "user.", f.KeyPrefix)
	require.True(t, f.SubjectSet)
	require.Nil(t, f.Subject, "user is the subject nil")
	require.True(t, f.IncludeSuppressed)
	require.Equal(t, 5, f.Limit)
	require.Equal(t, 2, f.Offset)

	_, err = ts.c.ListClaims(ts.ctx, wire.MemoryClaimQuery{Subject: entityAnna})
	require.NoError(t, err)
	require.Equal(t, entityAnna, *mem.filters[2].Subject)

	for _, bad := range []wire.MemoryClaimQuery{{Status: "maybe"}, {Limit: MaxClaims + 1}} {
		_, err = ts.c.ListClaims(ts.ctx, bad)
		apiCode(t, err, wire.CodeInvalidRequest)
	}

	got, err := ts.c.GetClaim(ts.ctx, claimA)
	require.NoError(t, err)
	require.Equal(t, "Works at Acme", got.Text)
	_, err = ts.c.GetClaim(ts.ctx, "nope")
	apiCode(t, err, wire.CodeNotFound)
	_, err = ts.c.GetClaim(ts.ctx, claimGone)
	apiCode(t, err, wire.CodeNotFound)

	v := "Globex"
	edited, err := ts.c.EditClaim(ts.ctx, claimA, wire.EditClaimRequest{Value: &v, ValidFrom: testutil.Ptr("2026-10-01")})
	require.NoError(t, err)
	require.Equal(t, claimB, edited.Claims[0].ID, "what the memory returned, with its view")
	require.Equal(t, "Lives in", edited.Claims[0].Label)
	require.Equal(t, "Globex", *mem.edits[0].Value)
	_, err = ts.c.EditClaim(ts.ctx, claimA, wire.EditClaimRequest{})
	apiCode(t, err, wire.CodeInvalidRequest)
	_, err = ts.c.EditClaim(ts.ctx, claimA, wire.EditClaimRequest{Text: "x", ValidTo: testutil.Ptr("soon")})
	apiCode(t, err, wire.CodeInvalidRequest)

	c, err := ts.c.ConfirmClaim(ts.ctx, claimB)
	require.NoError(t, err)
	require.Equal(t, wire.ClaimActive, c.Status)
	c, err = ts.c.RejectClaim(ts.ctx, claimB)
	require.NoError(t, err)
	require.Equal(t, wire.ClaimRejected, c.Status)
	c, err = ts.c.HideClaim(ts.ctx, claimA, wire.HideClaimRequest{Hidden: true})
	require.NoError(t, err)
	require.True(t, c.Hidden)
	_, err = ts.c.ConfirmClaim(ts.ctx, claimGone)
	apiCode(t, err, wire.CodeNotFound)

	rc, err := ts.c.ForgetClaim(ts.ctx, claimA)
	require.NoError(t, err)
	require.Equal(t, 1, rc.Claims)
	require.Equal(t, []wire.LaneReceipt{{Lane: "claims", OK: true},
		{Lane: "mem0", OK: false, Note: "Mem0 did not answer; it will be retried"}}, rc.Lanes)
}

func TestMemorySearchClaims(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	ts := newTestServer(t, mem)
	// The words are searched in the store: a claim there that the memory also lists is found.
	from := testutil.T0
	err := ts.Repos.Memory.Apply(ts.ctx, store.MemoryChange{Claims: []model.Claim{{ID: claimB, Key: "user.home", Value: []byte(`"Pune"`),
		Text: "Lives in Pune", Cardinality: model.CardinalityOne, Status: model.ClaimCandidate,
		Trust: model.TrustInferred, Sensitivity: model.SensitivityPersonal, Confidence: 0.5, MentionedAt: from,
		RecordedAt: from, SourceKind: model.SourceUserUI, Channel: model.ChannelUI, Extractor: "user",
		SourceDeviceID: ts.DB.DeviceID(), ContentHash: memory.Hash("pune")}}})
	require.NoError(t, err)
	list, err := ts.c.ListClaims(ts.ctx, wire.MemoryClaimQuery{Q: "pune", Status: "all"})
	require.NoError(t, err)
	require.Len(t, list.Claims, 1)
	require.Equal(t, claimB, list.Claims[0].ID)
	require.Zero(t, mem.filters[0].Limit, "the filter is read whole, then paged by search rank")
	list, err = ts.c.ListClaims(ts.ctx, wire.MemoryClaimQuery{Q: "pune", Status: "all", Offset: 1})
	require.NoError(t, err)
	require.Empty(t, list.Claims)
}

func TestMemoryRoutes(t *testing.T) {
	t.Parallel()
	mem := newFakeMemory()
	ts := newTestServer(t, mem)

	st, err := ts.c.MemoryStatus(ts.ctx)
	require.NoError(t, err)
	require.True(t, st.Enabled)
	require.Equal(t, map[string]int{"active": 2}, st.Claims)
	require.Equal(t, map[string]int{}, st.Jobs)
	require.Equal(t, []wire.MemoryLane{{Lane: "vec", Enabled: true, Items: -1, Detail: "down"}}, st.Lanes)
	require.Equal(t, wire.Millis(testutil.T0), *st.LastSleptAt)
	require.Nil(t, st.LastLearnedAt)
	require.True(t, st.LedgerOK)

	exp, err := ts.c.ExplainMemory(ts.ctx, "claim:"+claimA)
	require.NoError(t, err)
	require.Equal(t, "I work at Acme", exp.Quote)
	require.Equal(t, claimA, exp.Claim.ID)
	require.Len(t, exp.Context, 1)
	require.Len(t, exp.History, 1)
	require.NotNil(t, exp.Links)
	_, err = ts.c.ExplainMemory(ts.ctx, "claim:short")
	apiCode(t, err, wire.CodeInvalidRequest)

	res, err := ts.c.Remember(ts.ctx, wire.RememberRequest{Text: "  I work at Acme  "}, client.RememberCLI)
	require.NoError(t, err)
	require.Len(t, res.Claims, 1)
	require.Equal(t, "Works at", res.Claims[0].Label)
	_, err = ts.c.Remember(ts.ctx, wire.RememberRequest{Text: "Pune"}, "")
	require.NoError(t, err)
	require.Equal(t, []memory.RememberRequest{{Text: "I work at Acme", Channel: model.ChannelCLI},
		{Text: "Pune", Channel: model.ChannelUI}}, mem.remember)
	_, err = ts.c.Remember(ts.ctx, wire.RememberRequest{Text: " "}, "")
	apiCode(t, err, wire.CodeInvalidRequest)
	_, err = ts.c.Remember(ts.ctx, wire.RememberRequest{Text: "x"}, "phone")
	apiCode(t, err, wire.CodeInvalidRequest)

	rr, err := ts.c.RecallMemory(ts.ctx, wire.RecallRequest{Query: "where do I work", AsOf: "2026-09-01",
		Lanes: []string{"claims", "vec"}})
	require.NoError(t, err)
	require.Equal(t, "lookup", rr.Intent)
	require.Equal(t, []string{"vec"}, rr.Degraded)
	require.Equal(t, []string{"claims"}, rr.Hits[0].Lanes)
	q := mem.queries[0]
	require.Equal(t, time.Date(2026, 9, 1, 23, 59, 59, 999e6, time.UTC), q.AsOf, "as of a day is as of its end")
	require.Equal(t, []memory.LaneName{memory.LaneClaims, memory.LaneVec}, q.Lanes)
	for _, bad := range []wire.RecallRequest{{Query: ""}, {Query: "x", AsOf: "yesterday"}, {Query: "x", Lanes: []string{"web"}}} {
		_, err = ts.c.RecallMemory(ts.ctx, bad)
		apiCode(t, err, wire.CodeInvalidRequest)
	}

	cl, err := ts.c.ListConflicts(ts.ctx, "")
	require.NoError(t, err)
	require.Equal(t, "two employers", cl.Conflicts[0].Reason)
	require.Equal(t, claimB, cl.Conflicts[0].B.ID)
	_, err = ts.c.ListConflicts(ts.ctx, "all")
	require.NoError(t, err)
	require.Equal(t, []string{model.ConflictOpen, ""}, mem.statuses)
	_, err = ts.c.ListConflicts(ts.ctx, "closed")
	apiCode(t, err, wire.CodeInvalidRequest)
	rs, err := ts.c.ResolveConflict(ts.ctx, conflictID, wire.ResolveConflictRequest{Resolution: wire.ResolveKeepA})
	require.NoError(t, err)
	require.Equal(t, "resolved", rs.Status)
	require.Equal(t, wire.ResolveKeepA, *rs.Resolution)
	_, err = ts.c.ResolveConflict(ts.ctx, conflictID, wire.ResolveConflictRequest{Resolution: "a"})
	apiCode(t, err, wire.CodeInvalidRequest)

	ents, err := ts.c.ListEntities(ts.ctx)
	require.NoError(t, err)
	require.Equal(t, []wire.MemoryEntity{{ID: entityAnna, Kind: "person", Name: "Anna", Aliases: []string{}}}, ents.Entities)

	_, err = ts.c.MemoryProfile(ts.ctx)
	apiCode(t, err, wire.CodeNotFound)
	mem.card = model.ProfileCard{Day: "2026-09-15", Text: "- [p1] Works at Acme", CreatedAt: testutil.T0}
	card, err := ts.c.MemoryProfile(ts.ctx)
	require.NoError(t, err)
	require.Equal(t, "- [p1] Works at Acme", card.Text)
	require.Equal(t, "2026-09-15", mem.cardDays[len(mem.cardDays)-1], "today's card")

	added, err := ts.c.AddResource(ts.ctx, wire.AddResourceRequest{Path: "/home/u/docs/../lease.pdf", Title: "Lease"})
	require.NoError(t, err)
	require.Equal(t, "/home/u/lease.pdf", added.Path)
	_, err = ts.c.AddResource(ts.ctx, wire.AddResourceRequest{Path: "lease.pdf"})
	apiCode(t, err, wire.CodeInvalidRequest)
	docs, err := ts.c.ListResources(ts.ctx)
	require.NoError(t, err)
	require.Len(t, docs.Resources, 1)
	rc, err := ts.c.RemoveResource(ts.ctx, resourceID)
	require.NoError(t, err)
	require.Equal(t, 1, rc.Episodes)
	require.NotNil(t, rc.Lanes)

	require.NoError(t, ts.c.RebuildMemory(ts.ctx, wire.RebuildRequest{Lane: "files"}))
	require.Equal(t, []memory.LaneName{memory.LaneFiles}, mem.rebuilt)
	apiCode(t, ts.c.RebuildMemory(ts.ctx, wire.RebuildRequest{Lane: "claims"}), wire.CodeInvalidRequest)

	rep, err := ts.c.SleepMemory(ts.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.SleepReport{Learned: 3, Profile: true}, *rep)

	ex, err := ts.c.ExportMemory(ts.ctx)
	require.NoError(t, err)
	require.Len(t, ex.Claims, 2)
	require.Equal(t, claimGone, ex.Conflicts[0].B.ID, "a conflict keeps the id of a claim not exported")
	require.NotNil(t, ex.Episodes)

	_, err = ts.c.ForgetAllMemory(ts.ctx, "yes")
	apiCode(t, err, wire.CodeInvalidRequest)
	all, err := ts.c.ForgetAllMemory(ts.ctx, client.ForgetAllConfirm)
	require.NoError(t, err)
	require.Equal(t, 2, all.Claims)
}

func TestSelfHistory(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, nil)
	thread := "88888888-8888-4888-8888-888888888888"
	since := testutil.T0
	_, err := ts.Repos.Self.Add(ts.ctx, model.SelfVersion{Attitude: "angry", Note: "Late again.", Since: &since,
		Reason: model.SelfChat, ThreadID: &thread})
	require.NoError(t, err)
	self, err := ts.c.AssistantSelf(ts.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.AssistantSelf{Attitude: "angry", Note: "Late again.", Since: testutil.Ptr(wire.Millis(since))}, *self)

	require.NoError(t, ts.c.ResetAssistantSelf(ts.ctx))
	self, err = ts.c.AssistantSelf(ts.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.AssistantSelf{}, *self)
	hist, err := ts.c.AssistantSelfHistory(ts.ctx, 0)
	require.NoError(t, err)
	require.Len(t, hist.Versions, 2)
	require.Equal(t, model.SelfReset, hist.Versions[0].Reason)
	require.Equal(t, model.SelfChat, hist.Versions[1].Reason)
	require.Equal(t, thread, *hist.Versions[1].ThreadID)
	b, err := json.Marshal(hist.Versions[0])
	require.NoError(t, err)
	require.Contains(t, string(b), `"since":null`)

	// Nothing writes the old key any more.
	_, err = store.GetLocal(ts.ctx, ts.DB.SQL(), store.KeyAssistantSelf)
	require.ErrorIs(t, err, store.ErrNotFound)
}
