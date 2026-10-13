package vec

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

// fakeEmbedder is an embedding server: a text's vector counts its words in
// dim buckets, so texts that share words are near.
type fakeEmbedder struct {
	t      *testing.T
	openAI bool

	mu       sync.Mutex
	dim      int
	model    string
	requests int
	inputs   []string
	fail     bool
}

func (f *fakeEmbedder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := "/api/embed"
	if f.openAI {
		want = "/v1/embeddings"
	}
	if r.URL.Path != want || r.Method != http.MethodPost {
		http.Error(w, "no such route", http.StatusNotFound)
		return
	}
	if f.fail {
		http.Error(w, "model not loaded", http.StatusInternalServerError)
		return
	}
	var req struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.requests++
	f.model = req.Model
	f.inputs = append(f.inputs, req.Input...)
	vecs := make([][]float32, len(req.Input))
	for i, text := range req.Input {
		vecs[i] = bag(text, f.dim)
	}
	w.Header().Set("Content-Type", "application/json")
	if !f.openAI {
		_ = json.NewEncoder(w).Encode(map[string]any{"model": req.Model, "embeddings": vecs})
		return
	}
	// The data come in reverse order, each with its index.
	type datum struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}
	data := make([]datum, len(vecs))
	for i := range vecs {
		j := len(vecs) - 1 - i
		data[i] = datum{Object: "embedding", Index: j, Embedding: vecs[j]}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data, "model": req.Model})
}

func (f *fakeEmbedder) stats() (requests int, inputs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, append([]string(nil), f.inputs...)
}

func (f *fakeEmbedder) set(fn func(f *fakeEmbedder)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// bag is a text's vector: its words counted in dim buckets, not normalised.
func bag(text string, dim int) []float32 {
	v := make([]float32, dim)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		h.Write([]byte(strings.Trim(w, ".,?!")))
		v[h.Sum32()%uint32(dim)] += 3
	}
	v[0] += 0.5 // never the zero vector
	return v
}

type fx struct {
	t     *testing.T
	ctx   context.Context
	path  string
	embed *fakeEmbedder
	srv   *httptest.Server
}

func newFx(t *testing.T, openAI bool) *fx {
	t.Helper()
	embed := &fakeEmbedder{t: t, openAI: openAI, dim: 16}
	srv := httptest.NewServer(embed)
	t.Cleanup(srv.Close)
	return &fx{t: t, ctx: context.Background(), path: filepath.Join(t.TempDir(), "memory", "index.db"),
		embed: embed, srv: srv}
}

func (f *fx) endpoint() string {
	if f.embed.openAI {
		return f.srv.URL + "/v1"
	}
	return f.srv.URL
}

func (f *fx) open(model string) *Index {
	f.t.Helper()
	ix, err := New(Options{Path: f.path, Endpoint: f.endpoint(), Model: model,
		Clock: clock.NewFake(time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC))})
	require.NoError(f.t, err)
	f.t.Cleanup(func() { ix.Close() })
	return ix
}

func rec(id, text string) memory.Record {
	return memory.Record{ID: id, Kind: "claim", Text: text, Meta: map[string]string{"sensitivity": "personal"}}
}

const (
	acme   = "claim:00000000-0000-4000-8000-000000000001"
	lisbon = "claim:00000000-0000-4000-8000-000000000002"
	gym    = "ep:00000000-0000-4000-8000-000000000003"
	health = "claim:00000000-0000-4000-8000-000000000004"
)

func refs(hits []memory.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Ref
	}
	return out
}

func TestIndexOllama(t *testing.T) {
	t.Parallel()
	f := newFx(t, false)
	checkIndex(t, f)
}

func TestIndexOpenAI(t *testing.T) {
	t.Parallel()
	f := newFx(t, true)
	checkIndex(t, f)
}

func checkIndex(t *testing.T, f *fx) {
	ix := f.open("nomic-embed-text")
	require.Equal(t, memory.LaneVec, ix.Name())

	hits, err := ix.Search(f.ctx, memory.Query{Text: "where do I work"})
	require.NoError(t, err)
	require.Empty(t, hits, "an empty index asks nothing")
	n, _ := f.embed.stats()
	require.Zero(t, n)

	require.NoError(t, ix.Health(f.ctx))
	recs := []memory.Record{rec(acme, "Works at Acme Corp"), rec(lisbon, "Lives in Lisbon"),
		rec(gym, "Gym on Tuesdays and Thursdays")}
	require.NoError(t, ix.Upsert(f.ctx, recs))
	count, err := ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	reqs, _ := f.embed.stats()
	require.Equal(t, 2, reqs, "one probe, one batch")
	require.Equal(t, "nomic-embed-text", f.embed.model)

	hits, err = ix.Search(f.ctx, memory.Query{Text: "works at acme corp", Budget: memory.Budget{Candidates: 2}})
	require.NoError(t, err)
	require.Len(t, hits, 2, "at most the candidates asked")
	require.Equal(t, acme, hits[0].Ref)
	require.Equal(t, "claim", hits[0].Kind)
	require.Equal(t, []memory.LaneName{memory.LaneVec}, hits[0].Lanes)
	require.Greater(t, hits[0].Score, hits[1].Score)
	require.InDelta(t, 1, hits[0].Score, 1e-3, "the same words point the same way: distance 0")

	hits, err = ix.Search(f.ctx, memory.Query{Text: "Gym on Tuesdays", Exclude: []string{gym}})
	require.NoError(t, err)
	require.NotContains(t, refs(hits), gym, "excluded refs are left out")
	require.Len(t, hits, 2)

	// Unchanged records are not embedded again; a changed one is.
	require.NoError(t, ix.Upsert(f.ctx, recs))
	again, _ := f.embed.stats()
	require.Equal(t, reqs+2, again, "two searches since, and nothing for the unchanged upsert")
	recs[1].Text = "Lives in Porto"
	require.NoError(t, ix.Upsert(f.ctx, recs))
	after, inputs := f.embed.stats()
	require.Equal(t, again+1, after)
	require.Equal(t, "Lives in Porto", inputs[len(inputs)-1])
	hits, err = ix.Search(f.ctx, memory.Query{Text: "porto"})
	require.NoError(t, err)
	require.Equal(t, lisbon, hits[0].Ref, "delete then insert replaced the vector")
	count, err = ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	require.NoError(t, ix.Delete(f.ctx, []string{acme, "claim:not-there"}))
	hits, err = ix.Search(f.ctx, memory.Query{Text: "acme corp"})
	require.NoError(t, err)
	require.NotContains(t, refs(hits), acme)
	count, err = ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	require.NoError(t, ix.Clear(f.ctx))
	count, err = ix.Count(f.ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	hits, err = ix.Search(f.ctx, memory.Query{Text: "porto"})
	require.NoError(t, err)
	require.Empty(t, hits)
	require.NoError(t, ix.Upsert(f.ctx, recs[:1]), "a cleared index takes new items")
	count, err = ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestIndexBatches(t *testing.T) {
	t.Parallel()
	f := newFx(t, false)
	ix := f.open("m")
	var recs []memory.Record
	for i := range 70 {
		recs = append(recs, rec(model.NewID(), "fact number "+strings.Repeat("x", i+1)))
	}
	require.NoError(t, ix.Upsert(f.ctx, recs))
	n, inputs := f.embed.stats()
	require.Equal(t, 3, n, "batches of 32")
	require.Len(t, inputs, 70)
	count, err := ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 70, count)
}

func TestIndexReopenAndModelChange(t *testing.T) {
	t.Parallel()
	f := newFx(t, false)
	ix := f.open("nomic-embed-text")
	require.NoError(t, ix.Upsert(f.ctx, []memory.Record{rec(acme, "Works at Acme Corp"), rec(lisbon, "Lives in Lisbon")}))
	require.NoError(t, ix.Close())

	ix = f.open("nomic-embed-text")
	count, err := ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 2, count, "the same model keeps the index")
	hits, err := ix.Search(f.ctx, memory.Query{Text: "acme"})
	require.NoError(t, err)
	require.Equal(t, acme, hits[0].Ref, "the width is remembered without a probe")
	require.NoError(t, ix.Close())

	ix = f.open("mxbai-embed-large")
	count, err = ix.Count(f.ctx)
	require.NoError(t, err)
	require.Zero(t, count, "another model starts over")
	hits, err = ix.Search(f.ctx, memory.Query{Text: "acme"})
	require.NoError(t, err)
	require.Empty(t, hits)

	// The model's width changes: the index starts over at the new width.
	require.NoError(t, ix.Upsert(f.ctx, []memory.Record{rec(acme, "Works at Acme Corp"), rec(lisbon, "Lives in Lisbon")}))
	f.embed.set(func(e *fakeEmbedder) { e.dim = 8 })
	require.Error(t, ix.Health(f.ctx), "health says the widths differ")
	_, err = ix.Search(f.ctx, memory.Query{Text: "acme"})
	require.Error(t, err)
	require.NoError(t, ix.Upsert(f.ctx, []memory.Record{rec(gym, "Gym on Tuesdays")}))
	count, err = ix.Count(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "the old vectors were dropped")
	hits, err = ix.Search(f.ctx, memory.Query{Text: "gym"})
	require.NoError(t, err)
	require.Equal(t, []string{gym}, refs(hits))
}

func TestIndexSensitive(t *testing.T) {
	t.Parallel()
	sensitive := memory.Record{ID: health, Kind: "claim", Text: "Takes insulin daily",
		Meta: map[string]string{"sensitivity": model.SensitivitySensitive}}

	t.Run("loopback embeds it", func(t *testing.T) {
		t.Parallel()
		f := newFx(t, false)
		ix := f.open("m")
		require.NoError(t, ix.Upsert(f.ctx, []memory.Record{rec(acme, "Works at Acme"), sensitive}))
		_, inputs := f.embed.stats()
		require.Contains(t, inputs, "Takes insulin daily")
		count, err := ix.Count(f.ctx)
		require.NoError(t, err)
		require.Equal(t, 2, count)
	})

	t.Run("elsewhere skips it", func(t *testing.T) {
		t.Parallel()
		f := newFx(t, false)
		// The endpoint names a host elsewhere; the client dials the fake.
		addr := f.srv.Listener.Addr().String()
		client := &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}}}
		ix, err := New(Options{Path: f.path, Endpoint: "http://embed.example.com:11434", Model: "m",
			HTTPClient: client})
		require.NoError(t, err)
		t.Cleanup(func() { ix.Close() })
		require.False(t, ix.local)

		// It was stored while not sensitive; now it is, and it is dropped.
		was := sensitive
		was.Meta = map[string]string{"sensitivity": model.SensitivityPersonal}
		require.NoError(t, ix.Upsert(f.ctx, []memory.Record{was}))
		count, err := ix.Count(f.ctx)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		require.NoError(t, ix.Upsert(f.ctx, []memory.Record{rec(acme, "Works at Acme"), sensitive}))
		n, inputs := f.embed.stats()
		require.Equal(t, 2, n)
		require.Equal(t, []string{"Takes insulin daily", "Works at Acme"}, inputs,
			"the sensitive record went out only while it was not sensitive")
		hits, err := ix.Search(f.ctx, memory.Query{Text: "insulin"})
		require.NoError(t, err)
		require.Equal(t, []string{acme}, refs(hits), "the sensitive vector is gone")
	})
}

func TestIndexErrors(t *testing.T) {
	t.Parallel()
	f := newFx(t, true)
	ix := f.open("m")
	f.embed.set(func(e *fakeEmbedder) { e.fail = true })
	err := ix.Health(f.ctx)
	require.ErrorContains(t, err, "model not loaded")
	require.ErrorContains(t, ix.Upsert(f.ctx, []memory.Record{rec(acme, "Works at Acme")}), "500")
	count, err := ix.Count(f.ctx)
	require.NoError(t, err)
	require.Zero(t, count)

	_, err = New(Options{Path: "", Endpoint: "http://127.0.0.1:1", Model: "m"})
	require.Error(t, err)
	_, err = New(Options{Path: filepath.Join(t.TempDir(), "index.db"), Model: "m"})
	require.Error(t, err)
}

func TestLoopback(t *testing.T) {
	t.Parallel()
	for endpoint, want := range map[string]bool{
		"http://localhost:11434":          true,
		"http://LOCALHOST:11434/v1":       true,
		"http://127.0.0.1:11434":          true,
		"http://127.8.9.10/v1":            true,
		"http://[::1]:8080/v1":            true,
		"https://api.openai.com/v1":       false,
		"http://192.168.0.10:11434":       false,
		"http://localhost.example.com/v1": false,
		"::not a url":                     false,
	} {
		require.Equal(t, want, loopback(endpoint), endpoint)
	}
}

func TestNormalizeAndBlob(t *testing.T) {
	t.Parallel()
	v := normalize([]float32{3, 4})
	require.InDelta(t, 0.6, v[0], 1e-6)
	require.InDelta(t, 0.8, v[1], 1e-6)
	require.Equal(t, []float32{0, 0}, normalize([]float32{0, 0}))
	require.Equal(t, []byte{0, 0, 0x80, 0x3f, 0, 0, 0, 0x40}, blob([]float32{1, 2}), "float32 little-endian")
}
