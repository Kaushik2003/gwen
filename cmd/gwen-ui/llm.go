package main

import "github.com/kzark/gwen/internal/wire"

func (a *App) GoalBreakdown(goalID string, req wire.BreakdownRequest) (*wire.LlmRun, error) {
	return call(a.api.GoalBreakdown(a.ctx, goalID, req))
}
func (a *App) PlanChat(req wire.PlanChatRequest) (*wire.LlmRun, error) {
	return call(a.api.PlanChat(a.ctx, req))
}
func (a *App) Retro(req wire.RetroRequest) (*wire.LlmRun, error) {
	return call(a.api.Retro(a.ctx, req))
}
func (a *App) GetLLMRun(id string) (*wire.LlmRun, error) { return call(a.api.GetLLMRun(a.ctx, id)) }
func (a *App) AcceptLLMRun(id string, req wire.AcceptRunRequest) (*wire.TaskList, error) {
	return call(a.api.AcceptLLMRun(a.ctx, id, req))
}
func (a *App) RejectLLMRun(id string) (*wire.LlmRun, error) {
	return call(a.api.RejectLLMRun(a.ctx, id))
}
func (a *App) AssistantChat(req wire.AssistantChatRequest) (*wire.LlmRun, error) {
	return call(a.api.AssistantChat(a.ctx, req))
}
func (a *App) AssistantSelf() (*wire.AssistantSelf, error) { return call(a.api.AssistantSelf(a.ctx)) }
func (a *App) ResetAssistantSelf() error                   { return wrap(a.api.ResetAssistantSelf(a.ctx)) }

func (a *App) ScheduleTask(req wire.ScheduleRequest) (*wire.PlanItem, error) {
	return call(a.api.ScheduleTask(a.ctx, req))
}
func (a *App) UnscheduleTask(req wire.UnscheduleRequest) (*wire.Unscheduled, error) {
	return call(a.api.UnscheduleTask(a.ctx, req))
}
func (a *App) GetReview(weekStart string) (*wire.WeeklyReview, error) {
	return call(a.api.GetReview(a.ctx, weekStart))
}
func (a *App) SaveReview(weekStart string, req wire.SaveReviewRequest) (*wire.WeeklyReview, error) {
	return call(a.api.SaveReview(a.ctx, weekStart, req))
}
func (a *App) EnergyReport(days int) (*wire.EnergyReport, error) {
	return call(a.api.EnergyReport(a.ctx, days))
}
func (a *App) LogEnergy(req wire.LogEnergyRequest) (*wire.EnergyLog, error) {
	return call(a.api.LogEnergy(a.ctx, req))
}
func (a *App) DeleteEnergy(id string) error { return wrap(a.api.DeleteEnergy(a.ctx, id)) }
