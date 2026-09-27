//go:build manual

package notify

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestManualDesktopActions shows a real idle nudge (docs/11-testing.md W4).
// Run it on KDE with
//
//	go test -tags manual -run TestManualDesktopActions -v ./internal/notify
//
// and click each of the three buttons in turn within two minutes; each click
// prints its action and shows the nudge again.
func TestManualDesktopActions(t *testing.T) {
	d := NewDesktop()
	if !d.Available() {
		t.Fatal("no notification service")
	}
	n := Notification{Kind: "idle", Title: "Still there?", Body: "No input for 3m. It stops counting as work at 10m.",
		Actions: []Action{{"back", "I'm back"}, {"break", "Start break"}, {"snooze", "Snooze 10m"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := d.Notify(ctx, n); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case ev := <-d.Actions():
			fmt.Printf("clicked %s on %s\n", ev.Action, ev.Kind)
			seen[ev.Action] = true
			if err := d.Notify(ctx, n); err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatalf("only saw %v", seen)
		}
	}
	d.Withdraw(ctx, "idle")
}
