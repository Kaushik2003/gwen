package main

import (
	"testing"

	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

var (
	gID   = "01926d2e-7a4b-7c3d-8e9f-00000000a0a0"
	cID   = "01926d2e-7a4b-7c3d-8e9f-00000000c0c0"
	i1ID  = "01926d2e-7a4b-7c3d-8e9f-00000000f1f1"
	i2ID  = "01926d2e-7a4b-7c3d-8e9f-00000000f2f2"
	goalG = wire.Goal{ID: gID, Title: "300 problems", Kind: wire.GoalQuantity, Unit: "problems",
		TargetQuantity: testutil.Ptr(300), MinutesPerUnit: testutil.Ptr(30), StartDay: testutil.Day0,
		DueDay: "2026-12-15", Status: wire.GoalActive, Progress: wire.GoalProgress{DoneQuantity: 12,
			RemainingQuantity: 288, RequiredPerDay: 3.2, ActualPerDay: 1.5, Pace: wire.PaceBehind,
			ProjectedFinishDay: testutil.Ptr("2027-03-20")}}
	commitC = wire.Commitment{ID: cID, Title: "Internship", ProjectID: &pID, RRule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
		StartMinute: testutil.Ptr(600), DurationMinutes: 300, CountsTowardTarget: true, ActiveFrom: "2026-09-01"}
	item1 = wire.PlanItem{ID: i1ID, Day: testutil.Day0, TaskID: tID, PlannedMinutes: 90,
		StartAt: testutil.Ptr(wire.Millis(testutil.At("15:00"))), Status: wire.PlanPlanned, Task: taskT}
	item2 = wire.PlanItem{ID: i2ID, Day: testutil.Day0, TaskID: tID, PlannedMinutes: 30, Position: 1,
		Status: wire.PlanSkipped, Pinned: true, Task: taskT}
	planP = wire.Plan{Day: testutil.Day0, CapacityMinutes: 150, PlannedMinutes: 90, Items: []wire.PlanItem{item1, item2}}
)

const (
	goalOut   = "0000a0a0  active  300 problems  12/300 problems  behind  due 2026-12-15  finish 2027-03-20\n"
	commitOut = "0000c0c0  10:00–15:00  5h  Internship  Internship  FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR  counts  from 2026-09-01\n"
	planOut   = "2026-09-15 · 1h 30m planned of 2h 30m capacity\n" +
		"0000f1f1  3:00 pm  1h 30m  planned          Two pointers\n" +
		"0000f2f2  --:--    30m     skipped  pinned  Two pointers\n"
	item1Out = "0000f1f1  3:00 pm  1h 30m  planned    Two pointers\n"
)

// plannerFake answers the planner lookups too.
func plannerFake() *clienttest.Fake {
	return fake().
		Returns("ListGoals", &wire.GoalList{Goals: []wire.Goal{goalG}}, nil).
		Returns("ListCommitments", &wire.CommitmentList{Commitments: []wire.Commitment{commitC}}, nil).
		Returns("GetPlan", &planP, nil)
}

func TestPlannerGolden(t *testing.T) {
	t.Parallel()
	template := taskT
	template.RRule, template.Quantity = testutil.Ptr("FREQ=DAILY"), testutil.Ptr(4)
	tests := []struct {
		name   string
		args   []string
		script func(f *clienttest.Fake)
		method string
		want   []any
		out    string
	}{
		{
			name: "goal ls", args: []string{"goal", "ls", "--status", "all"},
			method: "ListGoals", want: []any{"all"}, out: goalOut,
		},
		{
			name: "goal show", args: []string{"goal", "show", "0000a0a0"},
			script: func(f *clienttest.Fake) { f.Returns("GetGoal", &goalG, nil) },
			method: "GetGoal", want: []any{gID},
			out: goalOut + "Needs 3.2 a day · doing 1.5 a day · started 2026-09-15\n",
		},
		{
			name: "goal add quantity",
			args: []string{"goal", "add", "300 problems", "--due", "2026-12-15", "--quantity", "300", "--unit", "problems",
				"--per-unit", "30m", "--project", "internship"},
			script: func(f *clienttest.Fake) { f.Returns("CreateGoal", &goalG, nil) },
			method: "CreateGoal",
			want: []any{wire.CreateGoalRequest{Title: "300 problems", Kind: wire.GoalQuantity, Unit: testutil.Ptr("problems"),
				TargetQuantity: testutil.Ptr(300), MinutesPerUnit: testutil.Ptr(30), ProjectID: &pID,
				StartDay: testutil.Day0, DueDay: "2026-12-15"}},
			out: goalOut,
		},
		{
			name: "goal add tasks", args: []string{"goal", "add", "Thesis", "--due", "2026-10-31", "--start", "2026-09-20"},
			script: func(f *clienttest.Fake) { f.Returns("CreateGoal", &goalG, nil) },
			method: "CreateGoal",
			want:   []any{wire.CreateGoalRequest{Title: "Thesis", Kind: wire.GoalTasks, StartDay: "2026-09-20", DueDay: "2026-10-31"}},
			out:    goalOut,
		},
		{
			name: "goal edit", args: []string{"goal", "edit", "0000a0a0", "--quantity", "250", "--per-unit", "25m",
				"--status", "abandoned", "--project", "none"},
			script: func(f *clienttest.Fake) { f.Returns("PatchGoal", &goalG, nil) },
			method: "PatchGoal",
			want: []any{gID, wire.PatchGoalRequest{TargetQuantity: wire.Some(250), MinutesPerUnit: wire.Some(25),
				ProjectID: wire.Null[string](), Status: testutil.Ptr("abandoned")}},
			out: goalOut,
		},
		{
			name: "goal rm", args: []string{"goal", "rm", "0000a0a0"},
			script: func(f *clienttest.Fake) { f.Returns("DeleteGoal", nil, nil) },
			method: "DeleteGoal", want: []any{gID, false}, out: "Deleted goal 300 problems.\n",
		},
		{
			name: "goal rm with tasks", args: []string{"goal", "rm", "0000a0a0", "--with-tasks"},
			script: func(f *clienttest.Fake) { f.Returns("DeleteGoal", nil, nil) },
			method: "DeleteGoal", want: []any{gID, true}, out: "Deleted goal 300 problems.\n",
		},
		{
			name: "commit ls", args: []string{"commit", "ls"}, method: "ListCommitments", out: commitOut,
		},
		{
			name: "commit add",
			args: []string{"commit", "add", "Internship", "--rrule", "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "--duration", "5h",
				"--at", "10:00", "--project", "Internship", "--from", "2026-09-01"},
			script: func(f *clienttest.Fake) { f.Returns("CreateCommitment", &commitC, nil) },
			method: "CreateCommitment",
			want: []any{wire.CreateCommitmentRequest{Title: "Internship", ProjectID: &pID, RRule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
				StartMinute: testutil.Ptr(600), DurationMinutes: 300, ActiveFrom: "2026-09-01"}},
			out: commitOut,
		},
		{
			name: "commit add floating, not counted", args: []string{"commit", "add", "Gym", "--rrule", "FREQ=DAILY",
				"--duration", "1h", "--no-count", "--until", "2026-12-31"},
			script: func(f *clienttest.Fake) { f.Returns("CreateCommitment", &commitC, nil) },
			method: "CreateCommitment",
			want: []any{wire.CreateCommitmentRequest{Title: "Gym", RRule: "FREQ=DAILY", DurationMinutes: 60,
				CountsTowardTarget: testutil.Ptr(false), ActiveFrom: testutil.Day0, ActiveUntil: testutil.Ptr("2026-12-31")}},
			out: commitOut,
		},
		{
			name: "commit edit", args: []string{"commit", "edit", "0000c0c0", "--at", "none", "--duration", "4h",
				"--until", "none", "--no-count"},
			script: func(f *clienttest.Fake) { f.Returns("PatchCommitment", &commitC, nil) },
			method: "PatchCommitment",
			want: []any{cID, wire.PatchCommitmentRequest{StartMinute: wire.Null[int](), DurationMinutes: testutil.Ptr(240),
				ActiveUntil: wire.Null[string](), CountsTowardTarget: testutil.Ptr(false)}},
			out: commitOut,
		},
		{
			name: "commit rm", args: []string{"commit", "rm", "0000c0c0"},
			script: func(f *clienttest.Fake) { f.Returns("DeleteCommitment", nil, nil) },
			method: "DeleteCommitment", want: []any{cID}, out: "Deleted commitment Internship.\n",
		},
		{name: "plan", args: []string{"plan"}, method: "GetPlan", want: []any{""}, out: planOut},
		{name: "plan for a day", args: []string{"plan", "2026-09-16"}, method: "GetPlan", want: []any{"2026-09-16"}, out: planOut},
		{
			name: "plan gen", args: []string{"plan", "gen"},
			script: func(f *clienttest.Fake) { f.Returns("GeneratePlan", &planP, nil) },
			method: "GeneratePlan", want: []any{wire.GeneratePlanRequest{Day: testutil.Day0}}, out: planOut,
		},
		{
			name: "plan gen empty", args: []string{"plan", "gen", "2026-09-16"},
			script: func(f *clienttest.Fake) {
				f.Returns("GeneratePlan", &wire.Plan{Day: "2026-09-16", CapacityMinutes: 450, Items: []wire.PlanItem{}}, nil)
			},
			method: "GeneratePlan", want: []any{wire.GeneratePlanRequest{Day: "2026-09-16"}},
			out: "2026-09-16 · 0s planned of 7h 30m capacity\nNothing planned.\n",
		},
		{
			name: "plan hours", args: []string{"plan", "hours", "--from", "19:00", "--work", "3h"},
			script: func(f *clienttest.Fake) {
				hours := planP
				hours.Hours = &wire.DayHours{StartMinute: testutil.Ptr(19 * 60), WorkMinutes: testutil.Ptr(180)}
				f.Returns("SetDayHours", &hours, nil)
			},
			method: "SetDayHours",
			want: []any{wire.SetDayHoursRequest{Day: testutil.Day0, StartMinute: testutil.Ptr(19 * 60),
				WorkMinutes: testutil.Ptr(180)}},
			out: "2026-09-15 · 1h 30m planned of 2h 30m capacity\nHours: from 19:00 · 3h for tasks\n" +
				"0000f1f1  3:00 pm  1h 30m  planned          Two pointers\n" +
				"0000f2f2  --:--    30m     skipped  pinned  Two pointers\n",
		},
		{
			name: "plan hours keeps what it is not given", args: []string{"plan", "hours", "2026-09-16", "--work", "0"},
			script: func(f *clienttest.Fake) {
				hours := planP
				hours.Hours = &wire.DayHours{StartMinute: testutil.Ptr(19 * 60), WorkMinutes: testutil.Ptr(180)}
				f.Returns("GetPlan", &hours, nil)
				off := wire.Plan{Day: "2026-09-16", Hours: &wire.DayHours{StartMinute: testutil.Ptr(19 * 60),
					WorkMinutes: testutil.Ptr(0)}, Items: []wire.PlanItem{}}
				f.Returns("SetDayHours", &off, nil)
			},
			method: "SetDayHours",
			want: []any{wire.SetDayHoursRequest{Day: "2026-09-16", StartMinute: testutil.Ptr(19 * 60),
				WorkMinutes: testutil.Ptr(0)}},
			out: "2026-09-16 · 0s planned of 0s capacity\nHours: from 19:00 · no time for tasks\nNothing planned.\n",
		},
		{
			name: "plan hours usual", args: []string{"plan", "hours", "--usual"},
			script: func(f *clienttest.Fake) { f.Returns("SetDayHours", &planP, nil) },
			method: "SetDayHours", want: []any{wire.SetDayHoursRequest{Day: testutil.Day0}}, out: planOut,
		},
		{
			name: "plan hours from usual", args: []string{"plan", "hours", "--from", "usual"},
			script: func(f *clienttest.Fake) { f.Returns("SetDayHours", &planP, nil) },
			method: "SetDayHours", want: []any{wire.SetDayHoursRequest{Day: testutil.Day0}}, out: planOut,
		},
		{
			name: "plan move at", args: []string{"plan", "move", "0000f1f1", "--at", "16:30"},
			script: func(f *clienttest.Fake) { f.Returns("PatchPlanItem", &item1, nil) },
			method: "PatchPlanItem",
			want:   []any{i1ID, wire.PatchPlanItemRequest{StartAt: wire.Some(wire.Millis(testutil.At("16:30")))}},
			out:    item1Out,
		},
		{
			name: "plan move position and minutes", args: []string{"plan", "move", "0000f2f2", "--position", "0", "--minutes", "45"},
			script: func(f *clienttest.Fake) { f.Returns("PatchPlanItem", &item1, nil) },
			method: "PatchPlanItem",
			want:   []any{i2ID, wire.PatchPlanItemRequest{Position: testutil.Ptr(0), PlannedMinutes: testutil.Ptr(45)}},
			out:    item1Out,
		},
		{
			name: "plan skip", args: []string{"plan", "skip", "0000f1f1"},
			script: func(f *clienttest.Fake) { f.Returns("PatchPlanItem", &item1, nil) },
			method: "PatchPlanItem", want: []any{i1ID, wire.PatchPlanItemRequest{Status: testutil.Ptr("skipped")}},
			out: item1Out,
		},
		{
			name: "plan unskip", args: []string{"plan", "unskip", "0000f2f2", "--day", "2026-09-15"},
			script: func(f *clienttest.Fake) { f.Returns("PatchPlanItem", &item1, nil) },
			method: "PatchPlanItem", want: []any{i2ID, wire.PatchPlanItemRequest{Status: testutil.Ptr("planned")}},
			out: item1Out,
		},
		{
			name: "brief", args: []string{"brief"},
			script: func(f *clienttest.Fake) {
				pending := item1
				pending.Day, pending.Status, pending.StartAt = "2026-09-14", wire.PlanRolled, nil
				f.Returns("Briefing", &wire.Briefing{Day: testutil.Day0, Pending: []wire.PlanItem{pending},
					Today: []wire.PlanItem{item1}, Reminders: []wire.Reminder{{TaskID: tID, Title: "Two pointers",
						DueDay: "2026-09-16", DaysLeft: 1, Message: "Due tomorrow"}}, Goals: []wire.Goal{goalG}}, nil)
			},
			method: "Briefing",
			out: "Briefing for 2026-09-15\n\nPending from earlier\n  0000cccc  2026-09-14  Two pointers\n" +
				"\nToday\n  " + item1Out +
				"\nReminders\n  0000cccc  Two pointers  Due tomorrow\n" +
				"\nGoals\n  " + goalOut,
		},
		{
			name: "brief with nothing", args: []string{"brief"},
			script: func(f *clienttest.Fake) {
				f.Returns("Briefing", &wire.Briefing{Day: testutil.Day0, Pending: []wire.PlanItem{}, Today: []wire.PlanItem{},
					Reminders: []wire.Reminder{}, Goals: []wire.Goal{}}, nil)
			},
			method: "Briefing", out: "Briefing for 2026-09-15\nNothing planned, pending, or due.\n",
		},
		{
			name:   "task add with goal, rrule, and quantity",
			args:   []string{"task", "add", "Katas", "--goal", "0000a0a0", "--rrule", "FREQ=DAILY", "--quantity", "4"},
			script: func(f *clienttest.Fake) { f.Returns("CreateTask", &template, nil) },
			method: "CreateTask",
			want: []any{wire.CreateTaskRequest{Title: "Katas", GoalID: &gID, RRule: testutil.Ptr("FREQ=DAILY"),
				Quantity: testutil.Ptr(4)}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m  ×4  repeats FREQ=DAILY\n",
		},
		{
			name:   "task edit clears goal, rrule, and quantity",
			args:   []string{"task", "edit", "0000cccc", "--goal", "none", "--rrule", "none", "--quantity", "0"},
			script: func(f *clienttest.Fake) { f.Returns("PatchTask", &taskT, nil) },
			method: "PatchTask",
			want: []any{tID, wire.PatchTaskRequest{GoalID: wire.Null[string](), RRule: wire.Null[string](),
				Quantity: wire.Null[int]()}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task ls by goal with templates", args: []string{"task", "ls", "--goal", "0000a0a0", "--templates"},
			method: "ListTasks", want: []any{wire.TaskQuery{GoalID: gID, Templates: true}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task done with a quantity", args: []string{"task", "done", "0000cccc", "--qty", "3"},
			script: func(f *clienttest.Fake) { f.Returns("CompleteTask", &taskT, nil) },
			method: "CompleteTask", want: []any{tID, wire.CompleteTaskRequest{QuantityDone: testutil.Ptr(3)}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := plannerFake()
			if tc.script != nil {
				tc.script(f)
			}
			out, errOut, code := runCLI(t, f, tc.args...)
			require.Equal(t, exitOK, code, errOut)
			if tc.want != nil {
				require.Equal(t, tc.want, lastCall(t, f, tc.method))
			} else {
				require.NotEmpty(t, f.CallsTo(tc.method))
			}
			require.Equal(t, tc.out, out)
		})
	}
}

func TestPlannerUsageErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"goal", "add", "x"},
		{"goal", "add", "x", "--due", "2026-12-01", "--quantity", "3"},
		{"goal", "add", "x", "--due", "2026-12-01", "--unit", "pages"},
		{"commit", "add", "x", "--duration", "1h"},
		{"commit", "add", "x", "--rrule", "FREQ=DAILY", "--duration", "1h", "--at", "25:00"},
		{"commit", "edit", "0000c0c0", "--count", "--no-count"},
		{"plan", "move", "0000f1f1"},
		{"plan", "hours"},
		{"plan", "hours", "--usual", "--work", "2h"},
		{"plan", "hours", "--from", "25:00"},
		{"plan", "hours", "--work", "soon"},
		{"plan", "chat"},
		{"plan", "a", "b"},
		{"plan", "someday"},
	} {
		_, errOut, code := runCLI(t, plannerFake(), args...)
		require.Equal(t, exitUsage, code, "%v: %s", args, errOut)
	}
	_, errOut, code := runCLI(t, plannerFake(), "plan", "skip", "0000beef")
	require.Equal(t, exitAPI, code)
	require.Contains(t, errOut, `no plan item on 2026-09-15 matches "0000beef"`)
}
