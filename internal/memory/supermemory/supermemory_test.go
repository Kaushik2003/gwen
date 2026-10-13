package supermemory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

const testKey = "sm_test"

// fakeDoc is a document the fake server holds.
type fakeDoc struct {
	id, container, content, date string
	meta                         map[string]any
}

// fake is a Supermemory server speaking the legacy v3/v4 API or v5, as the
// API reference describes them.
type fake struct {
	t        *testing.T
	mu       sync.Mutex
	v5       bool
	docs     map[string]*fakeDoc // by custom id
	bodies   map[string][]map[string]any
	queries  []string // raw query strings of v5 list calls
	noMeta   bool     // v5: leave metadata off results, so only included.document has it
	failWith int      // a status to answer every request with
}

func newFake(t *testing.T, v5 bool) (*fake, *Lane) {
	f := &fake{t: t, v5: v5, docs: map[string]*fakeDoc{}, bodies: map[string][]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, New(Options{URL: srv.URL, APIKey: testKey, HTTPClient: srv.Client()})
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+testKey {
		reply(w, 401, map[string]any{"error": "Unauthorized"})
		return
	}
	if f.failWith != 0 {
		reply(w, f.failWith, map[string]any{"error": "Limit reached", "details": "10,000 documents"})
		return
	}
	var body map[string]any
	if r.ContentLength != 0 {
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
	}
	route := r.Method + " " + r.URL.Path
	f.bodies[route] = append(f.bodies[route], body)
	legacy := !f.v5 && strings.HasPrefix(r.URL.Path, "/v")
	v5 := f.v5 && strings.HasPrefix(r.URL.Path, "/ns/gwen/")
	switch {
	case legacy && route == "POST /v3/documents":
		f.store(body["customId"].(string), body["containerTag"].(string), body)
		reply(w, 200, map[string]any{"id": "sm_" + body["customId"].(string), "status": "queued"})
	case legacy && r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v3/documents/"):
		id := strings.TrimPrefix(r.URL.Path, "/v3/documents/")
		if f.docs[id] == nil {
			reply(w, 404, map[string]any{"error": "Document not found"})
			return
		}
		delete(f.docs, id)
		w.WriteHeader(204)
	case legacy && route == "POST /v3/search":
		f.searchLegacy(w, body)
	case legacy && route == "POST /v3/documents/list":
		reply(w, 200, map[string]any{"memories": []any{}, "pagination": map[string]any{"currentPage": 1, "limit": 1,
			"totalItems": len(f.docs), "totalPages": len(f.docs)}})
	case legacy && route == "POST /v4/profile":
		reply(w, 200, map[string]any{"profile": map[string]any{
			"static":  []any{"Rents a flat in Leeds", "Signed a lease"},
			"dynamic": []any{"Signed a lease", " Moving in March "}}})
	case v5 && route == "POST /ns/gwen/document":
		f.store(body["id"].(string), "gwen", body)
		reply(w, 200, map[string]any{"id": body["id"], "status": "queued"})
	case v5 && route == "DELETE /ns/gwen/document":
		var errs []any
		count := 0
		for _, id := range body["ids"].([]any) {
			if f.docs[id.(string)] == nil {
				errs = append(errs, map[string]any{"id": id, "error": "Document not found"})
				continue
			}
			delete(f.docs, id.(string))
			count++
		}
		reply(w, 200, map[string]any{"count": count, "errors": append([]any{}, errs...)})
	case v5 && route == "POST /ns/gwen/search":
		f.searchV5(w, body)
	case v5 && route == "POST /ns/gwen/list/documents":
		f.queries = append(f.queries, r.URL.RawQuery)
		reply(w, 200, map[string]any{"documents": []any{}, "chunks": []any{}, "memories": []any{},
			"pagination": map[string]any{"currentPage": 1, "limit": 1, "totalItems": len(f.docs), "totalPages": 1}})
	case v5 && route == "POST /ns/gwen/profile":
		reply(w, 200, map[string]any{"profile": map[string]any{
			"static":  []any{map[string]any{"id": "m1", "memory": "Rents a flat in Leeds"}},
			"dynamic": []any{map[string]any{"id": "m2", "memory": "Moving in March"}}}})
	default:
		w.WriteHeader(404) // as a server without the route answers
		w.Write([]byte("404 Not Found"))
	}
}

func (f *fake) store(id, container string, body map[string]any) {
	d := &fakeDoc{id: id, container: container, content: body["content"].(string)}
	d.meta, _ = body["metadata"].(map[string]any)
	d.date, _ = body["documentDate"].(string)
	if d.date == "" {
		d.date, _ = body["date"].(string)
	}
	f.docs[id] = d
}

// chunk is a paragraph of a document that shares words with the query.
type chunk struct {
	doc   *fakeDoc
	pos   int
	text  string
	score float64
}

func (f *fake) match(query string) []chunk {
	words := strings.Fields(strings.ToLower(query))
	var out []chunk
	ids := make([]string, 0, len(f.docs))
	for id := range f.docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for pos, para := range strings.Split(f.docs[id].content, "\n\n") {
			n := 0
			for _, w := range words {
				if strings.Contains(strings.ToLower(para), w) {
					n++
				}
			}
			if n > 0 {
				out = append(out, chunk{f.docs[id], pos, para, float64(n) / float64(len(words))})
			}
		}
	}
	return out
}

func (f *fake) searchLegacy(w http.ResponseWriter, body map[string]any) {
	if body["containerTag"] != "gwen" {
		reply(w, 200, map[string]any{"results": []any{}, "timing": 1, "total": 0})
		return
	}
	byDoc := map[string]map[string]any{}
	var order []string
	for _, c := range f.match(body["q"].(string)) {
		r := byDoc[c.doc.id]
		if r == nil {
			r = map[string]any{"documentId": "sm_" + c.doc.id, "metadata": c.doc.meta, "title": c.doc.meta["title"],
				"createdAt": "2026-10-01T08:00:00.000Z", "updatedAt": "2026-10-01T08:00:00.000Z", "type": "text",
				"score": 0.0, "chunks": []any{}}
			byDoc[c.doc.id] = r
			order = append(order, c.doc.id)
		}
		r["chunks"] = append(r["chunks"].([]any), map[string]any{"content": c.text, "position": c.pos,
			"score": c.score, "isRelevant": true})
		r["score"] = max(r["score"].(float64), c.score)
	}
	results := []any{}
	for _, id := range order {
		results = append(results, byDoc[id])
	}
	reply(w, 200, map[string]any{"results": results, "timing": 12, "total": len(results)})
}

func (f *fake) searchV5(w http.ResponseWriter, body map[string]any) {
	results := []any{}
	for i, c := range f.match(body["query"].(string)) {
		r := map[string]any{"id": "chunk_" + string(rune('a'+i)), "chunk": c.text, "similarity": c.score,
			"metadata": c.doc.meta, "isLatest": true, "isInference": false,
			"system": map[string]any{"updatedAt": "2026-10-01T08:00:00Z"},
			"included": map[string]any{"document": map[string]any{"id": c.doc.id, "title": c.doc.meta["title"],
				"type": "text", "metadata": c.doc.meta, "summary": nil,
				"system": map[string]any{"createdAt": "2026-10-01T08:00:00Z", "updatedAt": "2026-10-01T08:00:00Z"}}}}
		if f.noMeta {
			r["metadata"] = map[string]any{}
		}
		results = append(results, r)
	}
	reply(w, 200, map[string]any{"results": results, "searchTime": 3})
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// lease is a resource with two paragraphs.
func lease() memory.Record {
	day := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	return memory.Record{ID: "0199a000-0000-7000-8000-00000000000a", Kind: "resource",
		Text:      "The tenant pays rent monthly.\n\nThe deposit is two months of rent.",
		ValidFrom: &day, At: day.Add(time.Hour), Meta: map[string]string{"title": "Lease.pdf"}}
}

func TestLegacyServer(t *testing.T) {
	ctx := context.Background()
	f, l := newFake(t, false)

	secret := memory.Record{ID: model.NewID(), Kind: "resource", Text: "bank details",
		Meta: map[string]string{"sensitivity": model.SensitivitySensitive}}
	other := memory.Record{ID: "doc:0199a000-0000-7000-8000-00000000000b", Text: "Recipe: rent a pasta machine.",
		At: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	err := l.Upsert(ctx, []memory.Record{lease(), secret, other, {ID: "bad id/../x", Text: "x"},
		{ID: model.NewID(), Kind: "claim", Text: "a claim"}})
	require.ErrorContains(t, err, `"bad id/../x" cannot be a document id`)
	require.Len(t, f.docs, 2, "the sensitive record, the claim, and the bad id are not sent")
	require.Equal(t, map[string]any{
		"content":      lease().Text,
		"containerTag": "gwen",
		"customId":     lease().ID,
		"metadata":     map[string]any{MetaResourceID: lease().ID, "title": "Lease.pdf"},
		"documentDate": "2026-02-01T00:00:00Z",
	}, f.bodies["POST /v3/documents"][0])
	require.Equal(t, "2026-03-01T12:00:00Z", f.docs["0199a000-0000-7000-8000-00000000000b"].date)

	// A foreign document in the container is never returned.
	f.mu.Lock()
	f.docs["foreign"] = &fakeDoc{id: "foreign", container: "gwen", content: "rent is due"}
	f.mu.Unlock()

	hits, err := l.Search(ctx, memory.Query{Text: "deposit rent", Budget: memory.Budget{Candidates: 500}})
	require.NoError(t, err)
	require.Equal(t, 100, int(f.bodies["POST /v3/search"][0]["limit"].(float64)), "never over 100, which returns nothing")
	require.Equal(t, true, f.bodies["POST /v3/search"][0]["onlyMatchingChunks"])
	require.Len(t, hits, 3)
	require.Equal(t, memory.Hit{Ref: "doc:" + lease().ID + "#c1", Kind: "resource",
		Text:  "The deposit is two months of rent.",
		Score: 1, At: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), Source: "document “Lease.pdf”",
		Trust: model.TrustExternal, Lanes: []memory.LaneName{memory.LaneSupermemory}}, hits[0])
	require.Equal(t, "doc:"+lease().ID+"#c0", hits[1].Ref)
	require.Equal(t, "doc:0199a000-0000-7000-8000-00000000000b#c0", hits[2].Ref)
	require.Empty(t, hits[2].Source)

	hits, err = l.Search(ctx, memory.Query{Text: "rent", Exclude: []string{"doc:" + lease().ID},
		Budget: memory.Budget{Candidates: 1}})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "doc:0199a000-0000-7000-8000-00000000000b#c0", hits[0].Ref)

	n, err := l.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.NoError(t, l.Health(ctx))

	facts, err := l.Profile(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"Rents a flat in Leeds", "Signed a lease", "Moving in March"}, facts)

	require.NoError(t, l.Delete(ctx, []string{lease().ID, "doc:never-there#c3", ""}))
	require.Nil(t, f.docs[lease().ID])
	require.Len(t, f.bodies["DELETE /v3/documents/never-there"], 1, "a part and the doc: prefix are dropped")
}

func TestV5Server(t *testing.T) {
	ctx := context.Background()
	f, l := newFake(t, true)
	f.noMeta = true

	require.NoError(t, l.Upsert(ctx, []memory.Record{lease()}))
	require.Equal(t, map[string]any{
		"content":  lease().Text,
		"id":       lease().ID,
		"metadata": map[string]any{MetaResourceID: lease().ID, "title": "Lease.pdf"},
		"date":     "2026-02-01T00:00:00Z",
	}, f.bodies["POST /ns/gwen/document"][0])

	hits, err := l.Search(ctx, memory.Query{Text: "deposit"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"query": "deposit", "searchMode": "chunks", "limit": 40.0,
		"include": map[string]any{"documents": true}}, f.bodies["POST /ns/gwen/search"][0])
	require.Len(t, hits, 1)
	require.Equal(t, "doc:"+lease().ID+"#chunk_a", hits[0].Ref, "the document's metadata names it")
	require.Equal(t, "document “Lease.pdf”", hits[0].Source)
	require.Equal(t, "The deposit is two months of rent.", hits[0].Text)

	n, err := l.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, []string{"page=1&limit=1"}, f.queries)

	facts, err := l.Profile(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"Rents a flat in Leeds", "Moving in March"}, facts)

	require.NoError(t, l.Delete(ctx, []string{lease().ID, "never-there"}))
	require.Equal(t, []any{lease().ID, "never-there"}, f.bodies["DELETE /ns/gwen/document"][0]["ids"])
	require.Empty(t, f.docs)
}

func TestFollowsAnUpgradeToV5(t *testing.T) {
	ctx := context.Background()
	f, l := newFake(t, false)
	require.NoError(t, l.Upsert(ctx, []memory.Record{lease()}))

	f.mu.Lock()
	f.v5 = true
	f.mu.Unlock()
	_, err := l.Search(ctx, memory.Query{Text: "rent"})
	require.Error(t, err, "the legacy route has gone")
	hits, err := l.Search(ctx, memory.Query{Text: "rent"})
	require.NoError(t, err, "the next call speaks v5")
	require.NotEmpty(t, hits)
	require.NotEmpty(t, f.bodies["POST /ns/gwen/search"])

	// Delete always asks first, so a missing route is never a missing document.
	f.mu.Lock()
	f.v5 = false
	f.mu.Unlock()
	require.NoError(t, l.Delete(ctx, []string{lease().ID}))
	require.Empty(t, f.docs)
}

func TestErrorsSayWhy(t *testing.T) {
	ctx := context.Background()
	f, l := newFake(t, false)
	f.failWith = http.StatusPaymentRequired
	require.ErrorContains(t, l.Upsert(ctx, []memory.Record{lease()}),
		"over its document limit: Limit reached: 10,000 documents")

	f.failWith = 0
	bad := New(Options{URL: l.url, APIKey: "wrong", HTTPClient: l.http})
	require.ErrorContains(t, bad.Health(ctx), "did not accept the API key: Unauthorized")

	down := New(Options{URL: "http://127.0.0.1:1"})
	_, err := down.Search(ctx, memory.Query{Text: "rent"})
	require.ErrorContains(t, err, "reach supermemory")

	hits, err := l.Search(ctx, memory.Query{Text: " "})
	require.NoError(t, err)
	require.Empty(t, hits)
	require.NoError(t, l.Delete(ctx, nil))
	require.NoError(t, l.Upsert(ctx, nil))

	def := New(Options{})
	require.Equal(t, DefaultURL, def.url)
	require.Equal(t, DefaultContainer, def.container)
	require.Equal(t, memory.LaneSupermemory, def.Name())
}
