package remote

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// w27dSuggestions are Claude Code's permission_suggestions for a Bash call.
func w27dSuggestions() []map[string]any {
	return []map[string]any{
		{"type": "addRules", "behavior": "allow", "destination": "session",
			"rules": []any{map[string]any{"toolName": "Bash", "ruleContent": "npm test"}}},
		{"type": "setMode", "mode": "acceptEdits", "destination": "localSettings"},
	}
}

// w27dHeld is a working session whose PermissionRequest hook gwend is
// holding because you are away. The hook's reply arrives on the channel.
func w27dHeld(t *testing.T) (r *w27dRig, id string, reply <-chan wire.CodeHookReply) {
	t.Helper()
	r = w27dReady(t)
	r.away.Store(true)
	addr, id := r.claude(r.dir("proj"))
	r.status(addr, "working")
	ev := wire.CodeHookEvent{Event: "PermissionRequest", ToolName: "Bash", ToolSummary: "npm test",
		ToolInput: map[string]any{"command": "npm test"}, Suggestions: w27dSuggestions()}
	ch := make(chan wire.CodeHookReply, 1)
	go func() { ch <- r.hook(addr, ev) }()
	w27dArmed(t, r.clk, 1) // the 10-minute limit: the request is held
	// Claude Code shows its own dialog on the laptop meanwhile.
	r.status(addr, "blocked")
	return r, id, ch
}

// w27dDecision is the hook output that gives Claude Code a decision.
func w27dDecision(t *testing.T, decision map[string]any) string {
	t.Helper()
	out, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PermissionRequest", "decision": decision}})
	require.NoError(t, err)
	return string(out)
}

// While you are away a PermissionRequest is held open and the phone
// decides; its answer is the hook's output, in the shape Claude Code reads
// (plan.md 3.14.7).
func TestHeldPermissionThePhoneDecides(t *testing.T) {
	t.Parallel()

	t.Run("the card offers the choices", func(t *testing.T) {
		t.Parallel()
		r, id, _ := w27dHeld(t)
		card := r.card(id)
		require.Equal(t, store.CodeNeedsYou, card.State)
		require.Equal(t, store.NeedPermission, card.Need)
		require.NotNil(t, card.Waiting)
		w := *card.Waiting
		require.True(t, w.Held)
		require.Equal(t, wire.WaitingPermission, w.Kind)
		require.Equal(t, "Bash", w.Tool)
		require.Equal(t, "npm test", w.Summary)
		require.NotNil(t, w.HeldUntil)
		require.Equal(t, r.clk.Now().Add(10*time.Minute).UnixMilli(), *w.HeldUntil)
		require.Equal(t, []wire.CodeOption{
			{Key: "allow", Label: "Allow once", Kind: "allow"},
			{Key: "always:0", Label: "Always allow Bash npm test for this session", Kind: "always"},
			{Key: "always:1", Label: "Accept edits without asking in this project", Kind: "always"},
			{Key: "deny", Label: "Deny", Kind: "deny"},
		}, w.Options)
	})

	cases := []struct {
		name     string
		answer   wire.CodeAnswer
		decision map[string]any
		state    string
	}{
		{"allow once", wire.CodeAnswer{Key: "allow"}, map[string]any{"behavior": "allow"}, store.CodeWorking},
		{
			"always allow a suggestion", wire.CodeAnswer{Key: "always:0"},
			map[string]any{"behavior": "allow", "updatedPermissions": []any{w27dSuggestions()[0]}}, store.CodeWorking,
		},
		{
			"deny with your reason", wire.CodeAnswer{Key: "deny", Message: "Run it in CI instead"},
			map[string]any{"behavior": "deny", "message": "Run it in CI instead"}, store.CodeNeedsYou,
		},
		{
			"deny without one", wire.CodeAnswer{Key: "deny", Message: "  "},
			map[string]any{"behavior": "deny", "message": "Denied from your phone"}, store.CodeNeedsYou,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, id, reply := w27dHeld(t)
			res, err := r.svc.Answer(t.Context(), id, tc.answer)
			require.NoError(t, err)
			require.True(t, res.OK)
			got := w27dRecv(t, reply, "the hook's reply")
			require.JSONEq(t, w27dDecision(t, tc.decision), got.Output)

			card := r.card(id)
			require.Nil(t, card.Waiting)
			require.Empty(t, card.Need)
			require.False(t, card.Unread, "answering it is looking at it")
			// Allowing lets Claude go on at once; after a denial herdr-core's
			// next status says what Claude does.
			require.Equal(t, tc.state, card.State)
		})
	}

	t.Run("a choice that isn't offered decides nothing", func(t *testing.T) {
		t.Parallel()
		r, id, reply := w27dHeld(t)
		for _, key := range []string{"always:2", "always:-1", "always:x", "option:1", ""} {
			_, err := r.svc.Answer(t.Context(), id, wire.CodeAnswer{Key: key})
			require.Equal(t, wire.CodeInvalidRequest, w27dCode(err), "key %q: %v", key, err)
		}
		require.True(t, r.card(id).Waiting.Held, "still held")
		select {
		case got := <-reply:
			t.Fatalf("the hook was answered with %q", got.Output)
		default:
		}
		_, err := r.svc.Answer(t.Context(), id, wire.CodeAnswer{Key: "allow"})
		require.NoError(t, err)
		require.JSONEq(t, w27dDecision(t, map[string]any{"behavior": "allow"}), w27dRecv(t, reply, "the hook's reply").Output)
	})
}

// A held request goes back to the laptop with no decision, so Claude Code
// shows its own dialog there, when you come back first, when 10 minutes
// pass, or when you answer the dialog on the laptop (plan.md 3.14.7).
func TestHeldPermissionIsReleasedWithoutADecision(t *testing.T) {
	t.Parallel()
	released := func(t *testing.T, r *w27dRig, id string, reply <-chan wire.CodeHookReply, notice string) {
		t.Helper()
		require.Empty(t, w27dRecv(t, reply, "the hook's reply").Output, "no decision")
		card := r.card(id)
		require.Equal(t, notice, card.Notice)
		if card.Waiting != nil {
			require.False(t, card.Waiting.Held)
			require.Nil(t, card.Waiting.HeldUntil)
		}
		// The phone can no longer decide it; only the laptop's dialog can.
		_, err := r.svc.Answer(t.Context(), id, wire.CodeAnswer{Key: "allow"})
		require.Error(t, err)
	}

	t.Run("you come back to the laptop", func(t *testing.T) {
		t.Parallel()
		r, id, reply := w27dHeld(t)
		// gwend looks every 2 seconds whether you are back; the wait below
		// is bounded only as a guard.
		r.away.Store(false)
		released(t, r, id, reply, "Moved back to the laptop")
		require.Equal(t, store.CodeNeedsYou, r.card(id).State, "the dialog on the laptop still waits")
	})

	t.Run("ten minutes pass", func(t *testing.T) {
		t.Parallel()
		r, id, reply := w27dHeld(t)
		r.clk.Advance(10*time.Minute - time.Second)
		require.True(t, r.card(id).Waiting.Held, "a second before the limit it is still held")
		select {
		case got := <-reply:
			t.Fatalf("released early with %q", got.Output)
		default:
		}
		r.clk.Advance(time.Second)
		released(t, r, id, reply, "Moved back to the laptop")
	})

	t.Run("you answer on the laptop", func(t *testing.T) {
		t.Parallel()
		r, id, reply := w27dHeld(t)
		addr := r.card(id).Terminal
		// Claude Code's own dialog was answered: Claude works on.
		r.status(addr, "working")
		released(t, r, id, reply, "")
		card := r.card(id)
		require.Equal(t, store.CodeWorking, card.State)
		require.Nil(t, card.Waiting)
	})
}

// At the laptop the hook returns at once with no decision, and nothing is
// held: Claude Code's dialog shows on the laptop as usual.
func TestPermissionAtTheLaptopIsNotHeld(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	addr, id := r.claude(r.dir("proj"))
	r.status(addr, "working")
	reply := r.hook(addr, wire.CodeHookEvent{Event: "PermissionRequest", ToolName: "Bash", ToolSummary: "npm test",
		Suggestions: w27dSuggestions()})
	require.Empty(t, reply.Output)
	require.Zero(t, r.clk.Armed(), "no hold is waiting")
	card := r.card(id)
	require.Equal(t, store.NeedPermission, card.Need)
	require.NotNil(t, card.Waiting)
	require.False(t, card.Waiting.Held)
	require.Equal(t, wire.WaitingPermission, card.Waiting.Kind)
}

// A held request ends only for the reasons above. herdr-core's blocked
// status may come a moment after the hook, and any other event for the
// terminal in between (Claude Code retitles it as it works) must not end
// the hold as if the dialog had been answered on the laptop.
func TestHeldPermissionOutlastsOtherEvents(t *testing.T) {
	t.Parallel()
	r := w27dReady(t)
	r.away.Store(true)
	addr, id := r.claude(r.dir("proj"))
	r.status(addr, "working")
	reply := make(chan wire.CodeHookReply, 1)
	go func() {
		reply <- r.hook(addr, wire.CodeHookEvent{Event: "PermissionRequest", ToolName: "Bash", ToolSummary: "npm test"})
	}()
	w27dArmed(t, r.clk, 1)
	r.core.SetTitle(addr, "✳ Running npm test")
	r.sync()
	select {
	case got := <-reply:
		t.Fatalf("a title change released the hold with %q", got.Output)
	default:
	}
	card := r.card(id)
	require.Equal(t, store.CodeNeedsYou, card.State)
	require.True(t, card.Waiting.Held)
}
