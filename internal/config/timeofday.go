package config

import (
	"fmt"
	"strconv"
	"time"
)

// TimeOfDay is a local wall-clock time, written "HH:MM" and held as minutes
// after local midnight (docs/03-data-model.md#time-representation).
type TimeOfDay int

// ParseTimeOfDay parses a strict 24-hour "HH:MM".
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	if len(s) != 5 || s[2] != ':' || !digits(s[:2]) || !digits(s[3:]) {
		return 0, fmt.Errorf("%q is not a time of day in HH:MM form", s)
	}
	h, _ := strconv.Atoi(s[:2]) // cannot fail: two ASCII digits
	m, _ := strconv.Atoi(s[3:])
	if h > 23 || m > 59 {
		return 0, fmt.Errorf("%q is not a time of day in HH:MM form", s)
	}
	return TimeOfDay(h*60 + m), nil
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Hour returns the hour, 0–23.
func (t TimeOfDay) Hour() int { return int(t) / 60 }

// Minute returns the minute within the hour, 0–59.
func (t TimeOfDay) Minute() int { return int(t) % 60 }

// String returns "HH:MM".
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute()) }

// On returns the instant at which the wall clock in loc reads t on the date
// y-m-d. Dates out of range are normalized, so d+1 is the next day.
func (t TimeOfDay) On(y int, m time.Month, d int, loc *time.Location) time.Time {
	return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, loc)
}
