package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Task attachments: what a task comes with for context. Links, notes, and
// files are kept as given; an app item (an email, an event, an issue, a page)
// is found and read through Connectors, and its text is kept as a snapshot so
// an agent handed the task, and Gwen in chat, can read it without asking the
// app again. Refresh takes a new snapshot.

// Connectors reaches apps through Composio.
type Connectors interface {
	// Connections returns each app Gwen attaches from and whether it is
	// connected.
	Connections(ctx context.Context) ([]wire.Connector, error)
	// Search finds items in an app; an empty query lists recent ones.
	Search(ctx context.Context, app, query string) ([]wire.ConnectorItem, error)
	// Fetch reads one item: its title, link, and text.
	Fetch(ctx context.Context, app, ref string) (ConnectorSnapshot, error)
	// Link returns the page that connects an app to Composio, for the
	// browser.
	Link(ctx context.Context, app string) (string, error)
}

// ConnectorSnapshot is an app item as read at one moment.
type ConnectorSnapshot struct {
	Title string
	URL   string
	Body  string
}

// connectorError is a failure reaching an app, with the adapter's own words.
func connectorError(err error) error {
	if err == nil {
		return nil
	}
	return &requestError{code: wire.CodeUnavailable, status: http.StatusServiceUnavailable, msg: err.Error()}
}

func (s *Server) noConnectors() error {
	return &requestError{code: wire.CodeUnavailable, status: http.StatusServiceUnavailable,
		msg: "Attaching from apps needs Composio on this computer: install the composio command and log in."}
}

func attachmentWire(a model.Attachment) wire.Attachment {
	return wire.Attachment{
		ID: a.ID, TaskID: a.TaskID, Kind: a.Kind, Title: a.Title, URL: a.URL, Path: a.Path, App: a.App, Ref: a.Ref,
		Body: a.Body, FetchedAt: wire.MillisPtr(a.FetchedAt), CreatedAt: wire.Millis(a.CreatedAt), Rev: a.Rev,
	}
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if _, err := s.Repos.Tasks.Get(r.Context(), id); err != nil {
		return err
	}
	as, err := s.Repos.Attachments.List(r.Context(), id)
	if err != nil {
		return err
	}
	out := wire.AttachmentList{Attachments: make([]wire.Attachment, len(as))}
	for i, a := range as {
		out.Attachments[i] = attachmentWire(a)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createAttachment(w http.ResponseWriter, r *http.Request) error {
	var req wire.CreateAttachmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	na := store.NewAttachment{TaskID: r.PathValue("id"), Kind: req.Kind, Title: req.Title, URL: req.URL,
		Path: req.Path, App: req.App, Ref: req.Ref, Body: req.Body}
	if req.Kind == model.AttachApp {
		if _, err := s.Repos.Tasks.Get(r.Context(), na.TaskID); err != nil {
			return err
		}
		if s.Connectors == nil {
			return s.noConnectors()
		}
		if strings.TrimSpace(req.Ref) == "" {
			return badRequest("ref", "an app item needs its id in the app")
		}
		snap, err := s.Connectors.Fetch(r.Context(), req.App, req.Ref)
		if err != nil {
			return connectorError(err)
		}
		at := s.now()
		na.Title, na.URL, na.Body, na.FetchedAt = firstNonEmpty(req.Title, snap.Title), snap.URL, snap.Body, &at
	}
	a, err := s.Repos.Attachments.Create(r.Context(), na)
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{a.TaskID}})
	writeJSON(w, http.StatusCreated, attachmentWire(a))
	return nil
}

func (s *Server) patchAttachment(w http.ResponseWriter, r *http.Request) error {
	var req wire.PatchAttachmentRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.Repos.Attachments.Update(r.Context(), r.PathValue("id"), store.AttachmentPatch{
		Title: req.Title, URL: req.URL, Body: req.Body, Rev: req.Rev,
	})
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{a.TaskID}})
	writeJSON(w, http.StatusOK, attachmentWire(a))
	return nil
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) error {
	a, err := s.Repos.Attachments.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := s.Repos.Attachments.Delete(r.Context(), a.ID); err != nil {
		return err
	}
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{a.TaskID}})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// refreshAttachment takes a new snapshot of an app item.
func (s *Server) refreshAttachment(w http.ResponseWriter, r *http.Request) error {
	a, err := s.Repos.Attachments.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if a.Kind != model.AttachApp {
		return badRequest("kind", "only an app item can be refreshed")
	}
	if s.Connectors == nil {
		return s.noConnectors()
	}
	snap, err := s.Connectors.Fetch(r.Context(), a.App, a.Ref)
	if err != nil {
		return connectorError(err)
	}
	at := s.now()
	title := firstNonEmpty(snap.Title, a.Title)
	a, err = s.Repos.Attachments.Update(r.Context(), a.ID, store.AttachmentPatch{
		Title: &title, URL: &snap.URL, Body: &snap.Body, FetchedAt: &at,
	})
	if err != nil {
		return err
	}
	s.Hub.Publish(wire.EventTasksChanged, wire.TasksChanged{TaskIDs: []string{a.TaskID}})
	writeJSON(w, http.StatusOK, attachmentWire(a))
	return nil
}

func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) error {
	if s.Connectors == nil {
		out := wire.ConnectorList{Connectors: []wire.Connector{}}
		for _, app := range store.AttachmentApps {
			out.Connectors = append(out.Connectors, wire.Connector{App: app, Name: app, Status: "unavailable"})
		}
		writeJSON(w, http.StatusOK, out)
		return nil
	}
	cs, err := s.Connectors.Connections(r.Context())
	if err != nil {
		return connectorError(err)
	}
	writeJSON(w, http.StatusOK, wire.ConnectorList{Connectors: cs})
	return nil
}

func (s *Server) searchConnector(w http.ResponseWriter, r *http.Request) error {
	if s.Connectors == nil {
		return s.noConnectors()
	}
	app := r.PathValue("app")
	if !slices.Contains(store.AttachmentApps, app) {
		return badRequest("app", "app must be one of %s", strings.Join(store.AttachmentApps, ", "))
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	items, err := s.Connectors.Search(ctx, app, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		return connectorError(err)
	}
	if items == nil {
		items = []wire.ConnectorItem{}
	}
	writeJSON(w, http.StatusOK, wire.ConnectorItems{Items: items})
	return nil
}

// linkConnector answers with the page that connects (or reconnects) an app.
func (s *Server) linkConnector(w http.ResponseWriter, r *http.Request) error {
	if s.Connectors == nil {
		return s.noConnectors()
	}
	app := r.PathValue("app")
	if !slices.Contains(store.AttachmentApps, app) {
		return badRequest("app", "app must be one of %s", strings.Join(store.AttachmentApps, ", "))
	}
	u, err := s.Connectors.Link(r.Context(), app)
	if err != nil {
		return connectorError(err)
	}
	writeJSON(w, http.StatusOK, wire.ConnectorLink{URL: u})
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
