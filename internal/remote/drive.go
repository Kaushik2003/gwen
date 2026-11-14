package remote

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

func errorsAs(err error, target any) bool { return errors.As(err, target) }

// Limits of what a phone may send.
const (
	maxPrompt = 32 << 10
	// sendWait is how long a send may wait for the person to finish typing
	// in that terminal at the laptop: herdr-core waits up to 60 seconds.
	sendWait = 75 * time.Second
)

// idemKey is once's key for a request's idempotency key: "" without one,
// so a request that names none always runs.
func idemKey(scope, idem string) string {
	if idem == "" {
		return ""
	}
	return scope + ":" + idem
}

// once runs fn for an idempotency key: the same key within ten minutes
// returns the first result without running again (plan.md 3.14.7).
func (s *Service) once(key string, fn func() (string, error)) (string, error) {
	if key == "" {
		return fn()
	}
	s.mu.Lock()
	if e, ok := s.idem[key]; ok {
		s.mu.Unlock()
		<-e.done
		s.mu.Lock()
		e = s.idem[key]
		s.mu.Unlock()
		return e.result, e.err
	}
	e := idemEntry{at: s.clk.Now(), done: make(chan struct{})}
	s.idem[key] = e
	s.mu.Unlock()
	e.result, e.err = fn()
	s.mu.Lock()
	if e.err != nil {
		delete(s.idem, key) // a failed send may be tried again
	} else {
		s.idem[key] = e
	}
	s.mu.Unlock()
	close(e.done)
	return e.result, e.err
}

func checkPrompt(text string) error {
	if strings.TrimSpace(text) == "" {
		return fail(wire.CodeInvalidRequest, "write something to send")
	}
	if len(text) > maxPrompt {
		return fail(wire.CodeInvalidRequest, "that message is too long; 32 KB at most")
	}
	return nil
}

// Prompt sends the next message to a session. herdr-core pastes it and
// presses Enter once nobody is typing in that terminal at the laptop. The
// result is sent, or queued when Claude was working.
func (s *Service) Prompt(ctx context.Context, id string, p wire.CodePrompt) (wire.CodePromptResult, error) {
	if err := checkPrompt(p.Text); err != nil {
		return wire.CodePromptResult{}, err
	}
	result, err := s.once(idemKey("p:"+id, p.Idem), func() (string, error) {
		s.mu.Lock()
		sess, c, err := s.live(id)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		if sess.State == store.CodeNeedsYou {
			s.mu.Unlock()
			return "", fail(wire.CodeConflict, "Answer Claude first")
		}
		result := wire.PromptSent
		if sess.State == store.CodeWorking {
			result = wire.PromptQueued
		}
		terminal := sess.Terminal
		s.mu.Unlock()
		sendCtx, cancel := context.WithTimeout(ctx, sendWait)
		defer cancel()
		if _, err := c.Send(sendCtx, terminal, p.Text, false); err != nil {
			return "", coreError(err)
		}
		s.mu.Lock()
		if sess := s.sessions[id]; sess != nil {
			s.seen(sess)
			s.save(ctx, sess)
		}
		s.mu.Unlock()
		return result, nil
	})
	return wire.CodePromptResult{Result: result}, err
}

// SendTerminal is Compose: text pasted and submitted in any terminal.
func (s *Service) SendTerminal(ctx context.Context, terminal string, p wire.CodePrompt) (wire.CodePromptResult, error) {
	if err := checkPrompt(p.Text); err != nil {
		return wire.CodePromptResult{}, err
	}
	result, err := s.once(idemKey("t:"+terminal, p.Idem), func() (string, error) {
		s.mu.Lock()
		t, c, err := s.terminal(terminal)
		s.mu.Unlock()
		if err != nil {
			return "", err
		}
		sendCtx, cancel := context.WithTimeout(ctx, sendWait)
		defer cancel()
		if _, err := c.Send(sendCtx, terminal, p.Text, false); err != nil {
			return "", coreError(err)
		}
		if st := claudeStatus(t); st != nil && st.State == "working" {
			return wire.PromptQueued, nil
		}
		return wire.PromptSent, nil
	})
	return wire.CodePromptResult{Result: result}, err
}

// Put saves a photo or file beside a terminal and returns where it landed.
func (s *Service) Put(ctx context.Context, terminal, name string, data []byte) (wire.CodePut, error) {
	if len(data) == 0 {
		return wire.CodePut{}, fail(wire.CodeInvalidRequest, "that file is empty")
	}
	if len(data) > herdr.MaxPut {
		return wire.CodePut{}, fail(wire.CodeInvalidRequest, "that file is too large; 8 MB at most")
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "file"
	}
	s.mu.Lock()
	_, c, err := s.terminal(terminal)
	s.mu.Unlock()
	if err != nil {
		return wire.CodePut{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	path, err := c.Put(ctx, terminal, name, data)
	if err != nil {
		return wire.CodePut{}, coreError(err)
	}
	return wire.CodePut{Path: path}, nil
}

// Control runs one of the session controls of plan.md 3.14.7.
func (s *Service) Control(ctx context.Context, id, action string) error {
	if action == wire.ControlOpenOnLaptop {
		return s.openOnLaptop(ctx, id)
	}
	s.mu.Lock()
	sess, c, err := s.live(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	terminal, state, pid := sess.Terminal, sess.State, sess.PID
	if action == wire.ControlEnd {
		sess.ending = true
	}
	s.mu.Unlock()
	switch action {
	case wire.ControlInterrupt:
		_, err = c.Interrupt(ctx, terminal)
	case wire.ControlCycleMode:
		err = c.Keys(ctx, terminal, []string{"shift+tab"})
	case wire.ControlCompact:
		_, err = s.sendCommand(ctx, c, terminal, "/compact")
	case wire.ControlClear:
		_, err = s.sendCommand(ctx, c, terminal, "/clear")
	case wire.ControlEnd:
		if state == store.CodeWorking || state == store.CodeNeedsYou {
			_ = c.Keys(ctx, terminal, []string{"esc"})
		}
		_, err = s.sendCommand(ctx, c, terminal, "/exit")
		go s.terminate(id, pid)
	default:
		return fail(wire.CodeInvalidRequest, "unknown action %q", action)
	}
	return coreError(err)
}

// sendCommand sends a slash command, into a dialog too if one is open.
func (s *Service) sendCommand(ctx context.Context, c *herdr.Client, terminal, command string) (herdr.SendResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sendWait)
	defer cancel()
	return c.Send(ctx, terminal, command, true)
}

// terminate ends Claude's process if /exit did not, ten seconds later.
func (s *Service) terminate(id string, pid int) {
	t := s.clk.NewTimer(10 * time.Second)
	<-t.C()
	s.mu.Lock()
	sess := s.sessions[id]
	alive := sess != nil && sess.State != store.CodeEnded
	if alive && sess.PID != 0 {
		pid = sess.PID
	}
	s.mu.Unlock()
	if alive && pid > 1 && syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

// openOnLaptop shows a session's terminal in its tab of the Gwen window
// (plan.md 3.15), opening the window when it is closed.
func (s *Service) openOnLaptop(ctx context.Context, id string) error {
	s.mu.Lock()
	sess, _, err := s.live(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	terminal := sess.Terminal
	s.mu.Unlock()
	if err := s.openWindow(ctx, "terminal:"+terminal); err != nil {
		return fail(wire.CodeUnavailable, "Couldn't open the Gwen window")
	}
	return nil
}

// NewTerminal opens a shell in a space: in a project's folder when the
// space is a project (plan.md 3.15), else as a tab of Gwen Terminal.
func (s *Service) NewTerminal(ctx context.Context, space string) (wire.CodeNewTerminal, error) {
	for _, p := range s.projectSpaces(ctx) {
		if p.ID != space {
			continue
		}
		c, err := s.ensureCore(ctx)
		if err != nil {
			return wire.CodeNewTerminal{}, err
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		t, err := runFree(ctx, c, herdr.RunParams{Argv: []string{shell}, Cwd: p.Folder, Name: filepath.Base(p.Folder), Keep: true, NoParent: true,
			Labels: map[string]string{ProjectLabel: p.ID},
			Env:    map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}})
		if err != nil {
			return wire.CodeNewTerminal{}, coreError(err)
		}
		return wire.CodeNewTerminal{Terminal: t.Address}, nil
	}
	found, folder := false, ""
	for _, sp := range s.spacesDoc() {
		if sp.ID == space {
			found, folder = true, sp.Folder
		}
	}
	if !found {
		return wire.CodeNewTerminal{}, fail(wire.CodeNotFound, "no space %s", space)
	}
	if err := s.ensureWindow(ctx); err != nil {
		return wire.CodeNewTerminal{}, fail(wire.CodeUnavailable, "Gwen Terminal isn't open on the laptop")
	}
	before := s.terminalSet()
	args := map[string]any{"space": space}
	if folder != "" {
		// Without it the app starts the shell where the focused terminal is.
		args["folder"] = folder
	}
	reply, err := s.termDo(ctx, "tab.new", args)
	if err != nil {
		return wire.CodeNewTerminal{}, fail(wire.CodeConflict, "%s", err.Error())
	}
	t := replyTerminal(reply)
	if t == "" {
		t = s.awaitNewTerminal(ctx, before, "")
	}
	return wire.CodeNewTerminal{Terminal: t}, nil
}

func (s *Service) terminalSet() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]bool, len(s.terms))
	for addr := range s.terms {
		out[addr] = true
	}
	return out
}

// awaitNewTerminal waits a few seconds for a terminal that was not there
// before; with marker set, one whose command line carries it.
func (s *Service) awaitNewTerminal(ctx context.Context, before map[string]bool, marker string) string {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, addr := range s.sortedTerminals() {
			t := s.terms[addr]
			if before[addr] || !t.Local() {
				continue
			}
			if marker == "" || hasArg(t, marker) {
				s.mu.Unlock()
				return addr
			}
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(100 * time.Millisecond):
		}
	}
	return ""
}

func hasArg(t herdr.Terminal, marker string) bool {
	for _, a := range t.Argv {
		if a == marker {
			return true
		}
	}
	if t.Foreground != nil {
		for _, a := range t.Foreground.Argv {
			if a == marker {
				return true
			}
		}
	}
	return false
}

// running is the live session of a Claude conversation, nil when it is not
// running. s.mu is held.
func (s *Service) running(claudeID string) *session {
	if claudeID == "" {
		return nil
	}
	for _, sess := range s.sessions {
		if sess.State != store.CodeEnded && sess.ClaudeSessionID == claudeID {
			return sess
		}
	}
	return nil
}

// Start starts a Claude Code session in a folder, in a tab of Gwen Terminal
// unless the request says otherwise (plan.md 3.14.5, 3.14.7). The folder
// must be inside remote.roots, as a phone's request must.
func (s *Service) Start(ctx context.Context, req wire.StartCodeSession) (wire.CodeSession, error) {
	return s.start(ctx, req, false)
}

// StartInProject starts a session in a project's folder, which you chose on
// this laptop, so remote.roots (the phone's limit) does not apply. Gwen's
// agent hands its jobs over through it (plan.md 3.16); no phone route does.
func (s *Service) StartInProject(ctx context.Context, req wire.StartCodeSession) (wire.CodeSession, error) {
	req.Create, req.GitInit = false, false
	return s.start(ctx, req, true)
}

func (s *Service) start(ctx context.Context, req wire.StartCodeSession, project bool) (wire.CodeSession, error) {
	if len(req.Prompt) > maxPrompt {
		return wire.CodeSession{}, fail(wire.CodeInvalidRequest, "that message is too long; 32 KB at most")
	}
	dir, err := s.startDir(ctx, req, project)
	if err != nil {
		return wire.CodeSession{}, err
	}
	sandbox := req.Sandbox != nil && *req.Sandbox
	var args []string
	claudeID := uuid.NewString()
	if req.Resume != "" {
		if _, err := uuid.Parse(req.Resume); err != nil {
			return wire.CodeSession{}, fail(wire.CodeInvalidRequest, "that isn't a conversation")
		}
		s.mu.Lock()
		live := s.running(req.Resume)
		s.mu.Unlock()
		if live != nil {
			return wire.CodeSession{}, &Error{Code: wire.CodeConflict, Message: "that conversation is already running",
				Details: map[string]any{"reason": "running", "session_id": live.ID}}
		}
		if last, err := s.repos.CodeSessions.LastOfClaude(ctx, req.Resume); err == nil {
			args = last.Args
			if req.Sandbox == nil {
				sandbox = last.Sandbox
			}
		}
		claudeID = req.Resume
	}
	if sandbox {
		if _, err := exec.LookPath("bwrap"); err != nil {
			return wire.CodeSession{}, fail(wire.CodeUnavailable, "the sandbox needs bubblewrap, which isn't installed")
		}
	}
	switch req.PermissionMode {
	case "":
	case "acceptEdits", "auto", "plan":
		// Kept in the session's args, so a resume runs in the same mode.
		if !slices.Contains(args, "--permission-mode") {
			args = append(args, "--permission-mode", req.PermissionMode)
		}
	default:
		return wire.CodeSession{}, fail(wire.CodeInvalidRequest, "%q isn't a permission mode Gwen starts sessions in", req.PermissionMode)
	}
	name := filepath.Base(dir)
	if n := terminalSlug(req.Name); n != "" {
		name = n
	}
	claude := FindClaude(s.home)
	argv := []string{claude}
	if sandbox {
		// `gwen sandbox` prepares the project's private sandbox state and
		// signs in from Gwen's credentials, so no secret is ever written
		// into a command line that the terminal's record keeps.
		argv = []string{s.opts.GwenBin, "sandbox"}
	}
	argv = append(argv, args...)
	if line := s.statusLineSettings(dir); line != "" {
		// Not kept in the session's args: a resume adds the current one.
		argv = append(argv, "--settings", line)
	}
	if req.Resume != "" {
		argv = append(argv, "--resume", req.Resume)
	} else {
		argv = append(argv, "--session-id", claudeID)
	}
	argv = append(argv, "-n", name)

	c, err := s.ensureCore(ctx)
	if err != nil {
		return wire.CodeSession{}, err
	}
	now := s.clk.Now().Truncate(time.Millisecond)
	sess := &session{CodeSession: store.CodeSession{ID: newSessionID(), ClaudeSessionID: claudeID, Cwd: dir, Sandbox: sandbox,
		Args: args, State: store.CodeStarting, StartedAt: now, UpdatedAt: now, SeenAt: &now}, stateAt: now}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.save(ctx, sess)
	s.mu.Unlock()

	// The session always starts in herdr-core; with a window (the default)
	// the Gwen window then opens its tab (plan.md 3.14.5, 3.15).
	if req.Prompt != "" {
		argv = append(argv, req.Prompt)
	}
	t, err := runFree(ctx, c, herdr.RunParams{Argv: argv, Cwd: dir, Name: name, NoParent: true, Keep: true,
		Labels: map[string]string{SessionLabel: sess.ID},
		Env:    map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor", "TERM_PROGRAM": "herdr"},
		Size:   &herdr.Size{Cols: 120, Rows: 40}})
	if err != nil {
		s.drop(ctx, sess)
		return wire.CodeSession{}, coreError(err)
	}
	terminal := t.Address
	if req.Window == nil || *req.Window {
		if err := s.openWindow(ctx, "terminal:"+terminal); err != nil {
			s.mu.Lock()
			sess.notice = "Couldn't open the Gwen window"
			s.mu.Unlock()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.Terminal == "" && sess.State != store.CodeEnded {
		sess.Terminal = terminal
		// An event may have made a card of its own for this terminal first.
		if other := s.byTerminal[terminal]; other != nil && other != sess {
			delete(s.sessions, other.ID)
			go func(id string) { _ = s.repos.CodeSessions.Delete(context.Background(), id) }(other.ID)
			s.opts.Publish(wire.EventCodeSessionRemoved, wire.CodeSessionRemoved{ID: other.ID})
			sess.sawClaude = other.sawClaude
		}
		s.byTerminal[terminal] = sess
		s.label(terminal, sess.ID)
		s.save(ctx, sess)
		// The core answers terminal.run before it sends the terminal's
		// created event. Until the event is applied the terminal is unknown,
		// and evaluate would end the new card; the event's own evaluate
		// finds the card instead. A kept terminal is never removed first.
		if _, known := s.terms[terminal]; known {
			s.evaluate(ctx, terminal)
		}
	}
	return s.wireSession(sess), nil
}

// drop forgets a session that never started.
func (s *Service) drop(ctx context.Context, sess *session) {
	s.mu.Lock()
	delete(s.sessions, sess.ID)
	s.mu.Unlock()
	_ = s.repos.CodeSessions.Delete(context.WithoutCancel(ctx), sess.ID)
	s.opts.Publish(wire.EventCodeSessionRemoved, wire.CodeSessionRemoved{ID: sess.ID})
}

// PhoneFolder resolves a folder the phone picked for a project, inside the
// folders the phone may open (remote.roots, plan.md 3.14.7), making its last
// element first when create is set (a git repository too with gitInit). A
// project's folder is where Claude starts for it without the roots check, so
// the phone may only name one it could open anyway.
func (s *Service) PhoneFolder(ctx context.Context, dir string, create, gitInit bool) (string, error) {
	return s.startDir(ctx, wire.StartCodeSession{Dir: dir, Create: create, GitInit: create && gitInit}, false)
}

// startDir resolves the folder of a new session, creating its last element
// when asked.
func (s *Service) startDir(ctx context.Context, req wire.StartCodeSession, project bool) (string, error) {
	dir := ExpandHome(strings.TrimSpace(req.Dir), s.home)
	if dir == "" {
		return "", fail(wire.CodeInvalidRequest, "choose a folder")
	}
	if project {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || !filepath.IsAbs(real) {
			return "", fail(wire.CodeNotFound, "that folder doesn't exist")
		}
		if st, err := os.Stat(real); err != nil || !st.IsDir() {
			return "", fail(wire.CodeNotFound, "that folder doesn't exist")
		}
		return real, nil
	}
	if req.Create {
		parent, err := s.allowedDir(filepath.Dir(dir))
		if err != nil {
			return "", fail(wire.CodeInvalidRequest, "%s", ErrOutsideRoots.Error())
		}
		base := filepath.Base(dir)
		if strings.HasPrefix(base, ".") || strings.ContainsAny(base, "/\x00") {
			return "", fail(wire.CodeInvalidRequest, "that folder name can't be used")
		}
		dir = filepath.Join(parent, base)
		if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fail(wire.CodeConflict, "couldn't make that folder: %v", err)
		}
		if req.GitInit {
			if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
				if _, err := git(ctx, dir, "init"); err != nil {
					return "", fail(wire.CodeConflict, "couldn't run git init there")
				}
			}
		}
	}
	real, err := s.allowedDir(dir)
	if err != nil {
		if errors.Is(err, ErrOutsideRoots) {
			return "", fail(wire.CodeInvalidRequest, "%s", ErrOutsideRoots.Error())
		}
		return "", fail(wire.CodeNotFound, "that folder doesn't exist")
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return "", fail(wire.CodeNotFound, "that folder doesn't exist")
	}
	return real, nil
}

// Conversations lists the newest 15 Claude conversations of a folder.
func (s *Service) Conversations(ctx context.Context, dir string) (wire.CodeConversations, error) {
	out := wire.CodeConversations{Conversations: []wire.CodeConversation{}}
	real, err := s.allowedDir(dir)
	if err != nil {
		return out, fail(wire.CodeInvalidRequest, "%s", ErrOutsideRoots.Error())
	}
	for _, file := range s.conversationFiles(real) {
		if len(out.Conversations) == 15 {
			break
		}
		cwd, title, active, ok := transcriptHead(file)
		if !ok || filepath.Clean(cwd) != real {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		conv := wire.CodeConversation{ClaudeSessionID: id, Title: title, ActiveAt: active.UnixMilli()}
		if conv.Title == "" {
			conv.Title = "Untitled conversation"
		}
		if last, err := s.repos.CodeSessions.LastOfClaude(ctx, id); err == nil {
			conv.Sandbox = last.Sandbox
		}
		s.mu.Lock()
		if live := s.running(id); live != nil {
			conv.SessionID = live.ID
		}
		s.mu.Unlock()
		out.Conversations = append(out.Conversations, conv)
	}
	return out, nil
}

// transcriptPath is where a session's conversation is kept.
func (s *Service) transcriptPath(sess *session) string {
	if sess.TranscriptPath != "" {
		if _, err := os.Stat(sess.TranscriptPath); err == nil || !sess.Sandbox {
			return sess.TranscriptPath
		}
	}
	if sess.ClaudeSessionID == "" {
		return ""
	}
	rel := filepath.Join("projects", projectDirName(sess.Cwd), sess.ClaudeSessionID+".jsonl")
	p := filepath.Join(s.home, ".claude", rel)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	// A sandboxed session in a folder with a very long path keeps its
	// conversation in its private state.
	if sess.Sandbox && s.opts.DataDir != "" {
		p = filepath.Join(SandboxStateDir(s.opts.DataDir, sess.Cwd), "claude", rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Transcript pages through a session's conversation: before pages back
// from a cursor (negative: the newest), after returns what is newer.
func (s *Service) Transcript(id string, before, after int64, limit int) (wire.CodeTranscript, error) {
	s.mu.Lock()
	sess := s.sessions[id]
	if sess == nil {
		s.mu.Unlock()
		return wire.CodeTranscript{}, fail(wire.CodeNotFound, "no session %s", id)
	}
	path := s.transcriptPath(sess)
	s.mu.Unlock()
	if path == "" {
		return wire.CodeTranscript{Events: []wire.CodeTranscriptEvent{}}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.files.get(path).page(before, after, limit), nil
}

func (s *Service) row(id string) (store.CodeSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	if sess == nil {
		return store.CodeSession{}, fail(wire.CodeNotFound, "no session %s", id)
	}
	return sess.CodeSession, nil
}

// Changes lists what a session's folder has changed since the last commit.
func (s *Service) Changes(ctx context.Context, id string) (wire.CodeChanges, error) {
	row, err := s.row(id)
	if err != nil {
		return wire.CodeChanges{}, err
	}
	return changes(ctx, row), nil
}

// Diff is one changed file's diff.
func (s *Service) Diff(ctx context.Context, id, path string) (wire.CodeDiff, error) {
	row, err := s.row(id)
	if err != nil {
		return wire.CodeDiff{}, err
	}
	d, ok := diff(ctx, row, path)
	if !ok {
		return wire.CodeDiff{}, fail(wire.CodeNotFound, "no changes in %s", path)
	}
	return d, nil
}

// terminalSlug is a title as herdr-core takes a terminal's name: 1 to 32 of
// a-z 0-9 - _ . starting with a letter or digit ("Fix add bug" is
// "fix-add-bug"); "" when nothing of it is left.
// runFree runs a terminal under its name made valid for herdr-core, or the
// first free one after it (name-2, name-3, ...): the core refuses a name a
// live terminal holds, so a second shell or session in a folder needs its own.
func runFree(ctx context.Context, c *herdr.Client, p herdr.RunParams) (herdr.Terminal, error) {
	base := terminalSlug(p.Name)
	if base == "" {
		base = "shell"
	}
	p.Name = base
	for i := 2; ; i++ {
		t, err := c.Run(ctx, p)
		if err == nil || !herdr.IsKind(err, "name_taken") || i > 99 {
			return t, err
		}
		suffix := "-" + strconv.Itoa(i)
		p.Name = strings.TrimRight(base[:min(len(base), 32-len(suffix))], "-") + suffix
	}
}

func terminalSlug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 32 {
			break
		}
	}
	return strings.TrimRight(b.String(), "-")
}
