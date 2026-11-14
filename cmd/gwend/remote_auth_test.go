package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/remote"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// The remote socket is the one door from the tailnet into this laptop, and
// the hook socket the one way in for `gwen hook` (plan.md 3.14.4, 3.14.6).
// These tests run a real gwend with the remote on and speak HTTP over both
// sockets, as Tailscale Serve and `gwen hook` do.

const (
	w27bOwner = "owner@example.com"
	// w27bToken is the paired phone's cookie.
	w27bToken = "pixel-cookie-token"
)

// w27bRemote is a gwend with the remote on, one paired phone, and the
// sockets it serves.
type w27bRemote struct {
	d        *daemon
	remote   string
	hook     string
	deviceID string
}

// w27bIsolate points every folder a remote gwend looks at (Claude's
// settings, Gwen Terminal's home, the core's socket) into the test, so the
// laptop's own are never read. It sets the environment, so a test using it
// cannot run in parallel.
func w27bIsolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, v := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		p := filepath.Join(dir, strings.ToLower(v))
		require.NoError(t, os.MkdirAll(p, 0o700))
		t.Setenv(v, p)
	}
	t.Setenv("HERDR_APP_HOME", "")
}

// w27bStartRemote starts gwend with [remote] enabled, owner (if any) as the
// Tailscale login allowed in, and one phone paired with w27bToken. peer
// decides who may speak on the remote socket; nil keeps the daemon's own
// rule, uid 0.
func w27bStartRemote(t *testing.T, owner string, peer func(uid int) bool) *w27bRemote {
	t.Helper()
	w27bIsolate(t)
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	db, err := store.Open(ctx, dataDir, clock.NewFake(t0))
	require.NoError(t, err)
	if owner != "" {
		require.NoError(t, store.SetLocal(ctx, db.SQL(), store.KeyRemoteOwner, owner, t0))
	}
	sum := sha256.Sum256([]byte(w27bToken))
	phone, err := store.NewRepos(db).RemoteDevices.Create(ctx, "Pixel", hex.EncodeToString(sum[:]))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// No herdr-core runs: the daemon looks for one at a socket nobody serves.
	core := filepath.Join(t.TempDir(), "core.sock")
	d := startDaemonWith(t, setup{dataDir: dataDir}, func(o *options) {
		cfg := fmt.Sprintf("[remote]\nenabled = true\ncore_socket = %q\n", core)
		require.NoError(t, os.WriteFile(o.configPath, []byte(cfg), 0o600))
		o.remote = true
		o.remotePeer = peer
	})
	runtime := filepath.Dir(d.socket)
	return &w27bRemote{d: d, remote: filepath.Join(runtime, remoteSocketName), hook: filepath.Join(runtime, hookSocketName),
		deviceID: phone.ID}
}

// w27bMe lets this test's own process speak on the remote socket in place
// of tailscaled. It checks the uid it is given, so a guard that asked the
// socket nothing would fail.
func w27bMe(uid int) bool { return uid == os.Getuid() }

// w27bReply is one answer on a socket.
type w27bReply struct {
	status int
	header http.Header
	body   []byte
}

// w27bError is the error code and message of an answer, "" when it is none.
func (r w27bReply) w27bError() (string, string) {
	var e wire.ErrorResponse
	_ = json.Unmarshal(r.body, &e)
	return e.Error.Code, e.Error.Message
}

// w27bCall sends one request over a fresh connection to socket. headers
// are set as given; an event stream is answered with its headers only.
func w27bCall(t *testing.T, socket, method, path string, headers map[string]string, body string) (w27bReply, error) {
	t.Helper()
	hc := &http.Client{
		Timeout: 10 * time.Second, // a failure guard: every answer here comes at once
		Transport: &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}},
	}
	req, err := http.NewRequest(method, "http://gwen.example.ts.net"+path, strings.NewReader(body))
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return w27bReply{}, err
	}
	defer resp.Body.Close()
	out := w27bReply{status: resp.StatusCode, header: resp.Header}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		out.body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
	}
	return out, nil
}

// w27bMust is w27bCall for a request that must get an answer.
func w27bMust(t *testing.T, socket, method, path string, headers map[string]string, body string) w27bReply {
	t.Helper()
	r, err := w27bCall(t, socket, method, path, headers, body)
	require.NoError(t, err, "%s %s", method, path)
	return r
}

// w27bPhone is what Tailscale Serve passes on for the paired phone: the
// owner's login and the phone's cookie, plus X-Gwen when it changes
// something. extra adds or, with "", removes headers.
func w27bPhone(extra map[string]string) map[string]string {
	h := map[string]string{
		"Tailscale-User-Login": w27bOwner,
		"Cookie":               remote.DeviceCookie + "=" + w27bToken,
		"X-Gwen":               "1",
	}
	for k, v := range extra {
		if v == "" {
			delete(h, k)
		} else {
			h[k] = v
		}
	}
	return h
}

// Every request on the remote socket passes four checks or gets 401: the
// owner's Tailscale login, a paired phone's cookie, X-Gwen on a change, and
// (TestRemoteSocketHangsUpOnAnyoneButTailscaled) the peer. Each row takes
// away one thing from a request that otherwise works.
func TestRemoteSocketAuthTable(t *testing.T) {
	r := w27bStartRemote(t, w27bOwner, w27bMe)
	cases := []struct {
		name         string
		method, path string
		headers      map[string]string
		body         string
		status       int
		message      string // the 401's message, when it matters
	}{
		{name: "the paired phone", method: "GET", path: "/v1/code/sessions", headers: w27bPhone(nil), status: 200},
		{name: "the paired phone, a change", method: "POST", path: "/v1/code/sessions/cs_none/mark",
			headers: w27bPhone(nil), body: `{"pinned":true}`, status: 404},
		{name: "no Tailscale login", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"Tailscale-User-Login": ""}), status: 401,
			message: "this Tailscale account may not use Gwen on this laptop"},
		{name: "another Tailscale login", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"Tailscale-User-Login": "friend@example.com"}), status: 401,
			message: "this Tailscale account may not use Gwen on this laptop"},
		{name: "the owner's login with different case", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"Tailscale-User-Login": strings.ToUpper(w27bOwner)}), status: 401},
		{name: "no cookie", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"Cookie": ""}), status: 401, message: "this phone isn't paired"},
		{name: "a cookie nobody paired", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"Cookie": remote.DeviceCookie + "=guess"}), status: 401,
			message: "this phone isn't paired"},
		{name: "an empty cookie", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"Cookie": remote.DeviceCookie + "="}), status: 401},
		{name: "no cookie on the event stream", method: "GET", path: "/v1/code/events",
			headers: w27bPhone(map[string]string{"Cookie": ""}), status: 401},
		// Without X-Gwen a page elsewhere on the tailnet could make the
		// phone's browser post a prompt with its cookie.
		{name: "a POST without X-Gwen", method: "POST", path: "/v1/code/sessions/cs_none/mark",
			headers: w27bPhone(map[string]string{"X-Gwen": ""}), body: `{"pinned":true}`, status: 401, message: "not allowed"},
		{name: "a POST with X-Gwen other than 1", method: "POST", path: "/v1/code/sessions/cs_none/prompt",
			headers: w27bPhone(map[string]string{"X-Gwen": "true"}), body: `{"text":"hi"}`, status: 401},
		{name: "a PUT without X-Gwen", method: "PUT", path: "/v1/code/sessions/cs_none",
			headers: w27bPhone(map[string]string{"X-Gwen": ""}), body: `{}`, status: 401},
		{name: "a DELETE without X-Gwen", method: "DELETE", path: "/v1/code/sessions/cs_none",
			headers: w27bPhone(map[string]string{"X-Gwen": ""}), status: 401},
		{name: "a PATCH without X-Gwen", method: "PATCH", path: "/v1/code/sessions/cs_none",
			headers: w27bPhone(map[string]string{"X-Gwen": ""}), body: `{}`, status: 401},
		{name: "a GET without X-Gwen", method: "GET", path: "/v1/code/sessions",
			headers: w27bPhone(map[string]string{"X-Gwen": ""}), status: 200},
		// The phone app and the pair page load before the phone has a
		// cookie: they skip that one check, and only that one.
		{name: "the app without a cookie", method: "GET", path: "/",
			headers: w27bPhone(map[string]string{"Cookie": ""}), status: 200},
		{name: "the pair page without a cookie", method: "GET", path: "/pair",
			headers: w27bPhone(map[string]string{"Cookie": ""}), status: 200},
		{name: "the app without a Tailscale login", method: "GET", path: "/",
			headers: w27bPhone(map[string]string{"Tailscale-User-Login": "", "Cookie": ""}), status: 401},
		{name: "pairing without a cookie reaches the code check", method: "POST", path: "/pair",
			headers: w27bPhone(map[string]string{"Cookie": ""}), body: `{"code":"wrong","name":"Pixel"}`, status: 401,
			message: remote.ErrBadPairCode.Error()},
		{name: "pairing from another login", method: "POST", path: "/pair",
			headers: w27bPhone(map[string]string{"Cookie": "", "Tailscale-User-Login": "friend@example.com"}),
			body:    `{"code":"wrong","name":"Pixel"}`, status: 401, message: "this Tailscale account may not use Gwen on this laptop"},
		{name: "pairing without X-Gwen", method: "POST", path: "/pair",
			headers: w27bPhone(map[string]string{"Cookie": "", "X-Gwen": ""}), body: `{"code":"wrong","name":"Pixel"}`,
			status: 401, message: "not allowed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := w27bMust(t, r.remote, tc.method, tc.path, tc.headers, tc.body)
			require.Equal(t, tc.status, got.status, "%s", got.body)
			if tc.status != 401 {
				return
			}
			code, msg := got.w27bError()
			require.Equal(t, wire.CodeUnauthorized, code)
			if tc.message != "" {
				require.Equal(t, tc.message, msg)
			}
			require.Equal(t, "no-store", got.header.Get("Cache-Control"))
		})
	}
}

// Revoking a phone on the laptop turns its cookie away on the very next
// request (plan.md 3.14.4).
func TestRemoteSocketRefusesARevokedCookie(t *testing.T) {
	r := w27bStartRemote(t, w27bOwner, w27bMe)
	require.Equal(t, 200, w27bMust(t, r.remote, "GET", "/v1/code/sessions", w27bPhone(nil), "").status)

	revoked := w27bMust(t, r.d.socket, "DELETE", "/v1/remote/devices/"+r.deviceID, nil, "")
	require.Equal(t, http.StatusNoContent, revoked.status, "%s", revoked.body)

	got := w27bMust(t, r.remote, "GET", "/v1/code/sessions", w27bPhone(nil), "")
	require.Equal(t, http.StatusUnauthorized, got.status)
	code, msg := got.w27bError()
	require.Equal(t, wire.CodeUnauthorized, code)
	require.Equal(t, "this phone isn't paired", msg)
	got = w27bMust(t, r.remote, "POST", "/v1/code/sessions/cs_none/mark", w27bPhone(nil), `{"seen":true}`)
	require.Equal(t, http.StatusUnauthorized, got.status)
}

// Before the remote was first turned on there is no owner, and then no
// login at all may come in: the check fails closed.
func TestRemoteSocketWithoutAnOwnerLetsNobodyIn(t *testing.T) {
	r := w27bStartRemote(t, "", w27bMe)
	for _, login := range []string{w27bOwner, ""} {
		got := w27bMust(t, r.remote, "GET", "/v1/code/sessions", w27bPhone(map[string]string{"Tailscale-User-Login": login}), "")
		require.Equal(t, http.StatusUnauthorized, got.status, "login %q", login)
	}
	got := w27bMust(t, r.remote, "GET", "/", w27bPhone(nil), "")
	require.Equal(t, http.StatusUnauthorized, got.status, "not even the app")
}

// Only tailscaled, which runs as root, may speak on the remote socket. Any
// other process gets no answer at all: the connection is closed, even when
// it sends everything a paired phone would.
func TestRemoteSocketHangsUpOnAnyoneButTailscaled(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, this test's process is the peer the socket accepts")
	}
	r := w27bStartRemote(t, w27bOwner, nil)
	for _, path := range []string{"/v1/code/sessions", "/", "/pair"} {
		got, err := w27bCall(t, r.remote, "GET", path, w27bPhone(nil), "")
		// The connection was made and then closed unanswered: not a dial error.
		require.ErrorIs(t, err, io.EOF, "GET %s: no answer, not even a 401, but got %d", path, got.status)
	}
	_, err := w27bCall(t, r.remote, "POST", "/pair", w27bPhone(map[string]string{"Cookie": ""}), `{"code":"x"}`)
	require.ErrorIs(t, err, io.EOF)

	// The laptop's own sockets still answer it.
	require.Equal(t, 200, w27bMust(t, r.d.socket, "GET", "/v1/code/sessions", nil, "").status)
}

// Each socket serves only its own list; everything else is 404, even for
// a caller that passes every check (plan.md 3.14.4). internal/api checks
// every route against the lists; this checks that gwend serves each
// handler on the right socket.
func TestEachSocketServesItsOwnRouteList(t *testing.T) {
	r := w27bStartRemote(t, w27bOwner, w27bMe)
	phone := w27bPhone(nil)

	// The phone's own routes, and Gwen herself on the phone (api/phone.go): your day, tasks, plan, her chats.
	for _, path := range []string{"/v1/code/sessions", "/v1/code/spaces", "/v1/code/events", "/", "/pair", "/v1/status", "/v1/tasks", "/v1/projects", "/v1/plan"} {
		got := w27bMust(t, r.remote, "GET", path, phone, "")
		require.Equal(t, http.StatusOK, got.status, "remote socket: GET %s: %s", path, got.body)
	}
	notOnThePhone := []struct{ method, path, body string }{
		{"GET", "/v1/health", ""}, {"GET", "/v1/events", ""}, {"DELETE", "/v1/tasks/x", ""}, {"POST", "/v1/tasks/delete", `{"ids":["x"]}`},
		{"DELETE", "/v1/agent/chats/x", ""}, {"DELETE", "/v1/projects/x", ""}, {"PATCH", "/v1/projects/x", `{"folder":"/"}`}, {"GET", "/v1/goals", ""},
		{"GET", "/v1/config", ""}, {"PATCH", "/v1/config", `{"remote":{"enabled":false}}`},
		{"GET", "/v1/credentials", ""}, {"PUT", "/v1/credentials/anthropic", `{"value":"x"}`}, {"GET", "/v1/memory/export", ""},
		{"GET", "/v1/remote/status", ""}, {"POST", "/v1/remote/pair-code", "{}"}, {"GET", "/v1/remote/devices", ""},
		{"DELETE", "/v1/remote/devices/" + r.deviceID, ""}, {"POST", "/v1/remote/hooks", `{"on":true}`},
		{"POST", "/v1/code/hook", `{"event":"Stop","terminal":"t1"}`},
	}
	for _, c := range notOnThePhone {
		got := w27bMust(t, r.remote, c.method, c.path, phone, c.body)
		require.Equal(t, http.StatusNotFound, got.status, "remote socket: %s %s: %s", c.method, c.path, got.body)
		code, _ := got.w27bError()
		require.Equal(t, wire.CodeNotFound, code)
	}
	// The phone is still paired: the DELETE above went nowhere.
	require.Equal(t, http.StatusOK, w27bMust(t, r.remote, "GET", "/v1/code/sessions", phone, "").status)

	// The hook socket: the hook, and nothing else.
	got := w27bMust(t, r.hook, "POST", "/v1/code/hook", nil, `{"event":"Stop","terminal":"t1"}`)
	require.Equal(t, http.StatusOK, got.status, "%s", got.body)
	var reply wire.CodeHookReply
	require.NoError(t, json.Unmarshal(got.body, &reply))
	require.Empty(t, reply.Output)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/v1/code/hook", ""}, {"GET", "/v1/code/sessions", ""}, {"POST", "/v1/code/sessions", `{}`},
		{"GET", "/v1/code/events", ""}, {"GET", "/v1/health", ""}, {"GET", "/v1/remote/devices", ""},
		{"POST", "/v1/remote/hooks", `{"on":true}`}, {"GET", "/", ""}, {"POST", "/pair", `{"code":"x"}`},
	} {
		got := w27bMust(t, r.hook, c.method, c.path, nil, c.body)
		require.Equal(t, http.StatusNotFound, got.status, "hook socket: %s %s: %s", c.method, c.path, got.body)
	}

	// The main socket: the code routes and the remote settings, but not the
	// hook or the phone's event stream.
	require.Equal(t, http.StatusOK, w27bMust(t, r.d.socket, "GET", "/v1/code/sessions", nil, "").status)
	devices := w27bMust(t, r.d.socket, "GET", "/v1/remote/devices", nil, "")
	require.Equal(t, http.StatusOK, devices.status)
	require.Contains(t, string(devices.body), r.deviceID)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/v1/code/hook", `{"event":"Stop","terminal":"t1"}`}, {"GET", "/v1/code/events", ""}, {"GET", "/pair", ""},
	} {
		got := w27bMust(t, r.d.socket, c.method, c.path, nil, c.body)
		require.Equal(t, http.StatusNotFound, got.status, "main socket: %s %s: %s", c.method, c.path, got.body)
	}
}

// Both extra sockets are private to the user, like the API socket.
func TestRemoteAndHookSocketsArePrivate(t *testing.T) {
	r := w27bStartRemote(t, w27bOwner, w27bMe)
	for _, p := range []string{r.remote, r.hook} {
		st, err := os.Stat(p)
		require.NoError(t, err)
		require.Equal(t, os.ModeSocket, st.Mode().Type(), p)
		require.Equal(t, os.FileMode(0o600), st.Mode().Perm(), p)
	}
}
