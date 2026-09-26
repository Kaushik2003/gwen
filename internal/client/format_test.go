package client_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/stretchr/testify/require"
)

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{-5 * time.Second, "0s"},
		{30 * time.Second, "30s"},
		{59*time.Second + 999*time.Millisecond, "59s"},
		{time.Minute, "1m"},
		{45*time.Minute + 59*time.Second, "45m"},
		{time.Hour, "1h"},
		{8 * time.Hour, "8h"},
		{time.Hour + 5*time.Minute, "1h 5m"},
		{7*time.Hour + 32*time.Minute + 10*time.Second, "7h 32m"},
		{26 * time.Hour, "26h"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, client.FormatDuration(tc.d))
		})
	}
	require.Equal(t, "3h 12m", client.FormatMillis((3*60+12)*60*1000))
}

func TestFormatTimeAndDay(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 15, 7, 5, 59, 0, time.UTC)
	require.Equal(t, "07:05", client.FormatTime(at))
	require.Equal(t, "2026-09-15", client.FormatDay(at))
	require.Equal(t, "19:30", client.FormatTime(time.Date(2026, 9, 15, 19, 30, 0, 0, time.UTC)))
}

func TestShortID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "2c3d4e5f", client.ShortID("01926d2e-7a4b-7c3d-8e9f-0a1b2c3d4e5f"))
	require.Equal(t, "abc", client.ShortID("abc"))
}

func TestResolveID(t *testing.T) {
	t.Parallel()
	a := "01926d2e-7a4b-7c3d-8e9f-00002c3d4e5f"
	b := "01926d2e-7a4b-7c3d-8e9f-00012c3d4e5f"
	c := "01926d2e-7a4b-7c3d-8e9f-ffffffffffff"
	ids := []string{a, b, c}
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "full id", input: a, want: a},
		{name: "full id in upper case", input: "01926D2E-7A4B-7C3D-8E9F-FFFFFFFFFFFF", want: c},
		{name: "unique eight-character suffix", input: "ffffffff", want: c},
		{name: "unique longer suffix", input: "02c3d4e5f", want: a},
		{name: "suffix shared by two ids", input: "2c3d4e5f", wantErr: client.ErrAmbiguousID},
		{name: "shorter than eight", input: "c3d4e5f", wantErr: client.ErrUnknownID},
		{name: "no match", input: "deadbeef", wantErr: client.ErrUnknownID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := client.ResolveID(tc.input, ids)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
