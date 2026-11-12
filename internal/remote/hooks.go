package remote

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

func randRead(b []byte) (int, error) { return crand.Read(b) }

// How long a permission request waits for the phone before it goes back to
// the laptop, and how often Gwen checks whether you came back.
const (
	holdFor       = 10 * time.Minute
	presenceCheck = 2 * time.Second
	hookRateLimit = 50 // events per second from one terminal
	maxTouched    = 500
)

// hold is a PermissionRequest hook call kept open while you are away.
type hold struct {
	ch          chan string
	suggestions []map[string]any
}

// rate counts one terminal's hook events in the current second.
type rate struct {
	second int64
	n      int
}

// ppid is the parent of a process, 0 when it cannot be read.
func ppid(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	// "pid (comm) state ppid ..."; comm may hold spaces and parentheses.
	i := strings.LastIndexByte(string(data), ')')
	if i < 0 {
		return 0
	}
	fields := strings.Fields(string(data[i+1:]))
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(fields[1])
	return n
}

// descends reports whether pid is ancestor or one of its descendants.
func descends(pid, ancestor int) bool {
	for range 64 {
		if pid == ancestor {
			return true
		}
		if pid <= 1 {
			return false
		}
		pid = ppid(pid)
	}
	return false
}

// claudeAncestor is the nearest claude process at or above pid.
func claudeAncestor(pid int) int {
	for range 16 {
		if pid <= 1 {
			return 0
		}
		if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil && strings.TrimSpace(string(comm)) == "claude" {
			return pid
		}
		if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil && strings.Contains(exe, "/claude/versions/") {
			return pid
		}
		pid = ppid(pid)
	}
	return 0
}

// HandleHook takes one Claude Code hook event from `gwen hook`. peerPID is
// the sending process, from the socket: the event counts only if that
// process runs inside the terminal it names (plan.md 3.14.6). The reply's
// Output is empty except for a held permission the phone answered.
func (s *Service) HandleHook(ctx context.Context, ev wire.CodeHookEvent, peerPID int) wire.CodeHookReply {
	s.mu.Lock()
	t, ok := s.terms[ev.Terminal]
	if !ok || !t.Local() || t.State != "live" || t.PID == 0 || !descends(peerPID, t.PID) {
		s.mu.Unlock()
		return wire.CodeHookReply{}
	}
	now := s.clk.Now()
	r := s.hookRate[t.PID]
	if r == nil {
		r = &rate{}
		s.hookRate[t.PID] = r
	}
	if sec := now.Unix(); r.second != sec {
		r.second, r.n = sec, 0
	}
	if r.n++; r.n > hookRateLimit {
		s.mu.Unlock()
		return wire.CodeHookReply{}
	}

	sess := s.byTerminal[ev.Terminal]
	if sess == nil {
		sess = s.adopt(ctx, t)
	}
	sess.sawClaude = true
	if pid := claudeAncestor(peerPID); pid != 0 {
		sess.PID = pid
	}
	if ev.TranscriptPath != "" && within(ev.TranscriptPath, filepath.Join(s.home, ".claude")) {
		sess.TranscriptPath = ev.TranscriptPath
	}
	if ev.PermissionMode != "" {
		sess.PermissionMode = ev.PermissionMode
	}
	sess.Sandbox = sess.Sandbox || ev.Sandbox
	if ev.SessionID != "" && ev.SessionID != sess.ClaudeSessionID {
		sess.ClaudeSessionID = ev.SessionID
	}
	if ev.SessionID != "" {
		s.oneCard(ctx, sess)
	}

	switch ev.Event {
	case "SessionStart":
		if ev.Cwd != "" {
			sess.Cwd = ev.Cwd
		}
		if sess.State == store.CodeStarting {
			s.setState(sess, store.CodeIdle)
		}
		if len(sess.StartStatus) == 0 {
			go s.snapshotStatus(sess.ID, sess.Cwd)
		}
	case "PreToolUse":
		sess.lastTool = ev.ToolName
		sess.Step = step(ev.ToolName, ev.ToolSummary)
		if ev.ToolName == "AskUserQuestion" {
			sess.question = ev.ToolInput
		}
	case "PostToolUse":
		if p := ev.ToolSummary; p != "" && len(sess.Touched) < maxTouched {
			seen := false
			for _, known := range sess.Touched {
				seen = seen || known == p
			}
			if !seen {
				sess.Touched = append(sess.Touched, p)
			}
		}
	case "Stop":
		sess.LastMessage = truncate(ev.LastMessage, 300)
		sess.Step, sess.lastTool, sess.question = "", "", nil
	case "SessionEnd":
		sess.Step = ""
		s.release(sess, "")
	case "PermissionRequest":
		return s.permission(ctx, sess, ev)
	default:
		s.mu.Unlock()
		return wire.CodeHookReply{}
	}
	s.save(ctx, sess)
	s.mu.Unlock()
	return wire.CodeHookReply{}
}

// oneCard keeps one card per conversation: ended cards of the conversation
// this session now runs are dropped (plan.md 3.14.5). s.mu is held.
func (s *Service) oneCard(ctx context.Context, sess *session) {
	for id, other := range s.sessions {
		if other == sess || other.State != store.CodeEnded || other.ClaudeSessionID != sess.ClaudeSessionID {
			continue
		}
		if other.Pinned {
			sess.Pinned = true
		}
		delete(s.sessions, id)
		go func(id string) { _ = s.repos.CodeSessions.Delete(context.Background(), id) }(id)
		s.opts.Publish(wire.EventCodeSessionRemoved, wire.CodeSessionRemoved{ID: id})
	}
	_ = ctx
}

// snapshotStatus records which files were already changed when the session
// started.
func (s *Service) snapshotStatus(id, cwd string) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	status := startStatus(ctx, cwd)
	if status == nil {
		return
	}
	// Paths are relative to the work tree, which Changes also uses.
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[id]; sess != nil && len(sess.StartStatus) == 0 {
		status[""] = true // a marker, so an empty tree is not snapshotted again
		sess.StartStatus = status
		s.save(ctx, sess)
	}
}

// step is what Claude is doing, in a few words.
func step(tool, summary string) string {
	switch tool {
	case "Bash":
		return "Running " + truncate(summary, 80)
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return "Editing " + shortPath(summary)
	case "Read":
		return "Reading " + shortPath(summary)
	case "Grep", "Glob", "WebSearch":
		return "Searching"
	case "":
		return ""
	}
	return tool
}

func shortPath(p string) string {
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) > 2 {
		return filepath.Join(parts[len(parts)-2:]...)
	}
	return p
}

func (s *Service) away() bool { return s.opts.Away != nil && s.opts.Away() }

// permission handles a PermissionRequest. While you are away the call stays
// open and the phone's answer becomes the hook's output; at the laptop it
// returns at once and Claude Code shows its own dialog. s.mu is held and is
// released here.
func (s *Service) permission(ctx context.Context, sess *session, ev wire.CodeHookEvent) wire.CodeHookReply {
	w := &wire.CodeWaiting{Kind: wire.WaitingPermission, Tool: ev.ToolName, Summary: ev.ToolSummary, Options: []wire.CodeOption{}}
	if ev.ToolName == "AskUserQuestion" {
		// A question is answered in its own dialog, never by the hook.
		sess.question = ev.ToolInput
		s.mu.Unlock()
		return wire.CodeHookReply{}
	}
	if !s.away() {
		sess.waiting = w
		sess.Need = store.NeedPermission
		id := sess.ID
		s.save(ctx, sess)
		s.mu.Unlock()
		go s.readDialog(id)
		return wire.CodeHookReply{}
	}
	h := &hold{ch: make(chan string, 1), suggestions: ev.Suggestions}
	until := s.clk.Now().Add(holdFor).UnixMilli()
	w.Held, w.HeldUntil = true, &until
	w.Options = append(w.Options, wire.CodeOption{Key: "allow", Label: "Allow once", Kind: "allow"})
	for i, sug := range ev.Suggestions {
		if label := suggestionLabel(sug); label != "" {
			w.Options = append(w.Options, wire.CodeOption{Key: fmt.Sprintf("always:%d", i), Label: label, Kind: "always"})
		}
	}
	w.Options = append(w.Options, wire.CodeOption{Key: "deny", Label: "Deny", Kind: "deny"})
	s.release(sess, "")
	sess.held, sess.waiting, sess.notice = h, w, ""
	if sess.State != store.CodeNeedsYou {
		s.setState(sess, store.CodeNeedsYou)
		sess.waiting, sess.held = w, h // setState cleared nothing here, but keep them explicit
	}
	sess.Need = store.NeedPermission
	id := sess.ID
	s.save(ctx, sess)
	s.mu.Unlock()

	timeout := s.clk.NewTimer(holdFor)
	defer timeout.Stop()
	check := time.NewTicker(presenceCheck)
	defer check.Stop()
	for {
		select {
		case out := <-h.ch:
			return wire.CodeHookReply{Output: out}
		case <-ctx.Done():
			s.unhold(id, h, "")
			return wire.CodeHookReply{}
		case <-timeout.C():
			s.unhold(id, h, "Moved back to the laptop")
			return wire.CodeHookReply{}
		case <-check.C:
			if !s.away() {
				s.unhold(id, h, "Moved back to the laptop")
				return wire.CodeHookReply{}
			}
		}
	}
}

// unhold gives a held request back to Claude Code's own dialog.
func (s *Service) unhold(id string, h *hold, notice string) {
	s.mu.Lock()
	sess := s.sessions[id]
	if sess == nil || sess.held != h {
		s.mu.Unlock()
		return
	}
	sess.held = nil
	if sess.waiting != nil {
		w := *sess.waiting
		w.Held, w.HeldUntil, w.Options = false, nil, []wire.CodeOption{}
		sess.waiting = &w
	}
	sess.notice = notice
	s.save(context.Background(), sess)
	s.mu.Unlock()
	go s.readDialog(id)
}

// release answers a held request, with a decision or with none. s.mu is held.
func (s *Service) release(sess *session, output string) {
	if sess.held == nil {
		return
	}
	select {
	case sess.held.ch <- output:
	default:
	}
	sess.held = nil
}

// suggestionLabel says one of Claude Code's permission suggestions in words.
func suggestionLabel(sug map[string]any) string {
	where := ""
	switch sug["destination"] {
	case "session":
		where = " for this session"
	case "localSettings", "projectSettings":
		where = " in this project"
	case "userSettings":
		where = " everywhere"
	}
	switch sug["type"] {
	case "addRules":
		rules, _ := sug["rules"].([]any)
		var names []string
		for _, r := range rules {
			rule, _ := r.(map[string]any)
			name, _ := rule["toolName"].(string)
			if content, _ := rule["ruleContent"].(string); content != "" {
				name += " " + truncate(content, 60)
			}
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			return ""
		}
		verb := "Always allow "
		if sug["behavior"] == "deny" {
			return ""
		}
		return verb + strings.Join(names, ", ") + where
	case "setMode":
		mode, _ := sug["mode"].(string)
		words := map[string]string{"acceptEdits": "Accept edits without asking", "plan": "Switch to plan mode",
			"default": "Ask before edits", "auto": "Let Claude decide (auto)", "bypassPermissions": ""}[mode]
		if words == "" {
			return ""
		}
		return words + where
	case "addDirectories":
		dirs, _ := sug["directories"].([]any)
		var names []string
		for _, d := range dirs {
			if p, ok := d.(string); ok {
				names = append(names, p)
			}
		}
		if len(names) == 0 {
			return ""
		}
		return "Always allow access to " + strings.Join(names, ", ") + where
	}
	return ""
}

// optionLine matches one numbered choice of a Claude Code dialog, with its
// box border and selection arrow.
var optionLine = regexp.MustCompile(`^[\s│|┃]*(?:[❯>›▶→]\s*)?(\d{1,2})\.\s+(.*?)[\s│|┃]*$`)

// dialogOptions finds the numbered choices of the dialog at the bottom of a
// screen: the last run of lines numbered 1, 2, 3, ...
func dialogOptions(screen string) []wire.CodeOption {
	var best, run []wire.CodeOption
	for _, line := range strings.Split(screen, "\n") {
		m := optionLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n == 1 {
			run = nil
		}
		if n != len(run)+1 {
			run = nil
			continue
		}
		run = append(run, wire.CodeOption{Key: "option:" + m[1], Label: strings.TrimSpace(m[2]), Kind: optionKind(m[2], n)})
		if len(run) >= 2 {
			best = append([]wire.CodeOption{}, run...)
		}
	}
	return best
}

func optionKind(label string, n int) string {
	l := strings.ToLower(label)
	switch {
	case strings.HasPrefix(l, "no"):
		return "deny"
	case strings.HasPrefix(l, "yes") && (strings.Contains(l, "don't ask") || strings.Contains(l, "always") || strings.Contains(l, "allow all")):
		return "always"
	case strings.HasPrefix(l, "yes") && n == 1:
		return "allow"
	}
	return "option"
}

// questionWaiting builds the phone's choices from an AskUserQuestion input:
// one question with a single choice can be answered from the phone.
func questionWaiting(input map[string]any) *wire.CodeWaiting {
	w := &wire.CodeWaiting{Kind: wire.WaitingQuestion, Tool: "AskUserQuestion", Options: []wire.CodeOption{}}
	questions, _ := input["questions"].([]any)
	if len(questions) != 1 {
		w.Note = "Answer in the Terminal tab or on the laptop"
		return w
	}
	q, _ := questions[0].(map[string]any)
	w.Question, _ = q["question"].(string)
	if multi, _ := q["multiSelect"].(bool); multi {
		w.Note = "Answer in the Terminal tab or on the laptop"
		return w
	}
	opts, _ := q["options"].([]any)
	for i, o := range opts {
		opt, _ := o.(map[string]any)
		label, _ := opt["label"].(string)
		if label == "" {
			continue
		}
		w.Options = append(w.Options, wire.CodeOption{Key: fmt.Sprintf("option:%d", i+1), Label: label, Kind: "option"})
	}
	if len(w.Options) == 0 {
		w.Note = "Answer in the Terminal tab or on the laptop"
	}
	return w
}

// readDialog reads the dialog a session that needs you is showing on the
// laptop, so the phone can offer its choices.
func (s *Service) readDialog(id string) {
	time.Sleep(400 * time.Millisecond) // the dialog draws after the status changes
	s.mu.Lock()
	sess := s.sessions[id]
	if sess == nil || sess.held != nil || sess.Terminal == "" || s.core == nil {
		s.mu.Unlock()
		return
	}
	c, terminal := s.core, sess.Terminal
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	screen, err := c.Read(ctx, terminal, 60)
	if err != nil {
		return
	}
	options := dialogOptions(screen)
	s.mu.Lock()
	defer s.mu.Unlock()
	sess = s.sessions[id]
	if sess == nil || sess.held != nil || sess.Terminal != terminal {
		return
	}
	if sess.State != store.CodeNeedsYou && sess.waiting == nil {
		return
	}
	var w wire.CodeWaiting
	switch {
	case sess.lastTool == "AskUserQuestion" && sess.question != nil:
		w = *questionWaiting(sess.question)
	case sess.waiting != nil:
		w = *sess.waiting
		w.Options = options
	case len(options) > 0:
		w = wire.CodeWaiting{Kind: wire.WaitingPermission, Options: options}
	default:
		// Something only the terminal can answer, such as Claude asking
		// whether to trust the folder.
		w = wire.CodeWaiting{Kind: wire.WaitingQuestion, Question: "Claude is waiting for an answer in its terminal."}
	}
	if w.Options == nil {
		w.Options = []wire.CodeOption{}
	}
	if len(w.Options) == 0 && w.Note == "" {
		w.Note = "Answer in the Terminal tab or on the laptop"
	}
	sess.waiting = &w
	sess.Need = s.needOf(sess)
	s.save(ctx, sess)
}

// Answer gives the phone's choice to a session that is waiting. For a held
// permission it decides; otherwise it presses the choice's number in the
// laptop's dialog and checks that the dialog went away.
func (s *Service) Answer(ctx context.Context, id string, a wire.CodeAnswer) (wire.CodeAnswerResult, error) {
	s.mu.Lock()
	sess, c, err := s.live(id)
	if err != nil {
		s.mu.Unlock()
		return wire.CodeAnswerResult{}, err
	}
	if h := sess.held; h != nil {
		decision := map[string]any{}
		switch {
		case a.Key == "allow":
			decision["behavior"] = "allow"
		case strings.HasPrefix(a.Key, "always:"):
			i, err := strconv.Atoi(strings.TrimPrefix(a.Key, "always:"))
			if err != nil || i < 0 || i >= len(h.suggestions) {
				s.mu.Unlock()
				return wire.CodeAnswerResult{}, fail(wire.CodeInvalidRequest, "no such choice")
			}
			decision["behavior"] = "allow"
			decision["updatedPermissions"] = []any{h.suggestions[i]}
		case a.Key == "deny":
			msg := strings.TrimSpace(a.Message)
			if msg == "" {
				msg = "Denied from your phone"
			}
			decision["behavior"], decision["message"] = "deny", truncate(msg, 2000)
		default:
			s.mu.Unlock()
			return wire.CodeAnswerResult{}, fail(wire.CodeInvalidRequest, "no such choice")
		}
		out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PermissionRequest", "decision": decision}})
		s.release(sess, string(out))
		sess.waiting, sess.Need = nil, ""
		s.seen(sess)
		if decision["behavior"] == "allow" {
			sess.State, sess.stateAt = store.CodeWorking, s.clk.Now()
		}
		s.save(ctx, sess)
		s.mu.Unlock()
		return wire.CodeAnswerResult{OK: true}, nil
	}
	w := sess.waiting
	digit, isOption := strings.CutPrefix(a.Key, "option:")
	valid := false
	if w != nil && isOption {
		for _, o := range w.Options {
			valid = valid || o.Key == a.Key
		}
	}
	terminal := sess.Terminal
	s.mu.Unlock()
	if !valid {
		return wire.CodeAnswerResult{}, fail(wire.CodeConflict, "that choice isn't on the laptop's screen any more")
	}
	before, _ := c.Read(ctx, terminal, 60)
	if len(dialogOptions(before)) == 0 {
		return wire.CodeAnswerResult{Message: "Didn't go through. Answer on the laptop."}, nil
	}
	if err := c.Keys(ctx, terminal, []string{digit}); err != nil {
		return wire.CodeAnswerResult{}, coreError(err)
	}
	wait := s.clk.NewTimer(2 * time.Second)
	select {
	case <-wait.C():
	case <-ctx.Done():
		wait.Stop()
		return wire.CodeAnswerResult{}, ctx.Err()
	}
	after, err := c.Read(ctx, terminal, 60)
	if err == nil && sameOptions(dialogOptions(before), dialogOptions(after)) {
		return wire.CodeAnswerResult{Message: "Didn't go through. Answer on the laptop."}, nil
	}
	s.mu.Lock()
	if sess := s.sessions[id]; sess != nil {
		sess.waiting = nil
		s.seen(sess)
		s.save(ctx, sess)
	}
	s.mu.Unlock()
	return wire.CodeAnswerResult{OK: true}, nil
}

func sameOptions(a, b []wire.CodeOption) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range a {
		if a[i].Label != b[i].Label {
			return false
		}
	}
	return true
}

// coreError turns a herdr-core error into one the API shows.
func coreError(err error) error {
	switch {
	case err == nil:
		return nil
	case herdr.IsKind(err, "not_found"):
		return fail(wire.CodeNotFound, "that terminal is gone")
	case herdr.IsKind(err, "blocked"):
		return fail(wire.CodeConflict, "Answer Claude first")
	case herdr.IsKind(err, "unavailable"):
		return fail(wire.CodeUnavailable, "that terminal is restarting; try again in a moment")
	}
	var e *herdr.Error
	if errorsAs(err, &e) {
		return fail(wire.CodeConflict, "%s", e.Message)
	}
	return fail(wire.CodeUnavailable, "Gwen Terminal's core isn't answering")
}
