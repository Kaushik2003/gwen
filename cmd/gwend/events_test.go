package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// stream opens GET /v1/events and returns a function reading the next event
// with a given name, skipping others.
func stream(t *testing.T, d *daemon) func(name string) wire.Event {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := d.c.Events(ctx)
	require.NoError(t, err)
	events := make(chan wire.Event, 64)
	go func() {
		defer close(events)
		for {
			ev, err := s.Next()
			if err != nil {
				return
			}
			events <- ev
		}
	}()
	return func(name string) wire.Event {
		t.Helper()
		for {
			select {
			case ev, ok := <-events:
				require.True(t, ok, "stream ended waiting for %s", name)
				if ev.Name == name {
					return ev
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("no %s event", name)
			}
		}
	}
}

func TestEventStream(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	s, err := d.c.Events(d.ctx)
	require.NoError(t, err)
	first, err := s.Next()
	require.NoError(t, err)
	require.Equal(t, wire.EventStateChanged, first.Name, "the first frame is state_changed")
	var st wire.Status
	require.NoError(t, json.Unmarshal(first.Data, &st))
	require.Equal(t, wire.StateOff, st.State)
	s.Close()

	next := stream(t, d)
	next(wire.EventStateChanged)
	p := d.project("P")
	next(wire.EventProjectsChanged)

	_, err = d.c.ClockIn(d.ctx, wire.ClockInRequest{ProjectID: &p.ID})
	require.NoError(t, err)
	require.JSONEq(t, `{"day":"2026-09-15"}`, string(next(wire.EventDayChanged).Data))
	var working wire.Status
	require.NoError(t, json.Unmarshal(next(wire.EventStateChanged).Data, &working))
	require.Equal(t, wire.StateWorking, working.State)

	_, err = d.c.CreateSegment(d.ctx, wire.CreateSegmentRequest{Day: "2026-09-14", Kind: "work",
		StartedAt: wire.Millis(testutil.AtOn("2026-09-14", "09:00")), EndedAt: testutil.Ptr(wire.Millis(testutil.AtOn("2026-09-14", "10:00")))})
	require.NoError(t, err)
	require.JSONEq(t, `{"day":"2026-09-14"}`, string(next(wire.EventDayChanged).Data), "editing a segment emits day_changed")

	_, err = d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "T"})
	require.NoError(t, err)
	var tc wire.TasksChanged
	require.NoError(t, json.Unmarshal(next(wire.EventTasksChanged).Data, &tc))
	require.Len(t, tc.TaskIDs, 1)

	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"nudge": {"snooze": "5m"}})
	require.NoError(t, err)
	var cfg wire.Config
	require.NoError(t, json.Unmarshal(next(wire.EventConfigChanged).Data, &cfg))
	require.Equal(t, "5m", cfg.Nudge.Snooze)
}

func TestPings(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	s, err := d.c.Events(d.ctx)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.Next()
	require.NoError(t, err)
	// A ping is a comment the reader skips; the stream stays open across it.
	d.clk.BlockUntil(2) // off: the heartbeat and the stream's ping; no engine deadline
	d.clk.Advance(25 * time.Second)
	p := d.project("after ping")
	ev, err := s.Next()
	require.NoError(t, err)
	require.Equal(t, wire.EventProjectsChanged, ev.Name)
	_ = p
}

func TestActivityDrivesTheEngine(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	next := stream(t, d)
	_, err := d.c.ClockIn(d.ctx, wire.ClockInRequest{})
	require.NoError(t, err)

	d.clk.Advance(3 * time.Minute)
	d.mon.emit(activity.Event{Kind: activity.Idle, Threshold: 3 * time.Minute, At: d.clk.Now()})
	d.sync()
	require.Equal(t, wire.StateIdlePending, d.status().State)
	require.Equal(t, []string{"idle"}, d.notif.kinds())
	var fired wire.NudgeFired
	require.NoError(t, json.Unmarshal(next(wire.EventNudgeFired).Data, &fired))
	require.Equal(t, "Still there?", fired.Title)

	// The hard deadline fires from the loop's own timer.
	d.clk.BlockUntil(2)
	d.clk.Advance(7 * time.Minute)
	require.Eventually(t, func() bool { return d.status().State == wire.StateBreakAuto }, 5*time.Second, time.Millisecond)
	st := d.status()
	require.Equal(t, "idle", st.OpenSegment.Source)
	require.Equal(t, wire.Millis(t0), st.OpenSegment.StartedAt, "reclaimed back to the last input")

	// A notification action is an input.
	d.notif.actions <- notify.ActionEvent{Kind: "break_long", Action: "end_break"}
	d.sync()
	require.Equal(t, wire.StateWorking, d.status().State)

	// Suspend is acknowledged only after it is committed.
	acked := make(chan wire.Status, 1)
	d.mon.emit(activity.Event{Kind: activity.Suspend, At: d.clk.Now(), Ack: func() {
		st, _ := d.loop.Status(context.Background())
		acked <- st
	}})
	require.Equal(t, wire.StateBreakAuto, (<-acked).State)
	d.mon.emit(activity.Event{Kind: activity.Resume, At: d.clk.Now()})
	d.mon.emit(activity.Event{Kind: activity.Locked, At: d.clk.Now()})
	d.mon.emit(activity.Event{Kind: activity.Unlocked, At: d.clk.Now()})
	d.mon.emit(activity.Event{Kind: activity.Active, At: d.clk.Now()})
	d.sync()
	require.Equal(t, wire.StateWorking, d.status().State)

	// An action the state refuses is dropped quietly.
	d.notif.actions <- notify.ActionEvent{Kind: "clock_in", Action: "clock_in"}
	d.notif.actions <- notify.ActionEvent{Kind: "x", Action: "unknown"}
	d.sync()
	require.Equal(t, wire.StateWorking, d.status().State)
	require.Contains(t, d.notif.withdrawn, "idle")
}

func TestFollowerReconnectsToADaemon(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	got := make(chan string, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Follower{API: d.c, Clock: d.clk, OnEvent: func(ev wire.Event) { got <- ev.Name }}.Run(ctx)
	require.Equal(t, wire.EventStateChanged, <-got)
}
