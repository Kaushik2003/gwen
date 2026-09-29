package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) GoalBreakdown(_ context.Context, goalID string, req wire.BreakdownRequest) (*wire.LlmRun, error) {
	return result[*wire.LlmRun](f, "GoalBreakdown", goalID, req)
}

func (f *Fake) Retro(_ context.Context, req wire.RetroRequest) (*wire.LlmRun, error) {
	return result[*wire.LlmRun](f, "Retro", req)
}

func (f *Fake) GetLLMRun(_ context.Context, id string) (*wire.LlmRun, error) {
	return result[*wire.LlmRun](f, "GetLLMRun", id)
}

func (f *Fake) AcceptLLMRun(_ context.Context, id string, req wire.AcceptRunRequest) (*wire.TaskList, error) {
	return result[*wire.TaskList](f, "AcceptLLMRun", id, req)
}

func (f *Fake) RejectLLMRun(_ context.Context, id string) (*wire.LlmRun, error) {
	return result[*wire.LlmRun](f, "RejectLLMRun", id)
}
