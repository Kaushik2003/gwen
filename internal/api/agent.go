package api

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/agent"
	"github.com/kzark/gwen/internal/harness"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Gwen's agent on the API (plan.md 3.16, W32): her chats, your messages,
// Stop, Undo, and, on agent.sock only, her tools for `gwen mcp`. The
// Server is the agent's Sources.

// maxNote is the most characters of the context note in front of a message.
const maxNote = 8000

// AgentTokenHeader carries a chat process's token on agent.sock.
const AgentTokenHeader = "X-Gwen-Agent-Token"

func (s *Server) agentOff() error {
	if s.Agent == nil {
		return Unavailable("Gwen's agent doesn't run on this device")
	}
	return nil
}

// --- agent.Sources

// Persona implements agent.Sources.
func (s *Server) Persona(ctx context.Context) string {
	cfg := s.Tracker.Config().LLM
	out := llm.PersonaPrompt(cfg)
	if self, err := s.assistantSelf(ctx); err == nil {
		if sp := llm.SelfPrompt(s.llmSelf(self)); sp != "" {
			out += "\n\n" + sp
		}
	}
	return out
}

// ProfileCard implements agent.Sources.
func (s *Server) ProfileCard(ctx context.Context) string { return s.profileCard(ctx).Text }

// Place implements agent.Sources: where a chat works, for its system prompt.
func (s *Server) Place(ctx context.Context, project *model.Project) string {
	projects, _ := s.Repos.Projects.List(ctx, wire.ArchivedFalse)
	var b strings.Builder
	if project == nil {
		b.WriteString("This chat is for everywhere: it belongs to no project, and you run in an empty folder of " +
			"Gwen's. To work on code, start a Claude session in a project's folder, or tell the user to open the " +
			"project's own chat. Their projects:\n")
		for _, p := range projects {
			fmt.Fprintf(&b, "- %s (id %s)", p.Name, p.ID[:8])
			if p.Folder != "" {
				fmt.Fprintf(&b, ", folder %s", p.Folder)
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "This chat belongs to the project %q (id %s).", project.Name, project.ID[:8])
	var subs []string
	for _, p := range projects {
		if p.ParentID != nil && *p.ParentID == project.ID {
			subs = append(subs, fmt.Sprintf("%s (id %s)", p.Name, p.ID[:8]))
		}
		if project.ParentID != nil && p.ID == *project.ParentID {
			fmt.Fprintf(&b, " It is a sub-project of %q.", p.Name)
		}
	}
	if len(subs) > 0 {
		fmt.Fprintf(&b, " Its sub-projects: %s.", strings.Join(subs, ", "))
	}
	if project.Folder == "" {
		b.WriteString("\nIt has no folder on this computer yet, so you run in an empty folder of Gwen's. If the user " +
			"wants work on its files, ask them to choose its folder (Projects › Choose its folder).")
		return b.String()
	}
	fmt.Fprintf(&b, "\nIts folder on this computer is %s, and you run there.", project.Folder)
	if _, test, err := s.Repos.Projects.Dir(ctx, project.ID); err == nil && test != "" {
		fmt.Fprintf(&b, " Its test command is `%s`.", test)
	}
	return b.String()
}

// AddMessage implements agent.Sources.
func (s *Server) AddMessage(ctx context.Context, threadID, role, text string) (model.ChatMessage, error) {
	_, msgs, err := s.Repos.Chats.Append(ctx, threadID, []model.ChatMessage{{Role: role, Text: text, At: s.now()}})
	if err != nil {
		return model.ChatMessage{}, err
	}
	return msgs[0], nil
}

// TurnDone implements agent.Sources: the memory learns from the turn.
func (s *Server) TurnDone(ctx context.Context, threadID string, user, reply model.ChatMessage) {
	s.afterTurn(ctx, threadID, []model.ChatMessage{user, reply}, memory.Note{}, nil)
}

// Note implements agent.Sources: the <gwen-context> block in front of a
// message (plan.md 3.16).
func (s *Server) Note(ctx context.Context, thread model.ChatThread, project *model.Project, focus *wire.AgentFocus, text string) string {
	var b strings.Builder
	now := s.now()
	today := civil.MustParse(s.today())
	fmt.Fprintf(&b, "<gwen-context>\nNow: %s.\n", now.In(s.Loc).Format("Monday 2 January 2006, 3:04 pm"))
	names := s.projectNames(ctx)
	if st, err := s.Tracker.Status(ctx); err == nil {
		switch st.State {
		case "off":
			b.WriteString("Tracking: off (not clocked in).\n")
		default:
			what := cmp.Or(names[deref(st.ProjectID)], "no project")
			if st.TaskID != nil {
				if t, err := s.Repos.Tasks.Get(ctx, *st.TaskID); err == nil {
					what = fmt.Sprintf("%q in %s", t.Title, what)
				}
			}
			since := ""
			if st.OpenSegment != nil {
				since = ", for " + shortDuration(now.Sub(time.UnixMilli(st.OpenSegment.StartedAt)))
			}
			fmt.Fprintf(&b, "Tracking: %s, %s%s.\n", strings.ReplaceAll(st.State, "_", " "), what, since)
		}
	}
	if focus != nil {
		switch focus.Kind {
		case "file":
			fmt.Fprintf(&b, "Open in the window: the file %s.\n", focus.Path)
		case "terminal":
			fmt.Fprintf(&b, "Open in the window: a terminal (%s).\n", focus.Terminal)
		case "history":
			b.WriteString("Open in the window: the project's git history.\n")
		case "project":
			b.WriteString("Open in the window: the project's home.\n")
		case "screen", "today":
			fmt.Fprintf(&b, "Open in the window: %s.\n", cmp.Or(focus.Screen, "Today"))
		}
		if sel := strings.TrimSpace(focus.Selection); sel != "" {
			fmt.Fprintf(&b, "Selected text:\n%s\n", agentCut(sel, 1500))
		}
	}
	tasks, _ := s.Repos.Tasks.List(ctx, store.TaskFilter{Status: model.TaskOpen})
	tasks = slices.DeleteFunc(tasks, func(t model.Task) bool {
		return t.ParentID != nil || (t.OccurrenceDay != nil && *t.OccurrenceDay > today.String())
	})
	plan, _ := s.Repos.Plans.Read(ctx, today.String(), s.planEnv())
	planned := map[string]string{}
	for _, e := range plan.Items {
		if e.Item.Status == model.PlanPlanned && e.Item.StartAt != nil {
			planned[e.Task.ID] = s.dayClock(today, *e.Item.StartAt)
		}
	}
	taskLine := func(t model.Task) string {
		parts := []string{fmt.Sprintf("[%s] %s · %s", t.ID[:8], t.Title, t.Stage)}
		if t.DueDay != nil {
			parts = append(parts, "due "+*t.DueDay)
		}
		if t.Estimate != nil {
			parts = append(parts, shortDuration(*t.Estimate))
		}
		if at, ok := planned[t.ID]; ok {
			parts = append(parts, "planned today "+at)
		}
		return "- " + strings.Join(parts, " · ")
	}
	if project != nil {
		scope, _ := s.projectScope(ctx, project.ID)
		fmt.Fprintf(&b, "Project: %s", project.Name)
		if project.Folder != "" {
			fmt.Fprintf(&b, " (%s)", project.Folder)
			if g := gitState(ctx, project.Folder); g != "" {
				fmt.Fprintf(&b, "; git: %s", g)
			}
		}
		b.WriteString(".\n")
		var mine []model.Task
		for _, t := range tasks {
			if t.ProjectID != nil && scope[*t.ProjectID] {
				mine = append(mine, t)
			}
		}
		sortTasks(mine, planned)
		fmt.Fprintf(&b, "Its open tasks (%d", len(mine))
		if len(mine) > 20 {
			b.WriteString(", the first 20")
			mine = mine[:20]
		}
		b.WriteString("):\n")
		for _, t := range mine {
			b.WriteString(taskLine(t) + "\n")
		}
		if s.Code != nil && project.Folder != "" {
			var lines []string
			for _, cs := range s.Code.Sessions().Sessions {
				if cs.State == "ended" || (cs.Cwd != project.Folder && !strings.HasPrefix(cs.Cwd, project.Folder+"/")) {
					continue
				}
				lines = append(lines, fmt.Sprintf("- %s (id %s): %s, %s", cs.Name, cs.ID, cs.State,
					agentCut(cmp.Or(cs.Step, cs.LastMessage), 120)))
			}
			if len(lines) > 0 {
				b.WriteString("Its Claude sessions:\n" + strings.Join(lines, "\n") + "\n")
			}
		}
	} else {
		counts := map[string]int{}
		for _, t := range tasks {
			counts[deref(t.ProjectID)]++
		}
		projects, _ := s.Repos.Projects.List(ctx, wire.ArchivedFalse)
		var ps []string
		for _, p := range projects {
			ps = append(ps, fmt.Sprintf("%s (%d open)", p.Name, counts[p.ID]))
		}
		if len(ps) > 0 {
			b.WriteString("Projects: " + strings.Join(ps, ", ") + ".\n")
		}
		var blocks []string
		for _, e := range plan.Items {
			if e.Item.Status != model.PlanPlanned {
				continue
			}
			at := "no time yet"
			if e.Item.StartAt != nil {
				at = s.dayClock(today, *e.Item.StartAt)
			}
			blocks = append(blocks, fmt.Sprintf("- %s %s (%s)", at, e.Task.Title, shortDuration(e.Item.Planned)))
		}
		if len(blocks) > 0 {
			if len(blocks) > 15 {
				blocks = blocks[:15]
			}
			b.WriteString("Today's plan:\n" + strings.Join(blocks, "\n") + "\n")
		}
		if s.Code != nil {
			var needs []string
			for _, cs := range s.Code.Sessions().Sessions {
				if cs.State == "needs_you" {
					needs = append(needs, cs.Name)
				}
			}
			if len(needs) > 0 {
				b.WriteString("Claude sessions waiting on the user: " + strings.Join(needs, ", ") + ".\n")
			}
		}
		sortTasks(tasks, planned)
		if len(tasks) > 20 {
			tasks = tasks[:20]
		}
		if len(tasks) > 0 {
			b.WriteString("Most urgent open tasks:\n")
			for _, t := range tasks {
				line := taskLine(t)
				if t.ProjectID != nil {
					line += " · " + names[*t.ProjectID]
				}
				b.WriteString(line + "\n")
			}
		}
	}
	if text != "" {
		if tr, err := s.transcript(ctx, thread.ID); err == nil {
			if n := s.memoryNote(ctx, text, thread.ID, tr); strings.TrimSpace(n.Text) != "" {
				b.WriteString("What memory brings up for this message:\n" + agentCut(n.Text, 4000) + "\n")
			}
		}
	}
	out := b.String()
	if len(out) > maxNote-20 {
		out = out[:maxNote-40] + "\n[cut]"
	}
	return out + "</gwen-context>"
}

// sortTasks puts tasks planned today first, then by priority, then by due day.
func sortTasks(tasks []model.Task, planned map[string]string) {
	slices.SortStableFunc(tasks, func(a, b model.Task) int {
		_, pa := planned[a.ID]
		_, pb := planned[b.ID]
		if pa != pb {
			if pa {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(a.Priority, b.Priority); c != 0 {
			return c
		}
		return cmp.Compare(cmp.Or(deref(a.DueDay), "9999"), cmp.Or(deref(b.DueDay), "9999"))
	})
}

func shortDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}

// gitState is a folder's branch, ahead and behind, and changed files, "" when
// it is not a git repository or git takes too long.
func gitState(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain=v2", "--branch").Output()
	if err != nil {
		return ""
	}
	branch, ab, changed := "", "", 0
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.ab "):
			f := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(f) == 2 {
				a, _ := strconv.Atoi(strings.TrimPrefix(f[0], "+"))
				bh, _ := strconv.Atoi(strings.TrimPrefix(f[1], "-"))
				if a > 0 || bh > 0 {
					ab = fmt.Sprintf(", %d ahead and %d behind", a, bh)
				}
			}
		case !strings.HasPrefix(line, "#") && line != "":
			changed++
		}
	}
	return fmt.Sprintf("branch %s%s, %d changed %s", branch, ab, changed, plural(changed, "file", "files"))
}

// --- endpoints

func (s *Server) agentStatus(w http.ResponseWriter, _ *http.Request) error {
	if err := s.agentOff(); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.Agent.Status())
	return nil
}

// agentChatWire is a chat with what this daemon knows of it right now.
func (s *Server) agentChatWire(ctx context.Context, t model.ChatThread, pending map[string]int) wire.AgentChat {
	out := wire.AgentChat{ID: t.ID, Title: t.Title, ProjectID: t.ProjectID, Messages: t.Messages, LastAt: wire.Millis(t.LastAt),
		Pending: pending[t.ID], State: wire.AgentIdle}
	if s.Agent != nil {
		state, errText, queue, _ := s.Agent.Live(t.ID)
		out.State, out.Error, out.Queued = state, errText, len(queue)
		if errText == "" {
			if ac, err := s.Repos.Agent.Chat(ctx, t.ID); err == nil && ac.State == wire.AgentError && state == wire.AgentIdle {
				out.Error = ac.Error
			}
		}
	}
	return out
}

// pendingByChat counts the approvals waiting in each chat.
func (s *Server) pendingByChat(ctx context.Context) map[string]int {
	out := map[string]int{}
	list, err := s.Repos.Runs.Approvals(ctx, store.ApprovalPending, 500)
	if err != nil {
		return out
	}
	for _, a := range list {
		if a.ThreadID != "" {
			out[a.ThreadID]++
		}
	}
	return out
}

func (s *Server) listAgentChats(w http.ResponseWriter, r *http.Request) error {
	place := store.ChatPlace{All: true}
	switch p := r.URL.Query().Get("project"); p {
	case "", "all":
	case "everywhere":
		place = store.ChatPlace{Everywhere: true}
	default:
		place = store.ChatPlace{Project: p}
	}
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 200 {
		limit = n
	}
	threads, err := s.Repos.Chats.ListIn(r.Context(), place, limit)
	if err != nil {
		return err
	}
	pending := s.pendingByChat(r.Context())
	out := wire.AgentChatList{Chats: []wire.AgentChat{}}
	for _, t := range threads {
		out.Chats = append(out.Chats, s.agentChatWire(r.Context(), t, pending))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createAgentChat(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateAgentChat
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.ProjectID != nil && *req.ProjectID == "" {
		req.ProjectID = nil
	}
	t, err := s.Repos.Chats.Create(r.Context(), "New chat", req.ProjectID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, s.agentChatWire(r.Context(), t, nil))
	return nil
}

func (s *Server) getAgentChat(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	t, err := s.Repos.Chats.Get(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	msgs, err := s.chatMessages(ctx, t.ID)
	if err != nil {
		return err
	}
	steps, err := s.Repos.Agent.Steps(ctx, t.ID)
	if err != nil {
		return err
	}
	out := wire.AgentChatDetail{Chat: s.agentChatWire(ctx, t, s.pendingByChat(ctx)), Messages: msgs,
		Steps: []wire.AgentStep{}, Approvals: []wire.Approval{}, Queue: []string{}}
	for _, st := range steps {
		out.Steps = append(out.Steps, agent.StepWire(st))
	}
	if list, err := s.Repos.Runs.Approvals(ctx, store.ApprovalPending, 500); err == nil {
		for _, a := range list {
			if a.ThreadID == t.ID {
				out.Approvals = append(out.Approvals, harness.WireApproval(a, ""))
			}
		}
	}
	if s.Agent != nil {
		_, _, out.Queue, out.Partial = s.Agent.Live(t.ID)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) patchAgentChat(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchAgentChat
	if err := decode(r, &req); err != nil {
		return err
	}
	t, err := s.Repos.Chats.Rename(r.Context(), r.PathValue("id"), req.Title)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.agentChatWire(r.Context(), t, s.pendingByChat(r.Context())))
	return nil
}

func (s *Server) deleteAgentChat(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if _, err := s.Repos.Chats.Get(r.Context(), id); err != nil {
		return err
	}
	if s.Agent != nil {
		s.Agent.Forget(r.Context(), id)
	}
	if err := s.Repos.Chats.Delete(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) sendAgentMessage(w http.ResponseWriter, r *http.Request) error {
	if err := s.agentOff(); err != nil {
		return err
	}
	var req wire.AgentMessageRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	id := r.PathValue("id")
	t, err := s.Repos.Chats.Get(r.Context(), id)
	if err != nil {
		return err
	}
	// The first message names a chat that still has the name it started with.
	if t.Messages == 0 && t.Title == "New chat" {
		if renamed, err := s.Repos.Chats.Rename(r.Context(), id, req.Text); err == nil {
			t = renamed
		}
	}
	if err := s.Agent.Send(r.Context(), id, req.Text, req.Focus); err != nil {
		switch {
		case errors.Is(err, agent.ErrUnavailable):
			return Unavailable("%s", strings.TrimPrefix(err.Error(), agent.ErrUnavailable.Error()+": "))
		case errors.Is(err, agent.ErrBusy):
			return store.Fail(store.ErrConflict, "%s", err.Error())
		}
		return err
	}
	writeJSON(w, http.StatusAccepted, s.agentChatWire(r.Context(), t, s.pendingByChat(r.Context())))
	return nil
}

func (s *Server) stopAgentChat(w http.ResponseWriter, r *http.Request) error {
	if err := s.agentOff(); err != nil {
		return err
	}
	s.Agent.Stop(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) undoAgentStep(w http.ResponseWriter, r *http.Request) error {
	if err := s.agentOff(); err != nil {
		return err
	}
	st, err := s.Agent.Undo(r.Context(), r.PathValue("id"))
	if errors.Is(err, agent.ErrNoUndo) {
		return store.Fail(store.ErrConflict, "%s", err.Error())
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

// AgentHandler serves agent.sock: Gwen's tools for `gwen mcp`, and nothing
// else, so the agent never reaches the approvals or the rest of the API.
func (s *Server) AgentHandler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, h func(http.ResponseWriter, *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				s.fail(w, r, err)
			}
		})
	}
	route("GET /v1/agent/tools", func(w http.ResponseWriter, r *http.Request) error {
		writeJSON(w, http.StatusOK, wire.AgentToolList{Tools: s.Tools()})
		return nil
	})
	route("POST /v1/agent/tools/{name}", func(w http.ResponseWriter, r *http.Request) error {
		if err := s.agentOff(); err != nil {
			return err
		}
		var input json.RawMessage
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
			return badRequest("input", "the tool's input is not JSON")
		}
		res, err := s.Agent.Tool(r.Context(), r.Header.Get(AgentTokenHeader), r.PathValue("name"), input)
		if errors.Is(err, agent.ErrNoToken) {
			return &requestError{code: wire.CodeUnauthorized, status: http.StatusForbidden, msg: "that token belongs to no running chat"}
		}
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, res)
		return nil
	})
	return mux
}
