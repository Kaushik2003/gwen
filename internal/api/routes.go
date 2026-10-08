package api

import (
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/timeengine"
)

// Handler returns the router for every route.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, h func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				s.fail(w, r, err)
			}
		})
	}
	route("GET /v1/health", s.health)
	route("GET /v1/status", s.status)
	route("GET /v1/events", s.events)

	route("POST /v1/day/clock-in", s.clockIn)
	route("POST /v1/day/clock-out", s.command(func(at time.Time) timeengine.Input { return timeengine.ClockOut{At: at} }))
	route("POST /v1/break/start", s.command(func(at time.Time) timeengine.Input { return timeengine.BreakStart{At: at} }))
	route("POST /v1/break/end", s.command(func(at time.Time) timeengine.Input { return timeengine.BreakEnd{At: at} }))
	route("POST /v1/session/switch", s.switchTo)
	route("POST /v1/nudge/snooze", s.command(func(at time.Time) timeengine.Input { return timeengine.Snooze{At: at} }))

	route("GET /v1/days", s.listDays)
	route("GET /v1/days/{day}", s.getDay)
	route("PATCH /v1/days/{day}", s.patchDay)
	route("POST /v1/segments", s.createSegment)
	route("PATCH /v1/segments/{id}", s.patchSegment)
	route("POST /v1/segments/{id}/split", s.splitSegment)
	route("DELETE /v1/segments/{id}", s.deleteSegment)

	route("GET /v1/projects", s.listProjects)
	route("POST /v1/projects", s.createProject)
	route("GET /v1/projects/{id}", s.getProject)
	route("PATCH /v1/projects/{id}", s.patchProject)
	route("DELETE /v1/projects/{id}", s.deleteProject)

	route("GET /v1/tasks", s.listTasks)
	route("POST /v1/tasks", s.createTask)
	route("GET /v1/tasks/{id}", s.getTask)
	route("PATCH /v1/tasks/{id}", s.patchTask)
	route("POST /v1/tasks/{id}/complete", s.completeTask)
	route("POST /v1/tasks/{id}/reopen", s.reopenTask)
	route("DELETE /v1/tasks/{id}", s.deleteTask)
	route("POST /v1/tasks/delete", s.bulkDeleteTasks)

	route("GET /v1/stats/summary", s.statsSummary)
	route("GET /v1/stats/heatmap", s.statsHeatmap)

	route("GET /v1/goals", s.listGoals)
	route("POST /v1/goals", s.createGoal)
	route("POST /v1/goals/preview", s.previewGoal)
	route("GET /v1/goals/{id}", s.getGoal)
	route("PATCH /v1/goals/{id}", s.patchGoal)
	route("DELETE /v1/goals/{id}", s.deleteGoal)
	route("GET /v1/commitments", s.listCommitments)
	route("POST /v1/commitments", s.createCommitment)
	route("PATCH /v1/commitments/{id}", s.patchCommitment)
	route("DELETE /v1/commitments/{id}", s.deleteCommitment)
	route("GET /v1/plan", s.getPlan)
	route("POST /v1/plan/generate", s.generatePlan)
	route("POST /v1/plan/hours", s.setDayHours)
	route("PATCH /v1/plan/items/{id}", s.patchPlanItem)
	route("POST /v1/plan/move", s.movePlanTask)
	route("POST /v1/plan/schedule", s.scheduleTask)
	route("POST /v1/plan/unschedule", s.unscheduleTask)
	route("GET /v1/briefing", s.briefing)

	route("GET /v1/reviews/{week}", s.getReview)
	route("PUT /v1/reviews/{week}", s.saveReview)
	route("GET /v1/energy", s.energyReport)
	route("POST /v1/energy", s.logEnergy)
	route("DELETE /v1/energy/{id}", s.deleteEnergy)

	route("GET /v1/calendar/status", s.calendarStatus)
	route("POST /v1/calendar/auth/start", s.calendarAuthStart)
	route("POST /v1/calendar/sync", s.calendarSync)
	route("GET /v1/calendar/calendars", s.calendarCalendars)

	route("POST /v1/goals/{id}/breakdown", s.breakdown)
	route("POST /v1/plan/chat", s.planChat)
	route("POST /v1/retro", s.retro)
	route("POST /v1/assistant/chat", s.assistantChat)
	route("GET /v1/llm/runs/{id}", s.getRun)
	route("POST /v1/llm/runs/{id}/accept", s.acceptRun)
	route("POST /v1/llm/runs/{id}/reject", s.rejectRun)

	route("GET /v1/sync/status", s.syncStatusHandler)
	route("POST /v1/sync/now", s.syncNow)

	route("GET /v1/config", s.getConfig)
	route("PATCH /v1/config", s.patchConfig)
	route("POST /v1/notify/test", s.notifyTest)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, errNotFoundRoute)
	})
	return mux
}

// ReadOnlyHandler returns the router the sync hub serves under /v1: every
// GET endpoint except status, events, config, and briefing, with plans read
// as stored (docs/07-integrations.md#read-only-dashboard).
func (s *Server) ReadOnlyHandler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, h func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				s.fail(w, r, err)
			}
		})
	}
	route("GET /v1/health", s.health)
	route("GET /v1/days", s.listDays)
	route("GET /v1/days/{day}", s.getDay)
	route("GET /v1/projects", s.listProjects)
	route("GET /v1/projects/{id}", s.getProject)
	route("GET /v1/tasks", s.listTasks)
	route("GET /v1/tasks/{id}", s.getTask)
	route("GET /v1/stats/summary", s.statsSummary)
	route("GET /v1/stats/heatmap", s.statsHeatmap)
	route("GET /v1/goals", s.listGoals)
	route("GET /v1/goals/{id}", s.getGoal)
	route("GET /v1/commitments", s.listCommitments)
	route("GET /v1/plan", s.storedPlan)
	route("GET /v1/calendar/status", s.calendarStatus)
	route("GET /v1/sync/status", s.syncStatusHandler)
	route("GET /v1/llm/runs/{id}", s.getRun)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, errNotFoundRoute)
	})
	return mux
}
