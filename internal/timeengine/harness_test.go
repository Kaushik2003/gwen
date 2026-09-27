package timeengine_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/testutil"
	te "github.com/kzark/gwen/internal/timeengine"
	"github.com/stretchr/testify/require"
)

var (
	P = testutil.Ptr("P")
	Q = testutil.Ptr("Q")
	R = testutil.Ptr("R")
)

// defaults is the configuration of the golden scenarios in docs/11-testing.md.
func defaults() te.Config {
	return te.Config{
		SoftIdle:      3 * time.Minute,
		HardIdle:      10 * time.Minute,
		BreakReminder: 15 * time.Minute,
		Repeat:        10 * time.Minute,
		Snooze:        10 * time.Minute,
		DayRollover:   4 * 60,
		DailyTarget:   8 * time.Hour,
		Location:      time.UTC,
	}
}

func at(hms string) time.Time                  { return testutil.At(hms) }
func on(day, hms string) time.Time             { return testutil.AtOn(day, hms) }
func soft(hms string) te.Idle                  { return te.Idle{Threshold: 3 * time.Minute, At: at(hms)} }
func hard(hms string) te.Idle                  { return te.Idle{Threshold: 10 * time.Minute, At: at(hms)} }
func clockIn(p *string, hms string) te.ClockIn { return te.ClockIn{ProjectID: p, At: at(hms)} }

type step struct {
	in      te.Input
	effects []te.Effect
	before  te.Snapshot
	after   te.Snapshot
}

type run struct {
	t     *testing.T
	e     *te.Engine
	steps []step
}

// start returns an engine that recovered nothing, as on a fresh install.
func start(t *testing.T, cfg te.Config) *run {
	t.Helper()
	e := te.New(cfg, te.Recovered{Now: on(testutil.Day0, "08:00")})
	require.Empty(t, e.RecoveryEffects())
	return &run{t: t, e: e}
}

// feed decides and accepts each input, checking on the way that Decide is pure
// (G18) and that transitions are recorded as a chain from the old state to the
// new one.
func (r *run) feed(inputs ...te.Input) *run {
	r.t.Helper()
	for _, in := range inputs {
		before := r.e.Status()
		d, err := r.e.Decide(in)
		require.NoError(r.t, err, "%T at %v", in, in)
		again, err := r.e.Decide(in)
		require.NoError(r.t, err)
		require.Equal(r.t, d, again, "Decide must be pure")
		require.Equal(r.t, before, r.e.Status(), "Decide must not change the engine")
		r.e.Accept(d)
		checkTransitions(r.t, before, d)
		r.steps = append(r.steps, step{in: in, effects: d.Effects, before: before, after: r.e.Status()})
	}
	return r
}

func checkTransitions(t *testing.T, before te.Snapshot, d te.Decision) {
	t.Helper()
	state := string(before.State)
	for _, e := range d.Effects {
		if rt, ok := e.(te.RecordTransition); ok {
			require.Equal(t, state, rt.From, "transition chain")
			require.NotEmpty(t, rt.Trigger)
			require.NotNil(t, rt.Data)
			state = rt.To
		}
	}
	require.Equal(t, string(d.Next.State), state, "the last transition must reach the new state")
}

func (r *run) state() te.State { return r.e.Status().State }

// segments folds every committed effect into a list of segments rendered as
// "kind/source project start–end".
func (r *run) segments() []string {
	var all []te.Effect
	for _, s := range r.steps {
		all = append(all, s.effects...)
	}
	return fold(all)
}

func fold(effects []te.Effect) []string {
	type seg struct {
		desc       string
		start, end time.Time
		truncated  bool
		open       bool
	}
	var segs []*seg
	for _, e := range effects {
		switch e := e.(type) {
		case te.OpenSegment:
			desc := e.Kind + "/" + e.Source
			if e.ProjectID != nil {
				desc += " " + *e.ProjectID
			}
			segs = append(segs, &seg{desc: desc, start: e.At, open: true})
		case te.CloseSegment:
			last := segs[len(segs)-1]
			if !last.open {
				panic("close without an open segment")
			}
			last.end, last.truncated, last.open = e.At, e.Truncated, false
		}
	}
	out := make([]string, len(segs))
	for i, s := range segs {
		end := ""
		if !s.open {
			end = clock(s.end)
		}
		out[i] = fmt.Sprintf("%s %s–%s", s.desc, clock(s.start), end)
		if s.truncated {
			out[i] += " truncated"
		}
	}
	return out
}

// days folds StartDay and CloseDay into "day opened–closed".
func (r *run) days() []string {
	var out []string
	for _, s := range r.steps {
		for _, e := range s.effects {
			switch e := e.(type) {
			case te.StartDay:
				out = append(out, fmt.Sprintf("%s %s–", e.Day, clock(e.At)))
			case te.CloseDay:
				out[len(out)-1] += clock(e.At)
			}
		}
	}
	return out
}

// nudges lists "+kind" for each Notify at its instant and "-kind" for each
// Withdraw at the input's instant.
func (r *run) nudges() []string {
	var out []string
	for _, s := range r.steps {
		for _, e := range s.effects {
			switch e := e.(type) {
			case te.Notify:
				out = append(out, clock(e.At)+" +"+e.Kind)
			case te.Withdraw:
				out = append(out, clock(instant(s.in))+" -"+e.Kind)
			}
		}
	}
	return out
}

func (r *run) notifies() []te.Notify {
	var out []te.Notify
	for _, s := range r.steps {
		for _, e := range s.effects {
			if n, ok := e.(te.Notify); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

func (r *run) transitions(i int) []te.RecordTransition {
	var out []te.RecordTransition
	for _, e := range r.steps[i].effects {
		if rt, ok := e.(te.RecordTransition); ok {
			out = append(out, rt)
		}
	}
	return out
}

func instant(in te.Input) time.Time {
	switch in := in.(type) {
	case te.ClockIn:
		return in.At
	case te.ClockOut:
		return in.At
	case te.BreakStart:
		return in.At
	case te.BreakEnd:
		return in.At
	case te.Switch:
		return in.At
	case te.Snooze:
		return in.At
	case te.Idle:
		return in.At
	case te.Active:
		return in.At
	case te.Locked:
		return in.At
	case te.Unlocked:
		return in.At
	case te.Suspend:
		return in.At
	case te.Resume:
		return in.At
	case te.Tick:
		return in.At
	case te.ConfigChanged:
		return in.At
	}
	panic(fmt.Sprintf("unknown input %T", in))
}

// clock renders an instant in UTC as "HH:MM", with seconds and milliseconds
// when present, prefixed by the date when it is not Day0.
func clock(t time.Time) string {
	t = t.UTC()
	s := t.Format("15:04")
	if t.Second() != 0 || t.Nanosecond() != 0 {
		s = t.Format("15:04:05")
	}
	if t.Nanosecond() != 0 {
		s = t.Format("15:04:05.000")
	}
	if day := t.Format(time.DateOnly); day != testutil.Day0 {
		s = day + " " + s
	}
	return s
}

func TestClockRendering(t *testing.T) {
	t.Parallel()
	require.Equal(t, "09:00", clock(at("09:00")))
	require.Equal(t, "12:00:30", clock(at("12:00:30")))
	require.Equal(t, "09:00:00.500", clock(at("09:00:00.500")))
	require.Equal(t, "2026-09-16 04:00", clock(on("2026-09-16", "04:00")))
	require.True(t, strings.HasPrefix(clock(on("2026-09-16", "04:00")), "2026-09-16"))
}
