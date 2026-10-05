package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// schedule puts a task on a day at the time the user chose and sends the
// events.
func (s *Server) schedule(ctx context.Context, taskID, day string, startAt *int64, minutes int) (store.PlanEntry, error) {
	e, ch, err := s.Repos.Plans.Schedule(ctx, taskID, day, wire.TimePtr(startAt), minutes, s.planEnv())
	if err != nil {
		return store.PlanEntry{}, err
	}
	s.publishChanges(ch)
	return e, nil
}

func (s *Server) scheduleTask(w http.ResponseWriter, r *http.Request) error {
	var req wire.ScheduleRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.TaskID == "" {
		return badRequest("task_id", "task_id is required")
	}
	if req.Day == "" {
		return badRequest("day", "day is required")
	}
	e, err := s.schedule(r.Context(), req.TaskID, req.Day, req.StartAt, req.PlannedMinutes)
	if err != nil {
		return err
	}
	items, err := s.entriesWire(r.Context(), []store.PlanEntry{e})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, items[0][0])
	return nil
}

func (s *Server) unscheduleTask(w http.ResponseWriter, r *http.Request) error {
	var req wire.UnscheduleRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.TaskID == "" {
		return badRequest("task_id", "task_id is required")
	}
	ch, err := s.Repos.Plans.Unschedule(r.Context(), req.TaskID, req.Day, s.planEnv())
	if err != nil {
		return err
	}
	s.publishChanges(ch)
	days := ch.PlanDays
	if days == nil {
		days = []string{}
	}
	writeJSON(w, http.StatusOK, wire.Unscheduled{Days: days})
	return nil
}

func reviewWire(rv model.WeeklyReview) wire.WeeklyReview {
	return wire.WeeklyReview{ID: rv.ID, WeekStart: rv.WeekStart, WentWell: rv.WentWell, WentBadly: rv.WentBadly,
		Energy: rv.Energy, Decisions: rv.Decisions, Improvements: rv.Improvements, Checklist: rv.Checklist,
		UpdatedAt: wire.Millis(rv.UpdatedAt)}
}

func (s *Server) getReview(w http.ResponseWriter, r *http.Request) error {
	week := r.PathValue("week")
	rv, err := s.Repos.Reviews.Get(r.Context(), week)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusOK, wire.WeeklyReview{WeekStart: week})
		return nil
	case err != nil:
		return err
	}
	writeJSON(w, http.StatusOK, reviewWire(rv))
	return nil
}

func (s *Server) saveReview(w http.ResponseWriter, r *http.Request) error {
	var req wire.SaveReviewRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	rv, err := s.Repos.Reviews.Save(r.Context(), store.ReviewInput{WeekStart: r.PathValue("week"),
		WentWell: req.WentWell, WentBadly: req.WentBadly, Energy: req.Energy, Decisions: req.Decisions,
		Improvements: req.Improvements, Checklist: req.Checklist})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, reviewWire(rv))
	return nil
}

// Energy report window and the check-ins a suggestion needs.
const (
	DefaultEnergyDays = 21 // three weeks, as Work the System suggests
	MaxEnergyDays     = 120
	minEnergyLogs     = 6
	primeHours        = 3 // the length of a suggested prime time
)

func (s *Server) logEnergy(w http.ResponseWriter, r *http.Request) error {
	var req wire.LogEnergyRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	at := s.now()
	if req.At != nil {
		at = time.UnixMilli(*req.At)
	}
	e, err := s.Repos.Energy.Log(r.Context(), at, req.Level)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, wire.EnergyLog{ID: e.ID, At: wire.Millis(e.At), Level: e.Level})
	return nil
}

func (s *Server) deleteEnergy(w http.ResponseWriter, r *http.Request) error {
	if err := s.Repos.Energy.Delete(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) energyReport(w http.ResponseWriter, r *http.Request) error {
	days := DefaultEnergyDays
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxEnergyDays {
			return badRequest("days", "days must be 1 to %d", MaxEnergyDays)
		}
		days = n
	}
	now := s.now()
	logs, err := s.Repos.Energy.List(r.Context(), now.AddDate(0, 0, -days), now.Add(time.Minute))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.energyWire(logs, days))
	return nil
}

// energyWire averages the check-ins by local hour and suggests the best
// window of primeHours whole hours as the prime time.
func (s *Server) energyWire(logs []model.EnergyLog, days int) wire.EnergyReport {
	out := wire.EnergyReport{Days: days, Logs: make([]wire.EnergyLog, len(logs)), Hours: []wire.EnergyHour{}}
	var sum [24]float64
	var count [24]int
	for i, e := range logs {
		out.Logs[i] = wire.EnergyLog{ID: e.ID, At: wire.Millis(e.At), Level: e.Level}
		h := e.At.In(s.Loc).Hour()
		sum[h] += float64(e.Level)
		count[h]++
	}
	for h := range 24 {
		if count[h] > 0 {
			out.Hours = append(out.Hours, wire.EnergyHour{Hour: h, Count: count[h],
				Average: math.Round(sum[h]/float64(count[h])*100) / 100})
		}
	}
	if len(logs) < minEnergyLogs {
		return out
	}
	best, bestAt := -1.0, -1
	for start := 0; start+primeHours <= 24; start++ {
		var total float64
		n := 0
		for h := start; h < start+primeHours; h++ {
			if count[h] > 0 {
				total += sum[h] / float64(count[h])
				n++
			}
		}
		if n < 2 { // a window needs check-ins in most of its hours
			continue
		}
		if avg := total / float64(n); avg > best {
			best, bestAt = avg, start
		}
	}
	if bestAt >= 0 {
		start, end := fmt.Sprintf("%02d:00", bestAt), fmt.Sprintf("%02d:00", (bestAt+primeHours)%24)
		if bestAt+primeHours == 24 {
			end = "23:59"
		}
		out.SuggestedStart, out.SuggestedEnd = &start, &end
	}
	return out
}

// weekOf is the Monday of the week holding day.
func weekOf(day civil.Day) civil.Day {
	return day.AddDays(-((int(day.Weekday()) + 6) % 7))
}
