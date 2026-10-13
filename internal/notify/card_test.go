package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestCardTakesOverTheDesktop(t *testing.T) {
	t.Parallel()
	desktop, phone := newFake("desktop"), newFake("phone")
	d := NewDispatcher(desktop, phone, nil, cfgWith(true, true))
	now := noon
	d.now = func() time.Time { return now }

	d.Send(idleNudge)
	d.Wait()
	require.Equal(t, []string{"idle"}, desktop.kinds(), "no card yet: the notification server shows it")

	nudges := d.Nudges(true)
	require.Len(t, nudges, 1)
	require.Equal(t, wire.Nudge{Kind: "idle", Title: idleNudge.Title, Body: idleNudge.Body, At: wire.Millis(noon),
		Actions: []wire.NudgeAction{{ID: "back", Label: "I'm back"}, {ID: "break", Label: "Start break"}, {ID: "snooze", Label: "Snooze 10m"}}}, nudges[0])

	now = now.Add(time.Minute)
	d.Nudges(true) // the card polls every few seconds
	d.Send(Notification{Kind: "break_long", Title: "Break", At: now.Add(-time.Second)})
	d.Wait()
	require.Equal(t, []string{"idle"}, desktop.kinds(), "the card shows it instead")
	require.Equal(t, []string{"idle", "break_long"}, phone.kinds(), "the phone still gets it")
	require.Equal(t, []string{"break_long", "idle"}, kindsOf(d.Nudges(false)), "newest first")

	d.Withdraw("idle")
	d.Wait()
	require.Equal(t, []string{"break_long"}, kindsOf(d.Nudges(false)))

	now = now.Add(cardFresh)
	d.Send(Notification{Kind: "clock_in"})
	d.Wait()
	require.Equal(t, []string{"idle", "clock_in"}, desktop.kinds(), "the card stopped polling")
}

func TestCardTestNudge(t *testing.T) {
	t.Parallel()
	desktop := newFake("desktop")
	d := NewDispatcher(desktop, newFake("phone"), nil, cfgWith(true, false))
	now := noon
	d.now = func() time.Time { return now }
	d.Nudges(true)
	require.Equal(t, wire.NotifyOK, d.Test(context.Background()).Desktop)
	require.Empty(t, desktop.kinds(), "the card shows the test")
	n := d.Nudges(true)
	require.Equal(t, []string{KindTest}, kindsOf(n))
	require.Equal(t, "winking", n[0].Mood)
	now = now.Add(testShown)
	require.Empty(t, d.Nudges(true), "the test leaves the card on its own")
}

func TestCardActs(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(newFake("desktop"), newFake("phone"), nil, cfgWith(true, false))
	d.Send(idleNudge)
	require.NoError(t, d.Act("idle", "break"))
	require.Equal(t, ActionEvent{Kind: "idle", Action: "break"}, <-d.Actions())
	require.ErrorIs(t, d.Act("idle", "clock_in"), ErrNoSuchNudge, "not one of its buttons")
	require.ErrorIs(t, d.Act("break_long", "end_break"), ErrNoSuchNudge, "not showing")
	d.Wait()
}

func TestCardChimes(t *testing.T) {
	t.Parallel()
	bus := &fakeBus{caps: []string{"actions"}, sigs: make(chan *dbus.Signal)}
	var mu sync.Mutex
	plays := 0
	desk := newDesktop(bus, func(context.Context) error { mu.Lock(); plays++; mu.Unlock(); return nil })
	d := NewDispatcher(desk, newFake("phone"), nil, cfgWith(true, false))
	d.Nudges(true)
	d.Send(idleNudge)
	d.Wait()
	require.Empty(t, bus.notifys, "the card shows it")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, plays, "and the daemon plays its sound")
}

func kindsOf(ns []wire.Nudge) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Kind)
	}
	return out
}

func TestPreviewShowsEveryNudge(t *testing.T) {
	t.Parallel()
	desktop := newFake("desktop")
	phone := newFake("phone")
	d := NewDispatcher(desktop, phone, nil, cfgWith(true, true))
	d.Nudges(true)
	planned := d.Preview()
	kinds := map[string]bool{}
	for _, n := range planned {
		kinds[n.Kind] = true
		require.True(t, n.Preview)
		require.NotEmpty(t, n.Mood)
	}
	require.Equal(t, map[string]bool{"idle": true, "break_long": true, "break_active": true, "clock_in": true}, kinds)
	require.Eventually(t, func() bool { return len(d.Nudges(true)) == 1 }, 5*time.Second, time.Millisecond)
	shown := d.Nudges(true)[0]
	require.Equal(t, planned[0].Title, shown.Title)
	require.NotNil(t, shown.UntilAt, "the idle preview counts down")

	require.NoError(t, d.Act("preview", "break"), "a preview's button only closes it")
	require.Empty(t, d.Nudges(true))
	select {
	case a := <-d.Actions():
		t.Fatalf("a preview acted: %v", a)
	default:
	}
	d.Wait()
	require.Empty(t, desktop.kinds(), "the card showed it")
	require.Empty(t, phone.kinds(), "never the phone")
}
