package timeengine_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/testutil"
	te "github.com/kzark/gwen/internal/timeengine"
	"github.com/stretchr/testify/require"
)

// The recovery scenarios of docs/11-testing.md#recovery-scenarios; now is
// 2026-09-15 11:30 unless stated.

func openDay(day, clockedIn string) *model.WorkDay {
	return &model.WorkDay{Day: day, TZ: "UTC", ClockedInAt: on(day, clockedIn)}
}

func openSeg(kind, source string, project *string, started time.Time) *model.Segment {
	return &model.Segment{Kind: kind, Source: source, ProjectID: project, StartedAt: started}
}

func tp(t time.Time) *time.Time { return &t }

func withoutRecords(effects []te.Effect) []te.Effect {
	var out []te.Effect
	for _, e := range effects {
		if _, ok := e.(te.RecordTransition); !ok {
			out = append(out, e)
		}
	}
	return out
}

func TestRecovery(t *testing.T) {
	t.Parallel()
	now := at("11:30")
	tests := []struct {
		name    string
		rec     te.Recovered
		effects []te.Effect
		state   te.State
		from    string // the recorded transition's From, when there are effects
	}{
		{
			name: "R1 crash mid-work",
			rec: te.Recovered{Now: now, WorkDay: openDay(testutil.Day0, "09:00"),
				Segment: openSeg("work", "user", P, at("09:00")), HeartbeatAt: tp(at("10:59:45"))},
			effects: []te.Effect{
				te.CloseSegment{At: at("10:59:45"), Truncated: true},
				te.OpenSegment{Kind: "break_auto", Source: "recovery", At: at("10:59:45")},
			},
			state: te.BreakAuto,
			from:  "working",
		},
		{
			name: "R2 quick restart",
			rec: te.Recovered{Now: now, WorkDay: openDay(testutil.Day0, "09:00"),
				Segment: openSeg("work", "user", P, at("09:00")), HeartbeatAt: tp(at("11:29:50"))},
			state: te.Working,
		},
		{
			name: "R3 clean shutdown, next day",
			rec: te.Recovered{Now: now, WorkDay: openDay("2026-09-14", "18:00"),
				Segment:    openSeg("work", "user", P, on("2026-09-14", "18:00")),
				ShutdownAt: tp(on("2026-09-14", "22:00"))},
			effects: []te.Effect{
				te.CloseSegment{At: on("2026-09-14", "22:00"), Truncated: false},
				te.CloseDay{At: on("2026-09-14", "22:00")},
			},
			state: te.Off,
			from:  "working",
		},
		{
			name: "R4 open day, no segment",
			rec: te.Recovered{Now: now, WorkDay: openDay(testutil.Day0, "09:00"),
				LastEndedAt: tp(at("10:00")), HeartbeatAt: tp(at("10:00:10"))},
			effects: []te.Effect{te.OpenSegment{Kind: "break_auto", Source: "recovery", At: at("10:00:10")}},
			state:   te.BreakAuto,
			from:    "off",
		},
		{
			name:  "R5 nothing open",
			rec:   te.Recovered{Now: now},
			state: te.Off,
		},
		{
			name: "R6 stale heartbeat",
			rec: te.Recovered{Now: now, WorkDay: openDay(testutil.Day0, "09:00"),
				Segment: openSeg("work", "user", P, at("11:00")), HeartbeatAt: tp(on("2026-09-14", "17:00"))},
			effects: []te.Effect{
				te.CloseSegment{At: at("11:00"), Truncated: true},
				te.OpenSegment{Kind: "break_auto", Source: "recovery", At: at("11:00")},
			},
			state: te.BreakAuto,
			from:  "working",
		},
		{
			name: "no heartbeat and no segment starts the gap at clock-in",
			rec:  te.Recovered{Now: now, WorkDay: openDay(testutil.Day0, "09:00")},
			effects: []te.Effect{
				te.OpenSegment{Kind: "break_auto", Source: "recovery", At: at("09:00")},
			},
			state: te.BreakAuto,
			from:  "off",
		},
		{
			name: "shutdown mid-break, same day",
			rec: te.Recovered{Now: now, WorkDay: openDay(testutil.Day0, "09:00"),
				Segment: openSeg("break_manual", "user", nil, at("10:00")), ShutdownAt: tp(at("10:30")),
				LastEndedAt: tp(at("10:00"))},
			effects: []te.Effect{
				te.CloseSegment{At: at("10:30")},
				te.OpenSegment{Kind: "break_auto", Source: "recovery", At: at("10:30")},
			},
			state: te.BreakAuto,
			from:  "break_manual",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := te.New(defaults(), tc.rec)
			got := e.RecoveryEffects()
			require.Equal(t, tc.effects, withoutRecords(got))
			require.Equal(t, tc.state, e.Status().State)
			if len(tc.effects) == 0 {
				require.Empty(t, got)
				return
			}
			last, ok := got[len(got)-1].(te.RecordTransition)
			require.True(t, ok, "recovery is recorded last")
			require.Equal(t, "recovery", last.Trigger)
			require.Equal(t, tc.from, last.From)
			require.Equal(t, string(tc.state), last.To)
		})
	}
}

func TestRecoveryQuickRestartKeepsBreak(t *testing.T) {
	t.Parallel()
	e := te.New(defaults(), te.Recovered{
		Now: at("11:30"), WorkDay: openDay(testutil.Day0, "09:00"),
		Segment:     openSeg("break_manual", "user", nil, at("11:20")),
		HeartbeatAt: tp(at("11:29:55")), ProjectID: Q,
	})
	require.Empty(t, e.RecoveryEffects())
	st := e.Status()
	require.Equal(t, te.BreakManual, st.State)
	require.Equal(t, Q, st.ProjectID, "work resumes on the last work segment's attribution")
	dl, ok := e.NextDeadline()
	require.True(t, ok)
	require.Equal(t, at("11:35"), dl, "break_long is measured from the break's start")

	r := &run{t: t, e: e}
	r.feed(te.BreakEnd{At: at("11:40")})
	require.Equal(t, []string{"break_manual/user 11:20–11:40", "work/user Q 11:40–"}, fold(append(
		[]te.Effect{te.OpenSegment{Kind: "break_manual", Source: "user", At: at("11:20")}}, r.steps[0].effects...)))
}

func TestRecoveryAttributionAndClosedDay(t *testing.T) {
	t.Parallel()
	e := te.New(defaults(), te.Recovered{Now: at("11:30"), ProjectID: P, ClosedDay: testutil.Day0})
	require.Equal(t, P, e.Status().ProjectID)
	r := &run{t: t, e: e}
	r.feed(te.Active{At: at("11:31")})
	require.Empty(t, r.nudges(), "no clock_in prompt on a day already clocked out")

	in, ok := te.InputForAction(te.ActionClockIn, at("11:32"), e.Status())
	require.True(t, ok)
	require.Equal(t, te.ClockIn{ProjectID: P, At: at("11:32")}, in)
}
