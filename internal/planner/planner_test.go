package planner_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func mins(n int) time.Duration { return time.Duration(n) * time.Minute }

// task builds an open task; n orders created_at.
func task(id string, n int, opts ...func(*planner.Task)) planner.Task {
	t := planner.Task{Task: model.Task{ID: id, Title: id, Status: model.TaskOpen, Priority: 2,
		Envelope: model.Envelope{CreatedAt: t0.Add(time.Duration(n) * time.Second), Rev: 1}}}
	for _, o := range opts {
		o(&t)
	}
	return t
}

func dueOn(d string) func(*planner.Task)   { return func(t *planner.Task) { t.DueDay = &d } }
func prio(p int) func(*planner.Task)       { return func(t *planner.Task) { t.Priority = p } }
func estimate(m int) func(*planner.Task)   { return func(t *planner.Task) { t.Estimate = ptr(mins(m)) } }
func forGoal(g string) func(*planner.Task) { return func(t *planner.Task) { t.GoalID = &g } }
func done(q *int) func(*planner.Task) {
	return func(t *planner.Task) { t.Status, t.DoneAt, t.QuantityDone = model.TaskDone, &t0, q }
}

func quantityGoal() planner.Goal {
	return planner.Goal{ID: "g", Title: "Problems", Kind: model.GoalQuantity, TargetQuantity: ptr(300),
		PerUnit: ptr(mins(30)), StartDay: "2026-09-15", DueDay: "2026-12-15", Status: model.GoalActive}
}

func sessionTemplate(rule string) planner.Task {
	return task("tmpl", 0, forGoal("g"), func(t *planner.Task) {
		t.RRule, t.Anchor = &rule, day("2026-09-15")
	})
}

func TestProgressWorkedExample(t *testing.T) {
	t.Parallel()
	g := quantityGoal()
	tasks := []planner.Task{sessionTemplate("FREQ=DAILY")}
	p := planner.Progress(g, tasks, day("2026-09-15"))
	require.Equal(t, 92, p.SessionsLeft)
	q, ok := planner.SessionQuantity(g, p, day("2026-09-15"))
	require.True(t, ok)
	require.Equal(t, 4, q, "ceil(300/92)")
	require.Equal(t, 120*time.Minute, time.Duration(q)*(*g.PerUnit))

	// Solve 3 that day: the next session is ceil(297/91) = 4 again.
	tasks = append(tasks, task("s1", 1, forGoal("g"), done(ptr(3))))
	p = planner.Progress(g, tasks, day("2026-09-16"))
	require.Equal(t, 297, p.Remaining)
	require.Equal(t, 91, p.SessionsLeft)
	q, _ = planner.SessionQuantity(g, p, day("2026-09-16"))
	require.Equal(t, 4, q)
}

func TestProgressValues(t *testing.T) {
	t.Parallel()
	g := quantityGoal()
	g.StartDay, g.DueDay, g.TargetQuantity = "2026-09-01", "2026-09-10", ptr(100)
	tasks := []planner.Task{
		sessionTemplate("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"),
		task("a", 1, forGoal("g"), done(ptr(20))),
		task("b", 2, forGoal("g"), done(ptr(10))),
		task("open", 3, forGoal("g"), func(t *planner.Task) { t.Quantity = ptr(50) }),
		task("deleted", 4, forGoal("g"), done(ptr(40)), func(t *planner.Task) { t.DeletedAt = &t0 }),
		task("other goal", 5, forGoal("h"), done(ptr(40))),
	}
	tasks[0].Anchor = day("2026-09-01")
	// Today 2026-09-05 (Saturday): day 5 of 10. 30 done, expected 50.
	p := planner.Progress(g, tasks, day("2026-09-05"))
	require.Equal(t, 100, p.Target)
	require.Equal(t, 30, p.Done)
	require.Equal(t, 70, p.Remaining)
	require.Equal(t, 10, p.DaysTotal)
	require.Equal(t, 5, p.DaysElapsed)
	require.Equal(t, 4, p.SessionsLeft, "weekdays from Sat 5th to Thu 10th: 7, 8, 9, 10")
	require.InDelta(t, 17.5, p.RequiredPerDay, 1e-9)
	require.InDelta(t, 6.0, p.ActualPerDay, 1e-9)
	require.InDelta(t, 50.0, p.ExpectedByNow, 1e-9)
	require.Equal(t, planner.PaceBehind, p.Pace)
	require.Equal(t, "2026-09-16", p.ProjectedFinish.String(), "5th + ceil(70/6) - 1")

	// Past the due day: no sessions left, everything remaining is required.
	p = planner.Progress(g, tasks, day("2026-09-12"))
	require.Equal(t, 0, p.SessionsLeft)
	require.Equal(t, 10, p.DaysElapsed, "clamped to days_total")
	require.InDelta(t, 70.0, p.RequiredPerDay, 1e-9)
	q, ok := planner.SessionQuantity(g, p, day("2026-09-12"))
	require.True(t, ok)
	require.Equal(t, 70, q)

	// Before the start: nothing elapsed, so nothing is expected yet.
	p = planner.Progress(g, tasks[:1], day("2026-08-30"))
	require.Equal(t, 0, p.DaysElapsed)
	require.Equal(t, planner.PaceOnTrack, p.Pace)
	require.Nil(t, p.ProjectedFinish)
	_, ok = planner.SessionQuantity(g, p, day("2026-08-30"))
	require.False(t, ok, "no session before start_day")
}

func TestProgressPaceAndFinish(t *testing.T) {
	t.Parallel()
	g := planner.Goal{ID: "g", Kind: model.GoalTasks, StartDay: "2026-09-01", DueDay: "2026-09-10", Status: model.GoalActive}
	mk := func(nDone, nOpen int) []planner.Task {
		var ts []planner.Task
		for i := range nDone {
			ts = append(ts, task(fmt.Sprintf("d%d", i), i, forGoal("g"), done(nil)))
		}
		for i := range nOpen {
			ts = append(ts, task(fmt.Sprintf("o%d", i), 100+i, forGoal("g")))
		}
		return ts
	}
	tests := []struct {
		name        string
		done, open  int
		today       string
		pace        string
		finish      string
		sessionLeft int
	}{
		{"ahead", 6, 4, "2026-09-05", planner.PaceAhead, "2026-09-08", 6},
		{"on track at 0.9", 9, 11, "2026-09-05", planner.PaceOnTrack, "2026-09-11", 6},
		{"behind", 1, 9, "2026-09-05", planner.PaceBehind, "2026-10-19", 6},
		{"all done finishes today", 3, 0, "2026-09-05", planner.PaceAhead, "2026-09-05", 6},
		{"nothing done has no finish", 0, 5, "2026-09-05", planner.PaceBehind, "", 6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := planner.Progress(g, mk(tc.done, tc.open), day(tc.today))
			require.Equal(t, tc.done+tc.open, p.Target)
			require.Equal(t, tc.pace, p.Pace)
			require.Equal(t, tc.sessionLeft, p.SessionsLeft)
			if tc.finish == "" {
				require.Nil(t, p.ProjectedFinish)
			} else {
				require.Equal(t, tc.finish, p.ProjectedFinish.String())
			}
		})
	}
}

func TestSessionQuantityNeedsActiveGoalAndWork(t *testing.T) {
	t.Parallel()
	g := quantityGoal()
	p := planner.Progress(g, []planner.Task{sessionTemplate("FREQ=DAILY")}, day("2026-09-20"))
	g.Status = model.GoalAbandoned
	_, ok := planner.SessionQuantity(g, p, day("2026-09-20"))
	require.False(t, ok)
	g.Status = model.GoalActive
	p.Remaining = 0
	_, ok = planner.SessionQuantity(g, p, day("2026-09-20"))
	require.False(t, ok)
}

func commitment(rule string, start *int, dur int, counts bool, project *string) planner.Commitment {
	return planner.Commitment{ID: rule, Title: "c", RRule: rule, StartMinute: start, Duration: mins(dur),
		CountsTowardTarget: counts, ProjectID: project, ActiveFrom: "2026-09-01"}
}

func TestCapacityWorkedExample(t *testing.T) {
	t.Parallel()
	internship := commitment("FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", ptr(10*60), 300, true, ptr("intern"))
	in := planner.CapacityInput{Day: day("2026-09-15"), Loc: time.UTC, DayStart: 9 * 60, DayEnd: 23 * 60,
		Buffer: 30 * time.Minute, Target: 8 * time.Hour, Commitments: []planner.Commitment{internship}}
	res := planner.Capacity(in)
	require.Equal(t, 150, res.Minutes, "min(480 - 300, 840 - 300) - 30")
	require.Equal(t, 540, res.Slot)
	require.Equal(t, 180, res.TargetLeft)
	require.Equal(t, []planner.Interval{
		{time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)},
		{time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC), time.Date(2026, 9, 15, 23, 0, 0, 0, time.UTC)},
	}, res.Free)

	floating := internship
	floating.StartMinute = nil
	in.Commitments = []planner.Commitment{floating}
	res = planner.Capacity(in)
	require.Equal(t, 150, res.Minutes, "a floating commitment takes its time from the slot total")
	require.Len(t, res.Free, 1)

	in.Day = day("2026-09-19") // Saturday: no internship
	require.Equal(t, 450, planner.Capacity(in).Minutes)
}

func TestCapacityToday(t *testing.T) {
	t.Parallel()
	internship := commitment("FREQ=DAILY", nil, 300, true, ptr("intern"))
	other := commitment("FREQ=DAILY", ptr(20*60), 60, false, nil)
	in := planner.CapacityInput{Day: day("2026-09-15"), Loc: time.UTC, Today: true,
		Now:      time.Date(2026, 9, 15, 14, 2, 0, 0, time.UTC),
		DayStart: 9 * 60, DayEnd: 23 * 60, Buffer: 30 * time.Minute, Target: 9 * time.Hour,
		Commitments: []planner.Commitment{internship, other},
		WorkedToday: []planner.ProjectWork{
			{ProjectID: ptr("intern"), Worked: 3 * time.Hour}, // already inside the commitment
			{ProjectID: ptr("study"), Worked: 50 * time.Minute},
			{ProjectID: nil, Worked: 10 * time.Minute},
		},
	}
	res := planner.Capacity(in)
	// Free: 14:05–20:00 and 21:00–23:00 = 475; slot 475 − 300 floating = 175.
	require.Equal(t, 175, res.Slot)
	require.Equal(t, time.Date(2026, 9, 15, 14, 5, 0, 0, time.UTC), res.Free[0].Start)
	// Target: 540 − 300 − 50 − 10 = 180.
	require.Equal(t, 180, res.TargetLeft)
	require.Equal(t, 145, res.Minutes)

	in.Now = time.Date(2026, 9, 15, 22, 50, 0, 0, time.UTC)
	require.Equal(t, 0, planner.Capacity(in).Minutes, "never negative")
}

func TestCommitmentOnDay(t *testing.T) {
	t.Parallel()
	c := commitment("FREQ=DAILY", nil, 60, true, nil)
	c.ActiveFrom, c.ActiveUntil = "2026-09-10", ptr("2026-09-12")
	require.False(t, planner.OnDay(c, day("2026-09-09")))
	require.True(t, planner.OnDay(c, day("2026-09-10")))
	require.True(t, planner.OnDay(c, day("2026-09-12")))
	require.False(t, planner.OnDay(c, day("2026-09-13")))
}

func TestUrgency(t *testing.T) {
	t.Parallel()
	d := day("2026-09-15")
	progress := map[string]planner.GoalProgress{
		"behind": {RequiredPerDay: 4, ActualPerDay: 1},
		"ahead":  {RequiredPerDay: 4, ActualPerDay: 6},
		"done":   {RequiredPerDay: 0, ActualPerDay: 3},
	}
	tests := []struct {
		name  string
		task  planner.Task
		roll  int
		score float64
	}{
		{"nothing", task("a", 0, prio(1)), 0, 0},
		{"priority 4", task("a", 0, prio(4)), 0, 0.20},
		{"priority 2", task("a", 0), 0, 0.20 / 3},
		{"due today", task("a", 0, prio(1), dueOn("2026-09-15")), 0, 0.40},
		{"overdue clamps", task("a", 0, prio(1), dueOn("2026-09-01")), 0, 0.40},
		{"due in 7 days", task("a", 0, prio(1), dueOn("2026-09-22")), 0, 0.20},
		{"due in 14 days", task("a", 0, prio(1), dueOn("2026-09-29")), 0, 0},
		{"behind pace", task("a", 0, prio(1), forGoal("behind")), 0, 0.25 * 0.75},
		{"ahead of pace", task("a", 0, prio(1), forGoal("ahead")), 0, 0},
		{"nothing required", task("a", 0, prio(1), forGoal("done")), 0, 0},
		{"inactive goal", task("a", 0, prio(1), forGoal("abandoned")), 0, 0},
		{"rolled twice", task("a", 0, prio(1)), 2, 0.15 * 0.4},
		{"rolled often clamps", task("a", 0, prio(1)), 9, 0.15},
		{"everything", task("a", 0, prio(4), dueOn("2026-09-15"), forGoal("behind")), 5, 0.40 + 0.25*0.75 + 0.20 + 0.15},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.InDelta(t, tc.score, planner.Urgency(tc.task, d, progress, tc.roll), 1e-9)
		})
	}
}

func item(id, taskID, d string, status string, opts ...func(*planner.PlanItem)) planner.PlanItem {
	it := planner.PlanItem{ID: id, TaskID: taskID, Day: d, Status: status, Planned: mins(60), Envelope: model.Envelope{Rev: 1}}
	for _, o := range opts {
		o(&it)
	}
	return it
}

func TestRollover(t *testing.T) {
	t.Parallel()
	today := day("2026-09-15")
	occ := func(id, due string) planner.Task {
		return task(id, 0, dueOn(due), func(t *planner.Task) { t.TemplateID, t.OccurrenceDay = ptr("tmpl"), &due })
	}
	tasks := map[string]planner.Task{
		"open":          task("open", 0),
		"done":          task("done", 1, done(nil)),
		"deleted":       task("deleted", 2, func(t *planner.Task) { t.DeletedAt = &t0 }),
		"expired":       occ("expired", "2026-09-14"),
		"current":       occ("current", "2026-09-15"),
		"expired, done": func() planner.Task { t := occ("expired, done", "2026-09-10"); t.Status = model.TaskDone; return t }(),
		"unplanned":     occ("unplanned", "2026-09-13"),
	}
	items := []planner.PlanItem{
		item("i1", "open", "2026-09-14", model.PlanPlanned),
		item("i2", "done", "2026-09-14", model.PlanPlanned),
		item("i3", "deleted", "2026-09-14", model.PlanPlanned),
		item("i4", "expired", "2026-09-14", model.PlanPlanned),
		item("i5", "current", "2026-09-14", model.PlanPlanned),
		item("i6", "missing", "2026-09-13", model.PlanPlanned),
		item("i7", "open", "2026-09-15", model.PlanPlanned),            // today: untouched
		item("i8", "open", "2026-09-13", model.PlanSkipped),            // settled already
		item("i9", "open", "2026-09-12", model.PlanPlanned, deletedIt), // deleted item
	}
	res := planner.Rollover(items, tasks, today)
	require.Equal(t, []planner.StatusChange{
		{ItemID: "i1", Day: "2026-09-14", Status: model.PlanRolled},
		{ItemID: "i2", Day: "2026-09-14", Status: model.PlanDone},
		{ItemID: "i3", Day: "2026-09-14", Status: model.PlanSkipped},
		{ItemID: "i4", Day: "2026-09-14", Status: model.PlanSkipped},
		{ItemID: "i5", Day: "2026-09-14", Status: model.PlanRolled},
		{ItemID: "i6", Day: "2026-09-13", Status: model.PlanSkipped},
	}, res.Changes)
	require.Equal(t, []string{"expired", "unplanned"}, res.Expire)
}

func deletedIt(it *planner.PlanItem) { it.DeletedAt = &t0 }

func TestPending(t *testing.T) {
	t.Parallel()
	tasks := map[string]planner.Task{
		"a": task("a", 0), "b": task("b", 1), "c": task("c", 2), "d": task("d", 3, done(nil)),
	}
	items := []planner.PlanItem{
		item("a1", "a", "2026-09-13", model.PlanRolled),
		item("a2", "a", "2026-09-14", model.PlanRolled, func(it *planner.PlanItem) { it.Position = 1 }),
		item("a3", "a", "2026-09-14", model.PlanRolled),
		item("b1", "b", "2026-09-14", model.PlanRolled),
		item("b2", "b", "2026-09-15", model.PlanPlanned), // re-planned today
		item("c1", "c", "2026-09-14", model.PlanSkipped),
		item("d1", "d", "2026-09-14", model.PlanRolled), // done since
	}
	got := planner.Pending(items, tasks)
	require.Len(t, got, 1)
	require.Equal(t, "a3", got[0].ID)
}

func slotDay() civil.Day { return day("2026-09-15") }

func at(hm string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", "2026-09-15 "+hm)
	if err != nil {
		panic(err)
	}
	return t
}

func free(spans ...string) []planner.Interval {
	var out []planner.Interval
	for i := 0; i < len(spans); i += 2 {
		out = append(out, planner.Interval{Start: at(spans[i]), End: at(spans[i+1])})
	}
	return out
}

type block struct {
	task    string
	minutes int
	start   string // "" when unslotted
	kept    string
}

func blocks(drafts []planner.PlanItemDraft) []block {
	var out []block
	for i, d := range drafts {
		if d.Position != i {
			panic("positions must count from 0")
		}
		b := block{task: d.TaskID, minutes: int(d.Planned / time.Minute), kept: d.ExistingID}
		if d.StartAt != nil {
			b.start = d.StartAt.Format("15:04")
		}
		out = append(out, b)
	}
	return out
}

func TestGenerateBlocks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		capacity int
		tasks    []planner.Task
		tracked  map[string]time.Duration
		want     []block
	}{
		{"default estimate is 60", 300, []planner.Task{task("a", 0)}, nil,
			[]block{{task: "a", minutes: 60, start: "09:00"}}},
		{"long work is split into two blocks of at most 90", 300, []planner.Task{task("a", 0, estimate(240))}, nil,
			[]block{{task: "a", minutes: 90, start: "09:00"}, {task: "a", minutes: 90, start: "10:40"}}},
		{"a leftover under 25 is not planned", 300, []planner.Task{task("a", 0, estimate(100))}, nil,
			[]block{{task: "a", minutes: 90, start: "09:00"}}},
		{"blocks round down to 5", 300, []planner.Task{task("a", 0, estimate(47))}, nil,
			[]block{{task: "a", minutes: 45, start: "09:00"}}},
		{"tracked time counts against the estimate, floored at 25", 300,
			[]planner.Task{task("a", 0, estimate(60)), task("b", 1, estimate(60))},
			map[string]time.Duration{"a": 50 * time.Minute, "b": 20 * time.Minute},
			[]block{{task: "a", minutes: 25, start: "09:00"}, {task: "b", minutes: 40, start: "09:25"}}},
		{"capacity caps the last block", 100, []planner.Task{task("a", 0, estimate(60)), task("b", 1, estimate(60))}, nil,
			[]block{{task: "a", minutes: 60, start: "09:00"}, {task: "b", minutes: 40, start: "10:00"}}},
		{"no block under 25 when capacity runs short", 80, []planner.Task{task("a", 0, estimate(60)), task("b", 1)}, nil,
			[]block{{task: "a", minutes: 60, start: "09:00"}}},
		{"urgency orders the pool", 300,
			[]planner.Task{task("low", 0, prio(1)), task("high", 1, prio(4)), task("due", 2, prio(1), dueOn("2026-09-16"))}, nil,
			[]block{{task: "due", minutes: 60, start: "09:00"}, {task: "high", minutes: 60, start: "10:00"},
				{task: "low", minutes: 60, start: "11:00"}}},
		{"ties break by due day, priority, then created_at", 600,
			[]planner.Task{
				task("late", 0, prio(1), dueOn("2026-10-30")),
				task("early", 1, prio(1), dueOn("2026-10-29")),
				task("newer", 3, prio(1)),
				task("older", 2, prio(1)),
			}, nil,
			[]block{{task: "early", minutes: 60, start: "09:00"}, {task: "late", minutes: 60, start: "10:00"},
				{task: "older", minutes: 60, start: "11:00"}, {task: "newer", minutes: 60, start: "12:00"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := planner.Generate(planner.GenerateInput{
				Day: slotDay(), Capacity: planner.CapacityResult{Minutes: tc.capacity, Free: free("09:00", "23:00")},
				Tasks: tc.tasks, Tracked: tc.tracked,
			})
			require.Equal(t, tc.want, blocks(got))
		})
	}
}

func TestGeneratePool(t *testing.T) {
	t.Parallel()
	occ := func(id, on, due string) planner.Task {
		return task(id, 0, dueOn(due), func(t *planner.Task) { t.TemplateID, t.OccurrenceDay = ptr("tmpl"), &on })
	}
	rule := "FREQ=DAILY"
	tasks := []planner.Task{
		task("template", 0, func(t *planner.Task) { t.RRule = &rule }),
		task("done", 1, done(nil)),
		task("deleted", 2, func(t *planner.Task) { t.DeletedAt = &t0 }),
		occ("future occurrence", "2026-09-16", "2026-09-16"),
		occ("expired occurrence", "2026-09-14", "2026-09-14"),
		occ("today's occurrence", "2026-09-15", "2026-09-15"),
		occ("this week's occurrence", "2026-09-14", "2026-09-20"),
		task("overdue", 3, dueOn("2026-09-01")),
		task("kept", 4),
	}
	got := planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 600, Free: free("09:00", "23:00")}, Tasks: tasks,
		Existing: []planner.PlanItem{item("k", "kept", "2026-09-15", model.PlanDone)},
	})
	var ids []string
	for _, d := range got {
		ids = append(ids, d.TaskID)
	}
	require.Equal(t, []string{"kept", "overdue", "today's occurrence", "this week's occurrence"}, ids)
}

func TestGenerateKeepsAndSlotsAroundPinned(t *testing.T) {
	t.Parallel()
	pinnedAt := at("10:00")
	existing := []planner.PlanItem{
		item("unpinned", "u", "2026-09-15", model.PlanPlanned, func(it *planner.PlanItem) { it.Position = 0 }),
		item("pinned", "p", "2026-09-15", model.PlanPlanned, func(it *planner.PlanItem) {
			it.Pinned, it.StartAt, it.Planned, it.Position = true, &pinnedAt, mins(90), 3
		}),
		item("skipped", "s", "2026-09-15", model.PlanSkipped, func(it *planner.PlanItem) { it.Position = 1 }),
		item("pinned, unslotted", "q", "2026-09-15", model.PlanPlanned, func(it *planner.PlanItem) {
			it.Pinned, it.Planned, it.Position = true, mins(30), 2
		}),
	}
	tasks := []planner.Task{task("u", 0, estimate(90)), task("p", 1), task("s", 2), task("q", 3), task("n", 4, estimate(30))}
	got := planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 250, Free: free("09:00", "12:00", "13:00", "15:00")},
		Existing: existing, Tasks: tasks,
	})
	// Capacity left: 250 − 90 − 30 pinned = 130. The unpinned item is
	// dropped and "u" is planned afresh.
	require.Equal(t, []block{
		{task: "p", minutes: 90, start: "10:00", kept: "pinned"},
		{task: "s", minutes: 60, kept: "skipped"},
		{task: "q", minutes: 30, kept: "pinned, unslotted"},
		{task: "u", minutes: 90, start: "13:00"}, // 09:00–10:00 is too short, and so is 11:40–12:00 after the gap
		{task: "n", minutes: 30, start: "09:00"},
	}, blocks(got))
}

func TestGenerateGapAfterLongBlock(t *testing.T) {
	t.Parallel()
	got := planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 400, Free: free("09:00", "12:00", "12:30", "14:00")},
		Tasks: []planner.Task{task("a", 0, estimate(180)), task("b", 1, estimate(80)), task("c", 2, estimate(30))},
	})
	require.Equal(t, []block{
		{task: "a", minutes: 90, start: "09:00"},
		{task: "a", minutes: 90, start: "12:30"}, // 10:40–12:00 is 80 minutes
		{task: "b", minutes: 80, start: "10:40"},
		{task: "c", minutes: 30}, // nothing left that fits
	}, blocks(got))
}

func TestGenerateRolloverCount(t *testing.T) {
	t.Parallel()
	latest := map[string]planner.PlanItem{
		"rolled": item("r", "rolled", "2026-09-14", model.PlanRolled, func(it *planner.PlanItem) { it.RolloverCount = 2 }),
		"done":   item("d", "done", "2026-09-14", model.PlanDone, func(it *planner.PlanItem) { it.RolloverCount = 4 }),
	}
	got := planner.Generate(planner.GenerateInput{
		Day: slotDay(), Capacity: planner.CapacityResult{Minutes: 600, Free: free("09:00", "23:00")},
		Tasks: []planner.Task{task("rolled", 0, estimate(120)), task("done", 1)}, Latest: latest,
	})
	require.Len(t, got, 3)
	for _, d := range got[:2] {
		require.Equal(t, "rolled", d.TaskID)
		require.Equal(t, 3, d.RolloverCount)
		require.Equal(t, "r", *d.RolledFromID)
	}
	require.Equal(t, 0, got[2].RolloverCount)
	require.Nil(t, got[2].RolledFromID)
}

func TestReminders(t *testing.T) {
	t.Parallel()
	today := day("2026-09-15")
	capacity := make([]int, 14)
	for i := range capacity {
		capacity[i] = 120
	}
	tasks := []planner.Task{
		task("overdue one", 0, dueOn("2026-09-14")),
		task("overdue three", 1, dueOn("2026-09-12")),
		task("today", 2, dueOn("2026-09-15")),
		task("tomorrow", 3, dueOn("2026-09-16")),
		// 3 days left: avail 360, a quarter is 90; 100 remaining minutes > 90.
		task("start now", 4, dueOn("2026-09-18"), estimate(100)),
		// 60 remaining < 90, untracked and unplanned.
		task("not started", 5, dueOn("2026-09-18")),
		task("started", 6, dueOn("2026-09-18")),
		task("planned today", 7, dueOn("2026-09-18")),
		task("far", 8, dueOn("2026-09-24")),
		task("no due", 9),
	}
	in := planner.ReminderInput{
		Today: today, Tasks: tasks, DayCapacity: capacity,
		Tracked:      map[string]time.Duration{"started": time.Minute},
		PlannedToday: map[string]bool{"planned today": true},
	}
	got := planner.Reminders(in)
	msgs := map[string]string{}
	for _, r := range got {
		msgs[r.TaskID] = r.Message
	}
	require.Len(t, got, 5, "at most five")
	require.Equal(t, []string{"overdue three", "overdue one", "today", "tomorrow", "start now"},
		[]string{got[0].TaskID, got[1].TaskID, got[2].TaskID, got[3].TaskID, got[4].TaskID})
	require.Equal(t, "Overdue by 3 days", msgs["overdue three"])
	require.Equal(t, "Overdue by 1 day", msgs["overdue one"])
	require.Equal(t, -1, got[1].DaysLeft)
	require.Equal(t, "Due today", msgs["today"])
	require.Equal(t, "Due tomorrow", msgs["tomorrow"])
	require.Equal(t, "Start now — due in 3 days, about 1.7h of work left", msgs["start now"])

	in.Tasks = tasks[4:]
	got = planner.Reminders(in)
	require.Len(t, got, 2)
	require.Equal(t, "start now", got[0].TaskID)
	require.Equal(t, "not started", got[1].TaskID)
	require.Equal(t, "Not started — due in 3 days", got[1].Message)

	in.Tasks = []planner.Task{task("whole hours", 0, dueOn("2026-09-18"), estimate(120))}
	require.Equal(t, "Start now — due in 3 days, about 2h of work left", planner.Reminders(in)[0].Message)
}
