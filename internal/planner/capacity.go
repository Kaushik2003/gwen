package planner

import (
	"slices"
	"time"

	"github.com/kzark/gwen/internal/planner/civil"
)

// Interval is the half-open span [Start, End).
type Interval struct {
	Start, End time.Time
}

func (i Interval) empty() bool { return !i.End.After(i.Start) }

// ProjectWork is time worked on one project; ProjectID is nil for unassigned.
type ProjectWork struct {
	ProjectID *string
	Worked    time.Duration
}

// CapacityInput is everything Capacity needs for one day.
type CapacityInput struct {
	Day   civil.Day
	Loc   *time.Location
	Today bool      // Day is the day Now belongs to
	Now   time.Time // used only when Today
	// DayStart and DayEnd bound the window, in minutes after local midnight.
	DayStart, DayEnd int
	Buffer           time.Duration
	// Target is the open work day's target when Today and a work day exists,
	// else tracking.daily_target.
	Target      time.Duration
	Commitments []Commitment // every live commitment; Capacity picks those on Day
	Busy        []Interval   // calendar busy time (v3)
	// WorkedToday is today's work by project; ignored unless Today.
	WorkedToday []ProjectWork
}

// CapacityResult is the minutes available for planned work and the free
// intervals that planned work may be slotted into.
type CapacityResult struct {
	Minutes    int
	Free       []Interval // ascending, disjoint
	Slot       int        // slot_minutes
	TargetLeft int        // target_left, in minutes
}

// OnDay reports whether commitment c occurs on d: its rule matches and d lies
// within [active_from, active_until].
func OnDay(c Commitment, d civil.Day) bool {
	from := civil.MustParse(c.ActiveFrom)
	if d.Before(from) || (c.ActiveUntil != nil && d.After(civil.MustParse(*c.ActiveUntil))) {
		return false
	}
	r, err := ParseRule(c.RRule)
	return err == nil && r.Matches(from, d)
}

// Capacity computes the minutes available for planned work on in.Day
// (docs/06-planner.md#capacity).
func Capacity(in CapacityInput) CapacityResult {
	free := []Interval{{in.Day.At(in.DayStart, in.Loc), in.Day.At(in.DayEnd, in.Loc)}}
	var floating, counting time.Duration
	countingProjects := map[string]bool{}
	for _, c := range in.Commitments {
		if !OnDay(c, in.Day) {
			continue
		}
		if c.StartMinute == nil {
			floating += c.Duration
		} else {
			start := in.Day.At(*c.StartMinute, in.Loc)
			free = subtract(free, Interval{start, start.Add(c.Duration)})
		}
		if c.CountsTowardTarget {
			counting += c.Duration
			if c.ProjectID != nil {
				countingProjects[*c.ProjectID] = true
			}
		}
	}
	for _, b := range in.Busy {
		free = subtract(free, b)
	}
	target := in.Target - counting
	if in.Today {
		free = subtract(free, Interval{in.Day.At(in.DayStart, in.Loc), ceil5(in.Now)})
		for _, w := range in.WorkedToday {
			if w.ProjectID == nil || !countingProjects[*w.ProjectID] {
				target -= w.Worked
			}
		}
	}
	slot := int((total(free) - floating) / time.Minute)
	targetLeft := int(target / time.Minute)
	return CapacityResult{
		Minutes:    max(0, min(targetLeft, slot)-int(in.Buffer/time.Minute)),
		Free:       free,
		Slot:       slot,
		TargetLeft: targetLeft,
	}
}

// ceil5 rounds t up to the next multiple of 5 minutes.
func ceil5(t time.Time) time.Time {
	r := t.Truncate(5 * time.Minute)
	if r.Before(t) {
		r = r.Add(5 * time.Minute)
	}
	return r
}

func total(is []Interval) time.Duration {
	var d time.Duration
	for _, i := range is {
		d += i.End.Sub(i.Start)
	}
	return d
}

// subtract removes cut from the ascending, disjoint intervals is.
func subtract(is []Interval, cut Interval) []Interval {
	if cut.empty() {
		return is
	}
	out := make([]Interval, 0, len(is)+1)
	for _, i := range is {
		if !cut.Start.Before(i.End) || !i.Start.Before(cut.End) {
			out = append(out, i)
			continue
		}
		if left := (Interval{i.Start, cut.Start}); !left.empty() {
			out = append(out, left)
		}
		if right := (Interval{cut.End, i.End}); !right.empty() {
			out = append(out, right)
		}
	}
	return slices.Clip(out)
}

// earliestFit is the earliest start at which d fits wholly inside one of the
// free intervals, and false when it fits nowhere.
func earliestFit(free []Interval, d time.Duration) (time.Time, bool) {
	for _, i := range free {
		if i.End.Sub(i.Start) >= d {
			return i.Start, true
		}
	}
	return time.Time{}, false
}
