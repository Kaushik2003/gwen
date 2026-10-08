package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/kzark/gwen/internal/gcal"
	"github.com/kzark/gwen/internal/wire"
)

// CalendarService is the Google Calendar sync as the API sees it
// (docs/07-integrations.md#google-calendar).
type CalendarService interface {
	// Available is nil when the calendar is enabled and its OAuth client exists.
	Available() error
	Status(ctx context.Context) gcal.Status
	AuthStart(ctx context.Context) (string, error)
	Sync(ctx context.Context) (gcal.Status, error)
	Calendars(ctx context.Context) ([]gcal.Calendar, error)
}

// calendar returns the service, or unavailable when it is not configured.
func (s *Server) calendar() (CalendarService, error) {
	if s.Calendar == nil {
		return nil, Unavailable("the calendar is not running in this daemon")
	}
	if err := s.Calendar.Available(); err != nil {
		return nil, calendarErr(err)
	}
	return s.Calendar, nil
}

func calendarErr(err error) error {
	if errors.Is(err, gcal.ErrUnavailable) {
		return Unavailable("%s", err.Error())
	}
	return err
}

func calendarWire(st gcal.Status) wire.CalendarStatus {
	out := wire.CalendarStatus{Enabled: st.Enabled, Connected: st.Connected, LastSyncAt: wire.MillisPtr(st.LastSyncAt)}
	if st.CalendarID != "" {
		out.CalendarID = &st.CalendarID
	}
	if st.LastError != "" {
		out.LastError = &st.LastError
	}
	return out
}

// PublishIntegration sends integration_changed with both integrations' status.
func (s *Server) PublishIntegration(ctx context.Context) {
	ev := wire.IntegrationChanged{Sync: s.syncStatus(ctx)}
	if s.Calendar != nil {
		ev.Calendar = calendarWire(s.Calendar.Status(ctx))
	}
	s.Hub.Publish(wire.EventIntegrationChanged, ev)
}

func (s *Server) calendarStatus(w http.ResponseWriter, r *http.Request) error {
	c, err := s.calendar()
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, calendarWire(c.Status(r.Context())))
	return nil
}

func (s *Server) calendarAuthStart(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	c, err := s.calendar()
	if err != nil {
		return err
	}
	url, err := c.AuthStart(r.Context())
	if err != nil {
		return calendarErr(err)
	}
	writeJSON(w, http.StatusOK, wire.CalendarAuth{AuthURL: url})
	return nil
}

func (s *Server) calendarSync(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if err := decode(r, &body); err != nil {
		return err
	}
	c, err := s.calendar()
	if err != nil {
		return err
	}
	st, err := c.Sync(r.Context())
	if err != nil {
		return calendarErr(err)
	}
	writeJSON(w, http.StatusOK, calendarWire(st))
	return nil
}

func (s *Server) calendarCalendars(w http.ResponseWriter, r *http.Request) error {
	c, err := s.calendar()
	if err != nil {
		return err
	}
	cals, err := c.Calendars(r.Context())
	if err != nil {
		return calendarErr(err)
	}
	out := wire.CalendarList{Calendars: []wire.CalendarInfo{}}
	for _, cal := range cals {
		out.Calendars = append(out.Calendars, wire.CalendarInfo{ID: cal.ID, Name: cal.Name, Color: cal.Color, Primary: cal.Primary})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
