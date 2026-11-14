package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// The three sockets serve three different route lists (plan.md 3.14.4):
// the remote socket only the phone app, /pair, and the code routes of the
// API table (3.9) without the hook; the hook socket only POST
// /v1/code/hook; the main socket everything but those two socket-only
// routes. Every route of this package is tried against each list, read from
// the package's own source, so a route added later is refused on the
// phone's socket until someone puts it on the list.

// w27bRoute is one registered pattern: a method and a path.
type w27bRoute struct{ method, path string }

func (r w27bRoute) String() string { return r.method + " " + r.path }

// w27bPhoneRoutes is what a paired phone may call: the code routes of the
// API table in plan.md 3.9, and the phone's event stream.
var w27bPhoneRoutes = []w27bRoute{
	{"GET", "/v1/code/sessions"},
	{"POST", "/v1/code/sessions"},
	{"GET", "/v1/code/sessions/{id}"},
	{"GET", "/v1/code/sessions/{id}/transcript"},
	{"GET", "/v1/code/sessions/{id}/changes"},
	{"GET", "/v1/code/sessions/{id}/changes/diff"},
	{"POST", "/v1/code/sessions/{id}/prompt"},
	{"POST", "/v1/code/sessions/{id}/control"},
	{"POST", "/v1/code/sessions/{id}/answer"},
	{"POST", "/v1/code/sessions/{id}/mark"},
	{"GET", "/v1/code/spaces"},
	{"POST", "/v1/code/spaces/{id}/terminal"},
	{"GET", "/v1/code/terminals/{t}/view"},
	{"POST", "/v1/code/terminals/{t}/send"},
	{"POST", "/v1/code/terminals/{t}/put"},
	{"GET", "/v1/code/folders"},
	{"GET", "/v1/code/recent-folders"},
	{"GET", "/v1/code/conversations"},
	{"POST", "/v1/code/transcribe"},
	{"GET", "/v1/code/events"},
	{"GET", "/v1/code/push"},
	{"PUT", "/v1/code/push"},
	{"DELETE", "/v1/code/push"},
	{"POST", "/v1/code/push/test"},
	// Gwen herself on the phone (api/phone.go): tracking, plan, tasks,
	// projects, Gwen's chats and approvals; nothing that deletes.
	{"GET", "/v1/status"},
	{"POST", "/v1/day/clock-in"},
	{"POST", "/v1/day/clock-out"},
	{"POST", "/v1/break/start"},
	{"POST", "/v1/break/end"},
	{"POST", "/v1/session/switch"},
	{"GET", "/v1/plan"},
	{"GET", "/v1/projects"},
	{"POST", "/v1/projects"},
	{"POST", "/v1/projects/{id}/folder"},
	{"GET", "/v1/tasks"},
	{"POST", "/v1/tasks"},
	{"GET", "/v1/tasks/{id}"},
	{"PATCH", "/v1/tasks/{id}"},
	{"POST", "/v1/tasks/{id}/complete"},
	{"POST", "/v1/tasks/{id}/reopen"},
	{"POST", "/v1/tasks/{id}/hand-off"},
	{"GET", "/v1/agent/status"},
	{"GET", "/v1/agent/chats"},
	{"POST", "/v1/agent/chats"},
	{"GET", "/v1/agent/chats/{id}"},
	{"POST", "/v1/agent/chats/{id}/messages"},
	{"POST", "/v1/agent/chats/{id}/stop"},
	{"GET", "/v1/approvals"},
	{"POST", "/v1/approvals/{id}"},
}

var (
	w27bPatternRE  = regexp.MustCompile(`"(GET|POST|PUT|PATCH|DELETE) (/[^"]*)"`)
	w27bWildcardRE = regexp.MustCompile(`\{[^}]+\}`)
)

// w27bRoutes is every /v1/ pattern registered anywhere in this package.
func w27bRoutes(t *testing.T) []w27bRoute {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	seen := map[w27bRoute]bool{}
	var out []w27bRoute
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range w27bPatternRE.FindAllStringSubmatch(string(src), -1) {
			r := w27bRoute{m[1], m[2]}
			if strings.HasPrefix(r.path, "/v1/") && !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	// A broken pattern above must not pass by finding nothing.
	require.Greater(t, len(out), 120, "the routes of this package were not all found")
	return out
}

// w27bServe sends one request to h and returns the status and the error
// body, if any.
func w27bServe(h http.Handler, method, target string) (int, wire.ErrorBody) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, target, strings.NewReader("{}")))
	var e wire.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	return w.Code, e.Error
}

// w27bTarget fills a pattern's wildcards.
func w27bTarget(path string) string { return w27bWildcardRE.ReplaceAllString(path, "x") }

// w27bRequireNotFound is the answer to a route a socket does not serve.
func w27bRequireNotFound(t *testing.T, h http.Handler, method, target string) {
	t.Helper()
	status, body := w27bServe(h, method, target)
	require.Equal(t, http.StatusNotFound, status, "%s %s", method, target)
	require.Equal(t, wire.CodeNotFound, body.Code, "%s %s", method, target)
	require.Equal(t, "no such endpoint", body.Message, "%s %s", method, target)
}

// w27bRequireServed is the answer of a served code route in a daemon that
// runs no remote: the route was found and said so with 503, never 404.
func w27bRequireServed(t *testing.T, h http.Handler, method, target string) {
	t.Helper()
	status, body := w27bServe(h, method, target)
	require.Equal(t, http.StatusServiceUnavailable, status, "%s %s: %+v", method, target, body)
}

// w27bPhoneApp stands in for ui/dist-mobile.
var w27bPhoneApp = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>Gwen</title>")},
	"assets/app-1234.js":   {Data: []byte("console.log('gwen')")},
	"manifest.webmanifest": {Data: []byte("{}")},
}

func TestRemoteSocketServesOnlyItsRouteList(t *testing.T) {
	t.Parallel()
	paired := 0
	s := &Server{}
	h := s.RemoteHandler(w27bPhoneApp, func(w http.ResponseWriter, _ *http.Request) {
		paired++
		w.WriteHeader(http.StatusNoContent)
	})

	listed := map[w27bRoute]bool{}
	for _, r := range w27bPhoneRoutes {
		listed[r] = true
		w27bRequireServed(t, h, r.method, w27bTarget(r.path))
	}
	for _, r := range w27bRoutes(t) {
		if !listed[r] {
			// The laptop's API, the remote settings, the hook, and any code
			// route the phone was not given.
			w27bRequireNotFound(t, h, r.method, w27bTarget(r.path))
		}
	}

	// The phone app: its files, and its own pages, which are all index.html.
	for _, target := range []string{"/", "/pair", "/s/cs_1", "/assets/app-1234.js", "/manifest.webmanifest"} {
		status, _ := w27bServe(h, http.MethodGet, target)
		require.Equal(t, http.StatusOK, status, target)
	}
	status, _ := w27bServe(h, http.MethodPost, "/pair")
	require.Equal(t, http.StatusNoContent, status)
	require.Equal(t, 1, paired, "POST /pair reaches the pairing handler")

	// A listed path with a method it does not have, and paths nobody serves.
	for _, r := range []w27bRoute{
		{http.MethodPost, "/"}, {http.MethodPut, "/pair"}, {http.MethodDelete, "/pair"}, {http.MethodPost, "/assets/app-1234.js"},
		{http.MethodGet, "/v1/code/sessions/x/prompt"}, {http.MethodDelete, "/v1/code/sessions"},
		{http.MethodPatch, "/v1/code/sessions/x"}, {http.MethodPut, "/v1/code/terminals/x/send"},
		{http.MethodGet, "/v1/code/hook"}, {http.MethodPost, "/v1/code/events"},
		{http.MethodGet, "/v1/"}, {http.MethodGet, "/v1/code"}, {http.MethodGet, "/v1/code/"}, {http.MethodGet, "/v1/nothing"},
	} {
		w27bRequireNotFound(t, h, r.method, r.path)
	}
}

func TestHookSocketServesOnlyTheHook(t *testing.T) {
	t.Parallel()
	h := (&Server{}).HookHandler()
	for _, r := range w27bRoutes(t) {
		if r == (w27bRoute{http.MethodPost, "/v1/code/hook"}) {
			w27bRequireServed(t, h, r.method, r.path)
			continue
		}
		w27bRequireNotFound(t, h, r.method, w27bTarget(r.path))
	}
	for _, r := range []w27bRoute{
		{http.MethodGet, "/v1/code/hook"}, {http.MethodPut, "/v1/code/hook"}, {http.MethodGet, "/"}, {http.MethodGet, "/pair"},
		{http.MethodPost, "/pair"}, {http.MethodGet, "/index.html"},
	} {
		w27bRequireNotFound(t, h, r.method, r.path)
	}
}

// The main socket serves the code routes to the dashboard and the CLI, and
// the remote settings; the hook, the phone's event stream, and its
// notifications live only on their own sockets.
func TestMainSocketLeavesTheHookAndPhoneEventsToTheirSockets(t *testing.T) {
	t.Parallel()
	h := (&Server{}).Handler()
	w27bRequireNotFound(t, h, http.MethodPost, "/v1/code/hook")
	w27bRequireNotFound(t, h, http.MethodGet, "/v1/code/events")
	w27bRequireNotFound(t, h, http.MethodGet, "/v1/code/push")
	for _, r := range w27bRoutes(t) {
		code := strings.HasPrefix(r.path, "/v1/code/") && r.path != "/v1/code/hook" && r.path != "/v1/code/events" &&
			!strings.HasPrefix(r.path, "/v1/code/push")
		if code || strings.HasPrefix(r.path, "/v1/remote/") {
			w27bRequireServed(t, h, r.method, w27bTarget(r.path))
		}
	}
	w27bRequireNotFound(t, h, http.MethodGet, "/pair")
}

// The remote socket's first check and the hook's trust both rest on
// SO_PEERCRED: across a Unix socket PeerCred names this very process, and
// on any other connection it says it cannot tell, so callers fail closed.
func TestPeerCredNamesTheProcessAcrossTheSocket(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "gw")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "p.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)

	type cred struct {
		uid, pid int
		ok       bool
	}
	got := make(chan cred, 1)
	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, pid, ok := PeerCred(r)
		got <- cred{uid, pid, ok}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{Handler: record, ConnContext: ConnContext, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })

	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	resp, err := hc.Get("http://gwend/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, cred{os.Getuid(), os.Getpid(), true}, <-got)

	tcp := httptest.NewUnstartedServer(record)
	tcp.Config.ConnContext = ConnContext
	tcp.Start()
	t.Cleanup(tcp.Close)
	resp, err = tcp.Client().Get(tcp.URL)
	require.NoError(t, err)
	resp.Body.Close()
	require.False(t, (<-got).ok, "a TCP peer has no credentials")

	record.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.False(t, (<-got).ok, "nor does a request without a connection")
}
