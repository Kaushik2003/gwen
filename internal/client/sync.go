package client

import (
	"context"
	"net/http"

	"github.com/kzark/gwen/internal/wire"
)

func (c *Client) SyncStatus(ctx context.Context) (*wire.SyncStatus, error) {
	return call[wire.SyncStatus](ctx, c, http.MethodGet, "/v1/sync/status", nil, nil)
}

func (c *Client) SyncNow(ctx context.Context) (*wire.SyncStatus, error) {
	return call[wire.SyncStatus](ctx, c, http.MethodPost, "/v1/sync/now", nil, empty)
}
