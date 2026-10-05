package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/stretchr/testify/require"
)

// dayReq is a day with 14:00–23:00 free but for 18:00–19:00, and two tasks.
func dayReq() llm.DayPlanRequest {
	return llm.DayPlanRequest{Day: "2026-09-15", Weekday: "Tuesday", Now: ptr("13:47"),
		Hours: llm.Hours{Start: "09:00", End: "23:00", WorkMinutes: 150},
		Busy:  []llm.BusyTime{{Title: "Gym", Start: "18:00", End: "19:00"}}, Floating: []llm.Floating{},
		Free: []llm.Span{{Start: "14:00", End: "18:00"}, {Start: "19:00", End: "23:00"}}, Done: []llm.DoneBlock{},
		Tasks: []llm.PlanTask{{Ref: "t1", Title: "DSA session", Priority: 3, RemainingMinutes: 90, OpenSteps: []string{}},
			{Ref: "t2", Title: "OS homework", Priority: 2, RemainingMinutes: 60, OpenSteps: []string{}}},
		Plan:     []llm.Block{},
		Messages: []llm.ChatMessage{{Role: llm.RoleUser, Text: "DSA first"}}}
}

const goodDayPlan = `{"reply": "DSA first, then the homework.", "items": [{"ref": "t1", "start": "14:00", "minutes": 90},
	{"ref": "t2", "start": "15:40", "minutes": 60}], "hours": null}`

func dayPlanner(t *testing.T, reply string) (llm.Planner, *provider) {
	t.Helper()
	p := &provider{reply: func(*http.Request) string { return reply }}
	srv := p.server(t, false)
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Endpoint = config.ProviderOpenAICompatible, srv.URL
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)
	return a, p
}

func TestDayPlan(t *testing.T) {
	t.Parallel()
	a, p := dayPlanner(t, "```json\n"+goodDayPlan+"\n```")
	out, err := a.DayPlan(context.Background(), dayReq())
	require.NoError(t, err)
	require.Equal(t, llm.DayPlanOutput{Reply: "DSA first, then the homework.", Items: []llm.PlannedBlock{
		{Ref: "t1", Start: "14:00", Minutes: 90}, {Ref: "t2", Start: "15:40", Minutes: 60}}}, out)

	_, body := p.last()
	msgs := body["messages"].([]any)
	system := msgs[0].(map[string]any)["content"].(string)
	require.Contains(t, system, "plan one day")
	require.True(t, strings.HasSuffix(system, llm.DayPlanSchema), "the schema closes the prompt")
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(msgs[1].(map[string]any)["content"].(string)), &sent))
	keys := make([]string, 0, len(sent))
	for k := range sent {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, []string{"day", "weekday", "now", "methods", "hours", "busy", "floating", "free", "done", "tasks",
		"plan", "messages"}, keys, "exactly the documented fields")

	a, _ = dayPlanner(t, `{"reply": "From 19:00, then.", "items": [{"ref": "t1", "start": "19:00", "minutes": 90}],
		"hours": {"start": "19:00", "work_minutes": 180}}`)
	out, err = a.DayPlan(context.Background(), dayReq())
	require.NoError(t, err)
	require.Equal(t, &llm.HoursChange{Start: ptr("19:00"), WorkMinutes: ptr(180)}, out.Hours)

	a, _ = dayPlanner(t, `{"reply": "Nothing today.", "items": [], "hours": {"start": null, "work_minutes": null}}`)
	out, err = a.DayPlan(context.Background(), dayReq())
	require.NoError(t, err)
	require.Empty(t, out.Items, "a day off is a plan too")
	require.Nil(t, out.Hours, "a change of nothing is no change")
}

func TestDayPlanValidation(t *testing.T) {
	t.Parallel()
	block := func(ref, start string, minutes int) string {
		b, _ := json.Marshal(map[string]any{"ref": ref, "start": start, "minutes": minutes})
		return string(b)
	}
	plan := func(hours string, blocks ...string) string {
		return `{"reply": "ok", "items": [` + strings.Join(blocks, ",") + `], "hours": ` + hours + `}`
	}
	many := make([]string, 31)
	for i := range many {
		many[i] = block("t1", "14:00", 5)
	}
	tests := []struct{ name, reply, want string }{
		{"not JSON", "Here is your day!", "not the day plan JSON"},
		{"no reply", `{"items": [], "hours": null}`, "no reply text"},
		{"empty reply", `{"reply": " ", "items": [], "hours": null}`, "1 to 2000"},
		{"long reply", `{"reply": "` + strings.Repeat("x", 2001) + `", "items": [], "hours": null}`, "1 to 2000"},
		{"unknown field", `{"reply": "ok", "items": [], "hours": null, "mood": "good"}`, "not the day plan JSON"},
		{"block field", plan("null", `{"ref": "t1", "start": "14:00", "minutes": 30, "why": "x"}`), "not valid"},
		{"block lacks start", plan("null", `{"ref": "t1", "minutes": 30}`), "lacks"},
		{"too many", plan("null", many...), "at most 30"},
		{"unknown ref", plan("null", block("t9", "14:00", 30)), "not one of the tasks"},
		{"bad start", plan("null", block("t1", "2pm", 30)), "not HH:MM"},
		{"hour 24", plan("null", block("t1", "24:00", 30)), "not HH:MM"},
		{"odd minutes", plan("null", block("t1", "14:00", 32)), "multiple of 5"},
		{"long block", plan("null", block("t1", "14:00", 485)), "multiple of 5"},
		{"short block", plan("null", block("t1", "14:00", 0)), "multiple of 5"},
		{"over busy time", plan("null", block("t1", "17:30", 60)), "not inside the free time"},
		{"in the past", plan("null", block("t1", "13:00", 30)), "not inside the free time"},
		{"overlap", plan("null", block("t1", "14:00", 60), block("t2", "14:30", 30)), "overlap"},
		{"before a later start", plan(`{"start": "19:00", "work_minutes": null}`, block("t1", "14:00", 30)), "before the day starts"},
		{"start after the end", plan(`{"start": "23:00", "work_minutes": null}`), "hours.start"},
		{"negative work", plan(`{"start": null, "work_minutes": -5}`), "work_minutes"},
		{"too much work", plan(`{"start": null, "work_minutes": 1441}`), "work_minutes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, _ := dayPlanner(t, tc.reply)
			_, err := a.DayPlan(context.Background(), dayReq())
			require.ErrorContains(t, err, tc.want)
		})
	}

	over := plan("null", block("t1", "14:00", 240))
	a, _ := dayPlanner(t, over)
	_, err := a.DayPlan(context.Background(), dayReq())
	require.NoError(t, err, "more than work_minutes is allowed; clients show it")
}

func TestClock(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]int{"00:00": 0, "09:05": 545, "23:59": 1439} {
		got, err := llm.Clock(s)
		require.NoError(t, err)
		require.Equal(t, want, got, s)
	}
	for _, s := range []string{"", "9:00", "24:00", "12:60", "+1:00", "12-00", "12:000"} {
		_, err := llm.Clock(s)
		require.Error(t, err, s)
	}
}
