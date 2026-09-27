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

// The fixture of docs/11-testing.md W3: work P 09:00–10:00, break
// 10:00–10:30, open work P from 10:30, now 11:00.
func TestDerivedTotalsFixture(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	w := f.startDay("09:00")
	f.open(model.KindWork, model.SourceUser, &p.ID, "09:00")
	f.close("10:00")
	f.open(model.KindBreakManual, model.SourceUser, nil, "10:00")
	f.close("10:30")
	f.open(model.KindWork, model.SourceUser, &p.ID, "10:30")

	s, err := f.r.Stats.DaySummary(f.ctx, w.ID, at("11:00"))
	require.NoError(t, err)
	require.Equal(t, int64(5_400_000), s.Worked.Milliseconds())
	require.Equal(t, int64(1_800_000), s.Break.Milliseconds())
	require.False(t, s.TargetMet)
	require.Equal(t, []store.ProjectTotal{{ProjectID: &p.ID, Name: "P", Color: p.Color, Worked: 90 * time.Minute}}, s.ByProject)
}

func TestByProjectResolvesDeletedAndUnassigned(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	gone := f.project("Gone")
	kept := f.project("Kept")
	add := func(project *string, start, end string) {
		_, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: testutil.Day0, Kind: model.KindWork, ProjectID: project,
			StartedAt: at(start), EndedAt: tp(at(end))}, store.DayDefaults{TZ: "UTC", TargetSeconds: 3 * 3600})
		require.NoError(t, err)
	}
	add(&gone.ID, "09:00", "09:30")
	add(&kept.ID, "10:00", "12:00")
	add(nil, "13:00", "14:00")
	require.NoError(t, f.r.Projects.Delete(f.ctx, gone.ID))

	days, err := f.r.Stats.Days(f.ctx, testutil.Day0, testutil.Day0, fxNow)
	require.NoError(t, err)
	require.Len(t, days, 1)
	d := days[0]
	require.True(t, d.TargetMet, "3.5 h worked against a 3 h target")
	require.Equal(t, []store.ProjectTotal{
		{ProjectID: &kept.ID, Name: "Kept", Color: kept.Color, Worked: 2 * time.Hour},
		{ProjectID: nil, Worked: time.Hour},
		{ProjectID: &gone.ID, Name: "Gone", Color: gone.Color, Worked: 30 * time.Minute},
	}, d.ByProject, "by worked time descending; deleted projects keep their names")
}

// workedDay records a closed work day with the given worked minutes against a
// one-hour target.
func (f *fx) workedDay(day string, minutes int) {
	f.t.Helper()
	start := testutil.AtOn(day, "09:00")
	_, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: day, Kind: model.KindWork, StartedAt: start,
		EndedAt: tp(start.Add(time.Duration(minutes) * time.Minute))}, store.DayDefaults{TZ: "UTC", TargetSeconds: 3600})
	require.NoError(f.t, err)
}

func TestStreaks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		days             map[string]int // day → worked minutes, target 60
		today            string
		current, longest int
	}{
		{"target met today", map[string]int{"2026-09-14": 60, "2026-09-15": 61}, "2026-09-15", 2, 2},
		{"met yesterday, not yet today", map[string]int{"2026-09-13": 60, "2026-09-14": 90, "2026-09-15": 10}, "2026-09-15", 2, 2},
		{"a day without a work day breaks the run", map[string]int{"2026-09-12": 60, "2026-09-14": 60}, "2026-09-15", 1, 1},
		{"longest across history", map[string]int{
			"2026-08-01": 60, "2026-08-02": 60, "2026-08-03": 60, "2026-08-04": 30,
			"2026-08-05": 60, "2026-09-14": 60,
		}, "2026-09-15", 1, 3},
		{"nothing", map[string]int{}, "2026-09-15", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFx(t)
			for day, minutes := range tc.days {
				f.workedDay(day, minutes)
			}
			s, err := f.r.Stats.Summary(f.ctx, "2026-09-01", "2026-09-15", tc.today, fxNow)
			require.NoError(t, err)
			require.Equal(t, tc.current, s.CurrentStreak, "current")
			require.Equal(t, tc.longest, s.LongestStreak, "longest")
		})
	}
}

func TestSummaryAndHeatmap(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	f.workedDay("2026-09-10", 30)
	f.workedDay("2026-09-11", 90)
	f.workedDay("2025-12-31", 45)
	_, err := f.r.Segments.Create(f.ctx, store.NewSegment{Day: "2026-09-11", Kind: model.KindWork, ProjectID: &p.ID,
		StartedAt: testutil.AtOn("2026-09-11", "12:00"), EndedAt: tp(testutil.AtOn("2026-09-11", "12:30"))}, store.DayDefaults{TZ: "UTC"})
	require.NoError(t, err)
	_, err = f.r.Segments.Create(f.ctx, store.NewSegment{Day: "2026-09-11", Kind: model.KindBreakManual,
		StartedAt: testutil.AtOn("2026-09-11", "12:30"), EndedAt: tp(testutil.AtOn("2026-09-11", "12:40"))}, store.DayDefaults{TZ: "UTC"})
	require.NoError(t, err)

	s, err := f.r.Stats.Summary(f.ctx, "2026-09-01", "2026-09-30", "2026-09-15", fxNow)
	require.NoError(t, err)
	require.Equal(t, 150*time.Minute, s.Worked)
	require.Equal(t, 10*time.Minute, s.Break)
	require.Equal(t, 2, s.DaysTracked)
	require.Equal(t, 1, s.DaysTargetMet)
	require.Equal(t, 75*time.Minute, s.AvgWorked)
	require.Equal(t, []store.ProjectTotal{
		{Worked: 120 * time.Minute},
		{ProjectID: &p.ID, Name: "P", Color: p.Color, Worked: 30 * time.Minute},
	}, s.ByProject)

	empty, err := f.r.Stats.Summary(f.ctx, "2026-01-01", "2026-01-31", "2026-09-15", fxNow)
	require.NoError(t, err)
	require.Zero(t, empty.AvgWorked)
	require.Equal(t, []store.ProjectTotal{}, empty.ByProject)

	heat, err := f.r.Stats.Heatmap(f.ctx, 2026, fxNow)
	require.NoError(t, err)
	require.Equal(t, []store.HeatmapDay{{Day: "2026-09-10", Worked: 30 * time.Minute}, {Day: "2026-09-11", Worked: 120 * time.Minute}}, heat)
	_, err = f.r.Stats.Heatmap(f.ctx, 0, fxNow)
	userErr(t, err, store.ErrInvalid, "year")

	days, err := f.r.Stats.Days(f.ctx, "2026-09-01", "2026-09-30", fxNow)
	require.NoError(t, err)
	require.Len(t, days, 2)
	require.Equal(t, "2026-09-10", days[0].Day)
	require.Equal(t, []store.ProjectTotal{{Worked: 30 * time.Minute}}, days[0].ByProject)
}

func TestHeatmapExcludesZeroDays(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.startDay("09:00")
	heat, err := f.r.Stats.Heatmap(f.ctx, 2026, at("09:00"))
	require.NoError(t, err)
	require.Empty(t, heat)
}

func TestRanges(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	tests := []struct {
		name, from, to, field string
	}{
		{"bad from", "2026-13-01", "2026-12-31", "from"},
		{"bad to", "2026-01-01", "tomorrow", "to"},
		{"backwards", "2026-02-01", "2026-01-01", "to"},
		{"too wide", "2025-01-01", "2026-01-03", "to"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.r.Stats.Days(f.ctx, tc.from, tc.to, fxNow)
			userErr(t, err, store.ErrInvalid, tc.field)
			_, err = f.r.Stats.Summary(f.ctx, tc.from, tc.to, "2026-09-15", fxNow)
			userErr(t, err, store.ErrInvalid, tc.field)
		})
	}
	_, err := f.r.Stats.Days(f.ctx, "2025-01-01", "2026-01-02", fxNow)
	require.NoError(t, err, "366 days apart is allowed")
}

func TestEngineEvents(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	old := testutil.AtOn("2026-06-01", "09:00")
	recent := testutil.AtOn("2026-09-19", "09:00")
	require.NoError(t, f.tx(func(tx *sql.Tx) error {
		if err := f.r.Events.Insert(f.ctx, tx, model.EngineEvent{At: old, Trigger: "clock_in", FromState: "off", ToState: "working"}); err != nil {
			return err
		}
		return f.r.Events.Insert(f.ctx, tx, model.EngineEvent{At: recent, Trigger: "idle", FromState: "working",
			ToState: "idle_pending", Data: map[string]any{"threshold_ms": 180000}})
	}))
	n, err := f.r.Events.Prune(f.ctx, fxNow.AddDate(0, 0, -90))
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	left, err := f.r.Events.List(f.ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, left, 1)
	require.Equal(t, "idle", left[0].Trigger)
	require.Equal(t, recent, left[0].At)
	require.Equal(t, map[string]any{"threshold_ms": float64(180000)}, left[0].Data)

	err = f.tx(func(tx *sql.Tx) error {
		return f.r.Events.Insert(f.ctx, tx, model.EngineEvent{At: recent, Data: map[string]any{"bad": func() {}}})
	})
	require.Error(t, err)
}
