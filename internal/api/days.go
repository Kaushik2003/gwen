package api

import (
	"context"
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

func (s *Server) listDays(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	days, err := s.Repos.Stats.Days(r.Context(), q.Get("from"), q.Get("to"), s.now())
	if err != nil {
		return err
	}
	out := wire.DayList{Days: make([]wire.DaySummary, len(days))}
	for i, d := range days {
		out.Days[i] = daySummaryWire(d)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) getDay(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	wd, err := s.Repos.WorkDays.GetByDay(ctx, r.PathValue("day"))
	if err != nil {
		return err
	}
	sum, err := s.Repos.Stats.DaySummary(ctx, wd.ID, s.now())
	if err != nil {
		return err
	}
	segs, err := s.Repos.Segments.ListByWorkDay(ctx, wd.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.DayDetail{WorkDay: workDayWire(wd), Summary: daySummaryWire(sum), Segments: segmentsWire(segs)})
	return nil
}

func (s *Server) patchDay(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchDayRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	wd, err := s.Repos.WorkDays.Update(r.Context(), r.PathValue("day"),
		store.WorkDayPatch{TargetSeconds: req.TargetSeconds, Note: req.Note, Rev: req.Rev})
	if err != nil {
		return err
	}
	s.dayChanged(r.Context(), wd)
	writeJSON(w, http.StatusOK, workDayWire(wd))
	return nil
}

// dayChanged sends day_changed, and state_changed when the day is the open
// one, whose totals Status carries.
func (s *Server) dayChanged(ctx context.Context, wd model.WorkDay) {
	s.Hub.Publish(wire.EventDayChanged, wire.DayChanged{Day: wd.Day})
	if wd.ClockedOutAt == nil {
		s.publishStatus(ctx)
	}
}

func (s *Server) dayOfSegment(ctx context.Context, g model.Segment) {
	if wd, err := s.Repos.WorkDays.Get(ctx, g.WorkDayID); err == nil {
		s.dayChanged(ctx, wd)
	}
}

func (s *Server) createSegment(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateSegmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.EndedAt == nil {
		return badRequest("ended_at", "ended_at is required")
	}
	cfg := s.Tracker.Config()
	g, err := s.Repos.Segments.Create(r.Context(), store.NewSegment{
		Day: req.Day, Kind: req.Kind, ProjectID: req.ProjectID, TaskID: req.TaskID,
		StartedAt: wire.Time(req.StartedAt), EndedAt: wire.TimePtr(req.EndedAt),
	}, store.DayDefaults{TZ: s.Loc.String(), TargetSeconds: int(cfg.Tracking.DailyTarget / time.Second)})
	if err != nil {
		return err
	}
	s.dayOfSegment(r.Context(), g)
	writeJSON(w, http.StatusCreated, segmentWire(g))
	return nil
}

// closedSegment loads a segment for a hand edit; the open segment belongs to
// the engine.
func (s *Server) closedSegment(ctx context.Context, id string) (model.Segment, error) {
	g, err := s.Repos.Segments.Get(ctx, id)
	if err != nil {
		return model.Segment{}, err
	}
	if g.EndedAt == nil {
		return model.Segment{}, &stateError{"the open segment is being tracked; switch or end it instead"}
	}
	return g, nil
}

func (s *Server) patchSegment(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchSegmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if _, err := s.closedSegment(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	g, err := s.Repos.Segments.Update(r.Context(), r.PathValue("id"), store.SegmentPatch{
		Kind:      req.Kind,
		ProjectID: nullable(req.ProjectID),
		TaskID:    nullable(req.TaskID),
		StartedAt: wire.TimePtr(req.StartedAt),
		EndedAt:   wire.TimePtr(req.EndedAt),
		Rev:       req.Rev,
	})
	if err != nil {
		return err
	}
	s.dayOfSegment(r.Context(), g)
	writeJSON(w, http.StatusOK, segmentWire(g))
	return nil
}

func (s *Server) splitSegment(w http.ResponseWriter, r *http.Request) error {
	var req wire.SplitSegmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if _, err := s.closedSegment(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	halves, err := s.Repos.Segments.Split(r.Context(), r.PathValue("id"), wire.Time(req.At))
	if err != nil {
		return err
	}
	s.dayOfSegment(r.Context(), halves[0])
	writeJSON(w, http.StatusOK, wire.SegmentList{Segments: segmentsWire(halves[:])})
	return nil
}

func (s *Server) deleteSegment(w http.ResponseWriter, r *http.Request) error {
	g, err := s.closedSegment(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := s.Repos.Segments.Delete(r.Context(), g.ID); err != nil {
		return err
	}
	s.dayOfSegment(r.Context(), g)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func nullable[T any](o wire.Optional[T]) store.Nullable[T] {
	return store.Nullable[T]{Set: o.Set, Value: o.Ptr()}
}
