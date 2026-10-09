package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// assistantDays is how many days from today a task's planned blocks are sent,
// and recentDays how many days up to today the time worked is.
const (
	assistantDays = 7
	recentDays    = 7
)

// assistantChat is POST /v1/assistant/chat: one message to the assistant,
// which answers and changes tasks, the plan, projects, and goals by itself.
func (s *Server) assistantChat(w http.ResponseWriter, r *http.Request) error {
	var req wire.AssistantChatRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	msg := strings.TrimSpace(req.Message)
	if n := utf8.RuneCountInString(msg); n < 1 || n > llm.MaxMessage {
		return badRequest("message", "a message is 1 to %d characters", llm.MaxMessage)
	}
	p, err := s.planner(r.Context())
	if err != nil {
		return err
	}
	ctx := r.Context()
	var history []wire.AssistantMessage
	if req.RunID != nil {
		if history, err = s.assistantHistory(ctx, *req.RunID); err != nil {
			return err
		}
	}
	now := s.now()
	history = append(history, wire.AssistantMessage{Role: llm.RoleUser, Text: msg, At: wire.Millis(now)})
	in, refs, err := s.assistantRequest(ctx, history)
	if err != nil {
		return err
	}
	out, err := p.Assistant(ctx, in)
	today := s.today()
	var run model.LLMRun
	if err != nil {
		slog.Warn("assistant failed", "provider", p.Name(), "err", err)
		run, err = s.Repos.LLMRuns.Create(ctx, model.RunAssistant, today, model.RunFailed, wire.RunError{Error: err.Error()})
	} else {
		results := s.applyActions(ctx, out.Actions, refs)
		// Her attitude only colours the reply; the reply and its changes stand without it.
		switched, serr := s.changeSelf(ctx, out.Attitude, out.SelfNote)
		if serr != nil {
			slog.Warn("assistant attitude not saved", "err", serr)
		}
		history = append(history, wire.AssistantMessage{Role: llm.RoleAssistant, Text: out.Reply,
			At: wire.Millis(s.now()), Mood: out.Mood, Attitude: switched, Actions: results})
		run, err = s.Repos.LLMRuns.Create(ctx, model.RunAssistant, today, model.RunOK,
			wire.AssistantOutput{Messages: history})
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, runWire(run))
	return nil
}

// getAssistantSelf is GET /v1/assistant/self.
func (s *Server) getAssistantSelf(w http.ResponseWriter, r *http.Request) error {
	self, err := s.assistantSelf(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, self)
	return nil
}

// resetAssistantSelf is DELETE /v1/assistant/self: the assistant forgets the
// attitude it chose and its note, and starts fresh.
func (s *Server) resetAssistantSelf(w http.ResponseWriter, r *http.Request) error {
	if err := store.DeleteLocal(r.Context(), s.DB.SQL(), store.KeyAssistantSelf); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// assistantSelf is how the assistant chose to treat the user, empty before
// it chose. A value that no longer reads is dropped, as if it never chose.
func (s *Server) assistantSelf(ctx context.Context) (wire.AssistantSelf, error) {
	var self wire.AssistantSelf
	v, err := store.GetLocal(ctx, s.DB.SQL(), store.KeyAssistantSelf)
	if errors.Is(err, store.ErrNotFound) {
		return self, nil
	}
	if err != nil {
		return self, err
	}
	if err := json.Unmarshal([]byte(v), &self); err != nil {
		slog.Warn("assistant self unreadable, starting fresh", "err", err)
		return wire.AssistantSelf{}, nil
	}
	return self, nil
}

// llmSelf is self as the prompts word it, with the day it was chosen.
func (s *Server) llmSelf(self wire.AssistantSelf) llm.Self {
	out := llm.Self{Attitude: self.Attitude, Note: self.Note}
	if self.Since != nil {
		out.Since = timeengine.ConfigFrom(s.Tracker.Config(), s.Loc).DayOf(time.UnixMilli(*self.Since))
	}
	return out
}

// changeSelf applies what a reply changed of how the assistant treats the
// user: a nil attitude or note keeps the one it had. It returns the attitude
// switched to, or "" when it kept its own.
func (s *Server) changeSelf(ctx context.Context, attitude, note *string) (string, error) {
	if attitude == nil && note == nil {
		return "", nil
	}
	self, err := s.assistantSelf(ctx)
	if err != nil {
		return "", err
	}
	was := self
	switched := ""
	if attitude != nil && *attitude != self.Attitude {
		switched, self.Attitude = *attitude, *attitude
		now := wire.Millis(s.now())
		self.Since = &now
	}
	if note != nil {
		self.Note = *note
	}
	if self.Attitude == was.Attitude && self.Note == was.Note {
		return "", nil
	}
	b, err := json.Marshal(self)
	if err != nil {
		return "", err
	}
	if err := store.SetLocal(ctx, s.DB.SQL(), store.KeyAssistantSelf, string(b), s.now()); err != nil {
		return "", err
	}
	slog.Info("assistant changed how it treats the user", "attitude", self.Attitude, "switched", switched != "")
	return switched, nil
}

// assistantHistory is the conversation of an answered assistant run.
func (s *Server) assistantHistory(ctx context.Context, runID string) ([]wire.AssistantMessage, error) {
	run, err := s.Repos.LLMRuns.Get(ctx, runID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, badRequest("run_id", "there is no run %s", runID)
	}
	if err != nil {
		return nil, err
	}
	if run.Kind != model.RunAssistant || run.Status != model.RunOK {
		return nil, badRequest("run_id", "run_id must be an assistant conversation that answered")
	}
	var out wire.AssistantOutput
	if err := json.Unmarshal(run.Output, &out); err != nil {
		return nil, fmt.Errorf("read run %s: %w", runID, err)
	}
	return out.Messages, nil
}

// refKind is what a ref names.
type refKind int

const (
	refTask refKind = iota
	refProject
	refGoal
)

type ref struct {
	kind  refKind
	id    string
	title string
}

// assistantRefs maps the refs sent, and the keys actions create, to rows.
type assistantRefs map[string]ref

func (m assistantRefs) get(r *string, kind refKind) (ref, error) {
	if r == nil || *r == "" {
		return ref{}, fmt.Errorf("no ref given")
	}
	got, ok := m[*r]
	if !ok || got.kind != kind {
		return ref{}, fmt.Errorf("%q is not a %s it knows", *r, [...]string{"task", "project", "goal"}[kind])
	}
	return got, nil
}

// assistantRequest builds what an assistant turn sends.
func (s *Server) assistantRequest(ctx context.Context, history []wire.AssistantMessage) (llm.AssistantRequest, assistantRefs, error) {
	env := s.planEnv()
	today := civil.MustParse(env.Today())
	now := s.now()
	refs := assistantRefs{}
	in := llm.AssistantRequest{Today: today.String(), Weekday: today.Weekday().String(), Now: s.dayClock(today, now),
		Methods: s.methods(), Busy: []llm.BusyTime{}, Free: []llm.Span{}, Projects: []llm.AssistantProject{},
		Goals: []llm.AssistantGoal{}, Tasks: []llm.AssistantTask{}, Messages: []llm.ChatMessage{}}

	dc, ch, err := s.Repos.Plans.DayPlan(ctx, today.String(), env)
	if err != nil {
		return in, nil, err
	}
	s.publishChanges(ch)
	at := func(t time.Time) string { return s.dayClock(today, t) }
	in.Hours = llm.Hours{Start: minuteClock(dc.Plan.Start), End: minuteClock(dc.Plan.End), WorkMinutes: dc.Plan.Capacity}
	for _, c := range dc.Commitments {
		if c.StartMinute != nil {
			start := today.At(*c.StartMinute, s.Loc)
			in.Busy = append(in.Busy, llm.BusyTime{Title: c.Title, Start: at(start), End: at(start.Add(c.Duration))})
		}
	}
	for _, b := range dc.Busy {
		in.Busy = append(in.Busy, llm.BusyTime{Title: cmp.Or(b.Title, "Busy"), Start: at(b.Start), End: at(b.End)})
	}
	for _, f := range dc.Free {
		if f.End.After(now) {
			in.Free = append(in.Free, llm.Span{Start: at(maxTime(f.Start, now)), End: at(f.End)})
		}
	}

	projects, err := s.Repos.Projects.List(ctx, wire.ArchivedFalse)
	if err != nil {
		return in, nil, err
	}
	projectRef := map[string]string{}
	for i, p := range projects {
		key := fmt.Sprintf("p%d", i+1)
		refs[key], projectRef[p.ID] = ref{refProject, p.ID, p.Name}, key
		in.Projects = append(in.Projects, llm.AssistantProject{Ref: key, Name: p.Name})
	}
	refOf := func(m map[string]string, id *string) *string {
		if id == nil {
			return nil
		}
		if r, ok := m[*id]; ok {
			return &r
		}
		return nil
	}
	goals, err := s.Repos.Goals.List(ctx, model.GoalActive)
	if err != nil {
		return in, nil, err
	}
	progress, err := s.Repos.Plans.Progress(ctx, goals, env)
	if err != nil {
		return in, nil, err
	}
	goalRef := map[string]string{}
	for i, g := range goals {
		key := fmt.Sprintf("g%d", i+1)
		refs[key], goalRef[g.ID] = ref{refGoal, g.ID, g.Title}, key
		pr := progress[g.ID]
		in.Goals = append(in.Goals, llm.AssistantGoal{Ref: key, Title: g.Title, Kind: g.Kind, Unit: g.Unit,
			Project: refOf(projectRef, g.ProjectID), StartDay: g.StartDay, DueDay: g.DueDay, Done: pr.Done,
			Remaining: pr.Remaining, Pace: pr.Pace, Specific: g.Specific, Measurable: g.Measurable,
			Assignable: g.Assignable, Realistic: g.Realistic})
	}

	tasks, err := s.Repos.Tasks.List(ctx, store.TaskFilter{Status: model.TaskOpen})
	if err != nil {
		return in, nil, err
	}
	tasks = slices.DeleteFunc(tasks, func(t model.Task) bool { return t.IsStep() })
	tasks = tasks[:min(len(tasks), llm.MaxAssistantTasks)]
	blocks := map[string][]llm.AssistantBlock{}
	for i := range assistantDays {
		d := today.AddDays(i)
		plan, err := s.Repos.Plans.Read(ctx, d.String(), env)
		if err != nil {
			return in, nil, err
		}
		for _, e := range plan.Items {
			if e.Item.Status != model.PlanPlanned {
				continue
			}
			var start *string
			if e.Item.StartAt != nil {
				c := s.dayClock(d, *e.Item.StartAt)
				start = &c
			}
			blocks[e.Task.ID] = append(blocks[e.Task.ID], llm.AssistantBlock{Day: d.String(), Start: start,
				Minutes: int(e.Item.Planned / time.Minute), Pinned: e.Item.Pinned})
		}
	}
	for i, t := range tasks {
		key := fmt.Sprintf("t%d", i+1)
		refs[key] = ref{refTask, t.ID, t.Title}
		at := llm.AssistantTask{Ref: key, Title: t.Title, Notes: truncate(t.Notes, llm.MaxAssistantNotes),
			Project: refOf(projectRef, t.ProjectID), Goal: refOf(goalRef, t.GoalID), Priority: t.Priority,
			DueDay: t.DueDay, StartDay: t.StartDay, EstimateMinutes: minutesOf(t.Estimate), Effort: t.Effort,
			Stage: t.Stage, DelegatedTo: t.DelegatedTo, Recurring: t.TemplateID != nil, Blocks: blocks[t.ID]}
		if t.StartMinute != nil {
			c := minuteClock(*t.StartMinute)
			at.StartTime = &c
		}
		if at.Blocks == nil {
			at.Blocks = []llm.AssistantBlock{}
		}
		in.Tasks = append(in.Tasks, at)
	}

	rv, err := s.Repos.Reviews.Latest(ctx, weekOf(today).AddDays(7).String())
	switch {
	case err == nil:
		in.Review = &llm.AssistantReview{WeekStart: rv.WeekStart, WentWell: rv.WentWell, WentBadly: rv.WentBadly,
			Energy: rv.Energy, Decisions: rv.Decisions, Improvements: rv.Improvements}
	case !errors.Is(err, store.ErrNotFound):
		return in, nil, err
	}

	days, err := s.Repos.Stats.Days(ctx, today.AddDays(-recentDays+1).String(), today.String(), now)
	if err != nil {
		return in, nil, err
	}
	in.Recent = []llm.AssistantDay{}
	for _, d := range days {
		in.Recent = append(in.Recent, llm.AssistantDay{Day: d.Day, WorkedMinutes: int(d.Worked / time.Minute),
			TargetMinutes: int(d.Target / time.Minute)})
	}

	for _, m := range history[max(0, len(history)-llm.MaxAssistantMessages):] {
		in.Messages = append(in.Messages, llm.ChatMessage{Role: m.Role, Text: m.Text})
	}
	return in, refs, nil
}

// methods are the planning methods as the LLM jobs see them.
func (s *Server) methods() llm.Methods {
	cfg := s.Tracker.Config().Planner
	m := llm.Methods{EatTheFrog: cfg.EatTheFrog}
	if p := cfg.Prime(); p != nil {
		m.PrimeTime = &llm.Span{Start: minuteClock(p[0]), End: minuteClock(p[1])}
	}
	return m
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// applyActions applies the assistant's actions in order. One that fails is
// reported and skipped; the rest go on. Today's plan is refreshed once at the
// end.
func (s *Server) applyActions(ctx context.Context, actions []llm.Action, refs assistantRefs) []wire.ActionResult {
	out := make([]wire.ActionResult, 0, len(actions))
	changed := false
	for _, a := range actions {
		res, err := s.applyAction(ctx, a, refs)
		res.Type = a.Type
		if err != nil {
			res.OK, res.Error = false, actionError(err)
			if res.Summary == "" {
				res.Summary = strings.ReplaceAll(a.Type, "_", " ")
			}
		} else {
			res.OK, changed = true, true
		}
		out = append(out, res)
	}
	if changed {
		s.replan(ctx)
		s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
		s.publishChanges(store.Changes{Goals: true})
	}
	return out
}

// actionError is why an action failed, as the user should read it.
func actionError(err error) string {
	var se *store.Error
	if errors.As(err, &se) {
		return se.Message
	}
	var re *requestError
	if errors.As(err, &re) {
		return re.msg
	}
	return userMessage(err)
}

func (s *Server) applyAction(ctx context.Context, a llm.Action, refs assistantRefs) (wire.ActionResult, error) {
	var res wire.ActionResult
	// link resolves a project or goal ref; "" clears it.
	link := func(r *string, kind refKind) (store.Nullable[string], error) {
		if r == nil {
			return store.Nullable[string]{}, nil
		}
		if *r == "" {
			return store.Nullable[string]{Set: true}, nil
		}
		got, err := refs.get(r, kind)
		if err != nil {
			return store.Nullable[string]{}, err
		}
		return store.Nullable[string]{Set: true, Value: &got.id}, nil
	}
	day := func(s *string) store.Nullable[string] {
		if s == nil {
			return store.Nullable[string]{}
		}
		if *s == "" {
			return store.Nullable[string]{Set: true}
		}
		return store.Nullable[string]{Set: true, Value: s}
	}
	positive := func(n *int) store.Nullable[int] {
		if n == nil {
			return store.Nullable[int]{}
		}
		if *n <= 0 {
			return store.Nullable[int]{Set: true}
		}
		return store.Nullable[int]{Set: true, Value: n}
	}
	text := func(s *string) string {
		if s == nil {
			return ""
		}
		return strings.TrimSpace(*s)
	}

	switch a.Type {
	case llm.ActCreateTask:
		res.Summary = fmt.Sprintf("Add %q", text(a.Title))
		project, err := link(a.Project, refProject)
		if err != nil {
			return res, err
		}
		goal, err := link(a.Goal, refGoal)
		if err != nil {
			return res, err
		}
		t, ch, err := s.Repos.Tasks.Create(ctx, store.NewTask{Title: text(a.Title), Notes: a.Notes,
			ProjectID: project.Value, GoalID: goal.Value, Priority: a.Priority, DueDay: day(a.DueDay).Value,
			StartDay: day(a.StartDay).Value, EstimateMinutes: positive(a.EstimateMinutes).Value,
			Effort: positive(a.Effort).Value, Stage: a.Stage, DelegatedTo: a.DelegatedTo})
		if err != nil {
			return res, err
		}
		s.publishChanges(ch)
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{t.ID}})
		if a.Key != nil && *a.Key != "" {
			refs[*a.Key] = ref{refTask, t.ID, t.Title}
		}
		res.Summary, res.TaskID = fmt.Sprintf("Added %q", t.Title)+s.taskDetail(ctx, t), &t.ID
		return res, nil

	case llm.ActUpdateTask:
		got, err := refs.get(a.Ref, refTask)
		if err != nil {
			return res, err
		}
		res.Summary, res.TaskID = fmt.Sprintf("Change %q", got.title), &got.id
		project, err := link(a.Project, refProject)
		if err != nil {
			return res, err
		}
		goal, err := link(a.Goal, refGoal)
		if err != nil {
			return res, err
		}
		var title *string
		if t := text(a.Title); t != "" {
			title = &t
		}
		t, err := s.Repos.Tasks.Update(ctx, got.id, store.TaskPatch{Title: title, Notes: a.Notes,
			Priority: a.Priority, ProjectID: project, GoalID: goal, DueDay: day(a.DueDay), StartDay: day(a.StartDay),
			EstimateMinutes: positive(a.EstimateMinutes), Effort: positive(a.Effort), Stage: a.Stage,
			DelegatedTo: a.DelegatedTo})
		if err != nil {
			return res, err
		}
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{t.ID}})
		res.Summary = fmt.Sprintf("Updated %q", t.Title) + s.changeDetail(ctx, a, t)
		return res, nil

	case llm.ActScheduleTask:
		got, err := refs.get(a.Ref, refTask)
		if err != nil {
			return res, err
		}
		res.Summary, res.TaskID = fmt.Sprintf("Schedule %q", got.title), &got.id
		d := civil.MustParse(s.today())
		if a.Day != nil && *a.Day != "" {
			if d, err = civil.Parse(*a.Day); err != nil {
				return res, fmt.Errorf("%q is not a day like 2026-10-05", *a.Day)
			}
		}
		var start *int64
		if a.Start != nil && *a.Start != "" {
			m, err := llm.Clock(*a.Start)
			if err != nil {
				return res, err
			}
			ms := d.At(m, s.Loc).UnixMilli()
			start = &ms
		}
		minutes := 30
		if a.Minutes != nil {
			minutes = *a.Minutes
		} else if t, err := s.Repos.Tasks.Get(ctx, got.id); err == nil && t.Estimate != nil {
			minutes = min(int(*t.Estimate/time.Minute), store.MaxScheduled)
		}
		e, err := s.schedule(ctx, got.id, d.String(), start, minutes)
		if err != nil {
			return res, err
		}
		res.Summary = fmt.Sprintf("Scheduled %q %s", got.title, s.blockText(d, e.Item))
		return res, nil

	case llm.ActUnschedule:
		got, err := refs.get(a.Ref, refTask)
		if err != nil {
			return res, err
		}
		res.Summary, res.TaskID = fmt.Sprintf("Take %q off the plan", got.title), &got.id
		d := ""
		if a.Day != nil {
			d = *a.Day
		}
		ch, err := s.Repos.Plans.Unschedule(ctx, got.id, d, s.planEnv())
		if err != nil {
			return res, err
		}
		s.publishChanges(ch)
		res.Summary = fmt.Sprintf("Took %q off the plan", got.title)
		if d != "" {
			res.Summary += " " + s.dayText(civil.MustParse(d))
		}
		return res, nil

	case llm.ActCompleteTask:
		got, err := refs.get(a.Ref, refTask)
		if err != nil {
			return res, err
		}
		res.Summary, res.TaskID = fmt.Sprintf("Complete %q", got.title), &got.id
		ch, err := s.Repos.Tasks.Complete(ctx, got.id, nil)
		if err != nil {
			return res, err
		}
		s.publishChanges(ch.Changes)
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{got.id}})
		res.Summary = fmt.Sprintf("Completed %q", got.title)
		return res, nil

	case llm.ActCreateProject:
		name := text(a.Name)
		if name == "" {
			name = text(a.Title)
		}
		res.Summary = fmt.Sprintf("Add the project %q", name)
		p, err := s.Repos.Projects.Create(ctx, name, nil)
		if err != nil {
			return res, err
		}
		if a.Key != nil && *a.Key != "" {
			refs[*a.Key] = ref{refProject, p.ID, p.Name}
		}
		res.Summary, res.ProjectID = fmt.Sprintf("Added the project %q", p.Name), &p.ID
		return res, nil

	case llm.ActCreateGoal:
		res.Summary = fmt.Sprintf("Add the goal %q", text(a.Title))
		project, err := link(a.Project, refProject)
		if err != nil {
			return res, err
		}
		start := s.today()
		if a.StartDay != nil && *a.StartDay != "" {
			start = *a.StartDay
		}
		due := ""
		if a.DueDay != nil {
			due = *a.DueDay
		}
		g, ch, err := s.Repos.Goals.Create(ctx, store.NewGoal{Title: text(a.Title), Kind: model.GoalTasks,
			ProjectID: project.Value, StartDay: start, DueDay: due,
			SMART: store.SMART{Specific: text(a.Specific), Measurable: text(a.Measurable),
				Assignable: text(a.Assignable), Realistic: text(a.Realistic)}})
		if err != nil {
			return res, err
		}
		s.publishChanges(ch)
		if a.Key != nil && *a.Key != "" {
			refs[*a.Key] = ref{refGoal, g.ID, g.Title}
		}
		res.Summary, res.GoalID = fmt.Sprintf("Added the goal %q, due %s", g.Title,
			s.dayText(civil.MustParse(g.DueDay))), &g.ID
		return res, nil

	case llm.ActUpdateGoal:
		got, err := refs.get(a.Ref, refGoal)
		if err != nil {
			return res, err
		}
		res.Summary, res.GoalID = fmt.Sprintf("Change the goal %q", got.title), &got.id
		var title, due *string
		if t := text(a.Title); t != "" {
			title = &t
		}
		if a.DueDay != nil && *a.DueDay != "" {
			due = a.DueDay
		}
		g, err := s.Repos.Goals.Update(ctx, got.id, store.GoalPatch{Title: title, DueDay: due, Specific: a.Specific,
			Measurable: a.Measurable, Assignable: a.Assignable, Realistic: a.Realistic})
		if err != nil {
			return res, err
		}
		res.Summary = fmt.Sprintf("Updated the goal %q", g.Title)
		return res, nil
	}
	return res, fmt.Errorf("%q is not an action", a.Type)
}

// dayText is "today", "tomorrow", or "on Thu 8 Oct".
func (s *Server) dayText(d civil.Day) string {
	today := civil.MustParse(s.today())
	switch d {
	case today:
		return "today"
	case today.AddDays(1):
		return "tomorrow"
	}
	return "on " + d.Midnight(s.Loc).Format("Mon 2 Jan")
}

// blockText is "today 2:00 pm–3:00 pm" or "tomorrow, 45m".
func (s *Server) blockText(d civil.Day, it model.PlanItem) string {
	if it.StartAt == nil {
		return fmt.Sprintf("%s, %dm", s.dayText(d), int(it.Planned/time.Minute))
	}
	start := it.StartAt.In(s.Loc)
	return fmt.Sprintf("%s %s–%s", s.dayText(d), start.Format("3:04 pm"), start.Add(it.Planned).Format("3:04 pm"))
}

// taskDetail is a new task's project and due day, for its summary.
func (s *Server) taskDetail(ctx context.Context, t model.Task) string {
	var parts []string
	if t.ProjectID != nil {
		if p, err := s.Repos.Projects.Get(ctx, *t.ProjectID); err == nil {
			parts = append(parts, "in "+p.Name)
		}
	}
	if t.DueDay != nil {
		parts = append(parts, "due "+s.dayText(civil.MustParse(*t.DueDay)))
	}
	if t.Stage != model.StageTodo {
		parts = append(parts, "to "+t.Stage)
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

// changeDetail says what an update changed.
func (s *Server) changeDetail(ctx context.Context, a llm.Action, t model.Task) string {
	var parts []string
	if a.Project != nil {
		name := "no project"
		if t.ProjectID != nil {
			if p, err := s.Repos.Projects.Get(ctx, *t.ProjectID); err == nil {
				name = p.Name
			}
		}
		parts = append(parts, "moved to "+name)
	}
	if a.Stage != nil {
		parts = append(parts, "now "+t.Stage)
	}
	if a.DelegatedTo != nil && t.DelegatedTo != "" {
		parts = append(parts, "handed to "+t.DelegatedTo)
	}
	if a.DueDay != nil {
		if t.DueDay == nil {
			parts = append(parts, "no due day")
		} else {
			parts = append(parts, "due "+s.dayText(civil.MustParse(*t.DueDay)))
		}
	}
	if a.Priority != nil {
		parts = append(parts, fmt.Sprintf("priority %d", t.Priority))
	}
	if a.Effort != nil && t.Effort != nil {
		parts = append(parts, [...]string{"", "easy", "medium", "hard"}[*t.Effort])
	}
	if a.EstimateMinutes != nil && t.Estimate != nil {
		parts = append(parts, fmt.Sprintf("about %dm", int(*t.Estimate/time.Minute)))
	}
	if len(parts) == 0 {
		return ""
	}
	return ": " + strings.Join(parts, ", ")
}
