package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// A day's hours sync like any other synced row, and a pulled change marks the
// day's plan as changed.
func TestDayHoursSync(t *testing.T) {
	t.Parallel()
	src := newFx(t)
	src.clk.Advance(time.Minute)
	_, _, err := src.r.Plans.SetHours(src.ctx, "2026-09-21", testutil.Ptr(19*60), testutil.Ptr(180), src.env())
	require.NoError(t, err)

	changed, err := src.r.Sync.Changed(src.ctx, time.UnixMilli(0))
	require.NoError(t, err)
	rows := store.Rows{}
	for _, tr := range changed {
		rows[tr.Table] = append(rows[tr.Table], tr.Row)
	}
	require.Len(t, rows["day_hours"], 1)
	b, err := json.Marshal(rows)
	require.NoError(t, err)
	decoded, err := store.DecodeRows(b)
	require.NoError(t, err)

	dst := newFx(t)
	res, err := dst.r.Sync.Apply(dst.ctx, decoded, false)
	require.NoError(t, err)
	_, planDays, err := dst.r.Sync.Days(dst.ctx, res)
	require.NoError(t, err)
	require.Equal(t, []string{"2026-09-21"}, planDays)
	p, err := dst.r.Plans.Read(dst.ctx, "2026-09-21", dst.env())
	require.NoError(t, err)
	require.Equal(t, 19*60, *p.Hours.StartMinute)
	require.Equal(t, 3*time.Hour, *p.Hours.Work)
	require.Equal(t, 180, p.Capacity)

	// Clearing them is a soft delete, which syncs too.
	src.clk.Advance(time.Minute)
	_, _, err = src.r.Plans.SetHours(src.ctx, "2026-09-21", nil, nil, src.env())
	require.NoError(t, err)
	later, err := src.r.Sync.Changed(src.ctx, src.clk.Now().Add(-30*time.Second))
	require.NoError(t, err)
	require.Len(t, later, 1)
	require.NotNil(t, later[0].Row["deleted_at"])
}
