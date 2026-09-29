package testutil_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAt(t *testing.T) {
	t.Parallel()
	require.Equal(t, testutil.T0, testutil.At("09:00"))
	require.Equal(t, time.Date(2026, 9, 15, 12, 0, 30, 0, time.UTC), testutil.At("12:00:30"))
	require.Equal(t, time.Date(2026, 9, 15, 9, 3, 0, 500_000_000, time.UTC), testutil.At("09:03:00.500"))
	require.Equal(t, time.Date(2026, 9, 16, 4, 0, 0, 0, time.UTC), testutil.AtOn("2026-09-16", "04:00"))

	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), testutil.AtIn(ny, "2026-11-01", "04:00").UTC())
	require.Panics(t, func() { testutil.At("9") })
}

func TestNewDB(t *testing.T) {
	t.Parallel()
	db := testutil.NewDB(t)
	require.Equal(t, int64(2), db.SchemaVersion())
	require.Equal(t, testutil.T0, db.Now())
	require.Len(t, db.DeviceID(), 36)
}

func TestSocketPathIsShort(t *testing.T) {
	t.Parallel()
	require.Less(t, len(testutil.SocketPath(t)), 108)
	require.Equal(t, "x", *testutil.Ptr("x"))
}
