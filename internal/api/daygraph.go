package api

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// The agentic timeline (plan.md 3.17): a day's plan as a graph of what waits
// on what. Each task gets where it is heading now, from what is tracked, what
// the agents are doing, and what it waits on; what is blocked or will run
// late says why, in a sentence the window, the phone, and Gwen all show.

// atRiskMinutes is how far past its plan a task must be heading to be at
// risk: less is the day's usual wobble.
const atRiskMinutes = 10

// graphInput is everything buildDayGraph reads.
type graphInput struct {
	Day      string
	Now      time.Time
	Loc      *time.Location
	From, To time.Time // the planner's day
	Plan     wire.Plan
	// Tasks are the tasks the graph may show, by id: the open ones, the
	// plan's, and the done ones they wait on.
	Tasks    map[string]wire.Task
	Segments []wire.Segment
	Spans    []store.AgentSpan
	Live     []wire.CodeSession
	Projects []model.Project
}

func (s *Server) getDayGraph(w http.ResponseWriter, r *http.Request) error {
	g, err := s.dayGraph(r.Context(), r.PathValue("day"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, g)
	return nil
}

// dayGraph loads a day and builds its graph.
func (s *Server) dayGraph(ctx context.Context, day string) (wire.DayGraph, error) {
	d, err := civil.Parse(day)
	if err != nil {
		return wire.DayGraph{}, badRequest("day", "day must be a date like 2026-09-15")
	}
	p, ch, err := s.Repos.Plans.Get(ctx, day, s.planEnv())
	if err != nil {
		return wire.DayGraph{}, err
	}
	s.publishChanges(ch)
	entries, err := s.entriesWire(ctx, p.Items)
	if err != nil {
		return wire.DayGraph{}, err
	}
	plan := planWire(p, entries[0])

	open, err := s.Repos.Tasks.List(ctx, store.TaskFilter{Status: model.TaskOpen})
	if err != nil {
		return wire.DayGraph{}, err
	}
	byID := map[string]model.Task{}
	for _, t := range open {
		byID[t.ID] = t
	}
	for _, it := range p.Items {
		if _, ok := byID[it.Task.ID]; !ok {
			byID[it.Task.ID] = it.Task
		}
	}
	// What they wait on that is done (or open, as when a step is waited on).
	var missing []string
	for _, t := range byID {
		for _, id := range t.WaitsOn {
			if _, ok := byID[id]; !ok && !slices.Contains(missing, id) {
				missing = append(missing, id)
			}
		}
	}
	for _, id := range missing {
		if t, err := s.Repos.Tasks.Get(ctx, id); err == nil {
			byID[t.ID] = t
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	tracked, err := s.Repos.Tasks.Tracked(ctx, ids, s.now())
	if err != nil {
		return wire.DayGraph{}, err
	}
	tasks := make(map[string]wire.Task, len(byID))
	for id, t := range byID {
		tasks[id] = taskWire(t, tracked[id])
	}

	var segs []wire.Segment
	if wd, err := s.Repos.WorkDays.GetByDay(ctx, day); err == nil {
		if list, err := s.Repos.Segments.ListByWorkDay(ctx, wd.ID); err == nil {
			segs = segmentsWire(list)
		}
	}
	cfg := s.Tracker.Config()
	dayStart := d.At(int(cfg.Tracking.DayRollover), s.Loc)
	spans, err := s.Repos.CodeSessions.Spans(ctx, dayStart, dayStart.Add(24*time.Hour))
	if err != nil {
		return wire.DayGraph{}, err
	}
	var live []wire.CodeSession
	if s.Code != nil {
		live = s.Code.Sessions().Sessions
	}
	projects, err := s.Repos.Projects.List(ctx, "false")
	if err != nil {
		return wire.DayGraph{}, err
	}
	from, to := d.At(p.Start, s.Loc), d.At(p.End, s.Loc)
	if !to.After(from) {
		to = to.Add(24 * time.Hour)
	}
	return buildDayGraph(graphInput{Day: day, Now: s.now(), Loc: s.Loc, From: from, To: to, Plan: plan, Tasks: tasks,
		Segments: segs, Spans: spans, Live: live, Projects: projects}), nil
}

// projection is where a task is heading.
type projection struct {
	ok         bool // start and end are known
	start, end time.Time
	// blockedBy is the open task in its way that has no end in sight; ""
	// when none is.
	blockedBy string
	// pushedBy is what it waits on whose end moved its start later than
	// planned; last is what it waits on that ends last.
	pushedBy, last string
}

func buildDayGraph(in graphInput) wire.DayGraph {
	if in.Loc == nil {
		in.Loc = time.Local
	}
	now := in.Now
	out := wire.DayGraph{Day: in.Day, Now: now.UnixMilli(), From: in.From.UnixMilli(), To: in.To.UnixMilli(),
		Tasks: []wire.GraphTask{}, Agents: []wire.AgentLane{}, Issues: []wire.GraphIssue{}, Critical: []string{}}
	tasks := in.Tasks
	if tasks == nil {
		tasks = map[string]wire.Task{}
	}

	// The plan's blocks by task, and the tasks in the order they first show.
	blocks := map[string][]wire.GraphBlock{}
	var order []string
	for _, it := range in.Plan.Items {
		if it.Status == model.PlanSkipped {
			continue
		}
		if _, ok := tasks[it.TaskID]; !ok {
			tasks[it.TaskID] = it.Task
		}
		if _, ok := blocks[it.TaskID]; !ok {
			order = append(order, it.TaskID)
		}
		blocks[it.TaskID] = append(blocks[it.TaskID], wire.GraphBlock{ItemID: it.ID, StartAt: it.StartAt,
			Minutes: it.PlannedMinutes, Status: it.Status, Pinned: it.Pinned})
	}
	for id := range blocks {
		slices.SortStableFunc(blocks[id], func(a, b wire.GraphBlock) int {
			return cmp.Compare(deref64(a.StartAt), deref64(b.StartAt))
		})
	}

	lanes := agentLanes(in)
	out.Agents = lanes
	// The session on each task: one that needs you wins, then one working.
	sessionOf := map[string]wire.AgentLane{}
	rank := func(state string) int {
		switch state {
		case store.CodeNeedsYou:
			return 3
		case store.CodeWorking, store.CodeStarting:
			return 2
		case store.CodeEnded:
			return 0
		}
		return 1
	}
	for _, l := range lanes {
		if l.TaskID == "" {
			continue
		}
		if have, ok := sessionOf[l.TaskID]; !ok || rank(l.State) > rank(have.State) {
			sessionOf[l.TaskID] = l
		}
	}

	// Who is in the graph: the plan's tasks, the agents' tasks, everything
	// open they wait on in turn, and the open tasks that wait on them.
	include := map[string]bool{}
	var queue []string
	add := func(id string) {
		if t, ok := tasks[id]; ok && !include[id] {
			include[id] = true
			order = appendOnce(order, id)
			if t.Status == model.TaskOpen {
				queue = append(queue, id) // what a done task waited on no longer matters
			}
		}
	}
	for _, id := range slices.Clone(order) {
		add(id)
	}
	for _, l := range lanes {
		if l.TaskID != "" && l.State != store.CodeEnded {
			add(l.TaskID)
		}
	}
	// Open work kept off the day by what it waits on: the very thing worth
	// seeing. Earliest made first, so the order stays put.
	var held []string
	for id, t := range tasks {
		if t.Status != model.TaskOpen || include[id] {
			continue
		}
		if slices.ContainsFunc(t.WaitsOn, func(w string) bool { o, ok := tasks[w]; return ok && o.Status == model.TaskOpen }) {
			held = append(held, id)
		}
	}
	slices.SortFunc(held, func(a, b string) int {
		return cmp.Or(cmp.Compare(tasks[a].CreatedAt, tasks[b].CreatedAt), cmp.Compare(a, b))
	})
	for _, id := range held {
		add(id)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, w := range tasks[id].WaitsOn {
			add(w)
		}
	}
	core := maps.Clone(include)
	var waiting []string
	for id, t := range tasks {
		if include[id] || t.Status != model.TaskOpen {
			continue
		}
		if slices.ContainsFunc(t.WaitsOn, func(w string) bool { return core[w] }) {
			waiting = append(waiting, id)
		}
	}
	slices.SortFunc(waiting, func(a, b string) int {
		return cmp.Or(cmp.Compare(tasks[a].CreatedAt, tasks[b].CreatedAt), cmp.Compare(a, b))
	})
	for _, id := range waiting {
		add(id)
	}

	working := map[string]time.Time{}
	for _, sg := range in.Segments {
		if sg.EndedAt == nil && sg.Kind == model.KindWork && sg.TaskID != nil {
			working[*sg.TaskID] = time.UnixMilli(sg.StartedAt)
		}
	}

	open := func(id string) bool { return include[id] && tasks[id].Status == model.TaskOpen }
	waitsOf := func(id string) []string {
		var ws []string
		for _, w := range tasks[id].WaitsOn {
			if include[w] {
				ws = append(ws, w)
			}
		}
		return ws
	}

	// Where each task is heading, what it waits on first.
	memo := map[string]projection{}
	visiting := map[string]bool{}
	var project func(id string) projection
	project = func(id string) projection {
		if p, ok := memo[id]; ok {
			return p
		}
		if visiting[id] {
			return projection{} // a loop, which the store refuses
		}
		visiting[id] = true
		defer delete(visiting, id)
		t := tasks[id]
		var p projection
		var left []wire.GraphBlock // the blocks still planned
		for _, b := range blocks[id] {
			if b.Status == model.PlanPlanned && b.StartAt != nil {
				left = append(left, b)
			}
		}
		if t.Status == model.TaskDone {
			if t.DoneAt != nil {
				at := time.UnixMilli(*t.DoneAt)
				p = projection{ok: true, start: at, end: at}
				if s, _, ok := blockSpan(blocks[id]); ok {
					p.start = s
				}
			}
			memo[id] = p
			return p
		}
		var depEnd time.Time
		for _, w := range waitsOf(id) {
			if !open(w) {
				continue
			}
			q := project(w)
			if !q.ok {
				if p.blockedBy == "" {
					p.blockedBy = w
				}
				continue
			}
			if q.end.After(depEnd) {
				depEnd, p.last = q.end, w
			}
		}
		lane, hasLane := sessionOf[id]
		agentBusy := hasLane && (lane.State == store.CodeWorking || lane.State == store.CodeStarting)
		rem := remaining(t)
		switch {
		case hasLane && lane.State == store.CodeNeedsYou && working[id].IsZero():
			// No end in sight until the user answers.
		case !working[id].IsZero():
			p.ok, p.start, p.end = true, working[id], now.Add(rem)
			if s, _, ok := blockSpan(blocks[id]); ok && s.Before(p.start) {
				p.start = s
			}
		case p.blockedBy != "":
		case len(left) > 0:
			s, e, _ := blockSpan(left)
			start := s
			if depEnd.After(start) {
				start, p.pushedBy = depEnd, p.last
			}
			if start.Before(now) && !agentBusy {
				start = now // its time came and went without it
			}
			p.ok, p.start, p.end = true, start, start.Add(e.Sub(s))
		case agentBusy:
			// Claude is on it with no time of yours planned: done when it is.
			p.ok, p.start, p.end = true, now, now.Add(rem)
		}
		memo[id] = p
		return p
	}

	// What each task is, worst first.
	states := map[string]string{}
	reasons := map[string]string{}
	others := map[string]string{}
	var state func(id string) string
	state = func(id string) string {
		if st, ok := states[id]; ok {
			return st
		}
		states[id] = wire.GraphReady // a loop's guard
		t := tasks[id]
		p := project(id)
		lane, hasLane := sessionOf[id]
		st, why, other := wire.GraphReady, "", ""
		slip := slipOf(blocks[id], p)
		switch {
		case t.Status == model.TaskDone:
			st = wire.GraphDone
		case hasLane && lane.State == store.CodeNeedsYou:
			st, why = wire.GraphWaitingOnYou, "Claude is waiting for you"
			if lane.Question != "" {
				why += ": " + cut(lane.Question, 140)
			}
		case !working[id].IsZero():
			st, why = wire.GraphWorking, "You're on it now"
			if slip >= atRiskMinutes {
				why += fmt.Sprintf(", heading %s past its plan", minutesText(slip))
			}
		case hasLane && (lane.State == store.CodeWorking || lane.State == store.CodeStarting):
			st, why = wire.GraphAgent, "Claude is working on it"
		case p.blockedBy != "":
			b := p.blockedBy
			other = b
			name := "“" + tasks[b].Title + "”"
			switch bs := state(b); {
			case bs == wire.GraphWaitingOnYou:
				st, why = wire.GraphWaitingOnYou, "Waits on "+name+", where Claude needs your answer"
			case bs == wire.GraphAgent:
				st, why = wire.GraphBlocked, "Waits on "+name+", which Claude is still working on"
			case bs == wire.GraphBlocked:
				st, why = wire.GraphBlocked, "Waits on "+name+", which is blocked itself"
			case tasks[b].Stage == model.StageWaiting && tasks[b].DelegatedTo != "":
				st, why = wire.GraphBlocked, "Waits on "+name+", which is with "+tasks[b].DelegatedTo
			case len(blocks[b]) > 0:
				st, why = wire.GraphBlocked, "Waits on "+name+", which has no time left on today's plan"
			default:
				st, why = wire.GraphBlocked, "Waits on "+name+", which isn't on today's plan"
			}
		case len(blocks[id]) == 0:
			st, why = wire.GraphUnplanned, "Not on today's plan"
			if t.Stage == model.StageWaiting && t.DelegatedTo != "" {
				why = "With " + t.DelegatedTo + ", not on today's plan"
			} else if p.last != "" {
				why = "Not on today's plan; it can start after “" + tasks[p.last].Title + "”"
				other = p.last
			} else if ws := waitsOf(id); len(ws) > 0 {
				for _, w := range ws {
					if open(w) {
						why, other = "Not on today's plan; it waits on “"+tasks[w].Title+"”", w
						break
					}
				}
			}
		case p.ok && slip >= atRiskMinutes:
			st = wire.GraphAtRisk
			if p.pushedBy != "" {
				other = p.pushedBy
				why = fmt.Sprintf("Starts after “%s”, which now ends at %s: %s later than planned",
					tasks[p.pushedBy].Title, clockText(memo[p.pushedBy].end, in.Loc), minutesText(slip))
			} else {
				why = fmt.Sprintf("Heading to end at %s, %s past its plan", clockText(p.end, in.Loc), minutesText(slip))
			}
		case p.ok && p.end.After(in.To) && !in.To.IsZero():
			st, why = wire.GraphAtRisk, "Runs past the end of your day ("+clockText(in.To, in.Loc)+")"
		}
		states[id], reasons[id], others[id] = st, why, other
		return st
	}

	blockers := map[string][]string{}
	for _, id := range order {
		if !include[id] {
			continue
		}
		for _, w := range waitsOf(id) {
			blockers[w] = append(blockers[w], id)
		}
	}
	for _, id := range order {
		if !include[id] {
			continue
		}
		st := state(id)
		t, p := tasks[id], project(id)
		gt := wire.GraphTask{ID: id, Title: t.Title, ProjectID: t.ProjectID, Status: t.Status, Stage: t.Stage,
			DelegatedTo: t.DelegatedTo, EstimateMinutes: t.EstimateMinutes, TrackedMs: t.TrackedMs, Rev: t.Rev,
			WaitsOn: append([]string{}, waitsOf(id)...), Blocks: append([]string{}, blockers[id]...),
			Planned: append([]wire.GraphBlock{}, blocks[id]...), State: st, Reason: reasons[id],
			SlipMinutes: max(0, slipOf(blocks[id], p))}
		if s, e, ok := blockSpan(blocks[id]); ok {
			gt.PlannedStart, gt.PlannedEnd = ptr64(s.UnixMilli()), ptr64(e.UnixMilli())
		}
		if p.ok {
			gt.ProjectedStart, gt.ProjectedEnd = ptr64(p.start.UnixMilli()), ptr64(p.end.UnixMilli())
		}
		if l, ok := sessionOf[id]; ok {
			gt.SessionID = l.SessionID
		}
		out.Tasks = append(out.Tasks, gt)
		kind := ""
		switch st {
		case wire.GraphWaitingOnYou, wire.GraphBlocked, wire.GraphAtRisk:
			kind = st
		}
		if kind != "" {
			out.Issues = append(out.Issues, wire.GraphIssue{Kind: kind, TaskID: id, OtherID: others[id],
				SessionID: gt.SessionID, Text: tasks[id].Title + ": " + lowerFirst(reasons[id])})
		}
	}
	severity := map[string]int{wire.GraphWaitingOnYou: 0, wire.GraphBlocked: 1, wire.GraphAtRisk: 2}
	start := func(id string) int64 {
		if s, _, ok := blockSpan(blocks[id]); ok {
			return s.UnixMilli()
		}
		return 1 << 62
	}
	slices.SortStableFunc(out.Issues, func(a, b wire.GraphIssue) int {
		return cmp.Or(cmp.Compare(severity[a.Kind], severity[b.Kind]), cmp.Compare(start(a.TaskID), start(b.TaskID)))
	})

	// The longest chain: back from the open task that waits on something
	// and is heading to end last, through what each waits on that ends last.
	var last string
	for _, id := range order {
		if !open(id) {
			continue
		}
		if p := memo[id]; p.ok && p.last != "" && (last == "" || p.end.After(memo[last].end)) {
			last = id
		}
	}
	var chain []string
	for id := last; id != "" && !slices.Contains(chain, id); id = memo[id].last {
		chain = append(chain, id)
	}
	if len(chain) >= 2 {
		slices.Reverse(chain)
		out.Critical = chain
	}
	return out
}

// agentLanes are the day's Claude sessions with their spans, by when they
// started.
func agentLanes(in graphInput) []wire.AgentLane {
	live := map[string]wire.CodeSession{}
	for _, cs := range in.Live {
		live[cs.ID] = cs
	}
	byID := map[string]*wire.AgentLane{}
	var order []string
	lane := func(id, cwd string) *wire.AgentLane {
		if l, ok := byID[id]; ok {
			return l
		}
		l := &wire.AgentLane{SessionID: id, Cwd: cwd, Name: filepath.Base(cwd), Spans: []wire.AgentSpan{},
			ProjectID: projectOfDir(in.Projects, cwd)}
		byID[id] = l
		order = append(order, id)
		return l
	}
	for _, sp := range in.Spans {
		l := lane(sp.SessionID, sp.Cwd)
		ws := wire.AgentSpan{State: sp.State, Need: sp.Need, TaskID: sp.TaskID, StartAt: sp.StartedAt.UnixMilli()}
		if sp.EndedAt != nil {
			ws.EndAt = ptr64(sp.EndedAt.UnixMilli())
		}
		l.Spans = append(l.Spans, ws)
		l.TaskID = cmp.Or(sp.SessionTask, sp.TaskID, l.TaskID)
		l.State = store.CodeEnded
		if sp.EndedAt == nil {
			l.State, l.Need = sp.State, sp.Need
		}
	}
	// A live session from before spans began shows from the day's start.
	for _, cs := range in.Live {
		if _, ok := byID[cs.ID]; ok || cs.State == store.CodeEnded {
			continue
		}
		l := lane(cs.ID, cs.Cwd)
		at := max(cs.StartedAt, in.From.UnixMilli())
		if at > in.Now.UnixMilli() {
			at = cs.StartedAt
		}
		l.Spans = append(l.Spans, wire.AgentSpan{State: cs.State, Need: cs.Need, TaskID: cs.TaskID, StartAt: at})
	}
	out := make([]wire.AgentLane, 0, len(order))
	for _, id := range order {
		l := byID[id]
		if cs, ok := live[id]; ok {
			l.State, l.Need = cs.State, cs.Need
			l.Name = cmp.Or(cs.Name, l.Name)
			l.TaskID = cmp.Or(cs.TaskID, l.TaskID)
			if w := cs.Waiting; w != nil && cs.State == store.CodeNeedsYou {
				l.Question = cmp.Or(w.Question, w.Summary)
			}
		}
		out = append(out, *l)
	}
	slices.SortStableFunc(out, func(a, b wire.AgentLane) int {
		return cmp.Compare(firstStart(a), firstStart(b))
	})
	return out
}

func firstStart(l wire.AgentLane) int64 {
	if len(l.Spans) == 0 {
		return 0
	}
	return l.Spans[0].StartAt
}

// projectOfDir is the project whose folder holds dir, the deepest one.
func projectOfDir(projects []model.Project, dir string) *string {
	var best *model.Project
	for i := range projects {
		p := &projects[i]
		if p.Folder == "" || !(dir == p.Folder || strings.HasPrefix(dir, strings.TrimSuffix(p.Folder, "/")+"/")) {
			continue
		}
		if best == nil || len(p.Folder) > len(best.Folder) {
			best = p
		}
	}
	if best == nil {
		return nil
	}
	id := best.ID
	return &id
}

// blockSpan is from the start of the first timed block to the end of the
// last one.
func blockSpan(bs []wire.GraphBlock) (start, end time.Time, ok bool) {
	for _, b := range bs {
		if b.StartAt == nil {
			continue
		}
		s := time.UnixMilli(*b.StartAt)
		e := s.Add(time.Duration(b.Minutes) * time.Minute)
		if !ok || s.Before(start) {
			start = s
		}
		if !ok || e.After(end) {
			end = e
		}
		ok = true
	}
	return start, end, ok
}

// slipOf is how many minutes past its planned end a task is heading.
func slipOf(bs []wire.GraphBlock, p projection) int {
	_, e, ok := blockSpan(bs)
	if !ok || !p.ok {
		return 0
	}
	return int(p.end.Sub(e) / time.Minute)
}

// remaining is the work left on a task: its estimate (an hour when it has
// none) less what is tracked, and at least 5 minutes.
func remaining(t wire.Task) time.Duration {
	est := 60
	if t.EstimateMinutes != nil {
		est = *t.EstimateMinutes
	}
	left := time.Duration(est)*time.Minute - time.Duration(t.TrackedMs)*time.Millisecond
	return max(left, 5*time.Minute).Truncate(time.Minute)
}

func clockText(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("3:04 pm")
}

func minutesText(m int) string {
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

// lowerFirst lowers a sentence's first letter to follow a colon, but not a
// name's.
func lowerFirst(s string) string {
	if s == "" || strings.HasPrefix(s, "Claude") {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func appendOnce(xs []string, x string) []string {
	if slices.Contains(xs, x) {
		return xs
	}
	return append(xs, x)
}

func ptr64(v int64) *int64 { return &v }

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
