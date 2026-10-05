package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// fx is a store over a fresh database whose clock reads 2026-09-20 12:00 UTC,
// after every instant the tests write, so hand edits are never in the future.
type fx struct {
	t   *testing.T
	ctx context.Context
	clk *clock.Fake
	db  *store.DB
	r   store.Repos
}

var fxNow = testutil.AtOn("2026-09-20", "12:00")

func newFx(t *testing.T) *fx {
	t.Helper()
	clk := clock.NewFake(fxNow)
	db := testutil.NewDBWithClock(t, clk)
	return &fx{t: t, ctx: context.Background(), clk: clk, db: db, r: store.NewRepos(db)}
}

func at(hms string) time.Time { return testutil.At(hms) }

func tp(t time.Time) *time.Time { return &t }

func (f *fx) tx(fn func(tx *sql.Tx) error) error { return f.db.InTx(f.ctx, fn) }

// startDay opens a work day for Day0 at the given time.
func (f *fx) startDay(hms string) model.WorkDay {
	f.t.Helper()
	var w model.WorkDay
	require.NoError(f.t, f.tx(func(tx *sql.Tx) error {
		var err error
		w, err = f.r.WorkDays.StartDay(f.ctx, tx, testutil.Day0, "UTC", 8*3600, at(hms))
		return err
	}))
	return w
}

func (f *fx) open(kind, source string, project *string, hms string) model.Segment {
	f.t.Helper()
	var g model.Segment
	require.NoError(f.t, f.tx(func(tx *sql.Tx) error {
		var err error
		g, err = f.r.Segments.OpenSegment(f.ctx, tx, kind, source, project, nil, at(hms))
		return err
	}))
	return g
}

func (f *fx) close(hms string) model.Segment {
	f.t.Helper()
	var g model.Segment
	require.NoError(f.t, f.tx(func(tx *sql.Tx) error {
		var err error
		g, err = f.r.Segments.CloseSegment(f.ctx, tx, at(hms), false)
		return err
	}))
	return g
}

func (f *fx) closeDay(hms string) model.WorkDay {
	f.t.Helper()
	var w model.WorkDay
	require.NoError(f.t, f.tx(func(tx *sql.Tx) error {
		var err error
		w, err = f.r.WorkDays.CloseDay(f.ctx, tx, at(hms))
		return err
	}))
	return w
}

func (f *fx) project(name string) model.Project {
	f.t.Helper()
	p, err := f.r.Projects.Create(f.ctx, name, nil)
	require.NoError(f.t, err)
	return p
}

func (f *fx) task(title string, project *string) model.Task {
	f.t.Helper()
	tk, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: title, ProjectID: project, EstimateMinutes: testutil.Ptr(60)})
	require.NoError(f.t, err)
	return tk
}

// userErr asserts err wraps sentinel as a *store.Error for field.
func userErr(t *testing.T, err error, sentinel error, field string) {
	t.Helper()
	require.ErrorIs(t, err, sentinel)
	var se *store.Error
	require.ErrorAs(t, err, &se)
	require.Equal(t, field, se.Field)
	require.NotEmpty(t, se.Message)
}
