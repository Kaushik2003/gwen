package remote

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// w27bHookService is a Service with no core whose only terminal is t1 as
// given, so HandleHook has something to check events against.
func w27bHookService(t *testing.T, t1 herdr.Terminal) (*Service, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: t.TempDir()})
	require.NoError(t, err)
	svc.mu.Lock()
	svc.terms[t1.Address] = t1
	svc.mu.Unlock()
	return svc, clk
}

// w27bChild starts a process that waits for its input to close: a real pid
// that is this test's child, gone when the test ends.
func w27bChild(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("cat")
	in, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		in.Close()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

// w27bToolEvent is a PreToolUse event for terminal t1 whose body claims pid.
func w27bToolEvent(pid int) wire.CodeHookEvent {
	return wire.CodeHookEvent{Event: "PreToolUse", Terminal: "t1", ToolName: "Bash", ToolSummary: "go test ./...", PID: pid}
}

// w27bCard is t1's card, nil when no event for it was accepted.
func w27bCard(svc *Service) *wire.CodeSession {
	for _, sess := range svc.Sessions().Sessions {
		if sess.Terminal == "t1" {
			return &sess
		}
	}
	return nil
}

// An event counts for a terminal only if the process that sent it (the
// socket's SO_PEERCRED pid) is that terminal's program or runs under it
// (plan.md 3.14.6). Anything else on this laptop that can reach the hook
// socket must not be able to put words in a session's mouth. The pids here
// are real processes, because the check walks /proc.
func TestHookIsAcceptedOnlyFromInsideItsTerminal(t *testing.T) {
	t.Parallel()
	self := os.Getpid()
	live := func(t *testing.T, pid int) herdr.Terminal {
		return herdr.Terminal{Address: "t1", State: "live", PID: pid, Cwd: t.TempDir()}
	}
	us := func(*testing.T) int { return self }
	cases := []struct {
		name     string
		terminal func(t *testing.T) herdr.Terminal
		sender   func(t *testing.T) int
		event    func(terminalPID int) wire.CodeHookEvent
		accepted bool
	}{
		{
			name:     "the terminal's own program",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, self) },
			sender:   us,
			accepted: true,
		},
		{
			// gwen hook runs under Claude, which runs under the terminal's shell.
			name:     "a descendant of the terminal's program",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, self) },
			sender:   w27bChild,
			accepted: true,
		},
		{
			name:     "the terminal program's parent",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, w27bChild(t)) },
			sender:   us,
		},
		{
			name:     "a process in another terminal",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, w27bChild(t)) },
			sender:   w27bChild,
		},
		{
			// The pid in the body is what gwen hook says; only the socket's
			// word counts.
			name:     "a stranger naming the terminal's pid in the body",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, w27bChild(t)) },
			sender:   us,
			event:    w27bToolEvent,
		},
		{
			name:     "a pid that does not exist",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, self) },
			sender:   func(*testing.T) int { return 1 << 30 },
		},
		{
			name: "a terminal whose program finished",
			terminal: func(t *testing.T) herdr.Terminal {
				t1 := live(t, self)
				t1.State = "finished"
				return t1
			},
			sender: us,
		},
		{
			name:     "a terminal with no program",
			terminal: func(t *testing.T) herdr.Terminal { return live(t, 0) },
			sender:   us,
		},
		{
			// A terminal on another machine has an address with a slash; its
			// pids mean nothing here.
			name: "a terminal on another machine",
			terminal: func(t *testing.T) herdr.Terminal {
				t1 := live(t, self)
				t1.Address = "laptop2/t1"
				return t1
			},
			sender: us,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			term := tc.terminal(t)
			svc, _ := w27bHookService(t, term)
			ev := w27bToolEvent(0)
			if tc.event != nil {
				ev = tc.event(term.PID)
			}
			reply := svc.HandleHook(t.Context(), ev, tc.sender(t))
			require.Empty(t, reply.Output)
			card := w27bCard(svc)
			if !tc.accepted {
				require.Nil(t, card, "the event is dropped and no card appears")
				return
			}
			require.NotNil(t, card, "the event makes a card for the terminal")
			require.Equal(t, "Running go test ./...", card.Step)
		})
	}
}

// An unknown terminal gets no card, whoever sends the event.
func TestHookForAnUnknownTerminalIsDropped(t *testing.T) {
	t.Parallel()
	svc, _ := w27bHookService(t, herdr.Terminal{Address: "t1", State: "live", PID: os.Getpid(), Cwd: t.TempDir()})
	ev := w27bToolEvent(0)
	ev.Terminal = "t2"
	svc.HandleHook(t.Context(), ev, os.Getpid())
	require.Empty(t, svc.Sessions().Sessions)
}

// A terminal's processes may post at most 50 events a second; the rest are
// dropped, and the next second starts afresh (plan.md 3.14.6). Each event
// here names a new edited file, so the card's list of touched files counts
// the events that got through.
func TestHookEventsAreLimitedTo50ASecond(t *testing.T) {
	t.Parallel()
	self := os.Getpid()
	svc, clk := w27bHookService(t, herdr.Terminal{Address: "t1", State: "live", PID: self, Cwd: t.TempDir()})
	edit := func(n int) {
		svc.HandleHook(t.Context(), wire.CodeHookEvent{Event: "PostToolUse", Terminal: "t1", ToolName: "Edit",
			ToolSummary: fmt.Sprintf("/code/file%d.go", n)}, self)
	}
	touched := func() int {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.byTerminal["t1"].Touched)
	}
	for n := range 60 {
		edit(n)
	}
	require.Equal(t, 50, touched())
	clk.Advance(time.Second)
	edit(100)
	require.Equal(t, 51, touched(), "a new second lets events through again")
}
