package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Every v1 endpoint's happy path, in the order a day of use would hit them.

func TestHealth(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	h, err := d.c.Health(d.ctx)
	require.NoError(t, err)
	require.True(t, h.OK)
	require.Equal(t, "1.0.0-test", h.Version)
	require.Equal(t, int64(7), h.SchemaVersion)
	require.NotZero(t, h.PID)
}

func TestTrackingCommands(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	st := d.status()
	require.Equal(t, wire.StateOff, st.State)
	require.Nil(t, st.WorkDay)
	require.Equal(t, "wayland", st.ActivityBackend)
	require.Equal(t, []string{}, st.Warnings)
	require.Equal(t, wire.Millis(t0), st.ServerNowAt)

	p := d.project("Internship")
	q := d.project("Study")
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "DSA", ProjectID: &q.ID})
	require.NoError(t, err)

	st, err = d.c.ClockIn(d.ctx, wire.ClockInRequest{ProjectID: &p.ID})
	require.NoError(t, err)
	require.Equal(t, wire.StateWorking, st.State)
	require.Equal(t, testutil.Day0, st.WorkDay.Day)
	require.Equal(t, "work", st.OpenSegment.Kind)
	require.Equal(t, p.ID, *st.OpenSegment.ProjectID)
	require.Equal(t, 8*3600, st.Today.TargetSeconds)

	d.clk.Advance(time.Hour)
	st, err = d.c.Switch(d.ctx, wire.SwitchRequest{TaskID: &tk.ID})
	require.NoError(t, err)
	require.Equal(t, q.ID, *st.ProjectID, "a task alone brings its project")
	require.Equal(t, int64(3600_000), st.Today.WorkedMs)

	d.clk.Advance(time.Hour)
	st, err = d.c.BreakStart(d.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.StateBreakManual, st.State)
	st, err = d.c.Snooze(d.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.Millis(d.clk.Now().Add(10*time.Minute)), *st.SnoozedUntilAt)

	d.clk.Advance(20 * time.Minute)
	st, err = d.c.BreakEnd(d.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.StateWorking, st.State)
	require.Equal(t, int64(20*60_000), st.Today.BreakMs)

	d.clk.Advance(time.Hour)
	st, err = d.c.ClockOut(d.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.StateOff, st.State)
	require.Nil(t, st.OpenSegment)

	day, err := d.c.GetDay(d.ctx, testutil.Day0)
	require.NoError(t, err)
	require.Len(t, day.Segments, 4)
	require.Equal(t, int64(3*3600_000), day.Summary.WorkedMs)
	require.NotNil(t, day.WorkDay.ClockedOutAt)
	require.Len(t, day.Summary.ByProject, 2)

	list, err := d.c.ListDays(d.ctx, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	require.Len(t, list.Days, 1)
	require.Equal(t, day.Summary, list.Days[0])

	task, err := d.c.GetTask(d.ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2*3600_000), task.TrackedMs)
}

func TestDayAndSegmentEdits(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	p := d.project("P")
	yesterday := "2026-09-14"
	at := func(hms string) int64 { return wire.Millis(testutil.AtOn(yesterday, hms)) }

	seg, err := d.c.CreateSegment(d.ctx, wire.CreateSegmentRequest{Day: yesterday, Kind: "work", ProjectID: &p.ID,
		StartedAt: at("09:00"), EndedAt: testutil.Ptr(at("11:00"))})
	require.NoError(t, err)
	require.Equal(t, "edit", seg.Source)

	day, err := d.c.GetDay(d.ctx, yesterday)
	require.NoError(t, err)
	require.Equal(t, "UTC", day.WorkDay.TZ)
	require.Equal(t, 8*3600, day.WorkDay.TargetSeconds, "from tracking.daily_target")

	wd, err := d.c.PatchDay(d.ctx, yesterday, wire.PatchDayRequest{TargetSeconds: testutil.Ptr(3600), Note: testutil.Ptr("half day"), Rev: &day.WorkDay.Rev})
	require.NoError(t, err)
	require.Equal(t, 3600, wd.TargetSeconds)
	require.Equal(t, "half day", wd.Note)

	moved, err := d.c.PatchSegment(d.ctx, seg.ID, wire.PatchSegmentRequest{ProjectID: wire.Null[string](),
		StartedAt: testutil.Ptr(at("08:30")), Rev: &seg.Rev})
	require.NoError(t, err)
	require.Nil(t, moved.ProjectID)
	require.Equal(t, at("08:30"), moved.StartedAt)

	halves, err := d.c.SplitSegment(d.ctx, seg.ID, wire.SplitSegmentRequest{At: at("10:00")})
	require.NoError(t, err)
	require.Len(t, halves.Segments, 2)
	require.Equal(t, seg.ID, halves.Segments[0].ID)

	require.NoError(t, d.c.DeleteSegment(d.ctx, halves.Segments[1].ID))
	day, err = d.c.GetDay(d.ctx, yesterday)
	require.NoError(t, err)
	require.Len(t, day.Segments, 1)
	require.Equal(t, int64(90*60_000), day.Summary.WorkedMs)
}

func TestProjectsAndTasks(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	p, err := d.c.CreateProject(d.ctx, wire.CreateProjectRequest{Name: "Internship"})
	require.NoError(t, err)
	require.Equal(t, "#3b82f6", p.Color)
	got, err := d.c.GetProject(d.ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, p, got)

	up, err := d.c.PatchProject(d.ctx, p.ID, wire.PatchProjectRequest{Archived: testutil.Ptr(true), Rev: &p.Rev})
	require.NoError(t, err)
	require.NotNil(t, up.ArchivedAt)
	live, err := d.c.ListProjects(d.ctx, "")
	require.NoError(t, err)
	require.Empty(t, live.Projects)
	all, err := d.c.ListProjects(d.ctx, wire.ArchivedAll)
	require.NoError(t, err)
	require.Len(t, all.Projects, 1)

	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{ProjectID: &p.ID, Title: "Report", DueDay: testutil.Ptr("2026-09-20")})
	require.NoError(t, err)
	require.Equal(t, 2, tk.Priority)
	require.Nil(t, tk.GoalID, "v2 fields are present and null")
	tk, err = d.c.PatchTask(d.ctx, tk.ID, wire.PatchTaskRequest{DueDay: wire.Null[string](), EstimateMinutes: wire.Some(45)})
	require.NoError(t, err)
	require.Nil(t, tk.DueDay)
	require.Equal(t, 45, *tk.EstimateMinutes)
	tk, err = d.c.CompleteTask(d.ctx, tk.ID, wire.CompleteTaskRequest{})
	require.NoError(t, err)
	require.Equal(t, "done", tk.Status)
	again, err := d.c.CompleteTask(d.ctx, tk.ID, wire.CompleteTaskRequest{})
	require.NoError(t, err)
	require.Equal(t, tk, again, "idempotent")
	tk, err = d.c.ReopenTask(d.ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, "open", tk.Status)
	tasks, err := d.c.ListTasks(d.ctx, wire.TaskQuery{ProjectID: p.ID})
	require.NoError(t, err)
	require.Len(t, tasks.Tasks, 1)

	require.NoError(t, d.c.DeleteTask(d.ctx, tk.ID))
	require.NoError(t, d.c.DeleteProject(d.ctx, p.ID))
	_, err = d.c.GetProject(d.ctx, p.ID)
	apiErr(t, err, wire.CodeNotFound)
}

func TestStats(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	for _, day := range []string{"2026-09-13", "2026-09-14"} {
		_, err := d.c.CreateSegment(d.ctx, wire.CreateSegmentRequest{Day: day, Kind: "work",
			StartedAt: wire.Millis(testutil.AtOn(day, "09:00")), EndedAt: testutil.Ptr(wire.Millis(testutil.AtOn(day, "17:00")))})
		require.NoError(t, err)
	}
	sum, err := d.c.StatsSummary(d.ctx, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	require.Equal(t, int64(16*3600_000), sum.WorkedMs)
	require.Equal(t, 2, sum.DaysTracked)
	require.Equal(t, 2, sum.DaysTargetMet)
	require.Equal(t, 2, sum.CurrentStreak, "today not met yet, so the run counts back from yesterday")
	require.Equal(t, "Unassigned", sum.ByProject[0].Name)
	require.Equal(t, "#6b7280", sum.ByProject[0].Color)

	heat, err := d.c.StatsHeatmap(d.ctx, 2026)
	require.NoError(t, err)
	require.Equal(t, 2026, heat.Year)
	require.Len(t, heat.Days, 2)
}

func TestConfigAndNotifyTest(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	cfg, err := d.c.GetConfig(d.ctx)
	require.NoError(t, err)
	require.Equal(t, "3m", cfg.Tracking.SoftIdle)

	cfg, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"tracking": {"soft_idle": "10s", "hard_idle": "30s"}})
	require.NoError(t, err)
	require.Equal(t, "10s", cfg.Tracking.SoftIdle)
	starts := d.mon.starts()
	require.Equal(t, []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second}, starts[len(starts)-1],
		"the monitor restarts with the new thresholds")
	saved, err := d.c.GetConfig(d.ctx)
	require.NoError(t, err)
	require.Equal(t, "30s", saved.Tracking.HardIdle)
	require.Equal(t, 0o600, int(fileMode(t, d.cfgPath)))

	res, err := d.c.NotifyTest(d.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.NotifyTestResult{Desktop: "ok", Phone: "disabled"}, *res)
}

// Every documented error code.

func TestErrorCodes(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	p := d.project("P")
	other := d.project("Other")
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{ProjectID: &p.ID, Title: "T"})
	require.NoError(t, err)

	t.Run("invalid_state with the state", func(t *testing.T) {
		_, err := d.c.BreakStart(d.ctx)
		ae := apiErr(t, err, wire.CodeInvalidState)
		require.Equal(t, 422, ae.Status)
		require.Equal(t, "cannot start a break while off", ae.Message)
		require.Equal(t, "off", ae.Details["state"])
	})
	t.Run("not_found for unknown attribution", func(t *testing.T) {
		_, err := d.c.ClockIn(d.ctx, wire.ClockInRequest{ProjectID: testutil.Ptr("nope")})
		ae := apiErr(t, err, wire.CodeNotFound)
		require.Equal(t, "project_id", ae.Details["field"])
		_, err = d.c.Switch(d.ctx, wire.SwitchRequest{TaskID: testutil.Ptr("nope")})
		apiErr(t, err, wire.CodeNotFound)
	})
	t.Run("invalid_request for a task of another project", func(t *testing.T) {
		_, err := d.c.ClockIn(d.ctx, wire.ClockInRequest{ProjectID: &other.ID, TaskID: &tk.ID})
		ae := apiErr(t, err, wire.CodeInvalidRequest)
		require.Equal(t, "task_id", ae.Details["field"])
	})
	t.Run("conflict on a duplicate name and a stale rev", func(t *testing.T) {
		_, err := d.c.CreateProject(d.ctx, wire.CreateProjectRequest{Name: "p"})
		apiErr(t, err, wire.CodeConflict)
		_, err = d.c.PatchProject(d.ctx, p.ID, wire.PatchProjectRequest{Name: testutil.Ptr("Q"), Rev: testutil.Ptr(int64(99))})
		ae := apiErr(t, err, wire.CodeConflict)
		require.Equal(t, "changed elsewhere", ae.Message)
	})
	t.Run("invalid_request on validation with the field", func(t *testing.T) {
		_, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "x", Priority: testutil.Ptr(9)})
		ae := apiErr(t, err, wire.CodeInvalidRequest)
		require.Equal(t, "priority", ae.Details["field"])
		_, err = d.c.ListDays(d.ctx, "2026-01-01", "2027-06-01")
		apiErr(t, err, wire.CodeInvalidRequest)
		_, err = d.c.ListProjects(d.ctx, "maybe")
		apiErr(t, err, wire.CodeInvalidRequest)
		_, err = d.c.GetDay(d.ctx, "not-a-day")
		apiErr(t, err, wire.CodeInvalidRequest)
	})
	t.Run("not_found for missing rows", func(t *testing.T) {
		_, err := d.c.GetDay(d.ctx, "2020-01-01")
		apiErr(t, err, wire.CodeNotFound)
		_, err = d.c.PatchDay(d.ctx, "2020-01-01", wire.PatchDayRequest{})
		apiErr(t, err, wire.CodeNotFound)
		_, err = d.c.GetTask(d.ctx, "nope")
		apiErr(t, err, wire.CodeNotFound)
		apiErr(t, d.c.DeleteSegment(d.ctx, "nope"), wire.CodeNotFound)
		apiErr(t, d.c.DeleteTask(d.ctx, "nope"), wire.CodeNotFound)
		_, err = d.c.PatchSegment(d.ctx, "nope", wire.PatchSegmentRequest{})
		apiErr(t, err, wire.CodeNotFound)
		_, err = d.c.SplitSegment(d.ctx, "nope", wire.SplitSegmentRequest{At: 1})
		apiErr(t, err, wire.CodeNotFound)
		_, err = d.c.ReopenTask(d.ctx, "nope")
		apiErr(t, err, wire.CodeNotFound)
		_, err = d.c.PatchTask(d.ctx, "nope", wire.PatchTaskRequest{})
		apiErr(t, err, wire.CodeNotFound)
		_, err = d.c.CompleteTask(d.ctx, "nope", wire.CompleteTaskRequest{})
		apiErr(t, err, wire.CodeNotFound)
		apiErr(t, d.c.DeleteProject(d.ctx, "nope"), wire.CodeNotFound)
	})
	t.Run("config errors name the key", func(t *testing.T) {
		_, err := d.c.PatchConfig(d.ctx, wire.ConfigPatch{"tracking": {"soft_idle": "20m"}})
		ae := apiErr(t, err, wire.CodeInvalidRequest)
		require.Equal(t, "tracking.soft_idle", ae.Details["key"])
		_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"tracking": {"nope": "1m"}})
		ae = apiErr(t, err, wire.CodeInvalidRequest)
		require.Equal(t, "tracking.nope", ae.Details["key"])
	})
	t.Run("stats and heatmap validation", func(t *testing.T) {
		_, err := d.c.StatsSummary(d.ctx, "x", "y")
		apiErr(t, err, wire.CodeInvalidRequest)
		_, err = d.c.StatsHeatmap(d.ctx, 0)
		apiErr(t, err, wire.CodeInvalidRequest)
	})
}

func TestTheOpenSegmentBelongsToTheEngine(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	p := d.project("P")
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{ProjectID: &p.ID, Title: "T"})
	require.NoError(t, err)
	st, err := d.c.ClockIn(d.ctx, wire.ClockInRequest{TaskID: &tk.ID})
	require.NoError(t, err)
	open := st.OpenSegment.ID

	_, err = d.c.PatchSegment(d.ctx, open, wire.PatchSegmentRequest{})
	apiErr(t, err, wire.CodeInvalidState)
	_, err = d.c.SplitSegment(d.ctx, open, wire.SplitSegmentRequest{At: 1})
	apiErr(t, err, wire.CodeInvalidState)
	apiErr(t, d.c.DeleteSegment(d.ctx, open), wire.CodeInvalidState)
	_, err = d.c.PatchProject(d.ctx, p.ID, wire.PatchProjectRequest{Archived: testutil.Ptr(true)})
	ae := apiErr(t, err, wire.CodeInvalidState)
	require.Equal(t, "working", ae.Details["state"])

	_, err = d.c.CreateSegment(d.ctx, wire.CreateSegmentRequest{Day: testutil.Day0, Kind: "work",
		StartedAt: wire.Millis(t0.Add(-time.Hour)), EndedAt: testutil.Ptr(wire.Millis(t0.Add(time.Hour)))})
	apiErr(t, err, wire.CodeInvalidRequest) // ends in the future
	_, err = d.c.CreateSegment(d.ctx, wire.CreateSegmentRequest{Day: testutil.Day0, Kind: "work",
		StartedAt: wire.Millis(t0.Add(-time.Hour)), EndedAt: testutil.Ptr(wire.Millis(t0.Add(time.Minute - time.Hour)))})
	require.NoError(t, err, "before the open segment is fine")
	d.clk.Advance(time.Hour)
	_, err = d.c.CreateSegment(d.ctx, wire.CreateSegmentRequest{Day: testutil.Day0, Kind: "work",
		StartedAt: wire.Millis(t0.Add(time.Minute)), EndedAt: testutil.Ptr(wire.Millis(t0.Add(2 * time.Minute)))})
	apiErr(t, err, wire.CodeConflict) // overlaps the open segment
}

func TestMalformedRequests(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	raw := func(method, path, body string) (int, string) {
		c := http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", d.socket)
		}}}
		req, err := http.NewRequest(method, "http://gwend"+path, strings.NewReader(body))
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	for _, tc := range []struct{ method, path, body, want string }{
		{"POST", "/v1/day/clock-in", `{"project_id": 5}`, `"field":"project_id"`},
		{"POST", "/v1/day/clock-out", `{"force": true}`, `"field":"force"`},
		{"POST", "/v1/break/start", ``, `invalid_request`},
		{"POST", "/v1/projects", `{"name": "x"} trailing`, `invalid_request`},
		{"POST", "/v1/projects", `not json`, `invalid_request`},
		{"PATCH", "/v1/config", `[1]`, `invalid_request`},
	} {
		status, body := raw(tc.method, tc.path, tc.body)
		require.Equal(t, 400, status, tc.body)
		require.Contains(t, body, tc.want, tc.body)
	}
	status, body := raw("GET", "/v1/nothing", "")
	require.Equal(t, 404, status)
	require.Contains(t, body, `"code":"not_found"`)
	require.True(t, bytes.Contains([]byte(body), []byte(`"details":{}`)))
	status, body = raw("PUT", "/v1/status", "")
	require.Equal(t, 404, status, "every non-2xx response carries the error envelope")
	require.Contains(t, body, `"code":"not_found"`)
}
