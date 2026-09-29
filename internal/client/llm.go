package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/kzark/gwen/internal/wire"
)

func (c *Client) GoalBreakdown(ctx context.Context, goalID string, req wire.BreakdownRequest) (*wire.LlmRun, error) {
	return call[wire.LlmRun](ctx, c, http.MethodPost, "/v1/goals/"+url.PathEscape(goalID)+"/breakdown", nil, req)
}

func (c *Client) Retro(ctx context.Context, req wire.RetroRequest) (*wire.LlmRun, error) {
	return call[wire.LlmRun](ctx, c, http.MethodPost, "/v1/retro", nil, req)
}

func (c *Client) GetLLMRun(ctx context.Context, id string) (*wire.LlmRun, error) {
	return call[wire.LlmRun](ctx, c, http.MethodGet, "/v1/llm/runs/"+url.PathEscape(id), nil, nil)
}

func (c *Client) AcceptLLMRun(ctx context.Context, id string, req wire.AcceptRunRequest) (*wire.TaskList, error) {
	return call[wire.TaskList](ctx, c, http.MethodPost, "/v1/llm/runs/"+url.PathEscape(id)+"/accept", nil, req)
}

func (c *Client) RejectLLMRun(ctx context.Context, id string) (*wire.LlmRun, error) {
	return call[wire.LlmRun](ctx, c, http.MethodPost, "/v1/llm/runs/"+url.PathEscape(id)+"/reject", nil, empty)
}
