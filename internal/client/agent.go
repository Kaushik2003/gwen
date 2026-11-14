package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/kzark/gwen/internal/wire"
)

// Gwen's agent (plan.md 3.16, W32).

func (c *Client) AgentStatus(ctx context.Context) (*wire.AgentStatus, error) {
	return call[wire.AgentStatus](ctx, c, http.MethodGet, "/v1/agent/status", nil, nil)
}

// ListAgentChats lists chats: a project's (its id), "everywhere", or "" for all.
func (c *Client) ListAgentChats(ctx context.Context, project string) (*wire.AgentChatList, error) {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	return call[wire.AgentChatList](ctx, c, http.MethodGet, "/v1/agent/chats", q, nil)
}

func (c *Client) CreateAgentChat(ctx context.Context, req wire.CreateAgentChat) (*wire.AgentChat, error) {
	return call[wire.AgentChat](ctx, c, http.MethodPost, "/v1/agent/chats", nil, req)
}

func (c *Client) GetAgentChat(ctx context.Context, id string) (*wire.AgentChatDetail, error) {
	return call[wire.AgentChatDetail](ctx, c, http.MethodGet, "/v1/agent/chats/"+url.PathEscape(id), nil, nil)
}

func (c *Client) PatchAgentChat(ctx context.Context, id string, req wire.PatchAgentChat) (*wire.AgentChat, error) {
	return call[wire.AgentChat](ctx, c, http.MethodPatch, "/v1/agent/chats/"+url.PathEscape(id), nil, req)
}

func (c *Client) DeleteAgentChat(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/agent/chats/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) SendAgentMessage(ctx context.Context, id string, req wire.AgentMessageRequest) (*wire.AgentChat, error) {
	return call[wire.AgentChat](ctx, c, http.MethodPost, "/v1/agent/chats/"+url.PathEscape(id)+"/messages", nil, req)
}

func (c *Client) StopAgentChat(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v1/agent/chats/"+url.PathEscape(id)+"/stop", nil, nil, nil)
}

func (c *Client) UndoAgentStep(ctx context.Context, id string) (*wire.AgentStep, error) {
	return call[wire.AgentStep](ctx, c, http.MethodPost, "/v1/agent/steps/"+url.PathEscape(id)+"/undo", nil, nil)
}
