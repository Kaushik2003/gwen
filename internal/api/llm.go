package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// planner builds the LLM adapter, or unavailable when none is configured.
func (s *Server) planner() (llm.Planner, error) {
	if s.LLM == nil {
		return nil, Unavailable("the LLM adapter is not running in this daemon")
	}
	p, err := s.LLM(s.Tracker.Config().LLM)
	if errors.Is(err, llm.ErrUnavailable) {
		return nil, Unavailable("%s", err.Error())
	}
	return p, err
}

func runWire(r model.LLMRun) wire.LlmRun {
	return wire.LlmRun{ID: r.ID, Kind: r.Kind, SubjectID: r.SubjectID, Status: r.Status, Output: r.Output,
		CreatedAt: wire.Millis(r.CreatedAt), UpdatedAt: wire.Millis(r.UpdatedAt)}
}

func (s *Server) breakdown(w http.ResponseWriter, r *http.Request) error {
	var req wire.BreakdownRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	p, err := s.planner()
	if err != nil {
		return err
	}
	ctx := r.Context()
	g, err := s.Repos.Goals.Get(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	linked, err := s.Repos.Tasks.List(ctx, store.TaskFilter{GoalID: g.ID, Status: "all"})
	if err != nil {
		return err
	}
	in := llm.BreakdownRequest{Title: g.Title, Kind: g.Kind, Unit: g.Unit, TargetQuantity: g.TargetQuantity,
		MinutesPerUnit: minutesOf(g.PerUnit), StartDay: g.StartDay, DueDay: g.DueDay,
		ExistingTasks: []string{}, Instructions: req.Instructions, Today: s.today()}
	for _, t := range linked {
		in.ExistingTasks = append(in.ExistingTasks, t.Title)
	}
	out, err := p.Breakdown(ctx, in)
	var run model.LLMRun
	if err != nil {
		slog.Warn("breakdown failed", "provider", p.Name(), "err", err)
		run, err = s.Repos.LLMRuns.Create(ctx, model.RunBreakdown, g.ID, model.RunFailed, wire.RunError{Error: err.Error()})
	} else {
		run, err = s.Repos.LLMRuns.Create(ctx, model.RunBreakdown, g.ID, model.RunOK, out)
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, runWire(run))
	return nil
}

func (s *Server) retro(w http.ResponseWriter, r *http.Request) error {
	var req wire.RetroRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	start, err := civil.Parse(req.WeekStart)
	if err != nil {
		return badRequest("week_start", "week_start must be a date like 2026-09-14")
	}
	p, err := s.planner()
	if err != nil {
		return err
	}
	in, err := s.retroData(r.Context(), start)
	if err != nil {
		return err
	}
	out, err := p.Retro(r.Context(), in)
	if err != nil {
		slog.Warn("retro fell back to the rules summary", "provider", p.Name(), "err", err)
	}
	run, err := s.Repos.LLMRuns.Create(r.Context(), model.RunRetro, start.String(), model.RunOK, out)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, runWire(run))
	return nil
}

// retroData gathers the week from start (docs/07-integrations.md#retro).
func (s *Server) retroData(ctx context.Context, start civil.Day) (llm.RetroRequest, error) {
	end := start.AddDays(6)
	in := llm.RetroRequest{WeekStart: start.String(), Projects: []llm.RetroProject{}, Goals: []llm.RetroGoal{}}
	days, err := s.Repos.Stats.Days(ctx, start.String(), end.String(), s.now())
	if err != nil {
		return in, err
	}
	byDay := map[string]store.DaySummary{}
	for _, d := range days {
		byDay[d.Day] = d
	}
	for d := start; !d.After(end); d = d.AddDays(1) {
		sum := byDay[d.String()]
		in.Days = append(in.Days, llm.RetroDay{Day: d.String(), WorkedMin: int(sum.Worked / time.Minute),
			BreakMin: int(sum.Break / time.Minute), TargetMin: int(sum.Target / time.Minute), TargetMet: sum.TargetMet})
	}
	sum, err := s.Repos.Stats.Summary(ctx, start.String(), end.String(), s.today(), s.now())
	if err != nil {
		return in, err
	}
	for _, p := range projectTotalsWire(sum.ByProject) {
		in.Projects = append(in.Projects, llm.RetroProject{Name: p.Name, WorkedMin: int(p.WorkedMs / 60_000)})
	}
	if in.Completed, err = s.Repos.LLMRuns.Completed(ctx, start.Midnight(s.Loc), end.AddDays(1).Midnight(s.Loc)); err != nil {
		return in, err
	}
	goals, err := s.Repos.Goals.List(ctx, model.GoalActive)
	if err != nil {
		return in, err
	}
	progress, err := s.Repos.Plans.Progress(ctx, goals, s.planEnv())
	if err != nil {
		return in, err
	}
	for _, g := range goals {
		p := progress[g.ID]
		in.Goals = append(in.Goals, llm.RetroGoal{Title: g.Title, Done: p.Done, Remaining: p.Remaining, Unit: g.Unit,
			RequiredPerDay: p.RequiredPerDay, ActualPerDay: p.ActualPerDay, Pace: p.Pace, DueDay: g.DueDay})
	}
	in.PlannedMinutes, in.DoneMinutes, err = s.Repos.LLMRuns.PlanMinutes(ctx, start.String(), end.String())
	return in, err
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.planner(); err != nil {
		return err
	}
	run, err := s.Repos.LLMRuns.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, runWire(run))
	return nil
}

func (s *Server) acceptRun(w http.ResponseWriter, r *http.Request) error {
	var req wire.AcceptRunRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if _, err := s.planner(); err != nil {
		return err
	}
	tasks, err := s.Repos.LLMRuns.Accept(r.Context(), r.PathValue("id"), req.Indexes)
	if err != nil {
		return err
	}
	s.publishChanges(store.Changes{TaskIDs: taskIDs(tasks), Goals: true})
	out := wire.TaskList{Tasks: make([]wire.Task, len(tasks))}
	for i, t := range tasks {
		out.Tasks[i] = taskWire(t, 0)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) rejectRun(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	if _, err := s.planner(); err != nil {
		return err
	}
	run, err := s.Repos.LLMRuns.Reject(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, runWire(run))
	return nil
}
