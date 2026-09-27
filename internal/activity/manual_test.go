//go:build manual

package activity

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
)

// TestManualMonitor runs the real monitor for two minutes and prints what it
// sees (docs/11-testing.md W2). Run it on KDE Plasma Wayland with
//
//	go test -tags manual -run TestManualMonitor -v ./internal/activity
//
// and then: leave the keyboard and mouse alone for 25 s (Idle at 5 s and 20 s),
// touch the mouse (Active), lock and unlock the screen, suspend and resume,
// and play a video with no input for 25 s (Idle must still fire).
func TestManualMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	m := New(ctx, clock.Real())
	fmt.Printf("backend: %s\n", m.Name())
	events, err := m.Start(ctx, []time.Duration{5 * time.Second, 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for ev := range events {
		fmt.Printf("%s %-8s %v\n", ev.At.Format("15:04:05.000"), ev.Kind, ev.Threshold)
		if ev.Ack != nil {
			ev.Ack()
		}
	}
}

// TestManualProbe only probes the backends and reports which one wins. It
// reads the compositor's globals and nothing else.
func TestManualProbe(t *testing.T) {
	m := newMonitor(context.Background(), clock.Real(), defaultProbers(clock.Real()), nil)
	fmt.Printf("backend: %s\n", m.Name())
}
