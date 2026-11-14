// Package subject parses and prints subject refs: the one way reminders,
// links, routines, and runs point at the thing they belong to (plan.md 3.3).
//
//	task:<uuid>
//	event:<calendar_id>/<event_id>
//	series:<calendar_id>/<recurring_event_id>
//	commitment:<uuid>@<YYYY-MM-DD>
//	goal:<uuid>
//
// Lookups try the instance first, then its parent: an event instance falls
// back to its series, a commitment day to the commitment. A task occurrence
// falls back to its template, which needs the database, so the store does
// that step (Parent returns no parent for a task).
package subject

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kzark/gwen/internal/planner/civil"
)

// Kinds of subject.
const (
	Task       = "task"
	Event      = "event"
	Series     = "series"
	Commitment = "commitment"
	Goal       = "goal"
)

// MaxLen is the longest ref accepted.
const MaxLen = 400

// Ref is a parsed subject ref. ID is the uuid for a task, goal, or
// commitment, the event or recurring event id for an event or a series.
// Calendar is set for events and series, Day for a commitment day ("" for
// the commitment itself).
type Ref struct {
	kind     string
	Calendar string
	ID       string
	Day      string
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// instanceSuffix is the start time Google adds to a recurring event's id to
// name one instance: abc_20261010T080000Z, or abc_20261010 for a day event.
var instanceSuffix = regexp.MustCompile(`_[0-9]{8}(T[0-9]{6}Z?)?$`)

// Parse reads a ref. It fails on an unknown kind, a malformed id, or a ref
// longer than MaxLen.
func Parse(s string) (Ref, error) {
	if s == "" || len(s) > MaxLen {
		return Ref{}, fmt.Errorf("a subject is 1 to %d characters", MaxLen)
	}
	kind, rest, ok := strings.Cut(s, ":")
	if !ok || rest == "" {
		return Ref{}, fmt.Errorf("%q is not a subject ref such as task:<id>", s)
	}
	switch kind {
	case Task, Goal:
		if !uuidPattern.MatchString(rest) {
			return Ref{}, fmt.Errorf("%q: a %s is named by its id", s, kind)
		}
		return Ref{kind: kind, ID: rest}, nil
	case Event, Series:
		cal, id, ok := strings.Cut(rest, "/")
		if !ok || cal == "" || id == "" || strings.ContainsAny(id, "/ ") {
			return Ref{}, fmt.Errorf("%q: an %s is <calendar>/<event id>", s, kind)
		}
		return Ref{kind: kind, Calendar: cal, ID: id}, nil
	case Commitment:
		id, day, _ := strings.Cut(rest, "@")
		if !uuidPattern.MatchString(id) {
			return Ref{}, fmt.Errorf("%q: a commitment is named by its id", s)
		}
		if day != "" {
			if _, err := civil.Parse(day); err != nil {
				return Ref{}, fmt.Errorf("%q: the day is YYYY-MM-DD", s)
			}
		}
		return Ref{kind: kind, ID: id, Day: day}, nil
	}
	return Ref{}, fmt.Errorf("%q: %q is not a kind of subject", s, kind)
}

// Of builds a ref of a kind and id: a task, goal, or commitment.
func Of(kind, id string) Ref { return Ref{kind: kind, ID: id} }

// Kind is the ref's kind: Task, Event, Series, Commitment, or Goal.
func (r Ref) Kind() string { return r.kind }

// IsZero reports whether r is no ref at all.
func (r Ref) IsZero() bool { return r.kind == "" }

// String prints the ref as Parse reads it.
func (r Ref) String() string {
	switch r.kind {
	case Event, Series:
		return r.kind + ":" + r.Calendar + "/" + r.ID
	case Commitment:
		if r.Day != "" {
			return r.kind + ":" + r.ID + "@" + r.Day
		}
	case "":
		return ""
	}
	return r.kind + ":" + r.ID
}

// Parent is the ref a lookup falls back to, and whether there is one: an
// event instance's series, a commitment day's commitment.
func (r Ref) Parent() (Ref, bool) {
	switch r.kind {
	case Event:
		if loc := instanceSuffix.FindStringIndex(r.ID); loc != nil && loc[0] > 0 {
			return Ref{kind: Series, Calendar: r.Calendar, ID: r.ID[:loc[0]]}, true
		}
	case Commitment:
		if r.Day != "" {
			return Ref{kind: Commitment, ID: r.ID}, true
		}
	}
	return Ref{}, false
}
