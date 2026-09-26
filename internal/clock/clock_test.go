package clock_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)

func fired(tm clock.Timer) (time.Time, bool) {
	select {
	case at := <-tm.C():
		return at, true
	default:
		return time.Time{}, false
	}
}

func TestFakeFiresOnlyOnAdvance(t *testing.T) {
	t.Parallel()
	f := clock.NewFake(t0)
	tm := f.NewTimer(10 * time.Second)

	_, ok := fired(tm)
	require.False(t, ok, "timer fired before Advance")

	f.Advance(9 * time.Second)
	_, ok = fired(tm)
	require.False(t, ok, "timer fired early")

	f.Advance(time.Second)
	at, ok := fired(tm)
	require.True(t, ok)
	require.Equal(t, t0.Add(10*time.Second), at)
	require.Equal(t, t0.Add(10*time.Second), f.Now())
}

func TestFakeFiresInDeadlineOrder(t *testing.T) {
	t.Parallel()
	f := clock.NewFake(t0)
	late := f.NewTimer(3 * time.Minute)
	early := f.NewTimer(time.Minute)
	f.Advance(time.Hour)

	at, ok := fired(early)
	require.True(t, ok)
	require.Equal(t, t0.Add(time.Minute), at)
	at, ok = fired(late)
	require.True(t, ok)
	require.Equal(t, t0.Add(3*time.Minute), at)
	require.Equal(t, t0.Add(time.Hour), f.Now())
	require.Zero(t, f.Armed())
}

func TestFakeStopAndReset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		act     func(f *clock.Fake, tm clock.Timer) bool
		wantRet bool
		advance time.Duration
		wantAt  time.Time
		fires   bool
	}{
		{
			name:    "stop before deadline",
			act:     func(_ *clock.Fake, tm clock.Timer) bool { return tm.Stop() },
			wantRet: true,
			advance: time.Hour,
		},
		{
			name: "stop after firing discards the value",
			act: func(f *clock.Fake, tm clock.Timer) bool {
				f.Advance(time.Minute)
				return tm.Stop()
			},
			wantRet: false,
			advance: time.Hour,
		},
		{
			name:    "reset moves the deadline",
			act:     func(_ *clock.Fake, tm clock.Timer) bool { return tm.Reset(2 * time.Minute) },
			wantRet: true,
			advance: 2 * time.Minute,
			wantAt:  t0.Add(2 * time.Minute),
			fires:   true,
		},
		{
			name: "reset after firing re-arms",
			act: func(f *clock.Fake, tm clock.Timer) bool {
				f.Advance(time.Minute)
				return tm.Reset(time.Minute)
			},
			wantRet: false,
			advance: time.Minute,
			wantAt:  t0.Add(2 * time.Minute),
			fires:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := clock.NewFake(t0)
			tm := f.NewTimer(time.Minute)
			require.Equal(t, tc.wantRet, tc.act(f, tm))
			f.Advance(tc.advance)
			at, ok := fired(tm)
			require.Equal(t, tc.fires, ok)
			if tc.fires {
				require.Equal(t, tc.wantAt, at)
			}
		})
	}
}

func TestFakeNonPositiveTimerFiresImmediately(t *testing.T) {
	t.Parallel()
	f := clock.NewFake(t0)
	tm := f.NewTimer(0)
	at, ok := fired(tm)
	require.True(t, ok)
	require.Equal(t, t0, at)
}

func TestFakeBlockUntil(t *testing.T) {
	t.Parallel()
	f := clock.NewFake(t0)
	armed := make(chan clock.Timer)
	go func() { armed <- f.NewTimer(time.Second) }()
	f.BlockUntil(1)
	f.Advance(time.Second)
	tm := <-armed
	_, ok := fired(tm)
	require.True(t, ok)
}

func TestFakeSetBackwardsPanics(t *testing.T) {
	t.Parallel()
	f := clock.NewFake(t0)
	require.Panics(t, func() { f.Set(t0.Add(-time.Second)) })
}

func TestRealClock(t *testing.T) {
	t.Parallel()
	c := clock.Real()
	require.False(t, c.Now().IsZero())
	tm := c.NewTimer(time.Hour)
	require.True(t, tm.Stop())
	require.False(t, tm.Reset(time.Hour))
	require.True(t, tm.Stop())
	require.NotNil(t, tm.C())
}
