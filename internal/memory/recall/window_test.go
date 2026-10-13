package recall

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/stretchr/testify/require"
)

func TestParseWindow(t *testing.T) {
	t.Parallel()
	ist := time.FixedZone("IST", 5*3600+1800)
	// Tuesday 13 Oct 2026, 15:00 in India; 09:30 UTC.
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, ist)
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, ist) }
	span := func(from, to time.Time) *memory.TimeRange { return &memory.TimeRange{From: from, To: to} }
	oneDay := func(y int, m time.Month, day int) *memory.TimeRange {
		return span(d(y, m, day), d(y, m, day+1))
	}

	for _, tc := range []struct {
		text string
		want *memory.TimeRange
	}{
		{"what did we do today?", oneDay(2026, 10, 13)},
		{"Anything from yesterday", oneDay(2026, 10, 12)},
		{"what did I say this week", span(d(2026, 10, 12), d(2026, 10, 19))},
		{"last week we talked", span(d(2026, 10, 5), d(2026, 10, 12))},
		{"this month", span(d(2026, 10, 1), d(2026, 11, 1))},
		{"what happened last month", span(d(2026, 9, 1), d(2026, 10, 1))},
		{"in the last 3 days", span(d(2026, 10, 10), d(2026, 10, 14))},
		{"over the past seven days", span(d(2026, 10, 6), d(2026, 10, 14))},
		{"3 days ago", oneDay(2026, 10, 10)},
		{"a day ago", oneDay(2026, 10, 12)},
		{"two weeks ago", span(d(2026, 9, 28), d(2026, 10, 5))},
		{"1 week ago", span(d(2026, 10, 5), d(2026, 10, 12))},
		{"what did we decide on Tuesday", oneDay(2026, 10, 13)},
		{"last tuesday", oneDay(2026, 10, 6)},
		{"on Monday", oneDay(2026, 10, 12)},
		{"last Sunday", oneDay(2026, 10, 11)},
		{"on wednesday", oneDay(2026, 10, 7)},
		{"in March", span(d(2026, 3, 1), d(2026, 4, 1))},
		{"in December", span(d(2025, 12, 1), d(2026, 1, 1))},
		{"in October", span(d(2026, 10, 1), d(2026, 11, 1))},
		{"on 3 Oct", oneDay(2026, 10, 3)},
		{"on the 3rd of October", oneDay(2026, 10, 3)},
		{"on Oct 20", oneDay(2025, 10, 20)},
		{"on September 5th", oneDay(2026, 9, 5)},
		{"on 2026-02-28", oneDay(2026, 2, 28)},
		{"on 2026-02-30", nil},
		{"Gym on Tuesdays", nil},
		{"I live in Lisbon", nil},
		{"what's my name", nil},
		{"", nil},
	} {
		require.Equal(t, tc.want, ParseWindow(tc.text, now, ist), tc.text)
	}

	// Days are the zone's: at 23:00 UTC on the 12th it is already the 13th in
	// India.
	late := time.Date(2026, 10, 12, 23, 0, 0, 0, time.UTC)
	require.Equal(t, oneDay(2026, 10, 13), ParseWindow("today", late, ist))
	require.Equal(t, &memory.TimeRange{From: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		To: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)}, ParseWindow("today", late, time.UTC))
}

func TestRouterWindowRollover(t *testing.T) {
	t.Parallel()
	// At 01:30 the user's day (rolling over at 04:00) is still the 12th.
	now := time.Date(2026, 10, 13, 1, 30, 0, 0, time.UTC)
	r := New(Options{Loc: time.UTC, Day: func(t time.Time) string { return t.Add(-4 * time.Hour).Format(time.DateOnly) }})
	w := r.window("what did we talk about today", now)
	require.Equal(t, &memory.TimeRange{From: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		To: now.Add(time.Millisecond)}, w, "today runs from the 12th until now")
	w = r.window("yesterday", now)
	require.Equal(t, &memory.TimeRange{From: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
		To: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}, w)
	require.Nil(t, r.window("hello", now))

	noon := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	require.Equal(t, &memory.TimeRange{From: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
		To: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}, r.window("today", noon))
}

func TestIntent(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]string{
		"Where do I work?":                    memory.IntentLookup,
		"where do i live these days":          memory.IntentLookup,
		"What's my sister's name":             memory.IntentLookup,
		"what is my blood type":               memory.IntentLookup,
		"When is Anna's birthday?":            memory.IntentLookup,
		"who is Marco":                        memory.IntentLookup,
		"do you remember where I parked":      memory.IntentLookup,
		"Do you know my dentist?":             memory.IntentLookup,
		"what did we decide about the trip":   memory.IntentEpisodic,
		"What did I say about Lisbon":         memory.IntentEpisodic,
		"remember when we went hiking":        memory.IntentEpisodic,
		"same as last time":                   memory.IntentEpisodic,
		"we talked about this":                memory.IntentEpisodic,
		"we discussed a plan":                 memory.IntentEpisodic,
		"anything from yesterday?":            memory.IntentEpisodic,
		"what does the lease say about pets":  memory.IntentResource,
		"check the contract":                  memory.IntentResource,
		"in the notes I gave you":             memory.IntentResource,
		"do i like jazz":                      memory.IntentPrefer,
		"pick my favourite food":              memory.IntentPrefer,
		"my favorite band":                    memory.IntentPrefer,
		"which do I prefer":                   memory.IntentPrefer,
		"how long did I work last week":       memory.IntentData,
		"how many hours did I study":          memory.IntentData,
		"how much did i work on Tuesday":      memory.IntentData,
		"show my tracked time":                memory.IntentData,
		"hours this week":                     memory.IntentData,
		"hello there":                         memory.IntentAny,
		"I moved to Porto":                    memory.IntentAny,
		"Gym on Tuesdays is hard":             memory.IntentAny,
		"tell me a joke about the filesystem": memory.IntentAny,
	} {
		w := ParseWindow(text, fxNow, time.UTC)
		require.Equal(t, want, intentOf(text, w), text)
	}
}

func TestWeights(t *testing.T) {
	t.Parallel()
	require.Equal(t, 2.0, weight(memory.IntentLookup, memory.LaneClaims))
	require.Equal(t, 2.0, weight(memory.IntentEpisodic, memory.LaneTime))
	require.Equal(t, 2.0, weight(memory.IntentResource, memory.LaneSupermemory))
	require.Equal(t, 1.5, weight(memory.IntentPrefer, memory.LaneVec))
	require.Equal(t, 3.0, weight(memory.IntentData, memory.LaneData))
	require.Equal(t, 0.0, weight(memory.IntentData, memory.LaneSupermemory))
	require.Equal(t, 0.8, weight(memory.IntentAny, memory.LaneMem0))
	require.Equal(t, 1.0, weight(memory.IntentAny, "other"), "a lane the table does not name weighs 1")
	require.Equal(t, 1.2, weight("unknown", memory.LaneClaims), "an unknown intent reads as any")
	for intent, row := range weights {
		require.Len(t, row, 9, intent)
	}
}

func TestWords(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{"dentist", "appointment"},
		words("What did we say about the dentist appointments yesterday?"))
	require.Empty(t, words("what did we talk about last week"))
	require.Equal(t, 2, overlap(words("dentist appointment"), "Booked the dentist for an appointment"))
	require.True(t, mentions("When is Anna's birthday?", "anna"))
	require.True(t, mentions("ask Mary Jane", "mary jane"))
	require.False(t, mentions("Annabel is here", "Anna"))
	require.False(t, mentions("anything", ""))
}
