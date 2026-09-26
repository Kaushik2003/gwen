// Package clock is the only source of the current time outside of main. The
// real implementation wraps package time; tests use NewFake, which advances only
// when told to. See docs/CONVENTIONS.md#time.
package clock

import "time"

// Clock tells the time and creates timers.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// Timer is a one-shot timer with the semantics of *time.Timer since Go 1.23:
// Stop and Reset discard any value not yet received from C.
type Timer interface {
	// C delivers the instant the timer fired.
	C() <-chan time.Time
	// Stop prevents the timer from firing and reports whether it was active.
	Stop() bool
	// Reset re-arms the timer to fire after d and reports whether it was active.
	Reset(d time.Duration) bool
}

// Real returns the wall clock.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time { return r.t.C }

func (r realTimer) Stop() bool { return r.t.Stop() }

func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }
