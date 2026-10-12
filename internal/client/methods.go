package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kzark/gwen/internal/wire"
)

func (c *Client) AssistantChat(ctx context.Context, req wire.AssistantChatRequest) (*wire.LlmRun, error) {
	return call[wire.LlmRun](ctx, c, http.MethodPost, "/v1/assistant/chat", nil, req)
}

func (c *Client) AssistantSelf(ctx context.Context) (*wire.AssistantSelf, error) {
	return call[wire.AssistantSelf](ctx, c, http.MethodGet, "/v1/assistant/self", nil, nil)
}

func (c *Client) ResetAssistantSelf(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/v1/assistant/self", nil, nil, nil)
}

// ListThreads lists the assistant chats after the first offset; limit 0
// means the daemon's default.
func (c *Client) ListThreads(ctx context.Context, limit, offset int) (*wire.ChatThreadList, error) {
	return call[wire.ChatThreadList](ctx, c, http.MethodGet, "/v1/assistant/threads",
		query("limit", count(limit), "offset", count(offset)), nil)
}

func (c *Client) GetThread(ctx context.Context, id string) (*wire.ChatThreadDetail, error) {
	return call[wire.ChatThreadDetail](ctx, c, http.MethodGet, "/v1/assistant/threads/"+url.PathEscape(id), nil, nil)
}

func (c *Client) DeleteThread(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/assistant/threads/"+url.PathEscape(id), nil, nil, nil)
}

// SearchThreads finds the messages of every chat that share words with q;
// limit 0 means the daemon's default.
func (c *Client) SearchThreads(ctx context.Context, q string, limit int) (*wire.ChatSearchResult, error) {
	return call[wire.ChatSearchResult](ctx, c, http.MethodGet, "/v1/assistant/search", query("q", q, "limit", count(limit)), nil)
}

func (c *Client) ScheduleTask(ctx context.Context, req wire.ScheduleRequest) (*wire.PlanItem, error) {
	return call[wire.PlanItem](ctx, c, http.MethodPost, "/v1/plan/schedule", nil, req)
}

func (c *Client) UnscheduleTask(ctx context.Context, req wire.UnscheduleRequest) (*wire.Unscheduled, error) {
	return call[wire.Unscheduled](ctx, c, http.MethodPost, "/v1/plan/unschedule", nil, req)
}

func (c *Client) GetReview(ctx context.Context, weekStart string) (*wire.WeeklyReview, error) {
	return call[wire.WeeklyReview](ctx, c, http.MethodGet, "/v1/reviews/"+url.PathEscape(weekStart), nil, nil)
}

func (c *Client) SaveReview(ctx context.Context, weekStart string, req wire.SaveReviewRequest) (*wire.WeeklyReview, error) {
	return call[wire.WeeklyReview](ctx, c, http.MethodPut, "/v1/reviews/"+url.PathEscape(weekStart), nil, req)
}

// EnergyReport reads the check-ins of the last days days; 0 means the
// daemon's default.
func (c *Client) EnergyReport(ctx context.Context, days int) (*wire.EnergyReport, error) {
	return call[wire.EnergyReport](ctx, c, http.MethodGet, "/v1/energy", query("days", count(days)), nil)
}

// count is n as a query value, or "" for 0, which leaves it out.
func count(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func (c *Client) LogEnergy(ctx context.Context, req wire.LogEnergyRequest) (*wire.EnergyLog, error) {
	return call[wire.EnergyLog](ctx, c, http.MethodPost, "/v1/energy", nil, req)
}

func (c *Client) DeleteEnergy(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/energy/"+url.PathEscape(id), nil, nil, nil)
}
