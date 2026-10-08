package client

import (
	"context"
	"net/http"

	"github.com/kzark/gwen/internal/wire"
)

func (c *Client) CalendarStatus(ctx context.Context) (*wire.CalendarStatus, error) {
	return call[wire.CalendarStatus](ctx, c, http.MethodGet, "/v1/calendar/status", nil, nil)
}

func (c *Client) CalendarAuthStart(ctx context.Context) (*wire.CalendarAuth, error) {
	return call[wire.CalendarAuth](ctx, c, http.MethodPost, "/v1/calendar/auth/start", nil, empty)
}

func (c *Client) CalendarSync(ctx context.Context) (*wire.CalendarStatus, error) {
	return call[wire.CalendarStatus](ctx, c, http.MethodPost, "/v1/calendar/sync", nil, empty)
}

func (c *Client) CalendarCalendars(ctx context.Context) (*wire.CalendarList, error) {
	return call[wire.CalendarList](ctx, c, http.MethodGet, "/v1/calendar/calendars", nil, nil)
}
