package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/kzark/gwen/internal/wire"
)

func (c *Client) CodeSessions(ctx context.Context) (*wire.CodeSessionList, error) {
	return call[wire.CodeSessionList](ctx, c, http.MethodGet, "/v1/code/sessions", nil, nil)
}

func (c *Client) StartCodeSession(ctx context.Context, req wire.StartCodeSession) (*wire.CodeSession, error) {
	return call[wire.CodeSession](ctx, c, http.MethodPost, "/v1/code/sessions", nil, req)
}

func (c *Client) RemoteStatus(ctx context.Context) (*wire.RemoteStatus, error) {
	return call[wire.RemoteStatus](ctx, c, http.MethodGet, "/v1/remote/status", nil, nil)
}

func (c *Client) RemotePairCode(ctx context.Context) (*wire.RemotePairCode, error) {
	return call[wire.RemotePairCode](ctx, c, http.MethodPost, "/v1/remote/pair-code", nil, empty)
}

func (c *Client) RemoteDevices(ctx context.Context) (*wire.RemoteDevices, error) {
	return call[wire.RemoteDevices](ctx, c, http.MethodGet, "/v1/remote/devices", nil, nil)
}

func (c *Client) RevokeRemoteDevice(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/remote/devices/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) SetRemoteHooks(ctx context.Context, on bool) (*wire.RemoteStatus, error) {
	return call[wire.RemoteStatus](ctx, c, http.MethodPost, "/v1/remote/hooks", nil, wire.RemoteHooks{On: on})
}

func (c *Client) CommitSessions(ctx context.Context, dir string) (*wire.CommitSessions, error) {
	return call[wire.CommitSessions](ctx, c, http.MethodGet, "/v1/code/commit-sessions", url.Values{"dir": {dir}}, nil)
}

func (c *Client) SummarizeCommit(ctx context.Context, req wire.SummarizeCommit) (*wire.CommitSummary, error) {
	return call[wire.CommitSummary](ctx, c, http.MethodPost, "/v1/code/commit-sessions/summary", nil, req)
}

func (c *Client) ClaudeUsage(ctx context.Context) (*wire.ClaudeUsage, error) {
	return call[wire.ClaudeUsage](ctx, c, http.MethodGet, "/v1/code/usage", nil, nil)
}

func (c *Client) CodeAnswer(ctx context.Context, id string, req wire.CodeAnswer) (*wire.CodeAnswerResult, error) {
	return call[wire.CodeAnswerResult](ctx, c, http.MethodPost, "/v1/code/sessions/"+url.PathEscape(id)+"/answer", nil, req)
}

func (c *Client) CodeMark(ctx context.Context, id string, req wire.CodeMark) error {
	return c.do(ctx, http.MethodPost, "/v1/code/sessions/"+url.PathEscape(id)+"/mark", nil, req, nil)
}

func (c *Client) CodeControl(ctx context.Context, id string, req wire.CodeControl) error {
	return c.do(ctx, http.MethodPost, "/v1/code/sessions/"+url.PathEscape(id)+"/control", nil, req, nil)
}

func (c *Client) CodeConversations(ctx context.Context, dir string) (*wire.CodeConversations, error) {
	return call[wire.CodeConversations](ctx, c, http.MethodGet, "/v1/code/conversations", url.Values{"dir": {dir}}, nil)
}
