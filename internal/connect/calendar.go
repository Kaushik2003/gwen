package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/planner/civil"
)

// MaxCalendarDays is the longest span one calendar.read covers.
const MaxCalendarDays = 14

// Busy is one event in the calendar cache.
type Busy struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// CalendarCache reads the events the calendar sync last fetched, and says
// whether the calendar is connected.
type CalendarCache interface {
	Busy(ctx context.Context, from, to time.Time) ([]Busy, error)
	Status(ctx context.Context) Status
}

// Calendar is the calendar connector: calendar.read over the sync's cache,
// so reading never calls Google.
func Calendar(cache CalendarCache, loc *time.Location, today func() string) Connector {
	return Simple{IDValue: "calendar", NameValue: "Google Calendar", StatusFunc: cache.Status, Caps: []Capability{{
		Name: "calendar.read", Title: "Read your calendar", Risk: Read,
		Input: `{"from": "YYYY-MM-DD", "to": "YYYY-MM-DD"} (both optional: today; at most 14 days)`,
		Check: Checked(func(in *calendarSpan) error {
			if in.From == "" {
				in.From = today()
			}
			if in.To == "" {
				in.To = in.From
			}
			from, err := civil.Parse(in.From)
			if err != nil {
				return errors.New("from is YYYY-MM-DD")
			}
			to, err := civil.Parse(in.To)
			if err != nil {
				return errors.New("to is YYYY-MM-DD")
			}
			if n := from.DaysUntil(to); n < 0 || n >= MaxCalendarDays {
				return fmt.Errorf("read 1 to %d days at a time", MaxCalendarDays)
			}
			return nil
		}),
		Summary: func(input json.RawMessage) string {
			var in calendarSpan
			_ = json.Unmarshal(input, &in)
			if in.From == in.To {
				return "Read your calendar for " + in.From
			}
			return "Read your calendar from " + in.From + " to " + in.To
		},
		Invoke: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			var in calendarSpan
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, err
			}
			if st := cache.Status(ctx); st.State != Connected {
				return nil, errors.New("the calendar isn't connected")
			}
			from, to := civil.MustParse(in.From).Midnight(loc), civil.MustParse(in.To).AddDays(1).Midnight(loc)
			busy, err := cache.Busy(ctx, from, to)
			if err != nil {
				return nil, err
			}
			type event struct {
				Title string `json:"title"`
				Day   string `json:"day"`
				Start string `json:"start"`
				End   string `json:"end"`
			}
			out := struct {
				Events []event `json:"events"`
			}{Events: []event{}}
			for _, b := range busy {
				s, e := b.Start.In(loc), b.End.In(loc)
				out.Events = append(out.Events, event{Title: b.Title, Day: s.Format(time.DateOnly), Start: s.Format("15:04"), End: e.Format("15:04")})
			}
			return json.Marshal(out)
		},
	}}}
}

type calendarSpan struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}
