package main

import "github.com/kzark/gwen/internal/wire"

func (a *App) GoalBreakdown(goalID string, req wire.BreakdownRequest) (*wire.LlmRun, error) {
	return call(a.api.GoalBreakdown(a.ctx, goalID, req))
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
