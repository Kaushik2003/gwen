package timeengine

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/stretchr/testify/require"
)

// Trigger names are the input names in snake case (05-time-engine.md#effects).
func TestTriggerNames(t *testing.T) {
	t.Parallel()
	want := map[Input]string{
		ClockIn{}: "clock_in", ClockOut{}: "clock_out", BreakStart{}: "break_start", BreakEnd{}: "break_end",
		Switch{}: "switch", Snooze{}: "snooze", Idle{}: "idle", Active{}: "active", Locked{}: "locked",
		Unlocked{}: "unlocked", Suspend{}: "suspend", Resume{}: "resume", Tick{}: "tick",
		ConfigChanged{}: "config_changed",
	}
	for in, name := range want {
		require.Equal(t, name, in.trigger(), "%T", in)
		require.True(t, in.instant().IsZero())
	}
	for _, e := range []Effect{StartDay{}, CloseDay{}, OpenSegment{}, CloseSegment{}, Notify{}, Withdraw{}, RecordTransition{}} {
		e.effect() // every effect type is sealed into Effect
	}
}

func TestFormatDurationMatchesClient(t *testing.T) {
	t.Parallel()
	for _, d := range []time.Duration{-time.Minute, 0, 999 * time.Millisecond, 45 * time.Second, time.Minute,
		59*time.Minute + 59*time.Second, time.Hour, time.Hour + 5*time.Minute, 26*time.Hour + time.Minute} {
		require.Equal(t, client.FormatDuration(d), formatDuration(d), d.String())
	}
}

func TestProgrammerErrorsPanic(t *testing.T) {
	t.Parallel()
	var s shownSet
	require.Panics(t, func() { s.ptr("nap") })
	require.Panics(t, func() { rolloverAfter("15/09/2026", Config{}) })
	require.Equal(t, time.UTC, Config{}.location(), "no zone means UTC")
	require.Equal(t, "2026-09-15", dayOf(time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC), Config{DayRollover: 240}))
}
