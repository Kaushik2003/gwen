package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/harness"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/subject"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// The harness routes (plan.md 3.10, W19): runs, approvals, the resources and
// routines of subjects, a project's folder, and the connections.
func (s *Server) harnessRoutes(route router) {
	route("GET /v1/runs", s.withHarness(s.listRuns))
	route("POST /v1/runs", s.withHarness(s.createRun))
	route("GET /v1/runs/{id}", s.withHarness(s.getRunDetail))
	route("POST /v1/runs/{id}/start", s.withHarness(s.runAction(func(ctx context.Context, id string) (store.Run, error) {
		return s.Harness.Start(ctx, id)
	})))
	route("POST /v1/runs/{id}/cancel", s.withHarness(s.runAction(func(ctx context.Context, id string) (store.Run, error) {
		return s.Harness.Cancel(ctx, id)
	})))
	route("POST /v1/runs/{id}/resume", s.withHarness(s.runAction(func(ctx context.Context, id string) (store.Run, error) {
		return s.Harness.Resume(ctx, id)
	})))
	route("POST /v1/runs/{id}/message", s.withHarness(s.messageRun))
	route("GET /v1/approvals", s.withHarness(s.listApprovals))
	route("POST /v1/approvals/{id}", s.withHarness(s.decideApproval))
	route("GET /v1/grants", s.listGrants)
	route("DELETE /v1/grants/{id}", s.deleteGrant)
	route("GET /v1/resources", s.getResources)
	route("PUT /v1/resources", s.putResources)
	route("GET /v1/routines", s.getRoutine)
	route("PUT /v1/routines", s.putRoutine)
	route("GET /v1/projects/{id}/dir", s.getProjectDir)
	route("PUT /v1/projects/{id}/dir", s.putProjectDir)
	route("GET /v1/connections", s.withHarness(s.listConnections))
	route("POST /v1/connections/{id}/connect", s.withHarness(s.linkConnection(true)))
	route("DELETE /v1/connections/{id}", s.withHarness(s.linkConnection(false)))
}

func (s *Server) withHarness(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.Harness == nil {
			return Unavailable("runs are not running in this daemon")
		}
		return h(w, r)
	}
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) error {
	var statuses []string
	if v := r.URL.Query().Get("status"); v != "" {
		statuses = strings.Split(v, ",")
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.Repos.Runs.Runs(r.Context(), statuses, limit)
	if err != nil {
		return err
	}
	out := wire.RunList{Runs: make([]wire.Run, len(runs))}
	for i, run := range runs {
		out.Runs[i] = harness.WireRun(run)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateRunRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	run, err := s.Harness.Create(r.Context(), req.Kind, req.Subject, req.Title, req.Brief, "")
	if err != nil {
		return err
	}
	if req.Start {
		if run, err = s.Harness.Start(r.Context(), run.ID); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusCreated, harness.WireRun(run))
	return nil
}

func (s *Server) getRunDetail(w http.ResponseWriter, r *http.Request) error {
	d, err := s.runDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) runDetail(ctx context.Context, id string) (wire.RunDetail, error) {
	run, err := s.Repos.Runs.Run(ctx, id)
	if err != nil {
		return wire.RunDetail{}, err
	}
	steps, err := s.Repos.Runs.Steps(ctx, id)
	if err != nil {
		return wire.RunDetail{}, err
	}
	log, err := s.Repos.Runs.Events(ctx, id, 200)
	if err != nil {
		return wire.RunDetail{}, err
	}
	approvals, err := s.Repos.Runs.Approvals(ctx, "", 500)
	if err != nil {
		return wire.RunDetail{}, err
	}
	out := wire.RunDetail{Run: harness.WireRun(run), Steps: []wire.RunStep{}, Log: []wire.RunEvent{}, Approvals: []wire.Approval{}}
	for _, st := range steps {
		out.Steps = append(out.Steps, harness.WireStep(st))
	}
	for _, ev := range log {
		out.Log = append(out.Log, harness.WireEvent(ev))
	}
	for _, a := range approvals {
		if a.RunID == id {
			out.Approvals = append(out.Approvals, harness.WireApproval(a, run.Title))
		}
	}
	return out, nil
}

func (s *Server) runAction(act func(ctx context.Context, id string) (store.Run, error)) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		run, err := act(r.Context(), r.PathValue("id"))
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, harness.WireRun(run))
		return nil
	}
}

func (s *Server) messageRun(w http.ResponseWriter, r *http.Request) error {
	var req wire.RunMessageRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" || len(text) > 16000 {
		return badRequest("text", "a reply is 1 to 16,000 characters")
	}
	run, err := s.Harness.Message(r.Context(), r.PathValue("id"), text)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, harness.WireRun(run))
	return nil
}

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request) error {
	list, err := s.approvals(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) approvals(ctx context.Context, status string) (wire.ApprovalList, error) {
	switch status {
	case "", store.ApprovalPending, store.ApprovalApproved, store.ApprovalRejected, store.ApprovalExpired:
	default:
		return wire.ApprovalList{}, badRequest("status", "status is pending, approved, rejected, or expired")
	}
	list, err := s.Repos.Runs.Approvals(ctx, status, 200)
	if err != nil {
		return wire.ApprovalList{}, err
	}
	out := wire.ApprovalList{Approvals: []wire.Approval{}}
	titles := map[string]string{}
	for _, a := range list {
		title, ok := titles[a.RunID]
		if !ok && a.RunID != "" {
			if run, err := s.Repos.Runs.Run(ctx, a.RunID); err == nil {
				title = run.Title
			}
			titles[a.RunID] = title
		}
		out.Approvals = append(out.Approvals, harness.WireApproval(a, title))
	}
	return out, nil
}

// decideApproval is POST /v1/approvals/{id}. Approvals happen only by a click
// or key press in the dashboard (section 7): voice and the assistant never
// call it.
func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) error {
	var req wire.DecideApprovalRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		return badRequest("decision", "decision is approve or reject")
	}
	a, err := s.Harness.Decide(r.Context(), r.PathValue("id"), req.Decision == "approve", req.Payload, req.Remember)
	if err != nil {
		return err
	}
	title := ""
	if a.RunID != "" {
		if run, err := s.Repos.Runs.Run(r.Context(), a.RunID); err == nil {
			title = run.Title
		}
	}
	writeJSON(w, http.StatusOK, harness.WireApproval(a, title))
	return nil
}

func (s *Server) listGrants(w http.ResponseWriter, r *http.Request) error {
	grants, err := s.Repos.Runs.Grants(r.Context())
	if err != nil {
		return err
	}
	out := wire.GrantList{Grants: []wire.Grant{}}
	for _, g := range grants {
		out.Grants = append(out.Grants, harness.WireGrant(g))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) deleteGrant(w http.ResponseWriter, r *http.Request) error {
	if err := s.Repos.Runs.DeleteGrant(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// subjectParam is the ?subject= of a request, parsed.
func subjectParam(r *http.Request) (subject.Ref, error) {
	ref, err := subject.Parse(r.URL.Query().Get("subject"))
	if err != nil {
		return subject.Ref{}, badRequest("subject", "%v", err)
	}
	return ref, nil
}

func resourcesWire(ref string, rs []store.Resource) wire.ResourceList {
	out := wire.ResourceList{Subject: ref, Resources: []wire.Resource{}}
	for _, res := range rs {
		out.Resources = append(out.Resources, wire.Resource{ID: res.ID, Kind: res.Kind, Title: res.Title, Value: res.Value})
	}
	return out
}

func (s *Server) getResources(w http.ResponseWriter, r *http.Request) error {
	ref, err := subjectParam(r)
	if err != nil {
		return err
	}
	rs, err := s.Repos.Subjects.Resources(r.Context(), ref.String())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, resourcesWire(ref.String(), rs))
	return nil
}

func (s *Server) putResources(w http.ResponseWriter, r *http.Request) error {
	ref, err := subjectParam(r)
	if err != nil {
		return err
	}
	var req wire.PutResourcesRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	in := make([]store.NewResource, len(req.Resources))
	for i, res := range req.Resources {
		in[i] = store.NewResource{Kind: res.Kind, Title: res.Title, Value: res.Value}
	}
	rs, err := s.Repos.Subjects.PutResources(r.Context(), ref.String(), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, resourcesWire(ref.String(), rs))
	return nil
}

func routineWire(ref string, rt store.Routine, inherited bool) wire.Routine {
	out := wire.Routine{Subject: ref, Steps: []wire.RoutineStep{}, AutoStart: rt.AutoStart, Inherited: inherited}
	for _, st := range rt.Steps {
		out.Steps = append(out.Steps, wire.RoutineStep{Title: st.Title, Capability: st.Capability, Input: st.Input})
	}
	return out
}

// RoutineOf finds the routine a subject's start runs: its own, else its
// parent's: an event's series, a commitment day's commitment, a task
// occurrence's template (plan.md 3.3).
func (s *Server) RoutineOf(ctx context.Context, ref string) (store.Routine, bool, error) {
	for _, candidate := range s.SubjectChain(ctx, ref) {
		rt, err := s.Repos.Subjects.Routine(ctx, candidate)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return store.Routine{}, false, err
		}
		return rt, true, nil
	}
	return store.Routine{}, false, nil
}

// SubjectChain is a subject and the parents a lookup falls back to, nearest
// first. It is also the scopes a grant for the subject applies to.
func (s *Server) SubjectChain(ctx context.Context, ref string) []string {
	r, err := subject.Parse(ref)
	if err != nil {
		return []string{ref}
	}
	out := []string{r.String()}
	if p, ok := r.Parent(); ok {
		out = append(out, p.String())
	}
	if r.Kind() == subject.Task {
		if t, err := s.Repos.Tasks.Get(ctx, r.ID); err == nil && t.TemplateID != nil {
			out = append(out, subject.Of(subject.Task, *t.TemplateID).String())
		}
	}
	return out
}

func (s *Server) getRoutine(w http.ResponseWriter, r *http.Request) error {
	ref, err := subjectParam(r)
	if err != nil {
		return err
	}
	rt, ok, err := s.RoutineOf(r.Context(), ref.String())
	if err != nil {
		return err
	}
	if !ok {
		rt = store.Routine{Steps: []store.RoutineStep{}}
	}
	writeJSON(w, http.StatusOK, routineWire(ref.String(), rt, ok && rt.Subject != ref.String()))
	return nil
}

func (s *Server) putRoutine(w http.ResponseWriter, r *http.Request) error {
	ref, err := subjectParam(r)
	if err != nil {
		return err
	}
	var req wire.PutRoutineRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	steps := make([]store.RoutineStep, len(req.Steps))
	for i, st := range req.Steps {
		// A step is checked against its capability now, so a typo fails here and not at 7 pm.
		if s.Harness != nil {
			c, _, ok := s.Harness.Registry().Capability(st.Capability)
			if !ok {
				return badRequest("steps", "step %d: there is no capability %q", i+1, st.Capability)
			}
			// Kept as given: defaults such as "today" are filled in when the step runs.
			if _, err := c.Check(st.Input); err != nil {
				return badRequest("steps", "step %d: %v", i+1, err)
			}
		}
		steps[i] = store.RoutineStep{Title: strings.TrimSpace(st.Title), Capability: st.Capability, Input: st.Input}
	}
	rt, err := s.Repos.Subjects.PutRoutine(r.Context(), ref.String(), steps, req.AutoStart)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, routineWire(ref.String(), rt, false))
	return nil
}

func (s *Server) getProjectDir(w http.ResponseWriter, r *http.Request) error {
	path, test, err := s.Repos.Projects.Dir(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, wire.ProjectDir{Path: path, TestCommand: test})
	return nil
}

func (s *Server) putProjectDir(w http.ResponseWriter, r *http.Request) error {
	var req wire.ProjectDir
	if err := decode(r, &req); err != nil {
		return err
	}
	path := ""
	if req.Path != "" {
		var err error
		if path, err = validFolder(req.Path); err != nil {
			return err
		}
	}
	test := strings.TrimSpace(req.TestCommand)
	if err := s.Repos.Projects.SetDir(r.Context(), r.PathValue("id"), path, test); err != nil {
		return err
	}
	s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	writeJSON(w, http.StatusOK, wire.ProjectDir{Path: path, TestCommand: test})
	return nil
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) error {
	out := wire.ConnectionList{Connections: []wire.Connection{}}
	for _, c := range s.Harness.Registry().Connectors() {
		st := c.Status(r.Context())
		_, linkable := c.(connect.Linker)
		conn := wire.Connection{ID: c.ID(), Name: c.Name(), State: st.State, Message: st.Message, Linkable: linkable,
			Capabilities: []wire.Capability{}}
		for _, cp := range c.Capabilities() {
			if cp.Risk == connect.Destructive {
				continue
			}
			conn.Capabilities = append(conn.Capabilities, wire.Capability{Name: cp.Name, Title: cp.Title, Risk: string(cp.Risk), Input: cp.Input})
		}
		out.Connections = append(out.Connections, conn)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) linkConnection(on bool) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		c, ok := s.Harness.Registry().Connector(id)
		if !ok {
			return store.Fail(store.ErrNotFound, "no connection %s", id)
		}
		l, ok := c.(connect.Linker)
		if !ok {
			return store.Fail(store.ErrConflict, "%s is set up elsewhere in Gwen and can't be connected or disconnected here", c.Name())
		}
		var err error
		if on {
			err = l.Connect(r.Context())
		} else {
			err = l.Disconnect(r.Context())
		}
		if err != nil {
			return err
		}
		s.Hub.Publish(wire.EventConnectionChanged, wire.ConnectionChanged{ID: id})
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

// startRoutine is the start hook: starting work on a task runs its routine,
// the steps it has for when it starts (plan.md 3.11), when it has one. It
// never fails the start; the run reports its own result.
func (s *Server) startRoutine(ctx context.Context, taskID string) {
	if s.Harness == nil {
		return
	}
	ref := subject.Of(subject.Task, taskID).String()
	rt, ok, err := s.RoutineOf(ctx, ref)
	if err != nil || !ok || len(rt.Steps) == 0 {
		return
	}
	title := "When it starts"
	if t, err := s.Repos.Tasks.Get(ctx, taskID); err == nil {
		title = t.Title
	}
	run, err := s.Harness.Create(ctx, harness.KindRoutine, ref, title, "", "")
	if err == nil {
		_, err = s.Harness.Start(ctx, run.ID)
	}
	if err != nil {
		slog.Warn("start routine", "task", taskID, "err", err)
	}
}

// StartTracking is the tracking.start capability: it switches to the task
// or project, clocking in first when the day is not started.
func (s *Server) StartTracking(ctx context.Context, projectID, taskID string) (string, error) {
	var project, task *string
	if projectID != "" {
		project = &projectID
	}
	if taskID != "" {
		task = &taskID
	}
	project, task, err := s.attribution(ctx, project, task)
	if err != nil {
		return "", err
	}
	st, err := s.Tracker.Status(ctx)
	if err != nil {
		return "", err
	}
	_, err = s.Tracker.Do(ctx, func(at time.Time) timeengine.Input {
		if st.State == wire.StateOff {
			return timeengine.ClockIn{ProjectID: project, TaskID: task, At: at}
		}
		return timeengine.Switch{ProjectID: project, TaskID: task, At: at}
	})
	if err != nil {
		return "", err
	}
	switch {
	case task != nil:
		if t, err := s.Repos.Tasks.Get(ctx, *task); err == nil {
			return t.Title, nil
		}
	case project != nil:
		if p, err := s.Repos.Projects.Get(ctx, *project); err == nil {
			return p.Name, nil
		}
	}
	return "work", nil
}
