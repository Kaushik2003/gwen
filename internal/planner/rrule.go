package planner

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/planner/civil"
)

// ErrRule is matched by every error ParseRule returns; the API answers it
// with invalid_request naming the rrule field.
var ErrRule = errors.New("invalid rrule")

// Recurrence frequencies (docs/06-planner.md#recurrence).
const (
	Daily   = "DAILY"
	Weekly  = "WEEKLY"
	Monthly = "MONTHLY"
)

// Rule is a parsed recurrence: the strict RRULE subset of
// docs/06-planner.md#recurrence.
type Rule struct {
	Freq       string
	Interval   int
	ByDay      []time.Weekday // WEEKLY only; empty means the anchor's weekday
	ByMonthDay int            // MONTHLY only, 1–28
	Until      *civil.Day     // inclusive
}

var weekdays = map[string]time.Weekday{
	"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
}

func ruleErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRule, fmt.Sprintf(format, args...))
}

// ParseRule parses s, rejecting anything outside the subset.
func ParseRule(s string) (Rule, error) {
	r := Rule{Interval: 1}
	seen := map[string]bool{}
	for part := range strings.SplitSeq(s, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" {
			return Rule{}, ruleErr("%q is not KEY=VALUE", part)
		}
		if seen[key] {
			return Rule{}, ruleErr("%s appears twice", key)
		}
		seen[key] = true
		switch key {
		case "FREQ":
			if value != Daily && value != Weekly && value != Monthly {
				return Rule{}, ruleErr("FREQ must be DAILY, WEEKLY, or MONTHLY")
			}
			r.Freq = value
		case "INTERVAL":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || strings.HasPrefix(value, "+") {
				return Rule{}, ruleErr("INTERVAL must be a positive integer")
			}
			r.Interval = n
		case "BYDAY":
			days := map[time.Weekday]bool{}
			for name := range strings.SplitSeq(value, ",") {
				wd, ok := weekdays[name]
				if !ok {
					return Rule{}, ruleErr("BYDAY takes MO TU WE TH FR SA SU, not %q", name)
				}
				if !days[wd] {
					days[wd] = true
					r.ByDay = append(r.ByDay, wd)
				}
			}
		case "BYMONTHDAY":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 28 || strings.HasPrefix(value, "+") {
				return Rule{}, ruleErr("BYMONTHDAY must be 1 to 28")
			}
			r.ByMonthDay = n
		case "UNTIL":
			if len(value) != 8 {
				return Rule{}, ruleErr("UNTIL must be a date like 20261215")
			}
			d, err := civil.Parse(value[:4] + "-" + value[4:6] + "-" + value[6:])
			if err != nil {
				return Rule{}, ruleErr("UNTIL must be a date like 20261215")
			}
			r.Until = &d
		default:
			return Rule{}, ruleErr("%s is not supported", key)
		}
	}
	switch {
	case r.Freq == "":
		return Rule{}, ruleErr("FREQ is required")
	case len(r.ByDay) > 0 && r.Freq != Weekly:
		return Rule{}, ruleErr("BYDAY is allowed only with FREQ=WEEKLY")
	case r.ByMonthDay != 0 && r.Freq != Monthly:
		return Rule{}, ruleErr("BYMONTHDAY is allowed only with FREQ=MONTHLY")
	case r.Freq == Monthly && r.ByMonthDay == 0:
		return Rule{}, ruleErr("FREQ=MONTHLY needs BYMONTHDAY")
	}
	return r, nil
}

// Matches reports whether d is an occurrence of r anchored at anchor.
func (r Rule) Matches(anchor, d civil.Day) bool {
	if d.Before(anchor) || (r.Until != nil && d.After(*r.Until)) {
		return false
	}
	switch r.Freq {
	case Daily:
		return anchor.DaysUntil(d)%r.Interval == 0
	case Weekly:
		if !r.onWeekday(anchor, d.Weekday()) {
			return false
		}
		weeks := anchor.Monday().DaysUntil(d.Monday()) / 7
		return weeks%r.Interval == 0
	case Monthly:
		months := (d.Y-anchor.Y)*12 + int(d.M-anchor.M)
		return d.D == r.ByMonthDay && months%r.Interval == 0
	}
	return false
}

func (r Rule) onWeekday(anchor civil.Day, wd time.Weekday) bool {
	if len(r.ByDay) == 0 {
		return wd == anchor.Weekday()
	}
	return slices.Contains(r.ByDay, wd)
}

// Next is the first occurrence strictly after d, and false when UNTIL leaves
// none.
func (r Rule) Next(anchor, d civil.Day) (civil.Day, bool) {
	from := d.AddDays(1)
	if from.Before(anchor) {
		from = anchor
	}
	// The longest gap between occurrences is under Interval+1 periods of the
	// frequency, and a month is at most 31 days.
	span := 0
	switch r.Freq {
	case Daily:
		span = r.Interval
	case Weekly:
		span = 7 * (r.Interval + 1)
	case Monthly:
		span = 31 * (r.Interval + 1)
	}
	for i := 0; i <= span; i++ {
		c := from.AddDays(i)
		if r.Until != nil && c.After(*r.Until) {
			return civil.Day{}, false
		}
		if r.Matches(anchor, c) {
			return c, true
		}
	}
	return civil.Day{}, false
}

// Occurrences returns the days in [from, to] matching rule anchored at
// anchor, ascending.
func Occurrences(rule string, anchor, from, to civil.Day) ([]civil.Day, error) {
	r, err := ParseRule(rule)
	if err != nil {
		return nil, err
	}
	return r.Between(anchor, from, to), nil
}

// Between returns the days in [from, to] matching r, ascending.
func (r Rule) Between(anchor, from, to civil.Day) []civil.Day {
	var out []civil.Day
	if from.Before(anchor) {
		from = anchor
	}
	for d := from; !d.After(to); d = d.AddDays(1) {
		if r.Until != nil && d.After(*r.Until) {
			break
		}
		if r.Matches(anchor, d) {
			out = append(out, d)
		}
	}
	return out
}

// OccurrenceDue is the due day of the occurrence on d: the day before the
// next occurrence, capped at UNTIL. The last occurrence is due on UNTIL.
func (r Rule) OccurrenceDue(anchor, d civil.Day) civil.Day {
	next, ok := r.Next(anchor, d)
	if !ok {
		return *r.Until // only UNTIL ends a rule
	}
	due := next.AddDays(-1)
	if r.Until != nil {
		due = civil.Min(due, *r.Until)
	}
	return due
}
