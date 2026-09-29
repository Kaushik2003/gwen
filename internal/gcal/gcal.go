// Package gcal syncs the plan with a dedicated Google Calendar and reads busy
// times from the user's other calendars, as specified in
// docs/07-integrations.md#google-calendar.
package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Scopes are the only access Gwen asks for.
var Scopes = []string{
	"https://www.googleapis.com/auth/calendar.app.created",
	"https://www.googleapis.com/auth/calendar.freebusy",
}

// Sync timing (docs/07-integrations.md#sync-cycle).
const (
	Interval   = 10 * time.Minute
	Debounce   = 30 * time.Second
	MinBackoff = time.Minute
	MaxBackoff = time.Hour
	// Window is how many days from today the calendar mirrors.
	Window = 14
)

// ErrUnavailable is matched by the error of every call made while the
// calendar is not configured; its message names the setup step.
var ErrUnavailable = errors.New("calendar unavailable")

type unavailable struct{ msg string }

func (e *unavailable) Error() string        { return e.msg }
func (e *unavailable) Is(target error) bool { return target == ErrUnavailable }

// Status is what GET /v1/calendar/status reports.
type Status struct {
	Enabled    bool
	Connected  bool
	CalendarID string
	LastSyncAt *time.Time
	LastError  string
}

// Changes is what a sync touched, for events.
type Changes struct {
	PlanDays []string // days whose plan items a pulled event changed
}

// Options configure a Service.
type Options struct {
	DB      *store.DB
	Repo    store.CalendarRepo
	Clock   clock.Clock
	CredDir string
	Loc     *time.Location
	// Config is the configuration in effect; it is read at every use.
	Config func() config.Config
	// DayOf is the day an instant belongs to.
	DayOf func(time.Time) string
	// OnAttempt runs after every sync or authorization attempt.
	OnAttempt func(Changes)
	// Endpoint overrides the Calendar API base URL, for tests.
	Endpoint string
	// HTTPClient is the base client for Google's endpoints, for tests.
	HTTPClient *http.Client
}

// Service owns the calendar sync. Its methods are safe for concurrent use;
// syncs never overlap.
type Service struct {
	o      Options
	syncMu sync.Mutex // one sync at a time

	mu       sync.Mutex
	lastSync *time.Time
	lastErr  string
	backoff  time.Duration // 0 after a success
	kick     chan struct{}
	authStop context.CancelFunc
}

// New returns a Service; call Run to sync in the background.
func New(o Options) *Service {
	if o.OnAttempt == nil {
		o.OnAttempt = func(Changes) {}
	}
	return &Service{o: o, kick: make(chan struct{}, 1)}
}

func (s *Service) ctx(ctx context.Context) context.Context {
	if s.o.HTTPClient != nil {
		return context.WithValue(ctx, oauth2.HTTPClient, s.o.HTTPClient)
	}
	return ctx
}

// oauthConfig reads the OAuth client; it is an unavailable error while
// calendar.enabled is false or the client file is missing.
func (s *Service) oauthConfig() (*oauth2.Config, error) {
	if !s.o.Config().Calendar.Enabled {
		return nil, &unavailable{"the calendar is turned off; run gwen setup calendar"}
	}
	b, err := config.ReadCredential(s.o.CredDir, config.CredGoogleClient)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &unavailable{"no Google OAuth client; run gwen setup calendar --client-file PATH"}
	}
	if err != nil {
		return nil, err
	}
	cfg, err := google.ConfigFromJSON(b, Scopes...)
	if err != nil {
		return nil, &unavailable{fmt.Sprintf("the Google OAuth client file is not a Desktop app client (%v); "+
			"run gwen setup calendar --client-file PATH again", err)}
	}
	return cfg, nil
}

// token reads the stored token; it is an unavailable error until connected.
func (s *Service) token() (*oauth2.Token, error) {
	b, err := config.ReadCredential(s.o.CredDir, config.CredGoogleToken)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &unavailable{"Google Calendar is not connected; run gwen cal connect"}
	}
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, fmt.Errorf("read %s: %w", config.CredGoogleToken, err)
	}
	return &tok, nil
}

func (s *Service) saveToken(tok *oauth2.Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return config.WriteCredential(s.o.CredDir, config.CredGoogleToken, b)
}

// Available is nil when the calendar is enabled and the OAuth client exists.
func (s *Service) Available() error {
	_, err := s.oauthConfig()
	return err
}

// Status reports the calendar's state.
func (s *Service) Status(ctx context.Context) Status {
	st := Status{Enabled: s.o.Config().Calendar.Enabled}
	if _, err := s.token(); err == nil {
		st.Connected = true
	}
	if id, err := store.GetLocal(ctx, s.o.DB.SQL(), store.KeyGcalCalendarID); err == nil {
		st.CalendarID = id
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st.LastSyncAt, st.LastError = s.lastSync, s.lastErr
	return st
}

// PlanChanged schedules a sync Debounce after the latest plan change.
func (s *Service) PlanChanged() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// wait is how long Run waits before the next scheduled sync.
func (s *Service) wait() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backoff > 0 {
		return s.backoff
	}
	return Interval
}

// record keeps the outcome of a sync and steps the backoff.
func (s *Service) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		now := s.o.Clock.Now()
		s.lastSync, s.lastErr, s.backoff = &now, "", 0
		return
	}
	s.lastErr = err.Error()
	s.backoff = min(max(2*s.backoff, MinBackoff), MaxBackoff)
}

// Run syncs every Interval, Debounce after PlanChanged, and after a failure
// with exponential backoff, until ctx ends. It syncs only while available
// and connected.
func (s *Service) Run(ctx context.Context) {
	t := s.o.Clock.NewTimer(Debounce) // a first sync shortly after start
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
			t.Stop()
			t.Reset(Debounce)
			continue
		case <-t.C():
		}
		if s.Available() == nil {
			if _, err := s.token(); err == nil {
				s.Sync(ctx)
			}
		}
		t.Reset(s.wait())
	}
}

// Sync runs one sync cycle and returns the resulting status. A failure is
// recorded in the status rather than returned; only an unavailable
// calendar is an error.
func (s *Service) Sync(ctx context.Context) (Status, error) {
	if err := s.Available(); err != nil {
		return Status{}, err
	}
	if _, err := s.token(); err != nil {
		return Status{}, err
	}
	s.syncMu.Lock()
	ch, err := s.cycle(ctx)
	s.syncMu.Unlock()
	s.record(err)
	if err != nil {
		slog.Warn("calendar sync failed", "err", err)
	} else {
		slog.Info("calendar synced", "plan_days", len(ch.PlanDays))
	}
	s.o.OnAttempt(ch)
	return s.Status(ctx), nil
}
