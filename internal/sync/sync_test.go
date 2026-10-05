package sync

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
