package gcal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// idKey is the private extended property naming an event's plan item.
const idKey = "gwen_id"

func isStatus(err error, codes ...int) bool {
	var ge *googleapi.Error
	return errors.As(err, &ge) && slices.Contains(codes, ge.Code)
}

func (c *Changes) plan(day string) {
	if !slices.Contains(c.PlanDays, day) {
		c.PlanDays = append(c.PlanDays, day)
		slices.Sort(c.PlanDays)
	}
}

// cycle runs steps 1 to 4 of docs/07-integrations.md#sync-cycle.
func (s *Service) cycle(ctx context.Context) (Changes, error) {
	var ch Changes
	cfg, err := s.oauthConfig()
	if err != nil {
		return ch, err
	}
	tok, err := s.token()
	if err != nil {
		return ch, err
	}
	ctx = s.ctx(ctx)
	ts := cfg.TokenSource(ctx, tok)
	defer func() {
		// Keep a refreshed token, so the next cycle need not refresh again.
		if t, err := ts.Token(); err == nil && t.AccessToken != tok.AccessToken {
			if err := s.saveToken(t); err != nil {
				slog.Warn("save refreshed calendar token", "err", err)
			}
		}
	}()
	opts := []option.ClientOption{option.WithHTTPClient(oauth2.NewClient(ctx, ts))}
	if s.o.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(s.o.Endpoint))
	}
	svc, err := calendar.NewService(ctx, opts...)
	if err != nil {
		return ch, fmt.Errorf("calendar client: %w", err)
	}
	calID, err := s.ensureCalendar(ctx, svc)
	if err != nil {
		return ch, fmt.Errorf("ensure the calendar: %w", err)
	}
	if err := s.pull(ctx, svc, calID, &ch); err != nil {
		return ch, fmt.Errorf("pull events: %w", err)
	}
	if err := s.push(ctx, svc, calID); err != nil {
		return ch, fmt.Errorf("push events: %w", err)
	}
	if err := s.busy(ctx, svc); err != nil {
		return ch, fmt.Errorf("read busy times: %w", err)
	}
	return ch, nil
}

// ensureCalendar returns the Gwen calendar's id, creating the calendar when
// none is stored or the stored one is gone.
func (s *Service) ensureCalendar(ctx context.Context, svc *calendar.Service) (string, error) {
	sqlDB := s.o.DB.SQL()
	id, err := store.GetLocal(ctx, sqlDB, store.KeyGcalCalendarID)
	switch {
	case err == nil:
		_, err := svc.Calendars.Get(id).Context(ctx).Do()
		if err == nil {
			return id, nil
		}
		if !isStatus(err, http.StatusNotFound, http.StatusGone) {
			return "", err
		}
		slog.Info("the Gwen calendar is gone; creating it again", "calendar_id", id)
	case !errors.Is(err, store.ErrNotFound):
		return "", err
	}
	cal, err := svc.Calendars.Insert(&calendar.Calendar{Summary: s.o.Config().Calendar.Name, TimeZone: s.o.Loc.String()}).
		Context(ctx).Do()
	if err != nil {
		return "", err
	}
	now := s.o.Clock.Now()
	if err := store.SetLocal(ctx, sqlDB, store.KeyGcalCalendarID, cal.Id, now); err != nil {
		return "", err
	}
	if err := s.o.Repo.ForgetCalendar(ctx); err != nil {
		return "", err
	}
	if err := store.DeleteLocal(ctx, sqlDB, store.KeyGcalSyncToken); err != nil {
		return "", err
	}
	slog.Info("created the Gwen calendar", "calendar_id", cal.Id)
	return cal.Id, nil
}

// pull applies changed events to their plan items.
func (s *Service) pull(ctx context.Context, svc *calendar.Service, calID string, ch *Changes) error {
	sqlDB := s.o.DB.SQL()
	links, err := s.o.Repo.Links(ctx)
	if err != nil {
		return err
	}
	linked := map[string]bool{}
	for _, l := range links {
		linked[l.PlanItemID] = true
	}
	token, err := store.GetLocal(ctx, sqlDB, store.KeyGcalSyncToken)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	page := ""
	for {
		call := svc.Events.List(calID).ShowDeleted(true).Context(ctx)
		if token != "" {
			call = call.SyncToken(token)
		}
		if page != "" {
			call = call.PageToken(page)
		}
		res, err := call.Do()
		if isStatus(err, http.StatusGone) && token != "" {
			slog.Info("calendar sync token expired; listing every event")
			token, page = "", ""
			if err := store.DeleteLocal(ctx, sqlDB, store.KeyGcalSyncToken); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		for _, ev := range res.Items {
			if err := s.apply(ctx, ev, linked, ch); err != nil {
				return err
			}
		}
		if res.NextPageToken != "" {
			page = res.NextPageToken
			continue
		}
		if res.NextSyncToken == "" {
			return nil
		}
		return store.SetLocal(ctx, sqlDB, store.KeyGcalSyncToken, res.NextSyncToken, s.o.Clock.Now())
	}
}

// apply brings one pulled event's changes to its linked plan item.
func (s *Service) apply(ctx context.Context, ev *calendar.Event, linked map[string]bool, ch *Changes) error {
	var id string
	if ev.ExtendedProperties != nil {
		id = ev.ExtendedProperties.Private[idKey]
	}
	if id == "" || !linked[id] {
		return nil
	}
	if ev.Status == "cancelled" {
		it, changed, err := s.o.Repo.SkipItem(ctx, id)
		switch {
		case errors.Is(err, store.ErrNotFound):
		case err != nil:
			return err
		case changed:
			ch.plan(it.Day)
		}
		return s.o.Repo.DeleteLink(ctx, id)
	}
	it, err := s.o.Repo.Item(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil // push removes the link
	}
	if err != nil {
		return err
	}
	start, end, ok := eventTimes(ev)
	if !ok {
		return nil
	}
	if it.StartAt != nil && it.StartAt.Equal(start) && it.StartAt.Add(it.Planned).Equal(end) {
		return nil
	}
	updated, err := time.Parse(time.RFC3339, ev.Updated)
	if err != nil || !updated.After(it.UpdatedAt) {
		return nil // Gwen's version is newer; push overwrites the event
	}
	planned := max(end.Sub(start).Round(time.Minute), time.Minute)
	day := s.o.DayOf(start)
	if _, err := s.o.Repo.MoveItem(ctx, id, start, planned, day); err != nil {
		return err
	}
	ch.plan(it.Day)
	ch.plan(day)
	return nil
}

// eventTimes are a timed event's start and end; all-day events have none.
func eventTimes(ev *calendar.Event) (start, end time.Time, ok bool) {
	if ev.Start == nil || ev.End == nil || ev.Start.DateTime == "" || ev.End.DateTime == "" {
		return start, end, false
	}
	start, err1 := time.Parse(time.RFC3339, ev.Start.DateTime)
	end, err2 := time.Parse(time.RFC3339, ev.End.DateTime)
	return start, end, err1 == nil && err2 == nil && end.After(start)
}

// push makes the calendar hold exactly the desired events.
func (s *Service) push(ctx context.Context, svc *calendar.Service, calID string) error {
	today := civil.MustParse(s.o.DayOf(s.o.Clock.Now()))
	desired, err := s.o.Repo.Desired(ctx, today.String(), today.AddDays(Window-1).String())
	if err != nil {
		return err
	}
	links, err := s.o.Repo.Links(ctx)
	if err != nil {
		return err
	}
	stale := map[string]store.GcalLink{}
	for _, l := range links {
		stale[l.PlanItemID] = l
	}
	for _, e := range desired {
		ev := s.eventFor(e)
		l, linked := stale[e.Item.ID]
		delete(stale, e.Item.ID)
		var got *calendar.Event
		switch {
		case !linked:
			got, err = svc.Events.Insert(calID, ev).Context(ctx).Do()
		case e.Item.UpdatedAt.After(l.SyncedAt):
			got, err = svc.Events.Patch(calID, l.EventID, ev).Context(ctx).Do()
			if isStatus(err, http.StatusNotFound, http.StatusGone) {
				got, err = svc.Events.Insert(calID, ev).Context(ctx).Do()
			}
		default:
			continue
		}
		if err != nil {
			return err
		}
		link := store.GcalLink{PlanItemID: e.Item.ID, EventID: got.Id, ETag: got.Etag, SyncedAt: s.o.DB.Now()}
		if err := s.o.Repo.PutLink(ctx, link); err != nil {
			return err
		}
	}
	for _, l := range stale {
		err := svc.Events.Delete(calID, l.EventID).Context(ctx).Do()
		if err != nil && !isStatus(err, http.StatusNotFound, http.StatusGone) {
			return err
		}
		if err := s.o.Repo.DeleteLink(ctx, l.PlanItemID); err != nil {
			return err
		}
	}
	return nil
}

// eventFor is the event a plan item is shown as.
func (s *Service) eventFor(e store.PlanEntry) *calendar.Event {
	summary := e.Task.Title
	if e.Item.Status == model.PlanDone {
		summary = "✓ " + summary
	}
	start := e.Item.StartAt.In(s.o.Loc)
	return &calendar.Event{
		Summary:            summary,
		Description:        "Planned by Gwen",
		Start:              &calendar.EventDateTime{DateTime: start.Format(time.RFC3339), TimeZone: s.o.Loc.String()},
		End:                &calendar.EventDateTime{DateTime: start.Add(e.Item.Planned).Format(time.RFC3339), TimeZone: s.o.Loc.String()},
		ExtendedProperties: &calendar.EventExtendedProperties{Private: map[string]string{idKey: e.Item.ID}},
		Transparency:       "opaque",
		Reminders:          &calendar.EventReminders{UseDefault: false, ForceSendFields: []string{"UseDefault"}},
	}
}

// busy replaces the cached busy times for today and the next 13 days.
func (s *Service) busy(ctx context.Context, svc *calendar.Service) error {
	today := civil.MustParse(s.o.DayOf(s.o.Clock.Now()))
	from, to := today.Midnight(s.o.Loc), today.AddDays(Window).Midnight(s.o.Loc)
	var rows []store.BusyInterval
	if ids := s.o.Config().Calendar.BusyCalendars; len(ids) > 0 {
		req := &calendar.FreeBusyRequest{TimeMin: from.Format(time.RFC3339), TimeMax: to.Format(time.RFC3339)}
		for _, id := range ids {
			req.Items = append(req.Items, &calendar.FreeBusyRequestItem{Id: id})
		}
		res, err := svc.Freebusy.Query(req).Context(ctx).Do()
		if err != nil {
			return err
		}
		for _, id := range ids {
			cal, ok := res.Calendars[id]
			if !ok {
				continue
			}
			if len(cal.Errors) > 0 {
				slog.Warn("busy times unavailable for a calendar", "calendar", id, "reason", cal.Errors[0].Reason)
				continue
			}
			for _, b := range cal.Busy {
				start, err1 := time.Parse(time.RFC3339, b.Start)
				end, err2 := time.Parse(time.RFC3339, b.End)
				if err1 == nil && err2 == nil {
					rows = append(rows, store.BusyInterval{CalendarID: id, Start: start, End: end})
				}
			}
		}
	}
	return s.o.Repo.ReplaceBusy(ctx, from, to, rows)
}
