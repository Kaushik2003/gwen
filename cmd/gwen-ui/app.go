package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound into the frontend. Its methods mirror client.API one to one,
// with the same names and wire types and without the context, except Events:
// the host forwards the event stream itself (docs/08-clients.md#gui).
type App struct {
	ctx     context.Context
	api     client.API
	host    *host
	voice   *dictation
	screen  string // the screen to open on, from --screen
	nowCard bool   // the tray's now card, from --now
}

// hostError carries an API failure to the frontend as JSON, so the UI can
// read the code and details (details.key on config errors, for instance).
type hostError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func (e *hostError) Error() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// codeDaemonNotRunning is what the frontend gets while gwend is down.
const codeDaemonNotRunning = "daemon_not_running"

func wrap(err error) error {
	if err == nil {
		return nil
	}
	var ae *client.APIError
	switch {
	case errors.As(err, &ae):
		return &hostError{Code: ae.Code, Message: ae.Message, Details: ae.Details}
	case errors.Is(err, client.ErrDaemonNotRunning):
		return &hostError{Code: codeDaemonNotRunning, Message: "Gwen isn't running", Details: map[string]any{}}
	}
	return &hostError{Code: wire.CodeInternal, Message: err.Error(), Details: map[string]any{}}
}

func call[T any](v T, err error) (T, error) { return v, wrap(err) }

func (a *App) Health() (*wire.Health, error) { return call(a.api.Health(a.ctx)) }
func (a *App) Status() (*wire.Status, error) { return call(a.api.Status(a.ctx)) }

func (a *App) ClockIn(req wire.ClockInRequest) (*wire.Status, error) {
	return call(a.api.ClockIn(a.ctx, req))
}
func (a *App) ClockOut() (*wire.Status, error)   { return call(a.api.ClockOut(a.ctx)) }
func (a *App) BreakStart() (*wire.Status, error) { return call(a.api.BreakStart(a.ctx)) }
func (a *App) BreakEnd() (*wire.Status, error)   { return call(a.api.BreakEnd(a.ctx)) }
func (a *App) Switch(req wire.SwitchRequest) (*wire.Status, error) {
	return call(a.api.Switch(a.ctx, req))
}
func (a *App) Snooze() (*wire.Status, error) { return call(a.api.Snooze(a.ctx)) }

func (a *App) ListDays(from, to string) (*wire.DayList, error) {
	return call(a.api.ListDays(a.ctx, from, to))
}
func (a *App) GetDay(day string) (*wire.DayDetail, error) { return call(a.api.GetDay(a.ctx, day)) }
func (a *App) PatchDay(day string, req wire.PatchDayRequest) (*wire.WorkDay, error) {
	return call(a.api.PatchDay(a.ctx, day, req))
}
func (a *App) CreateSegment(req wire.CreateSegmentRequest) (*wire.Segment, error) {
	return call(a.api.CreateSegment(a.ctx, req))
}
func (a *App) PatchSegment(id string, req wire.PatchSegmentRequest) (*wire.Segment, error) {
	return call(a.api.PatchSegment(a.ctx, id, req))
}
func (a *App) SplitSegment(id string, req wire.SplitSegmentRequest) (*wire.SegmentList, error) {
	return call(a.api.SplitSegment(a.ctx, id, req))
}
func (a *App) DeleteSegment(id string) error { return wrap(a.api.DeleteSegment(a.ctx, id)) }

func (a *App) ListProjects(archived string) (*wire.ProjectList, error) {
	return call(a.api.ListProjects(a.ctx, archived))
}
func (a *App) CreateProject(req wire.CreateProjectRequest) (*wire.Project, error) {
	return call(a.api.CreateProject(a.ctx, req))
}
func (a *App) GetProject(id string) (*wire.Project, error) { return call(a.api.GetProject(a.ctx, id)) }
func (a *App) PatchProject(id string, req wire.PatchProjectRequest) (*wire.Project, error) {
	return call(a.api.PatchProject(a.ctx, id, req))
}
func (a *App) DeleteProject(id string) error { return wrap(a.api.DeleteProject(a.ctx, id)) }

func (a *App) ListTasks(q wire.TaskQuery) (*wire.TaskList, error) {
	return call(a.api.ListTasks(a.ctx, q))
}
func (a *App) CreateTask(req wire.CreateTaskRequest) (*wire.Task, error) {
	return call(a.api.CreateTask(a.ctx, req))
}
func (a *App) GetTask(id string) (*wire.Task, error) { return call(a.api.GetTask(a.ctx, id)) }
func (a *App) PatchTask(id string, req wire.PatchTaskRequest) (*wire.Task, error) {
	return call(a.api.PatchTask(a.ctx, id, req))
}
func (a *App) CompleteTask(id string, req wire.CompleteTaskRequest) (*wire.Task, error) {
	return call(a.api.CompleteTask(a.ctx, id, req))
}
func (a *App) ReopenTask(id string) (*wire.Task, error) { return call(a.api.ReopenTask(a.ctx, id)) }
func (a *App) DeleteTask(id string) error               { return wrap(a.api.DeleteTask(a.ctx, id)) }
func (a *App) DeleteTasks(req wire.DeleteTasksRequest) (*wire.DeletedTasks, error) {
	return call(a.api.DeleteTasks(a.ctx, req))
}

func (a *App) StatsSummary(from, to string) (*wire.StatsSummary, error) {
	return call(a.api.StatsSummary(a.ctx, from, to))
}
func (a *App) StatsHeatmap(year int) (*wire.Heatmap, error) {
	return call(a.api.StatsHeatmap(a.ctx, year))
}

func (a *App) GetConfig() (*wire.Config, error) { return call(a.api.GetConfig(a.ctx)) }
func (a *App) PatchConfig(patch wire.ConfigPatch) (*wire.Config, error) {
	return call(a.api.PatchConfig(a.ctx, patch))
}

func (a *App) NotifyTest() (*wire.NotifyTestResult, error) { return call(a.api.NotifyTest(a.ctx)) }

func (a *App) ListGoals(status string) (*wire.GoalList, error) {
	return call(a.api.ListGoals(a.ctx, status))
}
func (a *App) CreateGoal(req wire.CreateGoalRequest) (*wire.Goal, error) {
	return call(a.api.CreateGoal(a.ctx, req))
}
func (a *App) GetGoal(id string) (*wire.Goal, error) { return call(a.api.GetGoal(a.ctx, id)) }
func (a *App) PatchGoal(id string, req wire.PatchGoalRequest) (*wire.Goal, error) {
	return call(a.api.PatchGoal(a.ctx, id, req))
}
func (a *App) DeleteGoal(id string, openTasks bool) error {
	return wrap(a.api.DeleteGoal(a.ctx, id, openTasks))
}
func (a *App) PreviewGoal(req wire.GoalPreviewRequest) (*wire.GoalProgress, error) {
	return call(a.api.PreviewGoal(a.ctx, req))
}

func (a *App) ListCommitments() (*wire.CommitmentList, error) {
	return call(a.api.ListCommitments(a.ctx))
}
func (a *App) CreateCommitment(req wire.CreateCommitmentRequest) (*wire.Commitment, error) {
	return call(a.api.CreateCommitment(a.ctx, req))
}
func (a *App) PatchCommitment(id string, req wire.PatchCommitmentRequest) (*wire.Commitment, error) {
	return call(a.api.PatchCommitment(a.ctx, id, req))
}
func (a *App) DeleteCommitment(id string) error { return wrap(a.api.DeleteCommitment(a.ctx, id)) }

func (a *App) GetPlan(day string) (*wire.Plan, error) { return call(a.api.GetPlan(a.ctx, day)) }
func (a *App) GeneratePlan(req wire.GeneratePlanRequest) (*wire.Plan, error) {
	return call(a.api.GeneratePlan(a.ctx, req))
}
func (a *App) SetDayHours(req wire.SetDayHoursRequest) (*wire.Plan, error) {
	return call(a.api.SetDayHours(a.ctx, req))
}
func (a *App) PatchPlanItem(id string, req wire.PatchPlanItemRequest) (*wire.PlanItem, error) {
	return call(a.api.PatchPlanItem(a.ctx, id, req))
}
func (a *App) MovePlanTask(req wire.MovePlanTaskRequest) (*wire.Plan, error) {
	return call(a.api.MovePlanTask(a.ctx, req))
}
func (a *App) Briefing() (*wire.Briefing, error) { return call(a.api.Briefing(a.ctx)) }

// Host-only methods.

// StartScreen is the screen the dashboard was opened on, once; "" after.
func (a *App) StartScreen() string {
	s := a.screen
	a.screen = ""
	return s
}

// IsNowCard reports whether this window is the tray's now card rather than
// the dashboard.
func (a *App) IsNowCard() bool { return a.nowCard }

// OpenDashboard opens the dashboard on a screen, or "" for Today; a running
// dashboard comes to the front instead.
func (a *App) OpenDashboard(screen string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "gwen-ui"
	}
	var args []string
	if screen != "" {
		args = []string{"--screen", screen}
	}
	return wrap(a.host.start(exe, args...))
}

// Version is the GUI's own version.
func (a *App) Version() string { return version }

// NewTopic returns a fresh ntfy topic.
func (a *App) NewTopic() string { return a.host.newTopic() }

// SetCredential writes one of the credential files of
// docs/07-integrations.md#credentials.
func (a *App) SetCredential(name, value string) error { return wrap(a.host.setCredential(name, value)) }

// OpenURL opens a link in the default browser.
func (a *App) OpenURL(url string) error { return wrap(a.host.openURL(url)) }

// EnableService starts gwend now and at every login, as gwen setup service.
func (a *App) EnableService() error { return wrap(a.host.enableService(a.ctx)) }

// SetAutostart adds or removes the tray from autostart, as gwen setup autostart.
func (a *App) SetAutostart(enable bool) error { return wrap(a.host.setAutostart(enable)) }

// Autostart reports whether the tray starts at login.
func (a *App) Autostart() bool { return a.host.autostart() }

// HasCredential reports whether a credential file is stored, without reading it.
func (a *App) HasCredential(name string) bool { return a.host.hasCredential(name) }

// ClaudeCode reports whether command is a signed-in Claude Code CLI, as gwen setup llm checks.
func (a *App) ClaudeCode(command string) ClaudeCodeStatus { return a.host.claudeCode(a.ctx, command) }

// ChooseGoogleClient asks for the Google OAuth client JSON in a file dialog,
// then installs it and turns the calendar on, as gwen setup calendar
// --client-file does. It is false when the dialog was cancelled.
func (a *App) ChooseGoogleClient() (bool, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Choose the OAuth client file from Google Cloud Console",
		Filters: []runtime.FileFilter{{DisplayName: "OAuth client (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return false, wrap(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, wrap(err)
	}
	return true, wrap(a.host.installGoogleClient(a.ctx, b))
}
