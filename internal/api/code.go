package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/remote"
	"github.com/kzark/gwen/internal/wire"
)

// Gwen on the phone (plan.md 3.14). The code routes are served three times:
// on the main socket for the dashboard and the CLI, on the remote socket for
// paired phones (RemoteHandler, behind the daemon's checks), and one of them
// on the hook socket for `gwen hook` (HookHandler).

type ctxKey int

const (
	connKey ctxKey = iota
	deviceKey
)

// ConnContext keeps the connection with its requests, so a handler can ask
// who is at the other end of the socket. Use it as http.Server.ConnContext.
func ConnContext(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, connKey, c)
}

// PeerCred is the uid and pid of the process at the other end of the Unix
// socket the request came on.
func PeerCred(r *http.Request) (uid, pid int, ok bool) {
	uc, isUnix := r.Context().Value(connKey).(*net.UnixConn)
	if !isUnix {
		return 0, 0, false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, 0, false
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credErr != nil || cred == nil {
		return 0, 0, false
	}
	return int(cred.Uid), int(cred.Pid), true
}

// WithDevice marks a request as coming from a paired phone.
func WithDevice(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), deviceKey, id))
}

func deviceOf(r *http.Request) string {
	id, _ := r.Context().Value(deviceKey).(string)
	return id
}

// withCode answers 503 when the daemon runs no remote.
func (s *Server) withCode(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.Code == nil {
			return Unavailable("code sessions are not running in this daemon")
		}
		return h(w, r)
	}
}

type router func(pattern string, h func(w http.ResponseWriter, r *http.Request) error)

// codeRoutes are the routes a phone and the dashboard share.
func (s *Server) codeRoutes(route router) {
	route("GET /v1/code/sessions", s.withCode(s.codeSessions))
	route("POST /v1/code/sessions", s.withCode(s.codeStart))
	route("GET /v1/code/sessions/{id}", s.withCode(s.codeSession))
	route("GET /v1/code/sessions/{id}/transcript", s.withCode(s.codeTranscript))
	route("GET /v1/code/sessions/{id}/changes", s.withCode(s.codeChanges))
	route("GET /v1/code/sessions/{id}/changes/diff", s.withCode(s.codeDiff))
	route("POST /v1/code/sessions/{id}/prompt", s.withCode(s.codePrompt))
	route("POST /v1/code/sessions/{id}/control", s.withCode(s.codeControl))
	route("POST /v1/code/sessions/{id}/answer", s.withCode(s.codeAnswer))
	route("POST /v1/code/sessions/{id}/mark", s.withCode(s.codeMark))
	route("GET /v1/code/spaces", s.withCode(s.codeSpaces))
	route("POST /v1/code/spaces/{id}/terminal", s.withCode(s.codeNewTerminal))
	route("GET /v1/code/terminals/{t}/view", s.withCode(s.codeView))
	route("POST /v1/code/terminals/{t}/send", s.withCode(s.codeSend))
	route("POST /v1/code/terminals/{t}/put", s.withCode(s.codePut))
	route("GET /v1/code/folders", s.withCode(s.codeFolders))
	route("GET /v1/code/recent-folders", s.withCode(s.codeRecentFolders))
	route("GET /v1/code/conversations", s.withCode(s.codeConversations))
	route("POST /v1/code/transcribe", s.withCode(s.codeTranscribe))
}

// remoteRoutes are the pairing and status routes, for the local owner only.
func (s *Server) remoteRoutes(route router) {
	route("GET /v1/remote/status", s.withCode(s.remoteStatus))
	route("POST /v1/remote/pair-code", s.withCode(s.remotePairCode))
	route("GET /v1/remote/devices", s.withCode(s.remoteDevices))
	route("DELETE /v1/remote/devices/{id}", s.withCode(s.remoteRevoke))
	route("POST /v1/remote/hooks", s.withCode(s.remoteHooks))
}

func (s *Server) router(mux *http.ServeMux) router {
	return func(pattern string, h func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				s.fail(w, r, err)
			}
		})
	}
}

// RemoteHandler is everything a paired phone may reach: the phone app, the
// code routes, and their event stream. Anything else is 404. The daemon
// wraps it in the checks of plan.md 3.14.4; pair handles POST /pair.
func (s *Server) RemoteHandler(app fs.FS, pair http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	route := s.router(mux)
	s.codeRoutes(route)
	route("GET /v1/code/events", s.withCode(s.codeEvents))
	s.pushRoutes(route)
	s.phoneRoutes(route)
	mux.HandleFunc("POST /pair", pair)
	files := http.FileServerFS(app)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			s.fail(w, r, errNotFoundRoute)
			return
		}
		// The app is one page: /pair, /s/<id> and the like are its own routes.
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(app, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
		} else if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, errNotFoundRoute) })
	return mux
}

// HookHandler serves the hook socket: POST /v1/code/hook and nothing else.
func (s *Server) HookHandler() http.Handler {
	mux := http.NewServeMux()
	s.router(mux)("POST /v1/code/hook", s.withCode(s.codeHook))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, errNotFoundRoute) })
	return mux
}

func (s *Server) codeSessions(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, s.Code.Sessions())
	return nil
}

func (s *Server) codeSession(w http.ResponseWriter, r *http.Request) error {
	sess, err := s.Code.Session(r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, sess)
	return nil
}

func (s *Server) codeStart(w http.ResponseWriter, r *http.Request) error {
	var body wire.StartCodeSession
	if err := decode(r, &body); err != nil {
		return err
	}
	// Starting can open Gwen Terminal and wait for it.
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	start := s.Code.Start
	if deviceOf(r) == "" && !body.Create && !body.GitInit {
		// The owner at this laptop (the Gwen window, the CLI) names a folder of their own, a project's: remote.roots
		// limits what a phone may open, not them.
		start = s.Code.StartInProject
	}
	sess, err := start(ctx, body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, sess)
	return nil
}

func queryInt(r *http.Request, name string, fallback int64) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, badRequest(name, "%s must be a whole number", name)
	}
	return n, nil
}

func (s *Server) codeTranscript(w http.ResponseWriter, r *http.Request) error {
	before, err := queryInt(r, "before", -1)
	if err != nil {
		return err
	}
	after, err := queryInt(r, "after", -1)
	if err != nil {
		return err
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		return err
	}
	t, err := s.Code.Transcript(r.PathValue("id"), before, after, int(limit))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

func (s *Server) codeChanges(w http.ResponseWriter, r *http.Request) error {
	c, err := s.Code.Changes(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, c)
	return nil
}

func (s *Server) codeDiff(w http.ResponseWriter, r *http.Request) error {
	d, err := s.Code.Diff(r.Context(), r.PathValue("id"), r.URL.Query().Get("path"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) codePrompt(w http.ResponseWriter, r *http.Request) error {
	var body wire.CodePrompt
	if err := decode(r, &body); err != nil {
		return err
	}
	res, err := s.Code.Prompt(r.Context(), r.PathValue("id"), body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

func (s *Server) codeControl(w http.ResponseWriter, r *http.Request) error {
	var body wire.CodeControl
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := s.Code.Control(r.Context(), r.PathValue("id"), body.Action); err != nil {
		return err
	}
	return s.codeSession(w, r)
}

func (s *Server) codeAnswer(w http.ResponseWriter, r *http.Request) error {
	var body wire.CodeAnswer
	if err := decode(r, &body); err != nil {
		return err
	}
	res, err := s.Code.Answer(r.Context(), r.PathValue("id"), body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

func (s *Server) codeMark(w http.ResponseWriter, r *http.Request) error {
	var body wire.CodeMark
	if err := decode(r, &body); err != nil {
		return err
	}
	sess, err := s.Code.Mark(r.Context(), r.PathValue("id"), body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, sess)
	return nil
}

func (s *Server) codeSpaces(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, s.Code.Spaces(r.Context()))
	return nil
}

func (s *Server) codeNewTerminal(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	t, err := s.Code.NewTerminal(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, t)
	return nil
}

func (s *Server) codeSend(w http.ResponseWriter, r *http.Request) error {
	var body wire.CodePrompt
	if err := decode(r, &body); err != nil {
		return err
	}
	res, err := s.Code.SendTerminal(r.Context(), r.PathValue("t"), body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

// readBody reads a raw body of at most limit bytes.
func readBody(r *http.Request, limit int, what string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	if err != nil {
		return nil, badRequest("", "couldn't read %s", what)
	}
	if len(data) > limit {
		return nil, badRequest("", "%s is too large", what)
	}
	return data, nil
}

// codePut takes one file as the raw body, named by ?name=.
func (s *Server) codePut(w http.ResponseWriter, r *http.Request) error {
	data, err := readBody(r, herdr.MaxPut, "that file")
	if err != nil {
		return err
	}
	res, err := s.Code.Put(r.Context(), r.PathValue("t"), r.URL.Query().Get("name"), data)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, res)
	return nil
}

// codeTranscribe takes 16 kHz mono 16-bit PCM as the raw body.
func (s *Server) codeTranscribe(w http.ResponseWriter, r *http.Request) error {
	pcm, err := readBody(r, remote.MaxSpeech, "that recording")
	if err != nil {
		return err
	}
	res, err := s.Code.Transcribe(r.Context(), pcm)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

func (s *Server) codeFolders(w http.ResponseWriter, r *http.Request) error {
	f, err := s.Code.Folders(r.URL.Query().Get("path"))
	if err != nil {
		if errors.Is(err, remote.ErrOutsideRoots) {
			return badRequest("path", "%s", err.Error())
		}
		return badRequest("path", "that folder can't be opened")
	}
	writeJSON(w, http.StatusOK, f)
	return nil
}

func (s *Server) codeRecentFolders(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, s.Code.RecentFolders())
	return nil
}

func (s *Server) codeConversations(w http.ResponseWriter, r *http.Request) error {
	c, err := s.Code.Conversations(r.Context(), r.URL.Query().Get("dir"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, c)
	return nil
}

// codeUsage serves GET /v1/code/usage: the newest reading of Claude's
// usage limits.
func (s *Server) codeUsage(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, s.Code.Usage())
	return nil
}

// commitSessions serves GET /v1/code/commit-sessions?dir=: the commits
// Claude conversations made in a folder.
func (s *Server) commitSessions(w http.ResponseWriter, r *http.Request) error {
	c, err := s.Code.CommitSessions(r.Context(), r.URL.Query().Get("dir"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, c)
	return nil
}

// summarizeCommit serves POST /v1/code/commit-sessions/summary: Gwen sums
// up a commit from the conversation that made it, and keeps the summary.
func (s *Server) summarizeCommit(w http.ResponseWriter, r *http.Request) error {
	var req wire.SummarizeCommit
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.ClaudeSessionID == "" || req.Commit == "" {
		return badRequest("claude_session_id", "a conversation and a commit are required")
	}
	text, err := s.Code.CommitConversation(req.Dir, req.ClaudeSessionID, req.Commit)
	if err != nil {
		return err
	}
	p, err := s.planner(r.Context())
	if err != nil {
		return err
	}
	out, err := p.CommitSummary(r.Context(), llm.CommitSummaryRequest{Subject: req.Subject, Conversation: text})
	if err != nil {
		return Unavailable("Gwen couldn't sum it up: %v", err)
	}
	if err := s.Code.KeepCommitSummary(r.Context(), req.ClaudeSessionID, req.Commit, out.Summary); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.CommitSummary{Summary: out.Summary})
	return nil
}

// codeHook takes one hook event. The answer waits when a permission request
// is held for the phone.
func (s *Server) codeHook(w http.ResponseWriter, r *http.Request) error {
	var ev wire.CodeHookEvent
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&ev); err != nil {
		return badRequest("", "malformed JSON")
	}
	_, pid, ok := PeerCred(r)
	if !ok {
		writeJSON(w, http.StatusOK, wire.CodeHookReply{})
		return nil
	}
	writeJSON(w, http.StatusOK, s.Code.HandleHook(r.Context(), ev, pid))
	return nil
}

// codeEvents serves GET /v1/code/events: the code events of the hub, for
// the phone. The first frame is code_spaces, so a client that just
// connected knows to load everything once.
func (s *Server) codeEvents(w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("events: response cannot stream")
	}
	// A phone's stream ends the moment it is unpaired or the remote is turned off.
	ctx, cancel := s.Code.PhoneContext(r.Context(), deviceOf(r))
	defer cancel()
	ch, err := s.Hub.subscribe(wire.EventCodeSpaces, wire.CodeSpacesChanged{})
	if err != nil {
		return err
	}
	defer s.Hub.unsubscribe(ch)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	ping := s.Hub.clk.NewTimer(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case frame, ok := <-ch:
			if !ok {
				return nil
			}
			if !phoneFrame(frame) {
				continue
			}
			if _, err := w.Write(frame); err != nil {
				return nil
			}
		case <-ping.C():
			// A named event, not a comment: the phone's page can hear it, and takes a stream that goes quiet for good as dead.
			if _, err := w.Write([]byte("event: ping\ndata: {}\n\n")); err != nil {
				return nil
			}
			ping.Reset(pingInterval)
		}
		flusher.Flush()
	}
}

var upgrader = websocket.Upgrader{ReadBufferSize: 4 << 10, WriteBufferSize: 64 << 10}

// viewOrigin is the check a terminal view's WebSocket passes: the phone app
// is served from the same origin as this socket, and a page from anywhere
// else may not open a terminal. Behind Tailscale Serve the request's Host is
// "localhost", so the published address is asked of the remote instead.
func (s *Server) viewOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if u, err := url.Parse(origin); err == nil && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	return s.Code.PhoneOrigin(r.Context(), origin)
}

// viewMessage is what the phone sends on a terminal view: keys to press, or
// text to type as it is.
type viewMessage struct {
	Keys []string `json:"keys"`
	Text string   `json:"text"`
}

// codeView serves the live terminal (plan.md 3.14.7): output as binary
// messages, the grid as {"cols", "rows"}, the end as {"closed"}; the phone
// sends viewMessage.
func (s *Server) codeView(w http.ResponseWriter, r *http.Request) error {
	// The view ends the moment the phone is unpaired or the remote is turned
	// off: a WebSocket is never refused like a new request would be.
	phone, endPhone := s.Code.PhoneContext(r.Context(), deviceOf(r))
	defer endPhone()
	view, err := s.Code.OpenView(phone, deviceOf(r), r.PathValue("t"))
	if err != nil {
		return err
	}
	defer view.Close()
	up := upgrader
	up.CheckOrigin = s.viewOrigin
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return nil // Upgrade already answered
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(phone)
	defer cancel()
	conn.SetReadLimit(64 << 10)
	go func() {
		defer cancel()
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if kind != websocket.TextMessage {
				continue
			}
			var msg viewMessage
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			callCtx, done := context.WithTimeout(ctx, 10*time.Second)
			if len(msg.Keys) > 0 {
				_ = view.Keys(callCtx, msg.Keys)
			}
			if msg.Text != "" {
				_ = view.Text(callCtx, msg.Text)
			}
			done()
		}
	}()
	write := func(kind int, data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		return conn.WriteMessage(kind, data)
	}
	cols, rows := view.Size()
	if err := write(websocket.TextMessage, fmt.Appendf(nil, `{"cols":%d,"rows":%d}`, cols, rows)); err != nil {
		return nil
	}
	for {
		f, err := view.Next(ctx)
		if err != nil {
			if phone.Err() != nil && r.Context().Err() == nil {
				_ = write(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "this phone can no longer use Gwen"))
			}
			return nil
		}
		switch {
		case f.Resize != nil:
			err = write(websocket.TextMessage, fmt.Appendf(nil, `{"cols":%d,"rows":%d}`, f.Resize.Cols, f.Resize.Rows))
		case f.Closed != nil:
			reason, _ := json.Marshal(map[string]string{"closed": f.Closed.Reason})
			_ = write(websocket.TextMessage, reason)
			_ = write(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return nil
		default:
			if err = write(websocket.BinaryMessage, f.Data); err == nil {
				view.Credit(len(f.Data))
			}
		}
		if err != nil {
			return nil
		}
	}
}

func (s *Server) remoteStatus(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, s.Code.Status(r.Context()))
	return nil
}

func (s *Server) remotePairCode(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	st := s.Code.Status(r.Context())
	if st.URL == "" {
		return Unavailable("Tailscale isn't running, so there is no address for the phone to open")
	}
	code, expires, err := s.Code.NewPairCode()
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.RemotePairCode{Code: code, URL: st.URL + "/pair#" + code, ExpiresAt: expires.UnixMilli()})
	return nil
}

func (s *Server) remoteDevices(w http.ResponseWriter, r *http.Request) error {
	devices, err := s.Code.Devices(r.Context())
	if err != nil {
		return err
	}
	out := wire.RemoteDevices{Devices: []wire.RemoteDevice{}}
	for _, d := range devices {
		out.Devices = append(out.Devices, wire.RemoteDevice{ID: d.ID, Name: d.Name, CreatedAt: d.CreatedAt.UnixMilli(),
			LastSeenAt: wire.MillisPtr(d.LastSeenAt)})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) remoteRevoke(w http.ResponseWriter, r *http.Request) error {
	if err := s.Code.Revoke(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) remoteHooks(w http.ResponseWriter, r *http.Request) error {
	var body wire.RemoteHooks
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := s.Code.SetHooks(body.On); err != nil {
		return err
	}
	return s.remoteStatus(w, r)
}
