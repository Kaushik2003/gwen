package recall

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/memory"
)

// Month and weekday names as a time phrase may spell them.
var (
	months = map[string]time.Month{
		"january": time.January, "jan": time.January, "february": time.February, "feb": time.February,
		"march": time.March, "mar": time.March, "april": time.April, "apr": time.April, "may": time.May,
		"june": time.June, "jun": time.June, "july": time.July, "jul": time.July, "august": time.August,
		"aug": time.August, "september": time.September, "sept": time.September, "sep": time.September,
		"october": time.October, "oct": time.October, "november": time.November, "nov": time.November,
		"december": time.December, "dec": time.December,
	}
	weekdays = map[string]time.Weekday{
		"monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday, "sunday": time.Sunday,
	}
	numbers = map[string]int{
		"a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7,
		"eight": 8, "nine": 9, "ten": 10,
	}
)

const (
	monthRE = `(january|february|march|april|may|june|july|august|september|october|november|december|` +
		`jan|feb|mar|apr|jun|jul|aug|sept|sep|oct|nov|dec)`
	weekdayRE = `(monday|tuesday|wednesday|thursday|friday|saturday|sunday)`
	countRE   = `(\d{1,3}|an?|one|two|three|four|five|six|seven|eight|nine|ten)`
)

// The time phrases ParseWindow reads, in the order it tries them.
var (
	isoDateRE     = regexp.MustCompile(`\bon\s+(\d{4})-(\d{2})-(\d{2})\b`)
	dayMonthRE    = regexp.MustCompile(`\bon\s+(?:the\s+)?(\d{1,2})(?:st|nd|rd|th)?(?:\s+of)?\s+` + monthRE + `\b`)
	monthDayRE    = regexp.MustCompile(`\bon\s+` + monthRE + `\.?\s+(\d{1,2})(?:st|nd|rd|th)?\b`)
	daysAgoRE     = regexp.MustCompile(`\b` + countRE + `\s+days?\s+ago\b`)
	weeksAgoRE    = regexp.MustCompile(`\b` + countRE + `\s+weeks?\s+ago\b`)
	lastDaysRE    = regexp.MustCompile(`\b(?:last|past)\s+` + countRE + `\s+days\b`)
	yesterdayRE   = regexp.MustCompile(`\byesterday\b`)
	todayRE       = regexp.MustCompile(`\btoday\b`)
	weekRE        = regexp.MustCompile(`\b(this|last)\s+week\b`)
	monthPhraseRE = regexp.MustCompile(`\b(this|last)\s+month\b`)
	weekdayPRE    = regexp.MustCompile(`\b(on|last)\s+` + weekdayRE + `\b`)
	inMonthRE     = regexp.MustCompile(`\bin\s+` + monthRE + `\b`)
)

// ParseWindow reads a time phrase in text: today, yesterday, this/last week,
// this/last month, last N days, N days/weeks ago, (on|last) <weekday>, in
// <month>, on <D Mon>/<Mon D>/<YYYY-MM-DD>. Days are loc's; weeks start on
// Monday; a date or month without a year is the latest one not after today.
// It returns nil when text names no time.
func ParseWindow(text string, now time.Time, loc *time.Location) *memory.TimeRange {
	if loc == nil {
		loc = time.Local
	}
	text = strings.ToLower(text)
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	day := func(t time.Time) *memory.TimeRange { return &memory.TimeRange{From: t, To: t.AddDate(0, 0, 1)} }
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)

	if m := isoDateRE.FindStringSubmatch(text); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		if t, ok := date(y, time.Month(mo), d, loc); ok {
			return day(t)
		}
	}
	if m := dayMonthRE.FindStringSubmatch(text); m != nil {
		d, _ := strconv.Atoi(m[1])
		if t, ok := latestDate(months[m[2]], d, today); ok {
			return day(t)
		}
	}
	if m := monthDayRE.FindStringSubmatch(text); m != nil {
		d, _ := strconv.Atoi(m[2])
		if t, ok := latestDate(months[m[1]], d, today); ok {
			return day(t)
		}
	}
	if m := daysAgoRE.FindStringSubmatch(text); m != nil {
		return day(today.AddDate(0, 0, -count(m[1])))
	}
	if m := weeksAgoRE.FindStringSubmatch(text); m != nil {
		from := monday.AddDate(0, 0, -7*count(m[1]))
		return &memory.TimeRange{From: from, To: from.AddDate(0, 0, 7)}
	}
	if m := lastDaysRE.FindStringSubmatch(text); m != nil {
		return &memory.TimeRange{From: today.AddDate(0, 0, -count(m[1])), To: today.AddDate(0, 0, 1)}
	}
	if yesterdayRE.MatchString(text) {
		return day(today.AddDate(0, 0, -1))
	}
	if todayRE.MatchString(text) {
		return day(today)
	}
	if m := weekRE.FindStringSubmatch(text); m != nil {
		from := monday
		if m[1] == "last" {
			from = monday.AddDate(0, 0, -7)
		}
		return &memory.TimeRange{From: from, To: from.AddDate(0, 0, 7)}
	}
	if m := monthPhraseRE.FindStringSubmatch(text); m != nil {
		from := firstOfMonth
		if m[1] == "last" {
			from = firstOfMonth.AddDate(0, -1, 0)
		}
		return &memory.TimeRange{From: from, To: from.AddDate(0, 1, 0)}
	}
	if m := weekdayPRE.FindStringSubmatch(text); m != nil {
		// "on Tuesday" may be today; "last Tuesday" is before it.
		back := (int(today.Weekday()) - int(weekdays[m[2]]) + 7) % 7
		if back == 0 && m[1] == "last" {
			back = 7
		}
		return day(today.AddDate(0, 0, -back))
	}
	if m := inMonthRE.FindStringSubmatch(text); m != nil {
		mo := months[m[1]]
		y := now.Year()
		if mo > now.Month() {
			y--
		}
		from := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		return &memory.TimeRange{From: from, To: from.AddDate(0, 1, 0)}
	}
	return nil
}

// count reads a number of a time phrase.
func count(s string) int {
	if n, ok := numbers[s]; ok {
		return n
	}
	n, _ := strconv.Atoi(s)
	return n
}

// date is y-m-d in loc; ok is false when there is no such day.
func date(y int, m time.Month, d int, loc *time.Location) (time.Time, bool) {
	t := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return t, t.Year() == y && t.Month() == m && t.Day() == d
}

// latestDate is the latest m-d not after today.
func latestDate(m time.Month, d int, today time.Time) (time.Time, bool) {
	t, ok := date(today.Year(), m, d, today.Location())
	if !ok || t.After(today) {
		t, ok = date(today.Year()-1, m, d, today.Location())
	}
	return t, ok
}
