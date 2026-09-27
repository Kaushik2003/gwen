package planner_test

import (
	"testing"

	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/stretchr/testify/require"
)

func day(s string) civil.Day { return civil.MustParse(s) }

func days(ds []civil.Day) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, d.String())
	}
	return out
}

func TestOccurrences(t *testing.T) {
	t.Parallel()
	// 2026-09-15 is a Tuesday.
	tests := []struct {
		name     string
		rule     string
		anchor   string
		from, to string
		want     []string
	}{
		{"daily", "FREQ=DAILY", "2026-09-15", "2026-09-14", "2026-09-17",
			[]string{"2026-09-15", "2026-09-16", "2026-09-17"}},
		{"daily with interval", "FREQ=DAILY;INTERVAL=3", "2026-09-15", "2026-09-16", "2026-09-25",
			[]string{"2026-09-18", "2026-09-21", "2026-09-24"}},
		{"weekly defaults to the anchor's weekday", "FREQ=WEEKLY", "2026-09-15", "2026-09-15", "2026-09-30",
			[]string{"2026-09-15", "2026-09-22", "2026-09-29"}},
		{"weekly by day", "INTERVAL=1;BYDAY=MO,WE,FR;FREQ=WEEKLY", "2026-09-15", "2026-09-14", "2026-09-21",
			[]string{"2026-09-16", "2026-09-18", "2026-09-21"}},
		{"weekly by day with interval counts weeks from the anchor's Monday", "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,SU",
			"2026-09-15", "2026-09-14", "2026-10-05",
			[]string{"2026-09-20", "2026-09-28", "2026-10-04"}},
		{"monthly", "FREQ=MONTHLY;BYMONTHDAY=15", "2026-09-15", "2026-09-01", "2026-12-31",
			[]string{"2026-09-15", "2026-10-15", "2026-11-15", "2026-12-15"}},
		{"monthly with interval", "FREQ=MONTHLY;BYMONTHDAY=1;INTERVAL=2", "2026-09-15", "2026-09-01", "2027-03-01",
			[]string{"2026-11-01", "2027-01-01", "2027-03-01"}},
		{"until is inclusive", "FREQ=DAILY;UNTIL=20260917", "2026-09-15", "2026-09-01", "2026-09-30",
			[]string{"2026-09-15", "2026-09-16", "2026-09-17"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := planner.Occurrences(tc.rule, day(tc.anchor), day(tc.from), day(tc.to))
			require.NoError(t, err)
			require.Equal(t, tc.want, days(got))
		})
	}
}

func TestParseRuleRejects(t *testing.T) {
	t.Parallel()
	for _, rule := range []string{
		"",
		"FREQ=YEARLY",
		"INTERVAL=2",
		"FREQ=DAILY;COUNT=5",
		"FREQ=MONTHLY;BYMONTHDAY=15;BYSETPOS=1",
		"FREQ=DAILY;UNTIL=20261215T000000Z",
		"FREQ=DAILY;UNTIL=20260230",
		"FREQ=MONTHLY;BYMONTHDAY=29",
		"FREQ=MONTHLY;BYMONTHDAY=0",
		"FREQ=MONTHLY",
		"FREQ=DAILY;BYDAY=MO",
		"FREQ=WEEKLY;BYMONTHDAY=3",
		"FREQ=WEEKLY;BYDAY=MON",
		"FREQ=WEEKLY;BYDAY=",
		"FREQ=DAILY;INTERVAL=0",
		"FREQ=DAILY;INTERVAL=-1",
		"FREQ=DAILY;INTERVAL=x",
		"FREQ=DAILY;FREQ=WEEKLY",
		"FREQ=DAILY;",
		"RRULE:FREQ=DAILY",
		"freq=daily",
	} {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()
			_, err := planner.ParseRule(rule)
			require.ErrorIs(t, err, planner.ErrRule)
			_, err = planner.Occurrences(rule, day("2026-09-15"), day("2026-09-15"), day("2026-09-20"))
			require.ErrorIs(t, err, planner.ErrRule)
		})
	}
}

func TestOccurrenceDue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, rule, anchor, on, want string
	}{
		{"daily is due the same day", "FREQ=DAILY", "2026-09-15", "2026-09-16", "2026-09-16"},
		{"weekly is due before next week's", "FREQ=WEEKLY", "2026-09-15", "2026-09-22", "2026-09-28"},
		{"weekdays: Friday's runs to Sunday", "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "2026-09-14", "2026-09-18", "2026-09-20"},
		{"the last is due on until", "FREQ=WEEKLY;UNTIL=20260925", "2026-09-15", "2026-09-22", "2026-09-25"},
		{"monthly with interval", "FREQ=MONTHLY;BYMONTHDAY=15;INTERVAL=3", "2026-09-15", "2026-09-15", "2026-12-14"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, err := planner.ParseRule(tc.rule)
			require.NoError(t, err)
			require.True(t, r.Matches(day(tc.anchor), day(tc.on)))
			require.Equal(t, tc.want, r.OccurrenceDue(day(tc.anchor), day(tc.on)).String())
		})
	}
}

func TestMatchesBeforeAnchor(t *testing.T) {
	t.Parallel()
	r, err := planner.ParseRule("FREQ=DAILY")
	require.NoError(t, err)
	require.False(t, r.Matches(day("2026-09-15"), day("2026-09-14")))
	next, ok := r.Next(day("2026-09-15"), day("2026-09-01"))
	require.True(t, ok)
	require.Equal(t, "2026-09-15", next.String())
}
