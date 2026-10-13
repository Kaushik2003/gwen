package mem0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

const testKey = "m0sk_test"

// fakeMemory is a memory the fake server holds.
type fakeMemory struct {
	id, user, run, text string
	meta                map[string]any
}

// fake is a Mem0 REST server as server/main.py describes it: X-API-Key auth,
// POST /memories, GET /memories, POST /search, DELETE /memories/{id}.
type fake struct {
	t        *testing.T
	mu       sync.Mutex
	mems     []*fakeMemory
	next     int
	adds     []map[string]any // bodies of POST /memories
	bare     bool             // answer lists as bare arrays
	flat     bool             // put metadata at the top level of items
	failNext int              // a status to answer the next request with
}

func newFake(t *testing.T) (*fake, *httptest.Server) {
	f := &fake{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "" {
		f.t.Error("an API key must not be sent as a bearer token")
	}
	if r.Header.Get("X-API-Key") != testKey {
		reply(w, 401, map[string]any{"detail": "Authentication required. Provide a Bearer token or X-API-Key header."})
		return
	}
	if f.failNext != 0 {
		status := f.failNext
		f.failNext = 0
		reply(w, status, map[string]any{"detail": "upstream failed"})
		return
	}
	var body map[string]any
	if r.Body != nil && r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			reply(w, 422, map[string]any{"detail": []any{map[string]any{"msg": "bad json"}}})
			return
		}
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/memories":
		f.add(w, body)
	case r.Method == http.MethodGet && r.URL.Path == "/memories":
		user := r.URL.Query().Get("user_id")
		top, _ := strconv.Atoi(r.URL.Query().Get("top_k"))
		if top == 0 {
			top = 20
		}
		var out []map[string]any
		for _, m := range f.mems {
			if m.user == user && len(out) < top {
				out = append(out, f.item(m, nil))
			}
		}
		f.list(w, out)
	case r.Method == http.MethodPost && r.URL.Path == "/search":
		filters, _ := body["filters"].(map[string]any)
		user, _ := filters["user_id"].(string)
		if user == "" {
			reply(w, 400, map[string]any{"detail": "filters must contain at least one of: user_id, agent_id, run_id."})
			return
		}
		top := int(body["top_k"].(float64))
		words := strings.Fields(strings.ToLower(body["query"].(string)))
		type scored struct {
			m     *fakeMemory
			score float64
		}
		var found []scored
		for _, m := range f.mems {
			hits := 0
			for _, w := range words {
				if strings.Contains(strings.ToLower(m.text), w) {
					hits++
				}
			}
			if m.user == user && hits > 0 {
				found = append(found, scored{m, float64(hits) / float64(len(words))})
			}
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
		var out []map[string]any
		for _, s := range found[:min(top, len(found))] {
			score := s.score
			out = append(out, f.item(s.m, &score))
		}
		f.list(w, out)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/memories/"):
		id := strings.TrimPrefix(r.URL.Path, "/memories/")
		for i, m := range f.mems {
			if m.id == id {
				f.mems = append(f.mems[:i], f.mems[i+1:]...)
				reply(w, 200, map[string]any{"message": "Memory deleted successfully"})
				return
			}
		}
		reply(w, 404, map[string]any{"detail": "Memory with id " + id + " not found"})
	default:
		reply(w, 404, map[string]any{"detail": "Not Found"})
	}
}

func (f *fake) add(w http.ResponseWriter, body map[string]any) {
	f.adds = append(f.adds, body)
	user, _ := body["user_id"].(string)
	run, _ := body["run_id"].(string)
	if user == "" && run == "" {
		reply(w, 400, map[string]any{"detail": "At least one identifier (user_id, agent_id, run_id) is required."})
		return
	}
	infer, ok := body["infer"].(bool)
	if !ok {
		infer = true
	}
	meta, _ := body["metadata"].(map[string]any)
	var results []map[string]any
	store := func(text, role string) {
		f.next++
		m := &fakeMemory{id: fmt.Sprintf("00000000-0000-4000-8000-%012d", f.next), user: user, run: run, text: text,
			meta: map[string]any{"role": role}}
		for k, v := range meta {
			m.meta[k] = v
		}
		f.mems = append(f.mems, m)
		results = append(results, map[string]any{"id": m.id, "memory": text, "event": "ADD", "role": role})
	}
	for _, raw := range body["messages"].([]any) {
		msg := raw.(map[string]any)
		content, role := msg["content"].(string), msg["role"].(string)
		if !infer {
			store(content, role)
			continue
		}
		// The "extractor": every sentence that mentions liking something.
		for s := range strings.SplitSeq(content, ".") {
			if s = strings.TrimSpace(s); strings.Contains(s, "like") {
				store("User says: "+s, role)
			}
		}
	}
	reply(w, 200, map[string]any{"results": results})
}

// item is a memory as the server lists it: the caller's metadata nested
// under "metadata", unless the fake flattens it.
func (f *fake) item(m *fakeMemory, score *float64) map[string]any {
	out := map[string]any{"id": m.id, "memory": m.text, "user_id": m.user, "hash": "x",
		"created_at": "2026-10-12T09:00:00.123456+00:00", "updated_at": nil, "role": m.meta["role"]}
	if score != nil {
		out["score"] = *score
	}
	meta := map[string]any{}
	for k, v := range m.meta {
		if k != "role" {
			meta[k] = v
		}
	}
	if f.flat {
		for k, v := range meta {
			out[k] = v
		}
	} else if len(meta) > 0 {
		out["metadata"] = meta
	}
	return out
}

func (f *fake) list(w http.ResponseWriter, items []map[string]any) {
	if items == nil {
		items = []map[string]any{}
	}
	if f.bare {
		reply(w, 200, items)
		return
	}
	reply(w, 200, map[string]any{"results": items})
}

func (f *fake) count(user string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.mems {
		if m.user == user {
			n++
		}
	}
	return n
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func claimRec(id, key, text string) memory.Record {
	vf := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return memory.Record{ID: id, Kind: "claim", Key: key, Text: text, ValidFrom: &vf,
		Meta: map[string]string{"sensitivity": model.SensitivityPersonal}}
}

func TestUpsertStoresClaimsWordForWordOnce(t *testing.T) {
	ctx := context.Background()
	f, srv := newFake(t)
	stored := map[string]string{}
	l := New(Options{URL: srv.URL + "/", APIKey: testKey, HTTPClient: srv.Client(),
		OnStored: func(claimID, externalID string) { stored[claimID] = externalID }})
	a, b, c := model.NewID(), model.NewID(), model.NewID()
	secret := claimRec(c, "user.fact", "Owes a friend money")
	secret.Meta["sensitivity"] = model.SensitivitySensitive
	recs := []memory.Record{
		claimRec(a, "user.employer", "Works at Acme"),
		claimRec("claim:"+b, "pref.music", "Likes jazz"),
		secret,
		{ID: model.NewID(), Kind: "resource", Text: "a document"},
		{ID: model.NewID(), Kind: "claim", Text: "  "},
	}
	require.NoError(t, l.Upsert(ctx, recs))

	require.Len(t, f.adds, 2, "sensitive claims, resources, and empty claims are not sent")
	require.Equal(t, map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "Works at Acme"}},
		"user_id":  DefaultUserID,
		"infer":    false,
		"metadata": map[string]any{MetaClaimID: a, MetaKey: "user.employer", MetaValidFrom: "2026-03-01T00:00:00Z"},
	}, f.adds[0])
	require.Equal(t, b, f.adds[1]["metadata"].(map[string]any)[MetaClaimID], "a claim ref is stored by its bare id")
	require.Len(t, stored, 2)
	first := stored[a]
	require.NotEmpty(t, first)

	// Again: nothing new is added, and the same ids are reported.
	clear(stored)
	require.NoError(t, l.Upsert(ctx, recs))
	require.Len(t, f.adds, 2)
	require.Equal(t, first, stored[a])

	// New words replace the old copy.
	require.NoError(t, l.Upsert(ctx, []memory.Record{claimRec(a, "user.employer", "Works at Initech")}))
	require.Len(t, f.adds, 3)
	require.NotEqual(t, first, stored[a])
	require.Equal(t, 2, f.count(DefaultUserID))

	// A duplicate left by an earlier failure is cleaned up.
	f.mu.Lock()
	f.mems = append(f.mems, &fakeMemory{id: "dup", user: DefaultUserID, text: "Likes jazz",
		meta: map[string]any{MetaClaimID: b}})
	f.mu.Unlock()
	require.NoError(t, l.Upsert(ctx, []memory.Record{claimRec(b, "pref.music", "Likes jazz")}))
	require.Equal(t, 2, f.count(DefaultUserID))
}

func TestSearchReturnsClaimRefs(t *testing.T) {
	ctx := context.Background()
	f, srv := newFake(t)
	l := New(Options{URL: srv.URL, APIKey: testKey, UserID: "kaushik", HTTPClient: srv.Client()})
	a, b, c := model.NewID(), model.NewID(), model.NewID()
	require.NoError(t, l.Upsert(ctx, []memory.Record{
		claimRec(a, "pref.music", "Likes jazz music"),
		claimRec(b, "pref.music", "Likes loud music at work"),
		claimRec(c, "pref.food", "Likes jazz brunch"),
	}))
	f.mu.Lock()
	f.mems = append(f.mems, &fakeMemory{id: "foreign", user: "kaushik", text: "music from elsewhere"})
	f.mu.Unlock()

	hits, err := l.Search(ctx, memory.Query{Text: "jazz music", Exclude: []string{"claim:" + c}})
	require.NoError(t, err)
	require.Len(t, hits, 2, "the excluded claim and the memory Gwen did not store are left out")
	require.Equal(t, "claim:"+a, hits[0].Ref)
	require.Equal(t, "claim", hits[0].Kind)
	require.Equal(t, "pref.music", hits[0].Key)
	require.Equal(t, "Likes jazz music", hits[0].Text)
	require.InDelta(t, 1.0, hits[0].Score, 1e-9)
	require.Equal(t, []memory.LaneName{memory.LaneMem0}, hits[0].Lanes)
	require.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), *hits[0].ValidFrom)
	require.Equal(t, time.Date(2026, 10, 12, 9, 0, 0, 123456000, time.UTC), hits[0].At.UTC())
	require.Equal(t, "claim:"+b, hits[1].Ref)

	hits, err = l.Search(ctx, memory.Query{Text: "music", Budget: memory.Budget{Candidates: 1}})
	require.NoError(t, err)
	require.Len(t, hits, 1)

	hits, err = l.Search(ctx, memory.Query{Text: "   "})
	require.NoError(t, err)
	require.Empty(t, hits)
}

func TestDecodingIsTolerant(t *testing.T) {
	ctx := context.Background()
	f, srv := newFake(t)
	f.bare, f.flat = true, true
	l := New(Options{URL: srv.URL, APIKey: testKey, HTTPClient: srv.Client()})
	a := model.NewID()
	require.NoError(t, l.Upsert(ctx, []memory.Record{claimRec(a, "pref.music", "Likes jazz")}))
	require.NoError(t, l.Upsert(ctx, []memory.Record{claimRec(a, "pref.music", "Likes jazz")}))
	require.Len(t, f.adds, 1, "a bare list with flat metadata still finds the stored copy")
	hits, err := l.Search(ctx, memory.Query{Text: "jazz"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "claim:"+a, hits[0].Ref)

	odd := `{"memories":[{"id":7,"data":"x","score":"0.5","metadata":null,"extra":{}}]}`
	items, err := decodeItems(json.RawMessage(odd))
	require.NoError(t, err)
	require.Equal(t, "7", items[0].ID)
	require.Equal(t, "x", items[0].Memory)
	require.InDelta(t, 0.5, items[0].Score, 1e-9)
	_, err = decodeItems(json.RawMessage(`"nope"`))
	require.Error(t, err)
}

func TestDeleteAndHealth(t *testing.T) {
	ctx := context.Background()
	f, srv := newFake(t)
	stored := map[string]string{}
	l := New(Options{URL: srv.URL, APIKey: testKey, HTTPClient: srv.Client(),
		OnStored: func(claimID, externalID string) { stored[claimID] = externalID }})
	a := model.NewID()
	require.NoError(t, l.Upsert(ctx, []memory.Record{claimRec(a, "pref.music", "Likes jazz")}))
	require.NoError(t, l.Delete(ctx, []string{stored[a], "never-there", ""}), "a memory already gone is fine")
	require.Zero(t, f.count(DefaultUserID))

	require.NoError(t, l.Health(ctx))
	f.failNext = 500
	err := l.Delete(ctx, []string{"x"})
	require.ErrorContains(t, err, "500 upstream failed")

	bad := New(Options{URL: srv.URL, APIKey: "wrong", HTTPClient: srv.Client()})
	require.ErrorContains(t, bad.Health(ctx), "did not accept the API key")

	require.ErrorContains(t, New(Options{}).Health(ctx), "no server address")
	down := New(Options{URL: "http://127.0.0.1:1", HTTPClient: srv.Client()})
	_, err = down.Search(ctx, memory.Query{Text: "jazz"})
	require.ErrorContains(t, err, "reach mem0")
}

func TestShadowExtractsAndCleansUp(t *testing.T) {
	ctx := context.Background()
	f, srv := newFake(t)
	l := New(Options{URL: srv.URL, APIKey: testKey, HTTPClient: srv.Client()})
	f.mu.Lock()
	f.mems = append(f.mems, &fakeMemory{id: "left-over", user: ShadowUserID, text: "User says: I like tea"})
	f.mu.Unlock()

	facts, err := l.Shadow(ctx, []Message{
		{Role: "user", Content: "I like jazz. I work at Acme. I also like rain."},
		{Role: "assistant", Content: "Noted that you like jazz."},
		{Role: "system", Content: "ignored"},
		{Role: "user", Content: " "},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"User says: I like jazz", "User says: I also like rain",
		"User says: Noted that you like jazz"}, facts)
	require.Zero(t, f.count(ShadowUserID), "what Mem0 stored, and what an earlier run left, is deleted")

	add := f.adds[len(f.adds)-1]
	require.Equal(t, true, add["infer"])
	require.Equal(t, ShadowUserID, add["user_id"])
	require.True(t, strings.HasPrefix(add["run_id"].(string), "shadow-"))
	require.Len(t, add["messages"], 2)

	facts, err = l.Shadow(ctx, []Message{{Role: "system", Content: "nothing to read"}})
	require.NoError(t, err)
	require.Empty(t, facts)
}
