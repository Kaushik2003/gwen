package api

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

var graphDay = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)

func clockOn(hm string) time.Time {
	t, err := time.Parse("15:04", hm)
	if err != nil {
		panic(err)
	}
	return graphDay.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute)
}

// graphFx builds a day: tasks by id, and their blocks.
type graphFx struct {
	in graphInput
}

func newGraphFx(now string) *graphFx {
	return &graphFx{in: graphInput{Day: "2026-09-15", Now: clockOn(now), Loc: time.UTC, From: clockOn("09:00"),
		To: clockOn("18:00"), Tasks: map[string]wire.Task{}}}
}

func (f *graphFx) task(id string, minutes int, waits ...string) *graphFx {
	f.in.Tasks[id] = wire.Task{ID: id, Title: id, Status: model.TaskOpen, Stage: model.StageTodo,
		EstimateMinutes: &minutes, WaitsOn: waits}
	return f
}

func (f *graphFx) block(id, start string, minutes int) *graphFx {
	at := clockOn(start).UnixMilli()
	f.in.Plan.Items = append(f.in.Plan.Items, wire.PlanItem{ID: "item-" + id + start, TaskID: id, StartAt: &at,
		PlannedMinutes: minutes, Status: model.PlanPlanned, Task: f.in.Tasks[id]})
	return f
}

func (f *graphFx) tracking(id, since string) *graphFx {
	f.in.Segments = append(f.in.Segments, wire.Segment{Kind: model.KindWork, TaskID: &id, StartedAt: clockOn(since).UnixMilli()})
	return f
}

func (f *graphFx) build(t *testing.T) (wire.DayGraph, map[string]wire.GraphTask) {
	t.Helper()
	g := buildDayGraph(f.in)
	by := map[string]wire.GraphTask{}
	for _, gt := range g.Tasks {
		by[gt.ID] = gt
	}
	return g, by
}

func TestDayGraphReady(t *testing.T) {
	t.Parallel()
	g, by := newGraphFx("08:00").task("api", 60).task("app", 60, "api").
		block("api", "09:00", 60).block("app", "10:00", 60).build(t)
	require.Equal(t, wire.GraphReady, by["api"].State)
	require.Equal(t, wire.GraphReady, by["app"].State)
	require.Equal(t, []string{"api"}, by["app"].WaitsOn)
	require.Equal(t, []string{"app"}, by["api"].Blocks)
	require.Empty(t, g.Issues)
	require.Equal(t, []string{"api", "app"}, g.Critical)
	require.Equal(t, clockOn("11:00").UnixMilli(), *by["app"].ProjectedEnd)
}

func TestDayGraphLatenessCascades(t *testing.T) {
	t.Parallel()
	// API was planned 09:00–10:00 but you are still on it at 10:00 with 40
	// minutes left, so App, planned right after, moves 40 minutes too.
	f := newGraphFx("10:00").task("api", 100).task("app", 60, "api").
		block("api", "09:00", 60).block("app", "10:00", 60).tracking("api", "09:00")
	f.in.Tasks["api"] = withTracked(f.in.Tasks["api"], 60*time.Minute)
	g, by := f.build(t)
	require.Equal(t, wire.GraphWorking, by["api"].State)
	require.Equal(t, "You're on it now, heading 40 min past its plan", by["api"].Reason)
	require.Equal(t, wire.GraphAtRisk, by["app"].State)
	require.Equal(t, 40, by["app"].SlipMinutes)
	require.Equal(t, "Starts after “api”, which now ends at 10:40 am: 40 min later than planned", by["app"].Reason)
	require.Len(t, g.Issues, 1)
	require.Equal(t, "app", g.Issues[0].TaskID)
	require.Equal(t, "api", g.Issues[0].OtherID)
}

func withTracked(t wire.Task, d time.Duration) wire.Task {
	t.TrackedMs = d.Milliseconds()
	return t
}

func TestDayGraphBlocked(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(f *graphFx)
		state string
		why   string
	}{
		{"what it waits on isn't planned", func(f *graphFx) {}, wire.GraphBlocked,
			"Waits on “review”, which isn't on today's plan"},
		{"what it waits on is with someone", func(f *graphFx) {
			r := f.in.Tasks["review"]
			r.Stage, r.DelegatedTo = model.StageWaiting, "Sam"
			f.in.Tasks["review"] = r
		}, wire.GraphBlocked, "Waits on “review”, which is with Sam"},
		{"Claude needs you on what it waits on", func(f *graphFx) {
			f.in.Spans = []store.AgentSpan{{SessionID: "s1", State: store.CodeNeedsYou, Need: store.NeedPermission,
				TaskID: "review", SessionTask: "review", StartedAt: clockOn("09:30"), Cwd: "/w/app"}}
			f.in.Live = []wire.CodeSession{{ID: "s1", Name: "app", Cwd: "/w/app", TaskID: "review", State: store.CodeNeedsYou,
				Waiting: &wire.CodeWaiting{Summary: "npm test"}}}
		}, wire.GraphWaitingOnYou, "Waits on “review”, where Claude needs your answer"},
		{"Claude is on what it waits on", func(f *graphFx) {
			f.in.Live = []wire.CodeSession{{ID: "s1", Name: "app", Cwd: "/w/app", TaskID: "review", State: store.CodeWorking,
				StartedAt: clockOn("09:30").UnixMilli()}}
		}, wire.GraphReady, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGraphFx("10:00").task("review", 30).task("ship", 60, "review").block("ship", "11:00", 60)
			tc.setup(f)
			g, by := f.build(t)
			require.Equal(t, tc.state, by["ship"].State)
			if tc.why != "" {
				require.Equal(t, tc.why, by["ship"].Reason)
				require.Equal(t, "ship: "+lowerFirst(tc.why), g.Issues[0].Text)
			}
			require.Contains(t, by, "review", "what it waits on is in the graph")
		})
	}
}

func TestDayGraphClaudeOnItIsHeadedForAnEnd(t *testing.T) {
	t.Parallel()
	f := newGraphFx("10:00").task("review", 30).task("ship", 60, "review").block("ship", "10:15", 60)
	f.in.Live = []wire.CodeSession{{ID: "s1", Name: "app", Cwd: "/w/app", TaskID: "review", State: store.CodeWorking,
		StartedAt: clockOn("09:30").UnixMilli()}}
	_, by := f.build(t)
	require.Equal(t, wire.GraphAgent, by["review"].State)
	require.Equal(t, "Claude is working on it", by["review"].Reason)
	require.Equal(t, "s1", by["review"].SessionID)
	require.Equal(t, clockOn("10:30").UnixMilli(), *by["review"].ProjectedEnd, "now plus what is left of it")
	require.Equal(t, wire.GraphAtRisk, by["ship"].State, "it starts 15 minutes later")
}

func TestDayGraphRunsPastTheDay(t *testing.T) {
	t.Parallel()
	_, by := newGraphFx("10:00").task("late", 60).block("late", "17:30", 60).build(t)
	require.Equal(t, wire.GraphAtRisk, by["late"].State)
	require.Equal(t, "Runs past the end of your day (6:00 pm)", by["late"].Reason)
}

func TestDayGraphShowsWhatWaitsOnThePlan(t *testing.T) {
	t.Parallel()
	_, by := newGraphFx("10:00").task("api", 60).task("docs", 30, "api").block("api", "11:00", 60).build(t)
	require.Equal(t, wire.GraphUnplanned, by["docs"].State)
	require.Equal(t, "Not on today's plan; it can start after “api”", by["docs"].Reason)
}

func TestDayGraphDoneBlocksNothing(t *testing.T) {
	t.Parallel()
	f := newGraphFx("10:00").task("api", 60).task("app", 60, "api").block("app", "10:00", 60)
	api := f.in.Tasks["api"]
	done := clockOn("09:40").UnixMilli()
	api.Status, api.DoneAt = model.TaskDone, &done
	f.in.Tasks["api"] = api
	_, by := f.build(t)
	require.Equal(t, wire.GraphDone, by["api"].State)
	require.Equal(t, wire.GraphReady, by["app"].State)
}

func TestDayGraphAgentLanes(t *testing.T) {
	t.Parallel()
	f := newGraphFx("10:00").task("api", 60).block("api", "11:00", 60)
	end := clockOn("09:20")
	f.in.Spans = []store.AgentSpan{
		{SessionID: "s1", State: store.CodeWorking, StartedAt: clockOn("09:00"), EndedAt: &end, Cwd: "/w/app/web", SessionTask: "api"},
		{SessionID: "s1", State: store.CodeNeedsYou, Need: store.NeedQuestion, StartedAt: end, Cwd: "/w/app/web", SessionTask: "api"},
	}
	f.in.Live = []wire.CodeSession{{ID: "s1", Name: "web", Cwd: "/w/app/web", State: store.CodeNeedsYou,
		Waiting: &wire.CodeWaiting{Question: "Which port?"}}}
	f.in.Projects = []model.Project{{ID: "p-app", Folder: "/w/app"}, {ID: "p-web", Folder: "/w/app/web"}, {ID: "p-x", Folder: "/w/ap"}}
	g, by := f.build(t)
	require.Len(t, g.Agents, 1)
	a := g.Agents[0]
	require.Equal(t, "web", a.Name)
	require.Equal(t, "p-web", *a.ProjectID, "the deepest folder wins")
	require.Equal(t, "api", a.TaskID)
	require.Equal(t, "Which port?", a.Question)
	require.Len(t, a.Spans, 2)
	require.Nil(t, a.Spans[1].EndAt)
	require.Equal(t, wire.GraphWaitingOnYou, by["api"].State)
	require.Equal(t, "Claude is waiting for you: Which port?", by["api"].Reason)
	require.Equal(t, wire.GraphWaitingOnYou, g.Issues[0].Kind)
}

func TestDayGraphShowsWorkHeldOffThePlan(t *testing.T) {
	t.Parallel()
	f := newGraphFx("10:00").task("review", 30).task("post", 60, "review").task("api", 60).block("api", "11:00", 60)
	r := f.in.Tasks["review"]
	r.Stage, r.DelegatedTo = model.StageWaiting, "Sam"
	f.in.Tasks["review"] = r
	g, by := f.build(t)
	require.Equal(t, wire.GraphBlocked, by["post"].State)
	require.Equal(t, "Waits on “review”, which is with Sam", by["post"].Reason)
	require.Equal(t, wire.GraphUnplanned, by["review"].State)
	require.Equal(t, "post", g.Issues[0].TaskID)
	require.Empty(t, g.Critical, "no chain on the plan")
}
