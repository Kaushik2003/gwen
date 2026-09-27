package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Fixtures: the clock reads 2026-09-15 10:30 UTC.
var (
	now      = testutil.At("10:30")
	pID      = "01926d2e-7a4b-7c3d-8e9f-00000000aaaa"
	qID      = "01926d2e-7a4b-7c3d-8e9f-00000000bbbb"
	tID      = "01926d2e-7a4b-7c3d-8e9f-00000000cccc"
	sID      = "01926d2e-7a4b-7c3d-8e9f-00000000dddd"
	wdID     = "01926d2e-7a4b-7c3d-8e9f-00000000eeee"
	projectP = wire.Project{ID: pID, Name: "Internship", Color: "#3b82f6"}
	projectQ = wire.Project{ID: qID, Name: "Study", Color: "#10b981", ArchivedAt: testutil.Ptr(int64(1))}
	taskT    = wire.Task{ID: tID, ProjectID: &qID, Title: "Two pointers", Status: "open", Priority: 3,
		DueDay: testutil.Ptr("2026-10-01"), TrackedMs: 65 * 60_000}
	segment = wire.Segment{ID: sID, WorkDayID: wdID, Kind: "work", Source: "user", ProjectID: &pID,
		StartedAt: wire.Millis(testutil.At("09:00")), EndedAt: testutil.Ptr(wire.Millis(testutil.At("10:00")))}
	today = wire.DaySummary{Day: testutil.Day0, TargetSeconds: 8 * 3600, WorkedMs: (5*60 + 47) * 60_000, BreakMs: 38 * 60_000,
		ByProject: []wire.ProjectTotal{{ProjectID: &pID, Name: "Internship", Color: "#3b82f6", WorkedMs: (5*60 + 47) * 60_000}}}
	workDay = wire.WorkDay{ID: wdID, Day: testutil.Day0, TZ: "UTC", ClockedInAt: wire.Millis(testutil.At("09:00")), TargetSeconds: 8 * 3600}
)

func working() *wire.Status {
	open := segment
	open.StartedAt, open.EndedAt = wire.Millis(testutil.At("07:18")), nil
	open.TaskID = &tID
	return &wire.Status{State: wire.StateWorking, OpenSegment: &open, ProjectID: &pID, TaskID: &tID,
		Today: &today, ServerNowAt: wire.Millis(now), Warnings: []string{}}
}

// fake answers the lookups every command may make.
func fake() *clienttest.Fake {
	cfg := config.Defaults().Wire()
	return clienttest.New().
		Returns("GetConfig", &cfg, nil).
		Returns("ListProjects", &wire.ProjectList{Projects: []wire.Project{projectP, projectQ}}, nil).
		Returns("ListTasks", &wire.TaskList{Tasks: []wire.Task{taskT}}, nil).
		Returns("GetDay", &wire.DayDetail{WorkDay: workDay, Summary: today, Segments: []wire.Segment{segment}}, nil)
}

func runCLI(t *testing.T, f *clienttest.Fake, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	c := &cli{
		newAPI: func(string) client.API { return f },
		stdin:  strings.NewReader(""),
		stdout: &out,
		stderr: &errb,
		clk:    clock.NewFake(now),
		loc:    time.UTC,
	}
	code = c.execute(context.Background(), args)
	return out.String(), errb.String(), code
}

// lastCall is the last call to method.
func lastCall(t *testing.T, f *clienttest.Fake, method string) []any {
	t.Helper()
	calls := f.CallsTo(method)
	require.NotEmpty(t, calls, "no call to %s", method)
	return calls[len(calls)-1].Args
}

func TestGolden(t *testing.T) {
	t.Parallel()
	endMs := wire.Millis(testutil.At("12:30"))
	tests := []struct {
		name   string
		args   []string
		script func(f *clienttest.Fake)
		method string // the endpoint the command must call
		want   []any  // its arguments after ctx, when checked
		out    string
	}{
		{
			name: "status", args: []string{"status"},
			script: func(f *clienttest.Fake) { f.Returns("Status", working(), nil) },
			method: "Status",
			out:    "Working · 3h 12m on Internship (task 0000cccc)\nToday   5h 47m worked · 38m break · target 8h (72%)\n",
		},
		{
			name: "status off", args: []string{"status"},
			script: func(f *clienttest.Fake) {
				f.Returns("Status", &wire.Status{State: wire.StateOff, Warnings: []string{"idle detection is unavailable"}}, nil)
			},
			method: "Status",
			out:    "Not clocked in.\nWarning: idle detection is unavailable\n",
		},
		{
			name: "status idle", args: []string{"status"},
			script: func(f *clienttest.Fake) {
				st := working()
				st.State, st.IdleSinceAt = wire.StateIdlePending, testutil.Ptr(wire.Millis(testutil.At("10:22")))
				f.Returns("Status", st, nil)
			},
			method: "Status",
			out:    "Idle since 10:22 (still counting)\nToday   5h 47m worked · 38m break · target 8h (72%)\n",
		},
		{
			name: "health", args: []string{"health"},
			script: func(f *clienttest.Fake) {
				f.Returns("Health", &wire.Health{OK: true, Version: "1.0.0", SchemaVersion: 1, PID: 4242}, nil)
			},
			method: "Health", out: "gwend 1.0.0 · schema 1 · pid 4242\n",
		},
		{
			name: "in by name and task suffix", args: []string{"in", "--project", "internship", "--task", "0000cccc"},
			script: func(f *clienttest.Fake) { f.Returns("ClockIn", working(), nil) },
			method: "ClockIn", want: []any{wire.ClockInRequest{ProjectID: &pID, TaskID: &tID}},
			out: "Working · 3h 12m on Internship (task 0000cccc)\nToday   5h 47m worked · 38m break · target 8h (72%)\n",
		},
		{
			name: "out", args: []string{"out"},
			script: func(f *clienttest.Fake) { f.Returns("ClockOut", &wire.Status{State: wire.StateOff}, nil) },
			method: "ClockOut", out: "Not clocked in.\n",
		},
		{
			name: "break", args: []string{"break"},
			script: func(f *clienttest.Fake) {
				st := working()
				st.State = wire.StateBreakManual
				st.OpenSegment.StartedAt = wire.Millis(testutil.At("10:18"))
				f.Returns("BreakStart", st, nil)
			},
			method: "BreakStart", out: "On break · 12m\nToday   5h 47m worked · 38m break · target 8h (72%)\n",
		},
		{
			name: "back", args: []string{"back"},
			script: func(f *clienttest.Fake) { f.Returns("BreakEnd", working(), nil) },
			method: "BreakEnd", out: "Working · 3h 12m on Internship (task 0000cccc)\nToday   5h 47m worked · 38m break · target 8h (72%)\n",
		},
		{
			name: "switch to unassigned", args: []string{"switch", "--project", "none"},
			script: func(f *clienttest.Fake) {
				st := working()
				st.ProjectID, st.TaskID = nil, nil
				f.Returns("Switch", st, nil)
			},
			method: "Switch", want: []any{wire.SwitchRequest{}},
			out: "Working · 3h 12m on Unassigned\nToday   5h 47m worked · 38m break · target 8h (72%)\n",
		},
		{
			name: "snooze", args: []string{"snooze"},
			script: func(f *clienttest.Fake) {
				st := working()
				st.SnoozedUntilAt = testutil.Ptr(wire.Millis(testutil.At("10:40")))
				f.Returns("Snooze", st, nil)
			},
			method: "Snooze",
			out:    "Working · 3h 12m on Internship (task 0000cccc)\nToday   5h 47m worked · 38m break · target 8h (72%)\nNudges snoozed until 10:40\n",
		},
		{
			name: "today", args: []string{"today"}, method: "GetDay", want: []any{testutil.Day0},
			out: "2026-09-15 · 5h 47m worked · 38m break · target 8h (72%)\n" +
				"0000dddd  09:00–10:00  work  Internship  user  1h\n" +
				"Internship  5h 47m\n",
		},
		{
			name: "log", args: []string{"log"},
			script: func(f *clienttest.Fake) {
				d := today
				d.TargetMet = true
				f.Returns("ListDays", &wire.DayList{Days: []wire.DaySummary{d}}, nil)
			},
			method: "ListDays", want: []any{"2026-09-09", "2026-09-15"},
			out: "2026-09-15  5h 47m worked  38m break  72%  ✓\n",
		},
		{
			name: "day show", args: []string{"day", "show", "2026-09-14"}, method: "GetDay", want: []any{"2026-09-14"},
			out: "2026-09-15 · 5h 47m worked · 38m break · target 8h (72%)\n" +
				"0000dddd  09:00–10:00  work  Internship  user  1h\n" +
				"Internship  5h 47m\n",
		},
		{
			name: "day set", args: []string{"day", "set", "2026-09-14", "--target", "4h", "--note", "sick"},
			script: func(f *clienttest.Fake) {
				wd := workDay
				wd.Day, wd.TargetSeconds = "2026-09-14", 4*3600
				f.Returns("PatchDay", &wd, nil)
			},
			method: "PatchDay", want: []any{"2026-09-14", wire.PatchDayRequest{TargetSeconds: testutil.Ptr(4 * 3600), Note: testutil.Ptr("sick")}},
			out: "2026-09-14 · target 4h\n",
		},
		{
			name: "seg add", args: []string{"seg", "add", "--kind", "work", "--start", "11:00", "--end", "12:30", "--project", "0000aaaa"},
			script: func(f *clienttest.Fake) { f.Returns("CreateSegment", &segment, nil) },
			method: "CreateSegment",
			want: []any{wire.CreateSegmentRequest{Day: testutil.Day0, Kind: "work", ProjectID: &pID,
				StartedAt: wire.Millis(testutil.At("11:00")), EndedAt: &endMs}},
			out: "0000dddd  09:00–10:00  work  Internship  user  1h\n",
		},
		{
			name: "seg add after midnight belongs to the work day", args: []string{"seg", "add", "--day", "2026-09-14", "--kind", "break_manual", "--start", "23:30", "--end", "01:00"},
			script: func(f *clienttest.Fake) { f.Returns("CreateSegment", &segment, nil) },
			method: "CreateSegment",
			want: []any{wire.CreateSegmentRequest{Day: "2026-09-14", Kind: "break_manual",
				StartedAt: wire.Millis(testutil.AtOn("2026-09-14", "23:30")), EndedAt: testutil.Ptr(wire.Millis(testutil.AtOn("2026-09-15", "01:00")))}},
			out: "0000dddd  09:00–10:00  work  Internship  user  1h\n",
		},
		{
			name: "seg edit", args: []string{"seg", "edit", "0000dddd", "--end", "10:15", "--project", "none"},
			script: func(f *clienttest.Fake) { f.Returns("PatchSegment", &segment, nil) },
			method: "PatchSegment",
			want:   []any{sID, wire.PatchSegmentRequest{EndedAt: testutil.Ptr(wire.Millis(testutil.At("10:15"))), ProjectID: wire.Null[string]()}},
			out:    "0000dddd  09:00–10:00  work  Internship  user  1h\n",
		},
		{
			name: "seg split", args: []string{"seg", "split", "0000dddd", "09:30"},
			script: func(f *clienttest.Fake) {
				a, b := segment, segment
				a.EndedAt, b.StartedAt = testutil.Ptr(wire.Millis(testutil.At("09:30"))), wire.Millis(testutil.At("09:30"))
				f.Returns("SplitSegment", &wire.SegmentList{Segments: []wire.Segment{a, b}}, nil)
			},
			method: "SplitSegment", want: []any{sID, wire.SplitSegmentRequest{At: wire.Millis(testutil.At("09:30"))}},
			out: "0000dddd  09:00–09:30  work  Internship  user  30m\n0000dddd  09:30–10:00  work  Internship  user  30m\n",
		},
		{
			name: "seg rm", args: []string{"seg", "rm", "0000dddd"},
			script: func(f *clienttest.Fake) { f.Returns("DeleteSegment", nil, nil) },
			method: "DeleteSegment", want: []any{sID}, out: "Removed segment 0000dddd.\n",
		},
		{
			name: "project ls", args: []string{"project", "ls", "--all"}, method: "ListProjects", want: []any{"all"},
			out: "0000aaaa  Internship  #3b82f6  \n0000bbbb  Study       #10b981  archived\n",
		},
		{
			name: "project show", args: []string{"project", "show", "Internship"},
			script: func(f *clienttest.Fake) { f.Returns("GetProject", &projectP, nil) },
			method: "GetProject", want: []any{pID}, out: "0000aaaa  Internship  #3b82f6  \n",
		},
		{
			name: "project add", args: []string{"project", "add", "Reading", "--color", "#f59e0b"},
			script: func(f *clienttest.Fake) {
				f.Returns("CreateProject", &wire.Project{ID: "01926d2e-7a4b-7c3d-8e9f-00000000ffff", Name: "Reading", Color: "#f59e0b"}, nil)
			},
			method: "CreateProject", want: []any{wire.CreateProjectRequest{Name: "Reading", Color: testutil.Ptr("#f59e0b")}},
			out: "0000ffff  Reading  #f59e0b  \n",
		},
		{
			name: "project edit", args: []string{"project", "edit", "internship", "--name", "Work"},
			script: func(f *clienttest.Fake) { f.Returns("PatchProject", &projectP, nil) },
			method: "PatchProject", want: []any{pID, wire.PatchProjectRequest{Name: testutil.Ptr("Work")}},
			out: "0000aaaa  Internship  #3b82f6  \n",
		},
		{
			name: "project archive", args: []string{"project", "archive", "0000aaaa"},
			script: func(f *clienttest.Fake) { f.Returns("PatchProject", &projectP, nil) },
			method: "PatchProject", want: []any{pID, wire.PatchProjectRequest{Archived: testutil.Ptr(true)}},
			out: "0000aaaa  Internship  #3b82f6  \n",
		},
		{
			name: "project unarchive by id", args: []string{"project", "unarchive", qID},
			script: func(f *clienttest.Fake) { f.Returns("PatchProject", &projectQ, nil) },
			method: "PatchProject", want: []any{qID, wire.PatchProjectRequest{Archived: testutil.Ptr(false)}},
			out: "0000bbbb  Study  #10b981  archived\n",
		},
		{
			name: "project rm", args: []string{"project", "rm", "Internship"},
			script: func(f *clienttest.Fake) { f.Returns("DeleteProject", nil, nil) },
			method: "DeleteProject", want: []any{pID}, out: "Deleted project Internship.\n",
		},
		{
			name: "task ls", args: []string{"task", "ls", "--project", "0000bbbb", "--status", "all", "--due-before", "2026-12-01"},
			method: "ListTasks", want: []any{wire.TaskQuery{ProjectID: qID, Status: "all", DueBefore: "2026-12-01"}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task show", args: []string{"task", "show", "0000cccc"},
			script: func(f *clienttest.Fake) { f.Returns("GetTask", &taskT, nil) },
			method: "GetTask", want: []any{tID}, out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task add", args: []string{"task", "add", "Two pointers", "--project", "internship", "--priority", "3", "--due", "2026-10-01", "--estimate", "1h30m", "--notes", "arrays"},
			script: func(f *clienttest.Fake) { f.Returns("CreateTask", &taskT, nil) },
			method: "CreateTask",
			want: []any{wire.CreateTaskRequest{ProjectID: &pID, Title: "Two pointers", Notes: testutil.Ptr("arrays"),
				Priority: testutil.Ptr(3), DueDay: testutil.Ptr("2026-10-01"), EstimateMinutes: testutil.Ptr(90)}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task edit", args: []string{"task", "edit", "0000cccc", "--title", "Sliding window", "--due", "none", "--estimate", "45m"},
			script: func(f *clienttest.Fake) { f.Returns("PatchTask", &taskT, nil) },
			method: "PatchTask",
			want: []any{tID, wire.PatchTaskRequest{Title: testutil.Ptr("Sliding window"), DueDay: wire.Null[string](),
				EstimateMinutes: wire.Some(45)}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task done", args: []string{"task", "done", "0000cccc"},
			script: func(f *clienttest.Fake) { f.Returns("CompleteTask", &taskT, nil) },
			method: "CompleteTask", want: []any{tID, wire.CompleteTaskRequest{}},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task reopen", args: []string{"task", "reopen", "0000cccc"},
			script: func(f *clienttest.Fake) { f.Returns("ReopenTask", &taskT, nil) },
			method: "ReopenTask", want: []any{tID},
			out: "0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n",
		},
		{
			name: "task rm", args: []string{"task", "rm", "0000cccc"},
			script: func(f *clienttest.Fake) { f.Returns("DeleteTask", nil, nil) },
			method: "DeleteTask", want: []any{tID}, out: "Deleted task 0000cccc.\n",
		},
		{
			name: "stats", args: []string{"stats"},
			script: func(f *clienttest.Fake) {
				f.Returns("StatsSummary", &wire.StatsSummary{From: "2026-08-17", To: "2026-09-15", WorkedMs: 40 * 3600_000,
					BreakMs: 3 * 3600_000, DaysTracked: 5, DaysTargetMet: 4, AvgWorkedMs: 8 * 3600_000, CurrentStreak: 3, LongestStreak: 7,
					ByProject: []wire.ProjectTotal{{ProjectID: &pID, Name: "Internship", WorkedMs: 40 * 3600_000}}}, nil)
			},
			method: "StatsSummary", want: []any{"2026-08-17", "2026-09-15"},
			out: "2026-08-17 to 2026-09-15\nWorked 40h over 5 days (average 8h) · 3h on breaks\n" +
				"Target met on 4 days · streak 3 (longest 7)\n  Internship  40h\n",
		},
		{
			name: "config get key", args: []string{"config", "get", "tracking.soft_idle"}, method: "GetConfig", out: "3m\n",
		},
		{
			name: "config set bool", args: []string{"config", "set", "nudge.phone", "true"},
			script: func(f *clienttest.Fake) {
				c := config.Defaults().Wire()
				c.Nudge.Phone = true
				f.Returns("PatchConfig", &c, nil)
			},
			method: "PatchConfig", want: []any{wire.ConfigPatch{"nudge": {"phone": true}}}, out: "nudge.phone = true\n",
		},
		{
			name: "config set list", args: []string{"config", "set", "calendar.busy_calendars", "primary, work"},
			script: func(f *clienttest.Fake) {
				c := config.Defaults().Wire()
				c.Calendar.BusyCalendars = []string{"primary", "work"}
				f.Returns("PatchConfig", &c, nil)
			},
			method: "PatchConfig", want: []any{wire.ConfigPatch{"calendar": {"busy_calendars": []any{"primary", "work"}}}},
			out: "calendar.busy_calendars = primary,work\n",
		},
		{
			name: "notify test", args: []string{"notify", "test"},
			script: func(f *clienttest.Fake) {
				f.Returns("NotifyTest", &wire.NotifyTestResult{Desktop: "ok", Phone: "disabled"}, nil)
			},
			method: "NotifyTest", out: "Desktop: ok\nPhone:   disabled\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := fake()
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

func TestConfigGetAll(t *testing.T) {
	t.Parallel()
	out, _, code := runCLI(t, fake(), "config", "get")
	require.Equal(t, exitOK, code)
	require.Contains(t, out, "tracking.soft_idle = 3m\n")
	require.Contains(t, out, "ntfy.topic = \"\"\n")
	require.Contains(t, out, "calendar.busy_calendars = primary\n")
	require.True(t, strings.HasPrefix(out, "calendar."), "keys are sorted")
}

func TestHeatmap(t *testing.T) {
	t.Parallel()
	f := fake().Returns("StatsHeatmap", &wire.Heatmap{Year: 2026, Days: []wire.HeatmapDay{
		{Day: "2026-01-01", WorkedMs: 2 * 3600_000}, // Thursday, 25 %
		{Day: "2026-01-02", WorkedMs: 4 * 3600_000}, // 50 %
		{Day: "2026-01-03", WorkedMs: 6 * 3600_000}, // 75 %
		{Day: "2026-01-04", WorkedMs: 7 * 3600_000}, // above 75 %
	}}, nil)
	out, _, code := runCLI(t, f, "stats", "heatmap")
	require.Equal(t, exitOK, code)
	require.Equal(t, []any{2026}, lastCall(t, f, "StatsHeatmap"))
	lines := strings.Split(out, "\n")
	require.Equal(t, "2026", lines[0])
	require.Equal(t, "Mon  ·", lines[1][:len("Mon  ·")], "2025 days are blank")
	require.True(t, strings.HasPrefix(lines[4], "Thu ░·"), lines[4])
	require.True(t, strings.HasPrefix(lines[5], "Fri ▒·"), lines[5])
	require.True(t, strings.HasPrefix(lines[6], "Sat ▓·"), lines[6])
	require.True(t, strings.HasPrefix(lines[7], "Sun █·"), lines[7])
	require.Equal(t, 53, len([]rune(lines[1]))-4, "one column per week")
}

func TestJSONPrintsTheWireJSON(t *testing.T) {
	t.Parallel()
	st := working()
	f := fake().Returns("Status", st, nil)
	out, _, code := runCLI(t, f, "--json", "status")
	require.Equal(t, exitOK, code)
	want, err := json.Marshal(st)
	require.NoError(t, err)
	require.Equal(t, string(want)+"\n", out)

	f = fake()
	out, _, code = runCLI(t, f, "--json", "project", "ls")
	require.Equal(t, exitOK, code)
	var list wire.ProjectList
	require.NoError(t, json.Unmarshal([]byte(out), &list))
	require.Len(t, list.Projects, 2)
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	t.Run("0 on success", func(t *testing.T) {
		_, _, code := runCLI(t, fake().Returns("Health", &wire.Health{}, nil), "health")
		require.Equal(t, exitOK, code)
	})
	t.Run("1 on an API error, printing its message", func(t *testing.T) {
		f := fake().Returns("BreakStart", nil, &client.APIError{Status: 422, Code: wire.CodeInvalidState,
			Message: "cannot start a break while off", Details: map[string]any{"state": "off"}})
		_, errOut, code := runCLI(t, f, "break")
		require.Equal(t, exitAPI, code)
		require.Equal(t, "gwen: cannot start a break while off\n", errOut)
	})
	t.Run("1 with changed elsewhere on a rev conflict", func(t *testing.T) {
		f := fake().Returns("PatchProject", nil, &client.APIError{Code: wire.CodeConflict, Message: "changed elsewhere",
			Details: map[string]any{"field": "rev"}})
		_, errOut, code := runCLI(t, f, "project", "edit", "Internship", "--name", "x")
		require.Equal(t, exitAPI, code)
		require.Contains(t, errOut, "Changed elsewhere — reloaded")
	})
	t.Run("2 on a usage error", func(t *testing.T) {
		for _, args := range [][]string{{"bogus"}, {"in", "--nope"}, {"day", "show"}, {"day", "show", "15/09"},
			{"seg", "add", "--kind", "work"}, {"project", "ls", "--archived", "--all"}, {"day", "set", "2026-09-14"},
			{"config", "set", "tracking.nope", "1"}, {"task", "add", "x", "--estimate", "soon"}} {
			_, errOut, code := runCLI(t, fake(), args...)
			require.Equal(t, exitUsage, code, "%v: %s", args, errOut)
		}
	})
	t.Run("2 on an ambiguous short id", func(t *testing.T) {
		f := fake().Returns("ListProjects", &wire.ProjectList{Projects: []wire.Project{
			{ID: "01926d2e-7a4b-7c3d-8e9f-10000000aaaa", Name: "A"}, {ID: "01926d2e-7a4b-7c3d-8e9f-20000000aaaa", Name: "B"},
		}}, nil)
		_, errOut, code := runCLI(t, f, "project", "show", "0000aaaa")
		require.Equal(t, exitUsage, code)
		require.Contains(t, errOut, "ambiguous id")
	})
	t.Run("3 when the daemon is not running", func(t *testing.T) {
		f := clienttest.New().Returns("Status", nil, client.ErrDaemonNotRunning)
		_, errOut, code := runCLI(t, f, "status")
		require.Equal(t, exitNoDaemon, code)
		require.Equal(t, "Gwen isn't running. Start it with: systemctl --user start gwend\n", errOut)
	})
	t.Run("1 for an unknown project name", func(t *testing.T) {
		_, errOut, code := runCLI(t, fake(), "in", "--project", "Nothing")
		require.Equal(t, exitAPI, code)
		require.Contains(t, errOut, `no project matches "Nothing"`)
	})
	t.Run("archived projects do not match by name", func(t *testing.T) {
		_, _, code := runCLI(t, fake(), "in", "--project", "study")
		require.Equal(t, exitAPI, code)
	})
}

// lockedBuffer is written by the command and read by the test concurrently.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestWatch(t *testing.T) {
	t.Parallel()
	s := clienttest.NewStream()
	f := fake().Returns("Events", s, nil)
	out := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	c := &cli{newAPI: func(string) client.API { return f }, stdin: strings.NewReader(""), stdout: out,
		stderr: &lockedBuffer{}, clk: clock.NewFake(now), loc: time.UTC}
	done := make(chan int, 1)
	go func() { done <- c.execute(ctx, []string{"watch"}) }()
	s.Send(wire.Event{ID: 1, Name: wire.EventProjectsChanged, Data: json.RawMessage(`{}`)})
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "projects_changed") }, 5*time.Second, time.Millisecond)
	cancel()
	require.Equal(t, exitOK, <-done)
	require.Equal(t, "10:30:00 projects_changed {}\n", out.String())
}
