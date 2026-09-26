package clock

import (
	"slices"
	"sync"
	"time"
)

// Fake is a Clock for tests. Time stands still until Advance or Set moves it,
// and timers fire only then, in deadline order. It is safe for concurrent use.
type Fake struct {
	mu     sync.Mutex
	cond   *sync.Cond
	now    time.Time
	active []*fakeTimer
}

// NewFake returns a fake clock reading t0.
func NewFake(t0 time.Time) *Fake {
	f := &Fake{now: t0}
	f.cond = sync.NewCond(&f.mu)
	return f
}

// Now returns the fake's current instant.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// NewTimer returns a timer that fires once the fake reaches Now()+d. A timer
// with d <= 0 fires immediately, like a real one.
func (f *Fake) NewTimer(d time.Duration) Timer {
	t := &fakeTimer{f: f, c: make(chan time.Time, 1)}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.arm(t, d)
	return t
}

// Advance moves the clock forward by d, firing every timer due by then.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	target := f.now.Add(d)
	f.mu.Unlock()
	f.Set(target)
}

// Set moves the clock to t, firing every timer due by then in deadline order.
// Moving backwards is a programmer error.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t.Before(f.now) {
		panic("clock: Fake.Set moves time backwards")
	}
	for {
		next := f.nextDue(t)
		if next == nil {
			break
		}
		f.now = next.deadline
		f.remove(next)
		next.fire()
	}
	f.now = t
}

// BlockUntil waits until at least n timers are armed. Tests call it before
// Advance so that a goroutine under test has had the chance to arm its timer.
func (f *Fake) BlockUntil(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.active) < n {
		f.cond.Wait()
	}
}

// Armed returns the number of timers waiting to fire.
func (f *Fake) Armed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.active)
}

// arm schedules t; f.mu must be held.
func (f *Fake) arm(t *fakeTimer, d time.Duration) {
	t.deadline = f.now.Add(d)
	if d <= 0 {
		t.fire()
		return
	}
	f.active = append(f.active, t)
	f.cond.Broadcast()
}

// nextDue returns the earliest armed timer due at or before t; f.mu must be held.
func (f *Fake) nextDue(t time.Time) *fakeTimer {
	var best *fakeTimer
	for _, ft := range f.active {
		if ft.deadline.After(t) {
			continue
		}
		if best == nil || ft.deadline.Before(best.deadline) {
			best = ft
		}
	}
	return best
}

// remove disarms t and reports whether it was armed; f.mu must be held.
func (f *Fake) remove(t *fakeTimer) bool {
	i := slices.Index(f.active, t)
	if i < 0 {
		return false
	}
	f.active = slices.Delete(f.active, i, i+1)
	return true
}

type fakeTimer struct {
	f        *Fake
	c        chan time.Time
	deadline time.Time
}

func (t *fakeTimer) C() <-chan time.Time { return t.c }

func (t *fakeTimer) Stop() bool {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	t.drain()
	return t.f.remove(t)
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	t.drain()
	was := t.f.remove(t)
	t.f.arm(t, d)
	return was
}

// fire delivers the deadline without blocking; the channel has room for one.
func (t *fakeTimer) fire() {
	select {
	case t.c <- t.deadline:
	default:
	}
}

func (t *fakeTimer) drain() {
	select {
	case <-t.c:
	default:
	}
}
