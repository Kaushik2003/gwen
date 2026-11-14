package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kzark/gwen/internal/wire"
)

func (c *Client) ListRuns(ctx context.Context, statuses []string, limit int) (*wire.RunList, error) {
	q := url.Values{}
	if len(statuses) > 0 {
		q.Set("status", strings.Join(statuses, ","))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return call[wire.RunList](ctx, c, http.MethodGet, "/v1/runs", q, nil)
}

func (c *Client) CreateRun(ctx context.Context, req wire.CreateRunRequest) (*wire.Run, error) {
	return call[wire.Run](ctx, c, http.MethodPost, "/v1/runs", nil, req)
}

func (c *Client) GetRun(ctx context.Context, id string) (*wire.RunDetail, error) {
	return call[wire.RunDetail](ctx, c, http.MethodGet, "/v1/runs/"+url.PathEscape(id), nil, nil)
}

func (c *Client) StartRun(ctx context.Context, id string) (*wire.Run, error) {
	return call[wire.Run](ctx, c, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/start", nil, empty)
}

func (c *Client) CancelRun(ctx context.Context, id string) (*wire.Run, error) {
	return call[wire.Run](ctx, c, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/cancel", nil, empty)
}

func (c *Client) ResumeRun(ctx context.Context, id string) (*wire.Run, error) {
	return call[wire.Run](ctx, c, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/resume", nil, empty)
}

func (c *Client) MessageRun(ctx context.Context, id string, req wire.RunMessageRequest) (*wire.Run, error) {
	return call[wire.Run](ctx, c, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/message", nil, req)
}

func (c *Client) ListApprovals(ctx context.Context, status string) (*wire.ApprovalList, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	return call[wire.ApprovalList](ctx, c, http.MethodGet, "/v1/approvals", q, nil)
}

func (c *Client) DecideApproval(ctx context.Context, id string, req wire.DecideApprovalRequest) (*wire.Approval, error) {
	return call[wire.Approval](ctx, c, http.MethodPost, "/v1/approvals/"+url.PathEscape(id), nil, req)
}

func (c *Client) GetResources(ctx context.Context, subject string) (*wire.ResourceList, error) {
	return call[wire.ResourceList](ctx, c, http.MethodGet, "/v1/resources", url.Values{"subject": {subject}}, nil)
}

func (c *Client) PutResources(ctx context.Context, subject string, req wire.PutResourcesRequest) (*wire.ResourceList, error) {
	return call[wire.ResourceList](ctx, c, http.MethodPut, "/v1/resources", url.Values{"subject": {subject}}, req)
}

func (c *Client) GetRoutine(ctx context.Context, subject string) (*wire.Routine, error) {
	return call[wire.Routine](ctx, c, http.MethodGet, "/v1/routines", url.Values{"subject": {subject}}, nil)
}

func (c *Client) PutRoutine(ctx context.Context, subject string, req wire.PutRoutineRequest) (*wire.Routine, error) {
	return call[wire.Routine](ctx, c, http.MethodPut, "/v1/routines", url.Values{"subject": {subject}}, req)
}

func (c *Client) GetProjectDir(ctx context.Context, id string) (*wire.ProjectDir, error) {
	return call[wire.ProjectDir](ctx, c, http.MethodGet, "/v1/projects/"+url.PathEscape(id)+"/dir", nil, nil)
}

func (c *Client) PutProjectDir(ctx context.Context, id string, req wire.ProjectDir) (*wire.ProjectDir, error) {
	return call[wire.ProjectDir](ctx, c, http.MethodPut, "/v1/projects/"+url.PathEscape(id)+"/dir", nil, req)
}

func (c *Client) ListConnections(ctx context.Context) (*wire.ConnectionList, error) {
	return call[wire.ConnectionList](ctx, c, http.MethodGet, "/v1/connections", nil, nil)
}

func (c *Client) ConnectConnection(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v1/connections/"+url.PathEscape(id)+"/connect", nil, empty, nil)
}

func (c *Client) DisconnectConnection(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/connections/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) ListGrants(ctx context.Context) (*wire.GrantList, error) {
	return call[wire.GrantList](ctx, c, http.MethodGet, "/v1/grants", nil, nil)
}

func (c *Client) DeleteGrant(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/grants/"+url.PathEscape(id), nil, nil, nil)
}
