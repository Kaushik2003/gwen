package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/remote"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/ui"
)

// Gwen on the phone (plan.md 3.14): the remote service, and its two extra
// sockets beside the API socket. remote.sock is published to the tailnet by
// Tailscale Serve and serves paired phones; hook.sock takes Claude Code's
// hook events from `gwen hook`.

const (
	remoteSocketName = "remote.sock"
	hookSocketName   = "hook.sock"
)

// sibling is a binary installed beside gwend, else the one on PATH.
func sibling(name string) string {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// newRemote builds the remote service over the store.
func newRemote(db *store.DB, repos store.Repos, o options, l *loop, hub *api.Hub) (*remote.Service, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	runtime := filepath.Dir(o.socketPath)
	credDir := config.CredentialsDir(o.dataDir)
	gwen := sibling("gwen")
	if gwen == "" {
		gwen = filepath.Join(home, ".local", "bin", "gwen")
	}
	phone := notify.NewNtfy(nil, credDir)
	return remote.New(remote.Options{
		DB: db, Repos: repos, Clock: o.clk, Config: l.Config, Publish: hub.Publish, Away: l.Away,
		Push: func(ctx context.Context, title, message, click string) error {
			phone.SetConfig(l.Config())
			return phone.Notify(ctx, notify.Notification{Kind: "code", Title: title, Body: message, Click: click})
		},
		Home: home, RuntimeDir: runtime,
		RemoteSocket: filepath.Join(runtime, remoteSocketName), HookSocket: filepath.Join(runtime, hookSocketName),
		OwnDirs: []string{o.dataDir, filepath.Dir(o.configPath)},
		GwenBin: gwen, SttBin: sibling("gwen-stt"), ModelsDir: filepath.Join(o.dataDir, "models"),
		DataDir: o.dataDir, CredDir: credDir, WatchWindow: true,
	})
}

// remoteSockets owns hook.sock, which is always served, and remote.sock,
// which is served while [remote] enabled is true.
type remoteSockets struct {
	svc     *remote.Service
	srv     *api.Server
	cfg     func() config.Config
	runtime string
	// peerOK decides whether the process at the other end of remote.sock
	// may speak: tailscaled, which runs as root. Tests replace it.
	peerOK func(uid int) bool

	mu     sync.Mutex
	remote *http.Server
	hook   *http.Server
}

func listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Only gwend makes these sockets, and the API socket's check has already
	// shown that no other gwend runs, so a file here is stale.
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func serveOn(ln net.Listener, h http.Handler) *http.Server {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ConnContext: api.ConnContext}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("socket stopped", "socket", ln.Addr().String(), "err", err)
		}
	}()
	return srv
}

// sync brings the sockets in line with the configuration.
func (rs *remoteSockets) sync() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.hook == nil {
		path := filepath.Join(rs.runtime, hookSocketName)
		ln, err := listenUnix(path)
		if err != nil {
			slog.Error("hook socket", "err", err)
		} else {
			rs.hook = serveOn(ln, rs.srv.HookHandler())
		}
	}
	enabled := rs.cfg().Remote.Enabled
	path := filepath.Join(rs.runtime, remoteSocketName)
	switch {
	case enabled && rs.remote == nil:
		app, err := fs.Sub(ui.MobileAssets, "dist-mobile")
		if err != nil {
			slog.Error("phone app", "err", err)
			return
		}
		ln, err := listenUnix(path)
		if err != nil {
			slog.Error("remote socket", "err", err)
			return
		}
		rs.remote = serveOn(ln, rs.guard(rs.srv.RemoteHandler(app, rs.pair)))
		slog.Info("phone remote on", "socket", path)
	case !enabled && rs.remote != nil:
		rs.remote.Close()
		// Closing the server never reaches a terminal view's WebSocket.
		rs.svc.EndPhones()
		rs.remote = nil
		os.Remove(path)
		slog.Info("phone remote off")
	}
}

func (rs *remoteSockets) close() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, srv := range []*http.Server{rs.remote, rs.hook} {
		if srv != nil {
			srv.Close()
		}
	}
	rs.svc.EndPhones()
	rs.remote, rs.hook = nil, nil
	os.Remove(filepath.Join(rs.runtime, remoteSocketName))
	os.Remove(filepath.Join(rs.runtime, hookSocketName))
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(wire.ErrorResponse{Error: wire.ErrorBody{Code: wire.CodeUnauthorized, Message: msg,
		Details: map[string]any{}}})
}

// guard is the four checks every request on the remote socket passes, or
// gets 401 (plan.md 3.14.4): the peer is tailscaled, the Tailscale login is
// the owner's, the device cookie belongs to a paired phone, and a request
// that changes something carries X-Gwen: 1. The phone app's own files and
// POST /pair skip the cookie check only, so an unpaired phone can show the
// pairing page.
func (rs *remoteSockets) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := api.PeerCred(r)
		if !ok || !rs.peerOK(uid) {
			// Not Tailscale Serve: say nothing and hang up.
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
					return
				}
			}
			unauthorized(w, "not allowed")
			return
		}
		owner := rs.svc.Owner(r.Context())
		if login := r.Header.Get("Tailscale-User-Login"); owner == "" || login == "" || login != owner {
			unauthorized(w, "this Tailscale account may not use Gwen on this laptop")
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			if r.Header.Get("X-Gwen") != "1" {
				unauthorized(w, "not allowed")
				return
			}
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; "+
			"media-src 'self' blob:; connect-src 'self' wss:; worker-src 'self' blob:; frame-ancestors 'none'")
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		h.Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(remote.DeviceCookie)
		if err != nil {
			unauthorized(w, "this phone isn't paired")
			return
		}
		device, err := rs.svc.Device(r.Context(), cookie.Value)
		if err != nil {
			unauthorized(w, "this phone isn't paired")
			return
		}
		next.ServeHTTP(w, api.WithDevice(r, device.ID))
	})
}

// pair serves POST /pair: it spends a pairing code and sets the device
// cookie.
func (rs *remoteSockets) pair(w http.ResponseWriter, r *http.Request) {
	var body wire.RemotePair
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		unauthorized(w, "that pairing code is not valid")
		return
	}
	token, err := rs.svc.Pair(r.Context(), body.Code, body.Name)
	if err != nil {
		unauthorized(w, remote.ErrBadPairCode.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: remote.DeviceCookie, Value: token, Path: "/", MaxAge: remote.DeviceCookieAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
