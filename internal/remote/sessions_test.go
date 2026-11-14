package remote

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// A terminal is a code session when Claude Code reports its status through
// herdr-core (OSC 7501, app claude-code) or its foreground program is
// claude (plan.md 3.14.5). Anything else stays a plain terminal, and a card
// is never made for a terminal on another machine or one that finished.
func TestWhichTerminalsAreCodeSessions(t *testing.T) {
	t.Parallel()
	const shellPID = 7001
	cases := []struct {
		name  string
		term  herdr.Terminal
		card  bool
		state string
	}{
		{
			// Claude Code run through node: only its status says what it is.
			name: "Claude Code's status (OSC 7501 app)",
			term: herdr.Terminal{Foreground: &herdr.Foreground{PID: 7002, Name: "node",
				Argv: []string{"node", "/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js"}},
				Status: &herdr.Status{State: "working", App: "claude-code"}, Reported: true},
			card: true, state: store.CodeWorking,
		},
		{
			name: "a status record from Claude Code",
			term: herdr.Terminal{Records: map[string]herdr.Status{"claude": {State: "idle", App: "claude-code"}}, Reported: true},
			card: true, state: store.CodeIdle,
		},
		{
			// Claude has started but not said anything yet.
			name: "the foreground program's argv[0] is claude",
			term: herdr.Terminal{Foreground: &herdr.Foreground{PID: w27dNoClaude, Argv: []string{"/home/u/.local/bin/claude", "--resume"}}},
			card: true, state: store.CodeIdle,
		},
		{
			name: "the foreground process is named claude",
			term: herdr.Terminal{Foreground: &herdr.Foreground{PID: w27dNoClaude, Name: "claude", Argv: []string{"/home/u/.local/share/claude/versions/2.1.322"}}},
			card: true, state: store.CodeIdle,
		},
		{
			name: "a shell",
			term: herdr.Terminal{Foreground: &herdr.Foreground{PID: shellPID, Name: "bash", Argv: []string{"-bash"}}},
		},
		{
			name: "an editor",
			term: herdr.Terminal{Foreground: &herdr.Foreground{PID: 7003, Name: "nvim", Argv: []string{"nvim", "main.go"}}},
		},
		{
			name: "another agent's status",
			term: herdr.Terminal{Status: &herdr.Status{State: "working", App: "codex"}, Reported: true},
		},
		{
			// The shell is in front again: the status is all that is left of
			// a Claude that died.
			name: "a status left behind by a Claude that died",
			term: herdr.Terminal{Foreground: &herdr.Foreground{PID: shellPID, Name: "bash", Argv: []string{"-bash"}},
				Status: &herdr.Status{State: "working", App: "claude-code"}, Reported: true},
		},
		{
			name: "a terminal that finished",
			term: herdr.Terminal{State: "finished", Status: &herdr.Status{State: "idle", App: "claude-code"}, Reported: true},
		},
		{
			name: "Claude on another machine",
			term: herdr.Terminal{Address: "pi/t1", Foreground: &herdr.Foreground{PID: w27dNoClaude, Name: "claude", Argv: []string{"claude"}},
				Status: &herdr.Status{State: "working", App: "claude-code"}, Reported: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := w27dReady(t)
			term := tc.term
			term.Cwd = r.dir("proj")
			if term.State != "finished" {
				term.PID = shellPID
			}
			addr := r.core.AddTerminal(term).Address
			r.sync()
			card := r.cardAt(addr)
			if !tc.card {
				require.Nil(t, card, "no card for this terminal")
				require.Empty(t, r.core.Calls("terminal.label"), "and no label on it")
				return
			}
			require.NotNil(t, card)
			require.Equal(t, tc.state, card.State)
			require.Equal(t, term.Cwd, card.Cwd)
			// The label keeps the card with the terminal across gwend restarts.
			var label struct {
				Terminal string            `json:"terminal"`
				Labels   map[string]string `json:"labels"`
			}
			require.NoError(t, r.core.WaitCalls("terminal.label", 1)[0].Decode(&label))
			require.Equal(t, addr, label.Terminal)
			require.Equal(t, map[string]string{SessionLabel: card.ID}, label.Labels)
		})
	}
}

// w27dAction is one thing that happens to a session: something herdr-core
// reports, a hook from inside the terminal, or someone looking at it.
type w27dAction struct {
	what string
	do   func(r *w27dRig, addr, id string)
}

func w27dStatus(state string) w27dAction {
	return w27dAction{"status " + state, func(r *w27dRig, addr, _ string) { r.status(addr, state) }}
}

func w27dHook(ev wire.CodeHookEvent) w27dAction {
	return w27dAction{"hook " + ev.Event + " " + ev.ToolName, func(r *w27dRig, addr, _ string) { r.hook(addr, ev) }}
}

func w27dCoreDoes(what string, do func(r *w27dRig, addr string)) w27dAction {
	return w27dAction{what, func(r *w27dRig, addr, _ string) {
		do(r, addr)
		r.sync()
	}}
}

var (
	w27dStatusCleared = w27dCoreDoes("status cleared", func(r *w27dRig, addr string) { r.core.SetStatus(addr, nil) })
	w27dShellBack     = w27dCoreDoes("its shell in front again", func(r *w27dRig, addr string) {
		t, _ := r.core.Terminal(addr)
		r.core.SetForeground(addr, &herdr.Foreground{PID: t.PID, Name: "bash", Argv: []string{"-bash"}})
	})
	w27dRetitled = w27dCoreDoes("a new title", func(r *w27dRig, addr string) { r.core.SetTitle(addr, "✳ Tests") })
	w27dFinished = w27dCoreDoes("the terminal finishes", func(r *w27dRig, addr string) {
		r.core.Finish(addr, herdr.CloseReason{Reason: "exited"})
	})
	w27dRemoved = w27dCoreDoes("the terminal is removed", func(r *w27dRig, addr string) { r.core.Remove(addr) })
)

// w27dStep is an action and what the card shows after it: its state and,
// when set, why it needs you.
type w27dStep struct {
	act   w27dAction
	state string
	need  string
}

// The card's state follows from what herdr-core and the hooks report, in
// order (plan.md 3.14.5). Each sequence is played against the fake core,
// as it would arrive from a real session, and the card is read after every
// step.
func TestCodeSessionStateTable(t *testing.T) {
	t.Parallel()
	permission := wire.CodeHookEvent{Event: "PermissionRequest", ToolName: "Bash", ToolSummary: "rm -rf build"}
	question := wire.CodeHookEvent{Event: "PreToolUse", ToolName: "AskUserQuestion",
		ToolInput: map[string]any{"questions": []any{map[string]any{"question": "Which database?",
			"options": []any{map[string]any{"label": "Postgres"}, map[string]any{"label": "SQLite"}}}}}}
	cases := []struct {
		name  string
		steps []w27dStep
	}{
		{"a Claude that has just started is idle", []w27dStep{
			{w27dStatus("idle"), store.CodeIdle, ""},
		}},
		{"a turn nobody looked at is done", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dStatus("idle"), store.CodeDone, ""},
		}},
		{
			// Only looking at it makes it idle; another event or the same
			// idle status again does not.
			"done stays done until someone looks", []w27dStep{
				{w27dStatus("working"), store.CodeWorking, ""},
				{w27dStatus("idle"), store.CodeDone, ""},
				{w27dRetitled, store.CodeDone, ""},
				{w27dStatus("idle"), store.CodeDone, ""},
			},
		},
		{"status done is done", []w27dStep{
			{w27dStatus("done"), store.CodeDone, ""},
		}},
		{"blocked needs you, then it works on", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dStatus("blocked"), store.CodeNeedsYou, store.NeedOther},
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dStatus("idle"), store.CodeDone, ""},
		}},
		{
			// Answering a question of Claude's own, with no work since it was
			// last idle, finishes nothing.
			"a question of Claude's own, once answered, is just idle", []w27dStep{
				{w27dStatus("blocked"), store.CodeNeedsYou, store.NeedOther},
				{w27dStatus("idle"), store.CodeIdle, ""},
			},
		},
		{
			// At the laptop the hook returns at once; the state comes from
			// herdr-core's status, and the hook says why.
			"a permission shown at the laptop", []w27dStep{
				{w27dStatus("working"), store.CodeWorking, ""},
				{w27dHook(permission), store.CodeWorking, ""},
				{w27dStatus("blocked"), store.CodeNeedsYou, store.NeedPermission},
				{w27dStatus("working"), store.CodeWorking, ""},
			},
		},
		{"a question from AskUserQuestion", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dHook(question), store.CodeWorking, ""},
			{w27dStatus("blocked"), store.CodeNeedsYou, store.NeedQuestion},
		}},
		{"an error, then work again", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dStatus("error"), store.CodeError, ""},
			{w27dStatus("working"), store.CodeWorking, ""},
		}},
		{"tool hooks follow along without changing the state", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dHook(wire.CodeHookEvent{Event: "PreToolUse", ToolName: "Bash", ToolSummary: "go test ./..."}), store.CodeWorking, ""},
			{w27dHook(wire.CodeHookEvent{Event: "PostToolUse", ToolName: "Edit", ToolSummary: "/code/proj/main.go"}), store.CodeWorking, ""},
			{w27dHook(wire.CodeHookEvent{Event: "Stop", LastMessage: "All tests pass."}), store.CodeWorking, ""},
			{w27dStatus("idle"), store.CodeDone, ""},
		}},
		{
			// /exit: the status clears first, and the card ends once the
			// shell is in front again.
			"Claude exits to its shell", []w27dStep{
				{w27dStatus("working"), store.CodeWorking, ""},
				{w27dStatusCleared, store.CodeWorking, ""},
				{w27dShellBack, store.CodeEnded, ""},
			},
		},
		{"Claude dies and leaves its status behind", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dShellBack, store.CodeEnded, ""},
		}},
		{"the terminal finishes", []w27dStep{
			{w27dStatus("idle"), store.CodeIdle, ""},
			{w27dFinished, store.CodeEnded, ""},
		}},
		{"the terminal is removed", []w27dStep{
			{w27dStatus("working"), store.CodeWorking, ""},
			{w27dRemoved, store.CodeEnded, ""},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := w27dReady(t)
			addr, id := r.claude(r.dir("proj"))
			for i, step := range tc.steps {
				step.act.do(r, addr, id)
				card := r.card(id)
				require.Equal(t, step.state, card.State, "after step %d (%s)", i+1, step.act.what)
				if step.need != "" {
					require.Equal(t, step.need, card.Need, "after step %d (%s)", i+1, step.act.what)
				}
			}
		})
	}
}

// A finished turn is done, and unread, until you open it: on the phone
// (mark seen), in Gwen Terminal (which records the turn it showed in the
// label herdr.seen), or by sending it the next message (plan.md 3.14.5).
// Then it is idle, and stays idle.
func TestDoneBecomesIdleWhenOpened(t *testing.T) {
	t.Parallel()
	finished := func(t *testing.T) (*w27dRig, string, string) {
		r := w27dReady(t)
		addr, id := r.claude(r.dir("proj"))
		r.status(addr, "working")
		r.status(addr, "idle")
		card := r.card(id)
		require.Equal(t, store.CodeDone, card.State)
		require.True(t, card.Unread, "nobody has looked at it yet")
		return r, addr, id
	}

	t.Run("opened on the phone", func(t *testing.T) {
		t.Parallel()
		r, addr, id := finished(t)
		card, err := r.svc.Mark(t.Context(), id, wire.CodeMark{Seen: w27dPtr(true)})
		require.NoError(t, err)
		require.Equal(t, store.CodeIdle, card.State)
		require.False(t, card.Unread)
		// Another event re-reads the terminal, whose status is still idle.
		r.core.SetTitle(addr, "✳ Done")
		r.sync()
		require.Equal(t, store.CodeIdle, r.card(id).State, "it does not fall back to done")
	})

	t.Run("opened in Gwen Terminal", func(t *testing.T) {
		t.Parallel()
		r := w27dReady(t)
		addr, id := r.claude(r.dir("proj"))
		r.core.SetTurn(addr, 3)
		r.status(addr, "working")
		r.status(addr, "idle")
		require.Equal(t, store.CodeDone, r.card(id).State)
		r.core.SetLabels(addr, map[string]string{"herdr.seen": "2"})
		r.sync()
		require.Equal(t, store.CodeDone, r.card(id).State, "seeing an older turn is not seeing this one")
		r.core.SetLabels(addr, map[string]string{"herdr.seen": "3"})
		r.sync()
		require.Equal(t, store.CodeIdle, r.card(id).State)
	})

	t.Run("sent a message", func(t *testing.T) {
		t.Parallel()
		r, _, id := finished(t)
		res, err := r.svc.Prompt(t.Context(), id, wire.CodePrompt{Text: "now commit it", Idem: "d1"})
		require.NoError(t, err)
		require.Equal(t, wire.PromptSent, res.Result)
		card := r.card(id)
		require.Equal(t, store.CodeIdle, card.State)
		require.False(t, card.Unread)
	})
}

// A session Gwen starts is "starting" until Claude says something: its
// SessionStart hook or its first status makes it idle. One that stays
// silent for 4 seconds is most likely asking whether to trust the folder,
// which only the terminal can answer, so the card says it needs you rather
// than sitting at Starting (plan.md 3.14.5).
func TestStartedSessionIsStartingUntilClaudeSpeaks(t *testing.T) {
	t.Parallel()
	start := func(t *testing.T) (*w27dRig, wire.CodeSession) {
		r := w27dReady(t)
		card, err := r.svc.Start(t.Context(), wire.StartCodeSession{Dir: r.dir("proj"), Window: w27dPtr(false)})
		require.NoError(t, err)
		require.Equal(t, store.CodeStarting, card.State)
		// Claude comes up in the terminal before it reports anything.
		r.core.SetForeground(card.Terminal, &herdr.Foreground{PID: w27dNoClaude, Name: "claude", Argv: []string{"claude"}})
		r.sync()
		require.Equal(t, store.CodeStarting, r.card(card.ID).State)
		return r, card
	}

	t.Run("SessionStart", func(t *testing.T) {
		t.Parallel()
		r, card := start(t)
		r.hook(card.Terminal, wire.CodeHookEvent{Event: "SessionStart", SessionID: card.ClaudeSessionID, Cwd: card.Cwd})
		require.Equal(t, store.CodeIdle, r.card(card.ID).State)
	})

	t.Run("its first status", func(t *testing.T) {
		t.Parallel()
		r, card := start(t)
		r.status(card.Terminal, "idle")
		require.Equal(t, store.CodeIdle, r.card(card.ID).State)
	})

	t.Run("silent for 4 seconds", func(t *testing.T) {
		t.Parallel()
		r, card := start(t)
		w27dArmed(t, r.clk, 1) // the 4-second check
		r.clk.Advance(startGrace - time.Millisecond)
		require.Equal(t, store.CodeStarting, r.card(card.ID).State, "not yet")
		r.clk.Advance(time.Millisecond)
		r.events.wait(t, "the card to need you", func() bool { return r.card(card.ID).State == store.CodeNeedsYou })
		got := r.card(card.ID)
		require.Equal(t, store.NeedOther, got.Need)
		require.NotNil(t, got.Waiting)
		require.Equal(t, "Claude is asking something before it starts.", got.Waiting.Question)
		require.Equal(t, "Answer in the Terminal tab or on the laptop", got.Waiting.Note)
		require.Empty(t, got.Waiting.Options)
		// Once it is answered on the laptop, Claude reports in as usual.
		r.status(card.Terminal, "idle")
		require.Equal(t, store.CodeIdle, r.card(card.ID).State)
	})
}
