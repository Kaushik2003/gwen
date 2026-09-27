package timeengine_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	te "github.com/kzark/gwen/internal/timeengine"
	"github.com/stretchr/testify/require"
)

// Rows of the transitions table in docs/05-time-engine.md#transitions that the
// golden scenarios do not already cover.

func TestClockInNudge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		inputs []te.Input
		want   []string
	}{
		{
			name:   "active prompts once per day",
			inputs: []te.Input{te.Active{At: at("08:30")}, te.Active{At: at("08:31")}},
			want:   []string{"08:30 +clock_in"},
		},
		{
			name:   "unlocked prompts",
			inputs: []te.Input{te.Locked{At: at("08:10")}, te.Unlocked{At: at("08:30")}},
			want:   []string{"08:30 +clock_in"},
		},
		{
			name:   "resume while off prompts",
			inputs: []te.Input{te.Resume{At: at("08:30")}},
			want:   []string{"08:30 +clock_in"},
		},
		{
			name:   "active while locked is ignored",
			inputs: []te.Input{te.Locked{At: at("08:10")}, te.Active{At: at("08:30")}},
		},
		{
			name: "not again on a day already clocked out",
			inputs: []te.Input{
				clockIn(P, "09:00"), te.ClockOut{At: at("17:00")}, te.Active{At: at("17:05")},
			},
		},
		{
			name: "prompts again on the next day",
			inputs: []te.Input{
				te.Active{At: at("08:30")}, clockIn(P, "09:00"), te.ClockOut{At: at("17:00")},
				te.Active{At: on("2026-09-16", "03:59")}, te.Active{At: on("2026-09-16", "04:00")},
			},
			want: []string{"08:30 +clock_in", "09:00 -clock_in", "2026-09-16 04:00 +clock_in"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := start(t, defaults()).feed(tc.inputs...)
			require.Equal(t, tc.want, r.nudges())
		})
	}
}

func TestWorkingTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       te.Input
		segments []string
		state    te.State
	}{
		{"suspend", te.Suspend{At: at("11:00")}, []string{"work/user P 09:00–11:00", "break_auto/suspend 11:00–"}, te.BreakAuto},
		{"break start", te.BreakStart{At: at("11:00")}, []string{"work/user P 09:00–11:00", "break_manual/user 11:00–"}, te.BreakManual},
		{"clock out", te.ClockOut{At: at("11:00")}, []string{"work/user P 09:00–11:00"}, te.Off},
		{"switch to unassigned", te.Switch{At: at("11:00")}, []string{"work/user P 09:00–11:00", "work/user 11:00–"}, te.Working},
		{"unlocked is a no-op", te.Unlocked{At: at("11:00")}, []string{"work/user P 09:00–"}, te.Working},
		{"resume is a no-op", te.Resume{At: at("11:00")}, []string{"work/user P 09:00–"}, te.Working},
		{"presence threshold is ignored", te.Idle{Threshold: te.PresenceThreshold, At: at("11:00")}, []string{"work/user P 09:00–"}, te.Working},
		{"unknown threshold is ignored", te.Idle{Threshold: time.Hour, At: at("11:00")}, []string{"work/user P 09:00–"}, te.Working},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := start(t, defaults()).feed(clockIn(P, "09:00"), tc.in)
			require.Equal(t, tc.segments, r.segments())
			require.Equal(t, tc.state, r.state())
		})
	}
}

func TestIdlePendingTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       te.Input
		segments []string
		state    te.State
	}{
		{"suspend reclaims", te.Suspend{At: at("10:05")}, []string{"work/user P 09:00–10:00", "break_auto/suspend 10:00–"}, te.BreakAuto},
		{"hard idle event reclaims", hard("10:10"), []string{"work/user P 09:00–10:00", "break_auto/idle 10:00–"}, te.BreakAuto},
		{"switch splits at the switch", te.Switch{ProjectID: Q, At: at("10:05")}, []string{"work/user P 09:00–10:05", "work/user Q 10:05–"}, te.Working},
		{"switch to the same attribution only resumes", te.Switch{ProjectID: P, At: at("10:05")}, []string{"work/user P 09:00–"}, te.Working},
		{"a repeated soft idle is a no-op", soft("10:04"), []string{"work/user P 09:00–"}, te.IdlePending},
		{"snooze keeps the state", te.Snooze{At: at("10:05")}, []string{"work/user P 09:00–"}, te.IdlePending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"), tc.in)
			require.Equal(t, tc.segments, r.segments())
			require.Equal(t, tc.state, r.state())
			if tc.state == te.Working || tc.state == te.BreakAuto {
				require.Contains(t, r.nudges(), clock(instant(tc.in))+" -idle")
				require.Nil(t, r.e.Status().IdleSince)
			}
		})
	}
}

func TestBreakTransitions(t *testing.T) {
	t.Parallel()
	auto := []te.Input{clockIn(P, "09:00"), te.Locked{At: at("10:00")}}
	manual := []te.Input{clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}}
	tests := []struct {
		name     string
		setup    []te.Input
		in       te.Input
		segments []string
		state    te.State
	}{
		{"auto: break start becomes manual", auto, te.BreakStart{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_auto/lock 10:00–10:20", "break_manual/user 10:20–"}, te.BreakManual},
		{"auto: break end", auto, te.BreakEnd{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_auto/lock 10:00–10:20", "work/user P 10:20–"}, te.Working},
		{"auto: clock out", auto, te.ClockOut{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_auto/lock 10:00–10:20"}, te.Off},
		{"auto: switch only changes attribution", auto, te.Switch{ProjectID: Q, At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_auto/lock 10:00–"}, te.BreakAuto},
		{"auto: suspend is a no-op", auto, te.Suspend{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_auto/lock 10:00–"}, te.BreakAuto},
		{"manual: clock out", manual, te.ClockOut{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_manual/user 10:00–10:20"}, te.Off},
		{"manual: unlocked is a no-op", manual, te.Unlocked{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_manual/user 10:00–"}, te.BreakManual},
		{"manual: locked is a no-op beyond the flag", manual, te.Locked{At: at("10:20")},
			[]string{"work/user P 09:00–10:00", "break_manual/user 10:00–"}, te.BreakManual},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := start(t, defaults()).feed(tc.setup...).feed(tc.in)
			require.Equal(t, tc.segments, r.segments())
			require.Equal(t, tc.state, r.state())
		})
	}
}

func TestBreakLongIsWithdrawnWhenTheBreakEnds(t *testing.T) {
	t.Parallel()
	for _, end := range []te.Input{te.BreakStart{At: at("10:16")}, te.Active{At: at("10:16")}, te.ClockOut{At: at("10:16")}} {
		r := start(t, defaults()).feed(clockIn(P, "09:00"), te.Suspend{At: at("10:00")}, te.Tick{At: at("10:15")}, end)
		require.Equal(t, []string{"10:15 +break_long", "10:16 -break_long"}, r.nudges(), "%T", end)
	}
}

func TestSnoozeDropsTheIdleNudge(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), te.Snooze{At: at("09:59")}, soft("10:03"))
	require.Equal(t, te.IdlePending, r.state())
	require.Empty(t, r.nudges(), "idle is dropped while snoozed")

	// Once the snooze ends, the next idle period nudges again.
	r.feed(te.Active{At: at("10:05")}, soft("10:12"))
	require.Equal(t, []string{"10:12 +idle"}, r.nudges())
}

func TestRolloverWhileIdle(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(
		clockIn(P, "22:00"),
		te.Idle{Threshold: 3 * time.Minute, At: on("2026-09-16", "03:58")},
		te.Tick{At: on("2026-09-16", "04:00")},
	)
	require.Equal(t, []string{"work/user P 22:00–2026-09-16 04:00", "work/activity P 2026-09-16 04:00–"}, r.segments())
	require.Equal(t, te.Working, r.state())
	require.Contains(t, r.nudges(), "2026-09-16 04:00 -idle")
}

func TestRolloverDuringBreakEndsTheDay(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "22:00"), te.BreakStart{At: on("2026-09-16", "03:30")},
		te.Tick{At: on("2026-09-16", "03:45")}, te.Tick{At: on("2026-09-16", "04:00")})
	require.Equal(t, te.Off, r.state())
	require.Equal(t, []string{"2026-09-15 22:00–2026-09-16 04:00"}, r.days())
	require.Equal(t, []string{"2026-09-16 03:45 +break_long", "2026-09-16 04:00 -break_long"}, r.nudges())
	require.Equal(t, []string{"work/user P 22:00–2026-09-16 03:30", "break_manual/user 2026-09-16 03:30–2026-09-16 04:00"}, r.segments())
}

func TestConfigChanged(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), te.BreakStart{At: at("10:00")})
	cfg := defaults()
	cfg.SoftIdle, cfg.HardIdle, cfg.BreakReminder = 10*time.Second, 30*time.Second, 5*time.Minute
	r.feed(te.ConfigChanged{Config: cfg, At: at("10:01")})
	require.Equal(t, cfg, r.e.Config())
	dl, _ := r.e.NextDeadline()
	require.Equal(t, at("10:05"), dl, "the first reminder follows the new setting")
	require.Equal(t, []time.Duration{te.PresenceThreshold, 10 * time.Second, 30 * time.Second}, cfg.Thresholds())

	r.feed(te.BreakEnd{At: at("10:02")}, te.Idle{Threshold: 10 * time.Second, At: at("10:03")})
	require.Equal(t, te.IdlePending, r.state(), "the new soft threshold applies")
	require.Equal(t, "No input for 10s. It stops counting as work at 30s.", r.notifies()[0].Body)
}

func TestNextDeadline(t *testing.T) {
	t.Parallel()
	_, ok := start(t, defaults()).e.NextDeadline()
	require.False(t, ok, "nothing is due while off")

	tests := []struct {
		name   string
		inputs []te.Input
		want   time.Time
	}{
		{"working: rollover", []te.Input{clockIn(P, "09:00")}, on("2026-09-16", "04:00")},
		{"idle_pending: hard", []te.Input{clockIn(P, "09:00"), soft("10:03")}, at("10:10")},
		{"idle_pending: hard from a clamped idle_since",
			[]te.Input{clockIn(P, "09:00"), te.Switch{ProjectID: Q, At: at("10:02")}, soft("10:03")}, at("10:12")},
		{"break: break_long", []te.Input{clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}}, at("10:15")},
		{"break: break_long repeats", []te.Input{clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}, te.Tick{At: at("10:15")}}, at("10:25")},
		{"break: snooze moves break_long", []te.Input{clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}, te.Snooze{At: at("10:10")}}, at("10:20")},
		{"break: a snooze ending before the reminder changes nothing",
			[]te.Input{clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}, te.Snooze{At: at("10:01")}}, at("10:15")},
		{"break: rollover comes first", []te.Input{clockIn(P, "22:00"), te.BreakStart{At: on("2026-09-16", "03:50")}}, on("2026-09-16", "04:00")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dl, ok := start(t, defaults()).feed(tc.inputs...).e.NextDeadline()
			require.True(t, ok)
			require.Equal(t, tc.want, dl)
		})
	}
}

func TestIdleNudgeText(t *testing.T) {
	t.Parallel()
	r := start(t, defaults()).feed(clockIn(P, "09:00"), soft("10:03"))
	n := r.notifies()[0]
	require.Equal(t, te.Notify{
		Kind:  "idle",
		Title: "Still there?",
		Body:  "No input for 3m. It stops counting as work at 10m.",
		Actions: []te.Action{
			{ID: "back", Label: "I'm back"}, {ID: "break", Label: "Start break"}, {ID: "snooze", Label: "Snooze 10m"},
		},
		At: at("10:03"),
	}, n)
	require.Equal(t, at("10:00"), *r.e.Status().IdleSince)
}

func TestInputForAction(t *testing.T) {
	t.Parallel()
	s := start(t, defaults()).feed(clockIn(Q, "09:00")).e.Status()
	now := at("10:00")
	for action, want := range map[string]te.Input{
		"back":      te.Active{At: now},
		"break":     te.BreakStart{At: now},
		"snooze":    te.Snooze{At: now},
		"end_break": te.BreakEnd{At: now},
		"clock_in":  te.ClockIn{ProjectID: Q, At: now},
	} {
		got, ok := te.InputForAction(action, now, s)
		require.True(t, ok, action)
		require.Equal(t, want, got, action)
	}
	_, ok := te.InputForAction("default", now, s)
	require.False(t, ok)
}

func TestConfigFrom(t *testing.T) {
	t.Parallel()
	c := config.Defaults()
	got := te.ConfigFrom(c, time.UTC)
	require.Equal(t, defaults(), got)
	require.Equal(t, []time.Duration{te.PresenceThreshold, 3 * time.Minute, 10 * time.Minute}, got.Thresholds())

	c.Tracking.SoftIdle = te.PresenceThreshold
	require.Equal(t, []time.Duration{te.PresenceThreshold, 10 * time.Minute}, te.ConfigFrom(c, time.UTC).Thresholds(),
		"a soft threshold equal to the presence threshold is reported once")
}

func TestStartDayCarriesTheZoneAndTarget(t *testing.T) {
	t.Parallel()
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	require.NoError(t, err)
	cfg := defaults()
	cfg.Location = kolkata
	cfg.DailyTarget = 7*time.Hour + 30*time.Minute
	r := start(t, cfg).feed(te.ClockIn{At: testutil.AtIn(kolkata, "2026-09-16", "03:30")})
	require.Equal(t, te.StartDay{Day: "2026-09-15", TZ: "Asia/Kolkata", TargetSeconds: 27000,
		At: testutil.AtIn(kolkata, "2026-09-16", "03:30")}, r.steps[0].effects[0],
		"03:30 belongs to the previous day with the 04:00 rollover")
}

func TestFormatDurationMatchesTheClients(t *testing.T) {
	t.Parallel()
	// Nudge text renders durations exactly as internal/client does.
	for _, d := range []time.Duration{0, -time.Second, 45 * time.Second, time.Minute, 15 * time.Minute,
		time.Hour, time.Hour + 5*time.Minute, 7*time.Hour + 32*time.Minute + 59*time.Second} {
		cfg := defaults()
		cfg.Snooze = max(d, time.Second)
		r := start(t, cfg).feed(clockIn(P, "09:00"), soft("10:03"))
		label := r.notifies()[0].Actions[2].Label
		require.Equal(t, "Snooze "+client.FormatDuration(cfg.Snooze), label)
	}
	cfg := defaults()
	cfg.BreakReminder = 65 * time.Minute
	r := start(t, cfg).feed(clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}, te.Tick{At: at("11:05")})
	require.Equal(t, "On a break for "+client.FormatDuration(65*time.Minute), r.notifies()[0].Title)
}

func TestIdleOutsideWorkIsIgnored(t *testing.T) {
	t.Parallel()
	for _, in := range []te.Input{soft("10:30"), hard("10:30")} {
		r := start(t, defaults()).feed(clockIn(P, "09:00"), te.BreakStart{At: at("10:00")}, in)
		require.Empty(t, r.steps[2].effects, "%v during a break", in)
		require.Equal(t, te.BreakManual, r.state())

		r = start(t, defaults()).feed(in)
		require.Empty(t, r.steps[0].effects, "%v while off", in)
	}
}

func TestRecoveryQuickRestartKeepsAutoBreak(t *testing.T) {
	t.Parallel()
	e := te.New(defaults(), te.Recovered{
		Now: at("11:30"), WorkDay: openDay(testutil.Day0, "09:00"),
		Segment: openSeg("break_auto", "lock", nil, at("11:00")), HeartbeatAt: tp(at("11:29:40")),
	})
	require.Empty(t, e.RecoveryEffects())
	require.Equal(t, te.BreakAuto, e.Status().State)
	require.Equal(t, "lock", e.Status().Segment.Source)
}

func TestConfigDayOf(t *testing.T) {
	t.Parallel()
	c := defaults()
	require.Equal(t, "2026-09-15", c.DayOf(on("2026-09-16", "03:59")))
	require.Equal(t, "2026-09-16", c.DayOf(on("2026-09-16", "04:00")))
}
