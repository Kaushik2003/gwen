package timeengine_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/testutil"
	te "github.com/kzark/gwen/internal/timeengine"
	"github.com/stretchr/testify/require"
)

// The golden scenarios of docs/11-testing.md#golden-scenarios.

func TestG1GraceReturn(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"))
	require.Equal(t, te.IdlePending, r.state())
	r.feed(te.Active{At: at("10:07")}, te.ClockOut{At: at("12:00")})

	require.Equal(t, []string{"work/user P 09:00–12:00"}, r.segments())
	require.Equal(t, []string{"10:03 +idle", "10:07 -idle"}, r.nudges())
	require.Equal(t, te.Off, r.state())
}

func TestG2HardReclaim(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "09:00"), soft("10:03"), hard("10:10"),
		te.Active{At: at("10:40")}, te.ClockOut{At: at("12:00")},
	)
	require.Equal(t, []string{
		"work/user P 09:00–10:00",
		"break_auto/idle 10:00–10:40",
		"work/activity P 10:40–12:00",
	}, r.segments())
	require.Equal(t, []string{"10:03 +idle", "10:10 -idle"}, r.nudges())
	require.Equal(t, []string{"2026-09-15 09:00–12:00"}, r.days())
}

func TestG3ReclaimByDeadline(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"))
	dl, ok := r.e.NextDeadline()
	require.True(t, ok)
	require.Equal(t, at("10:10"), dl)

	r.feed(te.Tick{At: at("10:10")}, te.Active{At: at("10:40")}, te.ClockOut{At: at("12:00")})
	require.Equal(t, []string{
		"work/user P 09:00–10:00",
		"break_auto/idle 10:00–10:40",
		"work/activity P 10:40–12:00",
	}, r.segments())
	require.Equal(t, []string{"10:03 +idle", "10:10 -idle"}, r.nudges())
}

func TestG4MissedSoft(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), hard("10:10"))
	require.Equal(t, []string{"work/user P 09:00–10:00", "break_auto/idle 10:00–"}, r.segments())
	require.Empty(t, r.nudges(), "no idle nudge when the soft threshold was missed")
	require.Equal(t, te.BreakAuto, r.state())
}

func TestG5LockDuringIdle(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "09:00"), soft("10:03"), te.Locked{At: at("10:05")},
		te.Active{At: at("10:20")}, te.Unlocked{At: at("10:30")},
	)
	require.Equal(t, []string{
		"work/user P 09:00–10:00",
		"break_auto/lock 10:00–10:30",
		"work/activity P 10:30–",
	}, r.segments())
	require.Empty(t, r.steps[3].effects, "Active while locked is ignored")
	require.Equal(t, []string{"10:03 +idle", "10:05 -idle"}, r.nudges(),
		"a break_long due at the unlock is sent and withdrawn in one decision, so neither is emitted")
	require.Equal(t, te.Working, r.state())
}

func TestG6LockWhileWorking(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), te.Locked{At: at("11:00")})
	require.Equal(t, []string{"work/user P 09:00–11:00", "break_auto/lock 11:00–"}, r.segments())
	require.Equal(t, te.BreakAuto, r.state())
	require.True(t, r.e.Status().Locked)
}

func TestG7ManualBreak(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "09:00"), te.BreakStart{At: at("12:00")},
		te.Active{At: at("12:00:30")}, te.Active{At: at("12:02")}, te.Active{At: at("12:03")},
		te.Tick{At: at("12:15")}, te.Tick{At: at("12:25")}, te.BreakEnd{At: at("12:30")},
	)
	require.Equal(t, []string{
		"12:02 +break_active",
		"12:15 +break_long",
		"12:25 +break_long",
		"12:30 -break_long",
		"12:30 -break_active",
	}, r.nudges())
	require.Equal(t, []string{
		"work/user P 09:00–12:00",
		"break_manual/user 12:00–12:30",
		"work/user P 12:30–",
	}, r.segments())

	n := r.notifies()
	require.Equal(t, "You're typing during a break", n[0].Title)
	require.Equal(t, []te.Action{{ID: "end_break", Label: "Back to work"}}, n[0].Actions)
	require.Equal(t, "On a break for 15m", n[1].Title)
	require.Equal(t, "Ready to get back to it?", n[1].Body)
	require.Equal(t, []te.Action{{ID: "end_break", Label: "Back to work"}, {ID: "snooze", Label: "Snooze 10m"}}, n[1].Actions)
	require.Equal(t, "On a break for 25m", n[2].Title)
}

func TestG8Snooze(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"), hard("10:10"), te.Snooze{At: at("10:12")})
	dl, ok := r.e.NextDeadline()
	require.True(t, ok)
	require.Equal(t, at("10:22"), dl, "the break_long at 10:15 moves to the end of the snooze")
	require.Equal(t, at("10:22"), *r.e.Status().SnoozedUntil)

	r.feed(te.Tick{At: at("10:15")}, te.Tick{At: at("10:22")}, te.Tick{At: at("10:32")})
	require.Empty(t, r.steps[4].effects, "nothing at 10:15")
	require.Equal(t, []string{"10:03 +idle", "10:10 -idle", "10:22 +break_long", "10:32 +break_long"}, r.nudges())
	require.Nil(t, r.e.Status().SnoozedUntil, "an expired snooze is cleared")
}

func TestG9BreakFromIdle(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"), te.BreakStart{At: at("10:05")})
	require.Equal(t, []string{"work/user P 09:00–10:00", "break_manual/user 10:00–"}, r.segments())
	require.Equal(t, te.BreakManual, r.state())
	require.Equal(t, at("10:00"), r.e.Status().Since)
}

func TestG10ClockOutFromIdle(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"), te.ClockOut{At: at("10:05")})
	require.Equal(t, []string{"work/user P 09:00–10:00"}, r.segments())
	require.Equal(t, []string{"2026-09-15 09:00–10:00"}, r.days())
	require.Equal(t, te.Off, r.state())
}

func TestG11SuspendAndResume(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "09:00"), te.Suspend{At: at("13:00")},
		te.Resume{At: at("14:00")}, te.Active{At: at("14:00:05")},
	)
	require.Equal(t, []string{
		"work/user P 09:00–13:00",
		"break_auto/suspend 13:00–14:00:05",
		"work/activity P 14:00:05–",
	}, r.segments())
	require.Empty(t, r.transitions(2), "no transition at Resume")
	require.Equal(t, te.BreakAuto, r.steps[2].after.State)
	require.Empty(t, r.nudges(), "a break_long hours stale is skipped, not fired")
}

func TestG12SuspendOverRollover(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "09:00"), te.Suspend{At: at("23:30")},
		te.Resume{At: on("2026-09-16", "08:00")}, te.Active{At: on("2026-09-16", "08:00:03")},
	)
	require.Equal(t, []string{
		"work/user P 09:00–23:30",
		"break_auto/suspend 23:30–2026-09-16 04:00",
	}, r.segments())
	require.Equal(t, []string{"2026-09-15 09:00–2026-09-16 04:00"}, r.days())
	require.Equal(t, te.Off, r.state())
	require.Equal(t, []string{"2026-09-16 08:00:03 +clock_in"}, r.nudges(), "no break_long at any point")

	n := r.notifies()[0]
	require.Equal(t, "Ready to start?", n.Title)
	require.Equal(t, "Clock in to start tracking.", n.Body)
	require.Equal(t, []te.Action{{ID: "clock_in", Label: "Clock in"}}, n.Actions)
}

func TestG13RolloverWhileWorking(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "20:00"), te.Tick{At: on("2026-09-16", "04:00")})
	require.Equal(t, []string{
		"work/user P 20:00–2026-09-16 04:00",
		"work/activity P 2026-09-16 04:00–",
	}, r.segments())
	require.Equal(t, []string{"2026-09-15 20:00–2026-09-16 04:00", "2026-09-16 2026-09-16 04:00–"}, r.days())
	require.Equal(t, te.Working, r.state())
	require.Equal(t, "2026-09-16", r.e.Status().Day)
	rt := r.transitions(1)
	require.Len(t, rt, 1, "the rollover is recorded")
	require.Equal(t, "tick", rt[0].Trigger)
}

func TestG14Switching(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "09:00"),
		te.Switch{ProjectID: Q, At: at("10:00")},
		te.Switch{ProjectID: Q, At: at("10:30")},
		te.BreakStart{At: at("11:00")},
		te.Switch{ProjectID: R, At: at("11:10")},
		te.BreakEnd{At: at("11:20")},
	)
	require.Empty(t, r.steps[2].effects, "a second switch to the same attribution has no effects")
	require.Empty(t, r.steps[4].effects, "a switch during a break only changes the attribution")
	require.Equal(t, R, r.steps[4].after.ProjectID)
	require.Equal(t, []string{
		"work/user P 09:00–10:00",
		"work/user Q 10:00–11:00",
		"break_manual/user 11:00–11:20",
		"work/user R 11:20–",
	}, r.segments())
}

func TestG15InvalidCommands(t *testing.T) {
	t.Parallel()
	off := func(t *testing.T) *run { return start(t, defaults()) }
	working := func(t *testing.T) *run { return off(t).feed(clockIn(P, "09:00")) }
	manual := func(t *testing.T) *run { return working(t).feed(te.BreakStart{At: at("10:00")}) }
	tests := []struct {
		name    string
		setup   func(*testing.T) *run
		in      te.Input
		message string
	}{
		{"break start while off", off, te.BreakStart{At: at("11:00")}, "cannot start a break while off"},
		{"break end while off", off, te.BreakEnd{At: at("11:00")}, "cannot end a break while off"},
		{"clock out while off", off, te.ClockOut{At: at("11:00")}, "cannot clock out while off"},
		{"snooze while off", off, te.Snooze{At: at("11:00")}, "cannot snooze nudges while off"},
		{"switch while off", off, te.Switch{ProjectID: Q, At: at("11:00")}, "cannot switch while off"},
		{"clock in while working", working, clockIn(Q, "11:00"), "cannot clock in while working"},
		{"break end while working", working, te.BreakEnd{At: at("11:00")}, "cannot end a break while working"},
		{"break start during a manual break", manual, te.BreakStart{At: at("11:00")}, "cannot start a break during a break"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := tc.setup(t)
			before := r.e.Status()
			_, err := r.e.Decide(tc.in)
			require.ErrorIs(t, err, te.ErrInvalidState)
			require.EqualError(t, err, tc.message)
			require.Equal(t, before, r.e.Status(), "state unchanged")
		})
	}
}

func TestG15InvalidWhileIdle(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"))
	for _, in := range []te.Input{clockIn(P, "10:04"), te.BreakEnd{At: at("10:04")}} {
		_, err := r.e.Decide(in)
		require.ErrorIs(t, err, te.ErrInvalidState)
		require.Contains(t, err.Error(), "while idle")
	}
	require.False(t, errors.Is(errors.New("other"), te.ErrInvalidState))
}

func TestG16SubSecondReclaim(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		te.ClockIn{ProjectID: P, At: at("09:00:00")},
		te.Idle{Threshold: 3 * time.Minute, At: at("09:03:00.500")},
		te.Tick{At: at("09:10:00.500")},
	)
	tick := r.steps[2].effects
	require.Equal(t, te.CloseSegment{At: at("09:00:00.500")}, tick[0])
	require.Equal(t, te.OpenSegment{Kind: "break_auto", Source: "idle", At: at("09:00:00.500")}, tick[1])
}

func TestG17DST(t *testing.T) {
	t.Parallel()
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	cfg := defaults()
	cfg.Location = ny
	r := start(t, cfg).feed(
		te.ClockIn{ProjectID: P, At: testutil.AtIn(ny, "2026-10-31", "22:00")},
		te.Tick{At: time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)}, // 01:30 EST, after the fall-back
	)
	st := r.e.Status()
	require.Equal(t, te.Working, st.State)
	require.Equal(t, "2026-10-31", st.Day)
	require.Equal(t, "America/New_York", st.TZ)
	dl, ok := r.e.NextDeadline()
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), dl.UTC(), "04:00 EST")
}

func TestG18DecideIsPure(t *testing.T) {
	t.Parallel()
	// feed checks purity on every step; this makes the check explicit on a
	// decision with catch-up, nudges, and a transition.
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"))
	before := r.e.Status()
	in := te.Active{At: at("10:30")} // the hard deadline at 10:10 is caught up first
	a, err := r.e.Decide(in)
	require.NoError(t, err)
	b, err := r.e.Decide(in)
	require.NoError(t, err)
	require.Equal(t, a, b)
	require.Equal(t, before, r.e.Status())
	r.e.Accept(a)
	require.Equal(t, te.Working, r.e.Status().State)
}
