package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) error {
	ps, err := s.Repos.Projects.List(r.Context(), r.URL.Query().Get("archived"))
	if err != nil {
		return err
	}
	out := wire.ProjectList{Projects: make([]wire.Project, len(ps))}
	for i, p := range ps {
		out.Projects[i] = projectWire(p)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateProjectRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	p, err := s.Repos.Projects.Create(r.Context(), req.Name, req.Color)
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	writeJSON(w, http.StatusCreated, projectWire(p))
	return nil
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.Repos.Projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, projectWire(p))
	return nil
}

// tracking reports whether the open segment is attributed to the project or
// task: those cannot be archived or deleted.
func (s *Server) tracking(ctx context.Context, projectID, taskID string) (bool, error) {
	snap, err := s.Tracker.Snapshot(ctx)
	if err != nil || snap.Segment == nil {
		return false, err
	}
	seg := snap.Segment
	return (projectID != "" && seg.ProjectID != nil && *seg.ProjectID == projectID) ||
		(taskID != "" && seg.TaskID != nil && *seg.TaskID == taskID), nil
}

func (s *Server) patchProject(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchProjectRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	id := r.PathValue("id")
	if req.Archived != nil && *req.Archived {
		busy, err := s.tracking(r.Context(), id, "")
		if err != nil {
			return err
		}
		if busy {
			return &stateError{"cannot archive the project being tracked"}
		}
	}
	p, err := s.Repos.Projects.Update(r.Context(), id,
		store.ProjectPatch{Name: req.Name, Color: req.Color, Archived: req.Archived, Rev: req.Rev})
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	writeJSON(w, http.StatusOK, projectWire(p))
	return nil
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) error {
	ctx, id := r.Context(), r.PathValue("id")
	busy, err := s.tracking(ctx, id, "")
	if err != nil {
		return err
	}
	if busy {
		return &stateError{"cannot delete the project being tracked"}
	}
	tasks, err := s.Repos.Tasks.List(ctx, store.TaskFilter{ProjectID: id, Status: "all"})
	if err != nil {
		return err
	}
	if err := s.Repos.Projects.Delete(ctx, id); err != nil {
		return err
	}
	s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	if len(tasks) > 0 {
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: taskIDs(tasks)})
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func taskIDs(ts []model.Task) []string {
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	return ids
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	templates := false
	switch q.Get("templates") {
	case "", "false":
	case "true":
		templates = true
	default:
		return badRequest("templates", "templates must be true or false")
	}
	ts, err := s.Repos.Tasks.List(r.Context(), store.TaskFilter{
		ProjectID: q.Get("project_id"), GoalID: q.Get("goal_id"), Status: q.Get("status"),
		DueBefore: q.Get("due_before"), Templates: templates,
	})
	if err != nil {
		return err
	}
	tracked, err := s.Repos.Tasks.Tracked(r.Context(), taskIDs(ts), s.now())
	if err != nil {
		return err
	}
	out := wire.TaskList{Tasks: make([]wire.Task, len(ts))}
	for i, t := range ts {
		out.Tasks[i] = taskWire(t, tracked[t.ID])
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// writeTask answers with one task and its tracked time.
func (s *Server) writeTask(w http.ResponseWriter, r *http.Request, status int, t model.Task) error {
	tracked, err := s.Repos.Tasks.Tracked(r.Context(), []string{t.ID}, s.now())
	if err != nil {
		return err
	}
	writeJSON(w, status, taskWire(t, tracked[t.ID]))
	return nil
}

func (s *Server) taskChanged(w http.ResponseWriter, r *http.Request, status int, t model.Task, err error) error {
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{t.ID}})
	return s.writeTask(w, r, status, t)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateTaskRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	t, err := s.Repos.Tasks.Create(r.Context(), store.NewTask{
		ProjectID: req.ProjectID, Title: req.Title, Notes: req.Notes, Priority: req.Priority,
		DueDay: req.DueDay, EstimateMinutes: req.EstimateMinutes, GoalID: req.GoalID, Quantity: req.Quantity,
		RRule: req.RRule,
	})
	return s.taskChanged(w, r, http.StatusCreated, t, err)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) error {
	t, err := s.Repos.Tasks.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return s.writeTask(w, r, http.StatusOK, t)
}

func (s *Server) patchTask(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchTaskRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	t, err := s.Repos.Tasks.Update(r.Context(), r.PathValue("id"), store.TaskPatch{
		ProjectID: nullable(req.ProjectID), Title: req.Title, Notes: req.Notes, Priority: req.Priority,
		DueDay: nullable(req.DueDay), EstimateMinutes: nullable(req.EstimateMinutes),
		GoalID: nullable(req.GoalID), Quantity: nullable(req.Quantity), RRule: nullable(req.RRule), Rev: req.Rev,
	})
	return s.taskChanged(w, r, http.StatusOK, t, err)
}

func (s *Server) completeTask(w http.ResponseWriter, r *http.Request) error {
	var req wire.CompleteTaskRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ch, err := s.Repos.Tasks.Complete(r.Context(), r.PathValue("id"), req.QuantityDone)
	if err != nil {
		return err
	}
	s.publishChanges(ch.Changes)
	return s.taskChanged(w, r, http.StatusOK, ch.Task, nil)
}

func (s *Server) reopenTask(w http.ResponseWriter, r *http.Request) error {
	var req struct{}
	if err := decode(r, &req); err != nil {
		return err
	}
	ch, err := s.Repos.Tasks.Reopen(r.Context(), r.PathValue("id"), s.today())
	if err != nil {
		return err
	}
	s.publishChanges(ch.Changes)
	return s.taskChanged(w, r, http.StatusOK, ch.Task, nil)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	busy, err := s.tracking(r.Context(), "", id)
	if err != nil {
		return err
	}
	if busy {
		return &stateError{"cannot delete the task being tracked"}
	}
	if err := s.Repos.Tasks.Delete(r.Context(), id); err != nil {
		return err
	}
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{id}})
	s.Hub.Publish(wire.EventPlanChanged, wire.PlanChanged{Day: s.today()}) // its items leave today's plan
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) statsSummary(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	sum, err := s.Repos.Stats.Summary(r.Context(), q.Get("from"), q.Get("to"), s.today(), s.now())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.StatsSummary{
		From: sum.From, To: sum.To, WorkedMs: sum.Worked.Milliseconds(), BreakMs: sum.Break.Milliseconds(),
		DaysTracked: sum.DaysTracked, DaysTargetMet: sum.DaysTargetMet, AvgWorkedMs: sum.AvgWorked.Milliseconds(),
		ByProject: projectTotalsWire(sum.ByProject), CurrentStreak: sum.CurrentStreak, LongestStreak: sum.LongestStreak,
	})
	return nil
}

func (s *Server) statsHeatmap(w http.ResponseWriter, r *http.Request) error {
	year := s.now().In(s.Loc).Year()
	if v := r.URL.Query().Get("year"); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil {
			return badRequest("year", "year must be a number like 2026")
		}
		year = y
	}
	days, err := s.Repos.Stats.Heatmap(r.Context(), year, s.now())
	if err != nil {
		return err
	}
	out := wire.Heatmap{Year: year, Days: make([]wire.HeatmapDay, len(days))}
	for i, d := range days {
		out.Days[i] = wire.HeatmapDay{Day: d.Day, WorkedMs: d.Worked.Milliseconds()}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
