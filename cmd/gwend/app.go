package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/wire"
)

// The app listener (plan.md 3.15). Gwen's screens are a mod inside Gwen
// Terminal, and a mod can fetch over HTTP but cannot open a Unix socket. So
// gwend also serves its API on 127.0.0.1, at a port the system picks, to
// whoever holds the token it wrote for this run in app.json beside the
// socket (mode 0600, so only this user's programs can read it).

// appFileName is the file the Gwen mod reads the port and token from.
const appFileName = "app.json"

// appFile is the content of app.json.
type appFile struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// appListener is the listener while it runs.
type appListener struct {
	srv  *http.Server
	path string
}

// startApp binds the listener, writes app.json into runtime, and serves h.
func startApp(runtime string, h http.Handler) (*appListener, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("app listener: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		ln.Close()
		return nil, fmt.Errorf("app listener: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	path := filepath.Join(runtime, appFileName)
	b, err := json.Marshal(appFile{Port: port, Token: token, PID: os.Getpid()})
	if err == nil {
		err = writePrivate(path, append(b, '\n'))
	}
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("app listener: %w", err)
	}
	// No write timeout: the long poll holds a request for up to 25 s.
	srv := &http.Server{Handler: appGuard(fmt.Sprintf("127.0.0.1:%d", port), token, h), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("app listener stopped", "err", err)
		}
	}()
	slog.Info("app listener on", "port", port)
	return &appListener{srv: srv, path: path}, nil
}

// close stops the listener and removes app.json.
func (a *appListener) close() {
	a.srv.Close()
	os.Remove(a.path)
}

// writePrivate writes a file only this user can read, replacing it in one step.
func writePrivate(path string, data []byte) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// appGuard is the two checks every request on the app listener passes, or
// gets 401: Host is this listener's own address, which a page that reached
// it through a DNS name it controls cannot send, and Authorization carries
// the token. It adds no CORS headers, so a web page can neither read an
// answer nor send the Authorization header.
func appGuard(host, token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(strings.TrimSpace(r.Header.Get("Authorization")))
		if r.Host != host || subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(wire.ErrorResponse{Error: wire.ErrorBody{Code: wire.CodeUnauthorized,
				Message: "not allowed", Details: map[string]any{}}})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
