package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/activity"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestSocketIsPrivate(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	require.Equal(t, 0o600, int(fileMode(t, d.socket)))
	require.Equal(t, 0o700, int(fileMode(t, filepath.Dir(d.socket))))
}

func TestStaleSocketIsReplaced(t *testing.T) {
	t.Parallel()
	socket := testutil.SocketPath(t)
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, ln.Close()) // the file stays, nothing answers

	d := startDaemon(t, setup{socket: socket})
	_, err = d.c.Health(d.ctx)
	require.NoError(t, err)
}

func TestSecondDaemonExits(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	err := run(context.Background(), options{
		dataDir: t.TempDir(), configPath: filepath.Join(t.TempDir(), "c.toml"), socketPath: d.socket,
		clk: clock.NewFake(t0), loc: time.UTC, level: new(slog.LevelVar),
		monitor:  func(context.Context, clock.Clock) activity.ActivityMonitor { return &fakeMonitor{name: "none"} },
		notifier: func(config.Config, string) (notifier, []string) { return &fakeNotifier{}, nil },
	})
	require.ErrorContains(t, err, "already running")
	_, err = d.c.Health(d.ctx)
	require.NoError(t, err, "the first daemon is untouched")
}

func TestShutdownWritesShutdownAt(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	d.clk.Advance(90 * time.Second)
	require.NoError(t, d.stop())

	db, err := store.Open(context.Background(), d.dataDir, clock.NewFake(t0))
	require.NoError(t, err)
	defer db.Close()
	at, err := store.GetLocalTime(context.Background(), db.SQL(), store.KeyShutdownAt)
	require.NoError(t, err)
	require.Equal(t, t0.Add(90*time.Second), at.UTC())
}

func TestHeartbeatEvery15s(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	db := d.loop.db
	heartbeat := func() time.Time {
		at, err := store.GetLocalTime(context.Background(), db.SQL(), store.KeyHeartbeatAt)
		if errors.Is(err, store.ErrNotFound) {
			return time.Time{}
		}
		require.NoError(t, err)
		return at.UTC()
	}
	require.True(t, heartbeat().IsZero())
	for i := 1; i <= 3; i++ {
		d.clk.BlockUntil(1)
		d.clk.Advance(15 * time.Second)
		want := t0.Add(time.Duration(i) * 15 * time.Second)
		require.Eventually(t, func() bool { return heartbeat().Equal(want) }, 5*time.Second, time.Millisecond)
	}
}

// seed writes a database as a crashed daemon left it: an open work day with
// an open work segment and a heartbeat 30 minutes before t0.
func seed(t *testing.T, dataDir string) {
	t.Helper()
	ctx := context.Background()
	clk := clock.NewFake(t0.Add(-time.Hour))
	db, err := store.Open(ctx, dataDir, clk)
	require.NoError(t, err)
	defer db.Close()
	r := store.NewRepos(db)
	require.NoError(t, db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := r.WorkDays.StartDay(ctx, tx, testutil.Day0, "UTC", 8*3600, t0.Add(-time.Hour)); err != nil {
			return err
		}
		_, err := r.Segments.OpenSegment(ctx, tx, model.KindWork, model.SourceUser, nil, nil, t0.Add(-time.Hour))
		return err
	}))
	require.NoError(t, store.SetLocalTime(ctx, db.SQL(), store.KeyHeartbeatAt, t0.Add(-30*time.Minute), t0))
	require.NoError(t, store.SetLocalTime(ctx, db.SQL(), store.KeyShutdownAt, t0.Add(-45*time.Minute), t0))
	require.NoError(t, db.InTx(ctx, func(tx *sql.Tx) error {
		return r.Events.Insert(ctx, tx, model.EngineEvent{At: t0.AddDate(0, 0, -100), Trigger: "clock_in", FromState: "off", ToState: "working"})
	}))
}

func TestRecoveryCommitsBeforeTheSocketAccepts(t *testing.T) {
	t.Parallel()
	dataDir := filepath.Join(t.TempDir(), "data")
	seed(t, dataDir)
	d := startDaemon(t, setup{dataDir: dataDir})

	// The first request after the socket is up already sees the recovery.
	st := d.status()
	require.Equal(t, wire.StateBreakAuto, st.State)
	require.Equal(t, "recovery", st.OpenSegment.Source)
	require.Equal(t, wire.Millis(t0.Add(-45*time.Minute)), st.OpenSegment.StartedAt, "shutdown_at wins over the heartbeat")
	day, err := d.c.GetDay(d.ctx, testutil.Day0)
	require.NoError(t, err)
	require.False(t, day.Segments[0].Truncated, "a clean shutdown is not truncated")
	require.Equal(t, wire.Millis(t0.Add(-45*time.Minute)), *day.Segments[0].EndedAt)

	_, err = store.GetLocal(context.Background(), d.loop.db.SQL(), store.KeyShutdownAt)
	require.ErrorIs(t, err, store.ErrNotFound, "shutdown_at is cleared at startup")
}

func TestOldEngineEventsArePruned(t *testing.T) {
	t.Parallel()
	dataDir := filepath.Join(t.TempDir(), "data")
	seed(t, dataDir)
	d := startDaemon(t, setup{dataDir: dataDir})
	events, err := d.loop.repos.Events.List(context.Background(), time.Time{})
	require.NoError(t, err)
	for _, e := range events {
		require.True(t, e.At.After(t0.AddDate(0, 0, -90)), "a row older than 90 days survived")
	}
	require.NotEmpty(t, events, "recovery recorded its transition")
}

// failingDays makes the engine's StartDay effect fail inside the transaction.
type failingDays struct{ store.WorkDayRepo }

func (failingDays) StartDay(context.Context, *sql.Tx, string, string, int, time.Time) (model.WorkDay, error) {
	return model.WorkDay{}, store.ErrConflict
}

func TestFailedCommitLeavesTheEngineUnchanged(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{wrapRepos: func(r store.Repos) store.Repos {
		r.WorkDays = failingDays{r.WorkDays}
		return r
	}})
	_, err := d.c.ClockIn(d.ctx, wire.ClockInRequest{})
	ae := apiErr(t, err, wire.CodeInternal)
	require.Equal(t, "internal error", ae.Message)
	require.Equal(t, wire.StateOff, d.status().State)
}

func TestUnavailableIdleDetectionIsAWarning(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{monName: activity.NameNone})
	st := d.status()
	require.Equal(t, "none", st.ActivityBackend)
	require.Len(t, st.Warnings, 1)
}

// The real dispatcher is what the loop drives.
var _ notifier = (*notify.Dispatcher)(nil)

func TestMainFlags(t *testing.T) {
	t.Parallel()
	require.Equal(t, 2, mainErr([]string{"--nope"}, io.Discard))
	h := newLogHandler(filepath.Join(t.TempDir(), "state", "gwend.log"), true, io.Discard, new(slog.LevelVar))
	require.NotNil(t, h)
	require.NotNil(t, localZone())
}
