package remote

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/term"
	"github.com/stretchr/testify/require"
)

// w27dFakeBin puts stand-ins for the named programs on an otherwise empty
// PATH, so the Service finds these and never a real Claude, bubblewrap or
// gwen-ui. Nothing runs them: Launch is replaced and the core is fake.
func w27dFakeBin(t *testing.T, names ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	}
	t.Setenv("PATH", bin)
	t.Setenv("CLAUDE_CONFIG_DIR", "") // your own status line is looked for in the rig's home only
	return bin
}

// w27dEnded is an ended card of conversation claudeID, as gwend keeps one
// for Resume.
func w27dEnded(id, claudeID string, sandbox bool, args ...string) store.CodeSession {
	ended := testutil.At("08:00:00")
	return store.CodeSession{ID: id, ClaudeSessionID: claudeID, Cwd: "/code/proj", Sandbox: sandbox, Args: args,
		State: store.CodeEnded, StartedAt: testutil.At("07:00:00"), UpdatedAt: ended, EndedAt: &ended}
}

// A session always starts as a terminal in herdr-core: Claude, or `gwen
// sandbox` for a sandboxed one, with the session's own Claude session id
// (or --resume and the flags it last ran with), the status line Gwen reads
// usage from, and the label that ties the terminal to its card. With the
// window on (the default) gwend then shows its tab in the Gwen window with
// gwen-ui --open (plan.md 3.14.5, 3.15).
func TestStartRunsClaudeInTheCore(t *testing.T) {
	bin := w27dFakeBin(t, "claude", "bwrap", "gwen-ui")
	claude := filepath.Join(bin, "claude")
	const gwen = "/opt/gwen/bin/gwen"
	const settings = `{"statusLine":{"command":"/opt/gwen/bin/gwen statusline","type":"command"}}`
	const conv = "6f1c2a3e-8d4b-4c1e-9a7f-2b5d8e0c4a91"
	resumable := []store.CodeSession{w27dEnded("cs_prior00001", conv, true, "--model", "opus")}

	cases := []struct {
		name    string
		setup   w27dSetup
		req     wire.StartCodeSession
		argv    func(claudeID string) []string
		window  bool
		sandbox bool
		notice  string
	}{
		{
			name:  "with a window, the default",
			setup: w27dSetup{gwenBin: gwen},
			argv: func(id string) []string {
				return []string{claude, "--settings", settings, "--session-id", id, "-n", "proj"}
			},
			window: true,
		},
		{
			name:  "without a window, with a first message",
			setup: w27dSetup{gwenBin: gwen},
			req:   wire.StartCodeSession{Window: w27dPtr(false), Prompt: "fix the failing test"},
			argv: func(id string) []string {
				return []string{claude, "--settings", settings, "--session-id", id, "-n", "proj", "fix the failing test"}
			},
		},
		{
			// gwen sandbox signs in from Gwen's credentials, so no secret is
			// in the command line herdr-core keeps.
			name:  "sandboxed",
			setup: w27dSetup{gwenBin: gwen},
			req:   wire.StartCodeSession{Sandbox: w27dPtr(true), Window: w27dPtr(false)},
			argv: func(id string) []string {
				return []string{gwen, "sandbox", "--settings", settings, "--session-id", id, "-n", "proj"}
			},
			sandbox: true,
		},
		{
			name:   "no status line without the gwen CLI",
			req:    wire.StartCodeSession{Window: w27dPtr(false)},
			argv:   func(id string) []string { return []string{claude, "--session-id", id, "-n", "proj"} },
			window: false,
		},
		{
			name:  "resumed, in the mode it last ran in",
			setup: w27dSetup{gwenBin: gwen, seed: resumable},
			req:   wire.StartCodeSession{Resume: conv, Window: w27dPtr(false)},
			argv: func(string) []string {
				return []string{gwen, "sandbox", "--model", "opus", "--settings", settings, "--resume", conv, "-n", "proj"}
			},
			sandbox: true,
		},
		{
			name:  "resumed, switched to Normal",
			setup: w27dSetup{gwenBin: gwen, seed: resumable},
			req:   wire.StartCodeSession{Resume: conv, Sandbox: w27dPtr(false)},
			argv: func(string) []string {
				return []string{claude, "--model", "opus", "--settings", settings, "--resume", conv, "-n", "proj"}
			},
			window: true,
		},
		{
			// The session runs anyway; the card says the window didn't open.
			name:  "the window can't open",
			setup: w27dSetup{gwenBin: gwen, launchErr: errors.New("systemd-run: no session bus")},
			argv: func(id string) []string {
				return []string{claude, "--settings", settings, "--session-id", id, "-n", "proj"}
			},
			window: true,
			notice: "Couldn't open the Gwen window",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := w27dNewRig(t, tc.setup)
			r.waitCore("ok")
			dir := r.dir("proj")
			req := tc.req
			req.Dir = dir
			card, err := r.svc.Start(t.Context(), req)
			require.NoError(t, err)

			runs := r.runs()
			require.Len(t, runs, 1)
			run := runs[0]
			if req.Resume == "" {
				_, err := uuid.Parse(card.ClaudeSessionID)
				require.NoError(t, err, "a new session gets its own Claude session id")
			} else {
				require.Equal(t, req.Resume, card.ClaudeSessionID)
			}
			require.Equal(t, tc.argv(card.ClaudeSessionID), run.Argv)
			require.Equal(t, dir, run.Cwd)
			require.Equal(t, "proj", run.Name)
			require.Equal(t, map[string]string{SessionLabel: card.ID}, run.Labels)
			require.True(t, run.Keep, "the shell stays when Claude exits")
			require.True(t, run.NoParent)

			terms := r.core.Terminals()
			require.Len(t, terms, 1)
			require.Equal(t, terms[0].Address, card.Terminal)
			require.Equal(t, store.CodeStarting, card.State)
			require.Equal(t, tc.sandbox, card.Sandbox)
			require.Equal(t, tc.notice, card.Notice)
			require.False(t, card.Unread, "you started it, so you have seen it")

			want := []w27dLaunch{}
			if tc.window {
				want = append(want, w27dLaunch{Argv: []string{filepath.Join(bin, "gwen-ui"), "--open", "terminal:" + card.Terminal}})
			}
			require.Equal(t, want, r.launched())
		})
	}
}

// What Start refuses, it refuses before anything runs.
func TestStartRefuses(t *testing.T) {
	w27dFakeBin(t, "claude", "bwrap", "gwen-ui")
	cases := []struct {
		name string
		req  func(r *w27dRig) wire.StartCodeSession
		code string
		msg  string
	}{
		{
			name: "a conversation id that isn't one",
			req: func(r *w27dRig) wire.StartCodeSession {
				return wire.StartCodeSession{Dir: r.dir("proj"), Resume: "../../etc/passwd"}
			},
			code: wire.CodeInvalidRequest, msg: "that isn't a conversation",
		},
		{
			name: "a folder outside the roots",
			req:  func(*w27dRig) wire.StartCodeSession { return wire.StartCodeSession{Dir: "/etc"} },
			code: wire.CodeInvalidRequest,
		},
		{
			name: "a hidden folder",
			req: func(r *w27dRig) wire.StartCodeSession {
				return wire.StartCodeSession{Dir: r.dir(".secret")}
			},
			code: wire.CodeInvalidRequest,
		},
		{
			name: "a sandbox without bubblewrap",
			req: func(r *w27dRig) wire.StartCodeSession {
				w27dFakeBin(r.t, "claude", "gwen-ui")
				return wire.StartCodeSession{Dir: r.dir("proj"), Sandbox: w27dPtr(true)}
			},
			code: wire.CodeUnavailable, msg: "the sandbox needs bubblewrap, which isn't installed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := w27dNewRig(t, w27dSetup{gwenBin: "/opt/gwen/bin/gwen"})
			r.waitCore("ok")
			_, err := r.svc.Start(t.Context(), tc.req(r))
			require.Error(t, err)
			require.Equal(t, tc.code, w27dCode(err), "%v", err)
			if tc.msg != "" {
				require.EqualError(t, err, tc.msg)
			}
			require.Empty(t, r.runs())
			require.Empty(t, r.launched())
			require.Empty(t, r.svc.Sessions().Sessions)
		})
	}
}

// w27dOlder is a version just older than v: its last number that is not 0,
// one less.
func w27dOlder(t *testing.T, v string) string {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	require.Len(t, parts, 3)
	for i := 2; i >= 0; i-- {
		n, err := strconv.Atoi(parts[i])
		require.NoError(t, err)
		if n > 0 {
			parts[i] = strconv.Itoa(n - 1)
			older := "v" + strings.Join(parts, ".")
			require.False(t, herdr.VersionAtLeast(older, v))
			return older
		}
	}
	t.Fatalf("no version is older than %s", v)
	return ""
}

// A core older than term/core.version, or of another protocol generation,
// is not used at all: the Service says it is out of date, subscribes to
// nothing, and starts no session in it (plan.md 3.14.5).
func TestOutdatedCoreStartsNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		core func(t *testing.T) herdrtest.Options
	}{
		{"an older version", func(t *testing.T) herdrtest.Options {
			return herdrtest.Options{Version: w27dOlder(t, term.CoreVersion())}
		}},
		{"another generation", func(*testing.T) herdrtest.Options { return herdrtest.Options{Generation: herdr.Generation + 1} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := w27dNewRig(t, w27dSetup{core: tc.core(t)})
			r.waitCore("outdated")
			const note = "Gwen Terminal's core is out of date. Run make term."
			require.Equal(t, note, r.svc.Sessions().Message)

			_, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: r.dir("proj")})
			require.Equal(t, wire.CodeUnavailable, w27dCode(err), "%v", err)
			require.EqualError(t, err, note)
			require.Empty(t, r.core.Calls("events.subscribe"))
			require.Empty(t, r.core.Calls("terminal.run"))
			require.Empty(t, r.launched(), "neither a core nor a window is started")
			require.Empty(t, r.svc.Sessions().Sessions)
		})
	}
}

// Resume starts a new session of the same conversation. While it runs, the
// ended card it came from is not listed, and once the new session reports
// the conversation's id that card is dropped for good, so one conversation
// keeps one card even after the new one ends too (plan.md 3.14.5). A pin
// moves to the new card.
func TestOneConversationKeepsOneCard(t *testing.T) {
	t.Parallel()
	const conv = "0d6c8f5a-3b2e-4f71-a9c4-7e1b5d2f8a60"
	old := w27dEnded("cs_old0000001", conv, false, "--model", "opus")
	old.Pinned = true
	r := w27dNewRig(t, w27dSetup{seed: []store.CodeSession{old}})
	r.waitCore("ok")
	dir := r.dir("proj")
	require.Len(t, r.cardsOf(conv), 1)

	card, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: dir, Resume: conv, Window: w27dPtr(false)})
	require.NoError(t, err)
	require.NotEqual(t, old.ID, card.ID)
	cards := r.cardsOf(conv)
	require.Len(t, cards, 1, "the ended card is hidden while its conversation runs")
	require.Equal(t, card.ID, cards[0].ID)

	// Claude comes up and reports the conversation it runs.
	r.core.SetForeground(card.Terminal, &herdr.Foreground{PID: w27dNoClaude, Name: "claude", Argv: []string{"claude"}})
	r.sync()
	r.hook(card.Terminal, wire.CodeHookEvent{Event: "SessionStart", SessionID: conv, Cwd: dir, Source: "resume"})
	require.Contains(t, r.events.removedIDs(), old.ID, "the old card is dropped")
	_, err = r.svc.Session(old.ID)
	require.Equal(t, wire.CodeNotFound, w27dCode(err))
	require.True(t, r.card(card.ID).Pinned, "the pin moves to the new card")

	// It ends: still one card.
	r.core.Finish(card.Terminal, herdr.CloseReason{Reason: "exited"})
	r.sync()
	cards = r.cardsOf(conv)
	require.Len(t, cards, 1)
	require.Equal(t, card.ID, cards[0].ID)
	require.Equal(t, store.CodeEnded, cards[0].State)

	// And it can be resumed again, now that nothing runs it.
	again, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: dir, Resume: conv, Window: w27dPtr(false)})
	require.NoError(t, err)
	require.Equal(t, []string{"--model", "opus"}, r.runs()[1].Argv[1:3], "with the flags it first ran with")
	cards = r.cardsOf(conv)
	require.Len(t, cards, 1)
	require.Equal(t, again.ID, cards[0].ID)
}

// Two Claudes on one conversation would write over each other's
// transcript, so Resume of a conversation that runs anywhere is refused
// with reason running and that session's id, which the phone opens instead
// (plan.md 3.14.5).
func TestResumingARunningConversationIsRefused(t *testing.T) {
	t.Parallel()
	const conv = "9a3e7c1b-5f2d-4b8a-8c6e-1d4f7a2b9e03"
	refused := func(t *testing.T, r *w27dRig, dir, running string) {
		t.Helper()
		before := len(r.runs())
		_, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: dir, Resume: conv})
		var e *Error
		require.ErrorAs(t, err, &e)
		require.Equal(t, wire.CodeConflict, e.Code)
		require.Equal(t, map[string]any{"reason": "running", "session_id": running}, e.Details)
		require.Len(t, r.runs(), before, "nothing is started")
		require.Empty(t, r.launched(), "and no window opens")
	}

	t.Run("one Gwen resumed a moment ago", func(t *testing.T) {
		t.Parallel()
		r := w27dNewRig(t, w27dSetup{seed: []store.CodeSession{w27dEnded("cs_old0000002", conv, false)}})
		r.waitCore("ok")
		dir := r.dir("proj")
		card, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: dir, Resume: conv, Window: w27dPtr(false)})
		require.NoError(t, err)
		// Still starting: Claude has not even reported in.
		require.Equal(t, store.CodeStarting, r.card(card.ID).State)
		refused(t, r, dir, card.ID)
	})

	t.Run("one you started yourself in a terminal", func(t *testing.T) {
		t.Parallel()
		r := w27dReady(t)
		dir := r.dir("proj")
		addr, id := r.claude(dir)
		r.hook(addr, wire.CodeHookEvent{Event: "SessionStart", SessionID: conv, Cwd: dir})
		refused(t, r, dir, id)

		// /clear brings a new Claude session id in the same terminal: the
		// card follows it, and the old conversation may be resumed.
		const next = "2c8b4e6f-1a3d-4e5b-9f7c-6d0a8b2e4c15"
		r.hook(addr, wire.CodeHookEvent{Event: "SessionStart", SessionID: next, Cwd: dir, Source: "clear"})
		card := r.card(id)
		require.Equal(t, next, card.ClaudeSessionID)
		require.Equal(t, addr, card.Terminal)
		_, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: dir, Resume: conv, Window: w27dPtr(false)})
		require.NoError(t, err)
	})
}

// Start labels the terminal and then brings the card in line with it. When
// the reply to terminal.run reaches Start before the event loop has applied
// the terminal's created event, the card must still stay with its new
// terminal. The event loop is held here on a document read, so the reply
// comes first every time.
func TestStartKeepsItsCardWhenTheReplyComesFirst(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	r.eventFirst.Store(false)
	held := make(chan struct{})
	r.core.Handle("doc.get", func(call herdrtest.Call) (any, error) {
		<-held
		return call.Builtin()
	})
	n := len(r.core.Calls("doc.get"))
	r.core.SetDoc("herdr-app", map[string]any{"spaces": []any{}})
	r.core.WaitCalls("doc.get", n+1) // the event loop now waits on this read

	card, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: r.dir("proj"), Window: w27dPtr(false)})
	close(held)
	require.NoError(t, err)
	require.Equal(t, store.CodeStarting, card.State)
	require.NotEmpty(t, card.Terminal)
	r.sync()
	r.core.SetForeground(card.Terminal, &herdr.Foreground{PID: w27dNoClaude, Name: "claude", Argv: []string{"claude"}})
	r.status(card.Terminal, "idle")
	require.Equal(t, store.CodeIdle, r.card(card.ID).State)
	require.Len(t, r.svc.Sessions().Sessions, 1, "Claude in its terminal is this card, not a new one")
}
