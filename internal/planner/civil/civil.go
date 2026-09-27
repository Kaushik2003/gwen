// Package civil is a local calendar date with no time or zone, for the
// planner's day arithmetic (docs/03-data-model.md#time-representation).
package civil

import (
	"fmt"
	"time"
)

// Day is a calendar date. The zero Day is not a valid date; use Parse or Of.
type Day struct {
	Y int
	M time.Month
	D int
}

// Parse reads a strict "YYYY-MM-DD" that names a real date.
func Parse(s string) (Day, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil || t.Format(time.DateOnly) != s {
		return Day{}, fmt.Errorf("%q is not a date like 2026-09-15", s)
	}
	return Of(t), nil
}

// MustParse is Parse for dates known to be valid, such as those read back
// from a column the schema checks. A malformed date panics.
func MustParse(s string) Day {
	d, err := Parse(s)
	if err != nil {
		panic("civil: " + err.Error())
	}
	return d
}

// Of is the date t's wall clock shows in t's location.
func Of(t time.Time) Day {
	y, m, d := t.Date()
	return Day{y, m, d}
}

// String returns "YYYY-MM-DD".
func (d Day) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Y, d.M, d.D) }

// IsZero reports whether d is the zero Day.
func (d Day) IsZero() bool { return d == Day{} }

// utc is midnight UTC on d, which has no DST, so differences are whole days.
func (d Day) utc() time.Time { return time.Date(d.Y, d.M, d.D, 0, 0, 0, 0, time.UTC) }

// AddDays returns the date n days after d; n may be negative.
func (d Day) AddDays(n int) Day { return Of(time.Date(d.Y, d.M, d.D+n, 0, 0, 0, 0, time.UTC)) }

// DaysUntil is the number of days from d to e: negative when e is earlier.
func (d Day) DaysUntil(e Day) int { return int(e.utc().Sub(d.utc()) / (24 * time.Hour)) }

// Weekday is d's day of the week.
func (d Day) Weekday() time.Weekday { return d.utc().Weekday() }

// Monday is the Monday on or before d.
func (d Day) Monday() Day { return d.AddDays(-((int(d.Weekday()) + 6) % 7)) }

// Compare returns -1, 0, or +1 as d is before, equal to, or after e.
func (d Day) Compare(e Day) int { return d.utc().Compare(e.utc()) }

// Before reports whether d is earlier than e.
func (d Day) Before(e Day) bool { return d.Compare(e) < 0 }

// After reports whether d is later than e.
func (d Day) After(e Day) bool { return d.Compare(e) > 0 }

// Min is the earlier of d and e.
func Min(d, e Day) Day {
	if e.Before(d) {
		return e
	}
	return d
}

// Midnight is the instant d begins on the wall clock in loc. On a day whose
// midnight a DST change skips, it is the first instant of the day.
func (d Day) Midnight(loc *time.Location) time.Time { return time.Date(d.Y, d.M, d.D, 0, 0, 0, 0, loc) }

// At is the instant at which the wall clock in loc reads minute minutes
// after midnight on d. Minutes past 1440 run into the following days.
func (d Day) At(minute int, loc *time.Location) time.Time {
	return time.Date(d.Y, d.M, d.D, 0, minute, 0, 0, loc)
}
