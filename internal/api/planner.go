package api

import (
	"context"
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// NewPlanEnv is the planner's environment at now under the configuration.
func NewPlanEnv(cfg config.Config, loc *time.Location, now time.Time) store.PlanEnv {
	return store.PlanEnv{
		Now: now, Loc: loc, DayOf: timeengine.ConfigFrom(cfg, loc).DayOf,
		DayStart: int(cfg.Planner.DayStart), DayEnd: int(cfg.Planner.DayEnd), Buffer: cfg.Planner.Buffer,
		DailyTarget: cfg.Tracking.DailyTarget,
	}
}

func (s *Server) planEnv() store.PlanEnv { return NewPlanEnv(s.Tracker.Config(), s.Loc, s.now()) }

// PublishChanges sends the events for what a planner write touched.
func PublishChanges(h *Hub, ch store.Changes) {
	if len(ch.TaskIDs) > 0 {
		h.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: ch.TaskIDs})
	}
	for _, day := range ch.PlanDays {
		h.Publish(wire.EventPlanChanged, wire.PlanChanged{Day: day})
	}
	if ch.Goals {
		h.Publish(wire.EventGoalsChanged, wire.GoalsChanged{})
	}
}

func (s *Server) publishChanges(ch store.Changes) { PublishChanges(s.Hub, ch) }

// goalsWire attaches today's progress to goals.
func (s *Server) goalsWire(ctx context.Context, goals []model.Goal) ([]wire.Goal, error) {
	progress, err := s.Repos.Plans.Progress(ctx, goals, s.planEnv())
	if err != nil {
		return nil, err
	}
	out := make([]wire.Goal, len(goals))
	for i, g := range goals {
		out[i] = goalWire(g, progress[g.ID])
	}
	return out, nil
}

func (s *Server) writeGoal(w http.ResponseWriter, r *http.Request, status int, g model.Goal) error {
	gs, err := s.goalsWire(r.Context(), []model.Goal{g})
	if err != nil {
		return err
	}
	writeJSON(w, status, gs[0])
	return nil
}

func (s *Server) listGoals(w http.ResponseWriter, r *http.Request) error {
	goals, err := s.Repos.Goals.List(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		return err
	}
	out, err := s.goalsWire(r.Context(), goals)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.GoalList{Goals: out})
	return nil
}

func (s *Server) createGoal(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateGoalRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	g, ch, err := s.Repos.Goals.Create(r.Context(), store.NewGoal{
		Title: req.Title, Kind: req.Kind, Unit: req.Unit, TargetQuantity: req.TargetQuantity,
		MinutesPerUnit: req.MinutesPerUnit, ProjectID: req.ProjectID, StartDay: req.StartDay, DueDay: req.DueDay,
	})
	if err != nil {
		return err
	}
	s.publishChanges(ch)
	return s.writeGoal(w, r, http.StatusCreated, g)
}

func (s *Server) getGoal(w http.ResponseWriter, r *http.Request) error {
	g, err := s.Repos.Goals.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return s.writeGoal(w, r, http.StatusOK, g)
}

func (s *Server) patchGoal(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchGoalRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	g, err := s.Repos.Goals.Update(r.Context(), r.PathValue("id"), store.GoalPatch{
		Title: req.Title, Kind: req.Kind, Unit: req.Unit, TargetQuantity: nullable(req.TargetQuantity),
		MinutesPerUnit: nullable(req.MinutesPerUnit), ProjectID: nullable(req.ProjectID), StartDay: req.StartDay,
		DueDay: req.DueDay, Status: req.Status, Rev: req.Rev,
	})
	if err != nil {
		return err
	}
	s.publishChanges(store.Changes{Goals: true})
	return s.writeGoal(w, r, http.StatusOK, g)
}

func (s *Server) deleteGoal(w http.ResponseWriter, r *http.Request) error {
	ch, err := s.Repos.Goals.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if len(ch.TaskIDs) > 0 {
		ch.PlanDays = append(ch.PlanDays, s.today()) // their items leave today's plan
	}
	s.publishChanges(ch)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) listCommitments(w http.ResponseWriter, r *http.Request) error {
	cs, err := s.Repos.Commitments.List(r.Context())
	if err != nil {
		return err
	}
	out := wire.CommitmentList{Commitments: make([]wire.Commitment, len(cs))}
	for i, c := range cs {
		out.Commitments[i] = commitmentWire(c)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createCommitment(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateCommitmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	c, err := s.Repos.Commitments.Create(r.Context(), store.NewCommitment{
		Title: req.Title, ProjectID: req.ProjectID, RRule: req.RRule, StartMinute: req.StartMinute,
		DurationMinutes: req.DurationMinutes, CountsTowardTarget: req.CountsTowardTarget,
		ActiveFrom: req.ActiveFrom, ActiveUntil: req.ActiveUntil,
	})
	if err != nil {
		return err
	}
	s.publishChanges(store.Changes{Goals: true})
	writeJSON(w, http.StatusCreated, commitmentWire(c))
	return nil
}

func (s *Server) patchCommitment(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchCommitmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	c, err := s.Repos.Commitments.Update(r.Context(), r.PathValue("id"), store.CommitmentPatch{
		Title: req.Title, ProjectID: nullable(req.ProjectID), RRule: req.RRule, StartMinute: nullable(req.StartMinute),
		DurationMinutes: req.DurationMinutes, CountsTowardTarget: req.CountsTowardTarget, ActiveFrom: req.ActiveFrom,
		ActiveUntil: nullable(req.ActiveUntil), Rev: req.Rev,
	})
	if err != nil {
		return err
	}
	s.publishChanges(store.Changes{Goals: true})
	writeJSON(w, http.StatusOK, commitmentWire(c))
	return nil
}

func (s *Server) deleteCommitment(w http.ResponseWriter, r *http.Request) error {
	if err := s.Repos.Commitments.Delete(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	s.publishChanges(store.Changes{Goals: true})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// entriesWire converts plan entries, with each task's tracked time.
func (s *Server) entriesWire(ctx context.Context, entries ...[]store.PlanEntry) ([][]wire.PlanItem, error) {
	var taskIDs []string
	for _, es := range entries {
		for _, e := range es {
			taskIDs = append(taskIDs, e.Task.ID)
		}
	}
	tracked, err := s.Repos.Tasks.Tracked(ctx, taskIDs, s.now())
	if err != nil {
		return nil, err
	}
	out := make([][]wire.PlanItem, len(entries))
	for i, es := range entries {
		out[i] = make([]wire.PlanItem, len(es))
		for j, e := range es {
			out[i][j] = planItemWire(e, tracked[e.Task.ID])
		}
	}
	return out, nil
}

func (s *Server) writePlan(w http.ResponseWriter, r *http.Request, p store.Plan, ch store.Changes) error {
	s.publishChanges(ch)
	items, err := s.entriesWire(r.Context(), p.Items)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.Plan{Day: p.Day, CapacityMinutes: p.Capacity, PlannedMinutes: p.Planned, Items: items[0]})
	return nil
}

func (s *Server) getPlan(w http.ResponseWriter, r *http.Request) error {
	p, ch, err := s.Repos.Plans.Get(r.Context(), r.URL.Query().Get("day"), s.planEnv())
	if err != nil {
		return err
	}
	return s.writePlan(w, r, p, ch)
}

func (s *Server) generatePlan(w http.ResponseWriter, r *http.Request) error {
	var req wire.GeneratePlanRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Day == "" {
		return badRequest("day", "day is required")
	}
	p, ch, err := s.Repos.Plans.Generate(r.Context(), req.Day, s.planEnv())
	if err != nil {
		return err
	}
	return s.writePlan(w, r, p, ch)
}

func (s *Server) patchPlanItem(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchPlanItemRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	e, err := s.Repos.Plans.UpdateItem(r.Context(), r.PathValue("id"), store.PlanItemPatch{
		StartAt:  store.Nullable[time.Time]{Set: req.StartAt.Set, Value: wire.TimePtr(req.StartAt.Ptr())},
		Position: req.Position, Pinned: req.Pinned, PlannedMinutes: req.PlannedMinutes, Status: req.Status, Rev: req.Rev,
	})
	if err != nil {
		return err
	}
	s.publishChanges(store.Changes{PlanDays: []string{e.Item.Day}})
	items, err := s.entriesWire(r.Context(), []store.PlanEntry{e})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items[0][0])
	return nil
}

func (s *Server) briefing(w http.ResponseWriter, r *http.Request) error {
	b, ch, err := s.Repos.Plans.Briefing(r.Context(), s.planEnv())
	if err != nil {
		return err
	}
	s.publishChanges(ch)
	lists, err := s.entriesWire(r.Context(), b.Pending, b.Today)
	if err != nil {
		return err
	}
	out := wire.Briefing{Day: b.Day, Pending: lists[0], Today: lists[1], Reminders: []wire.Reminder{},
		Goals: make([]wire.Goal, len(b.Goals))}
	for _, rm := range b.Reminders {
		out.Reminders = append(out.Reminders, wire.Reminder{TaskID: rm.TaskID, Title: rm.Title,
			DueDay: rm.DueDay.String(), DaysLeft: rm.DaysLeft, Message: rm.Message})
	}
	for i, g := range b.Goals {
		out.Goals[i] = goalWire(g.Goal, g.Progress)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
