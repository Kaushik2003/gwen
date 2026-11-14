package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// phoneRoutes are Gwen herself on the paired phone (the phone redo, approved
// 2026-11-13): your tracking (clock in, switch, breaks), today's plan, your
// tasks and projects, and Gwen's agent chats with their approvals. They sit
// behind the same checks as the code routes (Tailscale identity and the
// paired-device cookie; plan.md 3.14.4). Nothing that deletes is here.
func (s *Server) phoneRoutes(route router) {
	// Like the code routes, they answer only while the remote runs.
	phone := func(pattern string, h func(w http.ResponseWriter, r *http.Request) error) {
		route(pattern, s.withCode(h))
	}
	phone("GET /v1/status", s.status)
	phone("POST /v1/day/clock-in", s.clockIn)
	phone("POST /v1/day/clock-out", s.command(func(at time.Time) timeengine.Input { return timeengine.ClockOut{At: at} }))
	phone("POST /v1/break/start", s.command(func(at time.Time) timeengine.Input { return timeengine.BreakStart{At: at} }))
	phone("POST /v1/break/end", s.command(func(at time.Time) timeengine.Input { return timeengine.BreakEnd{At: at} }))
	phone("POST /v1/session/switch", s.switchTo)

	phone("GET /v1/plan", s.getPlan)
	phone("GET /v1/projects", s.listProjects)
	phone("POST /v1/projects", s.phoneCreateProject)
	phone("POST /v1/projects/{id}/folder", s.phoneProjectFolder)
	phone("GET /v1/tasks", s.listTasks)
	phone("POST /v1/tasks", s.createTask)
	phone("GET /v1/tasks/{id}", s.getTask)
	phone("PATCH /v1/tasks/{id}", s.patchTask)
	phone("POST /v1/tasks/{id}/complete", s.completeTask)
	phone("POST /v1/tasks/{id}/reopen", s.reopenTask)
	phone("POST /v1/tasks/{id}/hand-off", s.phoneHandOff)

	phone("GET /v1/agent/status", s.agentStatus)
	phone("GET /v1/agent/chats", s.listAgentChats)
	phone("POST /v1/agent/chats", s.createAgentChat)
	phone("GET /v1/agent/chats/{id}", s.getAgentChat)
	phone("POST /v1/agent/chats/{id}/messages", s.sendAgentMessage)
	phone("POST /v1/agent/chats/{id}/stop", s.stopAgentChat)
	phone("GET /v1/approvals", s.withHarness(s.listApprovals))
	phone("POST /v1/approvals/{id}", s.withHarness(s.decideApproval))
}

// phoneProjectRequest is the phone's POST /v1/projects body: a name, a
// colour, and optionally a folder the phone may open, made first when
// create_folder is set (a git repository too with git_init). POST
// /v1/projects/{id}/folder takes the folder fields alone.
type phoneProjectRequest struct {
	Name         string  `json:"name"`
	Color        *string `json:"color,omitempty"`
	Folder       string  `json:"folder,omitempty"`
	CreateFolder bool    `json:"create_folder,omitempty"`
	GitInit      bool    `json:"git_init,omitempty"`
}

// phoneCreateProject serves the phone's POST /v1/projects: a new project,
// with a folder only inside the folders the phone may open (unlike the
// window's, which takes any folder on this computer).
func (s *Server) phoneCreateProject(w http.ResponseWriter, r *http.Request) error {
	var req phoneProjectRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) == "" {
		return badRequest("name", "give the project a name")
	}
	folder := ""
	if strings.TrimSpace(req.Folder) != "" {
		var err error
		if folder, err = s.Code.PhoneFolder(r.Context(), req.Folder, req.CreateFolder, req.GitInit); err != nil {
			return err
		}
	}
	p, err := s.Repos.Projects.Create(r.Context(), strings.TrimSpace(req.Name), req.Color, nil)
	if err != nil {
		return err
	}
	if folder != "" {
		if p, err = s.Repos.Projects.SetFolder(r.Context(), p.ID, folder); err != nil {
			return err
		}
	}
	s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	writeJSON(w, http.StatusCreated, projectWire(p))
	return nil
}

// phoneProjectFolder serves POST /v1/projects/{id}/folder: the phone gives a
// project its folder, only one inside the folders the phone may open.
func (s *Server) phoneProjectFolder(w http.ResponseWriter, r *http.Request) error {
	var req phoneProjectRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Folder) == "" {
		return badRequest("folder", "pick a folder")
	}
	folder, err := s.Code.PhoneFolder(r.Context(), req.Folder, req.CreateFolder, req.GitInit)
	if err != nil {
		return err
	}
	p, err := s.Repos.Projects.SetFolder(r.Context(), r.PathValue("id"), folder)
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventProjectsChanged, wire.ProjectsChanged{})
	writeJSON(w, http.StatusOK, projectWire(p))
	return nil
}

// phoneHandOffRequest is the POST /v1/tasks/{id}/hand-off body: the project
// to work in (the task's own when nil), the brief, and Sandbox or Normal.
type phoneHandOffRequest struct {
	ProjectID *string `json:"project_id"`
	Brief     string  `json:"brief"`
	Sandbox   bool    `json:"sandbox"`
}

// phoneHandOff serves POST /v1/tasks/{id}/hand-off: the phone's Hand it to an
// agent, done as the window does it (ui/src/shell/HandOff.tsx): a Claude
// session in the project's folder with the brief and the task's attached
// context as its first message, its terminal linked to the task, and the
// task waiting on Claude. The window links the terminal itself; the phone
// can't, so the laptop does all of it here.
func (s *Server) phoneHandOff(w http.ResponseWriter, r *http.Request) error {
	var req phoneHandOffRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	task, err := s.Repos.Tasks.Get(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	projectID := ""
	if task.ProjectID != nil {
		projectID = *task.ProjectID
	}
	if req.ProjectID != nil && *req.ProjectID != "" {
		projectID = *req.ProjectID
	}
	if projectID == "" {
		return badRequest("project_id", "pick the project it works for: Claude works in a project's folder")
	}
	p, err := s.Repos.Projects.Get(ctx, projectID)
	if err != nil {
		return err
	}
	if p.Folder == "" {
		return badRequest("project_id", "%s has no folder yet. Give it one in Gwen on the laptop: Claude needs one to work in", p.Name)
	}
	brief := strings.TrimSpace(req.Brief)
	if brief == "" {
		return badRequest("brief", "say what Claude should do")
	}
	if list, err := s.Repos.Attachments.List(ctx, task.ID); err == nil {
		if more := attachmentsBrief(list); more != "" {
			brief += "\n\n" + more
		}
	}
	// Starting can open Gwen Terminal and wait for it.
	startCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	window, sandbox := false, req.Sandbox
	sess, err := s.Code.StartInProject(startCtx, wire.StartCodeSession{Dir: p.Folder, Prompt: brief, Window: &window, Sandbox: &sandbox})
	if err != nil {
		return err
	}
	if sess.Terminal != "" {
		if err := s.Code.LinkTask(startCtx, sess.Terminal, task.ID); err != nil {
			slog.Warn("hand-off: the session's terminal wasn't linked to its task", "task", task.ID, "err", err)
		}
	}
	stage, who := model.StageWaiting, "Claude"
	patch := store.TaskPatch{Stage: &stage, DelegatedTo: &who}
	if task.ProjectID == nil || *task.ProjectID != p.ID {
		patch.ProjectID = store.Nullable[string]{Set: true, Value: &p.ID}
	}
	if _, err := s.Repos.Tasks.Update(ctx, task.ID, patch); err != nil {
		slog.Warn("hand-off: the task wasn't moved to Waiting", "task", task.ID, "err", err)
	} else {
		s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{task.ID}})
	}
	writeJSON(w, http.StatusCreated, sess)
	return nil
}

// attachmentsBrief is a task's attached context as the brief carries it, as
// the window's attachmentsBrief (ui/src/task/Attachments.tsx) writes it.
func attachmentsBrief(list []model.Attachment) string {
	if len(list) == 0 {
		return ""
	}
	// Each text is cut so a few long emails still leave the brief a sensible size.
	each := max(1500, 24000/len(list))
	apps := map[string]string{"gmail": "Gmail", "googlecalendar": "Google Calendar", "github": "GitHub", "notion": "Notion"}
	parts := make([]string, 0, len(list))
	for _, a := range list {
		var head string
		switch a.Kind {
		case "link":
			head = fmt.Sprintf("- Link: %s <%s>", a.Title, a.URL)
		case "file":
			head = "- File: " + a.Path
		case "note":
			head = "- Note: " + a.Title
		default:
			name := apps[a.App]
			if name == "" {
				name = a.App
			}
			head = fmt.Sprintf("- %s: %s", name, a.Title)
			if a.URL != "" {
				head += " <" + a.URL + ">"
			}
			if a.FetchedAt != nil {
				head += " (as of " + a.FetchedAt.Format("2 Jan 2006") + ")"
			}
		}
		body := ""
		if a.Kind == "note" || a.Kind == "app" {
			body = strings.TrimSpace(a.Body)
		}
		if r := []rune(body); len(r) > each {
			body = string(r[:each]) + fmt.Sprintf("\n[… cut; %d more characters]", len(r)-each)
		}
		if body != "" {
			head += "\n\n    " + strings.ReplaceAll(body, "\n", "\n    ")
		}
		parts = append(parts, head)
	}
	return "Context attached to this task:\n\n" + strings.Join(parts, "\n\n")
}

// phoneEvents are the events the phone's stream carries besides the code_
// ones: what its Today and Gwen screens show.
var phoneEvents = []string{wire.EventStateChanged, wire.EventDayChanged, wire.EventTasksChanged, wire.EventPlanChanged,
	wire.EventProjectsChanged, wire.EventAgentChanged, wire.EventAgentText, wire.EventAgentStep,
	wire.EventApprovalRequested, wire.EventApprovalDecided}

// phoneFrame reports whether an event frame goes to the phone.
func phoneFrame(frame []byte) bool {
	_, rest, ok := strings.Cut(string(frame[:min(len(frame), 96)]), "\nevent: ")
	if !ok {
		return false
	}
	if strings.HasPrefix(rest, "code_") {
		return true
	}
	name, _, _ := strings.Cut(rest, "\n")
	for _, e := range phoneEvents {
		if name == e {
			return true
		}
	}
	return false
}
