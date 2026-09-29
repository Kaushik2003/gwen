package civil_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"valid", "2026-09-15", true},
		{"leap day", "2028-02-29", true},
		{"not a leap year", "2026-02-29", false},
		{"no padding", "2026-9-15", false},
		{"time", "2026-09-15T10:00", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := civil.Parse(tc.in)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.in, d.String())
		})
	}
	require.Panics(t, func() { civil.MustParse("nope") })
}

func TestArithmetic(t *testing.T) {
	t.Parallel()
	d := civil.MustParse("2026-09-15") // a Tuesday
	require.Equal(t, "2026-10-01", d.AddDays(16).String())
	require.Equal(t, "2026-08-31", d.AddDays(-15).String())
	require.Equal(t, 91, d.DaysUntil(civil.MustParse("2026-12-15")))
	require.Equal(t, -1, d.DaysUntil(civil.MustParse("2026-09-14")))
	require.Equal(t, time.Tuesday, d.Weekday())
	require.Equal(t, "2026-09-14", d.Monday().String())
	require.Equal(t, "2026-09-14", civil.MustParse("2026-09-14").Monday().String())
	require.Equal(t, "2026-09-14", civil.MustParse("2026-09-20").Monday().String(), "Sunday belongs to the week before")
	require.True(t, d.Before(d.AddDays(1)))
	require.True(t, d.After(d.AddDays(-1)))
	require.Equal(t, d, civil.Min(d, d.AddDays(3)))
	require.True(t, civil.Day{}.IsZero())
}

func TestDaysAcrossDST(t *testing.T) {
	t.Parallel()
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	// 2026-11-01 is 25 hours long in New York; day counts must not notice.
	d := civil.MustParse("2026-10-31")
	require.Equal(t, 2, d.DaysUntil(civil.MustParse("2026-11-02")))
	nov1 := civil.MustParse("2026-11-01")
	require.Equal(t, 25*time.Hour, nov1.AddDays(1).Midnight(ny).Sub(nov1.Midnight(ny)))
	require.Equal(t, time.Date(2026, 11, 1, 23, 0, 0, 0, ny), nov1.At(23*60, ny))
	require.Equal(t, civil.MustParse("2026-11-02"), civil.Of(nov1.At(24*60+30, ny)))
}
