// Package remote is Gwen on the phone and Claude Code sessions in Gwen
// Terminal (plan.md 3.14): it follows herdr-core's terminals, keeps a card
// for every Claude Code session in them, takes Claude Code's hook events,
// starts and drives sessions, reads their transcripts and changes, and
// pairs phones. The daemon serves it on the main socket (local owner), the
// remote socket (paired phones through Tailscale Serve), and the hook
// socket (`gwen hook`).
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/term"
)

// SessionLabel is the herdr-core label that ties a terminal to its card, so
// a session keeps the same card after gwend restarts.
const SessionLabel = "gwen.session"

// endedKept is how long an ended session stays listed.
const endedKept = 24 * time.Hour

// Error is a failure the API shows as it is. Code is a wire error code.
type Error struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string { return e.Message }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Options is what the service needs from the daemon.
type Options struct {
	DB     *store.DB
	Repos  store.Repos
	Clock  clock.Clock
	Config func() config.Config
	// Publish sends an event to the dashboard and to paired phones.
	Publish func(name string, data any)
	// Away reports that the laptop is locked or has had no input for two
	// minutes; nil means never away.
	Away func() bool
	// PhoneAway reports that phone notifications should go out: the laptop
	// is locked or has had no input for two minutes. It is kept apart from
	// Away, which only decides whether a permission request waits for the
	// phone; nil falls back to Away.
	PhoneAway func() bool
	// Desktop shows a desktop notification that a session needs you or ended;
	// nil shows none.
	Desktop func(a DesktopAlert)
	// Resolved withdraws that notification once no session needs you any more;
	// nil does nothing.
	Resolved func()
	// Push sends one phone notification; nil sends none.
	Push func(ctx context.Context, title, message, click string) error
	Home string
	// RuntimeDir is $XDG_RUNTIME_DIR/gwen. RemoteSocket and HookSocket are
	// the two extra sockets in it.
	RuntimeDir   string
	RemoteSocket string
	HookSocket   string
	// OwnDirs are Gwen's config and data folders, never opened from a phone.
	OwnDirs []string
	// GwenBin is the gwen CLI, which Claude Code runs as its hook.
	GwenBin string
	// DataDir is Gwen's data folder, where sandboxes keep their private
	// state; CredDir holds Gwen's credentials.
	DataDir string
	CredDir string
	// ModelsDir is where the speech models live; SttBin is gwen-stt.
	ModelsDir string
	SttBin    string

	// Launch and Dial replace systemd-run and the core connection in tests.
	Launch func(ctx context.Context, unit string, argv []string) error
	Dial   func(ctx context.Context, socket string) (*herdr.Client, error)
}

// Service is the remote. It is safe for concurrent use.
type Service struct {
	opts  Options
	db    *store.DB
	repos store.Repos
	clk   clock.Clock
	cfg   func() config.Config
	home  string

	ownDirs []string
	pair    pairing
	origin  phoneOrigin
	files   transcripts
	commits commitScans
	stt     sttHelper
	awake   inhibitor

	mu         sync.Mutex
	core       *herdr.Client
	coreState  string // ok, down, outdated
	coreNote   string
	coreVer    string
	terms      map[string]herdr.Terminal
	spaces     []docSpace
	sessions   map[string]*session
	byTerminal map[string]*session
	idem       map[string]idemEntry
	views      map[string][]*View // open terminal views by device, oldest first
	// phones are the cancel funcs of each phone's open connections, by
	// device and then by number; ended are the phones unpaired since
	// gwend started (phones.go).
	phones     map[string]map[int]context.CancelFunc
	phoneN     int
	ended      map[string]bool
	hookRate   map[int]*rate
	spacesDirt bool
	kick       chan struct{}
	// usage is the newest reading of Claude's usage limits, usageKept the
	// one last written to the database (usage.go).
	usage, usageKept wire.ClaudeUsage
}

// session is a card: the stored row and what only matters while it runs.
type session struct {
	store.CodeSession
	sawClaude bool // Claude has run in its terminal
	worked    bool // it has been working since it was last idle
	notice    string
	lastTool  string
	question  map[string]any // the AskUserQuestion input it is waiting on
	waiting   *wire.CodeWaiting
	held      *hold
	stateAt   time.Time // when State last changed
	pushedAt  time.Time
	pushedMsg string // what the push at pushedAt said
	phoneOwed string // a push the phone has not had, because you were at the laptop
	// alertMsg is a desktop alert waiting for readDialog to learn what the
	// session asks, so the alert can say it; "" when none waits.
	alertMsg string
	ending   bool
	// stat totals its changes for the card; counting is set while a count
	// runs, and recount asks for one more after it.
	stat              *wire.CodeStat
	counting, recount bool
	// status is the state of Claude's own status last acted on: an event
	// that leaves it as it was (a new title, a resize) changes nothing, so
	// it never undoes what a hook said since, such as a held permission.
	status string
}

type idemEntry struct {
	at     time.Time
	result string
	done   chan struct{}
	err    error
}

// New builds the service; Run starts it.
func New(o Options) (*Service, error) {
	if o.Clock == nil || o.Config == nil || o.Repos.CodeSessions == nil || o.Repos.RemoteDevices == nil {
		return nil, errors.New("remote: missing options")
	}
	if o.Publish == nil {
		o.Publish = func(string, any) {}
	}
	s := &Service{opts: o, db: o.DB, repos: o.Repos, clk: o.Clock, cfg: o.Config, home: o.Home,
		coreState: "down", terms: map[string]herdr.Terminal{}, sessions: map[string]*session{},
		byTerminal: map[string]*session{}, idem: map[string]idemEntry{}, views: map[string][]*View{},
		phones: map[string]map[int]context.CancelFunc{}, ended: map[string]bool{},
		hookRate: map[int]*rate{}, kick: make(chan struct{}, 1)}
	for _, d := range o.OwnDirs {
		if real, err := filepath.EvalSymlinks(d); err == nil {
			s.ownDirs = append(s.ownDirs, real)
		}
	}
	s.stt = sttHelper{bin: o.SttBin, models: o.ModelsDir, clk: o.Clock}
	s.loadUsage(context.Background())
	return s, nil
}

// Run follows herdr-core until ctx ends: it connects, reconnects after 1,
// 2, 5, then every 10 seconds, and takes a fresh snapshot each time.
func (s *Service) Run(ctx context.Context) {
	s.load(ctx)
	go s.janitor(ctx)
	delays := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second}
	attempt, started := 0, false
	for ctx.Err() == nil {
		connected := s.follow(ctx)
		if connected {
			attempt = 0
		}
		if !started {
			// Gwen's terminals start with gwend, so the phone finds them after
			// a login without the Gwen window being opened first. Once only: a
			// core stopped by hand later stays stopped until something needs it.
			started = true
			if !connected {
				s.startCoreAtBoot(ctx)
			}
		}
		t := s.clk.NewTimer(delays[min(attempt, len(delays)-1)])
		attempt++
		select {
		case <-ctx.Done():
		case <-t.C():
		case <-s.kick:
		}
		t.Stop()
	}
	s.stt.close()
	s.awake.release()
}

// startCoreAtBoot starts Gwen's core when none is running and one is
// installed. An out-of-date core is left for `make term` to replace.
func (s *Service) startCoreAtBoot(ctx context.Context) {
	s.mu.Lock()
	state := s.coreState
	s.mu.Unlock()
	hc := CoreBin(s.home)
	if state != "down" || hc == "" {
		return
	}
	if err := s.launch(ctx, coreUnit, coreArgv(hc)...); err != nil {
		slog.Warn("start herdr-core", "err", err)
	}
}

// load brings back the sessions that had not ended when gwend last ran.
// Their terminals are found again by label when the core connects.
func (s *Service) load(ctx context.Context) {
	if n, err := s.repos.CodeSessions.Prune(ctx, s.clk.Now().Add(-endedKept)); err == nil && n > 0 {
		slog.Info("pruned code sessions", "rows", n)
	}
	rows, err := s.repos.CodeSessions.List(ctx, s.clk.Now().Add(-endedKept))
	if err != nil {
		slog.Error("load code sessions", "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range rows {
		sess := &session{CodeSession: row, stateAt: row.UpdatedAt, sawClaude: row.State != store.CodeStarting,
			worked: row.State == store.CodeWorking || row.State == store.CodeNeedsYou}
		s.sessions[row.ID] = sess
	}
}

func (s *Service) socket() string {
	if p := s.cfg().Remote.CoreSocket; p != "" {
		return ExpandHome(p, s.home)
	}
	return herdr.DefaultSocket()
}

func (s *Service) dial(ctx context.Context) (*herdr.Client, error) {
	if s.opts.Dial != nil {
		return s.opts.Dial(ctx, s.socket())
	}
	return herdr.Dial(ctx, s.socket())
}

func (s *Service) setCore(state, note, version string) {
	s.mu.Lock()
	changed := s.coreState != state || s.coreNote != note
	s.coreState, s.coreNote, s.coreVer = state, note, version
	s.mu.Unlock()
	if changed {
		s.remoteChanged()
	}
}

// follow holds one connection to the core and applies its events until it
// drops. It reports whether it connected.
func (s *Service) follow(ctx context.Context) bool {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	c, err := s.dial(dialCtx)
	cancel()
	if err != nil {
		s.setCore("down", "Gwen Terminal's core isn't running.", "")
		return false
	}
	defer c.Close()
	hello := c.Hello()
	if hello.Generation != herdr.Generation || !herdr.VersionAtLeast(hello.Version, term.CoreVersion()) {
		s.setCore("outdated", "Gwen Terminal's core is out of date. Run make term.", hello.Version)
		return false
	}
	events, terms, err := c.Subscribe(ctx)
	if err != nil {
		s.setCore("down", "Gwen Terminal's core isn't answering.", hello.Version)
		return false
	}
	s.snapshot(ctx, c, terms)
	s.setCore("ok", "", hello.Version)
	for {
		ev, err := events.Next(ctx)
		if err != nil {
			break
		}
		switch ev.Type {
		case "missed":
			s.snapshot(ctx, c, ev.Missed)
		case "doc":
			var d struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(ev.Data, &d) == nil && d.Name == "herdr-app" {
				s.readDoc(ctx, c)
			}
		default:
			s.mu.Lock()
			if herdr.Apply(s.terms, ev) {
				s.evaluate(ctx, ev.Terminal)
				s.spacesDirt = true
			}
			s.mu.Unlock()
		}
	}
	s.mu.Lock()
	s.core = nil
	s.mu.Unlock()
	if ctx.Err() == nil {
		s.setCore("down", "Gwen Terminal's core stopped.", hello.Version)
	}
	return true
}

// snapshot replaces everything known about the core's terminals.
func (s *Service) snapshot(ctx context.Context, c *herdr.Client, terms []herdr.Terminal) {
	s.readDoc(ctx, c)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.core = c
	s.terms = make(map[string]herdr.Terminal, len(terms))
	for _, t := range terms {
		s.terms[t.Address] = t
	}
	for addr := range s.terms {
		s.evaluate(ctx, addr)
	}
	// Sessions whose terminal is gone, or never came back after a reboot.
	for _, sess := range s.sessions {
		if sess.State == store.CodeEnded {
			continue
		}
		// What a session is waiting on is not stored: read its dialog again.
		if sess.State == store.CodeNeedsYou && sess.waiting == nil && s.byTerminal[sess.Terminal] == sess {
			go s.readDialog(sess.ID)
		}
		if t, ok := s.terms[sess.Terminal]; sess.Terminal == "" || !ok || t.Labels[SessionLabel] != sess.ID {
			if s.byTerminal[sess.Terminal] == sess {
				continue
			}
			s.end(ctx, sess)
		}
	}
	s.spacesDirt = true
}

func (s *Service) readDoc(ctx context.Context, c *herdr.Client) {
	doc, err := c.Doc(ctx, "herdr-app")
	if err != nil {
		return
	}
	spaces := parseSpaces(doc)
	s.mu.Lock()
	s.spaces = spaces
	s.spacesDirt = true
	s.mu.Unlock()
}

// janitor publishes space changes at most twice a second, forgets old
// idempotency keys, drops sessions that ended a day ago, and keeps the
// laptop awake while a session works.
func (s *Service) janitor(ctx context.Context) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		s.mu.Lock()
		dirty := s.spacesDirt
		s.spacesDirt = false
		s.mu.Unlock()
		if dirty {
			s.opts.Publish(wire.EventCodeSpaces, wire.CodeSpacesChanged{})
		}
		if n++; n%20 != 0 { // the rest every ten seconds
			continue
		}
		now := s.clk.Now()
		var gone []string
		busy := false
		s.mu.Lock()
		for key, e := range s.idem {
			if now.Sub(e.at) > 10*time.Minute {
				delete(s.idem, key)
			}
		}
		for id, sess := range s.sessions {
			if sess.EndedAt != nil && now.Sub(*sess.EndedAt) > endedKept {
				delete(s.sessions, id)
				gone = append(gone, id)
			}
			busy = busy || sess.State == store.CodeWorking || sess.State == store.CodeNeedsYou
		}
		s.payPhone()
		for _, sess := range s.sessions {
			if sess.alertMsg != "" && sess.State == store.CodeNeedsYou {
				s.alert(sess) // readDialog never reported back
			}
		}
		s.mu.Unlock()
		for _, id := range gone {
			_ = s.repos.CodeSessions.Delete(ctx, id)
			s.opts.Publish(wire.EventCodeSessionRemoved, wire.CodeSessionRemoved{ID: id})
		}
		s.awake.set(busy, s.cfg().Remote.KeepAwake)
	}
}

func (s *Service) remoteChanged() { s.opts.Publish(wire.EventRemoteChanged, wire.RemoteChanged{}) }

func (s *Service) sortedTerminals() []string {
	out := make([]string, 0, len(s.terms))
	for addr := range s.terms {
		out = append(out, addr)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimPrefix(out[i], "t"))
		b, _ := strconv.Atoi(strings.TrimPrefix(out[j], "t"))
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

var shells = map[string]bool{"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "nu": true, "ksh": true, "tcsh": true}

// claudeStatus is the status Claude Code reported in t, nil when none did.
func claudeStatus(t herdr.Terminal) *herdr.Status {
	if t.Status != nil && t.Status.App == "claude-code" {
		return t.Status
	}
	for _, rec := range t.Records {
		if rec.App == "claude-code" {
			return &rec
		}
	}
	return nil
}

// isClaude reports whether Claude Code runs in t now: it reports as
// claude-code, or the foreground program is claude (plan.md 3.14.5). A
// status left behind by a Claude that died does not count once the
// terminal's own shell is in front again.
func isClaude(t herdr.Terminal) bool {
	if !t.Local() || t.State != "live" {
		return false
	}
	fg := t.Foreground
	if fg != nil {
		name := fg.Name
		if len(fg.Argv) > 0 {
			name = filepath.Base(fg.Argv[0])
		}
		if name == "claude" || fg.Name == "claude" {
			return true
		}
		if fg.PID == t.PID && shells[strings.TrimPrefix(name, "-")] {
			return false
		}
	}
	return claudeStatus(t) != nil
}

// claudePID is Claude's process in t when the foreground is Claude itself.
func claudePID(t herdr.Terminal) int {
	if fg := t.Foreground; fg != nil && (fg.Name == "claude" || len(fg.Argv) > 0 && filepath.Base(fg.Argv[0]) == "claude") {
		return fg.PID
	}
	return 0
}

func newSessionID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	raw := make([]byte, 10)
	if _, err := randRead(raw); err != nil {
		panic(err)
	}
	for i, c := range raw {
		b[i] = alphabet[int(c)%len(alphabet)]
	}
	return "cs_" + string(b)
}

// evaluate brings the card of the terminal at addr in line with the
// terminal: it finds or makes the card, and moves it through the states of
// plan.md 3.14.5. s.mu is held.
func (s *Service) evaluate(ctx context.Context, addr string) {
	t, ok := s.terms[addr]
	sess := s.byTerminal[addr]
	if !ok || t.State != "live" {
		if sess != nil {
			s.end(ctx, sess)
		}
		return
	}
	if !t.Local() {
		return
	}
	claude := isClaude(t)
	if sess == nil {
		if known := s.sessions[t.Labels[SessionLabel]]; known != nil && known.State != store.CodeEnded &&
			(known.Terminal == "" || known.Terminal == addr) {
			sess = known
			sess.Terminal = addr
			s.byTerminal[addr] = sess
		}
	}
	if sess == nil {
		if !claude {
			return
		}
		sess = s.adopt(ctx, t)
	}
	if !claude {
		// Claude left the terminal; one that has not started yet keeps waiting.
		if sess.sawClaude {
			s.end(ctx, sess)
		}
		return
	}
	if !sess.sawClaude && sess.State == store.CodeStarting && claudeStatus(t) == nil {
		// Claude is up but has said nothing yet: it may be asking whether to
		// trust the folder, which it does before it reports any status.
		go s.checkStart(sess.ID)
	}
	sess.sawClaude = true
	if pid := claudePID(t); pid != 0 {
		sess.PID = pid
	}
	state, changed := sess.State, false
	if st := claudeStatus(t); st != nil && st.State != sess.status {
		sess.status = st.State
		switch st.State {
		case "working":
			state, sess.worked = store.CodeWorking, true
		case "blocked":
			state = store.CodeNeedsYou
		case "error":
			state = store.CodeError
		case "done":
			state = store.CodeDone
		case "idle":
			// Finishing a turn is "done" until someone opens it; a Claude
			// that only answered a question of its own just becomes idle.
			switch {
			case sess.worked:
				state, sess.worked = store.CodeDone, false
			case sess.State == store.CodeDone:
			default:
				state = store.CodeIdle
			}
		}
	}
	if state == store.CodeDone && seenInApp(t) {
		state = store.CodeIdle
	}
	if state != sess.State {
		s.setState(sess, state)
		changed = true
	}
	if id, set := t.Labels[TaskLabel]; set && id != sess.TaskID {
		changed = true // linked to a task, or unlinked: save copies it
	}
	if changed || sess.UpdatedAt.IsZero() {
		s.save(ctx, sess)
	}
}

// startGrace is how long a new Claude may stay silent before its card says
// it needs you.
const startGrace = 4 * time.Second

// checkStart looks at a session that started and reported no status: a
// Claude that is waiting at a question of its own (trust this folder?)
// needs you, and only the terminal can answer it.
func (s *Service) checkStart(id string) {
	t := s.clk.NewTimer(startGrace)
	<-t.C()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	if sess == nil || sess.State != store.CodeStarting {
		return
	}
	term, ok := s.terms[sess.Terminal]
	if !ok || !isClaude(term) || claudeStatus(term) != nil {
		return
	}
	sess.State, sess.stateAt, sess.Need = store.CodeNeedsYou, s.clk.Now(), store.NeedOther
	sess.waiting = &wire.CodeWaiting{Kind: wire.WaitingQuestion, Options: []wire.CodeOption{},
		Question: "Claude is asking something before it starts.", Note: "Answer in the Terminal tab or on the laptop"}
	s.push(sess)
	s.save(context.Background(), sess)
}

// seenInApp reports whether the person has looked at the terminal's latest
// turn in Gwen Terminal, which records the turn it last showed in a label.
func seenInApp(t herdr.Terminal) bool {
	seen, err := strconv.ParseInt(t.Labels["herdr.seen"], 10, 64)
	return err == nil && t.Turn > 0 && seen >= t.Turn
}

// adopt makes a card for a terminal that turned out to run Claude, and
// labels the terminal with it. s.mu is held.
func (s *Service) adopt(ctx context.Context, t herdr.Terminal) *session {
	now := s.clk.Now().Truncate(time.Millisecond)
	cwd := t.Cwd
	if t.Foreground != nil && t.Foreground.Cwd != "" {
		cwd = t.Foreground.Cwd
	}
	sess := &session{CodeSession: store.CodeSession{ID: newSessionID(), Terminal: t.Address, Cwd: cwd,
		State: store.CodeIdle, StartedAt: now, UpdatedAt: now}, stateAt: now, sawClaude: true}
	s.sessions[sess.ID] = sess
	s.byTerminal[t.Address] = sess
	s.label(t.Address, sess.ID)
	return sess
}

// label ties a terminal to its card, off the event loop.
func (s *Service) label(terminal, id string) {
	c := s.core
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Label(ctx, terminal, map[string]string{SessionLabel: id}); err != nil {
			slog.Debug("label terminal", "terminal", terminal, "err", err)
		}
	}()
}

// setState moves a card to a state. s.mu is held.
func (s *Service) setState(sess *session, state string) {
	prev := sess.State
	sess.State = state
	sess.stateAt = s.clk.Now()
	if state != store.CodeNeedsYou {
		sess.Need, sess.waiting = "", nil
		s.release(sess, "")
	}
	switch state {
	case store.CodeNeedsYou:
		sess.Need = s.needOf(sess)
		if sess.held == nil {
			go s.readDialog(sess.ID)
		}
	case store.CodeWorking:
		sess.notice = ""
		sess.phoneOwed = ""
	case store.CodeIdle, store.CodeDone, store.CodeError:
		sess.Step = ""
	}
	if prev != state && (state == store.CodeNeedsYou || state == store.CodeDone || state == store.CodeError) {
		s.push(sess)
	}
	if prev == store.CodeNeedsYou && state != store.CodeNeedsYou {
		sess.alertMsg = ""
		s.resolved()
	}
}

func (s *Service) needOf(sess *session) string {
	switch {
	case sess.held != nil || sess.waiting != nil && sess.waiting.Kind == wire.WaitingPermission:
		return store.NeedPermission
	case sess.lastTool == "AskUserQuestion":
		return store.NeedQuestion
	}
	return store.NeedOther
}

// end closes a card: its terminal is gone or Claude left it. s.mu is held.
func (s *Service) end(ctx context.Context, sess *session) {
	if sess.State == store.CodeEnded {
		return
	}
	now := s.clk.Now().Truncate(time.Millisecond)
	s.release(sess, "")
	if s.byTerminal[sess.Terminal] == sess {
		delete(s.byTerminal, sess.Terminal)
		if t, ok := s.terms[sess.Terminal]; ok && t.State == "live" && s.core != nil {
			// The shell stays; the next Claude in it gets a new card.
			c, addr := s.core, sess.Terminal
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = c.Label(ctx, addr, map[string]string{SessionLabel: ""})
			}()
		}
	}
	sess.State, sess.Need, sess.Step, sess.waiting = store.CodeEnded, "", "", nil
	sess.Terminal, sess.PID, sess.EndedAt, sess.stateAt = "", 0, &now, now
	if sess.TranscriptPath != "" {
		s.files.drop(sess.TranscriptPath)
	}
	// A session that never ran Claude and never got a conversation leaves
	// nothing worth resuming.
	if sess.ClaudeSessionID == "" && !sess.sawClaude && sess.notice == "" {
		delete(s.sessions, sess.ID)
		go func(id string) { _ = s.repos.CodeSessions.Delete(context.Background(), id) }(sess.ID)
		s.opts.Publish(wire.EventCodeSessionRemoved, wire.CodeSessionRemoved{ID: sess.ID})
		return
	}
	s.save(ctx, sess)
}

// save stores a card and tells the dashboard and phones. s.mu is held.
func (s *Service) save(ctx context.Context, sess *session) {
	sess.UpdatedAt = s.clk.Now().Truncate(time.Millisecond)
	if t, ok := s.terms[sess.Terminal]; ok && sess.Terminal != "" {
		// The task it works on outlives the terminal's label.
		if id, set := t.Labels[TaskLabel]; set {
			sess.TaskID = id
		}
	}
	row := sess.CodeSession
	if err := s.repos.CodeSessions.Save(context.WithoutCancel(ctx), row); err != nil {
		slog.Error("save code session", "id", sess.ID, "err", err)
	}
	s.opts.Publish(wire.EventCodeSession, s.wireSession(sess))
}

// findBranch reads the branch of the git work tree dir is in.
func findBranch(dir string) string {
	for range 12 {
		if branch, ok := branchOf(dir); ok {
			return branch
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// wireSession is a card as the API sends it. s.mu is held.
func (s *Service) wireSession(sess *session) wire.CodeSession {
	out := wire.CodeSession{
		ID: sess.ID, Terminal: sess.Terminal, Name: filepath.Base(sess.Cwd), Cwd: sess.Cwd, Branch: findBranch(sess.Cwd),
		Sandbox: sess.Sandbox, TaskID: sess.TaskID, State: sess.State, Need: sess.Need, Step: sess.Step, LastMessage: sess.LastMessage,
		PermissionMode: sess.PermissionMode, ClaudeSessionID: sess.ClaudeSessionID, Pinned: sess.Pinned,
		Notice: sess.notice, Helpers: []wire.CodeTerminal{}, Changes: sess.stat,
		StartedAt: sess.StartedAt.UnixMilli(), UpdatedAt: sess.UpdatedAt.UnixMilli(), EndedAt: wire.MillisPtr(sess.EndedAt),
	}
	if sess.stat == nil {
		s.countChanges(sess)
	}
	switch sess.State {
	case store.CodeNeedsYou, store.CodeDone, store.CodeError:
		out.Unread = sess.SeenAt == nil || sess.SeenAt.Before(sess.stateAt)
	}
	if sess.waiting != nil {
		w := *sess.waiting
		w.Options = append([]wire.CodeOption{}, w.Options...)
		out.Waiting = &w
	}
	if sess.Terminal != "" {
		if t, ok := s.terms[sess.Terminal]; ok {
			out.Window = t.Labels["herdr.tab"] != ""
		}
		for _, addr := range s.sortedTerminals() {
			if t := s.terms[addr]; t.Parent == sess.Terminal && t.State == "live" {
				out.Helpers = append(out.Helpers, s.wireTerminal(t))
			}
		}
	}
	return out
}

// Sessions lists the cards: live ones, and those that ended in the last 24
// hours, without ended ones a newer session of the same conversation
// replaced.
func (s *Service) Sessions() wire.CodeSessionList {
	host, _ := os.Hostname()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := wire.CodeSessionList{Sessions: []wire.CodeSession{}, Core: s.coreState, Message: s.coreNote, Machine: host}
	live := map[string]bool{}
	for _, sess := range s.sessions {
		if sess.State != store.CodeEnded && sess.ClaudeSessionID != "" {
			live[sess.ClaudeSessionID] = true
		}
	}
	for _, sess := range s.sessions {
		if sess.State == store.CodeEnded && live[sess.ClaudeSessionID] {
			continue
		}
		out.Sessions = append(out.Sessions, s.wireSession(sess))
	}
	sort.Slice(out.Sessions, func(i, j int) bool { return out.Sessions[i].UpdatedAt > out.Sessions[j].UpdatedAt })
	return out
}

// Session returns one card.
func (s *Service) Session(id string) (wire.CodeSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	if sess == nil {
		return wire.CodeSession{}, fail(wire.CodeNotFound, "no session %s", id)
	}
	return s.wireSession(sess), nil
}

// live returns a card that has a terminal, and the core to reach it with.
// s.mu is held.
func (s *Service) live(id string) (*session, *herdr.Client, error) {
	sess := s.sessions[id]
	if sess == nil {
		return nil, nil, fail(wire.CodeNotFound, "no session %s", id)
	}
	if sess.State == store.CodeEnded || sess.Terminal == "" {
		return nil, nil, fail(wire.CodeConflict, "that session has ended; resume it to keep going")
	}
	if s.core == nil {
		return nil, nil, fail(wire.CodeUnavailable, "Gwen Terminal's core isn't running")
	}
	return sess, s.core, nil
}

// terminal returns a live local terminal and the core. s.mu is held.
func (s *Service) terminal(addr string) (herdr.Terminal, *herdr.Client, error) {
	t, ok := s.terms[addr]
	if !ok || t.State != "live" || !t.Local() {
		return herdr.Terminal{}, nil, fail(wire.CodeNotFound, "no terminal %s", addr)
	}
	if s.core == nil {
		return herdr.Terminal{}, nil, fail(wire.CodeUnavailable, "Gwen Terminal's core isn't running")
	}
	return t, s.core, nil
}

// Mark pins a card or records that it was looked at.
func (s *Service) Mark(ctx context.Context, id string, m wire.CodeMark) (wire.CodeSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	if sess == nil {
		return wire.CodeSession{}, fail(wire.CodeNotFound, "no session %s", id)
	}
	if m.Pinned != nil {
		sess.Pinned = *m.Pinned
	}
	if m.Seen != nil && *m.Seen {
		s.seen(sess)
	}
	s.save(ctx, sess)
	return s.wireSession(sess), nil
}

// seen records that someone looked: a finished session becomes idle.
// s.mu is held.
func (s *Service) seen(sess *session) {
	now := s.clk.Now().Truncate(time.Millisecond)
	sess.SeenAt = &now
	if sess.State == store.CodeDone {
		sess.State, sess.stateAt = store.CodeIdle, now
	}
}

// ensureCore makes sure a core runs, starting one through systemd-run so it
// gets the desktop session's environment (plan.md 3.14.5).
func (s *Service) ensureCore(ctx context.Context) (*herdr.Client, error) {
	s.mu.Lock()
	c, state, note := s.core, s.coreState, s.coreNote
	s.mu.Unlock()
	if c != nil {
		return c, nil
	}
	if state == "outdated" {
		return nil, fail(wire.CodeUnavailable, "%s", note)
	}
	hc := CoreBin(s.home)
	if hc == "" {
		return nil, fail(wire.CodeUnavailable, "Gwen Terminal isn't installed; run make term")
	}
	if err := s.launch(ctx, coreUnit, coreArgv(hc)...); err != nil {
		slog.Warn("start herdr-core", "err", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case s.kick <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		s.mu.Lock()
		c = s.core
		s.mu.Unlock()
		if c != nil {
			return c, nil
		}
	}
	return nil, fail(wire.CodeUnavailable, "Gwen Terminal's core didn't start")
}

// Status is everything Settings › Phone shows.
func (s *Service) Status(ctx context.Context) wire.RemoteStatus {
	cfg := s.cfg().Remote
	ts := tailscaleStatus(ctx)
	hooks := ReadHooks(ClaudeSettingsPath(s.home), s.opts.GwenBin)
	devices, _ := s.repos.RemoteDevices.List(ctx)
	out := wire.RemoteStatus{Enabled: cfg.Enabled, Tailscale: ts.State, Host: ts.Host, Certificates: ts.Certs, Port: cfg.Port,
		Owner: s.Owner(ctx), Socket: s.opts.RemoteSocket, Hooks: hooks.On, HooksNote: hooks.Note,
		Terminal: termBin(s.home) != "", Voice: s.stt.available(), Devices: len(devices)}
	if _, err := exec.LookPath("bwrap"); err == nil {
		out.Sandbox = true
	}
	if token, err := config.ReadCredentialLine(s.opts.CredDir, config.CredClaudeSandboxToken); err == nil && token != "" {
		out.SandboxSignedIn = true
	}
	if ts.Host != "" {
		out.URL = fmt.Sprintf("https://%s:%d", ts.Host, cfg.Port)
		if port := cfg.Port; port == 443 {
			out.URL = "https://" + ts.Host
		}
	}
	if ts.State == "ok" && cfg.Enabled {
		out.Published = servePublished(ctx, cfg.Port, s.opts.RemoteSocket)
		s.claimOwner(ctx, ts.Login)
		out.Owner = s.Owner(ctx)
	}
	s.mu.Lock()
	out.Core, out.CoreVersion = s.coreState, s.coreVer
	s.mu.Unlock()
	return out
}

// SetHooks adds or removes Gwen's hooks in Claude Code's settings.
func (s *Service) SetHooks(on bool) error {
	if err := SetHooks(ClaudeSettingsPath(s.home), s.opts.GwenBin, on); err != nil {
		return fail(wire.CodeConflict, "%s", err.Error())
	}
	s.remoteChanged()
	return nil
}
