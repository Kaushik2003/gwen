// Package sync is the sync hub and its client (docs/07-integrations.md#sync-hub):
// the hub's network API and sign-in on the Raspberry Pi, and the tracking
// device's push and pull.
package sync

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
)

// Pull and session limits.
const (
	PullLimit     = 1000
	SessionMaxAge = 30 * 24 * time.Hour
	CookieName    = "gwen_session"
)

// PushRequest is the POST /sync/v1/push body.
type PushRequest struct {
	DeviceID string          `json:"device_id"`
	Rows     json.RawMessage `json:"rows"`
}

// PushResponse is the POST /sync/v1/push response.
type PushResponse struct {
	Applied int `json:"applied"`
	Ignored int `json:"ignored"`
}

// PullResponse is the GET /sync/v1/pull response.
type PullResponse struct {
	Rows   store.Rows `json:"rows"`
	Cursor string     `json:"cursor"`
	More   bool       `json:"more"`
}

// Health is the GET /sync/v1/health response.
type Health struct {
	OK            bool  `json:"ok"`
	SchemaVersion int64 `json:"schema_version"`
}

// Hub serves the hub API and its sign-in. The token is read from the
// credentials directory at every request, so replacing it needs no restart.
type Hub struct {
	DB      *store.DB
	Repo    store.SyncRepo
	CredDir string
	Clock   clock.Clock
}

func (h *Hub) token() (string, error) {
	tok, err := config.ReadCredentialLine(h.CredDir, config.CredSyncToken)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && tok == "") {
		return "", errors.New("the hub has no sync token; create credentials/sync_token")
	}
	return tok, err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "details": map[string]any{}}})
}

// session is the cookie value for a sign-in at issued: the instant and an
// HMAC of it keyed by the token, so the token itself never sits in a cookie.
func session(token string, issued time.Time) string {
	ts := strconv.FormatInt(issued.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("gwen-session:" + ts))
	return ts + "." + hex.EncodeToString(mac.Sum(nil))
}

// authorized reports whether r carries the token as a bearer or a live
// session cookie.
func (h *Hub) authorized(r *http.Request, token string) bool {
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return subtle.ConstantTimeCompare([]byte(bearer), []byte(token)) == 1
	}
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	ts, _, ok := strings.Cut(c.Value, ".")
	secs, err := strconv.ParseInt(ts, 10, 64)
	if !ok || err != nil {
		return false
	}
	issued := time.Unix(secs, 0)
	if h.Clock.Now().Sub(issued) > SessionMaxAge {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(session(token, issued))) == 1
}

// Require wraps next so that only requests with the token reach it.
func (h *Hub) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := h.token()
		if err != nil {
			slog.Error("hub request refused", "err", err)
			writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
			return
		}
		if !h.authorized(r, token) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign in with the hub's sync token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Login is POST /login: a correct token, as the form field or JSON "token",
// sets the session cookie for 30 days.
func (h *Hub) Login(w http.ResponseWriter, r *http.Request) {
	token, err := h.token()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
		return
	}
	given := ""
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Token string `json:"token"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
		given = body.Token
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		given = r.PostFormValue("token")
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(given)), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "that is not the hub's sync token")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: session(token, h.Clock.Now()), Path: "/",
		MaxAge: int(SessionMaxAge / time.Second), HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

// Handler serves /sync/v1/, every route behind the token.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sync/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Health{OK: true, SchemaVersion: h.DB.SchemaVersion()})
	})
	mux.HandleFunc("POST /sync/v1/push", h.push)
	mux.HandleFunc("GET /sync/v1/pull", h.pull)
	mux.HandleFunc("/sync/v1/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return h.Require(mux)
}

func (h *Hub) push(w http.ResponseWriter, r *http.Request) {
	var req PushRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&req); err != nil || req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "the body must be {\"device_id\", \"rows\"}")
		return
	}
	rows, err := store.DecodeRows(req.Rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("rows: %v", err))
		return
	}
	res, err := h.Repo.Apply(r.Context(), rows, true)
	var se *store.Error
	switch {
	case errors.As(err, &se):
		writeError(w, http.StatusBadRequest, "invalid_request", se.Message)
		return
	case err != nil:
		slog.Error("push failed", "device_id", req.DeviceID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if res.Ignored > 0 {
		slog.Warn("push rows ignored", "device_id", req.DeviceID, "ignored", res.Ignored)
	}
	slog.Info("pushed", "device_id", req.DeviceID, "applied", res.Applied, "ignored", res.Ignored)
	writeJSON(w, http.StatusOK, PushResponse{Applied: res.Applied, Ignored: res.Ignored})
}

func (h *Hub) pull(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	device := q.Get("device_id")
	if device == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "device_id is required")
		return
	}
	cursor, err := store.ParseCursor(q.Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cursor must be one a pull returned")
		return
	}
	limit := PullLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive number")
			return
		}
		limit = min(n, PullLimit)
	}
	rows, next, more, err := h.Repo.Pull(r.Context(), device, cursor, limit)
	if err != nil {
		slog.Error("pull failed", "device_id", device, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, PullResponse{Rows: rows, Cursor: strconv.FormatInt(next, 10), More: more})
}
