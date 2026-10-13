package service

import (
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestDataLane summarises tracked data in words: time worked, tasks done,
// goals, and energy, for the window a query names or the last 7 days.
func TestDataLane(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	d := f.s.data
	hits, err := d.Search(f.ctx, memory.Query{Text: "I work at Acme"}, nil)
	require.NoError(t, err)
	require.Empty(t, hits, "a message that asks about no data gets none")
	hits, err = d.Search(f.ctx, memory.Query{Text: "How many hours did I put in, and which tasks did I finish?"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"data:worked", "data:tasks_done"}, hitRefs(hits))
	require.Equal(t, "From 25 Sep to 1 Oct they tracked no work.", hits[0].Text)
	require.Equal(t, "From 25 Sep to 1 Oct they finished no tasks.", hits[1].Text)
	require.Equal(t, "data", hits[0].Kind)
	require.Equal(t, model.TrustObserved, hits[0].Trust)

	task, _, err := f.r.Tasks.Create(f.ctx, store.NewTask{Title: "Essay"})
	require.NoError(t, err)
	_, err = f.r.Tasks.Complete(f.ctx, task.ID, nil)
	require.NoError(t, err)
	_, err = f.r.Energy.Log(f.ctx, f.clk.Now().Add(-time.Hour), 2)
	require.NoError(t, err)
	_, err = f.r.Energy.Log(f.ctx, f.clk.Now().Add(-25*time.Hour), 5)
	require.NoError(t, err)
	_, _, err = f.r.Goals.Create(f.ctx, store.NewGoal{Title: "Thesis", Kind: model.GoalTasks, StartDay: "2026-09-01",
		DueDay: "2026-12-01"})
	require.NoError(t, err)

	hits, err = d.Search(f.ctx, memory.Query{Intent: memory.IntentData}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"data:worked", "data:tasks_done", "data:goals", "data:energy"}, hitRefs(hits))
	require.Equal(t, "From 25 Sep to 1 Oct they finished 1 task: “Essay”.", hits[1].Text)
	require.True(t, strings.HasPrefix(hits[2].Text, "Their active goals: “Thesis”, "), hits[2].Text)
	require.Contains(t, hits[2].Text, "due Tue 1 Dec")
	require.Equal(t, "From 25 Sep to 1 Oct they checked in their energy 2 times, 3.5 of 5 on average (lowest 2, "+
		"highest 5).", hits[3].Text)

	// Today only.
	today := civil.MustParse("2026-10-01").Midnight(time.UTC)
	hits, err = d.Search(f.ctx, memory.Query{Text: "how was my energy today"},
		&memory.TimeRange{From: today, To: today.Add(24 * time.Hour)})
	require.NoError(t, err)
	require.Equal(t, []string{"data:energy"}, hitRefs(hits))
	require.Equal(t, "On Thu 1 Oct they checked in their energy 1 time, 2.0 of 5 on average (lowest 2, highest 2).",
		hits[0].Text)
	hits, err = d.Search(f.ctx, memory.Query{Text: "what did I get done, am I behind on my goals?"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"data:tasks_done", "data:goals"}, hitRefs(hits))

	sums, err := d.summaries(f.ctx)
	require.NoError(t, err)
	require.Len(t, sums, 4)
	require.True(t, strings.HasPrefix(sums[0], "From 4 Sep to 1 Oct "), sums[0])
}

// TestDataLaneWorked reads time worked against the target from the stats.
func TestDataLaneWorked(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	start := testutil.AtOn("2026-09-30", "09:00").UnixMilli()
	end := start + (7*time.Hour + 20*time.Minute).Milliseconds()
	_, err := f.db.SQL().ExecContext(f.ctx, `INSERT INTO work_days (id, day, tz, clocked_in_at, clocked_out_at,
		target_seconds, created_at, updated_at, device_id, rev)
		VALUES ('00000000-0000-4000-8000-0000000000d1', '2026-09-30', 'UTC', ?, ?, 28800, 0, 0, 'd', 1)`, start, end)
	require.NoError(t, err)
	_, err = f.db.SQL().ExecContext(f.ctx, `INSERT INTO segments (id, work_day_id, kind, source, started_at, ended_at,
		created_at, updated_at, device_id, rev) VALUES ('00000000-0000-4000-8000-0000000000e1',
		'00000000-0000-4000-8000-0000000000d1', 'work', 'user', ?, ?, 0, 0, 'd', 1)`, start, end)
	require.NoError(t, err)
	hits, err := f.s.data.Search(f.ctx, memory.Query{Text: "how long did I work this week?"}, nil)
	require.NoError(t, err)
	require.Equal(t, "From 25 Sep to 1 Oct they worked 7h 20m against a target of 8h, on 1 day; they met the day's "+
		"target on 0 of them.", hits[0].Text)
	require.Equal(t, "Time worked by day, 25 Sep to 1 Oct: Wed 30 Sep 7h 20m of 8h.", hits[1].Text)
}

func TestSpanWords(t *testing.T) {
	t.Parallel()
	d := civil.MustParse
	require.Equal(t, "on Thu 1 Oct", spanWords(d("2026-10-01"), d("2026-10-01")))
	require.Equal(t, "from 5 to 11 Oct", spanWords(d("2026-10-05"), d("2026-10-11")))
	require.Equal(t, "from 28 Sep to 4 Oct", spanWords(d("2026-09-28"), d("2026-10-04")))
	require.Equal(t, "from 28 Dec 2026 to 3 Jan 2027", spanWords(d("2026-12-28"), d("2027-01-03")))
	require.Equal(t, "7h 32m", duration(7*time.Hour+32*time.Minute))
	require.Equal(t, "8h", duration(8*time.Hour))
	require.Equal(t, "45m", duration(45*time.Minute))
}

func hitRefs(hits []memory.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Ref
	}
	return out
}
