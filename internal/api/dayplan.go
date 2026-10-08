package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// planChat is POST /v1/plan/chat: one message of a day plan conversation
// (docs/07-integrations.md#day-plan).
func (s *Server) planChat(w http.ResponseWriter, r *http.Request) error {
	var req wire.PlanChatRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Day == "" {
		return badRequest("day", "day is required")
	}
	msg := strings.TrimSpace(req.Message)
	if n := utf8.RuneCountInString(msg); n < 1 || n > llm.MaxMessage {
		return badRequest("message", "a message is 1 to %d characters", llm.MaxMessage)
	}
	p, err := s.planner()
	if err != nil {
		return err
	}
	ctx := r.Context()
	var prev *wire.DayPlanOutput
	if req.RunID != nil {
		if prev, err = s.conversation(ctx, *req.RunID, req.Day); err != nil {
			return err
		}
	}
	dc, ch, err := s.Repos.Plans.DayPlan(ctx, req.Day, s.planEnv())
	if err != nil {
		return err
	}
	s.publishChanges(ch)
	in, byRef, err := s.dayPlanRequest(ctx, dc, prev, msg)
	if err != nil {
		return err
	}
	out, err := p.DayPlan(ctx, in)
	var run model.LLMRun
	if err != nil {
		slog.Warn("day plan failed", "provider", p.Name(), "err", err)
		run, err = s.Repos.LLMRuns.Create(ctx, model.RunDayPlan, req.Day, model.RunFailed, wire.RunError{Error: err.Error()})
	} else {
		run, err = s.Repos.LLMRuns.Create(ctx, model.RunDayPlan, req.Day, model.RunOK,
			s.dayPlanWire(out, in, byRef, civil.MustParse(req.Day)))
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, runWire(run))
	return nil
}

// conversation is the output of the run a message continues: a day plan of
// day that answered, holding fewer than the most user messages.
func (s *Server) conversation(ctx context.Context, runID, day string) (*wire.DayPlanOutput, error) {
	run, err := s.Repos.LLMRuns.Get(ctx, runID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, badRequest("run_id", "there is no run %s", runID)
	}
	if err != nil {
		return nil, err
	}
	if run.Kind != model.RunDayPlan || run.SubjectID != day || (run.Status != model.RunOK && run.Status != model.RunAccepted) {
		return nil, badRequest("run_id", "run_id must be a day plan of %s that answered", day)
	}
	var out wire.DayPlanOutput
	if err := json.Unmarshal(run.Output, &out); err != nil {
		return nil, fmt.Errorf("read run %s: %w", runID, err)
	}
	users := 0
	for _, m := range out.Messages {
		if m.Role == llm.RoleUser {
			users++
		}
	}
	if users >= llm.MaxUserMessages {
		return nil, badRequest("message", "a conversation holds %d messages; start a new one", llm.MaxUserMessages)
	}
	return &out, nil
}

// dayPlanRequest builds what a day plan sends, and the sent tasks by ref.
func (s *Server) dayPlanRequest(ctx context.Context, dc store.DayPlanContext, prev *wire.DayPlanOutput, msg string) (llm.DayPlanRequest, map[string]store.Candidate, error) {
	d := civil.MustParse(dc.Plan.Day)
	at := func(t time.Time) string { return s.dayClock(d, t) }
	in := llm.DayPlanRequest{Day: d.String(), Weekday: d.Weekday().String(), Methods: s.methods(),
		Hours: llm.Hours{Start: minuteClock(dc.Plan.Start), End: minuteClock(dc.Plan.End), WorkMinutes: dc.Plan.Capacity},
		Busy:  []llm.BusyTime{}, Floating: []llm.Floating{}, Free: []llm.Span{}, Done: []llm.DoneBlock{},
		Tasks: []llm.PlanTask{}, Plan: []llm.Block{}, Messages: []llm.ChatMessage{}}
	if d.String() == s.today() {
		now := at(s.now())
		in.Now = &now
	}
	for _, c := range dc.Commitments {
		if c.StartMinute == nil {
			in.Floating = append(in.Floating, llm.Floating{Title: c.Title, Minutes: int(c.Duration / time.Minute)})
			continue
		}
		start := d.At(*c.StartMinute, s.Loc)
		in.Busy = append(in.Busy, llm.BusyTime{Title: c.Title, Start: at(start), End: at(start.Add(c.Duration))})
	}
	for _, b := range dc.Busy {
		in.Busy = append(in.Busy, llm.BusyTime{Title: cmp.Or(b.Title, "Busy"), Start: at(b.Start), End: at(b.End)})
	}
	slices.SortStableFunc(in.Busy, func(a, b llm.BusyTime) int { return cmp.Compare(a.Start, b.Start) })
	for _, f := range dc.Free {
		in.Free = append(in.Free, llm.Span{Start: at(f.Start), End: at(f.End)})
	}
	for _, e := range dc.Plan.Items {
		if e.Item.Status == model.PlanDone {
			in.Done = append(in.Done, llm.DoneBlock{Title: e.Task.Title, Start: s.clockPtr(d, e.Item.StartAt),
				Minutes: int(e.Item.Planned / time.Minute)})
		}
	}

	projects, err := s.Repos.Projects.List(ctx, "all")
	if err != nil {
		return llm.DayPlanRequest{}, nil, err
	}
	names := map[string]string{}
	for _, p := range projects {
		names[p.ID] = p.Name
	}
	byRef, refOf := map[string]store.Candidate{}, map[string]string{}
	for i, c := range dc.Candidates[:min(len(dc.Candidates), llm.MaxPlanTasks)] {
		t := c.Task
		pt := llm.PlanTask{Ref: fmt.Sprintf("t%d", i+1), Title: t.Title, Notes: truncate(t.Notes, llm.MaxPlanNotes),
			Priority: t.Priority, DueDay: t.DueDay, EstimateMinutes: minutesOf(t.Estimate),
			RemainingMinutes: c.Remaining, Urgency: math.Round(c.Urgency*100) / 100, Effort: t.Effort, OpenSteps: []string{},
			Skipped: c.Skipped}
		if t.ProjectID != nil {
			if name, ok := names[*t.ProjectID]; ok {
				pt.Project = &name
			}
		}
		if c.Goal != nil {
			pt.Goal = &c.Goal.Title
		}
		if c.Pace != "" {
			pt.Pace = &c.Pace
		}
		for _, st := range c.OpenSteps {
			pt.OpenSteps = append(pt.OpenSteps, st.Title)
		}
		in.Tasks = append(in.Tasks, pt)
		byRef[pt.Ref] = c
		refOf[t.ID] = pt.Ref
	}

	if prev != nil {
		for _, b := range prev.Items {
			if ref, ok := refOf[b.TaskID]; ok {
				start := at(time.UnixMilli(b.StartAt))
				in.Plan = append(in.Plan, llm.Block{Ref: ref, Start: &start, Minutes: b.PlannedMinutes})
			}
		}
		for _, m := range prev.Messages {
			in.Messages = append(in.Messages, llm.ChatMessage{Role: m.Role, Text: m.Text})
		}
	} else {
		for _, e := range dc.Plan.Items {
			if ref, ok := refOf[e.Task.ID]; ok && e.Item.Status == model.PlanPlanned {
				in.Plan = append(in.Plan, llm.Block{Ref: ref, Start: s.clockPtr(d, e.Item.StartAt),
					Minutes: int(e.Item.Planned / time.Minute)})
			}
		}
	}
	in.Messages = append(in.Messages, llm.ChatMessage{Role: llm.RoleUser, Text: msg})
	return in, byRef, nil
}

// dayPlanWire is the stored output of an ok day plan: the conversation with
// the reply, the blocks resolved to their tasks in start order, and the change
// of hours.
func (s *Server) dayPlanWire(out llm.DayPlanOutput, in llm.DayPlanRequest, byRef map[string]store.Candidate, d civil.Day) wire.DayPlanOutput {
	res := wire.DayPlanOutput{Items: []wire.ProposedBlock{}}
	for _, m := range in.Messages {
		res.Messages = append(res.Messages, wire.ChatMessage{Role: m.Role, Text: m.Text})
	}
	res.Messages = append(res.Messages, wire.ChatMessage{Role: llm.RoleAssistant, Text: strings.TrimSpace(out.Reply)})
	for _, b := range out.Items {
		t := byRef[b.Ref].Task
		m, _ := llm.Clock(b.Start) // validated
		res.Items = append(res.Items, wire.ProposedBlock{TaskID: t.ID, Title: t.Title, ProjectID: t.ProjectID,
			StartAt: d.At(m, s.Loc).UnixMilli(), PlannedMinutes: b.Minutes})
	}
	slices.SortStableFunc(res.Items, func(a, b wire.ProposedBlock) int { return cmp.Compare(a.StartAt, b.StartAt) })
	if h := out.Hours; h != nil {
		res.Hours = &wire.DayHours{WorkMinutes: h.WorkMinutes}
		if h.Start != nil {
			m, _ := llm.Clock(*h.Start)
			res.Hours.StartMinute = &m
		}
	}
	return res
}

// dayClock is t as HH:MM on day d, clamped to the day.
func (s *Server) dayClock(d civil.Day, t time.Time) string {
	switch {
	case t.Before(d.Midnight(s.Loc)):
		return "00:00"
	case !t.Before(d.AddDays(1).Midnight(s.Loc)):
		return "23:59"
	}
	return t.In(s.Loc).Format("15:04")
}

func (s *Server) clockPtr(d civil.Day, t *time.Time) *string {
	if t == nil {
		return nil
	}
	c := s.dayClock(d, *t)
	return &c
}

// minuteClock writes minutes after midnight as HH:MM.
func minuteClock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// truncate cuts s to at most n characters.
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
