package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/packaging/icons/tray"
	"github.com/stretchr/testify/require"
)

var (
	now    = testutil.At("12:12")
	pID    = "p-internship"
	qID    = "q-study"
	live   = []wire.Project{{ID: pID, Name: "Internship"}, {ID: qID, Name: "Study"}}
	opened = wire.Millis(testutil.At("09:00"))
)

// status has the open segment since 09:00 and 5h 47m worked of an 8h target
// today, so the day's total and the segment's age differ.
func status(state string, project *string) *wire.Status {
	return &wire.Status{State: state, ProjectID: project, ServerNowAt: wire.Millis(now),
		OpenSegment: &wire.Segment{StartedAt: opened},
		Today:       &wire.DaySummary{WorkedMs: (5*time.Hour + 47*time.Minute).Milliseconds(), TargetSeconds: 8 * 3600}}
}

func lines(m Menu, l ...string) Menu {
	m.Lines, m.Tooltip = l, strings.Join(l, "\n")
	return m
}

func TestMenuFor(t *testing.T) {
	t.Parallel()
	projects := []MenuProject{{Name: "Unassigned"}, {ID: &pID, Name: "Internship", Checked: true}, {ID: &qID, Name: "Study"}}
	tests := []struct {
		name     string
		status   *wire.Status
		daemonUp bool
		want     Menu
	}{
		{
			name: "daemon down", status: nil, daemonUp: false,
			want: lines(Menu{Icon: tray.Off, StartGwen: true}, "Gwen isn't running"),
		},
		{
			name: "off", status: &wire.Status{State: wire.StateOff}, daemonUp: true,
			want: lines(Menu{Icon: tray.Off, ClockIn: true}, "Not clocked in"),
		},
		{
			name: "working counts the day, not the segment", status: status(wire.StateWorking, &pID), daemonUp: true,
			want: lines(Menu{Icon: tray.Working, StartBreak: true, SwitchProject: true, Snooze: true, ClockOut: true, Projects: projects},
				"Working · 5h 47m today", "Internship", "72% of 8h · 2h 13m left"),
		},
		{
			name: "working goes on from the day's total after a break", status: func() *wire.Status {
				s := status(wire.StateWorking, &pID)
				s.OpenSegment.StartedAt = wire.Millis(testutil.At("12:12"))
				s.Today.WorkedMs = (15 * time.Minute).Milliseconds()
				return s
			}(), daemonUp: true,
			want: lines(Menu{Icon: tray.Working, StartBreak: true, SwitchProject: true, Snooze: true, ClockOut: true, Projects: projects},
				"Working · 15m today", "Internship", "3% of 8h · 7h 45m left"),
		},
		{
			name: "idle pending", status: status(wire.StateIdlePending, &pID), daemonUp: true,
			want: lines(Menu{Icon: tray.Idle, StartBreak: true, SwitchProject: true, Snooze: true, ClockOut: true, Projects: projects},
				"Idle · 5h 47m today", "Internship", "72% of 8h · 2h 13m left"),
		},
		{
			name: "target met", status: func() *wire.Status {
				s := status(wire.StateWorking, &pID)
				s.Today.WorkedMs = (8*time.Hour + 30*time.Minute).Milliseconds()
				return s
			}(), daemonUp: true,
			want: lines(Menu{Icon: tray.Working, StartBreak: true, SwitchProject: true, Snooze: true, ClockOut: true, Projects: projects},
				"Working · 8h 30m today", "Internship", "Target of 8h met"),
		},
		{
			name: "no target", status: func() *wire.Status {
				s := status(wire.StateWorking, &pID)
				s.Today.TargetSeconds = 0
				return s
			}(), daemonUp: true,
			want: lines(Menu{Icon: tray.Working, StartBreak: true, SwitchProject: true, Snooze: true, ClockOut: true, Projects: projects},
				"Working · 5h 47m today", "Internship"),
		},
		{
			name: "automatic break", status: func() *wire.Status {
				s := status(wire.StateBreakAuto, &pID)
				s.OpenSegment.StartedAt = wire.Millis(testutil.At("12:00"))
				return s
			}(), daemonUp: true,
			want: lines(Menu{Icon: tray.Break, StartBreak: true, EndBreak: true, SwitchProject: true, Snooze: true, ClockOut: true, Projects: projects},
				"On break · 12m", "Worked 5h 47m today", "72% of 8h · 2h 13m left"),
		},
		{
			name: "manual break, unassigned", status: func() *wire.Status {
				s := status(wire.StateBreakManual, nil)
				s.OpenSegment.StartedAt = wire.Millis(testutil.At("12:00"))
				return s
			}(), daemonUp: true,
			want: lines(Menu{Icon: tray.Break, EndBreak: true, SwitchProject: true, Snooze: true, ClockOut: true,
				Projects: []MenuProject{{Name: "Unassigned", Checked: true}, {ID: &pID, Name: "Internship"}, {ID: &qID, Name: "Study"}}},
				"On break · 12m", "Worked 5h 47m today", "72% of 8h · 2h 13m left"),
		},
		{
			name: "daemon down after a status", status: status(wire.StateWorking, &pID), daemonUp: false,
			want: lines(Menu{Icon: tray.Off, StartGwen: true}, "Gwen isn't running"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, menuFor(tc.status, tc.daemonUp, live, now))
		})
	}
}

func TestMenuForListsAtMostTenProjects(t *testing.T) {
	t.Parallel()
	var many []wire.Project
	for i := range 15 {
		many = append(many, wire.Project{ID: fmt.Sprintf("p%02d", i), Name: fmt.Sprintf("P%02d", i)})
	}
	m := menuFor(status(wire.StateWorking, nil), true, many, now)
	require.Len(t, m.Projects, 11, "Unassigned plus ten")
	require.Equal(t, "P09", m.Projects[10].Name)
}

// fakeView records every menu shown.
type fakeView struct {
	mu    sync.Mutex
	menus []Menu
}

func (v *fakeView) show(m Menu) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.menus = append(v.menus, m)
}

func (v *fakeView) last() Menu {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.menus) == 0 {
		return Menu{}
	}
	return v.menus[len(v.menus)-1]
}

type fakeRunner struct {
	mu   sync.Mutex
	runs [][]string
}

func (r *fakeRunner) start(name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, append([]string{name}, args...))
	return nil
}

func TestControllerFollowsTheDaemon(t *testing.T) {
	t.Parallel()
	stream := clienttest.NewStream()
	f := clienttest.New().
		Returns("Events", stream, nil).
		Returns("ListProjects", &wire.ProjectList{Projects: live}, nil)
	clk := clock.NewFake(now.Add(2 * time.Second)) // the local clock runs 2 s ahead
	v := &fakeView{}
	c := &controller{api: f, clk: clk, view: v, exec: &fakeRunner{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.run(ctx)

	headline := func() string {
		if l := v.last().Lines; len(l) > 0 {
			return l[0]
		}
		return ""
	}
	require.Eventually(t, func() bool { return headline() == "Gwen isn't running" }, 5*time.Second, time.Millisecond)
	data, err := json.Marshal(status(wire.StateWorking, &pID))
	require.NoError(t, err)
	stream.Send(wire.Event{ID: 1, Name: wire.EventStateChanged, Data: data})
	require.Eventually(t, func() bool {
		m := v.last()
		return m.Icon == tray.Working && m.Tooltip == "Working · 5h 47m today\nInternship\n72% of 8h · 2h 13m left"
	}, 5*time.Second, time.Millisecond, "skew-corrected, with the project's name")

	// The timer re-renders every second and goes on from the day's total.
	require.Equal(t, time.Second, renderInterval)
	clk.BlockUntil(1)
	clk.Advance(59 * time.Second)
	require.Eventually(t, func() bool { return headline() == "Working · 5h 47m today" }, 5*time.Second, time.Millisecond)
	clk.BlockUntil(1)
	clk.Advance(renderInterval)
	require.Eventually(t, func() bool { return headline() == "Working · 5h 48m today" }, 5*time.Second, time.Millisecond)

	stream.Send(wire.Event{ID: 2, Name: wire.EventProjectsChanged, Data: json.RawMessage(`{}`)})
	require.Eventually(t, func() bool { return len(f.CallsTo("ListProjects")) >= 2 }, 5*time.Second, time.Millisecond)
}

func TestControllerCommands(t *testing.T) {
	t.Parallel()
	off := &wire.Status{State: wire.StateOff}
	f := clienttest.New().
		Returns("ClockIn", status(wire.StateWorking, &pID), nil).
		Returns("ClockOut", off, nil).
		Returns("BreakStart", status(wire.StateBreakManual, &pID), nil).
		Returns("BreakEnd", status(wire.StateWorking, &pID), nil).
		Returns("Snooze", status(wire.StateWorking, &pID), nil).
		Returns("Switch", status(wire.StateWorking, &qID), nil)
	run := &fakeRunner{}
	v := &fakeView{}
	c := &controller{api: f, clk: clock.NewFake(now), view: v, exec: run}
	wait := func(method string, n int) {
		t.Helper()
		require.Eventually(t, func() bool { return len(f.CallsTo(method)) == n }, 5*time.Second, time.Millisecond, method)
	}

	// Clock in resumes the last attribution seen.
	c.setStatus(status(wire.StateWorking, &qID))
	c.setStatus(off)
	c.clockIn()
	wait("ClockIn", 1)
	require.Equal(t, []any{wire.ClockInRequest{ProjectID: &qID}}, f.CallsTo("ClockIn")[0].Args)

	c.startBreak()
	wait("BreakStart", 1)
	c.endBreak()
	wait("BreakEnd", 1)
	c.snooze()
	wait("Snooze", 1)
	c.switchTo(&qID)
	wait("Switch", 1)
	require.Equal(t, []any{wire.SwitchRequest{ProjectID: &qID}}, f.CallsTo("Switch")[0].Args)
	c.clockOut()
	wait("ClockOut", 1)
	require.Eventually(t, func() bool { return v.last().ClockIn }, 5*time.Second, time.Millisecond)

	c.openDashboard()
	c.startGwen()
	require.Equal(t, [][]string{{"gwen-ui"}, {"systemctl", "--user", "start", "gwend.service"}}, run.runs)
}

func TestControllerShowsTheDaemonDown(t *testing.T) {
	t.Parallel()
	f := clienttest.New().Returns("BreakStart", nil, client.ErrDaemonNotRunning)
	v := &fakeView{}
	c := &controller{api: f, clk: clock.NewFake(now), view: v, exec: &fakeRunner{}}
	c.setStatus(status(wire.StateWorking, &pID))
	c.startBreak()
	require.Eventually(t, func() bool { return v.last().StartGwen }, 5*time.Second, time.Millisecond)
}

func TestSingleInstance(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/run/tray.lock"
	unlock, err := singleInstance(path)
	require.NoError(t, err)
	_, err = singleInstance(path)
	require.ErrorContains(t, err, "already running")
	unlock()
	again, err := singleInstance(path)
	require.NoError(t, err)
	again()
}
