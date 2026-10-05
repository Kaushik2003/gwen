package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// The daemon's clock reads 2026-09-15 08:00 UTC, before the 09:00 window.

func TestNewTasksJoinTodaysPlan(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	_, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Report", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)

	late, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Slides", EstimateMinutes: testutil.Ptr(30),
		Priority: testutil.Ptr(4), DueDay: testutil.Ptr(testutil.Day0)})
	require.NoError(t, err)
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 2, "a task added after the plan joins it without regenerating")
	require.Equal(t, late.ID, plan.Items[0].Task.ID, "most urgent first")

	_, err = d.c.PatchTask(d.ctx, late.ID, wire.PatchTaskRequest{Stage: testutil.Ptr(wire.StageSomeday)})
	require.NoError(t, err)
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1, "someday tasks are never planned")
}

func TestScheduleByHand(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	at := wire.Millis(testutil.At("14:00"))
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Dentist prep", EstimateMinutes: testutil.Ptr(60),
		Stage: testutil.Ptr(wire.StageInbox), Effort: testutil.Ptr(3),
		Schedule: &wire.TaskSchedule{Day: testutil.Day0, StartAt: &at, PlannedMinutes: 45}})
	require.NoError(t, err)
	require.Equal(t, wire.StageTodo, tk.Stage, "putting it on the calendar processes it")
	require.Equal(t, 3, *tk.Effort)
	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	require.Equal(t, at, *plan.Items[0].StartAt)
	require.Equal(t, 45, plan.Items[0].PlannedMinutes)
	require.True(t, plan.Items[0].Pinned)

	tomorrow := "2026-09-16"
	moved, err := d.c.ScheduleTask(d.ctx, wire.ScheduleRequest{TaskID: tk.ID, Day: tomorrow, PlannedMinutes: 60})
	require.NoError(t, err)
	require.Equal(t, tomorrow, moved.Day)
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Empty(t, plan.Items, "moved to tomorrow, and today's plan leaves it there")

	un, err := d.c.UnscheduleTask(d.ctx, wire.UnscheduleRequest{TaskID: tk.ID})
	require.NoError(t, err)
	require.Equal(t, []string{tomorrow}, un.Days)
	plan, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1, "off tomorrow, so the planner takes it again")

	_, err = d.c.ScheduleTask(d.ctx, wire.ScheduleRequest{TaskID: tk.ID, Day: "2026-09-14", PlannedMinutes: 30})
	require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
	_, err = d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "x", Schedule: &wire.TaskSchedule{Day: testutil.Day0,
		PlannedMinutes: 1}})
	apiErr(t, err, wire.CodeInvalidRequest)
}

func TestReviewsAndEnergy(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	rv, err := d.c.GetReview(d.ctx, "2026-09-14")
	require.NoError(t, err)
	require.Empty(t, rv.ID, "a week without a review reads as empty")
	rv, err = d.c.SaveReview(d.ctx, "2026-09-14", wire.SaveReviewRequest{WentWell: testutil.Ptr("Up at 06:00"),
		Checklist: testutil.Ptr(5)})
	require.NoError(t, err)
	require.NotEmpty(t, rv.ID)
	rv, err = d.c.SaveReview(d.ctx, "2026-09-14", wire.SaveReviewRequest{Improvements: testutil.Ptr("Estimate better")})
	require.NoError(t, err)
	require.Equal(t, "Up at 06:00", rv.WentWell, "nil fields are unchanged")
	require.Equal(t, "Estimate better", rv.Improvements)
	require.Equal(t, 5, rv.Checklist)
	_, err = d.c.SaveReview(d.ctx, "2026-09-15", wire.SaveReviewRequest{})
	apiErr(t, err, wire.CodeInvalidRequest)

	for _, c := range []struct {
		at    string
		level int
	}{{"2026-09-14 08:10", 3}, {"2026-09-14 09:30", 5}, {"2026-09-14 10:15", 5}, {"2026-09-14 11:40", 4},
		{"2026-09-14 14:00", 2}, {"2026-09-14 16:20", 1}, {"2026-09-13 09:45", 4}} {
		ms := wire.Millis(testutil.AtOn(c.at[:10], c.at[11:]))
		_, err := d.c.LogEnergy(d.ctx, wire.LogEnergyRequest{Level: c.level, At: &ms})
		require.NoError(t, err)
	}
	rep, err := d.c.EnergyReport(d.ctx, 0)
	require.NoError(t, err)
	require.Len(t, rep.Logs, 7)
	require.Equal(t, "09:00", *rep.SuggestedStart)
	require.Equal(t, "12:00", *rep.SuggestedEnd)
	_, err = d.c.LogEnergy(d.ctx, wire.LogEnergyRequest{Level: 6})
	apiErr(t, err, wire.CodeInvalidRequest)
	require.NoError(t, d.c.DeleteEnergy(d.ctx, rep.Logs[0].ID))
	apiErr(t, d.c.DeleteEnergy(d.ctx, rep.Logs[0].ID), wire.CodeNotFound)
}

func TestAssistantChat(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	reply, sent := "", ""
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System   string `json:"system"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		sent = body.System + "\n" + body.Messages[0].Content
		text, _ := json.Marshal(reply)
		fmt.Fprintf(w, `{"content": [{"type": "text", "text": %s}], "stop_reason": "end_turn"}`, text)
	}))
	t.Cleanup(fake.Close)
	d := startDaemonWith(t, setup{llm: true}, func(o *options) { o.anthropicURL = fake.URL })
	_, err := d.c.PatchConfig(d.ctx, wire.ConfigPatch{"llm": {"provider": "anthropic", "assistant_name": "Rex",
		"personality": "sergeant", "instructions": "Call me boss."}})
	require.NoError(t, err)
	require.NoError(t, config.WriteCredential(config.CredentialsDir(d.dataDir), config.CredLLMAPIKey, []byte("sk\n")))
	p, err := d.c.CreateProject(d.ctx, wire.CreateProjectRequest{Name: "School"})
	require.NoError(t, err)

	null := func(fields string) string {
		all := map[string]any{}
		for _, k := range []string{"type", "ref", "key", "title", "notes", "project", "goal", "priority", "due_day",
			"start_day", "estimate_minutes", "effort", "stage", "delegated_to", "day", "start", "minutes", "name",
			"specific", "measurable", "assignable", "realistic"} {
			all[k] = nil
		}
		require.NoError(t, json.Unmarshal([]byte(fields), &all))
		b, _ := json.Marshal(all)
		return string(b)
	}
	mu.Lock()
	reply = `{"reply": "Done, boss.", "actions": [` +
		null(`{"type": "create_task", "key": "n1", "title": "Essay", "project": "p1", "estimate_minutes": 90, "effort": 3}`) + `,` +
		null(`{"type": "schedule_task", "ref": "n1", "day": "2026-09-15", "start": "10:00", "minutes": 60}`) + `,` +
		null(`{"type": "update_task", "ref": "t9", "stage": "waiting"}`) + `,` +
		null(`{"type": "create_goal", "title": "Pass the exam", "due_day": "2026-10-30", "specific": "Score 80%"}`) + `]}`
	mu.Unlock()
	run, err := d.c.AssistantChat(d.ctx, wire.AssistantChatRequest{Message: "Add my essay at ten"})
	require.NoError(t, err)
	require.Equal(t, wire.RunAssistant, run.Kind)
	require.Equal(t, wire.RunOK, run.Status)
	var out wire.AssistantOutput
	require.NoError(t, json.Unmarshal(run.Output, &out))
	require.Len(t, out.Messages, 2)
	require.Equal(t, "Done, boss.", out.Messages[1].Text)
	acts := out.Messages[1].Actions
	require.Len(t, acts, 4)
	require.True(t, acts[0].OK, acts[0].Error)
	require.Equal(t, `Added "Essay", in School`, acts[0].Summary)
	require.True(t, acts[1].OK, acts[1].Error)
	require.Equal(t, `Scheduled "Essay" today 10:00–11:00`, acts[1].Summary)
	require.False(t, acts[2].OK, "an unknown ref fails alone")
	require.True(t, acts[3].OK, acts[3].Error)

	mu.Lock()
	require.Contains(t, sent, "Your name is Rex.")
	require.Contains(t, sent, "drill sergeant")
	require.Contains(t, sent, "Call me boss.")
	mu.Unlock()

	tasks, err := d.c.ListTasks(d.ctx, wire.TaskQuery{ProjectID: p.ID})
	require.NoError(t, err)
	require.Len(t, tasks.Tasks, 1)
	require.Equal(t, 3, *tasks.Tasks[0].Effort)
	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	require.Equal(t, wire.Millis(testutil.At("10:00")), *plan.Items[0].StartAt)
	goals, err := d.c.ListGoals(d.ctx, "active")
	require.NoError(t, err)
	require.Equal(t, "Score 80%", goals.Goals[0].Specific)

	mu.Lock()
	reply = `{"reply": "You have the essay at ten.", "actions": []}`
	mu.Unlock()
	run, err = d.c.AssistantChat(d.ctx, wire.AssistantChatRequest{Message: "What's next?", RunID: &run.ID})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(run.Output, &out))
	require.Len(t, out.Messages, 4, "the conversation goes on")
	mu.Lock()
	require.Contains(t, sent, `"blocks":[{"day":"2026-09-15","start":"10:00","minutes":60,"pinned":true}]`)
	mu.Unlock()
}
