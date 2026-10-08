package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// The daemon's clock reads 2026-09-15 08:00 UTC, before the usual 09:00–23:00
// window opens (docs/06-planner.md#day-hours).
func TestDayHoursEndpoint(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	next := stream(t, d)
	_, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Report", EstimateMinutes: testutil.Ptr(120)})
	require.NoError(t, err)
	next(wire.EventTasksChanged)

	plan, err := d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	require.Equal(t, wire.PlanWindow{StartMinute: 9 * 60, EndMinute: 23 * 60}, plan.Window)
	require.Nil(t, plan.Hours)
	require.Equal(t, 450, plan.CapacityMinutes)
	next(wire.EventPlanChanged)

	plan, err = d.c.SetDayHours(d.ctx, wire.SetDayHoursRequest{Day: testutil.Day0, StartMinute: testutil.Ptr(19 * 60),
		WorkMinutes: testutil.Ptr(180)})
	require.NoError(t, err)
	require.Equal(t, wire.PlanWindow{StartMinute: 19 * 60, EndMinute: 23 * 60}, plan.Window)
	require.Equal(t, &wire.DayHours{StartMinute: testutil.Ptr(19 * 60), WorkMinutes: testutil.Ptr(180)}, plan.Hours)
	require.Equal(t, 180, plan.CapacityMinutes, "3 h from 19:00")
	require.Len(t, plan.Items, 2)
	require.Equal(t, wire.Millis(testutil.At("19:00")), *plan.Items[0].StartAt, "replanned into the new window")
	require.Equal(t, 120, plan.PlannedMinutes)
	require.JSONEq(t, `{"day":"2026-09-15"}`, string(next(wire.EventPlanChanged).Data))

	plan, err = d.c.SetDayHours(d.ctx, wire.SetDayHoursRequest{Day: testutil.Day0, WorkMinutes: testutil.Ptr(0)})
	require.NoError(t, err)
	require.Equal(t, 9*60, plan.Window.StartMinute, "the start is usual again")
	require.Equal(t, &wire.DayHours{WorkMinutes: testutil.Ptr(0)}, plan.Hours)
	require.Zero(t, plan.CapacityMinutes)
	require.Empty(t, plan.Items, "a day off")

	plan, err = d.c.SetDayHours(d.ctx, wire.SetDayHoursRequest{Day: testutil.Day0})
	require.NoError(t, err)
	require.Nil(t, plan.Hours, "both null deletes the row")
	require.Equal(t, 450, plan.CapacityMinutes)

	future, err := d.c.SetDayHours(d.ctx, wire.SetDayHoursRequest{Day: "2026-09-16", StartMinute: testutil.Ptr(14 * 60)})
	require.NoError(t, err)
	require.Equal(t, 14*60, future.Window.StartMinute)
	got, err := d.c.GetPlan(d.ctx, "2026-09-16")
	require.NoError(t, err)
	require.Equal(t, future.Hours, got.Hours, "kept for that day")

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			req   wire.SetDayHoursRequest
			field string
		}{
			{wire.SetDayHoursRequest{}, "day"},
			{wire.SetDayHoursRequest{Day: "2026-09-14", WorkMinutes: testutil.Ptr(60)}, "day"},
			{wire.SetDayHoursRequest{Day: testutil.Day0, StartMinute: testutil.Ptr(23 * 60)}, "start_minute"},
			{wire.SetDayHoursRequest{Day: testutil.Day0, StartMinute: testutil.Ptr(-1)}, "start_minute"},
			{wire.SetDayHoursRequest{Day: testutil.Day0, WorkMinutes: testutil.Ptr(1441)}, "work_minutes"},
		} {
			_, err := d.c.SetDayHours(d.ctx, tc.req)
			require.Equal(t, tc.field, apiErr(t, err, wire.CodeInvalidRequest).Details["field"], "%+v", tc.req)
		}
	})
}

// fakeModel is an Anthropic endpoint that records what each request sent.
type fakeModel struct {
	mu    sync.Mutex
	reply string
	sent  []map[string]any // each request's user message, decoded
}

func (m *fakeModel) say(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reply = s
}

func (m *fakeModel) last(t *testing.T) map[string]any {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	require.NotEmpty(t, m.sent)
	return m.sent[len(m.sent)-1]
}

func (m *fakeModel) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.Unmarshal(b, &body)
		var sent map[string]any
		json.Unmarshal([]byte(body.Messages[0].Content), &sent)
		m.mu.Lock()
		m.sent = append(m.sent, sent)
		text, _ := json.Marshal(m.reply)
		m.mu.Unlock()
		fmt.Fprintf(w, `{"content": [{"type": "text", "text": %s}], "stop_reason": "end_turn"}`, text)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPlanChatEndpoints(t *testing.T) {
	t.Parallel()
	model := &fakeModel{}
	srv := model.server(t)
	d := startDaemonWith(t, setup{llm: true}, func(o *options) { o.anthropicURL = srv.URL })
	ask := func(day, msg string, run *string) (*wire.LlmRun, error) {
		return d.c.PlanChat(d.ctx, wire.PlanChatRequest{Day: day, Message: msg, RunID: run})
	}
	_, err := ask(testutil.Day0, "plan my day", nil)
	apiErr(t, err, wire.CodeUnavailable)

	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"llm": {"provider": "anthropic"}})
	require.NoError(t, err)
	require.NoError(t, config.WriteCredential(config.CredentialsDir(d.dataDir), config.CredLLMAPIKey, []byte("sk\n")))
	dsa, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "DSA", Priority: testutil.Ptr(4),
		EstimateMinutes: testutil.Ptr(90)})
	require.NoError(t, err)
	osHW, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "OS homework", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	reading, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Reading", Priority: testutil.Ptr(1),
		EstimateMinutes: testutil.Ptr(30)})
	require.NoError(t, err)
	_, err = d.c.CreateCommitment(d.ctx, wire.CreateCommitmentRequest{Title: "Gym", RRule: "FREQ=DAILY",
		StartMinute: testutil.Ptr(18 * 60), DurationMinutes: 60, CountsTowardTarget: new(bool), ActiveFrom: testutil.Day0})
	require.NoError(t, err)

	// A first message: the day's planned items are the current plan.
	model.say(`{"reply": "DSA from 14:00, then the homework.", "items": [{"ref": "t1", "start": "14:00", "minutes": 90},
		{"ref": "t2", "start": "15:40", "minutes": 60}], "hours": {"start": "14:00", "work_minutes": 180}}`)
	run1, err := ask(testutil.Day0, "Start at 14:00, 3 hours, DSA first", nil)
	require.NoError(t, err)
	require.Equal(t, wire.RunOK, run1.Status)
	require.Equal(t, wire.RunDayPlan, run1.Kind)
	require.Equal(t, testutil.Day0, run1.SubjectID)
	var out1 wire.DayPlanOutput
	require.NoError(t, json.Unmarshal(run1.Output, &out1))
	require.Equal(t, []wire.ChatMessage{{Role: "user", Text: "Start at 14:00, 3 hours, DSA first"},
		{Role: "assistant", Text: "DSA from 14:00, then the homework."}}, out1.Messages)
	require.Equal(t, []wire.ProposedBlock{
		{TaskID: dsa.ID, Title: "DSA", StartAt: wire.Millis(testutil.At("14:00")), PlannedMinutes: 90},
		{TaskID: osHW.ID, Title: "OS homework", StartAt: wire.Millis(testutil.At("15:40")), PlannedMinutes: 60},
	}, out1.Items)
	require.Equal(t, &wire.DayHours{StartMinute: testutil.Ptr(14 * 60), WorkMinutes: testutil.Ptr(180)}, out1.Hours)

	sent := model.last(t)
	require.Equal(t, "Tuesday", sent["weekday"])
	require.Equal(t, "08:00", sent["now"])
	require.Equal(t, map[string]any{"start": "09:00", "end": "23:00", "work_minutes": float64(450)}, sent["hours"])
	require.Equal(t, []any{map[string]any{"title": "Gym", "start": "18:00", "end": "19:00"}}, sent["busy"])
	require.Equal(t, []any{map[string]any{"start": "09:00", "end": "18:00"}, map[string]any{"start": "19:00", "end": "23:00"}},
		sent["free"])
	tasks := sent["tasks"].([]any)
	require.Len(t, tasks, 3)
	first := tasks[0].(map[string]any)
	require.Equal(t, "t1", first["ref"])
	require.Equal(t, "DSA", first["title"], "most urgent first")
	require.Len(t, sent["plan"], 3, "the day's planned items")
	require.Len(t, sent["messages"], 1)

	// A follow-up goes on from the previous proposal.
	model.say(`{"reply": "Homework after the gym.", "items": [{"ref": "t1", "start": "14:00", "minutes": 90},
		{"ref": "t2", "start": "19:00", "minutes": 60}], "hours": {"start": null, "work_minutes": 150}}`)
	run2, err := ask(testutil.Day0, "Move the homework after the gym", &run1.ID)
	require.NoError(t, err)
	sent = model.last(t)
	require.Equal(t, []any{
		map[string]any{"ref": "t1", "start": "14:00", "minutes": float64(90)},
		map[string]any{"ref": "t2", "start": "15:40", "minutes": float64(60)},
	}, sent["plan"])
	require.Len(t, sent["messages"], 3)
	var out2 wire.DayPlanOutput
	require.NoError(t, json.Unmarshal(run2.Output, &out2))
	require.Len(t, out2.Messages, 4)

	// Accepting replaces the planned items and keeps the skipped one.
	before, err := d.c.GetPlan(d.ctx, testutil.Day0)
	require.NoError(t, err)
	var skipped wire.PlanItem
	for _, it := range before.Items {
		if it.TaskID == reading.ID {
			skipped = it
		}
	}
	_, err = d.c.PatchPlanItem(d.ctx, skipped.ID, wire.PatchPlanItemRequest{Status: testutil.Ptr(wire.PlanSkipped)})
	require.NoError(t, err)
	next := stream(t, d)
	accepted, err := d.c.AcceptLLMRun(d.ctx, run2.ID, wire.AcceptRunRequest{Indexes: []int{0, 1}})
	require.NoError(t, err)
	require.Equal(t, []string{dsa.ID, osHW.ID}, []string{accepted.Tasks[0].ID, accepted.Tasks[1].ID})
	require.JSONEq(t, `{"day":"2026-09-15"}`, string(next(wire.EventPlanChanged).Data))
	plan, err := d.c.GetPlan(d.ctx, testutil.Day0)
	require.NoError(t, err)
	require.Equal(t, &wire.DayHours{WorkMinutes: testutil.Ptr(150)}, plan.Hours)
	require.Equal(t, 150, plan.CapacityMinutes)
	var planned []wire.PlanItem
	for i, it := range plan.Items {
		require.Equal(t, i, it.Position)
		if it.Status == wire.PlanPlanned {
			planned = append(planned, it)
		}
	}
	require.Len(t, planned, 2)
	require.Equal(t, wire.Millis(testutil.At("14:00")), *planned[0].StartAt)
	require.Equal(t, wire.Millis(testutil.At("19:00")), *planned[1].StartAt)
	require.True(t, planned[0].Pinned && planned[1].Pinned)
	require.Equal(t, skipped.ID, plan.Items[0].ID, "kept, and first by its earlier start")
	require.Equal(t, wire.PlanSkipped, plan.Items[0].Status)
	regen, err := d.c.GeneratePlan(d.ctx, wire.GeneratePlanRequest{Day: testutil.Day0})
	require.NoError(t, err)
	require.Equal(t, plan.Items, regen.Items, "regenerating keeps the accepted plan")

	got, err := d.c.GetLLMRun(d.ctx, run2.ID)
	require.NoError(t, err)
	require.Equal(t, wire.RunAccepted, got.Status)
	_, err = d.c.AcceptLLMRun(d.ctx, run2.ID, wire.AcceptRunRequest{Indexes: []int{0}})
	apiErr(t, err, wire.CodeConflict)
	model.say(`{"reply": "Fine as it is.", "items": [], "hours": null}`)
	run3, err := ask(testutil.Day0, "Thanks", &run2.ID)
	require.NoError(t, err, "an accepted conversation goes on")
	_, err = d.c.AcceptLLMRun(d.ctx, run3.ID, wire.AcceptRunRequest{Indexes: []int{}})
	require.NoError(t, err, "an empty plan clears the planned items")

	t.Run("errors", func(t *testing.T) {
		_, err := ask(testutil.Day0, " ", nil)
		require.Equal(t, "message", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = ask("", "hi", nil)
		require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = ask("2026-09-14", "hi", nil)
		require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = ask(testutil.Day0, "hi", testutil.Ptr("nope"))
		require.Equal(t, "run_id", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
		_, err = ask("2026-09-16", "hi", &run1.ID)
		require.Equal(t, "run_id", apiErr(t, err, wire.CodeInvalidRequest).Details["field"], "another day's run")

		model.say("Sure! Here is your day.")
		failed, err := ask(testutil.Day0, "hi", nil)
		require.NoError(t, err)
		require.Equal(t, wire.RunFailed, failed.Status)
		require.Contains(t, string(failed.Output), "not the day plan JSON")
		_, err = ask(testutil.Day0, "hi", &failed.ID)
		require.Equal(t, "run_id", apiErr(t, err, wire.CodeInvalidRequest).Details["field"], "a failed run")
	})

	t.Run("20 messages at most", func(t *testing.T) {
		model.say(`{"reply": "Okay.", "items": [], "hours": null}`)
		var last *string
		for i := range 20 {
			run, err := ask("2026-09-16", fmt.Sprintf("message %d", i), last)
			require.NoError(t, err)
			last = &run.ID
		}
		_, err := ask("2026-09-16", "one more", last)
		require.Equal(t, "message", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
	})

	t.Run("a task gets no more time than it has left", func(t *testing.T) {
		day := "2026-09-17"
		essay, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Essay", Priority: testutil.Ptr(4),
			EstimateMinutes: testutil.Ptr(90)})
		require.NoError(t, err)
		model.say(`{"reply": "Okay.", "items": [], "hours": null}`)
		_, err = ask(day, "plan the 17th", nil)
		require.NoError(t, err)
		ref := ""
		for _, tk := range model.last(t)["tasks"].([]any) {
			if m := tk.(map[string]any); m["title"] == "Essay" {
				ref = m["ref"].(string)
			}
		}
		require.NotEmpty(t, ref)
		model.say(fmt.Sprintf(`{"reply": "Essay twice.", "items": [{"ref": %q, "start": "10:00", "minutes": 60},
			{"ref": %q, "start": "14:00", "minutes": 60}], "hours": null}`, ref, ref))
		run, err := ask(day, "plan the 17th", nil)
		require.NoError(t, err)
		_, err = d.c.AcceptLLMRun(d.ctx, run.ID, wire.AcceptRunRequest{Indexes: []int{0, 1}})
		require.NoError(t, err)
		plan, err := d.c.GetPlan(d.ctx, day)
		require.NoError(t, err)
		var minutes []int
		for _, it := range plan.Items {
			if it.TaskID == essay.ID {
				minutes = append(minutes, it.PlannedMinutes)
			}
		}
		require.Equal(t, []int{60, 30}, minutes, "90 minutes of work get 90 minutes")
	})

	t.Run("a past day cannot be accepted", func(t *testing.T) {
		model.say(`{"reply": "Okay.", "items": [{"ref": "t1", "start": "20:00", "minutes": 30}], "hours": null}`)
		run, err := ask("2026-09-16", "plan tomorrow", nil)
		require.NoError(t, err)
		d.clk.Advance(48 * time.Hour)
		d.sync()
		_, err = d.c.AcceptLLMRun(d.ctx, run.ID, wire.AcceptRunRequest{Indexes: []int{0}})
		require.Equal(t, "day", apiErr(t, err, wire.CodeInvalidRequest).Details["field"])
	})
}
