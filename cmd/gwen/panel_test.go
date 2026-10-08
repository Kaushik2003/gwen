package main

import (
	"fmt"
	"testing"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestPanelLine(t *testing.T) {
	t.Parallel()
	st := working()
	st.StateSinceAt = wire.Millis(testutil.At("07:18"))
	block := func(id, title, at string, minutes int, status string) wire.PlanItem {
		return wire.PlanItem{Task: wire.Task{ID: id, Title: title}, StartAt: testutil.Ptr(wire.Millis(testutil.At(at))), PlannedMinutes: minutes, Status: status}
	}
	plan := &wire.Plan{Day: testutil.Day0, Items: []wire.PlanItem{
		block("late", "Essay", "15:00", 60, "planned"),
		block("done", "Reading", "10:00", 60, "done"),
		block("now", "Two pointers", "10:00", 60, "planned"),
		block("past", "Email", "08:00", 30, "planned"),
	}}
	energy := &wire.EnergyReport{Logs: []wire.EnergyLog{
		{ID: "e2", At: wire.Millis(testutil.At("09:00")), Level: 4},
		{ID: "e1", At: wire.Millis(testutil.At("08:00")), Level: 2},
	}}
	f := fake().Returns("Status", st, nil).Returns("GetProject", &projectP, nil).Returns("GetTask", &taskT, nil).Returns("GetPlan", plan, nil).
		Returns("ListProjects", &wire.ProjectList{Projects: []wire.Project{projectP}}, nil).Returns("EnergyReport", energy, nil)
	out, errOut, code := runCLI(t, f, "panel")
	require.Equal(t, exitOK, code, errOut)
	require.JSONEq(t, `{"state": "working", "worked_ms": 20820000, "break_ms": 2280000, "target_ms": 28800000, "since_ms": 11520000,
		"project": "Internship", "color": "#3b82f6", "task": "Two pointers",
		"next": {"task_id": "now", "project_id": "none", "title": "Two pointers", "start_at": `+fmt.Sprint(wire.Millis(testutil.At("10:00")))+`, "minutes": 60, "now": true},
		"more": 1, "project_id": "`+pID+`", "projects": [{"id": "`+pID+`", "name": "Internship", "color": "#3b82f6"}],
		"energy": {"id": "e2", "at": `+fmt.Sprint(wire.Millis(testutil.At("09:00")))+`, "level": 4}}`, out)
	require.Equal(t, []any{1}, lastCall(t, f, "EnergyReport"), "the last day's check-ins")
}

func TestPanelLineWithoutTheDaemon(t *testing.T) {
	t.Parallel()
	out, errOut, code := runCLI(t, fake().Returns("Status", nil, client.ErrDaemonNotRunning), "panel")
	require.Equal(t, exitOK, code, errOut, "the widget shows Gwen as off rather than an error")
	require.JSONEq(t, `{"state": "down", "worked_ms": 0, "break_ms": 0, "target_ms": 0, "since_ms": 0, "project": "", "color": "", "task": "",
		"next": null, "more": 0, "project_id": "", "projects": null, "energy": null}`, out)
}

func TestEnergy(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"4", "good", "Good"} {
		f := fake().Returns("LogEnergy", &wire.EnergyLog{ID: "e", At: wire.Millis(testutil.At("10:42")), Level: 4}, nil)
		out, errOut, code := runCLI(t, f, "energy", arg)
		require.Equal(t, exitOK, code, errOut)
		require.Equal(t, []any{wire.LogEnergyRequest{Level: 4}}, lastCall(t, f, "LogEnergy"))
		require.Equal(t, "Energy: Good at 10:42 am\n", out)
	}
	for _, arg := range []string{"0", "6", "tired"} {
		_, errOut, code := runCLI(t, fake(), "energy", arg)
		require.NotEqual(t, exitOK, code, arg)
		require.Contains(t, errOut, "1 to 5")
	}
}
