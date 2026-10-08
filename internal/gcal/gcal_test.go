package gcal

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/calendar/v3"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// fakeCalendar is the part of the Calendar API a sync uses.
type fakeCalendar struct {
	mu        sync.Mutex
	calendars map[string]*calendar.Calendar
	events    map[string]*calendar.Event // by id
	changed   map[string]int             // event id → seq of its last change
	seq       int
	created   int  // calendars ever created
	expire    bool // every sync token is 410 Gone
	fail      bool // every call is a 500
	others    map[string][]*calendar.Event // the busy calendars' events, by calendar id
	list      []*calendar.CalendarListEntry
	scopeless bool // reading other calendars is refused for want of a scope
	lists     []string // the sync token of each list call
	inserts   int
	patches   int
	deletes   int
}

func newFakeCalendar() *fakeCalendar {
	return &fakeCalendar{calendars: map[string]*calendar.Calendar{}, events: map[string]*calendar.Event{},
		changed: map[string]int{}, others: map[string][]*calendar.Event{}}
}

func (f *fakeCalendar) touch(ev *calendar.Event) {
	f.seq++
	f.changed[ev.Id] = f.seq
	ev.Etag = fmt.Sprintf(`"%d"`, f.seq)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func scopeError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprint(w, `{"error": {"code": 403, "message": "Request had insufficient authentication scopes.",
		"errors": [{"message": "Insufficient Permission", "domain": "global", "reason": "insufficientPermissions"}]}}`)
}

func apiError(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error": {"code": %d, "message": "fake"}}`, code)
}

func (f *fakeCalendar) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /calendars/{cal}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := f.calendars[r.PathValue("cal")]
		if !ok {
			apiError(w, http.StatusNotFound)
			return
		}
		writeJSON(w, c)
	})
	mux.HandleFunc("POST /calendars", func(w http.ResponseWriter, r *http.Request) {
		var c calendar.Calendar
		json.NewDecoder(r.Body).Decode(&c)
		f.created++
		c.Id = fmt.Sprintf("cal-%d", f.created)
		f.calendars[c.Id] = &c
		writeJSON(w, c)
	})
	mux.HandleFunc("GET /calendars/{cal}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, ours := f.calendars[r.PathValue("cal")]; !ours {
			if f.scopeless {
				scopeError(w)
				return
			}
			evs, ok := f.others[r.PathValue("cal")]
			if !ok {
				apiError(w, http.StatusNotFound)
				return
			}
			writeJSON(w, calendar.Events{Items: evs})
			return
		}
		token := r.URL.Query().Get("syncToken")
		f.lists = append(f.lists, token)
		if token != "" && f.expire {
			f.expire = false
			apiError(w, http.StatusGone)
			return
		}
		since := 0
		if token != "" {
			since, _ = strconv.Atoi(strings.TrimPrefix(token, "tok-"))
		}
		res := calendar.Events{Items: []*calendar.Event{}, NextSyncToken: fmt.Sprintf("tok-%d", f.seq)}
		for id, ev := range f.events {
			if f.changed[id] > since && (token != "" || ev.Status != "cancelled") {
				res.Items = append(res.Items, ev)
			}
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("POST /calendars/{cal}/events", func(w http.ResponseWriter, r *http.Request) {
		var ev calendar.Event
		json.NewDecoder(r.Body).Decode(&ev)
		ev.Id, ev.Status = fmt.Sprintf("ev-%d", f.seq+1), "confirmed"
		f.events[ev.Id] = &ev
		f.touch(&ev)
		f.inserts++
		writeJSON(w, ev)
	})
	mux.HandleFunc("PATCH /calendars/{cal}/events/{ev}", func(w http.ResponseWriter, r *http.Request) {
		ev, ok := f.events[r.PathValue("ev")]
		if !ok {
			apiError(w, http.StatusNotFound)
			return
		}
		var patch calendar.Event
		json.NewDecoder(r.Body).Decode(&patch)
		ev.Summary, ev.Start, ev.End = patch.Summary, patch.Start, patch.End
		f.touch(ev)
		f.patches++
		writeJSON(w, ev)
	})
	mux.HandleFunc("DELETE /calendars/{cal}/events/{ev}", func(w http.ResponseWriter, r *http.Request) {
		ev, ok := f.events[r.PathValue("ev")]
		if !ok {
			apiError(w, http.StatusGone)
			return
		}
		ev.Status = "cancelled"
		f.touch(ev)
		f.deletes++
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /users/me/calendarList", func(w http.ResponseWriter, r *http.Request) {
		if f.scopeless {
			scopeError(w)
			return
		}
		items := slices.Clone(f.list)
		for id, c := range f.calendars {
			items = append(items, &calendar.CalendarListEntry{Id: id, Summary: c.Summary})
		}
		writeJSON(w, calendar.CalendarList{Items: items})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			apiError(w, http.StatusInternalServerError)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// move changes an event as a person would in Google Calendar.
func (f *fakeCalendar) move(eventID string, start, end, updated time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev := f.events[eventID]
	ev.Start = &calendar.EventDateTime{DateTime: start.Format(time.RFC3339)}
	ev.End = &calendar.EventDateTime{DateTime: end.Format(time.RFC3339)}
	ev.Updated = updated.Format(time.RFC3339)
	f.touch(ev)
}

func (f *fakeCalendar) cancel(eventID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events[eventID].Status = "cancelled"
	f.touch(f.events[eventID])
}

func (f *fakeCalendar) event(id string) calendar.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.events[id]
}

func (f *fakeCalendar) counts() (inserts, patches, deletes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inserts, f.patches, f.deletes
}

// fx is a Service against a fake Calendar API, on a store whose clock reads
// 2026-09-15 08:00 UTC.
type fx struct {
	t       *testing.T
	ctx     context.Context
	clk     *clock.Fake
	db      *store.DB
	repos   store.Repos
	cal     *fakeCalendar
	s       *Service
	credDir string
	cfg     config.Config
	changes []Changes
}

func newFx(t *testing.T) *fx {
	t.Helper()
	f := &fx{t: t, ctx: context.Background(), clk: clock.NewFake(testutil.At("08:00")), cal: newFakeCalendar(),
		credDir: t.TempDir(), cfg: config.Defaults()}
	f.cfg.Calendar.Enabled = true
	f.db = testutil.NewDBWithClock(t, f.clk)
	f.repos = store.NewRepos(f.db)
	srv := httptest.NewServer(f.cal.handler())
	t.Cleanup(srv.Close)
	writeClient(t, f.credDir, "http://127.0.0.1:1/token")
	tok, _ := json.Marshal(map[string]any{"access_token": "at", "token_type": "Bearer", "refresh_token": "rt",
		"expiry": time.Now().Add(24 * time.Hour)})
	require.NoError(t, config.WriteCredential(f.credDir, config.CredGoogleToken, tok))
	var mu sync.Mutex
	f.s = New(Options{
		DB: f.db, Repo: f.repos.Calendar, Clock: f.clk, CredDir: f.credDir, Loc: time.UTC,
		Config: func() config.Config { return f.cfg },
		DayOf:  func(t time.Time) string { return timeengine.ConfigFrom(f.cfg, time.UTC).DayOf(t) },
		OnAttempt: func(c Changes) {
			mu.Lock()
			defer mu.Unlock()
			f.changes = append(f.changes, c)
		},
		Endpoint: srv.URL + "/",
	})
	return f
}

func writeClient(t *testing.T, dir, tokenURL string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"installed": map[string]any{
		"client_id": "client", "client_secret": "secret", "auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": tokenURL, "redirect_uris": []string{"http://localhost"},
	}})
	require.NoError(t, config.WriteCredential(dir, config.CredGoogleClient, b))
}

func (f *fx) env() store.PlanEnv {
	return store.PlanEnv{Now: f.clk.Now(), Loc: time.UTC, DayOf: timeengine.ConfigFrom(f.cfg, time.UTC).DayOf,
		DayStart: 9 * 60, DayEnd: 23 * 60, Buffer: 30 * time.Minute, DailyTarget: 8 * time.Hour, Busy: true}
}

func (f *fx) sync() Status {
	f.t.Helper()
	st, err := f.s.Sync(f.ctx)
	require.NoError(f.t, err)
	return st
}

func (f *fx) plan() store.Plan {
	f.t.Helper()
	p, _, err := f.repos.Plans.Get(f.ctx, "", f.env())
	require.NoError(f.t, err)
	return p
}

func (f *fx) link(itemID string) (store.GcalLink, bool) {
	f.t.Helper()
	links, err := f.repos.Calendar.Links(f.ctx)
	require.NoError(f.t, err)
	for _, l := range links {
		if l.PlanItemID == itemID {
			return l, true
		}
	}
	return store.GcalLink{}, false
}

func at(hm string) time.Time { return testutil.At(hm) }

func TestFirstSyncCreatesTheCalendarAndPushes(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	tk, _, err := f.repos.Tasks.Create(f.ctx, store.NewTask{Title: "Report", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	p := f.plan()
	require.Len(t, p.Items, 1)
	item := p.Items[0].Item

	st := f.sync()
	require.Empty(t, st.LastError)
	require.Equal(t, "cal-1", st.CalendarID)
	require.True(t, st.Connected)
	require.NotNil(t, st.LastSyncAt)
	require.Equal(t, "Gwen", f.cal.calendars["cal-1"].Summary)
	require.Equal(t, "UTC", f.cal.calendars["cal-1"].TimeZone)

	l, ok := f.link(item.ID)
	require.True(t, ok)
	ev := f.cal.event(l.EventID)
	require.Equal(t, "Report", ev.Summary)
	require.Equal(t, "Planned by Gwen", ev.Description)
	require.Equal(t, item.ID, ev.ExtendedProperties.Private["gwen_id"])
	require.Equal(t, "opaque", ev.Transparency)
	require.NotNil(t, ev.Reminders)
	require.False(t, ev.Reminders.UseDefault)
	require.Equal(t, at("09:00").Format(time.RFC3339), ev.Start.DateTime)
	require.Equal(t, at("10:00").Format(time.RFC3339), ev.End.DateTime)
	require.Equal(t, l.ETag, ev.Etag)

	// Nothing changed: nothing is written.
	f.clk.Advance(time.Minute)
	f.sync()
	ins, pat, del := f.cal.counts()
	require.Equal(t, [3]int{1, 0, 0}, [3]int{ins, pat, del})

	// An edited item is patched.
	f.clk.Advance(time.Minute)
	_, _, err = f.repos.Plans.UpdateItem(f.ctx, item.ID, store.PlanItemPatch{
		StartAt: store.Nullable[time.Time]{Set: true, Value: testutil.Ptr(at("11:00"))}}, f.env())
	require.NoError(t, err)
	f.clk.Advance(time.Minute)
	f.sync()
	require.Equal(t, at("11:00").Format(time.RFC3339), f.cal.event(l.EventID).Start.DateTime)
	_, pat, _ = f.cal.counts()
	require.Equal(t, 1, pat)

	// A done item is marked with a tick.
	f.clk.Advance(time.Minute)
	_, err = f.repos.Tasks.Complete(f.ctx, tk.ID, nil)
	require.NoError(t, err)
	f.clk.Advance(time.Minute)
	f.sync()
	require.Equal(t, "✓ Report", f.cal.event(l.EventID).Summary)

	// An item that leaves the desired set loses its event.
	f.clk.Advance(time.Minute)
	_, err = f.repos.Tasks.Reopen(f.ctx, tk.ID, testutil.Day0)
	require.NoError(t, err)
	_, _, err = f.repos.Plans.UpdateItem(f.ctx, item.ID, store.PlanItemPatch{Status: testutil.Ptr(model.PlanSkipped)}, f.env())
	require.NoError(t, err)
	f.clk.Advance(time.Minute)
	f.sync()
	_, _, del = f.cal.counts()
	require.Equal(t, 1, del)
	_, ok = f.link(item.ID)
	require.False(t, ok)
	require.Equal(t, "cancelled", f.cal.event(l.EventID).Status)
}

func TestPullAppliesMovesAndCancellations(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	for _, title := range []string{"Moved", "Cancelled"} {
		_, _, err := f.repos.Tasks.Create(f.ctx, store.NewTask{Title: title, EstimateMinutes: testutil.Ptr(60)})
		require.NoError(t, err)
		f.clk.Advance(time.Second)
	}
	p := f.plan()
	require.Len(t, p.Items, 2)
	moved, cancelled := p.Items[0].Item, p.Items[1].Item
	f.sync()
	lm, _ := f.link(moved.ID)
	lc, _ := f.link(cancelled.ID)

	f.clk.Advance(time.Minute)
	f.cal.move(lm.EventID, at("13:00"), at("13:45"), f.clk.Now())
	f.cal.cancel(lc.EventID)
	f.clk.Advance(time.Minute)
	f.changes = nil
	f.sync()

	got, err := f.repos.Calendar.Item(f.ctx, moved.ID)
	require.NoError(t, err)
	require.Equal(t, at("13:00"), *got.StartAt)
	require.Equal(t, 45*time.Minute, got.Planned)
	require.True(t, got.Pinned)
	require.Equal(t, testutil.Day0, got.Day)
	require.Equal(t, []string{testutil.Day0}, f.changes[0].PlanDays)

	got, err = f.repos.Calendar.Item(f.ctx, cancelled.ID)
	require.NoError(t, err)
	require.Equal(t, model.PlanSkipped, got.Status)
	_, ok := f.link(cancelled.ID)
	require.False(t, ok)

	// A move older than Gwen's own edit loses: push overwrites the event.
	f.clk.Advance(time.Minute)
	stale := f.clk.Now()
	f.clk.Advance(time.Minute)
	_, _, err = f.repos.Plans.UpdateItem(f.ctx, moved.ID, store.PlanItemPatch{
		StartAt: store.Nullable[time.Time]{Set: true, Value: testutil.Ptr(at("15:00"))}}, f.env())
	require.NoError(t, err)
	f.cal.move(lm.EventID, at("16:00"), at("16:30"), stale)
	f.clk.Advance(time.Minute)
	f.sync()
	got, err = f.repos.Calendar.Item(f.ctx, moved.ID)
	require.NoError(t, err)
	require.Equal(t, at("15:00"), *got.StartAt)
	require.Equal(t, at("15:00").Format(time.RFC3339), f.cal.event(lm.EventID).Start.DateTime)
}

func TestExpiredSyncTokenListsEverything(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.sync()
	f.sync()
	f.cal.expire = true
	st := f.sync()
	require.Empty(t, st.LastError)
	require.Equal(t, []string{"", "tok-0", "tok-0", ""}, f.cal.lists, "a 410 clears the token and lists in full")
}

func TestAMissingCalendarIsCreatedAgain(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	_, _, err := f.repos.Tasks.Create(f.ctx, store.NewTask{Title: "Report", EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	item := f.plan().Items[0].Item
	f.sync()
	f.cal.mu.Lock()
	delete(f.cal.calendars, "cal-1")
	f.cal.mu.Unlock()
	st := f.sync()
	require.Equal(t, "cal-2", st.CalendarID)
	ins, _, _ := f.cal.counts()
	require.Equal(t, 2, ins, "the event is pushed to the new calendar")
	_, ok := f.link(item.ID)
	require.True(t, ok)
}

// timed is a timed event from hm to hm on Day0.
func timed(id, title, from, to string) *calendar.Event {
	return &calendar.Event{Id: id, Summary: title, Status: "confirmed",
		Start: &calendar.EventDateTime{DateTime: at(from).Format(time.RFC3339)}, End: &calendar.EventDateTime{DateTime: at(to).Format(time.RFC3339)}}
}

func (f *fx) busyRows() []string {
	rows, err := f.db.SQL().Query(`SELECT calendar_id, event_id, title FROM calendar_busy ORDER BY start_at, event_id`)
	require.NoError(f.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c, e, title string
		require.NoError(f.t, rows.Scan(&c, &e, &title))
		out = append(out, c+" "+e+" "+title)
	}
	return out
}

func TestBusyTimesAreReplacedAndReduceCapacity(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	before := f.plan().Capacity
	f.cal.others["primary"] = []*calendar.Event{timed("standup", "Standup", "10:00", "11:00")}
	f.sync()
	require.Equal(t, []string{"primary standup Standup"}, f.busyRows())
	p, _, err := f.repos.Plans.Generate(f.ctx, testutil.Day0, f.env())
	require.NoError(t, err)
	require.Equal(t, before, p.Capacity, "the target, not the window, limits a free day")
	require.Len(t, p.Events, 1)
	require.Equal(t, "Standup", p.Events[0].Title)
	require.True(t, p.Events[0].Start.Equal(at("10:00")))
	env := f.env()
	env.DayEnd = 17 * 60 // 8 h window: now busy time binds
	p, _, err = f.repos.Plans.Generate(f.ctx, testutil.Day0, env)
	require.NoError(t, err)
	require.Equal(t, 7*60-30, p.Capacity)

	f.cal.others["primary"] = []*calendar.Event{timed("review", "Review", "14:00", "14:30")}
	f.sync()
	require.Equal(t, []string{"primary review Review"}, f.busyRows())
}

func TestWhichEventsAreBusy(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.cfg.Calendar.BusyCalendars = []string{"primary", "work@example.com", "gone@example.com"}
	free := timed("free", "Gym", "07:00", "08:00")
	free.Transparency = "transparent"
	declined := timed("declined", "All hands", "12:00", "13:00")
	declined.Attendees = []*calendar.EventAttendee{{Email: "me@example.com", Self: true, ResponseStatus: "declined"}}
	location := timed("location", "Office", "09:00", "17:00")
	location.EventType = "workingLocation"
	cancelled := timed("cancelled", "Lunch", "13:00", "14:00")
	cancelled.Status = "cancelled"
	allDay := func(id string, transparency string) *calendar.Event {
		return &calendar.Event{Id: id, Summary: id, Status: "confirmed", Transparency: transparency,
			Start: &calendar.EventDateTime{Date: testutil.Day0}, End: &calendar.EventDateTime{Date: civil.MustParse(testutil.Day0).AddDays(1).String()}}
	}
	f.cal.others["primary"] = []*calendar.Event{free, declined, location, cancelled, allDay("birthday", "transparent")}
	f.cal.others["work@example.com"] = []*calendar.Event{timed("1on1", "1:1", "15:00", "15:30"), allDay("offsite", "")}
	st := f.sync()
	require.Empty(t, st.LastError, "a busy calendar that is gone is skipped")
	require.Equal(t, []string{"work@example.com offsite offsite", "primary free Gym", "work@example.com 1on1 1:1"}, f.busyRows())
	var offsite store.BusyInterval
	for _, e := range f.plan().Events {
		if e.Title == "offsite" {
			offsite = e
		}
	}
	require.True(t, offsite.Start.Equal(civil.MustParse(testutil.Day0).Midnight(time.UTC)), "an all-day event runs midnight to midnight")
}

func TestACalendarListedTwiceCountsOnce(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.cfg.Calendar.BusyCalendars = []string{"primary", "me@example.com"}
	standup := timed("standup", "Standup", "10:00", "11:00")
	f.cal.others["primary"] = []*calendar.Event{standup}
	f.cal.others["me@example.com"] = []*calendar.Event{standup}
	require.Empty(t, f.sync().LastError)
	require.Equal(t, []string{"primary standup Standup"}, f.busyRows())
}

func TestMissingScopeAsksToReconnect(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.cal.scopeless = true
	st := f.sync()
	require.Contains(t, st.LastError, "connected again")
	_, err := f.s.Calendars(f.ctx)
	require.ErrorIs(t, err, ErrUnavailable)
	require.Contains(t, err.Error(), "connected again")
}

func TestCalendarsListsAllButGwens(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.sync() // creates Gwen's calendar
	f.cal.list = []*calendar.CalendarListEntry{
		{Id: "work@example.com", Summary: "work", SummaryOverride: "Work", BackgroundColor: "#16a765"},
		{Id: "me@example.com", Summary: "me@example.com", Primary: true},
		{Id: "anniv@example.com", Summary: "Birthdays"},
	}
	cals, err := f.s.Calendars(f.ctx)
	require.NoError(t, err)
	require.Equal(t, []Calendar{
		{ID: "primary", Name: "me@example.com", Primary: true},
		{ID: "anniv@example.com", Name: "Birthdays"},
		{ID: "work@example.com", Name: "Work", Color: "#16a765"},
	}, cals)
}

func TestFailuresBackOff(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	require.Equal(t, Interval, f.s.wait())
	f.cal.fail = true
	var waits []time.Duration
	for range 8 {
		st := f.sync()
		require.NotEmpty(t, st.LastError)
		waits = append(waits, f.s.wait())
	}
	require.Equal(t, []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour}, waits)
	f.cal.fail = false
	st := f.sync()
	require.Empty(t, st.LastError)
	require.Equal(t, Interval, f.s.wait(), "a success resets the backoff")
}

func TestUnavailable(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.cfg.Calendar.Enabled = false
	_, err := f.s.Sync(f.ctx)
	require.ErrorIs(t, err, ErrUnavailable)
	f.cfg.Calendar.Enabled = true
	require.NoError(t, os.Remove(filepath.Join(f.credDir, config.CredGoogleToken)))
	_, err = f.s.Sync(f.ctx)
	require.ErrorIs(t, err, ErrUnavailable)
	require.NoError(t, f.s.Available(), "authorization needs only the client")
	require.NoError(t, os.Remove(filepath.Join(f.credDir, config.CredGoogleClient)))
	_, err = f.s.AuthStart(f.ctx)
	require.ErrorIs(t, err, ErrUnavailable)
	require.False(t, f.s.Status(f.ctx).Connected)
	require.NoError(t, config.WriteCredential(f.credDir, config.CredGoogleClient, []byte(`{"web": {}}`)))
	require.ErrorIs(t, f.s.Available(), ErrUnavailable, "a malformed client is a setup problem")
}

func TestAuthorizationUsesPKCEAndChecksState(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	require.NoError(t, os.Remove(filepath.Join(f.credDir, config.CredGoogleToken)))
	var challenge string
	exchanged := make(chan url.Values, 1)
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("code") != "the-code" {
			http.Error(w, `{"error": "invalid_grant"}`, http.StatusBadRequest)
			return
		}
		exchanged <- r.Form
		writeJSON(w, map[string]any{"access_token": "new", "token_type": "Bearer", "refresh_token": "refresh",
			"expires_in": 3600})
	}))
	t.Cleanup(tokenSrv.Close)
	writeClient(t, f.credDir, tokenSrv.URL)

	raw, err := f.s.AuthStart(f.ctx)
	require.NoError(t, err)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, "accounts.google.com", u.Host)
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.Equal(t, "offline", q.Get("access_type"))
	require.Equal(t, "consent", q.Get("prompt"))
	require.Equal(t, strings.Join(Scopes, " "), q.Get("scope"))
	challenge = q.Get("code_challenge")
	redirect := q.Get("redirect_uri")
	require.True(t, strings.HasPrefix(redirect, "http://127.0.0.1:"), redirect)

	resp, err := http.Get(redirect + "?state=forged&code=the-code")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.False(t, f.s.Status(f.ctx).Connected)

	resp, err = http.Get(redirect + "?state=" + url.QueryEscape(q.Get("state")) + "&code=the-code")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Contains(t, string(body), "connected")
	form := <-exchanged
	require.Equal(t, "authorization_code", form.Get("grant_type"))
	require.Equal(t, redirect, form.Get("redirect_uri"))
	require.True(t, f.s.Status(f.ctx).Connected)
	b, err := config.ReadCredential(f.credDir, config.CredGoogleToken)
	require.NoError(t, err)
	require.Contains(t, string(b), `"refresh_token":"refresh"`)
	require.NotEmpty(t, f.changes, "the attempt is reported")
}
