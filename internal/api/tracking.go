package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// command serves a tracking command that takes {}.
func (s *Server) command(build func(at time.Time) timeengine.Input) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var body struct{}
		if err := decode(r, &body); err != nil {
			return err
		}
		return s.do(w, r, build)
	}
}

func (s *Server) do(w http.ResponseWriter, r *http.Request, build func(at time.Time) timeengine.Input) error {
	st, err := s.Tracker.Do(r.Context(), build)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (s *Server) clockIn(w http.ResponseWriter, r *http.Request) error {
	var req wire.ClockInRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	project, task, err := s.attribution(r.Context(), req.ProjectID, req.TaskID)
	if err != nil {
		return err
	}
	return s.do(w, r, func(at time.Time) timeengine.Input {
		return timeengine.ClockIn{ProjectID: project, TaskID: task, At: at}
	})
}

func (s *Server) switchTo(w http.ResponseWriter, r *http.Request) error {
	var req wire.SwitchRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	project, task, err := s.attribution(r.Context(), req.ProjectID, req.TaskID)
	if err != nil {
		return err
	}
	return s.do(w, r, func(at time.Time) timeengine.Input {
		return timeengine.Switch{ProjectID: project, TaskID: task, At: at}
	})
}

// attribution checks that the ids exist and agree. A task given without a
// project brings its own project.
func (s *Server) attribution(ctx context.Context, project, task *string) (*string, *string, error) {
	if task != nil {
		t, err := s.Repos.Tasks.Get(ctx, *task)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, store.FailField(store.ErrNotFound, "task_id", "no task %s", *task)
		}
		if err != nil {
			return nil, nil, err
		}
		switch {
		case project == nil:
			project = t.ProjectID
		case t.ProjectID != nil && *t.ProjectID != *project:
			return nil, nil, badRequest("task_id", "the task belongs to another project")
		}
	}
	if project != nil {
		_, err := s.Repos.Projects.Get(ctx, *project)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, store.FailField(store.ErrNotFound, "project_id", "no project %s", *project)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	return project, task, nil
}

// publishStatus sends state_changed after an edit that changes the open work
// day's totals.
func (s *Server) publishStatus(ctx context.Context) {
	if st, err := s.Tracker.Status(ctx); err == nil {
		s.Hub.Publish(wire.EventStateChanged, st)
	}
}

func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.Notify.Test(r.Context()))
	return nil
}
