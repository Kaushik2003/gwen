package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

const token = "0123456789abcdef"

type node struct {
	clk     *clock.Fake
	db      *store.DB
	repos   store.Repos
	credDir string
}

func newNode(t *testing.T) *node {
	t.Helper()
	n := &node{clk: clock.NewFake(testutil.At("09:00")), credDir: t.TempDir()}
	n.db = testutil.NewDBWithClock(t, n.clk)
	n.repos = store.NewRepos(n.db)
	require.NoError(t, config.WriteCredential(n.credDir, config.CredSyncToken, []byte(token+"\n")))
	return n
}

func newHub(t *testing.T) (*node, *Hub, *httptest.Server) {
	t.Helper()
	n := newNode(t)
	h := &Hub{DB: n.db, Repo: n.repos.Sync, CredDir: n.credDir, Clock: n.clk}
	mux := http.NewServeMux()
	mux.Handle("/sync/v1/", h.Handler())
	mux.HandleFunc("POST /login", h.Login)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return n, h, srv
}

func newClient(t *testing.T, n *node, hubURL string) *Client {
	t.Helper()
	cfg := config.Defaults()
	cfg.Sync.HubURL = hubURL
	return NewClient(ClientOptions{DB: n.db, Repo: n.repos.Sync, Clock: n.clk, CredDir: n.credDir,
		Config: func() config.Config { return cfg }})
}

func get(t *testing.T, u string, header map[string]string, cookie *http.Cookie) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

func TestTheHubNeedsTheToken(t *testing.T) {
	t.Parallel()
	hub, _, srv := newHub(t)
	health := srv.URL + "/sync/v1/health"
	require.Equal(t, http.StatusUnauthorized, get(t, health, nil, nil))
	require.Equal(t, http.StatusUnauthorized, get(t, health, map[string]string{"Authorization": "Bearer wrong"}, nil))
	require.Equal(t, http.StatusOK, get(t, health, map[string]string{"Authorization": "Bearer " + token}, nil))
	require.Equal(t, http.StatusUnauthorized, get(t, srv.URL+"/sync/v1/pull?device_id=x",
		map[string]string{"Authorization": "Bearer " + token + "x"}, nil))

	require.NoError(t, os.Remove(filepath.Join(hub.credDir, config.CredSyncToken)))
	require.Equal(t, http.StatusServiceUnavailable, get(t, health, map[string]string{"Authorization": "Bearer " + token}, nil))
}

func TestLoginSetsASessionCookie(t *testing.T) {
	t.Parallel()
	hub, _, srv := newHub(t)
	resp, err := http.PostForm(srv.URL+"/login", url.Values{"token": {"wrong"}})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp, err = http.Post(srv.URL+"/login", "application/json", strings.NewReader(`{"token": "`+token+`"}`))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	require.Equal(t, CookieName, c.Name)
	require.True(t, c.HttpOnly)
	require.Equal(t, http.SameSiteStrictMode, c.SameSite)
	require.Equal(t, 30*24*3600, c.MaxAge)
	require.NotContains(t, c.Value, token, "the cookie does not carry the token")
	health := srv.URL + "/sync/v1/health"
	require.Equal(t, http.StatusOK, get(t, health, nil, c))

	forged := *c
	forged.Value = strings.Split(c.Value, ".")[0] + ".00"
	require.Equal(t, http.StatusUnauthorized, get(t, health, nil, &forged))
	hub.clk.Advance(31 * 24 * time.Hour)
	require.Equal(t, http.StatusUnauthorized, get(t, health, nil, c), "sessions end after 30 days")
}

func count(t *testing.T, db *store.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, db.SQL().QueryRow(q).Scan(&n))
	return n
}

func TestPushPullAndRestore(t *testing.T) {
	t.Parallel()
	hub, _, srv := newHub(t)
	ctx := context.Background()
	laptop := newNode(t)
	p, err := laptop.repos.Projects.Create(ctx, "Study", nil)
	require.NoError(t, err)
	tk, _, err := laptop.repos.Tasks.Create(ctx, store.NewTask{Title: "Read", ProjectID: &p.ID})
	require.NoError(t, err)

	c := newClient(t, laptop, srv.URL+"/")
	st, err := c.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.True(t, st.Configured)
	require.NotNil(t, st.LastPushAt)
	require.NotNil(t, st.LastPullAt)
	require.Equal(t, 1, count(t, hub.db, `SELECT count(*) FROM tasks`))
	require.Equal(t, 2, count(t, hub.db, `SELECT count(*) FROM hub_changes`))
	wm, err := store.GetLocalTime(ctx, laptop.db.SQL(), store.KeySyncPushWatermark)
	require.NoError(t, err)
	require.Equal(t, tk.UpdatedAt.UnixMilli(), wm.UnixMilli())

	// Within the overlap everything is pushed again, without duplicates.
	laptop.clk.Advance(time.Minute)
	_, err = c.Sync(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count(t, hub.db, `SELECT count(*) FROM tasks`))
	require.Equal(t, 2, count(t, hub.db, `SELECT count(*) FROM hub_changes`))

	// A reinstalled device pulls the whole history, and not its own writes.
	restored := newNode(t)
	r := newClient(t, restored, srv.URL)
	own, err := restored.repos.Projects.Create(ctx, "Chores", nil)
	require.NoError(t, err)
	var applied []store.Applied
	r.o.OnApplied = func(a store.Applied) { applied = append(applied, a) }
	_, err = r.Sync(ctx)
	require.NoError(t, err)
	got, err := restored.repos.Tasks.Get(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, "Read", got.Title)
	require.Len(t, applied, 1)
	require.Equal(t, []string{p.ID}, applied[0].IDs["projects"], "its own project comes back from nobody")
	cursor, err := store.GetLocal(ctx, restored.db.SQL(), store.KeySyncPullCursor)
	require.NoError(t, err)
	require.Equal(t, "2", cursor, "the last row it was sent; its own row, seq 3, is skipped")

	// The laptop then receives the other device's project.
	_, err = c.Sync(ctx)
	require.NoError(t, err)
	gotOwn, err := laptop.repos.Projects.Get(ctx, own.ID)
	require.NoError(t, err)
	require.Equal(t, "Chores", gotOwn.Name)
}

func TestClientUnavailableAndFailures(t *testing.T) {
	t.Parallel()
	n := newNode(t)
	c := newClient(t, n, "")
	_, err := c.Sync(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	require.False(t, c.Status(context.Background()).Configured)

	c = newClient(t, n, "http://127.0.0.1:1")
	st, err := c.Sync(context.Background())
	require.NoError(t, err)
	require.Contains(t, st.LastError, "connection refused")
	require.NotNil(t, st.LastPushAt, "nothing to push is a successful push")
	require.Nil(t, st.LastPullAt)

	_, err = n.repos.Projects.Create(context.Background(), "P", nil)
	require.NoError(t, err)
	st, err = c.Sync(context.Background())
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(st.LastError, "push:"), st.LastError)
	_, err = store.GetLocalTime(context.Background(), n.db.SQL(), store.KeySyncPushWatermark)
	require.ErrorIs(t, err, store.ErrNotFound, "a failed push keeps the watermark")

	require.NoError(t, os.Remove(filepath.Join(n.credDir, config.CredSyncToken)))
	_, err = c.Sync(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
}

const (
	threadID = "00000000-0000-7000-8000-0000000000c1"
	claimID  = "00000000-0000-7000-8000-0000000000d1"
)

// remember writes memory on n as its own, at its clock's time: a chat, a
// claim and an episode of each sensitivity, an entity, a conflict, a link,
// and a version of the assistant's self.
func remember(t *testing.T, n *node) {
	t.Helper()
	now, dev := n.clk.Now().UnixMilli(), n.db.DeviceID()
	id := func(i int) string { return fmt.Sprintf("00000000-0000-7000-8000-0000000001%02d", i) }
	for _, q := range []string{
		`INSERT INTO chat_threads (id, title, %s) VALUES ('` + threadID + `', 'The cat', %s)`,
		`INSERT INTO chat_messages (id, thread_id, role, text, at, %s)
			VALUES ('` + id(1) + `', '` + threadID + `', 'user', 'My cat Miso is ill', 1, %s)`,
		`INSERT INTO self_versions (id, attitude, reason, thread_id, %s)
			VALUES ('` + id(2) + `', 'warm', 'chat', '` + threadID + `', %s)`,
		`INSERT INTO entities (id, kind, name, %s) VALUES ('` + id(3) + `', 'pet', 'Miso', %s)`,
		`INSERT INTO episodes (id, kind, channel, speaker, body, at, sensitivity, content_hash, %s)
			VALUES ('` + id(4) + `', 'note', 'dashboard', 'user', 'Miso sees the vet', 1, 'personal', 'h1', %s)`,
		`INSERT INTO episodes (id, kind, channel, speaker, body, at, sensitivity, content_hash, %s)
			VALUES ('` + id(5) + `', 'note', 'dashboard', 'user', 'My diagnosis', 1, 'sensitive', 'h2', %s)`,
		`INSERT INTO claims (id, key, value, text, cardinality, status, trust, sensitivity, confidence, mentioned_at,
			recorded_at, source_kind, source_device_id, channel, content_hash, %s)
			VALUES ('` + claimID + `', 'user.pet', '"a cat"', 'They have a cat, Miso', 'many', 'active', 'owner_said',
			'personal', 0.75, 1, 1, 'msg', 'dev', 'chat', 'h3', %s)`,
		`INSERT INTO claims (id, key, value, text, cardinality, status, trust, sensitivity, confidence, mentioned_at,
			recorded_at, source_kind, source_device_id, channel, content_hash, %s)
			VALUES ('` + id(6) + `', 'user.health', '"ill"', 'They are ill', 'one', 'active', 'owner_said',
			'sensitive', 1, 1, 1, 'msg', 'dev', 'chat', 'h4', %s)`,
		`INSERT INTO conflicts (id, claim_a, claim_b, sensitivity, status, %s)
			VALUES ('` + id(7) + `', '` + claimID + `', '` + id(8) + `', 'personal', 'open', %s)`,
		`INSERT INTO memory_links (id, from_kind, from_id, to_kind, to_id, relation, %s)
			VALUES ('` + id(9) + `', 'claim', '` + claimID + `', 'entity', '` + id(3) + `', 'about', %s)`,
	} {
		q = fmt.Sprintf(q, "created_at, updated_at, device_id, rev", "?, ?, ?, 1")
		_, err := n.db.SQL().Exec(q, now, now, dev)
		require.NoError(t, err, q)
	}
}

// memoryPushed is what remember writes that may leave the device: all but
// the sensitive claim and episode.
var memoryPushed = map[string]int{"chat_threads": 1, "chat_messages": 1, "self_versions": 1, "entities": 1,
	"episodes": 1, "claims": 1, "conflicts": 1, "memory_links": 1}

// gate stands in front of a hub. At schema 9 it is a hub from before schema
// negotiation: it says 9 and refuses a whole push holding a table it does
// not know, as that gwend's checkRow did. It counts the rows of every push it
// lets through, by table, and keeps every pull's query.
type gate struct {
	mu     gosync.Mutex
	schema int64
	pushed map[string]int
	pulls  []url.Values
}

func newGatedHub(t *testing.T) (*node, *gate, string) {
	t.Helper()
	n := newNode(t)
	h := &Hub{DB: n.db, Repo: n.repos.Sync, CredDir: n.credDir, Clock: n.clk}
	g := &gate{schema: store.LegacySyncSchema, pushed: map[string]int{}}
	next := h.Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch r.URL.Path {
		case "/sync/v1/health":
			writeJSON(w, http.StatusOK, Health{OK: true, SchemaVersion: g.schema})
			return
		case "/sync/v1/push":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read push: %v", err)
			}
			var req PushRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("decode push: %v", err)
			}
			rows, err := store.DecodeRows(req.Rows)
			if err != nil {
				t.Errorf("decode pushed rows: %v", err)
			}
			known := store.SyncTablesBetween(0, g.schema)
			for table := range rows {
				if !slices.Contains(known, table) {
					writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("%q is not a synced table", table))
					return
				}
			}
			for table, rs := range rows {
				g.pushed[table] += len(rs)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		case "/sync/v1/pull":
			g.pulls = append(g.pulls, r.URL.Query())
		}
		next.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return n, g, srv.URL
}

// reset sets the hub's schema and returns what was pushed since the last
// reset.
func (g *gate) reset(schema int64) map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	pushed := g.pushed
	g.schema, g.pushed = schema, map[string]int{}
	return pushed
}

// An older hub is sent only the tables it knows, so its sync goes on. Once
// it is updated, the tables it now takes go up once, from the beginning,
// sensitive rows excepted.
func TestAnOldHubIsSentOnlyWhatItKnows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	hub, g, hubURL := newGatedHub(t)
	laptop := newNode(t)
	advance := func() {
		laptop.clk.Advance(time.Hour)
		hub.clk.Advance(time.Hour)
	}
	remember(t, laptop)
	advance()
	_, err := laptop.repos.Projects.Create(ctx, "Study", nil)
	require.NoError(t, err)

	c := newClient(t, laptop, hubURL)
	st, err := c.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.Equal(t, map[string]int{"projects": 1}, g.reset(10))
	require.Zero(t, count(t, hub.db, `SELECT count(*) FROM chat_messages`))
	require.Zero(t, count(t, hub.db, `SELECT count(*) FROM claims`))
	_, err = store.GetLocal(ctx, laptop.db.SQL(), store.KeySyncPushSchema)
	require.ErrorIs(t, err, store.ErrNotFound, "the hub's schema is the one assumed")
	require.Equal(t, "10", g.pulls[0].Get("schema_version"), "every pull says the client's schema")

	advance()
	st, err = c.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	// The project is pushed again while it is the newest row pushed, within
	// the overlap, as before.
	want := map[string]int{"projects": 1}
	maps.Copy(want, memoryPushed)
	require.Equal(t, want, g.reset(10))
	require.Equal(t, 1, count(t, hub.db, `SELECT count(*) FROM claims`))
	require.Equal(t, 1, count(t, hub.db, `SELECT count(*) FROM episodes`))
	require.Zero(t, count(t, hub.db, `SELECT count(*) FROM claims WHERE sensitivity = 'sensitive'`))
	require.Equal(t, 1, count(t, hub.db, `SELECT count(*) FROM chat_messages WHERE text = 'My cat Miso is ill'`))
	var confidence float64
	require.NoError(t, hub.db.SQL().QueryRow(`SELECT confidence FROM claims WHERE id = ?`, claimID).Scan(&confidence))
	require.InDelta(t, 0.75, confidence, 0)
	pushed, err := store.GetLocal(ctx, laptop.db.SQL(), store.KeySyncPushSchema)
	require.NoError(t, err)
	require.Equal(t, "10", pushed)

	advance()
	st, err = c.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.Equal(t, map[string]int{"projects": 1}, g.reset(10), "the memory went up once")

	// From now on a change to memory goes with the others.
	_, err = laptop.db.SQL().Exec(`UPDATE claims SET suppressed = 1, updated_at = ?, rev = rev + 1 WHERE id = ?`,
		laptop.clk.Now().UnixMilli(), claimID)
	require.NoError(t, err)
	st, err = c.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.Equal(t, map[string]int{"projects": 1, "claims": 1}, g.reset(10))
	require.Equal(t, 1, count(t, hub.db, `SELECT count(*) FROM claims WHERE suppressed = 1`))
}

// A hub put back to an older gwend no longer takes the newer tables; once it
// is updated again, they go up in full again.
func TestAHubPutBackGetsTheNewerTablesAgain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	hub, g, hubURL := newGatedHub(t)
	laptop := newNode(t)
	remember(t, laptop)
	c := newClient(t, laptop, hubURL)
	projects := 0
	// sync makes a project an hour later and syncs with the hub at schema.
	sync := func(schema int64) map[string]int {
		t.Helper()
		laptop.clk.Advance(time.Hour)
		hub.clk.Advance(time.Hour)
		projects++
		_, err := laptop.repos.Projects.Create(ctx, fmt.Sprint("P", projects), nil)
		require.NoError(t, err)
		g.reset(schema)
		st, err := c.Sync(ctx)
		require.NoError(t, err)
		require.Empty(t, st.LastError)
		return g.reset(schema)
	}
	want := map[string]int{"projects": 1}
	maps.Copy(want, memoryPushed)
	require.Equal(t, want, sync(10), "a first push sends everything once")

	require.Equal(t, map[string]int{"projects": 2}, sync(store.LegacySyncSchema),
		"the new project, and the one before within the overlap")
	pushed, err := store.GetLocal(ctx, laptop.db.SQL(), store.KeySyncPushSchema)
	require.NoError(t, err)
	require.Equal(t, "9", pushed)

	want["projects"] = 2
	require.Equal(t, want, sync(10))
}

// rawPull is one pull as a client sends it, with extra query parameters.
func rawPull(t *testing.T, base, device, extra string) (store.Rows, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/sync/v1/pull?device_id="+device+extra, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var raw struct {
		Rows   json.RawMessage `json:"rows"`
		Cursor string          `json:"cursor"`
		More   bool            `json:"more"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
	require.False(t, raw.More)
	rows, err := store.DecodeRows(raw.Rows)
	require.NoError(t, err)
	return rows, raw.Cursor
}

func tablesOf(r store.Rows) []string {
	out := []string{}
	for table := range r {
		out = append(out, table)
	}
	slices.Sort(out)
	return out
}

// A client that does not say its schema is one from before memory: the hub
// leaves out the tables it cannot take, and its cursor still moves past
// them.
func TestTheHubHoldsBackTablesAClientCannotTake(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	hub, _, srv := newHub(t)
	laptop := newNode(t)
	remember(t, laptop)
	_, err := laptop.repos.Projects.Create(ctx, "Study", nil)
	require.NoError(t, err)
	st, err := newClient(t, laptop, srv.URL).Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.Equal(t, 9, count(t, hub.db, `SELECT count(*) FROM hub_changes`))

	for _, extra := range []string{"", "&schema_version=9", "&schema_version=nonsense", "&schema_version=-1"} {
		rows, cursor := rawPull(t, srv.URL, "old-phone", extra)
		require.Equal(t, []string{"projects"}, tablesOf(rows), extra)
		require.Equal(t, "9", cursor, extra)
	}
	rows, cursor := rawPull(t, srv.URL, "new-phone", "&schema_version=10")
	require.Equal(t, []string{"chat_messages", "chat_threads", "claims", "conflicts", "entities", "episodes",
		"memory_links", "projects", "self_versions"}, tablesOf(rows))
	require.Equal(t, 9, rows.Count())
	require.Equal(t, "9", cursor)
}

// A client whose schema rose pulls everything again, once, to get the rows
// the hub held back while it could not take them.
func TestAClientWhoseSchemaRoseStartsItsPullOver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, _, srv := newHub(t)
	laptop := newNode(t)
	remember(t, laptop)
	_, err := laptop.repos.Projects.Create(ctx, "Study", nil)
	require.NoError(t, err)
	_, err = newClient(t, laptop, srv.URL).Sync(ctx)
	require.NoError(t, err)

	// The phone ran a gwend from before memory, which pulled without saying
	// its schema.
	phone := newNode(t)
	rows, cursor := rawPull(t, srv.URL, phone.db.DeviceID(), "")
	_, err = phone.repos.Sync.Apply(ctx, rows, false)
	require.NoError(t, err)
	require.NoError(t, store.SetLocal(ctx, phone.db.SQL(), store.KeySyncPullCursor, cursor, phone.clk.Now()))
	require.Equal(t, "9", cursor)

	p := newClient(t, phone, srv.URL)
	var applied []store.Applied
	p.o.OnApplied = func(a store.Applied) { applied = append(applied, a) }
	st, err := p.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.Len(t, applied, 1)
	require.Equal(t, []string{claimID}, applied[0].IDs["claims"])
	require.Empty(t, applied[0].IDs["projects"], "the project it had is not written again")
	require.Equal(t, 1, count(t, phone.db, `SELECT count(*) FROM claims`))
	require.Equal(t, 1, count(t, phone.db, `SELECT count(*) FROM chat_messages`))
	pulled, err := store.GetLocal(ctx, phone.db.SQL(), store.KeySyncPullSchema)
	require.NoError(t, err)
	require.Equal(t, "10", pulled)

	applied = nil
	st, err = p.Sync(ctx)
	require.NoError(t, err)
	require.Empty(t, st.LastError)
	require.Empty(t, applied, "once is enough")
	cursor, err = store.GetLocal(ctx, phone.db.SQL(), store.KeySyncPullCursor)
	require.NoError(t, err)
	require.Equal(t, "9", cursor)
}
