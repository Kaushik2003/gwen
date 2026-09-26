package client_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// log records Follower callbacks in order.
type log struct {
	mu      sync.Mutex
	entries []string
	changed chan struct{}
}

func newLog() *log { return &log{changed: make(chan struct{}, 100)} }

func (l *log) add(s string) {
	l.mu.Lock()
	l.entries = append(l.entries, s)
	l.mu.Unlock()
	l.changed <- struct{}{}
}

// waitFor blocks until n entries have been recorded.
func (l *log) waitFor(t *testing.T, n int) []string {
	t.Helper()
	for {
		l.mu.Lock()
		if len(l.entries) >= n {
			out := append([]string(nil), l.entries...)
			l.mu.Unlock()
			return out
		}
		l.mu.Unlock()
		select {
		case <-l.changed:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %d callbacks; have %v", n, l.entries)
		}
	}
}

func TestFollowerReconnectsWithBackoff(t *testing.T) {
	t.Parallel()
	clk := clock.NewFake(testutil.T0)
	first, second := clienttest.NewStream(), clienttest.NewStream()

	var mu sync.Mutex
	attempt := 0
	results := []func() (any, error){
		func() (any, error) { return nil, client.ErrDaemonNotRunning },
		func() (any, error) { return nil, client.ErrDaemonNotRunning },
		func() (any, error) { return first, nil },
		func() (any, error) { return second, nil },
	}
	fake := clienttest.New().On("Events", func(...any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		r := results[min(attempt, len(results)-1)]
		attempt++
		return r()
	})

	l := newLog()
	f := client.Follower{
		API:          fake,
		Clock:        clk,
		OnConnect:    func() { l.add("connect") },
		OnEvent:      func(ev wire.Event) { l.add("event " + ev.Name) },
		OnDisconnect: func(err error) { l.add("disconnect " + err.Error()) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Run(ctx) }()

	l.waitFor(t, 1) // first attempt failed; backoff 1s
	clk.BlockUntil(1)
	clk.Advance(time.Second)
	l.waitFor(t, 2) // second attempt failed; backoff 2s
	clk.BlockUntil(1)
	clk.Advance(time.Second)
	require.Equal(t, 2, len(fake.CallsTo("Events")), "must not retry before the doubled backoff")
	clk.Advance(time.Second)

	first.Send(wire.Event{ID: 1, Name: wire.EventStateChanged})
	first.End(errors.New("connection reset"))
	l.waitFor(t, 5) // connect, event, disconnect; backoff resets to 1s
	clk.BlockUntil(1)
	clk.Advance(time.Second)
	second.Send(wire.Event{ID: 1, Name: wire.EventStateChanged})
	got := l.waitFor(t, 7)

	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, []string{
		"disconnect gwen isn't running",
		"disconnect gwen isn't running",
		"connect",
		"event state_changed",
		"disconnect connection reset",
		"connect",
		"event state_changed",
	}, got)
}

func TestFollowerBackoffCaps(t *testing.T) {
	t.Parallel()
	clk := clock.NewFake(testutil.T0)
	fake := clienttest.New().Returns("Events", nil, client.ErrDaemonNotRunning)
	l := newLog()
	f := client.Follower{API: fake, Clock: clk, OnEvent: func(wire.Event) {}, OnDisconnect: func(error) { l.add("x") }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Run(ctx)

	// 1, 2, 4, 8, 16, 30, 30: seven failures take 91 s.
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	for i, d := range want {
		l.waitFor(t, i+1)
		clk.BlockUntil(1)
		clk.Advance(d*time.Second - time.Millisecond)
		require.Len(t, fake.CallsTo("Events"), i+1, "retried early at step %d", i)
		clk.Advance(time.Millisecond)
	}
	l.waitFor(t, len(want)+1)
}
