package main

import (
	"context"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/composio"
	"github.com/kzark/gwen/internal/wire"
)

// searchLimit is how many items one app search shows.
const searchLimit = 15

// connectors is the API's Connectors over the composio command on this
// computer: task attachments from Gmail, Google Calendar, GitHub, and Notion.
type connectors struct{ c *composio.Client }

// newConnectors returns the Connectors over the composio command; the
// client finds it on PATH or in its usual install places, since a user
// service's PATH may leave out ~/.local/bin.
func newConnectors() connectors { return connectors{c: &composio.Client{}} }

func (x connectors) Connections(ctx context.Context) ([]wire.Connector, error) {
	// With Composio unreachable, the client still names every app as
	// unavailable; the picker shows that rather than an error.
	cs, err := x.c.Connections(ctx)
	if err != nil && len(cs) == 0 {
		return nil, err
	}
	out := make([]wire.Connector, len(cs))
	for i, c := range cs {
		out[i] = wire.Connector{App: c.App, Name: c.Name, Status: c.Status}
	}
	return out, nil
}

func (x connectors) Search(ctx context.Context, app, query string) ([]wire.ConnectorItem, error) {
	items, err := x.c.Search(ctx, app, query, searchLimit)
	if err != nil {
		return nil, err
	}
	out := make([]wire.ConnectorItem, len(items))
	for i, it := range items {
		out[i] = wire.ConnectorItem{App: it.App, Ref: it.Ref, Title: it.Title, URL: it.URL, Snippet: it.Snippet,
			When: wire.MillisPtr(it.When)}
	}
	return out, nil
}

func (x connectors) Fetch(ctx context.Context, app, ref string) (api.ConnectorSnapshot, error) {
	s, err := x.c.Fetch(ctx, app, ref)
	if err != nil {
		return api.ConnectorSnapshot{}, err
	}
	return api.ConnectorSnapshot{Title: s.Title, URL: s.URL, Body: s.Body}, nil
}

func (x connectors) Link(ctx context.Context, app string) (string, error) {
	return x.c.LinkURL(ctx, app)
}
