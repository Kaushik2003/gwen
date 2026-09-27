package store_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// One test per invariant of docs/03-data-model.md#invariants.

func TestInvariant1OneOpenWorkDay(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.startDay("09:00")
	err := f.tx(func(tx *sql.Tx) error {
		_, err := f.r.WorkDays.StartDay(f.ctx, tx, "2026-09-16", "UTC", 0, testutil.AtOn("2026-09-16", "09:00"))
		return err
	})
	require.ErrorIs(t, err, store.ErrConflict)

	// Starting the day that is already open is a no-op.
	var w model.WorkDay
	require.NoError(t, f.tx(func(tx *sql.Tx) error {
		var err error
		w, err = f.r.WorkDays.StartDay(f.ctx, tx, testutil.Day0, "UTC", 0, at("10:00"))
		return err
	}))
	require.Equal(t, at("09:00"), w.ClockedInAt)
}

func TestInvariant2OneOpenSegmentOnTheOpenDay(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	err := f.tx(func(tx *sql.Tx) error {
		_, err := f.r.Segments.OpenSegment(f.ctx, tx, model.KindWork, model.SourceUser, nil, nil, at("09:00"))
		return err
	})
	require.ErrorIs(t, err, store.ErrConflict, "a segment needs an open work day")

	f.startDay("09:00")
	f.open(model.KindWork, model.SourceUser, nil, "09:00")
	err = f.tx(func(tx *sql.Tx) error {
		_, err := f.r.Segments.OpenSegment(f.ctx, tx, model.KindBreakManual, model.SourceUser, nil, nil, at("09:30"))
		return err
	})
	require.ErrorIs(t, err, store.ErrConflict)

	err = f.tx(func(tx *sql.Tx) error {
		_, err := f.r.Segments.CloseSegment(f.ctx, tx, at("10:00"), false)
		if err != nil {
			return err
		}
		_, err = f.r.Segments.CloseSegment(f.ctx, tx, at("10:00"), false)
		return err
	})
	require.ErrorIs(t, err, store.ErrConflict, "closing needs an open segment")
}

func TestInvariant3ClosedDayHasNoOpenSegment(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	err := f.tx(func(tx *sql.Tx) error {
		_, err := f.r.WorkDays.CloseDay(f.ctx, tx, at("10:00"))
		return err
	})
	require.ErrorIs(t, err, store.ErrConflict, "closing needs an open day")

	f.startDay("09:00")
	f.open(model.KindWork, model.SourceUser, nil, "09:00")
	err = f.tx(func(tx *sql.Tx) error {
		_, err := f.r.WorkDays.CloseDay(f.ctx, tx, at("10:00"))
		return err
	})
	require.ErrorIs(t, err, store.ErrConflict)

	f.close("10:00")
	w := f.closeDay("09:30")
	require.Equal(t, at("10:00"), *w.ClockedOutAt, "the day closes no earlier than its last segment")
}

func TestInvariant4NoOverlap(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	day := "2026-09-14"
	on := func(hms string) time.Time { return testutil.AtOn(day, hms) }
	create := func(start, end string) (model.Segment, error) {
		return f.r.Segments.Create(f.ctx, store.NewSegment{Day: day, Kind: model.KindWork,
			StartedAt: on(start), EndedAt: tp(on(end))}, store.DayDefaults{TZ: "UTC", TargetSeconds: 3600})
	}
	a, err := create("09:00", "10:00")
	require.NoError(t, err)
	_, err = create("10:00", "11:00")
	require.NoError(t, err, "touching is allowed")
	_, err = create("09:30", "09:45")
	userErr(t, err, store.ErrConflict, "started_at")
	_, err = create("08:00", "12:00")
	userErr(t, err, store.ErrConflict, "started_at")

	_, err = f.r.Segments.Update(f.ctx, a.ID, store.SegmentPatch{EndedAt: tp(on("10:30"))})
	userErr(t, err, store.ErrConflict, "started_at")
	_, err = f.r.Segments.Update(f.ctx, a.ID, store.SegmentPatch{StartedAt: tp(on("08:00"))})
	require.NoError(t, err, "an update is not an overlap with itself")
}

func TestInvariant4OpenSegmentRunsOn(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.startDay("09:00")
	f.open(model.KindWork, model.SourceUser, nil, "09:00")
	_, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
		StartedAt: at("20:00"), EndedAt: tp(at("21:00"))}, store.DayDefaults{TZ: "UTC"})
	userErr(t, err, store.ErrConflict, "started_at")
	_, err = f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
		StartedAt: at("08:00"), EndedAt: tp(at("09:00"))}, store.DayDefaults{TZ: "UTC"})
	require.NoError(t, err, "before the open segment is fine")
}

func TestInvariant5EditsWidenTheDay(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.startDay("09:00")
	f.open(model.KindWork, model.SourceUser, nil, "09:00")
	f.close("12:00")
	closed := f.closeDay("12:00")

	g, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
		StartedAt: at("07:00"), EndedAt: tp(at("08:00"))}, store.DayDefaults{TZ: "UTC"})
	require.NoError(t, err)
	w, err := f.r.WorkDays.GetByDay(f.ctx, testutil.Day0)
	require.NoError(t, err)
	require.Equal(t, at("07:00"), w.ClockedInAt)
	require.Equal(t, closed.Rev+1, w.Rev)

	_, err = f.r.Segments.Update(f.ctx, g.ID, store.SegmentPatch{StartedAt: tp(at("13:00")), EndedAt: tp(at("14:00"))})
	require.NoError(t, err)
	w, err = f.r.WorkDays.GetByDay(f.ctx, testutil.Day0)
	require.NoError(t, err)
	require.Equal(t, at("14:00"), *w.ClockedOutAt)
}

func TestInvariant6BreaksHaveNoProject(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	_, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindBreakManual,
		ProjectID: &p.ID, StartedAt: at("09:00"), EndedAt: tp(at("10:00"))}, store.DayDefaults{TZ: "UTC"})
	userErr(t, err, store.ErrInvalid, "project_id")

	tk := f.task("t", nil)
	_, err = f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindBreakManual,
		TaskID: &tk.ID, StartedAt: at("09:00"), EndedAt: tp(at("10:00"))}, store.DayDefaults{TZ: "UTC"})
	userErr(t, err, store.ErrInvalid, "task_id")

	g, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
		ProjectID: &p.ID, StartedAt: at("09:00"), EndedAt: tp(at("10:00"))}, store.DayDefaults{TZ: "UTC"})
	require.NoError(t, err)
	brk, err := f.r.Segments.Update(f.ctx, g.ID, store.SegmentPatch{Kind: testutil.Ptr(model.KindBreakManual)})
	require.NoError(t, err)
	require.Nil(t, brk.ProjectID, "changing kind to a break clears the project")
	require.Nil(t, brk.TaskID)
}

func TestInvariant7ShortSegments(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.startDay("09:00")
	f.open(model.KindWork, model.SourceUser, nil, "09:00")
	g := f.close("09:00:00.999")
	require.NotNil(t, g.DeletedAt, "closing under 1000 ms tombstones")
	_, err := f.r.Segments.Get(f.ctx, g.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	f.open(model.KindWork, model.SourceUser, nil, "10:00")
	kept := f.close("10:00:01")
	require.Nil(t, kept.DeletedAt, "exactly 1000 ms is kept")

	_, err = f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
		StartedAt: at("11:00"), EndedAt: tp(at("11:00:00.500"))}, store.DayDefaults{TZ: "UTC"})
	userErr(t, err, store.ErrInvalid, "ended_at")
	_, err = f.r.Segments.Update(f.ctx, kept.ID, store.SegmentPatch{EndedAt: tp(at("10:00:00.200"))})
	userErr(t, err, store.ErrInvalid, "ended_at")
	_, err = f.r.Segments.Split(f.ctx, kept.ID, at("10:00:00.500"))
	userErr(t, err, store.ErrInvalid, "at")
}

func TestInvariant8TaskMatchesProject(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p, q := f.project("P"), f.project("Q")
	ofP := f.task("of P", &p.ID)
	loose := f.task("no project", nil)
	mk := func(project, task *string, start string) (model.Segment, error) {
		return f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
			ProjectID: project, TaskID: task, StartedAt: at(start), EndedAt: tp(at(start).Add(time.Hour))}, store.DayDefaults{TZ: "UTC"})
	}
	_, err := mk(&q.ID, &ofP.ID, "09:00")
	userErr(t, err, store.ErrConflict, "task_id")

	g, err := mk(&q.ID, &loose.ID, "10:00")
	require.NoError(t, err, "a task without a project goes with any project")
	require.Equal(t, q.ID, *g.ProjectID)

	g, err = mk(nil, &ofP.ID, "11:00")
	require.NoError(t, err)
	require.Equal(t, p.ID, *g.ProjectID, "a task alone brings its project")

	_, err = mk(nil, testutil.Ptr("nope"), "12:00")
	userErr(t, err, store.ErrNotFound, "task_id")
	_, err = mk(testutil.Ptr("nope"), nil, "12:00")
	userErr(t, err, store.ErrNotFound, "project_id")
}

func TestInvariant9GapsAreAllowed(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	for _, start := range []string{"09:00", "11:00"} {
		_, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork,
			StartedAt: at(start), EndedAt: tp(at(start).Add(time.Hour))}, store.DayDefaults{TZ: "UTC"})
		require.NoError(t, err)
	}
}

func TestCreateSegmentMakesAClosedWorkDay(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	g, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: "2026-09-14", Kind: model.KindWork,
		StartedAt: testutil.AtOn("2026-09-14", "09:00"), EndedAt: tp(testutil.AtOn("2026-09-14", "10:00"))},
		store.DayDefaults{TZ: "Asia/Kolkata", TargetSeconds: 7200})
	require.NoError(t, err)
	require.Equal(t, model.SourceEdit, g.Source)
	w, err := f.r.WorkDays.GetByDay(f.ctx, "2026-09-14")
	require.NoError(t, err)
	require.Equal(t, g.WorkDayID, w.ID)
	require.Equal(t, "Asia/Kolkata", w.TZ)
	require.Equal(t, 2*time.Hour, w.Target)
	require.Equal(t, g.StartedAt, w.ClockedInAt)
	require.Equal(t, *g.EndedAt, *w.ClockedOutAt)

	tests := []struct {
		name  string
		seg   store.NewSegment
		field string
	}{
		{"bad day", store.NewSegment{Day: "yesterday", Kind: model.KindWork, StartedAt: at("09:00"), EndedAt: tp(at("10:00"))}, "day"},
		{"no end", store.NewSegment{Day: testutil.Day0, Kind: model.KindWork, StartedAt: at("09:00")}, "ended_at"},
		{"bad kind", store.NewSegment{Day: testutil.Day0, Kind: "nap", StartedAt: at("09:00"), EndedAt: tp(at("10:00"))}, "kind"},
		{"ends before it starts", store.NewSegment{Day: testutil.Day0, Kind: model.KindWork, StartedAt: at("10:00"), EndedAt: tp(at("09:00"))}, "ended_at"},
		{"in the future", store.NewSegment{Day: "2026-09-20", Kind: model.KindWork, StartedAt: fxNow, EndedAt: tp(fxNow.Add(time.Hour))}, "ended_at"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.r.Segments.Create(f.ctx, tc.seg, store.DayDefaults{TZ: "UTC"})
			userErr(t, err, store.ErrInvalid, tc.field)
		})
	}
}

func TestSegmentUpdateSplitDelete(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	f.startDay("09:00")
	open := f.open(model.KindWork, model.SourceUser, &p.ID, "09:00")
	closed := f.close("12:00")
	require.Equal(t, open.ID, closed.ID)
	require.Equal(t, int64(2), closed.Rev)

	f.clk.Advance(time.Minute)
	halves, err := f.r.Segments.Split(f.ctx, closed.ID, at("10:30"))
	require.NoError(t, err)
	first, second := halves[0], halves[1]
	require.Equal(t, closed.ID, first.ID, "the first half keeps the id")
	require.Equal(t, at("10:30"), *first.EndedAt)
	require.Equal(t, int64(3), first.Rev)
	require.Equal(t, at("10:30"), second.StartedAt)
	require.Equal(t, at("12:00"), *second.EndedAt)
	require.Equal(t, model.SourceEdit, second.Source)
	require.Equal(t, p.ID, *second.ProjectID)
	require.Equal(t, int64(1), second.Rev)

	list, err := f.r.Segments.ListByWorkDay(f.ctx, first.WorkDayID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, first.ID, list[0].ID)

	_, err = f.r.Segments.Split(f.ctx, first.ID, at("08:00"))
	userErr(t, err, store.ErrInvalid, "at")
	_, err = f.r.Segments.Update(f.ctx, first.ID, store.SegmentPatch{Rev: testutil.Ptr(int64(1))})
	userErr(t, err, store.ErrConflict, "rev")

	moved, err := f.r.Segments.Update(f.ctx, first.ID, store.SegmentPatch{
		ProjectID: store.Nullable[string]{Set: true}, StartedAt: tp(at("09:15")), Rev: &first.Rev})
	require.NoError(t, err)
	require.Nil(t, moved.ProjectID)
	require.Equal(t, at("09:15"), moved.StartedAt)
	require.Equal(t, first.Rev+1, moved.Rev)
	require.Equal(t, f.clk.Now(), moved.UpdatedAt)

	require.NoError(t, f.r.Segments.Delete(f.ctx, second.ID))
	_, err = f.r.Segments.Get(f.ctx, second.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, f.r.Segments.Delete(f.ctx, second.ID), store.ErrNotFound)
}

func TestHandEditsRefuseTheOpenSegment(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.startDay("09:00")
	g := f.open(model.KindWork, model.SourceUser, nil, "09:00")
	_, err := f.r.Segments.Update(f.ctx, g.ID, store.SegmentPatch{})
	require.ErrorIs(t, err, store.ErrConflict)
	_, err = f.r.Segments.Split(f.ctx, g.ID, at("10:00"))
	require.ErrorIs(t, err, store.ErrConflict)
	require.ErrorIs(t, f.r.Segments.Delete(f.ctx, g.ID), store.ErrConflict)
}

func TestEngineEffectsAndReads(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	cur, err := f.r.WorkDays.Current(f.ctx)
	require.NoError(t, err)
	require.Nil(t, cur)
	none, err := f.r.Segments.Current(f.ctx)
	require.NoError(t, err)
	require.Nil(t, none)
	last, err := f.r.WorkDays.LatestClosedDay(f.ctx)
	require.NoError(t, err)
	require.Empty(t, last)

	w := f.startDay("09:00")
	require.Equal(t, "UTC", w.TZ)
	require.Equal(t, 8*time.Hour, w.Target)
	f.open(model.KindWork, model.SourceUser, &p.ID, "09:00")
	f.close("10:00")
	f.open(model.KindBreakAuto, model.SourceIdle, nil, "10:00")
	cur, err = f.r.WorkDays.Current(f.ctx)
	require.NoError(t, err)
	require.Equal(t, w.ID, cur.ID)
	seg, err := f.r.Segments.Current(f.ctx)
	require.NoError(t, err)
	require.Equal(t, model.KindBreakAuto, seg.Kind)
	end, err := f.r.Segments.LatestEnd(f.ctx, w.ID)
	require.NoError(t, err)
	require.Equal(t, at("10:00"), *end)
	work, err := f.r.Segments.LatestWork(f.ctx)
	require.NoError(t, err)
	require.Equal(t, p.ID, *work.ProjectID)

	require.NoError(t, f.tx(func(tx *sql.Tx) error {
		_, err := f.r.Segments.CloseSegment(f.ctx, tx, at("10:30"), true)
		return err
	}))
	closed := f.closeDay("10:30")
	require.Equal(t, at("10:30"), *closed.ClockedOutAt)
	last, err = f.r.WorkDays.LatestClosedDay(f.ctx)
	require.NoError(t, err)
	require.Equal(t, testutil.Day0, last)

	list, err := f.r.Segments.ListByWorkDay(f.ctx, w.ID)
	require.NoError(t, err)
	require.True(t, list[1].Truncated)

	// Clocking back in reopens the same work day; the time between is a gap.
	reopened := f.startDay("13:00")
	require.Equal(t, w.ID, reopened.ID)
	require.Nil(t, reopened.ClockedOutAt)
	require.Equal(t, at("09:00"), reopened.ClockedInAt)

	byID, err := f.r.WorkDays.Get(f.ctx, w.ID)
	require.NoError(t, err)
	require.Equal(t, reopened, byID)
	_, err = f.r.WorkDays.Get(f.ctx, "nope")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestWorkDayUpdate(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	w := f.startDay("09:00")
	up, err := f.r.WorkDays.Update(f.ctx, testutil.Day0, store.WorkDayPatch{TargetSeconds: testutil.Ptr(3600),
		Note: testutil.Ptr("sick"), Rev: &w.Rev})
	require.NoError(t, err)
	require.Equal(t, time.Hour, up.Target)
	require.Equal(t, "sick", up.Note)
	require.Equal(t, w.Rev+1, up.Rev)

	_, err = f.r.WorkDays.Update(f.ctx, testutil.Day0, store.WorkDayPatch{Rev: &w.Rev})
	userErr(t, err, store.ErrConflict, "rev")
	_, err = f.r.WorkDays.Update(f.ctx, testutil.Day0, store.WorkDayPatch{TargetSeconds: testutil.Ptr(-1)})
	userErr(t, err, store.ErrInvalid, "target_seconds")
	_, err = f.r.WorkDays.Update(f.ctx, "2026-01-01", store.WorkDayPatch{})
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.r.WorkDays.GetByDay(f.ctx, "someday")
	userErr(t, err, store.ErrInvalid, "day")
}
